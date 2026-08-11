package gui

import (
	"image"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// §10.2.4's 30-second warning, drawn by Run.
//
// wipeWarningDelay is the gap between the warning and the wipe. §10.2.4 row 1
// supplies the 3:00 as a VALUE via idleTimeout; this is the separate 30 s the
// same row requires and which no existing constant carries.
const wipeWarningDelay = 30 * time.Second

// The two lead sentences §10.2.4 rows 1 and 4 draw -- what is actually at
// risk differs, but the countdown, the touch-to-keep affordance and the
// 3:00/3:30 schedule do not (R0 report, task9-r0.md I1).
//
// Row 1's text is false at passphrase entry: nothing has been decrypted yet,
// and telling an operator the machine holds "decrypted seed material" on a
// screen they know they have not unlocked teaches them the warning is
// furniture -- the same reasoning §10.2 step 3 uses to refuse a constant
// hash.
const (
	wipeWarningSubjectSecret     = "This machine still holds decrypted seed material and has been idle."
	wipeWarningSubjectPassphrase = "This machine holds a partly typed passphrase and has been idle."
)

// warnBufHook reports the warning buffer's size after each warning frame. Nil
// in production.
//
// It exists because A-C1's three mutation rows are otherwise UNWRITABLE:
// op.Buffer's args/refs are unexported with no accessor (gui/op/op.go:28), and
// the buffer itself is a field of a closure-local struct in runWithFlow. Without
// this, "the warning grew ctx.B unboundedly" -- the Critical this phase's most
// expensive finding was about -- has no test that can fail.
var warnBufHook func(args, refs int)

// wipeWarningOp draws the warning: one op out, no state touched, so a test can
// assert on its extracted text without driving Run's clock.
//
// Run draws this rather than the flow, and that is FORCED rather than
// preferred: at 3:00 of no input the flow is parked inside ctx.Frame and only
// Run has control. A flow-drawn warning would require every shared screen to
// learn a new signal, which the design's central constraint forbids, and the
// screensaver cannot carry it (saver draws no text). Replacing the screen
// entirely doubles as the privacy blanking a walked-away machine wants.
//
// It takes the buffer and Styles EXPLICITLY rather than a *Context, because the
// one buffer it must never use is ctx.B -- see a.warnBuf in run_flow.go. Styles
// is passed because Colors carries only Background/Text/Primary
// (gui/theme.go:30) and no text style. That also rules out calling
// layoutTitle(ctx, ...), whose two lines are inlined below.
//
// subject is the sentence the body opens with -- wipeWarningSubjectSecret or
// wipeWarningSubjectPassphrase, chosen by the caller from ctx.wipe (in scope
// at Run's call site) via wipeGuard.warningSubject(). Passed as a plain
// string rather than *wipeGuard so this function stays testable without a
// guard at all.
func wipeWarningOp(buf *op.Buffer, st Styles, th *Colors, dims image.Point, remaining time.Duration, subject string) op.Op {
	const margin = 8
	secs := int(remaining.Seconds() + 0.5)
	if secs < 0 {
		secs = 0
	}
	// layoutTitlef (gui/gui.go:1865) inlined -- it needs only ctx.B and Styles.title.
	title, titleSz := widget.Labelw(buf, st.title, dims.X-2*16, th.Text, "WIPING SECRET DATA")
	body, bodySz := widget.Labelwf(buf, st.body, dims.X-2*margin, th.Text,
		subject+"\n\nIt will be erased in %d seconds.\n\nTouch the screen to keep it.", secs)
	return op.Layer(
		body.Offset(image.Pt((dims.X-bodySz.X)/2, margin+titleSz.Y+margin)),
		title.Offset(image.Pt((dims.X-titleSz.X)/2, margin)),
		// Background LAST: op.Layer paints later ops BEHIND earlier ones
		// (gui.go:353, :591, :790).
		op.Color(buf, th.Background),
	)
}
