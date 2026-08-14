//go:build js

package main

import (
	"image"
	"syscall/js"

	"seedhammer.com/gui"
)

// The walk API: what an automated §4.5 emulator walk drives the machine with.
//
// WHY IT EXISTS. The plan makes an emulator walk the closing gate of every
// stage, and until this file there was no way to write one: input was raw
// canvas pointer events synthesised by whoever was driving the browser, so a
// "walk" was a remembered click sequence, not a script. Five review rounds
// accepted "emulator walk" as a gate before anyone asked what would perform it.
//
// These are DRIVING primitives only. They inject the same events the canvas
// would and choose which payload the platform serves; they add no screens,
// bypass no flow, and cannot make the GUI do anything a finger could not.
// Anything that let a walk skip a step would make the walk prove less than the
// operator's own hands do, which is the opposite of the point.
//
// The READING half is window.shScreen, next door in screen_js.go. It is a
// separate file because it is a separate kind of thing, and because the
// paragraph above has to stay literally true of this one: driving and reading
// are what a walk alternates between, and only one of them touches the
// machine's state. Driving without reading is not a walk -- see screen.go.

// installWalkAPI exposes the driving primitives as window.shTap / window.shSysw.
//
//	shTap(x, y)     tap at DEVICE coordinates (0..479, 0..319)
//	shTapHold(x,y,ms) press, hold, release -- the hold-to-confirm idiom
//	shSysw(which)   choose the systemwide payload: "records" | "cards" | "none"
func installWalkAPI(p *platform) {
	js.Global().Set("shTap", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return "shTap(x, y): device coordinates required"
		}
		x, y := args[0].Int(), args[1].Int()
		p.tap(x, y, true)
		p.tap(x, y, false)
		return nil
	}))

	// A press and a release as separate calls, so a walk can express the
	// hold-to-confirm gesture the engrave flow requires. Splitting it is
	// deliberate: a single "hold(ms)" would make the walk's timing the
	// emulator's business, and the GUI's own hold threshold is what should
	// decide whether a hold counted.
	js.Global().Set("shPress", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return "shPress(x, y): device coordinates required"
		}
		p.tap(args[0].Int(), args[1].Int(), true)
		return nil
	}))
	js.Global().Set("shRelease", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return "shRelease(x, y): device coordinates required"
		}
		p.tap(args[0].Int(), args[1].Int(), false)
		return nil
	}))

	// Which payload the platform serves. A walk must be able to CHOOSE, because
	// the two blobs exist for different stages: the records payload has the
	// classes Load Payload's journey needs, the cards payload has the cosigner
	// mk1s Build policy's gather needs, and no single blob is both without
	// invalidating a published document. "none" emulates a machine with an
	// empty region, which is the case §10.1's detection is written for.
	//
	// Takes effect on the NEXT read. The session caches what it loaded, so a
	// walk switching mid-session sees the payload it already loaded — which is
	// the machine's behaviour too, and not a limitation worth papering over.
	js.Global().Set("shSysw", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return "shSysw(\"records\"|\"cards\"|\"none\")"
		}
		switch which := args[0].String(); which {
		case "records", "cards", "none":
			p.syswChoice = which
			return which
		default:
			return "unknown payload: " + which + " (want records, cards or none)"
		}
	}))
}

// tap injects one pointer event at device coordinates, through the SAME queue
// the canvas listener uses. Not a shortcut into the GUI: an event posted here is
// indistinguishable from a finger, which is what makes a walk evidence.
func (p *platform) tap(x, y int, pressed bool) {
	p.post(gui.PointerEvent{
		Pressed: pressed,
		Pos:     image.Pt(x, y),
	}.Event())
	p.Wakeup()
}
