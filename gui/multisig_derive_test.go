package gui

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// TestDeriveMultisigLeg: the operator's leg for the full-policy fixture (seed at
// slot @1, origin m/48'/0'/0'/2'). The mk1 stub == WalletPolicyIDStubChunks of
// the SUPPLIED md1 (I-4); the mk1 Path == the matched slot origin; ms1
// round-trips the entropy. Full mode includes ms1; watch-only omits it.
func TestDeriveMultisigLeg(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	m := abandonAboutMnemonic()
	idx, origin, _, ok := findUserSlot(m, "", &chaincfg.MainNetParams, keys)
	if !ok || idx != 1 {
		t.Fatalf("findUserSlot idx=%d ok=%v, want match @1", idx, ok)
	}

	b, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, chunks, true)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(full): %v", err)
	}

	// md1 leg is the SUPPLIED strings verbatim.
	if len(b.MD1) != len(chunks) {
		t.Fatalf("MD1 len = %d, want %d (verbatim supply)", len(b.MD1), len(chunks))
	}
	for i := range chunks {
		if b.MD1[i] != chunks[i] {
			t.Fatalf("MD1[%d] not verbatim:\n got %s\nwant %s", i, b.MD1[i], chunks[i])
		}
	}

	// mk1: bound stub + matched path.
	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	wantStub, err := md.WalletPolicyIDStubChunks(chunks)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != wantStub {
		t.Fatalf("mk1 stubs = %v, want [%v] (bound to the supplied policy)", card.Stubs, wantStub)
	}
	if card.Path != "m/48h/0h/0h/2h" {
		t.Fatalf("mk1 path = %q, want m/48h/0h/0h/2h", card.Path)
	}
	if card.Fingerprint != "73c5da0a" {
		t.Fatalf("mk1 fingerprint = %q, want 73c5da0a", card.Fingerprint)
	}

	// ms1 round-trips the entropy.
	if b.MS1 == "" {
		t.Fatal("full mode produced no ms1")
	}
	ms1str, err := codex32.New(b.MS1)
	if err != nil {
		t.Fatalf("codex32.New: %v", err)
	}
	_, _, ent, err := codex32.DecodeMS1(ms1str)
	if err != nil {
		t.Fatalf("DecodeMS1: %v", err)
	}
	if !bytes.Equal(ent, m.Entropy()) {
		t.Fatalf("ms1 entropy = %x, want %x", ent, m.Entropy())
	}

	// Watch-only: no ms1.
	wo, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, chunks, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(watch-only): %v", err)
	}
	if wo.MS1 != "" {
		t.Fatalf("watch-only ms1 = %q, want empty", wo.MS1)
	}
}
