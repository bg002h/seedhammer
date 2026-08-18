package gui

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

// engravedAwarePlatform is a testPlatform that also records the engraved-text
// hook, the way cmd/emu's does: it keeps the announced strings by id and
// commits one only when that id is reported engraved.
type engravedAwarePlatform struct {
	*testPlatform
	candidates map[uint64]string
	engraved   []string
	// unknown counts PlateEngraved calls for ids nobody announced. The hook
	// contract says ignore them; counting them is how the test can tell
	// "ignored" from "never happened".
	unknown int
}

func newEngravedAwarePlatform() *engravedAwarePlatform {
	return &engravedAwarePlatform{
		testPlatform: newPlatform(),
		candidates:   make(map[uint64]string),
	}
}

func (p *engravedAwarePlatform) PlateText(ids []uint64, text string) {
	for _, id := range ids {
		p.candidates[id] = text
	}
}

func (p *engravedAwarePlatform) PlateEngraved(id uint64) {
	s, ok := p.candidates[id]
	if !ok {
		p.unknown++
		return
	}
	p.engraved = append(p.engraved, s)
}

// TestValidateMdmkAnnouncesOneIdPerVariant pins the first half of the seam.
//
// One id per VARIANT, not per string: the operator has yet to choose between
// TEXT+QR, TEXT ONLY and QR ONLY, and any of the three may be the one that gets
// cut. Announce a single id and the census would silently depend on which
// variant they picked.
func TestValidateMdmkAnnouncesOneIdPerVariant(t *testing.T) {
	const md1 = "md1yqpqqxqq8xtwhw4xwn4qh"
	p := newEngravedAwarePlatform()

	labels, plates, err := validateMdmk(p, md1, "", "")
	if err != nil {
		t.Fatalf("validateMdmk: %v", err)
	}
	if len(plates) != len(labels) || len(plates) == 0 {
		t.Fatalf("got %d plates for %d labels", len(plates), len(labels))
	}
	if len(p.candidates) != len(plates) {
		t.Fatalf("announced %d ids for %d plates -- a variant the operator can choose is not "+
			"in the census", len(p.candidates), len(plates))
	}
	for i, pl := range plates {
		if pl.id == 0 {
			t.Errorf("plate %d (%s) carries id 0, which the hook treats as 'not from "+
				"validateMdmk' and ignores forever", i, labels[i])
			continue
		}
		if got := p.candidates[pl.id]; got != md1 {
			t.Errorf("plate %d (%s) announced as %q, want %q", i, labels[i], got, md1)
		}
	}

	// Ids are never reused, across strings or across calls. Reuse is the one
	// failure that cannot be seen from a single call: a recycled id would let a
	// plate cut an hour ago claim a string validated since.
	seen := make(map[uint64]bool)
	for _, pl := range plates {
		if seen[pl.id] {
			t.Errorf("id %d appears twice in one call", pl.id)
		}
		seen[pl.id] = true
	}
	_, more, err := validateMdmk(p, "mk1yqpqqxqq8xtwhw4xwn4qh", "", "")
	if err == nil {
		for _, pl := range more {
			if seen[pl.id] {
				t.Errorf("id %d was reused by a later validateMdmk -- a finished plate would be "+
					"attributed to whichever string was announced last", pl.id)
			}
		}
	}
}

// TestNothingIsEngravedUntilItIsAccepted is the half that keeps the census
// honest, and it is the whole reason the seam is two events rather than one.
//
// validateMdmk running is not evidence that anything was cut. The operator
// picks a variant, watches the plate, and still has to accept it -- or press
// Back, which is an ordinary thing to do. A census built from "a string was
// validated" would report plates nobody has.
func TestNothingIsEngravedUntilItIsAccepted(t *testing.T) {
	p := newEngravedAwarePlatform()

	if _, _, err := validateMdmk(p, "md1yqpqqxqq8xtwhw4xwn4qh", "", ""); err != nil {
		t.Fatalf("validateMdmk: %v", err)
	}
	if len(p.engraved) != 0 {
		t.Fatalf("validating a string recorded it as engraved: %q -- the operator has not even "+
			"chosen a variant yet", p.engraved)
	}
}

// TestAnUnannouncedPlateIsIgnored pins the mis-attribution the id exists to
// prevent, and it is the failure that would INFLATE a gate rather than break
// it.
//
// A seed, passphrase or free-text plate never passes through validateMdmk, so
// it carries id 0. Without the id -- if a consumer simply assumed the last
// string it heard about belonged to the next plate that finished -- validating
// an md1, backing out, and later cutting an unrelated seed plate would record
// the md1 as engraved when no such plate exists. A gate that reports a plate
// nobody cut is worse than no gate.
func TestAnUnannouncedPlateIsIgnored(t *testing.T) {
	p := newEngravedAwarePlatform()

	if _, _, err := validateMdmk(p, "md1yqpqqxqq8xtwhw4xwn4qh", "", ""); err != nil {
		t.Fatalf("validateMdmk: %v", err)
	}
	// The seed plate finishes: id 0, never announced.
	notifyPlateEngraved(p, 0)

	if len(p.engraved) != 0 {
		t.Errorf("an unannounced plate was recorded as %q -- a plate that never went through "+
			"validateMdmk entered the census", p.engraved)
	}
	if p.unknown != 1 {
		t.Errorf("the unannounced id was seen %d times, want 1 -- if the hook never fired at "+
			"all, this test is not exercising the path it claims to", p.unknown)
	}

	// And the announced one still works, so the check above is not passing
	// because the hook is simply dead.
	var anyID uint64
	for id := range p.candidates {
		if anyID == 0 || id < anyID {
			anyID = id
		}
	}
	notifyPlateEngraved(p, anyID)
	if !slices.Contains(p.engraved, "md1yqpqqxqq8xtwhw4xwn4qh") {
		t.Errorf("an ANNOUNCED plate was not recorded either (%q) -- the assertions above prove "+
			"nothing, because nothing is being recorded at all", p.engraved)
	}
}

// TestEngraveScreenReportsTheStringItEngraved covers the production call site
// the tests above do not: they drive the hook functions directly, so every one
// of them would still pass with the notify deleted from EngraveScreen.Engrave.
// This one drives the REAL screen to completion.
//
// The assertion that matters is the one BEFORE the final click. At that point
// the job has finished and the plate is cut -- and the operator has not
// accepted it, so nothing may be recorded yet. Reporting at engraveDone instead
// would pass every other test in this file and count a plate the operator
// walked away from.
func TestEngraveScreenReportsTheStringItEngraved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const md1 = "md1yqpqqxqq8xtwhw4xwn4qh"
		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.engraver = e
		ctx := NewContext(p)

		_, plates, err := validateMdmk(p, md1, "", "")
		if err != nil || len(plates) == 0 {
			t.Fatalf("validateMdmk(%q): %v, %d plates", md1, err, len(plates))
		}
		scr := NewEngraveScreen(ctx, plates[0])
		success := false
		frame, quit := runUI(ctx, func() {
			success = scr.Engrave(ctx, &engraveTheme)
		})
		defer quit()

		// Press next until connect is reached, then hold to confirm.
		click(&ctx.Router, Button3, Button3, Button3)
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
	loop:
		for {
			frame()
			select {
			case <-e.closes:
				break loop
			case <-p.wakeups:
			}
		}

		// Pump until the screen has actually RENDERED the finished state before
		// asserting on it. Breaking out of the loop above only means the
		// engraver closed; the flow has not necessarily re-entered its switch
		// on engraveDone yet. Measured: without this, reporting at engraveDone
		// instead of at accept SURVIVED the assertion below -- the mutant's
		// notify simply had not run by the time the check ran, and the check
		// passed for a reason that had nothing to do with the property.
		for scr.job.Status().State != engraveDone {
			frame()
		}
		synctest.Wait()
		frame()

		if len(p.engraved) != 0 {
			t.Fatalf("the plate was recorded as engraved (%q) while the operator had not yet "+
				"accepted it -- Back from here is an ordinary thing to do, and would leave a "+
				"plate in the census that nobody took", p.engraved)
		}

		click(&ctx.Router, Button3)
		synctest.Wait()
		if _, ok := frame(); ok || !success {
			t.Fatal("the engrave did not complete, so this test proves nothing")
		}
		if !slices.Contains(p.engraved, md1) {
			t.Errorf("an accepted engrave recorded %q, want it to contain %q", p.engraved, md1)
		}
	})
}
