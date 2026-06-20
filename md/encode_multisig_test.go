package md

import "testing"

// ─── T6c Phase A: EncodeMultisig — wallet-policy sortedmulti md1 ─────────────

// TestEncodeMultisigRequestPlumbing constructs a request and asserts the fields
// are wired through (compile-time + value checks on the public surface).
func TestEncodeMultisigRequestPlumbing(t *testing.T) {
	req := EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true},
			{Fingerprint: [4]byte{5, 6, 7, 8}, FpPresent: false},
		},
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   OriginShared,
		SharedOrigin: []PathComponent{{Hardened: true, Value: 48}},
	}
	if len(req.Cosigners) != 2 || req.K != 2 {
		t.Fatalf("request fields not plumbed: %+v", req)
	}
	if req.Script != MultisigWsh || req.OriginMode != OriginShared {
		t.Fatalf("enum fields not plumbed: %+v", req)
	}
	// SlotInfo is the ordering-verification handle element.
	s := SlotInfo{Index: 1, Fingerprint: [4]byte{5, 6, 7, 8}, FpPresent: false}
	if s.Index != 1 || s.FpPresent {
		t.Fatalf("SlotInfo not plumbed: %+v", s)
	}
	// Enum identity: the three script wrappers + two origin modes are distinct.
	if MultisigWsh == MultisigShWsh || MultisigShWsh == MultisigSh {
		t.Fatal("MultisigScript values not distinct")
	}
	if OriginShared == OriginDivergent {
		t.Fatal("OriginMode values not distinct")
	}
}
