package gui

import (
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/stepper"
)

type Engraver interface {
	stepper.Writer
	Close() error
	Stats() EngraverStats
}

type engraveJob struct {
	pl     Platform
	spline bspline.Curve
	// conf is the config the spline was PLANNED with, carried from Plate.Conf
	// rather than read back from pl. See Plate.Conf for why.
	conf engrave.StepperConfig
	opts jobOptions

	quit      chan<- struct{}
	errs      <-chan error
	progress  <-chan uint64
	lock      chan Engraver
	status    engraveStatus
	nknots    int
	safePoint engrave.SafePointer
}

type jobOptions int

const (
	suppressStalls jobOptions = 1 << iota
)

type engraveStatus struct {
	State engraveState
	// Completed is the number of engraver ticks completed.
	//
	// uint64, not uint: this is a RUNNING TOTAL over a whole job and the
	// firmware target is 32-bit. The widest real plate is ~1.2x MaxUint32
	// (see bspline.Attributes.Duration), so a uint counter wraps partway
	// through and the countdown that subtracts it goes backwards. The
	// progress channel is 64-bit for the same reason -- see reportProgress.
	Completed uint64
	// Error is the error message, in case of state
	// engraveFailed.
	Error string
}

type engraveState int

const (
	engraveIdle engraveState = iota
	engraveRunning
	engraveStopping
	engraveStopped
	engraveFailed
	engraveDone
)

func newEngraverJob(p Platform, spline bspline.Curve, conf engrave.StepperConfig, opts jobOptions) *engraveJob {
	return &engraveJob{
		pl:     p,
		spline: spline,
		conf:   conf,
		opts:   opts,
	}
}

// catchup is the motion that returns the head to the last safe point after an
// interruption, planned with the PLATE's config rather than the platform's.
//
// Extracted so it can be asserted: Resume synthesises new motion, and computing
// it at a feed the plate was not planned at is wrong the moment the feed is
// selectable. Reading e.pl.EngraverParams() here instead broke no test until
// this seam existed (mutation-tested 2026-08-06).
func (e *engraveJob) catchup() []bspline.Knot {
	return e.safePoint.Resume(e.conf)
}

func (e *engraveJob) Stop() {
	if e.status.State != engraveRunning {
		return
	}
	e.status.State = engraveStopping
	if e.quit != nil {
		close(e.quit)
		e.quit = nil
	}
}

func (e *engraveJob) Start() {
	if e.errs != nil {
		// Job is already running.
		return
	}
	errs := make(chan error, 1)
	progress := make(chan uint64, 1)
	quit := make(chan struct{})
	e.lock = make(chan Engraver, 1)
	e.errs = errs
	e.quit = quit
	e.progress = progress
	e.status.Error = ""
	e.status.State = engraveRunning
	go func() {
		defer e.pl.Wakeup()
		errs <- e.runEngraving(quit, progress)
	}()
}

// releaseResumeState zeroes the job's resume state once the job is provably
// abandoned. F-108.
//
// history is RESUME state, not cut state: its lifetime is the job, not the
// goroutine, because catchup() re-reads it on the operator's hold-to-resume
// (gui/gui.go:2747). Zeroing it at the goroutine's exit would drive that
// resume's catch-up motion to the origin at T:0 and cut a wrong plate -- worse
// than the residency it fixes.
//
// TERMINAL-ONLY. A terminal state is the receive on e.errs, so runEngraving has
// provably returned, with its defers complete, and there is no live writer.
//
// TWO non-terminal returns skip this, not one: Engrave returning on ctx.Done
// (the wipe) AND the double-Back return in engraveStopping, where the goroutine
// is still winding down. Neither is covered elsewhere -- the wipe unwind is
// ctx.B.Scrub() + Drawer.Release() and reaches no engrave state. That hole is
// F-110, not a covered case. Skipping is still right: zeroing under a live
// goroutine races it, and a wrecked plate is worse than the residue.
//
// Safe to call only where a restart is impossible. Start() has exactly one
// caller, inside EngraveScreen.Engrave's own loop, and every Engrave call site
// constructs a fresh EngraveScreen -- so "Engrave has returned" is that point.
func (e *engraveJob) releaseResumeState() {
	switch e.status.State {
	case engraveStopped, engraveDone, engraveFailed:
	default:
		return
	}
	e.safePoint.ClearHistory()
}

func (e *engraveJob) Stats() EngraverStats {
	select {
	case d := <-e.lock:
		st := d.Stats()
		e.lock <- d
		return st
	default:
		return EngraverStats{}
	}
}

func (e *engraveJob) Status() engraveStatus {
	select {
	case p := <-e.progress:
		e.status.Completed += p
	default:
	}
	select {
	case err := <-e.errs:
		e.errs = nil
		if e.status.State == engraveStopping {
			e.status.State = engraveStopped
		} else {
			e.status.State = engraveDone
		}
		if err != nil {
			e.status.State = engraveFailed
			e.status.Error = err.Error()
		}
	default:
	}
	if e.status.State == engraveRunning {
		// Restart if requested.
		e.Start()
	}
	return e.status
}

func (e *engraveJob) runEngraving(quit <-chan struct{}, progress chan uint64) (cerr error) {
	stall := e.opts&suppressStalls == 0
	d, err := e.pl.Engraver(stall)
	if err != nil {
		return err
	}
	// Offer the plan to the engraver before the first Write, so a consumer can
	// show the whole plate at the moment the cut begins rather than watching it
	// appear. nknots is the resume marker: it is the count of knots already
	// emitted, so past zero this pass is a hold-to-resume over the same spline.
	// notifyPlate is a no-op on the machine and PlateAware is not in the image
	// at all -- see plate_hook.go.
	notifyPlate(d, e.spline, e.conf, e.nknots > 0)
	e.lock <- d
	defer func() {
		d := <-e.lock
		if err := d.Close(); cerr == nil {
			cerr = err
		}
	}()

	drv := stepper.NewDriver(d)
	res := newSplineResumer(drv, e.catchup())
	skipKnots := e.nknots
	for k := range e.spline {
		// TODO: use iter.Pull to resume the spline if the goroutine stack cost is
		// reasonable.
		if skipKnots > 0 {
			skipKnots--
			continue
		}
		e.nknots++
		t, err := res.Knot(k)
		e.safePoint.Knot(k)
		e.safePoint.Progress(t)
		if !reportProgress(quit, progress, uint64(t)) || err != nil {
			return err
		}
	}
	return drv.Flush()
}

// reportProgress hands the driver's per-knot tick count to the UI through a
// one-slot channel, folding into whatever report is still unread.
//
// uint64 because the fold makes it an ACCUMULATOR, not a delta: every knot the
// UI does not collect is added to the pending value, so an unread channel
// carries a running total of the whole job. In practice the engrave screen
// collects it twice a second, but "the UI is prompt" is not a bound the type
// should have to rely on -- see engraveStatus.Completed for what the total
// reaches.
func reportProgress(quit <-chan struct{}, progress chan uint64, t uint64) bool {
	var p0 uint64
	select {
	case <-quit:
		return false
	case p0 = <-progress:
		progress <- t + p0
	case progress <- t:
	}
	return true
}

type Knotter interface {
	Knot(k bspline.Knot) (completed uint, err error)
}

func newSplineResumer(drv Knotter, catchup []bspline.Knot) *splineResumer {
	return &splineResumer{
		drv:     drv,
		catchup: catchup,
	}
}

type splineResumer struct {
	drv      Knotter
	catchup  []bspline.Knot
	progress int
}

func (s *splineResumer) Knot(k bspline.Knot) (completed uint, cerr error) {
	if c := s.catchup; c != nil {
		s.catchup = nil
		// F-108: c is a FRESH copy of the history knots, made per restart by
		// SafePointer.Resume, and once s.catchup is nil nothing outside this
		// block can reach it -- so a job-level sweep would zero a nil slice
		// while every restart left another intact copy behind.
		//
		// defer rather than a clear after the loop: the loop returns early on a
		// driver error and that path must zero too. Registered once per job, not
		// per knot -- this block runs only on the first resumed knot.
		defer clear(c)
		// Fast forward until the most recent knot.
		for _, k := range c {
			t, err := s.drv.Knot(k)
			s.progress += int(t)
			// Don't (double-)count the resuming knots as progress on the original spline.
			s.progress -= int(k.T)
			if err != nil {
				return 0, err
			}
		}
	}
	t, err := s.drv.Knot(k)
	s.progress += int(t)
	p := max(0, s.progress)
	s.progress -= p
	return uint(p), err
}
