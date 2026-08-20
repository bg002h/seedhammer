package gui

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"
)

// ─── D-4: the Build-policy cosigner gather is titled "Engrave Bundle" ────────
//
// Observed in the emulator 2026-08-13 (SPEC §2.2 D-4). The title comes from
// layoutTitle INSIDE the shared gatherer, so it reads "Engrave Bundle" whichever
// of the five callers is on screen. On the Build path that is a screen claiming
// to be a different program, in the middle of authoring a wallet policy.
//
// It is also why the walk driver could not identify this screen: measured
// 2026-08-14 by driving both flows and squashing shScreen(), the Build-policy
// gather and the Engrave Bundle gather were identical character for character.

// TestBuildGatherIsNotTitledEngraveBundle drives the Build path to the cosigner
// gather and reads the title off the screen.
func TestBuildGatherIsNotTitledEngraveBundle(t *testing.T) {
	records := cosignerCardRecords(t, 1)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkParamPickers(t, ctx, frame)
		content, ok := pumpUntil(frame, "mk1 keys: 1", 32)
		if !ok {
			t.Fatalf("the Build-policy cosigner gather was not reached; got %q", content)
		}
		if uiContains(content, "Engrave Bundle") {
			t.Errorf("the Build-policy cosigner gather is titled \"Engrave Bundle\" — "+
				"a screen naming a different program, mid-flow (D-4): %q", content)
		}
		if !uiContains(content, buildCosignerGatherTitle) {
			t.Errorf("the gather does not carry its own title %q: %q",
				buildCosignerGatherTitle, content)
		}
	})
}

// TestSuppliedGatherKeepsItsTitle is the other half, and it is why the fix is a
// parameter rather than a rename: four flows share this gatherer and none of
// them is authoring a policy. "Engrave Bundle" is CORRECT for bundleFlow, and a
// stage that renamed the shared default would have fixed one screen by breaking
// four.
func TestSuppliedGatherKeepsItsTitle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()
		content, ok := pumpUntil(frame, "mk1 keys: 0", 32)
		if !ok {
			t.Fatalf("the Engrave Bundle gather was not reached; got %q", content)
		}
		if !uiContains(content, "Engrave Bundle") {
			t.Errorf("bundleFlow's own gather lost its title: %q", content)
		}
	})
}

// TestGatherTitleReachesTheRefusalsToo: the two "Done" refusals inside the
// gatherer are drawn with the SAME title, and one of them is reachable from
// Build (a payload holding enough complete cards PLUS a half chunk set). A
// refusal titled for the wrong program is the defect D-4 names, one screen
// deeper, where an operator is already confused.
func TestGatherTitleReachesTheRefusalsToo(t *testing.T) {
	// TARGETS THE RESUME FORM, where the body now lives (2026-08-19): the
	// gather was split so a Back at the review can return to it WITH the cards
	// still on the pile, leaving bundleGatherFlow a thin delegating wrapper.
	// Checking the wrapper would have passed vacuously — which the anti-vacuity
	// guard below caught the moment the split landed.
	body := funcBody(t, "bundle_flow.go", "func bundleGatherFlowResume(")
	if strings.Contains(body, `"Engrave Bundle"`) {
		t.Error("bundleGatherFlowResume still hard-codes \"Engrave Bundle\" somewhere " +
			"in its body, so on the Build path it names the wrong program")
	}
	// Non-vacuous: the slice must actually contain the function.
	if !strings.Contains(body, "bundleDoneDecision") {
		t.Fatalf("funcBody did not capture bundleGatherFlowResume; got %d bytes", len(body))
	}
	// The wrapper must not smuggle a hard-coded title back in either.
	wrapper := funcBody(t, "bundle_flow.go", "func bundleGatherFlow(")
	if strings.Contains(wrapper, `"Engrave Bundle"`) {
		t.Error("bundleGatherFlow's wrapper hard-codes \"Engrave Bundle\"")
	}
	if !strings.Contains(wrapper, "bundleGatherFlowResume") {
		t.Fatalf("bundleGatherFlow no longer delegates; got %d bytes", len(wrapper))
	}
}

// funcBody returns the source of the named top-level func in a gui file, from
// its signature to the next top-level `func`. Blunt on purpose (see
// cmd/emu/needle_test.go's productionSites for the same reasoning): a
// concatenated or fmt.Sprintf'd title is one this must still see.
func funcBody(t *testing.T, file, sig string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	src := string(b)
	i := strings.Index(src, sig)
	if i < 0 {
		t.Fatalf("%s does not contain %q — this guard protects nothing", file, sig)
	}
	rest := src[i+len(sig):]
	if j := strings.Index(rest, "\nfunc "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}
