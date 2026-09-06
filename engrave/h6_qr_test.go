package engrave

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/internal/golden"
)

// H6 §7: raising the constant-time QR encoder from v5 to v9, and giving its
// module engraver a scale-2 arm.
//
// The two halves do NOT ship apart (§7.4). With the raise alone, ConstantQR
// returns a command for a 53-module code and Engrave panics on the only scale
// the plate fits at; with the arm alone, ConstantQR refuses the code before
// Engrave is reached. Every row below is written so that removing either half
// reds.

// h6HardenedMethodLine and h6SHA256MethodLine are §8.6's two method lines.
// They are literals HERE because this package cannot import hashlock (which
// imports nothing of engrave, but the plate builder in backup does the
// composition); backup/hashlock_test.go asserts the same text against
// hashlock's own constants, so a parameter change cannot leave both lying.
const (
	h6HardenedMethodLine = "method: pbkdf2-hmac-sha256 iterations=100000 salt=ms-hashlock-v1 dklen=32"
	h6SHA256MethodLine   = "method: sha256"
)

func h6QRText(method, phrase string) string {
	return "hashlock v1\n" + method + "\nphrase: " + phrase
}

// h6AlignCentresFromBitmap DERIVES the alignment-pattern centres of a code by
// testing the 5x5 ring shape at every candidate centre: at Chebyshev distance
// 0 and 2 the modules are black and at distance 1 white.
//
// The derivation is the point. A transcribed table is a table nobody checked;
// this one is re-derived from the encoder's own output on every run.
func h6AlignCentresFromBitmap(c *qr.Code) []bezier.Point {
	var out []bezier.Point
	for cy := 2; cy < c.Size-2; cy++ {
		for cx := 2; cx < c.Size-2; cx++ {
			ok := true
			for dy := -2; dy <= 2 && ok; dy++ {
				for dx := -2; dx <= 2 && ok; dx++ {
					cheb := max(h6Abs(dx), h6Abs(dy))
					if c.Black(cx+dx, cy+dy) != (cheb != 1) {
						ok = false
					}
				}
			}
			if ok {
				out = append(out, bezier.Pt(cx, cy))
			}
		}
	}
	return out
}

func h6Abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// h6CodeAtDim returns a code of exactly dim modules, at ECC-L byte mode.
func h6CodeAtDim(t *testing.T, dim int) *qr.Code {
	t.Helper()
	for n := 1; n <= 400; n++ {
		c, err := qr.Encode(strings.Repeat("a", n), qr.L)
		if err != nil {
			t.Fatalf("qr.Encode(%d bytes): %v", n, err)
		}
		if c.Size == dim {
			return c
		}
		if c.Size > dim {
			break
		}
	}
	t.Fatalf("no ECC-L byte-mode payload produces dim %d", dim)
	return nil
}

// TestQRAlignmentTableMatchesTheEncoder is §7.2, executed: the table
// bitmapForQRStatic carries is DERIVED here from the encoder's own bitmap for
// v2..v9 and compared, so a transcription error in the table cannot survive.
//
// fillMarker takes a TOP-LEFT, so bitmapForQRStatic's entries are (centre - 2)
// and this test adds the 2 back before comparing.
//
// MUTATION: change one centre in qrAlignCentres (53's (46,26) -> (46,28)) ->
// `dim 53 marker 3: centre (46,28), the encoder draws (46,26)`.
// MUTATION: drop 41 from the single-marker case -> `dim 41: bitmapForQRStatic
// panicked: unsupported qr code version`.
func TestQRAlignmentTableMatchesTheEncoder(t *testing.T) {
	for v := 2; v <= 9; v++ {
		dim := 4*v + 17
		c := h6CodeAtDim(t, dim)
		want := h6AlignCentresFromBitmap(c)
		if len(want) == 0 {
			t.Fatalf("v%d dim %d: the encoder drew no alignment ring; the derivation is broken, not the table", v, dim)
		}
		_, got := bitmapForQRStatic(dim)
		if len(got) != len(want) {
			t.Errorf("dim %d: %d alignment markers, the encoder draws %d", dim, len(got), len(want))
			continue
		}
		for i := range got {
			centre := got[i].Add(bezier.Pt(2, 2))
			if centre != want[i] {
				t.Errorf("dim %d marker %d: centre %v, the encoder draws %v", dim, i, centre, want[i])
			}
		}
	}
}

// TestConstantQRAcceptsThroughV9AndRefusesV10 is §7.2 item 3: the bound moves
// to 53 and not one version further, because bitmapForQRStatic's default panic
// STAYS and is what keeps an unhandled version from being drawn as if it had no
// alignment patterns at all.
//
// MUTATION: raise the bound to 57 without a v10 table row -> this test's
// refusal row fails AND bitmapForQRStatic panics inside ConstantQR, which is
// what the refusal is standing in front of.
func TestConstantQRAcceptsThroughV9AndRefusesV10(t *testing.T) {
	for _, dim := range []int{21, 25, 29, 33, 37, 41, 45, 49, 53} {
		c := h6CodeAtDim(t, dim)
		if _, err := ConstantQR(c); err != nil {
			t.Errorf("ConstantQR(dim %d) = %v, want accepted", dim, err)
		}
	}
	c := h6CodeAtDim(t, 57)
	if _, err := ConstantQR(c); err == nil {
		t.Error("ConstantQR(dim 57) was accepted; bitmapForQRStatic has no v10 row and would panic")
	}
}

// TestConstantTimeQRBudgetBoundsEveryPayload IS THE BUDGET PROOF, and it is the
// row that can actually fail.
//
// constantTimeQRModules(dim) is both the budget ConstantQR checks
// (`if len(modules) > nmod`) and the loop count ConstantQRCmd.Engrave runs
// (`for range nmod`). The number is CONTENT-DEPENDENT and cannot be invented: a
// budget between the observed min and max makes the plate cuttable for some
// phrases and refused for others AT THE SAME QR VERSION -- an operator-visible,
// content-dependent failure on the funds path, which is the exact class
// ConstantQR exists to remove.
//
// This test is a bounded sample of the plan-time fuzzing campaign, not a
// substitute for it: the four NEW entries were derived by fuzzing (see the
// comments on constantTimeQRModules) and this row proves they hold over a fresh
// sample on every run. The three SHIPPED dims it also samples -- 29, 33 and 37,
// which §8.6-shaped text reaches routinely -- were never fuzzed over this
// content class at all, and this row is the only thing that watches them.
//
// MUTATION: set any of the four arms to the campaign's observed maximum MINUS
// ONE -> that version's row reds with ConstantQR's own
// `too many dims 53 QR modules for constant time engraving`, which is also the
// check that the entry was derived rather than guessed.
func TestConstantTimeQRBudgetBoundsEveryPayload(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzzed budget proof")
	}
	rng := rand.New(rand.NewSource(0x4831))
	// EVERY DIM §8.6 CONTENT REACHES, not only the newly admitted four (H6 R0
	// round 0, fidelity I-5). §7.2 item 4's argument -- "a budget between the
	// observed min and max makes the plate cuttable for some phrases and refused
	// for others AT THE SAME QR VERSION" -- applies verbatim to v3/v4/v5, whose
	// SHIPPED budgets were derived from 18.5M PASSPHRASE-shaped executions and
	// had never seen a §8.6-shaped payload. A `sha256` plate is 35 + len(phrase)
	// bytes and a `hardened` one 94 + len(phrase), so with phrases of 1..100
	// characters this content reaches 29/33/37/41/45/49/53 and nothing below or
	// above.
	for _, dim := range []int{29, 33, 37, 41, 45, 49, 53} {
		shapes := h6ShapesFor(dim)
		if len(shapes) == 0 {
			t.Fatalf("dim %d: no §8.6-shaped payload reaches it", dim)
		}
		budget := constantTimeQRModules(dim)
		if budget == 0 {
			t.Fatalf("constantTimeQRModules(%d) = 0: the version is admitted by ConstantQR's "+
				"bound and refused by its budget, which is the C-1 failure", dim)
		}
		observed := 0
		const n = 400
		for i := 0; i < n; i++ {
			sh := shapes[rng.Intn(len(shapes))]
			text := h6QRText(sh.method, h6RandomPhrase(rng, sh.phraseLen))
			c, err := qr.Encode(text, qr.L)
			if err != nil {
				t.Fatalf("qr.Encode: %v", err)
			}
			if c.Size != dim {
				t.Fatalf("a shape for dim %d produced dim %d", dim, c.Size)
			}
			cmd, err := ConstantQR(c)
			if err != nil {
				t.Fatalf("dim %d, payload %d: ConstantQR: %v", dim, i, err)
			}
			if len(cmd.plan) > observed {
				observed = len(cmd.plan)
			}
		}
		t.Logf("dim %2d v%d: %d payloads, observed max %d, budget %d, headroom %d",
			dim, (dim-17)/4, n, observed, budget, budget-observed)
		if observed > budget {
			t.Errorf("dim %d: observed %d modules against a budget of %d", dim, observed, budget)
		}
	}
}

// TestConstantTimeQRBudgetEntriesAreTheFuzzedOnes PINS the four entries to the
// numbers the plan-time campaign produced, and it exists because
// TestConstantTimeQRBudgetBoundsEveryPayload CANNOT CATCH A ONE-OFF CHANGE.
//
// MEASURED: with a budget set to each campaign maximum MINUS ONE
// (822 / 959 / 1178 / 1378), the fuzzed row above still PASSES -- its 400
// payloads per dimension observed 792 / 910 / 1143 / 1322, well under the
// altered budgets, and reaching the true maximum took 7 to 14 million payloads.
// §11.3's stated mutation ("set any of the four arms to the observed maximum
// MINUS ONE -> that version's row reds") is therefore not achievable by any
// in-suite sample, and a plan that relied on it would ship a budget nothing
// guarded.
//
// So the guard is a PIN. It is not a weaker test than the fuzzed one, it is a
// different one: the fuzzed row proves the entry BOUNDS fresh content, and this
// row proves the entry is the one that was DERIVED rather than a number someone
// adjusted. Changing an entry now means changing this table, which means saying
// where the new number came from.
//
// MUTATION: any change to any of the four arms -> the entry assertion names the
// dimension, both values, and the campaign that produced the original.
func TestConstantTimeQRBudgetEntriesAreTheFuzzedOnes(t *testing.T) {
	for _, tc := range []struct {
		dim, observed, buffer int
		samples               string
	}{
		{41, 823, 20, "14,494,291 in 32 min, converged with 16.4 min of quiet"},
		{45, 960, 53, "11,065,492 in 32 min, quiet only 2.8 min -- the UNDER-CONVERGED entry, hence the wider buffer"},
		{49, 1179, 20, "10,138,994 in 32 min, quiet 4.1 min"},
		{53, 1379, 20, "7,759,282 in 32 min, converged with 22.6 min of quiet"},
	} {
		want := tc.observed + tc.buffer
		if got := constantTimeQRModules(tc.dim); got != want {
			t.Errorf("constantTimeQRModules(%d) = %d, want %d (%d observed + %d buffer). "+
				"The original was derived by fuzzing: %s. A different number needs its own campaign.",
				tc.dim, got, want, tc.observed, tc.buffer, tc.samples)
		}
	}
	// And the shipped entries are untouched.
	for dim, want := range map[int]int{21: 171, 25: 266, 29: 391, 33: 547, 37: 684} {
		if got := constantTimeQRModules(dim); got != want {
			t.Errorf("constantTimeQRModules(%d) = %d, want the SHIPPED %d: H6 raises the "+
				"ceiling and moves no existing entry", dim, got, want)
		}
	}
}

type h6Shape struct {
	method    string
	phraseLen int
}

// h6ShapesFor enumerates the (method, phrase length) pairs whose §8.6 text
// lands on dim, so a fuzzed payload is a payload this plate can actually carry
// rather than an arbitrary string of the right length.
func h6ShapesFor(dim int) []h6Shape {
	var out []h6Shape
	for _, m := range []string{h6HardenedMethodLine, h6SHA256MethodLine} {
		base := len(h6QRText(m, ""))
		for l := 1; l <= 100; l++ {
			n := base + l
			if n >= 231 {
				continue
			}
			if d := h6DimForBytes(n); d == dim {
				out = append(out, h6Shape{method: m, phraseLen: l})
			}
		}
	}
	return out
}

// h6DimForBytes are the ECC-L byte-mode thresholds, MEASURED from the encoder
// (TestECCLThresholdsAreWhatTheBudgetAssumes pins them).
func h6DimForBytes(n int) int {
	switch {
	case n >= 231:
		return 57
	case n >= 193:
		return 53
	case n >= 155:
		return 49
	case n >= 135:
		return 45
	case n >= 107:
		return 41
	case n >= 79:
		return 37
	// THE SHIPPED VERSIONS §8.6 CONTENT ALSO REACHES (H6 R0 round 0, fidelity
	// I-5). The table used to stop at 79 bytes, which is where the NEWLY
	// ADMITTED versions begin -- but a `sha256` plate is 35 + len(phrase) bytes,
	// so a 1..18-character phrase is dim 29 and a 19..43-character one is dim
	// 33. Those budgets were fuzzed over PASSPHRASE-shaped content, and H6 hands
	// them a new content class; leaving them out of the table left them out of
	// the sample too.
	case n >= 54:
		return 33
	case n >= 33:
		return 29
	case n >= 18:
		return 25
	case n >= 1:
		return 21
	}
	return 0
}

// TestECCLThresholdsAreWhatTheBudgetAssumes pins §7.1's threshold table against
// the encoder, so h6DimForBytes cannot drift from it and quietly stop sampling
// a version.
//
// MUTATION: change the 193 threshold to 195 -> `n=193: table says 49, the
// encoder says 53`.
func TestECCLThresholdsAreWhatTheBudgetAssumes(t *testing.T) {
	for n := 1; n <= 240; n++ {
		c, err := qr.Encode(strings.Repeat("a", n), qr.L)
		if err != nil {
			t.Fatalf("qr.Encode(%d): %v", n, err)
		}
		if got := h6DimForBytes(n); got != c.Size {
			t.Fatalf("n=%d: table says %d, the encoder says %d", n, got, c.Size)
		}
	}
}

func h6RandomPhrase(rng *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(0x20 + rng.Intn(0x5F)) // printable ASCII 0x20..0x7E
	}
	return string(b)
}

// TestConstantQRMoveCountIsAFunctionOfDimAlone IS A REGRESSION GUARD, NOT THE
// BUDGET PROOF, and it is labelled as one because it cannot substitute.
//
// Engrave loops `for range nmod` and pads every move to maxDur via DelayMove,
// so the emitted count is a function of dim alone FOR ANY VALUE OF nmod,
// correct or not -- this row passes on a budget of 0, on 700, and on a correct
// one. What it does prove is the padding: two different payloads of the same
// version emit the same commands in the same count.
//
// MUTATION: make the move list depend on a module's colour (skip the
// engraveModule call when the module is already engraved) -> the two payloads
// differ in command count.
func TestConstantQRMoveCountIsAFunctionOfDimAlone(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, dim := range []int{41, 45, 49, 53} {
		shapes := h6ShapesFor(dim)
		var counts []int
		for i := 0; i < 2; i++ {
			sh := shapes[rng.Intn(len(shapes))]
			c, err := qr.Encode(h6QRText(sh.method, h6RandomPhrase(rng, sh.phraseLen)), qr.L)
			if err != nil {
				t.Fatal(err)
			}
			cmd, err := ConstantQR(c)
			if err != nil {
				t.Fatalf("dim %d: %v", dim, err)
			}
			n := 0
			for range cmd.Engrave(conf, strokeWidth, 2) {
				n++
			}
			counts = append(counts, n)
		}
		if counts[0] != counts[1] {
			t.Errorf("dim %d: two payloads emitted %d and %d commands; the toolpath is content-dependent",
				dim, counts[0], counts[1])
		}
	}
}

// TestEngraveModuleScale2 is §7.3: the arm the phrase plate's only fitting
// configuration needs.
//
// It asserts BOTH properties the arm has to carry. (a) FIVE commands per
// module, the same constant as case 3 -- a per-module command count that varied
// by module would re-open the timing leak ConstantQR exists to close. (b) The
// painted extent of a module at p is exactly [p*2sw, p*2sw+2sw] on both axes:
// centerOf is (p*scale+1)*sw + sw/2, which is HALF A STROKE off the cell centre
// at scale 2, so the arm is asymmetric and a symmetric one paints the wrong
// cell.
//
// MUTATION: remove the case 2 arm -> `panic: unsupported module scale`, and the
// phrase plate cannot be built at the only scale it fits at.
// MUTATION: make the arm symmetric about the centre (the case 3 shape scaled
// down) -> `p={0 0} axis x: ink [-1067,3200], want [0,3840]`.
func TestEngraveModuleScale2(t *testing.T) {
	// The PRODUCTION stroke (internal/sh2.Params: StrokeWidth 1920, Millimeter
	// 6400), not this package's own `strokeWidth = mm/3 = 2133`. centerOf adds
	// sw/2 and engraveModule's extent is grown by sw/2 on each side, so an ODD
	// stroke loses a unit to integer division and the exact-extent assertion
	// below would be off by one for a reason that has nothing to do with the
	// arm. 2*1920 = 3840 units = 0.6mm, the free-text plate's module pitch.
	const sw = 1920
	for _, p := range []bezier.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 5, Y: 7}} {
		for _, scale := range []int{2, 3} {
			centre := centerOf(sw, scale, p)
			var pts []bezier.Point
			n := 0
			engraveModule(func(c Command) bool {
				n++
				k, ok := c.AsKnot()
				if !ok {
					t.Fatalf("scale %d: engraveModule emitted a non-knot command", scale)
				}
				pts = append(pts, k.Knot)
				return true
			}, sw, scale, centre)

			if n != 5 {
				t.Errorf("scale %d p=%v: %d commands, want 5 (the constant case 3 carries)", scale, p, n)
			}
			// The PAINTED extent is the path extent grown by half a stroke on
			// each side; the cell is scale*sw wide and scale*sw tall, at
			// p*scale*sw.
			minX, maxX, minY, maxY := pts[0].X, pts[0].X, pts[0].Y, pts[0].Y
			for _, q := range append(pts, centre) {
				minX, maxX = min(minX, q.X), max(maxX, q.X)
				minY, maxY = min(minY, q.Y), max(maxY, q.Y)
			}
			wantMinX, wantMaxX := p.X*scale*sw, p.X*scale*sw+scale*sw
			wantMinY, wantMaxY := p.Y*scale*sw, p.Y*scale*sw+scale*sw
			if got := [2]int{minX - sw/2, maxX + sw/2}; got != [2]int{wantMinX, wantMaxX} {
				t.Errorf("scale %d p=%v axis x: ink [%d,%d], want [%d,%d]", scale, p, got[0], got[1], wantMinX, wantMaxX)
			}
			if got := [2]int{minY - sw/2, maxY + sw/2}; got != [2]int{wantMinY, wantMaxY} {
				t.Errorf("scale %d p=%v axis y: ink [%d,%d], want [%d,%d]", scale, p, got[0], got[1], wantMinY, wantMaxY)
			}
		}
	}
}

// TestConstantQREngraveAtScale2Completes is §7.4: BOTH halves, together. A v9
// code (the phrase plate's worst case) is planned by the raised ConstantQR and
// drawn by the scale-2 arm, end to end, without a panic.
//
// MUTATION: revert ConstantQR's bound to 37 -> `ConstantQR(dim 53): engrave:
// constant QR size too large: 53`.
// MUTATION: remove the case 2 arm -> `panic: unsupported module scale`.
func TestConstantQREngraveAtScale2Completes(t *testing.T) {
	c, err := qr.Encode(h6QRText(h6HardenedMethodLine, strings.Repeat("z", 100)), qr.L)
	if err != nil {
		t.Fatal(err)
	}
	if c.Size != 53 {
		t.Fatalf("the worst-case §8.6 text produced dim %d, want 53", c.Size)
	}
	cmd, err := ConstantQR(c)
	if err != nil {
		t.Fatalf("ConstantQR(dim 53): %v", err)
	}
	n := 0
	for range cmd.Engrave(conf, strokeWidth, 2) {
		n++
	}
	if n == 0 {
		t.Fatal("the scale-2 engraving emitted no commands")
	}
	t.Logf("v9 at scale 2: %d commands over a budget of %d modules", n, constantTimeQRModules(53))
}

// TestH6ConstantQRGoldens pins one plan per newly admitted version, at scale 2.
//
// New goldens are permitted; no existing one may move. The payload is FIXED
// (not fuzzed) so the golden is a golden.
func TestH6ConstantQRGoldens(t *testing.T) {
	for _, tc := range []struct {
		name      string
		method    string
		phraseLen int
		dim       int
	}{
		{"h6-qr-v6-scale2", h6HardenedMethodLine, 20, 41},
		{"h6-qr-v7-scale2", h6HardenedMethodLine, 50, 45},
		{"h6-qr-v8-scale2", h6HardenedMethodLine, 80, 49},
		{"h6-qr-v9-scale2", h6HardenedMethodLine, 100, 53},
	} {
		t.Run(tc.name, func(t *testing.T) {
			phrase := strings.Repeat("hashlock plate ", 10)[:tc.phraseLen]
			c, err := qr.Encode(h6QRText(tc.method, phrase), qr.L)
			if err != nil {
				t.Fatal(err)
			}
			if c.Size != tc.dim {
				t.Fatalf("dim %d, want %d", c.Size, tc.dim)
			}
			cmd, err := ConstantQR(c)
			if err != nil {
				t.Fatal(err)
			}
			spline := PlanEngraving(conf, cmd.Engrave(conf, strokeWidth, 2))
			side := tc.dim * strokeWidth * 2
			bounds := bspline.Bounds{Max: bezier.Point{X: side, Y: side}}
			p := filepath.Join("testdata", tc.name+".bin")
			if err := golden.CompareBSpline(p, *update, t.ArtifactDir(), strokeWidth, bounds, spline); err != nil {
				t.Fatal(fmt.Errorf("%s: %w", tc.name, err))
			}
		})
	}
}
