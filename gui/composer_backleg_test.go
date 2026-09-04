package gui

import (
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/md"
)

// ═══ THE BACK LEG OUT OF THE PATH LIST ══════════════════════════════════════
//
// S4 walk W-6 (the operator, on bgbb50775) and W-7 (found measuring it).
//
// composerFlow's Back leg used to run composerWrapperPick ALONE and then jump
// straight back into the path list, which is two defects in four lines:
//
//   W-6, navigation: "Start from?" was passed once and could never be reached
//   again. Back landed on the script choice -- a screen the operator had left
//   two steps ago -- and re-picking a script skipped the preset screen, so the
//   six archetypes S0b shipped were unreachable for the life of a composition
//   unless the operator discarded it and started over.
//
//   W-7, funds safety: the wrapper was assigned DIRECTLY, bypassing
//   composerShapeGuard (§8j's confirm) and composerApplyShapeEdit (the
//   discard). §7d's rule -- "any change that moves slot NUMBERING (the
//   wrapper, the path count, or a path's key count) after at least one slot
//   has been assigned discards ALL assignments; the operator is told so before
//   the edit is accepted" -- was met by the path list's own "Change the
//   script" row and unmet by the Back leg one function away. Slot numbering is
//   §5's first-appearance order, which the wrapper changes: measured below,
//   wsh and tr permute the SAME shape's slots at the SAME slot count, so
//   composerSizeAssignments kept st.assigned untouched and the key seated as
//   "Path 1 key 1 of 2" became Path 2's sole spending key. No screen names a
//   slot's path after seating (composerMappingLines prints index and origin
//   only), so nothing said so.

// TestComposerWrapperChangePermutesSlotsAtEqualCount is WHY the discard
// matters -- the codec is the authority and this reads its answer.
func TestComposerWrapperChangePermutesSlotsAtEqualCount(t *testing.T) {
	list := md.PathList{Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
	}}
	slotsFor := func(w md.ComposeWrapper) []md.ComposeSlot {
		l := list
		l.Wrapper = w
		c, err := md.Compose(l)
		if err != nil {
			t.Fatalf("wrapper %v: %v", w, err)
		}
		return c.Slots()
	}
	wsh, tr := slotsFor(md.ComposeWsh), slotsFor(md.ComposeTr)
	if len(wsh) != len(tr) {
		t.Fatalf("the two wrappers no longer agree on the slot COUNT for this shape "+
			"(wsh %d, tr %d); the carried-assignment hazard this guards needs equal "+
			"counts, so re-derive the fixture rather than deleting the test", len(wsh), len(tr))
	}
	same := true
	for i := range wsh {
		if wsh[i].Path != tr[i].Path || wsh[i].Ordinal != tr[i].Ordinal {
			same = false
		}
	}
	if same {
		t.Fatalf("wsh and tr number this shape's slots identically, so the fixture no "+
			"longer demonstrates the hazard §8j exists for.\nwsh=%+v\ntr=%+v", wsh, tr)
	}
	// The measured permutation, pinned: under tr the internal key is extracted
	// from the first single-key path, which is path 1 here, so slot @0 changes
	// which path it serves.
	if wsh[0].Path != 0 || tr[0].Path != 1 {
		t.Errorf("the permutation is not the one the tests below rely on: "+
			"wsh @0 serves path %d (want 0), tr @0 serves path %d (want 1)", wsh[0].Path, tr[0].Path)
	}
}

// TestComposerBackFromThePathListReturnsToTheStartFromScreen is W-6.
//
// Back is the INVERSE of the way in: paths -> "Start from?" -> "Which
// script?", and picking a script walks forward through "Start from?" again.
func TestComposerBackFromThePathListReturnsToTheStartFromScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		pumpUntil(frame, "Which script?", 24)
		click(&ctx.Router, Down) // -> Segwit (wsh)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Start from?", 24)
		click(&ctx.Router, Down) // -> plain-multisig
		click(&ctx.Router, Button3)

		got, ok := pumpUntil(frame, "Path 1: 2-of-3", 24)
		if !ok {
			t.Fatalf("the preset did not seed the path list.\nLast frame: %q", got)
		}

		// (1) BACK RETURNS TO THE SCREEN BEFORE THE PATH LIST.
		click(&ctx.Router, Button1)
		if got, ok = pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("Back at the path list did not return to \"Start from?\"; it landed "+
				"here instead, and the preset picker was then unreachable for the life "+
				"of the composition (W-6).\nLast frame: %q", got)
		}

		// (2) BACK AGAIN REACHES THE WRAPPER (W-1's rule, unchanged).
		click(&ctx.Router, Button1)
		if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("Back on the preset picker did not return to the wrapper choice.\nLast frame: %q", got)
		}

		// (3) AND PICKING A SCRIPT WALKS FORWARD THROUGH "Start from?" AGAIN,
		// which is the half the operator reported: re-picking a script used to
		// skip it.
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("re-picking a script skipped \"Start from?\" (W-6).\nLast frame: %q", got)
		}

		// (4) AND THE BLANK ROW KEEPS THE COMPOSITION. It is the default row
		// and the safest-looking one; blanking the list there would make the
		// screen a trap rather than a way back.
		click(&ctx.Router, Button3) // row 0 = Build my own paths
		if got, ok = pumpUntil(frame, "Path 1: 2-of-3", 24); !ok {
			t.Fatalf("\"%s\" discarded the composition on the way back; §7b's rule is "+
				"that going back loses nothing.\nLast frame: %q", composerPresetBlankRow, got)
		}
	})
}

// TestComposerBackLegWrapperChangeAsksBeforeDiscardingSeats is W-7, walked
// from a keyed payload -- the only kind of test that can fail on it, because
// every screen it passes has its own green unit test.
func TestComposerBackLegWrapperChangeAsksBeforeDiscardingSeats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{
			composerTestKeyRecord, composerTestKeyRecord2, composerTestNowRecord,
		}, nil)

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		pumpUntil(frame, "Build a new policy", 24)
		click(&ctx.Router, Down, Down)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Which script?", 24)
		click(&ctx.Router, Down) // Segwit (wsh)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Start from?", 24)
		click(&ctx.Router, Button3) // Build my own paths

		// PATH 1: 2-of-2, PATH 2: a single key -- the shape the test above
		// measured the permutation on.
		pumpUntil(frame, "Add a spend path", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Button3) // Keys
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Down) // 1 -> 2
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Down) // 1 -> 2
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "Path 1: 2-of-2", 24); !ok {
			t.Fatalf("path 1 was never added.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // -> Add a spend path
		click(&ctx.Router, Button3)
		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Button3) // Keys
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Button3) // 1
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Button3) // 1
		if got, ok := pumpUntil(frame, "Path 2: 1 key", 24); !ok {
			t.Fatalf("path 2 was never added.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down, Down) // -> Done
		click(&ctx.Router, Button3)
		if _, ok := pumpUntil(frame, "Sorted keys, or your order?", 24); ok {
			click(&ctx.Router, Button3)
		}
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// SEAT @0 AND @1 from the payload's two key: records, and leave @2.
		if got, ok := pumpUntil(frame, "Slot @0", 24); !ok {
			t.Fatalf("seating never started.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Slot @1", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Slot @2", 24)
		click(&ctx.Router, Down) // Type a seed -> Leave unseated
		click(&ctx.Router, Button3)

		// §8p's shortfall, then "Back to the paths": the path list, holding
		// two seats.
		if got, ok := pumpUntil(frame, "Unfilled:", 12); !ok {
			t.Fatalf("§8p never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "What now?", 12); !ok {
			t.Fatalf("§8p's choice never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Back to the paths
		if got, ok := pumpUntil(frame, "Spend paths", 12); !ok {
			t.Fatalf("§8p's \"Back to the paths\" did not reach the path list.\nLast frame: %q", got)
		}

		// THE BACK LEG, with seats held: wsh -> tr renumbers.
		click(&ctx.Router, Button1)
		if got, ok := pumpUntil(frame, "Start from?", 12); !ok {
			t.Fatalf("Back at the path list did not return to \"Start from?\".\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1)
		if got, ok := pumpUntil(frame, "Which script?", 12); !ok {
			t.Fatalf("Back did not reach the wrapper picker.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Taproot (tr)
		pumpUntil(frame, "Start from?", 12)
		click(&ctx.Router, Button3) // Build my own paths: keep the shape

		// (1) §8j IS ASKED, BEFORE THE EDIT IS ACCEPTED.
		got, ok := pumpUntil(frame, "CLEARS THE KEYS", 24)
		if !ok {
			t.Fatalf("the wrapper changed on the Back leg without asking §8j, which §7d "+
				"requires before an edit that moves slot numbering is accepted (W-7). "+
				"The path list's own \"Change the script\" row asks it.\nLast frame: %q", got)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		// (2) AND THE SEATS ARE GONE: every slot expects a key again. A
		// carried seat here does not fail -- it derives another wallet's
		// address and shows it to the operator as proof.
		if got, ok = pumpUntil(frame, "Spend paths", 24); !ok {
			t.Fatalf("the path list never came back after the confirm.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down, Down) // -> Done
		click(&ctx.Router, Button3)
		if _, ok = pumpUntil(frame, "Sorted keys, or your order?", 24); ok {
			click(&ctx.Router, Button3)
		}
		if got, ok = pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew after the wrapper change.\nLast frame: %q", got)
		}
		if got, ok = composerPageUntil(t, ctx, frame, "Slot @0", 10); !ok {
			t.Fatalf("the stub screen never named slot @0.\nLast frame: %q", got)
		}
		if uiContains(got, "73c5da0a") {
			t.Errorf("a seat survived a wrapper change on the Back leg: the stub screen "+
				"still shows a seated slot, and under tr slot @0 serves a different "+
				"path than it did under wsh, so this key now spends where the operator "+
				"never put it (W-7).\nFrame: %q", got)
		}
		if !uiContains(got, "expects a key at") {
			t.Errorf("the stub screen shows no unseated slot after a discard that should "+
				"have cleared every seat.\nFrame: %q", got)
		}
	})
}

// ═══ THE SIGNATURE IS NOT THE NUMBERING ═════════════════════════════════════
//
// Verification of the W-6/W-7 fix (composer-S4-W6-verification.md) found that
// the fix above opened a SECOND door to W-7's failure class, and that a third
// was already open. Both are one defect: composerShapeSignature captured the
// wrapper, the path count and each path's key count -- §7d's own enumeration --
// while md's lowerTr numbers slots from an internal key it picks with
// isBareSingle() (Keys.N == 1, no Lock, no Hash). So a LOCK or a HASH decides
// which path owns slot @0 under tr, and two lists the signature calls equal
// can number their slots completely differently. composerApplyShapeEdit then
// compared signatures, saw no move, and kept every seat.
//
// The fix is to stop re-deriving the codec's rule in the GUI: the signature
// now carries md.Composed.Slots(), the mapping itself.

// TestComposerShapeSignatureSeesTheCodecsNumbering is the unit statement of it.
func TestComposerShapeSignatureSeesTheCodecsNumbering(t *testing.T) {
	// Hand-built and preset shapes that §7d's enumeration cannot tell apart:
	// same wrapper, same path count, same key counts. Under tr the hand-built
	// list has a bare single at path 1 and the preset has none, so @0 changes
	// which path it serves.
	hand := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
	}}
	var preset md.PathList
	for _, p := range composerPresets(md.ComposeTr) {
		if p.name == "decaying-multisig" {
			preset = p.list
		}
	}
	if len(preset.Paths) == 0 {
		t.Fatal("the decaying-multisig preset is gone; re-derive this fixture")
	}
	handSlots, presetSlots := composerSlotsFor(t, hand), composerSlotsFor(t, preset)
	if len(handSlots) != len(presetSlots) {
		t.Fatalf("the fixture no longer has equal slot counts (%d vs %d), so it cannot "+
			"demonstrate the hazard", len(handSlots), len(presetSlots))
	}
	moved := false
	for i := range handSlots {
		if handSlots[i].Path != presetSlots[i].Path {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("the two shapes no longer permute; re-derive the fixture.\nhand=%+v\npreset=%+v",
			handSlots, presetSlots)
	}
	if composerShapeSignature(hand) == composerShapeSignature(preset) {
		t.Errorf("two shapes whose slots the codec numbers DIFFERENTLY share a shape "+
			"signature, so composerApplyShapeEdit keeps every seat across the change "+
			"and keys spend where the operator never put them.\nhand   %+v\npreset %+v\nsig %q",
			handSlots, presetSlots, composerShapeSignature(hand))
	}
}

func composerSlotsFor(t *testing.T, list md.PathList) []md.ComposeSlot {
	t.Helper()
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	return c.Slots()
}

// TestComposerBackLegPresetAsksBeforeDiscardingSeats is C-1, walked from a
// keyed payload on the operator's own route -- no lock screen, no hash screen.
func TestComposerBackLegPresetAsksBeforeDiscardingSeats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{
			composerTestKeyRecord, composerTestKeyRecord2, composerTestNowRecord,
		}, nil)

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		pumpUntil(frame, "Build a new policy", 24)
		click(&ctx.Router, Down, Down)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Which script?", 24)
		click(&ctx.Router, Button3) // Taproot (tr)
		pumpUntil(frame, "Start from?", 24)
		click(&ctx.Router, Button3) // Build my own paths

		// [2-of-2, 1 key, 1 key]: the shape whose signature equals
		// decaying-multisig's while its tr numbering does not.
		addPath := func(rowsDown int, keys int) {
			t.Helper()
			pumpUntil(frame, "Add a spend path", 24)
			for range rowsDown {
				click(&ctx.Router, Down)
			}
			click(&ctx.Router, Button3)
			pumpUntil(frame, "What can spend on this path?", 24)
			click(&ctx.Router, Button3) // Keys
			pumpUntil(frame, "how many keys?", 24)
			for range keys - 1 {
				click(&ctx.Router, Down)
			}
			click(&ctx.Router, Button3)
			pumpUntil(frame, "how many must sign?", 24)
			for range keys - 1 {
				click(&ctx.Router, Down)
			}
			click(&ctx.Router, Button3)
		}
		addPath(0, 2) // Path 1: 2-of-2
		addPath(1, 1) // Path 2: 1 key
		addPath(2, 1) // Path 3: 1 key
		if got, ok := pumpUntil(frame, "Path 3: 1 key", 24); !ok {
			t.Fatalf("the three-path shape was never built.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down, Down, Down) // -> Done
		click(&ctx.Router, Button3)
		if _, ok := pumpUntil(frame, "Sorted keys, or your order?", 24); ok {
			click(&ctx.Router, Button3)
		}
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		pumpUntil(frame, "Slot @0", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Slot @1", 24)
		click(&ctx.Router, Button3)
		// The remaining slots have no source left: rows are [Type a seed,
		// Leave unseated]. This shape has four slots and the payload two keys.
		for range 2 {
			pumpUntil(frame, "choose a key", 24)
			click(&ctx.Router, Down) // Type a seed -> Leave unseated
			click(&ctx.Router, Button3)
		}
		if got, ok := pumpUntil(frame, "Unfilled:", 16); !ok {
			t.Fatalf("§8p never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		pumpUntil(frame, "What now?", 12)
		click(&ctx.Router, Button3) // Back to the paths
		if got, ok := pumpUntil(frame, "Spend paths", 12); !ok {
			t.Fatalf("no path list.\nLast frame: %q", got)
		}

		// BACK, then a PRESET: the route W-6's fix created.
		click(&ctx.Router, Button1)
		if got, ok := pumpUntil(frame, "Start from?", 12); !ok {
			t.Fatalf("Back did not reach the preset screen.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down, Down, Down, Down) // -> decaying-multisig
		click(&ctx.Router, Button3)

		got, ok := pumpUntil(frame, "CLEARS THE KEYS", 24)
		if !ok {
			t.Fatalf("a preset replaced a seated shape with no §8j. Under tr the preset "+
				"numbers the slots onto different paths than the hand-built shape did, "+
				"and §7d's enumeration cannot see it, so every seat was carried onto a "+
				"slot that now serves another path.\nLast frame: %q", got)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		if got, ok = pumpUntil(frame, "Spend paths", 24); !ok {
			t.Fatalf("the path list never came back.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down, Down, Down) // -> Done
		click(&ctx.Router, Button3)
		if _, ok = pumpUntil(frame, "Sorted keys, or your order?", 24); ok {
			click(&ctx.Router, Button3)
		}
		if got, ok = pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew after the preset.\nLast frame: %q", got)
		}
		if got, ok = composerPageUntil(t, ctx, frame, "Slot @0", 10); !ok {
			t.Fatalf("the stub screen never named slot @0.\nLast frame: %q", got)
		}
		if uiContains(got, "73c5da0a") {
			t.Errorf("a seat survived the preset replacement.\nFrame: %q", got)
		}
	})
}

// TestComposerLockEditUnderTrDiscardsTheSeatsItMoves is I-1: the same root
// cause on the path editor's lock arm, which was already open on 70008da.
//
// §7d's "a lock or hash edit moves no slot, keeps assignments" is TRUE under
// wsh and FALSE under tr, where clearing a lock can hand slot @0 to another
// path. TestComposerLockAndHashEditsAreNotGuardedByTheDiscardConfirm keeps the
// wsh half honest: the confirm must still not fire where nothing moves.
func TestComposerLockEditUnderTrDiscardsTheSeatsItMoves(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		// Under tr: path 2 is the bare single and owns @0. Clearing path 1's
		// lock makes path 1 the FIRST bare single, so @0 changes hands.
		st := &composerState{reg: &seedRegistry{}, list: md.PathList{
			Wrapper: md.ComposeTr,
			Paths: []md.SpendPath{
				{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
				{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
			},
		}}
		before := composerSlotsFor(t, st.list)
		composerSizeAssignments(st)
		st.assigned[0].src = 0
		st.sources = []composerSource{{kind: composerSourceKey, seedID: -1}}

		frame, quit := runUI(ctx, func() { composerPathEdit(ctx, &descriptorTheme, st, 0) })
		defer quit()

		pumpUntil(frame, "Path 1:", 16)
		click(&ctx.Router, Down) // -> Time lock
		click(&ctx.Router, Button3)

		got, ok := pumpUntil(frame, "CLEARS THE KEYS", 24)
		if !ok {
			t.Fatalf("§8j did not fire before a lock edit that CAN renumber under tr; "+
				"the arm is unguarded because §7d's premise (\"a lock or hash edit "+
				"moves no slot\") is false for this wrapper.\nLast frame: %q", got)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		if got, ok = pumpUntil(frame, "What kind of time lock?", 24); !ok {
			t.Fatalf("the lock editor never drew after the confirm.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // None: clears path 1's lock
		for range 8 {
			frame()
		}
		after := composerSlotsFor(t, st.list)
		if before[0].Path == after[0].Path {
			t.Fatalf("the fixture no longer renumbers: @0 serves path %d before and after",
				before[0].Path)
		}
		if st.assigned[0].src >= 0 {
			t.Errorf("slot @0 is still seated (src=%d) after a lock edit that moved it "+
				"from path %d to path %d: the key the operator seated now spends on a "+
				"path they never chose it for.",
				st.assigned[0].src, before[0].Path, after[0].Path)
		}
	})
}

// TestComposerBackAtTheWrapperPickerLeavesTheComposer pins the leg's sole exit
// (verification M-1: it was asserted by nothing).
//
// composerStartStep returning false is now the ONLY way out of the Back leg,
// and it must stay the way out: the wrapper picker is the composer's opening
// screen, where Back leaves, and a leg that could not exit would strand the
// operator in a flow with a built shape and no door.
func TestComposerBackAtTheWrapperPickerLeavesTheComposer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		done := make(chan struct{})
		frame, quit := runUI(ctx, func() {
			composerFlow(ctx, &descriptorTheme)
			close(done)
		})
		defer quit()

		pumpUntil(frame, "Which script?", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Start from?", 24)
		click(&ctx.Router, Button3) // Build my own paths
		pumpUntil(frame, "Add a spend path", 24)

		// Three Backs: the path list -> "Start from?" -> the wrapper -> out.
		click(&ctx.Router, Button1)
		pumpUntil(frame, "Start from?", 12)
		click(&ctx.Router, Button1)
		pumpUntil(frame, "Which script?", 12)
		click(&ctx.Router, Button1)
		for range 12 {
			frame()
		}
		select {
		case <-done:
		default:
			c, _ := frame()
			t.Fatalf("Back at the wrapper picker did not leave the composer; the flow is "+
				"still drawing, so the leg has no exit.\nFrame: %q", c)
		}
	})
}

// TestComposerEditCanRenumberIsExactOverEveryReachableShape is the fold
// verification's own census, committed as a gate.
//
// The probe is the whole condition on §8j for a lock or hash edit, so it is
// wrong in two directions and both are defects: a FALSE NEGATIVE lets
// composerApplyShapeEdit discard the operator's seating with no confirm and no
// chance to decline (r2 I-2, measured at 1,200 pairs), and a FALSE POSITIVE
// threatens every seat for an edit that clears none and leaves the field
// uneditable if the operator declines (r2 I-3, 288 pairs). One hand-picked
// case cannot see either; an enumeration can.
//
// THE ORACLE IS INDEPENDENT OF THE PROBE: it varies the field over the values
// the EDITOR can actually produce -- the lock screen's None plus two lock
// kinds, the hash screen's "No hash lock" plus a digest -- and asks whether
// ANY of them moves the signature away from where it is now. The probe
// compares two points; the oracle sweeps the arm's range.
func TestComposerEditCanRenumberIsExactOverEveryReachableShape(t *testing.T) {
	digestA, digestB := new([32]byte), new([32]byte)
	for i := range digestA {
		digestA[i] = 0xab
		digestB[i] = 0x7f
	}
	variants := []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 26280}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Hash: digestA},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Lock: &md.Lock{Kind: md.LockAfterHeight, Value: 1000000}, Hash: digestA},
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 13140}},
		{Hash: digestA}, // key-less: the shape I-2 was hiding in
	}
	// The values each arm can actually reach from its own screens.
	lockValues := []*md.Lock{
		nil,
		{Kind: md.LockOlderBlocks, Value: 26280},
		{Kind: md.LockAfterHeight, Value: 1000000},
	}
	hashValues := []*[32]byte{nil, digestA, digestB}

	var lists []md.PathList
	for _, w := range []md.ComposeWrapper{md.ComposeWsh, md.ComposeTr} {
		for i := range variants {
			lists = append(lists, md.PathList{Wrapper: w, Paths: []md.SpendPath{variants[i]}})
			for j := range variants {
				lists = append(lists, md.PathList{Wrapper: w, Paths: []md.SpendPath{variants[i], variants[j]}})
				for k := range variants {
					lists = append(lists, md.PathList{Wrapper: w, Paths: []md.SpendPath{
						variants[i], variants[j], variants[k],
					}})
				}
			}
		}
	}

	var checked, falseNeg, falsePos int
	for _, list := range lists {
		// Seats can only be held on a list the codec accepts, so that is the
		// reachable set.
		if _, err := md.Compose(list); err != nil {
			continue
		}
		now := composerShapeSignature(list)
		for idx := range list.Paths {
			for _, field := range []composerShapeField{composerFieldLock, composerFieldHash} {
				truth := false
				switch field {
				case composerFieldLock:
					for _, v := range lockValues {
						probe := composerListWithPaths(list)
						probe.Paths[idx].Lock = v
						if composerShapeSignature(probe) != now {
							truth = true
						}
					}
				case composerFieldHash:
					for _, v := range hashValues {
						probe := composerListWithPaths(list)
						probe.Paths[idx].Hash = v
						if composerShapeSignature(probe) != now {
							truth = true
						}
					}
				}
				got := composerEditCanRenumber(list, idx, field)
				checked++
				switch {
				case truth && !got:
					falseNeg++
					if falseNeg == 1 {
						t.Errorf("FALSE NEGATIVE: composerEditCanRenumber says a %v edit on "+
							"path %d cannot renumber, but an edit the screen offers moves the "+
							"signature. §8j is not asked and composerApplyShapeEdit then "+
							"discards every seat in silence (r2 I-2).\nwrapper=%v paths=%+v",
							field, idx, list.Wrapper, list.Paths)
					}
				case !truth && got:
					falsePos++
					if falsePos == 1 {
						t.Errorf("FALSE POSITIVE: composerEditCanRenumber says a %v edit on "+
							"path %d can renumber, but no value the screen offers moves the "+
							"signature. §8j threatens seats it will not clear, and declining "+
							"leaves the field uneditable (r2 I-3).\nwrapper=%v paths=%+v",
							field, idx, list.Wrapper, list.Paths)
					}
				}
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("the enumeration collapsed to %d cases; it is meant to sweep thousands, "+
			"so a shrinking corpus is the finding", checked)
	}
	t.Logf("checked %d (list, path, field) cases: %d false negatives, %d false positives",
		checked, falseNeg, falsePos)
	if falseNeg != 0 || falsePos != 0 {
		t.Errorf("the probe is not exact: %d false negatives, %d false positives over %d cases",
			falseNeg, falsePos, checked)
	}
}

// TestComposerHashEditOnAKeylessPathAsksBeforeItDiscards is r2 I-2, walked --
// and it is what pins the CALL SITE, which the census above cannot see: the
// census proves the probe is exact for each field, and this proves the hash
// arm passes its own field to it.
//
// Clearing the hash on a key-less path empties that path, the list stops
// composing, and composerApplyShapeEdit clears every seat. That discard is the
// safe direction, but §7d promises the operator is TOLD before it is accepted
// and can decline. The first version of the probe cleared the hash in both its
// variants, so it answered "no move" for exactly this shape and the operator's
// seating vanished without a screen.
func TestComposerHashEditOnAKeylessPathAsksBeforeItDiscards(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		digest := new([32]byte)
		for i := range digest {
			digest[i] = 0xab
		}
		st := &composerState{reg: &seedRegistry{}, list: md.PathList{
			Wrapper: md.ComposeWsh,
			Paths: []md.SpendPath{
				{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
				{Hash: digest}, // key-less, wsh-only (§4b)
			},
		}}
		composerSizeAssignments(st)
		st.assigned[0].src = 0
		st.assigned[1].src = 1
		st.sources = []composerSource{
			{kind: composerSourceKey, seedID: -1}, {kind: composerSourceKey, seedID: -1},
		}

		frame, quit := runUI(ctx, func() { composerPathEdit(ctx, &descriptorTheme, st, 1) })
		defer quit()

		pumpUntil(frame, "Path 2:", 16)
		click(&ctx.Router, Down, Down) // Keys -> Time lock -> Hash lock
		click(&ctx.Router, Button3)

		got, ok := pumpUntil(frame, "CLEARS THE KEYS", 24)
		if !ok {
			t.Fatalf("§8j was not asked before a hash edit that can empty this path and "+
				"discard every seat.\nLast frame: %q", got)
		}
		// DECLINE: the seating must survive, which is the whole point of being
		// asked.
		click(&ctx.Router, Button1)
		for range 8 {
			frame()
		}
		for i, a := range st.assigned {
			if a.src < 0 {
				t.Errorf("slot @%d lost its seat although the operator DECLINED the confirm; "+
					"a guard that discards whatever the answer is, is not a guard", i)
			}
		}
		if st.list.Paths[1].Hash == nil {
			t.Errorf("the hash was cleared although the operator declined the confirm")
		}
	})
}
