package gui

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bip85"
	"seedhammer.com/engrave"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
)

// validBip85Words is the set of child word counts the BIP-39 application
// supports (biptool's guard n<12||24<n||n%3!=0 -> exactly {12,18,24}).
func validBip85Words(n int) bool {
	return n == 12 || n == 18 || n == 24
}

// bip85MaxIndex is the BIP-85 / BIP-32 hardened-child ceiling: an un-hardened
// index in [0, 2^31-1]. It equals hdkeychain.HardenedKeyStart-1; biptool rejects
// anything >= HardenedKeyStart with "bip32: path element out of range".
const bip85MaxIndex = hdkeychain.HardenedKeyStart - 1 // = 2147483647 = 2^31-1

// parseBip85Index parses a typed decimal child index, WIDTH-SAFE on every target.
// It uses strconv.ParseUint(s,10,64) — NEVER a bare int — so a value > the 64-bit
// host's int is still caught, not wrapped. It rejects empty input, any non-[0-9]
// rune (sign, whitespace, '.', "0x", letters — all typeable on the keyboard), and
// any value > 2^31-1 (the hardened max). Leading zeros are accepted ("007" -> 7),
// matching base-10 ParseUint. The returned value is guaranteed in [0, 2^31-1], so
// it fits an int on every target.
func parseBip85Index(s string) (int, error) {
	if s == "" {
		return 0, errors.New("bip85: empty index")
	}
	v, err := strconv.ParseUint(s, 10, 64) // base 10; rejects sign/whitespace/0x/letters/overflow
	if err != nil {
		return 0, fmt.Errorf("bip85: invalid index %q", s)
	}
	if v > bip85MaxIndex {
		return 0, fmt.Errorf("bip85: index %s exceeds the maximum %d", s, bip85MaxIndex)
	}
	return int(v), nil // safe: v <= 2^31-1
}

// deriveBip85Child re-creates biptool's `derive bip39` (cmd/biptool/main.go:137-189)
// from a TYPED master mnemonic + optional passphrase: it walks the FULLY-hardened
// BIP-85 path m/83696968'/39'/0'/{words}'/{index}', extracts the leaf's 32-byte EC
// private key, runs bip85.Entropy (HMAC-SHA512), keeps the LEADING entLen bytes,
// and maps them to a child BIP-39 mnemonic via bip39.New.
//
// SECURITY: every secret buffer is scrubbed before return — the PBKDF2 seed, each
// intermediate ExtendedKey (.Zero), the privkey serialization, and the 64-byte
// HMAC output (wipeBytes). The caller still owns scrubbing the master and the
// returned child mnemonic (see bip85DeriveFlow). Deterministic: no CSPRNG.
func deriveBip85Child(m bip39.Mnemonic, passphrase string, words, index int) (bip39.Mnemonic, error) {
	if !validBip85Words(words) {
		return nil, fmt.Errorf("bip85: invalid child word count: %d", words)
	}
	if index < 0 {
		return nil, fmt.Errorf("bip85: invalid index: %d", index)
	}
	if index > bip85MaxIndex {
		// Defense-in-depth (independent of the picker's parseBip85Index): a 64-bit
		// host int > 2^31-1 would otherwise be silently truncated/wrapped by the
		// uint32(index)+h cast below into a different/UNHARDENED element. Reject it.
		return nil, fmt.Errorf("bip85: index %d exceeds the maximum %d", index, bip85MaxIndex)
	}

	const h = hdkeychain.HardenedKeyStart
	seed := bip39.MnemonicSeed(m, passphrase)
	defer wipeBytes(seed)

	xkey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	// Fully-hardened path: 83696968' / 39' / 0' (English) / words' / index'.
	path := []uint32{
		bip85.PathRoot,
		39 + h,
		0 + h,
		uint32(words) + h,
		uint32(index) + h,
	}
	k := xkey
	for _, p := range path {
		next, derr := k.Derive(p)
		k.Zero() // scrub master + each intermediate (Derive returns fresh buffers)
		if derr != nil {
			return nil, derr
		}
		k = next
	}
	// Leaf EC private key. ECPrivKey returns (*PrivateKey, error); it cannot fire
	// for a master+hardened walk, but never .Serialize() a nil.
	pkey, err := k.ECPrivKey()
	if err != nil {
		k.Zero()
		return nil, err
	}
	priv := pkey.Serialize() // 32-byte secret
	k.Zero()
	defer wipeBytes(priv)

	hmacOut := bip85.Entropy(priv) // 64-byte secret
	defer wipeBytes(hmacOut)

	entLen := (words*11 - words/3) / 8 // 12->16, 18->24, 24->32
	if entLen < 16 || entLen > 32 || entLen%4 != 0 {
		// Unreachable for words in {12,18,24}; guard so bip39.New never panics.
		return nil, errors.New("bip85: internal entropy-length error")
	}
	child := bip39.New(hmacOut[:entLen]) // LEADING entLen bytes
	return child, nil
}

// engraveBip85Child computes the CHILD's OWN bare-seed master fingerprint and
// engraves the child mnemonic (words + standard SeedQR) via the engraveSeed
// PRIMITIVE — the exact Backup-Wallet path. R0-I-A: the plate's MasterFingerprint
// MUST be the child's own bare fp (the child is a bare mnemonic, no passphrase),
// NEVER the master's, otherwise the steel carries a fingerprint that does not
// match the engraved words. This skips backupWalletFlow's passphrase-fp picker.
func engraveBip85Child(params engrave.Params, child bip39.Mnemonic) (Plate, uint32, error) {
	mfp, err := masterFingerprintFor(child, &chaincfg.MainNetParams, "") // child's OWN bare fp; propagate err (R0-A1)
	if err != nil {
		return Plate{}, 0, err
	}
	plate, err := engraveSeed(params, child, mfp)
	if err != nil {
		return Plate{}, 0, err
	}
	return plate, mfp, nil
}

// bip85WordChoices / bip85IndexChoices are the picker's in-spec, validated-by-
// construction bounds (R0-I-B): word count = biptool's {12,18,24}; index is a
// bounded small set 0..9 (no free-form numeric entry — there is no reusable
// numeric-entry widget; a larger index space is a FOLLOWUP). The application is
// FIXED to BIP-39 (the only engrave-as-words-faithful BIP-85 app).
var bip85WordChoices = []int{12, 18, 24}
var bip85IndexChoices = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

// bip85ParamPickFlow picks the child BIP-39 word count then the bounded index.
// Returns ok==false on Back from the FIRST screen; Back from the index screen
// re-shows the word-count screen. The returned (words,index) are always in-spec.
func bip85ParamPickFlow(ctx *Context, th *Colors) (words, index int, ok bool) {
	wordCS := &ChoiceScreen{
		Title:   "Child Seed",
		Lead:    "Child word count",
		Choices: []string{"12 WORDS", "18 WORDS", "24 WORDS"},
	}
	for {
		wsel, wok := wordCS.Choose(ctx, th)
		if !wok {
			return 0, 0, false
		}
		idxCS := &ChoiceScreen{
			Title:   "Child Seed",
			Lead:    "Child index",
			Choices: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
		}
		isel, iok := idxCS.Choose(ctx, th)
		if !iok {
			continue // Back from index -> re-pick the word count.
		}
		return bip85WordChoices[wsel], bip85IndexChoices[isel], true
	}
}

// bip85IndexEntryFlow lets the operator TYPE the child index on a cleartext
// keyboard (the index is public, not a secret). It clones typeAddressFlow
// (gui/verify_address.go:44-71): Back (Button1) -> (0,false); OK (Button3) parses
// kbd.Fragment via parseBip85Index. On a parse error it shows the message and
// RE-PROMPTS (clears the keyboard, re-loops) — it NEVER returns a silent 0 and
// NEVER aborts. Only a valid index in [0,2^31-1] returns (idx,true).
func bip85IndexEntryFlow(ctx *Context, th *Colors) (int, bool) {
	kbd := NewAddressKeyboard(ctx)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return 0, false
		}
		if okBtn.Clicked(ctx) {
			idx, err := parseBip85Index(kbd.Fragment)
			if err != nil {
				showError(ctx, th, "Child index", "Enter a whole number 0 to 2147483647.")
				// R0-m1: keep the readout CLEARTEXT on re-prompt (the index is
				// public). kbd.Clear() resets revealed=false, so re-create the
				// cleartext keyboard rather than just clearing it.
				kbd = NewAddressKeyboard(ctx)
				continue
			}
			return idx, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Child index")
		ctx.Frame(op.Layer(kbdOp, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return 0, false
}

// childSeedWarning shows the MANDATORY, operator-acknowledged warning that the
// flow is about to engrave a CHILD SEED — anyone with the child mnemonic controls
// the child wallet, so engrave onto YOUR OWN steel only. Hold to confirm; Back
// cancels. Returns true only on an acknowledged confirm. Mirrors stubZeroWarning.
func childSeedWarning(ctx *Context, th *Colors) bool {
	warn := &ConfirmWarningScreen{
		Title: "Child Seed",
		Body: "This engraves a NEW CHILD SEED derived from your master. Anyone holding " +
			"these words controls the child wallet — engrave onto your OWN steel only.\n\n" +
			"Hold button to confirm.",
		Icon: assets.IconHammer,
	}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, res := warn.Layout(ctx, th, dims)
		switch res {
		case ConfirmNo:
			return false
		case ConfirmYes:
			return true
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// bip85SeedHook is a test-only seam to observe the master + child mnemonics (to
// assert both are scrubbed on exit, I-3). nil in production. Mirrors
// singleSigSeedHook.
var bip85SeedHook func(master, child bip39.Mnemonic)

// bip85DeriveFlow is the bip85Derive program: a hand-typed BIP-39 MASTER seed
// (SECRET, typed-only — NEVER a scan) + optional passphrase ON THE MASTER -> pick
// the child params (app fixed BIP-39, word count {12,18,24}, bounded index 0..9)
// -> derive the child BIP-39 mnemonic via BIP-85 -> unskippable child-seed warning
// -> engrave the child (words + standard SeedQR) via the engraveSeed primitive,
// stamping the CHILD's own bare fingerprint.
//
// SECURITY SPINE (mirror gui/singlesig.go):
//   - TYPED-ONLY master (I-3): from seedEntryFlow ONLY; never an NFC scan.
//   - TWO secrets scrubbed (I-3): the master AND the derived child mnemonic, both
//     []Word zeroed on EVERY exit (derive/abort/warning-abort/engrave-abort/error).
//     The privkey serialization + HMAC output are wiped inside deriveBip85Child.
//   - Mainnet-only; child engraved onto owner-held steel only, never NFC.
func bip85DeriveFlow(ctx *Context, th *Colors) {
	// TYPED-ONLY master (never a scan).
	master, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	var child bip39.Mnemonic
	// Scrub BOTH secrets on EVERY exit path (I-3). child is nil until derived.
	// This is the ONLY scrub defer and (being registered first) runs LAST/LIFO,
	// so it zeroes both backing arrays after every other defer. The test's
	// bip85SeedHook (called synchronously below, after child = c) holds the slice
	// headers and reads their contents AFTER the flow returns and this defer ran.
	defer func() {
		for i := range master {
			master[i] = 0
		}
		for i := range child {
			child[i] = 0
		}
	}()

	// Optional passphrase ON THE MASTER.
	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	for {
		words, index, ok := bip85ParamPickFlow(ctx, th)
		if !ok {
			return
		}
		c, err := deriveBip85Child(master, passphrase, words, index)
		if err != nil {
			showError(ctx, th, "BIP-85 Child", "Couldn't derive the child seed.")
			continue
		}
		child = c
		// Test-only seam: observe BOTH mnemonics synchronously while they are
		// non-nil. nil in production. The captured slice headers alias the backing
		// arrays the top-level scrub defer zeroes on exit (mirrors
		// singleSigSeedHook, gui/singlesig.go:36-38 — observed-then-scrubbed).
		if bip85SeedHook != nil {
			bip85SeedHook(master, child)
		}

		// Unskippable child-seed warning before any engrave.
		if !childSeedWarning(ctx, th) {
			// Abort: scrub this child immediately and re-pick params.
			for i := range child {
				child[i] = 0
			}
			child = nil
			continue
		}

		plate, _, err := engraveBip85Child(ctx.Platform.EngraverParams(), child)
		if err != nil {
			showError(ctx, th, "BIP-85 Child", "Couldn't build the child seed plate.")
			for i := range child {
				child[i] = 0
			}
			child = nil
			continue
		}
		if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
			return
		}
		// Engrave backed out -> re-pick params (scrub this child first).
		for i := range child {
			child[i] = 0
		}
		child = nil
	}
}
