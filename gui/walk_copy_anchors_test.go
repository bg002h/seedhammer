package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// TestEmulatorWalksQuoteCopyThatStillExists is the boundary for a class that
// has now bitten three times, each time silently.
//
// THE WALKS UNDER cmd/emu ARE GATES THAT CI CANNOT RUN. They drive the real
// screens in a browser against emu.wasm, and they assert on SCREEN TEXT -- so
// every operator-facing string they name is a dependency on production copy
// that no compiler, no `go vet`, and no `node --check` can see. Rewording a
// body leaves the walk syntactically perfect and broken at the first frame it
// waits for.
//
// THE THREE OCCURRENCES, all in this cycle:
//   - shots_composer.js asserted `Type 64 hex` after the row was renamed
//     `Type a digest` -- in a commit that edited that same file two lines
//     below for a different stale string.
//   - shots_composer.js asserted "The hash must be SHA-256" after §7.2 split
//     §8i and retired that wording.
//   - walk_hashlock_phrase.js waited for "run ms hashlock with this phrase"
//     while the reconcile body had said "--kind <kind>" since the §13.2 fold,
//     and then "--method <m>" as well. It had been stale across two folds.
//
// So the rule is stated here, at the boundary, rather than fixed a fourth time
// at an occurrence: A WALK MAY NAME PRODUCTION COPY ONLY THROUGH THIS TABLE.
// Each row asserts the fragment is BOTH what the copy function still produces
// AND what the walk file still contains, so the two cannot drift apart in
// either direction -- a reworded body fails here, and so does a walk edited to
// expect text production never emits.
//
// This does not gate every string a walk uses; screen TITLES and fixed labels
// are not composer copy. What it gates is every fragment that comes from a
// composerCopy* body, which is where all three failures were.
func TestEmulatorWalksQuoteCopyThatStillExists(t *testing.T) {
	for _, tc := range []struct {
		walk     string // file under cmd/emu
		fragment string // what the walk waits for
		produced string // the copy that must still contain it
		why      string
	}{
		{"walk_hashlock_phrase.js", "32-byte preimage", composerCopyHashKindLead(),
			"§7.1's kind screen lead"},
		{"walk_hashlock_phrase.js", "run ms hashlock --kind",
			composerCopyHashlockReconcile("b8..cb", "hardened", 28, md.KindSha256),
			"§13.2's reconcile instruction, stale across two folds before this gate"},
		{"walk_hashlock_phrase.js", "ripemd160",
			composerCopyHashlockConfirm("09..6b", "hardened", 28, "", "", md.KindRipemd160),
			"§12 item 2's non-sha256 trial: the confirm modal must name the kind the " +
				"walk then asserts through the seam"},
		{"shots_composer.js", "The preimage must be", composerCopyHashRule(),
			"§8i's ENTRY body, which names no hash function since §7.2"},
		{"shots_composer.js", "Type a digest", composerHashRowHex,
			"the typed-digest row, renamed from `Type 64 hex` by §7.1"},
	} {
		t.Run(tc.walk+"/"+tc.fragment, func(t *testing.T) {
			if !strings.Contains(tc.produced, tc.fragment) {
				t.Errorf("PRODUCTION NO LONGER EMITS %q (%s).\nThe walk waits for it and would "+
					"throw at that frame -- and no compiler, vet or node --check can see it, "+
					"because the walks do not run in CI.\nProduced now: %q",
					tc.fragment, tc.why, tc.produced)
			}
			raw, err := os.ReadFile(filepath.Join("..", "cmd", "emu", tc.walk))
			if err != nil {
				t.Fatalf("reading the walk: %v", err)
			}
			if !strings.Contains(string(raw), tc.fragment) {
				t.Errorf("cmd/emu/%s no longer contains %q (%s). Either the walk was edited "+
					"away from the copy, or this row is stale -- both mean the gate below it "+
					"is not gating what it names.", tc.walk, tc.fragment, tc.why)
			}
		})
	}
}
