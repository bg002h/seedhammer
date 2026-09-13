package gui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

// composerTemplateChunks composes a two-path wsh template with no keys, the
// shape Part A's exit engraves.
func composerTemplateChunks(t *testing.T) []string {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
	}}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatalf("Chunks: %v", err)
	}
	return chunks
}

// TestComposerStubLinesTeachTheStubAndTheOrigins is §7c: the labels are
// LITERAL, the mk encode command is present, §8d is present, and every
// unseated slot names the origin a card must declare to seat there.
func TestComposerStubLinesTeachTheStubAndTheOrigins(t *testing.T) {
	chunks := composerTemplateChunks(t)
	lines, err := composerStubLines(chunks, nil, composerStubUnchanged)
	if err != nil {
		t.Fatalf("composerStubLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Template-ID:",
		"mk1 stub (template):",
		"mk encode --xpub",
		"--policy-id-stub",
		composerCopyOwnWallet(),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the stub screen does not say %q:\n%s", want, joined)
		}
	}
	// FOUR SLOTS, FOUR EXPECTED-ORIGIN LINES, each at a DISTINCT account: the
	// §4f invariant is what makes the template seatable at all.
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		line := k.OriginPath.String()
		if !strings.Contains(joined, line) {
			t.Errorf("no line names slot @%d's expected origin %s:\n%s", k.Index, line, joined)
		}
		if seen[line] {
			t.Errorf("two slots declare the same origin %s with no fingerprints, which "+
				"errSeatSlotContested makes unseatable (§4f's invariant)", line)
		}
		seen[line] = true
	}
	if len(keys) != 4 {
		t.Fatalf("the fixture has %d slots, want 4", len(keys))
	}
}

// TestComposerStubScreenSaysTheIdChangedAfterAnEdit is §8s's first body and
// §12 item 5's condition test for it.
func TestComposerStubScreenSaysTheIdChangedAfterAnEdit(t *testing.T) {
	chunks := composerTemplateChunks(t)
	fresh, err := composerStubLines(chunks, nil, composerStubUnchanged)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(fresh, "\n"), composerCopyIdChanged()) {
		t.Error("a first showing carries the changed-id line, which would be false")
	}
	after, err := composerStubLines(chunks, nil, composerStubIdMoved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(after, "\n"), composerCopyIdChanged()) {
		t.Error("a re-show after an edit does not carry §8s's changed-id line")
	}
	assertModalBodyFits(t, "the §8s changed-id line", errorScreenBody, composerCopyIdChanged())
	assertModalBodyFits(t, "the §8d own-wallet line", errorScreenBody, composerCopyOwnWallet())
}

// TestComposerStubScreenIsPagedAtItsMeasuredCapacity is §12 item 5's rule for
// a variable-length screen: assert the PAGING, since a fits assertion cannot
// pin a body with no single source string.
func TestComposerStubScreenIsPagedAtItsMeasuredCapacity(t *testing.T) {
	// A 32-slot template: the grammar's maximum, and the case the screen
	// exists to survive.
	list := md.PathList{Wrapper: md.ComposeWsh}
	for i := 0; i < 4; i++ {
		list.Paths = append(list.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 8}})
	}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatalf("md.Compose: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := composerStubLines(chunks, nil, composerStubUnchanged)
	if err != nil {
		t.Fatal(err)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	_, shown, _ := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, 0, -1)
	if shown >= len(lines) {
		t.Fatalf("a 32-slot stub screen claims all %d lines fit one frame; it must page", len(lines))
	}
	t.Logf("stub screen: %d lines for 32 slots, %d per frame, %d pages",
		len(lines), shown, (len(lines)+shown-1)/shown)
	// And the LAST page is reachable: paging forward by the reported count
	// terminates rather than looping short of the end.
	start, pages := 0, 0
	for start < len(lines) && pages < 64 {
		_, n, _ := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, start, -1)
		if n == 0 {
			t.Fatalf("paging stalled at line %d: composerPageLines drew nothing", start)
		}
		start += n
		pages++
	}
	if start < len(lines) {
		t.Errorf("paging reached line %d of %d before the page cap; the tail is unreachable",
			start, len(lines))
	}
	// It draws.
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerStubFlow(ctx, &descriptorTheme, chunks, nil, composerStubUnchanged)
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the stub screen never drew")
		}
		assertFrameHasBody(t, ink(), "the composer stub-teaching screen")
		if !uiContains(content, "mk1 stub (template):") {
			t.Errorf("the first frame does not carry the stub label.\nFrame: %q", content)
		}
	})
}

// TestComposerTemplateEngraveScreenUsesTheStubLabel pins the §7c relabelling
// on the SHIPPED screen: its 4-byte value is a STUB, and calling it
// "Template-ID" beside a 16-byte id of the same name is how an operator comes
// to compare the wrong one against a coordinator.
func TestComposerTemplateEngraveScreenUsesTheStubLabel(t *testing.T) {
	lines := templateConsentLines(md.Template{N: 3, Renderable: true, Policy: md.PolicySortedMulti, K: 2},
		[4]byte{0xde, 0xad, 0xbe, 0xef}, 0, md.PolicyShape{})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "mk1 stub (template): deadbeef") {
		t.Errorf("the template-engrave screen does not label its 4-byte value as a stub:\n%s", joined)
	}
	if strings.Contains(joined, "Template-ID: deadbeef") {
		t.Errorf("the template-engrave screen still calls a 4-byte stub Template-ID, which "+
			"is the label md.WalletIdKind gives the 16-byte id:\n%s", joined)
	}
}

// TestComposerIdChangedComparesIdsNotChunks is journey I-5 / F-520: the §8s
// banner claims "this id changed" and "cards minted with the old stub will not
// seat here", and both are propositions about the ID.
//
// The predicate compared CHUNK STRINGS, which is a different question. A
// journey walk measured the banner firing on a revisit where the Template-ID
// printed two lines below it was byte-identical (F-520). Comparing the thing
// the sentence is about makes that impossible by construction, whichever leg
// produced the differing chunks -- and if two chunk sets ever did carry one id,
// the cards WOULD seat and there was nothing to warn about.
//
// WHAT THIS TEST CANNOT DO, stated plainly: it does not reproduce the leg that
// produced differing chunks for an unchanged shape. composerTemplateChunksFor
// is deterministic -- measured, twice over one state, byte-identical -- so the
// leg is a state change that moves the chunks without moving the id, and it is
// still unidentified. This fix removes the whole CLASS rather than that one
// leg, which is why it does not wait on finding it.
//
// MUTATION: compare the chunk slices instead of the ids and the "same shape,
// re-derived" case still passes, because derivation is deterministic -- that is
// exactly why the old predicate looked correct. Return a constant false and the
// "genuinely different shape" case fails; return a constant true and the
// unchanged case fails.
// deltaWalk mirrors composerFlow's own use of the pair: compare against memory,
// then record what this reading advertised. A test that compared without
// recording would be testing a caller nobody writes.
func deltaWalk(readings ...[]string) []composerStubChange {
	seen := composerOriginMemory{}
	var shown []string
	out := make([]composerStubChange, 0, len(readings))
	for _, r := range readings {
		change, advertised := composerStubDelta(shown, seen, r)
		seen.remember(advertised)
		shown = r
		out = append(out, change)
	}
	return out
}

func TestComposerStubDeltaNamesWhatMoved(t *testing.T) {
	chunksFor := func(t *testing.T, list md.PathList) []string {
		t.Helper()
		c, err := md.Compose(list)
		if err != nil {
			t.Fatalf("md.Compose: %v", err)
		}
		ch, err := c.Chunks()
		if err != nil {
			t.Fatalf("Chunks: %v", err)
		}
		return ch
	}
	oneKey := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
	}}
	twoOfThree := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}

	t.Run("never shown one is not a change", func(t *testing.T) {
		if got := deltaWalk(chunksFor(t, oneKey)); got[0] != composerStubUnchanged {
			t.Error("the first visit reported a changed id; there was nothing to change from")
		}
	})

	t.Run("same shape re-derived is not a change", func(t *testing.T) {
		a := chunksFor(t, oneKey)
		b := chunksFor(t, oneKey)
		if got := deltaWalk(a, b); got[1] != composerStubUnchanged {
			t.Error("re-deriving an untouched shape reported a changed id. This is the " +
				"false statement F-520 measured, on the screen whose job is to be " +
				"copied onto steel: an operator who has already minted cosigner " +
				"cards reads that their cards, and other people's, are now useless")
		}
	})

	t.Run("a genuinely different shape is a change", func(t *testing.T) {
		if got := deltaWalk(chunksFor(t, oneKey), chunksFor(t, twoOfThree)); got[1] != composerStubIdMoved {
			t.Error("editing 1 key to 2-of-3 did not report a changed id; the banner " +
				"would be silent on the day it is true")
		}
	})

	t.Run("an unreadable id is reported as changed", func(t *testing.T) {
		// Between a spurious warning and a missing one on a screen that is
		// about to become steel, the spurious one is the survivable mistake.
		if got := deltaWalk([]string{"md1notacard"}, chunksFor(t, oneKey)); got[1] != composerStubIdMoved {
			t.Error("an unreadable prior set was silently treated as unchanged")
		}
	})
}

// TestComposerStubDeltaCatchesOriginDriftUnderOneId is review I-2, and it is the
// case that made narrowing this predicate to the id alone a mistake.
//
// The Template-ID is origin-invariant BY CONSTRUCTION -- md/template_id.go
// hashes the use-site path and the tree and nothing else, no keys, no
// fingerprints, no origins. Seating, meanwhile, DECLARES origins, and the codec
// hands the still-unseated slots the lowest free accounts. So seating slot @0 at
// an unusual account shifts what slots @1 and @2 advertise, under an id that
// cannot move.
//
// Those "expects a key at" lines are not decoration: they are the screen's own
// instruction for minting the cosigner card. An operator who wrote one down,
// minted the card, then seated @0 and came back would see a different origin
// advertised with no warning at all. The card passes layer 1 -- the stub is the
// top four bytes of an unchanged id -- and is refused by slotMatchesCard at
// layer 2 with errSeatNoSlot.
//
// The commit that narrowed this said "if two chunk sets really do carry one id,
// the cards DO seat and there was nothing to warn about." That claim is what
// this test falsifies.
//
// MUTATION: drop the origin comparison from composerStubDelta and this reports
// Unchanged -- the silence the operator would have engraved against.
func TestComposerStubDeltaCatchesOriginDriftUnderOneId(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	chunksWith := func(t *testing.T, declared []*md.SlotOrigin) []string {
		t.Helper()
		c, err := md.ComposeWith(list, declared)
		if err != nil {
			t.Fatalf("md.ComposeWith: %v", err)
		}
		ch, err := c.Chunks()
		if err != nil {
			t.Fatalf("Chunks: %v", err)
		}
		return ch
	}

	before := chunksWith(t, make([]*md.SlotOrigin, 3))

	seated := make([]*md.SlotOrigin, 3)
	seated[0] = &md.SlotOrigin{
		// Account 5', which the defaults would not have chosen, so the
		// unseated slots below it shift down.
		Origin:      composerTestOrigin(2, 5),
		Fingerprint: [4]byte{0xde, 0xad, 0xbe, 0xef},
		FpPresent:   true,
	}
	after := chunksWith(t, seated)

	// The fixture must actually hold the id still, or this test proves nothing.
	beforeID, beforeKind, err := md.FormAwareIdChunks(before)
	if err != nil {
		t.Fatalf("id before: %v", err)
	}
	afterID, afterKind, err := md.FormAwareIdChunks(after)
	if err != nil {
		t.Fatalf("id after: %v", err)
	}
	if beforeID != afterID || beforeKind != afterKind {
		t.Fatalf("the fixture moved the id (%x -> %x): it no longer isolates origin drift",
			beforeID, afterID)
	}
	if slices.Equal(before, after) {
		t.Fatal("the fixture produced identical chunks: seating declared nothing")
	}

	beforeOrigins, err := composerAdvertisedOrigins(before)
	if err != nil {
		t.Fatalf("origins before: %v", err)
	}
	afterOrigins, err := composerAdvertisedOrigins(after)
	if err != nil {
		t.Fatalf("origins after: %v", err)
	}
	moved := 0
	for idx, was := range beforeOrigins {
		if now, ok := afterOrigins[idx]; ok && now != was {
			moved++
		}
	}
	if moved == 0 {
		t.Fatalf("no slot that still advertises an origin moved:\n before %v\n after  %v",
			beforeOrigins, afterOrigins)
	}

	if got := deltaWalk(before, after)[1]; got != composerStubOriginsMoved {
		t.Errorf("composerStubDelta = %v, want composerStubOriginsMoved.\n"+
			"The id did not move and the origins did. An operator who minted a "+
			"cosigner card against\n  %v\nwould come back to\n  %v\nand be told "+
			"nothing; the card fails slotMatchesCard with errSeatNoSlot.",
			got, beforeOrigins, afterOrigins)
	}

	// And the screen must SAY origins, not id -- the whole point of the split.
	lines, err := composerStubLines(after, nil, deltaWalk(before, after)[1])
	if err != nil {
		t.Fatalf("composerStubLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, composerCopyOriginsChanged()) {
		t.Errorf("the screen does not carry the origins line.\n%s", joined)
	}
	if strings.Contains(joined, composerCopyIdChanged()) {
		t.Errorf("the screen claims the id changed; it did not.\n%s", joined)
	}
}

// TestComposerStubDeltaIgnoresSeatingASoleSlot is the false positive the fold's
// own re-review found: the fix for review I-2 introduced one while removing
// another.
//
// Comparing every slot's origin meant a SEATED slot counted. Seat the only slot
// of a one-key policy at a non-default account and the origins list changes --
// so the screen said "Cards minted for the old origins will not seat here" with
// no unseated slot left to mint a card for. That is the class of warning this
// predicate exists to remove, restated one revision later.
//
// The rule the code follows now: a slot still ADVERTISING an origin is an
// instruction to mint against; a seated slot is a report of what was seated.
// Only instructions are compared.
//
// MUTATION: stop skipping seated slots in composerAdvertisedOrigins and this
// reports OriginsMoved. TestComposerStubDeltaCatchesOriginDriftUnderOneId is
// the other half and must stay green under the fix -- there the slots that move
// are @1 and @2, both unseated.
func TestComposerStubDeltaIgnoresSeatingASoleSlot(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
	}}
	chunksWith := func(t *testing.T, declared []*md.SlotOrigin) []string {
		t.Helper()
		c, err := md.ComposeWith(list, declared)
		if err != nil {
			t.Fatalf("md.ComposeWith: %v", err)
		}
		ch, err := c.Chunks()
		if err != nil {
			t.Fatalf("Chunks: %v", err)
		}
		return ch
	}

	before := chunksWith(t, make([]*md.SlotOrigin, 1))
	seated := []*md.SlotOrigin{{
		Origin:      composerTestOrigin(2, 7),
		Fingerprint: [4]byte{0xde, 0xad, 0xbe, 0xef},
		FpPresent:   true,
	}}
	after := chunksWith(t, seated)

	if slices.Equal(before, after) {
		t.Fatal("seating the sole slot changed nothing on the wire: the fixture proves nothing")
	}
	if got := deltaWalk(before, after)[1]; got != composerStubUnchanged {
		lines, err := composerStubLines(after, nil, got)
		if err == nil {
			t.Errorf("seating the ONLY slot reported %v and the screen says:\n%s\n"+
				"There is no unseated slot left to mint a card for, so this warns "+
				"about work nobody can have done.", got, strings.Join(lines, "\n"))
		} else {
			t.Errorf("seating the ONLY slot reported %v, want Unchanged", got)
		}
	}
}

// TestComposerStubDeltaSurvivesAFullySeatedGap is review I-4, and it is the
// case an intersection-of-adjacent-readings rule cannot see.
//
// The walk, one id throughout, no shape edit at any point:
//
//	E1  nothing seated. "Slot @2 expects a key at m/48h/0h/2h/2h", and a
//	    cosigner mints a card for exactly that.
//	E2  every slot seated. The advertising set is now EMPTY.
//	E3  @2 released. It advertises a DIFFERENT origin, because the accounts
//	    below it were taken while it was away.
//
// Comparing each reading against the one before it is silent at every step: E2
// has nothing to differ from, and at E3 the previous reading's advertising set
// was empty. Meanwhile the card minted at E1 matches no slot and is refused
// with errSeatNoSlot.
//
// So the memory is per SLOT and never pruned: a slot that leaves the
// advertising set keeps its last advertised origin on the books, and the
// comparison still has something to compare against when it returns.
//
// MUTATION: make composerOriginMemory.remember prune slots missing from `now`,
// or compare against the previous reading instead of the memory, and E3 goes
// silent.
func TestComposerStubDeltaSurvivesAFullySeatedGap(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	chunksWith := func(t *testing.T, declared []*md.SlotOrigin) []string {
		t.Helper()
		c, err := md.ComposeWith(list, declared)
		if err != nil {
			t.Fatalf("md.ComposeWith: %v", err)
		}
		ch, err := c.Chunks()
		if err != nil {
			t.Fatalf("Chunks: %v", err)
		}
		return ch
	}
	seat := func(account uint32, b byte) *md.SlotOrigin {
		return &md.SlotOrigin{
			Origin:      composerTestOrigin(2, account),
			Fingerprint: [4]byte{0xde, 0xad, 0xbe, b},
			FpPresent:   true,
		}
	}

	e1 := chunksWith(t, make([]*md.SlotOrigin, 3))
	e2 := chunksWith(t, []*md.SlotOrigin{seat(0, 0), seat(1, 1), seat(2, 2)})
	// @2 released; @0 and @1 stay seated, at accounts that push @2's advertised
	// origin somewhere it has not been before.
	e3 := chunksWith(t, []*md.SlotOrigin{seat(2, 0), seat(1, 1), nil})

	// The fixture must hold the id still, or this is a test about shape edits.
	ids := make([]string, 0, 3)
	for _, r := range [][]string{e1, e2, e3} {
		id, kind, err := md.FormAwareIdChunks(r)
		if err != nil {
			t.Fatalf("id: %v", err)
		}
		ids = append(ids, fmt.Sprintf("%v:%x", kind, id))
	}
	if ids[0] != ids[1] || ids[1] != ids[2] {
		t.Fatalf("the fixture moved the id across the walk (%v): it no longer "+
			"isolates origin drift", ids)
	}

	// E1's advertised origin for @2 must differ from E3's, or there is nothing
	// for the memory to catch and this test would pass vacuously.
	a1, err := composerAdvertisedOrigins(e1)
	if err != nil {
		t.Fatalf("origins e1: %v", err)
	}
	a2, err := composerAdvertisedOrigins(e2)
	if err != nil {
		t.Fatalf("origins e2: %v", err)
	}
	a3, err := composerAdvertisedOrigins(e3)
	if err != nil {
		t.Fatalf("origins e3: %v", err)
	}
	if len(a2) != 0 {
		t.Fatalf("E2 still advertises %v: the fixture has no gap to survive", a2)
	}
	if a1[2] == "" || a3[2] == "" || a1[2] == a3[2] {
		t.Fatalf("@2 advertises %q at E1 and %q at E3: the fixture has no drift",
			a1[2], a3[2])
	}

	got := deltaWalk(e1, e2, e3)
	if got[2] != composerStubOriginsMoved {
		t.Errorf("E3 reported %v, want OriginsMoved.\n"+
			"A cosigner minted a card for @2 at %s, every slot was then seated, "+
			"and @2 now asks for %s. The card matches no slot and is refused with "+
			"errSeatNoSlot -- and the operator was told nothing at any step.",
			got[2], a1[2], a3[2])
	}
}
