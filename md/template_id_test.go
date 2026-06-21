package md

import "testing"

// ─── Task 1: isWalletPolicy predicate (Some-AND-non-empty, I1) ───────────────
//
// Mirrors Rust md-codec encode.rs is_wallet_policy: pubkeys present AND
// non-empty. A keyless template (pubkeys:null) is NOT a wallet policy; a keyed
// policy IS; a desynced descriptor that left pubPresent set with an empty
// pubkeys slice must NOT slip through as a wallet-policy (the I1 bug class).
func TestIsWalletPolicy(t *testing.T) {
	full := cell7WpkhDescriptor() // keyed: pubPresent + 1 xpub

	tmpl := cell7WpkhDescriptor()
	tmpl.tlv.pubPresent = false
	tmpl.tlv.pubkeys = nil

	if !isWalletPolicy(full) {
		t.Fatal("keyed descriptor must be a wallet-policy")
	}
	if isWalletPolicy(tmpl) {
		t.Fatal("keyless template must NOT be a wallet-policy")
	}

	// I1: pubPresent stays true but the pubkeys slice is empty (a strip that
	// nulled the slice but forgot to clear the flag) → must be false.
	desync := cell7WpkhDescriptor()
	desync.tlv.pubkeys = nil // pubPresent still true, pubkeys empty
	if isWalletPolicy(desync) {
		t.Fatal("empty pubkeys must NOT be a wallet-policy (I1)")
	}
}
