package gui

import (
	"image"
	"testing"
	"time"

	"seedhammer.com/gui/op"
)

// The Run-level harness. Run has zero test coverage at a01b666 and
// testPlatform.AppendEvents ignores its deadline, so no test can drive Run's
// clock -- which is why §10.2.4's timer, its warning and the unwind are all
// unobservable without this. B2a-ii shipped two Criticals invisible to a green
// suite for want of exactly this kind of instrument.

// deadlinePlatform is a testPlatform whose AppendEvents actually BLOCKS until
// the deadline it is handed, so time.Sleep inside a synctest bubble advances
// Run's idle arithmetic. The embedded testPlatform supplies everything else.
// It models Platform.AppendEvents' wakeup channel, because NOT modelling it
// silently diverged from the device on the one path §10.2.4 row 2 depends on:
// a cut that ends while the loop is parked is un-parked ONLY by pl.Wakeup(),
// and with the wakeup unmodelled this harness reproduced F-106's own doubled
// window -- warning at 6m0s against a 3m0s spec -- with the fix applied
// (R0 round 0, I2/C1).
type deadlinePlatform struct {
	*testPlatform
	// queued events are delivered on the next AppendEvents and refresh Run's
	// last-input clock exactly as a real touch does.
	queued []Event
	// tickFloor is the minimum fake time one AppendEvents costs. See the
	// method's comment: without it a derivation freezes the bubble clock.
	tickFloor time.Duration
	// dirties counts Dirty calls; onDirty observes them. Together they are the
	// screensaver's only visibility, and the only un-park seam.
	dirties int
	onDirty func(n int)
	// poll, when non-nil, REPLACES the deadline block below: it is called once
	// per AppendEvents and returns the readings the panel produced on that
	// poll. polls counts the calls, and is the positive control for any test
	// that asserts on the ABSENCE of an effect (a generator that silently
	// stopped producing would otherwise pass for the wrong reason).
	//
	// It exists for F-103, whose subject is a panel that never stops
	// reporting -- and that is the device's own shape, not a convenience:
	// cmd/controller/platform_sh2.go:371's "don't starve touch input" fast
	// path returns the instant the touch IRQ has fired, BEFORE the deadline
	// timer is ever armed, so under a continuously-asserting panel
	// AppendEvents never blocks at all. tickFloor is still charged, for the
	// same reason it is charged below: without it a synctest bubble's clock
	// would never advance and no timer in Run could ever fire.
	//
	// nil in every other test, where the deadline block is the whole point.
	poll  func() []Event
	polls int
}

func newDeadlinePlatform() *deadlinePlatform {
	p := newPlatform()
	// The 240x240 default is a fiction no shipped device has (gui/gui_test.go:390).
	p.display = sh2DisplaySize
	// 10ms crosses idleTimeout in 18,000 ticks -- cheap under synctest, and
	// well inside maxRunFrames.
	return &deadlinePlatform{testPlatform: p, tickFloor: 10 * time.Millisecond}
}

// AppendEvents honours the deadline. Returning the slice UNCHANGED when nothing
// is queued is the property Run's `len(evts) > 0` refresh depends on: a
// platform that appended a synthetic event every tick would refresh the idle
// clock forever and NO timer could ever fire -- the test would pass by never
// arming anything.
//
// The FLOOR is load-bearing, not defensive. unlockDerive calls
// ctx.WakeupAt(time.Now()) before every ctx.Frame (unlock_kdf.go:295), so on a
// derivation every deadline is already expired, `time.Until` is <= 0, nothing
// ever durably blocks, and a synctest bubble's clock NEVER ADVANCES. Without
// the floor, Task 5's test passes with its own mutant applied -- a false PASS,
// not a hang. The floor is a field so a long test can trade fidelity for ticks.
//
// The zero deadline (first tick, before any WakeupAt) lands here too:
// time.Until(time.Time{}) is hugely negative, so it takes the floor. That is
// correct, and stated because it looks like a bug worth "fixing".
func (p *deadlinePlatform) AppendEvents(deadline time.Time, evts []Event) []Event {
	if len(p.queued) > 0 {
		evts = append(evts, p.queued...)
		p.queued = p.queued[:0]
		return evts
	}
	if p.poll != nil {
		p.polls++
		time.Sleep(p.tickFloor)
		return append(evts, p.poll()...)
	}
	d := time.Until(deadline)
	if d < p.tickFloor {
		d = p.tickFloor
	}
	// A Wakeup() CUTS the block short, exactly as
	// cmd/controller/platform_sh2.go:384 returns early on <-p.wakeups.
	// This is not decoration: engraveJob.Start's goroutine ends with
	// `defer e.pl.Wakeup()` (gui/engraver.go:110), and for a cut that finishes
	// while the loop is parked that wakeup is the ONLY thing that un-parks it.
	// Neither syncArmed call can see such an edge -- the pre-block call runs
	// before the sleep, the post-block call runs after it returns -- so the
	// promptness of row 2's fresh window rests entirely here (R0 round 0, I2).
	//
	// It cannot livelock the bubble ON THE WAKEUPS THIS FILE DRIVES: Wakeup
	// drains before it sends on a buffered-1 channel, so a STALE pending
	// wakeup is consumed once and the next block takes the floor again.
	//
	// That argument defeats a stale wakeup, NOT a caller that resignals every
	// iteration -- ConfirmDelay.Progress (gui/gui.go:321) does exactly that
	// for the duration of any press-and-hold confirm, and against such a
	// caller this select would return immediately every time and the bubble
	// clock would stop advancing. No test that constructs a deadlinePlatform
	// drives that UI path today (checked across all seven). If one ever does,
	// this is the line that breaks, and the floor has to be applied to the
	// wakeup branch too rather than only to the timer.
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-p.wakeups:
	}
	return evts
}

// Dirty counts refreshes and is the ONLY way a test can see the screensaver.
// saver.State.Draw writes straight to the platform (gui/saver/saver.go:311
// calls screen.Dirty) and never reaches onDraw, so without this, step 1.3's
// "must observe Run's saver activate" has nothing to observe. onDirty is also
// the only same-goroutine seam that can UN-park a parked flow.
func (p *deadlinePlatform) Dirty(r image.Rectangle) error {
	p.dirties++
	if p.onDirty != nil {
		p.onDirty(p.dirties)
	}
	return p.testPlatform.Dirty(r)
}

// tap queues one physical touch -- press AND release, the pair every existing
// touch test sends (gui/start_screen_touch_test.go:49).
func (p *deadlinePlatform) tap() {
	pos := image.Pt(sh2DisplaySize.X/2, sh2DisplaySize.Y/2)
	p.queued = append(p.queued,
		PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event(),
		PointerEvent{Pressed: false, Entered: true, Pos: pos}.Event(),
	)
}

// maxRunFrames bounds runSession. Run parks the flow whenever the screensaver
// activates -- the inner loop draws and `continue`s without ever returning
// control -- and under synctest the 40ms throttle costs no real time, so a
// mutant that trips the saver spins until `go test`'s 10-minute timeout and
// reports a SIGQUIT dump instead of a failure. A HANG IS WORSE THAN A FAILURE,
// and Task 7 runs many mutants unattended.
const maxRunFrames = 100000

// runSession drives Run's real body with a chosen flow, returning the extracted
// text of everything DRAWN -- content frames and Run's own warning alike.
//
// Observation goes through onDraw, not through the iterator, because Run yields
// func() bool: no value reaches the consumer (gui.go:2934). A test that counted
// iterator ticks would be asserting on nothing at all.
//
// onDraw is also the ONLY same-goroutine injection point a test has: Run blocks
// the test goroutine, and the parked flow cannot tap for itself, so
// "tap during the warning" must call p.tap() from inside the observer. Doing it
// from another goroutine is a data race.
// It returns parked=true instead of failing, because PARKING IS SOMETIMES THE
// EXPECTED RESULT: the unarmed screensaver legitimately parks the flow forever
// (the saver branch continues without returning control), and two of this
// plan's own tests -- step 1.3's saver self-check and step 4.1's "not armed" --
// exist precisely to observe that. A harness that always t.Fatal'd on a park
// would fail them unconditionally. Callers that require completion use
// mustFinish.
func runSession(t *testing.T, p *deadlinePlatform, flow func(ctx *Context, version string), onDraw func(o op.Op, text string)) (drawn []string, parked bool) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(flowBound); !ok {
			panic(r) // a real bug; never swallow it
		}
		t.Errorf("test flow exceeded %d iterations without ctx.Done -- Run is "+
			"discarding every frame (wiping stuck true?)", maxRunFrames)
	}()
	r := image.Rectangle{Max: sh2DisplaySize}
	observe := func(o op.Op) {
		d := new(op.Drawer)
		txt := d.ExtractText(r, o)
		drawn = append(drawn, txt)
		if onDraw != nil {
			onDraw(o, txt)
		}
	}
	ticks := 0
	for range runWithFlow(p, "test", flow, observe) {
		ticks++
		if ticks > maxRunFrames {
			// Stop ranging: yield returns false, Run sets ctx.Done, the flow
			// unwinds. Never t.Fatal from in here -- that would Goexit through
			// a live iterator.
			parked = true
			break
		}
	}
	return drawn, parked
}

// mustFinish is runSession for the common case, where a park is a failure.
func mustFinish(t *testing.T, p *deadlinePlatform, flow func(ctx *Context, version string), onDraw func(o op.Op, text string)) []string {
	t.Helper()
	drawn, parked := runSession(t, p, flow, onDraw)
	if parked {
		last := ""
		if len(drawn) > 0 {
			last = drawn[len(drawn)-1]
		}
		t.Fatalf("Run exceeded %d ticks without terminating -- flow is probably parked "+
			"(screensaver?). %d frames drawn, last = %q", maxRunFrames, len(drawn), last)
	}
	return drawn
}

// boundedFlow wraps a test flow so it cannot spin forever.
//
// maxRunFrames alone is NOT enough, and the gap is specific: ticks are counted
// in runSession's range body, which is driven by yield() at the top of Run's
// INNER loop -- but the discard guard (`if wiping { continue }`) is the first
// statement of the range body and skips that loop entirely. A mutant that
// leaves `wiping` stuck true therefore burns CPU with ZERO ticks and zero fake
// time, and the cap never trips. The flow is the thing spinning, so bounding
// the flow is the only cap that survives the guard.
// It PANICS with a sentinel rather than calling t.Errorf and returning, and the
// difference is the whole point. Returning ends only the current SESSION: the
// session loop sees `wiping` still true, builds a fresh Context, and calls the
// flow again with a fresh counter -- so the mutant this exists to catch runs
// forever, now emitting an unbounded stream of failures. t.Fatal is barred too,
// since Goexit through a live iterator is not safe. The panic unwinds
// runWithFlow entirely and cannot re-enter the session loop; runSession recovers
// it and turns it into one clean failure.
func boundedFlow(t *testing.T, body func(ctx *Context) bool) func(*Context, string) {
	t.Helper()
	return func(ctx *Context, _ string) {
		for n := 0; !ctx.Done; n++ {
			if n > maxRunFrames {
				panic(flowBound{})
			}
			if !body(ctx) {
				return
			}
		}
	}
}

// flowBound is boundedFlow's sentinel. A distinct type so runSession can
// re-panic anything else rather than swallowing a real bug.
type flowBound struct{}

// drawnContains reports whether any drawn frame carried str, with uiContains'
// whitespace-insensitive matching (gui/gui_test.go:516).
func drawnContains(drawn []string, str string) bool {
	for _, c := range drawn {
		if uiContains(c, str) {
			return true
		}
	}
	return false
}

// assertDrawn fails with the frames actually drawn, so a miss is diagnosable
// without a rerun.
func assertDrawn(t *testing.T, drawn []string, str string) {
	t.Helper()
	if !drawnContains(drawn, str) {
		t.Errorf("no drawn frame contains %q; got %d frames: %q", str, len(drawn), drawn)
	}
}
