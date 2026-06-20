package gui

import (
	"fmt"
	"image"
	"strings"

	"seedhammer.com/backup"
	"seedhammer.com/codex32"
	"seedhammer.com/font/constant"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// codex32StatusLine returns the window-aware length readout for an in-progress
// codex32 fragment of length n. There is no single target: BIP-93 short totals
// are 48..93, the firmware long window is 125..127, and 94..124 is a dead zone
// that is not (yet) an error.
func codex32StatusLine(n int) string {
	switch {
	case n < codex32.ShortCodeMinLength:
		return fmt.Sprintf("%d chars", n)
	case n <= codex32.ShortCodeMaxLength:
		return fmt.Sprintf("short · %d chars", n)
	case n < codex32.LongCodeMinLength:
		return fmt.Sprintf("%d chars — keep typing", n)
	case n <= codex32.LongCodeMaxLength:
		return fmt.Sprintf("long · %d chars", n)
	default:
		return "too long"
	}
}

// codex32FieldLine renders the parsed header fields as "id NAME · thr 2 · share C",
// each segment appearing once its field is known. Returns "" if nothing is known.
func codex32FieldLine(f codex32.Fields) string {
	var segs []string
	if f.IdentifierKnown {
		segs = append(segs, "id "+strings.ToUpper(f.Identifier))
	}
	if f.ThresholdKnown {
		segs = append(segs, fmt.Sprintf("thr %d", f.Threshold))
	}
	if f.ShareIndexKnown {
		segs = append(segs, "share "+strings.ToUpper(string(f.ShareIndex)))
	}
	return strings.Join(segs, " · ")
}

// codex32Feedback returns an error label to show under the entry, or "" if the
// fragment is fine so far. Field errors (from ParsePrefix) show eagerly; a
// checksum/structure error from New shows only once the fragment reaches a valid
// length window (so a half-typed string isn't flagged "wrong length").
func codex32Feedback(frag string, perr, nerr error) string {
	if perr != nil {
		return codex32.Describe(perr)
	}
	n := len(frag)
	inWindow := (n >= codex32.ShortCodeMinLength && n <= codex32.ShortCodeMaxLength) ||
		(n >= codex32.LongCodeMinLength && n <= codex32.LongCodeMaxLength)
	if inWindow && nerr != nil {
		return codex32.Describe(nerr)
	}
	return ""
}

// codex32ConfirmAction is the result of the pre-engrave codex32 confirm screen.
type codex32ConfirmAction int

const (
	codex32Back    codex32ConfirmAction = iota // Button1
	codex32Engrave                             // Button3
	codex32Recover                             // Button2 — only offered for a share (index != S)
)

// confirmCodex32Flow shows a pre-engrave review of a (New-valid) codex32 string.
// For an unshared secret it offers Back/Engrave; for a share (index != S) it also
// offers Recover (reconstruct the secret from k shares). It branches display on
// the RAW ParsePrefix fields (NOT Split(), which remaps an unshared secret's
// threshold 0→1). The codex32 string is engraved verbatim.
func confirmCodex32Flow(ctx *Context, th *Colors, scan codex32.String) codex32ConfirmAction {
	f, _ := codex32.ParsePrefix(scan.String()) // scan is New-valid → no error
	title := "Confirm Codex32 Share"
	lines := []string{"id " + strings.ToUpper(f.Identifier)}
	if f.Unshared {
		title = "Confirm Codex32 Secret"
		lines = append(lines, "Unshared secret (S)")
	} else {
		lines = append(lines,
			"Share "+strings.ToUpper(string(f.ShareIndex))+" of a k-of-n set",
			"Engrave this share, or Recover the secret",
		)
	}
	lines = append(lines, fmt.Sprintf("%d chars", len(scan.String())))

	backBtn := &Clickable{Button: Button1}
	recoverBtn := &Clickable{Button: Button2}
	engraveBtn := &Clickable{Button: Button3, AltButton: Center}
	// "Show secret" is offered only for an unshared secret that actually decodes
	// as an m-format ms1 (a plain BIP-93 secret is not decodable). Probe once.
	_, _, ent, msErr := codex32.DecodeMS1(scan)
	wipeBytes(ent) // L1: scrub the discarded probe entropy (nil on err -> no-op)
	showSecret := f.Unshared && msErr == nil
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return codex32Back
		}
		// Always drain Button2 — even for an unshared secret, where Recover is not
		// offered — so an unconsumed event cannot block the router queue head in a
		// direct-call (non-runUI) context. Act on it only for a share. (R0 C1)
		recoverClicked := recoverBtn.Clicked(ctx) // always drained (queue-head idiom)
		switch {
		case showSecret && recoverClicked:
			ms1DecodeFlow(ctx, th, scan) // display-only "Show secret" sub-flow
			continue
		case !f.Unshared && recoverClicked:
			return codex32Recover
		}
		if engraveBtn.Clicked(ctx) {
			return codex32Engrave
		}
		dims := ctx.Platform.DisplaySize()
		navBtns := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}
		switch {
		case showSecret:
			navBtns = append(navBtns, NavButton{Clickable: recoverBtn, Style: StyleSecondary, Icon: assets.IconInfo}) // Show secret
		case !f.Unshared:
			navBtns = append(navBtns, NavButton{Clickable: recoverBtn, Style: StyleSecondary, Icon: assets.IconRight}) // Recover
		}
		navBtns = append(navBtns, NavButton{Clickable: engraveBtn, Style: StylePrimary, Icon: assets.IconHammer})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)

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
	return codex32Back
}

// showCodex32Error displays a dismissible error modal (Button3 dismisses) over a
// blank background; returns when dismissed or ctx.Done.
func showCodex32Error(ctx *Context, th *Colors, msg string) {
	errScr := &ErrorScreen{Title: "Invalid share", Body: msg}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, dismissed := errScr.Layout(ctx, th, dims)
		if dismissed {
			return
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
}

// recoverCodex32Flow collects shares 2..k (k = the first share's threshold),
// validating each against the set as it is added, then reconstructs the unshared
// secret via Interpolate(shares,'S'). Returns (secret, true) on success, or
// (_, false) if the user backs out or recovery fails.
func recoverCodex32Flow(ctx *Context, th *Colors, first codex32.String) (codex32.String, bool) {
	f, _ := codex32.ParsePrefix(first.String())
	if !f.ThresholdKnown || f.Threshold < 2 { // unreachable for a New-valid share; defensive
		return codex32.String{}, false
	}
	k := f.Threshold
	id := strings.ToUpper(f.Identifier)
	shares := []codex32.String{first}
	for len(shares) < k {
		title := fmt.Sprintf("Share %d of %d · id %s", len(shares)+1, k, id)
		obj, ok := inputCodex32Flow(ctx, th, title)
		if !ok {
			return codex32.String{}, false // Back exits recovery
		}
		cand, isCodex32 := obj.(codex32.String)
		if !isCodex32 {
			showCodex32Error(ctx, th, "enter a codex32 share (ms1…)")
			continue
		}
		pf, _ := codex32.ParsePrefix(cand.String())
		if pf.Unshared {
			showCodex32Error(ctx, th, "enter a share, not the secret")
			continue
		}
		if err := codex32.ConsistentShares(append(shares, cand)); err != nil {
			showCodex32Error(ctx, th, codex32.Describe(err))
			continue
		}
		shares = append(shares, cand)
	}
	secret, err := codex32.Interpolate(shares, 'S')
	if err != nil { // defense-in-depth; should not happen after ConsistentShares + exactly k
		showCodex32Error(ctx, th, codex32.Describe(err))
		return codex32.String{}, false
	}
	return secret, true
}

// engraveCodex32 confirms a codex32 string and engraves it verbatim. A share may
// instead be recovered into the unshared secret, which is then re-confirmed and
// engraved. Returns true (recognized/handled) in all terminal cases — Back is a
// deliberate decline, NOT "Unknown format".
func engraveCodex32(ctx *Context, th *Colors, scan codex32.String) bool {
	for {
		switch confirmCodex32Flow(ctx, th, scan) {
		case codex32Back:
			return true
		case codex32Recover:
			secret, ok := recoverCodex32Flow(ctx, th, scan)
			if !ok {
				continue // back to the original share's confirm
			}
			scan = secret // recovered unshared secret; loop re-confirms it (no Recover offered for S)
			continue
		case codex32Engrave:
			id, _, _ := scan.Split()
			s := backup.SeedString{Title: id, Seed: scan.String(), Font: constant.Font}
			backupSeedStringFlow(ctx, th, s)
			return true
		}
	}
}

// codex32Keys is the on-screen codex32 keypad: digit row + the BIP-39
// full-QWERTY letter rows. b/i/o are present (for visual familiarity) but
// statically dimmed by newCodex32Keyboard, since bech32 excludes them.
const codex32Keys = "1234567890\nqwertyuiop\nasdfghjkl\nzxcvbnm"

// newCodex32Keyboard builds the codex32 keypad and statically disables the
// never-valid b/i/o keys. Per-instance, so the BIP-39 keyboard is unaffected.
// Disabling via allKeys also disables the same elements in the per-row keys
// slices (shared backing array); Clear() does not reset disabled.
func newCodex32Keyboard(ctx *Context) *Keyboard {
	kbd := NewKeyboard(ctx, codex32Keys)
	for i := range kbd.allKeys {
		switch kbd.allKeys[i].r {
		case 'b', 'i', 'o':
			kbd.allKeys[i].disabled = true
		}
	}
	return kbd
}

// validateMStar runs the per-HRP completeness/validity check for an m*1 fragment
// and returns the typed value the engrave dispatch routes: a codex32.String for
// ms (via New), or an mdmkText for md/mk (via ValidMD/ValidMK). The third return
// is New's error for ms feedback (nil for md/mk). (Phase B; SPEC §4.1(a).)
func validateMStar(frag string, f codex32.Fields) (obj any, valid bool, msErr error) {
	switch {
	case strings.EqualFold(f.HRP, "ms"):
		s, err := codex32.New(frag)
		if err == nil {
			return s, true, nil
		}
		return nil, false, err
	case strings.EqualFold(f.HRP, "md"):
		if codex32.ValidMD(frag) {
			return mdmkText(frag), true, nil
		}
	case strings.EqualFold(f.HRP, "mk"):
		if codex32.ValidMK(frag) {
			return mdmkText(frag), true, nil
		}
	}
	return nil, false, nil
}

// mstarStatusLine is the HRP-aware length readout. ms reuses codex32StatusLine
// (total windows); md/mk report a data-part window state. (SPEC §4.1(b).)
func mstarStatusLine(frag string, f codex32.Fields) string {
	switch {
	case strings.EqualFold(f.HRP, "md"), strings.EqualFold(f.HRP, "mk"):
		if codex32.MStarInWindow(frag) {
			return fmt.Sprintf("%s · %d chars", strings.ToLower(f.HRP), len(frag))
		}
		return fmt.Sprintf("%d chars", len(frag))
	default: // ms or pre-separator
		return codex32StatusLine(len(frag))
	}
}

// mstarFeedback is the HRP-aware advisory error label. md/mk SUPPRESS the
// codex32-share-schema ParsePrefix errors (which fire spuriously on md/mk data,
// e.g. "bad threshold") and show only a generic "bad checksum" once in the
// per-HRP length window; ms delegates to codex32Feedback. (SPEC §4.1(c).)
func mstarFeedback(frag string, f codex32.Fields, perr, msErr error, valid bool) string {
	if valid || frag == "" {
		return ""
	}
	if strings.EqualFold(f.HRP, "md") || strings.EqualFold(f.HRP, "mk") {
		if codex32.MStarInWindow(frag) {
			return "bad checksum"
		}
		return ""
	}
	return codex32Feedback(frag, perr, msErr) // ms (or pre-separator) path unchanged
}

// confirmCorrectionFlow shows the proposed correction's per-position diff and
// asks the user to confirm it against their source card BEFORE the corrected
// string is accepted. The per-position diff is the UNIVERSAL anchor for all three
// m*1 (SPEC §2.3); for ms ONLY it also shows the decoded id·thr·share header line
// (the codex32 share schema does not exist for md/mk). Button1 rejects, Button3
// accepts; Button2 is drained every frame so it cannot block the queue head.
func confirmCorrectionFlow(ctx *Context, th *Colors, res codex32.CorrectionResult, hrp string) bool {
	lines := make([]string, 0, len(res.Edits)+2)
	for _, e := range res.Edits {
		// e.Pos is a full-string rune index (HRP + the '1' separator included);
		// +1 makes it 1-based for the human comparing against their source card.
		lines = append(lines, fmt.Sprintf("pos %d: %c → %c", e.Pos+1, rune(e.Was), rune(e.Now)))
	}
	if hrp == "ms" {
		if f, err := codex32.ParsePrefix(res.Corrected); err == nil {
			if fl := codex32FieldLine(f); fl != "" {
				lines = append(lines, fl)
			}
		}
	}
	lines = append(lines, "Compare each change to your source card.")

	backBtn := &Clickable{Button: Button1}
	drainBtn := &Clickable{Button: Button2}
	acceptBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		drainBtn.Clicked(ctx) // drain Button2 (R0-C1 idiom)
		if acceptBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		nav, _ := layoutNavigation(&ctx.B, th, dims,
			NavButton{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Clickable: acceptBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Apply this correction?")

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
