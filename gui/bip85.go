package gui

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bip85"
	"seedhammer.com/engrave"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// validBip85Words is the set of child word counts the BIP-39 application
// supports (biptool's guard n<12||24<n||n%3!=0 -> exactly {12,18,24}).
func validBip85Words(n int) bool {
	return n == 12 || n == 18 || n == 24
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
