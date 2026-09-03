package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

// TestComposerKeySourcesLabelFingerprintAndOrigin is §7d's label rule: a
// key: record is labelled fingerprint PLUS origin, because two keys sharing
// a fingerprint (one master, two accounts) is the normal C5 case and a
// fingerprint alone would render them identically.
func TestComposerKeySourcesLabelFingerprintAndOrigin(t *testing.T) {
	p := newPlatform()
	ctx := NewContext(p)
	ctx.sysw = composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil)
	got := composerKeySources(ctx)
	if len(got) != 2 {
		t.Fatalf("composerKeySources returned %d sources, want 2", len(got))
	}
	if got[0].label == got[1].label {
		t.Fatalf("two keys from one master render identically as %q; the origin must "+
			"distinguish them (§7g's pack row: labels show fingerprint AND origin)", got[0].label)
	}
	for _, s := range got {
		if !strings.Contains(s.label, "73c5da0a") {
			t.Errorf("the label omits the fingerprint: %q", s.label)
		}
		if !strings.Contains(s.label, "48") {
			t.Errorf("the label omits the origin: %q", s.label)
		}
		if s.xpub == "" {
			t.Errorf("the source carries no xpub, so it can seat nothing: %+v", s)
		}
		if len(s.origin) == 0 {
			t.Errorf("the source carries no origin components; §6a refuses a key: record " +
				"with an empty origin, so this cannot happen from a classified record")
		}
		assertChoiceLabelFits(t, s.label)
	}
}

// TestComposerSourcesRefuseAnUncomparedPayload inherits take's guard: a
// record may not be handed to a program until the payload it came from has
// been authenticated (§12.2). The DOOR counts through has() and is exempt;
// SEATING consumes and is not.
func TestComposerSourcesRefuseAnUncomparedPayload(t *testing.T) {
	p := newPlatform()
	ctx := NewContext(p)
	s := &syswSession{}
	// load(payload, identity, sealed, cliffAbove, compared, digestShown):
	// compared=false is the one that matters here.
	s.load(composerPayloadWith([]string{composerTestKeyRecord}, nil), [32]byte{},
		false, true, false, true)
	ctx.sysw = s
	if got := composerKeySources(ctx); len(got) != 0 {
		t.Errorf("seating took %d keys from an UNCOMPARED payload; take/takeAll refuse "+
			"until one of [compared]'s two routes has run", len(got))
	}
	// CONTROL: the same payload, compared, yields the key -- so the assertion
	// above is measuring the gate and not a broken fixture.
	s2 := &syswSession{}
	s2.load(composerPayloadWith([]string{composerTestKeyRecord}, nil), [32]byte{}, false, true, true, true)
	ctx.sysw = s2
	if got := composerKeySources(ctx); len(got) != 1 {
		t.Fatalf("INCONCLUSIVE: the compared control yielded %d keys, want 1", len(got))
	}
}

// TestComposerSeatPromptIsTheSpecString is §8s's two seating prompts and
// §12 item 5's condition test for them: "Path N" is the OPERATOR's listed
// path index, never an emitted leaf index (§7d).
func TestComposerSeatPromptIsTheSpecString(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	// Slot @0 under wsh is path 1's first key of three.
	if got := composerSeatPrompt(st, 0); got != composerCopySeatPrompt(0, 1, 1, 3) {
		t.Errorf("composerSeatPrompt(@0) = %q, want %q", got, composerCopySeatPrompt(0, 1, 1, 3))
	}
	// Slot @3 is path 2's only key.
	if got := composerSeatPrompt(st, 3); got != composerCopySeatPrompt(3, 2, 1, 1) {
		t.Errorf("composerSeatPrompt(@3) = %q, want %q", got, composerCopySeatPrompt(3, 2, 1, 1))
	}
	assertModalBodyFits(t, "the §8s seating prompt", errorScreenBody, composerCopySeatPrompt(2, 1, 2, 3))
	assertModalBodyFits(t, "the §8s key-path seating prompt", errorScreenBody, composerCopySeatKeyPathPrompt(0))
}

// TestComposerPickListPagesAPayloadLargerThanAFrame is §9 item 7's reason for
// existing, driven end to end rather than at the primitive.
func TestComposerPickListPagesAPayloadLargerThanAFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rows := make([]string, 24)
		for i := range rows {
			rows[i] = composerNumberedLines(24)[i]
		}
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerPickScreen(ctx, &descriptorTheme, "Seat", composerCopySeatPrompt(2, 1, 2, 3), rows)
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the pick list never drew")
		}
		assertFrameHasBody(t, ink(), "the composer seating pick list")
		if uiContains(content, "entry 23 marker") {
			t.Error("all 24 rows drew on one frame, so this fixture no longer exercises paging")
		}
	})
}

// TestComposerSlotOrderAgreesWithTheCodec is the one assertion that keeps
// every seating prompt honest. composerSlotOrder walks §5's numbering rule in
// Go; md.Compose walks the Rust port of it. If they disagree, the operator is
// told they are filling path 1's second key while the key lands in path 2 --
// a silent mis-seat, which is exactly the failure gui/key_card_seating.go
// :24-27 refuses to allow anywhere else.
func TestComposerSlotOrderAgreesWithTheCodec(t *testing.T) {
	digest := [32]byte{0x44}
	for _, list := range []md.PathList{
		composerTwoPathList(),
		{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}}, {Keys: &md.KeySet{K: 2, N: 3, Sorted: true}}}},
		{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}}, {Keys: &md.KeySet{K: 1, N: 1}}}},
		{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 2}, Hash: &digest},
			{Keys: &md.KeySet{K: 2, N: 2}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 7}}}},
	} {
		c, err := md.Compose(list)
		if err != nil {
			t.Fatalf("md.Compose: %v", err)
		}
		slots := c.Slots()
		order := composerSlotOrder(list)
		if len(order) != len(slots) {
			t.Fatalf("composerSlotOrder gives %d slots, md.Compose gives %d for %+v",
				len(order), len(slots), list)
		}
		for i, s := range slots {
			if int(s.Index) != i {
				t.Fatalf("md.Compose slot %d has Index %d; this walk assumes dense "+
					"ascending indices", i, s.Index)
			}
			if order[i].path != s.Path+1 {
				t.Errorf("slot @%d: the prompt says Path %d, md.Compose says path %d",
					i, order[i].path, s.Path+1)
			}
		}
	}
}
