//go:build !refugium

package gui

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// Characterization tests for StartScreen.Flow's debug-command arm (Refugium
// plan §4.1, R0 M-6), written BEFORE that arm moved into handleDebugCommand so
// the move is a refactor the default build can prove it did not change.
//
// They are !refugium because they pin the DEFAULT build's behaviour: the
// Refugium build carries neither command (debugcmd_refugium.go), and no tag
// reaches the start screen there at all (§4.2).

// driveStartScreenTag runs StartScreen.Flow with a reader that delivers rec
// once, pumping frames until done reports true or the deadline passes. It
// returns what Flow returned, whether it returned at all, and the last frame.
func driveStartScreenTag(t *testing.T, p *testPlatform, rec string, done func() bool) (act startScreenAction, returned bool, last string) {
	t.Helper()
	p.nfc = func() io.ReadCloser { return &oneShotNFC{rec: []byte(rec)} }
	ctx := NewContext(p)
	var gotOK bool
	frame, quit := runUI(ctx, func() {
		s := new(StartScreen)
		act, gotOK = s.Flow(ctx, &descriptorTheme)
		returned = gotOK
	})
	defer quit()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		content, ok := frame()
		if !ok {
			// Flow returned: no more frames.
			return act, returned, last
		}
		last = content
		if done() {
			return act, returned, last
		}
		time.Sleep(time.Millisecond)
	}
	return act, returned, last
}

// TestDebugCommandLockBootCharacterization: a `command: lock-boot` tag on the
// start screen calls Platform.LockBoot exactly once and STAYS on the start
// screen -- Flow does not return an action for it.
func TestDebugCommandLockBootCharacterization(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"LockBoot succeeds", nil},
		{"LockBoot fails", errors.New("otp: refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			var calls atomic.Int32
			p.lockBoot = func() error {
				calls.Add(1)
				return tc.err
			}
			// Keep pumping past the first call, so a second call or a return
			// would be seen.
			var after int
			_, returned, last := driveStartScreenTag(t, p, "command: lock-boot", func() bool {
				if calls.Load() == 0 {
					return false
				}
				after++
				return after > 50
			})
			if n := calls.Load(); n != 1 {
				t.Fatalf("LockBoot called %d times, want exactly 1", n)
			}
			if returned {
				t.Fatalf("StartScreen.Flow returned on a lock-boot tag; it must stay on the start screen")
			}
			if !uiContains(last, "Backup Wallet") {
				t.Fatalf("after lock-boot the screen is not the start screen; got %q", last)
			}
		})
	}
}

// TestDebugCommandQAProgramCharacterization: a `command: FOREVERLAURA!` tag
// returns the qaProgram action, and never touches LockBoot.
func TestDebugCommandQAProgramCharacterization(t *testing.T) {
	p := newPlatform()
	p.lockBoot = func() error {
		t.Error("LockBoot called for FOREVERLAURA!")
		return nil
	}
	act, returned, _ := driveStartScreenTag(t, p, "command: FOREVERLAURA!", func() bool { return false })
	if !returned {
		t.Fatal("StartScreen.Flow did not return on a FOREVERLAURA! tag")
	}
	if act.prog != qaProgram || act.scan != nil {
		t.Fatalf("FOREVERLAURA! returned %+v, want {prog: qaProgram, scan: nil}", act)
	}
}

// TestDebugCommandUnknownCharacterization: any other command is logged and
// then handed on as the scanned object itself (the arm falls through to the
// common `return startScreenAction{scan: cnt}`), which engraveObjectFlow then
// reports as an unknown format.
func TestDebugCommandUnknownCharacterization(t *testing.T) {
	p := newPlatform()
	p.lockBoot = func() error {
		t.Error("LockBoot called for an unknown command")
		return nil
	}
	act, returned, _ := driveStartScreenTag(t, p, "command: nope", func() bool { return false })
	if !returned {
		t.Fatal("StartScreen.Flow did not return on an unknown debug command")
	}
	dc, ok := act.scan.(debugCommand)
	if !ok || dc.Command != "nope" {
		t.Fatalf("unknown command returned %+v, want scan: debugCommand{\"nope\"}", act)
	}
}
