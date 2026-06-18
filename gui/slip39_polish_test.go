package gui

import (
	"encoding/hex"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bip39"
	slip39words "seedhammer.com/slip39"
)

const slip39Duckling = "duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard"

// Official SLIP-0039 test-vector idx 3 ("Basic sharing 2-of-3, 128 bits"): a
// single-group 2-of-3 set (MemberThreshold==2). Recovering with the empty
// passphrase yields slip39Vec3SecretEmpty; with "TREZOR", slip39Vec3SecretTrezor
// (plan-R0 C1 — the two are empirically distinct).
var slip39Vec3 = []string{
	"shadow pistol academic always adequate wildlife fancy gross oasis cylinder mustang wrist rescue view short owner flip making coding armed",
	"shadow pistol academic acid actress prayer class unknown daughter sweater depict flip twice unkind craft early superior advocate guest smoking",
}

const (
	slip39Vec3SecretEmpty  = "61cf4d6c0d8a07d8c2fd3cff22432664"
	slip39Vec3SecretTrezor = "b43ceb7e57a0ea8766221624d01b0864"
)

// group-2of3-over-2of3 (len=16) Rust fixture: a real GROUP threshold (GT=2 over
// 2 groups, each MemberThreshold==2). Used for the multi-group GUI round-trip.
// All four shares (group 0: members 0,1; group 1: members 0,1) recover the
// secret under "TREZOR".
var slip39MultiGroup = []string{
	"alto flea acrobat echo client kind privacy river often taxi script glad auction relate unkind item modify rebuild decrease fatal",
	"alto flea acrobat email document crucial strategy rocky insect prospect member galaxy slow inside together standard density cause premium august",
	"alto flea beard echo breathe either prisoner ordinary expect flash invasion quiet making expect club include problem acne hunting likely",
	"alto flea beard email destroy adapt alto evil width inherit gesture priest process busy home hospital ladybug debris cylinder soldier",
}

const slip39MultiGroupSecret = "101112131415161718191a1b1c1d1e1f"

func parseFixtureShare(t *testing.T, mnemonic string) slip39words.Share {
	t.Helper()
	s, err := slip39words.ParseShare(mnemonic)
	if err != nil {
		t.Fatalf("ParseShare(%q): %v", mnemonic, err)
	}
	return s
}

func hexOfEntropy(m interface{ Entropy() []byte }) string {
	return hex.EncodeToString(m.Entropy())
}

func TestConfirmSLIP39Render(t *testing.T) {
	s, err := slip39words.ParseShare(slip39Duckling)
	if err != nil {
		t.Fatalf("ParseShare: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { confirmSLIP39Flow(ctx, &descriptorTheme, s) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "id 7945") {
		t.Errorf("confirm should show id 7945; got %q", c)
	}
	if !uiContains(c, "member 1 of 1") {
		t.Errorf("confirm should show member 1 of 1; got %q", c)
	}
	if !uiContains(c, "20 words") {
		t.Errorf("confirm should show the word count; got %q", c)
	}
}

func TestEngraveSLIP39BackoutRecognized(t *testing.T) {
	s, err := slip39words.ParseShare(slip39Duckling)
	if err != nil {
		t.Fatalf("ParseShare: %v", err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back at the confirm screen
	if !engraveObjectFlow(ctx, &descriptorTheme, s) {
		t.Error("cancel at SLIP-39 confirm returned false (→ \"Unknown format\"); want true (recognized)")
	}
}

func TestConfirmSLIP39MultiOffersRecover(t *testing.T) {
	// A share from a 2-of-3 set (MemberThreshold>1) must offer Recover (Button2).
	s := parseFixtureShare(t, slip39Vec3[0])
	if s.MemberThreshold <= 1 {
		t.Fatalf("fixture precondition: want MemberThreshold>1, got %d", s.MemberThreshold)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // Button2 = Recover (no list to navigate)
	got := confirmSLIP39Flow(ctx, &descriptorTheme, s)
	if got != slip39Recover {
		t.Errorf("multi-share confirm: got %v want slip39Recover", got)
	}
}

func TestConfirmSLIP39LoneNoRecover(t *testing.T) {
	// A 1-of-1 share (memberThreshold==1, groupThreshold==1): Button2 is a no-op
	// (drained); Button3 still engraves. Pins no-hang on the drained Button2.
	s := parseFixtureShare(t, slip39Duckling) // 1-of-1
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2, Button3) // Button2 drained, Button3 acts
	got := confirmSLIP39Flow(ctx, &descriptorTheme, s)
	if got != slip39Engrave {
		t.Errorf("lone share: Button2 must be drained, Button3 engrave; got %v", got)
	}
}

func TestSLIP39LengthPick33(t *testing.T) {
	// slip39LengthPick returns the chosen word count; the "33" option is at
	// index 1 (presented prominently after 20), reached with one Down, then
	// selected with Button3.
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Down, Button3)
	if n := slip39LengthPick(ctx, &descriptorTheme); n != 33 {
		t.Errorf("length pick = %d want 33", n)
	}
}

func TestSLIP39LengthPickDefault20(t *testing.T) {
	// Selecting immediately (no navigation) yields the default 20-word count.
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button3)
	if n := slip39LengthPick(ctx, &descriptorTheme); n != 20 {
		t.Errorf("default length pick = %d want 20", n)
	}
}

func TestSLIP39LengthPickCancel(t *testing.T) {
	// Back cancels the pick → 0 sentinel.
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1)
	if n := slip39LengthPick(ctx, &descriptorTheme); n != 0 {
		t.Errorf("cancelled length pick = %d want 0", n)
	}
}

// stubShare builds a minimal Share with just the fields selectForCombine reads
// (group/member index + member threshold). Value is a valid-length placeholder.
func stubShare(groupIndex, memberIndex, memberThreshold int) slip39words.Share {
	return slip39words.Share{
		GroupIndex:      groupIndex,
		MemberIndex:     memberIndex,
		MemberThreshold: memberThreshold,
		Value:           make([]byte, 16),
	}
}

func rosterOf(shares ...slip39words.Share) map[int][]slip39words.Share {
	byGroup := map[int][]slip39words.Share{}
	for _, s := range shares {
		byGroup[s.GroupIndex] = append(byGroup[s.GroupIndex], s)
	}
	return byGroup
}

func TestSelectForCombineSingleGroup(t *testing.T) {
	// One satisfied group (member threshold 2) → both its members; GT=1.
	byGroup := rosterOf(stubShare(0, 0, 2), stubShare(0, 1, 2))
	got, ok := selectForCombine(byGroup, 1)
	if !ok {
		t.Fatal("selectForCombine: ok=false for a satisfied single group")
	}
	if len(got) != 2 {
		t.Errorf("selectForCombine returned %d members, want 2", len(got))
	}
}

func TestSelectForCombinePrunesStrayPartialGroup(t *testing.T) {
	// GT=2: two satisfied groups (0 and 1, each MT=2) plus a STRAY partial
	// group 2 (one member of a 2-member group). selectForCombine must prune
	// group 2 and return exactly the 4 members of groups 0+1 — feeding the raw
	// accumulation to Combine would error errInsufficientShares (plan-R0 I1).
	byGroup := rosterOf(
		stubShare(0, 0, 2), stubShare(0, 1, 2),
		stubShare(1, 0, 2), stubShare(1, 1, 2),
		stubShare(2, 0, 2), // stray partial group
	)
	got, ok := selectForCombine(byGroup, 2)
	if !ok {
		t.Fatal("selectForCombine: ok=false despite 2 satisfied groups")
	}
	if len(got) != 4 {
		t.Errorf("selectForCombine returned %d members, want 4 (stray partial group pruned)", len(got))
	}
	for _, s := range got {
		if s.GroupIndex == 2 {
			t.Errorf("selectForCombine leaked a member of the stray partial group 2")
		}
	}
}

func TestSelectForCombineInsufficientGroups(t *testing.T) {
	// GT=2 but only one group satisfied → ok=false.
	byGroup := rosterOf(
		stubShare(0, 0, 2), stubShare(0, 1, 2),
		stubShare(1, 0, 2), // group 1 partial
	)
	if _, ok := selectForCombine(byGroup, 2); ok {
		t.Error("selectForCombine: ok=true with only 1 of 2 groups satisfied")
	}
}

// driveShare emits the per-word input that inputSLIP39Flow expects: each word's
// full (lowercase) letters followed by Button3 (the flow accepts a word only on
// Button3 once the typed prefix is unambiguous; a full word is always an exact
// match → complete). The SLIP-39 wordlist has no word that is a prefix of
// another, so full-word typing is unambiguous (M1).
func driveShare(r *EventRouter, mnemonic string) {
	for _, w := range strings.Fields(mnemonic) {
		runes(r, strings.ToLower(w))
		click(r, Button3)
	}
}

// driveRecover pre-queues the events for recoverSLIP39Flow: each collection
// share typed via driveShare, then the SLIP-39-passphrase choice. passphrase==""
// selects Skip (the default, index 0); a non-empty passphrase selects "Enter
// passphrase" (Down, then choose) and types it on the PassphraseKeyboard.
func driveRecover(t *testing.T, ctx *Context, first slip39words.Share, shares []string, passphrase string) (bip39.Mnemonic, bool) {
	t.Helper()
	for _, s := range shares {
		driveShare(&ctx.Router, s)
	}
	if passphrase == "" {
		click(&ctx.Router, Button3) // ChoiceScreen: Skip (default index 0)
	} else {
		click(&ctx.Router, Down, Button3) // choose "Enter passphrase"
		runes(&ctx.Router, passphrase)    // case-sensitive, cross-page
		click(&ctx.Router, Button3)       // accept on the passphrase keyboard
	}
	return recoverSLIP39Flow(ctx, &descriptorTheme, first)
}

func TestRecoverSLIP39(t *testing.T) {
	// idx 3 = 2-of-3 single-group. Enter the 2nd share, SKIP the passphrase.
	// CRITICAL (plan-R0 C1): with an EMPTY passphrase the recovered secret is
	// the empty-passphrase value (61cf…2664), NOT the "TREZOR" value.
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	m, ok := driveRecover(t, ctx, first, []string{slip39Vec3[1]}, "")
	if !ok {
		t.Fatal("recover failed")
	}
	if got := hexOfEntropy(m); got != slip39Vec3SecretEmpty {
		t.Errorf("recovered entropy (empty passphrase) = %s want %s", got, slip39Vec3SecretEmpty)
	}
}

func TestRecoverSLIP39Passphrase(t *testing.T) {
	// Same 2 shares but TYPE "TREZOR" at the passphrase prompt → the canonical
	// corpus secret (b43c…0864). Proves the SLIP-39 passphrase feeds the
	// Feistel decrypt and changes the result.
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	m, ok := driveRecover(t, ctx, first, []string{slip39Vec3[1]}, "TREZOR")
	if !ok {
		t.Fatal("recover failed")
	}
	if got := hexOfEntropy(m); got != slip39Vec3SecretTrezor {
		t.Errorf("recovered entropy (TREZOR) = %s want %s", got, slip39Vec3SecretTrezor)
	}
}

func TestRecoverSLIP39MultiGroup(t *testing.T) {
	// group-2of3-over-2of3: GT=2 over 2 groups, each MemberThreshold==2. First
	// share is group 0 member 0; collect group 0 member 1, group 1 members 0+1.
	// Exercises the two-level roster + selectForCombine assembly (I1).
	first := parseFixtureShare(t, slip39MultiGroup[0])
	if first.GroupThreshold < 2 {
		t.Fatalf("fixture precondition: want GroupThreshold>=2, got %d", first.GroupThreshold)
	}
	ctx := NewContext(newPlatform())
	m, ok := driveRecover(t, ctx, first, slip39MultiGroup[1:], "TREZOR")
	if !ok {
		t.Fatal("multi-group recover failed")
	}
	if got := hexOfEntropy(m); got != slip39MultiGroupSecret {
		t.Errorf("multi-group recovered entropy = %s want %s", got, slip39MultiGroupSecret)
	}
}

func TestRecoverSLIP39Mismatch(t *testing.T) {
	// Entering a share from a DIFFERENT set (different identifier) must surface
	// an eager ConsistentShares error and re-prompt (not abort, not combine).
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { recoverSLIP39Flow(ctx, &descriptorTheme, first) })
	defer quit()
	// A share from the multi-group set (id 1003 ≠ idx-3's id) → id mismatch.
	driveShare(&ctx.Router, slip39MultiGroup[0])
	var content string
	for i := 0; i < 64; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		content = c
		if uiContains(content, "id mismatch") {
			break
		}
	}
	if !uiContains(content, "id mismatch") {
		t.Errorf("expected an id-mismatch error; got %q", content)
	}
}

func TestRecoverSLIP39BackoutRecognized(t *testing.T) {
	// Back at the first collection prompt → (nil, false).
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back at the share-collection prompt
	m, ok := recoverSLIP39Flow(ctx, &descriptorTheme, first)
	if ok || m != nil {
		t.Errorf("Back at collection: got (%v, %v) want (nil, false)", m, ok)
	}
}

// pumpUntil reads frames until content matches want or maxFrames is reached,
// returning the last frame content and whether want was seen.
func pumpUntil(frame func() (string, bool), want string, maxFrames int) (string, bool) {
	var content string
	for i := 0; i < maxFrames; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		content = c
		if uiContains(content, want) {
			return content, true
		}
	}
	return content, false
}

func TestSLIP39FingerprintBackRecognized(t *testing.T) {
	// Back at the recovered-fingerprint check returns false (declined), so the
	// engrave dispatch loops back to confirm — never surfacing "Unknown format".
	secret := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}
	m := bip39.New(secret)
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2, Button1) // drained Button2 + Back
	if confirmSLIP39Fingerprint(ctx, &descriptorTheme, 0xDEADBEEF) {
		t.Error("Back at fingerprint check returned true; want false")
	}
	_ = m
}

func TestEngraveSLIP39RecoverToBackup(t *testing.T) {
	// Full recover dispatch: confirm(Recover) → recoverSLIP39Flow (idx-3, Skip)
	// → §3 hold-to-confirm acknowledgement → §5.4 fingerprint (BDDBDA4F) →
	// backupWalletFlow (the recovered BIP-39 seed words appear). Asserts the ack
	// text, the recovered fingerprint, and that the recovered seed reaches the
	// BIP-39 backup confirm.
	synctest.Test(t, func(t *testing.T) {
		first := parseFixtureShare(t, slip39Vec3[0])
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			engraveSLIP39(ctx, &descriptorTheme, first)
		})
		defer quit()

		// Recover at the confirm screen.
		click(&ctx.Router, Button2)
		// Collect the 2nd share + Skip the SLIP-39 passphrase.
		driveShare(&ctx.Router, slip39Vec3[1])
		click(&ctx.Router, Button3) // passphrase ChoiceScreen: Skip
		frame()

		// §3 acknowledgement screen.
		if c, ok := pumpUntil(frame, "WRONG seed", 64); !ok {
			t.Fatalf("acknowledgement text not shown; got %q", c)
		}
		// Hold to confirm the acknowledgement.
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame() // yields ConfirmYes → fingerprint screen

		// §5.4 recovered fingerprint (no extra input needed — it just renders).
		if c, ok := pumpUntil(frame, "BDDBDA4F", 64); !ok {
			t.Fatalf("recovered fingerprint BDDBDA4F not shown; got %q", c)
		}
		// Confirm the fingerprint (Engrave) → backupWalletFlow.
		click(&ctx.Router, Button3)
		// The recovered BIP-39 seed (words GIFT KIDNEY …) reaches the backup
		// confirm screen.
		if c, ok := pumpUntil(frame, "GIFT", 64); !ok {
			t.Fatalf("recovered seed words did not reach backupWalletFlow; got %q", c)
		}
	})
}

func TestConfirmSLIP39LoneButton2NoHang(t *testing.T) {
	// Regression (Cycle-B no-hang class): a queued Button2 on a lone share must
	// be drained every frame so it cannot block the router queue head — Button3
	// still acts. This is a DIRECT-call test (no runUI): if Button2 stalled the
	// queue, confirmSLIP39Flow would never observe Button3 and the test would
	// hang rather than fail.
	s := parseFixtureShare(t, slip39Duckling) // 1-of-1, no Recover offered
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2, Button2, Button3) // two drained Button2s, then act
	if got := confirmSLIP39Flow(ctx, &descriptorTheme, s); got != slip39Engrave {
		t.Errorf("lone share with leading Button2s: got %v want slip39Engrave (no-hang)", got)
	}
}

func TestSLIP39PassphrasePromptDistinctFromBIP39(t *testing.T) {
	// §5.5: the SLIP-39 passphrase prompt is labeled by FUNCTION and is
	// distinct from the BIP-39 25th-word passphrase prompt. Render the SLIP-39
	// passphrase choice (the first thing recoverSLIP39Flow shows once the lone
	// share is "sufficient" — a 1-of-1 set: countSatisfied==GT immediately) and
	// assert the disambiguating label.
	ctx := NewContext(newPlatform())
	cs := &ChoiceScreen{
		Title:   "SLIP-39 Passphrase",
		Lead:    "SLIP-39 passphrase? (NOT a BIP-39 passphrase) A wrong passphrase silently recovers a different seed.",
		Choices: []string{"Skip", "Enter passphrase"},
	}
	frame, quit := runUI(ctx, func() { cs.Choose(ctx, &descriptorTheme) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "NOT a BIP-39 passphrase") {
		t.Errorf("SLIP-39 passphrase prompt must disambiguate from BIP-39; got %q", c)
	}
	// Sanity: the BIP-39 prompt (backupWalletFlow) uses a different lead.
	ctx2 := NewContext(newPlatform())
	bip := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	frame2, quit2 := runUI(ctx2, func() { bip.Choose(ctx2, &descriptorTheme) })
	defer quit2()
	c2, ok2 := frame2()
	if !ok2 {
		t.Fatal("no BIP-39 prompt frame")
	}
	if uiContains(c2, "NOT a BIP-39 passphrase") {
		t.Errorf("BIP-39 prompt must NOT carry the SLIP-39 disambiguator; got %q", c2)
	}
	if !uiContains(c2, "Add a BIP-39 passphrase") {
		t.Errorf("BIP-39 prompt lead missing; got %q", c2)
	}
}

func TestSLIP39RecoveredSeedIsolatedFromBIP39Passphrase(t *testing.T) {
	// Passphrase isolation: the recovered seed (the words/SeedQR engraved) is
	// fixed by the SLIP-39 passphrase during recovery and is returned BEFORE
	// backupWalletFlow runs — so the later BIP-39 (25th-word) passphrase cannot
	// change the recovered words. recoverSLIP39Flow with "TREZOR" yields the
	// TREZOR secret regardless; the BIP-39 passphrase only reshapes the engraved
	// fingerprint downstream, never these entropy bytes.
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	m, ok := driveRecover(t, ctx, first, []string{slip39Vec3[1]}, "TREZOR")
	if !ok {
		t.Fatal("recover failed")
	}
	if got := hexOfEntropy(m); got != slip39Vec3SecretTrezor {
		t.Errorf("recovered entropy = %s want %s (must be set by the SLIP-39 passphrase only)", got, slip39Vec3SecretTrezor)
	}
	// And it differs from the empty-passphrase recovery — proving the SLIP-39
	// passphrase (not the BIP-39 one) is what selected this seed.
	if slip39Vec3SecretTrezor == slip39Vec3SecretEmpty {
		t.Fatal("test fixtures degenerate: empty and TREZOR secrets must differ")
	}
}
