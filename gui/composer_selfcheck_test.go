package gui

import (
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

// composerSeatedFixture is a fully seated 2-of-3 wsh policy and the chunks it
// composes to: one composerState whose assignments agree with the artifact,
// which is the state the self-check must ACCEPT before any of its refusals
// mean anything.
func composerSeatedFixture(t *testing.T) (*composerState, []string) {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	st := &composerState{list: list, reg: &seedRegistry{}}
	st.sources = []composerSource{
		{kind: composerSourceKey, seedID: -1}, {kind: composerSourceKey, seedID: -1},
		{kind: composerSourceKey, seedID: -1},
	}
	declared := make([]*md.SlotOrigin, 3)
	st.assigned = make([]composerAssignment, 3)
	for i := range st.assigned {
		fp := [4]byte{0x73, 0xc5, 0xda, byte(i)}
		origin := composerTestOrigin(2, uint32(i))
		st.assigned[i] = composerAssignment{
			src: i, account: uint32(i), origin: origin,
			fingerprint: fp, fpPresent: true,
		}
		declared[i] = &md.SlotOrigin{Origin: origin, Fingerprint: fp, FpPresent: true}
	}
	c, err := md.ComposeWith(list, declared)
	if err != nil {
		t.Fatalf("md.ComposeWith: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	return st, chunks
}

// composerOtherWalletChunks is a DIFFERENT wallet's artifact, for the
// injection that swaps the whole chunk set.
func composerOtherWalletChunks(t *testing.T) []string {
	t.Helper()
	c, err := md.Compose(md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}},
	}})
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	return chunks
}

// TestComposerSelfCheckRefusesAFaultInjectedBuilderOutput is §12 item 4's
// last clause, and the one gate here that no input can reach.
//
// A GATE THAT HAS NEVER EXECUTED IS A HYPOTHESIS. The check exists so a
// builder defect in the shape, the seating, the origins, the fingerprints or
// the use-site cannot reach steel as a REVIEWED wallet, and the only way to
// run it is to break the builder's output on purpose.
func TestComposerSelfCheckRefusesAFaultInjectedBuilderOutput(t *testing.T) {
	st, chunks := composerSeatedFixture(t) // a 2-of-3 wsh, every slot seated
	if err := composerSelfCheck(st, chunks); err != nil {
		t.Fatalf("INCONCLUSIVE: the self-check refuses an HONEST build: %v -- every "+
			"assertion below would then pass for the wrong reason", err)
	}
	for _, tc := range []struct {
		name    string
		breakIt func(*composerState, []string) []string
	}{
		{"a slot's origin moves", func(st *composerState, c []string) []string {
			st.assigned[0].origin = composerTestOrigin(2, 31)
			return c
		}},
		{"a slot's fingerprint moves", func(st *composerState, c []string) []string {
			st.assigned[0].fingerprint = [4]byte{0xff, 0xff, 0xff, 0xff}
			return c
		}},
		{"the shape gains a path the chunks do not have", func(st *composerState, c []string) []string {
			st.list.Paths = append(st.list.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 1}})
			return c
		}},
		{"the chunks are another wallet's", func(st *composerState, c []string) []string {
			return composerOtherWalletChunks(t)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, chunks := composerSeatedFixture(t)
			got := tc.breakIt(st, chunks)
			if err := composerSelfCheck(st, got); err == nil {
				t.Errorf("the self-check ACCEPTED a build where %s; §8q's refusal would "+
					"never fire and a wrong wallet would reach steel as reviewed", tc.name)
			}
		})
	}
	assertModalBodyFits(t, "the §8q self-check refusal", errorScreenBody, composerCopySelfCheckFailed())
	assertModalBodyFits(t, "the §8l unchecked-policy warning", confirmWarningBody,
		composerConfirmBody(composerCopyNothingChecked()))
}

// TestComposerConsentRefusesThroughTheHookAndSaysSection8q drives the SCREEN,
// so the refusal is proven to reach the operator and not merely to be
// returned by a function.
func TestComposerConsentRefusesThroughTheHookAndSaysSection8q(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st, chunks := composerSeatedFixture(t)
		composerSelfCheckFaultHook = func(c []string) []string { return composerOtherWalletChunks(t) }
		defer func() { composerSelfCheckFaultHook = nil }()
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerConsentFlow(ctx, &descriptorTheme, st, chunks)
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the consent flow drew nothing")
		}
		assertFrameHasBody(t, ink(), "the §8q self-check refusal")
		if !uiContains(content, "does not match what you built") {
			t.Errorf("the refusal does not say §8q's words.\nFrame: %q", content)
		}
		if !uiContains(content, "start again") {
			t.Errorf("the refusal does not give the operator an exit.\nFrame: %q", content)
		}
	})
}

// TestComposerSelfCheckFaultHookIsNilInProduction: the seam must not be able
// to weaken the gate on a shipped device.
func TestComposerSelfCheckFaultHookIsNilInProduction(t *testing.T) {
	if composerSelfCheckFaultHook != nil {
		t.Error("composerSelfCheckFaultHook is non-nil at rest")
	}
}
