package gui

import (
	"fmt"
	"image"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seedxor"
)

// seedXORPartCount asks how many parts (N-of-N; min 2). 0 = Back.
func seedXORPartCount(ctx *Context, th *Colors) int {
	cs := &ChoiceScreen{Title: "Seed XOR", Lead: "How many parts?", Choices: []string{"2", "3", "4", "5"}}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return 0
	}
	return sel + 2
}

// seedXORPartLength asks the word length of the parts (Coldcard-interop). 0 = Back.
// Mechanically required: inputWordsFlow fills a pre-sized slice, so the length
// must be known before entry; parts 2..N inherit this (no per-part re-pick).
func seedXORPartLength(ctx *Context, th *Colors) int {
	cs := &ChoiceScreen{Title: "Seed XOR", Lead: "Words per part?", Choices: []string{"12", "18", "24"}}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return 0
	}
	return []int{12, 18, 24}[sel]
}

// combineSeedXORFlow collects N parts, XORs them into the recovered seed, and
// gates engrave behind the mandatory fingerprint check. (nil,false) on Back/abort.
func combineSeedXORFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	n := seedXORPartCount(ctx, th)
	if n == 0 {
		return nil, false
	}
	nwords := seedXORPartLength(ctx, th)
	if nwords == 0 {
		return nil, false
	}
	parts := make([]bip39.Mnemonic, 0, n)
	for i := 0; i < n; i++ {
		m := emptyBIP39Mnemonic(nwords)
		inputWordsFlow(ctx, th, m, 0, fmt.Sprintf("Part %d of %d", i+1, n))
		// I1 guard: inputWordsFlow returns a PARTIAL slice on Back. Only a
		// complete, checksum-valid part may be collected — else Entropy()
		// panics in Combine. A partial/invalid part aborts the whole flow.
		if !isMnemonicComplete(m) || !m.Valid() {
			return nil, false
		}
		parts = append(parts, m)
	}
	seed, err := seedxor.Combine(parts)
	if err != nil {
		showError(ctx, th, "Seed XOR", seedxor.Describe(err))
		return nil, false
	}
	mfp, ferr := masterFingerprintFor(seed, &chaincfg.MainNetParams, "")
	if ferr != nil {
		showError(ctx, th, "Seed XOR", "could not derive the fingerprint")
		return nil, false
	}
	if !confirmSeedXORFingerprint(ctx, th, mfp) {
		return nil, false
	}
	return seed, true
}

// confirmSeedXORFingerprint shows the recovered master fingerprint behind a
// mandatory, unskippable gate before engraving. Seed XOR has no authentication
// tag — any wrong part still XORs to some valid-looking wallet — so this gate is
// load-bearing for the whole operation. Clone of confirmSLIP39Fingerprint with
// Seed-XOR-specific wording; Button2 is drained unconditionally (no-hang).
func confirmSeedXORFingerprint(ctx *Context, th *Colors, mfp uint32) bool {
	lines := []string{
		fmt.Sprintf("Fingerprint %.8X", mfp),
		"Seed XOR has no built-in check — any wrong part still makes a valid wallet. Confirm this matches your records before engraving.",
	}
	backBtn := &Clickable{Button: Button1}
	drainBtn := &Clickable{Button: Button2}
	okBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		drainBtn.Clicked(ctx) // drain Button2 (no-hang)
		if okBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Recovered Fingerprint")
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(leadingSize)
		body := make([]op.Op, 0, len(lines))
		y := content.Min.Y + 8
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}
