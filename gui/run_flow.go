package gui

import (
	"image"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/saver"
)

// effectiveInput reports whether evts holds operator input that RESOLVES TO A
// STATE CHANGE, and advances *pressed to the panel's new contact state.
//
// This is F-103. §10.2.4's clock used to be refreshed on `len(evts) > 0`, and
// arrival is not presence: the factory protective film resting on the panel
// makes ft6x36 assert contact continuously, and processTouch's dedupe is exact
// equality on (touching, pos) (cmd/controller/platform_sh2.go:401), so a
// reading that drifts by ONE pixel is delivered as a fresh event. The machine
// then never goes idle -- and because the warning is nested inside
// `if a.idle.active`, it never warns either. No countdown, no wipe, nothing on
// screen, indefinitely, on a machine holding a decrypted seed. Observed on real
// hardware 2026-08-09; reproduced on a host in
// gui/idle_effective_input_test.go.
//
// The classification, per event kind the router handles:
//
//   - POINTER: effective only when the CONTACT state changes -- released ->
//     pressed, or pressed -> released. A reading that only moves the contact
//     point is NOT effective. There is no cursor and no hover on this
//     hardware: processTouch emits a position only while contact is asserted
//     (it zeroes Pos on release), so a position-only event means "the contact
//     point moved while still held". That is exactly and only what an object
//     resting on the panel produces, and what a human produces only in the
//     middle of a drag -- which is always bracketed by the down and up edges
//     that DO count. The cost to a genuine operator is therefore one
//     uninterrupted drag longer than 3:30 with no press or release in it; the
//     longest hold this UI has is confirmDelay, one second (gui/gui.go:323).
//   - RUNE and BUTTON: always effective. Each is a discrete, self-terminating
//     operator action, and the SH2 has neither a keypad nor a keyboard -- the
//     only producer in the tree is cmd/controller/debug_sh2.go, which
//     synthesises them from the debug serial line in press/release pairs.
//     Nothing can emit them as a continuous stream.
//   - FRAME: never effective. A frame event carries a camera image; it is
//     machine output, not operator input, and a source delivering frames at
//     30 Hz is precisely the shape of the defect above. No platform in this
//     tree produces one today, which is why this line costs nothing and is
//     written down anyway -- a future scan path must not silently re-open
//     F-103.
//
// What this does NOT cover, said plainly rather than left for a reviewer: a
// panel whose contact FLICKERS -- repeatedly crossing the detection threshold
// -- produces genuine press and release edges and would still hold the clock.
// That is strictly narrower than the behaviour it replaces, which was tripped
// by any non-identical reading at all including pure position jitter, but it
// is not closed. Closing it needs a plausibility bound on how fast a human can
// tap, which is a tunable in the middle of a funds-safety control and is not
// worth adding blind.
//
// The whole batch is scanned; there is no early return. Returning at the first
// effective event would leave *pressed stale, and press/release arrive in one
// batch on more than one platform (gui/run_harness_test.go's tap does exactly
// that) -- so the next genuine press edge would be missed.
func effectiveInput(evts []Event, pressed *bool) bool {
	effective := false
	for _, e := range evts {
		pe, ok := e.AsPointer()
		if !ok {
			if _, isFrame := e.AsFrame(); isFrame {
				continue
			}
			effective = true
			continue
		}
		if pe.Pressed != *pressed {
			*pressed = pe.Pressed
			effective = true
		}
	}
	return effective
}

// runWithFlow is Run's body, with the flow and a draw observer as parameters.
//
// The flow parameter exists because Run had no test at a01b666; onDraw exists
// because Run yields func() bool -- no content reaches the consumer, so without
// it a test can observe nothing Run draws, including §10.2.4's warning. Both
// are nil/uiFlow in production.
func runWithFlow(pl Platform, version string, flow func(ctx *Context, version string), onDraw func(op.Op)) func(yield func() bool) {
	return func(yield func() bool) {
		a := struct {
			mask *image.Alpha
			// warnBuf is the warning's OWN buffer. It must not build into
			// ctx.B: Context.Frame resets that buffer AFTER the callback
			// (gui/gui.go:75) and Run's event loop runs INSIDE the callback, so
			// while the flow is parked ctx.B is never reset. Appending a
			// warning per second for 30 s grew it to 228 KB live / 245 KB
			// reserved on the 32-bit target -- measured -- and each of the ~7
			// doublings memcpy'd the PARKED frame, which on SeedScreen.Confirm
			// is the twelve words, into an array nothing ever zeroes.
			warnBuf op.Buffer
			idle    struct {
				start  time.Time
				active bool
				state  saver.State
			}
			armed bool
			// pressed is the panel's CONTACT state, tracked across ticks so a
			// pointer reading can be classified as a state CHANGE rather than
			// as an arrival. See effectiveInput.
			//
			// It lives here, above the session loop, deliberately: contact is
			// physical and a wipe does not lift a finger. Resetting it per
			// session would manufacture a spurious press edge on the first
			// reading after every wipe -- which is the one moment the clock
			// most needs to be honest.
			pressed bool
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
			// syncArmed processes a change in ctx.wipe.armed(), and is called
			// TWICE per iteration -- once before the loop blocks and once
			// after. What each call actually buys, measured (R0 round 0):
			//
			//   before  catches an edge created by the FLOW -- guard
			//           installation, and any bracket handover -- on the frame
			//           it happens, while ctx.Wakeup still holds a deadline
			//           computed from the PRE-edge clock. Worth a WAKEUP: the
			//           edge would otherwise wait for the next one, and when
			//           that wakeup IS the idle deadline the window doubles.
			//           That is F-106, and this call is the fix.
			//   after   catches an edge created by the ENGRAVE GOROUTINE at
			//           the end of the block instead of at the start of the
			//           next iteration. Worth one TURNAROUND, not one wakeup:
			//           there is no blocking call between this sample point
			//           and the next iteration's pre-block one, so the
			//           pre-block call subsumes it. Deleting it leaves the
			//           whole ./gui/ suite green -- measured -- and costs at
			//           most one screensaver frame drawn where a warning frame
			//           belonged, on the single tick an async edge lands. It
			//           is kept for that tick's branch consistency, and
			//           carries no §10.2.4 guarantee of its own.
			//
			// Engrave-side edges are covered by NEITHER call -- the pre-block
			// call runs before the sleep, the post-block call after it
			// returns. What un-parks the loop for those is pl.Wakeup(), which
			// engraveJob.Start's goroutine calls on the way out
			// (gui/engraver.go:110) and which platform_sh2.go:384 returns
			// early on. TestCutEndingDuringTheParkStartsAFreshWindow pins it.
			//
			// Idempotent by the `armed != a.armed` guard, so calling it twice
			// with no change between cannot move the clock. That guard is
			// load-bearing: without it the two-call structure would reset
			// a.idle.start every iteration and the window would NEVER fire,
			// which is strictly worse than the 2x it fixes. Measured: dropping
			// it fails 12 tests across ./gui/.
			//
			// The structure cannot EXTEND a window. It does not change the set
			// of clock stamps, only their timing, and only ever earlier -- so
			// it can make the wipe fire sooner, never later.
			syncArmed := func(now time.Time) bool {
				armed := ctx.wipe.armed()
				if armed != a.armed {
					a.armed = armed
					if armed {
						// Both §10.2.4 rows land here, and which one it is
						// depends on WHY arming changed:
						//   row 1  the guard is INSTALLED -- the residency
						//          begins, and its window opens with it.
						//          "Resident" is a lifetime and wipe_guard.go
						//          is that seam, so this is row 1 working, not
						//          a spurious edge that happens to be harmless.
						//   row 2  a job finished -- a finished cut starts a
						//          FRESH window.
						// With the clock reset, `idle` recomputes false on this
						// very tick and the block below clears a.idle.active by
						// itself.
						//
						// Deliberately NOT also clearing a.idle.active here. It
						// would only change the edge TICK, and changing it is
						// worse: `d` still holds the frame drawn before the
						// saver activated, in a different EngraveScreen state,
						// so routing a touch against it could hit the wrong
						// widget. Swallowing the edge-tick touch is exactly
						// today's screensaver-dismissal behaviour.
						a.idle.start = now
					}
				}
				return armed
			}

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

			// draw is the content path lifted out of the range body so the
			// warning can use it too: the screensaver writes straight to the
			// platform (saver.State.Draw) and cannot carry an op.
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
				// The frame is on the screen; offer it to a platform that wants
				// to read it. AFTER the chunk loop, so a consumer can never see
				// a frame the display did not get -- and inside draw rather than
				// in the range body below, so §10.2.4's warning reaches it too.
				// gui/frame_hook.go; a no-op in the firmware, which does not
				// contain the interface.
				notifyFrame(pl, content)
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
					// BEFORE the block. The wipe guard is installed INSIDE yield()
					// above -- ctx.wipe = g in the unlock arms -- so an arm edge is
					// already pending here, while ctx.Wakeup below still holds the
					// deadline computed from the PRE-edge clock.
					//
					// Leaving it until after AppendEvents means the edge is processed
					// at whatever the next wakeup happens to be -- and when that
					// wakeup IS the idle deadline, row 2 restarts the window at the
					// exact instant the wipe should have fired. Measured on hardware
					// before this call existed: warning at 6:00 and wipe at 6:30
					// against a 3:00 spec, deterministically (F-106).
					//
					// This can reset a.idle.start after ctx.Wakeup was computed, so
					// the loop may sleep to a now-stale EARLIER deadline, wake, find
					// itself not idle and reschedule. That is one extra wakeup on the
					// frame that installs a guard, never a missed window.
					syncArmed(time.Now())
					wakeup := ctx.Wakeup
					evts = pl.AppendEvents(wakeup, evts[:0])
					now := time.Now()
					// §10.2.4: the timer keys on the SESSION BRACKET's
					// lifetime, never on seal.RecordsResident -- which reads
					// false while the flow still holds the words and the
					// plate's spline closure.
					// AFTER the block. This is the SECONDARY of the two calls:
					// it advances an engrave-goroutine edge by one loop
					// turnaround, not by a wakeup, and nothing in the suite
					// pins it -- see the declaration comment above for what it
					// does and does not buy.
					//
					// It must stay `syncArmed(now)`. Substituting the bare
					// `ctx.wipe.armed()` this replaced is one character away
					// and NOT equivalent: a fresh sample with no stamp enters
					// the warning branch below on a tick whose a.idle.start is
					// still the pre-edge clock, so the warning draws at the
					// edge and the wipe follows wipeWarningDelay later instead
					// of a fresh idleTimeout.
					// TestCutEndingAfterTheDeadlineStartsAFreshWindow pins that.
					armed := syncArmed(now)
					// ONE clock, not two. An earlier draft tracked a separate
					// wipe origin; every one of its refresh points was also an
					// idle.start refresh point, so the two were provably equal
					// -- and the single place they were allowed to diverge is
					// exactly where the latch bug below came from.
					//
					// keepAwake holds off the SCREENSAVER but is ignored while
					// armed: a screen must never be able to postpone a §10.2.4
					// wipe. Read before ctx.Reset(), which clears it.
					//
					// effectiveInput, NOT len(evts) > 0. That is F-103: event
					// ARRIVAL is not evidence an operator is present, and a
					// panel with the factory film on it arrives forever.
					//
					// Bound to its own variable rather than inlined as the
					// left operand of ||, because effectiveInput also ADVANCES
					// a.pressed: written inline, a later reordering of the two
					// terms would short-circuit past the tracking and lose the
					// contact state silently.
					effective := effectiveInput(evts, &a.pressed)
					if effective || (ctx.keepAwake && !armed) {
						a.idle.start = now
					}
					ctx.Reset()
					if !a.idle.active {
						ctx.Router.Events(d, evts...)
					}
					// The test-only wipe trigger: nil in production. It is the
					// ONLY trigger that exists in Task 3's commit, since
					// §10.2.4's timer below arrives in Task 4 -- which is what
					// lets the unwind be tested on the commit introducing it.
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
						// Armed and idle IS §10.2.4's window. The warning takes
						// the screen the saver would otherwise have had, which
						// is why this is one branch and not a gate on the
						// saver: they can never both run.
						if armed { // §10.2.4's window: warn, then wipe
							wipeAt := idleWakeup.Add(wipeWarningDelay)
							if now.Sub(wipeAt) >= 0 {
								wiping = true
								ctx.Done = true
								break
							}
							a.warnBuf.Reset()
							draw(wipeWarningOp(&a.warnBuf, ctx.Styles, &descriptorTheme,
								pl.DisplaySize(), wipeAt.Sub(now), ctx.wipe.warningSubject()))
							// The only way a test can see WHICH buffer the
							// warning went into, or that it is not growing:
							// op.Buffer's fields are unexported and `a` is a
							// closure local. Nil in production.
							if warnBufHook != nil {
								args, refs := a.warnBuf.Len()
								warnBufHook(args, refs)
							}
							ctx.WakeupAt(now.Add(time.Second))
							continue
						}
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
			// SCRUB the abandoned buffer. Context.Frame ran c.B.Reset() on
			// the last frame drawn (gui/gui.go:75) -- but Reset TRUNCATES
			// args and clears only refs (gui/op/op.go:374), and op.Glyph
			// encodes every rendered rune into args. So on the SeedScreen
			// path the twelve words come back VERBATIM AND IN ORDER from the
			// backing array. An earlier comment here claimed the buffer was
			// "already zeroed"; it was half right, and the half it got wrong
			// is the seed. This session's Context is abandoned below, so
			// nothing will overwrite that array.
			ctx.B.Scrub()
			// The Drawer outlives this Context by design -- allocated above the
			// session loop so a wipe does not reallocate the mask -- so it is
			// the one thing that can still reach the Buffer abandoned below.
			// Its stale frameOps hold each drawn frame's mask SOURCE as an
			// interface-value copy living in the Drawer's own array, which
			// Scrub cannot reach: Scrub zeroes ctx.B's arrays, not this one.
			//
			// Draw clears as it goes, so this is defence in depth rather than
			// the only barrier -- it releases at the moment of abandonment
			// instead of on session 2's first frame. Said plainly because it
			// has a testing consequence: NO host test can distinguish deleting
			// this line from keeping it, since Draw closes the same path one
			// frame later. See gui/op/release_test.go for the tests that do
			// bite.
			//
			// Order against Scrub does not matter -- Release frees nothing,
			// ctx.B holds its own headers to the same arrays, and ctx is live
			// across both. What matters is that BOTH follow the last draw().
			d.Release()
		}
	}
}
