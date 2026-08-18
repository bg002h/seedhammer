package gui

import (
	"slices"
	"strings"
	"testing"

	"seedhammer.com/bspline"
)

// ─── S6b GATE 1.2: the marking renders in every offered variant, and does
// not change which variants are offered ───────────────────────────────────

// TestValidateMdmkMarkingRendersInEveryVariant is GATE 1.2 (spec S6b §1.2,
// R-F's required gate): the offered variant SET is pinned for a representative
// md1 and mk1, title empty and set -- so a title/footer band cannot silently
// drop a variant, which is exactly what backup/backup.go's own comment warns
// about (TEXT+QR is the tightest fit) -- and the marking renders in EACH
// offered variant, QR ONLY included: title and footer are plate rows, not
// paragraph content.
func TestValidateMdmkMarkingRendersInEveryVariant(t *testing.T) {
	const (
		md1Str = "md1yqpqqxqq8xtwhw4xwn4qh"
		mk1Str = "mk1yqpqqxqq8xtwhw4xwn4qh"
	)
	for _, s := range []string{md1Str, mk1Str} {
		p := newPlatform()
		labelsEmpty, platesEmpty, err := validateMdmk(p, s, "", "")
		if err != nil {
			t.Fatalf("%s: unmarked validateMdmk: %v", s, err)
		}
		labelsMarked, platesMarked, err := validateMdmk(p, s, "PASSWORD REQUIRED", "COMB FP: FC60 C6DF")
		if err != nil {
			t.Fatalf("%s: marked validateMdmk: %v", s, err)
		}
		if !slices.Equal(labelsEmpty, labelsMarked) {
			t.Errorf("%s: marking changed the offered variant set: %v -> %v", s, labelsEmpty, labelsMarked)
		}
		if !slices.Contains(labelsMarked, "QR ONLY") {
			t.Fatalf("%s: QR ONLY not offered; this gate needs it in the set to prove title/footer render there too", s)
		}
		if len(platesMarked) != len(platesEmpty) {
			t.Fatalf("%s: %d marked plates vs %d unmarked", s, len(platesMarked), len(platesEmpty))
		}
		ink := func(pl Plate) bspline.Bounds { return bspline.Measure(pl.Spline).Bounds }
		for i, label := range labelsMarked {
			if ink(platesMarked[i]) == ink(platesEmpty[i]) {
				t.Errorf("%s %s: marked and unmarked plates ink identically; the title/footer did not render", s, label)
			}
		}
	}
}

// TestSingleSigPlateMark pins spec S6b §1.2's exact strings for all four
// (full, hasPassphrase) combinations -- the composition engraveSingleSigFlow (gui/singlesig.go)
// calls, upstream of validateMdmk. R-A's predicate ("the set contains a
// seed") is exactly `full` for this flow: watch-only never carries the ms1.
func TestSingleSigPlateMark(t *testing.T) {
	for _, tc := range []struct {
		full, hasPassphrase   bool
		wantTitle, wantFooter string
	}{
		{full: true, hasPassphrase: true, wantTitle: "PASSWORD REQUIRED", wantFooter: "COMB FP: FC60 C6DF"},
		{full: true, hasPassphrase: false, wantTitle: "", wantFooter: "SEED FP: FC60 C6DF"},
		{full: false, hasPassphrase: true, wantTitle: "", wantFooter: ""},
		{full: false, hasPassphrase: false, wantTitle: "", wantFooter: ""},
	} {
		title, footer := singleSigPlateMark(tc.full, tc.hasPassphrase, 0xFC60C6DF)
		if title != tc.wantTitle || footer != tc.wantFooter {
			t.Errorf("full=%v hasPassphrase=%v: got (%q, %q), want (%q, %q)",
				tc.full, tc.hasPassphrase, title, footer, tc.wantTitle, tc.wantFooter)
		}
	}
}

// ─── S6b GATE 1.3: only the single-sig engrave marks ─────────────────────

// TestBundlePlateMarkSuppressesMS1 is GATE 1.3's mechanism: bundleEngrave
// applies the title/footer marking to every plate in its plan EXCEPT a
// cardMS1's (spec S6b §1.2 -- "A cardMS1 plate is never marked" -- and §1.3's
// mechanism note). A secret ms1 share must never carry a wallet's
// fingerprint: it would tie a secret to a specific wallet on an artifact
// whose whole design posture is that it never leaves owner-held steel.
func TestBundlePlateMarkSuppressesMS1(t *testing.T) {
	const title, footer = "PASSWORD REQUIRED", "COMB FP: FC60 C6DF"
	for _, kind := range []bundleCardKind{cardMK1, cardMD1} {
		gotTitle, gotFooter := bundlePlateMark(kind, title, footer)
		if gotTitle != title || gotFooter != footer {
			t.Errorf("kind %v: marking suppressed (%q, %q), want passed through", kind, gotTitle, gotFooter)
		}
	}
	gotTitle, gotFooter := bundlePlateMark(cardMS1, title, footer)
	if gotTitle != "" || gotFooter != "" {
		t.Errorf("cardMS1: marked (%q, %q), want both empty -- an ms1 plate must never be marked", gotTitle, gotFooter)
	}
}

// TestOnlySingleSigMarksPlates is GATE 1.3's other half, a SOURCE fact: spec
// S6b §1.3's table names every validateMdmk / bundleEngrave call path, and
// only engraveSingleSigFlow (gui/singlesig.go) passes non-empty marking. Asserted on raw source,
// the same idiom TestBothEngraveFlowsGateOnACompletedSet
// (multisig_verify_report_test.go) and
// TestMs1ReminderIsTitledForTheProgramThatShowedIt (bundle_abort_prose_test.go)
// already use in this package, because none of these flows is driven by a
// unit test here.
func TestOnlySingleSigMarksPlates(t *testing.T) {
	for file, want := range map[string]string{
		"gui.go":              `validateMdmk(ctx.Platform, str, "", "")`,                      // mdmkFlow -- not decidable from a scanned string
		"unlock_platelist.go": `validateMdmk(ctx.Platform, str, "", "")`,                      // scanned record
		"derive_xpub.go":      `validateMdmk(ctx.Platform, s, "", "")`,                        // F-205's flow, R-B
		"multisig.go":         `bundleEngrave(ctx, th, "Engrave Multisig", cardsOut, "", "")`, // R-B
		"multisig_build.go":   `bundleEngrave(ctx, th, "Build Policy", cardsOut, "", "")`,     // R-B
		"bundle_flow.go":      `bundleEngrave(ctx, th, "Engrave Bundle", cards, "", "")`,      // scanned, no derivation
	} {
		src := readGuiFile(t, file)
		if !strings.Contains(src, want) {
			t.Errorf("%s: source does not contain the unmarked call %q; GATE 1.3 (only single-sig marks) may have been crossed", file, want)
		}
	}
	// The one caller that DOES mark must not pass an empty marking, or
	// R2/R3/R4 would never render.
	src := readGuiFile(t, "singlesig.go")
	if !strings.Contains(src, `bundleEngrave(ctx, th, "Engrave Single-Sig", cards, `) {
		t.Fatalf("singlesig.go's bundleEngrave call not found in the shape this gate expects; the table is stale")
	}
	if strings.Contains(src, `bundleEngrave(ctx, th, "Engrave Single-Sig", cards, "", "")`) {
		t.Error("singlesig.go passes empty marking to bundleEngrave; R2/R3/R4 would never render")
	}
}
