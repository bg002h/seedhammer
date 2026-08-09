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
		ctx := NewContext(pl)
		a := struct {
			mask *image.Alpha
			idle struct {
				start  time.Time
				active bool
				state  saver.State
			}
		}{}
		a.idle.start = time.Now()

		it := func(yield func(op.Op) bool) {
			ctx.FrameCallback = func(op op.Op) {
				ctx.Done = ctx.Done || !yield(op)
			}
			versionText := "Firmware: " + version + "\nHardware: " + pl.HardwareVersion()
			if !pl.Features().Has(FeatureSecureBoot) {
				versionText += " (UNLOCKED)"
			}
			flow(ctx, versionText)
		}
		startTime := time.Now()
		var evts []Event
		stats := new(runtimeStats)
		d := new(op.Drawer)

		// draw is the content path lifted out of the range body so onDraw has
		// a single place to observe everything Run draws.
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
	}
}
