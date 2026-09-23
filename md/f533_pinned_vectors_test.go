package md

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ─── F-529: the two taproot key-reuse vectors, pinned fork-side ──────────────
//
// `keyed_tr_multi_a` and `keyed_tr_sortedmulti_a` are two of the three
// key-reuse vectors F-529 records a re-vendor from the primary would DELETE.
// They are the whole evidence for F-533's refusal — tr(@0, multi_a(2,@0,@1)):
// one slot at the taproot internal key and again inside a leaf — so a re-vendor
// would take the predicate's only vendored witnesses with it and leave a
// refusal nobody can reproduce.
//
// So a fork-side copy of each md1 chunk set lives in md/testdata/forkbuilt/,
// beside the F-531 repeated-seat fixtures, and the loaders below read the PIN
// in preference to the vendored file. Nothing here waits on F-529 in either
// direction: if the re-vendor lands the tests keep their fixtures, and until it
// does the gate below proves the pin is still byte-for-byte what the primary
// ships.
//
// THE PIN IS NOT A GENERATOR, and that is the difference from
// dup_seat_fixture_test.go. That fixture's shape cannot be produced by any
// shipped encoder, so its anti-decay gate rebuilds it from the tree. These two
// CAN be produced by the primary and were; the drift that matters is the pin
// silently diverging from the vendored corpus, which is what this compares.

// pinnedKeyReuseVectors are the TAPROOT internal-key reuse pins.
var pinnedKeyReuseVectors = []string{
	"keyed_tr_multi_a",
	"keyed_tr_sortedmulti_a",
}

// pinnedDuplicateVectors are the MINISCRIPT duplicate pins: one slot at two use
// sites inside a single wsh expression, which is the shape Core itself refuses
// ("... is not sane: contains duplicate public keys").
//
// A SEPARATE LIST because it is a different shape with a different authority,
// and TestPinnedKeyReuseVectorsAreTheShapeTheyClaim cannot cover it: that test
// asserts a taproot internal key that is a placeholder and repeats inside the
// taptree, and this policy is not a taproot policy at all. Folding the name
// into pinnedKeyReuseVectors would make that test fail on the trBody assertion;
// leaving it in no list at all is how a pin ends up measured by nothing.
var pinnedDuplicateVectors = []string{
	"keyed_wsh_timelock_hashlock",
}

// allPinnedVectors is every name carrying a fork-side card pin, and is what the
// anti-drift check below runs over.
func allPinnedVectors() []string {
	return append(append([]string(nil), pinnedKeyReuseVectors...), pinnedDuplicateVectors...)
}

// primaryMovedTo records, per pinned name, the EXACT wallet_descriptor_template_id
// of the policy descriptor-mnemonic b2c5d693 replaced the pinned one with.
//
// F-529's premise was that a re-vendor DELETES these three vectors. Measured,
// it does not: b2c5d693 still ships all three, CHANGED -- reuse-bearing
// policies replaced by reuse-free ones. So the anti-drift check's "the vendored
// file is gone" skip never fires, and without this table the check simply fails
// on a divergence that is expected and understood.
//
// The full 32-hex id, not an 8-char prefix: this is the one thing standing
// between "the primary moved to the policy we know about" and "the primary
// moved somewhere else entirely", and a prefix is a materially weaker pin than
// "an exact shape" promises. Any THIRD policy under these names fails here.
var primaryMovedTo = map[string]string{
	"keyed_tr_multi_a":            "8c1c05666abdf6b29df9c2056c0cb49e",
	"keyed_tr_sortedmulti_a":      "09903620dbcf4e059300f23391079052",
	"keyed_wsh_timelock_hashlock": "71ff3b7424b35d410c16b6d992aa437e",
}

// pinnedChunks reads a fork-side pin, or !ok when the name has none.
func pinnedChunks(name string) ([]string, bool) {
	raw, err := os.ReadFile(filepath.Join("testdata", "forkbuilt", name+".md1.txt"))
	if err != nil {
		return nil, false
	}
	return md1Lines(string(raw)), true
}

// md1Lines pulls the md1 strings out of a phrase or pin file, dropping the
// `chunk-set-id:` header the vendored form carries and the spaces a phrase is
// grouped with.
func md1Lines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	return out
}

// TestPinnedVectorsAgreeWithTheVendoredCorpusOrWithARecordedMove is the
// anti-drift half, reworked for what the re-vendor actually did (F-630 D6).
//
// IT USED TO SKIP when the vendored file was GONE, on F-529's premise that a
// re-vendor would delete these three vectors. Measured at descriptor-mnemonic
// b2c5d693: all three are still shipped, CHANGED -- the reuse-BEARING policies
// were replaced by reuse-FREE ones. So the skip never fires and the old test
// fails instead, on a divergence that is expected. Worse, its failure message
// prescribed the destructive remedy ("re-copy the vendored phrase"), which
// would overwrite F-533's and F-514's only witnesses with policies that no
// longer carry the reuse under test.
//
// The check is therefore: the pin is byte-identical to the vendored card, OR
// the vendored record is EXACTLY the policy we recorded the primary moving to.
// A pinned gap with an exact shape -- a third policy under these names fails.
//
// THIS FILE DELIBERATELY READS BOTH TIERS, and is the one exception to the
// pairing rule in md/vector_fixtures_test.go: comparing them IS the job. It
// reads the vendored record through os.ReadFile rather than vectorRecordFor for
// exactly that reason -- vectorRecordFor would hand back the pin and the
// comparison would be with itself.
//
// MUTATION: change one character of a pinned md1 string and the matching
// subtest fails, because the pin then matches neither the vendored card nor
// (through it) the recorded move.
func TestPinnedVectorsAgreeWithTheVendoredCorpusOrWithARecordedMove(t *testing.T) {
	for _, name := range allPinnedVectors() {
		t.Run(name, func(t *testing.T) {
			pin, ok := pinnedChunks(name)
			if !ok {
				t.Fatalf("no pin at testdata/forkbuilt/%s.md1.txt; the fork-side witness "+
					"for this policy is gone", name)
			}
			if len(pin) == 0 {
				t.Fatalf("the pin for %s carries no md1 strings", name)
			}
			raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".phrase.txt"))
			if err != nil {
				t.Skipf("the vendored %s is gone (%v); the pin is now the only copy, "+
					"which is exactly what it is for", name, err)
			}
			if vendored := md1Lines(string(raw)); slices.Equal(pin, vendored) {
				return // still the same policy on both sides
			}

			// They differ. That is ALLOWED only where we recorded which policy
			// the primary moved to, and only for that exact policy.
			want, recorded := primaryMovedTo[name]
			if !recorded {
				t.Fatalf("the pin and the vendored %s have diverged and no move is recorded "+
					"for this name. Find out WHY the primary changed it before touching "+
					"either copy: the pin is a witness for a refusal, and overwriting it "+
					"with the vendored card destroys the evidence", name)
			}
			recRaw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".conformance.json"))
			if err != nil {
				t.Fatalf("the pin and the vendored %s have diverged, and the vendored "+
					"record that would identify the new policy is unreadable: %v", name, err)
			}
			var rec struct {
				TemplateID string `json:"wallet_descriptor_template_id"`
			}
			if err := json.Unmarshal(recRaw, &rec); err != nil {
				t.Fatalf("parse vendored %s.conformance.json: %v", name, err)
			}
			if rec.TemplateID != want {
				t.Fatalf("the pin and the vendored %s have diverged, and the vendored policy "+
					"is not the one we recorded the primary moving to:\n"+
					"  vendored wallet_descriptor_template_id %s\n"+
					"  recorded move                          %s\n"+
					"A THIRD policy under this name means the primary moved again. Record the "+
					"new id here only after establishing that the pinned witness is still the "+
					"shape its tests claim", name, rec.TemplateID, want)
			}
		})
	}
}

// TestPinnedDuplicateVectorsAreTheShapeTheyClaim measures the MINISCRIPT pin,
// the same way TestPinnedKeyReuseVectorsAreTheShapeTheyClaim measures the
// taproot ones: a slot at two use sites inside one wsh expression.
//
// Without it keyed_wsh_timelock_hashlock's pin is the only one whose shape
// nothing asserts -- the taproot shape test cannot cover it (it is not a
// taproot policy) and a pin of anything else would leave F-514's duplicate
// warning untested while looking covered.
func TestPinnedDuplicateVectorsAreTheShapeTheyClaim(t *testing.T) {
	for _, name := range pinnedDuplicateVectors {
		t.Run(name, func(t *testing.T) {
			chunks, ok := pinnedChunks(name)
			if !ok {
				t.Fatalf("no pin for %s", name)
			}
			d, err := Reassemble(chunks)
			if err != nil {
				t.Fatalf("Reassemble: %v", err)
			}
			if _, isTr := d.tree.body.(trBody); isTr {
				t.Fatal("the pin is a TAPROOT policy; a taproot internal key sits OUTSIDE " +
					"the miniscript, so it is the other shape and the other test's subject")
			}
			if d.tree.tag != tagWsh {
				t.Fatalf("the pin's outer script is %v, not wsh; Core's duplicate refusal is "+
					"about one slot repeating inside one WSH/SH miniscript", d.tree.tag)
			}
			var counts [256]int
			countKeySlots(d.tree, &counts)
			var repeated []int
			for slot, n := range counts {
				if n > 1 {
					repeated = append(repeated, slot)
				}
			}
			if len(repeated) == 0 {
				t.Fatalf("no slot appears twice anywhere under the wsh; this pin does not "+
					"carry the duplicate it exists to preserve (slots seen: %d)",
					countNonZero(counts))
			}
			t.Logf("slots at two or more use sites under one wsh miniscript: @%v", repeated)
		})
	}
}

func countNonZero(counts [256]int) int {
	n := 0
	for _, c := range counts {
		if c > 0 {
			n++
		}
	}
	return n
}

// TestPinnedKeyReuseVectorsAreTheShapeTheyClaim measures the pin rather than
// trusting its name: a taproot internal key that is a PLACEHOLDER (not NUMS)
// and appears again inside the taptree. That is the shape F-533 refuses, and a
// pin of anything else would leave the refusal untested while looking covered.
func TestPinnedKeyReuseVectorsAreTheShapeTheyClaim(t *testing.T) {
	for _, name := range pinnedKeyReuseVectors {
		t.Run(name, func(t *testing.T) {
			chunks, ok := pinnedChunks(name)
			if !ok {
				t.Fatalf("no pin for %s", name)
			}
			d, err := Reassemble(chunks)
			if err != nil {
				t.Fatalf("Reassemble: %v", err)
			}
			b, isTr := d.tree.body.(trBody)
			if !isTr {
				t.Fatalf("the pin is not a taproot policy (%T); F-533 is a taproot rule",
					d.tree.body)
			}
			if b.isNums() {
				t.Fatal("the pin's internal key is the NUMS H-point, so there is no " +
					"placeholder to reuse and the shape is not the one F-533 refuses")
			}
			if b.tree == nil {
				t.Fatal("the pin has no taptree, so the internal key cannot repeat in one")
			}
			var counts [256]int
			countKeySlots(*b.tree, &counts)
			if counts[b.keyIndex] == 0 {
				t.Fatalf("slot @%d is the internal key and appears nowhere in the taptree; "+
					"this pin does not carry the reuse", b.keyIndex)
			}
		})
	}
}

// ─── The forkbuilt directory is a CLOSED, HASHED set ────────────────────────
//
// Two gaps the whole-diff review reproduced, both the same shape: a pin with
// no integrity check is a fixture anyone can edit into something else.
//
//  1. NOTHING HASHED forkbuilt/*.conformance.json. Editing a pinned record's
//     `template` and `descriptor` together turns it into a DIFFERENT POLICY
//     and every gate stays green -- D1' compares the two to each other, so a
//     consistent edit satisfies it. The identical edit to a VENDORED record
//     reds immediately, because the provenance pin hashes that tier. The
//     pinned tier had no such backstop.
//  2. THE CARD TIER'S MEMBERSHIP WAS INFERRED FROM A FILE EXISTING -- exactly
//     what D5g forbids for records, and for the same reason. A smuggled
//     forkbuilt/<name>.md1.txt was accepted in silence by both packages.
//
// So: this set is exhaustive and hashed. Changing a pin is a deliberate,
// visible edit to this table, which is what makes `git diff` mean something
// for fixtures nobody upstream will ever correct.
//
// TO RE-PIN (only when the change is intended and explained in the commit):
//
//	for f in md/testdata/forkbuilt/*; do echo "$(basename $f) $(sha256sum $f | cut -d' ' -f1)"; done
var forkbuiltPinnedFiles = map[string]string{
	"dup_seat_wsh_sortedmulti_k1.md1.txt":          "bfe49796dbf8a4c7f6440fce2df85d7a2b6582b69ddbdea34fc0589da25cfeca",
	"dup_seat_wsh_sortedmulti_k1_keyless.md1.txt":  "b35e1b0c2f67c67567a2c5d90524057bf31b54e87cc49375d6930aa5b8872b4b",
	"dup_seat_wsh_sortedmulti_k2.md1.txt":          "b836b38ca08dd250f36753fa6ffc216963f93b32e18f5db91027ab37e81013d4",
	"keyed_tr_multi_a.conformance.json":            "bf42efab0146c4baeb8de51f814bae3e8168821f9e22f1b65720f048cc7a67c3",
	"keyed_tr_multi_a.md1.txt":                     "65af36d6406285aef3a38678b1326ffee5d70b9d5b8fdc606d6c097060fada37",
	"keyed_tr_sortedmulti_a.conformance.json":      "2c8757048e1452d4cb4c2772fab4020e963dc2a2139274af8ab5e2828b97ead0",
	"keyed_tr_sortedmulti_a.md1.txt":               "93c8cdf9f937be33204f0c60e67f6e28b6cd87ea089f018e1eee12333d20b258",
	"keyed_wsh_timelock_hashlock.conformance.json": "4474e8a642d12e6c31da529bf804fa6fc8e4fd21dad32a9a71a46d354138f6e0",
	"keyed_wsh_timelock_hashlock.md1.txt":          "109ce8c78128e0162648da1da53ce151611990e642f9fc952bb8ba5aeeb9ad08",
}

func TestForkbuiltPinsAreAClosedHashedSet(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "forkbuilt"))
	if err != nil {
		t.Fatalf("read forkbuilt: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		seen[name] = true
		want, listed := forkbuiltPinnedFiles[name]
		if !listed {
			t.Errorf("testdata/forkbuilt/%s is on disk but not in forkbuiltPinnedFiles. A "+
				"fork-side pin is a fixture with no upstream custodian, so an unlisted one "+
				"is either a smuggled fixture or a pin somebody forgot to record", name)
			continue
		}
		body, err := os.ReadFile(filepath.Join("testdata", "forkbuilt", name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != want {
			t.Errorf("testdata/forkbuilt/%s has CHANGED\n  on disk %s\n  pinned  %s\n"+
				"Nothing upstream will ever correct this file, so an unexplained edit to it "+
				"is indistinguishable from a fixture quietly becoming a different policy", name, got, want)
		}
	}
	for name := range forkbuiltPinnedFiles {
		if !seen[name] {
			t.Errorf("forkbuiltPinnedFiles names testdata/forkbuilt/%s, which is GONE", name)
		}
	}
	if len(forkbuiltPinnedFiles) == 0 {
		t.Fatal("INCONCLUSIVE: the pin table is empty, so this gate asserts nothing")
	}
}
