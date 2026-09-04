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
