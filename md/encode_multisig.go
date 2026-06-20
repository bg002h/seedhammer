package md

import "errors"

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
		return nil, [4]byte{}, nil, errMultisigBadScript
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
	stub, err = WalletPolicyIDStub(d)
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
