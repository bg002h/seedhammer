package gui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/synctest"
)

// THE JOURNEY CAPTURE (G-P3.20).
//
// It is the WALK, instrumented -- not a second driver. Every screen this
// records is a screen TestWalkQRPathFromATxRecordToThePostCutScreen asserts on,
// driven by the same harness through the same flow, so the document and the
// test cannot drift: a screen that changes fails the walk before it reaches
// the journey.
//
// A journey whose device screens are produced by a script nobody runs beside
// the tests is a journey that rots. This one is a test.
//
// Set TX_JOURNEY_OUT to a path to write the capture; otherwise it is a no-op
// pass, so the file costs nothing on an ordinary run.
//
// WHAT IT IS NOT, said plainly: these frames come from op.Drawer.ExtractText
// over the firmware's own op tree, NOT from the emulator's 480x320 framebuffer
// the way design/journeys/*.pdf do. Same tree, same text, different renderer --
// so it can show WHAT the device says and cannot show how it LOOKS. The
// framebuffer capture needs a WASM build and playwright; it belongs with the
// P4 hardware session, where a photograph of real steel goes beside it.
func TestCaptureTransactionJourney(t *testing.T) {
	out := os.Getenv("TX_JOURNEY_OUT")
	if out == "" {
		t.Skip("set TX_JOURNEY_OUT=<path> to capture the journey")
	}
	var b strings.Builder
	synctest.Test(t, func(t *testing.T) {
		w := newTxWalk(t, "tx:"+rawHexOfEven(t))
		defer w.quit()
		shot := func(label, want string) {
			got := w.until(want)
			fmt.Fprintf(&b, "\n### %s\n\n```\n%s\n```\n", label, wrapFrame(got))
		}
		shot("1. Review — the transaction the payload holds", "Engrave this transaction?")
		w.confirm()
		shot("2. Which kind of plate", "Choose plate kind")
		w.confirm()
		shot("3. The plan, before anything is cut", "Engrave?")
		w.confirm()
		shot("4. The plate, named and numbered", "Engrave this plate")
		w.engraveOnePlate()
		synctest.Wait()
		click(&w.ctx.Router, Button3)
		synctest.Wait()
		shot("5. After the last plate — test it now", "TEST THEM NOW")
		click(&w.ctx.Router, Button2)
		shot("5 (page 2). The command that reads it back", "mt inspect")
		click(&w.ctx.Router, Button2)
		shot("5 (page 3). Why this machine cannot check it for you", "no camera")
	})
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes to %s", b.Len(), out)
}

// wrapFrame makes ExtractText's run-together output readable. The extractor
// concatenates text ops with no separators, so this is presentation only --
// nothing here changes what was captured.
func wrapFrame(s string) string {
	const width = 46
	var out []string
	for len(s) > width {
		out = append(out, s[:width])
		s = s[width:]
	}
	return strings.Join(append(out, s), "\n")
}
