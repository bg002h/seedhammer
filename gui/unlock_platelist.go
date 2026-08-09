package gui

import (
	"fmt"
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
)

// §10.3's plate list — paged, selectable, and Back IS Lock.
//
// Modelled on bundleReviewFlow, which already fits Back / Page / OK inside the
// three-slot nav budget (layoutNavigation indexes a fixed [3]int, so a fourth
// affordance panics). It differs in three deliberate ways: it is selectable
// rather than read-only, entries are labelled from §6.3's grouping rather than
// from anything the sealer asserted, and Back is the session exit.
//
// B1 holds only public records, so there is nothing to wipe on the way out. The
// SHAPE still has to be right here, or B2 inherits a list it cannot extend
// without a fourth nav slot.

// unlockEngraveHook is a test-only seam that observes which record OK selected,
// so a test can assert the SELECTED record's bytes reach the engrave path
// rather than merely that some screen appeared. nil in production. Mirrors
// bip85SeedHook, the sanctioned in-file seam.
var unlockEngraveHook func(label string, record []byte)

// plateLabel renders §10.2.2's label for one entry, from the CLASSIFIED type
// and the §6.3 card grouping — never from anything the sealer asserted, and
// never by re-deriving either in the UI.
//
//	one card of this HRP      mk1 1/2            plate within the card
//	several cards of this HRP mk1 2/3 | 1/2      card, then plate within it
//
// §10.2.2's own examples are single-sig, where there is exactly one card of
// each HRP, so the first form reproduces them exactly. The second exists
// because a 2-of-3 is mk1 x6 across THREE cards: a flat mk1 1/6..6/6 leaves the
// operator unable to tell which cosigner a plate belongs to, which is §6.4's
// incomplete-backup-believed-complete hazard wearing a label.
func plateLabel(r seal.AdmittedRecord, i int) string {
	var hrp string
	switch r.HRP {
	case 'd':
		hrp = "md1"
	case 'k':
		hrp = "mk1"
	default:
		// Unreachable FOR THE PUBLIC SECTION: §10.2.1's allow-list admits only
		// ClassMDMK there and pass 3 sets HRP for every one of them. REACHABLE
		// for the encrypted section once B2a-ii wires it (F-77), where ms1
		// records, bare mnemonics and every card of a failed grouping all keep
		// HRP 0 — label_encrypted.go discards a grouping failure precisely
		// because this branch renders it honestly. Named rather than
		// mislabelled — an entry that is somehow neither must look odd, not look
		// like an md1.
		return fmt.Sprintf("record %d", i+1)
	}
	if r.CardTotal > 1 {
		return fmt.Sprintf("%s %d/%d | %d/%d", hrp, r.CardIndex, r.CardTotal, r.PlateIndex, r.PlateTotal)
	}
	return fmt.Sprintf("%s %d/%d", hrp, r.PlateIndex, r.PlateTotal)
}

// unlockPlateListFlow lists every public record and engraves the selected one.
// Returning leaves the session (§10.2.2: Lock, Back, an error and ctx.Done are
// one exit, not four).
func unlockPlateListFlow(ctx *Context, th *Colors, recs []seal.AdmittedRecord) {
	labels := make([]string, len(recs))
	for i, r := range recs {
		labels[i] = plateLabel(r, i)
	}

	backBtn := &Clickable{Button: Button1}
	pageBtn := &Clickable{Button: Button2}
	okBtn := &Clickable{Button: Button3, AltButton: Center}
	// One hit target per entry. The zero Button is None, so these respond to
	// touch only and never collide with the three nav slots.
	entries := make([]Clickable, len(labels))

	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	contentTop := leadingSize + 8
	contentBottom := dims.Y - leadingSize
	start, sel := 0, 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return
		}
		for i := range entries {
			if entries[i].Clicked(ctx) {
				sel = i
			}
		}
		if okBtn.Clicked(ctx) && sel < len(recs) {
			unlockEngraveFlow(ctx, th, recs[sel], labels[sel])
			// start is untouched, so the operator lands back on the page they
			// engraved from. Dropping them to page 1 after every plate is how
			// a fifteen-record bundle gets one plate cut twice and another not
			// at all.
			continue
		}

		shown := 0
		y := contentTop
		body := make([]op.Op, 0, len(labels))
		for i := start; i < len(labels); i++ {
			col := th.Text
			if i == sel {
				col = th.Background
			}
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, col, labels[i])
			if i > start && y+sz.Y > contentBottom {
				break
			}
			pos := image.Pt((dims.X-sz.X)/2, y)
			hit := image.Rectangle{
				Min: pos.Sub(image.Pt(buttonPadX, buttonPadY)),
				Max: pos.Add(sz).Add(image.Pt(buttonPadX, buttonPadY)),
			}
			entry := lbl.Offset(pos)
			if i == sel {
				entry = op.Layer(entry,
					op.Compose(
						op.Color(&ctx.B, th.Text),
						op.RoundedRect2(&ctx.B, hit, cornerRadius),
					),
				)
			}
			body = append(body, op.Layer(entry, op.Input(&ctx.B, &entries[i]).Clip(hit)))
			y += sz.Y + 6
			shown++
			if y > contentBottom {
				break
			}
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(labels) {
				start += shown
			} else {
				start = 0
			}
			// The selection follows the page. Leaving it behind would let OK
			// engrave a record the operator cannot see.
			sel = start
			continue
		}

		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, unlockTitle)
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}

// unlockEngraveFlow engraves one public record: validateMdmk -> a variant
// ChoiceScreen -> NewEngraveScreen.
//
// It does NOT reuse mdmkFlow, and that is deliberate. mdmkFlow prepends an
// "Inspect key" / "Inspect descriptor" entry that calls mk1GatherFlow or
// md1GatherFlow, and both prime a FRESH gatherer with only the single string
// handed to them; when that alone is not a complete card — true for every
// chunked record, which is the ordinary case — they open the NFC reader and
// wait for the operator to tap the remaining physical tags. A payload-derived
// record has no tags to tap: every chunk is already in p.Public, and the
// gatherer has no way to reach them. Inspecting a payload-sourced card is a
// legitimate thing to want and is filed as F-76; it needs a gatherer primed
// from the payload, and it is not B1.
func unlockEngraveFlow(ctx *Context, th *Colors, rec seal.AdmittedRecord, label string) {
	// AdmittedRecord.Record is []byte deliberately, so that B2 can zero it. The
	// conversion below is HARMLESS HERE — B1 holds public data only — and
	// ACTIVELY WRONG in B2, where the same call shape on a secret record makes
	// an unwipeable copy that Payload.Wipe cannot reach.
	//
	// It is written at the call site, with this comment, rather than behind a
	// String() helper on AdmittedRecord: a helper is an invitation to call it
	// on a secret.
	str := string(rec.Record)
	if unlockEngraveHook != nil {
		unlockEngraveHook(label, rec.Record)
	}
	variants, plates, err := validateMdmk(ctx.Platform.EngraverParams(), str)
	if err != nil || len(plates) == 0 {
		showError(ctx, th, unlockTitle, "This record does not fit any plate size.")
		return
	}
	cs := &ChoiceScreen{Title: label, Lead: "Choose engraving", Choices: variants}
	for {
		choice, ok := cs.Choose(ctx, th)
		if !ok {
			return
		}
		if NewEngraveScreen(ctx, plates[choice]).Engrave(ctx, &engraveTheme) {
			return
		}
	}
}
