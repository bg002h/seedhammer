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
