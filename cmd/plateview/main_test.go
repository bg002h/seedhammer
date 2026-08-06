package main

import (
	"fmt"
	"strings"
	"testing"

	"seedhammer.com/gui"
)

// The listing is what a preview IS for anyone reading it in a terminal, and it
// has its own strings: gui's readouts print "0.0mm" when they get a mixed plate
// wrong, and this one prints "fixed layout". A test written against the device's
// wording would pass here however wrong this file is.

// TestSizeLabelNamesTheRungsOfALadder is spec 7.18 at the site that renders it.
// A size ladder has no valid Preview.SizeMM -- it is 0 -- so the zero branch
// would call the plate a "fixed layout", which is the same defect as "0.0mm"
// in this tool's own words.
func TestSizeLabelNamesTheRungsOfALadder(t *testing.T) {
	for _, tc := range []struct {
		plate string
		want  string
	}{
		{"sizeproof-front", "5.0-3.8mm"},
		{"sizeproof-back", "4.4-3.0mm"},
	} {
		p, err := gui.BuildPreview(engraverParams, tc.plate, gui.PreviewOpts{})
		if err != nil {
			t.Fatalf("%s: %v", tc.plate, err)
		}
		got := sizeLabel(p)
		if got != tc.want {
			t.Errorf("%s: sizeLabel = %q, want %q", tc.plate, got, tc.want)
		}
		if got == "fixed layout" {
			t.Errorf("%s: a size ladder is reported as a fixed layout", tc.plate)
		}
		if strings.Contains(got, "0.0mm") {
			t.Errorf("%s: sizeLabel = %q", tc.plate, got)
		}
		// And the listing carries the SIZE of every row beside its face: the
		// same face is cut at two rungs on one plate, so neither alone says
		// what a row is.
		var sb strings.Builder
		describe(&sb, tc.plate, p)
		out := sb.String()
		if len(p.Sizes) != len(p.Rows) {
			t.Fatalf("%s: %d sizes against %d rows", tc.plate, len(p.Sizes), len(p.Rows))
		}
		for i, s := range p.Sizes {
			if want := fmt.Sprintf("%4.1f]", s); !strings.Contains(out, want) {
				t.Errorf("%s: row %d is cut at %.1fmm and the listing never says so:\n%s",
					tc.plate, i, s, out)
			}
		}
	}
}

// TestSizeLabelIsUnchangedForEveryOtherPlate: the range branch is taken FIRST,
// so it has to be inert on a plate whose rows are all one size -- and "fixed
// layout" has to survive for a plate the free-text fitter never lays out at all.
func TestSizeLabelIsUnchangedForEveryOtherPlate(t *testing.T) {
	for _, name := range gui.PreviewPlates() {
		if strings.HasPrefix(name, "sizeproof-") {
			continue
		}
		p, err := gui.BuildPreview(engraverParams, name, gui.PreviewOpts{Text: "a note"})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		want := "fixed layout"
		if p.SizeMM != 0 {
			want = fmt.Sprintf("%.1fmm", p.SizeMM)
		}
		if got := sizeLabel(p); got != want {
			t.Errorf("%s: sizeLabel = %q, want %q", name, got, want)
		}
	}
	// The seed plate is the "fixed layout" case, and it must really be
	// reachable or that branch is dead.
	seed, err := gui.BuildPreview(engraverParams, "seed", gui.PreviewOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := sizeLabel(seed); got != "fixed layout" {
		t.Errorf("the seed plate reports %q, want %q", got, "fixed layout")
	}
}
