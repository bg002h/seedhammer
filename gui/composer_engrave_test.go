package gui

import (
	"strings"
	"testing"
)

// composerStateSeated builds a state with `slots` slots of which the first
// `seated` hold a source, which is the only input composerFormsFor reads.
func composerStateSeated(slots, seated int) *composerState {
	st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
	st.assigned = make([]composerAssignment, slots)
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
	for i := 0; i < seated; i++ {
		st.assigned[i].src = i
	}
	return st
}

// TestComposerFormsForOfferWhatSection7fAllows is §7f's three seating states,
// and each arm is a refusal to offer something the artifact cannot carry: a
// key-less composition HAS no concrete policy, and a partially seated one has
// no policy id until every slot is seated.
func TestComposerFormsForOfferWhatSection7fAllows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		slots  int
		seated int
		want   []composerForm
	}{
		{"nothing seated collapses to template only", 4, 0,
			[]composerForm{composerFormTemplateOnly}},
		{"partially seated offers no form A", 4, 2,
			[]composerForm{composerFormTemplateAndCards}},
		{"fully seated offers both", 4, 4,
			[]composerForm{composerFormConcrete, composerFormTemplateAndCards}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := composerFormsFor(composerStateSeated(tc.slots, tc.seated))
			if len(got) != len(tc.want) {
				t.Fatalf("offered %d form(s), want %d: %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("form %d is %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestComposerFormLabelsFitTheChoicePanel: ChoiceScreen draws its rows with
// widget.Label, which does not wrap, so a long label is drawn off the edge and
// the operator picks a truncated option.
func TestComposerFormLabelsFitTheChoicePanel(t *testing.T) {
	for _, f := range []composerForm{
		composerFormConcrete, composerFormTemplateAndCards, composerFormTemplateOnly,
	} {
		label := composerFormLabel(f)
		if label == "" {
			t.Errorf("form %v has no label", f)
		}
		assertChoiceLabelFits(t, label)
	}
}

// TestComposerSecretIsCutOnceForASeedThatFilledSeveralSlots is §7f verbatim
// ("a seed that filled several slots is cut ONCE") and the reason the picker
// this task once carried is gone: nothing consumed its answer, so §7f's secret
// plate had no builder at all. composerSecretCards is that builder, and ms1 is
// the form the plate planner carries as a bundle card (cardMS1); the
// words-plus-SeedQR plate is a backup.Seed and is filed with F-455.
func TestComposerSecretIsCutOnceForASeedThatFilledSeveralSlots(t *testing.T) {
	st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
	id, err := st.reg.add("seed 1", composerTestMnemonic(t), "", composerMainNet())
	if err != nil {
		t.Fatal(err)
	}
	st.sources = []composerSource{{kind: composerSourceSeed, seedID: id, fpPresent: true}}
	st.assigned = make([]composerAssignment, 3)
	for i := range st.assigned {
		st.assigned[i] = composerAssignment{src: 0}
	}
	cards, err := composerSecretCards(st)
	if err != nil {
		t.Fatalf("composerSecretCards: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("one seed at three slots produced %d secret plate(s), want 1 -- §7f "+
			"says a seed that filled several slots is cut ONCE, and three bearer "+
			"plates for one secret is three times the exposure for no recovery value",
			len(cards))
	}
	if cards[0].kind != cardMS1 {
		t.Errorf("the secret card is kind %v, want cardMS1 (the kind bundlePlateMark "+
			"never marks, gui/bundle_flow.go:574)", cards[0].kind)
	}
	if len(cards[0].strings) != 1 || len(cards[0].strings[0]) == 0 {
		t.Errorf("the secret card carries no ms1 string: %+v", cards[0].strings)
	}
	// A WATCH-ONLY build cuts none, which is the whole of the mode choice.
	none, err := composerSecretCards(&composerState{reg: &seedRegistry{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("a composition with no seed produced %d secret plate(s)", len(none))
	}
}

// TestComposerEngraveModeReusesMultisigBuildsLabels: "Full" must name what it
// LEAVES OUT when a passphrase was used, because a BIP-39 passphrase is a
// required spending factor and is never engraved -- a set labelled
// "Full (seed + keys)" that omits one is F-132's shape.
func TestComposerEngraveModeReusesMultisigBuildsLabels(t *testing.T) {
	bare := buildFullModeLabel(false)
	withPass := buildFullModeLabel(true)
	if bare == withPass {
		t.Fatalf("buildFullModeLabel renders the same label with and without a "+
			"passphrase (%q); the composer inherits the honesty of that label", bare)
	}
	if !strings.Contains(strings.ToLower(withPass), "passphrase") {
		t.Errorf("the passphrased Full label does not name the missing factor: %q", withPass)
	}
	assertChoiceLabelFits(t, bare)
	assertChoiceLabelFits(t, withPass)
	assertChoiceLabelFits(t, "Watch-only (keys)")
}
