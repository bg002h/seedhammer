package gui

import (
	"fmt"
	"image"
	"strings"

	"seedhammer.com/codex32"
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

// confirmCodex32Flow shows a pre-engrave review of a (New-valid) codex32 share
// and returns true to engrave, false to go back. It branches on the RAW share
// index from ParsePrefix (NOT Split(), which remaps an unshared secret's
// threshold 0→1, mislabeling it). The codex32 string is engraved verbatim;
// multi-share recovery is a separate cycle.
func confirmCodex32Flow(ctx *Context, th *Colors, scan codex32.String) bool {
	f, _ := codex32.ParsePrefix(scan.String()) // scan is New-valid → no error
	lines := []string{"id " + strings.ToUpper(f.Identifier)}
	if f.Unshared {
		lines = append(lines, "Unshared secret (S)")
	} else {
		lines = append(lines,
			"Share "+strings.ToUpper(string(f.ShareIndex))+" of a k-of-n set",
			"engraves THIS share, not a recovered seed",
		)
	}
	lines = append(lines, fmt.Sprintf("%d chars", len(scan.String())))

	backBtn := &Clickable{Button: Button1}
	engraveBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if engraveBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: engraveBtn, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm Codex32 Share")

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
		frameOps := append([]op.Op{nav, title}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
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
