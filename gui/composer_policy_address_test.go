package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"seedhammer.com/md"
)

// TestDeviceDerivesTheComposerTimelockHashlockPolicy is the address gate for the
// shape the Wallet Policy composer can now build end to end: three spend paths
// under one wsh wrapper carrying `pkh`, a RELATIVE timelock, an ABSOLUTE one and
// a `sha256` hashlock.
//
// WHY IT EXISTS. The composer gained locks and hashlocks (H6), so an operator can
// compose this policy on the machine — and until this test, nothing asserted that
// the address the DEVICE derives from it is the address anyone else derives. The
// expectation is not computed here: it is the conformance record the Rust primary
// generates for `keyed_compose_wsh_timelock_hashlock`, vendored under
// `md/testdata/vectors/` and sha-pinned by `md/compose_vectors_pin_test.go`. So a
// disagreement between the two implementations fails HERE, and a drifted vendored
// copy fails THERE.
//
// The same addresses were confirmed a third time outside both codebases: bitcoind
// generated them from the equivalent descriptor and mined to one (the regtest
// forms of the same witness programs).
//
// MUTATION: return the change chain from the receive branch (swap `false` for
// `true` in the call below) -> every index mismatches, because chain 1 derives a
// different key.
func TestDeviceDerivesTheComposerTimelockHashlockPolicy(t *testing.T) {
	const vector = "keyed_compose_wsh_timelock_hashlock"

	chunks := loadVectorChunks(t, vector)
	if len(chunks) == 0 {
		t.Fatalf("%s: no md1 chunks in the vendored vector", vector)
	}
	tmpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("the vector seats %d keys, want 3 (template %v)", len(keys), tmpl)
	}

	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "vectors", vector+".conformance.json"))
	if err != nil {
		t.Fatalf("read conformance record: %v", err)
	}
	var rec struct {
		Chains map[string]struct {
			Addresses []string `json:"addresses"`
		} `json:"chains"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse conformance record: %v", err)
	}
	want := rec.Chains["0"].Addresses
	if len(want) == 0 {
		t.Fatalf("%s: the conformance record carries no receive addresses", vector)
	}

	at, ok := complexAddressSource(chunks, keys)
	if !ok {
		t.Fatal("the device cannot derive an address for a policy its own composer builds")
	}
	for i, w := range want {
		got, err := at(uint32(i), false)
		if err != nil {
			t.Fatalf("derive receive[%d]: %v", i, err)
		}
		if got != w {
			t.Errorf("receive[%d]\n device %s\n rust   %s", i, got, w)
		}
	}
	t.Logf("%d receive addresses agree with the Rust conformance record", len(want))
}
