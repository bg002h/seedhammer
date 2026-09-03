package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/sysw"
)

// ═══ F-76 / F-437: THE PAYLOAD DOOR, WALKED ══════════════════════════════════
//
// Every measurement in this file was taken first by the S2 journey walk against
// fork e456970, where the door handed its card-gatherer ONE record:
//
//	J2      Wallet Policy, payload holding all six chunks of one md1 card
//	        -> "md1 descriptors: 0"          (the card was invisible)
//	J2BUNDLE  Engrave Bundle, same payload   -> "md1 descriptors: 0"
//	FU2     Engrave Bundle, payload holding both chunks of one mk1 card
//	        -> "mk1 keys: 0"
//	FU2     CONTROL, all chunks seeded by hand -> "mk1 keys: 1"
//	FU1     Done at the zero screen -> "Dropped an incomplete card. Scan all
//	        its chunks to include it." — advice that BLAMES A PAYLOAD CARRYING
//	        EVERY CHUNK, and on a reader-less machine cannot be followed at all
//	FU3/J2  the door itself reads "FROM PAYLOAD / ENTER IT", and ENTER IT lands
//	        in an NFC card gather. There is no keyboard (F-437).
//
// The control is what makes these tests measure the DOOR rather than the cards:
// the same chunks, seeded directly, always completed. So each walk below is
// paired with the seeded control wherever the control is not already shipped.

// ─── The fixtures ────────────────────────────────────────────────────────────
//
// Real `me` containers, so the walk starts at bytes the host actually writes
// rather than at a struct literal. The RECORDS inside them are this package's
// own committed card fixtures — wshSortedmultiChunks (gui/md1_gather_test.go,
// the 6-chunk BlueWallet 2-of-3 wsh(sortedmulti) card) and mk1CardA
// (gui/bundle_testdata_test.go, a 2-chunk mk1 key card) — so the container and
// the expectation cannot drift apart silently: f76Session asserts the opened
// records EQUAL those constants, which is a stronger binding than the hash.
//
// Regenerate (from this worktree, with the strings one per line):
//
//	me sysw pack --no-passphrase --in <records> --out gui/testdata/<name>.bin
//
//	f76_md1_card_payload.bin     wshSortedmultiChunks, all 6      (complete)
//	f76_mk1_card_payload.bin     mk1CardA, both chunks            (complete)
//	f76_md1_partial_payload.bin  wshSortedmultiChunks[0:5]        (INCOMPLETE)
//
// The partial container is deliberate, not a broken fixture: a payload that
// genuinely lacks a chunk is the one case the "incomplete card" refusal can
// still reach once the door is fixed, and `me` warns about it at pack time
// ("record N ... an md1/mk1 this tool could not decode").
const (
	f76Md1CardPayload    = "testdata/f76_md1_card_payload.bin"
	f76Md1CardSHA256     = "59eb99f7b60ff1526d31dcc27d156ee838950d97496dcc30b7c556d90b1c87a3"
	f76Mk1CardPayload    = "testdata/f76_mk1_card_payload.bin"
	f76Mk1CardSHA256     = "03aa3113272d92fae73ee0dd5c30baedf0a90a3021cf6d49cdc413977f98c412"
	f76Md1PartialPayload = "testdata/f76_md1_partial_payload.bin"
	f76Md1PartialSHA256  = "0875699344b48c3b4e26a8df053ee62dcff06b2b9c67e53bb09dc5a2415a514c"
)

// f76Session opens a committed container the way the firmware does, checks it
// holds exactly `want`, and loads it into a compared session.
//
// The record check is the point. A hash pins the BYTES; this pins the MEANING —
// that the payload under walk is the very card set the control seeds by hand.
func f76Session(t *testing.T, path, sum string, want []string) *syswSession {
	t.Helper()
	blob, err := os.ReadFile(path)
	if err != nil {
		// FATAL, never a skip: the file is committed, so its absence means a
		// broken checkout, and a test that answers "I could not tell" by
		// reporting success is the default failure mode in this tree.
		t.Fatalf("INCONCLUSIVE: %s is unreadable: %v", path, err)
	}
	if got := hex.EncodeToString(sliceSHA(blob)); got != sum {
		t.Fatalf("%s hashes to %s, the fixture pins %s -- regenerate it with the "+
			"invocation in this file's header and re-pin", path, got, sum)
	}
	pay, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("the firmware's own sysw.Open cannot read what `me` wrote: %v", err)
	}
	if len(pay.Secret) != 0 {
		t.Fatalf("%s holds %d secret record(s); md1/mk1 are public", path, len(pay.Secret))
	}
	if len(pay.Public) != len(want) {
		t.Fatalf("%s holds %d public record(s), want %d", path, len(pay.Public), len(want))
	}
	for i, rec := range pay.Public {
		if rec != want[i] {
			t.Fatalf("%s record %d is not the committed card fixture:\n got %q\nwant %q",
				path, i, rec, want[i])
		}
		if got := sysw.Classify(rec); got != sysw.ClassMDMK {
			t.Fatalf("%s record %d classifies as %v, want ClassMDMK -- the door "+
				"would never be offered", path, i, got)
		}
	}
	s := &syswSession{}
	// compared=true: takeAll refuses until [compared] is earned, and earning it
	// is syswLoadFlow's business, which this walk starts after.
	s.load(pay, sysw.Identity(blob), false, true, true, true)
	return s
}

func sliceSHA(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// f76Platform is a reader-less machine (phase-1 hardware as the operator holds
// it): Features() reports no FeatureNFC, so every "scan it" instruction on
// screen is one nobody can follow.
func f76Platform() *testPlatform {
	p := newPlatform()
	p.display = sh2DisplaySize
	return p
}

// f76NFCPlatform has a reader that never delivers a tag — the machine the NFC
// arm of every message below is written for.
func f76NFCPlatform() *testPlatform {
	p := f76Platform()
	p.nfc = func() io.ReadCloser { return &fakeNFC{closed: make(chan struct{})} }
	return p
}

// ─── F-76: the door delivers the WHOLE card ──────────────────────────────────

// J2, re-run. Wallet Policy, a payload holding all six chunks of one good md1
// card: measured "md1 descriptors: 0" before the fix.
func TestF76WalletPolicyCountsACompleteMd1CardFromThePayload(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
	defer quit()

	// (0) THE COMPOSER'S DOOR, which is now the first screen in every
	// state (SPEC_wallet_policy_composer §7a). "Scan cards" is index 0, so
	// one Down selects "From payload", which is the route this walk takes.
	if _, ok := pumpUntil(frame, "Build a new policy", 16); !ok {
		t.Fatal("the composer door never drew")
	}
	click(&ctx.Router, Down)
	click(&ctx.Router, Button3)
	got, ok := pumpUntil(frame, "Cards from where?", 16)
	if !ok {
		t.Fatalf("the md1 card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // FROM PAYLOAD, row 0

	got, ok = pumpUntil(frame, "md1 descriptors: 1", 64)
	if !ok {
		t.Errorf("the payload's SIX chunks assembled into no card at the Wallet "+
			"Policy door (J2 measured `md1 descriptors: 0`).\nLast frame: %q", got)
	}
}

// J2BUNDLE, re-run: the same payload at Engrave Bundle's door.
func TestF76BundleCountsACompleteMd1CardFromThePayload(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	got, ok := pumpUntil(frame, "Cards from where?", 16)
	if !ok {
		t.Fatalf("the md1 card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)

	got, ok = pumpUntil(frame, "md1 descriptors: 1", 64)
	if !ok {
		t.Errorf("Engrave Bundle counted no md1 card from a payload holding all "+
			"six chunks (J2BUNDLE measured 0).\nLast frame: %q", got)
	}
}

// FU2, re-run: a 2-chunk mk1 KEY card, measured "mk1 keys: 0" from the payload
// against "mk1 keys: 1" for the same chunks seeded directly.
func TestF76BundleCountsACompleteMk1CardFromThePayload(t *testing.T) {
	want := mk1CardA(t)
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Mk1CardPayload, f76Mk1CardSHA256, want)

	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	got, ok := pumpUntil(frame, "Cards from where?", 16)
	if !ok {
		t.Fatalf("the mk1 card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)

	got, ok = pumpUntil(frame, "mk1 keys: 1", 64)
	if !ok {
		t.Errorf("the payload's TWO mk1 chunks assembled into no key card "+
			"(FU2 measured `mk1 keys: 0`; the all-seeded control measured 1)."+
			"\nLast frame: %q", got)
	}
}

// THE CONTROL FU2 ALREADY RAN, kept in the tree so a future regression cannot
// be blamed on the cards. Same chunks, seeded past the door: the count was 1
// before the fix and must stay 1 after it.
func TestF76BundleCountsTheSameMk1CardWhenSeededDirectly(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.syswBundleSeeds = mk1CardA(t)

	frame, quit := runUI(ctx, func() {
		bundleGatherFlowResume(ctx, &descriptorTheme, "Engrave Bundle", nil)
	})
	defer quit()

	got, ok := pumpUntil(frame, "mk1 keys: 1", 32)
	if !ok {
		t.Fatalf("INCONCLUSIVE: the control failed too, so the card fixture is "+
			"broken and the door tests above measure nothing.\nLast frame: %q", got)
	}
}

// ─── F-76's original scope: INSPECT ──────────────────────────────────────────
//
// mdmkFlow's "Inspect descriptor" / "Inspect key" primed a fresh gatherer with
// the ONE scanned string and then opened the reader for the rest. When the rest
// are in the payload there are no tags to tap, so the operator was stranded on
// a scan-waiting screen with no route forward but Back.

func TestF76InspectDescriptorCompletesFromThePayload(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	frame, quit := runUI(ctx, func() {
		md1GatherFlow(ctx, &descriptorTheme, wshSortedmultiChunks[0])
	})
	defer quit()

	got, ok := pumpUntil(frame, "Engrave Descriptor", 64)
	if !ok {
		t.Errorf("Inspect stranded on the gather instead of reaching the "+
			"descriptor screen; every remaining chunk was in the payload."+
			"\nLast frame: %q", got)
	}
	if uiContains(got, "Scan the next chunk") {
		t.Errorf("Inspect is still asking for tags that do not exist.\nFrame: %q", got)
	}
}

func TestF76InspectKeyCompletesFromThePayload(t *testing.T) {
	want := mk1CardA(t)
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Mk1CardPayload, f76Mk1CardSHA256, want)

	var card any
	var ok bool
	frame, quit := runUI(ctx, func() {
		c, o := mk1GatherFlow(ctx, &descriptorTheme, want[0])
		card, ok = c, o
	})
	// A completed set draws NO frame here: mk1GatherFlow decodes and returns
	// without entering its scan loop. So a frame at all is the defect.
	got, drew := frame()
	quit()
	if drew {
		t.Errorf("Inspect key drew a gather screen instead of completing from the "+
			"payload.\nFrame: %q", got)
	}
	if !ok {
		t.Fatal("mk1GatherFlow did not return a card, though the payload held " +
			"every chunk of it")
	}
	if c, isCard := card.(interface{ String() string }); isCard {
		_ = c // the decode itself is the assertion; mk.Card has no String()
	}
}

// ─── F-76 second widening: the message must stop blaming the payload ─────────

// FU1, re-run against a payload that is GENUINELY short a chunk — the one case
// that can still reach this refusal once the door is fixed. On a reader-less
// machine the only route is a re-pack, and the message must name it.
func TestF76IncompletePayloadGetsTheRepackAdvice(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1PartialPayload, f76Md1PartialSHA256, wshSortedmultiChunks[:5])

	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	got, ok := pumpUntil(frame, "Cards from where?", 16)
	if !ok {
		t.Fatalf("the card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)
	if got, ok = pumpUntil(frame, "md1 descriptors: 0", 32); !ok {
		t.Fatalf("a 5-of-6 payload must NOT assemble a card.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // Done

	got, ok = pumpUntil(frame, "me sysw pack", 32)
	if !ok {
		t.Errorf("a payload genuinely missing a chunk was not told to re-pack."+
			"\nLast frame: %q", got)
	}
	if !uiContains(got, "missing") && !uiContains(got, "does not carry") {
		t.Errorf("the refusal does not say what is actually wrong.\nFrame: %q", got)
	}
}

// The NFC arm. FU1-NFC measured "Dropped an incomplete card. Scan all its
// chunks to include it." — true only if the card came from a tag. With a
// payload loaded it may equally have come from the payload, and then scanning
// is the wrong instruction, so the message must name BOTH routes.
func TestF76IncompletePayloadNamesBothRoutesOnAnNFCMachine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(f76NFCPlatform())
		ctx.sysw = f76Session(t, f76Md1PartialPayload, f76Md1PartialSHA256, wshSortedmultiChunks[:5])

		frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
		defer quit()

		got, ok := pumpUntil(frame, "Cards from where?", 16)
		if !ok {
			t.Fatalf("the card offer never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		if got, ok = pumpUntil(frame, "md1 descriptors: 0", 32); !ok {
			t.Fatalf("a 5-of-6 payload must NOT assemble a card.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3) // Done

		got, ok = pumpUntil(frame, "Dropped an incomplete card", 32)
		if !ok {
			t.Fatalf("no refusal drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "me sysw pack") {
			t.Errorf("with a payload loaded the refusal names only the reader, so "+
				"an operator whose card came from the payload is sent scanning "+
				"tags that never existed (F-76 widening b).\nFrame: %q", got)
		}
	})
}

// The complete-in-payload case must never reach that message at all. Same door,
// same button, a payload holding every chunk: Done PROCEEDS.
func TestF76CompletePayloadNeverSeesTheIncompleteRefusal(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	if got, ok := pumpUntil(frame, "Cards from where?", 16); !ok {
		t.Fatalf("the card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)
	if got, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
		t.Fatalf("the card did not assemble.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // Done

	got, ok := pumpUntil(frame, "cards verified", 64)
	if !ok {
		t.Fatalf("Done did not proceed to the review.\nLast frame: %q", got)
	}
	if uiContains(got, "Dropped an incomplete card") {
		t.Errorf("a payload carrying every chunk was told a card was incomplete."+
			"\nFrame: %q", got)
	}
}

// ─── FUNDS SAFETY: priming must not bypass the checksum ──────────────────────
//
// The gatherer is the only thing standing between a corrupted chunk and a
// plate, and it validates through the BCH checksum on the way in. Priming from
// memory must therefore go through offer(), not around it.
func TestF76ACorruptedChunkInThePayloadIsStillRefused(t *testing.T) {
	corrupt := make([]string, len(wshSortedmultiChunks))
	copy(corrupt, wshSortedmultiChunks)
	// One symbol, in the middle of the last chunk's body: enough to break the
	// BCH checksum without changing the length or the hrp.
	bad := []byte(corrupt[5])
	if bad[40] == 'q' {
		bad[40] = 'p'
	} else {
		bad[40] = 'q'
	}
	corrupt[5] = string(bad)
	if corrupt[5] == wshSortedmultiChunks[5] {
		t.Fatal("INCONCLUSIVE: the corruption changed nothing")
	}

	// (1) The gatherer's own door refuses it — the same call an NFC scan makes.
	if st := (&md1Gatherer{}).offer(corrupt[5]); st != gatherIgnored {
		t.Errorf("md1Gatherer.offer accepted a corrupted chunk (status %v)", st)
	}
	// (2) So does classification, so it never even becomes a ClassMDMK record.
	if got := sysw.Classify(corrupt[5]); got == sysw.ClassMDMK {
		t.Errorf("a corrupted chunk classified as ClassMDMK")
	}

	// (3) And the whole door: five good chunks plus one corrupted one assemble
	// into NO card, and the operator is told so rather than handed a partial.
	ctx := NewContext(f76Platform())
	ctx.sysw = sessionWith(corrupt...)

	frame, quit := runUI(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	defer quit()

	if got, ok := pumpUntil(frame, "Cards from where?", 16); !ok {
		t.Fatalf("the card offer never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)
	got, ok := pumpUntil(frame, "md1 descriptors: 0", 32)
	if !ok {
		t.Fatalf("a set with a corrupted chunk ASSEMBLED A CARD -- priming went "+
			"around the checksum.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // Done
	if got, ok = pumpUntil(frame, "Dropped an incomplete card", 32); !ok {
		t.Errorf("the corrupted set produced no refusal.\nLast frame: %q", got)
	}
}

// The four arms of the refusal, as a table, because two of them are not
// reachable from a walk on this hardware and an unreachable arm is exactly
// where a wrong instruction survives.
//
// The RULE each row encodes: name a route only when the operator has it.
func TestF76PendingMessageNamesOnlyRoutesThatExist(t *testing.T) {
	for _, tc := range []struct {
		name             string
		reader, payload  bool
		wantScan, wantMe bool
	}{
		{"reader and payload", true, true, true, true},
		{"reader only", true, false, true, false},
		{"payload only (phase-1 hardware)", false, true, false, true},
		{"neither", false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bundlePendingMessage(tc.reader, tc.payload)
			if !strings.HasPrefix(got, "Dropped an incomplete card") {
				t.Errorf("the refusal no longer says what happened: %q", got)
			}
			if scans := strings.Contains(got, "Scan"); scans != tc.wantScan {
				t.Errorf("names scanning = %v, want %v (reader=%v): %q",
					scans, tc.wantScan, tc.reader, got)
			}
			if packs := strings.Contains(got, "me sysw pack"); packs != tc.wantMe {
				t.Errorf("names a re-pack = %v, want %v (payload=%v): %q",
					packs, tc.wantMe, tc.payload, got)
			}
		})
	}
}

// syswPrimeCard is PRIMED-ONLY, and this is the counterexample that rule
// exists for: an unprimed gatherer adopts the first set it is offered, so a
// payload card would silently become the card under inspection.
func TestF76PrimingNeverSubstitutesACardForAnUnprimedGatherer(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	g := &md1Gatherer{}
	if g.isPrimed() {
		t.Fatal("a fresh gatherer reports itself primed")
	}
	syswPrimeCard(ctx, g)
	if g.isPrimed() {
		t.Error("the payload primed a gatherer that had identified no set; a " +
			"payload card would then answer a question about a different one")
	}
	if g.complete() {
		t.Error("the payload COMPLETED an unprimed gatherer")
	}
}

// And the other half: a primed gatherer accepts only ITS OWN set from the
// payload. A payload holding a foreign card cannot fill the gaps in this one.
func TestF76PrimingOnlyEverAddsToTheIdentifiedSet(t *testing.T) {
	ctx := NewContext(f76Platform())
	// A payload of mk1 chunks against an md1 gatherer primed on an md1 chunk:
	// a whole card's worth of valid records, none of them this card's.
	ctx.sysw = f76Session(t, f76Mk1CardPayload, f76Mk1CardSHA256, mk1CardA(t))

	g := &md1Gatherer{}
	if st := g.offer(wshSortedmultiChunks[0]); st != gatherAdded {
		t.Fatalf("INCONCLUSIVE: priming chunk refused (%v)", st)
	}
	syswPrimeCard(ctx, g)
	if got := len(g.set); got != 1 {
		t.Errorf("the gatherer holds %d chunk(s) after priming from a payload "+
			"carrying none of them, want 1", got)
	}
	if g.complete() {
		t.Error("a foreign payload completed the set")
	}
}
