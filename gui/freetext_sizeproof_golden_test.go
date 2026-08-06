package gui

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/backup"
	"seedhammer.com/bspline"
	"seedhammer.com/internal/golden"
)

// update re-records the goldens in this package. Scope it with -run: a bare
// `go test ./... -update` also rewrites backup's sixteen, and those are frozen.
var update = flag.Bool("update", false, "update golden files")

// The two SIZEPROOF! plates as GOLDENS: what closes font/constant's glyph
// IDENTITY gap.
//
// The rules that were already there catch a wrong ADVANCE
// (TestUniformAdvance), a glyph outside its cell (TestGlyphsStayInTheirCell),
// a glyph starting at the origin (TestNoGlyphStartsAtOrigin) and a glyph
// missing altogether (TestPrintableASCIICoverage). Not one of them catches a
// shape that is simply WRONG while staying in its cell with the right advance:
// 'a' drawn as a circle, 'm' missing a leg, '3' reversed all pass. The hazard is
// neither hypothetical nor rare -- font/constant is a B-SPLINE, so control
// points are not interpolated and an inserted "collinear" midpoint changes the
// rendered curve. That is how '#' grew a spurious diagonal and '$'s bowl
// collapsed to a triangle, with every mechanical test green on both.
//
// WHAT WAS ALREADY PINNED, and what was not. MEASURED 2026-08-05 by mutating one
// glyph at a time, regenerating constant.bin and running the whole tree -- not
// read off the test names, which is how the gap was overstated before:
//
//	glyph                    caught before these goldens existed
//	-----------------------  ------------------------------------------------
//	Q  uppercase             engrave.TestConstantFont, and backup's seed,
//	                         codex32 and slip39 plate goldens
//	a  lowercase             backup.TestPassphraseGolden, and nothing else
//	~  asciitilde            NOTHING IN THE TREE
//	{  leftcurlybrace        NOTHING IN THE TREE
//
// The structure behind that table: engrave/testdata/font-constant.bin sweeps
// constantAlphabet, which is "0123456789A-Z" and 36 of the face's 96 glyphs, and
// widening it is forbidden at engrave/engrave.go:770. Every other font/constant
// glyph was pinned only INCIDENTALLY -- by whichever characters happened to
// appear in a plate golden's text -- so the lowercase letters had one plate each
// and the punctuation had none. '~' and '{' could have been scribbles.
//
// font/sh is the other way round: engrave.TestFonts sweeps every rune the face
// decodes, so engrave/testdata/font-sh.bin already pinned all 95. What these two
// goldens add for it is the shipped RUNGS on the real plate rather than a fresh
// alphabet.
//
// The two ladder sides ARE the 95-character sweep in both shipped faces at five
// rungs, so two artifacts pin every glyph's path at once -- and they are the
// same plates being cut in steel, so the test and the plate cannot disagree
// about what was proven. TestSizeLadderGoldensPinEveryGlyph below states that
// coverage as an assertion rather than as this paragraph.
//
// THE CONTRACT HERE IS NOT backup/testdata's, and the difference matters.
// Those sixteen goldens are FROZEN: a moved byte is a finding, and -update has
// never been run on them. These two exist in order to move. A glyph edit is
// supposed to move them; what they buy is that the movement is SEEN, attributed
// to the glyph that caused it, and re-recorded in the same commit as the edit --
// instead of shipping to steel unnoticed.
//
// So when one fails, do not reach for -update first. Run
//
//	go test ./gui -run TestSizeLadderGoldens -artifacts -outputdir=/some/dir
//
// which writes, into that directory,
//
//	sizeproof-front.bin.svg       the plate the code draws NOW
//	sizeproof-front.bin.orig.svg  the plate this golden holds
//
// at the production stroke. Diff the pair and the glyph that moved is visible at
// plate scale, in every rung it appears in. Only once the diff is the edit you
// meant, re-record with
//
//	go test ./gui -run TestSizeLadderGoldens -update
//
// and commit the .bin next to the SVG change that caused it.
func TestSizeLadderGoldens(t *testing.T) {
	P := proofParams()
	for _, side := range ftSideTables {
		t.Run(side.trigger, func(t *testing.T) {
			// Resolved through the trigger table and ftEvaluate, so the golden
			// is over a plate the operator can actually reach; ftLadderPlate
			// also fails a side that came out uniform, which would be a golden
			// over a plate that is not a ladder at all.
			p, plate := ftLadderPlate(t, P, side.trigger)

			// toPlate is the device's own last step, and it is used here rather
			// than a bare PlanEngraving because it applies the fail-closed
			// [3mm, 82mm] bound as well as planning the toolpath. A golden can
			// then never be recorded for a plate the machine would refuse.
			pl, err := toPlate(backup.EngraveFitted(P, plate), P)
			if err != nil {
				t.Fatalf("%s: the ladder does not survive toPlate: %v", side.trigger, err)
			}
			// A golden over an empty engraving passes forever, and -update would
			// bake that in silently. bspline.Measure unions only ENGRAVED
			// segments, so this asks about ink and not about travel moves.
			if bspline.Measure(pl.Spline).Bounds.Empty() {
				t.Fatalf("%s: the plate cuts nothing", side.trigger)
			}

			name := ftLadderGoldenName(t, p)
			dir := t.ArtifactDir()
			bounds := bspline.Bounds{Max: SquarePlate.Dims(P.Millimeter)}
			err = golden.CompareBSpline(filepath.Join("testdata", name+".bin"),
				*update, dir, P.StrokeWidth, bounds, pl.Spline)
			if err != nil {
				t.Fatalf("%s moved: %v\n\n"+
					"A glyph's PATH has changed, or the plate's composition has. Re-run with\n"+
					"  -artifacts -outputdir=DIR\n"+
					"and diff these two renders of the plate at the production stroke:\n"+
					"  %s   what the code draws NOW\n"+
					"  %s   what this golden holds\n"+
					"The glyph that moved is visible at plate scale, in every rung it appears in.\n"+
					"Only once that diff is the edit you meant, re-record with\n"+
					"  go test ./gui -run TestSizeLadderGoldens -update\n"+
					"and commit the .bin beside the font change that caused it. NEVER re-record\n"+
					"with a bare `go test ./... -update`: backup's sixteen goldens are frozen.",
					side.trigger, err,
					filepath.Join(dir, name+".bin.svg"),
					filepath.Join(dir, name+".bin.orig.svg"))
			}
		})
	}
}

// ftLadderGoldenName is the golden's basename, taken from the SIDE the proof
// names -- the same word cmd/plateview's -plate takes, so the artifact and the
// render of it are called the same thing.
//
// A proof with no side is refused rather than named: both sides would land on
// "sizeproof-.bin" and the second would silently overwrite the first under
// -update, leaving one plate pinned twice and the other not at all.
func ftLadderGoldenName(t *testing.T, p *ftProof) string {
	t.Helper()
	if p.Side == "" {
		t.Fatalf("%s names no side, so it has no golden name of its own", p.Trigger)
	}
	return "sizeproof-" + strings.ToLower(p.Side)
}

// ftLadderUnpinnedGlyph is the one glyph the ladder cannot carry: the
// visible-space mark. It is not printable ASCII, so it is not in the sweep, and
// no free-text plate can hold it -- backup's passphraseGlyphs substitutes it for
// every space on the PASSPHRASE plate, and nothing else in the tree engraves it.
// It is pinned there instead, by the passphrase-0-plain, passphrase-1-qr and
// passphrase-3-max-qr goldens, whose passphrases carry spaces.
const ftLadderUnpinnedGlyph = backup.SpaceMark

// TestSizeLadderGoldensPinEveryGlyph turns the claim the goldens are worth
// having into an assertion: every glyph either shipped face can cut appears on
// the two ladder plates, so the pair really is a complete identity test and not
// merely a large one.
//
// It is the guard against the goldens quietly becoming partial. A glyph added to
// a face -- or the sweep trimmed to buy a row back after an edit costs one --
// leaves the two .bin files passing while the new or dropped glyph is pinned
// nowhere. That is the same failure mode this whole file exists to close, one
// level up.
//
// MEASURED 2026-08-05: font/sh decodes exactly the 95 printable ASCII runes and
// nothing else; font/constant decodes those 95 plus 0x1F. Scanning to 0x2000 is
// far past both and costs nothing.
//
// One of the 95 has no path to pin. Measured, 0x20 inks nothing in either face,
// so what the goldens hold for it is its ADVANCE, in the position of every glyph
// that follows it on the row. A space that stopped being blank would show up as
// ink; a space whose advance changed would move the rest of the sweep.
func TestSizeLadderGoldensPinEveryGlyph(t *testing.T) {
	// The exemption must be LIVE. If font/constant loses the mark, this constant
	// is excusing a glyph that no longer exists and would go on excusing the
	// next one to take its place.
	if _, _, ok := ftFaceConst.Face.Decode(ftLadderUnpinnedGlyph); !ok {
		t.Fatalf("%s cannot decode the visible-space mark %#x; the exemption below is stale",
			ftFaceConst.Name, ftLadderUnpinnedGlyph)
	}
	if strings.ContainsRune(ftProofSweep, ftLadderUnpinnedGlyph) {
		t.Fatalf("the sweep carries %#x after all; the exemption below is unnecessary",
			ftLadderUnpinnedGlyph)
	}
	for _, f := range []ftFace{ftFaceSH, ftFaceConst} {
		var unpinned []rune
		for r := rune(0); r < 0x2000; r++ {
			if _, _, ok := f.Face.Decode(r); !ok {
				continue
			}
			if r == ftLadderUnpinnedGlyph || strings.ContainsRune(ftProofSweep, r) {
				continue
			}
			unpinned = append(unpinned, r)
		}
		if len(unpinned) > 0 {
			t.Errorf("%s can cut %d glyph(s) the size ladder does not carry: %q %v. "+
				"The goldens pin what is on the plate, so these have no identity test at all.",
				f.Name, len(unpinned), string(unpinned), unpinned)
		}
	}
}
