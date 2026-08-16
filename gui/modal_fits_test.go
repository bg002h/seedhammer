package gui

import (
	"strings"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// ─── F-185: the CLASS check, not another per-screen trim ─────────────────────
//
// A modal's body SCROLLS (Warning.Layout binds Up/Down) and nothing on the frame
// says so. The SH2 has no directional buttons, so a body that runs past the
// viewport is text the operator cannot reach and is not told exists. S4 found
// this on the one screen whose whole purpose is to tell an operator what to do
// about a seed<->key mismatch: the host route -- the single safe action the
// screen offered -- sat below the fold, and the rendered first frame ended
//
//	"...rewrite the payload on the host with `"
//
// WHY NO GO TEST CAUGHT IT, and this is the transferable half. Every content
// assertion in this package checks the string that was SUBMITTED, never the
// pixels that were DRAWN: ExtractText walks the op tree, so a body the panel
// renders as nothing still "appears" to uiContains. This is F-179's seam by a
// different route -- there a missing glyph blanked a line, here the viewport ate
// one -- and a string assertion cannot see either.
//
// So the check compares the DRAWN FRAME against the SOURCE STRING. It is not a
// character budget: capacity depends on how the words WRAP, not on how many
// there are. Measured 2026-08-16 on sh2DisplaySize with 5-to-7-letter filler,
// both modal shapes drew 588 normalized characters in full; F-185's real refusal
// was cut at ~500. A constant calibrated on either number would be wrong for the
// other, which is why this measures each body instead of budgeting all of them.
//
// AND IT CARRIES A MARGIN, which is the half F-185 explicitly says its own fix
// lacked: "the new test pins 'it draws today', not a margin. A +65-character
// mutation still fits on the first frame, so a future edit can re-break it
// without turning the test red." So every screen is asserted to fit with
// modalBodyMargin characters of headroom to spare, and the measured headroom is
// logged so the number is a fact rather than a hope.

// modalBodyMargin is the growth a screen must tolerate before this check has any
// right to call it safe.
//
// 80 CHARACTERS, AND THE NUMBER IS F-185's OWN. That entry measured a +65
// character mutation still fitting on the first frame of the screen it had just
// pinned, and named that as the defect in its fix. A margin at or below 65 would
// reproduce the criticism verbatim; 80 clears it with the smallest round number
// that does.
const modalBodyMargin = 80

// normalizeDrawn strips case and every kind of whitespace.
//
// ExtractText concatenates glyph runs with no separators at all -- a drawn frame
// reads "Thisdevice-authoredmultisigpolicyisNOT..." -- so the source string has
// to be reduced the same way before the two can be compared. uiContains already
// does exactly this to its needle, for exactly this reason; this is that rule
// applied to both sides.
func normalizeDrawn(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '\n', '\t', '\r', '\v', '\f':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bodyDrawnFully reports whether the drawn frame carries the WHOLE body, and
// when it does not, how far in it got.
//
// The cut point is found by prefix search rather than reported as a bare
// boolean, because "the body did not fit" and "the body was cut after the third
// sentence" cost the same to compute and only one of them tells the next author
// what to trim. A body that is cut mid-word is reported at the last whole
// character that drew, which is where the operator's screen actually ends.
func bodyDrawnFully(drawn, body string) (ok bool, drewChars, wantChars int) {
	d, b := normalizeDrawn(drawn), normalizeDrawn(body)
	if len(b) == 0 {
		return true, 0, 0
	}
	if strings.Contains(d, b) {
		return true, len(b), len(b)
	}
	// Longest prefix of the body that IS on the frame. Binary search: prefix
	// containment is monotone -- if a prefix of length n is absent, so is every
	// longer one -- so this is exact rather than a probe.
	lo, hi := 0, len(b)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if strings.Contains(d, b[:mid]) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return false, lo, len(b)
}

// modalRenderer draws a modal's FIRST frame with the given body and returns that
// frame's extracted text. First frame deliberately: pumpUntil would advance past
// it, and a later frame proves only that the text exists somewhere in a screen
// nobody can scroll.
type modalRenderer func(t *testing.T, body string) string

// errorScreenBody renders showError -- the shape behind every refusal, every
// notice and every reminder in this flow.
func errorScreenBody(t *testing.T, body string) string {
	t.Helper()
	return firstModalFrame(t, func(ctx *Context) {
		showError(ctx, &descriptorTheme, "Modal Fit", body)
	})
}

// confirmWarningBody renders ConfirmWarningScreen -- the hold-to-confirm shape,
// which is what the mandatory EXPERIMENTAL warning is.
func confirmWarningBody(t *testing.T, body string) string {
	t.Helper()
	return firstModalFrame(t, func(ctx *Context) {
		w := &ConfirmWarningScreen{Title: "Modal Fit", Body: body, Icon: assets.IconHammer}
		for !ctx.Done {
			dims := ctx.Platform.DisplaySize()
			d, res := w.Layout(ctx, &descriptorTheme, dims)
			if res == ConfirmNo || res == ConfirmYes {
				return
			}
			ctx.Frame(op.Layer(d, op.Color(&ctx.B, descriptorTheme.Background)))
		}
	})
}

// firstModalFrame runs a modal until it draws once and returns that frame's
// text. It also holds the frame to the raster floor, because a comparison
// against a BLANK frame would fail every body for the wrong reason and a
// comparison against an empty one would pass none at all.
func firstModalFrame(t *testing.T, ui func(*Context)) string {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, _, ink, quit := runUITouchRaster(ctx, func() { ui(ctx) })
	defer quit()
	first, ok := frame()
	if !ok {
		t.Fatal("the modal never rendered a frame")
	}
	if ink() < buildWalkRasterFloor {
		t.Fatalf("the modal drew only %d ink pixels (floor %d) -- this frame is near "+
			"blank, so comparing text against it would report a truncation that is "+
			"really a blanking", ink(), buildWalkRasterFloor)
	}
	return first
}

// modalFiller is padding that grows a body the way prose grows it: whole words
// of ordinary length, separated by spaces, so the wrap behaves like real text
// rather than like one unbreakable token.
func modalFiller(n int) string {
	const words = "and the operator must read every word of this before any steel is cut so "
	var b strings.Builder
	for len(normalizeDrawn(b.String())) < n {
		b.WriteString(words)
	}
	s := b.String()
	// Trim to exactly n normalized characters, on a rune boundary.
	for len(normalizeDrawn(s)) > n {
		s = s[:len(s)-1]
	}
	return s
}

// modalHeadroom measures how many more characters of ordinary prose this body
// could carry and still be drawn IN FULL on the first frame.
//
// Binary search over the padding length. Growth is monotone in the same
// direction as fit -- a body that is cut does not un-cut when more is appended
// -- so the search is exact.
func modalHeadroom(t *testing.T, r modalRenderer, body string) int {
	t.Helper()
	const ceiling = 1200
	lo, hi := 0, ceiling
	for lo < hi {
		mid := (lo + hi + 1) / 2
		padded := body + " " + modalFiller(mid)
		ok, _, _ := bodyDrawnFully(r(t, padded), padded)
		if ok {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// assertModalBodyFits is the class check: this body is DRAWN IN FULL on its own
// first frame, and it has modalBodyMargin characters of room left.
func assertModalBodyFits(t *testing.T, what string, r modalRenderer, body string) {
	t.Helper()
	drawn := r(t, body)
	ok, drew, want := bodyDrawnFully(drawn, body)
	if !ok {
		t.Errorf("%s: the screen draws %d of the body's %d characters. The operator is "+
			"told nothing about the missing %d, because this modal's scroller is bound "+
			"to buttons the SH2 does not have.\ncut after: ...%s\nlost:      %s",
			what, drew, want, want-drew,
			tailOf(normalizeDrawn(body)[:drew], 48),
			headOf(normalizeDrawn(body)[drew:], 96))
		return
	}
	head := modalHeadroom(t, r, body)
	t.Logf("%s: %d chars drawn in full, headroom %d chars (margin %d)",
		what, want, head, modalBodyMargin)
	if head < modalBodyMargin {
		t.Errorf("%s fits today with only %d characters to spare, under the %d-character "+
			"margin. F-185's own fix failed exactly here: a +65-character edit still fit, "+
			"so the screen could be re-broken without turning a test red. Shorten this "+
			"body rather than lowering the margin.", what, head, modalBodyMargin)
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func headOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TestModalFitCheckCatchesATruncatedBody is the MUTATION PROOF for the check
// above, and it is the reason this file is a class check rather than a fourth
// per-screen trim.
//
// A guard that has only ever been run against bodies that fit has never been
// shown to be able to fail. So: take a real production body, confirm it passes,
// then grow it past its own measured headroom and confirm the primitive reports
// the truncation AND says where. Both directions are asserted, because a check
// that fails everything is as useless as one that passes everything.
func TestModalFitCheckCatchesATruncatedBody(t *testing.T) {
	body := multisigBuildExperimentalWarningBody()

	// Direction 1: the real body is drawn in full.
	ok, drew, want := bodyDrawnFully(confirmWarningBody(t, body), body)
	if !ok {
		t.Fatalf("INCONCLUSIVE: the unmutated warning is already cut (%d of %d drawn), "+
			"so this test cannot show the check DETECTING a mutation", drew, want)
	}
	head := modalHeadroom(t, confirmWarningBody, body)
	t.Logf("unmutated: %d chars drawn in full, headroom %d chars", want, head)

	// Direction 2: grown past its headroom, the check reports the cut and names
	// the point. The overshoot is deliberate rather than +1 -- the headroom search
	// lands on a wrap boundary, and this test is about the check's ability to see
	// a truncation, not about its resolution at the boundary.
	mutated := body + " " + modalFiller(head+120)
	ok, drew, want = bodyDrawnFully(confirmWarningBody(t, mutated), mutated)
	if ok {
		t.Fatalf("the check did NOT go red on a body %d characters past its measured "+
			"headroom of %d: it is not able to see the defect it exists for",
			head+120, head)
	}
	t.Logf("mutated (+%d chars past headroom): the check went RED, reporting %d of %d "+
		"characters drawn, cut after ...%q",
		120, drew, want, tailOf(normalizeDrawn(mutated)[:drew], 40))

	// And the cut point is INSIDE the body's growth, not at zero: a check that
	// reports "0 of N drawn" for every long body has found the frame, not the fold.
	if drew < len(normalizeDrawn(body)) {
		t.Errorf("the check reported the cut at %d characters, before the end of the "+
			"unmutated body (%d) which is known to fit. That is a blank-frame report "+
			"wearing a truncation's clothes.", drew, len(normalizeDrawn(body)))
	}
}

// TestModalsThisBlockTouchesAreDrawnInFull applies the class check to the
// remaining modals S5's screens-and-prose block edited or introduced.
//
// The per-screen tests next to each string check what it SAYS; this checks that
// the operator can read all of it. Two different questions, and F-185 exists
// because the second had no owner: every content assertion in this package reads
// the string submitted, so a body cut by the viewport passes them all.
//
// SCOPED TO WHAT THIS BLOCK TOUCHED, deliberately. F-185's second open item is
// that "every other long modal in the firmware carries the same unmeasured
// exposure", and that is still true -- the check now exists and is a one-line
// call, but pointing it at every modal in the package is a sweep this block does
// not own and would fold into an unrelated diff.
func TestModalsThisBlockTouchesAreDrawnInFull(t *testing.T) {
	for _, tc := range []struct {
		what string
		body string
	}{
		{
			"the end-of-engrave ms1 reminder (F-182's screen)",
			bundleMs1ReminderText(),
		},
		{
			"the build's unshowable-keys refusal",
			"Couldn't show the keys this policy holds, so it was not engraved. Build " +
				"again; if it happens twice, rewrite the payload on the host with `me sysw pack`.",
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			assertModalBodyFits(t, tc.what, errorScreenBody, tc.body)
		})
	}
}
