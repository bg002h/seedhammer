package gui

import (
	"image"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/seal"
)

// hold presses a nav button and holds it past confirmDelay, which is how the
// engrave screen is armed to start a job. Requires synctest for the sleep.
// Mirrors ppFlowRun.hold (gui/passphrase_flow_test.go) -- duplicated rather
// than shared, matching this file's own established convention of each flow
// test owning its harness.
func (h *sessionHarness) hold(b Button) {
	h.t.Helper()
	dims := h.ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	pos := image.Pt(dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
	d := h.drawer()
	tag, _, hit := d.Hit(pos)
	if !hit {
		h.t.Fatalf("hold %v: no touch target at %v", b, pos)
	}
	if c, ok := tag.(*Clickable); !ok || (c.Button != b && c.AltButton != b) {
		h.t.Fatalf("hold %v: the target at %v is %v", b, pos, tag)
	}
	h.ctx.Router.Events(d, PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event())
	h.next("hold press")
	time.Sleep(confirmDelay)
	h.next("hold confirm")
}

// TestWipeGuardLifecycleAndArmed -- §10.2.4's residency seam (Task 2). Nothing
// reads armed() in production yet; this pins the seam itself, at flow level,
// per the plan's own instruction to assert on ctx.wipe rather than on a
// return value.
//
// Two properties:
//
//   - The guard brackets unlockSecretSession's OWN lifetime: nil before the
//     session ever runs, installed for its duration, nil again once it
//     returns.
//   - armed() reads false while a job is actually cutting a secret plate --
//     §10.2.4 row 2, never wipe with the needle down -- and true both before
//     the cut starts and once it is done, which is the walk-away window the
//     timer exists for.
func TestWipeGuardLifecycleAndArmed(t *testing.T) {
	p := unlockedPayload(t, "A")
	if len(p.Secret) != 1 || p.Secret[0].Class != seal.ClassMnemonic {
		t.Fatalf("premise broken: vector A must carry one BIP-39 mnemonic, got %d records of class %v",
			len(p.Secret), p.Secret[0].Class)
	}

	synctest.Test(t, func(t *testing.T) {
		pf := newPlatform()
		pf.display = sh2DisplaySize
		pf.engraver = newEngraver()
		ctx := NewContext(pf)
		if ctx.wipe != nil {
			t.Fatal("ctx.wipe is non-nil before unlockSecretSession ever ran")
		}

		returned := false
		frame, drawer, quit := runUITouch(ctx, func() {
			unlockSecretSession(ctx, &descriptorTheme, p)
			returned = true
		})
		defer quit()
		h := &sessionHarness{t: t, ctx: ctx, done: &returned}
		h.frame, h.drawer = frame, drawer

		h.mustReach("SECRET seed material")
		if ctx.wipe == nil {
			t.Fatal("ctx.wipe is nil while the secret session is offering a plate -- the bracket did not install")
		}

		h.choose(0) // Cut this plate
		h.mustReach("Engrave Seed")
		h.tapNav(Button3) // confirm the seed
		h.mustReach("Insert a blank plate")
		if ctx.wipe == nil || ctx.wipe.job == nil {
			t.Fatal("ctx.wipe.job is nil on the engrave screen; the job bracket did not register")
		}
		if !ctx.wipe.armed() {
			t.Error("armed() is false on the engrave-idle screen; nothing is cutting yet, so the " +
				"walk-away timer should be live")
		}

		h.hold(Button3) // hold-to-start: s.job.Start() has now run
		// job.Start() ran synchronously in the flow goroutine, immediately
		// before the frame h.hold just captured -- so this read is not racing
		// the background engraving goroutine for the FIRST observation.
		if st := ctx.wipe.job.Status().State; st != engraveRunning {
			t.Fatalf("premise broken: job status is %v right after Start(), want engraveRunning", st)
		}
		if ctx.wipe.armed() {
			t.Fatal("armed() is true immediately after the job started -- a wipe could fire with the needle down")
		}

	loop:
		for i := 0; i < 4096; i++ {
			select {
			case <-pf.engraver.closes:
				break loop
			case <-pf.wakeups:
			}
			h.next("engraving")
		}
		h.mustReach("completed successfully")
		if !ctx.wipe.armed() {
			t.Error("armed() is false on the plate-done screen; the walk-away window never opens")
		}

		h.tapNav(Button3) // dismiss the success screen
		synctest.Wait()
		for i := 0; i < 64 && !returned; i++ {
			if _, ok := h.frame(); !ok {
				break
			}
		}
		if !returned {
			t.Fatal("the secret session did not end")
		}
		if ctx.wipe != nil {
			t.Error("ctx.wipe is non-nil after unlockSecretSession returned -- the bracket did not uninstall")
		}
	})
}
