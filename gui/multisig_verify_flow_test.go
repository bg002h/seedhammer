package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S5: the verify checks WHAT THIS RUN ENGRAVED ────────────────────────────
//
// THIS FILE DRIVES THE REAL FLOW, and that is its whole reason to exist
// alongside multisig_verify_legs_test.go. That file exercises the comparator
// (verifyMultisigLegs) against legs a helper built; it is blind to the derive
// loop, which is where the obligation list is decided. The previous attempt at
// this defect shipped a mechanism inside that loop which NO test could see:
// deleting it left the suite green, because every test built its own legs.
//
// So the assertions below go through multisigVerifyFlow itself -- gather, seed,
// passphrase, derive, compare, screen -- and the screen is the assertion. A
// change to the derive loop that breaks the operator's verify breaks this test.

// s5OneSlotReadback is a run that engraved ONE plate for a seed filling TWO
// slots: the policy md1 plus @0's key plate, with {@0} as the expectation.
//
// UNTIL F-188 THIS WAS THE SUPPLY PATH'S OWN SHIPPED SHAPE. It is not any more
// -- that path now cuts a plate per matched slot (supplyEngraveTail) -- but the
// shape is still reachable and still has to verify: the BUILD path engraves the
// slots the operator DECLARED, and a seed that also accounts for an admitted
// cosigner card's slot fills more than were cut. So this is the intersection
// rule's fixture, no longer the supply path's portrait.
//
// Master A fills TWO slots of Trace B, @0 and @1, at DIFFERENT origins and so
// with DIFFERENT keys (asserted below, not assumed -- it is the fact that makes
// a same-key dedupe inert here, and the fact that made the old "this key is
// reused" message false).
//
// It returns the readback records in payload order and the plate's slot.
func s5OneSlotReadback(t *testing.T) (records []string, md1 []string, plate []string, slot int) {
	t.Helper()
	md1, plates, _ := s5TraceBEngraved(t, false) // watch-only: no ms1 either side

	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	filled := allUserSlots(m, "", &chaincfg.MainNetParams, keys)
	if len(filled) < 2 {
		t.Fatalf("master A fills slots %v of Trace B; this test needs a seed filling MORE "+
			"slots than the run engraved, or the intersection it exercises is a no-op", filled)
	}
	idx, origin, reused, ok := findUserSlot(m, "", &chaincfg.MainNetParams, keys)
	if !ok {
		t.Fatal("findUserSlot: master A matches no slot of Trace B")
	}
	if len(reused) < 2 || idx != filled[0] {
		t.Fatalf("findUserSlot returned (@%d, reused %v), want the first of %v", idx, reused, filled)
	}

	// THE PREMISE, MEASURED. The two slots must carry DIFFERENT keys at
	// DIFFERENT origins: that is what makes an engrave of one plate and the seed's
	// own answer ("I fill two slots") disagree, and it is why deduplicating legs
	// by key could never have fixed this.
	other := filled[1]
	if keys[idx].OriginPath.String() == keys[other].OriginPath.String() {
		t.Fatalf("slots @%d and @%d share the origin %s, so this fixture no longer "+
			"models a seed that fills more slots than were cut", idx, other, origin)
	}
	if keys[idx].Xpub == keys[other].Xpub {
		t.Fatalf("slots @%d and @%d declare the SAME key, so a same-key dedupe would "+
			"cover this fixture and it is not the measured shape", idx, other)
	}

	// The one plate this run cut: the one carrying @idx's key.
	b, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, md1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(@%d): %v", idx, err)
	}
	plate = plates[s5PlateFor(t, plates, verifyLeg{Slot: idx, B: b})]

	records = append(records, md1...)
	records = append(records, plate...)
	return records, md1, plate, idx
}

// s5DriveVerify runs multisigVerifyFlow over a readback and returns the last
// frame plus whether the flow returned.
//
// It asserts every screen on the way past, for s5DriveToGate's reason: tapping
// through a screen that did not appear silently answers the NEXT one, and a walk
// that does that reports a verdict for a verify nobody ran.
func s5DriveVerify(t *testing.T, records []string, expected []int, phrase string) (last string, done bool) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	// The gatherer's payload seam: every card enters through the SAME offer() a
	// scanned card takes (gui/bundle_flow.go:161-167), so this feeds the real
	// gather rather than hand-building its output.
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() {
		multisigVerifyFlow(ctx, &descriptorTheme, false, expected)
		done = true
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 32); !ok {
		return c, done
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Choose number of words", 32); !ok {
		t.Fatalf("the gather did not hand off to seed entry; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, phrase)
	if c, ok := pumpUntil(frame, "passphrase", 200); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	last = ""
	for i := 0; i < 64; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		if uiContains(last, "Verify OK") || uiContains(last, "Verify Failed") ||
			uiContains(last, "Verify Incomplete") || uiContains(last, "not checked yet") {
			break
		}
	}
	return last, done
}

// TestVerifyOneSlotRunChecksTheONEPlateItEngraved is the regression.
//
// One seed, two slots, one plate. The verify's derive loop asked the SEED which
// slots it fills and made a leg for each, so a one-plate run's complete and
// correct output verified as "Verify Failed: no read-back key plate carries slot
// @1's key" -- an honest operator told their good plates are bad.
//
// F-188 removed this shape from the SUPPLY path by making that path cut both
// plates. It did not remove the shape: a BUILD holding a subset of the slots its
// seed accounts for still produces it, so the rule stays pinned here.
//
// The fix is not a relaxation of what the comparator demands: only the
// ENGRAVER knows which slots it cut a plate for, so it passes that set in and
// the derive loop restricts itself to it. Every plate still has to be read back
// and matched.
func TestVerifyOneSlotRunChecksTheONEPlateItEngraved(t *testing.T) {
	records, _, _, slot := s5OneSlotReadback(t)
	last, _ := s5DriveVerify(t, records, []int{slot}, fixtureMasterA)
	if !uiContains(last, "Verify OK") {
		t.Fatalf("a run that engraved ONE plate did not verify against it. Final screen: %q\n"+
			"The flow engraved one plate, for @%d; a verify that then demands a plate for "+
			"every slot the seed fills is calling this machine's own correct output bad",
			last, slot)
	}
}

// TestVerifyStillFailsWhenTheENGRAVEDPlateIsWrong is the other direction, and it
// is what stops the test above from being satisfiable by "report OK".
//
// Same shape, same expectation, but the plate read back is a foreign key card.
// It must FAIL, and it must NAME the slot -- that is the only thing the operator
// can act on.
func TestVerifyStillFailsWhenTheENGRAVEDPlateIsWrong(t *testing.T) {
	_, md1, _, slot := s5OneSlotReadback(t)
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	foreign, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams,
		singleSigPath(44), md.ScriptPkh)
	if err != nil {
		t.Fatalf("deriving a foreign mk1: %v", err)
	}
	records := append(append([]string(nil), md1...), foreign.MK1...)
	last, _ := s5DriveVerify(t, records, []int{slot}, fixtureMasterA)
	if !uiContains(last, "Verify Failed") {
		t.Fatalf("a readback whose only key plate belongs to another wallet did not FAIL. "+
			"Final screen: %q", last)
	}
}

// TestVerifyRefusesAnEmptyExpectation: a verify with nothing to prove is an
// error, not a vacuous pass.
//
// Both call sites guarantee at least one slot today (errBuildNoHeldSlot on the
// build path, supplyEngraveTail's matched-slot indices on the supply path, with
// a zero-match seed refused before it). This pins the
// refusal anyway, because the failure mode of an expectation that arrives empty
// is a screen saying the plates are fine over a comparison that never ran -- the
// most expensive false GREEN this flow can produce, and the reason
// errVerifyNoLegs exists one level down.
func TestVerifyRefusesAnEmptyExpectation(t *testing.T) {
	records, _, _, _ := s5OneSlotReadback(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)
	frame, quit := runUI(ctx, func() { multisigVerifyFlow(ctx, &descriptorTheme, false, nil) })
	defer quit()
	c, ok := pumpUntil(frame, "nothing to verify", 16)
	if !ok {
		t.Fatalf("a verify handed NO engraved slot did not refuse; got %q", c)
	}
	if uiContains(c, "mk1 keys:") {
		t.Errorf("the refusal came only after the gather ran: %q. Nothing can be proved, "+
			"so the operator must not be asked to present plates first", c)
	}
}

// TestVerifyBuildShapeChecksEveryEngravedPlate is the build path's arm: three
// engraved plates, two masters, and the flow must ask for the SECOND seed rather
// than stopping once the first seed's slots are covered.
//
// It is the guard against fixing the supply case by making the expectation the
// only thing that matters: expected {0,1,2} with master A alone covers @0 and
// @1, which is NOT a pass.
func TestVerifyBuildShapeChecksEveryEngravedPlate(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	if len(plates) != 3 {
		t.Fatalf("Trace B engraved %d plate(s), want 3", len(plates))
	}
	var records []string
	records = append(records, md1...)
	for _, p := range plates {
		records = append(records, p...)
	}
	last, done := s5DriveVerify(t, records, []int{0, 1, 2}, fixtureMasterA)
	if uiContains(last, "Verify OK") {
		t.Fatalf("master A alone verified a THREE-plate build. @2 is master B's and no "+
			"seed proving it was ever typed. Final screen: %q", last)
	}
	if !uiContains(last, "not checked yet") {
		t.Fatalf("the flow did not offer the next seed after master A covered @0 and @1; "+
			"final screen %q (flow returned: %v)", last, done)
	}
}

// TestBuildEngraveTailReturnsTheSlotsItCut pins the other end of the channel.
//
// The verify's obligation list is only as good as its provenance: it must be the
// slots this tail actually minted a plate for, one index per mk1, in the same
// order. A tail returning the POLICY's slots, or the held slots it was asked
// for rather than the ones it engraved, would hand the verify an obligation for
// steel that does not exist.
func TestBuildEngraveTailReturnsTheSlotsItCut(t *testing.T) {
	p, reg, sources, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	legs, slots, cardsOut, err := buildEngraveTail(sources, p.Script, reg, s5Net, cards, out, false)
	if err != nil {
		t.Fatalf("buildEngraveTail: %v", err)
	}
	want := []int{0, 1, 2}
	if len(slots) != len(want) {
		t.Fatalf("the tail cut plates for slots %v, want %v", slots, want)
	}
	for i := range want {
		if slots[i] != want[i] {
			t.Fatalf("the tail cut plates for slots %v, want %v", slots, want)
		}
	}
	if len(slots) != len(legs) {
		t.Fatalf("%d slot(s) alongside %d leg(s): the two are parallel or the verify's "+
			"obligation list is not this run's", len(slots), len(legs))
	}
	var mk1s int
	for _, c := range cardsOut {
		if c.kind == cardMK1 {
			mk1s++
		}
	}
	if mk1s != len(slots) {
		t.Fatalf("the tail engraved %d key plate(s) but reported %d slot(s). The verify "+
			"would then demand a plate that was never cut, or skip one that was", mk1s, len(slots))
	}
	// And each reported slot's plate must carry THAT slot's key -- an index list
	// that is right in length and wrong in content is the same defect.
	_, keys, err := md.ExpandWalletPolicyChunks(out)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	for i, s := range slots {
		card, derr := mk.Decode(legs[i].MK1)
		if derr != nil {
			t.Fatalf("leg %d's mk1 does not decode: %v", i, derr)
		}
		if card.Path != keys[s].OriginPath.String() {
			t.Fatalf("the plate reported for slot @%d declares origin %s, but @%d's key "+
				"lives at %s", s, card.Path, s, keys[s].OriginPath)
		}
	}
}

// TestBuildPassesTheTailsSlotsToTheVerify pins the WIRING at the build call
// site, which no behavioural test in this package can reach: the verify offer
// sits after bundleEngrave, and driving a real engrave is not something a unit
// test does.
//
// The type system only demands *a* []int. This demands the one the tail
// returned, because any other list is a set of slots this run did not cut.
func TestBuildPassesTheTailsSlotsToTheVerify(t *testing.T) {
	body := funcBody(t, "multisig_build.go", "func buildMultisigPolicyFlow(")
	if !strings.Contains(body, "engravedSlots, cardsOut, terr := buildEngraveTail(") {
		t.Fatal("buildMultisigPolicyFlow no longer names the tail's held-slot indices; " +
			"the verify's obligation list has lost its provenance")
	}
	if !strings.Contains(body, "multisigVerifyFlow(ctx, th, full, engravedSlots)") {
		t.Error("the build path does not hand the tail's own slot indices to the verify. " +
			"Any other list is an obligation over plates this run did not cut")
	}
}
