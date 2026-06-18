package gui

import (
	"encoding/hex"
	"testing"
	"time"

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

// Silence unused warnings for fixtures/helpers consumed by later tasks during
// incremental TDD; removed once those tests land.
var _ = slip39Vec3SecretEmpty
var _ = slip39Vec3SecretTrezor
var _ = slip39MultiGroup
var _ = slip39MultiGroupSecret
var _ = time.Second
var _ = hexOfEntropy
