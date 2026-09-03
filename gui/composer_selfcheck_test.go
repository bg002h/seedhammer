package gui

import (
	"os"
	"strings"
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
	// `want`, where set, is a substring of the arm's OWN refusal, so a row
	// cannot pass by tripping a different assertion earlier in the check --
	// which is how a coverage table comes to report ten arms covered while
	// four do the work. The first four rows predate review r0 and assert only
	// that the build is refused; the six added for r0 I-2 name their arm.
	//
	// `fixture` is the honest build the row breaks. Most rows use the seated
	// 2-of-3, but the lock and digest arms need a shape that HAS a lock and a
	// digest (perturbing a nil one trips the COUNT arm above them instead),
	// and §4f's arm needs a chunk set the composer's own builder refuses to
	// emit -- see composerCollidingOriginChunks.
	for _, tc := range []struct {
		name    string
		fixture func(*testing.T) (*composerState, []string)
		breakIt func(*composerState, []string) []string
		want    string
	}{
		{"a slot's origin moves", composerSeatedFixture, func(st *composerState, c []string) []string {
			st.assigned[0].origin = composerTestOrigin(2, 31)
			return c
		}, ""},
		{"a slot's fingerprint moves", composerSeatedFixture, func(st *composerState, c []string) []string {
			st.assigned[0].fingerprint = [4]byte{0xff, 0xff, 0xff, 0xff}
			return c
		}, ""},
		{"the shape gains a path the chunks do not have", composerSeatedFixture, func(st *composerState, c []string) []string {
			st.list.Paths = append(st.list.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 1}})
			return c
		}, ""},
		{"the chunks are another wallet's", composerSeatedFixture, func(st *composerState, c []string) []string {
			return composerOtherWalletChunks(t)
		}, ""},

		// ─── review r0 I-2: one row per arm that survived `if false {` ───────

		// §7c names the lock operand as an input to the template id, so a
		// builder that moved it would change the wallet while the shape screen
		// still read the operator's own number.
		{"a path's lock VALUE moves", composerLockedDigestFixture, func(st *composerState, c []string) []string {
			st.list.Paths[0].Lock.Value++
			return c
		}, "path 1's lock is"},

		// The digest is the other §7c template-id input, and the one an
		// operator cannot check by eye.
		{"a path's sha256 digest moves", composerLockedDigestFixture, func(st *composerState, c []string) []string {
			d := *st.list.Paths[1].Hash
			d[0] ^= 0xff
			st.list.Paths[1].Hash = &d
			return c
		}, "path 2's digest differs from the shape's"},

		// A fingerprint on a slot the operator never seated is a key nobody
		// chose, declared on a plate that says the slot is open.
		{"an UNSEATED slot declares a fingerprint", composerSeatedFixture, func(st *composerState, c []string) []string {
			st.assigned[2].src = -1
			return c
		}, "unseated slot @2 declares a fingerprint"},

		// Presence, not value: a slot whose fingerprint is elided seats a card
		// by origin alone (gui/key_card_seating.go:151-159), so presence is
		// itself a seating rule and not a cosmetic field.
		{"a seated slot's fingerprint PRESENCE differs", composerSeatedFixture, func(st *composerState, c []string) []string {
			st.assigned[0].fpPresent = false
			return c
		}, "slot @0's fingerprint presence differs from the mapping review"},

		// §4f's invariant ON THE DECODED md1, which is the only place it is
		// checked for a key-less or partially seated template --
		// composerInvariantViolation reads composer UI state and deliberately
		// skips unseated slots.
		{"the decoded md1 puts two slots at ONE origin with no fingerprints",
			composerCollidingOriginFixture, func(st *composerState, c []string) []string {
				return c
			}, "without two distinct fingerprints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, chunks := tc.fixture(t)
			if tc.want == "" {
				if err := composerSelfCheck(st, chunks); err != nil {
					t.Fatalf("INCONCLUSIVE: this row's fixture is refused BEFORE it is "+
						"broken: %v", err)
				}
			}
			got := tc.breakIt(st, chunks)
			err := composerSelfCheck(st, got)
			if err == nil {
				t.Fatalf("the self-check ACCEPTED a build where %s; §8q's refusal would "+
					"never fire and a wrong wallet would reach steel as reviewed", tc.name)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the build was refused, but by a DIFFERENT arm than this row "+
					"covers -- so the arm named here is still untested.\n got:  %v\n"+
					" want a refusal containing: %q", err, tc.want)
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

// composerLockedDigestFixture is an honest build that HAS a lock and a digest,
// so the self-check's lock-VALUE and digest arms are reachable: perturbing a
// nil lock or a nil hash trips the count arm above them instead, which is why
// the seated 2-of-3 cannot exercise either.
func composerLockedDigestFixture(t *testing.T) (*composerState, []string) {
	t.Helper()
	var digest [32]byte
	for i := range digest {
		digest[i] = 0x5a
	}
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true},
			Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Hash: &digest},
	}}
	st := &composerState{list: list, reg: &seedRegistry{}}
	n := composerSlotCount(list)
	declared := make([]*md.SlotOrigin, n)
	st.assigned = make([]composerAssignment, n)
	st.sources = make([]composerSource, n)
	for i := range st.assigned {
		st.sources[i] = composerSource{kind: composerSourceKey, seedID: -1}
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

// composerCollidingOriginFixture pairs a SHIPPED md1 whose two slots sit at one
// origin with no fingerprints against a state whose shape matches it exactly.
//
// THE COMPOSER'S OWN BUILDER CANNOT PRODUCE THIS, and that is the point:
// md.ComposeWith refuses it ("two slots declare the same origin without two
// distinct fingerprints"), so no fault a test can inject through the builder
// reaches §4f's arm on the DECODED md1. md/testdata/template/wsh_sortedmulti.tmpl.md1.txt
// is a 2-of-2 wsh whose slots both declare m/48'/0'/0'/2' and carry no
// fingerprint -- exactly the template §4f says cannot be restored -- so it is
// the artifact that drives the arm. Both slots are UNSEATED here, so the
// per-slot origin and fingerprint arms are skipped and §4f's is what fires.
func composerCollidingOriginFixture(t *testing.T) (*composerState, []string) {
	t.Helper()
	raw, err := os.ReadFile("../md/testdata/template/wsh_sortedmulti.tmpl.md1.txt")
	if err != nil {
		t.Fatalf("INCONCLUSIVE: the colliding-origin artifact is missing: %v", err)
	}
	var chunks []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			chunks = append(chunks, line)
		}
	}
	if len(chunks) == 0 {
		t.Fatal("INCONCLUSIVE: the colliding-origin artifact holds no md1 chunk")
	}
	st := &composerState{reg: &seedRegistry{}}
	st.list = md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
	}}
	composerSizeAssignments(st)
	return st, chunks
}

// TestComposerUseSiteGuardRefusesEveryShapeButTheFixedOne is the sixth arm of
// review r0 I-2, and it is a PREDICATE test rather than a fault-injection row
// because the arm's dispatch cannot be driven by any artifact this tree can
// build.
//
// MEASURED while folding I-2: every md1 the tree can produce or read carries
// the fixed <0;1>/* use-site. md.ComposeWith always emits it; all 61 vendored
// compose vectors carry it; both readable md/testdata/template fixtures carry
// it; and md's per-slot useSiteOverrides are unexported with no exported
// constructor, so composerSelfCheckFaultHook -- which rewrites CHUNK STRINGS --
// has nothing to rewrite them into. So the guard's PREDICATE is tested here in
// both directions, and the fact that its dispatch has no reachable input is
// filed rather than faked: a row that cannot fail is the defect I-2 is about.
func TestComposerUseSiteGuardRefusesEveryShapeButTheFixedOne(t *testing.T) {
	fixed := md.UseSite{
		HasMultipath: true,
		Multipath:    []md.UseSiteAlt{{Value: 0}, {Value: 1}},
	}
	if !composerUseSiteIsFixed(fixed) {
		t.Fatal("INCONCLUSIVE: the guard refuses the fixed <0;1>/* use-site itself, so " +
			"every rejection below would pass for the wrong reason")
	}
	for _, tc := range []struct {
		name string
		u    md.UseSite
	}{
		{"no multipath at all", md.UseSite{}},
		{"a hardened wildcard", md.UseSite{
			HasMultipath: true, WildcardHardened: true,
			Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 1}}}},
		{"one alternative", md.UseSite{
			HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}}}},
		{"three alternatives", md.UseSite{
			HasMultipath: true,
			Multipath:    []md.UseSiteAlt{{Value: 0}, {Value: 1}, {Value: 2}}}},
		{"receive chain is not 0", md.UseSite{
			HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 2}, {Value: 1}}}},
		{"change chain is not 1", md.UseSite{
			HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 3}}}},
		{"a hardened alternative", md.UseSite{
			HasMultipath: true,
			Multipath:    []md.UseSiteAlt{{Value: 0, Hardened: true}, {Value: 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if composerUseSiteIsFixed(tc.u) {
				t.Errorf("the guard accepts %s as the fixed <0;1>/* use-site, so a slot "+
					"whose addresses come from another chain would pass the self-check",
					tc.name)
			}
		})
	}
}

// composerHonestBuildFor composes an offered preset with every slot seated at
// its own account -- the honest build an operator gets by picking that preset
// and seating it -- and returns the state and the chunks the device would
// actually decode.
func composerHonestBuildFor(t *testing.T, w md.ComposeWrapper, list md.PathList) (*composerState, []string) {
	t.Helper()
	scriptType := uint32(2)
	if w == md.ComposeTr {
		scriptType = 3
	}
	n := composerSlotCount(list)
	st := &composerState{list: list, reg: &seedRegistry{}}
	st.assigned = make([]composerAssignment, n)
	st.sources = make([]composerSource, n)
	declared := make([]*md.SlotOrigin, n)
	for i := range st.assigned {
		fp := [4]byte{0x73, 0xc5, 0xda, byte(i)}
		origin := composerTestOrigin(scriptType, uint32(i))
		st.sources[i] = composerSource{kind: composerSourceKey, seedID: -1}
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

// TestComposerSelfCheckAcceptsEveryOfferedPresetsHonestBuild is the fault
// table's missing CONTROL, and it found a Critical the moment it was written.
//
// §7e's self-check compares the DECODED md1 against what the operator built,
// and §8q's refusal is what an operator sees when it disagrees: "The policy on
// this device does not match what you built. Go back and check the path list,
// or start again." So a self-check that refuses an HONEST build does not
// mis-report a detail -- it makes that wallet unbuildable on this device, and
// says the device's own output is wrong.
//
// MEASURED before the fix: 4 of the 12 offered (wrapper, preset) pairs were
// refused -- tiered-recovery and decaying-multisig under BOTH wrappers, with
// "path 2 is 1-of-2 in the shape and 0-of-0 decoded". md.Branch's K and N are
// documented as set ONLY for a branch that is exactly a threshold over keys
// (md/policy_shape.go:45-48, "Zero means 'not a plain k-of-N' — NOT '1-of-1'"),
// and a multi behind a timelock lowers to and_v(v:multi(k,…),older(n)), which
// is not. The self-check read K/N outside their own domain; Keys is the field
// the contract sets in that case.
//
// EVERY OFFERED PAIR, not a sample: task A10 pinned the presets' CHUNKS against
// the Rust primary and never ran one through the device's own consent gate, so
// the two facts a preset needs -- right bytes, and acceptable to this device --
// were proved separately and only the first was proved at all.
func TestComposerSelfCheckAcceptsEveryOfferedPresetsHonestBuild(t *testing.T) {
	for _, w := range []md.ComposeWrapper{md.ComposeWsh, md.ComposeTr, md.ComposeShWsh, md.ComposeSh} {
		for _, pre := range composerPresets(w) {
			t.Run(composerWrapperName(w)+"/"+pre.name, func(t *testing.T) {
				st, chunks := composerHonestBuildFor(t, w, pre.list)
				if err := composerSelfCheck(st, chunks); err != nil {
					t.Errorf("§7e's self-check REFUSES the honest build of an offered "+
						"preset, so an operator who picks it meets §8q on a correct "+
						"composition and the wallet cannot be built on this device: %v", err)
				}
			})
		}
	}
}

// TestComposerSelfCheckStillComparesKeyCountsUnderALock is the other half: the
// fix must not turn a real disagreement into a pass. A branch whose multi sits
// under a lock reports Keys but not K/N, so the key COUNT is what remains
// comparable -- and it must still be compared.
func TestComposerSelfCheckStillComparesKeyCountsUnderALock(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true},
			Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
	}}
	st, chunks := composerHonestBuildFor(t, md.ComposeWsh, list)
	if err := composerSelfCheck(st, chunks); err != nil {
		t.Fatalf("INCONCLUSIVE: the honest locked 2-of-3 is refused: %v", err)
	}
	// The shape now claims a key the chunks do not carry.
	st.list.Paths[0].Keys = &md.KeySet{K: 2, N: 4, Sorted: true}
	err := composerSelfCheck(st, chunks)
	if err == nil {
		t.Fatal("the self-check ACCEPTED a shape claiming 4 keys against chunks carrying " +
			"3, on a path whose multi sits under a lock -- the fix for the K/N contract " +
			"must not stop comparing what the contract DOES set")
	}
	if !strings.Contains(err.Error(), "keys") {
		t.Errorf("the refusal does not name the key count: %v", err)
	}
}
