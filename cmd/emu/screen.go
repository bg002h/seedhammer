// This file deliberately carries NO build tag, for the reason toolpath.go and
// plate.go give: the JS glue that hands this to a page is //go:build js and can
// never be tested on this host, while the half that decides WHAT
// window.shScreen reports can be, and is (screen_test.go).
//
// WHAT IT IS FOR. An automated walk of the firmware is a script of taps, and a
// tap is evidence only if the walk can say which screen it landed on. Without
// that, one mis-timed step makes every step after it a guess -- measured the
// hard way on this cycle's first walk attempt, where four consecutive "steps"
// sat on the same screen and nothing noticed until the artifacts came out
// wrong. shTap without shScreen is a hope, not a walk.
//
// The platform is handed PIXELS, and pixels are where the text has already
// gone: recovering "Load Payload" from a 480x320 RGBA buffer is OCR, not a
// read. What this reads instead is the op the drawer was about to rasterise,
// which arrives through gui.FrameAware -- an interface that exists in this
// build and NOT in the firmware's, for the reason gui/frame_hook.go gives.

package main

import (
	"image"
	"sync"

	"seedhammer.com/gui/op"
)

// screenRecorder holds the text of the last frame the GUI drew, and how many
// frames it has seen.
//
// THE COUNT IS A DIAGNOSTIC, NOT A CLOCK, and confusing the two is the mistake
// a walk makes first. It says how many frames have been drawn -- nothing about
// whether any of them changed the screen. Measured in a browser against the
// boot offer: a press redraws twice for the button's own pressed state with the
// text unchanged, and only the release navigates. A walk that waits for this
// number to move therefore reads the screen it was already on.
//
// What it is genuinely for is one question, once a step has already gone wrong:
// WAS ANYTHING DRAWN AT ALL. Zero means the GUI never processed the tap -- it
// is parked or wedged -- and that is a different bug from being on the wrong
// screen. It emphatically does NOT report whether a tap hit a widget: measured,
// a tap at (5,5) on empty screen still drew three frames, because the GUI
// redraws on any pointer event. See cmd/emu/index.html's shStep for the message
// this feeds.
type screenRecorder struct {
	// bounds is the DISPLAY, and clipping to it is what makes this a report of
	// what an operator can SEE rather than of what the flow composed.
	// op.Drawer collects a glyph's rune only after the glyph survives
	// intersection with the destination (gui/op/op.go:416), so a line scrolled
	// off the panel is absent here exactly as it is absent from the panel.
	bounds image.Rectangle

	mu     sync.Mutex
	text   string
	frames int
}

func newScreenRecorder(bounds image.Rectangle) *screenRecorder {
	return &screenRecorder{bounds: bounds}
}

// Frame records the text of one drawn frame.
//
// It extracts SYNCHRONOUSLY and keeps the string, never the op, because the op
// is only valid for the length of this call: Context.Frame resets the buffer
// backing it as soon as the callback returns (gui/gui.go:75), so a retained op
// reads whatever the next frame built over it. gui/frame_hook.go states the
// contract; this is the implementation that has to honour it.
//
// The cost is one extra full pass over the frame -- op.Drawer.ExtractText draws
// it to a scratch buffer to see which glyphs land -- so the emulator draws
// every frame twice. That is a doubling by construction rather than an
// estimate, it buys the only mechanism a walk has for reading the screen, and
// it is confined to a build that already exists to be watched rather than to be
// fast.
//
// A fresh Drawer per frame, as every caller of ExtractText in this tree does:
// a Drawer accumulates input regions across a draw and nothing here resets one.
func (s *screenRecorder) Frame(content op.Op) {
	d := new(op.Drawer)
	txt := d.ExtractText(s.bounds, content)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = txt
	s.frames++
}

// Snapshot returns the last frame's text and the number of frames recorded.
//
// Both come from one lock hold, so a walk that reads them together cannot see
// a count from one frame beside the text of another -- which is precisely the
// pair it uses to decide whether its tap has landed.
func (s *screenRecorder) Snapshot() (text string, frames int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text, s.frames
}
