package md

import (
	"os"
	"path/filepath"
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

// pinnedKeyReuseVectors are the vector names carrying a fork-side pin.
var pinnedKeyReuseVectors = []string{
	"keyed_tr_multi_a",
	"keyed_tr_sortedmulti_a",
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

// TestPinnedKeyReuseVectorsStillMatchTheVendoredCorpus is the anti-drift half.
//
// While the vendored vector is still there, the pin must be exactly it — a pin
// that has quietly diverged is a fixture testing a policy the primary never
// shipped. Once the re-vendor F-529 describes removes the vendored file, the
// pin becomes the only copy and this test says so rather than failing: that is
// the outcome the pin exists to produce, not a regression.
//
// MUTATION: change one character of a pinned md1 string and the matching
// subtest fails while the vendored file is present.
func TestPinnedKeyReuseVectorsStillMatchTheVendoredCorpus(t *testing.T) {
	for _, name := range pinnedKeyReuseVectors {
		t.Run(name, func(t *testing.T) {
			pin, ok := pinnedChunks(name)
			if !ok {
				t.Fatalf("no pin at testdata/forkbuilt/%s.md1.txt; F-533's refusal has "+
					"no fork-side witness left", name)
			}
			if len(pin) == 0 {
				t.Fatalf("the pin for %s carries no md1 strings", name)
			}
			raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".phrase.txt"))
			if err != nil {
				t.Skipf("the vendored %s is gone (%v); the pin is now the only copy, "+
					"which is exactly what it is for", name, err)
			}
			vendored := md1Lines(string(raw))
			if len(vendored) != len(pin) {
				t.Fatalf("pin has %d chunks, vendored has %d", len(pin), len(vendored))
			}
			for i := range pin {
				if pin[i] != vendored[i] {
					t.Fatalf("chunk %d has drifted:\n pin      %s\n vendored %s\n"+
						"Re-copy the vendored phrase into testdata/forkbuilt/%s.md1.txt "+
						"after checking WHY the primary changed it", i, pin[i], vendored[i], name)
				}
			}
		})
	}
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
			if b.isNums {
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
