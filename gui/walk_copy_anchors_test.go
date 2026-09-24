package gui

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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
	// EVERY FRAGMENT THE WALKS WAIT FOR, checked against EVERY copy body this
	// package declares -- not a hand-picked list.
	//
	// THE HAND-PICKED LIST WAS THE DEFECT. The first version of this gate held
	// four rows and its comment claimed it covered "every fragment that comes
	// from a composerCopy* body". Measured by review: it covered 6 of 26. Three
	// rewordings the walks wait for -- "One phrase per policy", "check the
	// digest matches", and §8.3's census heading -- would each break a walk
	// while this test printed ok. A gate that names its own coverage wrongly is
	// worse than no gate, because it stops anyone looking.
	//
	// So coverage is now STRUCTURAL. Every quoted fragment passed to waitFor or
	// must in the walk files must either appear in some copy body, or be listed
	// in notComposerCopy below WITH A REASON. Adding a waitFor for new copy
	// fails here until one of those is true.
	//
	// composerCopyTable() is the enumeration it checks against, and
	// TestComposerCopyTableCoversEveryBody proves that table holds every
	// composerCopy* function declared in composer_copy.go -- so "some copy
	// body" really does mean all of them.
	corpus := make([]string, 0, 128)
	for _, r := range composerCopyTable() {
		corpus = append(corpus, normalizeDrawn(r.got))
	}
	// Bodies that take arguments are rendered by the table at one sample; add
	// the kind-bearing variants the walks actually meet.
	for _, k := range composerHashKinds {
		corpus = append(corpus,
			normalizeDrawn(composerCopyHashlockConfirm("b8..cb", "hardened", 28, "", "", k)),
			normalizeDrawn(composerCopyHashlockReconcile("b8..cb", "hardened", 28, k)),
			normalizeDrawn(composerCopyHashRuleForKinds([]md.HashKind{k})),
			normalizeDrawn(composerCopyTwentyByteUnseen(k)),
			normalizeDrawn(composerHashRowHex),
			normalizeDrawn(composerHashRowPhrase))
	}
	// ARGUMENT-DEPENDENT BODIES AT THE ARGUMENTS THE WALKS USE. The table
	// renders each body once, at the spec's own example; §8.3's heading is
	// rendered there at n=2 and the walk meets it at n=1, so the table's
	// sample did not contain the walk's string. That is the shape that made
	// the first version of this gate under-cover: a body IS in the table and
	// still does not produce the fragment being waited for.
	for n := 1; n <= 3; n++ {
		corpus = append(corpus, normalizeDrawn(composerCopyPreimagePlateHeading(n)))
	}
	// §8g at the liana-same-seed arm's seating (F-671): one seed in all three
	// of kofn-recovery's path-1 slots, a 2-of-3.
	corpus = append(corpus, normalizeDrawn(composerCopySameSeedThreshold([]uint8{0, 1, 2}, 2, 3)))
	// ROW BUILDERS, WHICH ARE NOT composerCopy* BODIES AND WERE THE GATE'S HOLE.
	// The whole-diff review found two walk assertions this gate could not see --
	// `hash 1` (the picker row, now `sha256 1 ...`) and `hash <digest>` (the
	// consent line, now `hash sha256 <digest>`) -- because both come from row
	// builders and the gate only consulted composerCopyTable(). Operator-facing
	// text is not only what lives in composer_copy.go, and a gate scoped to one
	// file is scoped to the wrong thing.
	digest := composerTestLock([32]byte{0xab})
	for _, k := range composerHashKinds {
		lock, ok := md.NewHashLock(k, bytes.Repeat([]byte{0xab}, k.DigestLen()))
		if !ok {
			t.Fatalf("%s rejected its own width", k.Token())
		}
		corpus = append(corpus,
			normalizeDrawn(composerHashRow(1, lock)),
			normalizeDrawn(composerHashInPayloadRow(1, lock)),
			normalizeDrawn(composerHashPreimageRow(1, lock)),
			normalizeDrawn(composerHashPhraseRow(1, lock)),
			normalizeDrawn(composerConsentHashLine(lock)))
	}
	corpus = append(corpus, normalizeDrawn(composerHashPhraseRow(1, nil)),
		normalizeDrawn(composerConsentHashLine(digest)))

	for _, walk := range []string{"walk_hashlock_phrase.js", "shots_composer.js"} {
		raw, err := os.ReadFile(filepath.Join("..", "cmd", "emu", walk))
		if err != nil {
			t.Fatalf("reading %s: %v", walk, err)
		}
		frags := walkWaitFragments(string(raw))
		if len(frags) < 10 {
			t.Fatalf("%s: extracted only %d fragments -- the extractor has stopped "+
				"matching this file's call shapes, so this gate is measuring nothing",
				walk, len(frags))
		}
		for _, f := range frags {
			if reason, exempt := notComposerCopy[f]; exempt {
				_ = reason
				continue
			}
			found := false
			for _, body := range corpus {
				if strings.Contains(body, normalizeDrawn(f)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("cmd/emu/%s waits for %q, which NO composer copy body emits.\n"+
					"The walks assert on screen text and do not run in CI, so a reworded "+
					"body leaves them syntactically perfect and broken at that frame.\n"+
					"Either the copy drifted, or this fragment is not composer copy and "+
					"belongs in notComposerCopy with a reason.", walk, f)
			}
		}
	}
}

// walkWaitFragments extracts the quoted strings a walk waits for or asserts on.
//
// IT FAILS LOUD IF IT MATCHES NOTHING (the caller checks the count): an
// extractor that silently stops matching turns this whole gate green forever,
// which is the failure mode of every "scan the tree" test.
func walkWaitFragments(src string) []string {
	re := regexp.MustCompile(`(?:waitFor|must)\((?:[^,()]*,\s*)?"((?:[^"\\]|\\.){6,}?)"`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		f := m[1]
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// notComposerCopy lists fragments that are NOT drawn by a composerCopy* body,
// each with why. Screen titles, fixed widget labels and menu rows live in the
// screens themselves; payload-derived text is data, not copy.
//
// An entry here is a claim that rewording the named thing cannot break a walk
// silently -- so keep the reasons specific enough to be checkable.
var notComposerCopy = map[string]string{
	// ── payload- and fixture-derived DATA, not copy ──────────────────────
	// Digests, fingerprints, origins and slot labels come from the payload the
	// walk loaded. Rewording is not possible; they change when the FIXTURE
	// changes, and the walk's own corpus constants are what gate that.
	"73c5da0a m/48h/0h/0h/2h":                 "payload/fixture-derived data",
	"73c5da0a m/48h/0h/1h/2h":                 "payload/fixture-derived data",
	"@0: 73c5da0a m/48'/0'/0'/2'":             "payload/fixture-derived data",
	"@1: 73c5da0a m/48'/0'/1'/2'":             "payload/fixture-derived data",
	"@2: b8688df1 m/48'/0'/0'/2'":             "payload/fixture-derived data",
	"Slot @0 expects a key at m/48h/0h/0h/2h": "payload/fixture-derived data",
	"Slot @0 expects a key at m/48h/0h/0h/3h": "payload/fixture-derived data",
	"Slot @0, Path 1 key 1 of 2":              "payload/fixture-derived data",
	"Slot @0: 73c5da0a m/48h/0h/0h/2h":        "payload/fixture-derived data",
	"Slot @1 expects a key at m/48h/0h/1h/2h": "payload/fixture-derived data",
	"Slot @1 expects a key at m/48h/0h/1h/3h": "payload/fixture-derived data",
	"Slot @2 expects a key at m/48h/0h/2h/2h": "payload/fixture-derived data",
	"Slot @2 expects a key at m/48h/0h/2h/3h": "payload/fixture-derived data",
	"Slot @2: b8688df1 m/48h/0h/0h/2h":        "payload/fixture-derived data",
	"Slots @0 and @1 are the same seed.":      "payload/fixture-derived data",
	"abababab..abababab":                      "payload/fixture-derived data",
	"hash abababab..abababab":                 "payload/fixture-derived data",

	// ── drawn by screens, not by a composerCopy* body ────────────────────
	// Screen titles, menu rows, computed path summaries, and the copy of OTHER
	// programs (Load Payload, the BIP-39 passphrase flow, Scan cards). These
	// can still drift -- a retitled screen breaks a walk exactly as a reworded
	// body does -- but they are outside what §8's copy table enumerates, so
	// this gate cannot check them and says so rather than implying it can.
	"(any slots)":                              "screen title, menu row, computed summary or another program's copy",
	"12960 blocks":                             "screen title, menu row, computed summary or another program's copy",
	"12960 blocks (about 90.0 days)":           "screen title, menu row, computed summary or another program's copy",
	"A SECRET is stored unencrypted in flash.": "screen title, menu row, computed summary or another program's copy",
	"Add a BIP-39 passphrase?":                 "screen title, menu row, computed summary or another program's copy",
	"Add a spend path":                         "screen title, menu row, computed summary or another program's copy",
	"Build a new policy":                       "screen title, menu row, computed summary or another program's copy",
	"Build my own paths":                       "screen title, menu row, computed summary or another program's copy",
	"FROM PAYLOAD":                             "screen title, menu row, computed summary or another program's copy",
	"Keep this payload loaded?":                "screen title, menu row, computed summary or another program's copy",
	"Keyless template - no addresses.":         "screen title, menu row, computed summary or another program's copy",
	"Template has no keys - no addresses.":     "noAddressLines (wallet_policy.go), shared with the Wallet Policy program; not composer copy",
	"Keys loaded: 2, plus 1 seed.":             "screen title, menu row, computed summary or another program's copy",
	"Leave unseated":                           "screen title, menu row, computed summary or another program's copy",
	"Load Payload":                             "screen title, menu row, computed summary or another program's copy",
	"Load it?":                                 "screen title, menu row, computed summary or another program's copy",
	"No hash lock":                             "screen title, menu row, computed summary or another program's copy",
	"No slot is seated":                        "screen title, menu row, computed summary or another program's copy",
	"No slot is seated, so there is a template and nothing else.": "screen title, menu row, computed summary or another program's copy",
	"Path 1: 2-of-2":                      "screen title, menu row, computed summary or another program's copy",
	"Path 1: 2-of-3":                      "screen title, menu row, computed summary or another program's copy",
	"Path 1: KEY-LESS (EXPERIMENTAL)":     "screen title, menu row, computed summary or another program's copy",
	"Path 1: hash only":                   "screen title, menu row, computed summary or another program's copy",
	"Path 2: 1 key":                       "screen title, menu row, computed summary or another program's copy",
	"Path 2: 1 key + 12960 blocks":        "screen title, menu row, computed summary or another program's copy",
	"Path 2: 1 key + hash + 12960 blocks": "screen title, menu row, computed summary or another program's copy",
	"Path 2: 2-of-3":                      "screen title, menu row, computed summary or another program's copy",
	"Payload Digest":                      "screen title, menu row, computed summary or another program's copy",
	"Payload Warnings":                    "screen title, menu row, computed summary or another program's copy",
	"Plates To Cut":                       "screen title, menu row, computed summary or another program's copy",
	"Policy-ID":                           "screen title, menu row, computed summary or another program's copy",
	"Review":                              "screen title, menu row, computed summary or another program's copy",
	"Scan cards":                          "screen title, menu row, computed summary or another program's copy",
	"Seat keys into this template?":       "screen title, menu row, computed summary or another program's copy",
	"SeedHammer":                          "screen title, menu row, computed summary or another program's copy",
	"Spend paths":                         "screen title, menu row, computed summary or another program's copy",
	"Stamp BOTH stubs on each key card:":  "screen title, menu row, computed summary or another program's copy",
	"TEXT ONLY":                           "screen title, menu row, computed summary or another program's copy",
	"TYPE IT":                             "screen title, menu row, computed summary or another program's copy",
	"Template plus key cards":             "screen title, menu row, computed summary or another program's copy",
	"The policy itself":                   "screen title, menu row, computed summary or another program's copy",
	"This device cannot confirm a key was derived at the origin it declares.": "screen title, menu row, computed summary or another program's copy",
	"This engraves 1 plate.": "screen title, menu row, computed summary or another program's copy",
	"Type a seed":            "screen title, menu row, computed summary or another program's copy",
	"Verify off-device.":     "screen title, menu row, computed summary or another program's copy",
	"Watch-only (keys)":      "screen title, menu row, computed summary or another program's copy",
	"Which form?":            "screen title, menu row, computed summary or another program's copy",
	"Which method?":          "screen title, menu row, computed summary or another program's copy",
	"md1 template: 1 plate (key-less wallet policy)": "screen title, menu row, computed summary or another program's copy",
	"seed 1":                               "screen title, menu row, computed summary or another program's copy",
	"slots: 0 / keys available: 2":         "screen title, menu row, computed summary or another program's copy",
	"slots: 2 / keys available: 2":         "screen title, menu row, computed summary or another program's copy",
	"there is a template and nothing else": "screen title, menu row, computed summary or another program's copy",
}
