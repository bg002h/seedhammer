package gui

import (
	"fmt"
	"image"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/mt"
	"seedhammer.com/sysw"
	"seedhammer.com/txqr"
)

// engraveTransaction: engrave an already-signed Bitcoin transaction, delivered
// as mt1 strings and/or a tx: record — via the systemwide payload (the primary
// path) or scanned over NFC — as TEXT plates (the mt1 strings verbatim, as
// many per plate as fit) or QR plates (the RAW transaction bytes, so any
// ordinary scanner yields a broadcastable transaction with no constellation
// knowledge).
//
// NOTHING HERE SIGNS, BUILDS OR BROADCASTS. The device engraves what it was
// handed, and only after the set CONFIRMS: complete, reassembling to bytes
// that parse as one transaction whose txid matches the set's chunk_set_id
// (mt.Decode). An unconfirmed set is refused with the missing piece named —
// steel is the one output that cannot be recut cheaply.

// transactionFontMM is the face every transaction plate is cut at: the tested
// legibility floor ("if a glyph reads at 3.0mm it reads at every rung",
// gui/freetext_proof.go), chosen because the brief's stated priority is the
// MINIMUM number of plates and 3.0 is the smallest proven rung.
const transactionFontMM = 3.0

// txCandidate is one engraveable transaction and where it came from.
//
// UNCONFIRMED CANDIDATES ARE STILL ENGRAVEABLE (operator rulings 2026-08-25a
// and 2026-08-25b). An incomplete set, or one that does not reassemble into a
// transaction, is REPORTED LOUDLY AND ENGRAVED -- it is not refused and it is
// not dropped. Two reasons, both operator-stated:
//
//   - a tx: payload is NOT necessarily regenerable. It carries a finalized
//     signing ceremony, and re-signing changes the artifact. Refusing can cost
//     the operator the ceremony; reporting cannot.
//   - every mt1 chunk is independently valid and BCH-protected, and carries
//     its own index and count. 201 engraved chunks plus a 202nd recovered
//     later reassemble -- which is how md/mk multi-card backups already work.
//
// What the operator loses instead is the LEGEND: `subst` replaces their chosen
// text, un-overridably, so the warning rides on the steel. The device has no
// camera and can never read a plate back to warn anyone later, so the plate is
// the only surface where a warning outlives the session.
type txCandidate struct {
	// tx is the zero value when !confirmed -- an unconfirmed set has no txid.
	tx mt.Tx
	// strs is the mt1 string set, in index order — nil when the transaction
	// arrived as a tx: record only, in which case TEXT plates are unavailable
	// (the device deliberately has no mt1 ENCODER: a Go encoder would be a
	// second implementation of a normative format, and Rust may never follow).
	strs []string
	src  syswSource
	// confirmed: the set reassembled into a transaction whose txid matches its
	// chunk_set_id. QR plates require it -- there are no transaction bytes to
	// encode without it -- but TEXT plates do not.
	confirmed bool
	// csid identifies the set even when nothing else does.
	csid uint32
	// subst is the MANDATORY legend when !confirmed. Empty when confirmed.
	subst string
}

// setIsComplete reads the chunk headers alone: count declared, and every index
// 0..count-1 present exactly once. It needs no reassembly, which is the point --
// it distinguishes "missing strings" from "will not decode" for the legend.
func setIsComplete(set []string) bool {
	if len(set) == 0 {
		return false
	}
	h, err := mt.ParseHeader(set[0])
	if err != nil {
		return false
	}
	seen := make(map[int]bool, len(set))
	for _, r := range set {
		hh, err := mt.ParseHeader(r)
		if err != nil || hh.TotalChunks != h.TotalChunks {
			return false
		}
		seen[hh.ChunkIndex] = true
	}
	return len(seen) == h.TotalChunks
}

// legendSubstitution is the text that replaces the operator's own legend on an
// unconfirmed plate (ruling 2026-08-25b). It is not dismissible: a warning the
// operator can turn off is not a control.
func legendSubstitution(complete bool) string {
	if complete {
		return "INCOMPLETE - DOES NOT DECODE - RE-ENCODE PAYLOAD"
	}
	return "INCOMPLETE - MISSING STRINGS - RE-ENCODE PAYLOAD"
}

func engraveTransactionFlow(ctx *Context, th *Colors) {
	engraveTransactionFlowSeeded(ctx, th, "")
}

// engraveTransactionFlowSeeded is the program body; seed is a single mt1
// string that arrived via the start screen's scanner, fed into the gather so
// a scanned chunk is never dropped on the floor.
func engraveTransactionFlowSeeded(ctx *Context, th *Colors, seed string) {
	cands, incomplete := payloadTransactions(ctx)
	if seed != "" {
		// A scan starts a gather; the payload is still reachable next run.
		if c, ok := transactionGatherFlow(ctx, th, seed); ok {
			transactionReviewAndEngrave(ctx, th, c)
		}
		return
	}
	hasReader := ctx.Platform.Features().Has(FeatureNFC)
	if len(cands) == 0 {
		if incomplete > 0 {
			showError(ctx, th, "Engrave Transaction",
				fmt.Sprintf("The payload holds %d mt1 string(s) but no COMPLETE transaction. "+
					"Pack every string of the set with `me sysw pack`.", incomplete))
			return
		}
		if hasReader {
			if c, ok := transactionGatherFlow(ctx, th, ""); ok {
				transactionReviewAndEngrave(ctx, th, c)
			}
			return
		}
		showError(ctx, th, "Engrave Transaction",
			"No transaction loaded. Pack mt1 strings (from `mt encode`) or a tx: record "+
				"(from `me tx`) with `me sysw pack --region`, then Load Payload.")
		return
	}
	// Pick a transaction. One is the common case; the payload MAY hold several.
	choices := make([]string, 0, len(cands)+1)
	for _, c := range cands {
		what := "raw bytes"
		if len(c.strs) > 0 {
			what = fmt.Sprintf("%d strings", len(c.strs))
		}
		choices = append(choices, fmt.Sprintf("TX %s | %d B | %s",
			strings.ToUpper(c.tx.TxidDisplay[:8]), len(c.tx.Raw), what))
	}
	if hasReader {
		choices = append(choices, "Scan instead")
	}
	idx := 0
	if len(choices) > 1 {
		cs := &ChoiceScreen{Title: "Engrave Transaction", Lead: "Which transaction?", Choices: choices}
		i, ok := cs.Choose(ctx, th)
		if !ok {
			return
		}
		idx = i
	}
	if idx == len(cands) { // the trailing "Scan instead"
		if c, ok := transactionGatherFlow(ctx, th, ""); ok {
			transactionReviewAndEngrave(ctx, th, c)
		}
		return
	}
	transactionReviewAndEngrave(ctx, th, cands[idx])
}

// payloadTransactions collects every CONFIRMED transaction the loaded payload
// carries: complete mt1 sets (grouped by chunk_set_id, confirmed by
// mt.Decode) and tx: records (already parse-confirmed at classification).
// A tx: record byte-identical to a decoded set is merged into it, so one
// transaction delivered both ways is offered once, with both plate kinds.
// incomplete counts mt1 strings belonging to no confirmed set.
func payloadTransactions(ctx *Context) (cands []txCandidate, incomplete int) {
	if ctx.sysw == nil {
		return nil, 0
	}
	mtRecs, ok := ctx.sysw.takeAll(sysw.ClassMt)
	if !ok {
		return nil, 0
	}
	sets := map[uint32][]string{}
	var order []uint32
	for _, r := range mtRecs {
		h, err := mt.ParseHeader(strings.TrimSpace(r))
		if err != nil {
			incomplete++
			continue
		}
		if _, seen := sets[h.ChunkSetID]; !seen {
			order = append(order, h.ChunkSetID)
		}
		sets[h.ChunkSetID] = append(sets[h.ChunkSetID], strings.TrimSpace(r))
	}
	for _, csid := range order {
		set := sets[csid]
		ordered := orderByIndex(set)
		tx, err := mt.Decode(set)
		if err != nil {
			// RULING 2026-08-25: report loudly, do not refuse. The set is still
			// offered; the operator engraves the strings they have and the
			// legend says so. `complete` distinguishes the two messages: a full
			// set that will not decode is a different problem from a gap.
			complete := setIsComplete(set)
			incomplete += len(set)
			cands = append(cands, txCandidate{
				strs: ordered, src: srcPayload, csid: csid,
				confirmed: false, subst: legendSubstitution(complete),
			})
			continue
		}
		cands = append(cands, txCandidate{
			tx: tx, strs: ordered, src: srcPayload, csid: csid,
			confirmed: true,
		})
	}
	txRecs, ok := ctx.sysw.takeAll(sysw.ClassTx)
	if ok {
	next:
		for _, r := range txRecs {
			body, err := sysw.DecodeBody(r)
			if err != nil {
				continue
			}
			tx, err := mt.ParseTx(body)
			if err != nil {
				continue
			}
			for _, c := range cands {
				if c.tx.TxidDisplay == tx.TxidDisplay {
					continue next // merged: the set candidate already carries the bytes
				}
			}
			cands = append(cands, txCandidate{tx: tx, src: srcPayload})
		}
	}
	return cands, incomplete
}

// orderByIndex returns the set sorted by chunk index, duplicates dropped —
// the order the TEXT plates engrave in. The strings are already BCH-valid
// (mt.Decode accepted the set), so ParseHeader cannot fail here; a record it
// somehow refuses is skipped rather than engraved out of place.
func orderByIndex(set []string) []string {
	byIdx := map[int]string{}
	max := -1
	for _, s := range set {
		h, err := mt.ParseHeader(s)
		if err != nil {
			continue
		}
		byIdx[h.ChunkIndex] = s
		if h.ChunkIndex > max {
			max = h.ChunkIndex
		}
	}
	out := make([]string, 0, max+1)
	for i := 0; i <= max; i++ {
		if s, ok := byIdx[i]; ok {
			out = append(out, s)
		}
	}
	return out
}

// ─── NFC gather ──────────────────────────────────────────────────────────────

// transactionGatherFlow accumulates scanned mt1 strings until one chunk set is
// COMPLETE AND CONFIRMED, then returns it. Sets are keyed by chunk_set_id, so
// chunks of several transactions may arrive interleaved; the first set to
// confirm wins. A complete set that does NOT confirm (bad parse, txid
// mismatch) is dropped with a message — it can never become engraveable.
func transactionGatherFlow(ctx *Context, th *Colors, seed string) (txCandidate, bool) {
	sets := map[uint32][]string{}
	var msg string
	offer := func(s string) (txCandidate, bool) {
		s = strings.TrimSpace(s)
		h, err := mt.ParseHeader(s)
		if err != nil {
			msg = "Not an mt1 string."
			return txCandidate{}, false
		}
		for _, have := range sets[h.ChunkSetID] {
			if have == s {
				msg = "Already scanned that string."
				return txCandidate{}, false
			}
		}
		sets[h.ChunkSetID] = append(sets[h.ChunkSetID], s)
		set := sets[h.ChunkSetID]
		distinct := map[int]bool{}
		for _, x := range set {
			if hh, err := mt.ParseHeader(x); err == nil {
				distinct[hh.ChunkIndex] = true
			}
		}
		if len(distinct) < h.TotalChunks {
			msg = fmt.Sprintf("String %d of %d. %d to go.",
				h.ChunkIndex+1, h.TotalChunks, h.TotalChunks-len(distinct))
			return txCandidate{}, false
		}
		tx, err := mt.Decode(set)
		if err != nil {
			// Complete and WRONG: parse or txid-binding failure. Unfixable by
			// more scanning; drop the set and say why.
			delete(sets, h.ChunkSetID)
			msg = "Set complete but does not confirm as one transaction. Dropped."
			return txCandidate{}, false
		}
		return txCandidate{tx: tx, strs: orderByIndex(set), src: srcNFC, csid: h.ChunkSetID, confirmed: true}, true
	}
	if seed != "" {
		if c, done := offer(seed); done {
			return c, true
		}
	}
	scans, stopScanner := startScanner(ctx, ctx.Platform.NFCReader())
	defer stopScanner()
	backBtn := &Clickable{Button: Button1}
	dims := ctx.Platform.DisplaySize()
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return txCandidate{}, false
		}
		select {
		case scan := <-scans:
			if s, ok := scan.Object.(mtText); ok {
				if c, done := offer(string(s)); done {
					return c, true
				}
			} else if scan.Object != nil {
				msg = "Not an mt1 string."
			}
		default:
		}
		var n int
		for _, set := range sets {
			n += len(set)
		}
		lines := []string{
			fmt.Sprintf("mt1 strings: %d", n),
			"Scan each string of the set.",
		}
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Engrave Transaction")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return txCandidate{}, false
}

// ─── review + engrave ────────────────────────────────────────────────────────

// transactionReviewLines is what the operator confirms against `mt encode`'s
// or `me tx`'s own report: the FULL txid (the value the host printed), the
// size, and the source (§3.3.3 F3 — anything not typed names its source).
func transactionReviewLines(c txCandidate) []string {
	if !c.confirmed {
		// RULING 2026-08-25: engraveable, loudly. No txid exists to show -- the
		// set never reassembled -- so the set id is the only identifier there is.
		return []string{
			"UNCONFIRMED SET", "",
			fmt.Sprintf("Set %05x, %d string(s).", c.csid, len(c.strs)),
			"",
			"This does NOT reassemble into",
			"a transaction. The strings are",
			"engraveable and each is valid,",
			"but the set is not complete.",
			"",
			"The plate legend WILL be",
			"replaced with:",
			c.subst,
			"",
			"QR plates are unavailable:",
			"there are no transaction bytes.",
		}
	}
	lines := []string{"Engrave this transaction?", ""}
	lines = append(lines, chunkString(c.tx.TxidDisplay, 16)...)
	lines = append(lines, "",
		fmt.Sprintf("%d bytes, %d in, %d out", len(c.tx.Raw), c.tx.Inputs, c.tx.Outputs),
		"Source: "+syswSourceName(c.src),
		"BEARER: anyone holding the",
		"plates can broadcast it.")
	return lines
}

func transactionReviewAndEngrave(ctx *Context, th *Colors, c txCandidate) {
	if !confirmReviewScreen(ctx, th, "Transaction", transactionReviewLines(c)) {
		return
	}
	var choices []string
	if len(c.strs) > 0 {
		choices = append(choices, "TEXT PLATES")
	}
	// QR encodes the RAW TRANSACTION. An unconfirmed set has none, so the
	// choice is withheld rather than offered and then failed (r9's lesson: a
	// gate that cannot pass is worse than one that is absent).
	if c.confirmed {
		choices = append(choices, "QR PLATES")
	}
	if len(choices) == 0 {
		return
	}
	for {
		cs := &ChoiceScreen{Title: "Engrave Transaction", Lead: "Choose plate kind", Choices: choices}
		i, ok := cs.Choose(ctx, th)
		if !ok {
			return
		}
		var plates []Plate
		var titles []string
		var note string
		var err error
		if choices[i] == "TEXT PLATES" {
			plates, titles, err = planTransactionTextPlates(ctx.Platform, c)
			if err == nil {
				note = fmt.Sprintf("%d plate(s), %d string(s)", len(plates), len(c.strs))
			}
		} else {
			plates, titles, note, err = planTransactionQRPlates(ctx.Platform, c.tx)
		}
		if err != nil {
			showError(ctx, th, "Engrave Transaction", err.Error())
			continue
		}
		// The configuration is stated BEFORE the first cut — which plates, how
		// many, at what protection — because plate count and ECC are the two
		// numbers the operator budgeted blanks and time by.
		if !confirmReviewScreen(ctx, th, "Plates", []string{note, "", "Engrave?"}) {
			continue
		}
		if engraveTransactionPlates(ctx, th, plates, titles) {
			return
		}
	}
}

// engraveTransactionPlates cuts the plan in order. A set-level Back warns the
// partial set is not a usable artifact — same posture as bundleEngrave, and
// like there a re-run mints byte-identical plates, so the honest advice is
// finish in one sitting or start over.
func engraveTransactionPlates(ctx *Context, th *Colors, plates []Plate, titles []string) bool {
	for i, p := range plates {
		cs := &ChoiceScreen{
			Title:   titles[i],
			Lead:    "Engrave this plate",
			Choices: []string{"ENGRAVE"},
		}
		engraved := false
		for !engraved {
			_, ok := cs.Choose(ctx, th)
			if !ok {
				showError(ctx, th, "Transaction Incomplete",
					fmt.Sprintf("Stopped at plate %d of %d. A partial set does not carry the "+
						"whole transaction. A re-run cuts the same plates byte for byte, but "+
						"starts at plate 1 - finish a set in one sitting, or start over.",
						i+1, len(plates)))
				return false
			}
			if NewEngraveScreen(ctx, p).Engrave(ctx, &engraveTheme) {
				engraved = true
			}
		}
	}
	return true
}

// ─── plate planning ──────────────────────────────────────────────────────────

// transactionPlateTitle is plate row 0: which transaction, which plate. Plate
// numbering is for HUMANS (the drawer-of-plates problem); the chunk headers
// and QR Structured Append indices are what machines order by, and the two
// numberings are never presented as one.
func transactionPlateTitle(tx mt.Tx, n, m int) string {
	return fmt.Sprintf("TX %s %d/%d", strings.ToUpper(tx.TxidDisplay[:8]), n, m)
}

// transactionSetPlateTitle names an UNCONFIRMED set, which has no txid because
// it never reassembled. The set id is the only identifier it has.
func transactionSetPlateTitle(csid uint32, n, m int) string {
	return fmt.Sprintf("SET %05X %d/%d", csid, n, m)
}

// planTransactionTextPlates packs the mt1 strings onto plates verbatim, in
// index order, AS MANY PER PLATE AS FIT — the brief's stated requirement, to
// minimise total plates. Greedy first-fit is optimal here because every chunk
// string has the same length (the final may be shorter): each paragraph costs
// the same rows, so no later reordering can save a plate.
//
// Fit is decided by the real thing: build the plate, plan the engraving,
// toPlate rejects overflow — the same one-source-of-truth rule the wrap code
// (backup/wrap.go) states for lines.
func planTransactionTextPlates(pl Platform, c txCandidate) ([]Plate, []string, error) {
	strs := c.strs
	params := pl.EngraverParams()
	// RULING 2026-08-25b: an unconfirmed set engraves, and the operator's own
	// legend is REPLACED -- un-overridably -- by the warning. There is no code
	// path that restores it: a warning the operator can turn off is not a
	// control, and the plate is the only surface that outlives the session on a
	// device with no camera.
	subst := c.subst
	// Two passes: first count the plates (titles carry n/m), then build.
	// build constructs the i-th plate holding strs[lo:hi] with title t.
	build := func(lo, hi int, title string) (Plate, error) {
		paras := make([]backup.Paragraph, 0, hi-lo+1)
		if subst != "" {
			paras = append(paras, backup.Paragraph{Text: subst})
		}
		for _, s := range strs[lo:hi] {
			paras = append(paras, backup.Paragraph{Text: s})
		}
		plate := backup.Text{
			Paragraphs: paras,
			Font:       sh.Font,
			FontSize:   transactionFontMM,
			Title:      title,
		}
		return toPlate(backup.EngraveText(params, plate), params)
	}
	// Pass 1: how many strings fit each plate. The title used for counting has
	// the widest realistic shape so the count cannot loosen when m grows.
	var bounds [][2]int
	for lo := 0; lo < len(strs); {
		hi := lo + 1
		if _, err := build(lo, hi, "TX WWWWWWWW 16/16"); err != nil {
			return nil, nil, fmt.Errorf("an mt1 string does not fit one plate: %w", err)
		}
		for hi < len(strs) {
			if _, err := build(lo, hi+1, "TX WWWWWWWW 16/16"); err != nil {
				break
			}
			hi++
		}
		bounds = append(bounds, [2]int{lo, hi})
		lo = hi
	}
	// Pass 2: build with the real titles.
	plates := make([]Plate, 0, len(bounds))
	titles := make([]string, 0, len(bounds))
	for i, b := range bounds {
		var title string
		if c.confirmed {
			title = transactionPlateTitle(c.tx, i+1, len(bounds))
		} else {
			title = transactionSetPlateTitle(c.csid, i+1, len(bounds))
		}
		p, err := build(b[0], b[1], title)
		if err != nil {
			return nil, nil, err
		}
		plates = append(plates, p)
		titles = append(titles, title)
	}
	return plates, titles, nil
}

// transactionLegend is plate 1's human block, per the measured QR findings:
// the txid (what the operator compares against the host's report), what the
// QR carries, and the one operational fact a recoverer must be told — scan
// order is IRRELEVANT, the order lives inside each symbol.
func transactionLegend(tx mt.Tx, symbols int, ecc qr.Level) string {
	eccName := map[qr.Level]string{qr.L: "L", qr.M: "M", qr.Q: "Q", qr.H: "H"}[ecc]
	lines := []string{
		"txid " + tx.TxidDisplay,
		fmt.Sprintf("raw signed bitcoin tx, %d bytes, %d qr, ecc %s", len(tx.Raw), symbols, eccName),
	}
	if symbols > 1 {
		lines = append(lines, "scan all qr, any order, then broadcast")
	} else {
		lines = append(lines, "scan, then broadcast")
	}
	return strings.Join(lines, "\n")
}

// planTransactionQRPlates chooses the QR configuration by the measured
// objective (QR findings, operator-ruled 2026-08-24):
//
//  1. fewest PLATES
//  2. fewest SYMBOLS   — Structured Append has no cross-symbol redundancy,
//     so every symbol is an independent fatal point
//  3. ECC floor M      — a CONSTRAINT, never traded; the only thing that
//     survives distributed damage
//  4. then highest ECC, then the largest module (0.9mm before 0.6mm)
//
// One symbol per plate. For P >= 2 plates the P-1-symbols-plus-legend-plate
// layout is tried BEFORE P symbols with the legend inline, because fewer
// symbols outranks everything but plate count. The 16-symbol cap is hard.
func planTransactionQRPlates(pl Platform, tx mt.Tx) ([]Plate, []string, string, error) {
	params := pl.EngraverParams()
	type layout struct {
		symbols     int
		legendAlone bool
	}
	eccName := map[qr.Level]string{qr.L: "L", qr.M: "M", qr.Q: "Q", qr.H: "H"}
	for plateCount := 1; plateCount <= txqr.MaxSymbols+1; plateCount++ {
		layouts := []layout{}
		if plateCount >= 2 && plateCount-1 <= txqr.MaxSymbols {
			layouts = append(layouts, layout{symbols: plateCount - 1, legendAlone: true})
		}
		if plateCount <= txqr.MaxSymbols {
			layouts = append(layouts, layout{symbols: plateCount, legendAlone: false})
		}
		for _, lay := range layouts {
			for _, ecc := range []qr.Level{qr.H, qr.Q, qr.M} {
				for _, scale := range []int{3, 2} { // 0.9mm, then 0.6mm modules
					plates, titles, ok := buildQRPlates(params, tx, lay.symbols, lay.legendAlone, ecc, scale)
					if ok {
						note := fmt.Sprintf("%d plate(s), %d QR, ECC %s, %s modules",
							plateCount, lay.symbols, eccName[ecc],
							map[int]string{3: "0.9mm", 2: "0.6mm"}[scale])
						return plates, titles, note, nil
					}
				}
			}
		}
	}
	return nil, nil, "", fmt.Errorf("transaction too large for QR plates at ECC M "+
		"(%d bytes; the Structured Append cap is %d symbols) - use TEXT plates",
		len(tx.Raw), txqr.MaxSymbols)
}

// buildQRPlates attempts one configuration; ok=false means it does not fit.
//
// A cheap geometry gate runs before any spline is planned: a symbol whose
// module grid plus quiet border cannot fit the usable width at this scale can
// never pass toPlate, and toPlate plans a full engraving to find that out.
func buildQRPlates(params engrave.Params, tx mt.Tx, symbols int, legendAlone bool, ecc qr.Level, scale int) ([]Plate, []string, bool) {
	set, err := txqr.EncodeSet(tx.Raw, symbols, ecc)
	if err != nil {
		return nil, nil, false
	}
	usable := params.F(85) - 2*params.I(3) - 2*params.I(2) // plate - margins - qr border
	for _, c := range set {
		if c.Size*params.StrokeWidth*scale > usable {
			return nil, nil, false
		}
	}
	plateCount := symbols
	if legendAlone {
		plateCount++
	}
	var plates []Plate
	var titles []string
	build := func(txt backup.Text, title string) bool {
		txt.Font = sh.Font
		txt.FontSize = transactionFontMM
		txt.Title = title
		p, err := toPlate(backup.EngraveText(params, txt), params)
		if err != nil {
			return false
		}
		plates = append(plates, p)
		titles = append(titles, title)
		return true
	}
	legend := transactionLegend(tx, symbols, ecc)
	plateNo := 1
	if legendAlone {
		if !build(backup.Text{Paragraphs: []backup.Paragraph{{Text: legend}}},
			transactionPlateTitle(tx, plateNo, plateCount)) {
			return nil, nil, false
		}
		plateNo++
	}
	for i, c := range set {
		paras := []backup.Paragraph{}
		if i == 0 && !legendAlone {
			paras = append(paras, backup.Paragraph{Text: legend, QR: c, QRScale: scale})
		} else {
			paras = append(paras, backup.Paragraph{QR: c, QRScale: scale})
		}
		if !build(backup.Text{Paragraphs: paras},
			transactionPlateTitle(tx, plateNo, plateCount)) {
			return nil, nil, false
		}
		plateNo++
	}
	return plates, titles, true
}
