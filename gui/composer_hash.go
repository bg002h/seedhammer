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
// THE COMPOSER DERIVES A PREIMAGE IN RAM FOR ONE SCREEN (H2) AND NEVER STORES,
// SHOWS OR ENGRAVES IT. It puts a digest in a script.

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

const composerHashRowPhrase = "Type a hashlock phrase"

// composerHashRowSet builds `Which hash?` ONCE and records where each named row
// sits, so the dispatch below is by label, never by index arithmetic (spec §5;
// r2 review C-4: the shipped default arm cleared the lock when a row moved).
//
// (Named composerHashRowSet rather than composerHashRows: the constructor below
// is composerHashRows, and Go does not allow a type and a func to share a name
// in the same package -- the plan's own tests call the constructor composerHashRows.)
type composerHashRowSet struct {
	labels    []string
	lead      string
	digests   [][32]byte
	phraseRow int
	hexRow    int
	noneRow   int
}

func composerHashRows(s *syswSession) composerHashRowSet {
	digests := composerPayloadDigests(s)
	labels := make([]string, 0, len(digests)+3)
	for i, d := range digests {
		labels = append(labels, composerHashRow(i+1, d))
	}
	r := composerHashRowSet{digests: digests, lead: "Which hash?"}
	r.phraseRow = len(labels)
	labels = append(labels, composerHashRowPhrase)
	r.hexRow = len(labels)
	labels = append(labels, "Type 64 hex")
	r.noneRow = len(labels)
	labels = append(labels, "No hash lock")
	r.labels = labels
	if len(digests) == 0 {
		r.lead = composerCopyHashlockNoPayloadLead()
	}
	return r
}

// composerHashByPhraseSync drops st.hashByPhrase once NO path carries a hash at
// all -- the one event after which no phrase-set hash can still be in the
// composition (r0 adversarial I-2 = fidelity M-2 = journey M-1: the flag was set
// and never cleared anywhere).
//
// It is deliberately NOT cleared when THIS path's hash is replaced by a payload
// row or a hex digest: another path may still be phrase-set, and clearing on
// that narrower event would drop §8h's phrase form while a phrase-set hash is
// still live -- the C16 shape (a composition-wide fact edited as though it were
// per-path). The residual staleness runs the SAFE way: an over-sticky flag makes
// composerCopyHashEveryPathPhrase name "the phrase and its method, OR the
// preimage plate", so the operator is told to back up one artifact too many,
// never one too few. Per-path provenance is filed as a follow-up (owning phase
// H3) rather than bolted on here, because it needs the same splicing discipline
// composerAddPath and "Remove path" already apply to Paths.
func composerHashByPhraseSync(st *composerState) {
	for _, p := range st.list.Paths {
		if p.Hash != nil {
			return
		}
	}
	st.hashByPhrase = false
}

// composerHashEdit sets or clears one path's hashlock.
func composerHashEdit(ctx *Context, th *Colors, st *composerState, idx int) bool {
	title := fmt.Sprintf("Path %d hash", idx+1)
	for {
		rows := composerHashRows(ctx.sysw)
		sel, ok := composerPickScreen(ctx, th, title, rows.lead, rows.labels)
		if !ok {
			return false // Back at `Which hash?` -- the ONLY false this function returns (spec §4.6)
		}
		// The §8i rule fires when the operator is TAKING a hash: a payload row,
		// the phrase row or the hex row -- stated as that predicate.
		taking := sel < len(rows.digests) || sel == rows.phraseRow || sel == rows.hexRow
		if taking {
			showError(ctx, th, title, composerCopyHashRule())
		}
		switch {
		case sel < len(rows.digests):
			d := rows.digests[sel]
			st.list.Paths[idx].Hash = &d
			return true
		case sel == rows.phraseRow:
			switch hashlockPhraseRoute(ctx, th, st, idx, rows.digests) {
			case hashlockAssigned:
				return true
			case hashlockBackToWhichHash:
				continue
			}
		case sel == rows.hexRow:
			d, ok := composerHexEntry(ctx, th)
			if !ok {
				continue // Back from hex entry returns to `Which hash?`, path intact
			}
			st.list.Paths[idx].Hash = &d
			return true
		case sel == rows.noneRow:
			st.list.Paths[idx].Hash = nil
			composerHashByPhraseSync(st)
			return true
		default:
			panic(fmt.Sprintf("composerHashEdit: pick returned row %d of %d", sel, len(rows.labels)))
		}
	}
}
