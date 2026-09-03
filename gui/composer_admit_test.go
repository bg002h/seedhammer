package gui

import (
	"os"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// TestComposerWalletPolicyAdmitsTheComposerClasses is C12, as a gate.
//
// The row used to be Descriptor + MDMK and its comment said "NO seed class
// ... least privilege". The composer AUTHORS a wallet and may fill a seat
// from a seed on this device, so the privilege the program needs changed.
// What is still refused is named too, because an admission test that only
// checks the additions cannot catch a row that admitted everything.
func TestComposerWalletPolicyAdmitsTheComposerClasses(t *testing.T) {
	for _, c := range []sysw.Class{
		sysw.ClassDescriptor, sysw.ClassMDMK,
		sysw.ClassMnemonic, sysw.ClassCodex32Secret, sysw.ClassPassphrase,
		sysw.ClassKey, sysw.ClassHash, sysw.ClassNow,
	} {
		if !admits(progWalletPolicy, c) {
			t.Errorf("progWalletPolicy refuses class %v, which SPEC §6a admits", c)
		}
	}
	for _, c := range []sysw.Class{sysw.ClassFreeText, sysw.ClassAddress, sysw.ClassUnknown} {
		if admits(progWalletPolicy, c) {
			t.Errorf("progWalletPolicy admits class %v, which SPEC §6a does not", c)
		}
	}
	// The three composer classes are admitted at Wallet Policy ALONE (§6a).
	for p := progBackupWallet; p <= progTransaction; p++ {
		if p == progWalletPolicy {
			continue
		}
		for _, c := range []sysw.Class{sysw.ClassKey, sysw.ClassHash, sysw.ClassNow} {
			if admits(p, c) {
				t.Errorf("program %d admits composer class %v; §6a admits the three at "+
					"Wallet Policy alone", p, c)
			}
		}
	}
}

// TestComposerSeedInAPayloadStillRaisesF1AtLoad is the §9 item 4 truth.
//
// The spec says the flag screens "fire inside the composer's seed step". They
// fire at LOAD, from syswLoadFlow's three call sites, and syswLoadWarnings
// consults no admission table -- so this behaviour is not created by the row
// change and is not per-program. The test pins the behaviour that IS relied
// on: a plaintext payload holding a seed raises F1, and the operator meets it
// before any program consumes anything.
func TestComposerSeedInAPayloadStillRaisesF1AtLoad(t *testing.T) {
	s := &syswSession{}
	// A payload the composer would use: a seed to seat from, and a key record.
	s.load(&sysw.Payload{
		Public: []string{composerTestKeyRecord},
		Secret: []string{composerTestMnemonicRecord},
	}, [32]byte{}, false, true, true, true)
	if !syswHasFlag(s, flagSecretInPlaintext) {
		t.Fatal("a plaintext payload holding a seed does not raise F1, so the operator " +
			"is never told a secret sits unencrypted in flash")
	}
	lines := syswLoadWarnings(s)
	if len(lines) == 0 {
		t.Fatal("syswLoadWarnings produced no line for an F1 payload")
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "SECRET is stored unencrypted in flash") {
			found = true
		}
	}
	if !found {
		t.Errorf("the F1 warning does not name the exposure: %q", lines)
	}
}

// TestComposerAddsNoPerProgramFlagScreenCall is the other half: the row
// change must not have grown a second place where the flags are shown.
//
// A negative inherits the scope of the search that produced it, so the scope
// is named: every non-test .go file in gui/, and the control below proves the
// query finds the calls that DO exist.
func TestComposerAddsNoPerProgramFlagScreenCall(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	callers := map[string]int{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(b), "syswLoadWarnings("); n > 0 {
			callers[name] += n
		}
	}
	// The CONTROL: the declaration and its one caller both live in
	// sysw_load.go. A query returning nothing everywhere would pass a
	// "no new callers" assertion for the wrong reason.
	if callers["sysw_load.go"] < 2 {
		t.Fatalf("INCONCLUSIVE: the query found %d mentions in sysw_load.go, where the "+
			"declaration and its one call both live -- the search is broken, not the tree",
			callers["sysw_load.go"])
	}
	delete(callers, "sysw_load.go")
	if len(callers) != 0 {
		t.Errorf("syswLoadWarnings gained per-program callers %v. The flags are a LOAD-time "+
			"mechanism (gui/sysw_load.go:210); a second site would show them twice and "+
			"put the admission table's classes behind two different rules", callers)
	}
}

// TestComposerMultisigBuildCarriesTheDeprecationComment is C7, whose whole
// deliverable is a comment. A comment-only deliverable with no gate is a
// deliverable nobody can tell was made.
func TestComposerMultisigBuildCarriesTheDeprecationComment(t *testing.T) {
	b, err := os.ReadFile("multisig_build.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		"Deprecated 2026-09-01 in favour of Wallet Policy",
		"Build a new policy",
		"No enforcement by operator ruling",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("gui/multisig_build.go does not carry %q -- SPEC §8e's whole "+
				"deliverable is this comment", want)
		}
	}
	// And it is a DEPRECATION, not a removal: the flow is still reachable.
	if !strings.Contains(src, "func buildMultisigPolicyFlow(") {
		t.Error("buildMultisigPolicyFlow is gone; C7 is comment-only, with no enforcement")
	}
}

// TestComposerWalletPolicyProgramCommentNoLongerSaysOutsideOnly pins the §6a
// rewrite. The comment argued the program's identity from a premise C12
// retires; leaving it standing is how a stale premise outlives its condition.
func TestComposerWalletPolicyProgramCommentNoLongerSaysOutsideOnly(t *testing.T) {
	b, err := os.ReadFile("gui.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, gone := range []string{
		"would drag a seed requirement or a plate census into a flow that needs neither",
	} {
		if strings.Contains(src, gone) {
			t.Errorf("gui.go still says %q. The composer DOES take a seed and DOES cut a "+
				"census inside this program, so the sentence is now false", gone)
		}
	}
	for _, want := range []string{"Build a new policy", "AUTHOR"} {
		if !strings.Contains(src, want) {
			t.Errorf("the walletPolicy program comment does not mention %q", want)
		}
	}
}

// TestComposerAdmitCommentNoLongerClaimsNoSeedClass is the sysw_admit.go half.
func TestComposerAdmitCommentNoLongerClaimsNoSeedClass(t *testing.T) {
	b, err := os.ReadFile("sysw_admit.go")
	if err != nil {
		t.Fatal(err)
	}
	// THE PHRASE ALONE IS NOT THE ASSERTION, and this is the second attempt at
	// it. progTransaction's row legitimately says "NO seed class and no
	// passphrase" (gui/sysw_admit.go:65) and is not touched by this cycle, so
	// scanning the file for "NO seed class" fails whatever the fold does --
	// and the rewritten comment QUOTES its own old wording, so it would fail
	// twice over. The claim retired is a whole sentence, unique to this row.
	if strings.Contains(string(b), "The Wallet Policy program never derives from a secret") {
		t.Error("sysw_admit.go still claims the Wallet Policy program never derives from " +
			"a secret, above a row that now admits Mnemonic, Cdx32 and Passphrase")
	}
	if !strings.Contains(string(b), "C12") {
		t.Error("the rewritten row does not cite C12, the ruling that reverses it")
	}
	// The CONTROL: the phrase that must SURVIVE, so this test cannot pass by
	// the file having been emptied.
	if !strings.Contains(string(b), "progTransaction") {
		t.Fatal("INCONCLUSIVE: sysw_admit.go no longer names progTransaction; the file " +
			"this test reads is not the admission table")
	}
}
