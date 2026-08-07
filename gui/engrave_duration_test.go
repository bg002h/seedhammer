package gui

import (
	"image"
	"reflect"
	"strings"
	"testing"

	"seedhammer.com/bspline"
	"seedhammer.com/gui/op"
)

// The tick counts below are MEASURED, not invented: they are what the two real
// proof patterns plan at 1.0mm/s with 8 passes, the slowest feed and the
// deepest cut the Engraving settings screen offers.
//
//	CONSTPROOF!  5,221,685,814 ticks  = 453:17 at 192,000 ticks/s
//	TEXTPROOF!   5,143,579,063 ticks  = 446:30
//
// MaxUint32 is 4,294,967,295, so both are past it -- CONSTPROOF! by 1.22x. The
// firmware target is pico-plus2 / RP2350, where uint is 32 bits.
//
// Measured, as a fraction of MaxUint32:
//
//	                8.0mm/s x1  1.0mm/s x1  1.0mm/s x8
//	CONSTPROOF!         0.045x      0.155x      1.216x
//	TEXTPROOF!          0.042x      0.154x      1.198x
//	BOTHPROOF!          0.038x      0.130x      1.018x
//
// The slowest feed at one pass reaches only 0.155x -- 6.4x of headroom, which
// is why the speed slice overflowed nothing and needed no widening. Passes
// multiply that by 7.8x (not 8: travel between glyphs is cut once however many
// times each glyph is struck) and take all three patterns over in one step.
const (
	ftLongestJobTicks = 5_221_685_814 // CONSTPROOF! at 1.0mm/s, 8 passes
	ftLongJobTicks    = 5_143_579_063 // TEXTPROOF! at 1.0mm/s, 8 passes
	// What ftLongestJobTicks became when a 32-bit accumulator carried it:
	// 5,221,685,814 - 2^32. It displayed as 80:27 -- a five-and-a-half-fold
	// understatement of a seven-and-a-half-hour job, and then it underflowed.
	ftWrappedJobTicks = ftLongestJobTicks - 1<<32
)

// TestEngraveTickAccountingIs64Bit pins the DECLARED TYPE of every field on the
// tick path, and it is a type assertion rather than a value assertion for a
// reason that is easy to get wrong: `uint` and `uint64` are the same type on
// the amd64 host these tests run on. No arithmetic this test could perform
// would tell them apart. The overflow exists only on the 32-bit firmware
// target, where no test runs at all -- which is exactly why it shipped.
//
// Reflection does distinguish them on every platform (reflect.Uint vs
// reflect.Uint64), so this is the one check that fails on the host if someone
// narrows the path back. Each lookup is checked for existence too: a field
// renamed out from under this test would otherwise report a clean pass.
func TestEngraveTickAccountingIs64Bit(t *testing.T) {
	field := func(v any, name string) reflect.Type {
		t.Helper()
		st := reflect.TypeOf(v).Elem()
		f, ok := st.FieldByName(name)
		if !ok {
			t.Fatalf("%s has no field %q; this test is pinning a field that no longer exists", st, name)
		}
		return f.Type
	}
	for _, c := range []struct {
		what string
		got  reflect.Type
	}{
		// The planner's accumulator, and the plate that carries it.
		{"bspline.Attributes.Duration", field((*bspline.Attributes)(nil), "Duration")},
		{"gui.Plate.Duration", field((*Plate)(nil), "Duration")},
		{"gui.EngraveScreen.duration", field((*EngraveScreen)(nil), "duration")},
		// The running total the countdown subtracts, and the channel that
		// feeds it -- see reportProgress for why the channel accumulates too.
		{"gui.engraveStatus.Completed", field((*engraveStatus)(nil), "Completed")},
		{"gui.engraveJob.progress element", field((*engraveJob)(nil), "progress").Elem()},
	} {
		if c.got.Kind() != reflect.Uint64 {
			t.Errorf("%s is %s; on the 32-bit firmware target that holds %d ticks and the longest real job is %d",
				c.what, c.got, uint64(^uint32(0)), uint64(ftLongestJobTicks))
		}
	}
}

// TestEngraveRemainingSurvivesTheLongestJob is the arithmetic half: the
// countdown must report the real remaining time for a job that no longer fits
// in 32 bits, and must not produce a number at all when the driver has run past
// the plan.
func TestEngraveRemainingSurvivesTheLongestJob(t *testing.T) {
	const tps = 30 * 6400 // engraverParams.TicksPerSecond: 192,000
	if engraverParams.TicksPerSecond != tps {
		t.Fatalf("the machine runs at %d ticks/s, not the %d these cases were computed at",
			engraverParams.TicksPerSecond, tps)
	}
	for _, c := range []struct {
		what                string
		duration, completed uint64
		want                string
	}{
		{"the longest job, untouched", ftLongestJobTicks, 0, "453:17"},
		{"the second-longest job", ftLongJobTicks, 0, "446:30"},
		// Partway through, and still past MaxUint32 on both operands.
		{"the longest job, half cut", ftLongestJobTicks, ftLongestJobTicks / 2, "226:39"},
		// The value a 32-bit accumulator produced for the FIRST case above.
		// It is here so the wrong answer is named: a countdown that reads
		// 80:27 for this job is the defect, not a rounding difference.
		{"the wrapped value the bug produced", ftWrappedJobTicks, 0, "80:27"},
		// Resume synthesises catch-up motion the plan never counted, so
		// completed can legitimately exceed duration. Unsigned, the
		// subtraction would wrap to ~2^64 and print a 15-digit hour count.
		//
		// The overruns here are sized to DISCRIMINATE, which a one-tick
		// overrun does not: rem would be 2^64-1, and the round-up "+tps-1"
		// then wraps it back down to 0, so the broken arithmetic returns the
		// right answer by accident. Anything more than a second of catch-up
		// leaves the wrap band, and a real catch-up move is seconds.
		{"cut past the plan by a catch-up move", ftLongestJobTicks, ftLongestJobTicks + 1_000_000, "0:00"},
		{"cut far past the plan", ftLongestJobTicks, ftLongestJobTicks * 2, "0:00"},
		{"exactly finished", ftLongestJobTicks, ftLongestJobTicks, "0:00"},
		// Rounding is UP: a job with one tick left is not "done".
		{"one tick left", 1, 0, "0:01"},
	} {
		if got := engraveRemaining(c.duration, c.completed, tps); got != c.want {
			t.Errorf("%s: countdown reads %q, want %q (%d ticks left of %d)",
				c.what, got, c.want, c.duration-min(c.completed, c.duration), c.duration)
		}
	}
	// A zero-tick machine must not divide by zero mid-job with the needle down.
	if got := engraveRemaining(ftLongestJobTicks, 0, 0); got != "--:--" {
		t.Errorf("a zero-tick machine produced %q, want a non-numeric readout", got)
	}
}

// TestEngraveScreenCountdownAtTheLongestJob is the CALL-SITE half. The one
// above proves engraveRemaining is right; this proves the engrave screen
// actually asks it, with the plate's duration and the job's progress the right
// way round. A screen that computed its own countdown inline -- which is what
// it did before this fix -- would pass the test above and still lie.
//
// It renders the real draw path at the panel the machine has and reads the
// glyphs back with op.Drawer.ExtractText, so a countdown that is computed
// correctly and then not drawn fails here.
func TestEngraveScreenCountdownAtTheLongestJob(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)

	job := newEngraverJob(p, nil, ctx.Platform.EngraverParams().StepperConfig, 0)
	// A non-nil errs channel, never written: Status() restarts a job it finds
	// running with no error channel, which would spawn a real engrave goroutine
	// out of a drawing test.
	job.errs = make(chan error)
	job.status = engraveStatus{State: engraveRunning}
	scr := &EngraveScreen{duration: ftLongestJobTicks, job: job}

	dims := ctx.Platform.DisplaySize()
	content := scr.draw(ctx, &engraveTheme, dims)
	got := new(op.Drawer).ExtractText(image.Rectangle{Max: dims}, content)

	if !strings.Contains(got, "453:17") {
		t.Errorf("the engrave screen counts down %q for a %d-tick job; want 453:17 (seven and a half hours)",
			got, uint64(ftLongestJobTicks))
	}
	// The wrapped reading, named so the regression is caught by identity and
	// not merely by absence: 80:27 is what a 32-bit accumulator showed.
	if strings.Contains(got, "80:27") {
		t.Errorf("the engrave screen shows the 32-bit-wrapped countdown 80:27 for a %d-tick job",
			uint64(ftLongestJobTicks))
	}
}
