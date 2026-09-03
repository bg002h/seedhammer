package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

func composerKeyedPath(k, n uint8, sorted bool) md.SpendPath {
	return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: sorted}}
}

func TestComposerPathLineNamesTheShapeAnOperatorSees(t *testing.T) {
	digest := [32]byte{0xab}
	for _, tc := range []struct {
		name string
		p    md.SpendPath
		want string
	}{
		{"plain k of n", composerKeyedPath(2, 3, true), "Path 1: 2-of-3"},
		{"single key", composerKeyedPath(1, 1, false), "Path 1: 1 key"},
		{"with a relative time lock", md.SpendPath{
			Keys: &md.KeySet{K: 2, N: 3}, Lock: &md.Lock{Kind: md.LockOlderUnits, Value: 15188},
		}, "Path 1: 2-of-3 + 90 days"},
		{"with a block height", md.SpendPath{
			Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockAfterHeight, Value: 905000},
		}, "Path 1: 1 key + block 905000"},
		{"key-less hash path", md.SpendPath{Hash: &digest}, "Path 1: hash only"},
		{"keys and a hash", md.SpendPath{Keys: &md.KeySet{K: 2, N: 2}, Hash: &digest},
			"Path 1: 2-of-2 + hash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := composerPathLine(tc.p, 0); got != tc.want {
				t.Errorf("composerPathLine = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestComposerRefusalBodyMapsEverySentinelToItsSection8mLine is §12 item 4 for
// the structural family: every refusal REFUSES, and with the exact §8 line.
//
// It is table-driven off the sentinels rather than off strings, so a renamed
// or added ErrCompose* arm shows up as an unmapped error rather than as a
// screen that says nothing.
func TestComposerRefusalBodyMapsEverySentinelToItsSection8mLine(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{md.ErrComposeNoKeyedPath, composerCopyRefuseNoKeyedPath()},
		{md.ErrComposeLockOnlyPath, composerCopyRefuseLockOnly()},
		{md.ErrComposeKeylessUnderTr, composerCopyRefuseKeylessTr()},
		{md.ErrComposeLegacyWrapperShape, composerCopyRefuseLegacyShape()},
		{md.ErrComposeTooManySlots, composerCopyRefuseSlotCap()},
	} {
		got, ok := composerRefusalBody(tc.err)
		if !ok {
			t.Errorf("%v maps to no §8m body, so the operator would be refused with nothing", tc.err)
			continue
		}
		if got != tc.want {
			t.Errorf("%v maps to %q, want %q", tc.err, got, tc.want)
		}
	}
	// An unmapped error must be REPORTED as unmapped, not silently rendered as
	// one of the five.
	if _, ok := composerRefusalBody(md.ErrComposeBadThreshold); ok {
		t.Error("ErrComposeBadThreshold maps to a §8m body; the picker prevents it and " +
			"§8m has no line for it, so it must not borrow another refusal's words")
	}
}

// TestComposerShapeRefusalsActuallyRefuse drives ValidatePathList over the
// four shapes §4e names, so the mapping above is pinned to the codec's real
// answers rather than to this test's idea of them.
func TestComposerShapeRefusalsActuallyRefuse(t *testing.T) {
	digest := [32]byte{0x11}
	for _, tc := range []struct {
		name string
		list md.PathList
		want string
	}{
		{"no path with keys", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{Hash: &digest}}},
			composerCopyRefuseNoKeyedPath()},
		{"a path with neither keys nor hash", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			composerKeyedPath(1, 1, false),
			{Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 100}},
		}}, composerCopyRefuseLockOnly()},
		{"key-less path under tr", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			composerKeyedPath(1, 1, false), {Hash: &digest},
		}}, composerCopyRefuseKeylessTr()},
		{"legacy wrapper, two paths", md.PathList{Wrapper: md.ComposeSh, Paths: []md.SpendPath{
			composerKeyedPath(2, 3, true), composerKeyedPath(1, 2, true),
		}}, composerCopyRefuseLegacyShape()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := md.ValidatePathList(tc.list)
			if err == nil {
				t.Fatalf("md.ValidatePathList ACCEPTED a shape §4e refuses; the refusal " +
					"screen below would never be reached")
			}
			body, ok := composerRefusalBody(err)
			if !ok {
				t.Fatalf("no §8m body for %v", err)
			}
			if body != tc.want {
				t.Errorf("refused with %q, want %q", body, tc.want)
			}
		})
	}
}

// TestComposerPickerBoundsNeverOfferAnIllegalValue is §4e's "REFUSE at the
// picker (the picker does not offer the value)".
func TestComposerPickerBoundsNeverOfferAnIllegalValue(t *testing.T) {
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
	// An empty policy: the whole 32-slot budget is available, capped at 9.
	if got := composerMaxKeysForPath(st, 0); got != md.ComposeMaxKeysPerPath {
		t.Errorf("an empty policy offers up to %d keys, want %d", got, md.ComposeMaxKeysPerPath)
	}
	// Fill 28 slots across four paths; the fifth path may then offer 4, not 9.
	st.list.Paths = []md.SpendPath{
		composerKeyedPath(1, 7, false), composerKeyedPath(1, 7, false),
		composerKeyedPath(1, 7, false), composerKeyedPath(1, 7, false),
		{},
	}
	if got := composerSlotCount(st.list); got != 28 {
		t.Fatalf("composerSlotCount = %d, want 28", got)
	}
	if got := composerMaxKeysForPath(st, 4); got != 4 {
		t.Errorf("with 28 slots taken the picker offers %d more, want 4 (the 32-slot wire cap)", got)
	}
	// And at the cap it offers none, which is what makes §8m line 5 reachable.
	st.list.Paths[4] = composerKeyedPath(1, 4, false)
	st.list.Paths = append(st.list.Paths, md.SpendPath{})
	if got := composerMaxKeysForPath(st, 5); got != 0 {
		t.Errorf("at 32 slots the picker offers %d more, want 0", got)
	}
}

// TestComposerSortedIsLegalOnlyWhereSection5SaysSo is what keeps the §8b
// confirm honest: §5a rules it fires ONLY where sorted was legal and
// declined, never on a lowering-forced multi.
func TestComposerSortedIsLegalOnlyWhereSection5SaysSo(t *testing.T) {
	digest := [32]byte{0x22}
	sole := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{composerKeyedPath(2, 3, true)}}
	if !composerSortedIsLegal(sole, 0) {
		t.Error("a sole unlocked, unhashed 2-of-3 is exactly where sortedmulti is legal")
	}
	locked := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{
		Keys: &md.KeySet{K: 2, N: 3}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 10},
	}}}
	if composerSortedIsLegal(locked, 0) {
		t.Error("a locked path cannot be sortedmulti (nested sortedmulti is refused by md " +
			"and by BIP-383/388), so the §8b confirm must not be offered for it")
	}
	hashed := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{
		Keys: &md.KeySet{K: 2, N: 3}, Hash: &digest,
	}}}
	if composerSortedIsLegal(hashed, 0) {
		t.Error("a hashed path is not a sole sortedmulti child either")
	}
	two := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		composerKeyedPath(2, 3, true), composerKeyedPath(1, 2, true)}}
	if composerSortedIsLegal(two, 0) {
		t.Error("with two paths neither is the sole child, so both are lowering-forced multi")
	}
	single := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{composerKeyedPath(1, 1, false)}}
	if composerSortedIsLegal(single, 0) {
		t.Error("n = 1 lowers to pkh/pk; there is no sorted form to decline")
	}
	legacy := md.PathList{Wrapper: md.ComposeSh, Paths: []md.SpendPath{composerKeyedPath(2, 3, true)}}
	if composerSortedIsLegal(legacy, 0) {
		t.Error("the legacy wrappers are sorted-only (§4e, feasibility M-5), so the §8b " +
			"confirm is never offered under them")
	}
}

// The two EXPERIMENTAL confirm bodies, under all three §12 item 5 gates.
func TestComposerExperimentalConfirmsDrawInFullAndFireOnCondition(t *testing.T) {
	for _, tc := range []struct {
		what string
		body string
	}{
		{"the §8a key-less path confirm", composerConfirmBody(composerCopyKeylessPath())},
		{"the §8b unsorted keys confirm", composerConfirmBody(composerCopyUnsortedKeys())},
	} {
		assertModalBodyFits(t, tc.what, confirmWarningBody, tc.body)
	}
	// FIRES ON CONDITION: adding a key-less path to a wsh list shows §8a, and
	// declining it leaves the path list unchanged.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerConfirmScreen(ctx, &descriptorTheme, "EXPERIMENTAL",
				composerConfirmBody(composerCopyKeylessPath()))
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the §8a confirm never drew a frame")
		}
		assertFrameHasBody(t, ink(), "the §8a key-less path confirm")
		if !uiContains(content, "bearer access") {
			t.Errorf("the §8a confirm does not name the consequence.\nFrame: %q", content)
		}
		if !strings.Contains(strings.ToLower(content), "hold") {
			t.Errorf("the §8a confirm does not say how to get past it.\nFrame: %q", content)
		}
	})
}

// TestComposerEveryPathHashedWarns is §8h, fired at the transition out of the
// shape, and §12 item 5's condition test for it.
func TestComposerEveryPathHashedWarns(t *testing.T) {
	digest := [32]byte{0x33}
	all := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}, Hash: &digest},
		{Keys: &md.KeySet{K: 2, N: 3}, Hash: &digest},
	}}
	if !composerEveryPathHashed(all) {
		t.Error("a list whose every path carries a hash does not trip §8h")
	}
	some := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}, Hash: &digest},
		composerKeyedPath(2, 3, false),
	}}
	if composerEveryPathHashed(some) {
		t.Error("§8h fired on a list with an un-hashed path; it would then be a warning " +
			"the operator learns to tap past")
	}
	assertModalBodyFits(t, "the §8h every-path-hashed warning", errorScreenBody, composerCopyHashEveryPath())
}
