package md

import (
	"encoding/hex"
	"testing"

	"seedhammer.com/codex32"
)

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

// mkXpub65 builds a 65-byte chainCode‖compressedPubkey from two hex strings.
func mkXpub65(t *testing.T, ccHex, pkHex string) (cc [32]byte, pk [33]byte) {
	t.Helper()
	ccb, err := hex.DecodeString(ccHex)
	if err != nil || len(ccb) != 32 {
		t.Fatalf("bad chaincode %q", ccHex)
	}
	pkb, err := hex.DecodeString(pkHex)
	if err != nil || len(pkb) != 33 {
		t.Fatalf("bad pubkey %q", pkHex)
	}
	copy(cc[:], ccb)
	copy(pk[:], pkb)
	return
}

// sharedOrigin4828 is m/48'/0'/0'/2' as RAW PathComponents (the T6b origin).
func sharedOrigin4828() []PathComponent {
	return []PathComponent{
		{Hardened: true, Value: 48}, {Hardened: true, Value: 0},
		{Hardened: true, Value: 0}, {Hardened: true, Value: 2},
	}
}

// TestEncodeMultisigSmoke: a 2-of-3 wsh(sortedmulti) over three distinct keys
// encodes to >=2 chunks, the returned stub == WalletPolicyIDStubChunks(out), and
// the slots reflect cosigner order with the right fp-presence.
func TestEncodeMultisigSmoke(t *testing.T) {
	cc, pk := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23")
	cc2, pk2 := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	req := EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{ChainCode: cc, CompressedPubkey: pk},
			{ChainCode: cc2, CompressedPubkey: pk2, Fingerprint: [4]byte{0xde, 0xad, 0xbe, 0xef}, FpPresent: true},
			{ChainCode: cc, CompressedPubkey: pk2},
		},
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   OriginShared,
		SharedOrigin: sharedOrigin4828(),
	}
	out, stub, slots, err := EncodeMultisig(req)
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	if len(out) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(out))
	}
	for _, s := range out {
		if !codex32.ValidMD(s) {
			t.Fatalf("chunk not ValidMD: %s", s)
		}
	}
	wantStub, err := WalletPolicyIDStubChunks(out)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if stub != wantStub {
		t.Fatalf("returned stub %x != WalletPolicyIDStubChunks(out) %x", stub, wantStub)
	}
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}
	for i, s := range slots {
		if int(s.Index) != i {
			t.Fatalf("slot %d Index = %d, want %d (order-preserving)", i, s.Index, i)
		}
	}
	if !slots[1].FpPresent || slots[1].Fingerprint != [4]byte{0xde, 0xad, 0xbe, 0xef} {
		t.Fatalf("slot 1 fp not plumbed: %+v", slots[1])
	}
	if slots[0].FpPresent {
		t.Fatalf("slot 0 should be fp-absent")
	}
}
