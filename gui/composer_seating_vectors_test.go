package gui

import (
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// SPEC §12 item 6, as vectors: the cards the composer mints SEAT into the
// template it engraved, through the SHIPPED seatKeyCards, and reproduce the
// keyed policy's addresses.
//
// WHY THE SHIPPED FUNCTION AND NOT A LOCAL CHECK. seatKeyCards
// (gui/key_card_seating.go:53) is what a restoring operator's device runs
// years later, with none of this flow's state. If the composer's own idea of
// "these cards fit this template" and that function's ever disagree, the
// plates are a backup that only the machine that cut them can read. So the
// assertion is: hand the shipped seater the shipped artifacts, and require it
// to reach the same keys and the same addresses.

// composerVector is one named shape, with the xpubs to seat into it.
type composerVector struct {
	name  string
	list  md.PathList
	slots int
	// addrs says this row's addresses must derive on this device. MEASURED
	// true for all five, taproot included: complexAddressSource
	// (gui/policy_address.go:44) reaches the tr script-tree shape the composer
	// emits. The flag stays as a column rather than being dropped, because the
	// S2 implementation report names taproot address derivation as a gap and a
	// row that stops deriving must fail loudly rather than quietly skip.
	addrs bool
}

func composerSeatingVectors() []composerVector {
	return []composerVector{
		{"wsh_sole_sortedmulti_2of2", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		}}, 2, true},
		{"wsh_sole_multi_1of2_unsorted", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 2, Sorted: false}},
		}}, 2, true},
		{"wsh_two_paths_single_keys", md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}},
			{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 1000}},
		}}, 2, true},
		{"sh_wsh_sole_sortedmulti_2of2", md.PathList{Wrapper: md.ComposeShWsh, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		}}, 2, true},
		{"tr_extracted_first_then_leaf", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 1, N: 1}},
			{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 7}},
		}}, 2, true},
	}
}

// composerVectorXpub derives slot i's account key AT THE WRAPPER'S OWN §4f
// origin, from the published "abandon" vector, using the DEVICE's own
// deriveAccountXpub.
//
// DERIVED, NOT PINNED, and mk is what forces it: mk.Encode gates the card on
// the xpub's depth and last child matching the declared path
// ("mk: xpub depth/child does not match path"), so a taproot slot at
// m/48'/0'/i'/3' cannot carry a key derived at .../2'. Pinning two constants
// and reusing them across wrappers is exactly the mistake that gate exists to
// catch, and it caught it here.
func composerVectorXpub(t *testing.T, scriptType, account uint32) (string, [4]byte) {
	t.Helper()
	const h = hdkeychain.HardenedKeyStart
	path := bip32.Path{48 | h, 0 | h, account | h, scriptType | h}
	xpub, masterFP, err := deriveAccountXpub(composerTestMnemonic(t), "", &chaincfg.MainNetParams, path)
	if err != nil {
		t.Fatalf("deriveAccountXpub(%s): %v", path, err)
	}
	var fp [4]byte
	binary.BigEndian.PutUint32(fp[:], masterFP)
	return xpub, fp
}

// composerVectorArtifacts composes the keyless template and the keyed policy
// from ONE path list, and returns the state whose assignments describe it.
//
// COMPOSED TWICE, NOT COPIED: md.Composed's doc says a copy shares the
// underlying descriptor, so Bind on one keys them both -- and a "template"
// that had been keyed by its own policy would make every assertion here
// tautological.
func composerVectorArtifacts(t *testing.T, v composerVector) (st *composerState, template, keyed []string) {
	t.Helper()
	n := v.slots
	declared := make([]*md.SlotOrigin, n)
	st = &composerState{list: v.list, reg: &seedRegistry{}}
	st.assigned = make([]composerAssignment, n)
	st.sources = make([]composerSource, n)
	pub := map[uint8][65]byte{}
	fps := map[uint8][4]byte{}
	scriptType := uint32(2)
	if v.list.Wrapper == md.ComposeTr {
		scriptType = 3
	}
	for i := 0; i < n; i++ {
		x, fp := composerVectorXpub(t, scriptType, uint32(i))
		origin := composerTestOrigin(scriptType, uint32(i))
		declared[i] = &md.SlotOrigin{Origin: origin, Fingerprint: fp, FpPresent: true}
		st.sources[i] = composerSource{kind: composerSourceKey, seedID: -1, xpub: x}
		st.assigned[i] = composerAssignment{
			src: i, account: uint32(i), origin: origin,
			fingerprint: fp, fpPresent: true, xpub: x,
		}
		cc, pk, _, err := decodeXpubBytes(x)
		if err != nil {
			t.Fatalf("%s: decodeXpubBytes: %v", v.name, err)
		}
		var b [65]byte
		copy(b[0:32], cc[:])
		copy(b[32:65], pk[:])
		pub[uint8(i)] = b
		fps[uint8(i)] = fp
	}
	ct, err := md.ComposeWith(v.list, declared)
	if err != nil {
		t.Fatalf("%s: md.ComposeWith (template): %v", v.name, err)
	}
	if template, err = ct.Chunks(); err != nil {
		t.Fatalf("%s: template Chunks: %v", v.name, err)
	}
	ck, err := md.ComposeWith(v.list, declared)
	if err != nil {
		t.Fatalf("%s: md.ComposeWith (keyed): %v", v.name, err)
	}
	if err := ck.Bind(pub, fps); err != nil {
		t.Fatalf("%s: Bind: %v", v.name, err)
	}
	if keyed, err = ck.Chunks(); err != nil {
		t.Fatalf("%s: keyed Chunks: %v", v.name, err)
	}
	return st, template, keyed
}

func composerDecodeCards(t *testing.T, st *composerState, template, keyed []string) []mk.Card {
	t.Helper()
	var cards []mk.Card
	for i := range st.assigned {
		if st.assigned[i].src < 0 {
			continue
		}
		strs, err := composerMintCard(st, uint8(i), template, keyed)
		if err != nil {
			t.Fatalf("composerMintCard(@%d): %v", i, err)
		}
		c, err := mk.Decode(strs)
		if err != nil {
			t.Fatalf("the card minted for @%d does not decode: %v", i, err)
		}
		cards = append(cards, c)
	}
	return cards
}

// TestComposerMintedCardsSeatThroughTheShippedSeater is §12 item 6's positive
// leg, over every named vector.
func TestComposerMintedCardsSeatThroughTheShippedSeater(t *testing.T) {
	for _, v := range composerSeatingVectors() {
		t.Run(v.name, func(t *testing.T) {
			st, template, keyed := composerVectorArtifacts(t, v)
			cards := composerDecodeCards(t, st, template, keyed)
			if len(cards) != v.slots {
				t.Fatalf("minted %d card(s) for %d seated slot(s)", len(cards), v.slots)
			}
			seated, err := seatKeyCards(template, cards)
			if err != nil {
				t.Fatalf("the SHIPPED seater refuses the composer's own cards: %v", err)
			}
			_, want, err := md.ExpandWalletPolicyChunks(keyed)
			if err != nil {
				t.Fatal(err)
			}
			if len(seated) != len(want) {
				t.Fatalf("seating gave %d slot(s), the keyed policy has %d", len(seated), len(want))
			}
			for i := range seated {
				if !seated[i].XpubPresent {
					t.Errorf("slot @%d came back from seating with no key", seated[i].Index)
					continue
				}
				if seated[i].Xpub != want[i].Xpub {
					t.Errorf("slot @%d seated a different key than the keyed policy holds",
						seated[i].Index)
				}
			}
		})
	}
}

// TestComposerSeatedTemplateReproducesTheKeyedPolicysAddresses is the half
// that matters to an operator: the plates derive the same wallet.
func TestComposerSeatedTemplateReproducesTheKeyedPolicysAddresses(t *testing.T) {
	derived := 0
	for _, v := range composerSeatingVectors() {
		t.Run(v.name, func(t *testing.T) {
			st, template, keyed := composerVectorArtifacts(t, v)
			cards := composerDecodeCards(t, st, template, keyed)
			seated, err := seatKeyCards(template, cards)
			if err != nil {
				t.Fatalf("seatKeyCards: %v", err)
			}
			tplT, _, err := md.ExpandWalletPolicyChunks(template)
			if err != nil {
				t.Fatal(err)
			}
			tplK, keysK, err := md.ExpandWalletPolicyChunks(keyed)
			if err != nil {
				t.Fatal(err)
			}
			atSeated, okSeated := policyAddressAt(template, tplT, seated)
			atKeyed, okKeyed := policyAddressAt(keyed, tplK, keysK)
			if okSeated != okKeyed {
				t.Fatalf("addresses derive from one form and not the other "+
					"(seated=%v keyed=%v); the two artifacts would then prove "+
					"different things", okSeated, okKeyed)
			}
			if !okKeyed {
				if v.addrs {
					t.Fatalf("this vector is marked address-checkable and no address " +
						"derives; either the row or the deriver is wrong")
				}
				t.Logf("%s: no address derives for this shape on this device "+
					"(the taproot script-tree gap); the ids still bind it", v.name)
				return
			}
			derived++
			for i := 0; i < addrProofPerChain; i++ {
				for _, change := range []bool{false, true} {
					a, err := atSeated(uint32(i), change)
					if err != nil {
						t.Fatalf("seated address %d/%v: %v", i, change, err)
					}
					b, err := atKeyed(uint32(i), change)
					if err != nil {
						t.Fatalf("keyed address %d/%v: %v", i, change, err)
					}
					if a != b {
						t.Errorf("address %d (change=%v) differs: seated %s, keyed %s",
							i, change, a, b)
					}
				}
			}
		})
	}
	if derived == 0 {
		t.Fatal("INCONCLUSIVE: no vector derived an address, so this test compared nothing")
	}
}

// TestComposerPartiallySeatedArtifactIsANamedVector is §7f's partially seated
// form and §12 item 6's last clause: the template plus TEMPLATE-STUB cards for
// the seated slots only, and the shipped seater must REFUSE it as incomplete
// rather than deriving a wallet from half a key set.
func TestComposerPartiallySeatedArtifactIsANamedVector(t *testing.T) {
	v := composerSeatingVectors()[0]
	st, template, _ := composerVectorArtifacts(t, v)
	st.assigned[1].src = -1 // one slot left unseated

	cards := composerDecodeCards(t, st, template, nil)
	if len(cards) != 1 {
		t.Fatalf("a partially seated composition minted %d card(s), want 1", len(cards))
	}
	tmplStub, err := md.FormAwareStubChunks(template)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards[0].Stubs) != 1 || cards[0].Stubs[0] != tmplStub {
		t.Errorf("the card carries %d stub(s) (%x); a partially seated composition has "+
			"no policy id yet, so it must carry the TEMPLATE stub alone (%x)",
			len(cards[0].Stubs), cards[0].Stubs, tmplStub)
	}
	if _, err := seatKeyCards(template, cards); err == nil {
		t.Error("the shipped seater ACCEPTED a template with an unfilled slot; a partial " +
			"seating is exactly the state that produces a plausible wrong address")
	}
}

// TestComposerNeverProducesTheAsymmetricOneCardTemplate is §12 item 6's NAMED
// NEGATIVE: "a named negative vector runs the asymmetric one-card case (one
// slot with a fingerprint, one without, at one origin) and asserts it is never
// produced".
//
// IT IS REFUSED TWICE, AND THE SECOND REFUSAL IS THE STRONGER ONE. §8v catches
// it at the mapping review, where the operator can act on it; md.ComposeWith
// refuses to EMIT it at all, so the artifact cannot leave this device even if
// a future screen forgot to ask. Both are asserted, because a defence that
// exists only in the UI is a defence one refactor from being gone.
//
// WHAT THIS MEANS FOR THE HAZARD §4f NAMES. The plan's own framing was that
// the hand-built asymmetric template would be shown to double-seat through
// seatKeyCards (slotMatchesCard skips the fingerprint test when the template
// declares none, gui/key_card_seating.go:151-159). Measured here: that
// template cannot be built through md.ComposeWith at all, so the double-seat
// is unreachable through this device's own builder and the demonstration
// belongs to the seating package's own tests, not to the composer's. Said
// plainly rather than left as an assertion that quietly tests nothing.
func TestComposerNeverProducesTheAsymmetricOneCardTemplate(t *testing.T) {
	origin := composerTestOrigin(2, 0)
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
	}}

	// (a) THE REVIEW REFUSES IT (§8v). One fingerprint present, one absent, at
	//     one origin: the asymmetric case, which is the dangerous one.
	st := &composerState{list: list, assigned: []composerAssignment{
		{src: 0, origin: origin, fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, fpPresent: true},
		{src: 1, origin: origin},
	}}
	if !composerInvariantViolation(st) {
		t.Fatal("(a) the mapping review would ACCEPT two slots at one origin with one " +
			"fingerprint; §8v exists to refuse exactly this")
	}

	// (b) THE BUILDER REFUSES TO EMIT IT. Same shape, handed straight to
	//     md.ComposeWith with the screens bypassed entirely.
	for _, tc := range []struct {
		name     string
		declared []*md.SlotOrigin
	}{
		{"one fingerprint, one without", []*md.SlotOrigin{
			{Origin: origin, Fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, FpPresent: true},
			{Origin: origin, FpPresent: false},
		}},
		{"neither declares one", []*md.SlotOrigin{
			{Origin: origin, FpPresent: false},
			{Origin: origin, FpPresent: false},
		}},
		{"the same fingerprint twice", []*md.SlotOrigin{
			{Origin: origin, Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true},
			{Origin: origin, Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := md.ComposeWith(list, tc.declared); err == nil {
				t.Errorf("md.ComposeWith EMITTED a template violating §4f's invariant; " +
					"one card could then fill both slots and be presented as reviewed")
			}
		})
	}

	// And the CONTROL: two distinct fingerprints at one origin is LEGAL, so
	// the refusals above are the invariant and not a blanket ban on sharing an
	// origin.
	ok := []*md.SlotOrigin{
		{Origin: origin, Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true},
		{Origin: origin, Fingerprint: [4]byte{9, 9, 9, 9}, FpPresent: true},
	}
	if _, err := md.ComposeWith(list, ok); err != nil {
		t.Errorf("INCONCLUSIVE: two DISTINCT fingerprints at one origin were refused (%v), "+
			"so the assertions above may be measuring a wider ban than §4f states", err)
	}
}
