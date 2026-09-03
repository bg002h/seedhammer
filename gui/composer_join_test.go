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
