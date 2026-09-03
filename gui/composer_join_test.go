package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/md"
)

// ═══ THE JOIN ═══════════════════════════════════════════════════════════════
//
// Two R0 lenses found the same Critical independently: Part B built fourteen
// production functions and nothing called any of them. Go does not error on an
// unused package-scope function, and every other TestComposer* calls these
// directly, so the suite was green over a feature no operator could reach.
//
// The two tests below are the gate against that class. The first is
// STRUCTURAL and cheap; the second is a WALK, and only a walk can fail for the
// right reason.

// TestComposerEveryScreenFunctionHasAProductionCaller is the cheap half.
//
// It parses gui/*.go (non-test), counts call expressions per function name,
// and requires every composer*Flow / composer*Pick / composer*Step / the
// named screen entry points to be CALLED somewhere other than their own
// declaration. "An unreachable feature" is not something the build gate can
// see, and this is what puts it inside a command.
func TestComposerEveryScreenFunctionHasAProductionCaller(t *testing.T) {
	fset := token.NewFileSet()
	declared := map[string]bool{}
	called := map[string]bool{}
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "composer") {
				declared[fn.Name.Name] = true
			}
			// EVERY body is scanned for calls, not just the composer ones:
			// composerDoorFlow and composerFlow are called from
			// walletPolicyFlow, which is not a composer* name, and a scan
			// restricted to composer* bodies reported the two entry points of
			// the feature as unreachable -- a false positive that would have
			// taught the next reader to distrust this test.
			if fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name != fn.Name.Name {
					called[id.Name] = true
				}
				return true
			})
		}
	}
	if len(declared) < 40 {
		t.Fatalf("INCONCLUSIVE: only %d composer* declarations found; the scan is broken, "+
			"not the tree", len(declared))
	}
	// THE EXEMPTIONS ARE NAMED AND ARGUED, never a silent skip: a test that
	// can be quieted by adding a name is not a gate.
	exempt := map[string]string{
		"composerDescriptorPlateFits":    "§13 item 1's measurement; its consumer, the concrete text/QR descriptor plate, is deferred to F-457 because md deliberately emits no text",
		"composerDescriptorCeilingChars": "the same measurement, called by TestComposerMeasureSection13Numbers",
	}
	var orphans []string
	for name := range declared {
		if called[name] {
			continue
		}
		if why, ok := exempt[name]; ok {
			t.Logf("exempt: %s -- %s", name, why)
			continue
		}
		orphans = append(orphans, name)
	}
	sort.Strings(orphans)
	if len(orphans) != 0 {
		t.Errorf("these composer functions have no production caller, so the screens they "+
			"draw cannot be reached by any operator: %v\n"+
			"This is the defect two R0 lenses found: fourteen of them at once, with a "+
			"green suite, because every test called them directly.", orphans)
	}
}

// TestComposerWalkFromAKeyedPayloadReachesTheEngraveScreen is the flow-level
// walk C-1 asks for: door → Build → wrapper → shape → stub → seating →
// mapping review → consent (self-check) → §8l → form choice → census.
//
// IT IS THE ONLY KIND OF TEST THAT CAN FAIL ON THE JOIN. Every screen below
// has its own unit test and all of them passed while the flow reached none of
// them.
func TestComposerWalkFromAKeyedPayloadReachesTheEngraveScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		// TWO key: records, at two accounts of one master -- C5's normal case,
		// and the shape §4f's invariant permits (distinct origins).
		ctx.sysw = composerSessionWith([]string{
			composerTestKeyRecord, composerTestKeyRecord2, composerTestNowRecord,
		}, nil)

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		// (1) THE DOOR, with §8r's key count for the payload actually loaded.
		got, ok := pumpUntil(frame, "Build a new policy", 24)
		if !ok {
			t.Fatalf("the door never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "Keys loaded: 2") {
			t.Errorf("the door does not count the payload's keys.\nFrame: %q", got)
		}
		click(&ctx.Router, Down, Down) // Scan cards -> From payload -> Build
		click(&ctx.Router, Button3)

		// (2) THE WRAPPER.
		if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // Taproot -> Segwit (wsh)
		click(&ctx.Router, Button3)

		// (2a) THE PRESET PICKER (§4d, task A10), which sits between the
		// wrapper and the path list on the FIRST pass only. Back is §7b's
		// BLANK route; this walk composes its own shape, so it declines.
		if got, ok = pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("the preset picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1)

		// (3) THE PATH LIST, whose live line must read the sources the FLOW
		// loaded -- the half that read `keys available: 0` for every payload.
		if got, ok = pumpUntil(frame, "Add a spend path", 24); !ok {
			t.Fatalf("the path list never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "keys available: 2") {
			t.Errorf("the path list does not count the payload's keys; §7b's live line "+
				"reads st.sources, which only the flow can populate.\nFrame: %q", got)
		}
		click(&ctx.Router, Button3) // an empty list opens on "Add a spend path"

		// (4) KEYS: 2 of 2.
		if got, ok = pumpUntil(frame, "What can spend on this path?", 24); !ok {
			t.Fatalf("the path kind picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Keys
		if got, ok = pumpUntil(frame, "how many keys?", 24); !ok {
			t.Fatalf("the key-count picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // 1 -> 2
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "how many must sign?", 24); !ok {
			t.Fatalf("the threshold picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down) // 1 -> 2
		click(&ctx.Router, Button3)

		// (5) DONE -> the key-order question, asked where `sole` is FINAL.
		if got, ok = pumpUntil(frame, "Path 1: 2-of-2", 24); !ok {
			t.Fatalf("the path list does not show the new path.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down) // path 1 -> Add -> Change script -> Done
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Sorted keys, or your order?", 24); !ok {
			t.Fatalf("the key-order question is not asked at the transition.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Sorted (usual)

		// (6) THE STUB SCREEN, paged: the checkmark is withheld until the last
		// page has been laid out once, so this pages to the end.
		if got, ok = pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// (7) SEATING, slot by slot, from the payload's own records.
		if got, ok = pumpUntil(frame, "choose a key", 32); !ok {
			t.Fatalf("seating never drew -- this is the join: the payload's keys are "+
				"loaded and no slot was ever offered.\nLast frame: %q", got)
		}
		if !uiContains(got, "Slot @0") {
			t.Errorf("the first seating prompt does not name slot @0.\nFrame: %q", got)
		}
		click(&ctx.Router, Button3) // the first remaining source
		if got, ok = pumpUntil(frame, "Slot @1", 32); !ok {
			t.Fatalf("the second slot was never offered.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)

		// (8) THE MAPPING REVIEW.
		if got, ok = pumpUntil(frame, "cannot confirm", 32); !ok {
			t.Fatalf("the mapping review never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// (9) THE STUB SCREEN AGAIN, now with the keyed policy's id, then
		//     CONSENT -- whose self-check must pass on an honest build.
		if got, ok = pumpUntil(frame, "mk1 stub (policy)", 32); !ok {
			t.Fatalf("the keyed stub screen never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// PAGE LOOKING FOR THE ADDRESSES. They are §7e's proof of WHICH wallet
		// this is, and they are several pages in on a policy with two paths
		// and four address lines -- which is exactly why the checkmark is
		// withheld until the last page has been laid out.
		if got, ok = composerPageUntil(t, ctx, frame, "Receive 0", 12); !ok {
			t.Fatalf("the consent surface never drew its addresses -- §8q would have "+
				"fired instead if the self-check refused.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// (10) §8l, unskippable, hold to confirm.
		if got, ok = pumpUntil(frame, "Nothing outside this device", 48); !ok {
			t.Fatalf("§8l never drew.\nLast frame: %q", got)
		}
		// HOLD, then let the fake clock run out the delay: the shipped walk
		// does exactly this (gui/multisig_build_walk_test.go:317-320), and a
		// press with no sleep leaves the confirm at partial progress forever.
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		// (11) THE FORM CHOICE (§7f), offered because every slot is seated.
		if got, ok = pumpUntil(frame, "Which form?", 48); !ok {
			t.Fatalf("the engrave form choice never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "Template plus key cards") {
			t.Errorf("form B is not offered on a fully seated composition.\nFrame: %q", got)
		}
		click(&ctx.Router, Down) // The policy itself -> Template plus key cards
		click(&ctx.Router, Button3)

		// (12) THE CENSUS, counting the md1 AND the minted key cards.
		if got, ok = pumpUntil(frame, "This engraves", 64); !ok {
			t.Fatalf("the plate census never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "mk1 key @0") {
			t.Errorf("the census does not count the minted key cards; §7f says it counts "+
				"CARD chunks too.\nFrame: %q", got)
		}
		// The BCH-versus-checksum line is on a later page of the census, which
		// is the shipped confirmReviewScreen and pages as it always did.
		if got, ok = composerPageUntil(t, ctx, frame, "error correction", 8); !ok {
			t.Errorf("the census never says how recovery detects an error.\nLast frame: %q", got)
		}
	})
}

// TestComposerBackAtTheMappingReviewKeepsTheSeatedKeys is C-1's regression, and
// it is a WALK because the defect is a state, not a value.
//
// composerSeatFlow marked each consumed key:/mk1 source `used` and re-asked
// EVERY slot from index 0 whenever seating was re-entered, while the pick list
// filters `used` sources out. So the second pass offered
// ["Type a seed", "Leave unseated"] and nothing else: the operator's own two
// keys were unreachable, and §7d's "Back keeps assignments" -- stated twice --
// was unmet. The assignments survived in st.assigned and were then overwritten
// by a re-ask that could not offer them back.
//
// THE ASSERTION IS THAT THE SECOND PASS STILL NAMES A FINGERPRINT. A frame
// holding only "Type a seed" and "Leave unseated" is the defect exactly.
func TestComposerBackAtTheMappingReviewKeepsTheSeatedKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{
			composerTestKeyRecord, composerTestKeyRecord2,
		}, nil)

		frame, quit := runUI(ctx, func() { composerFlow(ctx, &descriptorTheme) })
		defer quit()

		// Wrapper -> wsh, decline the preset, one 2-of-2 path, Done, Sorted.
		if got, ok := pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "Start from?", 24); !ok {
			t.Fatalf("the preset picker never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1)
		if got, ok := pumpUntil(frame, "Add a spend path", 24); !ok {
			t.Fatalf("the path list never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		pumpUntil(frame, "What can spend on this path?", 24)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many keys?", 24)
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		pumpUntil(frame, "how many must sign?", 24)
		click(&ctx.Router, Down)
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, "Path 1: 2-of-2", 24); !ok {
			t.Fatalf("the path was never added.\nLast frame: %q", got)
		}
		click(&ctx.Router, Down, Down, Down) // -> Done
		click(&ctx.Router, Button3)
		pumpUntil(frame, "Sorted keys, or your order?", 24)
		click(&ctx.Router, Button3)

		// The stub screen, paged to the end so the checkmark is offered.
		if got, ok := pumpUntil(frame, "mk1 stub (template)", 32); !ok {
			t.Fatalf("the stub screen never drew.\nLast frame: %q", got)
		}
		composerPageToEnd(t, ctx, frame)

		// PASS 1: seat both slots from the payload's own key: records.
		got, ok := pumpUntil(frame, "choose a key", 32)
		if !ok {
			t.Fatalf("seating never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "73c5da0a") {
			t.Fatalf("INCONCLUSIVE: the FIRST pass does not offer the payload's keys, so "+
				"this test cannot tell a lost seat from a payload that never had "+
				"one.\nFrame: %q", got)
		}
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "Slot @1", 32); !ok {
			t.Fatalf("the second slot was never offered.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)

		// The mapping review, then BACK out of it.
		if got, ok = pumpUntil(frame, "Key mapping", 32); !ok {
			t.Fatalf("the mapping review never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button1) // Back

		// PASS 2: whatever screen the Back lands on, the operator's own keys
		// must still be reachable. Before the fix this frame read
		// "Slot @0 ... Type a seed Leave unseated" and nothing else.
		if got, ok = pumpUntil(frame, "choose a key", 32); !ok {
			t.Fatalf("Back at the mapping review did not land on a seating screen.\n"+
				"Last frame: %q", got)
		}
		// IT LANDS ON THE LAST SEATED SLOT, @1 -- not back at @0. This is the
		// half that tells RESUMING from RE-ASKING: with the re-ask, seating
		// restarts at slot @0, and because the Back released only @1 the frame
		// would still name a fingerprint while the operator is a screen further
		// back than they were. So the slot number is the assertion, not just
		// the presence of a key.
		if !uiContains(got, "Slot @1") {
			t.Errorf("Back at the mapping review did not land on the last seated slot: "+
				"seating re-asked from slot @0 instead of resuming at @1, so every "+
				"earlier assignment is overwritten by a pass that cannot offer the "+
				"sources those assignments still hold (SPEC §7d).\nFrame: %q", got)
		}
		if !uiContains(got, "73c5da0a") {
			t.Errorf("after a Back at the mapping review the payload's own keys are no "+
				"longer offered: every source is still marked used and the slot was "+
				"re-asked from scratch, so the policy the operator just reviewed "+
				"cannot be rebuilt (SPEC §7d, \"Back keeps assignments\").\nFrame: %q", got)
		}
	})
}

// TestComposerSeatingReleasesASourceWhenItsAssignmentIsDropped is C-1's unit
// half: a released assignment must release the source it held, or the source
// is filtered out of every later pick list while seating nothing.
func TestComposerSeatingReleasesASourceWhenItsAssignmentIsDropped(t *testing.T) {
	st := &composerState{}
	st.list = md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
	}}
	st.sources = []composerSource{
		{kind: composerSourceKey, label: "a", fingerprint: [4]byte{1}, fpPresent: true, used: true},
		{kind: composerSourceKey, label: "b", fingerprint: [4]byte{2}, fpPresent: true, used: true},
	}
	composerSizeAssignments(st)
	st.assigned[0] = composerAssignment{src: 0}
	st.assigned[1] = composerAssignment{src: 1}

	if !composerReleaseSeat(st, 1) {
		t.Fatal("releasing a seated slot reported nothing to release")
	}
	if st.assigned[1].src != -1 {
		t.Errorf("slot @1 still holds source %d after release", st.assigned[1].src)
	}
	if st.sources[1].used {
		t.Error("source 1 is still marked used after its only assignment was released, " +
			"so no later pick list will offer it while nothing holds it")
	}
	if !st.sources[0].used {
		t.Error("releasing slot @1 released slot @0's source too")
	}
	if composerReleaseSeat(st, 1) {
		t.Error("releasing an already-unseated slot reported that it released something")
	}
}

// TestComposerMoveUpDiscardsUnconditionally is I-1.
//
// composerShapeSignature carries the wrapper, the path count and each path's
// key count -- §7d's own list -- so swapping two paths with EQUAL key counts
// left it identical and composerApplyShapeEdit discarded nothing, AFTER §8j had
// already told the operator "Every key you seated will be cleared". §5 numbers
// slots by first appearance in listed order, so the retained assignments then
// denoted different spend paths, and composerSelfCheck agreed because st.list
// moved with them.
//
// Move up now discards unconditionally, which is what §8j promised.
func TestComposerMoveUpDiscardsUnconditionally(t *testing.T) {
	st := &composerState{}
	st.list = md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 3, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
	}}
	st.sources = []composerSource{
		{kind: composerSourceKey, label: "a", used: true},
		{kind: composerSourceKey, label: "b", used: true},
	}
	composerSizeAssignments(st)
	for i := range st.assigned {
		st.assigned[i] = composerAssignment{src: i % 2}
	}

	before := composerShapeSignature(st.list)
	discarded := composerMoveUp(st, 1)
	if after := composerShapeSignature(st.list); after != before {
		t.Fatalf("INCONCLUSIVE: this fixture's swap MOVED the signature (%q -> %q), so it "+
			"cannot show that an equal-key-count reorder discards", before, after)
	}
	if !discarded {
		t.Error("Move up discarded nothing on an equal-key-count swap, after §8j had " +
			"already promised the operator every seated key would be cleared")
	}
	for i, a := range st.assigned {
		if a.src != -1 {
			t.Errorf("slot @%d still holds source %d after a Move up", i, a.src)
		}
	}
	for i, s := range st.sources {
		if s.used {
			t.Errorf("source %d is still marked used after a Move up discarded every seat", i)
		}
	}
}
