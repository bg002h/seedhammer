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
	ErrComposeBadThreshold           = errors.New("md: compose: threshold needs 1 <= k <= n <= 9")
	ErrComposeTooManySlots           = errors.New("md: compose: this wallet would have more key slots than the wire holds (32)")
	ErrComposeLegacyWrapperShape     = errors.New("md: compose: sh and sh-wsh admit exactly one sortedmulti path")
	ErrComposeLockOutOfRange         = errors.New("md: compose: lock operand outside §4c")
	ErrComposeWrongSlotCount         = errors.New("md: compose: declarations given for a different number of slots than the policy has")
	ErrComposeIndistinguishableSlots = errors.New("md: compose: two slots declare the same origin without two distinct fingerprints; a template like that cannot be restored")
)

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

// SpendPath is one alternative way to spend: optional keys, optional sha256
// preimage, optional lock. A path with neither keys nor a hash is refused.
type SpendPath struct {
	Keys *KeySet
	Hash *[32]byte
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
	for i := 0; i < n; i++ {
		x, ok := pubkeys[uint8(i)]
		if !ok {
			return fmt.Errorf("md: compose: Bind has no key for slot @%d", i)
		}
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
	for i, p := range list.Paths {
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
		parts = append(parts, node{tag: tagSha256, body: hash256Body(*h)})
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
