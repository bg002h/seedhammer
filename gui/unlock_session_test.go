package gui

import (
	"image"
	"iter"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/op"
	"seedhammer.com/seal"
)

// §10.2.2 -- the secret session. Every record that classifies as a secret is
// offered FIRST, consecutively, and each is wiped as its plate leaves the
// screen BY ANY ROUTE.
//
// EVERY WIPE ASSERTION HERE IS ON A BUFFER, never on a return value. A return
// value cannot tell a wipe from a missing wipe, which is what unlockSecretHook
// exists for: it observes each stage with the record's live bytes, so a test
// can read the actual memory at the actual instant.

// allZeroBytes is the residency question, asked of one buffer.
func allZeroBytes(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// secretEvent is one unlockSecretHook firing, with the WHOLE secret set's
// residency snapshotted at that instant.
//
// The snapshot is the point. "Each is wiped as its plate leaves" is a claim
// about the ORDER of wipes relative to offers, and only a reading taken at the
// moment of the next offer can see it -- an after-the-fact check passes
// whether the wipe happened early, late, or in a lump at the end.
type secretEvent struct {
	stage string
	idx   int
	// passedZero is the buffer the hook was HANDED, read at hook time.
	passedZero bool
	// zero[i] is whether secret record i read all-zero at that instant.
	zero []bool
}

// watchSecrets installs unlockSecretHook and returns the log it appends to.
func watchSecrets(t *testing.T, p *seal.Payload) *[]secretEvent {
	t.Helper()
	evts := new([]secretEvent)
	unlockSecretHook = func(stage string, idx int, rec []byte) {
		z := make([]bool, len(p.Secret))
		for i := range p.Secret {
			z[i] = allZeroBytes(p.Secret[i].Record)
		}
		*evts = append(*evts, secretEvent{
			stage: stage, idx: idx, passedZero: allZeroBytes(rec), zero: z,
		})
	}
	t.Cleanup(func() { unlockSecretHook = nil })
	return evts
}

func (e *secretEvent) String() string {
	return e.stage + "@" + string(rune('0'+e.idx))
}

// offers returns the indices offered, in order.
func offers(evts []secretEvent) []int {
	var out []int
	for _, e := range evts {
		if e.stage == "offered" {
			out = append(out, e.idx)
		}
	}
	return out
}

// unlockedPayload runs §10.2 steps 1-3 and 8-9 headlessly, which is the state
// unlockSecretSession is called in. It goes through the real pipeline (the
// vector's own derived key) rather than hand-building a Payload, so the records
// under test are the ones the device would hold.
func unlockedPayload(t *testing.T, name string) *seal.Payload {
	t.Helper()
	v := sealVector(t, name)
	blob := v.blob(t)
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("vector %s: Inspect: %v", name, err)
	}
	words := vectorPassphrase(t, v)
	key := seal.DeriveKey(seal.NormalisePassphrase(joinWords(words)), p.Header.Salt[:],
		int(p.Header.Iterations))
	defer clear(key)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("vector %s: UnlockWithKey: %v", name, err)
	}
	for i, r := range p.Secret {
		if allZeroBytes(r.Record) {
			t.Fatalf("vector %s: record %d arrived already zero", name, i)
		}
	}
	return p
}

// sessionHarness drives unlockSecretSession by touch.
type sessionHarness struct {
	t       *testing.T
	ctx     *Context
	frame   func() (string, bool)
	drawer  func() *op.Drawer
	done    *bool
	content string
	// targets reports the drawn touch targets of the CURRENT frame together
	// with the text drawn inside each. nil unless the harness was built by
	// runSecretSessionLabelled -- see chooseLabelled.
	targets func() []labelledTarget
}

func runSecretSession(t *testing.T, p *seal.Payload) *sessionHarness {
	t.Helper()
	pf := newPlatform()
	pf.display = sh2DisplaySize
	pf.engraver = newEngraver()
	ctx := NewContext(pf)
	returned := false
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	frame, drawer, quit := runUITouch(ctx, func() {
		unlockSecretSession(ctx, &descriptorTheme, p)
		returned = true
	})
	h.frame, h.drawer = frame, drawer
	t.Cleanup(quit)
	return h
}

func (h *sessionHarness) next(what string) string {
	h.t.Helper()
	c, ok := h.frame()
	if !ok {
		h.t.Fatalf("the session produced no frame (%s); last frame %q", what, h.content)
	}
	h.content = c
	return c
}

func (h *sessionHarness) pump(n int, want string) bool {
	h.t.Helper()
	for i := 0; i < n; i++ {
		c, ok := h.frame()
		if !ok {
			return false
		}
		h.content = c
		if uiContains(c, want) {
			return true
		}
	}
	return false
}

func (h *sessionHarness) mustReach(want string) string {
	h.t.Helper()
	if !h.pump(256, want) {
		h.t.Fatalf("never reached %q; last frame %q", want, h.content)
	}
	return h.content
}

func (h *sessionHarness) tapNav(b Button) {
	h.t.Helper()
	tapNavSlot(h.t, h.ctx, h.drawer(), b)
}

// choose taps the n'th choice of the ChoiceScreen currently up, then confirms.
// ChoiceScreen children carry the zero Button, so plateHitPoints finds them the
// same way it finds plate-list entries.
func (h *sessionHarness) choose(n int) {
	h.t.Helper()
	pts := plateHitPoints(h.ctx, h.drawer())
	if n >= len(pts) {
		h.t.Fatalf("the choice screen drew %d touch targets; cannot select #%d (%q)",
			len(pts), n, h.content)
	}
	tap(&h.ctx.Router, h.drawer(), pts[n])
	h.next("after selecting choice")
	h.tapNav(Button3)
	// Tolerant, deliberately: confirming the LAST secret ends the session, so
	// there is legitimately no further frame. A fatal here would make every
	// whole-session test depend on there being one more screen after it.
	if c, ok := h.frame(); ok {
		h.content = c
	}
}

// ---------------------------------------------------------------------------
// the offer order
// ---------------------------------------------------------------------------

// TestSecretSessionOffersEverySecretFirstAndInOrder — §11.2 requires vector F's
// THREE ms1 records to be the ones offered first, while its twelve mk1/md1
// records are ordinary plates.
//
// Why plural is load-bearing: under a singular implementation the operator
// engraves one of three, the plate list then shows only mk1/md1 so nothing
// looks missing, and they store an incomplete backup of a 2-of-3 believing it
// complete -- §6.4's own "worst available outcome".
//
// MUTATED IN BOTH DIRECTIONS, per the plan: `at = at[:1]` gives ONE offer and
// IsSecret widened to ClassMDMK gives FIFTEEN, so the count is asserted exactly
// rather than as "at least one".
func TestSecretSessionOffersEverySecretFirstAndInOrder(t *testing.T) {
	p := unlockedPayload(t, "F")
	if len(p.Secret) != 15 {
		t.Fatalf("premise broken: vector F must carry 15 encrypted records, got %d", len(p.Secret))
	}
	var wantIdx []int
	for i, r := range p.Secret {
		if seal.IsSecret(r.Class) {
			wantIdx = append(wantIdx, i)
		}
	}
	if len(wantIdx) != 3 {
		t.Fatalf("premise broken: vector F must carry three ms1 records, got %d", len(wantIdx))
	}

	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	for i := 0; i < 3; i++ {
		h.mustReach("SECRET seed material")
		h.choose(1) // Skip
	}
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("the session did not end after every secret was offered")
	}

	got := offers(*evts)
	if len(got) != len(wantIdx) {
		t.Fatalf("the session offered %d records (%v), want exactly %d (%v) -- one offer "+
			"means only the first secret was offered, fifteen means IsSecret admitted the cards",
			len(got), got, len(wantIdx), wantIdx)
	}
	for i := range got {
		if got[i] != wantIdx[i] {
			t.Errorf("offer %d was record %d, want %d", i, got[i], wantIdx[i])
		}
	}
	// CONSECUTIVE, and never an md1/mk1.
	for _, e := range *evts {
		if !seal.IsSecret(p.Secret[e.idx].Class) {
			t.Errorf("the session touched record %d, which classifies as %v and is not a secret",
				e.idx, p.Secret[e.idx].Class)
		}
	}
}

// TestSecretSessionWipesEachBeforeTheNextIsOffered — §6d's load-bearing row.
//
// At the moment record k+1 is offered, record k's buffer must read all-zero.
// A wipe that happened in a lump at the end of the session, or after the whole
// loop, passes every after-the-fact check and fails this one.
func TestSecretSessionWipesEachBeforeTheNextIsOffered(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	for i := 0; i < 3; i++ {
		h.mustReach("SECRET seed material")
		h.choose(1) // Skip
	}
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}

	var offered []secretEvent
	for _, e := range *evts {
		if e.stage == "offered" {
			offered = append(offered, e)
		}
	}
	if len(offered) < 2 {
		t.Fatalf("only %d records were offered; this test needs a payload with several "+
			"secrets, which is why it uses vector F", len(offered))
	}
	for k, e := range offered {
		// The record being offered is LIVE -- offering a zeroed record would be
		// offering to engrave nothing.
		if e.passedZero {
			t.Errorf("record %d was offered already zeroed", e.idx)
		}
		if e.zero[e.idx] {
			t.Errorf("record %d read all-zero at the moment it was offered", e.idx)
		}
		// And every EARLIER secret is already gone.
		for j := 0; j < k; j++ {
			prev := offered[j].idx
			if !e.zero[prev] {
				t.Errorf("when record %d was offered, record %d (offered %d earlier) was "+
					"still resident; §10.2.2 wipes each as its plate leaves, not in a lump",
					e.idx, prev, k-j)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// every route wipes
// ---------------------------------------------------------------------------

// TestSecretSessionSkipWipes — Skip builds no plate at all, so this is the
// deferred backstop doing the work rather than the early clear(rec).
func TestSecretSessionSkipWipes(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	first := firstSecretIdx(t, p)
	if allZeroBytes(p.Secret[first].Record) {
		t.Fatal("premise broken: the record is zero before Skip")
	}
	h.choose(1) // Skip
	h.mustReach("SECRET seed material")
	if !allZeroBytes(p.Secret[first].Record) {
		t.Errorf("Skip left record %d resident: %q", first, p.Secret[first].Record)
	}
	assertSecretWiped(t, *evts, first)
}

// TestSecretSessionBackWipes — Back and Skip are the same outcome and both
// wipe. There is deliberately no third option: a "later" that kept the record
// resident is the state §10.2.2 exists to prevent.
func TestSecretSessionBackWipes(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	first := firstSecretIdx(t, p)
	if allZeroBytes(p.Secret[first].Record) {
		t.Fatal("premise broken: the record is zero before Back")
	}
	h.tapNav(Button1) // Back out of the Cut/Skip choice
	h.mustReach("SECRET seed material")
	if !allZeroBytes(p.Secret[first].Record) {
		t.Errorf("Back left record %d resident: %q", first, p.Secret[first].Record)
	}
	assertSecretWiped(t, *evts, first)
}

// TestSecretRecordIsZeroWHILETheEngraveScreenIsUp — THE assertion that pins the
// early wipe, and the only one that can.
//
// §10.2.2's clear(rec) fires when the plate is BUILT, before Engrave is
// entered. Every after-the-fact check passes whether the wipe is before or
// after Engrave returns; this one reads the buffer while the engrave screen is
// still on screen and the job still holds the spline.
//
// The reason it must be early: Back while the job is RUNNING does not return
// from Engrave (gui/gui.go:2648-2654 calls Stop() and keeps rendering), so
// waiting for the return would leave the seed resident for the whole ~21-minute
// cut and across the abort-mid-plate path §10.2.2 calls the machine's most
// ordinary recovery.
func TestSecretRecordIsZeroWHILETheEngraveScreenIsUp(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	first := firstSecretIdx(t, p)
	if allZeroBytes(p.Secret[first].Record) {
		t.Fatal("premise broken: the record is zero before Cut")
	}
	h.choose(0) // Cut this plate
	// engraveIdle's body. Reaching it means NewEngraveScreen was constructed
	// and Engrave was entered -- the plate exists and holds the geometry.
	h.mustReach("Insert a blank plate")

	// READ THE BUFFER WITHOUT LEAVING THE SCREEN.
	if !allZeroBytes(p.Secret[first].Record) {
		t.Fatalf("record %d is STILL RESIDENT while its engrave screen is up: %q\n"+
			"§10.2.2's clear(rec) must fire when the plate is BUILT, not when Engrave "+
			"returns -- Back while running calls Stop() and keeps rendering, so waiting "+
			"for the return leaves the seed resident across the whole cut and lets the "+
			"operator resume without a fresh unlock", first, p.Secret[first].Record)
	}
	// The hook must ALSO have seen it wiped -- but note the deferred backstop
	// has not run yet, because Engrave has not returned. So the wipe observed
	// above is the EARLY one and nothing else.
	for _, e := range *evts {
		if e.stage == "wiped" && e.idx == first {
			t.Fatal("the deferred backstop already ran; the engrave screen is not up, so " +
				"this test is not observing the early wipe it exists to pin")
		}
	}
}

// TestSecretSessionCancelledEngraveLeavesNothing — §10.2.2's literal bullet.
// Necessarily true given the row above, asserted anyway.
func TestSecretSessionCancelledEngraveLeavesNothing(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	first := firstSecretIdx(t, p)
	h.choose(0) // Cut
	h.mustReach("Insert a blank plate")
	h.tapNav(Button1) // Back out of the engrave screen -- the job is idle, so this returns
	h.mustReach("SECRET seed material")
	if !allZeroBytes(p.Secret[first].Record) {
		t.Errorf("a cancelled engrave left record %d resident: %q", first, p.Secret[first].Record)
	}
	assertSecretWiped(t, *evts, first)
}

// TestSecretSessionEngravesAMnemonic — vector A's single secret is a bare
// BIP-39 mnemonic, so it takes the OTHER arm: SeedScreen.Confirm, the bare
// fingerprint, engraveSeed.
func TestSecretSessionEngravesAMnemonic(t *testing.T) {
	p := unlockedPayload(t, "A")
	if len(p.Secret) != 1 || p.Secret[0].Class != seal.ClassMnemonic {
		t.Fatalf("premise broken: vector A must carry one BIP-39 mnemonic, got %d records "+
			"of class %v", len(p.Secret), p.Secret[0].Class)
	}
	evts := watchSecrets(t, p)
	h := runSecretSession(t, p)
	content := h.mustReach("SECRET seed material")
	// Labelled by CLASSIFIED type, never by anything the sealer asserted.
	if !uiContains(content, "seed words") {
		t.Errorf("a mnemonic secret is not labelled \"seed words\": %q", content)
	}
	h.choose(0) // Cut
	// SeedScreen draws the numbered word list.
	h.mustReach("1:")
	if allZeroBytes(p.Secret[0].Record) {
		t.Error("the record was wiped before SeedScreen confirmed it; the operator would " +
			"be confirming a plate built from nothing")
	}
	h.tapNav(Button3) // Confirm the seed
	h.mustReach("Insert a blank plate")
	if !allZeroBytes(p.Secret[0].Record) {
		t.Errorf("a mnemonic record is still resident while its engrave screen is up: %q",
			p.Secret[0].Record)
	}
	h.tapNav(Button1) // leave the engrave screen
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	assertSecretWiped(t, *evts, 0)
	if p.SecretsResident() {
		t.Error("SecretsResident() is true after the mnemonic session ended")
	}
}

// TestSecretsResidentIsFalseWhenTheSessionEnds — the §10.2.4 predicate B2b will
// key its timer on. It goes false when the LAST secret's plate is built, which
// for a completed session is when the session returns.
func TestSecretsResidentIsFalseWhenTheSessionEnds(t *testing.T) {
	p := unlockedPayload(t, "F")
	if !p.SecretsResident() {
		t.Fatal("premise broken: a freshly unlocked vector F has no resident secrets")
	}
	h := runSecretSession(t, p)
	for i := 0; i < 3; i++ {
		h.mustReach("SECRET seed material")
		h.choose(1) // Skip
	}
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("the session did not end")
	}
	if p.SecretsResident() {
		t.Error("SecretsResident() is still true when the session ended")
	}
	// The twelve md1/mk1 cards are UNTOUCHED -- they are not secret (§6.3) and
	// the plate list is about to offer them.
	live := 0
	for _, r := range p.Secret {
		if r.Class == seal.ClassMDMK && !allZeroBytes(r.Record) {
			live++
		}
	}
	if live != 12 {
		t.Errorf("%d of vector F's twelve encrypted md1/mk1 cards survived the session, "+
			"want all 12 -- the session must not wipe ordinary plates", live)
	}
}

// firstSecretIdx is the index of the first record the session will offer.
func firstSecretIdx(t *testing.T, p *seal.Payload) int {
	t.Helper()
	for i, r := range p.Secret {
		if seal.IsSecret(r.Class) {
			return i
		}
	}
	t.Fatal("the payload carries no secret record")
	return -1
}

// assertSecretWiped checks the hook OBSERVED the wipe for idx, with the buffer it was
// handed reading zero at that instant.
func assertSecretWiped(t *testing.T, evts []secretEvent, idx int) {
	t.Helper()
	for _, e := range evts {
		if e.stage == "wiped" && e.idx == idx {
			if !e.passedZero {
				t.Errorf("the wipe hook for record %d fired with a NON-ZERO buffer", idx)
			}
			return
		}
	}
	t.Errorf("no wipe was ever observed for record %d; events were %v", idx, evts)
}

// ---------------------------------------------------------------------------
// §6c — SeedScreen.NoEdit
// ---------------------------------------------------------------------------

// TestSeedScreenNoEditClosesBOTHRoutes.
//
// The guard goes on the CLICK HANDLER, not on the layout (R0 round 1, I1).
// Skipping only the nav slot does not close the route: Filter.matches
// (gui/event.go:155-159) gates a buttonEvent on button identity alone, with no
// bounds check, so editBtn.Clicked keeps consuming ButtonFilter(Button2)
// whether or not anything was drawn for it. The Button2 half of this test is
// the one that discriminates a handler guard from a layout guard.
//
// Both directions: with NoEdit set NEITHER route reaches word entry, and with
// it clear BOTH still do -- the second half is the existing scan-path behaviour
// and it must not regress.
func TestSeedScreenNoEditClosesBOTHRoutes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		noEdit   bool
		wantEdit bool
	}{
		{"NoEdit set", true, false},
		{"NoEdit clear (the scan path)", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range []string{"nav slot tap", "Button2"} {
				t.Run(route, func(t *testing.T) {
					m := validMnemonic(12)
					pf := newPlatform()
					pf.display = sh2DisplaySize
					ctx := NewContext(pf)
					ss := &SeedScreen{NoEdit: tc.noEdit}
					frame, drawer, quit := runUITouch(ctx, func() {
						ss.Confirm(ctx, &descriptorTheme, m)
					})
					defer quit()
					if _, ok := frame(); !ok {
						t.Fatal("SeedScreen produced no frame")
					}
					switch route {
					case "nav slot tap":
						if !tc.noEdit {
							tapNavSlot(t, ctx, drawer(), Button2)
						} else {
							// With NoEdit the slot is not drawn, so there is
							// nothing to tap -- assert that, then tap where it
							// would have been to prove no hidden target.
							if _, _, hit := drawer().Hit(navSlotPoint(ctx, Button2)); hit {
								t.Error("the edit nav slot still has a touch target with NoEdit set")
							}
							tap(&ctx.Router, drawer(), navSlotPoint(ctx, Button2))
						}
					case "Button2":
						// click, not press: Clickable.Next sets
						// clicked = !e.Pressed && c.Pressed
						// (gui/widget.go:74), so a bare press never clicks.
						click(&ctx.Router, Button2)
					}
					reached := false
					for i := 0; i < 16; i++ {
						c, ok := frame()
						if !ok {
							break
						}
						if uiContains(c, "Word 1 of 12") {
							reached = true
							break
						}
					}
					if reached != tc.wantEdit {
						if tc.wantEdit {
							t.Errorf("%s no longer reaches word entry with NoEdit clear; the "+
								"scan path's edit affordance regressed", route)
						} else {
							t.Errorf("%s still reaches word entry with NoEdit set -- editing an "+
								"authoritative payload seed produces a self-consistent plate "+
								"that does not restore the payload's wallet", route)
						}
					}
				})
			}
		})
	}
}

// TestUnlockSecretLabelNamesByClassification. Never by anything the sealer
// asserted, and never by rendering the record's contents.
func TestUnlockSecretLabelNamesByClassification(t *testing.T) {
	for _, tc := range []struct {
		c    seal.Classification
		i, n int
		want string
	}{
		{seal.ClassCodex32Secret, 0, 1, "ms1"},
		{seal.ClassCodex32Secret, 0, 3, "ms1 1/3"},
		{seal.ClassCodex32Secret, 2, 3, "ms1 3/3"},
		{seal.ClassMnemonic, 0, 1, "seed words"},
		{seal.ClassMnemonic, 1, 2, "seed words 2/2"},
		{seal.ClassMDMK, 0, 1, "secret"},
	} {
		if got := unlockSecretLabel(tc.c, tc.i, tc.n); got != tc.want {
			t.Errorf("unlockSecretLabel(%v, %d, %d) = %q, want %q", tc.c, tc.i, tc.n, got, tc.want)
		}
	}
}

// unused guard so bip39 stays imported if the mnemonic test is edited.
var _ = bip39.Mnemonic(nil)

// §10.2.2 for the SECOND copy. bip39.Parse returns an independent []Word of the
// same seed, and it is a local — no test could see it, which is how a full seed
// stayed live through an entire ~21-minute cut with the suite green (B2a-ii
// whole-diff review, lens 1 C1).
//
// The assertion is at the instant the plate reaches Engrave, NOT after the flow
// returns. That distinction is the whole test: a deferred clear(m) satisfies
// every after-the-fact check and leaves the seed resident for the cut.
func TestMnemonicWordsAreZeroWhenThePlateReachesEngrave(t *testing.T) {
	var atEngrave []bip39.Word
	unlockMnemonicHook = func(m bip39.Mnemonic) {
		atEngrave = append([]bip39.Word(nil), m...)
	}
	t.Cleanup(func() { unlockMnemonicHook = nil })

	// Vector A's single secret record is a bare 24-word mnemonic.
	p := unlockedPayload(t, "A")
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	h.choose(0) // Cut this plate
	// The mnemonic arm shows SeedScreen.Confirm first — the 24 words and an
	// "Engrave Seed" confirm. The codex32 arm has no such screen, which is why
	// its sibling test reaches the engrave in one step.
	h.mustReach("EngraveSeed")
	h.tapNav(Button3)
	h.mustReach("Insert a blank plate")

	// D2: pin clear(rec) on the MNEMONIC arm too. The sibling test covers it only
	// on the codex32 arm, because vector F's three secrets are all ms1 — so
	// deleting clear(rec) here left the suite green.
	first := firstSecretIdx(t, p)
	if !allZeroBytes(p.Secret[first].Record) {
		t.Fatalf("seal's record is STILL RESIDENT while the engrave screen is up: %q",
			p.Secret[first].Record)
	}

	if atEngrave == nil {
		t.Fatal("the plate never reached Engrave; the test asserted nothing")
	}
	if len(atEngrave) != 24 {
		t.Fatalf("observed %d words, want 24 — wrong vector or wrong hook", len(atEngrave))
	}
	for i, w := range atEngrave {
		if w != 0 {
			t.Fatalf("word %d is still %d at Engrave entry: the seed is live for the "+
				"whole cut, and neither p.Wipe() nor SecretsResident() can reach it", i, w)
		}
	}
}

// ---------------------------------------------------------------------------
// what the operator is actually pressing
// ---------------------------------------------------------------------------

// labelledTarget is one drawn touch target and the text drawn INSIDE it.
type labelledTarget struct {
	pt   image.Point
	text string
}

// runSecretSessionLabelled is runSecretSession with each touch target's own
// drawn label recorded beside it.
//
// It exists because every session test above selects a choice by TOUCH-TARGET
// INDEX, and nothing anywhere asserted which label sits at which index — so
// swapping the two shipped green, leaving the button labelled "Skip" cutting the
// seed plate and the button labelled "Cut this plate" wiping the record
// (B2a-ii whole-diff review, lens 2 I5).
//
// The extraction happens INSIDE the frame callback, where the op tree is still
// live. op.Drawer.ExtractText collects only the glyphs whose clip meets the
// rectangle it is handed (gui/op/op.go), so reading against one target's own
// clip rect yields that target's label and nothing else — which is what turns
// "the Nth target does X" into "the target LABELLED Skip does X".
func runSecretSessionLabelled(t *testing.T, p *seal.Payload) *sessionHarness {
	t.Helper()
	pf := newPlatform()
	pf.display = sh2DisplaySize
	pf.engraver = newEngraver()
	ctx := NewContext(pf)
	returned := false
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	var last *op.Drawer
	var lastTargets []labelledTarget
	next, quit := iter.Pull(func(yield func(content string) bool) {
		ctx.FrameCallback = func(o op.Op) {
			r := image.Rectangle{Max: ctx.Platform.DisplaySize()}
			d := new(op.Drawer)
			content := d.ExtractText(r, o)
			last, lastTargets = d, nil
			for _, pt := range plateHitPoints(ctx, d) {
				_, rect, ok := d.Hit(pt)
				if !ok {
					continue
				}
				// ChoiceScreen.Draw expands each label's background by
				// buttonPadY top and bottom (gui/gui.go), so ADJACENT targets
				// overlap by that much. Trim back to the label's own rows
				// before reading, or a target can be credited with its
				// neighbour's text.
				rect.Min.Y += buttonPadY
				rect.Max.Y -= buttonPadY
				lastTargets = append(lastTargets, labelledTarget{
					pt: pt, text: new(op.Drawer).ExtractText(rect, o),
				})
			}
			ctx.Reset()
			ctx.Done = ctx.Done || !yield(content)
		}
		unlockSecretSession(ctx, &descriptorTheme, p)
		returned = true
	})
	h.frame = next
	h.drawer = func() *op.Drawer { return last }
	h.targets = func() []labelledTarget { return lastTargets }
	t.Cleanup(quit)
	return h
}

// chooseLabelled taps the choice whose DRAWN LABEL is want, then confirms.
//
// It fails if no target carries that label, or if two do. That refusal is the
// assertion: choose(n) cannot tell a screen whose buttons were swapped from one
// that was not.
func (h *sessionHarness) chooseLabelled(want string) {
	h.t.Helper()
	if h.targets == nil {
		h.t.Fatal("this harness records no labels; build it with runSecretSessionLabelled")
	}
	var pt image.Point
	var drawn []string
	found := 0
	for _, tg := range h.targets() {
		drawn = append(drawn, tg.text)
		if uiContains(tg.text, want) {
			pt = tg.pt
			found++
		}
	}
	if found != 1 {
		h.t.Fatalf("%d touch targets are labelled %q; the screen drew %q (%q)",
			found, want, drawn, h.content)
	}
	tap(&h.ctx.Router, h.drawer(), pt)
	h.next("after selecting " + want)
	h.tapNav(Button3)
	if c, ok := h.frame(); ok {
		h.content = c
	}
}

// TestSecretPlateChoicesAreBoundToTheirLabels — §10.2.2's two buttons, bound to
// the outcomes their text promises.
//
// Both directions of the swap are harmful on a screen whose whole job is
// deciding whether seed material becomes steel: one engraves a seed the operator
// asked NOT to engrave, the other wipes a secret record they asked to cut — and
// on the last secret the session then simply ends, which is §6.4's
// incomplete-backup-believed-complete again.
//
// So both are driven, in one session: Skip first (it must build no plate, and
// must still wipe), then Cut on the next record (it must reach the engrave
// screen).
func TestSecretPlateChoicesAreBoundToTheirLabels(t *testing.T) {
	p := unlockedPayload(t, "F")
	evts := watchSecrets(t, p)
	h := runSecretSessionLabelled(t, p)
	h.mustReach("SECRET seed material")

	// Both labels are drawn, on real targets, exactly once each.
	var drawn []string
	for _, tg := range h.targets() {
		drawn = append(drawn, tg.text)
	}
	if len(drawn) != 2 {
		t.Fatalf("the secret plate screen drew %d touch targets (%q), want 2 -- §10.2.2 "+
			"gives the operator Cut or Skip and deliberately no third option", len(drawn), drawn)
	}

	first := firstSecretIdx(t, p)
	if allZeroBytes(p.Secret[first].Record) {
		t.Fatal("premise broken: the record is zero before the choice")
	}

	// SKIP builds no plate.
	h.chooseLabelled("Skip")
	reached := false
	for i := 0; i < 64; i++ {
		if uiContains(h.content, "Insert a blank plate") {
			t.Fatalf("the button labelled \"Skip\" built a plate: %q\n"+
				"it engraves a seed the operator asked not to engrave", h.content)
		}
		if uiContains(h.content, "SECRET seed material") {
			reached = true
			break
		}
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
	}
	if !reached {
		t.Fatalf("Skip did not advance to the next secret; last frame %q", h.content)
	}
	// ...and it still wipes, which is the half that is NOT swappable.
	if !allZeroBytes(p.Secret[first].Record) {
		t.Errorf("Skip left record %d resident: %q", first, p.Secret[first].Record)
	}
	assertSecretWiped(t, *evts, first)

	// CUT builds one.
	h.chooseLabelled("Cut this plate")
	h.mustReach("Insert a blank plate")
}

// TestPayloadSeedScreenRefusesEditing — §6c's NoEdit, at the ONE production call
// site that sets it.
//
// TestSeedScreenNoEditClosesBOTHRoutes is a good test of the WIDGET and kills
// both the handler-guard and the layout-guard mutants. What nothing covered is
// that unlockEngraveMnemonic actually SETS the flag: `&SeedScreen{NoEdit: true}`
// could be written `&SeedScreen{}` with the whole suite green (B2a-ii whole-diff
// review, lens 2 I4).
//
// The harm is the one unlock_session.go's own comment states: with the flag
// clear the operator can edit a word of an AUTHORITATIVE payload seed;
// inputWordsFlow mutates m in place, and masterFingerprintFor(m, …) and
// engraveSeed(params, m, mfp) then both read the edited value — so the steel
// plate is internally self-consistent, carries a matching fingerprint, and does
// not restore the payload's wallet. Nothing on the plate or the screen says so.
func TestPayloadSeedScreenRefusesEditing(t *testing.T) {
	// Vector A's single secret is a bare 24-word mnemonic, so it takes the arm
	// that builds the SeedScreen.
	p := unlockedPayload(t, "A")
	if len(p.Secret) != 1 || p.Secret[0].Class != seal.ClassMnemonic {
		t.Fatalf("premise broken: vector A must carry one BIP-39 mnemonic, got %d records",
			len(p.Secret))
	}
	h := runSecretSession(t, p)
	h.mustReach("SECRET seed material")
	h.choose(0) // Cut this plate
	h.mustReach("EngraveSeed")

	// The layout route: with NoEdit the edit slot is not drawn, so there is no
	// target to hit.
	if _, _, hit := h.drawer().Hit(navSlotPoint(h.ctx, Button2)); hit {
		t.Error("the edit nav slot still carries a touch target on a PAYLOAD seed")
	}
	// The HANDLER route, which is the one that discriminates: Filter.matches
	// gates a buttonEvent on button identity alone with no bounds check
	// (gui/event.go), so editBtn.Clicked keeps consuming ButtonFilter(Button2)
	// whether or not anything was drawn for it.
	click(&h.ctx.Router, Button2)
	for i := 0; i < 16; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
		if uiContains(c, "Word 1 of") {
			t.Fatalf("Button2 reached word entry on a payload seed: %q\n"+
				"editing an authoritative payload seed produces a self-consistent plate, "+
				"with a matching fingerprint, that does not restore the payload's wallet", c)
		}
	}
}

// TestSecretSessionNumbersEachClassSeparately — the pass-3 fold (commit
// d0baf13), which rewrote unlockSecretSession to count seen/total PER
// Classification and landed with no test that can fail (B2a-ii whole-diff
// review, lens 2 I6).
//
// No canonical vector mixes the two secret classes — A 0/1, B 0/1, C 0/6, D 5/1,
// E 5/0, F 0/15, G 12/3 are all-ms1 or all-mnemonic — so reverting the fold
// survived the whole suite, and TestUnlockSecretLabelNamesByClassification
// exercises the FORMATTER with hand-supplied i and n, never the counter that
// feeds it. This builds the missing shape from two vectors' own records.
//
// Numbering across all secrets while naming per class renders "ms1 1/2" then
// "seed words 2/2", which tells the operator there are two ms1 cards and they
// are holding the second.
func TestSecretSessionNumbersEachClassSeparately(t *testing.T) {
	d := sealVector(t, "D") // one ms1
	a := sealVector(t, "A") // one bare 24-word mnemonic
	if len(d.Secret) != 1 || len(a.Secret) != 1 {
		t.Fatalf("premise broken: D has %d and A has %d secret records, want one each",
			len(d.Secret), len(a.Secret))
	}
	blob := sealBlobForTest(t, nil, []string{d.Secret[0], a.Secret[0]},
		fixturePassphrase, fixtureIterations)
	p := unlockedBlob(t, blob, fixturePassphrase)
	if len(p.Secret) != 2 {
		t.Fatalf("the fixture admitted %d records, want 2", len(p.Secret))
	}
	if p.Secret[0].Class != seal.ClassCodex32Secret || p.Secret[1].Class != seal.ClassMnemonic {
		t.Fatalf("the fixture is not the mixed-class shape: %v then %v",
			p.Secret[0].Class, p.Secret[1].Class)
	}

	h := runSecretSession(t, p)
	// ONE of each class, so NEITHER may be numbered: unlockSecretLabel adds
	// "i+1/n" only when n > 1, and n counts this class alone.
	one := h.mustReach("SECRET seed material")
	if !uiContains(one, "ms1") {
		t.Errorf("the first secret is not labelled \"ms1\": %q", one)
	}
	if uiContains(one, "1/2") {
		t.Errorf("the ms1 card is numbered ACROSS classes: %q\n"+
			"this tells the operator there are two ms1 cards and they hold the first", one)
	}
	h.choose(1) // Skip
	two := h.mustReach("SECRET seed material")
	if !uiContains(two, "seed words") {
		t.Errorf("the second secret is not labelled \"seed words\": %q", two)
	}
	if uiContains(two, "2/2") {
		t.Errorf("the mnemonic card is numbered ACROSS classes: %q\n"+
			"this tells the operator they are holding the second of two seed-word cards", two)
	}
}

// unlockedBlob is unlockedPayload for a blob that no single vector can supply.
func unlockedBlob(t *testing.T, blob []byte, passphrase string) *seal.Payload {
	t.Helper()
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := seal.DeriveKey(seal.NormalisePassphrase(passphrase), p.Header.Salt[:],
		int(p.Header.Iterations))
	defer clear(key)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	for i, r := range p.Secret {
		if allZeroBytes(r.Record) {
			t.Fatalf("record %d arrived already zero", i)
		}
	}
	return p
}
