package gui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// F-216: seating gathered mk1 cards onto a keyless template.
//
// The fixture is built from a KEYED conformance vector rather than typed: strip
// it to its template (that is the card an operator engraved), then mint one mk1
// per slot carrying that slot's real xpub, origin and fingerprint, stubbed on
// the TEMPLATE id. Everything then has Rust-derived ground truth behind it.

const seatVector = "keyed_tr_with_leaf"

// seatFixture returns (templateCards, keyCards, expectedReceive0).
func seatFixture(t *testing.T) ([]string, []mk.Card, string) {
	t.Helper()
	keyed := loadVectorChunks(t, seatVector)
	tmpl, err := md.StripToTemplate(keyed)
	if err != nil {
		t.Fatalf("StripToTemplate: %v", err)
	}
	stub, err := md.FormAwareStubChunks(tmpl)
	if err != nil {
		t.Fatalf("FormAwareStubChunks: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(keyed)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "vectors", seatVector+".conformance.json"))
	if err != nil {
		t.Fatalf("read vector: %v", err)
	}
	var rec struct {
		Keys []struct {
			Index int    `json:"index"`
			Xpub  string `json:"xpub"`
		} `json:"keys"`
		Chains map[string]struct {
			Addresses []string `json:"addresses"`
		} `json:"chains"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse vector: %v", err)
	}
	xpubOf := map[int]string{}
	for _, k := range rec.Keys {
		xpubOf[k.Index] = k.Xpub
	}

	cards := make([]mk.Card, 0, len(keys))
	for _, k := range keys {
		xp, ok := xpubOf[int(k.Index)]
		if !ok {
			t.Fatalf("vector has no xpub for @%d", k.Index)
		}
		fp := ""
		if k.FingerprintPresent {
			fp = hexString(k.Fingerprint[:])
		}
		cards = append(cards, mk.Card{
			Network:     "mainnet",
			Path:        k.OriginPath.String(),
			Fingerprint: fp,
			Stubs:       [][4]byte{stub},
			Xpub:        xp,
		})
	}
	return tmpl, cards, rec.Chains["0"].Addresses[0]
}

// The seated template must derive what Rust derived from the FULL policy card.
// That is the whole claim: seating reconstructs the same wallet.
func TestSeatedTemplateDerivesTheVectorAddress(t *testing.T) {
	tmpl, cards, want := seatFixture(t)
	seated, err := seatKeyCards(tmpl, cards)
	if err != nil {
		t.Fatalf("seating refused a complete, correct gather: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("the seated template still cannot derive an address")
	}
	got, err := at(0, false)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got != want {
		t.Fatalf("seated wallet derives the wrong address:\n got  %s\n want %s (rust)", got, want)
	}
}

// A1 — ORDER-INVARIANCE. The operator's hands must not decide the mapping.
func TestSeatingIsIndependentOfGatherOrder(t *testing.T) {
	tmpl, cards, want := seatFixture(t)
	if len(cards) < 2 {
		t.Fatal("fixture needs >= 2 cards, or order cannot vary")
	}
	rev := make([]mk.Card, len(cards))
	for i := range cards {
		rev[i] = cards[len(cards)-1-i]
	}
	seated, err := seatKeyCards(tmpl, rev)
	if err != nil {
		t.Fatalf("reversed gather refused: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("reversed gather does not derive")
	}
	got, _ := at(0, false)
	if got != want {
		t.Fatalf("gather ORDER changed the wallet:\n got  %s\n want %s", got, want)
	}
}

// A5 — a card from another policy is refused at membership, and NOTHING is
// returned to derive from.
func TestACardFromAnotherPolicyIsRefused(t *testing.T) {
	tmpl, cards, _ := seatFixture(t)
	cards[0].Stubs = [][4]byte{{0xde, 0xad, 0xbe, 0xef}}
	seated, err := seatKeyCards(tmpl, cards)
	if !errors.Is(err, errSeatNotThisPolicy) {
		t.Fatalf("a foreign card was accepted; err = %v", err)
	}
	if seated != nil {
		t.Fatal("a refusal returned keys to derive from")
	}
}

// A5 — an incomplete gather refuses rather than deriving from what it has.
func TestAnIncompleteGatherRefuses(t *testing.T) {
	tmpl, cards, _ := seatFixture(t)
	seated, err := seatKeyCards(tmpl, cards[:1])
	if !errors.Is(err, errSeatSlotUnfilled) {
		t.Fatalf("a partial gather was accepted; err = %v", err)
	}
	if seated != nil {
		t.Fatal("a partial gather returned keys to derive from")
	}
}

// The undecidable state: two DIFFERENT cards claiming one slot.
func TestTwoDifferentCardsForOneSlotAreRefused(t *testing.T) {
	tmpl, cards, _ := seatFixture(t)
	// Give card 1 card 0's declaration but keep its own key.
	cards[1].Path = cards[0].Path
	cards[1].Fingerprint = cards[0].Fingerprint
	_, err := seatKeyCards(tmpl, cards)
	if !errors.Is(err, errSeatSlotContested) {
		t.Fatalf("a contested slot was silently resolved; err = %v", err)
	}
}

// Scanning the SAME card twice is not a contest — it is an operator scanning
// twice, and refusing it would be a false alarm on an ordinary mistake.
func TestTheSameCardTwiceIsNotAContest(t *testing.T) {
	tmpl, cards, want := seatFixture(t)
	dup := append(append([]mk.Card{}, cards...), cards[0])
	seated, err := seatKeyCards(tmpl, dup)
	if err != nil {
		t.Fatalf("a re-scanned card was treated as a contest: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("does not derive")
	}
	if got, _ := at(0, false); got != want {
		t.Fatalf("re-scanning changed the wallet: got %s want %s", got, want)
	}
}

// A card belonging to the policy but declaring an origin no slot names.
func TestACardMatchingNoSlotIsRefused(t *testing.T) {
	tmpl, cards, _ := seatFixture(t)
	cards[0].Path = "m/48'/0'/99'/2'"
	_, err := seatKeyCards(tmpl, cards)
	if !errors.Is(err, errSeatNoSlot) {
		t.Fatalf("a card matching no slot was accepted; err = %v", err)
	}
}

// THE NOTATION TRAP, pinned. bip32 renders `m/48h/…` and mk carries `m/48'/…`;
// a string comparison matches nothing and every card is refused, which reads as
// a corrupt card rather than a formatting mismatch.
func TestApostropheAndHNotationBothSeat(t *testing.T) {
	tmpl, cards, want := seatFixture(t)
	for i := range cards {
		p := []rune(cards[i].Path)
		for j, r := range p {
			if r == 'h' {
				p[j] = '\''
			}
		}
		cards[i].Path = string(p)
	}
	seated, err := seatKeyCards(tmpl, cards)
	if err != nil {
		t.Fatalf("apostrophe notation was refused: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("does not derive")
	}
	if got, _ := at(0, false); got != want {
		t.Fatalf("notation changed the wallet: got %s want %s", got, want)
	}
}

// THE FINGERPRINT CLAUSE, covered by the only shape that needs it.
//
// In the fixture above the two slots differ by ORIGIN, so origin alone
// disambiguates and ignoring the fingerprint changes nothing — a mutation that
// dropped the fingerprint check passed the whole suite.
//
// This policy seats TWO MASTERS AT ONE PATH: `@0` and `@1` both declare
// `48'/0'/0'/2'` and differ only in their declared fingerprint. That is legal
// (two people may both use the standard path — F-217 refuses one origin bound
// to two different keys only when the FINGERPRINT also matches), and it is the
// one arrangement where the fingerprint is load-bearing for seating.
func seatSameOriginFixture(t *testing.T) ([]string, []mk.Card, string) {
	t.Helper()
	const name = "seat_same_origin_two_masters"
	tmplCards := loadVectorChunks(t, name)
	stub, err := md.FormAwareStubChunks(tmplCards)
	if err != nil {
		t.Fatalf("stub: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(tmplCards)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "vectors", name+".conformance.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var rec struct {
		Keys []struct {
			Index int    `json:"index"`
			Xpub  string `json:"xpub"`
		} `json:"keys"`
		Chains map[string]struct {
			Addresses []string `json:"addresses"`
		} `json:"chains"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// ALREADY a keyless template, and one that DECLARES fingerprints — which is
	// the whole point. `StripToTemplate` drops fingerprints along with the keys
	// (deliberately: a fingerprint identifies a master, so it is exactly the
	// key-identifying material a template-only engrave omits). A stripped
	// template therefore CANNOT seat two masters at one path, and refuses; only
	// a template encoded with `--fingerprint` and no `--key` can.
	tmpl := tmplCards
	xp := map[int]string{}
	for _, k := range rec.Keys {
		xp[k.Index] = k.Xpub
	}
	var cards []mk.Card
	for _, k := range keys {
		if !k.FingerprintPresent {
			t.Fatalf("@%d has no declared fingerprint; this fixture depends on it", k.Index)
		}
		cards = append(cards, mk.Card{
			Network:     "mainnet",
			Path:        k.OriginPath.String(),
			Fingerprint: hexString(k.Fingerprint[:]),
			Stubs:       [][4]byte{stub},
			Xpub:        xp[int(k.Index)],
		})
	}
	return tmpl, cards, rec.Chains["0"].Addresses[0]
}

func TestFingerprintDisambiguatesTwoMastersAtOnePath(t *testing.T) {
	tmpl, cards, want := seatSameOriginFixture(t)
	// Non-vacuous: the two slots MUST share an origin, or this proves nothing.
	if cards[0].Path != cards[1].Path {
		t.Fatalf("fixture slots differ by origin (%s vs %s); the fingerprint is not load-bearing",
			cards[0].Path, cards[1].Path)
	}
	if cards[0].Fingerprint == cards[1].Fingerprint {
		t.Fatal("fixture slots share a fingerprint; nothing can disambiguate them")
	}
	seated, err := seatKeyCards(tmpl, cards)
	if err != nil {
		t.Fatalf("a correct two-master gather was refused: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("the seated policy does not derive")
	}
	got, err := at(0, false)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got != want {
		t.Fatalf("seating put the keys in the wrong slots:\n got  %s\n want %s (rust)", got, want)
	}
}

// And SWAPPING the two cards' fingerprints must change the wallet — which is
// what makes the test above a check on SEATING rather than on arithmetic.
//
// The fixture is `multi`, not `sortedmulti`, and that is not incidental:
// sortedmulti SORTS its keys before building the script, so which slot a key
// lands in does not move the address and a misseating is invisible to any
// address comparison. Measured — with the sortedmulti form, swapping the two
// fingerprints changed nothing at all. Any future cross-check of seating has to
// use an order-sensitive policy or it proves nothing.
func TestSwappingFingerprintsSeatsTheOtherWallet(t *testing.T) {
	tmpl, cards, want := seatSameOriginFixture(t)
	cards[0].Fingerprint, cards[1].Fingerprint = cards[1].Fingerprint, cards[0].Fingerprint
	seated, err := seatKeyCards(tmpl, cards)
	if err != nil {
		t.Fatalf("the swapped gather was refused rather than seated: %v", err)
	}
	at, ok := complexAddressSource(tmpl, seated)
	if !ok {
		t.Fatal("does not derive")
	}
	got, _ := at(0, false)
	if got == want {
		t.Fatal("swapping the fingerprints changed nothing — the fingerprint is not being used to seat")
	}
}

// THE ADMISSIBILITY BOUNDARY, pinned rather than discovered later.
//
// `StripToTemplate` drops the fingerprints along with the keys — deliberately,
// because a fingerprint identifies a master and is exactly the key-identifying
// material a template-only engrave exists to omit. So a STRIPPED template of a
// policy whose slots share an origin has nothing left to tell them apart, and
// the device must REFUSE rather than pick.
//
// This is the case an operator will actually hit, so the screen has to say it.
func TestAStrippedTemplateCannotSeatTwoMastersAtOnePath(t *testing.T) {
	// The keyed form of the same colliding-origin policy.
	keyedT := "wsh(multi(2,@0/48'/0'/0'/2'/<0;1>/*,@1/48'/0'/0'/2'/<0;1>/*))"
	_ = keyedT
	tmplWithFp := loadVectorChunks(t, "seat_same_origin_two_masters")
	_, keys, err := md.ExpandWalletPolicyChunks(tmplWithFp)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// Confirm the premise: this template DOES declare fingerprints.
	for _, k := range keys {
		if !k.FingerprintPresent {
			t.Fatalf("@%d has no fingerprint; the contrast this test draws does not exist", k.Index)
		}
	}
	// Now strip them away, as StripToTemplate would.
	stripped := make([]md.ExpandedKey, len(keys))
	copy(stripped, keys)
	for i := range stripped {
		stripped[i].FingerprintPresent = false
		stripped[i].Fingerprint = [4]byte{}
	}
	// Two cards at the one shared origin can no longer be told apart.
	stub, err := md.FormAwareStubChunks(tmplWithFp)
	if err != nil {
		t.Fatalf("stub: %v", err)
	}
	a := mk.Card{Path: keys[0].OriginPath.String(), Stubs: [][4]byte{stub}, Xpub: "x"}
	b := mk.Card{Path: keys[1].OriginPath.String(), Stubs: [][4]byte{stub}, Xpub: "y"}
	if a.Path != b.Path {
		t.Fatal("fixture slots do not share an origin")
	}
	// slotMatchesCard is the predicate under test; with no declared fingerprint
	// BOTH cards match BOTH slots, which is precisely the undecidable state.
	for si, slot := range stripped {
		for ci, c := range []mk.Card{a, b} {
			ok, err := slotMatchesCard(slot, c)
			if err != nil {
				t.Fatalf("match: %v", err)
			}
			if !ok {
				t.Fatalf("without a fingerprint, card %d should still match slot %d by origin", ci, si)
			}
		}
	}
}

// ─── D3's first half, through the CONSENT SURFACE ───────────────────────────
//
// The tests above exercise `seatKeyCards` directly. These go through
// `walletPolicyConsentLines`, which is what an operator actually sees — the
// join is where this kind of feature usually fails, with every component green.

func TestConsentSeatsKeyCardsAndShowsRustAddresses(t *testing.T) {
	tmpl, cards, want := seatFixture(t)
	lines, err := walletPolicyConsentLines(tmpl, cards)
	if err != nil {
		t.Fatalf("consent refused a complete, correct gather: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("the consent screen does not show the Rust-derived address %s:\n%s", want, joined)
	}
	for _, forbidden := range []string{"Keyless template - no addresses", "no addresses"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("the screen still says %q after a successful seating:\n%s", forbidden, joined)
		}
	}
}

// D3's SECOND half must survive: skipping the gather still reaches consent,
// without address proof. Adding the first half must not have removed it.
func TestConsentWithoutKeyCardsStillSaysKeyless(t *testing.T) {
	tmpl, _, _ := seatFixture(t)
	lines, err := walletPolicyConsentLines(tmpl, nil)
	if err != nil {
		t.Fatalf("a keyless template alone was refused: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "no addresses") {
		t.Fatalf("a keyless template with no cards should say so:\n%s", joined)
	}
}

// A FULL-POLICY card must not be re-seated. Seating over a card that already
// carries its keys would let a stray key card silently replace a declared one.
func TestConsentDoesNotSeatOverAFullPolicyCard(t *testing.T) {
	keyed := loadVectorChunks(t, seatVector)
	_, cards, want := seatFixture(t)
	// Hand the FULL policy card the key cards too; the policy's own keys win.
	lines, err := walletPolicyConsentLines(keyed, cards)
	if err != nil {
		t.Fatalf("a full policy card with key cards alongside was refused: %v", err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), want) {
		t.Fatal("the full-policy card's own addresses are not shown")
	}
}

// Every typed refusal must reach the operator as its OWN sentence. "Your cards
// were refused" is accurate, actionable by nobody, and indistinguishable from a
// broken device.
func TestEachSeatingRefusalHasItsOwnSentence(t *testing.T) {
	tmpl, cards, _ := seatFixture(t)

	foreign := append([]mk.Card{}, cards...)
	foreign[0].Stubs = [][4]byte{{0xde, 0xad, 0xbe, 0xef}}

	noSlot := append([]mk.Card{}, cards...)
	noSlot[0].Path = "m/48'/0'/99'/2'"

	for name, tc := range map[string]struct {
		cards []mk.Card
		want  string
	}{
		"not this policy": {foreign, "different stub"},
		"matches no slot": {noSlot, "matches no slot"},
		"incomplete":      {cards[:1], "no key card yet"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := walletPolicyConsentLines(tmpl, tc.cards)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the refusal does not say %q:\n%s", tc.want, err.Error())
			}
		})
	}
}
