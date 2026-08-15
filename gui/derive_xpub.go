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
	"seedhammer.com/sysw"
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
// seedEntryFlow offers every source a seed may come from: typed, scanned, or
// the systemwide payload.
//
// **Verify flows must NOT call this** — they call seedEntryFlowTypedOnly. See
// its comment for why the distinction is two functions rather than a parameter.
func seedEntryFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	for {
		m, src, ok := syswSeedPicker(ctx, th)
		if !ok {
			// Back at the SOURCE picker leaves seed entry entirely. It used to
			// fall through to the word-count picker, which was invisible while
			// the picker only appeared for a loaded payload; now that the picker
			// is the first screen of every seed entry, a Back that landed the
			// operator one screen DEEPER would be the wrong sentence on the most
			// common path there is.
			return nil, false
		}
		if src == srcTyped {
			return seedEntryFlowTypedOnly(ctx, th)
		}
		if m != nil {
			return m, true
		}
		// The chosen source produced nothing — Back out of the scan, a declined
		// acceptance, or a payload record that would not parse. Re-offer the
		// sources rather than dropping the operator out of seed entry.
	}
}

// seedEntryFlowTypedOnly offers ONE source: the keyboard.
//
// TWO ENTRY POINTS, NOT A BOOLEAN, and the choice is the mechanism rather than a
// style preference. §7.4 forbids a payload-sourced secret from reaching a
// verification comparison: a verify that accepted the same secret the engrave
// used would compare the engrave source against itself and pass
// unconditionally, certifying a WRONG PLATE as good.
//
// A boolean parameter can be passed wrongly and the wrong value still compiles.
// A verify flow that has no way to NAME the payload source cannot reach it by
// any argument, which makes the test structural — no verify flow mentions
// seedEntryFlow — instead of behavioural.
func seedEntryFlowTypedOnly(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	cs := &ChoiceScreen{Title: "Input Seed", Lead: "Choose number of words", Choices: []string{"12 WORDS", "24 WORDS"}}
	for {
		choice, ok := cs.Choose(ctx, th)
		if !ok {
			return nil, false
		}
		mnemonic := emptyBIP39Mnemonic([]int{12, 24}[choice])
		inputWordsFlow(ctx, th, mnemonic, 0, "", wordEntryOpts{checksumGate: true})
		if !isEmptyMnemonic(mnemonic) {
			return mnemonic, true
		}
		// Back out of word entry without finishing -> re-show the count picker.
	}
}

// syswSeedPicker offers every source §3.1 names that this machine ACTUALLY HAS:
// Typed, Scanned when there is a reader, and the payload when one is loaded and
// holds a seed.
//
//	(nil, srcTyped,  false)  Back: leave seed entry
//	(nil, srcTyped,  true)   the operator wants the keyboard
//	(m,   src,       true)   a seed arrived from src, and was accepted
//	(nil, src,       true)   that source produced nothing; the caller re-offers
//
// §13 D9: A PICKER WITH ONE ROW IS NOT A CHOICE, AND IS NOT DRAWN. Stage 10 made
// this the first screen of every seed entry in four programs — the most-walked
// path in the firmware — where on a machine with neither source it cost a click
// to offer the keyboard the operator was going to get anyway. So the rows are
// built first and the screen is skipped when only one survives, which makes the
// rule one comparison instead of a condition repeated per row.
//
// THE READER IS ASKED FOR VIA Features(), NOT VIA `NFCReader() != nil`. The
// obvious probe is a trap: cmd/emu's reader() HANDS OUT the pending tag and
// marks it consumed, so probing to decide whether to draw a row would eat the
// operator's tag before they had chosen anything. FeatureNFC (gui.go) exists to
// answer the same question for free.
func syswSeedPicker(ctx *Context, th *Colors) (bip39.Mnemonic, syswSource, bool) {
	type seedSource struct {
		label string
		src   syswSource
	}
	// PAYLOAD FIRST when there is one, because ChoiceScreen opens on index 0 and
	// an operator who loaded a payload almost always means to use it (operator
	// ruling 2026-08-12, matching the boot offer's LOAD default). Typing stays
	// one tap away; nothing is forced, only re-ordered.
	var rows []seedSource
	if ctx.sysw != nil && ctx.sysw.has(sysw.ClassMnemonic) {
		rows = append(rows, seedSource{"FROM PAYLOAD", srcPayload})
	}
	rows = append(rows, seedSource{"TYPE IT", srcTyped})
	if ctx.Platform.Features().Has(FeatureNFC) {
		rows = append(rows, seedSource{"SCAN", srcNFC})
	}
	if len(rows) == 1 {
		// Exactly the keyboard, so say so by going there rather than by drawing
		// a menu of one.
		return nil, srcTyped, true
	}
	choices := make([]string, len(rows))
	for i, r := range rows {
		choices[i] = r.label
	}
	cs := &ChoiceScreen{Title: "Input Seed", Lead: "Where from?", Choices: choices}
	choice, ok := cs.Choose(ctx, th)
	if !ok {
		return nil, srcTyped, false
	}
	switch rows[choice].src {
	case srcTyped:
		return nil, srcTyped, true
	case srcNFC:
		m, ok := scanSeedFlow(ctx, th)
		if !ok {
			return nil, srcNFC, true
		}
		if !syswSourceAccept(ctx, th, "Input Seed", sysw.ClassMnemonic, srcNFC) {
			return nil, srcNFC, true
		}
		return m, srcNFC, true
	case srcPayload:
		body, ok := ctx.sysw.take(sysw.ClassMnemonic)
		if !ok {
			return nil, srcPayload, true
		}
		m, err := bip39.ParseMnemonic(body)
		if err != nil {
			return nil, srcPayload, true
		}
		if !syswSourceAccept(ctx, th, "Input Seed", sysw.ClassMnemonic, srcPayload) {
			return nil, srcPayload, true
		}
		return m, srcPayload, true
	}
	return nil, srcTyped, false
}

// scanSeedFlow collects a seed from a tag.
//
// IT ACCEPTS A bip39.Mnemonic AND NOTHING ELSE. The scanner recognises seven
// forms and this seam is seed material only; admitting any of the others because
// "it came off a tag" would hand a program a class its §3.3.2 row never granted
// it, by a door the table cannot see. Source is a FLAG input, never an admission
// input (§3.3.2a).
//
// Returns (nil, false) on Back.
func scanSeedFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	scans, stopScanner := startScanner(ctx, ctx.Platform.NFCReader())
	defer stopScanner()
	backBtn := &Clickable{Button: Button1}
	dims := ctx.Platform.DisplaySize()
	msg := ""
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		select {
		case scan := <-scans:
			if m, ok := scan.Object.(bip39.Mnemonic); ok {
				return m, true
			}
			switch {
			case scan.Object != nil:
				// Recognised, and not a seed. Named distinctly from an
				// unreadable tag: the operator held up the wrong card, and
				// "unrecognized" would send them cleaning the reader.
				msg = "That tag is not a seed."
			case scan.Status == scanUnknownFormat:
				msg = "Unrecognized tag."
			case scan.Status == scanFailed:
				msg = "Scan failed - try again."
			}
		default:
		}
		lines := []string{"Hold the seed tag to the reader."}
		if msg != "" {
			lines = append(lines, msg)
		}
		lineWidth := dims.X - 2*8
		y := leadingSize + 8
		body := make([]op.Op, 0, len(lines))
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Scan Seed")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return nil, false
}

// deriveXpubFlow is the engraveXpub program: a BIP-39 seed (SECRET) is turned
// into a PUBLIC account xpub and engraved as an mk1 key card.
//
// SECURITY SPINE: the seed/mnemonic/passphrase are SECRET — never emitted over
// NFC, never engraved. They are NOT keyboard-only: the seed arrives through
// seedEntryFlow, the source picker (payload / keyboard / scan). The claim that
// they were is what S3 retired on 2026-08-15, in the same sweep that took the
// nine stale seed-entry comments out of gui/ (SPEC §2.2 D-5); this one survived
// that sweep's grep only because it is spelt differently here.
//
// The retired phrase is deliberately not quoted. S3's gate is a grep for it
// returning nothing, so a comment that names it to explain its removal puts it
// straight back and fails the gate. Measured: it did, on the first run.
//
// The ONLY engraved output is the public
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
		// §3.3.2 admits ClassPassphrase to this program, so the payload is
		// offered before the keyboard (plan stage 13b). NOT passphraseFlow: see
		// syswPassphraseFlow for the two normative rules a shared edit inside
		// passphraseFlow would have broken.
		if pass, ok := syswPassphraseFlow(ctx, th); ok {
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
	for i, s := range strs {
		labels, plates, err := validateMdmk(ctx.Platform, s)
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
			"partial set - discard the partial plate(s) and start over.", done, total))
}
