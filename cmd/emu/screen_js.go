//go:build js

package main

import "syscall/js"

// installScreenAPI exposes the last drawn frame to the page as window.shScreen.
//
//	shScreen()      the text of the screen as it is NOW
//	shScreenSeq()   how many frames have been drawn since boot
//
// TWO CALLS RATHER THAN ONE OBJECT, because the two are read at different
// rates and for different reasons: a walk polls the sequence in a loop and
// reads the text once, after it moves. Returning a JSON object would make the
// common console call -- somebody typing shScreen() to see where they are --
// go through JSON.parse for nothing.
//
// DO NOT WAIT ON THE COUNT TO DECIDE THE SCREEN CHANGED. It answers "has
// anything been drawn", not "am I somewhere new", and the difference is not
// academic -- measured on this build, driving the boot offer:
//
//	seq 2   the boot offer
//	shPress(453,249)     -> seq 4, text UNCHANGED (the button's pressed state)
//	shRelease(453,249)   -> seq 6, text is the next screen
//
// A press alone moves the count by two without moving the screen. So a walk
// that taps and then waits for the count to change reads the frame after the
// PRESS, sees the screen it was already on, asserts against it, and passes
// while standing still. That is the failure this deliverable exists to
// prevent, and the naive use of this API walks straight into it.
//
// Poll the TEXT instead, bounded by a timeout, and keep the count for the
// failure message -- where it distinguishes "no frames at all, so the tap hit
// nothing" from "frames drew, so the flow went somewhere else". index.html
// ships shWaitFor/shStep, which do exactly that; prefer them to hand-rolling
// the loop.
//
// (Reading the text IMMEDIATELY after shTap happens to work here -- the Go
// scheduler runs the GUI goroutine before control returns to JS -- but that is
// an artefact of a flow that redraws instantly, and it is false the moment a
// step lands on a KDF, an engrave, or anything else that parks.)
//
// Text arrives with no spaces. op.Drawer collects the runes of glyphs that
// actually INK, and a space inks nothing, so "Load Payload" reads as
// "LoadPayload". That is the drawer's own model of the screen and not a defect;
// match against it rather than against what the source says.
func installScreenAPI(s *screenRecorder) {
	js.Global().Set("shScreen", js.FuncOf(func(js.Value, []js.Value) any {
		txt, _ := s.Snapshot()
		return txt
	}))
	js.Global().Set("shScreenSeq", js.FuncOf(func(js.Value, []js.Value) any {
		_, n := s.Snapshot()
		return n
	}))
}
