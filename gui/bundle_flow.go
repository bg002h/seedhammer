package gui

import (
	"fmt"
	"image"
	"seedhammer.com/sysw"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// bundleFlow is the engraveBundle program: gather a bundle of PUBLIC md1/mk1
// cards over NFC (Phase 1), review/confirm them (Phase 2), then engrave all
// cards' plates verbatim with cross-card guidance + a set-level abort (Phase 3,
// Task 4). The device analogue of host `me bundle`.
//
// SECURITY SPINE: md1/mk1 are PUBLIC → NFC-gathered. An ms1 (codex32 secret) is
// REFUSED in this channel (hand-typed only); a single mk1 is refused (malformed,
// no integrity). No secret is ever gathered, displayed, or engraved.
func bundleFlow(ctx *Context, th *Colors) {
	// A payload card is offered ONCE, before gathering, and then the operator
	// continues scanning as usual — the bundle is a SET, and a source that
	// short-circuited the gather would cap it at whatever the payload held.
	if body, ok := syswOffer(ctx, th, sysw.ClassMDMK, "First card from where?"); ok {
		ctx.syswBundleSeeds = []string{body}
	}
	for {
		cards, ok := bundleGatherFlow(ctx, th, "Engrave Bundle")
		if !ok {
			return // Back / empty bundle.
		}
		if !bundleReviewFlow(ctx, th, cards) {
			// Back from review → resume adding cards. The gather flow starts a
			// fresh accumulator; the operator re-scans (mirrors single-card flows,
			// which also don't persist a half-built set across Back).
			continue
		}
		bundleEngrave(ctx, th, "Engrave Bundle", cards)
		return
	}
}

// ─── Phase 1: gather ─────────────────────────────────────────────────────────

// bundleGatherScreen holds the gather UI state: the accumulator and the last
// per-scan feedback message. Its rendering + status-mapping are factored out
// (pure) so they are unit-tested without an NFC reader.
type bundleGatherScreen struct {
	g   *bundleGatherer
	msg string
	// hasReader is FeatureNFC for this machine. Every operator instruction on
	// this screen that names scanning is conditioned on it — see tally().
	hasReader bool
}

// feedback maps a per-offer status to the operator message (R0-C1/C2). The ms1
// and single-mk1 refusals are explicit, never silent.
func (s *bundleGatherScreen) feedback(status bundleOfferStatus) string {
	switch status {
	case bundleRefusedMs1:
		return "Type the ms1 share on-device, never over NFC."
	case bundleRefusedSingleMK1:
		if !s.hasReader {
			return "Incomplete key card: the payload is missing some of its chunks."
		}
		return "Incomplete key card: scan all its chunks."
	case bundleCardComplete:
		return "Card added."
	case bundleAddedSingleMD1:
		return "Descriptor added."
	case bundleDuplicate:
		return "Already captured that card."
	case bundleDropped:
		return "Not an md1/mk1 card."
	default: // bundleChunkProgress — shown via the tally, no message.
		return ""
	}
}

// tally returns the running on-screen tally of verified cards by type.
//
// THE CLOSING LINE IS READER-AWARE (S1 fold, I2). It read "Scan a card, or
// Done." unconditionally, which is the prompt text spec P0 item 6 rules here;
// only the TITLE is S2's D-4 work.
//
// Keyed on FeatureNFC rather than on which flow is calling, because the property
// is about the MACHINE: on a reader-equipped unit scanning really is available
// and saying so is correct.
//
// CORRECTED 2026-08-15 (operator). This comment used to justify itself with
// "phase-1 hardware has no reader", and that was false: the SH2's ST25R3916 is
// soldered to every board (platform_sh2.go), so Features() reports FeatureNFC
// unconditionally and the reader-less wording reaches no machine an operator
// holds. What phase 1 actually changes is SCOPE — the payload and the keyboard
// are the primary data entries, and NFC is a later pass.
//
// The predicate is left keyed on FeatureNFC deliberately: it is TRUE of the
// hardware, it is the honest question to ask about a machine, and an operator
// who removes NFC does it with a knife — at which point a platform that learns
// to report the change gets the right wording for free. Scanning stays
// AVAILABLE and simply stops being the headline; the payload review that spec
// P0 item 6 requires is the primary surface, and it lands on both arms.
func (s *bundleGatherScreen) tally() []string {
	var nMD, nMK int
	for _, c := range s.g.cards {
		switch c.kind {
		case cardMD1:
			nMD++
		case cardMK1:
			nMK++
		}
	}
	closing := "Scan a card, or Done."
	if !s.hasReader {
		closing = "Done when you have reviewed these."
	}
	return []string{
		fmt.Sprintf("md1 descriptors: %d", nMD),
		fmt.Sprintf("mk1 keys: %d", nMK),
		closing,
	}
}

// bundleGatherFlow accumulates distinct verified cards via NFC, returning them
// on "Done adding cards" (Button3) or (nil,false) on Back / an empty bundle. It
// owns its own scanner goroutine (clone of mk1GatherFlow's shell). With
// testPlatform.NFCReader()==nil the goroutine doesn't run; the gatherer +
// review flow are driven directly in tests.
//
// `title` IS THE CALLER'S, and that is D-4's fix (SPEC §2.2). This gatherer has
// five call sites and only one of them is bundleFlow, so the hard-coded
// "Engrave Bundle" was a screen naming a different program to four of them —
// most visibly mid-way through authoring a wallet policy, where the operator has
// no reason to think they are engraving anything yet.
//
// A PARAMETER RATHER THAN A RENAME. "Engrave Bundle" is CORRECT for bundleFlow;
// renaming the shared default would have fixed one screen by breaking four. It
// is threaded to the two "Done" refusals as well, because one of them is
// reachable from Build (see bundleDonePending below) and a refusal titled for
// the wrong program is D-4 one screen deeper, where the operator is already
// stuck.
func bundleGatherFlow(ctx *Context, th *Colors, title string) ([]bundleCard, bool) {
	scr := &bundleGatherScreen{
		g:         &bundleGatherer{},
		hasReader: ctx.Platform.Features().Has(FeatureNFC),
	}
	// A payload card enters through the SAME offer() every scanned card does,
	// so it is deduplicated, chunk-assembled and validated identically. A
	// separate insertion path would be a second way for a card to become part
	// of a bundle, and only one of them would have the checks.
	//
	// Fed in the order the caller supplies, and this loop reorders nothing. That
	// alone does NOT give record order downstream: a chunked card completes on
	// its LAST chunk, so `g.cards` is completion order and an interleaved
	// payload diverges. The Build path buys the guarantee upstream instead, by
	// grouping each card's chunks contiguously before setting syswBundleSeeds
	// (groupRecordsByCard, gui/multisig_build_payload.go). @N order is
	// identity-bearing (md/encode_multisig.go's ordering contract), so which
	// side of this seam owns the guarantee is worth being exact about.
	for _, seed := range ctx.syswBundleSeeds {
		if seed == "" {
			continue
		}
		scr.g.offer(mdmkText(seed))
	}
	ctx.syswBundleSeeds = nil
	// One loop, one shape, one backoff -- see startScanner (F-126). A nil
	// reader is handled there and yields a channel that never delivers.
	scans, stopScanner := startScanner(ctx, ctx.Platform.NFCReader())
	defer stopScanner()
	backBtn := &Clickable{Button: Button1}
	doneBtn := &Clickable{Button: Button3, AltButton: Center}
	dims := ctx.Platform.DisplaySize()
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		if doneBtn.Clicked(ctx) {
			switch bundleDoneDecision(scr.g) {
			case bundleDoneEmpty:
				// Reader-aware for tally()'s reason: on a machine with no reader
				// "scan a card's chunks first" is a dead end dressed as advice.
				msg := "No complete cards yet. Scan a card's chunks first."
				if !scr.hasReader {
					msg = "No complete cards. Pack them on the host with `me sysw pack` " +
						"and load the payload again."
				}
				showError(ctx, th, title, msg)
			case bundleDonePending:
				// A card is mid-chunk-set: warn it's incomplete and drop it so the
				// operator never engraves a partial. Then proceed with the complete
				// cards (if any), or fall back to the gather screen if none.
				//
				// REACHABLE FROM BUILD (fold, I2): a payload holding enough complete
				// cards PLUS a half chunk set classifies as auto-fill/select, so the
				// pre-gather refusal does not fire and this message is what the
				// operator reads. It said "scan all its chunks".
				scr.g.dropPending()
				pendingMsg := "Dropped an incomplete card. Scan all its chunks to include it."
				if !scr.hasReader {
					pendingMsg = "Dropped an incomplete card: the payload does not carry " +
						"all of its chunks. Rewrite it on the host with `me sysw pack` to include it."
				}
				showError(ctx, th, title, pendingMsg)
				if len(scr.g.cards) > 0 {
					return scr.g.cards, true
				}
			case bundleDoneProceed:
				return scr.g.cards, true
			}
		}
		select {
		case scan := <-scans:
			if scan.Object != nil {
				scr.msg = scr.feedback(scr.g.offer(scan.Object))
			}
		default:
		}
		lines := scr.tally()
		if scr.msg != "" {
			lines = append(lines, scr.msg)
		}
		lineWidth := dims.X - 2*8
		y := leadingSize + 8
		body := make([]op.Op, 0, len(lines))
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: doneBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return nil, false
}

// bundleDoneOutcome is the result of pressing "Done adding cards".
type bundleDoneOutcome int

const (
	bundleDoneEmpty   bundleDoneOutcome = iota // 0 complete cards — nothing to engrave
	bundleDonePending                          // a card is mid-chunk-set — warn + drop
	bundleDoneProceed                          // >=1 complete card, nothing pending
)

// bundleDoneDecision classifies the "Done" gate (Option A). A pending
// (half-scanned) card takes precedence so the operator is always warned before
// a partial card is stranded (risk #2), even when complete cards exist.
func bundleDoneDecision(g *bundleGatherer) bundleDoneOutcome {
	if g.pending() {
		return bundleDonePending
	}
	if len(g.cards) == 0 {
		return bundleDoneEmpty
	}
	return bundleDoneProceed
}

// ─── Phase 2: review / confirm ───────────────────────────────────────────────

// bundleReviewFlow shows the accumulated bundle (count + per-card type +
// verified summary) and lets the operator Confirm (Button3 → true) or Back
// (Button1 → false, resume adding). Read-only; every listed card is already
// integrity-verified (I-1).
func bundleReviewFlow(ctx *Context, th *Colors, cards []bundleCard) bool {
	lines := []string{fmt.Sprintf("%d cards verified:", len(cards))}
	for i, c := range cards {
		lines = append(lines, fmt.Sprintf("%d. %s OK", i+1, c.label))
		if c.summary != "" {
			lines = append(lines, chunkString(c.summary, 24)...)
		}
	}

	backBtn := &Clickable{Button: Button1}
	contBtn := &Clickable{Button: Button3, AltButton: Center}
	pageBtn := &Clickable{Button: Button2}
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	contentTop := leadingSize + 8
	contentBottom := dims.Y - leadingSize
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Bundle")
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

// ─── Phase 3: guided verbatim engrave ────────────────────────────────────────

// bundlePlate is one plate in the cross-card engrave plan: the verbatim card
// string to engrave, plus the "Card X of Y | Plate P of Q" guidance context.
type bundlePlate struct {
	cardIdx    int    // 1-based card position
	cardTotal  int    // total cards in the bundle
	plateIdx   int    // 1-based plate position within this card
	plateTotal int    // total plates for this card
	str        string // the VERBATIM gathered chunk string (I-4)
	label      string // the card label, for the abort warning
}

// bundlePlatePlan flattens the verified cards into a per-plate engrave plan, in
// card-then-plate order. Every plate carries a gathered string UNMODIFIED (I-4)
// — md1 + mk1 alike, no re-encode. A standalone md1 card yields exactly 1 plate.
func bundlePlatePlan(cards []bundleCard) []bundlePlate {
	var plan []bundlePlate
	for ci, c := range cards {
		for pi, s := range c.strings {
			plan = append(plan, bundlePlate{
				cardIdx:    ci + 1,
				cardTotal:  len(cards),
				plateIdx:   pi + 1,
				plateTotal: len(c.strings),
				str:        s,
				label:      c.label,
			})
		}
	}
	return plan
}

// bundleEngrave is the Phase-3 guided verbatim engrave. It is a SIBLING of
// multiPlateEngrave (R0-M2: Go has no default params; deriveXpubFlow's call site
// at derive_xpub.go:162 stays BYTE-UNCHANGED), reusing the same per-plate
// validateMdmk + ChoiceScreen + NewEngraveScreen machinery. It loops the plan,
// titling each plate "Card X of Y | Plate P of Q"; a set-level Back records no
// completed state and warns the partial bundle is unusable (I-5). At the end it
// shows the ms1 reminder (mirror host bundle.rs:296-306).
//
// `title` NAMES THE PROGRAM THE OPERATOR CHOSE (F-182). The end-of-engrave ms1
// reminder used to be hard-coded "Engrave Bundle", and that modal is shown at the
// end of a Build-policy engrave too -- measured: S2's walk cuts 9 plates and
// reaches it -- so an operator who picked "Build policy" off the menu was told,
// by a screen with no other context, that they were somewhere else. It is a
// PARAMETER and not a rename because bundleEngrave is shared by the bundle flow,
// single-sig, the supplied-md1 path and Build alike; the same judgement S2 made
// for bundleGatherFlow's title, applied to the screen at the other end.
func bundleEngrave(ctx *Context, th *Colors, title string, cards []bundleCard) {
	plan := bundlePlatePlan(cards)
	for _, p := range plan {
		labels, plates, err := validateMdmk(ctx.Platform, p.str)
		if err != nil || len(plates) == 0 {
			// A verified card whose string can't fit a plate is unexpected; abort
			// the whole set rather than engrave a partial bundle.
			bundleAbortWarning(ctx, th, cards, p)
			return
		}
		cs := &ChoiceScreen{
			Title:   fmt.Sprintf("Card %d of %d | Plate %d of %d", p.cardIdx, p.cardTotal, p.plateIdx, p.plateTotal), // F-78: "·" is a zero-pixel glyph in this font
			Lead:    "Choose engraving",
			Choices: labels,
		}
		engraved := false
		for !engraved {
			idx, ok := cs.Choose(ctx, th)
			if !ok {
				// Set-level abort: a partial bundle can't be used.
				bundleAbortWarning(ctx, th, cards, p)
				return
			}
			if NewEngraveScreen(ctx, plates[idx]).Engrave(ctx, &engraveTheme) {
				engraved = true
			}
			// Engrave returned without completing (Back) → re-show this plate's
			// variant picker.
		}
	}
	// Whole bundle engraved → remind the operator about the SECRET ms1 share(s)
	// ONLY when none were engraved here (R0-I2): the T5 gather path never produces
	// a cardMS1, so it still shows the reminder; the T6a-2 full single-sig engrave
	// DOES engrave the ms1, so the reminder is suppressed. The decision is derived
	// from the cards slice — no signature/param change (T5's call site is
	// byte-unchanged).
	if bundleShowMs1Reminder(cards) {
		showError(ctx, th, title, bundleMs1ReminderText())
	}
}

// bundleShowMs1Reminder reports whether the end-of-engrave ms1 hand-engrave
// reminder should be shown: true unless an ms1 share was itself engraved in this
// run (any card of kind cardMS1). R0-I2: the gate is purely cards-derived so
// bundleEngrave keeps its T5 signature.
func bundleShowMs1Reminder(cards []bundleCard) bool {
	for _, c := range cards {
		if c.kind == cardMS1 {
			return false
		}
	}
	return true
}

// bundleAbortWarning informs the operator that aborting mid-bundle leaves a
// partial, unusable backup; it records NO completed state (I-5). Dismiss-only.
func bundleAbortWarning(ctx *Context, th *Colors, cards []bundleCard, p bundlePlate) {
	showError(ctx, th, "Bundle Incomplete",
		bundleAbortWarningText(p, bundleSetCarriesASecret(cards)))
}

// bundleSetCarriesASecret reports whether this engrave set puts a SEED on steel:
// true when any card is an ms1.
//
// It is bundleShowMs1Reminder's exact complement, stated in terms of it rather
// than as a second hand-written scan, and derived from the cards for the same
// R0-I2 reason -- the decision comes off the slice, so no other flow's call site
// changes shape to carry it. The two are one question: an ms1 in the set means
// the device cut the seed plate, which is simultaneously why no hand-engrave
// reminder is owed and why an abort must not say "discard".
func bundleSetCarriesASecret(cards []bundleCard) bool {
	return !bundleShowMs1Reminder(cards)
}

// bundleAbortWarningText is what an interrupted operator reads. It replaces
//
//	"A partial bundle can't be used - discard the engraved plate(s) and start
//	 the bundle over."
//
// which was wrong in two directions at once.
//
// (1) "DISCARD" IS AN INSTRUCTION TO BIN A SECRET. Correct for a public plate;
// for one carrying a seed it tells the operator to put their key in the rubbish,
// and nothing in the sentence distinguished them. Full mode cuts the ms1 seed
// plate FIRST, so at almost any abort in a full build the plate already on the
// bench is exactly the one that must not be discarded. This is not a new S5
// requirement -- it corrects text that is WRONG TODAY on a device that engraves
// seeds. The word is DESTROY and the method is named, because "destroy" on its
// own is a word an operator satisfies by putting the plate somewhere else. It is
// said ONLY for a set that carries a seed: a warning that cries DESTROY over an
// md1 chunk is a warning that gets ignored on the run where it matters.
//
// (2) "START THE BUNDLE OVER" THROWS AWAY WORK THAT IS STILL GOOD. The tail cuts
// 6 to 9 plates over hours, nothing records which were cut, and a power loss
// loses that state -- but the encoders are DETERMINISTIC (no rand in md/, mk/ or
// codex32/, and mk's chunk_set_id derives from the bytecode rather than from
// randomness), so re-running the same inputs mints byte-identical plates and only
// the missing ones need cutting. TestReRunMintsByteIdenticalPlates pins the
// property; this is where the operator is told it exists.
//
// AND THIS IS WHERE IT HAS TO BE SAID. The restore document carries the set
// inventory, and it is printed at the end of a SUCCESSFUL run -- an operator
// whose engrave died never reaches it. This modal is the only screen they get.
//
// Its length is gated by the F-185 class check (gui/modal_fits_test.go): the body
// scrolls, nothing on the frame says so, and this machine has no button that
// scrolls it.
func bundleAbortWarningText(p bundlePlate, secret bool) string {
	msg := fmt.Sprintf("Stopped at card %d of %d (%s). This set is not a usable backup yet.\n"+
		"To finish it, run this again and give the same answers: it cuts the same "+
		"plates, byte for byte, so you only cut the ones you are missing.\n",
		p.cardIdx, p.cardTotal, p.label)
	if secret {
		return msg + "If you throw any of it away instead, a plate with your seed on it " +
			"must be DESTROYED, not binned: cut it up or grind the words off."
	}
	return msg + "No plate in this set carries a seed."
}

// bundleMs1ReminderText is the end-of-bundle reminder that the SECRET ms1
// share(s) must be hand-engraved separately (mirror host bundle.rs:296-306).
func bundleMs1ReminderText() string {
	return "Bundle engraved. Also hand-engrave your ms1 share(s) - they are never sent over NFC."
}
