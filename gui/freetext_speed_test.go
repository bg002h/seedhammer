package gui

import (
	"slices"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// The Speed screen is tested through its option builder and through the PLANNED
// PLATE, never through a label. A speed plumbed to the screen and not to the
// planner would pass every assertion about the screen alone.

// TestSpeedOptionsAreBoundedAndNonZero pins the two properties that make a
// fixed list safe where a numeric box would not be. Verified 2026-08-06 by
// driving the real planner: EngravingSpeed=0 panics (engrave.go:1117, in
// timeScaler.Scale) and Jerk=0 panics (engrave.go:1155 via bezier.go:300), so a
// zero reaching the planner is a crash with the needle down.
//
// The ceiling is physical, not a preference: above 8mm/s engraving crosses into
// the StallGuard window (TCOOLTHRS 234) and the hammer strikes start reading as
// stalls. See cmd/controller/platform_sh2.go's minimumStallVelocity.
func TestSpeedOptionsAreBoundedAndNonZero(t *testing.T) {
	P := newPlatform().EngraverParams()
	_, speeds := ftSpeedOptions(P, true, 0)
	if len(speeds) != len(ftSpeedRungs) {
		t.Fatalf("the Speed screen offers %d entries, want %d", len(speeds), len(ftSpeedRungs))
	}
	// ChoiceScreen does not scroll and op.Layer draws content OVER the title, so
	// a list past roughly seven entries is silently covered rather than clipped.
	if len(ftSpeedRungs) > 7 {
		t.Errorf("%d speed rungs will overflow ChoiceScreen silently", len(ftSpeedRungs))
	}
	for i, s := range speeds {
		if s <= 0 {
			t.Errorf("entry %d offers %v mm/s; zero or negative panics the planner", i, s)
		}
		if s > ftSpeedCeilingMM {
			t.Errorf("entry %d offers %.1fmm/s, above the %.1fmm/s ceiling", i, s, ftSpeedCeilingMM)
		}
	}
}

// TestSpeedNeverTouchesSpeedOrTicks. Speed above TicksPerSecond is silently
// rate-limited, not rejected: stepper.go:49-53 clamps to +-1 microstep per tick,
// fill() has no return value and Driver.Knot() inspects no error, so no error
// path exists. The loss is permanent -- Interpolator.Step() returns true only
// for the segment's fixed planned tick count and then stops however far behind
// the physical position has fallen -- so every later stroke is offset. Only
// EngravingSpeed may move.
func TestSpeedNeverTouchesSpeedOrTicks(t *testing.T) {
	P := newPlatform().EngraverParams()
	_, speeds := ftSpeedOptions(P, true, 0)
	for _, s := range speeds {
		got := ftParamsAtSpeed(P, s)
		if got.Speed != P.Speed {
			t.Errorf("%.1fmm/s moved Speed %d -> %d", s, P.Speed, got.Speed)
		}
		if got.TicksPerSecond != P.TicksPerSecond {
			t.Errorf("%.1fmm/s moved TicksPerSecond %d -> %d", s, P.TicksPerSecond, got.TicksPerSecond)
		}
		if got.Acceleration != P.Acceleration || got.Jerk != P.Jerk {
			t.Errorf("%.1fmm/s moved acceleration or jerk", s)
		}
		if got.EngravingSpeed == 0 {
			t.Errorf("%.1fmm/s produced EngravingSpeed 0, which panics the planner", s)
		}
		if got.EngravingSpeed > got.TicksPerSecond {
			t.Errorf("%.1fmm/s produced EngravingSpeed %d above TicksPerSecond %d",
				s, got.EngravingSpeed, got.TicksPerSecond)
		}
	}
}

// TestSpeedReachesTheMotion is the load-bearing test: it asserts the chosen feed
// changes the PLANNED TOOLPATH, not a caption. Everything else here would pass
// with the value wired to a label and never to the planner.
//
// The ratio is well under 8x because travel moves do not slow down with the
// engraving feed -- only the cuts do -- so this asserts a floor rather than a
// figure. A plate that ignored the speed entirely would come back at 1.0x.
func TestSpeedReachesTheMotion(t *testing.T) {
	P := newPlatform().EngraverParams()
	const text = "the wallet is in the safe"

	fast, err := ftBuildPlate(ftParamsAtSpeed(P, 8.0), &ftPlanConst, text, "", "", false, 3.0, 0)
	if err != nil {
		t.Fatal(err)
	}
	slow, err := ftBuildPlate(ftParamsAtSpeed(P, 1.0), &ftPlanConst, text, "", "", false, 3.0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fast.Duration == 0 || slow.Duration == 0 {
		t.Fatalf("a plate planned with no duration: fast=%d slow=%d", fast.Duration, slow.Duration)
	}
	ratio := float64(slow.Duration) / float64(fast.Duration)
	if ratio < 3 {
		t.Errorf("1.0mm/s took %d ticks against 8.0mm/s %d ticks (%.2fx); the feed is not reaching the planner",
			slow.Duration, fast.Duration, ratio)
	}
	// And the plate carries the config it was planned with, so the resume path
	// cannot fall back to the platform's.
	if slow.Conf.EngravingSpeed != ftParamsAtSpeed(P, 1.0).EngravingSpeed {
		t.Errorf("the plate's snapshotted EngravingSpeed is %d, want %d",
			slow.Conf.EngravingSpeed, ftParamsAtSpeed(P, 1.0).EngravingSpeed)
	}
}

// TestDefaultSpeedReproducesTodaysPlate is what proves the feature is additive:
// the machine default must plan exactly what the program planned before the
// screen existed.
func TestDefaultSpeedReproducesTodaysPlate(t *testing.T) {
	P := newPlatform().EngraverParams()
	const text = "the wallet is in the safe"

	def := ftDefaultSpeedMM(P)
	got, err := ftBuildPlate(ftParamsAtSpeed(P, def), &ftPlanConst, text, "", "", false, 3.0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ftBuildPlate(P, &ftPlanConst, text, "", "", false, 3.0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := ftSpline(t, got), ftSpline(t, want); !slices.Equal(g, w) {
		t.Errorf("the default feed does not reproduce the pre-feature plate (%d knots vs %d)", len(g), len(w))
	}
	if got.Duration != want.Duration {
		t.Errorf("the default feed plans %d ticks against the pre-feature %d", got.Duration, want.Duration)
	}
	// Untouched params must be byte-identical, not merely equivalent.
	if ftParamsAtSpeed(P, 0) != P {
		t.Errorf("a zero speed altered the params; 0 must mean 'leave them alone'")
	}
}

// TestSpeedIsLockedOffAProofComposition. The proof gate is what keeps this
// feature away from seed, descriptor and passphrase plates -- it is load-bearing
// safety, not a convenience, because nothing on the finished steel records what
// feed it was cut at.
func TestSpeedIsLockedOffAProofComposition(t *testing.T) {
	P := newPlatform().EngraverParams()
	labels, speeds := ftSpeedOptions(P, false, 0)
	if len(labels) != 1 || len(speeds) != 1 {
		t.Errorf("the Speed screen offers %d entries with no proof loaded, want exactly 1", len(labels))
	} else if speeds[0] != 0 {
		t.Errorf("taking the only entry set the speed to %v, want it left alone", speeds[0])
	}
	if _, speeds := ftSpeedOptions(P, true, 0); len(speeds) != len(ftSpeedRungs) {
		t.Errorf("a loaded proof offers %d speeds, want the full list of %d",
			len(speeds), len(ftSpeedRungs))
	}
}

// TestSpeedNoteOnlyAppearsWhenNonDefault. The operator must not be able to
// approve a non-default feed without seeing it, and an ordinary plate must not
// grow a line that never varies.
func TestSpeedNoteOnlyAppearsWhenNonDefault(t *testing.T) {
	P := newPlatform().EngraverParams()
	def := ftDefaultSpeedMM(P)
	if got := ftSpeedNote(P, 0); got != "" {
		t.Errorf("an untouched feed noted %q, want no note", got)
	}
	if got := ftSpeedNote(P, def); got != "" {
		t.Errorf("the default feed noted %q, want no note", got)
	}
	other := float32(1.0)
	if def == other {
		other = 2.0
	}
	if got := ftSpeedNote(P, other); got == "" {
		t.Errorf("a %.1fmm/s feed produced no note; the operator would approve it unseen", other)
	}
}

// TestEveryPlateSnapshotsItsConfig: toPlate is the one place a spline is
// planned, so every plate -- seed, descriptor, passphrase, free text -- carries
// the config it was planned with. That is what lets the resume path stop
// re-reading the platform (gui/engraver.go), and it is the seam the eventual
// system-wide feature needs.
func TestEveryPlateSnapshotsItsConfig(t *testing.T) {
	P := newPlatform().EngraverParams()
	slow := ftParamsAtSpeed(P, 1.0)
	plate, err := toPlate(func(yield func(engrave.Command) bool) {
		yield(engrave.Move(bezier.Pt(P.Millimeter*10, P.Millimeter*10)))
		yield(engrave.Line(bezier.Pt(P.Millimeter*20, P.Millimeter*10)))
	}, slow)
	if err != nil {
		t.Fatal(err)
	}
	if plate.Conf != slow.StepperConfig {
		t.Errorf("toPlate did not snapshot the config it planned with")
	}
	if plate.Conf.EngravingSpeed == P.EngravingSpeed {
		t.Errorf("this test needs a non-default feed to be meaningful")
	}
}

// TestFlowCarriesTheChosenSpeedToTheEngraver drives the WHOLE program and
// asserts the plate handed to the engraver was planned at the feed the operator
// picked. Everything above it tests ftParamsAtSpeed and ftBuildPlate directly,
// which a flow that never called them would pass.
//
// Mutation-tested: dropping ftParamsAtSpeed from the engrave step left the
// entire suite green until this test existed.
func TestFlowCarriesTheChosenSpeedToTheEngraver(t *testing.T) {
	var got Plate
	var seen bool
	freetextEngraveHook = func(p Plate) { got, seen = p, true }
	t.Cleanup(func() { freetextEngraveHook = nil })

	h, _ := startFT(t)
	ftPastQR(h, false)
	// A proof composition is what unlocks the feed.
	ftTypeTrigger(h, ftProofTriggerConst)
	ftOK(h)
	h.tapWidget("proofYes")
	h.mustReach("lines")

	// Pick the slowest rung, which is the furthest from the default, through
	// the gear -- the flow no longer stops on a Speed step of its own.
	ftTapKey(h, ppSettings)
	ftChoose(h, "settings", 0) // Speed
	slowest := len(ftSpeedRungs) - 1
	ftChoose(h, "speed", slowest)
	h.tapNav(Button1) // leave settings
	h.mustReach("lines")
	ftOK(h)
	h.mustReach("Title")
	ftOK(h)
	h.mustReach("Footer")
	ftOK(h)
	h.mustReach("Confirm")
	ftOK(h)
	h.step()

	if !seen {
		t.Fatal("the flow never handed a plate to the engraver")
	}
	P := h.ctx.Platform.EngraverParams()
	want := ftParamsAtSpeed(P, ftSpeedRungs[slowest]).EngravingSpeed
	if got.Conf.EngravingSpeed != want {
		t.Errorf("engraved at %d microsteps/s, want %d (%.1fmm/s) -- the chosen feed did not reach the plate",
			got.Conf.EngravingSpeed, want, ftSpeedRungs[slowest])
	}
	if got.Conf.EngravingSpeed == P.EngravingSpeed {
		t.Errorf("this test needs a feed different from the machine default to mean anything")
	}
}

// TestCatchupUsesThePlatesConfig pins the WIRING: an engrave job resumes from
// the config its PLATE was planned with, never from the platform's.
//
// The behavioural consequence is LATENT for this slice, and that is worth
// stating rather than overclaiming. SafePointer.Resume plans its leading move
// with engrave=false, and appendLine then takes vlim from conf.Speed rather than
// conf.EngravingSpeed (engrave/engrave.go:1136-1141) -- and a proof-scoped feed
// never moves Speed. So with only EngravingSpeed adjustable the plate's config
// and the platform's produce identical catch-up, and mutating catchup() to read
// the platform back breaks nothing observable TODAY.
//
// It stops being latent the moment acceleration, jerk or travel speed become
// adjustable, which is the stated upgrade path. This test therefore differs the
// configs in a field Resume actually reads, so it fails on that mutation now
// rather than after the upgrade quietly reintroduces the divergence.
func TestCatchupUsesThePlatesConfig(t *testing.T) {
	p := newPlatform()
	P := p.EngraverParams()

	// Differ in Speed, which is what Resume's leading move actually reads.
	conf := P.StepperConfig
	conf.Speed = conf.Speed / 2
	job := newEngraverJob(p, nil, conf, 0)

	// Advance the safe point: Progress only moves it once the history ends in a
	// CLAMPED triple -- k0.Ctrl == k1.Ctrl and k1 == k2 -- which is exactly the
	// tripled control point a polyline vertex produces.
	at := bezier.Pt(10*P.Millimeter, 10*P.Millimeter)
	job.safePoint.Knot(bspline.Knot{Ctrl: bezier.Pt(P.Millimeter, P.Millimeter), T: 100})
	for range 3 {
		job.safePoint.Knot(bspline.Knot{Ctrl: at, T: 100, Engrave: true})
	}
	job.safePoint.Progress(400)

	got := job.catchup()
	wrong := job.safePoint.Resume(P.StepperConfig)
	if len(got) == 0 || len(wrong) == 0 {
		t.Fatalf("no catch-up motion was planned (%d / %d knots); the safe point never advanced",
			len(got), len(wrong))
	}
	if slices.Equal(got, wrong) {
		t.Fatalf("both configs plan identical catch-up; this test cannot discriminate")
	}
	if want := job.safePoint.Resume(conf); !slices.Equal(got, want) {
		t.Errorf("catch-up was planned with the platform's config, not the plate's")
	}
}
