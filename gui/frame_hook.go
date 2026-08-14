//go:build !tinygo

// The drawn-frame seam, and the second reason this package carries a
// build-tagged pair. gui/plate_hook.go is the first; read it too, because the
// argument for the split is the same one and is written out there in full.
//
// cmd/emu wants to READ THE SCREEN. An automated walk of the firmware -- the
// thing §4.5 makes the closing gate of every stage -- is a script of taps, and
// a tap is only evidence if the walk can say which screen it landed on. Without
// that, a mis-timed step makes every step after it a guess: measured the hard
// way on this cycle's first walk attempt, where four consecutive "steps" sat on
// the same screen and nothing noticed until the artifacts came out wrong.
//
// The frame content exists inside Run and reaches nothing outside it. The
// platform is handed PIXELS -- Dirty and NextChunk, a framebuffer -- and pixels
// are where the text has already gone; recovering "Load Payload" from a
// 480x320 RGBA buffer is OCR, not a read. What a walk needs is the op the
// drawer was about to rasterise, which run_flow.go's draw closure holds for the
// length of one call and no longer.
//
// WHY NOT ctx.FrameCallback. That field is not a spare hook: run_flow.go's run
// loop OWNS it (gui/run_flow.go:208), installing the closure that turns
// Context.Frame into the loop's iterator step and that carries §10.2.4's
// early-return-once-Done fix. A consumer that assigned it would silently
// unhook the run loop from the flow -- the frames would stop arriving at the
// screen, not merely arrive twice -- and nothing about it would fail to
// compile.
//
// WHY `!tinygo` AND NOT A PLAIN TYPE ASSERTION. Everything plate_hook.go says
// about a spline holds here with less indirection, not more: a spline is the
// operator's words drawn as geometry, and a frame's content op is the
// operator's words drawn as WORDS -- the seed screen's twelve of them, the
// passphrase, the ms1. The emulator's consumer is JavaScript on a page, outside
// anything Go can wipe. So the firmware must not merely decline to use this
// hook, it must not carry it, and the copy inventory of seed-derived material
// stays true for the image by CONSTRUCTION rather than by argument.
//
// The cost of the split, measured: ZERO image bytes. The firmware build of this
// call site compiles byte-identically with and without it. That is not what
// plate_hook's equivalent costs, and the numbers, the positive control that
// makes the zero believable, and the deliberate refusal to explain the
// difference are all in frame_hook_tinygo.go. Measured because
// plate_hook_tinygo.go's first version of this sentence was written from a
// guess about the compiler and was false.
package gui

import "seedhammer.com/gui/op"

// FrameAware is an OPTIONAL interface a [Platform] may implement to be handed
// each frame's content, immediately after that frame has been drawn.
//
// "Frame" here is a DISPLAY frame -- one composed screen, the thing
// [Context.Frame] emits. It is not the camera frame [Event.AsFrame] reports;
// the two words collide in this package and only one of them reaches this
// interface.
//
// It is on the Platform rather than on the Engraver -- where [PlateAware]
// lives -- because the lifetimes differ and each hook is hung on the thing
// whose lifetime matches it. A plate belongs to a job, and Platform.Engraver is
// called once per job, so PlateAware cannot drift from the plate it describes.
// A frame belongs to no job at all: frames are drawn on the start screen, in
// the middle of a gather, under §10.2.4's warning. The Platform is what is alive
// for all of them.
//
// content is valid ONLY FOR THE DURATION OF THE CALL, and this is stricter than
// PlateAware's "must not retain it past the job". [Context.Frame] resets ctx.B
// as soon as the callback returns (gui/gui.go:75), and run_flow.go's draw runs
// INSIDE that callback, so the buffer backing content is reused by the very
// next frame. An implementation that stores the op and reads it later is
// reading whatever has since been built over it. Extract what is wanted
// synchronously -- [op.Drawer.ExtractText] is the emulator's answer -- and keep
// the result, not the op.
//
// It does NOT see the screensaver. saver.State.Draw writes straight to the
// platform (gui/saver/saver.go:311 calls screen.Dirty) and never composes an
// op, so while the saver is up a consumer still holds the last real frame. That
// is the same blind spot gui/run_harness_test.go's onDraw has, and it is
// harmless for the reason it is harmless there: the saver draws no text.
type FrameAware interface {
	Frame(content op.Op)
}

// notifyFrame offers the drawn frame to pl, if pl wants it.
func notifyFrame(pl Platform, content op.Op) {
	if fa, ok := pl.(FrameAware); ok {
		fa.Frame(content)
	}
}
