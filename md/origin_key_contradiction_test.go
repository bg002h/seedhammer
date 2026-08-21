package md

import (
	"errors"
	"testing"
)

// F-217, convergence port. A card binding ONE key origin to DIFFERENT keys
// describes a wallet that cannot exist: BIP-32 is deterministic, so a
// (fingerprint, path) pair names exactly one extended key.
//
// The primary found this in NINE of nine multi-key conformance vectors. This
// port agreed with Rust on every one of them — same bytes, same ids, same
// addresses — which is exactly why it needs its own check: addresses derive
// from the keys a card CARRIES, never from the origin it declares, so nothing
// downstream in either language can see the contradiction.

func cosigner(fp byte, pub byte, origin []PathComponent) MultisigCosigner {
	c := MultisigCosigner{FpPresent: true, Origin: origin}
	c.Fingerprint = [4]byte{fp, fp, fp, fp}
	// A distinct, well-formed-looking key per `pub`. The bytes never reach
	// secp256k1 here — checkOriginKeyConsistency runs before any key parsing —
	// and using recognisable filler keeps a failure message readable.
	c.CompressedPubkey[0] = 0x02
	c.CompressedPubkey[1] = pub
	c.ChainCode[0] = pub
	return c
}

func bip48(account uint32) []PathComponent {
	const h = 0x80000000
	return []PathComponent{
		{Value: 48 | h}, {Value: 0 | h}, {Value: account | h}, {Value: 2 | h},
	}
}

func req(mode OriginMode, shared []PathComponent, cs ...MultisigCosigner) EncodeMultisigRequest {
	return EncodeMultisigRequest{
		Cosigners:    cs,
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   mode,
		SharedOrigin: shared,
	}
}

// The `--path` shape: one shared origin over two different keys of one master.
func TestSharedOriginOverDifferentKeysIsRefused(t *testing.T) {
	_, _, _, err := EncodeMultisig(req(OriginShared, bip48(0),
		cosigner(0x73, 1, nil), cosigner(0x73, 2, nil)))
	if !errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatalf("a card claiming one origin for two keys was accepted; err = %v", err)
	}
}

// The same in DIVERGENT mode, when two cosigners happen to declare the same path.
func TestDivergentModeAlsoRefusesAMatchingPair(t *testing.T) {
	_, _, _, err := EncodeMultisig(req(OriginDivergent, nil,
		cosigner(0x73, 1, bip48(0)), cosigner(0x73, 2, bip48(0))))
	if !errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatalf("divergent mode accepted a matching-origin pair with different keys; err = %v", err)
	}
}

// The CORRECT card — each key at its own account — must still encode. Without
// this the refusal above could be satisfied by refusing everything.
func TestDivergentOriginsPerAccountStillEncode(t *testing.T) {
	_, _, _, err := EncodeMultisig(req(OriginDivergent, nil,
		cosigner(0x73, 1, bip48(0)), cosigner(0x73, 2, bip48(1))))
	if errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatal("two accounts of one master were refused as contradictory")
	}
}

// Two DIFFERENT masters at one path is ordinary multisig. Refusing it would
// break the common case.
func TestDifferentFingerprintsAtOnePathAreFine(t *testing.T) {
	_, _, _, err := EncodeMultisig(req(OriginShared, bip48(0),
		cosigner(0x73, 1, nil), cosigner(0xAA, 2, nil)))
	if errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatal("two different masters at one path were refused")
	}
}

// The SAME key twice at one origin is consistent: one origin, one key. It is
// key reuse (the build flow's own refusal), not this error.
func TestSameKeyTwiceIsNotThisError(t *testing.T) {
	_, _, _, err := EncodeMultisig(req(OriginShared, bip48(0),
		cosigner(0x73, 1, nil), cosigner(0x73, 1, nil)))
	if errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatal("key reuse was reported as an impossible origin")
	}
}

// Without a fingerprint the origin path names no master, so nothing is
// provable and nothing is claimed.
func TestNoFingerprintMeansNoClaim(t *testing.T) {
	a := cosigner(0x73, 1, nil)
	b := cosigner(0x73, 2, nil)
	b.FpPresent = false
	_, _, _, err := EncodeMultisig(req(OriginShared, bip48(0), a, b))
	if errors.Is(err, ErrOriginKeyContradiction) {
		t.Fatal("a contradiction was claimed against a cosigner with no fingerprint")
	}
}
