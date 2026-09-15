package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"seedhammer.com/md"
	"strings"
	"testing"
	"time"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/hashlock"
	"seedhammer.com/internal/golden"
	"seedhammer.com/internal/sh2"
)

// H6 §6: the dedicated hashlock preimage plate.
//
// Two forms on one layout: the ms1 kind-0x03 string (text only) and the phrase
// with its method (optional QR). The plate is NOT a seed plate -- both forms
// carry NOT A SEED in the footer band -- and it is not a bundleCard.

const (
	h6HardenedMethodLine = "method: pbkdf2-hmac-sha256 iterations=100000 salt=ms-hashlock-v1 dklen=32"
	h6SHA256MethodLine   = "method: sha256"
	// h6WorstPhrase is the 100-character cap of SPEC_ms_hashlock §4.3.
	h6WorstPhrase = "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"
	// h6CorpusMS1 is the vendored corpus's kind row: 75 characters, id hash.
	h6CorpusMS1 = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c"
)

// h6QRText delegates to hashlock.QRText rather than re-spelling the format.
// It WAS a second copy -- "hashlock v1\n" + method + "\nphrase: " -- and when
// SPEC_hashlock_kinds §13.1 added the `hash:` line, the copy silently kept
// emitting the old three-line form while the corpus moved. One implementation
// is the point; a test helper that re-spells the thing under test cannot
// disagree with it usefully.
func h6QRText(hardened bool, kind md.HashKind, phrase string) string {
	return hashlock.QRText(hardened, kind, phrase)
}

// h6WorstLocator is §6.3's widest header: the template stub (29 characters, the
// longest locator row this plate produces) on a composer-native path.
var h6WorstLocator = []string{"path 2", "hash  b867db87..edbc96cb", "mk1 stub (template): 1a2b3c4d"}

func h6WorstPhrasePlate(qr bool) Hashlock {
	return Hashlock{
		Form:    HashlockPhrase,
		Phrase:  h6WorstPhrase,
		Method:  h6HardenedMethodLine,
		Locator: h6WorstLocator,
		QR:      qr,
		QRText:  h6QRText(true, md.KindSha256, h6WorstPhrase),
		Font:    constant.Font,
	}
}

func h6StringPlate() Hashlock {
	return Hashlock{
		Form:    HashlockString,
		MS1:     h6CorpusMS1,
		Locator: h6WorstLocator,
		Font:    constant.Font,
	}
}

// TestHashlockWorstCaseFitsAtExactlyOneRung is §6.5's fit gate, run against the
// real layout rather than against arithmetic.
//
// The worst-case phrase plate -- a 100-character phrase, the hardened method
// line, three header rows and a QR -- fits at 3.0mm with 1.20mm to spare and at
// NO larger rung. The margin is the whole point: it is why §6.5's method line is
// pinned at 73 characters and why the QR is at scale 2.
//
// MUTATION: grow the method line to 79 characters -> it wraps to 3 rows at
// 3.0mm instead of 2, the body is 11 rows, and EngraveHashlock refuses with
// `11 rows at 3.0mm need 427520 units against a budget of 416000` (66.80mm
// against 65.00mm).
//
// SEVENTY-NINE, NOT SEVENTY-FOUR, and the spec's own §11.4 says "add one
// character", which CANNOT FAIL: 39 characters per line at 3.0mm means the
// line wraps to 2 rows anywhere from 40 to 78 characters, so 74, 75, ... 78 all
// still fit -- MEASURED, all four pass. The threshold is §6.5 consequence 3's
// own number ("a 79th character would make it 3"), and this row is written to
// it so the mutation is one that fires.
// MUTATION: set hashlockQRScale to 3 -> the envelope is 47.70mm, 3.0mm needs
// 510080 units (79.70mm), and every rung reds.
func TestHashlockWorstCaseFitsAtExactlyOneRung(t *testing.T) {
	P := prodParams
	plate := h6WorstPhrasePlate(true)
	code, err := HashlockQRCode(plate)
	if err != nil {
		t.Fatal(err)
	}
	if code.Size != 53 {
		t.Fatalf("the worst-case §8.6 text encodes to dim %d, want 53 (v9)", code.Size)
	}
	want := map[float32]bool{6.0: false, 5.0: false, 4.4: false, 3.8: false, 3.4: false, 3.0: true}
	for _, size := range FontSizes {
		l := hashlockLayoutFor(P, plate, size, code.Size)
		if l.fits() != want[size] {
			t.Errorf("%.1fmm: fits = %v (%d rows, %d units against %d), want %v",
				size, l.fits(), len(l.rows), l.total, l.budget, want[size])
		}
		if size == 3.0 {
			if spare := l.budget - l.total; spare != P.F(1.2) {
				t.Errorf("3.0mm spare = %d units (%.2fmm), want 1.20mm",
					spare, float64(spare)/float64(P.Millimeter))
			}
		}
	}
	if _, err := EngraveHashlock(P, plate); err != nil {
		t.Fatalf("EngraveHashlock(the worst case): %v", err)
	}
	chosen, ok := hashlockFit(P, plate, code.Size)
	if !ok || chosen.fontMM != 3.0 {
		t.Errorf("the worst case laid out at %.1fmm (ok=%v), want 3.0mm", chosen.fontMM, ok)
	}
}

// TestHashlockRefusesWhatItCannotLayOut: EngraveHashlock refuses at the bottom
// rung rather than drawing a plate that runs off the steel, and the refusal
// names the measured ceiling.
//
// MUTATION: return the bottom rung's layout instead of an error -> the plate is
// engraved past the budget and this row fails.
func TestHashlockRefusesWhatItCannotLayOut(t *testing.T) {
	plate := h6WorstPhrasePlate(true)
	// Four locator rows instead of three: one row over the 1.20mm of spare.
	plate.Locator = append(append([]string{}, h6WorstLocator...), "matches hash 1 in the payload")
	_, err := EngraveHashlock(prodParams, plate)
	if err == nil {
		t.Fatal("EngraveHashlock accepted a plate one row over the budget")
	}
	if !strings.Contains(err.Error(), "budget of 416000") {
		t.Errorf("the refusal does not name the measured ceiling: %v", err)
	}
}

// TestHashlockStringFormIsLowercaseAndUngrouped pins §6.1's decision.
//
// The fork's ms1 SEED plate upper-cases and groups in tens; `ms hashlock`'s own
// engraving card groups too. This plate does neither, and that is measured
// rather than preferred: grouped in tens the 75 characters take 8 rows at 6.0mm
// (a 10-character group plus a separator plus the next group is 21 and will not
// fit on a 19-character line) against 4 ungrouped.
//
// MUTATION: group in tens -> the body grows by four rows and the plate no
// longer fits at 6.0mm.
// MUTATION: strings.ToUpper the MS1 -> the row comparison below fails, and the
// constant-time stringer panics because passphraseAlphabet holds no uppercase
// bech32 the seed stringer's 36-character alphabet would.
func TestHashlockStringFormIsLowercaseAndUngrouped(t *testing.T) {
	P := prodParams
	l, ok := hashlockFit(P, h6StringPlate(), 0)
	if !ok {
		t.Fatal("the string form does not fit at any rung")
	}
	if l.fontMM != 6.0 {
		t.Errorf("the string form laid out at %.1fmm, want 6.0mm", l.fontMM)
	}
	var body []string
	for _, r := range l.rows {
		if r.secret {
			body = append(body, r.text)
		}
	}
	if got := strings.Join(body, ""); got != h6CorpusMS1 {
		t.Errorf("the engraved string is %q, want the plate string %q verbatim", got, h6CorpusMS1)
	}
	if len(body) != 4 {
		t.Errorf("the 75-character string took %d rows at 6.0mm, want 4 (19 characters per line)", len(body))
	}
	for _, r := range body {
		if r != strings.ToLower(r) {
			t.Errorf("row %q is not lowercase", r)
		}
	}
}

// TestHashlockBodyWrapsOnCharacters is §6.5's wrap rule.
//
// MUTATION: wrap on word boundaries -> the 100-character phrase and the
// 75-character ms1 string are SINGLE TOKENS with no break point, so the layout
// either produces one 100-character row (which runs off the plate) or fails
// outright.
//
// THE WORST-CASE ROW IS WHAT MAKES THAT MUTATION FIRE HERE, and it was added
// after running it: with only the four-word sample below, a word wrapper puts
// all 28 characters on one 39-character line, the "every row but the last is
// full" loop has no rows to walk, and this row PASSES on the mutation it
// documents. (The suite as a whole still caught it -- the fit gate, the string
// form and three goldens all red -- but a row whose stated mutation cannot
// fire is a row that proves less than it claims.) A 100-character single token
// has no break point at all, so the wrapper's behaviour is visible in the row
// lengths themselves.
func TestHashlockBodyWrapsOnCharacters(t *testing.T) {
	P := prodParams
	perLine := CharsPerLine(P, constant.Font, 3.0)
	for _, tc := range []struct {
		name   string
		phrase string
	}{
		// A phrase of words, at a rung whose line length falls INSIDE a word.
		{"a phrase of words", "correct horse battery staple correct horse battery staple"},
		// The §4.3 cap as a SINGLE TOKEN: nothing for a word wrapper to break.
		{"the worst case, one token", h6WorstPhrase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plate := Hashlock{Form: HashlockPhrase, Phrase: tc.phrase,
				Method: h6SHA256MethodLine, Font: constant.Font}
			l := hashlockLayoutFor(P, plate, 3.0, 0)
			var body []string
			for _, r := range l.rows {
				if r.secret {
					body = append(body, r.text)
				}
			}
			joined := strings.Join(body, "")
			if joined != passphraseGlyphs(plate.Phrase) {
				t.Errorf("the wrapped rows rejoin to %q, want the phrase glyphs", joined)
			}
			if want := (len(joined) + perLine - 1) / perLine; len(body) != want {
				t.Errorf("%d characters took %d rows at %d per line, want %d",
					len(joined), len(body), perLine, want)
			}
			for i, r := range body[:len(body)-1] {
				if len(r) != perLine {
					t.Errorf("row %d is %d characters, want a full %d-character row: the wrap broke early, "+
						"which is what a word wrapper does", i, len(r), perLine)
				}
			}
		})
	}
}

// TestHashlockBandsAndBodyRows is §6.2 and §6.3.
//
// The three band literals are inside MaxTitleLen and are engraved VERBATIM, not
// through TitleString. The locator rows are BODY rows -- not because they are
// too wide (every one of them is 6 to 30 characters against a 32-character band
// cap at 3.0mm) but because a band holds at most TWO LINES and this stage has
// already spent both on the title, the space legend and NOT A SEED.
//
// MUTATION: draw any locator row in a band -> the band-ink height below exceeds
// the height the band literals alone produce.
func TestHashlockBandsAndBodyRows(t *testing.T) {
	P := prodParams
	for _, lit := range []string{hashlockTitleString, hashlockTitlePhrase, hashlockFooter} {
		if len(lit) > MaxTitleLen {
			t.Errorf("band literal %q is %d characters, over MaxTitleLen %d", lit, len(lit), MaxTitleLen)
		}
		if lit != strings.ToUpper(lit) {
			t.Errorf("band literal %q is not upper-case; it is engraved VERBATIM, never through TitleString", lit)
		}
	}
	if got := len(h6WorstLocator[2]); got != 29 {
		t.Errorf("the longest locator row is %d characters, want 29", got)
	}

	for _, tc := range []struct {
		name  string
		plate Hashlock
		lines int // band lines in the BOTTOM band
	}{
		{"string form", h6StringPlate(), 1},
		{"phrase form, no space", h6WorstPhrasePlate(false), 1},
		{"phrase form, a space", func() Hashlock {
			p := h6WorstPhrasePlate(false)
			p.Phrase = "correct horse"
			return p
		}(), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, ok := hashlockFit(P, tc.plate, 0)
			if !ok {
				t.Fatal("the plate does not fit at any rung")
			}
			// (a) EVERY body row is inside the inner margins, which is what
			// "the locator rows are BODY rows" means geometrically.
			lo, hi := P.I(innerMargin), P.F(plateSize)-P.I(innerMargin)
			if l.textY < lo || l.textY+l.blockH > hi {
				t.Errorf("the body spans [%d,%d], outside the inner margins [%d,%d]",
					l.textY, l.textY+l.blockH, lo, hi)
			}
			// (b) Each band carries at most the lines it declares. A band
			// offers innerMargin - outerMargin = 7mm and three 3mm lines need
			// 9mm and run off the plate edge, so this is the real constraint
			// on the locator -- not width.
			e, err := EngraveHashlock(P, tc.plate)
			if err != nil {
				t.Fatal(err)
			}
			top, bottom := h6BandInk(t, P, e)
			if h := top.Max.Y - top.Min.Y; h >= P.F(plateSmallFontSize) {
				t.Errorf("the top band inks %d units tall, over one %0.1fmm line: a body row is being drawn in it",
					h, plateSmallFontSize)
			}
			if h := bottom.Max.Y - bottom.Min.Y; h >= tc.lines*P.F(plateSmallFontSize) {
				t.Errorf("the bottom band inks %d units tall, over %d %0.1fmm line(s): a body row is being drawn in it",
					h, tc.lines, plateSmallFontSize)
			}
		})
	}
}

// h6BandInk returns the ink bounds inside each screw-hole band -- above
// innerMargin and below plateSize-innerMargin.
func h6BandInk(t testing.TB, P engrave.Params, e engrave.Engraving) (top, bottom bspline.Bounds) {
	t.Helper()
	lo, hi := P.I(innerMargin), P.F(plateSize)-P.I(innerMargin)
	first, firstB := true, true
	for k := range engrave.PlanEngraving(P.StepperConfig, e) {
		// ENGRAVING knots only. A travel move crosses both bands on its way to
		// the body on every plate, so counting moves would report the whole
		// band as inked and this assertion could never fail.
		if !k.Engrave {
			continue
		}
		p := k.Ctrl
		switch {
		case p.Y < lo:
			if first {
				top.Min, top.Max, first = p, p, false
			}
			top.Min = bezier.Pt(min(top.Min.X, p.X), min(top.Min.Y, p.Y))
			top.Max = bezier.Pt(max(top.Max.X, p.X), max(top.Max.Y, p.Y))
		case p.Y >= hi:
			if firstB {
				bottom.Min, bottom.Max, firstB = p, p, false
			}
			bottom.Min = bezier.Pt(min(bottom.Min.X, p.X), min(bottom.Min.Y, p.Y))
			bottom.Max = bezier.Pt(max(bottom.Max.X, p.X), max(bottom.Max.Y, p.Y))
		}
	}
	if first || firstB {
		t.Fatal("a band drew nothing: the title or the footer is missing, so this test proves nothing")
	}
	return top, bottom
}

// TestHashlockQREncodesTheTextNotTheString is §6.4 and §8.6.
//
// MUTATION: encode plate.MS1 -> the module comparison against §8.6's text
// fails.
// MUTATION: encode the SpaceMark-substituted glyphs -> the real-space
// comparison fails, which is the row standing between a reader and a phrase
// whose spaces a scanner would hand back as 0x1F.
func TestHashlockQREncodesTheTextNotTheString(t *testing.T) {
	plate := h6WorstPhrasePlate(true)
	plate.MS1 = h6CorpusMS1
	plate.Phrase = "correct horse battery staple"
	plate.QRText = h6QRText(true, md.KindSha256, plate.Phrase)

	got, err := HashlockQRCode(plate)
	if err != nil {
		t.Fatal(err)
	}
	want, err := qrpkg.Encode(h6QRText(true, md.KindSha256, "correct horse battery staple"), qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	h6SameCode(t, got, want)

	notWant, err := qrpkg.Encode(h6CorpusMS1, qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if h6CodesEqual(got, notWant) {
		t.Error("the QR encodes the ms1 string; decision 1 declines a QR of the string form entirely")
	}
	marked, err := qrpkg.Encode(h6QRText(true, md.KindSha256, passphraseGlyphs(plate.Phrase)), qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if h6CodesEqual(got, marked) {
		t.Error("the QR carries SpaceMark; a scanner that saw it would hand a reader different bytes")
	}
	// And the text block DOES carry the mark, on the same plate.
	l := hashlockLayoutFor(prodParams, plate, 3.0, got.Size)
	var body string
	for _, r := range l.rows {
		if r.secret {
			body += r.text
		}
	}
	if !strings.ContainsRune(body, SpaceMark) {
		t.Error("the text block engraves a real space; one space and two look identical on steel")
	}
}

func h6CodesEqual(a, b *qrpkg.Code) bool {
	if a.Size != b.Size {
		return false
	}
	for y := 0; y < a.Size; y++ {
		for x := 0; x < a.Size; x++ {
			if a.Black(x, y) != b.Black(x, y) {
				return false
			}
		}
	}
	return true
}

func h6SameCode(t testing.TB, got, want *qrpkg.Code) {
	t.Helper()
	if !h6CodesEqual(got, want) {
		t.Fatalf("the QR is not §8.6's text: dim %d against %d", got.Size, want.Size)
	}
}

// TestHashlockRefusesAQROnTheStringForm: decision 1, enforced rather than
// documented.
//
// MUTATION: drop the guard -> a preimage-string plate carries a QR of the ms1
// string, which is exactly the machine-readable copy of a spend secret ruling
// A2 declined.
func TestHashlockRefusesAQROnTheStringForm(t *testing.T) {
	plate := h6StringPlate()
	plate.QR = true
	plate.QRText = h6CorpusMS1
	if _, err := EngraveHashlock(prodParams, plate); err == nil {
		t.Fatal("a QR was accepted on the string form")
	}
}

// TestHashlockQRTextMatchesTheMSCorpus is the lockstep that keeps this
// package's two method-line literals from being a transcription.
//
// §8.6's text is decided in mnemonic-secret (`ms_codec::hashlock::qr_text`) and
// pinned by the `qr_text` rows of the vendored corpus, so the plate builder
// asserts against those rows rather than against itself. Without this the
// literals above would be exactly the "a literal it transcribed itself" the
// corpus exists to prevent, and a parameter change on the Rust side would move
// the plate's QR and nothing here would notice.
//
// MUTATION: change one character of h6HardenedMethodLine -> the hardened rows
// fail with the corpus text beside the built one.
// MUTATION: build the text as "phrase: %s\nmethod: %s" -> every row fails.
func TestHashlockQRTextMatchesTheMSCorpus(t *testing.T) {
	raw, err := os.ReadFile("../hashlock/testdata/hashlock-v0.8.json")
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no vendored hashlock corpus: %v", err)
	}
	var corpus struct {
		QRText []struct {
			Name   string `json:"name"`
			Method string `json:"method"`
			Kind   string `json:"kind"`
			Phrase string `json:"phrase"`
			Text   string `json:"qr_text"`
			Bytes  int    `json:"bytes"`
		} `json:"qr_text"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the vendored corpus: %v", err)
	}
	if len(corpus.QRText) < 7 {
		t.Fatalf("the corpus carries %d qr_text rows; H6 §11.2 pins seven", len(corpus.QRText))
	}
	for _, row := range corpus.QRText {
		kind, ok := md.HashKindFromToken(row.Kind)
		if !ok {
			t.Fatalf("row %s: corpus kind %q is not a §6 token", row.Name, row.Kind)
		}
		got := h6QRText(row.Method != "sha256", kind, row.Phrase)
		if got != row.Text {
			t.Errorf("row %s:\n got %q\nwant %q", row.Name, got, row.Text)
		}
		if len(got) != row.Bytes {
			t.Errorf("row %s: %d bytes, the corpus says %d", row.Name, len(got), row.Bytes)
		}
	}
}

// TestHashlockGoldens pins both forms at their fitting rungs, with and without
// the QR. New goldens are permitted; no existing one may move.
func TestHashlockGoldens(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plate Hashlock
	}{
		{"hashlock-string-6mm", h6StringPlate()},
		{"hashlock-phrase-noqr", h6WorstPhrasePlate(false)},
		{"hashlock-phrase-qr-v9", h6WorstPhrasePlate(true)},
		{"hashlock-phrase-space-legend", func() Hashlock {
			p := h6WorstPhrasePlate(false)
			p.Phrase = "correct horse battery staple"
			p.Method = h6SHA256MethodLine
			return p
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := EngraveHashlock(prodParams, tc.plate)
			if err != nil {
				t.Fatal(err)
			}
			spline := engrave.PlanEngraving(prodParams.StepperConfig, e)
			bounds := bspline.Bounds{Max: bezier.Point{X: plateSize * mm, Y: plateSize * mm}}
			p := filepath.Join("testdata", tc.name+".bin")
			if err := golden.CompareBSpline(p, *update, t.ArtifactDir(), prodParams.StrokeWidth, bounds, spline); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestHashlockPlateCutDurationsAreBounded LOGS what each form of this plate
// costs to cut, at PRODUCTION params, and bounds the worst case (§11.4, H6 R0
// round 0 journey I-4).
//
// IT EXISTS BECAUSE §12 item 8'S GATE HAS A COST, AND THE COST DECIDES WHETHER
// THE GATE IS RUN. That item budgeted the QR scan gate at "~2 s a try" with the
// single-character test-plate pattern, against "~21 min" for a full plate --
// the md1-plate figure, not this plate's. MEASURED here: the worst-case phrase
// plate WITH its v9 QR is 43m31s and the QR alone is 32m12s, so the gate is
// half an hour per attempt, and the single-character pattern cannot be applied
// to it at all: a constant-time QR is indivisible by construction, because
// ConstantQRCmd.Engrave runs `for range nmod` and pads every move to maxDur,
// which is the whole reason the toolpath is content-independent.
//
// THE BOUND IS A TRIPWIRE, NOT A PIN. Durations depend on the stepper profile
// and on the layout, and pinning them to the second would red on any harmless
// change. One hour is the number a layout change that DOUBLED this plate would
// cross, which is the class worth catching: a plate that no longer fits in one
// sitting changes the acceptance procedure, not just a number in a document.
//
// MUTATION: cut the production engraving speed to a third -> RUN, the worst
// case is 1h14m43s and this row reds with its own message. (The obvious
// mutation, engraving the QR at scale 3, reds EARLIER and elsewhere -- RUN,
// "the hashlock plate does not fit at any font size: 10 rows at 3.0mm need
// 510080 units against a budget of 416000" -- because the layout gate catches
// a doubled envelope before any duration is computed.)
func TestHashlockPlateCutDurationsAreBounded(t *testing.T) {
	p := sh2.Params()
	const bound = time.Hour
	for _, tc := range []struct {
		what  string
		plate Hashlock
	}{
		{"worst-case phrase plate WITH the v9 QR", h6WorstPhrasePlate(true)},
		{"the same plate with the QR removed", h6WorstPhrasePlate(false)},
		{"the string-form plate", h6StringPlate()},
	} {
		e, err := EngraveHashlock(p, tc.plate)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		d := engrave.TimePlan(p.StepperConfig, e)
		t.Logf("%-40s %v", tc.what, d)
		if d > bound {
			t.Errorf("%s takes %v to cut, over the %v bound: a plate that no longer fits "+
				"in one sitting changes §12 item 8's acceptance procedure", tc.what, d, bound)
		}
	}
	// The QR ALONE, which is the only reduction available to the gate: cutting
	// it onto a blank saves the text block and nothing more.
	code, err := HashlockQRCode(h6WorstPhrasePlate(true))
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := engrave.ConstantQR(code)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%-40s %v (dim %d, scale %d)", "the v9 QR alone",
		engrave.TimePlan(p.StepperConfig, cmd.Engrave(p.StepperConfig, p.StrokeWidth, hashlockQRScale)),
		code.Size, hashlockQRScale)
	// And one 6 mm character, the single-character test-plate pattern, so the
	// two costs sit side by side rather than one being assumed from the other.
	st := engrave.NewPassphraseStringer(constant.Font, p, 6*p.Millimeter)
	t.Logf("%-40s %v", "one 6 mm character", engrave.TimePlan(p.StepperConfig,
		engrave.Engraving(func(yield func(engrave.Command) bool) { st.String(yield, "W") })))
}
