package main

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"seedhammer.com/font/poppins"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/text"
	"seedhammer.com/gui/widget"
	"seedhammer.com/internal/sh2"
)

// The display, as the emulator's platform reports it. Hard-coding 480x320 here
// instead would let this suite keep passing against a panel size the emulator
// no longer has.
var emuDisplay = image.Rectangle{Max: image.Pt(sh2.DisplayWidth, sh2.DisplayHeight)}

// labelOp builds a real text op the way the GUI does -- through widget.Label
// over a poppins face -- so what this suite feeds the recorder is the same kind
// of value gui.FrameAware delivers, not a stand-in.
//
// buf is a parameter rather than an internal detail because op.Layer refuses to
// combine ops from different buffers (gui/op/op.go:539 panics "TODO"), and a
// frame is one buffer's worth of work: gui.Context builds every op of a frame
// into ctx.B. Taking the buffer here makes this suite compose frames the way
// the GUI composes them instead of the way that happens to compile.
func labelOp(t *testing.T, buf *op.Buffer, s string) op.Op {
	t.Helper()
	st := text.Style{Face: poppins.Regular16, LineHeightScale: 0.75}
	o, _ := widget.Label(buf, st, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, s)
	return o
}

// TestScreenRecorderReportsTheLastFrame is the property window.shScreen exists
// for: the CURRENT screen, not the first one and not an accumulation.
func TestScreenRecorderReportsTheLastFrame(t *testing.T) {
	r := newScreenRecorder(emuDisplay)

	if txt, n := r.Snapshot(); txt != "" || n != 0 {
		t.Fatalf("a fresh recorder reported (%q, %d), want (\"\", 0) -- a walk must be able "+
			"to tell 'nothing drawn yet' from 'drew an empty screen'", txt, n)
	}

	// A fresh buffer per frame, as gui.Context.Frame gives the GUI: it resets
	// ctx.B after every frame, so no two frames ever share live buffer content.
	r.Frame(labelOp(t, new(op.Buffer), "FIRSTSCREEN"))
	r.Frame(labelOp(t, new(op.Buffer), "SECONDSCREEN"))

	txt, n := r.Snapshot()
	if txt != "SECONDSCREEN" {
		t.Errorf("shScreen would report %q, want %q -- reporting a stale frame is exactly the "+
			"failure that let four walk steps sit on one screen unnoticed", txt, "SECONDSCREEN")
	}
	if n != 2 {
		t.Errorf("recorded %d frames, want 2 -- the count is what a walk waits on before it "+
			"trusts the text", n)
	}
}

// TestScreenRecorderIgnoresTextOffTheDisplay pins the difference between what
// the flow COMPOSED and what the operator can SEE.
//
// A scrolled list composes every row and draws the ones that fit. A recorder
// that reported all of them would let a walk assert it had reached a row that
// is off the panel -- and the operator's finger cannot reach an off-panel row,
// so the walk would be proving something the machine does not offer.
//
// It works because op.Drawer collects a glyph's rune only after the glyph
// survives intersection with the destination rect (gui/op/op.go:416). That is
// an implementation detail of the drawer, which is why it is pinned HERE: if it
// ever changes, this fails rather than shScreen quietly starting to report
// invisible text.
func TestScreenRecorderIgnoresTextOffTheDisplay(t *testing.T) {
	buf := new(op.Buffer)
	onScreen := labelOp(t, buf, "VISIBLE")
	offScreen := labelOp(t, buf, "SCROLLEDAWAY").Offset(image.Pt(0, sh2.DisplayHeight*4))
	frame := op.Layer(onScreen, offScreen)

	display := newScreenRecorder(emuDisplay)
	display.Frame(frame)
	got, _ := display.Snapshot()
	if got != "VISIBLE" {
		t.Errorf("shScreen would report %q, want %q -- text drawn past the bottom of the panel "+
			"is being reported as though an operator could see it", got, "VISIBLE")
	}

	// THE POSITIVE CONTROL, and the test is worth little without it. An offset
	// that silently produced no drawing at all -- a wrong sign, a dropped op,
	// widget.Label returning nothing for the second call -- would satisfy the
	// assertion above for a reason that has nothing to do with clipping, and
	// the test would read as proof of a property it never exercised. So run the
	// SAME frame past a recorder whose bounds reach further: if the rune shows
	// up there, the off-screen text was genuinely drawn and genuinely excluded,
	// and the only difference between the two results is the bounds.
	wide := newScreenRecorder(image.Rect(-2000, -2000, 2000, 2000))
	wide.Frame(frame)
	if got, _ := wide.Snapshot(); !strings.Contains(got, "SCROLLEDAWAY") {
		t.Fatalf("the off-display label is absent even from a recorder bounded to %v (got %q) -- "+
			"it was never drawn, so the assertion above proves nothing about clipping",
			image.Rect(-2000, -2000, 2000, 2000), got)
	}
}

// TestScreenRecorderReportsAnEmptyScreenAsEmpty is the negative control for the
// two tests above.
//
// Without it, a recorder that returned "" unconditionally -- the shape of every
// wiring mistake between gui.FrameAware and this type -- would pass the
// off-display test and fail only the other one, and the off-display test would
// look like it was proving something it was not. See the counted-ink lesson in
// gui's own suite (c4f50fe): ExtractText cannot distinguish a blank screen from
// a screen it never saw, so a test that asserts absence needs a sibling that
// asserts presence through the same path.
func TestScreenRecorderReportsAnEmptyScreenAsEmpty(t *testing.T) {
	r := newScreenRecorder(emuDisplay)
	r.Frame(labelOp(t, new(op.Buffer), ""))

	txt, n := r.Snapshot()
	if txt != "" {
		t.Errorf("an empty frame reported %q, want \"\"", txt)
	}
	if n != 1 {
		t.Errorf("an empty frame recorded %d frames, want 1 -- a frame with no text is still a "+
			"frame, and a walk waiting on the count would hang without it", n)
	}
}
