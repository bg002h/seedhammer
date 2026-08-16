package gui

import (
	"bytes"
	"strings"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── F-188: the SUPPLY path engraves a plate PER MATCHED SLOT ────────────────
//
// The supply path used to call findUserSlot, engrave ONE plate for the first
// matched slot, and say:
//
//	This key is reused at slots @0 and @1; engraving the first (@0).
//
// The sentence is FALSE, and the falsity is the tell. allUserSlots derives the
// seed at EACH slot's own OriginPath, so a seed filling @0 and @1 holds two
// DIFFERENT keys at two DIFFERENT origins. It is one seed at several accounts,
// never one key repeated -- and the plate that was not cut is a slot the
// operator cannot prove membership of.
//
// The engrave rule now matches the verify rule at the source: one plate per
// matched slot, one ms1 per distinct seed (the supply path has exactly one), and
// the count stated BEFORE the first plate is cut. This is the change that makes
// an operator cut more plates than the same inputs produced yesterday, so the
// census is not a nicety here.
//
// THESE TESTS DRIVE THE REAL FLOW. A helper-level test cannot see the wiring,
// and the wiring is where the previous attempt at this defect class shipped a
// mechanism no test could see.

// s5SuppliedTraceBMd1 is Trace B's assembled policy, used here as a SUPPLIED
// md1: 3-of-4 sortedmulti with master A at @0 and @1 (two accounts, two
// origins, two keys), master B at @2 and a payload cosigner at @3.
//
// The operator of the supply path types master A, so the flow matches TWO slots.
func s5SuppliedTraceBMd1(t *testing.T) []string {
	t.Helper()
	p, _, _, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	return out
}

// s5SupplyPremise measures the shape this whole file rests on, rather than
// assuming it: master A must fill EXACTLY two slots of the supplied policy, at
// DIFFERENT origins and with DIFFERENT keys.
//
// If any of that stops holding, these tests are asserting something else and
// must say so instead of reporting green.
func s5SupplyPremise(t *testing.T, md1 []string) (m bip39.Mnemonic, keys []md.ExpandedKey, slots []int) {
	t.Helper()
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	_, keys, err = md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	slots = allUserSlots(m, "", s5Net, keys)
	if len(slots) != 2 {
		t.Fatalf("master A fills slots %v of the supplied policy, want exactly 2. This "+
			"file's subject is a seed the supply path matched at SEVERAL slots", slots)
	}
	if keys[slots[0]].OriginPath.String() == keys[slots[1]].OriginPath.String() {
		t.Fatalf("slots @%d and @%d share the origin %s, so this fixture no longer models "+
			"one seed at several accounts", slots[0], slots[1], keys[slots[0]].OriginPath)
	}
	if keys[slots[0]].Xpub == keys[slots[1]].Xpub {
		t.Fatalf("slots @%d and @%d declare the SAME key. The shipped message called that "+
			"a reused key; the measured shape is two DIFFERENT keys, and if the fixture "+
			"ever became a genuine repeat this file would be testing the other thing",
			slots[0], slots[1])
	}
	return m, keys, slots
}

// s5SupplyDrive is one drive of supplyMultisigPolicyFlow, with the screens it
// passed on the way.
type s5SupplyDrive struct {
	// announce is the frame carrying the multi-slot notice, or "" when the flow
	// went straight from the passphrase to the engrave mode.
	announce string
	// census is every page of the "Plate Count" review screen, joined.
	census string
	// engrave is the first engrave-style picker frame ("Card 1 of N"), i.e. the
	// first thing the machine says before it cuts anything.
	engrave string
}

// s5DriveSupply drives supplyMultisigPolicyFlow from the gather to the first
// engrave-style picker.
//
// It asserts every screen on the way past, for s5DriveToGate's reason: tapping
// through a screen that did not appear silently answers the NEXT one, and a walk
// that does that reports a verdict for a flow nobody ran.
func s5DriveSupply(t *testing.T, md1 []string, phrase string, full bool) *s5SupplyDrive {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	// The gatherer's payload seam: the md1 chunks enter through the SAME offer()
	// a scanned card takes, so this feeds the real gather rather than
	// hand-building its output.
	ctx.syswBundleSeeds = append([]string(nil), md1...)

	frame, quit := runUI(ctx, func() { supplyMultisigPolicyFlow(ctx, &descriptorTheme) })
	t.Cleanup(quit)

	if c, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
		t.Fatalf("the supplied md1 never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
		t.Fatalf("the gather did not hand off to seed entry; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, phrase)
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()

	d := &s5SupplyDrive{}

	// The multi-slot notice sits between the passphrase and the engrave mode and
	// is dismissible. Pump a short budget for the MODE screen: if it does not
	// arrive, what is on screen is the notice.
	c, ok := pumpUntil(frame, "What to engrave?", 8)
	if !ok {
		d.announce = c
		click(&ctx.Router, Button3) // dismiss
		frame()
		if c, ok = pumpUntil(frame, "What to engrave?", 32); !ok {
			t.Fatalf("the engrave-mode choice was not reached after the notice; got %q", c)
		}
	}
	if !full {
		click(&ctx.Router, Down) // "Watch-only (keys)"
		frame()
	}
	click(&ctx.Router, Button3)
	frame()

	c, ok = pumpUntil(frame, "Plates To Cut", 64)
	if !ok {
		t.Fatalf("the plate census was not shown before the tail; got %q.\n"+
			"This is the change that makes the operator cut MORE plates than the same "+
			"inputs produced yesterday, so the count has to arrive before the first one", c)
	}
	// The census PAGES: the card inventory is what the operator is confirming and
	// it does not fit one screen, so collect every page.
	pages := []string{c}
	for i := 0; i < 6; i++ {
		click(&ctx.Router, Button2)
		next, ok := frame()
		if !ok {
			break
		}
		if next == pages[0] {
			break
		}
		pages = append(pages, next)
	}
	d.census = strings.Join(pages, "\n")

	click(&ctx.Router, Button3) // confirm the census
	frame()
	c, ok = pumpUntil(frame, "Choose engraving", 96)
	if !ok {
		t.Fatalf("the engrave-style picker was not reached; got %q", c)
	}
	d.engrave = c
	return d
}

// TestSupplyFlowEngravesAPlatePerMatchedSlot is this block's central claim, and
// it is the FLOW-level one: a seed filling @0 and @1 of the supplied policy gets
// TWO key plates, one seed plate and one descriptor -- in
// multisigEngraveCardsMulti's order.
//
// The assertion is the machine's own census and its own card counter, which is
// what the operator reads before the first plate is cut. A change that reverts
// the engrave to one-plate-for-the-first-match fails here, at the screen.
func TestSupplyFlowEngravesAPlatePerMatchedSlot(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	_, _, slots := s5SupplyPremise(t, md1)

	d := s5DriveSupply(t, md1, fixtureMasterA, true /* full */)
	t.Logf("census screen: %q", d.census)
	t.Logf("first engrave screen: %q", d.engrave)

	// The census names one card per matched slot, NUMBERED, plus the one seed
	// plate and the descriptor. Under the shipped behaviour there is a single
	// unnumbered "mk1 key".
	for _, want := range []string{"mk1 key 1 of 2", "mk1 key 2 of 2", "ms1 secret share", "md1 descriptor"} {
		if !uiContains(d.census, want) {
			t.Errorf("the plate census does not carry %q. The operator holds a seed that is "+
				"at slots %v of this policy, and every one of them needs its own key plate:\n%q",
				want, slots, d.census)
		}
	}
	if strings.Count(strings.ToLower(strings.ReplaceAll(d.census, " ", "")), "ms1secretshare") != 1 {
		t.Errorf("the census does not carry EXACTLY ONE ms1 entry. The supply path has one "+
			"seed, so a second seed plate is a duplicate secret on steel:\n%q", d.census)
	}

	// AND THE CARD SET ITSELF, as the engrave counter states it: full mode is
	// ms1 + mk1 + mk1 + md1 = 4 cards.
	if !uiContains(d.engrave, "Card 1 of 4") {
		t.Errorf("the engrave set is not ms1 + 2 mk1 + md1. The first plate announces %q, "+
			"want \"Card 1 of 4\"", d.engrave)
	}
}

// TestSupplyFlowAnnouncesWhatWillBeCut pins the OPERATOR-FACING half of F-188.
//
// The shipped sentence apologised for a plate it was dropping AND described a
// shape the code does not produce ("this key is reused"). The replacement has to
// state what WILL be cut, and it must not carry the false claim forward.
func TestSupplyFlowAnnouncesWhatWillBeCut(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	_, _, slots := s5SupplyPremise(t, md1)

	d := s5DriveSupply(t, md1, fixtureMasterA, true /* full */)
	t.Logf("multi-slot notice: %q", d.announce)

	if d.announce == "" {
		t.Fatalf("a seed matched at slots %v produced no notice at all. The operator is "+
			"about to cut more plates than this policy's slot count suggests, and nothing "+
			"said why", slots)
	}
	if uiContains(d.announce, "reused") {
		t.Errorf("the notice still calls this a REUSED key:\n%q\nThe keys at slots %v are "+
			"DIFFERENT keys at DIFFERENT origins. One seed, several accounts", d.announce, slots)
	}
	if uiContains(d.announce, "engraving the first") {
		t.Errorf("the notice still apologises for a plate it is dropping:\n%q", d.announce)
	}
	for _, want := range []string{"@0", "@1", "2 key plates"} {
		if !uiContains(d.announce, want) {
			t.Errorf("the notice does not carry %q, so it does not say what will be cut:\n%q",
				want, d.announce)
		}
	}
}

// s5DriveVerifyTolerant drives multisigVerifyFlow over a readback that the flow
// may REFUSE before it ever asks for a seed, and returns the last frame.
//
// s5DriveVerify cannot be used for that: it fatals when the gather does not hand
// off to seed entry, which is precisely the outcome a short readback is allowed
// to have. What the caller asserts on is the VERDICT, not the route to it.
func s5DriveVerifyTolerant(t *testing.T, records []string, expected []int, engravedMd1 []string, phrase string) string {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() { multisigVerifyFlow(ctx, &descriptorTheme, false, expected, engravedMd1) })
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	// Route 1: refused before any secret is asked for.
	c, ok := pumpUntil(frame, "Choose number of words", 48)
	if !ok {
		return c
	}
	// Route 2: the flow wants the seed.
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, phrase)
	if c2, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c2)
	}
	click(&ctx.Router, Button3) // Skip
	last := ""
	for i := 0; i < 96; i++ {
		f, ok := frame()
		if !ok {
			break
		}
		last = f
		if uiContains(last, "Verify OK") || uiContains(last, "Verify Failed") ||
			uiContains(last, "Verify Incomplete") || uiContains(last, "not checked yet") {
			break
		}
	}
	return last
}

// TestVerifyRefusesAPartialReadbackOfAThreePlateBuild is the regression arm the
// previous block DISCLOSED and did not write.
//
// Trace B engraves three key plates across two masters. The operator presents
// only the two that master A accounts for. Under the shipped `len(readbackMk1s)`
// loop bound the flow derived two legs, matched them to the two plates, and
// showed VERIFY OK -- without ever asking for master B's seed. Master B's plate
// was never looked at, and k=3 needs it.
//
// The obligation is what the ENGRAVE cut, never what the readback offered. Any
// verdict other than "Verify OK" is acceptable here; that one is not.
func TestVerifyRefusesAPartialReadbackOfAThreePlateBuild(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false) // watch-only: no ms1 either side
	if len(plates) != 3 {
		t.Fatalf("Trace B engraved %d plate(s), want 3", len(plates))
	}
	// The two plates master A can prove, found by DECODING rather than by
	// trusting the engrave order.
	legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
	if len(legs) != 2 {
		t.Fatalf("master A re-derived %d leg(s) of Trace B, want 2. This test needs a "+
			"readback that one seed can fully account for", len(legs))
	}
	records := append([]string(nil), md1...)
	for _, l := range legs {
		records = append(records, plates[s5PlateFor(t, plates, l)]...)
	}

	last := s5DriveVerifyTolerant(t, records, []int{0, 1, 2}, md1, fixtureMasterA)
	if uiContains(last, "Verify OK") {
		t.Fatalf("a THREE-plate build verified clean against a TWO-plate readback. Final "+
			"screen: %q\nMaster B's plate was never presented and master B's seed was never "+
			"asked for, so nothing about @2 was checked. That is the false GREEN this "+
			"expectation list exists to remove", last)
	}
	if last == "" {
		t.Error("the flow said NOTHING about a readback it could not verify. Silence after " +
			"a verify is indistinguishable from a pass to the operator who walks away")
	}
	t.Logf("short readback verdict: %q", last)
}

// TestSupplyEngraveTailCutsAPlatePerMatchedSlot is the tail's own contract,
// asserted on the CARDS rather than on a screen: every matched slot gets an mk1
// derived AT THAT SLOT'S OWN ORIGIN, the one seed gets exactly one ms1, the
// supplied md1 is carried verbatim, and the order is
// multisigEngraveCardsMulti's -- every ms1, then every mk1, then the md1.
//
// The order is a CONTRACT, not a rendering choice (oracle.CheckArtifactShape
// requires those kinds as consecutive runs in that sequence), so it is asserted
// here rather than left to the emitter's doc comment.
func TestSupplyEngraveTailCutsAPlatePerMatchedSlot(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	m, keys, matched := s5SupplyPremise(t, md1)

	engraved, cards, err := supplyEngraveTail(m, "", s5Net, keys, matched, md1, true)
	if err != nil {
		t.Fatalf("supplyEngraveTail(full): %v", err)
	}

	// THE OBLIGATION LIST IS THIS RUN'S STEEL. It comes back from the loop that
	// cut the plates, so it cannot be a set the engrave never made.
	if len(engraved) != len(matched) {
		t.Fatalf("the tail cut plates for slots %v, want %v", engraved, matched)
	}
	for i := range matched {
		if engraved[i] != matched[i] {
			t.Fatalf("the tail cut plates for slots %v, want %v", engraved, matched)
		}
	}

	wantKinds := []bundleCardKind{cardMS1, cardMK1, cardMK1, cardMD1}
	if len(cards) != len(wantKinds) {
		t.Fatalf("the tail emitted %d card(s), want %d (1 ms1 + 2 mk1 + 1 md1)", len(cards), len(wantKinds))
	}
	for i, want := range wantKinds {
		if cards[i].kind != want {
			var got []bundleCardKind
			for _, c := range cards {
				got = append(got, c.kind)
			}
			t.Fatalf("the engrave set is %v, want %v. multisigEngraveCardsMulti's order is a "+
				"CONTRACT: every ms1, then every mk1, then the md1", got, wantKinds)
		}
	}

	// ONE ms1 FOR ONE SEED. The supply path has exactly one, so a second seed
	// plate would be a duplicate secret on steel with no recovery benefit.
	ms1s := 0
	for _, c := range cards {
		if c.kind == cardMS1 {
			ms1s++
			if len(c.strings) != 1 {
				t.Errorf("an ms1 card carries %d string(s), want 1", len(c.strings))
			}
		}
	}
	if ms1s != 1 {
		t.Errorf("the tail engraved %d ms1 plate(s) for ONE seed, want 1", ms1s)
	}

	// EACH mk1 CARRIES ITS OWN SLOT'S KEY, AT ITS OWN ORIGIN. This is the half a
	// cardinality check is blind to: two plates both derived at @0's origin is
	// still "two plates", and it is still a slot the operator cannot prove.
	mk1s := [][]string{}
	for _, c := range cards {
		if c.kind == cardMK1 {
			mk1s = append(mk1s, c.strings)
		}
	}
	if len(mk1s) != len(matched) {
		t.Fatalf("%d key plate(s) for %d matched slot(s)", len(mk1s), len(matched))
	}
	for i, s := range matched {
		card, derr := mk.Decode(mk1s[i])
		if derr != nil {
			t.Fatalf("the plate for @%d does not decode: %v", s, derr)
		}
		if card.Path != keys[s].OriginPath.String() {
			t.Errorf("the plate for @%d declares origin %s, but @%d's key lives at %s",
				s, card.Path, s, keys[s].OriginPath)
		}
		cc, pk, _, xerr := decodeXpubBytes(card.Xpub)
		if xerr != nil {
			t.Fatalf("the plate for @%d carries an undecodable xpub: %v", s, xerr)
		}
		if !bytes.Equal(cc[:], keys[s].Xpub[0:32]) || !bytes.Equal(pk[:], keys[s].Xpub[32:65]) {
			t.Errorf("the plate for @%d does not carry @%d's key. One seed at several "+
				"accounts holds a DIFFERENT key in each, so a plate cut at the wrong origin "+
				"proves membership of nothing", s, s)
		}
	}

	// THE SUPPLIED md1 IS ENGRAVED VERBATIM (I-2).
	last := cards[len(cards)-1]
	if !equalStringSlice(last.strings, md1) {
		t.Errorf("the md1 card is not the supplied policy verbatim:\n got %v\nwant %v",
			last.strings, md1)
	}

	// WATCH-ONLY carries no secret at all.
	_, watch, werr := supplyEngraveTail(m, "", s5Net, keys, matched, md1, false)
	if werr != nil {
		t.Fatalf("supplyEngraveTail(watch-only): %v", werr)
	}
	for _, c := range watch {
		if c.kind == cardMS1 {
			t.Fatal("a WATCH-ONLY engrave emitted a seed plate")
		}
	}
	if len(watch) != len(matched)+1 {
		t.Errorf("a watch-only engrave emitted %d card(s), want %d (2 mk1 + 1 md1)",
			len(watch), len(matched)+1)
	}
}

// TestSupplyEngraveVerifiesItsOwnOutput closes the loop the F-188 ruling is
// about: the engrave rule and the verify rule now agree at the source, so the
// supply path's OWN complete output must verify clean end to end.
//
// It runs the REAL verify flow -- gather, seed, passphrase, derive, compare,
// screen -- over the plates this tail actually minted, with the obligation list
// the tail actually returned. Before F-188 the two rules disagreed and this
// shape could only be made to pass by teaching the checker to tolerate the
// disagreement.
func TestSupplyEngraveVerifiesItsOwnOutput(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	m, keys, matched := s5SupplyPremise(t, md1)

	engraved, cards, err := supplyEngraveTail(m, "", s5Net, keys, matched, md1, false)
	if err != nil {
		t.Fatalf("supplyEngraveTail: %v", err)
	}
	records := append([]string(nil), md1...)
	plates := 0
	for _, c := range cards {
		if c.kind == cardMK1 {
			records = append(records, c.strings...)
			plates++
		}
	}
	if plates != 2 {
		t.Fatalf("the tail minted %d key plate(s), want 2", plates)
	}

	last, _ := s5DriveVerify(t, records, engraved, md1, fixtureMasterA)
	if !uiContains(last, "Verify OK") {
		t.Fatalf("the supply path's own COMPLETE output did not verify. Final screen: %q\n"+
			"It engraved a plate for each of slots %v and the operator presented both; a "+
			"verify that then reports anything else is calling this machine's own correct "+
			"output bad", last, engraved)
	}
}
