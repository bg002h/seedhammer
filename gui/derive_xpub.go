package gui

import (
	"fmt"
	"image"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/mk"
)

// scriptTypeChoices is the stage-1 list of the six standard script types the
// path picker offers. Order is load-bearing (the resolver indexes by it).
func scriptTypeChoices() []string {
	return []string{
		"BIP-44 legacy",
		"BIP-49 nested-segwit",
		"BIP-84 native-segwit",
		"BIP-86 taproot",
		"BIP-48 multisig",
		"BIP-87 multisig",
	}
}

// scriptTypePurpose maps each stage-1 choice index to its BIP-43 purpose and a
// flag for whether the path has a BIP-48 script-type suffix (.../2' for P2WSH).
var scriptTypePurpose = []struct {
	purpose uint32
	bip48   bool // append the script-type suffix 2' (P2WSH multisig)
}{
	{44, false},
	{49, false},
	{84, false},
	{86, false},
	{48, true},
	{87, false},
}

// pathPickerFlow is the two-stage standard-path picker (R0-I4): stage 1 picks
// one of the six script types, stage 2 picks the network. It resolves to one of
// the 14 standard paths. The BIP-48 entry maps to .../<coin>'/0'/2' (P2WSH).
// Returns (path, network params, network name, ok). ok==false on Back.
func pathPickerFlow(ctx *Context, th *Colors) (bip32.Path, *chaincfg.Params, string, bool) {
	for {
		stage1 := &ChoiceScreen{Title: "Script type", Lead: "Choose address type", Choices: scriptTypeChoices()}
		sIdx, ok := stage1.Choose(ctx, th)
		if !ok {
			return nil, nil, "", false
		}
		stage2 := &ChoiceScreen{Title: "Network", Lead: "Choose network", Choices: []string{"Mainnet", "Testnet"}}
		nIdx, ok := stage2.Choose(ctx, th)
		if !ok {
			// Back from network -> re-pick the script type.
			continue
		}
		sp := scriptTypePurpose[sIdx]
		var coin uint32
		var net *chaincfg.Params
		var netName string
		if nIdx == 0 {
			coin, net, netName = 0, &chaincfg.MainNetParams, "mainnet"
		} else {
			coin, net, netName = 1, &chaincfg.TestNet3Params, "testnet"
		}
		const hardened = 0x80000000
		path := bip32.Path{sp.purpose | hardened, coin | hardened, 0 | hardened}
		if sp.bip48 {
			path = append(path, 2|hardened) // P2WSH multisig script type
		}
		return path, net, netName, true
	}
}

// seedEntryFlow reuses the typed BIP-39 word entry (12 or 24 words) and returns
// the SECRET mnemonic. Returns ok==false on Back. The caller MUST scrub the
// returned mnemonic when done.
func seedEntryFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	cs := &ChoiceScreen{Title: "Input Seed", Lead: "Choose number of words", Choices: []string{"12 WORDS", "24 WORDS"}}
	for {
		choice, ok := cs.Choose(ctx, th)
		if !ok {
			return nil, false
		}
		mnemonic := emptyBIP39Mnemonic([]int{12, 24}[choice])
		inputWordsFlow(ctx, th, mnemonic, 0, "")
		if !isEmptyMnemonic(mnemonic) {
			return mnemonic, true
		}
		// Back out of word entry without finishing -> re-show the count picker.
	}
}

// deriveXpubFlow is the engraveXpub program: a hand-typed BIP-39 seed (SECRET)
// is turned into a PUBLIC account xpub and engraved as an mk1 key card.
//
// SECURITY SPINE: the seed/mnemonic/passphrase are SECRET — typed-only, never
// emitted over NFC, never engraved. The ONLY engraved output is the public
// account xpub (via .Neuter, inside deriveAccountXpub). This flow NEVER calls
// engraveSeed/backup.EngraveSeed. The mnemonic []Word is zeroed once derivation
// completes.
func deriveXpubFlow(ctx *Context, th *Colors) {
	mnemonic, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	// Scrub the SECRET mnemonic when this flow returns (zero the []Word slice —
	// wipeBytes only applies to []byte).
	defer func() {
		for i := range mnemonic {
			mnemonic[i] = 0
		}
	}()

	// Optional passphrase.
	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	for {
		path, net, netName, ok := pathPickerFlow(ctx, th)
		if !ok {
			return
		}
		xpub, mfp, err := deriveAccountXpub(mnemonic, passphrase, net, path)
		if err != nil {
			showError(ctx, th, "Account Xpub", "Couldn't derive the account key.")
			continue
		}
		card := mk.Card{
			Network:     netName,
			Path:        path.String(),
			Fingerprint: fmt.Sprintf("%08x", mfp),
			Stubs:       [][4]byte{{0, 0, 0, 0}},
			Xpub:        xpub,
		}
		strs, err := mk.Encode(card)
		if err != nil {
			showError(ctx, th, "Account Xpub", "Couldn't encode the key card.")
			continue
		}

		// Read-only verify display, then Continue / Back.
		if !xpubVerifyFlow(ctx, th, card) {
			continue // Back -> re-pick the path
		}

		// Mandatory, operator-acknowledged stub-0 warning (§2.4).
		if !stubZeroWarning(ctx, th) {
			continue // Back -> re-pick the path
		}

		// Multi-plate engrave sequencing with a defined set-level abort.
		multiPlateEngrave(ctx, th, strs)
		return
	}
}

// xpubVerifyFlow shows the decoded account metadata for operator verification
// (read-only). Continue (Button3) proceeds; Back (Button1) returns false. Paged
// gap-free so the long xpub tail is always reachable (mirrors mk1DisplayFlow).
func xpubVerifyFlow(ctx *Context, th *Colors, card mk.Card) bool {
	lines := []string{
		"Network: " + card.Network,
		"Path: " + card.Path,
		"Fingerprint: " + card.Fingerprint,
		"Account xpub:",
	}
	lines = append(lines, chunkString(card.Xpub, 20)...)

	backBtn := &Clickable{Button: Button1}
	contBtn := &Clickable{Button: Button3}
	pageBtn := &Clickable{Button: Button2}
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	content, _ = content.CutBottom(leadingSize)
	contentTop := content.Min.Y + 8
	contentBottom := content.Max.Y
	start := 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if contBtn.Clicked(ctx) {
			return true
		}
		shown := 0
		y := contentTop
		body := make([]op.Op, 0, len(lines))
		for i := start; i < len(lines); i++ {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, lines[i])
			if i > start && y+sz.Y > contentBottom {
				break
			}
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
			shown++
			if y > contentBottom {
				break
			}
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(lines) {
				start += shown
			} else {
				start = 0
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Verify Xpub")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight},
			{Clickable: contBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}

// stubZeroWarning shows the MANDATORY, operator-acknowledged warning that the
// card carries a placeholder policy stub (00000000) and is NOT bound to a wallet
// policy (§2.4). The operator must hold the confirm button to proceed; Back
// cancels. Returns true only on an acknowledged confirm.
func stubZeroWarning(ctx *Context, th *Colors) bool {
	warn := &ConfirmWarningScreen{
		Title: "Unbound Key Card",
		Body: "This card carries a placeholder policy stub (00000000) and is NOT bound to a " +
			"wallet policy.\n\nHold button to confirm.",
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

// multiPlateEngrave sequences the N mk1 chunk strings as N plates ("Plate i of
// N"), engraving each in turn (§2.6). Set-level abort (R0-I3): a partial set
// cannot be reassembled, so backing out mid-sequence shows a clear "incomplete,
// discard and start over" warning rather than silently exiting as if done. No
// completed-backup state is recorded for a partial set.
func multiPlateEngrave(ctx *Context, th *Colors, strs []string) {
	total := len(strs)
	params := ctx.Platform.EngraverParams()
	for i, s := range strs {
		labels, plates, err := validateMdmk(params, s)
		if err != nil || len(plates) == 0 {
			showError(ctx, th, "Account Xpub", "This key card doesn't fit a plate.")
			return
		}
		// Let the operator pick an engraving variant for this plate.
		cs := &ChoiceScreen{
			Title:   fmt.Sprintf("Plate %d of %d", i+1, total),
			Lead:    "Choose engraving",
			Choices: labels,
		}
		engraved := false
		for !engraved {
			idx, ok := cs.Choose(ctx, th)
			if !ok {
				// Abort mid-sequence: a partial set can't be restored.
				abortWarning(ctx, th, i, total)
				return
			}
			if NewEngraveScreen(ctx, plates[idx]).Engrave(ctx, &engraveTheme) {
				engraved = true
			}
			// Engrave returned without completing (Back) -> re-show the variant
			// picker for this same plate.
		}
	}
}

// abortWarning informs the operator that an incomplete chunk set cannot be
// restored and must be discarded; re-entering the flow re-derives identical
// strings deterministically. It is dismiss-only (no completed state recorded).
func abortWarning(ctx *Context, th *Colors, done, total int) {
	showError(ctx, th, "Incomplete Backup",
		fmt.Sprintf("Engraved %d of %d plates. This key card set can't be restored from a "+
			"partial set — discard the partial plate(s) and start over.", done, total))
}
