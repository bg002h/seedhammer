package md

// The wallet-policy COMPOSER's tree builder (SPEC_wallet_policy_composer.md
// §5, FIXED lowering) — a line-for-line port of the Rust primary's
// md-codec::compose::{lowering,tr} at descriptor-mnemonic 66bdf2f4. Rust is
// normative: every branch here has a vendored vector or a chunk-set literal
// in compose_test.go that the primary produced, and a divergence is fixed
// HERE, never by editing a vector (CLAUDE.md, Rust-primary rule).
//
// What it does, in order: validate the path list (the primary's validate()),
// number slots by first appearance in the EMITTED text (the taproot internal
// key first, then listed order), lower each path to its node, chain the paths
// (`or_d` under a bare-multi head, `or_i` otherwise; a right-leaning taptree
// spine), resolve slot origins (§4f: declared, else the wrapper's default at
// the lowest account no other slot holds; two slots may share an origin only
// with two distinct fingerprints), and assemble the descriptor the rest of
// this package already knows how to split, identify and emit.
//
// It emits no text. A rendering that cannot be re-parsed is the defect this
// package's invariant exists to prevent; the GUI shows the STRUCTURE
// (PolicyShape) and the ids, and the md1 chunks are the artifact.

import (
	"bytes"
	"errors"
	"fmt"
)

// ComposeWrapper is the outermost script form (§4a).
type ComposeWrapper uint8

const (
	ComposeTr ComposeWrapper = iota
	ComposeWsh
	ComposeShWsh
	ComposeSh
)

// ScriptType is BIP-48's script-type component for the wrapper's default
// origins (§4f): 2 for wsh and sh, 1 for sh-wsh, 3 for tr. It is the same
// table gui/multisig_build_slots.go's multisigScriptTypeComponent applies to
// Multisig Build's three wrappers, extended by the taproot arm (§9 item 8).
func (w ComposeWrapper) ScriptType() uint32 {
	switch w {
	case ComposeShWsh:
		return 1
	case ComposeTr:
		return 3
	default:
		return 2
	}
}

func (w ComposeWrapper) isLegacy() bool { return w == ComposeSh || w == ComposeShWsh }

// LockKind is the operator's lock unit (§4c).
type LockKind uint8

const (
	// LockOlderBlocks — older(n), n blocks, 1..=65535.
	LockOlderBlocks LockKind = iota
	// LockOlderUnits — older(0x400000 + u), u units of 512 seconds, 1..=65535.
	LockOlderUnits
	// LockAfterHeight — after(h), a block height, 1..=499,999,999.
	LockAfterHeight
	// LockAfterTime — after(t), a Unix time, 500,000,000..=2,147,483,647.
	LockAfterTime
)

// Lock is one timelock in the operator's units.
type Lock struct {
	Kind  LockKind
	Value uint32
}

// Limits, the primary's compose::{MAX_PATHS, MAX_KEYS_PER_PATH, MAX_SLOTS}.
const (
	ComposeMaxPaths       = 8
	ComposeMaxKeysPerPath = 9
	ComposeMaxSlots       = 32

	sequenceTypeFlag    uint32 = 1 << 22
	locktimeThreshold   uint32 = 500_000_000
	maxAbsoluteLocktime uint32 = 0x7fff_ffff
)

// The refusals, one sentinel per arm of the primary's ComposeError so callers
// (and tests) match with errors.Is; the wrapped message carries the operands,
// with paths numbered from 1 as the primary's Display does (and as §7d's
// "Path N" prompts count).
var (
	ErrComposeNoPaths                = errors.New("md: compose: a wallet needs at least one spend path")
	ErrComposeTooManyPaths           = errors.New("md: compose: more than 8 spend paths")
	ErrComposeNoKeyedPath            = errors.New("md: compose: every path is key-less; at least one path must hold a key")
	ErrComposeLockOnlyPath           = errors.New("md: compose: a path with neither keys nor a hash is not a spend path")
	ErrComposeKeylessUnderTr         = errors.New("md: compose: a key-less path is not expressible under tr")
	ErrComposeTwoKeylessPaths        = errors.New("md: compose: a policy admits at most one key-less path; two of them lower to a malleable or_i")
	ErrComposeRepeatedKeyMaterial    = errors.New("md: compose: the same extended key is bound at two slots; BIP 388 requires the keys to be pairwise distinct")
	ErrComposeBadThreshold           = errors.New("md: compose: threshold needs 1 <= k <= n <= 9")
	ErrComposeTooManySlots           = errors.New("md: compose: this wallet would have more key slots than the wire holds (32)")
	ErrComposeLegacyWrapperShape     = errors.New("md: compose: sh and sh-wsh admit exactly one sortedmulti path")
	ErrComposeLockOutOfRange         = errors.New("md: compose: lock operand outside §4c")
	ErrComposeWrongSlotCount         = errors.New("md: compose: declarations given for a different number of slots than the policy has")
	ErrComposeIndistinguishableSlots = errors.New("md: compose: two slots declare the same origin without two distinct fingerprints; a template like that cannot be restored")
)

// RepeatedKeyMaterialError names the two slots Bind found bound to the same
// key, so a caller can say WHICH -- an operator repairs "slots @1 and @2",
// not "two slots". It carries ErrComposeRepeatedKeyMaterial for errors.Is.
type RepeatedKeyMaterialError struct{ A, B uint8 }

func (e RepeatedKeyMaterialError) Error() string {
	return fmt.Sprintf("%v: slots @%d and @%d", ErrComposeRepeatedKeyMaterial, e.A, e.B)
}

func (e RepeatedKeyMaterialError) Unwrap() error { return ErrComposeRepeatedKeyMaterial }

// operand is the tag and consensus operand this lock encodes to (§4c).
func (l Lock) operand() (tag, uint32, error) {
	switch l.Kind {
	case LockOlderBlocks:
		if l.Value == 0 || l.Value > 0xffff {
			return 0, 0, errors.New("older in blocks needs 1..=65535")
		}
		return tagOlder, l.Value, nil
	case LockOlderUnits:
		if l.Value == 0 || l.Value > 0xffff {
			return 0, 0, errors.New("older in 512-second units needs 1..=65535")
		}
		return tagOlder, sequenceTypeFlag + l.Value, nil
	case LockAfterHeight:
		if l.Value == 0 || l.Value >= locktimeThreshold {
			return 0, 0, errors.New("after height needs 1..=499999999")
		}
		return tagAfter, l.Value, nil
	case LockAfterTime:
		if l.Value < locktimeThreshold || l.Value > maxAbsoluteLocktime {
			return 0, 0, errors.New("after time needs 500000000..=2147483647")
		}
		return tagAfter, l.Value, nil
	}
	return 0, 0, errors.New("unknown lock kind")
}

// Check is the DEVICE-SIDE §4c range gate (§12 item 7): a unit gate on the
// emitter's input, independent of what md's decoder would accept.
func (l Lock) Check() error {
	_, _, err := l.operand()
	return err
}

// lockFromWire is operand's inverse: the kind and operator-unit value a
// decoded older/after node denotes. older carries bit 22 for 512-second
// units; after is a time at or above 500,000,000 and a height below it
// (BIP-68 / BIP-65, the same split §4c's bands are built on).
func lockFromWire(t tag, operand uint32) Lock {
	if t == tagOlder {
		if operand&sequenceTypeFlag != 0 {
			return Lock{Kind: LockOlderUnits, Value: operand &^ sequenceTypeFlag}
		}
		return Lock{Kind: LockOlderBlocks, Value: operand}
	}
	if operand >= locktimeThreshold {
		return Lock{Kind: LockAfterTime, Value: operand}
	}
	return Lock{Kind: LockAfterHeight, Value: operand}
}

// KeySet is k-of-n over FRESH slots (§4b). Sorted asks for sortedmulti /
// sortedmulti_a where the position allows it; false asks for multi / multi_a
// there, which is EXPERIMENTAL.
type KeySet struct {
	K, N   uint8
	Sorted bool
}

// HashKind is which hash the SCRIPT commits to (SPEC_hashlock_kinds §5).
//
// Deliberately NOT the wire tag set: a hashlock field typed as a tag can hold
// tagWpkh. tag() is the total function into it.
//
// NOT the same axis as a phrase record's METHOD, which is how a preimage was
// derived. They share the token "sha256" and mean different things.
type HashKind uint8

const (
	// KindSha256 is sha256(X) -- the bare form on the wire.
	KindSha256 HashKind = iota
	// KindHash256 is hash256(X) = sha256(sha256(X)).
	KindHash256
	// KindRipemd160 is ripemd160(X), the bare primitive.
	KindRipemd160
	// KindHash160 is hash160(X) = ripemd160(sha256(X)).
	KindHash160
)

// DigestLen is THE ONLY PLACE A DIGEST LENGTH IS WRITTEN (spec §5). The hex
// rule is DigestLen()*2, which is what makes the parser, the validator and the
// formatter agree by construction rather than by review.
func (k HashKind) DigestLen() int {
	switch k {
	case KindSha256, KindHash256:
		return 32
	case KindRipemd160, KindHash160:
		return 20
	}
	// NOT `default: return 32`. Go does not check switch exhaustiveness, so a
	// fifth kind added without touching this function would silently be read
	// at sha256's width -- 32 bytes of a 20-byte digest, or twelve bytes of
	// alloc-gate padding committed into a script. An unknown kind is a
	// PROGRAMMING error and never operator input: the only constructor is
	// sysw's hashKindFromToken, which returns a known kind or refuses. So this
	// fails loudly rather than guessing.
	panic("md: unknown HashKind -- a kind was added without updating DigestLen")
}

// Token is the lowercase miniscript fragment name, which is also the §6 record
// token and md compose's option name.
func (k HashKind) Token() string {
	switch k {
	case KindSha256:
		return "sha256"
	case KindHash256:
		return "hash256"
	case KindRipemd160:
		return "ripemd160"
	case KindHash160:
		return "hash160"
	}
	panic("md: unknown HashKind -- a kind was added without updating Token")
}

// tag is the wire tag. Total by construction -- every kind has exactly one.
func (k HashKind) tag() tag {
	switch k {
	case KindSha256:
		return tagSha256
	case KindHash256:
		return tagHash256
	case KindRipemd160:
		return tagRipemd160
	case KindHash160:
		return tagHash160
	}
	// A silent tagSha256 here is the worst of the four: the wallet lowers to a
	// hash fragment the operator's preimage does not satisfy.
	panic("md: unknown HashKind -- a kind was added without updating tag")
}

// HashKindFromToken maps a §6 record token to a kind. CASE IS REJECTED, NEVER
// FOLDED -- no strings.ToLower here, by rule, because that is how two parsers
// come to disagree about what a record means.
//
// THE ONLY MAPPING IN THE TREE. sysw had its own and two tests were about to
// grow theirs; three copies of a rule whose whole point is that no two readers
// of a record disagree is the defect the rule exists to prevent.
func HashKindFromToken(t string) (HashKind, bool) {
	switch t {
	case "sha256":
		return KindSha256, true
	case "hash256":
		return KindHash256, true
	case "ripemd160":
		return KindRipemd160, true
	case "hash160":
		return KindHash160, true
	}
	return 0, false
}

// HashLock is a hashlock: which hash, and the digest it commits to (spec §5).
//
// THE DIGEST IS A FIXED [32]byte FOR THE ALLOC GATE, so a 20-byte kind carries
// twelve bytes of zero padding. Digest() hides that padding and is the only
// correct way to read it.
//
// == ON THIS TYPE IS A COMPILE ERROR, ON PURPOSE. Rust hit exactly this trap in
// phase 1: deriving equality over the fixed array made the padding OBSERVABLE,
// so two ripemd160 locks with identical 20-byte digests differing in the
// padding compared unequal and hashed differently. Rust could fix it by
// hand-writing the impls; Go's == on a struct holding a [32]byte compares all
// 32 bytes and CANNOT be overridden. So the zero-size func field below makes
// the struct non-comparable: a caller who writes a == b, or uses a HashLock as
// a map key, gets a compile error instead of a silent wrong answer. Use Equal
// and MapKey. Spec §5 puts eleven map/set/equality sites on this type.
type HashLock struct {
	_      [0]func()
	kind   HashKind
	digest [32]byte
}

// NewHashLock builds one, refusing a digest that is not this kind's width.
// A wrong-width digest is an error at the boundary, never truncated or padded.
func NewHashLock(kind HashKind, digest []byte) (*HashLock, bool) {
	if len(digest) != kind.DigestLen() {
		return nil, false
	}
	h := &HashLock{kind: kind}
	copy(h.digest[:], digest)
	return h, true
}

// Kind is which hash the script commits to.
func (h *HashLock) Kind() HashKind { return h.kind }

// Digest is the digest AT ITS KIND'S WIDTH -- 20 bytes or 32, never the
// alloc-gate padding.
func (h *HashLock) Digest() []byte { return h.digest[:h.kind.DigestLen()] }

// Equal compares two locks by kind and visible digest. Use this, not ==, which
// does not compile on this type and would compare the padding if it did.
func (h *HashLock) Equal(o *HashLock) bool {
	if h == nil || o == nil {
		return h == o
	}
	return h.kind == o.kind && bytes.Equal(h.Digest(), o.Digest())
}

// MapKey is a stable key for map and set use, since the struct itself cannot be
// one. It carries the kind so that the same bytes under different kinds are
// different keys -- §5's point that the kind is part of identity.
func (h *HashLock) MapKey() string {
	return h.kind.Token() + ":" + string(h.Digest())
}

// SpendPath is one alternative way to spend: optional keys, optional hashlock,
// optional lock. A path with neither keys nor a hash is refused.
type SpendPath struct {
	Keys *KeySet
	Hash *HashLock
	Lock *Lock
}

func (p SpendPath) isBareMulti() bool {
	return p.Keys != nil && p.Keys.N >= 2 && p.Hash == nil && p.Lock == nil
}

func (p SpendPath) isBareSingle() bool {
	return p.Keys != nil && p.Keys.N == 1 && p.Hash == nil && p.Lock == nil
}

// PathList is the operator's ordered list under one wrapper.
type PathList struct {
	Wrapper ComposeWrapper
	Paths   []SpendPath
}

// SlotOrigin is one slot's declared origin (and optional fingerprint); a nil
// *SlotOrigin in ComposeWith means "unseated: take the §4f default".
type SlotOrigin struct {
	Origin      []PathComponent
	Fingerprint [4]byte
	FpPresent   bool
}

// ComposeSlot maps an emitted slot index to the path and ordinal it came from.
type ComposeSlot struct {
	Index   uint8
	Path    int
	Ordinal uint8
}

// ComposeExperimentalKind marks a shape the primary admits only under
// --experimental (§5; the GUI shows the §8 warning for each).
type ComposeExperimentalKind uint8

const (
	ExperimentalKeylessPath ComposeExperimentalKind = iota
	ExperimentalUnsortedKeys
)

// ComposeExperimental is one mark: the kind and the path it is about.
type ComposeExperimental struct {
	Kind ComposeExperimentalKind
	Path int
}

// Composed is a built, not-yet-keyed (or keyed via Bind) descriptor with its
// slot map. A copy of a Composed shares the underlying descriptor: Bind on one
// keys them both (it is not copy-on-write). Compose again for a second artifact.
type Composed struct {
	d               *descriptor
	slots           []ComposeSlot
	internalKeyPath int // -1 when the internal key is NUMS
	experimental    []ComposeExperimental
}

// Slots is the emitted slot map, index-ascending.
func (c Composed) Slots() []ComposeSlot { return c.slots }

// InternalKeyPath is the path extracted as the taproot internal key, if any.
func (c Composed) InternalKeyPath() (int, bool) {
	return c.internalKeyPath, c.internalKeyPath >= 0
}

// Experimental lists the §5 experimental marks, path-ascending.
func (c Composed) Experimental() []ComposeExperimental { return c.experimental }

// Chunks emits the md1 chunk set (always chunk form, as the primary's
// force_chunked vectors are).
func (c Composed) Chunks() ([]string, error) { return split(c.d) }

// Stub is the form-aware 4-byte stub a key card carries for this artifact.
func (c Composed) Stub() ([4]byte, error) { return FormAwareStub(c.d) }

// TemplateID is the key-independent wallet descriptor template id.
func (c Composed) TemplateID() ([16]byte, error) { return WalletDescriptorTemplateId(c.d) }

// Bind attaches a 65-byte chaincode‖compressed-pubkey per slot (every slot
// required) and optional fingerprints (added to, or replacing, the ones the
// declarations carried), producing the KEYED form. It is what Rust's MANIFEST
// binding did to make the keyed_compose_* vectors.
func (c *Composed) Bind(pubkeys map[uint8][65]byte, fingerprints map[uint8][4]byte) error {
	n := int(c.d.n)
	if len(pubkeys) != n {
		return fmt.Errorf("md: compose: Bind needs a key for each of %d slots, got %d", n, len(pubkeys))
	}
	pubs := make([]idxPub, n)
	// BIP 388's pairwise-distinct rule, applied to the BYTES (fable review r0
	// I-2). The primary refuses the same extended key at two positions --
	// `md decompose` prints BIP 388's own sentence for it and calls the wallet
	// UNSUPPORTED, never invalid -- and this port had no counterpart, so the
	// device could MINT a policy the primary would not decompose. Measured on
	// md 0.16.2 with the lens-1 descriptor: the device emitted six keyed
	// chunks and `md decompose` of the descriptor they decode to reported
	// "the same extended key is used at 2 positions".
	//
	// THE COMPARISON IS THE 65 BYTES, NOT THE XPUB STRING. depth and parent
	// fingerprint are header fields, so one key has many serialisations and a
	// string comparison misses the re-serialised copy `md descriptor` itself
	// prints. Measured against the oracle: `md decompose` refuses the repeat
	// whether the two positions share a use-site (`/<0;1>/*` twice) or not
	// (`/<0;1>/*` and `/<2;3>/*`), so the material alone decides.
	//
	// Convergence, not a lead (CLAUDE.md, Rust-primary rule): the rule is
	// already correct in Rust and this brings the Go port to it.
	seen := map[[65]byte]int{}
	for i := 0; i < n; i++ {
		x, ok := pubkeys[uint8(i)]
		if !ok {
			return fmt.Errorf("md: compose: Bind has no key for slot @%d", i)
		}
		if j, dup := seen[x]; dup {
			return RepeatedKeyMaterialError{A: uint8(j), B: uint8(i)}
		}
		seen[x] = i
		pubs[i] = idxPub{idx: uint8(i), xpub: x}
	}
	c.d.tlv.pubkeys = pubs
	c.d.tlv.pubPresent = true
	if len(fingerprints) > 0 {
		merged := map[uint8][4]byte{}
		for _, f := range c.d.tlv.fingerprints {
			merged[f.idx] = f.fp
		}
		for idx, fp := range fingerprints {
			if int(idx) >= n {
				return fmt.Errorf("md: compose: Bind fingerprint for slot @%d beyond %d slots", idx, n)
			}
			merged[idx] = fp
		}
		fps := make([]idxFP, 0, len(merged))
		for i := 0; i < n; i++ {
			if fp, ok := merged[uint8(i)]; ok {
				fps = append(fps, idxFP{idx: uint8(i), fp: fp})
			}
		}
		c.d.tlv.fingerprints = fps
		c.d.tlv.fpPresent = len(fps) > 0
	}
	return nil
}

// DefaultOrigin is §4f's m/48'/0'/<account>'/<script-type>'.
func DefaultOrigin(w ComposeWrapper, account uint32) []PathComponent {
	return []PathComponent{
		{Hardened: true, Value: 48},
		{Hardened: true, Value: 0},
		{Hardened: true, Value: account},
		{Hardened: true, Value: w.ScriptType()},
	}
}

// ValidatePathList is the primary's validate(): the slot count on success.
func ValidatePathList(list PathList) (int, error) {
	if len(list.Paths) == 0 {
		return 0, ErrComposeNoPaths
	}
	if len(list.Paths) > ComposeMaxPaths {
		return 0, fmt.Errorf("%w: got %d", ErrComposeTooManyPaths, len(list.Paths))
	}
	slots := 0
	anyKeyed := false
	keyless := make([]int, 0, len(list.Paths))
	for i, p := range list.Paths {
		if p.Keys == nil {
			keyless = append(keyless, i+1)
		}
		if ks := p.Keys; ks != nil {
			if ks.K == 0 || ks.N == 0 || ks.K > ks.N || ks.N > ComposeMaxKeysPerPath {
				return 0, fmt.Errorf("%w: path %d has %d-of-%d", ErrComposeBadThreshold, i+1, ks.K, ks.N)
			}
			slots += int(ks.N)
			anyKeyed = true
		} else if p.Hash == nil {
			return 0, fmt.Errorf("%w: path %d", ErrComposeLockOnlyPath, i+1)
		} else if list.Wrapper == ComposeTr {
			return 0, fmt.Errorf("%w: path %d", ErrComposeKeylessUnderTr, i+1)
		}
		if p.Lock != nil {
			if err := p.Lock.Check(); err != nil {
				return 0, fmt.Errorf("%w: path %d: %v", ErrComposeLockOutOfRange, i+1, err)
			}
		}
	}
	if !anyKeyed {
		return 0, ErrComposeNoKeyedPath
	}
	// AT MOST ONE KEY-LESS PATH, wherever it sits and whatever lock it carries.
	//
	// §5 chains paths as or_i(P, R) unless the head is a bare multi, and
	// or_i(X, Z) is non-malleable only when one arm is `safe` -- rust-miniscript
	// malleability.rs and Core's miniscript.h both require X.s || Z.s. A
	// key-less path needs no signature, so it is never safe; with two of them
	// anywhere in the list, the innermost or_i containing both has two unsafe
	// arms and the whole script is malleable. A timelock does NOT rescue it:
	// `older` is not a signature.
	//
	// The Rust primary refuses the same set, and refuses it structurally
	// rather than by case: `md compose` re-parses its own lowering through
	// `md encode`, which reports "Miniscript is malleable". Measured against
	// md 0.16.2 over twelve lists (gui/composer_fable_r0_funds_test.go, which
	// also runs the CLI as an oracle): every list with >= 2 key-less paths is
	// refused -- [keyed,K,K], [keyed,K,K+older(5)], [K,K+older(5),keyed],
	// [keyed,K+older(5),K+after(200)], [K+older(5),keyed,K+after(200)],
	// [keyed,2of2,K,K], [keyed,K,2of2,K] -- and every list with at most one is
	// admitted.
	//
	// THIS PORT HAS NO POST-LOWERING PARSE to catch it the primary's way: this
	// package "emits no text" by design (the header above), so the primary's
	// re-parse has no counterpart here and the rule is stated instead. Rust
	// remains normative (CLAUDE.md, Rust-primary rule); md-codec's validate()
	// is taking the same rule, and this is the port of it.
	if len(keyless) > 1 {
		return 0, fmt.Errorf("%w: paths %d and %d", ErrComposeTwoKeylessPaths, keyless[0], keyless[1])
	}
	if slots > ComposeMaxSlots {
		return 0, fmt.Errorf("%w: got %d", ErrComposeTooManySlots, slots)
	}
	if list.Wrapper.isLegacy() {
		sole := len(list.Paths) == 1 && list.Paths[0].isBareMulti()
		sorted := list.Paths[0].Keys != nil && list.Paths[0].Keys.Sorted
		if !(sole && sorted) {
			return 0, ErrComposeLegacyWrapperShape
		}
	}
	return slots, nil
}

// Compose lowers an all-unseated list: every slot takes its §4f default.
func Compose(list PathList) (Composed, error) {
	slots, err := ValidatePathList(list)
	if err != nil {
		return Composed{}, err
	}
	return lowerPathList(list, make([]*SlotOrigin, slots))
}

// ComposeWith lowers a list whose slots may carry declared origins (one entry
// per emitted slot, index-ascending; nil = unseated).
func ComposeWith(list PathList, declared []*SlotOrigin) (Composed, error) {
	slots, err := ValidatePathList(list)
	if err != nil {
		return Composed{}, err
	}
	if len(declared) != slots {
		return Composed{}, fmt.Errorf("%w: %d given, policy has %d", ErrComposeWrongSlotCount, len(declared), slots)
	}
	return lowerPathList(list, declared)
}

// ─── lowering (the primary's lowering.rs) ─────────────────────────────────────

type numberedPath struct {
	path      SpendPath
	pathIndex int
	slots     []uint8
}

func keyLeaf(single, multi, sorted tag, ks KeySet, slots []uint8, sortedLegal bool) node {
	if ks.N == 1 {
		return node{tag: single, body: keyArgBody{index: slots[0]}}
	}
	t := multi
	if sortedLegal && ks.Sorted {
		t = sorted
	}
	idx := make([]uint8, len(slots))
	copy(idx, slots)
	return node{tag: t, body: multiKeysBody{k: ks.K, indices: idx}}
}

func verifyNode(x node) node { return node{tag: tagVerify, body: childrenBody{children: []node{x}}} }

func andV(a, b node) node {
	return node{tag: tagAndV, body: childrenBody{children: []node{verifyNode(a), b}}}
}

// pathBody lowers one path: KEYS, then sha256, then the lock, right-nested as
// and_v(v:KEYS, and_v(v:sha256(H), LOCK)).
func pathBody(p numberedPath, tap, sortedLegal bool) node {
	var parts []node
	if ks := p.path.Keys; ks != nil {
		if tap {
			parts = append(parts, keyLeaf(tagPkK, tagMultiA, tagSortedMultiA, *ks, p.slots, sortedLegal))
		} else {
			parts = append(parts, keyLeaf(tagPkH, tagMulti, tagSortedMulti, *ks, p.slots, sortedLegal))
		}
	}
	if h := p.path.Hash; h != nil {
		// THE BODY'S WIDTH IS THE KIND'S, never 32 by default. The digest is
		// stored in a fixed [32]byte for the alloc gate, so writing the whole
		// array here would commit twelve bytes of padding into the script --
		// which compiles, round-trips, and cannot be spent. Switched on the
		// KIND so a fifth kind is a compile error rather than a fall-through.
		var b body
		switch h.Kind() {
		case KindSha256, KindHash256:
			var d hash256Body
			copy(d[:], h.Digest())
			b = d
		case KindRipemd160, KindHash160:
			var d hash160Body
			copy(d[:], h.Digest())
			b = d
		}
		parts = append(parts, node{tag: h.Kind().tag(), body: b})
	}
	if l := p.path.Lock; l != nil {
		t, v, err := l.operand()
		if err != nil {
			panic("md: compose: lock validated by ValidatePathList: " + err.Error())
		}
		parts = append(parts, node{tag: t, body: timelockBody(v)})
	}
	acc := parts[len(parts)-1]
	for i := len(parts) - 2; i >= 0; i-- {
		acc = andV(parts[i], acc)
	}
	return acc
}

// wshChain chains the paths: or_d when the head is a bare multi (its
// satisfaction is a clean DUP-IF-able boolean), or_i otherwise.
func wshChain(paths []numberedPath) node {
	sole := len(paths) == 1
	nodes := make([]node, len(paths))
	for i, p := range paths {
		nodes[i] = pathBody(p, false, sole && p.path.isBareMulti())
	}
	acc := nodes[len(nodes)-1]
	for i := len(paths) - 2; i >= 0; i-- {
		t := tagOrI
		if paths[i].path.isBareMulti() {
			t = tagOrD
		}
		acc = node{tag: t, body: childrenBody{children: []node{nodes[i], acc}}}
	}
	return acc
}

// numberSlots assigns slot indices by first appearance: `first` (the taproot
// internal key's path, or -1) before listed order.
func numberSlots(list PathList, first int) ([]numberedPath, []ComposeSlot) {
	order := make([]int, 0, len(list.Paths))
	if first >= 0 {
		order = append(order, first)
	}
	for i := range list.Paths {
		if i != first {
			order = append(order, i)
		}
	}
	var next uint8
	var slots []ComposeSlot
	byPath := make([]numberedPath, len(list.Paths))
	for _, pi := range order {
		p := list.Paths[pi]
		var mine []uint8
		if p.Keys != nil {
			for ord := uint8(0); ord < p.Keys.N; ord++ {
				slots = append(slots, ComposeSlot{Index: next, Path: pi, Ordinal: ord})
				mine = append(mine, next)
				next++
			}
		}
		byPath[pi] = numberedPath{path: p, pathIndex: pi, slots: mine}
	}
	return byPath, slots
}

func experimentalMarks(list PathList, soleSortedLegal func(int) bool) []ComposeExperimental {
	var out []ComposeExperimental
	for i, p := range list.Paths {
		switch {
		case p.Keys == nil:
			out = append(out, ComposeExperimental{Kind: ExperimentalKeylessPath, Path: i})
		case p.Keys.N >= 2 && !p.Keys.Sorted && soleSortedLegal(i):
			out = append(out, ComposeExperimental{Kind: ExperimentalUnsortedKeys, Path: i})
		}
	}
	return out
}

func sameOrigin(a, b []pathComponent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func originTaken(taken [][]pathComponent, o []pathComponent) bool {
	for _, t := range taken {
		if sameOrigin(t, o) {
			return true
		}
	}
	return false
}

// resolveOrigins is §4f: declared origins stand; every unseated slot takes the
// wrapper's default at the lowest account no other slot (declared or filled
// earlier) holds; then the pairwise invariant.
func resolveOrigins(list PathList, declared []*SlotOrigin) (pathDecl, []idxFP, error) {
	n := len(declared)
	origins := make([][]pathComponent, n)
	fps := make([]*[4]byte, n)
	var taken [][]pathComponent
	for i, s := range declared {
		if s != nil {
			origins[i] = toComponents(s.Origin)
			taken = append(taken, origins[i])
			if s.FpPresent {
				fp := s.Fingerprint
				fps[i] = &fp
			}
		}
	}
	for i, s := range declared {
		if s != nil {
			continue
		}
		for account := uint32(0); ; account++ {
			candidate := toComponents(DefaultOrigin(list.Wrapper, account))
			if !originTaken(taken, candidate) {
				taken = append(taken, candidate)
				origins[i] = candidate
				break
			}
		}
	}
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			if sameOrigin(origins[a], origins[b]) {
				distinct := fps[a] != nil && fps[b] != nil && *fps[a] != *fps[b]
				if !distinct {
					return pathDecl{}, nil, fmt.Errorf("%w: slots @%d and @%d", ErrComposeIndistinguishableSlots, a, b)
				}
			}
		}
	}
	allSame := true
	for i := 1; i < n; i++ {
		if !sameOrigin(origins[0], origins[i]) {
			allSame = false
			break
		}
	}
	var pd pathDecl
	if allSame {
		shared := originPath{components: origins[0]}
		pd = pathDecl{n: uint8(n), shared: &shared}
	} else {
		div := make([]originPath, n)
		for i := range origins {
			div[i] = originPath{components: origins[i]}
		}
		pd = pathDecl{n: uint8(n), divergent: div}
	}
	var out []idxFP
	for i, fp := range fps {
		if fp != nil {
			out = append(out, idxFP{idx: uint8(i), fp: *fp})
		}
	}
	return pd, out, nil
}

func finishComposed(list PathList, declared []*SlotOrigin, tree node, slots []ComposeSlot, ik int, exp []ComposeExperimental) (Composed, error) {
	pd, fps, err := resolveOrigins(list, declared)
	if err != nil {
		return Composed{}, err
	}
	d := &descriptor{
		n:        uint8(len(declared)),
		pathDecl: pd,
		useSite: useSitePath{
			hasMultipath:     true,
			multipath:        []alternative{{hardened: false, value: 0}, {hardened: false, value: 1}},
			wildcardHardened: false,
		},
		tree: tree,
		tlv: tlvSection{
			fpPresent:    len(fps) > 0,
			fingerprints: fps,
		},
	}
	return Composed{d: d, slots: slots, internalKeyPath: ik, experimental: exp}, nil
}

func lowerPathList(list PathList, declared []*SlotOrigin) (Composed, error) {
	if list.Wrapper == ComposeTr {
		return lowerTr(list, declared)
	}
	numbered, slots := numberSlots(list, -1)
	sole := len(list.Paths) == 1
	inner := wshChain(numbered)
	var tree node
	switch list.Wrapper {
	case ComposeSh:
		tree = node{tag: tagSh, body: childrenBody{children: []node{inner}}}
	case ComposeShWsh:
		tree = node{tag: tagSh, body: childrenBody{children: []node{{tag: tagWsh, body: childrenBody{children: []node{inner}}}}}}
	default:
		tree = node{tag: tagWsh, body: childrenBody{children: []node{inner}}}
	}
	exp := experimentalMarks(list, func(i int) bool { return sole && list.Paths[i].isBareMulti() })
	return finishComposed(list, declared, tree, slots, -1, exp)
}

// ─── taproot (the primary's tr.rs) ────────────────────────────────────────────

// lowerTr extracts the FIRST-LISTED unlocked, unhashed single key as the
// internal key (else NUMS); the remaining paths become leaves on a
// right-leaning spine (depth of leaf j is min(j, m-1)).
func lowerTr(list PathList, declared []*SlotOrigin) (Composed, error) {
	ik := -1
	for i, p := range list.Paths {
		if p.isBareSingle() {
			ik = i
			break
		}
	}
	numbered, slots := numberSlots(list, ik)
	var leafPaths []numberedPath
	for _, n := range numbered {
		if n.pathIndex != ik {
			leafPaths = append(leafPaths, n)
		}
	}
	m := len(leafPaths)
	leaves := make([]node, m)
	for i, n := range leafPaths {
		leaves[i] = pathBody(n, true, m == 1 && n.path.isBareMulti())
	}
	var spine *node
	if m > 0 {
		acc := leaves[m-1]
		for i := m - 2; i >= 0; i-- {
			acc = node{tag: tagTapTree, body: childrenBody{children: []node{leaves[i], acc}}}
		}
		spine = &acc
	}
	tree := node{tag: tagTr, body: trBody{isNums: ik < 0, keyIndex: 0, tree: spine}}
	exp := experimentalMarks(list, func(i int) bool { return m == 1 && i != ik && list.Paths[i].isBareMulti() })
	return finishComposed(list, declared, tree, slots, ik, exp)
}
