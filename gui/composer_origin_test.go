package gui

import (
	"testing"

	"seedhammer.com/md"
)

// SPEC_wallet_policy_composer.md §4f / §9 item 8: the composer's default
// origins use the same BIP-48 script-type table Multisig Build applies,
// extended by tr = 3'. The two tables live in two packages; this test is the
// tie between them.
func TestComposerOriginTableAgreesWithMultisigBuild(t *testing.T) {
	for _, tc := range []struct {
		script  md.MultisigScript
		wrapper md.ComposeWrapper
	}{
		{md.MultisigWsh, md.ComposeWsh},
		{md.MultisigShWsh, md.ComposeShWsh},
		{md.MultisigSh, md.ComposeSh},
	} {
		if a, b := multisigScriptTypeComponent(tc.script), tc.wrapper.ScriptType(); a != b {
			t.Errorf("script %v: multisig table %d, composer table %d", tc.script, a, b)
		}
	}
	if got := md.ComposeTr.ScriptType(); got != 3 {
		t.Errorf("tr script type = %d, want 3 (BIP-48 has no taproot row; §4f fixes 3')", got)
	}
	// And the whole default origin for a taproot slot at account 0.
	want := []md.PathComponent{{Hardened: true, Value: 48}, {Hardened: true, Value: 0}, {Hardened: true, Value: 0}, {Hardened: true, Value: 3}}
	got := md.DefaultOrigin(md.ComposeTr, 0)
	if len(got) != len(want) {
		t.Fatalf("DefaultOrigin(tr,0) = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DefaultOrigin(tr,0)[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
