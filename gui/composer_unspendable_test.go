package gui

import (
	"encoding/hex"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// F-449 stage 4, SPEC_liana_unspendable_internal_key §0b and §7 on the device.

func composerStateFor(list md.PathList, kind md.UnspendableKind) *composerState {
	st := &composerState{reg: &seedRegistry{}, list: list, unspendable: kind}
	composerSizeAssignments(st)
	return st
}

// TestComposerUnspendablePredicateOnEveryTrPreset is §9 row 4's gate: §0b's
// predicate over ALL SIX tr presets, firing on exactly kofn-recovery and
// tiered-recovery. It is asserted as a SET, so a seventh preset added later
// fails until someone decides where it belongs.
//
// Mutations, each measured:
//   - drop conjunct 1 (the InternalKeyPath check): fires on
//     simple-timelocked-inheritance, whose real key path counts as the one
//     unlocked path -- r1's inverted rule, SPEC §0b's table row 1;
//   - drop conjunct 2 (the class check): fires on plain-multisig,
//     hashlock-gated and decaying-multisig;
//   - evaluate the classifier on the NUMS composition instead of kind 1
//     (class 2 NOT skipped): fires on nothing -- r1's rule, rows 2 and 3.
func TestComposerUnspendablePredicateOnEveryTrPreset(t *testing.T) {
	want := map[string]bool{
		"plain-multisig":                false,
		"simple-timelocked-inheritance": false,
		"kofn-recovery":                 true,
		"tiered-recovery":               true,
		"hashlock-gated":                false,
		"decaying-multisig":             false,
	}
	presets := composerPresets(md.ComposeTr)
	if len(presets) != len(want) {
		t.Fatalf("%d tr presets, the gate names %d", len(presets), len(want))
	}
	for _, p := range presets {
		w, named := want[p.name]
		if !named {
			t.Fatalf("tr preset %q is not in the gate's table", p.name)
		}
		fire, drop := composerUnspendableFires(composerStateFor(p.list, md.UnspendableNums))
		if fire != w {
			t.Errorf("%s: fires = %v, want %v (drop %+v)", p.name, fire, w, drop)
		}
	}
	for _, w := range []md.ComposeWrapper{md.ComposeWsh, md.ComposeShWsh, md.ComposeSh} {
		for _, p := range composerPresets(w) {
			if fire, _ := composerUnspendableFires(composerStateFor(p.list, md.UnspendableNums)); fire {
				t.Errorf("%s under wrapper %v: fires, and only a tr policy has a key path", p.name, w)
			}
		}
	}
}

// runUnspendableStep drives composerUnspendableStep alone, by touch.
func runUnspendableStep(t *testing.T, st *composerState, ret *bool) *sessionHarness {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	h := &sessionHarness{t: t, ctx: ctx, done: new(bool)}
	frame, drawer, quit := runUITouch(ctx, func() {
		*ret = composerUnspendableStep(ctx, &descriptorTheme, st)
		*h.done = true
	})
	h.frame, h.drawer = frame, drawer
	t.Cleanup(quit)
	return h
}

// TestComposerUnspendableScreenFirstPage is §0b COPY and the first-page gate:
// both rows are TAPPABLE on the first page (a paginated screen hides its
// overflow, so the count is taken from the hit areas, not the text), and the
// copy names each row's coordinators and says the two are different wallets.
//
// Mutation: lengthening the lead by ~250 characters pushes the Liana row to
// page 2: 1 tappable row, and this fails.
func TestComposerUnspendableScreenFirstPage(t *testing.T) {
	var ret bool
	h := runUnspendableStep(t, composerStateFor(composerTrPreset(t, "kofn-recovery"), md.UnspendableNums), &ret)
	body := h.mustReach("Which key path?")
	if pts := plateHitPoints(h.ctx, h.drawer()); len(pts) != 2 {
		t.Fatalf("the key-path screen drew %d tappable rows on its first page, want 2:\n%q", len(pts), body)
	}
	for _, want := range []string{"DIFFERENT WALLETS", "different addresses",
		"NUMS point", "Liana key", "Bitcoin Core", "Nunchuk", "Liana (v15.0)"} {
		if !uiContains(body, want) {
			t.Errorf("the first page does not say %q:\n%q", want, body)
		}
	}
}

// TestComposerUnspendableDefaultRow is §0b DEFAULT ROW, BOTH assertions -- one
// alone cannot fail. Proven by CONSEQUENCE (a highlight is a colour inversion
// ExtractText cannot see): Button3 with no tap takes the seeded row.
//
// THE FIRST ENTRY STARTS FROM A ZERO-VALUE composerState, as composerFlow
// builds it (a struct literal that never names the field), so the zero-value
// trap is inside the test: if UnspendableKind's zero value were Liana, a
// press-through would build the Liana wallet.
//
// Mutations: seeding `initial := 1` (a constant) fails "first entry";
// `initial := 0` (a constant) fails "re-entry"; swapping md's UnspendableNums
// and UnspendableLiana constants fails "first entry".
func TestComposerUnspendableDefaultRow(t *testing.T) {
	for _, tc := range []struct {
		what string
		st   func() *composerState
		want md.UnspendableKind
	}{
		{"first entry opens on NUMS", func() *composerState {
			st := &composerState{reg: &seedRegistry{}, list: composerTrPreset(t, "kofn-recovery")}
			composerSizeAssignments(st)
			return st
		}, md.UnspendableNums},
		{"re-entry after choosing Liana opens on Liana", func() *composerState {
			return composerStateFor(composerTrPreset(t, "kofn-recovery"), md.UnspendableLiana)
		}, md.UnspendableLiana},
	} {
		t.Run(tc.what, func(t *testing.T) {
			st := tc.st()
			var ret bool
			h := runUnspendableStep(t, st, &ret)
			h.mustReach("Which key path?")
			h.tapNav(Button3) // press straight through, no row tapped
			h.pump(8, "")
			if !*h.done || !ret {
				t.Fatalf("the step did not return forward (done %v, ret %v)", *h.done, ret)
			}
			if st.unspendable != tc.want {
				t.Fatalf("pressing through left kind %v, want %v", st.unspendable, tc.want)
			}
		})
	}
}

// TestComposerUnspendableRowTapChoosesLiana: the Liana row is reachable by
// TOUCH, the only input the SH2 has (no directional buttons), and Back leaves
// the kind as it was.
func TestComposerUnspendableRowTapChoosesLiana(t *testing.T) {
	st := composerStateFor(composerTrPreset(t, "tiered-recovery"), md.UnspendableNums)
	var ret bool
	h := runUnspendableStep(t, st, &ret)
	h.mustReach("Which key path?")
	h.tapRow(1, 2)
	h.pump(8, "")
	if !ret || st.unspendable != md.UnspendableLiana {
		t.Fatalf("tapping the Liana row and taking it left kind %v (ret %v)", st.unspendable, ret)
	}

	st = composerStateFor(composerTrPreset(t, "tiered-recovery"), md.UnspendableLiana)
	h = runUnspendableStep(t, st, &ret)
	h.mustReach("Which key path?")
	h.tapNav(Button1)
	h.pump(8, "")
	if ret || st.unspendable != md.UnspendableLiana {
		t.Fatalf("Back returned %v and left kind %v, want false and Liana", ret, st.unspendable)
	}
}

// TestComposerUnspendableResetIsThePredicate is §0b RESET, both halves:
//   - a Back-edit that makes a conjunct false drops the kind, and SAYS so
//     naming the fact that moved (here conjunct 1: path 1 became a bare
//     single key, so it is the real key path);
//   - the converse, which catches an over-eager reset: a Back-edit that keeps
//     both conjuncts true (kofn -> tiered) keeps the kind.
//
// Mutations: removing the reset (`st.unspendable = md.UnspendableNums`) fails
// the first half at the kind; resetting on every entry fails the second;
// dropping the showError fails the first at the signal.
func TestComposerUnspendableResetIsThePredicate(t *testing.T) {
	realKey := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 26280}},
	}}
	st := composerStateFor(realKey, md.UnspendableLiana)
	var ret bool
	h := runUnspendableStep(t, st, &ret)
	body := h.mustReach("LIANA KEY DROPPED")
	if !uiContains(body, "Path 1 is one key with no lock") {
		t.Errorf("the drop does not name its cause:\n%q", body)
	}
	h.tapNav(Button3)
	h.pump(8, "")
	if !ret || st.unspendable != md.UnspendableNums {
		t.Fatalf("after the drop: ret %v kind %v, want true and NUMS", ret, st.unspendable)
	}

	st = composerStateFor(composerTrPreset(t, "tiered-recovery"), md.UnspendableLiana)
	ret = false
	h = runUnspendableStep(t, st, &ret)
	h.mustReach("Which key path?")
	h.tapNav(Button3)
	h.pump(8, "")
	// THE STEP MUST HAVE RETURNED FORWARD (R0 m3): without this, a press that
	// never registered leaves the kind at Liana too, and the half passes on a
	// screen still waiting for input.
	if !*h.done || !ret {
		t.Fatalf("the re-entered step did not return forward (done %v, ret %v)", *h.done, ret)
	}
	if st.unspendable != md.UnspendableLiana {
		t.Fatalf("an edit that keeps both conjuncts true reset the kind to %v", st.unspendable)
	}
}

// TestComposerLianaKeyDroppedFits: the RESET signal is a showError modal, so it
// gets F-185's class check at the longest cause it can carry.
func TestComposerLianaKeyDroppedFits(t *testing.T) {
	for _, d := range []composerUnspendableDrop{
		{notTr: true}, {keyPath: 8}, {class: "two paths with one lock"}, {unbuilt: true},
	} {
		assertModalBodyFits(t, "the §0b reset signal", errorScreenBody,
			composerCopyLianaKeyDropped(composerUnspendableDropCause(d)))
	}
}

// TestComposerKeyPathChoiceIsPlacedBeforeTheChunks is §0b PLACEMENT, on the
// real flow: after the path list's Done the NEXT screen is the key-path choice,
// not the stub screen, and the stub screen then shows the Template-ID OF THE
// CHOSEN KIND -- which is only possible if the chunks were built after the
// choice. Back from the stub and forward again re-enters the choice seeded on
// Liana, and pressing through keeps the Liana id (no changed-id banner can
// fire, because nothing changed).
//
// Mutation: moving composerUnspendableStep below composerTemplateChunksFor
// makes the first stub screen show the NUMS id.
func TestComposerKeyPathChoiceIsPlacedBeforeTheChunks(t *testing.T) {
	list := composerTrPreset(t, "kofn-recovery")
	idOf := func(kind md.UnspendableKind) string {
		c, err := md.ComposeWithUnspendable(list, make([]*md.SlotOrigin, 4), kind)
		if err != nil {
			t.Fatal(err)
		}
		id, err := c.TemplateID()
		if err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(id[:])
	}
	liana, nums := idOf(md.UnspendableLiana), idOf(md.UnspendableNums)
	if liana == nums {
		t.Fatal("the two kinds share a Template-ID; SPEC §3e is broken below this test")
	}

	p := newEngravedAwarePlatform()
	p.engraver = newEngraver()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
	defer quit()
	h := &sessionHarness{t: t, ctx: ctx, frame: frame, drawer: drawer, done: new(bool)}

	// BY TOUCH THROUGHOUT (R0 m2): the SH2 has no directional buttons, so
	// every row is tapped and every nav press is a tap on the nav slot.
	h.mustReach("Build a new policy")
	h.choose(1) // Scan cards, [Build a new policy]
	h.mustReach("Which script?")
	h.choose(0) // [Taproot (tr)]
	h.mustReach("Start from?")
	h.choose(3) // Build my own paths, plain-multisig, simple-timelocked-inheritance, [kofn-recovery]
	h.mustReach("Done")
	composerPickDone(t, h)

	body := h.mustReach("Which key path?")
	if uiContains(body, "mk1 stub") {
		t.Fatalf("the stub screen drew before the key-path choice:\n%q", body)
	}
	h.tapRow(1, 2) // Liana key
	body = h.mustReach("mk1 stub (template)")
	if !strings.Contains(strings.ToLower(body), liana) {
		t.Fatalf("the stub screen does not show the Liana Template-ID %s:\n%q", liana, body)
	}

	h.tapNav(Button1) // Back from the stub -> the path list
	h.mustReach("Done")
	composerPickDone(t, h)
	h.mustReach("Which key path?")
	h.tapNav(Button3) // re-entry, straight through
	body = h.mustReach("mk1 stub (template)")
	if !strings.Contains(strings.ToLower(body), liana) {
		t.Fatalf("re-entry pressed through did not keep the Liana id %s:\n%q", liana, body)
	}
}

// composerPickDone takes the path list's `Done` row, which is the LAST row.
func composerPickDone(t *testing.T, h *sessionHarness) {
	t.Helper()
	pts := plateHitPoints(h.ctx, h.drawer())
	if len(pts) == 0 {
		t.Fatalf("the path list drew no rows:\n%q", h.content)
	}
	tap(&h.ctx.Router, h.drawer(), pts[len(pts)-1])
	h.next("after selecting Done")
	h.tapNav(Button3)
}
