package gui

import (
	"errors"
	"fmt"
	"image"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/address"
	"seedhammer.com/bip32"
	"seedhammer.com/bip380"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
)

// ─── T6a-2: watch-only restore doc (direct descriptor; sh-wpkh-safe) ──────────
//
// The restore doc is the PUBLIC artifact the operator keeps to re-import the
// wallet watch-only: the master fingerprint, the concrete output descriptor, and
// the first receive + change addresses. It carries NO secret.
//
// R0-I1 (Option Y): the restore doc builds the descriptor DIRECTLY from the
// engraved xpub + chosen script (rather than re-encoding to md1 and decoding
// back). This keeps the restore-doc path independent of the verify-projection
// path and threads the REAL ParentFingerprint for a canonical xpub. (The md1
// verify path DOES now render sh(wpkh) → P2SH_P2WPKH via the InnerWpkh
// discriminant; both paths agree on the BIP-49 descriptor.)
//
// The Key carries the REAL ParentFingerprint (R0-I1): Key.String() re-serializes
// the xpub from (KeyData, ChainCode, ParentFingerprint, depth/childnum); a
// dropped parentFP would emit a non-canonical xpub that does NOT byte-match the
// engraved mk1 — a silent restore-doc defect an address-match alone would hide.
// Children are set EXPLICITLY to <0;1>/* (R0-m4), matching useSiteToChildren.

var errRestoreBadScript = errors.New("singlesig: unknown script for the restore doc")

// scriptKindToBip380 maps the chosen md.ScriptKind to the bip380.Script for the
// restore descriptor: 44→P2PKH, 49→P2SH_P2WPKH, 84→P2WPKH, 86→P2TR.
func scriptKindToBip380(script md.ScriptKind) (bip380.Script, error) {
	switch script {
	case md.ScriptPkh:
		return bip380.P2PKH, nil
	case md.ScriptShWpkh:
		return bip380.P2SH_P2WPKH, nil
	case md.ScriptWpkh:
		return bip380.P2WPKH, nil
	case md.ScriptTr:
		return bip380.P2TR, nil
	default:
		return 0, errRestoreBadScript
	}
}

// singleSigRestoreDescriptor builds the watch-only *bip380.Descriptor directly
// from the engraved account xpub + the chosen script + path. masterFP feeds the
// [fp/origin] prefix; parentFP (the REAL non-zero parent fp from the xpub
// decode, R0-I1) makes the serialized xpub canonical. Display-only, no secret.
func singleSigRestoreDescriptor(xpub string, masterFP, parentFP uint32, script md.ScriptKind, path bip32.Path) (*bip380.Descriptor, error) {
	bscript, err := scriptKindToBip380(script)
	if err != nil {
		return nil, err
	}
	chainCode, compressedPubkey, decodedParentFP, err := decodeXpubBytes(xpub)
	if err != nil {
		return nil, err
	}
	// Prefer the supplied parentFP (threaded from the derive); fall back to the
	// decode (they are the same — the decode is the source of the supplied one).
	if parentFP == 0 {
		parentFP = decodedParentFP
	}
	desc := &bip380.Descriptor{
		Script: bscript,
		Type:   bip380.Singlesig,
		Keys: []bip380.Key{{
			Network:           &chaincfg.MainNetParams, // mainnet-only (D1).
			MasterFingerprint: masterFP,
			ParentFingerprint: parentFP,
			DerivationPath:    path,
			Children: []bip380.Derivation{
				{Type: bip380.RangeDerivation, Index: 0, End: 1}, // <0;1>
				{Type: bip380.WildcardDerivation},                // /*
			},
			KeyData:   append([]byte(nil), compressedPubkey[:]...), // 33B compressed pubkey.
			ChainCode: append([]byte(nil), chainCode[:]...),        // 32B chain code.
		}},
	}
	return desc, nil
}

// singleSigRestoreLines builds the plain-screen display lines for the restore
// doc: the master fingerprint, the descriptor, and the first receive + change
// addresses. Public-only.
func singleSigRestoreLines(masterFP uint32, desc *bip380.Descriptor) ([]string, error) {
	recv0, err := address.Receive(desc, 0)
	if err != nil {
		return nil, err
	}
	change0, err := address.Change(desc, 0)
	if err != nil {
		return nil, err
	}
	lines := []string{
		fmt.Sprintf("Master fp: %08x", masterFP),
		"Descriptor:",
	}
	lines = append(lines, chunkString(desc.Encode(), 20)...)
	lines = append(lines, "First receive:", recv0, "First change:", change0)
	return lines, nil
}

// restoreDocFlow displays the watch-only restore doc on a plain, paged screen
// (NOT DescriptorScreen — the 0-alloc gate, recon Topic-8). It is display-only;
// no secret material, no engrave. parentFP (R0-I1) is threaded so the displayed
// descriptor's xpub is canonical (byte-matches the engraved mk1).
func restoreDocFlow(ctx *Context, th *Colors, xpub string, masterFP, parentFP uint32, script md.ScriptKind, path bip32.Path) {
	desc, err := singleSigRestoreDescriptor(xpub, masterFP, parentFP, script, path)
	if err != nil {
		showError(ctx, th, "Restore Doc", "Couldn't build the watch-only descriptor.")
		return
	}
	lines, err := singleSigRestoreLines(masterFP, desc)
	if err != nil {
		showError(ctx, th, "Restore Doc", "Couldn't derive the restore addresses.")
		return
	}
	restoreDocScreen(ctx, th, lines)
}

// restoreDocScreen is a plain, paged, read-only display of the restore lines
// (Back/Page/Done). It mirrors xpubVerifyFlow's gap-free paging so the long
// descriptor tail is always reachable, but routes through plain widget labels —
// never DescriptorScreen (the alloc gate).
func restoreDocScreen(ctx *Context, th *Colors, lines []string) {
	backBtn := &Clickable{Button: Button1}
	doneBtn := &Clickable{Button: Button3, AltButton: Center}
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
		if backBtn.Clicked(ctx) || doneBtn.Clicked(ctx) {
			return
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Restore Doc")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight},
			{Clickable: doneBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
