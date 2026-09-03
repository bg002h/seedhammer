package gui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// composerPresetVector is the vendored export of one archetype: the wrapper it
// was exported at, and the snake_case vector name the corpus stores it under.
type composerPresetVector struct {
	wrapper md.ComposeWrapper
	preset  string // the label composerPresets() offers
	vector  string // md/testdata/vectors/<vector>.*
}

// composerPinnedPresets is the six (wrapper, preset) pairs S0b exported a
// vector for -- five under wsh and kofn-recovery under tr.
//
// THE WRAPPER IS HALF OF EACH ASSERTION'S IDENTITY. A table iterated at one
// wrapper against vectors exported at another is a test that passes by
// comparing the wrong things, and it would pass loudly, so the wrapper is in
// the lookup AND in the sub-test name.
func composerPinnedPresets() []composerPresetVector {
	return []composerPresetVector{
		{md.ComposeWsh, "plain-multisig", "keyed_compose_preset_plain_multisig"},
		{md.ComposeWsh, "simple-timelocked-inheritance", "keyed_compose_preset_simple_timelocked_inheritance"},
		{md.ComposeWsh, "tiered-recovery", "keyed_compose_preset_tiered_recovery"},
		{md.ComposeWsh, "hashlock-gated", "keyed_compose_preset_hashlock_gated"},
		{md.ComposeWsh, "decaying-multisig", "keyed_compose_preset_decaying_multisig"},
		{md.ComposeTr, "kofn-recovery", "keyed_compose_preset_kofn_recovery"},
	}
}

// composerUnpinnedPresets is the OTHER six offered pairs. S0b exports one
// vector per ARCHETYPE, not one per pair, so these have no oracle and are
// checked structurally -- named here so a green run is never read as pinning
// twelve shapes. When a later export adds a pair's vector, that pair moves
// into the pinned test and out of this one.
func composerUnpinnedPresets() []composerPresetVector {
	return []composerPresetVector{
		{md.ComposeTr, "plain-multisig", ""},
		{md.ComposeTr, "simple-timelocked-inheritance", ""},
		{md.ComposeTr, "tiered-recovery", ""},
		{md.ComposeTr, "hashlock-gated", ""},
		{md.ComposeTr, "decaying-multisig", ""},
		{md.ComposeWsh, "kofn-recovery", ""},
	}
}

func composerWrapperName(w md.ComposeWrapper) string {
	switch w {
	case md.ComposeTr:
		return "tr"
	case md.ComposeWsh:
		return "wsh"
	case md.ComposeShWsh:
		return "sh-wsh"
	case md.ComposeSh:
		return "sh"
	}
	return fmt.Sprintf("wrapper(%d)", w)
}

// composerLookupPreset finds a preset by name in the table for ITS OWN
// wrapper. A miss is fatal: a renamed preset must fail, never silently assert
// nothing.
func composerLookupPreset(t *testing.T, w md.ComposeWrapper, name string) composerPreset {
	t.Helper()
	for _, p := range composerPresets(w) {
		if p.name == name {
			return p
		}
	}
	t.Fatalf("composerPresets(%s) offers no preset named %q", composerWrapperName(w), name)
	return composerPreset{}
}

func composerVectorPath(vector, ext string) string {
	return filepath.Join("..", "md", "testdata", "vectors", vector+"."+ext)
}

// composerLoadPhraseChunks reads a vendored .phrase.txt. Its leading
// `chunk-set-id:` line is a header, not a chunk.
func composerLoadPhraseChunks(t *testing.T, vector string) []string {
	t.Helper()
	p := composerVectorPath(vector, "phrase.txt")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no vendored vector at %s -- re-run "+
			"scripts/vendor-compose-vectors.sh (F-453/S0b): %v", p, err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		t.Fatalf("INCONCLUSIVE: %s holds no md1 chunk", p)
	}
	return out
}

// composerVectorTLV reads the keys and fingerprints the vector was bound with.
//
// The .phrase.txt is the KEYED chunk set, so reproducing it takes the same two
// steps md/compose_test.go's own parity test takes: Compose the unseated
// shape, then Bind the vector's keys and fingerprints, which is what the
// primary's MANIFEST binding did when the vector was made. Comparing an
// UNBOUND template's chunks against a keyed phrase would compare two different
// artifacts and fail for a reason that is not the shape.
func composerVectorTLV(t *testing.T, vector string) (map[uint8][65]byte, map[uint8][4]byte) {
	t.Helper()
	p := composerVectorPath(vector, "descriptor.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no vendored vector at %s -- re-run "+
			"scripts/vendor-compose-vectors.sh (F-453/S0b): %v", p, err)
	}
	var doc struct {
		TLV struct {
			Fingerprints [][2]json.RawMessage `json:"fingerprints"`
			Pubkeys      [][2]json.RawMessage `json:"pubkeys"`
		} `json:"tlv"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", p, err)
	}
	decode := func(idxRaw, hexRaw json.RawMessage, want int) (uint8, []byte) {
		var idx uint8
		if err := json.Unmarshal(idxRaw, &idx); err != nil {
			t.Fatalf("%s: slot index %s: %v", p, idxRaw, err)
		}
		var s string
		if err := json.Unmarshal(hexRaw, &s); err != nil {
			t.Fatalf("%s: value for slot %d: %v", p, idx, err)
		}
		b, err := hex.DecodeString(s)
		if err != nil || len(b) != want {
			t.Fatalf("%s: slot %d holds %d bytes (want %d): %v", p, idx, len(b), want, err)
		}
		return idx, b
	}
	pubkeys := map[uint8][65]byte{}
	for _, row := range doc.TLV.Pubkeys {
		idx, b := decode(row[0], row[1], 65)
		var k [65]byte
		copy(k[:], b)
		pubkeys[idx] = k
	}
	fps := map[uint8][4]byte{}
	for _, row := range doc.TLV.Fingerprints {
		idx, b := decode(row[0], row[1], 4)
		var f [4]byte
		copy(f[:], b)
		fps[idx] = f
	}
	if len(pubkeys) == 0 {
		t.Fatalf("INCONCLUSIVE: %s is a keyed vector carrying no pubkeys", p)
	}
	return pubkeys, fps
}

// TestComposerPresetsReproduceTheirVendoredVectors is the Rust-primary gate on
// §4d. A preset is a normative PathList shape, so the Go table is checked
// against the primary's OWN chunks rather than against this test's idea of the
// shape.
//
// IF A PRESET'S CHUNKS DIFFER, THE VECTOR WINS AND THE GO TABLE CHANGES. If
// the vector itself looks wrong, stop and record it: the fix lands in Rust
// first, with a vector, and only then is ported here.
func TestComposerPresetsReproduceTheirVendoredVectors(t *testing.T) {
	for _, v := range composerPinnedPresets() {
		t.Run(composerWrapperName(v.wrapper)+"/"+v.preset, func(t *testing.T) {
			p := composerLookupPreset(t, v.wrapper, v.preset)
			if p.list.Wrapper != v.wrapper {
				t.Fatalf("preset %q built for wrapper %s, not %s", p.name,
					composerWrapperName(p.list.Wrapper), composerWrapperName(v.wrapper))
			}
			c, err := md.Compose(p.list)
			if err != nil {
				t.Fatalf("md.Compose(%s/%s): %v", composerWrapperName(v.wrapper), v.preset, err)
			}
			pubkeys, fps := composerVectorTLV(t, v.vector)
			if err := c.Bind(pubkeys, fps); err != nil {
				t.Fatalf("Bind %s: %v", v.vector, err)
			}
			got, err := c.Chunks()
			if err != nil {
				t.Fatalf("Chunks: %v", err)
			}
			want := composerLoadPhraseChunks(t, v.vector)
			if len(got) != len(want) {
				t.Fatalf("%s: %d chunks, the vector has %d:\n got %v\nwant %v",
					v.vector, len(got), len(want), got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("%s chunk %d differs -- the VECTOR is right and the Go "+
						"preset table is wrong:\n got  %s\n want %s",
						v.vector, i, got[i], want[i])
				}
			}
		})
	}
}

// TestComposerPresetsWithoutAVectorAreStructurallyValid covers the six offered
// pairs that have NO exported vector, and asserts only what there is an oracle
// for. These pairs are UNPINNED and this test says so, so a green run never
// reads as pinning twelve shapes.
func TestComposerPresetsWithoutAVectorAreStructurallyValid(t *testing.T) {
	for _, v := range composerUnpinnedPresets() {
		t.Run(composerWrapperName(v.wrapper)+"/"+v.preset, func(t *testing.T) {
			p := composerLookupPreset(t, v.wrapper, v.preset)
			if _, err := md.ValidatePathList(p.list); err != nil {
				t.Fatalf("md.ValidatePathList refuses the %s/%s preset: %v",
					composerWrapperName(v.wrapper), v.preset, err)
			}
			c, err := md.Compose(p.list)
			if err != nil {
				t.Fatalf("md.Compose(%s/%s): %v", composerWrapperName(v.wrapper), v.preset, err)
			}
			chunks, err := c.Chunks()
			if err != nil {
				t.Fatalf("Chunks: %v", err)
			}
			if len(chunks) == 0 {
				t.Errorf("the %s/%s preset composes to no chunks",
					composerWrapperName(v.wrapper), v.preset)
			}
		})
	}
}

// TestComposerPresetsUnderLegacyWrappersOfferOnlyPlainKofN is §4d's last
// clause. Offering a shape sh or sh(wsh) refuses would turn the menu into a
// refusal the operator meets only after choosing.
func TestComposerPresetsUnderLegacyWrappersOfferOnlyPlainKofN(t *testing.T) {
	for _, w := range []md.ComposeWrapper{md.ComposeSh, md.ComposeShWsh} {
		got := composerPresets(w)
		if len(got) != 1 {
			t.Fatalf("composerPresets(%s) offers %d presets, §4d admits one",
				composerWrapperName(w), len(got))
		}
		if got[0].name != "plain-multisig" {
			t.Errorf("composerPresets(%s) offers %q, want plain-multisig",
				composerWrapperName(w), got[0].name)
		}
		if _, err := md.ValidatePathList(got[0].list); err != nil {
			t.Errorf("the one %s preset is refused by md.ValidatePathList: %v",
				composerWrapperName(w), err)
		}
	}
}

// TestComposerPresetLabelsFitTheirRows: ChoiceScreen draws rows with
// widget.Label, which does not wrap, so a long label is drawn off the panel
// and the operator picks a truncated option.
func TestComposerPresetLabelsFitTheirRows(t *testing.T) {
	seen := map[string]bool{}
	for _, w := range []md.ComposeWrapper{md.ComposeTr, md.ComposeWsh, md.ComposeShWsh, md.ComposeSh} {
		for _, p := range composerPresets(w) {
			if seen[p.name] {
				continue
			}
			seen[p.name] = true
			assertChoiceLabelFits(t, p.name)
		}
	}
	if len(seen) != 6 {
		t.Errorf("the four wrappers offer %d distinct preset names, §4d names six", len(seen))
	}
}
