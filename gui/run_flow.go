package gui

import (
	"image"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/saver"
)

// runWithFlow is Run's body, with the flow and a draw observer as parameters.
//
// The flow parameter exists because Run had no test at a01b666; onDraw exists
// because Run yields func() bool -- no content reaches the consumer, so
// without it a test can observe nothing Run draws. Both are nil/uiFlow in
// production.
func runWithFlow(pl Platform, version string, flow func(ctx *Context, version string), onDraw func(op.Op)) func(yield func() bool) {
	return func(yield func() bool) {
		a := struct {
			mask *image.Alpha
			idle struct {
				start  time.Time
				active bool
				state  saver.State
			}
			armed bool
		}{}
		versionText := "Firmware: " + version + "\nHardware: " + pl.HardwareVersion()
		if !pl.Features().Has(FeatureSecureBoot) {
			versionText += " (UNLOCKED)"
		}
		stats := new(runtimeStats)
		d := new(op.Drawer)
		// The SESSION loop. A wipe unwinds the flow and re-enters it with a
		// fresh Context; everything above this line survives, because a wipe
		// must not reallocate the mask or restart the frame-time baseline.
		for {
			ctx := NewContext(pl)
			a.idle.start = time.Now()
			// active is reset too. It gates Router.Events, so a session
			// inheriting it eats that first TICK's events -- one tick, not the
			// whole session, since the line below recomputes it immediately.
			a.idle.active = false
			a.armed = false
			wiping := false

			it := func(yield func(op.Op) bool) {
				ctx.FrameCallback = func(o op.Op) {
					// NOT `ctx.Done = ctx.Done || !yield(o)`, which is what
					// gui.go:2949 has today and what Task 1 moved here
					// verbatim. That form reads ctx.Done BEFORE calling
					// yield, so a Done set from INSIDE the call -- exactly
					// what the wipe does -- is discarded when the assignment
					// writes back staleFalse || !true. Measured: the flag is
					// false again by the time Frame returns, so the wipe
					// never persists. Four review rounds read the line as
					// "Done is sticky"; it is sticky against a later false,
					// not against a mutation during the call.
					//
					// The early return preserves what the || was silently
					// also doing: once Done is true, yield is never called
					// again. An operand swap would fix the clobber and lose
					// that.
					if ctx.Done {
						return
					}
					if !yield(o) {
						ctx.Done = true
					}
				}
				flow(ctx, versionText)
			}
			startTime := time.Now()
			var evts []Event

			// draw is the content path lifted out of the range body so onDraw
			// has a single place to observe everything Run draws.
			draw := func(content op.Op) {
				d.Reset()
				dirty := image.Rectangle{Max: pl.DisplaySize()}
				if err := pl.Dirty(dirty); err != nil {
					panic(err)
				}
				for {
					fb, ok := pl.NextChunk()
					if !ok {
						break
					}
					fbdims := fb.Bounds().Size()
					npix := fbdims.X * fbdims.Y
					if a.mask == nil || len(a.mask.Pix) < npix {
						a.mask = image.NewAlpha(image.Rectangle{Max: fbdims})
					}
					a.mask.Rect = image.Rectangle{Max: fbdims}
					d.Draw(fb, a.mask, content)
				}
				if onDraw != nil {
					onDraw(content)
				}
			}

			for content := range it {
				// NO DISCARD GUARD HERE, deliberately. An earlier draft had
				// `if wiping { continue }` to swallow the extra frame the two
				// fall-through screens emit (gui.go:2460, :2758). The
				// FrameCallback fix above makes it UNREACHABLE: wiping implies
				// Done implies no yield implies this body never runs. Measured
				// -- with the fix in place, deleting the guard left the whole
				// ./gui/ suite green, including the test named after it.
				// Dead code in firmware implies a protection that is not
				// operating; the property is pinned by the FrameCallback
				// mutation row instead, which is killable.
				layoutTime := time.Since(startTime)
				draw(content)
				drawTime := time.Since(startTime)
				if debug {
					stats.Dump(drawTime, layoutTime)
				}
				for {
					if ctx.Done || !yield() {
						return
					}
					wakeup := ctx.Wakeup
					evts = pl.AppendEvents(wakeup, evts[:0])
					now := time.Now()
					if len(evts) > 0 {
						a.idle.start = now
					}
					ctx.Reset()
					if !a.idle.active {
						ctx.Router.Events(d, evts...)
					}
					// The test-only wipe trigger: nil in production. It is the
					// ONLY trigger that exists in Task 3's commit, since
					// §10.2.4's timer arrives in Task 4 -- which is what lets
					// the unwind be tested on the commit introducing it.
					// Package-level test hooks are this package's idiom
					// (unlock_session.go:40, unlock_kdf.go:60, and 8 others).
					if wipeNowHook != nil && wipeNowHook() {
						wiping = true
						ctx.Done = true
						break // unwind, never exit
					}
					idleWakeup := a.idle.start.Add(idleTimeout)
					idle := now.Sub(idleWakeup) >= 0
					if a.idle.active != idle {
						a.idle.active = idle
						if idle {
							a.idle.state = saver.State{}
						}
					}
					if a.idle.active {
						a.idle.state.Draw(pl)
						// Throttle screen saver speed.
						const minFrameTime = 40 * time.Millisecond
						ctx.WakeupAt(now.Add(minFrameTime))
						continue
					}
					ctx.WakeupAt(idleWakeup)
					break
				}
				startTime = time.Now()
			}
			if !wiping {
				return
			}
		}
	}
}
