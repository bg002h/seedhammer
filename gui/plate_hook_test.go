package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// plateAwarePlatform hands out an engraver that accepts the plan.
//
// It overrides testPlatform rather than widening it: testPlatform.engraver is
// concrete, and every other gui test reaches into that field. Making it an
// interface to serve one test would put a PlateAware method on the engraver
// the whole package shares.
type plateAwarePlatform struct {
	*testPlatform
	e Engraver
}

func (p *plateAwarePlatform) Engraver(stall bool) (Engraver, error) { return p.e, nil }

// plateAwareEngraver is a testEngraver that also accepts the plan.
type plateAwareEngraver struct {
	*testEngraver

	mu    sync.Mutex
	calls []plateCall
}

type plateCall struct {
	knots   int
	resumed bool
}

// Plate counts the knots it is handed, which is how the test checks that the
// spline arrives WHOLE. runEngraving is about to range the same value, so a
// hook that consumed it would leave the job with nothing to cut -- and that
// failure would look like an empty plate, not like a broken hook.
func (e *plateAwareEngraver) Plate(spline bspline.Curve, conf engrave.StepperConfig, resumed bool) {
	n := 0
	for range spline {
		n++
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, plateCall{knots: n, resumed: resumed})
}

func (e *plateAwareEngraver) seen() []plateCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]plateCall(nil), e.calls...)
}

// waitIdle drives Status() until the job leaves engraveRunning, as the engrave
// screen's own loop does.
func waitIdle(t *testing.T, job *engraveJob) engraveState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := job.Status()
		if st.State != engraveRunning {
			return st.State
		}
		if time.Now().After(deadline) {
			t.Fatal("the engrave job never left engraveRunning")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestPlateHookFiresOncePerJobWithTheWholeSpline pins the seam cmd/emu's plate
// overlay hangs on: an Engraver that implements PlateAware is handed the plan
// BEFORE the first Write, once per job, and told whether this job is a resume.
//
// The resumed flag is what lets the emulator keep ONE recording across an
// abort and a hold-to-resume -- the property cmd/emu/platform.go's single
// recorder exists for. Get it backwards and every resume restarts the overlay,
// which is indistinguishable from the plate itself restarting.
func TestPlateHookFiresOncePerJobWithTheWholeSpline(t *testing.T) {
	const knots = 3

	e := &plateAwareEngraver{testEngraver: newEngraver()}
	p := &plateAwarePlatform{testPlatform: newPlatform(), e: e}

	spline := func(yield func(bspline.Knot) bool) {
		for range knots {
			if !yield(bspline.Knot{}) {
				return
			}
		}
	}
	job := newEngraverJob(p, spline, p.EngraverParams().StepperConfig, suppressStalls)

	job.Start()
	if st := waitIdle(t, job); st != engraveDone {
		t.Fatalf("first run ended in state %v, want engraveDone", st)
	}

	got := e.seen()
	if len(got) != 1 {
		t.Fatalf("PlateAware.Plate called %d times for one job, want 1", len(got))
	}
	if got[0].resumed {
		t.Error("the FIRST run of a job reported resumed=true -- a fresh plate would be " +
			"drawn as a continuation of whatever the recorder still held")
	}
	if got[0].knots != knots {
		t.Errorf("the hook saw %d knots, want %d -- the spline handed to the hook is not "+
			"the one the job cuts", got[0].knots, knots)
	}

	// A resume is a SECOND runEngraving over the same job, with nknots already
	// past zero -- the path EngraveScreen takes on hold-to-resume.
	job.Start()
	if st := waitIdle(t, job); st != engraveDone {
		t.Fatalf("resumed run ended in state %v, want engraveDone", st)
	}

	got = e.seen()
	if len(got) != 2 {
		t.Fatalf("PlateAware.Plate called %d times across two runs, want 2", len(got))
	}
	if !got[1].resumed {
		t.Error("the resumed run reported resumed=false -- the overlay would reset its " +
			"recording mid-plate, which is the one thing the single recorder exists to avoid")
	}
}

// TestPlateHookIsAbsentFromTheFirmwareBuild is the structural half, and it is
// the reason the hook is split across two files instead of being one type
// assertion in engraver.go.
//
// A Plate spline is SEED-DERIVED GEOMETRY -- the same material F-107/F-108 are
// about -- and the emulator's consumer for it is JavaScript on a page, outside
// anything Go can wipe. So the firmware must not merely decline to use the
// hook, it must not CONTAIN it: `//go:build !tinygo` puts PlateAware and its
// type assertion outside the image the way gui/preview.go already puts the
// plate previews outside it.
//
// Nothing about a hook that leaked into the TinyGo build would fail to compile,
// which is why this is a test and not a comment.
//
// Mutations this pins: drop the `!tinygo` constraint from plate_hook.go; name
// PlateAware from any unconstrained gui file; delete the tinygo stub.
func TestPlateHookIsAbsentFromTheFirmwareBuild(t *testing.T) {
	const (
		hostFile   = "plate_hook.go"
		tinygoFile = "plate_hook_tinygo.go"
	)

	for _, f := range []struct{ name, want string }{
		{hostFile, "!tinygo"},
		{tinygoFile, "tinygo"},
	} {
		b, err := os.ReadFile(f.name)
		if err != nil {
			t.Fatalf("reading %s: %v -- the hook's build split is what keeps seed-derived "+
				"geometry out of the firmware image", f.name, err)
		}
		if got := buildConstraint(string(b)); got != "//go:build "+f.want {
			t.Errorf("%s carries %q, want %q", f.name, got, "//go:build "+f.want)
		}
	}

	// And no OTHER file in gui may name the interface IN CODE, tests excepted:
	// this file must be free to talk about it.
	//
	// The identifier is looked for in the parsed AST rather than in the bytes.
	// A substring scan is what this test did first, and it failed on
	// engraver.go and on the tinygo stub -- both of which only mention
	// PlateAware in a comment POINTING AT the split, which is the opposite of
	// the defect. A check that forbids naming the rule is a check that gets the
	// explanation deleted to make it pass.
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading gui: %v", err)
	}
	fset := token.NewFileSet()
	var scanned int
	for _, ent := range ents {
		name := ent.Name()
		if ent.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		// Mode 0: comments are not parsed, so only real identifiers are here.
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		var uses bool
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "PlateAware" {
				uses = true
			}
			return !uses
		})
		if uses && name != hostFile {
			t.Errorf("%s uses PlateAware in code but is not %s -- the interface and the type "+
				"assertion that reaches it belong in the one file the firmware build drops",
				name, hostFile)
		}
	}
	if scanned < 20 {
		t.Fatalf("INCONCLUSIVE: only %d non-test .go files scanned in gui, which is too few "+
			"to be this package -- the walk is looking in the wrong place", scanned)
	}
}

// buildConstraint returns the //go:build line preceding the package clause, or
// "" if the file carries none.
func buildConstraint(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return ""
		}
		if strings.HasPrefix(line, "//go:build ") {
			return line
		}
	}
	return ""
}
