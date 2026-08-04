package engrave

import (
	"encoding/binary"
	"sort"
	"testing"
	"unicode"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/vector"
)

// The passphrase alphabet must cover every printable ASCII rune plus the
// visible-space mark, be in ascending codepoint order (NewConstantStringer
// binary-searches it, engrave.go:1208-1210), and construct without panicking.
func TestPassphraseAlphabet(t *testing.T) {
	if passphraseAlphabet[0] != 0x1F {
		t.Errorf("alphabet must start with the space mark 0x1F, got %#x", passphraseAlphabet[0])
	}
	var last rune = -1
	for _, r := range passphraseAlphabet {
		if r <= last {
			t.Fatalf("alphabet not ascending at %q", r)
		}
		last = r
	}
	for r := rune(0x20); r <= 0x7E; r++ {
		found := false
		for _, a := range passphraseAlphabet {
			if a == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("alphabet missing %q", r)
		}
	}
	if got, want := len([]rune(passphraseAlphabet)), 96; got != want {
		t.Errorf("alphabet has %d runes, want %d (95 printable + mark)", got, want)
	}
	// Must not panic: exercises the ascending-order check (engrave.go:1210),
	// the alphabet-subset-of-face check (:1215), uniform advance (:1218) and
	// the zero-run assertion.
	NewPassphraseStringer(constant.Font, params(), 4*mm)
}

// planTicks sums the duration of every knot of a constant-time string. It is
// the quantity a timing observer measures.
func planTicks(s *ConstantStringer, txt string) uint {
	var total uint
	for k := range PlanEngraving(conf, func(yield func(Command) bool) {
		s.String(yield, txt)
	}) {
		total += k.T
	}
	return total
}

// advDur is the padding target of every move between slots, and -- per spec
// §3.5.0 -- of every move between the runs of one glyph too.
func advDur(s *ConstantStringer) uint {
	return timeMove(s.conf, s.advDist+s.startEndDist)
}

// TestPassphraseRunPartition pins how many engrave runs each glyph costs.
// Padding is per run, so a k-run glyph costs k units of runeDuration; a redraw
// that changes k changes the plate's engraving time and the size of the timing
// disclosure §3.5.0 accepts, and must be a deliberate edit here.
//
// NOTE: spec §3.5.0's "Reduce k before quantizing" requires '#', '*', '$' and
// 'x' to be single strokes, so that max k = 2 and the worst case is 2L. '*'
// and 'x' were reduced; '#' is still FOUR runs and '$' still two, so max k = 4
// and the 2L bound does not hold yet. See the Task 4 report.
func TestPassphraseRunPartition(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)
	byRuns := map[int]string{}
	for _, cr := range s.alphabet {
		byRuns[len(cr.Info)] += string(cr.R)
	}
	want := map[int]string{
		0: " ",
		2: "!\"$%:;=?ij",
		4: "#",
	}
	for k, w := range want {
		if got := byRuns[k]; got != w {
			t.Errorf("glyphs with %d runs: got %q, want %q", k, got, w)
		}
	}
	if got, want := len(byRuns[1]), 96-1-10-1; got != want {
		t.Errorf("single-run glyphs: got %d, want %d", got, want)
	}
	if !s.hasMultiRun {
		t.Error("hasMultiRun not set on an alphabet with multi-run glyphs")
	}
	if shared := NewConstantStringer(constant.Font, params(), 4*mm); shared.hasMultiRun {
		t.Error("hasMultiRun set on the single-run shared alphabet")
	}
}

// TestPassphraseZeroRunSlot exercises the zero-run path of paddedString.
//
// 0x20 has an advance but an empty spline, so it engraves no runs at all. It
// cannot be given its own Delay: with no knots to spend it on, timeScaler
// would still hold the whole unit when the next move arrives and panic
// "scale already in effect" mid-plate. paddedString instead folds the unit
// into the padded move to the slot.
//
// The plate path never reaches this: spec §3.3 substitutes the visible-space
// mark 0x1F for every space before layout. The stringer is therefore driven
// directly here, which is the only way the path is covered at all.
func TestPassphraseZeroRunSlot(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)

	// Must not panic, and the space must not distort the plan's shape.
	spline := verifiedEngraving(t, conf, PlanEngraving(conf, func(yield func(Command) bool) {
		s.String(yield, "A A")
	}))
	var zeroRun uint
	for k := range spline {
		zeroRun += k.T
	}

	oneRun := planTicks(s, "AAA")
	if zeroRun != oneRun {
		t.Errorf("a zero-run slot costs %d ticks, a one-run slot %d: they must be identical", zeroRun, oneRun)
	}

	// The slot itself costs exactly one padded move plus one padded unit.
	slot := zeroRun - planTicks(s, "AA")
	if want := advDur(s) + s.runeDuration; slot != want {
		t.Errorf("zero-run slot costs %d ticks, want advDur+runeDuration = %d+%d = %d",
			slot, advDur(s), s.runeDuration, want)
	}

	// A space in the first slot is padded to centerDur rather than advDur, so
	// only assert it engraves; and a leading and trailing space together must
	// still cost the same as two ordinary glyphs.
	if got, want := planTicks(s, " A "), planTicks(s, "BAB"); got != want {
		t.Errorf("spaces at the ends cost %d ticks, ordinary glyphs %d", got, want)
	}
}

// TestPassphrasePerRunTiming is the weakened form of TestConstantWords that
// per-run quantization leaves standing (spec §3.5.0, §7(b)).
//
// Padding is per run, so two strings are indistinguishable exactly when they
// agree in length AND in their per-slot run counts -- not, as before, for any
// two strings of the same length.
func TestPassphrasePerRunTiming(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)
	profile := func(txt string) Profile {
		return ProfileSpline(PlanEngraving(conf, func(yield func(Command) bool) {
			s.String(yield, txt)
		}))
	}
	// Run counts 1,2,2,1,4 in all four, over disjoint two-run glyph sets.
	ref := profile("Ai:B#")
	for _, txt := range []string{
		"Cj;D#", // i->j, :->;
		"Z!\"Y#",
		"0=%1#",
		"e$?f#",
	} {
		if got := profile(txt); !got.Equal(ref) {
			t.Errorf("%q: not constant against %q", txt, "Ai:B#")
		}
	}
	// The assertion has teeth: a string of the same length but a different
	// unit count must be distinguishable, or the checks above prove nothing.
	if profile("ABCDE").Equal(ref) {
		t.Error("a 5-unit string and a 10-unit string have the same profile; the test is vacuous")
	}

	// The intra-glyph, inter-run move is padded to advDur -- the same target
	// as an inter-glyph move (spec §3.5.0, normative). Were it padded to
	// anything else, a two-run glyph would cost a different amount from two
	// one-run glyphs and the multi-run glyphs' POSITIONS in a row would leak,
	// not merely their count. Same length is not required here: padDur does
	// not depend on the length when shortest == longest.
	if got, want := planTicks(s, "Ai"), planTicks(s, "AAA"); got != want {
		t.Errorf("a 1-run + 2-run pair costs %d ticks, three 1-run glyphs %d: "+
			"the inter-run move is not padded to advDur", got, want)
	}
	if got, want := planTicks(s, "#"), planTicks(s, "AAAA"); got != want {
		t.Errorf("the four-run '#' costs %d ticks, four 1-run glyphs %d", got, want)
	}
}

// TestPassphraseEngraveAlphabet engraves every rune of the alphabet.
//
// Construction alone proves nothing: every failure mode of per-run
// quantization -- "scale already in effect", "invalid scale", "unaligned
// delay", "delay during spline", "unclamped spline" -- fires while the plan is
// being drawn, not while the stringer is being built.
func TestPassphraseEngraveAlphabet(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)

	// Whole alphabet in one call, kinematics verified.
	var total uint
	var prof Profile
	{
		spline := verifiedEngraving(t, conf, PlanEngraving(conf, func(yield func(Command) bool) {
			s.String(yield, passphraseAlphabet)
		}))
		var seg bspline.Segment
		for k := range spline {
			_, ticks, _ := seg.Knot(k)
			total += ticks
		}
		prof = ProfileSpline(PlanEngraving(conf, func(yield func(Command) bool) {
			s.String(yield, passphraseAlphabet)
		}))
	}

	// The whole-alphabet call must cost exactly the sum of its units: one
	// padded unit per engrave run, one for the zero-run glyph, and one padded
	// move per unit.
	units, engraveBlocks := 0, 0
	for _, r := range passphraseAlphabet {
		idx, found := sort.Find(len(s.alphabet), func(i int) int {
			return int(r - s.alphabet[i].R)
		})
		if !found {
			t.Fatalf("alphabet rune %q not in the stringer", r)
		}
		runs := len(s.alphabet[idx].Info)
		units += max(runs, 1)
		engraveBlocks += runs
	}
	// The fixed overhead is anchored on a one-unit call rather than restated:
	// the call opens with an UNPADDED approach move from the machine origin
	// (engrave.go, paddedString's first Move) and closes with a padDur park,
	// neither of which depends on the content -- see spec §3.5.0's
	// "the accepted disclosure" for why the bookends are separable.
	unit := s.runeDuration + advDur(s)
	base := planTicks(s, "A") - unit
	if want := base + uint(units)*unit; total != want {
		t.Errorf("alphabet engraves in %d ticks, want %d (%d units over %d runes)",
			total, want, units, len([]rune(passphraseAlphabet)))
	}
	// Each engrave block starts and ends one move<->engrave transition, and
	// ProfileSpline appends the grand total. The zero-run glyph contributes a
	// unit of time but no engrave block.
	if got, want := len(prof.Pattern), 2*engraveBlocks+1; got != want {
		t.Errorf("plan switches %d times, want %d (%d engrave blocks)", got, want, engraveBlocks)
	}

	// And every rune on its own, so a defect in one glyph is not masked by
	// the rest of the alphabet.
	for _, r := range passphraseAlphabet {
		t.Run(string(r), func(t *testing.T) {
			spline := verifiedEngraving(t, conf, PlanEngraving(conf, func(yield func(Command) bool) {
				s.String(yield, string(r))
			}))
			for range spline {
			}
		})
	}
}

// TestPassphrasePaddingCoversEveryMove asserts the invariant spec §3.5.0 (ii)
// exists to protect: every move the stringer can be asked to make fits inside
// the duration it is padded to.
//
// Nothing checks this at construction. It fires at engrave time, as
// timeScaler's "invalid scale" panic (Reset with n < d), and only for the
// content that happens to need the move the bounds failed to cover -- so a
// plate could engrave for a hundred characters and then crash on the one
// glyph pair that exceeds the padding.
//
// This asserts the invariant, NOT the accumulation that establishes it. On the
// current face the two coincide: no glyph's second or later run reaches outside
// the box of the first runs, so accumulating over the first run only yields
// byte-identical startEndDist and center, and no test can tell the two apart.
// (ii) is therefore a correctness requirement for glyphs that do not yet exist
// -- this test is what will catch the first one that does.
func TestPassphrasePaddingCoversEveryMove(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)
	advDur, centerDur := advDur(s), timeMove(s.conf, (s.startEndDist+1)/2)
	padDur := timeMove(s.conf, s.startEndDist)

	var runs []constantPlan
	for _, cr := range s.alphabet {
		runs = append(runs, cr.Info...)
		if len(cr.Info) == 0 {
			// The zero-run glyph is moved to and parked at the slot origin.
			runs = append(runs, constantPlan{})
		}
	}
	for _, from := range runs {
		// The opening move, from the data-independent start position.
		if got := timeMove(s.conf, ManhattanDist(s.center, from.Start)); got > centerDur {
			t.Fatalf("opening move to a run start takes %d ticks, padded to centerDur=%d", got, centerDur)
		}
		// The closing park move. dot has advanced one slot past the last
		// glyph, and the park sits half a slot back (engrave.go, paddedString).
		park := bezier.Pt(s.advDist/2+s.center.X, s.center.Y)
		if got := timeMove(s.conf, ManhattanDist(from.End, park)); got > padDur {
			t.Fatalf("park move from a run end takes %d ticks, padded to padDur=%d", got, padDur)
		}
		for _, to := range runs {
			// dx = 0 is a move between two runs of one glyph; dx = advDist
			// is a move on to the next glyph. Both are padded to advDur.
			for _, dx := range []int{0, s.advDist} {
				d := ManhattanDist(from.End, bezier.Pt(to.Start.X+dx, to.Start.Y))
				if got := timeMove(s.conf, d); got > advDur {
					t.Fatalf("a move of %d steps takes %d ticks, padded to advDur=%d", d, got, advDur)
				}
			}
		}
	}
}

// TestPassphrasePaddedStringGuard pins the guarantee PaddedString loses under
// per-run quantization (spec §3.5.0, I4).
//
// paddedString runs exactly `longest` slots whatever the content, repeating
// runes to fill. On a single-run alphabet every slot costs one unit, so the
// call's duration is independent of the string INCLUDING its length within
// [shortest, longest] -- what seed plates rely on. With multi-run glyphs the
// total is the sum of the runs actually engraved, repeats included, so padding
// to a range would disclose the length. A prose prohibition would be
// self-defeating because String is PaddedString(yield, txt, n, n).
func TestPassphrasePaddedStringGuard(t *testing.T) {
	s := NewPassphraseStringer(constant.Font, params(), 4*mm)
	drain := func(f func(yield func(Command) bool) bool) (panicked any) {
		defer func() { panicked = recover() }()
		for range PlanEngraving(conf, func(yield func(Command) bool) { f(yield) }) {
		}
		return nil
	}
	if got := drain(func(yield func(Command) bool) bool {
		return s.PaddedString(yield, "ab", 2, 4)
	}); got == nil {
		t.Error("PaddedString accepted shortest != longest on a multi-run alphabet")
	}
	// String must keep working: it is PaddedString(yield, txt, n, n).
	if got := drain(func(yield func(Command) bool) bool {
		return s.String(yield, "ab")
	}); got != nil {
		t.Errorf("String panicked on a multi-run alphabet: %v", got)
	}
	if got := drain(func(yield func(Command) bool) bool {
		return s.PaddedString(yield, "ab", 2, 2)
	}); got != nil {
		t.Errorf("PaddedString panicked with shortest == longest: %v", got)
	}
	// The shared, single-run alphabet keeps the stronger guarantee, which
	// TestConstantWords exercises across the whole BIP-39 list.
	shared := NewConstantStringer(constant.Font, params(), 4*mm)
	if got := drain(func(yield func(Command) bool) bool {
		return shared.PaddedString(yield, "AB", 2, 4)
	}); got != nil {
		t.Errorf("the guard fired on the single-run shared alphabet: %v", got)
	}
}

// TestZeroRunGlyphAssertion proves the construction-time assertion is not
// vacuous. A glyph with knots but no engrave run would silently vanish from
// the plate: paddedString would take the zero-run branch, which draws nothing.
// The only zero-run rune in the real face, 0x20, has no knots at all.
func TestZeroRunGlyphAssertion(t *testing.T) {
	// 'A' has a path but never puts the pen down.
	face := buildTestFace(map[rune][]vector.Knot{
		'A': {
			{Ctrl: bezier.Pt(100, -100)},
			{Ctrl: bezier.Pt(100, -100)},
			{Ctrl: bezier.Pt(100, -100)},
		},
	})
	defer func() {
		if recover() == nil {
			t.Error("a glyph with a non-empty path but no engrave run was accepted")
		}
	}()
	newConstantStringer(face, params(), 4*mm, "A")
}

// buildTestFace assembles a minimal vector face, one advance-600 glyph per
// entry, in the format cmd/vectorfont emits.
func buildTestFace(glyphs map[rune][]vector.Knot) *vector.Face {
	const offIndex = vector.OffSplines - unicode.MaxASCII*vector.IndexElemSize
	bo := binary.LittleEndian
	data := make([]byte, vector.OffSplines)
	bo.PutUint16(data[0:], 900) // ascent
	bo.PutUint16(data[2:], 900) // height
	for r, ks := range glyphs {
		start := len(data)
		for _, k := range ks {
			var b [5]byte
			if k.Line {
				b[0] = 1
			}
			bo.PutUint16(b[1:], uint16(int16(k.Ctrl.X)))
			bo.PutUint16(b[3:], uint16(int16(k.Ctrl.Y)))
			data = append(data, b[:]...)
		}
		g := data[offIndex+int(r)*vector.IndexElemSize:]
		bo.PutUint16(g[0:], 600)
		bo.PutUint16(g[2:], uint16(start))
		bo.PutUint16(g[4:], uint16(len(data)))
	}
	return vector.NewFace(data)
}

// TestPassphraseStringerTiming records the cost of the separate instance
// (spec O5). What proves O5 is that every existing golden still passes; these
// numbers only say what the passphrase instance costs.
func TestPassphraseStringerTiming(t *testing.T) {
	shared := NewConstantStringer(constant.Font, params(), 4*mm)
	pass := NewPassphraseStringer(constant.Font, params(), 4*mm)
	t.Logf("shared runeDuration=%d  passphrase runeDuration=%d", shared.runeDuration, pass.runeDuration)
	t.Logf("shared center=%v  passphrase center=%v", shared.center, pass.center)
	t.Logf("shared startEndDist=%d  passphrase startEndDist=%d", shared.startEndDist, pass.startEndDist)
}
