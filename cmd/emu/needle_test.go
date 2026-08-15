package main

// UNTAGGED, for the reason engraved.go gives: this reads gui's SOURCE off disk
// rather than referencing any //go:build js symbol, so it runs on the host.

// WHY THIS FILE EXISTS (F-169).
//
// §4.5 makes an emulator walk the closing gate of every stage from S1 on, and
// all five of those stages edit buildMultisigPolicyFlow — which sits behind
// "Engrave Multisig -> Build policy". The walk that existed drove the SIBLING
// choice, "Engrave Bundle", so every one of those gates named a flow no walk
// entered.
//
// A walk written by editing that script's goTo target would have looked
// identical and still proved nothing, because every needle it used is
// AMBIGUOUS. Measured, not assumed:
//
//	"First card from where?"   3 production sites
//	"Which md1?"               2 production sites
//	"Choose policy type"       1
//
// So a walk must anchor on a string that exists in ONE production flow, and
// "one" has to be a machine-checked fact rather than a claim in a comment —
// the counts above drift every time somebody adds a screen. That is what this
// file checks. It is the standing half of the gate; the walk asserts the needle
// appears, this asserts the needle could only have come from one place.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFlowNeedles are the strings a Build-policy walk may anchor on. Each MUST
// have exactly one production site, and that site must be inside the build
// flow's own file — a needle unique to some OTHER flow identifies the wrong
// thing just as badly as an ambiguous one.
//
// Keep this list SHORT and load-bearing. It is not an inventory of the flow's
// screens; it is the set a walk is allowed to trust.
var buildFlowNeedles = []struct {
	text string
	file string // the single production file the needle must live in
}{
	{"Choose policy type", "gui/multisig_build.go"},
	{"How many keys (n)?", "gui/multisig_build.go"},
	{"Which slot is your key?", "gui/multisig_build.go"},
	// The front door, one level above the build flow. A walk uses this to prove
	// it reached Engrave Multisig at all before choosing "Build policy".
	{"Supply or build a policy?", "gui/multisig.go"},
	// S1's bounded-selection surface, reached ONLY when the payload supplied
	// more cosigner cards than the policy has open slots. It is the first needle
	// that proves something about the CARDS rather than the parameters: a walk
	// seeing it has a payload-fed cosigner set that had to be narrowed, which is
	// the whole `0..n` ruling on screen.
	{"Payload cards", "gui/multisig_build_payload.go"},
	{"Use payload card", "gui/multisig_build_payload.go"},
}

// decoyNeedles are strings a stage author reaches for FIRST and must not use.
// Pinned with their measured counts so this test fails loudly if one ever
// becomes unique — at which point it is promoted deliberately, not by accident.
var decoyNeedles = []struct {
	text string
	want int
}{
	// Two sites: the build flow's wallet-policy form picker and singlesig's.
	{"Which md1?", 2},
	// TWO sites since S1: bundleFlow and supplyMultisigPolicyFlow. It was three
	// — buildMultisigPolicyFlow had the same picker — and S1 removed that one,
	// because the Build path now takes the WHOLE cosigner set from the payload
	// and a source picker with one answer is a tap that teaches nothing. Still a
	// decoy, and still the reason "the walk reached a card gather" proves
	// nothing: two flows can draw it.
	{"First card from where?", 2},
	// The gather's title comes from the SHARED gatherer, so it reads
	// "Engrave Bundle" even when the operator arrived via Build policy. A walk
	// that trusted it would report the wrong flow with total confidence.
	{"Engrave Bundle", 0}, // 0 == "at least one, count not pinned"; see the test
}

func TestBuildFlowNeedlesHaveExactlyOneProductionSite(t *testing.T) {
	for _, n := range buildFlowNeedles {
		sites := productionSites(t, n.text)
		if len(sites) != 1 {
			t.Errorf("needle %q has %d production site(s), want exactly 1:\n  %s\n"+
				"a walk anchoring on this cannot prove which flow it is in",
				n.text, len(sites), strings.Join(sites, "\n  "))
			continue
		}
		if got := sites[0]; got != n.file {
			t.Errorf("needle %q is unique but lives in %s, want %s — "+
				"it identifies a different flow than the walk thinks",
				n.text, got, n.file)
		}
	}
}

func TestDecoyNeedlesAreStillAmbiguous(t *testing.T) {
	for _, d := range decoyNeedles {
		sites := productionSites(t, d.text)
		switch {
		case d.want == 0:
			if len(sites) == 0 {
				t.Errorf("decoy %q has no production site at all — it was renamed, "+
					"so this guard now protects nothing", d.text)
			}
		case len(sites) != d.want:
			t.Errorf("decoy %q now has %d production site(s), pinned at %d:\n  %s\n"+
				"if it became UNIQUE, promote it to buildFlowNeedles deliberately; "+
				"if it grew, update the pin",
				d.text, len(sites), d.want, strings.Join(sites, "\n  "))
		}
	}
}

// TestNeedleSiteCounterCanCount is the mutation proof for the counter itself.
//
// Without it, a productionSites that silently returned nothing would make
// EVERY decoy look unique and every needle look absent, and the two tests above
// would report whatever that bug implied — the false-PASS shape this whole
// stage exists to remove. So the counter is exercised against strings whose
// answers are known independently of gui's contents.
func TestNeedleSiteCounterCanCount(t *testing.T) {
	// A string that cannot occur in gui's source.
	if got := productionSites(t, "zzz-this-string-is-not-in-gui-zzz"); len(got) != 0 {
		t.Errorf("counter found %d site(s) for an impossible string: %v", len(got), got)
	}
	// A string that certainly occurs many times.
	if got := productionSites(t, "func "); len(got) < 5 {
		t.Errorf("counter found %d file(s) containing %q; gui has far more, "+
			"so the counter is not reading the tree", len(got), "func ")
	}
}

// productionSites returns the gui/*.go files containing text, excluding tests.
// Returns one entry per FILE, deduplicated — two hits in one file still mean
// one place a walk could be.
//
// Deliberately blunt substring matching over source bytes, the same thing
// `git grep -F` does, because the alternative (parsing Go and reading string
// literals) would quietly stop counting a needle built by concatenation or
// fmt.Sprintf — and a needle a walk can SEE on screen is one this must count.
func productionSites(t *testing.T, text string) []string {
	t.Helper()
	dir := filepath.Join("..", "..", "gui")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	checked := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.Contains(string(b), text) {
			out = append(out, "gui/"+name)
		}
	}
	// A floor, so a misrooted path cannot make every needle look unique by
	// finding nothing at all. gui is a large package; if this ever legitimately
	// drops below the floor the number is wrong, not the guard.
	if checked < 30 {
		t.Fatalf("only %d production .go file(s) under %s — the path is wrong, "+
			"and every count from it is meaningless", checked, dir)
	}
	return out
}
