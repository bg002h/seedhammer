package gui

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/engrave"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// proofParams is the production engraving geometry, taken from the platform the
// flow itself asks -- never hand-rolled. Every capacity figure the patterns are
// tuned to depends on it.
func proofParams() engrave.Params {
	return newPlatform().EngraverParams()
}

// proofCases is the four patterns: two faces, each with and without a QR.
func proofCases(t *testing.T) []struct {
	name  string
	proof *ftProof
	qr    bool
} {
	t.Helper()
	var out []struct {
		name  string
		proof *ftProof
		qr    bool
	}
	if len(ftProofs) != 2 {
		t.Fatalf("there are %d proofs, want one per engraving face", len(ftProofs))
	}
	for i := range ftProofs {
		p := &ftProofs[i]
		for _, qr := range []bool{false, true} {
			out = append(out, struct {
				name  string
				proof *ftProof
				qr    bool
			}{fmt.Sprintf("%s/qr=%v", p.Face.Name, qr), p, qr})
		}
	}
	return out
}

// Every pattern must land at 3.0mm IN ITS OWN FACE -- the SMALLEST rung and the
// hardest legibility case, which is the entire point of the proof (user
// directive 2026-08-04). A pattern that drifted to a larger rung would still
// engrave, and would silently test an easier question than the operator asked
// for.
func TestProofPatternsLandAtSmallestRung(t *testing.T) {
	P := proofParams()
	smallest := backup.FontSizes[len(backup.FontSizes)-1]
	if smallest != 3.0 {
		t.Fatalf("the smallest rung is %.1fmm, not 3.0mm; the patterns and their titles are tuned to 3.0", smallest)
	}
	for _, tc := range proofCases(t) {
		text := tc.proof.For(tc.qr)
		fontMM, lines, code, err := backup.Fit(P, tc.proof.Face.Face, text, tc.proof.Title, ftProofFooter, tc.qr)
		if err != nil {
			t.Fatalf("%s: the proof does not fit: %v", tc.name, err)
		}
		if fontMM != smallest {
			t.Errorf("%s: proof lands at %.1fmm, want %.1fmm -- %d chars is the wrong length "+
				"to select the smallest rung", tc.name, fontMM, smallest, utf8.RuneCountInString(text))
		}
		if len(lines) == 0 {
			t.Errorf("%s: no lines", tc.name)
		}
		if tc.qr != (code != nil) {
			t.Errorf("%s: qr=%v but code!=nil is %v", tc.name, tc.qr, code != nil)
		}
	}
}

// TestProofPatternsFillThePlate is the user's "use as much of the plate as
// fits, not a round number" (2026-08-04), as relaxed the same day: aim for
// roughly nine tenths of capacity and keep the rest as margin.
//
// Stated in ROWS rather than characters, and as a BAND rather than a number.
// Rows are what the plate has -- under word wrap a character count is not a
// capacity, and the two faces do not even share a grid (44 columns at 3.0mm in
// font/sh against 39 in font/constant). A pinned character count would go stale
// every time a glyph moves, which is the opposite of what this guards.
//
// The lower bound is the "as much as fits" half: a pattern using half the plate
// wastes legible area a proof exists to use. The upper bound and the growth
// check are the margin: auto-fit is all-or-nothing, so a pattern balanced on the
// 3.0mm limit is REFUSED outright by any later edit that costs a line, and the
// operator gets no proof at all instead of a slightly smaller one.
func TestProofPatternsFillThePlate(t *testing.T) {
	P := proofParams()
	for _, tc := range proofCases(t) {
		text := tc.proof.For(tc.qr)
		used, avail, ok := backup.Admissible(P, tc.proof.Face.Face, text, tc.proof.Title, ftProofFooter, tc.qr)
		if !ok {
			t.Fatalf("%s: the pattern is not admissible: %d of %d rows", tc.name, used, avail)
		}
		if lo := avail * 3 / 4; used < lo {
			t.Errorf("%s: the pattern uses only %d of %d rows (want at least %d); a proof should use "+
				"the plate it is cut on", tc.name, used, avail, lo)
		}
		if hi := avail - 2; used > hi {
			t.Errorf("%s: the pattern uses %d of %d rows (want at most %d); it is close enough to the "+
				"limit that a glyph edit could refuse the plate outright", tc.name, used, avail, hi)
		}
		// The margin, stated the way it will actually be spent: 5% more text
		// must still fit, AT THE SAME RUNG. Appended as short words so it wraps
		// like the prose it stands in for.
		grown := text + " " + strings.TrimSpace(strings.Repeat("pad ", len(text)/80+1))
		size, _, _, err := backup.Fit(P, tc.proof.Face.Face, grown, tc.proof.Title, ftProofFooter, tc.qr)
		pct := 100 * (len(grown) - len(text)) / len(text)
		if err != nil {
			t.Errorf("%s: %d%% more text no longer fits at all; the pattern has no margin", tc.name, pct)
		} else if size != 3.0 {
			t.Errorf("%s: %d%% more text drops the plate to %.1fmm", tc.name, pct, size)
		}
		t.Logf("%s: %d characters, %d of %d rows", tc.name, utf8.RuneCountInString(text), used, avail)
	}
}

// The QR variants exist because the QR competes for the plate. If a no-QR
// pattern were used with a QR it would be REFUSED and the operator would get no
// proof at all -- so this pins that the split is load-bearing, not cosmetic.
func TestProofNoQRPatternWouldNotFitWithAQR(t *testing.T) {
	P := proofParams()
	for i := range ftProofs {
		p := &ftProofs[i]
		if _, _, _, err := backup.Fit(P, p.Face.Face, p.Text, p.Title, ftProofFooter, true); err == nil {
			t.Errorf("%s: the no-QR pattern now fits with a QR -- the split is no longer needed, "+
				"or the capacity changed and the lengths should be revisited", p.Face.Name)
		}
		if _, _, _, err := backup.Fit(P, p.Face.Face, p.TextQR, p.Title, ftProofFooter, true); err != nil {
			t.Errorf("%s: the QR variant does not fit with a QR: %v", p.Face.Name, err)
		}
	}
}

// TestProofPatternsAreFaceSpecific: the two faces do NOT share a column grid,
// so each needs its own tuned length. This pins that the split earns its keep:
// the same text in the other face is a different number of rows, and at least
// one cross-face combination breaks a requirement the proof depends on -- sits
// on the capacity limit, falls short of it, or lands on the wrong rung
// altogether. If every pattern served either face equally well there would be
// no reason for four of them.
func TestProofPatternsAreFaceSpecific(t *testing.T) {
	P := proofParams()
	smallest := backup.FontSizes[len(backup.FontSizes)-1]
	a, b := ftProofs[0].Face, ftProofs[1].Face
	if backup.CharsPerLine(P, a.Face, smallest) == backup.CharsPerLine(P, b.Face, smallest) {
		t.Fatalf("font/%s and font/%s have the same %d-column grid at %.1fmm; there would be nothing "+
			"to tune per face", a.Name, b.Name, backup.CharsPerLine(P, a.Face, smallest), smallest)
	}
	seen := map[string]string{}
	for _, tc := range proofCases(t) {
		if where, dup := seen[tc.proof.For(tc.qr)]; dup {
			t.Errorf("%s and %s are the same pattern", tc.name, where)
		}
		seen[tc.proof.For(tc.qr)] = tc.name
	}
	crossWrong := 0
	for i := range ftProofs {
		p := &ftProofs[i]
		other := &ftProofs[(i+1)%len(ftProofs)]
		for _, qr := range []bool{false, true} {
			// This face's own pattern satisfies this face's requirements --
			// that is TestProofPatternsFillThePlate. What is asked here is
			// whether the OTHER face's pattern would, and it must not always.
			cross, avail, crossOK := backup.Admissible(P, p.Face.Face, other.For(qr), p.Title, ftProofFooter, qr)
			size, _, _, err := backup.Fit(P, p.Face.Face, other.For(qr), p.Title, ftProofFooter, qr)
			if !crossOK || err != nil || size != smallest || cross > avail-2 || cross < avail*3/4 {
				crossWrong++
				t.Logf("font/%s qr=%v: %s's pattern would be %d of %d rows at %.1fmm (err=%v)",
					p.Face.Name, qr, other.Face.Name, cross, avail, size, err)
			}
		}
	}
	if crossWrong == 0 {
		t.Error("every pattern would serve either face equally well; the per-face split is decoration")
	}
	t.Logf("%d of 4 cross-face combinations are wrong for the face they land in", crossWrong)
}

// Every printable ASCII character must appear in every pattern, or the proof is
// not a proof. Derived from the codepoint range, so a typo in the literal
// cannot survive.
func TestProofCoversEveryPrintableASCII(t *testing.T) {
	for _, tc := range proofCases(t) {
		text := tc.proof.For(tc.qr)
		var missing []rune
		for r := rune(0x20); r <= 0x7E; r++ {
			if !strings.ContainsRune(text, r) {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s: proof omits %d printable ASCII: %q", tc.name, len(missing), missing)
		}
	}
}

// TestProofSweepOpensWithARealSpace: the sweep starts at 0x20 because free text
// engraves spaces as spaces, and the sweep is derived from the codepoint range.
// Dropping the leading space costs the plate nothing visible and would make the
// sweep no longer a sweep.
func TestProofSweepOpensWithARealSpace(t *testing.T) {
	if !strings.HasPrefix(ftProofSweep, " !") {
		t.Errorf("the sweep no longer opens at 0x20: %q", ftProofSweep[:min(4, len(ftProofSweep))])
	}
	if got, want := utf8.RuneCountInString(ftProofSweep), 0x7E-0x20+1; got != want {
		t.Errorf("the sweep is %d characters, want %d (0x20..0x7E, one each)", got, want)
	}
	seen := map[rune]bool{}
	prev := rune(0x1F)
	for _, r := range ftProofSweep {
		if seen[r] {
			t.Errorf("the sweep repeats %q", r)
		}
		if r != prev+1 {
			t.Errorf("the sweep is not in codepoint order at %q (previous %q)", r, prev)
		}
		seen[r] = true
		prev = r
	}
}

// ftConfusableGroups is every group the confusable table promises, as it must
// appear on the plate: no group may be broken across two engraved lines.
var ftConfusableGroups = []string{
	"rnmrn", "0OoO", "1lI|", "!i|", "5SsS", "2ZzZ", "8B", "9g6",
	"adoq", "ceo", "uvw", "\\/", "^~", "_-", ",;.:", "'`\"", "{[(<>)]}",
}

// TestProofConfusablesSurviveTheWrap is the reason the table exists separately
// from the codepoint sweep -- codepoint order separates exactly the characters
// that get confused -- and it is asserted on the ENGRAVED LINES, never on the
// source constant.
//
// Asserting on the constant was the original defect: a group that wraps apart
// is still a substring of the source and the check passed while the plate had
// the two halves on different rows, which is precisely where the comparison is
// lost. With a QR the lines narrow to 14 characters, so this is not theoretical.
func TestProofConfusablesSurviveTheWrap(t *testing.T) {
	P := proofParams()
	for _, tc := range proofCases(t) {
		_, lines, _, err := backup.Fit(P, tc.proof.Face.Face, tc.proof.For(tc.qr), tc.proof.Title, ftProofFooter, tc.qr)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		for _, g := range ftConfusableGroups {
			if !strings.Contains(ftProofConfusables, g) {
				t.Errorf("the confusable table lost %q", g)
				continue
			}
			intact := false
			for _, l := range lines {
				if strings.Contains(l, g) {
					intact = true
					break
				}
			}
			if !intact {
				t.Errorf("%s: the group %q is split across engraved lines; the comparison it exists "+
					"for cannot be made on the plate.\nlines: %q", tc.name, g, lines)
			}
		}
	}
}

// TestProofConfusableSeparatorIsNotItselfConfusable: the separator was " | "
// until the execution review, and '|' is a member of two groups in the table,
// so the plate read "1lI| | 5SsS" -- ambiguous at exactly the size under test.
func TestProofConfusableSeparatorIsNotItselfConfusable(t *testing.T) {
	const sep = " + "
	if !strings.Contains(ftProofConfusables, sep) {
		t.Fatalf("the table no longer uses %q as its separator: %q", sep, ftProofConfusables)
	}
	septok := strings.TrimSpace(sep)
	for _, g := range ftConfusableGroups {
		if strings.Contains(g, septok) {
			t.Errorf("the separator %q is a member of the group %q; the plate cannot be read unambiguously",
				septok, g)
		}
	}
	// And no group contains a space, which is what makes the wrap unable to
	// split one: WrapText breaks at spaces and nowhere else.
	for _, g := range ftConfusableGroups {
		if strings.ContainsRune(g, ' ') {
			t.Errorf("the group %q contains a space, so the wrap can break it apart", g)
		}
	}
}

// TestProofTitlesStateTheMeasuredGrid is M2: on permanent steel the title is
// the only record of what was tested, and both numbers in it must be the LIVE
// measurement rather than a claim. "TEXTPROOF 6.0mm 44" and "TEXTPROOF 3.0mm
// 99" both survived the original suite.
func TestProofTitlesStateTheMeasuredGrid(t *testing.T) {
	P := proofParams()
	smallest := backup.FontSizes[len(backup.FontSizes)-1]
	rows := backup.LinesPerPlate(P, smallest)
	for i := range ftProofs {
		p := &ftProofs[i]
		cols := backup.CharsPerLine(P, p.Face.Face, smallest)
		want := fmt.Sprintf("%.1fmm %dx%d", smallest, cols, rows)
		if !strings.Contains(p.Title, want) {
			t.Errorf("%s: the title %q does not state the measured grid %q -- the plate would claim "+
				"a size or a column count it does not have", p.Face.Name, p.Title, want)
		}
		// And it names its own face, or two plates cut in different faces are
		// indistinguishable a year later.
		if !strings.Contains(strings.ToLower(p.Title), strings.ToLower(p.Face.Name[:2])) {
			t.Errorf("%s: the title %q does not name the face", p.Face.Name, p.Title)
		}
		if n := utf8.RuneCountInString(p.Title); n > ftMaxLineLen {
			t.Errorf("%s: the title is %d characters, past the %d cap", p.Face.Name, n, ftMaxLineLen)
		}
		// The two titles must differ, or the plate does not identify itself.
		for j := range ftProofs {
			if j != i && ftProofs[j].Title == p.Title {
				t.Errorf("%s and %s carry the same title %q", p.Face.Name, ftProofs[j].Face.Name, p.Title)
			}
		}
	}
}

// The footer sits exactly at the cap, so the plate exercises the cap on a
// screw-hole row rather than merely staying clear of it, and it carries
// descenders because that is the row where a glyph is most likely to reach a
// screw hole.
func TestProofFooterSitsExactlyAtTheCap(t *testing.T) {
	if n := utf8.RuneCountInString(ftProofFooter); n != ftMaxLineLen {
		t.Errorf("the footer is %d characters, want exactly %d (the cap) so the plate tests it",
			n, ftMaxLineLen)
	}
	for _, r := range "gjpqy" {
		if !strings.ContainsRune(ftProofFooter, r) {
			t.Errorf("the footer lost the descender %q; it is the row nearest a screw hole", r)
		}
	}
}

// TestProofRunesDecodeInTheirOwnFace: engrave.String PANICS on a rune the face
// cannot decode, mid-plate, so this is checked twice -- rune by rune, and by
// building each plate end to end under recover().
func TestProofRunesDecodeInTheirOwnFace(t *testing.T) {
	P := proofParams()
	for _, tc := range proofCases(t) {
		text := tc.proof.For(tc.qr)
		for _, s := range []string{text, tc.proof.Title, ftProofFooter} {
			for _, r := range s {
				if r == '\n' {
					// A wrap-time block boundary. WrapText consumes it; no
					// engrave.String call ever sees one, because Fit's lines
					// are guaranteed newline-free.
					continue
				}
				if _, _, ok := tc.proof.Face.Face.Decode(r); !ok {
					t.Errorf("%s: %q does not decode in font/%s", tc.name, r, tc.proof.Face.Name)
				}
			}
		}
		// And no engraved line carries one, which is what makes the skip above
		// safe rather than a hole in the check.
		if _, lines, _, err := backup.Fit(P, tc.proof.Face.Face, text, tc.proof.Title, ftProofFooter, tc.qr); err == nil {
			for _, l := range lines {
				if strings.ContainsRune(l, '\n') {
					t.Errorf("%s: an engraved line contains a newline: %q", tc.name, l)
				}
			}
		}
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s: building the plate panicked: %v", tc.name, p)
				}
			}()
			if _, err := ftBuildPlate(P, tc.proof.Face, text, tc.proof.Title, ftProofFooter, tc.qr); err != nil {
				t.Errorf("%s: the plate does not build: %v", tc.name, err)
			}
		}()
	}
}

// TestProofCarriesBothCasesInRunningText is the user's directive of 2026-08-04
// and the review's I2. Measured before the fix: lowercase 26/26, uppercase 7/26
// without a QR and 4/26 with one -- fourteen to seventeen capitals appeared
// ONLY inside the dense unspaced sweep, where every neighbour is a capital and
// there is no word shape to read by.
//
// The uppercase case is not hypothetical: backup.EngraveSeed upper-cases the
// seed and EVERY mnemonic word before engraving, so uppercase running text is
// what a seed plate IS.
func TestProofCarriesBothCasesInRunningText(t *testing.T) {
	// The running text is everything except the sweep and the confusable table:
	// those two are dense reference blocks, not reading material.
	running := func(text string) string {
		blocks := strings.Split(text, "\n")
		if len(blocks) < 3 {
			return ""
		}
		return strings.Join(blocks[2:], " ")
	}
	for _, tc := range proofCases(t) {
		run := running(tc.proof.For(tc.qr))
		if run == "" {
			t.Fatalf("%s: the pattern has no running-text block", tc.name)
		}
		for _, cls := range []struct {
			name     string
			lo, hi   rune
			wantAll  bool
			examples string
		}{
			{"lowercase", 'a', 'z', true, ""},
			{"uppercase", 'A', 'Z', true, ""},
		} {
			var missing []rune
			for r := cls.lo; r <= cls.hi; r++ {
				if !strings.ContainsRune(run, r) {
					missing = append(missing, r)
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s: %d of 26 %s letters never appear in running text: %q",
					tc.name, len(missing), cls.name, string(missing))
			}
		}
		// And the uppercase run must be WORDS, not the sweep smuggled back in:
		// a real seed plate's uppercase is spaced into word shapes.
		if !strings.Contains(run, ftProofUpperPangram) {
			t.Errorf("%s: the uppercase pangram is not in the running text", tc.name)
		}
		if !strings.Contains(run, ftProofLowerPangram) {
			t.Errorf("%s: the lowercase pangram is not in the running text", tc.name)
		}
	}
}

// TestProofSeedWordsAreRealBIP39Words: the uppercase block exists to reproduce
// the reading task a seed plate sets, so the words have to be words a seed
// plate can actually carry. Checked against the wordlist, not against a list
// copied next to it.
func TestProofSeedWordsAreRealBIP39Words(t *testing.T) {
	words := strings.Fields(ftProofSeedWords)
	if len(words) < 6 {
		t.Errorf("only %d uppercase seed words; the block is too short to read as running text", len(words))
	}
	for _, w := range words {
		if w != strings.ToUpper(w) {
			t.Errorf("%q is not upper case; backup.EngraveSeed upper-cases every mnemonic word", w)
		}
		// The wordlist itself is stored UPPER CASE -- which is the same fact
		// this block exists to exercise -- so the lookup needs no folding.
		idx, ok := bip39.ClosestWord(w)
		if !ok || bip39.LabelFor(idx) != w {
			t.Errorf("%q is not a BIP-39 word", w)
		}
	}
	for _, tc := range proofCases(t) {
		if !strings.Contains(tc.proof.For(tc.qr), ftProofSeedWords) {
			t.Errorf("%s: the uppercase seed-word block is missing from the pattern", tc.name)
		}
	}
}

// TestProofPatternsCarryTheirProse is M3: before this test a degenerate pattern
// -- sweep + confusables + strings.Repeat("xy ", 200) -- passed the entire
// suite. Nothing pinned that the plate carries real prose, which is the only
// part that shows ragged line ends, varying word lengths and the reading
// experience the two reference blocks cannot.
func TestProofPatternsCarryTheirProse(t *testing.T) {
	P := proofParams()
	// Which prose block each pattern is built from, as LITERAL text rather than
	// as the constants themselves. Naming the constants would pin only the
	// composition and not the prose: replacing ftProofLorem1's value with
	// strings.Repeat("xy ", 40) would leave every such assertion green, which
	// is exactly the degenerate pattern the review demonstrated.
	const (
		lorem1   = "Lorem ipsum dolor sit amet, consectetur adipiscing elit"
		lorem2   = "Ut enim ad minim veniam, quis nostrud exercitation"
		lorem3   = "Duis aute irure dolor in reprehenderit in voluptate"
		lorem4   = "Excepteur sint occaecat cupidatat non proident"
		pangram1 = "How vexingly quick daft zebras jump."
		pangram2 = "Sphinx of black quartz, judge my vow."
	)
	want := map[string][]string{
		"sh/qr=false":       {lorem1, lorem2, lorem3, lorem4, pangram1},
		"sh/qr=true":        {pangram1, pangram2},
		"constant/qr=false": {lorem1, lorem2, lorem3, pangram1, pangram2},
		"constant/qr=true":  {pangram1},
	}
	for _, tc := range proofCases(t) {
		text := tc.proof.For(tc.qr)
		blocks, known := want[tc.name]
		if !known {
			t.Fatalf("no prose composition recorded for %q", tc.name)
		}
		for _, b := range blocks {
			if !strings.Contains(text, b) {
				t.Errorf("%s: the pattern no longer carries %.40q...", tc.name, b)
			}
		}
		// The prose must actually reach the plate as WRAPPED prose: at least
		// half the engraved lines have to be ragged, or the pattern is all
		// reference blocks and no reading.
		_, lines, _, err := backup.Fit(P, tc.proof.Face.Face, text, tc.proof.Title, ftProofFooter, tc.qr)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		// Joining the lines back with a space reconstitutes the prose exactly:
		// WrapText breaks at spaces, so no prose word is ever split. A pattern
		// whose prose ran off the end of the plate fails here even though the
		// source-string check above passed.
		engraved := strings.Join(lines, " ")
		for _, b := range blocks {
			if !strings.Contains(engraved, b) {
				t.Errorf("%s: %.40q... is in the pattern but not on the plate; the prose is being "+
					"cut off by the wrap", tc.name, b)
			}
		}
		// Anti-degeneracy: a pattern of one word repeated satisfies every
		// containment check above that is not literal, and satisfies the
		// printable-ASCII and confusable checks outright. Real prose has many
		// distinct words; strings.Repeat("xy ", 200) has one.
		distinct := map[string]bool{}
		for _, w := range strings.Fields(engraved) {
			distinct[w] = true
		}
		const minWords = 40
		if len(distinct) < minWords {
			t.Errorf("%s: the plate carries only %d distinct words (want at least %d); a legibility "+
				"proof has to be read, not scanned for a repeat", tc.name, len(distinct), minWords)
		}
		t.Logf("%s: %d distinct words on the plate", tc.name, len(distinct))
	}
}

// TestProofPangramsAreLiteral pins the two case pangrams by their text. They
// are what makes uppercase and lowercase RUNNING text present in every pattern,
// and an assertion phrased in terms of the constants cannot see them replaced.
func TestProofPangramsAreLiteral(t *testing.T) {
	if ftProofUpperPangram != "THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG." {
		t.Errorf("the uppercase pangram changed: %q", ftProofUpperPangram)
	}
	if ftProofLowerPangram != "pack my box with five dozen liquor jugs." {
		t.Errorf("the lowercase pangram changed: %q", ftProofLowerPangram)
	}
	if !strings.Contains(ftProofSeedWords, "ABANDON ABILITY") {
		t.Errorf("the uppercase seed-word block changed: %q", ftProofSeedWords)
	}
}

// ---- the trigger ------------------------------------------------------------

// TestProofTriggersAreDistinctAndExact: the two triggers select different
// faces, so confusing them engraves the wrong proof. Neither may be a prefix of
// the other, and the match is case-sensitive whole-field equality.
func TestProofTriggersAreDistinctAndExact(t *testing.T) {
	seen := map[string]bool{}
	for i := range ftProofs {
		p := &ftProofs[i]
		if seen[p.Trigger] {
			t.Fatalf("two proofs share the trigger %q", p.Trigger)
		}
		seen[p.Trigger] = true
		got, ok := ftProofForTrigger(p.Trigger)
		if !ok || got != p {
			t.Errorf("%q does not resolve to its own proof", p.Trigger)
		}
	}
	for i := range ftProofs {
		for j := range ftProofs {
			if i == j {
				continue
			}
			a, b := ftProofs[i].Trigger, ftProofs[j].Trigger
			if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
				t.Errorf("%q and %q are prefixes of one another", a, b)
			}
		}
	}
	// Case-sensitive, whole-field, and exact in EVERY position. The near
	// misses are GENERATED rather than listed: a match that happened to ignore
	// one character -- the first, say -- passes any hand-written list of
	// plausible typos, and there is no reason the position a defect lands on
	// would be the one someone thought to write down.
	real := map[string]bool{}
	for i := range ftProofs {
		real[ftProofs[i].Trigger] = true
	}
	near := []string{
		strings.ToLower(ftProofs[0].Trigger),
		strings.TrimSuffix(ftProofs[0].Trigger, "!"),
		ftProofs[0].Trigger + " ",
		" " + ftProofs[0].Trigger,
		"see " + ftProofs[0].Trigger + " for details",
	}
	for i := range ftProofs {
		trig := ftProofs[i].Trigger
		for pos := range len(trig) {
			// One character replaced, one deleted, one case-flipped.
			near = append(near, trig[:pos]+"X"+trig[pos+1:], trig[:pos]+trig[pos+1:])
			if r := trig[pos]; 'A' <= r && r <= 'Z' {
				near = append(near, trig[:pos]+string(r+('a'-'A'))+trig[pos+1:])
			}
		}
	}
	for _, n := range near {
		if real[n] {
			continue // a near miss that happens to spell the other trigger
		}
		if _, ok := ftProofForTrigger(n); ok {
			t.Errorf("%q triggers a proof; the match must be whole-field, exact and case-sensitive", n)
		}
	}
}

// Whole-field equality driven through ftProofOffer, with the frame callback
// proving the prompt was never PUT UP -- without it a mutation that showed the
// prompt and had it dismissed would return false too, and it stops such a
// mutation spinning in ftProofPrompt's loop until timeout.
func TestProofNeedsTheWholeField(t *testing.T) {
	newCtx := func(drew *bool) *Context {
		ctx := NewContext(newPlatform())
		ctx.FrameCallback = func(o op.Op) { *drew = true; ctx.Done = true }
		return ctx
	}
	called, drew := false, false
	ctx := newCtx(&drew)
	load := func(*ftProof) string { called = true; return "" }
	for _, typed := range []string{"", "see TEXTPROOF! for details", "TEXTPROOF",
		"textproof!", "CONSTPROOF", ftProofTriggerSH + " ", "TEXTPROOF!CONSTPROOF!"} {
		if _, ok := ftProofOffer(ctx, &descriptorTheme, typed, load); ok {
			t.Errorf("%q triggered the offer", typed)
		}
	}
	if called {
		t.Error("the loader ran for a field that is not a trigger")
	}
	if drew {
		t.Error("a field that is not a trigger still put the prompt up")
	}
	// A nil loader disables the trigger: it must not prompt and must not panic.
	for i := range ftProofs {
		drew = false
		if _, ok := ftProofOffer(newCtx(&drew), &descriptorTheme, ftProofs[i].Trigger, nil); ok {
			t.Errorf("%s: a nil loader still reported the pattern as loaded", ftProofs[i].Face.Name)
		}
		if drew {
			t.Errorf("%s: a nil loader still put the prompt up, which no answer could act on",
				ftProofs[i].Face.Name)
		}
	}
}

// The loader writes ALL of the fields the prompt promises -- text, title,
// footer and FACE -- and never the QR choice.
func TestProofLoaderWritesEveryPromisedField(t *testing.T) {
	for _, tc := range proofCases(t) {
		text, title, footer := "old text", "old title", "old footer"
		face := ftFace{"stale", nil}
		useQR := tc.qr
		got := ftProofLoader(&text, &title, &footer, &face, &useQR)(tc.proof)
		if text != tc.proof.For(tc.qr) {
			t.Errorf("%s: text not loaded", tc.name)
		}
		if got != text {
			t.Errorf("%s: the loader returned %d characters but stored %d; the screen would re-seed "+
				"from a different pattern than the flow holds", tc.name, len(got), len(text))
		}
		if title != tc.proof.Title {
			t.Errorf("%s: title not loaded, got %q", tc.name, title)
		}
		if footer != ftProofFooter {
			t.Errorf("%s: footer not loaded, got %q", tc.name, footer)
		}
		if face != tc.proof.Face {
			t.Errorf("%s: face not loaded, got %q -- the plate would be cut in the wrong font",
				tc.name, face.Name)
		}
		if useQR != tc.qr {
			t.Errorf("%s: the loader changed the QR choice to %v -- it must never do that, "+
				"the operator chose it one step earlier and it decides what a scanner returns",
				tc.name, useQR)
		}
	}
}

// ---- the prompt -------------------------------------------------------------

// TestProofPromptSaysWhatItWillDo is M4's third survivor: gutting the "REPLACES
// ALL THREE" copy left the whole suite green. The operator is one tap from
// losing a body they may have spent minutes typing, and this sentence is the
// only warning.
func TestProofPromptSaysWhatItWillDo(t *testing.T) {
	for i := range ftProofs {
		p := &ftProofs[i]
		body := ftProofAsk(p) + " " + ftProofReplaces(p) + " " + ftProofKeep(p)
		for _, want := range []string{
			"REPLACES ALL THREE",  // that it destroys work
			p.Title,               // what the title becomes
			ftProofFooter,         // what the footer becomes
			p.Face.Name,           // which face it will be cut in
			"3.0mm",               // at which size
			p.Trigger,             // what declining leaves behind
			"Back = no",           // and which answer declines
			"can be a real plate", // that the typed trigger is usable text
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the prompt never says %q.\nprompt: %q", p.Face.Name, want, body)
			}
		}
		// The two prompts must differ, or the operator cannot tell from the
		// screen which trigger they typed.
		other := &ftProofs[(i+1)%len(ftProofs)]
		if ftProofAsk(p) == ftProofAsk(other) {
			t.Errorf("both triggers ask the same question %q", ftProofAsk(p))
		}
	}
}

// TestProofPromptFitsPanel: the prompt must fit the panel the machine actually
// has, 480x320, WITHOUT scrolling. The codebase's only scroller
// (Warning.Layout) is bound to ButtonFilter(Up/Down), which nothing on
// SeedHammer II emits, so anything past the bottom edge is unreadable.
//
// Measured as rectangles against the same budget the layout spends. It cannot
// be done from ExtractText: that collects runes from every drawn text op
// regardless of where they landed.
func TestProofPromptFitsPanel(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := ctx.Platform.DisplaySize()
	if dims != sh2DisplaySize {
		t.Fatalf("the fit test is running at %v, not the real %v panel", dims, sh2DisplaySize)
	}
	area := ppConfirmArea(dims)
	for i := range ftProofs {
		pr := &ftProofs[i]
		_, sz := ftProofBody(ctx, &descriptorTheme, area.Dx(), pr)
		if sz.Y > area.Dy() {
			t.Errorf("%s: the prompt needs %dpx of height but only %dpx is available on a %v panel; "+
				"the overflow would be unreadable, because the scroller is bound to buttons the machine does not have",
				pr.Face.Name, sz.Y, area.Dy(), dims)
		}
		if sz.X > area.Dx() {
			t.Errorf("%s: the prompt is %dpx wide in a %dpx area", pr.Face.Name, sz.X, area.Dx())
		}
	}
	// layoutTitle wraps at width-32 and draws at y=8 inside the leadingSize band.
	if h := ctx.Styles.title.Measure(dims.X-32, "%s", ftProofPromptTitle).Y; h > leadingSize-8 {
		t.Errorf("the title %q is %dpx tall and wraps out of the %dpx title band", ftProofPromptTitle, h, leadingSize-8)
	}
}

// TestProofNavIconsMeanWhatTheyShow is M4's first two survivors, together:
// swapping the nav icons so the CHECKMARK means "no", and noBtn returning true
// so Back also loads. Both left the suite green, because every test tapped by
// registered tag and the operator has only the glyph.
//
// So the tap is aimed at the button carrying a given ICON, found through
// ftProofNav, and the ANSWER is what is asserted.
func TestProofNavIconsMeanWhatTheyShow(t *testing.T) {
	for _, tc := range []struct {
		icon image.Image
		name string
		want bool
	}{
		{assets.IconCheckmark, "checkmark", true},
		{assets.IconBack, "back", false},
	} {
		for i := range ftProofs {
			pr := &ftProofs[i]
			h := newPPHarness(t)
			var answer, answered bool
			h.start(func() {
				answer = ftProofPrompt(h.ctx, &descriptorTheme, pr)
				answered = true
			})
			if !uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("%s: the prompt did not render its warning; got %q", pr.Face.Name, h.content)
			}
			no, okNo := h.widget("proofNo").(*Clickable)
			yes, okYes := h.widget("proofYes").(*Clickable)
			if !okNo || !okYes {
				t.Fatal("the prompt did not register both answers")
			}
			var target *Clickable
			for _, b := range ftProofNav(no, yes) {
				if b.Icon == tc.icon {
					if target != nil {
						t.Fatalf("two buttons carry the %s icon", tc.name)
					}
					target = b.Clickable
				}
			}
			if target == nil {
				t.Fatalf("%s: no button carries the %s icon", pr.Face.Name, tc.name)
			}
			// point() fails unless the target was drawn, sits on the panel and
			// is the topmost thing at its own centre.
			h.tapAt(h.point(target, "the "+tc.name+" button"))
			for i := 0; i < 8 && !answered; i++ {
				h.frame()
			}
			if !answered {
				t.Fatalf("%s: tapping the %s button left the prompt up; got %q", pr.Face.Name, tc.name, h.content)
			}
			if answer != tc.want {
				t.Errorf("%s: tapping the button showing the %s icon answered %v, want %v -- "+
					"the operator gets the opposite of what they read",
					pr.Face.Name, tc.name, answer, tc.want)
			}
		}
	}
}

// ---- end to end, through the production flow --------------------------------

// ftTypeTrigger types a trigger on the real keyboard, one tap per character,
// cycling pages by touch. A trigger the operator cannot type fails here.
func ftTypeTrigger(h *ppHarness, trigger string) {
	h.t.Helper()
	h.typeString(trigger)
	if got := ftKbd(h).Fragment; got != trigger {
		h.t.Fatalf("typing %q left %q in the field", trigger, got)
	}
}

// TestProofE2ELoadsTheWholePlate is the review's I1: the flow-level wiring had
// ZERO coverage and six mutations survived it, two of them wrong-plate-capable
// and one of them the deletion of the entire trigger block.
//
// It drives engraveTextFlow by touch from the QR choice to the engrave, and
// asserts what freetextPlateHook was handed -- the FACE, the size, the title,
// the footer, the line count and the QR state. Nothing here synthesizes a
// ButtonEvent.
func TestProofE2ELoadsTheWholePlate(t *testing.T) {
	for _, tc := range proofCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			h, r := startFT(t)
			ftPastQR(h, tc.qr)
			ftTypeTrigger(h, tc.proof.Trigger)
			ftOK(h)

			// The prompt is up, and it is THIS proof's prompt.
			if !uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("OK on the trigger did not offer the pattern; frame %q", h.content)
			}
			if !uiContains(h.content, "Load the "+tc.proof.Face.Name+" test pattern") {
				t.Errorf("the prompt does not name the %s face; frame %q", tc.proof.Face.Name, h.content)
			}
			h.tapWidget("proofYes")
			h.mustReach("lines")

			// The field now holds the pattern for THIS face and THIS QR
			// choice, and the flow is still on the Text step so the operator
			// sees what landed.
			want := tc.proof.For(tc.qr)
			if got := ftKbd(h).Fragment; got != want {
				t.Fatalf("the field holds %d characters, want the %d-character %s pattern",
					len(got), len(want), tc.name)
			}
			if uiContains(h.content, "optional") {
				t.Errorf("accepting the pattern jumped past the Text step; frame %q", h.content)
			}
			if !uiContains(h.content, "3.0mm") {
				t.Errorf("the readout does not show the pattern fitting at 3.0mm; frame %q", h.content)
			}
			// And the ROW COUNT, which is what tells the operator how much of
			// the plate the pattern uses and whether it still fits. It is
			// face-dependent -- admission wraps to the face's own column grid --
			// and reported for the wrong face it is simply a different number,
			// with nothing on screen to say so.
			P := proofParams()
			used, avail, _ := backup.Admissible(P, tc.proof.Face.Face, want, tc.proof.Title, ftProofFooter, tc.qr)
			if !uiHas(h.content, fmt.Sprintf("%d/%dlines", used, avail)) {
				t.Errorf("the readout does not show %d/%d lines for the %s pattern; frame %q",
					used, avail, tc.name, h.content)
			}

			// Title and footer were written too, and the flow carries them.
			ftOK(h)
			h.mustReach("Title")
			if got := ftKbd(h).Fragment; got != tc.proof.Title {
				t.Errorf("the Title field holds %q, want %q", got, tc.proof.Title)
			}
			ftOK(h)
			h.mustReach("Footer")
			if got := ftKbd(h).Fragment; got != ftProofFooter {
				t.Errorf("the Footer field holds %q, want %q", got, ftProofFooter)
			}
			ftOK(h)
			h.mustReach("Confirm")
			if !uiContains(h.content, "font: "+tc.proof.Face.Name) {
				t.Errorf("the confirm screen does not name the %s face; frame %q", tc.proof.Face.Name, h.content)
			}

			// What the operator APPROVES has to be the plate that gets cut, and
			// the wrap is face-dependent: the same text breaks into a different
			// number of lines in each face. A screen evaluated in one face and a
			// plate engraved in the other agree about the size and the QR and
			// disagree about every line.
			_, wantLines, _, err := backup.Fit(P, tc.proof.Face.Face, want, tc.proof.Title, ftProofFooter, tc.qr)
			if err != nil {
				t.Fatal(err)
			}
			if !uiContains(h.content, strconv.Itoa(len(wantLines))+" lines") {
				t.Errorf("the confirm screen does not report the %d lines the plate will carry; frame %q",
					len(wantLines), h.content)
			}
			pages := strings.Join(ftConfirmPages(h), "\n")
			for i, l := range wantLines {
				if !uiHas(pages, strings.ReplaceAll(l, " ", "")) {
					t.Fatalf("engraved line %d (%q) appears on no page of the confirm screen", i, l)
				}
			}
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if r.gotFont != tc.proof.Face.Face {
				t.Errorf("the plate was cut in the wrong face; the composition was fitted in font/%s",
					tc.proof.Face.Name)
			}
			if r.gotSize != 3.0 {
				t.Errorf("engraved at %.1fmm, want 3.0mm -- the proof tests the rung its length selects", r.gotSize)
			}
			if r.gotTitle != tc.proof.Title {
				t.Errorf("engraved title %q, want %q", r.gotTitle, tc.proof.Title)
			}
			if r.gotFooter != ftProofFooter {
				t.Errorf("engraved footer %q, want %q", r.gotFooter, ftProofFooter)
			}
			if len(r.gotLines) != len(wantLines) {
				t.Errorf("engraved %d lines, want %d", len(r.gotLines), len(wantLines))
			}
			if (r.gotQR != nil) != tc.qr {
				t.Errorf("engraved with a QR = %v, want %v", r.gotQR != nil, tc.qr)
			}
			if !strings.HasPrefix(strings.Join(r.gotLines, "\n"), " !\"") {
				t.Errorf("the engraved plate does not start with the codepoint sweep: %q", r.gotLines[0])
			}
		})
	}
}

// TestProofE2EDecliningEngravesTheTypedText: declining is not a cancel. The
// typed trigger is ordinary text and must engrave as itself -- at ITS auto-fit
// size, in the default face, with no title and no footer.
//
// This is what makes the trigger safe to have at all: an operator whose note
// genuinely reads TEXTPROOF! loses nothing by typing it.
func TestProofE2EDecliningEngravesTheTypedText(t *testing.T) {
	for i := range ftProofs {
		pr := &ftProofs[i]
		t.Run(pr.Face.Name, func(t *testing.T) {
			h, r := startFT(t)
			ftPastQR(h, false)
			ftTypeTrigger(h, pr.Trigger)
			ftOK(h)
			if !uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("the prompt did not appear; frame %q", h.content)
			}
			h.tapWidget("proofNo")
			// Declining falls through to the field's own validation, which
			// accepts the trigger as the ordinary text it is.
			h.mustReach("Title")
			ftOK(h) // no title
			h.mustReach("Footer")
			ftOK(h) // no footer
			h.mustReach("Confirm")
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if r.gotTitle != "" || r.gotFooter != "" {
				t.Errorf("declining still loaded the proof's title/footer: %q / %q", r.gotTitle, r.gotFooter)
			}
			if r.gotFont != ftFaceSH.Face {
				t.Error("declining still switched the engraving face")
			}
			if len(r.gotLines) != 1 || r.gotLines[0] != pr.Trigger {
				t.Errorf("engraved %q, want the single line %q", r.gotLines, pr.Trigger)
			}
			if r.gotSize == 3.0 {
				t.Errorf("an %d-character text engraved at 3.0mm; the proof pattern was loaded anyway",
					len(pr.Trigger))
			}
		})
	}
}

// TestProofE2EIsScopedToTheTextField: the triggers fit the 18-character title
// and footer fields, and firing there would clobber a body the operator had
// already typed. ftLineEntryFlow must never offer them.
func TestProofE2EIsScopedToTheTextField(t *testing.T) {
	for _, step := range []string{"Title", "Footer"} {
		t.Run(step, func(t *testing.T) {
			h, r := startFT(t)
			ftPastQR(h, false)
			h.typeString("hi")
			ftOK(h)
			h.mustReach("Title")
			if step == "Footer" {
				ftOK(h)
				h.mustReach("Footer")
			}
			ftSetText(h, ftProofTriggerSH)
			ftOK(h)
			if uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("the %s field offered the test pattern; frame %q", step, h.content)
			}
			// It was accepted as the ordinary 10-character text it is, and it
			// engraves on that row -- driven all the way to the plate, because
			// a screen assertion cannot tell "kept" from "silently dropped".
			if step == "Title" {
				h.mustReach("Footer")
				ftOK(h)
			}
			h.mustReach("Confirm")
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			got := r.gotTitle
			if step == "Footer" {
				got = r.gotFooter
			}
			if got != ftProofTriggerSH {
				t.Errorf("the %s engraved as %q, want the typed %q", step, got, ftProofTriggerSH)
			}
			if len(r.gotLines) != 1 || r.gotLines[0] != "hi" {
				t.Errorf("the body engraved as %q; the trigger clobbered it", r.gotLines)
			}
		})
	}
}
