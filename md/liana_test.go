package md

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ─── SPEC §8.1 Go leg: the recipe against LIANA's own golden xpubs ───────────

type lianaCase struct {
	Name           string   `json:"name"`
	Accepted       bool     `json:"accepted"`
	LeafPubkeysHex []string `json:"leaf_pubkeys_hex"`
	ExpectedXpub   string   `json:"expected_xpub"`
}

// lianaCases loads the vendored evidence AND checks it against its provenance
// pin, so a hand-edited or stale copy is a failure rather than a silent pass.
func lianaCases(t *testing.T) []lianaCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "liana_cases.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %v -- run scripts/vendor-liana-cases.sh", err)
	}
	pinRaw, err := os.ReadFile(filepath.Join("testdata", "liana_cases.provenance.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin: %v", err)
	}
	var pin struct {
		Commit string `json:"commit"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(pinRaw, &pin); err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != pin.SHA256 || pin.Commit == "" {
		t.Fatalf("liana_cases.json disagrees with its pin (or the pin names no commit)")
	}
	var cs []lianaCase
	if err := json.Unmarshal(raw, &cs); err != nil {
		t.Fatal(err)
	}
	// Nine at descriptor-mnemonic cf35d61a: the eight fable-r0 evidence shapes
	// plus stage 2's nested ACCEPT. A count, not ">0": a truncated copy that
	// kept one case would otherwise pass.
	if len(cs) != 9 {
		t.Fatalf("liana_cases.json carries %d cases, want 9", len(cs))
	}
	return cs
}

func caseLeaves(t *testing.T, c lianaCase) [][33]byte {
	t.Helper()
	out := make([][33]byte, 0, len(c.LeafPubkeysHex))
	for _, h := range c.LeafPubkeysHex {
		b, err := hex.DecodeString(h)
		if err != nil || len(b) != 33 {
			t.Fatalf("%s: leaf pubkey %q: %v (len %d)", c.Name, h, err, len(b))
		}
		var pk [33]byte
		copy(pk[:], b)
		out = append(out, pk)
	}
	return out
}

func TestLianaRecipeReproducesEveryGoldenXpub(t *testing.T) {
	for _, c := range lianaCases(t) {
		want, err := parseExtendedKey(c.ExpectedXpub)
		if err != nil {
			t.Fatalf("%s: expected_xpub: %v", c.Name, err)
		}
		if want.depth != 0 || want.parentFP != 0 || want.child != 0 {
			t.Fatalf("%s: golden xpub is not depth 0 / parent 0 / child 0", c.Name)
		}
		if got := lianaUnspendableKey(caseLeaves(t, c)); got != want.material {
			t.Errorf("%s: recipe\n  go:     %x\n  liana:  %x", c.Name, got, want.material)
		}
	}
}

// Sorting or deduplicating is the PR-1746 recipe, a DIFFERENT xpub (SPEC §2).
// Asserted on real cases so the property is not vacuous: each case below has
// leaves that are not already in sorted order.
func TestLianaRecipeIsOrderSensitiveAndKeepsDuplicates(t *testing.T) {
	sortedDiffers := 0
	for _, c := range lianaCases(t) {
		leaves := caseLeaves(t, c)
		sorted := append([][33]byte(nil), leaves...)
		sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i][:], sorted[j][:]) < 0 })
		if !slicesEqual33(sorted, leaves) {
			if lianaUnspendableKey(sorted) == lianaUnspendableKey(leaves) {
				t.Errorf("%s: sorting the leaves did not change the key", c.Name)
			}
			sortedDiffers++
		}
	}
	if sortedDiffers == 0 {
		t.Fatal("every case is already sorted -- the order property was never exercised")
	}
	// Dedup: a synthetic duplicate. Two occurrences of one key hash differently
	// from one occurrence.
	one := caseLeaves(t, lianaCases(t)[0])[:1]
	two := append(append([][33]byte(nil), one...), one...)
	if lianaUnspendableKey(one) == lianaUnspendableKey(two) {
		t.Error("a duplicated leaf key hashed the same as a single one -- the recipe deduplicates")
	}
}

func slicesEqual33(a, b [][33]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
