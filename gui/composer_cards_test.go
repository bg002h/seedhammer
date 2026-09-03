package gui

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// composerCardFixture is a two-slot wsh policy with REAL xpubs seated: the
// smallest composition that can be minted into cards and seated back.
//
// Both slots carry master fingerprint 73c5da0a at DIFFERENT accounts, which is
// C5's normal case (one person in two paths holds two accounts) and is the
// case §4f's invariant permits -- distinct origins, so no fingerprint
// ambiguity arises.
func composerCardFixture(t *testing.T) (st *composerState, template, keyed []string) {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
	}}
	xpubs := []string{composerTestXpubA, composerTestXpubB}
	fp := [4]byte{0x73, 0xc5, 0xda, 0x0a}

	declared := make([]*md.SlotOrigin, len(xpubs))
	st = &composerState{list: list, reg: &seedRegistry{}}
	st.assigned = make([]composerAssignment, len(xpubs))
	st.sources = make([]composerSource, len(xpubs))
	pub := map[uint8][65]byte{}
	fps := map[uint8][4]byte{}
	for i, x := range xpubs {
		origin := composerTestOrigin(2, uint32(i))
		declared[i] = &md.SlotOrigin{Origin: origin, Fingerprint: fp, FpPresent: true}
		st.sources[i] = composerSource{kind: composerSourceKey, seedID: -1, xpub: x}
		st.assigned[i] = composerAssignment{
			src: i, account: uint32(i), origin: origin,
			fingerprint: fp, fpPresent: true, xpub: x,
		}
		cc, pk, _, err := decodeXpubBytes(x)
		if err != nil {
			t.Fatalf("decodeXpubBytes(%s): %v", x[:12], err)
		}
		var b [65]byte
		copy(b[0:32], cc[:])
		copy(b[32:65], pk[:])
		pub[uint8(i)] = b
		fps[uint8(i)] = fp
	}

	// COMPOSED TWICE, NOT COPIED. md.Composed's doc is explicit: a copy shares
	// the underlying descriptor, so Bind on one keys them both -- and a
	// "template" that had been keyed by its own policy would make every
	// assertion below tautological.
	ct, err := md.ComposeWith(list, declared)
	if err != nil {
		t.Fatalf("md.ComposeWith (template): %v", err)
	}
	if template, err = ct.Chunks(); err != nil {
		t.Fatal(err)
	}
	ck, err := md.ComposeWith(list, declared)
	if err != nil {
		t.Fatalf("md.ComposeWith (keyed): %v", err)
	}
	if err := ck.Bind(pub, fps); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if keyed, err = ck.Chunks(); err != nil {
		t.Fatal(err)
	}
	return st, template, keyed
}

// TestComposerMintCardCarriesBothStubsAndRoundTrips is §7d's minting rule: a
// key: record is MINTED as an mk1 carrying the fingerprint, the origin, the
// xpub and BOTH stubs, and mk.Encode is deterministic so a re-mint is exact.
func TestComposerMintCardCarriesBothStubsAndRoundTrips(t *testing.T) {
	st, template, keyed := composerCardFixture(t)
	strs, err := composerMintCard(st, 0, template, keyed)
	if err != nil {
		t.Fatalf("composerMintCard: %v", err)
	}
	card, err := mk.Decode(strs)
	if err != nil {
		t.Fatalf("the minted card does not decode: %v", err)
	}
	if card.Xpub != composerTestXpubA {
		t.Errorf("the card carries xpub %q, want the seated one", card.Xpub)
	}
	if card.Fingerprint != "73c5da0a" {
		t.Errorf("the card carries fingerprint %q, want 73c5da0a", card.Fingerprint)
	}
	// THE PATH IS COMPARED STRUCTURALLY, NOT AS A STRING, and that is the
	// lesson slotMatchesCard states in its own comment
	// (gui/key_card_seating.go:130-137): mk's codec round-trips the origin and
	// Decode renders it `m/48h/0h/0h/2h` where composerOriginText wrote
	// `m/48'/0'/0'/2'`. The same path in two notations. A string comparison
	// here would fail on a correct card, and a string comparison in the
	// seating would refuse every card while looking like corruption.
	gotPath, err := bip32.ParsePath(card.Path)
	if err != nil {
		t.Fatalf("the card's declared path %q does not parse: %v", card.Path, err)
	}
	wantPath := make(bip32.Path, 0, 4)
	for _, c := range composerTestOrigin(2, 0) {
		v := c.Value
		if c.Hardened {
			v += hdkeychain.HardenedKeyStart
		}
		wantPath = append(wantPath, v)
	}
	if len(gotPath) != len(wantPath) {
		t.Fatalf("the card declares %d components, want %d", len(gotPath), len(wantPath))
	}
	for i := range gotPath {
		if gotPath[i] != wantPath[i] {
			t.Errorf("the card declares %v, want %v (component %d)", gotPath, wantPath, i)
		}
	}
	// BOTH STUBS, in template-then-policy order (§7c "stamping BOTH stubs").
	want, err := md.ComposerStubs(template, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 2 {
		t.Fatalf("a template and a KEYED policy give %d stub(s), want 2 -- if these "+
			"two forms share a stub the whole re-mint story collapses", len(want))
	}
	if len(card.Stubs) != len(want) {
		t.Fatalf("the card carries %d stub(s), want %d", len(card.Stubs), len(want))
	}
	for i := range want {
		if card.Stubs[i] != want[i] {
			t.Errorf("stub %d is %x, want %x", i, card.Stubs[i], want[i])
		}
	}
}

// TestComposerMintCardWithNoKeyedPolicyCarriesTheTemplateStubAlone is §7f's
// PARTIALLY seated rule: the policy id does not exist until every slot is
// seated, so a card cut then must not claim one.
func TestComposerMintCardWithNoKeyedPolicyCarriesTheTemplateStubAlone(t *testing.T) {
	st, template, _ := composerCardFixture(t)
	strs, err := composerMintCard(st, 0, template, nil)
	if err != nil {
		t.Fatalf("composerMintCard: %v", err)
	}
	card, err := mk.Decode(strs)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Stubs) != 1 {
		t.Fatalf("a card cut before the policy exists carries %d stubs, want 1", len(card.Stubs))
	}
	tmplStub, err := md.FormAwareStubChunks(template)
	if err != nil {
		t.Fatal(err)
	}
	if card.Stubs[0] != tmplStub {
		t.Errorf("the single stub is %x, want the TEMPLATE's %x", card.Stubs[0], tmplStub)
	}
}

// TestComposerReMintPreservesExistingStubsInOrder is §7d's whole reason for
// APPENDING rather than replacing: a payload card stays indexed to the wallets
// it already belonged to. reStubMk1 (gui/template_engrave.go:41) REPLACES,
// which is right for its own flow and would be wrong here.
func TestComposerReMintPreservesExistingStubsInOrder(t *testing.T) {
	st, template, keyed := composerCardFixture(t)
	prior := [][4]byte{{1, 2, 3, 4}, {5, 6, 7, 8}}
	st.sources[0] = composerSource{
		kind: composerSourceCard, seedID: -1, xpub: composerTestXpubA,
		card: mk.Card{
			Network: "mainnet", Path: "m/48'/0'/0'/2'",
			Fingerprint: "73c5da0a", Stubs: prior, Xpub: composerTestXpubA,
		},
	}
	strs, err := composerMintCard(st, 0, template, keyed)
	if err != nil {
		t.Fatalf("composerMintCard: %v", err)
	}
	card, err := mk.Decode(strs)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Stubs) != 4 {
		t.Fatalf("a re-minted card carries %d stubs, want 4 (2 prior + 2 composed)", len(card.Stubs))
	}
	for i, s := range prior {
		if card.Stubs[i] != s {
			t.Errorf("prior stub %d moved: got %x, want %x", i, card.Stubs[i], s)
		}
	}
	// APPENDED ONCE: re-minting a card that already carries a composed stub
	// must not duplicate it, or the stub list grows on every pass and can
	// push the card into another chunk for nothing.
	again := mk.AppendStubs(card, card.Stubs[2], card.Stubs[3])
	if len(again.Stubs) != 4 {
		t.Errorf("re-appending stubs the card already carries grew it to %d", len(again.Stubs))
	}
}

// TestComposerMintCardsSkipsUnseatedSlotsAndNamesTheRest: an unseated slot has
// no card, and a refusal to mint one is what keeps the census honest about how
// many plates a partially seated composition cuts.
func TestComposerMintCardsSkipsUnseatedSlotsAndNamesTheRest(t *testing.T) {
	st, template, keyed := composerCardFixture(t)
	st.assigned[1].src = -1
	cards, err := composerMintCards(st, template, keyed)
	if err != nil {
		t.Fatalf("composerMintCards: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("one seated slot of two produced %d card(s), want 1", len(cards))
	}
	if cards[0].kind != cardMK1 {
		t.Errorf("the minted card is kind %v, want cardMK1", cards[0].kind)
	}
	if cards[0].label != "mk1 key @0" {
		t.Errorf("the card is labelled %q, want \"mk1 key @0\"", cards[0].label)
	}
	if _, err := composerMintCard(st, 1, template, keyed); err == nil {
		t.Error("minting an UNSEATED slot succeeded; a card for a slot holding no key " +
			"is a plate that vouches for a wallet nobody composed")
	}
}
