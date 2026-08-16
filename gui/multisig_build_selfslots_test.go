package gui

import (
	"strings"
	"testing"
	"testing/synctest"
)

// ─── S5: the @S picker as a SET ──────────────────────────────────────────────
//
// Until this block the @S picker was a single-select ChoiceScreen and
// buildParamPickFlow wrote `p.SelfSlots = []int{sIdx}`. The MODEL and the
// engrave tail already accepted several held slots, so the flow could not build
// Trace B at all -- the plan's flagship wallet, where one operator holds three
// of four slots across two masters -- and S5's gate ("Trace B completes by test
// AND by emulator walk") was unreachable through the screens.
//
// THE SHAPE IS COMPOSED, NOT INVENTED. There is no multi-select widget in this
// package (measured: zero hits for MultiSelect/Checkbox/toggle-list over
// gui/*.go), and the shipped idiom for "produce a subset as []int" is
// buildCosignerPickFlow's loop of bounded ChoiceScreens. This reuses it, so
// every screen is a widget that already ships and is already tested, rather
// than a new toggle-list whose selection marker would be one more thing that
// can raster to zero pixels (F-78/F-151).

// s5PickSlots drives multisigSelfSlotPickFlow through a scripted sequence of
// taps and returns what it produced.
//
// `taps` is a list of (downs, needle) pairs: pump until the needle is on
// screen, press Down that many times, then confirm. A screen that does not
// appear is a FATAL, because tapping through a picker that did not draw
// silently answers the NEXT one -- which is how a walk reports a pass for a
// policy nobody asked for.
type s5Tap struct {
	needle string
	downs  int
	back   bool // Button1 instead of Button3
}

func s5PickSlots(t *testing.T, n int, taps []s5Tap) ([]int, bool) {
	t.Helper()
	var (
		got  []int
		ok   bool
		done bool
	)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			got, ok = multisigSelfSlotPickFlow(ctx, &descriptorTheme, n)
			done = true
		})
		defer quit()
		for i, tap := range taps {
			c, seen := pumpUntil(frame, tap.needle, 32)
			if !seen {
				t.Fatalf("tap %d: the screen carrying %q never appeared; the flow is on %q",
					i, tap.needle, c)
			}
			for d := 0; d < tap.downs; d++ {
				click(&ctx.Router, Down)
				frame()
			}
			if tap.back {
				click(&ctx.Router, Button1)
			} else {
				click(&ctx.Router, Button3)
			}
			frame()
		}
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("the slot picker did not return after the scripted taps")
		}
	})
	return got, ok
}

// TestSelfSlotPickerDefaultIsTheShippedOne is the regression floor: accepting
// every default still produces exactly {@0}, which is what the pre-S5
// single-select picker produced and what every shipped walk taps.
//
// It is asserted FIRST because the multi-select's whole risk is that it changes
// the ordinary single-slot build under the operator's feet.
func TestSelfSlotPickerDefaultIsTheShippedOne(t *testing.T) {
	got, ok := s5PickSlots(t, 3, []s5Tap{
		{needle: "Which slot is your key?"},   // @0, the default row
		{needle: "Do you hold another slot?"}, // "NO, THAT IS ALL", the default row
	})
	if !ok {
		t.Fatal("accepting every default abandoned the picker")
	}
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("the all-defaults pick is %v, want [0]; the shipped single-slot build "+
			"must be unchanged by the multi-select", got)
	}
}

// TestSelfSlotPickerSelectsASet is this block's central claim: the operator can
// hold SEVERAL slots, and the set that comes back is the set they picked.
//
// The set is NOT contiguous and NOT in pick order ({@2, @0} picked, {0,2}
// returned) so it cannot pass by returning a range or by returning the taps
// verbatim.
func TestSelfSlotPickerSelectsASet(t *testing.T) {
	got, ok := s5PickSlots(t, 4, []s5Tap{
		{needle: "Which slot is your key?", downs: 2}, // @2
		{needle: "Do you hold another slot?", downs: 1},
		{needle: "Which other slot is yours?", downs: 0}, // remaining @0,@1,@3 -> @0
		{needle: "Do you hold another slot?", downs: 0},  // done
	})
	if !ok {
		t.Fatal("the picker abandoned on a legitimate two-slot set")
	}
	want := []int{0, 2}
	if len(got) != len(want) {
		t.Fatalf("picked %v, want %v: the second slot never reached the set", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("picked %v, want %v (ASCENDING and distinct, which is what "+
				"buildSlotSources' account numbering walks)", got, want)
		}
	}
}

// TestSelfSlotPickerTakesTraceB is the shape S5 exists for: three of four slots.
func TestSelfSlotPickerTakesTraceB(t *testing.T) {
	got, ok := s5PickSlots(t, 4, []s5Tap{
		{needle: "Which slot is your key?", downs: 0},    // @0
		{needle: "Do you hold another slot?", downs: 1},  // yes
		{needle: "Which other slot is yours?", downs: 0}, // remaining @1,@2,@3 -> @1
		{needle: "Do you hold another slot?", downs: 1},  // yes
		{needle: "Which other slot is yours?", downs: 0}, // remaining @2,@3 -> @2
		{needle: "Do you hold another slot?", downs: 0},  // done
	})
	if !ok {
		t.Fatal("the picker abandoned on Trace B's slot set")
	}
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("Trace B's picker produced %v, want %v. This is the plan's flagship "+
			"wallet and the flow could not express it at all before this block", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Trace B's picker produced %v, want %v", got, want)
		}
	}
}

// TestSelfSlotPickerBackAbandons pins Back at BOTH surfaces the picker owns.
//
// Back must never be a way to CONFIRM a partial set: an operator who has picked
// @0, been asked about a second slot and pressed Back has not said "just @0" --
// they have said "take me out of here". The stage loop reads ok=false as "step
// back one stage", which is buildParamPickFlow's documented rule for every
// stage above the first.
func TestSelfSlotPickerBackAbandons(t *testing.T) {
	t.Run("Back at the first slot screen", func(t *testing.T) {
		got, ok := s5PickSlots(t, 3, []s5Tap{
			{needle: "Which slot is your key?", back: true},
		})
		if ok {
			t.Fatalf("Back at the first slot screen returned ok=true with %v", got)
		}
	})
	t.Run("Back at the another-slot question does not confirm the partial set", func(t *testing.T) {
		got, ok := s5PickSlots(t, 3, []s5Tap{
			{needle: "Which slot is your key?"},
			{needle: "Do you hold another slot?", back: true},
		})
		if ok {
			t.Fatalf("Back at the another-slot question CONFIRMED %v. Back is not a "+
				"way to say \"that is all\"; the screen has a row that says that", got)
		}
	})
	t.Run("Back at the which-other-slot screen", func(t *testing.T) {
		got, ok := s5PickSlots(t, 3, []s5Tap{
			{needle: "Which slot is your key?"},
			{needle: "Do you hold another slot?", downs: 1},
			{needle: "Which other slot is yours?", back: true},
		})
		if ok {
			t.Fatalf("Back at the which-other-slot screen returned ok=true with %v", got)
		}
	})
}

// TestSelfSlotPickerNeverAsksAOneAnswerQuestion is this package's own rule,
// applied: "a picker with one possible answer is a tap that teaches nothing"
// (syswSeedPicker says so in those words; buildCosignerPickFlow short-circuits
// on it). When exactly one slot is left unheld, saying YES to "another slot?"
// takes it without asking which.
func TestSelfSlotPickerNeverAsksAOneAnswerQuestion(t *testing.T) {
	got, ok := s5PickSlots(t, 2, []s5Tap{
		{needle: "Which slot is your key?"},             // @0
		{needle: "Do you hold another slot?", downs: 1}, // yes -> only @1 is left
	})
	if !ok {
		t.Fatal("the picker abandoned when the last remaining slot was taken")
	}
	want := []int{0, 1}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("picked %v, want %v: with one slot left there is nothing to choose "+
			"between, so no screen may be drawn for it", got, want)
	}
}

// TestSelfSlotSetReachesParams is the wiring claim, and it is separate from the
// picker's own test on purpose: a picker that returns the right set and a
// buildParamPickFlow that drops it on the floor are two green units and a
// broken flow. It drives the WHOLE parameter stage loop and reads p.SelfSlots.
func TestSelfSlotSetReachesParams(t *testing.T) {
	var (
		p    buildPolicyParams
		ok   bool
		done bool
	)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			p, ok = buildParamPickFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		type step struct {
			needle string
			downs  int
		}
		for i, s := range []step{
			{"Choose policy type", 0},            // wsh
			{"How many keys (n)?", 1},            // n = 3
			{"Required signatures (k of 3)?", 1}, // k = 2
			{"Which slot is your key?", 0},       // @0
			{"Do you hold another slot?", 1},     // yes
			{"Which other slot is yours?", 1},    // remaining @1,@2 -> @2
			{"Do you hold another slot?", 0},     // done
			{"Include key fingerprints?", 0},     // omit
		} {
			c, seen := pumpUntil(frame, s.needle, 32)
			if !seen {
				t.Fatalf("stage %d: %q never appeared; the flow is on %q", i, s.needle, c)
			}
			for d := 0; d < s.downs; d++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("buildParamPickFlow did not return")
		}
	})
	if !ok {
		t.Fatal("buildParamPickFlow abandoned on a complete parameter walk")
	}
	if p.N != 3 || p.K != 2 {
		t.Fatalf("picked a %d-of-%d, want 2-of-3", p.K, p.N)
	}
	want := []int{0, 2}
	if len(p.SelfSlots) != len(want) {
		t.Fatalf("p.SelfSlots = %v, want %v: the multi-select's set does not reach the "+
			"params, so every stage downstream still builds a one-slot policy", p.SelfSlots, want)
	}
	for i := range want {
		if p.SelfSlots[i] != want[i] {
			t.Fatalf("p.SelfSlots = %v, want %v", p.SelfSlots, want)
		}
	}
}

// TestSelfSourceQuestionNamesEverySlotItAnswersFor is §0.1 clause 3 on the ONE
// assumption this block makes.
//
// The flow asks the derived-vs-`both` question ONCE and applies the answer to
// EVERY held slot (buildPolicyParams.SelfFromCard is a single bool, and it is
// part of the model this block does not redesign). That is an assumption, and
// clause 3 puts an assumption's announcement on the surface where the decision
// is taken -- so the question itself must name every slot it will be applied
// to. A screen reading "Is your @0 key on a card?" that silently also answers
// for @1 and @2 is the invisible kind.
//
// The SINGLE-slot wording is asserted unchanged in the same test, because it is
// a pinned walk needle with exactly one production site
// (cmd/emu/needle_test.go's "key on a card?").
func TestSelfSourceQuestionNamesEverySlotItAnswersFor(t *testing.T) {
	t.Run("one held slot keeps the shipped wording", func(t *testing.T) {
		lead := buildSelfSourceLead([]int{2})
		if !strings.Contains(lead, "key on a card?") {
			t.Errorf("the one-slot lead is %q; it no longer carries the pinned needle "+
				"%q, so three shipped walks can no longer find this screen",
				lead, "key on a card?")
		}
		if !strings.Contains(lead, "@2") {
			t.Errorf("the one-slot lead %q does not name the slot", lead)
		}
	})
	t.Run("several held slots are all named", func(t *testing.T) {
		lead := buildSelfSourceLead([]int{0, 1, 2})
		for _, want := range []string{"@0", "@1", "@2"} {
			if !strings.Contains(lead, want) {
				t.Errorf("the lead %q does not name %s, so the operator answers about one "+
					"slot while the device applies the answer to three", lead, want)
			}
		}
	})
}
