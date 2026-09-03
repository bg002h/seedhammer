package gui

import (
	"encoding/hex"
	"fmt"
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/sysw"
)

// Hashlock entry (SPEC §6c, C25).
//
// THE PAYLOAD IS THE PRIMARY SOURCE and typing is the fallback, because a
// 64-character hex digest typed on a four-button device is a transcription
// with no checksum behind it. A hash: record was checked on the host.
//
// THE 32-BYTE RULE IS STATED AT ENTRY AND AGAIN AT CONSENT (§8i). sha256(H)
// compiles to OP_SIZE <32> OP_EQUALVERIFY OP_SHA256 <H> OP_EQUAL, so the
// preimage MUST be exactly 32 bytes: a digest of a passphrase directly can
// never be spent, and the reference wallet's own README records months of
// exactly that.
//
// THE COMPOSER NEVER DERIVES, STORES OR ENGRAVES A PREIMAGE this cycle
// (§14). It takes a digest and puts it in a script.

// composerHexKeys is the fallback pad's alphabet: hex digits only, so an
// entry that is 64 characters long is 64 VALID characters by construction.
const composerHexKeys = "0123456789\nabcdef"

// composerHashRow is §6c's row form: `hash <i>  <first 8>..<last 8>`, in the
// host's pack order. A full 64-hex row would be CUT rather than wrapped at
// the 436 px label budget, and a cut digest is worse than an elided one --
// the operator cannot tell which end is missing.
func composerHashRow(i int, digest [32]byte) string {
	h := hex.EncodeToString(digest[:])
	return fmt.Sprintf("hash %d  %s..%s", i, h[:8], h[56:])
}

// composerPayloadDigests returns every well-formed hash: record, in payload
// order. A malformed one is ClassUnknown and INERT under the shipped contract
// (sysw/descriptor.go:46-48): it reaches no screen, and its only device-side
// signal is the door's not-understood count (§6a).
func composerPayloadDigests(s *syswSession) [][32]byte {
	if s == nil || !s.loaded {
		return nil
	}
	var out [][32]byte
	for _, r := range s.records {
		if r.class != sysw.ClassHash {
			continue
		}
		d, err := sysw.ParseHashRecord(r.body)
		if err != nil {
			// Unreachable: a record that does not parse is not ClassHash. The
			// arm exists so no value is consumed from a call that errored.
			continue
		}
		out = append(out, d)
	}
	return out
}

// composerHexEntry is the fallback: 64 hex characters, accepted only when
// exactly 64 are present (§6c).
func composerHexEntry(ctx *Context, th *Colors) ([32]byte, bool) {
	var out [32]byte
	kbd := NewKeyboard(ctx, composerHexKeys)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if len(kbd.Fragment) > 64 {
			kbd.Fragment = kbd.Fragment[:64]
		}
		frag := kbd.Fragment
		valid := len(frag) == 64
		if backBtn.Clicked(ctx) {
			return out, false
		}
		clicked := okBtn.Clicked(ctx)
		if valid && clicked {
			raw, err := hex.DecodeString(frag)
			if err != nil || len(raw) != 32 {
				// The pad offers hex alone, so this is unreachable; it refuses
				// rather than returning a zero digest, because a silently zero
				// hashlock is spendable by anyone who knows the preimage of
				// zero.
				showError(ctx, th, "Hash lock", "That is not a 32-byte digest.")
				continue
			}
			copy(out[:], raw)
			return out, true
		}

		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))

		shown := frag
		if shown == "" {
			shown = " "
		}
		word, frgSize := widget.Labelw(&ctx.B, ctx.Styles.word, dims.X-50, th.Background, shown)
		r := image.Rectangle{Max: frgSize}
		r.Min.Y -= 3
		r.Max.Y += buttonPadY
		r.Min.X -= buttonPadX
		r.Max.X += buttonPadX
		top, _ := content.CutBottom(kbdsz.Y)
		wordOff := top.Center(frgSize)
		word = op.Layer(word, op.Compose(
			op.Color(&ctx.B, th.Text),
			op.RoundedRect2(&ctx.B, r, cornerRadius),
		)).Offset(wordOff)

		count, csz := widget.Label(&ctx.B, ctx.Styles.body, th.Text,
			fmt.Sprintf("%d of 64 hex", len(frag)))
		countOp := count.Offset(image.Pt((dims.X-csz.X)/2, wordOff.Y+frgSize.Y+8))

		navBtns := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if valid {
			navBtns = append(navBtns, NavButton{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Hash lock")
		ctx.Frame(op.Layer(kbdOp, word, countOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return out, false
}

// composerHashEdit sets or clears one path's hashlock.
func composerHashEdit(ctx *Context, th *Colors, st *composerState, idx int) bool {
	title := fmt.Sprintf("Path %d hash", idx+1)
	digests := composerPayloadDigests(ctx.sysw)
	rows := make([]string, 0, len(digests)+2)
	for i, d := range digests {
		rows = append(rows, composerHashRow(i+1, d))
	}
	rows = append(rows, "Type 64 hex")
	rows = append(rows, "No hash lock")
	sel, ok := composerPickScreen(ctx, th, title, "Which hash?", rows)
	if !ok {
		return false
	}
	// §8i, ONCE THE OPERATOR IS ACTUALLY TAKING A HASH. It used to be shown
	// unconditionally on entry, so an operator whose next choice was "No hash
	// lock" met a modal in front of a clear. It governs how the preimage must
	// have been produced, which is only a question for someone choosing one.
	if sel <= len(digests) {
		showError(ctx, th, title, composerCopyHashRule())
	}
	switch {
	case sel < len(digests):
		d := digests[sel]
		st.list.Paths[idx].Hash = &d
		return true
	case sel == len(digests):
		d, ok := composerHexEntry(ctx, th)
		if !ok {
			return false
		}
		st.list.Paths[idx].Hash = &d
		return true
	default:
		st.list.Paths[idx].Hash = nil
		return true
	}
}
