package gui

import (
	"fmt"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/mk"
)

// ─── S1 fixtures: cosigner cards as the PAYLOAD actually carries them ────────
//
// These are derived through the DEVICE'S OWN path (bip39.MnemonicSeed →
// hdkeychain.NewMaster → bip32.Derive → Neuter → mk.Encode), which is
// character-for-character what cmd/buildpayloadcards does for
// cmd/emu/sysw_cards_payload.bin. Building them here rather than vendoring a
// blob keeps the unit tests independent of the emulator artifact while still
// exercising the SAME card shape the walk meets: real depth-4 BIP-48 account
// xpubs, real master fingerprints, multi-chunk sets.
//
// mk1CardA/mk1CardB (bundle_testdata_test.go) are deliberately NOT reused: they
// share one xpub and differ only by fingerprint, so a policy assembled from them
// carries DUPLICATE KEYS and cannot prove which slot got which card. Slot order
// is identity-bearing here, so the fixtures must be distinguishable by key.
//
// TEST MATERIAL, public by construction: all three masters are BIP-39's own
// published vectors. Never put funds behind them.

const (
	fixtureMasterA = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	fixtureMasterB = "legal winner thank year wave sausage worth useful legal winner thank yellow"
	fixtureMasterC = "letter advice cage absurd amount doctor acoustic avoid letter advice cage above"
)

// fixtureStub mirrors cmd/buildpayloadcards' placeholder policy-id stub: mk1
// requires one, and buildCosignerCards decodes the card without checking it, so
// a fixed value keeps these cards usable for any policy a test builds.
var fixtureStub = [4]byte{0x5b, 0x48, 0xaf, 0x35}

// The fixture roster, ordered. Index i is "payload card i". Five entries is the
// most any S1 case needs (n tops out at 5, and the 0..n ruling allows a payload
// carrying n cards).
//
// ─── F-180: THIS ORDER IS NOT THE EMULATOR PAYLOAD'S, AND THAT IS DELIBERATE ──
//
// Here                             A@0, B@0, C@0, A@1, B@1
// cmd/buildpayloadcards/main.go    A@0, A@1, B@0, C@0   (cmd/emu/sysw_cards_payload.bin)
//
// Both are right for what they serve and NEITHER may be quietly re-ordered to
// match the other. This roster is prefix-addressed: cosignerCardRecords(t, n)
// takes the first n entries and S1's tests assert WHICH card fills WHICH slot,
// so Trace A's 2-of-3 wants its two foreign cosigners (B@0, C@0) in the first
// three. The emulator blob is walked by tap sequence and needs the two
// same-master accounts adjacent at the front, because Trace B holds A@0 and A@1
// and the picker walks payload record order.
//
// WHAT IT COSTS, and why saying it here is the whole fix: A TAP SEQUENCE
// MEASURED AGAINST ONE IS WRONG AGAINST THE OTHER. Reaching Trace A's B@0 + C@0
// is SKIP, USE, USE in a Go test over this roster and SKIP, SKIP in the emulator
// over that blob. F-180 was filed because neither file said so, and a draft test
// that carried the emulator's sequence across selected A@1 instead -- caught only
// because S2's foreign-origin refusal happened to fire on it.
//
// Keep the two paragraphs above in sync with the twin note in
// cmd/buildpayloadcards/main.go. If one roster is ever changed, the other's note
// is the thing that goes stale first.
var cosignerCardRoster = []struct {
	mnemonic string
	account  uint32
}{
	{fixtureMasterA, 0},
	{fixtureMasterB, 0},
	{fixtureMasterC, 0},
	{fixtureMasterA, 1},
	{fixtureMasterB, 1},
}

var (
	cosignerCardOnce sync.Once
	cosignerCardSets [][]string
	cosignerCardErr  error
)

// cosignerCardFixtures returns the first n distinct, complete mk1 chunk sets
// from the roster, in roster order. Each set is >= 2 chunks and round-trips
// through mk.Decode; every set has a distinct chunk_set_id and a distinct xpub.
//
// Derivation is done ONCE for the package (five PBKDF2 seeds) and shared,
// because S1's n×cards matrix asks for these fixtures 22 times.
func cosignerCardFixtures(tb testing.TB, n int) [][]string {
	tb.Helper()
	if n > len(cosignerCardRoster) {
		tb.Fatalf("asked for %d cosigner cards; the roster holds %d", n, len(cosignerCardRoster))
	}
	cosignerCardOnce.Do(func() {
		net := &chaincfg.MainNetParams
		for _, w := range cosignerCardRoster {
			m, err := bip39.ParseMnemonic(w.mnemonic)
			if err != nil {
				cosignerCardErr = fmt.Errorf("ParseMnemonic: %w", err)
				return
			}
			pathStr := fmt.Sprintf("m/48'/0'/%d'/2'", w.account)
			path, err := bip32.ParsePath(pathStr)
			if err != nil {
				cosignerCardErr = fmt.Errorf("ParsePath %s: %w", pathStr, err)
				return
			}
			master, err := hdkeychain.NewMaster(bip39.MnemonicSeed(m, ""), net)
			if err != nil {
				cosignerCardErr = fmt.Errorf("NewMaster: %w", err)
				return
			}
			pk, err := master.ECPubKey()
			if err != nil {
				cosignerCardErr = fmt.Errorf("ECPubKey: %w", err)
				return
			}
			acct, err := bip32.Derive(master, path)
			if err != nil {
				cosignerCardErr = fmt.Errorf("Derive %s: %w", pathStr, err)
				return
			}
			pub, err := acct.Neuter()
			if err != nil {
				cosignerCardErr = fmt.Errorf("Neuter: %w", err)
				return
			}
			strs, err := mk.Encode(mk.Card{
				Network:     "mainnet",
				Path:        pathStr,
				Fingerprint: fmt.Sprintf("%08x", bip32.Fingerprint(pk)),
				Stubs:       [][4]byte{fixtureStub},
				Xpub:        pub.String(),
			})
			if err != nil {
				cosignerCardErr = fmt.Errorf("mk.Encode: %w", err)
				return
			}
			cosignerCardSets = append(cosignerCardSets, strs)
		}
	})
	if cosignerCardErr != nil {
		tb.Fatalf("building cosigner card fixtures: %v", cosignerCardErr)
	}
	return cosignerCardSets[:n]
}

// cosignerCardRecords flattens the first n fixture card sets into the flat
// record list a payload carries — chunk by chunk, cards contiguous and in
// roster order. This is the exact shape sysw.Payload.Public holds.
func cosignerCardRecords(tb testing.TB, n int) []string {
	tb.Helper()
	var out []string
	for _, set := range cosignerCardFixtures(tb, n) {
		out = append(out, set...)
	}
	return out
}

// TestCosignerCardFixturesAreDistinctAndComplete guards the fixtures themselves.
// Every S1 assertion below is measured against these; a roster that silently
// collapsed to one card would make "all four arrived" and "one arrived" the same
// observation.
func TestCosignerCardFixturesAreDistinctAndComplete(t *testing.T) {
	sets := cosignerCardFixtures(t, len(cosignerCardRoster))
	if len(sets) != len(cosignerCardRoster) {
		t.Fatalf("got %d card sets, want %d", len(sets), len(cosignerCardRoster))
	}
	seenCsid := map[uint32]int{}
	seenXpub := map[string]int{}
	for i, set := range sets {
		if len(set) < 2 {
			t.Errorf("card %d is %d chunk(s); the plan's tests need multi-chunk sets "+
				"so chunk assembly is actually exercised", i, len(set))
		}
		card, err := mk.Decode(set)
		if err != nil {
			t.Fatalf("card %d does not decode: %v", i, err)
		}
		h, err := mk.ParseHeader(set[0])
		if err != nil {
			t.Fatalf("card %d header: %v", i, err)
		}
		if !h.Chunked {
			t.Errorf("card %d is not chunked, so the bundle gatherer refuses it "+
				"as a single mk1 (host parity)", i)
		}
		if prev, dup := seenCsid[h.ChunkSetID]; dup {
			t.Errorf("cards %d and %d share chunk_set_id %#x — the gatherer keys "+
				"cards by csid, so they would merge into one", prev, i, h.ChunkSetID)
		}
		seenCsid[h.ChunkSetID] = i
		if prev, dup := seenXpub[card.Xpub]; dup {
			t.Errorf("cards %d and %d share an xpub, so slot assignment is not "+
				"observable from the assembled policy", prev, i)
		}
		seenXpub[card.Xpub] = i
	}
	t.Logf("%d distinct cosigner cards, %d records total",
		len(sets), len(cosignerCardRecords(t, len(sets))))
}
