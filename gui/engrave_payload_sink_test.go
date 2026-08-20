package gui

import (
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// engraveToCompletion drives an EngraveScreen for the given plate all the way
// through the confirm-and-hold sequence and returns every stepper word the
// engraver received.
//
// The click/press/confirmDelay sequence is copied from TestEngraveScreenCancel
// and TestEngraveScreenError, which are the two tests that already know how to
// reach the cutting state.
func engraveToCompletion(t *testing.T, plateIdx int) (words int, digest uint64) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.engraver = e
		ctx := NewContext(p)

		scr := newTestEngraveScreenAt(t, ctx, plateIdx)
		frame, quit := runUI(ctx, func() {
			scr.Engrave(ctx, &engraveTheme)
		})
		defer quit()

		// Next until connect, then hold to start.
		click(&ctx.Router, Button3, Button3, Button3)
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)

		// Wait for the job to OPEN the engraver, then for it to CLOSE. Split
		// so a stall names which half stalled instead of "never closed".
		// time.Sleep advances synctest's fake clock, which is what lets the
		// engrave goroutine make progress between frames.
		opened := false
		for i := 0; i < 2048 && !opened; i++ {
			select {
			case <-e.opens:
				opened = true
			default:
				frame()
				time.Sleep(10 * time.Millisecond)
			}
		}
		if !opened {
			t.Fatalf("engraver never opened after %d frames (wrote %d words)",
				2048, mustWords(e))
		}

		closed := false
		for i := 0; i < 4096 && !closed; i++ {
			select {
			case <-e.closes:
				closed = true
			default:
				frame()
				time.Sleep(10 * time.Millisecond)
			}
		}
		if !closed {
			t.Fatalf("engraver opened but never closed (wrote %d words)", mustWords(e))
		}
		words, digest = e.engraved()
	})
	return words, digest
}

func mustWords(e *testEngraver) int {
	n, _ := e.engraved()
	return n
}

// TestEngraveWalkActuallyCutsSomething is the first test in this package able
// to observe that an engrave walk produced any cut at all.
//
// Until the payload sink was added (P1, 2026-08-19), testEngraver.Write
// returned len(steps) and dropped the words, so a walk could be seen to ATTEMPT
// engraving and never to have engraved anything. A regression that planned an
// empty spline, or wrote zero words, was invisible to all 18 walk files.
func TestEngraveWalkActuallyCutsSomething(t *testing.T) {
	words, _ := engraveToCompletion(t, 0)
	if words == 0 {
		t.Fatal("engrave walk completed having written ZERO stepper words: " +
			"the walk engraved nothing, which no test in this package could " +
			"previously detect")
	}
	t.Logf("plate 0 produced %d stepper words", words)
}

// TestEngravedStreamDependsOnThePlate is the differential half: it is not
// enough that SOMETHING was cut, it must be cut FROM THE PLATE HANDED OVER.
//
// A stub that emitted a fixed warm-up pattern, or a wiring defect that always
// engraved plate 0, satisfies the test above and fails this one.
func TestEngravedStreamDependsOnThePlate(t *testing.T) {
	wa, da := engraveToCompletion(t, 0)
	wb, db := engraveToCompletion(t, 1)
	if wa == 0 || wb == 0 {
		t.Fatalf("both plates must cut something: plate0=%d plate1=%d words", wa, wb)
	}
	if wa == wb && da == db {
		t.Fatalf("plates 0 and 1 produced an IDENTICAL cut (%d words, digest %#x); "+
			"the cut does not depend on the plate", wa, da)
	}
	t.Logf("plate 0: %d words digest %#x / plate 1: %d words digest %#x -- cuts differ",
		wa, da, wb, db)
}

// TestEngraveSinkDistinguishesStreams tests the SINK ITSELF, not a walk.
//
// It exists because the walk-level differential above is satisfied by the word
// COUNT alone -- plates 0 and 1 happen to differ in length -- so nothing there
// would notice a digest that never mixed. These cases pin the digest directly:
// equal-length streams that differ, and streams that differ only in order.
func TestEngraveSinkDistinguishesStreams(t *testing.T) {
	digestOf := func(words []uint32) (int, uint64) {
		e := newEngraver()
		if _, err := e.Write(words); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return e.engraved()
	}

	n1, d1 := digestOf([]uint32{1, 2, 3})
	n2, d2 := digestOf([]uint32{1, 2, 4})
	if n1 != n2 {
		t.Fatalf("fixtures must be equal length: %d vs %d", n1, n2)
	}
	if d1 == d2 {
		t.Fatal("digest does not distinguish equal-length streams that differ")
	}

	_, dFwd := digestOf([]uint32{1, 2, 3})
	_, dRev := digestOf([]uint32{3, 2, 1})
	if dFwd == dRev {
		t.Fatal("digest is order-insensitive: two plates cutting the same moves " +
			"in a different order would look identical")
	}

	// Repeating a stream must reproduce the digest, or the differential test
	// above would flag every rerun as a change.
	if _, again := digestOf([]uint32{1, 2, 3}); again != dFwd {
		t.Fatalf("digest is not deterministic: %#x then %#x", dFwd, again)
	}

	// A FAILED write engraved nothing, so it must not be recorded -- otherwise
	// the sink claims cuts the machine never made.
	e := newEngraver()
	e.ioErr = errors.New("io")
	if _, err := e.Write([]uint32{9, 9, 9}); err == nil {
		t.Fatal("Write must surface the injected error")
	}
	if n, _ := e.engraved(); n != 0 {
		t.Fatalf("a failed write was recorded as %d engraved words", n)
	}
}

// TestEngraveSinkCanKeepRawWords pins the opt-in escape hatch: a test that
// needs real geometry (the T4-sim decoder, when it is built) can have the whole
// stream, while the 18 walk files that only need "was anything cut" keep paying
// O(1) and hold nothing.
//
// The capture is a TEMP FILE under t.TempDir(), not memory and not the repo --
// a real plate is ~81 MB, which is fine as scratch on a PC and unacceptable as
// either RAM or a worktree artifact. The framework deletes it with the test.
func TestEngraveSinkCanKeepRawWords(t *testing.T) {
	off := newEngraver()
	if _, err := off.Write([]uint32{7, 8, 9}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if path := off.engravedWordsFile(); path != "" {
		t.Fatalf("spilled without opt-in: %s", path)
	}
	if n, _ := off.engraved(); n != 3 {
		t.Fatalf("count must be kept regardless of opt-in, got %d", n)
	}

	on := newEngraver()
	on.keepWordsIn(t)
	if _, err := on.Write([]uint32{7, 8, 9}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	path := on.engravedWordsFile()
	if path == "" {
		t.Fatal("opt-in did not create a spill file")
	}
	if !strings.HasPrefix(path, os.TempDir()) && !strings.Contains(path, t.TempDir()) {
		t.Fatalf("spill must live under the test temp dir, got %s", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill: %v", err)
	}
	if len(raw) != 12 {
		t.Fatalf("spill should be 3 words = 12 bytes, got %d", len(raw))
	}
	for i, want := range []uint32{7, 8, 9} {
		if got := binary.LittleEndian.Uint32(raw[4*i:]); got != want {
			t.Fatalf("word %d: got %d want %d", i, got, want)
		}
	}
}

// TestEngraveSinkSpillIsCleanedUp proves the temp file does not outlive its
// test -- the whole reason it is under t.TempDir() rather than a fixed path.
func TestEngraveSinkSpillIsCleanedUp(t *testing.T) {
	var path string
	t.Run("inner", func(t *testing.T) {
		e := newEngraver()
		e.keepWordsIn(t)
		if _, err := e.Write([]uint32{1, 2, 3}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		path = e.engravedWordsFile()
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("spill missing during the test: %v", err)
		}
	})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("spill survived its test at %s (err=%v)", path, err)
	}
}
