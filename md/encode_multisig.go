package md

import (
	"errors"
	"fmt"
)

// ─── EncodeMultisig (T6c Phase A) — byte-faithful sortedmulti WALLET-POLICY md1 ─
//
// EncodeMultisig builds a wallet-policy *descriptor for a sortedmulti k-of-n
// multisig under one of three top-level wrappers (wsh / sh(wsh) / sh) and emits
// the CHUNKED md1 strings via the shipped split. It mirrors EncodeSingleSig: the
// caller supplies parsed PUBLIC key material (no secret bytes), and the wire +
// identity core (writeNode/canonicalize/WalletPolicyId) is reused UNCHANGED.
//
// ORDERING CONTRACT (load-bearing — read before calling): EncodeMultisig is
// EXACTLY order-preserving. Cosigners[i] is assigned placeholder @i; there is NO
// hidden key sort (canonicalize is the identity permutation for this AST). Two
// callers supplying the same N keys in DIFFERENT orders mint DIFFERENT, both
// valid, md1 cards with DIFFERENT WalletPolicyId — only the order matching the
// coordinator's policy binds. The caller (Phase B) owns coordinator-matching
// order. To let a caller verify ordering BEFORE engraving to steel, EncodeMultisig
// returns the assigned per-slot @N→fingerprint map and the 4-byte
// WalletPolicyIDStub (== WalletPolicyIDStubChunks(out)).

// MultisigScript selects the top-level wrapper over sortedmulti.
type MultisigScript int

const (
	MultisigWsh   MultisigScript = iota // wsh(sortedmulti(k,...))      → P2WSH
	MultisigShWsh                       // sh(wsh(sortedmulti(k,...)))  → P2SH-P2WSH
	MultisigSh                          // sh(sortedmulti(k,...))        → legacy P2SH
)

// OriginMode picks the BIP-32 origin declaration: a single shared origin for all
// cosigners (path_decl.Shared) or per-cosigner divergent origins
// (path_decl.Divergent, len == n). It is explicit so a nil/empty origin is never
// silently overloaded as the shared/divergent discriminant (R0 recommendation).
type OriginMode int

const (
	OriginShared    OriginMode = iota // all cosigners share SharedOrigin
	OriginDivergent                   // each cosigner uses its own Cosigner.Origin
)

// MultisigCosigner is one parsed PUBLIC cosigner key. ChainCode‖CompressedPubkey
// form the 65-byte Pubkeys TLV entry. Fingerprint is emitted only if FpPresent
// (the T6b card is fp-ABSENT, so an always-fp encoder would not byte-match it).
// Origin is the RAW BIP-32 origin used in OriginDivergent mode (ignored in
// OriginShared mode); RAW = Hardened flag + bare value, the PathComponent form.
type MultisigCosigner struct {
	ChainCode        [32]byte
	CompressedPubkey [33]byte
	Fingerprint      [4]byte
	FpPresent        bool
	Origin           []PathComponent
}

// EncodeMultisigRequest is the EncodeMultisig parameter struct. K is the
// threshold; n is len(Cosigners). The cosigner ORDER fixes @0..@{n-1}.
type EncodeMultisigRequest struct {
	Cosigners    []MultisigCosigner
	K            uint8
	Script       MultisigScript
	OriginMode   OriginMode
	SharedOrigin []PathComponent // used iff OriginMode == OriginShared
}

// SlotInfo is one entry of the ordering-verification handle returned by
// EncodeMultisig: it records which placeholder index a cosigner was assigned and
// that cosigner's fingerprint (so a caller can match @N against a coordinator).
type SlotInfo struct {
	Index       uint8
	Fingerprint [4]byte
	FpPresent   bool
}

var (
	errMultisigEmptySharedOrigin = errors.New("md: EncodeMultisig OriginShared requires a non-empty SharedOrigin")
	errMultisigEmptyDivergent    = errors.New("md: EncodeMultisig OriginDivergent requires a non-empty Origin for every cosigner")
	errMultisigBadScript         = errors.New("md: EncodeMultisig unknown script kind")
	errMultisigBadOriginMode     = errors.New("md: unknown multisig origin mode")

	// ErrOriginKeyContradiction — two cosigners declare the SAME master
	// fingerprint and the SAME origin path while carrying DIFFERENT keys.
	//
	// BIP-32 is deterministic: that pair identifies exactly ONE extended key, so
	// such a card describes a wallet that cannot exist. Provable from the
	// request alone — no seed, no network, no derivation.
	//
	// CONVERGENCE PORT of `md_codec::validate::validate_origin_key_consistency`
	// (descriptor-mnemonic fe4b1ec9), which found the shape in NINE of nine
	// multi-key conformance vectors. The port needed no change to AGREE with
	// Rust on those vectors — wire bytes, ids and addresses all moved together
	// — which is exactly why it needs this check rather than inheriting one:
	// addresses derive from the keys a card CARRIES, never from the origin it
	// declares, so nothing downstream in either language can see it.
	//
	// Distinct from the build flow's duplicate-key refusal
	// (`errBuildDuplicateKey`): that is the SAME key in two slots, this is
	// DIFFERENT keys claiming one origin. Opposite defects, opposite remedies.
	ErrOriginKeyContradiction = errors.New("md: two cosigners declare the same key origin but different keys; one origin identifies exactly one key")
)

// EncodeMultisig assembles a sortedmulti k-of-n wallet-policy md1 over the given
// cosigners in CALLER ORDER (which fixes @0..@{n-1}; see the ordering contract on
// the package doc above). It returns the chunked md1 strings (>=2), the 4-byte
// WalletPolicyIDStub, and the per-slot @N→fingerprint map (SlotInfo), plus an
// error. It refuses unsupported shapes/params via typed errors (k/n bounds and
// k<=n are enforced by the shipped split pipeline; this function adds the
// origin-mode and script-kind guards).
func EncodeMultisig(req EncodeMultisigRequest) (out []string, stub [4]byte, slots []SlotInfo, err error) {
	n := len(req.Cosigners)

	if err := checkOriginKeyConsistency(req); err != nil {
		return nil, [4]byte{}, nil, err
	}

	// Build the path declaration per the EXPLICIT origin mode.
	var pd pathDecl
	switch req.OriginMode {
	case OriginShared:
		if len(req.SharedOrigin) == 0 {
			return nil, [4]byte{}, nil, errMultisigEmptySharedOrigin
		}
		so := originPath{components: toComponents(req.SharedOrigin)}
		pd = pathDecl{n: uint8(n), shared: &so}
	case OriginDivergent:
		paths := make([]originPath, n)
		for i, c := range req.Cosigners {
			if len(c.Origin) == 0 {
				return nil, [4]byte{}, nil, errMultisigEmptyDivergent
			}
			paths[i] = originPath{components: toComponents(c.Origin)}
		}
		pd = pathDecl{n: uint8(n), divergent: paths}
	default:
		return nil, [4]byte{}, nil, errMultisigBadOriginMode
	}

	// The multisig tree per wrapper (sortedmulti{k, [0..n-1]} in cosigner order).
	tree, terr := multiSigTree(req.Script, req.K, n)
	if terr != nil {
		return nil, [4]byte{}, nil, terr
	}

	// N pubkey TLV entries (idx-ascending, cosigner order) + optional per-cosigner
	// fingerprint entries (only the present subset, idx-ascending).
	pubkeys := make([]idxPub, n)
	var fps []idxFP
	slots = make([]SlotInfo, n)
	for i, c := range req.Cosigners {
		var xpub [65]byte
		copy(xpub[:32], c.ChainCode[:])
		copy(xpub[32:], c.CompressedPubkey[:])
		pubkeys[i] = idxPub{idx: uint8(i), xpub: xpub}
		if c.FpPresent {
			fps = append(fps, idxFP{idx: uint8(i), fp: c.Fingerprint})
		}
		slots[i] = SlotInfo{Index: uint8(i), Fingerprint: c.Fingerprint, FpPresent: c.FpPresent}
	}

	d := &descriptor{
		n:        uint8(n),
		pathDecl: pd,
		// useSite = <0;1>/* — hasMultipath, alts {0},{1}, unhardened wildcard.
		useSite: useSitePath{
			hasMultipath:     true,
			multipath:        []alternative{{hardened: false, value: 0}, {hardened: false, value: 1}},
			wildcardHardened: false,
		},
		tree: tree,
		tlv: tlvSection{
			pubPresent:   true,
			pubkeys:      pubkeys,
			fpPresent:    len(fps) > 0,
			fingerprints: fps,
		},
	}

	out, err = split(d)
	if err != nil {
		return nil, [4]byte{}, nil, err
	}
	// Form-aware stub (C2): keyed → WalletPolicyId, keyless template → WDT-Id.
	stub, err = FormAwareStub(d)
	if err != nil {
		return nil, [4]byte{}, nil, err
	}
	return out, stub, slots, nil
}

// toComponents converts the public RAW []PathComponent into the internal
// []pathComponent (same shape; Hardened/Value → hardened/value).
func toComponents(in []PathComponent) []pathComponent {
	out := make([]pathComponent, len(in))
	for i, c := range in {
		out[i] = pathComponent{hardened: c.Hardened, value: c.Value}
	}
	return out
}

// multiSigTree returns the wallet-policy tree for the three sortedmulti wrappers,
// each wrapping sortedmulti{k, [0..n-1]} (indices in cosigner order):
//
//	MultisigWsh   -> node{tagWsh, [node{tagSortedMulti, multiKeysBody{k,[0..n-1]}}]}
//	MultisigShWsh -> node{tagSh,  [node{tagWsh, [node{tagSortedMulti, ...}]}]}
//	MultisigSh    -> node{tagSh,  [node{tagSortedMulti, ...}]}
//
// k/n bounds (k,n in 1..32, k<=n) are enforced downstream by writeNode's
// multiKeysBody guards (errThresholdRange/errChildCount/errKGreaterThanN); this
// helper only fixes the wrapper shape and rejects an unknown script kind.
func multiSigTree(script MultisigScript, k uint8, n int) (node, error) {
	indices := make([]uint8, n)
	for i := range indices {
		indices[i] = uint8(i)
	}
	sm := node{tag: tagSortedMulti, body: multiKeysBody{k: k, indices: indices}}
	switch script {
	case MultisigWsh:
		return node{tag: tagWsh, body: childrenBody{children: []node{sm}}}, nil
	case MultisigShWsh:
		inner := node{tag: tagWsh, body: childrenBody{children: []node{sm}}}
		return node{tag: tagSh, body: childrenBody{children: []node{inner}}}, nil
	case MultisigSh:
		return node{tag: tagSh, body: childrenBody{children: []node{sm}}}, nil
	default:
		return node{}, errMultisigBadScript
	}
}

// checkOriginKeyConsistency refuses a request whose cosigners bind one key
// origin to several different keys (F-217).
//
// SCOPE, matching the Rust primary exactly so the two cannot drift:
//   - both cosigners must have FpPresent. Without a fingerprint the origin path
//     names no master, so no contradiction is provable and none is claimed.
//   - the SAME key in two slots at one origin is CONSISTENT here. That is key
//     reuse — a different hazard with its own refusal — and one message
//     explaining both would explain neither.
//   - two DIFFERENT fingerprints at one path are fine; two people may both use
//     48'/0'/0'/2'. Refusing that would break every ordinary multisig.
//
// In OriginShared mode every cosigner has the same origin by construction, so
// the comparison is on fingerprint and key alone — which is precisely the case
// `--path` produced on the host.
func checkOriginKeyConsistency(req EncodeMultisigRequest) error {
	sameOrigin := func(a, b MultisigCosigner) bool {
		if req.OriginMode == OriginShared {
			return true
		}
		if len(a.Origin) != len(b.Origin) {
			return false
		}
		for i := range a.Origin {
			if a.Origin[i] != b.Origin[i] {
				return false
			}
		}
		return true
	}
	for i := range req.Cosigners {
		a := req.Cosigners[i]
		if !a.FpPresent {
			continue
		}
		for j := i + 1; j < len(req.Cosigners); j++ {
			b := req.Cosigners[j]
			if !b.FpPresent || a.Fingerprint != b.Fingerprint || !sameOrigin(a, b) {
				continue
			}
			if a.ChainCode == b.ChainCode && a.CompressedPubkey == b.CompressedPubkey {
				continue // same key at one origin: consistent, and not this error
			}
			return fmt.Errorf("%w: @%d and @%d both claim %x", ErrOriginKeyContradiction, i, j, a.Fingerprint)
		}
	}
	return nil
}
