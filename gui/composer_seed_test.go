package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestComposerSeedAccountsAreOrdinalsPerMaster is §4f's account rule, and the
// C29/C5 case that makes it necessary: one seed at several slots must derive
// several DIFFERENT keys.
func TestComposerSeedAccountsAreOrdinalsPerMaster(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	st.sources = []composerSource{
		{kind: composerSourceSeed, seedID: 0, fingerprint: [4]byte{1, 2, 3, 4}, fpPresent: true},
		{kind: composerSourceSeed, seedID: 1, fingerprint: [4]byte{9, 9, 9, 9}, fpPresent: true},
	}
	st.assigned = make([]composerAssignment, 4)
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
	// Seed 0 at slots @0 and @2; seed 1 at @1.
	st.assigned[0].src, st.assigned[2].src, st.assigned[1].src = 0, 0, 1
	if got := composerSeedAccountFor(st, 0, 0); got != 0 {
		t.Errorf("the FIRST slot a master fills gets account %d, want 0", got)
	}
	if got := composerSeedAccountFor(st, 2, 0); got != 1 {
		t.Errorf("the SECOND slot the same master fills gets account %d, want 1 -- "+
			"account 0 twice would mint one key at two slots, which md refuses at encode", got)
	}
	if got := composerSeedAccountFor(st, 1, 1); got != 0 {
		t.Errorf("a different master's first slot gets account %d, want 0: the ordinal is "+
			"per MASTER, not per flow", got)
	}
}

// TestComposerSeedOriginFollowsSection4fPerWrapper pins the origin table,
// including the taproot 3' arm the shipped multisigScriptTypeComponent does
// not have (gui/multisig_build_slots.go:125-130 returns only 1' or 2').
func TestComposerSeedOriginFollowsSection4fPerWrapper(t *testing.T) {
	for _, tc := range []struct {
		w    md.ComposeWrapper
		want uint32
	}{
		{md.ComposeWsh, 2},
		{md.ComposeSh, 2},
		{md.ComposeShWsh, 1},
		{md.ComposeTr, 3},
	} {
		got := md.DefaultOrigin(tc.w, 0)
		if len(got) != 4 {
			t.Fatalf("wrapper %v: DefaultOrigin has %d components, want 4 (m/48'/0'/a'/T')", tc.w, len(got))
		}
		if got[0].Value != 48 || !got[0].Hardened {
			t.Errorf("wrapper %v: first component %+v, want 48'", tc.w, got[0])
		}
		if got[1].Value != 0 || !got[1].Hardened {
			t.Errorf("wrapper %v: coin %+v, want 0' (mainnet only, §4f and gui/policy_address.go:61)", tc.w, got[1])
		}
		if got[3].Value != tc.want || !got[3].Hardened {
			t.Errorf("wrapper %v: script type %+v, want %d'", tc.w, got[3], tc.want)
		}
	}
}

// TestComposerSeedHookIsObservationOnly is recon risk 6, as a gate: the hook
// must never be mistaken for the scrub. It fires and it zeroes nothing; the
// registry's scrub is what zeroes, and composerFlow installs it with a defer
// before any seed exists.
func TestComposerSeedHookIsObservationOnly(t *testing.T) {
	reg := &seedRegistry{}
	id, err := reg.add("t", composerTestMnemonic(t), "", composerMainNet())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reg.at(id)
	if !ok || len(got.Mnemonic) != 12 {
		t.Fatalf("the fixture seed did not register (%d words, ok=%v)", len(got.Mnemonic), ok)
	}
	// A NON-ZERO WORD, deliberately chosen: the "abandon" vector's first
	// eleven words are index 0, so asserting Mnemonic[0] != 0 before the
	// scrub would fail on a correctly registered seed, and asserting it is 0
	// afterwards would pass on a seed that was never written. The last word,
	// "about", is index 3.
	if got.Mnemonic[11] == 0 {
		t.Fatalf("the fixture's last word is %v, want a non-zero index -- this test "+
			"cannot distinguish scrubbed from unwritten otherwise", got.Mnemonic[11])
	}
	reg.scrub()
	after, _ := reg.at(id)
	for i, w := range after.Mnemonic {
		if w != 0 {
			t.Fatalf("word %d survived scrub as %v; C14 asks for Multisig Build's "+
				"treatment and this is that mechanism", i, w)
		}
	}
	if composerSeedHook != nil {
		t.Error("composerSeedHook is non-nil in a test that did not set it; it must be " +
			"nil in production, exactly as buildMultisigSeedHook is")
	}
}
