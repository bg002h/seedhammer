package gui

import (
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S5: multi-slot self, divergent origins, and the engrave tail ────────────
//
// TEST MATERIAL, public by construction: masters A, B and C are BIP-39's own
// published vectors (multisig_build_payload_testdata_test.go). Never put funds
// behind them.
//
// TRACE B is the plan's flagship and most of this file is about it: n=4, k=3,
// the operator holds @0 = A account 0, @1 = A account 1 and @2 = B account 0,
// and @3 is a cosigner's card off the payload. It is the wallet spec round 0's
// C1 would have refused; it is why S5 exists.

// s5Net is the network every test in this file derives on. One place, so a test
// cannot pass by comparing two derivations on different networks.
var s5Net = &chaincfg.MainNetParams

// s5Registry registers each phrase as its own (seed, passphrase="") entry, in
// order, and returns the registry plus the seedIDs.
//
// It registers a master ONCE even when several held slots use it, which is the
// shape the flow produces: buildSlotSources numbers the accounts off the MASTER
// FINGERPRINT, so one master filling two slots gets accounts 0 and 1 and the two
// keys differ.
func s5Registry(t *testing.T, phrases ...string) (*seedRegistry, []int) {
	t.Helper()
	reg := &seedRegistry{}
	ids := make([]int, 0, len(phrases))
	for _, phrase := range phrases {
		m, err := bip39.ParseMnemonic(phrase)
		if err != nil {
			t.Fatalf("ParseMnemonic(%.20s...): %v", phrase, err)
		}
		id, err := reg.add("your seed", m, "", s5Net)
		if err != nil {
			t.Fatalf("registering %.20s...: %v", phrase, err)
		}
		ids = append(ids, id)
	}
	return reg, ids
}

// s5TraceB builds Trace B's inputs through the PRODUCTION projection: the params
// the pickers would carry, the registry, the slot sources, the derived held keys
// and the one payload card.
//
// It goes through buildSlotSources and buildSelfKeys rather than hand-building
// the slot model, because the account numbering (and therefore the whole
// no-duplicate-key argument) is that function's, and a test that hand-assigned
// accounts would prove nothing about the flow.
func s5TraceB(t *testing.T) (p buildPolicyParams, reg *seedRegistry, sources []slotSource, self []heldSlotKey, cards []mk.Card) {
	t.Helper()
	// ONE REGISTRY ENTRY PER HELD SLOT, because that is the shape the flow builds:
	// buildMultisigPolicyFlow calls buildSeedForSlot once per slot in p.SelfSlots
	// (gui/multisig_build.go:194-201) and buildSeedForSlot calls reg.add()
	// unconditionally (:510). So master A held at @0 AND @1 occupies TWO entries
	// carrying two different SeedIDs.
	//
	// This fixture previously registered A once and passed ids[0] twice, with a
	// comment asserting that was the flow's shape. It was not, and the gap hid a
	// Critical: the tail's ms1 dedupe keyed on SeedID, which under the real shape
	// never fires, so Trace B minted a redundant seed plate and numbered it
	// "2 of 3". A fixture modelling a registry the flow cannot produce cannot see
	// a defect in the flow.
	reg, ids := s5Registry(t, fixtureMasterA, fixtureMasterA, fixtureMasterB)
	p = buildPolicyParams{
		Script:    md.MultisigWsh,
		N:         4,
		K:         3,
		SelfSlots: []int{0, 1, 2},
	}
	seedIDs := []int{ids[0], ids[1], ids[2]} // @0 -> A, @1 -> A (2nd entry), @2 -> B
	chosen := []int{2}                       // payload card 3 (roster index 2) is C@0
	cards = []mk.Card{dupTestCard(t, 2)}
	sources = buildSlotSources(p, seedIDs, chosen, reg)
	var err error
	self, err = buildSelfKeys(sources, p.Script, reg, s5Net)
	if err != nil {
		t.Fatalf("buildSelfKeys(Trace B): %v", err)
	}
	if len(self) != 3 {
		t.Fatalf("Trace B derived %d held key(s), want 3", len(self))
	}
	return p, reg, sources, self, cards
}

// TestMultiSlotSelfAssembles is plan test 1: Trace B's shape assembles, with
// DIVERGENT origins and the correct per-slot origin on every slot.
//
// The origins are read back off the ASSEMBLED BYTES, not off the inputs. That is
// the only way to see the origin mode at all: under OriginShared every slot
// reports the one shared path, so a build that silently stayed shared would
// report m/48h/0h/0h/2h at @1 and the assertion below is what separates the two.
func TestMultiSlotSelfAssembles(t *testing.T) {
	p, _, _, self, cards := s5TraceB(t)

	out, stub, slots, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v.\nThis is the plan's flagship wallet: "+
			"one operator holding three of four slots across two masters", err)
	}
	if len(slots) != 4 {
		t.Fatalf("assembled %d slot(s), want 4", len(slots))
	}
	tpl, keys, err := md.ExpandWalletPolicyChunks(out)
	if err != nil {
		t.Fatalf("Trace B's own md1 does not decode: %v", err)
	}
	if tpl.K != 3 || tpl.N != 4 || tpl.Policy != md.PolicySortedMulti || tpl.Root != md.ScriptWsh {
		t.Fatalf("assembled %d-of-%d %v/%v, want 3-of-4 sortedmulti under wsh",
			tpl.K, tpl.N, tpl.Root, tpl.Policy)
	}

	// THE PER-SLOT ORIGINS, which is the whole test.
	wantOrigins := []string{
		"m/48h/0h/0h/2h", // @0  master A, account 0
		"m/48h/0h/1h/2h", // @1  master A, account 1
		"m/48h/0h/0h/2h", // @2  master B, account 0
		cards[0].Path,    // @3  the payload card's own declared origin
	}
	if len(keys) != len(wantOrigins) {
		t.Fatalf("decoded %d key(s), want %d", len(keys), len(wantOrigins))
	}
	for i, want := range wantOrigins {
		if got := keys[i].OriginPath.String(); got != want {
			t.Errorf("@%d origin = %s, want %s. A policy that declares the wrong "+
				"derivation path is a policy a BIP-48-aware coordinator restores at the "+
				"wrong place", i, got, want)
		}
	}
	// DIVERGENCE ASSERTED DIRECTLY, so this cannot pass on a shared-origin build
	// that happens to agree at three of four slots.
	if keys[0].OriginPath.String() == keys[1].OriginPath.String() {
		t.Fatalf("@0 and @1 declare the SAME origin (%s); Trace B holds two accounts "+
			"of ONE master, so the policy MUST be divergent-origin", keys[0].OriginPath)
	}

	// And the keys are the ones the operator actually holds: @1 is master A at
	// account 1, not master A at the shared origin.
	for i, h := range self {
		cc, pk, _, derr := decodeXpubBytes(h.Xpub)
		if derr != nil {
			t.Fatalf("decodeXpubBytes(held %d): %v", i, derr)
		}
		var want [65]byte
		copy(want[0:32], cc[:])
		copy(want[32:65], pk[:])
		if keys[h.Slot].Xpub != want {
			t.Errorf("@%d does not carry the key derived for it", h.Slot)
		}
	}
	t.Logf("Trace B assembled: %d md1 chunk(s), stub %x, origins %s / %s / %s / %s",
		len(out), stub, keys[0].OriginPath, keys[1].OriginPath, keys[2].OriginPath,
		keys[3].OriginPath)
}

// TestCosignerCardOriginIsHonoured is plan test 2 and spec R-3: the CARD's
// declared origin reaches the descriptor, not the flow's shared origin.
//
// Before S5 cosignerFromCard discarded card.Origin entirely and S2 refused any
// card that declared anything else, so this shape could not be built at all. The
// fixture is the one the delivered payload already carries: card A@1, master A's
// SECOND account, at m/48h/0h/1h/2h.
func TestCosignerCardOriginIsHonoured(t *testing.T) {
	card := dupTestCard(t, 3) // A@1
	if card.Path == multisigSharedOrigin().String() {
		t.Fatalf("card A@1 declares %q, the shared origin, so this test cannot tell "+
			"an honoured origin from a stamped one", card.Path)
	}

	// The unit claim first: the cosigner carries the card's origin at all.
	c, err := cosignerFromCard(card, false)
	if err != nil {
		t.Fatalf("cosignerFromCard(A@1): %v", err)
	}
	wantPath, err := bip32.ParsePath(card.Path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", card.Path, err)
	}
	want := originComponents(wantPath)
	if len(c.Origin) != len(want) {
		t.Fatalf("cosignerFromCard produced a %d-component origin for a card declaring "+
			"%q (%d components); the card's origin is being discarded",
			len(c.Origin), card.Path, len(want))
	}
	for i := range want {
		if c.Origin[i] != want[i] {
			t.Fatalf("origin component %d = %+v, want %+v", i, c.Origin[i], want[i])
		}
	}

	// And through the assembler, on the bytes: self = master C at the shared
	// origin, the card at @1.
	selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
	p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}}
	out, _, _, err := assembleBuildPolicy(p,
		selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{card})
	if err != nil {
		t.Fatalf("a 2-of-2 whose cosigner card declares %q was refused: %v", card.Path, err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(out)
	if err != nil {
		t.Fatalf("the assembled md1 does not decode: %v", err)
	}
	if got := keys[1].OriginPath.String(); got != card.Path {
		t.Errorf("@1 declares %s but its card says %s. Stamping the flow's origin over "+
			"a card that says otherwise puts a derivation path on steel that the card "+
			"itself disagrees with", got, card.Path)
	}
	if keys[0].OriginPath.String() != multisigSharedOrigin().String() {
		t.Errorf("@0, the operator's own derived slot, declares %s, want %s",
			keys[0].OriginPath, multisigSharedOrigin())
	}
}

// TestLegDerivedAtHeldSlotOrigin is plan test 3, C2's FIRST scenario: a slot
// held at m/48'/0'/1'/2' produces an mk1 derived THERE, not at
// multisigSharedOrigin().
//
// The mk1's key is asserted to be one the DESCRIPTOR contains, which is the
// binding that matters: an mk1 derived at the wrong origin is a key card
// asserting membership in a wallet whose slot holds a different key, and the
// operator finds out when they try to spend.
func TestLegDerivedAtHeldSlotOrigin(t *testing.T) {
	p, reg, sources, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(out)
	if err != nil {
		t.Fatalf("the assembled md1 does not decode: %v", err)
	}

	legs, _, _, err := buildEngraveTail(sources, p.Script, reg, s5Net, cards, out, false)
	if err != nil {
		t.Fatalf("buildEngraveTail: %v", err)
	}
	if len(legs) != 3 {
		t.Fatalf("Trace B derived %d leg(s), want 3", len(legs))
	}

	// @1 is the held slot at account 1. Its mk1 must declare m/48h/0h/1h/2h AND
	// carry the key that lives there.
	const wantOrigin = "m/48h/0h/1h/2h"
	card, err := mk.Decode(legs[1].MK1)
	if err != nil {
		t.Fatalf("the @1 leg's mk1 does not decode: %v", err)
	}
	if card.Path != wantOrigin {
		t.Errorf("the @1 leg's mk1 declares origin %s, want %s. Deriving every leg at "+
			"the shared origin is exactly what this test exists to catch",
			card.Path, wantOrigin)
	}
	cc, pk, _, err := decodeXpubBytes(card.Xpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes(mk1 @1): %v", err)
	}
	var got [65]byte
	copy(got[0:32], cc[:])
	copy(got[32:65], pk[:])
	if got != keys[1].Xpub {
		t.Errorf("the @1 leg's key is not the key the descriptor holds at @1")
	}
	// NON-VACUITY: it must not be @0's key either, or "the descriptor contains it"
	// would be satisfied by a leg derived at the shared origin.
	if got == keys[0].Xpub {
		t.Errorf("the @1 leg carries @0's key, so it was derived at the shared origin " +
			"and merely landed on a slot the descriptor also contains")
	}
}

// TestOneMk1PerHeldSlot is plan test 4: cardinality, ruled in §4.1a item 2.
//
// A held slot the operator cannot engrave a key card for is a slot they cannot
// prove membership of, so the count is exact in both directions: not fewer (a
// missing leg) and not more (a plate for a slot they do not hold).
func TestOneMk1PerHeldSlot(t *testing.T) {
	p, reg, sources, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	legs, _, cardsOut, err := buildEngraveTail(sources, p.Script, reg, s5Net, cards, out, false)
	if err != nil {
		t.Fatalf("buildEngraveTail: %v", err)
	}

	held := 0
	for _, s := range sources {
		if s.Kind == slotFromSeed || s.Kind == slotFromBoth {
			held++
		}
	}
	if held != 3 {
		t.Fatalf("Trace B's sources hold %d slot(s), want 3", held)
	}
	if len(legs) != held {
		t.Errorf("%d leg(s) for %d held slot(s)", len(legs), held)
	}
	mk1s := 0
	for _, c := range cardsOut {
		if c.kind == cardMK1 {
			mk1s++
		}
	}
	if mk1s != held {
		t.Errorf("the engrave set carries %d mk1 card(s) for %d held slot(s); a held "+
			"slot with no key card is a slot the operator cannot prove", mk1s, held)
	}
	// Every mk1 is DISTINCT: three copies of one leg would satisfy a count.
	seen := map[string]int{}
	for i, l := range legs {
		key := strings.Join(l.MK1, "|")
		if prev, dup := seen[key]; dup {
			t.Errorf("legs %d and %d are byte-identical mk1s, so the count is met by "+
				"repeating one leg", prev, i)
		}
		seen[key] = i
	}
}

// TestFullModeEngravesMs1ForEveryMaster is plan test 5, C2's SECOND scenario: a
// 3-of-4 across masters A and B in FULL mode engraves BOTH ms1s.
//
// The spec permits a refusal arm; the plan picks the engrave-both arm, because a
// test asserting a disjunction passes on either branch and so cannot be
// mutation-checked. Losing B otherwise leaves two legs against k=3: unspendable,
// from a backup labelled "Full (seed + keys)".
//
// EACH ms1 IS DECODED AND ITS ENTROPY COMPARED TO THE MASTER IT CLAIMS. "Two ms1
// plates were cut" is satisfied by two copies of master A's seed, which is the
// exact bug this test exists to catch and which passes every other gate in the
// plan.
func TestFullModeEngravesMs1ForEveryMaster(t *testing.T) {
	p, reg, sources, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	_, _, cardsOut, err := buildEngraveTail(sources, p.Script, reg, s5Net, cards, out, true)
	if err != nil {
		t.Fatalf("buildEngraveTail(full): %v", err)
	}

	var ms1s []string
	for _, c := range cardsOut {
		if c.kind == cardMS1 {
			if len(c.strings) != 1 {
				t.Fatalf("an ms1 card carries %d string(s), want 1", len(c.strings))
			}
			ms1s = append(ms1s, c.strings[0])
		}
	}
	if len(ms1s) != 2 {
		t.Fatalf("a FULL build across masters A and B engraved %d ms1 plate(s), want 2. "+
			"A backup labelled \"Full (seed + keys)\" that is missing a master leaves "+
			"two legs against k=3: unspendable", len(ms1s))
	}

	// THE ORDER IS A CONTRACT: oracle.CheckArtifactShape requires every ms1, then
	// every mk1, then the md1, as consecutive runs.
	kinds := make([]bundleCardKind, len(cardsOut))
	for i, c := range cardsOut {
		kinds[i] = c.kind
	}
	wantKinds := []bundleCardKind{cardMS1, cardMS1, cardMK1, cardMK1, cardMK1, cardMD1}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("the engrave set is %d card(s), want %d (2 ms1 + 3 mk1 + 1 md1)",
			len(kinds), len(wantKinds))
	}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("engrave card %d is kind %v, want %v; the oracle's artifact shape "+
				"requires all ms1s, then all mk1s, then the md1", i, kinds[i], wantKinds[i])
		}
	}

	// EACH ms1's ENTROPY, against the master it claims. Both masters must be
	// present and neither may appear twice.
	wantEntropy := map[string]string{
		"A": s5Entropy(t, fixtureMasterA),
		"B": s5Entropy(t, fixtureMasterB),
	}
	got := map[string]bool{}
	for i, s := range ms1s {
		ent := s5DecodeMS1(t, s)
		matched := ""
		for name, want := range wantEntropy {
			if ent == want {
				matched = name
			}
		}
		if matched == "" {
			t.Fatalf("ms1 plate %d carries entropy %s, which is neither master A's (%s) "+
				"nor master B's (%s)", i, ent, wantEntropy["A"], wantEntropy["B"])
		}
		if got[matched] {
			t.Fatalf("master %s's seed was engraved TWICE and the other master not at "+
				"all: a \"Full\" backup with a master missing", matched)
		}
		got[matched] = true
	}
	if !got["A"] || !got["B"] {
		t.Fatalf("engraved masters %v, want both A and B", got)
	}
	t.Logf("full Trace B: %d ms1 + %d mk1 + 1 md1 = %d cards, both masters present",
		len(ms1s), len(cardsOut)-len(ms1s)-1, len(cardsOut))
}

// s5Entropy is a mnemonic's entropy as hex, for comparing against a decoded ms1.
func s5Entropy(t *testing.T, phrase string) string {
	t.Helper()
	m, err := bip39.ParseMnemonic(phrase)
	if err != nil {
		t.Fatalf("ParseMnemonic(%.20s...): %v", phrase, err)
	}
	if !m.Valid() {
		t.Fatalf("%.20s... is not a valid mnemonic", phrase)
	}
	return hexOf(m.Entropy())
}

// s5DecodeMS1 decodes an engraved ms1 back to its entropy, as hex. The plates
// are read, not trusted: comparing the string this test just produced against
// itself would prove nothing.
func s5DecodeMS1(t *testing.T, s string) string {
	t.Helper()
	cs, err := codex32.New(s)
	if err != nil {
		t.Fatalf("codex32.New(%q): %v", s, err)
	}
	_, _, entropy, err := codex32.DecodeMS1(cs)
	if err != nil {
		t.Fatalf("DecodeMS1(%q): %v", s, err)
	}
	return hexOf(entropy)
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

// TestReRunMintsByteIdenticalPlates is plan test 7's ASSERTION half: re-running
// the same inputs mints byte-identical plates.
//
// It is the designed answer to interruption. Trace B's tail is 3-4 plates over
// hours (6-9 before F-423's packing), nothing records which were cut, and a
// power loss loses that state.
// Recovery is possible only because the encoders are deterministic -- no `rand`
// in md/, mk/ or codex32/, and mk's chunk_set_id derives from the bytecode
// rather than randomness -- so the operator re-runs and re-cuts only what is
// missing. That property is load-bearing and was pinned by nothing.
func TestReRunMintsByteIdenticalPlates(t *testing.T) {
	p, reg, sources, self, cards := s5TraceB(t)
	first, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	second, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not re-assemble: %v", err)
	}
	// THE POLICY ITSELF re-mints identically, which is the half the operator
	// re-runs from: a second run that produced a different md1 would be a
	// different wallet, not a resumed engrave.
	if strings.Join(first, "|") != strings.Join(second, "|") {
		t.Fatalf("two assemblies of the SAME inputs produced different md1 chunks")
	}

	runTail := func() []bundleCard {
		t.Helper()
		_, _, out, terr := buildEngraveTail(sources, p.Script, reg, s5Net, cards, first, true)
		if terr != nil {
			t.Fatalf("buildEngraveTail: %v", terr)
		}
		return out
	}
	a, b := runTail(), runTail()
	if len(a) != len(b) {
		t.Fatalf("a re-run produced %d card(s) against %d; an interrupted operator "+
			"cannot tell which plates are still missing", len(b), len(a))
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].label != b[i].label {
			t.Fatalf("card %d changed identity between runs: %v/%q then %v/%q",
				i, a[i].kind, a[i].label, b[i].kind, b[i].label)
		}
		if len(a[i].strings) != len(b[i].strings) {
			t.Fatalf("card %d is %d plate(s) then %d", i, len(a[i].strings), len(b[i].strings))
		}
		for j := range a[i].strings {
			if a[i].strings[j] != b[i].strings[j] {
				t.Fatalf("card %d plate %d differs between two runs of the SAME inputs:\n"+
					" first %s\nsecond %s\nThe recovery procedure this stage documents "+
					"rests on the encoders being deterministic", i, j, a[i].strings[j], b[i].strings[j])
			}
		}
	}
}

// TestAssembleBuildPolicyStaysSharedWhenOriginsAgree is the other side of the
// origin-mode rule, and it is what keeps S2's committed byte-identity golden
// meaningful.
//
// OriginShared and OriginDivergent are DIFFERENT WIRE FORMS of the same set of
// paths, so a build that switched to divergent unconditionally would mint
// different bytes for Trace A -- a policy every existing coordinator comparison
// and the pinned `md` golden would then disagree with. The rule is: divergent
// only when the origins actually differ.
func TestAssembleBuildPolicyStaysSharedWhenOriginsAgree(t *testing.T) {
	selfXpub, selfFP := dupTestSelf(t, fixtureMasterA)
	cards := []mk.Card{dupTestCard(t, 1), dupTestCard(t, 2)} // B@0, C@0, both at the shared origin
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{0}}
	out, _, _, err := assembleBuildPolicy(p,
		selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), cards)
	if err != nil {
		t.Fatalf("Trace A did not assemble: %v", err)
	}
	golden, _, _ := s2TraceAPolicy(t)
	if len(out) != len(golden) {
		t.Fatalf("Trace A assembled %d chunk(s), want %d", len(out), len(golden))
	}
	for i := range golden {
		if out[i] != golden[i] {
			t.Fatalf("Trace A chunk %d changed:\n got %s\nwant %s\nAll three slots declare "+
				"one origin, so the policy must stay OriginShared; switching to divergent "+
				"here would re-mint every shipped wallet id", i, out[i], golden[i])
		}
	}

	// PERMISSIVE ON SPELLING, STRICT ON VALUE (§0.1). m/48'/0'/0'/2' and
	// m/48h/0h/0h/2h are the same path, so a card spelling it with apostrophes
	// must not tip the policy into divergent mode over notation.
	for _, spelling := range []string{"m/48h/0h/0h/2h", "m/48'/0'/0'/2'"} {
		spelt := cards[0]
		spelt.Path = spelling
		got, _, _, serr := assembleBuildPolicy(
			buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}},
			selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{spelt})
		if serr != nil {
			t.Fatalf("a card declaring %q was refused: %v", spelling, serr)
		}
		ref, _, _, rerr := assembleBuildPolicy(
			buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}},
			selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{cards[0]})
		if rerr != nil {
			t.Fatalf("the reference build was refused: %v", rerr)
		}
		if strings.Join(got, "|") != strings.Join(ref, "|") {
			t.Errorf("spelling the shared origin %q changed the assembled bytes; that is "+
				"a difference over NOTATION, which §0.1 forbids", spelling)
		}
	}
}

// TestDerivedSlotOriginIsTemplateAware is plan §0.1a, from S5: the BIP-48 SCRIPT
// TYPE component follows the template.
//
// Measured against the BIP through the pinned oracle (plan §0.1a): BIP-48
// assigns 2' to native segwit and 1' to nested segwit, and registers NOTHING for
// legacy sh. Before S5 the device stamped 2' on all three, so an sh(wsh) build
// put the native-segwit path on steel.
func TestDerivedSlotOriginIsTemplateAware(t *testing.T) {
	for _, tc := range []struct {
		script md.MultisigScript
		want   string
		why    string
	}{
		{md.MultisigWsh, "m/48h/0h/0h/2h", "BIP-48's recommended default for native segwit"},
		{md.MultisigShWsh, "m/48h/0h/0h/1h", "BIP-48's assignment for nested segwit"},
		{md.MultisigSh, "m/48h/0h/0h/2h", "no BIP assigns legacy P2SH a path; this device's convention"},
	} {
		if got := derivedSlotOrigin(tc.script, 0).String(); got != tc.want {
			t.Errorf("derivedSlotOrigin(%v, 0) = %s, want %s (%s)", tc.script, got, tc.want, tc.why)
		}
	}
	// The account component still moves independently of the script type, or the
	// two bindings have been collapsed into one.
	if got := derivedSlotOrigin(md.MultisigShWsh, 3).String(); got != "m/48h/0h/3h/1h" {
		t.Errorf("derivedSlotOrigin(sh(wsh), 3) = %s, want m/48h/0h/3h/1h", got)
	}
	// NON-VACUITY: nested segwit must differ from the other two, or the switch is
	// satisfied by a constant.
	if derivedSlotOrigin(md.MultisigShWsh, 0).String() == derivedSlotOrigin(md.MultisigWsh, 0).String() {
		t.Error("nested segwit and native segwit derive at the same path, so the " +
			"template-awareness is not there")
	}
}

// TestTwoHeldSlotsFromOneMasterDoNotCollide is the argument that makes Trace B
// buildable at all, asserted rather than assumed.
//
// SPEC §4.1 refuses a policy whose slots hold the same key, permanently and
// source-independently. Two slots held from ONE master would be exactly that
// unless they derive at DIFFERENT accounts -- so the account numbering
// buildSlotSources performs is a funds-safety mechanism, not bookkeeping, and it
// is keyed on the MASTER FINGERPRINT so that one master registered twice still
// gets two accounts.
func TestTwoHeldSlotsFromOneMasterDoNotCollide(t *testing.T) {
	// The registry the flow builds when the operator types master A for BOTH held
	// slots: two ENTRIES, one master.
	reg, ids := s5Registry(t, fixtureMasterA, fixtureMasterA)
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{0, 1}}
	sources := buildSlotSources(p, ids, []int{1}, reg)
	if sources[0].Account == sources[1].Account {
		t.Fatalf("both held slots were assigned account %d; one master at one account "+
			"twice is ONE key, and §4.1 refuses it", sources[0].Account)
	}
	self, err := buildSelfKeys(sources, p.Script, reg, s5Net)
	if err != nil {
		t.Fatalf("buildSelfKeys: %v", err)
	}
	if len(self) != 2 {
		t.Fatalf("derived %d held key(s), want 2", len(self))
	}
	if self[0].Xpub == self[1].Xpub {
		t.Fatal("both held slots derived the SAME key")
	}
	if _, _, _, err := assembleBuildPolicy(p, self, []mk.Card{dupTestCard(t, 1)}); err != nil {
		t.Fatalf("a legitimate multi-account wallet was refused: %v.\nThis is the wallet "+
			"SPEC 4.1 exists to make buildable", err)
	}
}

// TestDepthZeroCosignerCardIsNamedRefusal is plan test 6 and spec M-1: a
// cosigner card declaring Path == "m" is refused by a NAMED screen, not by a
// fall-through "Couldn't assemble the wallet policy."
//
// THE PREMISE IS CHECKED, NOT ASSUMED. "m" parses to a ZERO-component path, and
// md.EncodeMultisig refuses a divergent-origin request holding an empty Origin
// (md/encode_multisig.go:104-106). The first subtest proves that at the md
// boundary directly, so the gui-level refusal below is attributing a real
// encoder refusal rather than pre-empting one that might not exist.
//
// The generic text describes a DEVICE problem and offers the operator nothing to
// do. A depth-0 card is their INPUT, and there is exactly one thing to do about
// it, so the screen says which slot and what to do.
func TestDepthZeroCosignerCardIsNamedRefusal(t *testing.T) {
	// The encoder's own refusal, at the md boundary. md keeps the error
	// unexported and this plan does not change md/, so the assertion is on its
	// text -- which is why the file:line above is cited.
	t.Run("md refuses a divergent request with an empty origin", func(t *testing.T) {
		req := md.EncodeMultisigRequest{
			Cosigners: []md.MultisigCosigner{
				{Origin: originComponents(multisigSharedOrigin())},
				{}, // no Origin at all: the depth-0 card
			},
			K:          1,
			Script:     md.MultisigWsh,
			OriginMode: md.OriginDivergent,
		}
		_, _, _, err := md.EncodeMultisig(req)
		if err == nil {
			t.Fatal("md.EncodeMultisig accepted a divergent request with an empty Origin, " +
				"so this test's premise is gone and the refusal below pre-empts nothing")
		}
		if !strings.Contains(err.Error(), "non-empty Origin") {
			t.Fatalf("md refused with %q; expected the OriginDivergent empty-origin error "+
				"(md/encode_multisig.go:104-106)", err)
		}
	})

	// And through the build path, where the refusal must be NAMED and must name
	// the slot.
	depthZero := dupTestCard(t, 1) // B@0, re-declared at depth 0
	depthZero.Path = "m"
	if p, err := bip32.ParsePath(depthZero.Path); err != nil || len(p) != 0 {
		t.Fatalf("ParsePath(%q) = %v, %v; this test needs it to parse to ZERO components",
			depthZero.Path, p, err)
	}
	selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
	p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}}
	out, _, _, err := assembleBuildPolicy(p,
		selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{depthZero})
	var eo errBuildEmptyOrigin
	if !errors.As(err, &eo) {
		t.Fatalf("a card declaring %q assembled or failed generically (err = %v); "+
			"spec M-1 requires a NAMED refusal", depthZero.Path, err)
	}
	if eo.Slot != 1 {
		t.Errorf("the refusal blames slot @%d; the depth-0 card fills @1 here", eo.Slot)
	}
	if out != nil {
		t.Errorf("md1 chunks were produced alongside the refusal: %v", out)
	}

	// The operator-facing text: which slot, what the card says, that nothing was
	// cut, and the routes that exist on this hardware (§0.1b -- the payload and
	// the keyboard, never "scan a card").
	msg := buildEmptyOriginMessage(eo, buildCosignerOrigins(p, []int{0}))
	for _, want := range []string{"@1", "Nothing was engraved", "me sysw pack"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the depth-0 refusal is missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "scan") || strings.Contains(msg, "Scan") {
		t.Errorf("the refusal tells the operator to scan, which is not the route this "+
			"phase offers:\n%s", msg)
	}
	if strings.ContainsAny(msg, "\u2014\u2013") {
		t.Errorf("the refusal carries an em- or en-dash, a zero-pixel glyph in the body "+
			"face, so the line does not draw:\n%s", msg)
	}
}
