package md

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Task 4: StripToTemplate golden-locked to toolkit bundle --md1-form=template ─
//
// Goldens generated from mnemonic-toolkit@6de53879 (v0.60.0, md-codec v0.37.0):
//   *.policy.md1.txt   = the FULL keyed wallet-policy md1 (strip INPUT)
//   *.tmpl.md1.txt     = the toolkit's keyless template md1 (expected OUTPUT)
// Both are emitted via md_codec::chunk::split (--group-size 0), the SAME wire
// dialect the fork's split() produces — so the comparison is byte-exact.

func loadTemplateMD1(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "template", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return strings.Fields(string(raw))
}

// stripGoldens pairs a strip-input policy md1 with its expected template md1 and
// whether canonical_origin elides the origin (true) or it is kept (false).
var stripGoldens = []struct {
	name         string
	policy, tmpl string
	originElided bool
}{
	{"wpkh single-sig (canonical → elided)", "wpkh.policy.md1.txt", "wpkh.tmpl.md1.txt", true},
	{"wsh-sortedmulti 2of2 (canonical → elided)", "wsh_sortedmulti.policy.md1.txt", "wsh_sortedmulti.tmpl.md1.txt", true},
	{"degrade2 11-key general (non-canonical → keep origins)", "degrade2_11key.policy.md1.txt", "degrade2_11key.tmpl.md1.txt", false},
}

func TestStripToTemplateGolden(t *testing.T) {
	for _, g := range stripGoldens {
		t.Run(g.name, func(t *testing.T) {
			policy := loadTemplateMD1(t, g.policy)
			wantTmpl := loadTemplateMD1(t, g.tmpl)

			got, err := StripToTemplate(policy)
			if err != nil {
				t.Fatalf("StripToTemplate: %v", err)
			}
			if len(got) != len(wantTmpl) {
				t.Fatalf("strip produced %d chunks, want %d\n got=%v\nwant=%v", len(got), len(wantTmpl), got, wantTmpl)
			}
			for i := range got {
				if got[i] != wantTmpl[i] {
					t.Fatalf("chunk %d mismatch:\n got=%s\nwant=%s", i, got[i], wantTmpl[i])
				}
			}

			// Decode the strip output and assert the keyless invariants + the
			// conditional-origin behaviour.
			d, err := Reassemble(got)
			if err != nil {
				t.Fatalf("Reassemble(strip output): %v", err)
			}
			if d.tlv.pubPresent || len(d.tlv.pubkeys) != 0 {
				t.Errorf("strip output still has pubkeys (pubPresent=%v len=%d)", d.tlv.pubPresent, len(d.tlv.pubkeys))
			}
			if d.tlv.fpPresent || len(d.tlv.fingerprints) != 0 {
				t.Errorf("strip output still has fingerprints (fpPresent=%v len=%d)", d.tlv.fpPresent, len(d.tlv.fingerprints))
			}
			if isWalletPolicy(d) {
				t.Error("strip output must be a keyless template, not a wallet-policy")
			}

			_, ok := canonicalOrigin(d.tree)
			if ok != g.originElided {
				t.Errorf("canonicalOrigin(tree) ok=%v, want %v", ok, g.originElided)
			}
			// C1: a non-canonical wrapper MUST keep explicit origins, else the
			// re-decode would have failed errMissingExplicitOrigin via the
			// validators. A successful Reassemble + the explicit-origin validator
			// proves the origins survived.
			if !g.originElided {
				if err := validateExplicitOriginRequired(d); err != nil {
					t.Errorf("non-canonical template dropped its origins (C1 regression): %v", err)
				}
			}
		})
	}
}

// TestTapTreeDepthChunks: a depth-≥2 taptree template (tr4 from the toolkit's
// examples-build, {{...},...}) reports depth 2 (the DD6 EXPERIMENTAL gate); a
// non-taproot template reports 0.
func TestTapTreeDepthChunks(t *testing.T) {
	tr4 := loadTemplateMD1(t, "tr4_depth2.tmpl.md1.txt")
	d, err := TapTreeDepthChunks(tr4)
	if err != nil {
		t.Fatalf("TapTreeDepthChunks(tr4): %v", err)
	}
	if d < 2 {
		t.Fatalf("tr4 depth = %d, want >= 2 (nested taptree)", d)
	}

	// A non-taproot template (wsh-sortedmulti) → 0.
	wsh := loadTemplateMD1(t, "wsh_sortedmulti.tmpl.md1.txt")
	d2, err := TapTreeDepthChunks(wsh)
	if err != nil {
		t.Fatalf("TapTreeDepthChunks(wsh): %v", err)
	}
	if d2 != 0 {
		t.Fatalf("wsh-sortedmulti depth = %d, want 0 (not taproot)", d2)
	}
}
