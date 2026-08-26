package gui

import (
	"errors"
	"fmt"
	"image"
	"sort"
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
	// confirmed: this device parsed the transaction, derived its txid, and
	// found every input signed. It is what the review screen VOUCHES for.
	//
	// IT IS NOT THE SAME QUESTION AS "can this be a QR" -- that is hasBytes().
	// Conflating them is what made the whole tx: path inert: a signed tx:
	// record was built without setting this field, whose zero value is false,
	// so the device showed a signed transaction the UNCONFIRMED SET screen and
	// then offered no plate kind at all.
	confirmed bool
	// csid identifies the set even when nothing else does. Zero for a tx:
	// record, which is not a set and has no chunk_set_id.
	csid uint32
	// subst is the MANDATORY legend when !confirmed. Empty when confirmed.
	subst string
	// unsigned lists the inputs carrying neither a scriptSig nor a witness.
	// Only ever non-empty for a tx: RECORD -- an mt1 set carrying an unsigned
	// transaction does not decode at all (mt.Decode refuses it), so it arrives
	// with no tx and no bytes.
	unsigned []int
}

// hasBytes reports whether there are transaction bytes to put into a QR.
//
// An unconfirmed SET has none: it never reassembled, so there is nothing to
// encode and the choice is withheld rather than offered and then failed. An
// UNSIGNED tx: record has all of them -- it parses perfectly, it simply cannot
// be broadcast -- so it engraves under the substituted legend, which is the
// 2026-08-25b posture applied to the class the ruling did not name.
func (c txCandidate) hasBytes() bool { return len(c.tx.Raw) > 0 }

// transactionPlateKinds is what the operator may choose for this candidate.
//
// EXTRACTED SO IT CAN BE TESTED. It was three lines inside
// transactionReviewAndEngrave, and the case where it came back EMPTY -- which
// makes the program return with no screen at all -- was reachable by the most
// ordinary payload there is.
func transactionPlateKinds(c txCandidate) []string {
	var choices []string
	if len(c.strs) > 0 {
		choices = append(choices, "TEXT PLATES")
	}
	if c.hasBytes() {
		choices = append(choices, "QR PLATES")
	}
	return choices
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

// legendUnsigned is the THIRD substitution, for the case the ruling did not
// name because the class was supposed to be refused at the host: a tx: record
// that parses and whose txid is right and which CANNOT BE BROADCAST.
//
// It reaches the device only when the operator passed `me sysw pack
// --allow-unsigned-inputs`, which exists for honest empty inputs (P2A anchor
// spends). Refusing here would make that override useless; engraving it under
// the operator's own legend would put a plate in a drawer that looks like a
// backup of a spendable transaction and is not. So: engrave, and put the
// warning where it outlives the session.
//
// Kept SHORT deliberately -- it shares plate 1 with a QR symbol, and
// TestTheUnsignedLegendStillFitsAQRPlate is what stops it growing past that.
const legendUnsigned = "UNSIGNED INPUT - CANNOT BE BROADCAST - RE-EXPORT"

// substitutionFor picks the legend that replaces the operator's own, from what
// ACTUALLY went wrong.
//
// ONE FUNCTION, USED BY BOTH DELIVERY PATHS, and that is the point of G-P3.9:
// the payload path engraved a complete-but-non-decoding set under a
// substituted legend while the NFC gather DROPPED the identical set with
// "Set complete but does not confirm as one transaction. Dropped." Two
// behaviours for one condition, and the drop contradicted ruling 2026-08-25b
// outright -- the operator loses every string they scanned.
func substitutionFor(set []string, err error) string {
	if errors.Is(err, mt.ErrUnsignedInputs) {
		// The bytes ARE a transaction and the txid IS right. Calling that
		// "DOES NOT DECODE" would send the operator to re-encode a payload
		// that is encoded perfectly well.
		return legendUnsigned
	}
	return legendSubstitution(setIsComplete(set))
}



// txNothingToEngrave is R11' as one function, so the two situations cannot
// drift into one sentence again.
//
// `incomplete` counts mt1 strings that belong to no offered set at all. It is a
// SUFFIX rather than a message of its own: the payload is still loaded and
// still holds things, so the operator needs the inventory either way.
func txNothingToEngrave(s *syswSession, incomplete int) string {
	if s == nil || !s.loaded {
		return "No payload is loaded.\n\nLoad one with Load Payload, or write one with " +
			"`me sysw pack --region`."
	}
	if !s.compared {
		// A payload nobody has authenticated is not a payload with nothing in
		// it, and the fix is different: compare the digest. Saying "holds no
		// transaction" here would be a claim about contents this session is
		// not allowed to read.
		return "This payload has not been checked, so nothing may be taken from it.\n\n" +
			"Compare its digest at Load Payload."
	}
	msg := "This payload holds no transaction.\n\nIt holds: " + txPayloadHolds(s) + "."
	if incomplete > 0 {
		msg += fmt.Sprintf("\n\n%d mt1 string(s) belong to no complete set. "+
			"Pack every string of the set with `me sysw pack`.", incomplete)
	}
	return msg
}

// txPayloadHolds is the payload's inventory, by class, counted.
//
// The operator cannot otherwise tell a payload with the WRONG contents from an
// empty one, and those have different fixes -- re-pack versus pack at all.
func txPayloadHolds(s *syswSession) string {
	seen := map[string]int{}
	for _, r := range s.records {
		seen[txClassName(r.class)]++
	}
	if len(seen) == 0 {
		return "nothing"
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", seen[k], k))
	}
	return strings.Join(parts, ", ")
}

// namedInputs is "input 1" / "inputs 1 and 3" / "inputs 0, 2 and 5", for an
// operator. Mirrors me-cli's `name_inputs`.
func namedInputs(idx []int) string {
	n := make([]string, 0, len(idx))
	for _, i := range idx {
		n = append(n, fmt.Sprintf("%d", i))
	}
	switch len(n) {
	case 0:
		return "No input"
	case 1:
		return "Input " + n[0]
	default:
		return "Inputs " + strings.Join(n[:len(n)-1], ", ") + " and " + n[len(n)-1]
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// txClassName names a record class for an operator.
//
// It lives HERE, in fork-native UI code, and not in package sysw: the Rust
// primary has no such function, and adding one to the port would be the port
// leading. These are words on a screen, not normative behaviour.
func txClassName(c sysw.Class) string {
	switch c {
	case sysw.ClassMnemonic:
		return "BIP-39 mnemonic"
	case sysw.ClassCodex32Secret:
		return "codex32 secret"
	case sysw.ClassPassphrase:
		return "passphrase"
	case sysw.ClassFreeText:
		return "free text"
	case sysw.ClassDescriptor:
		return "descriptor"
	case sysw.ClassMDMK:
		return "md1/mk1 card"
	case sysw.ClassAddress:
		return "address"
	case sysw.ClassMt:
		return "mt1 chunk"
	case sysw.ClassTx:
		return "raw transaction"
	}
	return "unrecognised record"
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
		// R11' -- TWO DISTINCT MESSAGES. The carousel entry is UNCONDITIONAL,
		// so the most common way to reach it is with NO PAYLOAD AT ALL: a
		// fresh boot where the operator declined the offer, or a machine that
		// has never had a payload written. Telling that operator their payload
		// "holds no transaction" names a payload that does not exist; telling
		// the other one to load a payload sends them to re-do a step they did.
		// Both name the fix; only the second may speak about contents.
		showError(ctx, th, "Engrave Transaction", txNothingToEngrave(ctx.sysw, incomplete))
		// THE MESSAGE IS SHOWN WHETHER OR NOT THERE IS A READER. The SH2 has
		// one soldered to every board, so gating it on !hasReader would show
		// it to nobody: the operator would be dropped into a scanner with no
		// statement of why. With a reader, the scanner follows the message.
		if hasReader {
			if c, ok := transactionGatherFlow(ctx, th, ""); ok {
				transactionReviewAndEngrave(ctx, th, c)
			}
		}
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
			incomplete += len(set)
			cands = append(cands, txCandidate{
				strs: ordered, src: srcPayload, csid: csid,
				confirmed: false, subst: substitutionFor(set, err),
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
				// Guarded on confirmed: an UNCONFIRMED set has the zero-value
				// tx, whose TxidDisplay is "", and merging a real record into
				// it would hand the operator a candidate with no bytes.
				if c.confirmed && c.tx.TxidDisplay == tx.TxidDisplay {
					continue next // merged: the set candidate already carries the bytes
				}
			}
			if !tx.EveryInputSigned {
				// The signature predicate, on the tx: class, ON THE DEVICE.
				// sysw.Classify requires only a structural parse, so this
				// record IS ClassTx and reaches here -- and its txid is
				// byte-identical to a signed version's, because stripping the
				// signatures is exactly what the txid ignores. Nothing else on
				// this screen can tell the two apart.
				cands = append(cands, txCandidate{
					tx: tx, src: srcPayload, confirmed: false,
					subst: legendUnsigned, unsigned: tx.UnsignedInputs,
				})
				continue
			}
			cands = append(cands, txCandidate{tx: tx, src: srcPayload, confirmed: true})
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

// txGather is the gather's STATE AND DECISION, lifted out of the frame loop.
//
// It is a type rather than a closure because the divergence G-P3.9 closed
// lived inside that closure and no test could reach it: every gather test in
// the tree drove screens, so the one line that decided a complete-but-broken
// set's fate was exercised by nothing.
type txGather struct {
	sets map[uint32][]string
	msg  string
}

func newTxGather() *txGather { return &txGather{sets: map[uint32][]string{}} }

// strings is how many distinct strings are held, across all sets.
func (g *txGather) strings() int {
	var n int
	for _, set := range g.sets {
		n += len(set)
	}
	return n
}

// offer accepts one scanned string. It returns a candidate when the set it
// belongs to becomes DECIDABLE -- confirmed, or complete and definitively not
// confirmable. Anything more scanning could still fix returns false with a
// message.
func (g *txGather) offer(s string) (txCandidate, bool) {
	s = strings.TrimSpace(s)
	{
		h, err := mt.ParseHeader(s)
		if err != nil {
			g.msg = "Not an mt1 string."
			return txCandidate{}, false
		}
		for _, have := range g.sets[h.ChunkSetID] {
			if have == s {
				g.msg = "Already scanned that string."
				return txCandidate{}, false
			}
		}
		g.sets[h.ChunkSetID] = append(g.sets[h.ChunkSetID], s)
		set := g.sets[h.ChunkSetID]
		distinct := map[int]bool{}
		for _, x := range set {
			if hh, err := mt.ParseHeader(x); err == nil {
				distinct[hh.ChunkIndex] = true
			}
		}
		if len(distinct) < h.TotalChunks {
			g.msg = fmt.Sprintf("String %d of %d. %d to go.",
				h.ChunkIndex+1, h.TotalChunks, h.TotalChunks-len(distinct))
			return txCandidate{}, false
		}
		tx, err := mt.Decode(set)
		if err != nil {
			// G-P3.9. Complete and WRONG. This used to DROP the set --
			// `delete(sets, ...)` and "Set complete but does not confirm as
			// one transaction. Dropped." -- which threw away every string the
			// operator had just scanned, and contradicted ruling 2026-08-25b
			// while the PAYLOAD path, three functions up, engraved the
			// identical set under a substituted legend. Two behaviours for one
			// condition.
			//
			// Now it is offered exactly as the payload path offers it: an
			// unconfirmed candidate whose legend is replaced. The review
			// screen says what is wrong and the operator decides. More
			// scanning cannot fix it, so returning is right -- dropping was
			// not.
			return txCandidate{
				strs: orderByIndex(set), src: srcNFC, csid: h.ChunkSetID,
				confirmed: false, subst: substitutionFor(set, err),
			}, true
		}
		return txCandidate{tx: tx, strs: orderByIndex(set), src: srcNFC, csid: h.ChunkSetID, confirmed: true}, true
	}
}

// transactionGatherFlow accumulates scanned mt1 strings until one chunk set is
// decidable, then returns it. Sets are keyed by chunk_set_id, so chunks of
// several transactions may arrive interleaved.
func transactionGatherFlow(ctx *Context, th *Colors, seed string) (txCandidate, bool) {
	g := newTxGather()
	if seed != "" {
		if c, done := g.offer(seed); done {
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
				if c, done := g.offer(string(s)); done {
					return c, true
				}
			} else if scan.Object != nil {
				g.msg = "Not an mt1 string."
			}
		default:
		}
		lines := []string{
			fmt.Sprintf("mt1 strings: %d", g.strings()),
			"Scan each string of the set.",
		}
		if g.msg != "" {
			lines = append(lines, g.msg)
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
	if !c.confirmed && c.hasBytes() {
		// A tx: record that PARSES and cannot be BROADCAST. Its txid is the
		// one the operator expects -- that is the whole hazard -- so the
		// screen states the txid AND immediately states what it is not worth.
		lines := []string{"UNSIGNED TRANSACTION", ""}
		lines = append(lines, chunkString(c.tx.TxidDisplay, 16)...)
		lines = append(lines, "",
			fmt.Sprintf("%d bytes, %d in, %d out", len(c.tx.Raw), c.tx.Inputs, c.tx.Outputs),
			namedInputs(c.unsigned)+" carr"+plural(len(c.unsigned), "ies", "y"),
			"neither a scriptSig nor a",
			"witness. This CANNOT be",
			"broadcast.",
			"",
			"The txid above is the same one",
			"a signed version would have.",
			"",
			"The plate legend WILL be",
			"replaced with:",
			c.subst,
			"",
			"Source: "+syswSourceName(c.src))
		return lines
	}
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
	// QR encodes the RAW TRANSACTION, so it is offered exactly when there ARE
	// bytes -- withheld rather than offered and then failed (r9's lesson: a
	// gate that cannot pass is worse than one that is absent).
	choices := transactionPlateKinds(c)
	if len(choices) == 0 {
		// UNREACHABLE by construction: every candidate has strings or bytes.
		// Said out loud because returning silently here is exactly the bug
		// that made the tx: path inert, and a silent return looks like a
		// cancel to the operator.
		showError(ctx, th, "Engrave Transaction",
			"This transaction has neither mt1 strings to engrave as text nor "+
				"bytes to encode as QR. Re-pack the payload with `me sysw pack`.")
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
			plates, titles, note, err = planTransactionQRPlates(ctx.Platform, c)
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
				// SAY "DISCARD IT" (§4.4, G-P3.13). The old text said a re-run
				// starts at plate 1 and left the operator to infer the rest --
				// and the inference most people make is the wrong one: keep
				// the half-cut blank and carry on later. A re-run mints
				// byte-identical plates FROM PLATE 1, so a kept partial plate
				// becomes a second, WRONG copy of a plate in the same drawer,
				// and the device has no camera to tell them apart afterwards.
				showError(ctx, th, "Transaction Incomplete",
					fmt.Sprintf("Stopped at plate %d of %d. A partial set does not carry the "+
						"whole transaction.\n\nDISCARD the plate in the machine. It is "+
						"half cut and nothing will finish it.\n\nA re-run cuts the same "+
						"plates byte for byte, starting at plate 1 - so keeping this one "+
						"leaves you two plates numbered %d/%d that are not the same.",
						i+1, len(plates), i+1, len(plates)))
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
//
// IT TAKES THE CANDIDATE, NOT THE TRANSACTION, and that is the whole reason
// this signature changed. When `c.subst` is set the operator's legend is
// replaced un-overridably (ruling 2026-08-25b) — and so is OURS. Built from
// `mt.Tx` alone, this function could not know, so an UNSIGNED transaction
// would have been cut under "raw signed bitcoin tx … scan, then broadcast":
// a plate stating in steel that a transaction which can never be broadcast is
// signed and ready to. The review screen promised a substitution the plate did
// not make.
// eccName is the engraved/reported name of an error-correction level. ONE
// table: it appears on the plate legend, in the plan note and in R16's
// refusal, and three copies is three chances to disagree.
var eccName = map[qr.Level]string{qr.L: "L", qr.M: "M", qr.Q: "Q", qr.H: "H"}

func transactionLegend(c txCandidate, symbols int, ecc qr.Level) string {
	signedness := "raw signed bitcoin tx"
	var lines []string
	if c.subst != "" {
		// The warning goes FIRST, and the word "signed" and the instruction to
		// broadcast both go away. Everything else stays: the txid is still how
		// this plate is identified, and scan order is still a fact a recoverer
		// needs.
		lines = append(lines, c.subst)
		signedness = "raw bitcoin tx, NOT BROADCASTABLE"
	}
	lines = append(lines,
		"txid "+c.tx.TxidDisplay,
		fmt.Sprintf("%s, %d bytes, %d qr, ecc %s", signedness, len(c.tx.Raw), symbols, eccName[ecc]),
	)
	switch {
	case c.subst != "" && symbols > 1:
		lines = append(lines, "scan all qr, any order, to read it back")
	case c.subst != "":
		lines = append(lines, "scan to read it back")
	case symbols > 1:
		lines = append(lines, "scan all qr, any order, then broadcast")
	default:
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
func planTransactionQRPlates(pl Platform, c txCandidate) ([]Plate, []string, string, error) {
	params := pl.EngraverParams()
	type layout struct {
		symbols     int
		legendAlone bool
	}
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
					plates, titles, ok := buildQRPlates(params, c, lay.symbols, lay.legendAlone, ecc, scale)
					if ok {
						note := fmt.Sprintf("%d plate(s), %d QR, ECC %s, %s modules",
							plateCount, lay.symbols, eccName[ecc], moduleLabel(scale))
						return plates, titles, note, nil
					}
				}
			}
		}
	}
	// R16 / §4.1a. NAME THE MODULE SIZE AND THE CEILING AT IT. "too large"
	// with a byte count tells the operator nothing about how much too large,
	// and the answer is not a property of the transaction -- it is a property
	// of the SMALLEST MODULE this device will emit. Without the module size
	// the sentence cannot be acted on and cannot be checked.
	const floorScale = 2 // 0.6mm, the smallest module the search ever tries
	return nil, nil, "", fmt.Errorf("%d bytes is too large for QR plates.\n\n"+
		"At %s modules - the smallest this machine cuts - %d Structured Append "+
		"symbols at ECC %s hold at most %d bytes.\n\nUse TEXT plates.",
		len(c.tx.Raw), moduleLabel(floorScale), txqr.MaxSymbols, eccName[qr.M],
		qrCeilingBytes(params, qr.M, floorScale))
}

// moduleLabel is the engraved module pitch for a stroke multiplier. ONE table,
// used by the note and by the refusal, because two operators comparing a
// success note against a failure message must be reading the same units.
func moduleLabel(scale int) string {
	switch scale {
	case 3:
		return "0.9mm"
	case 2:
		return "0.6mm"
	}
	return fmt.Sprintf("%dx", scale)
}

// qrCeilingBytes is the largest transaction that COULD have fitted: 16
// Structured Append symbols at the given ECC and module, on this plate.
//
// MEASURED BY SEARCH, not written down. The answer depends on the plate
// geometry, the stroke width AND on which QR version txqr's encoder picks, so
// a constant here would be a number that goes stale the first time any of the
// three moves -- and it would go stale silently, in a refusal message nobody
// reads until the day it matters.
//
// Called only on the failure path, so its cost never lands on a working cut.
func qrCeilingBytes(params engrave.Params, ecc qr.Level, scale int) int {
	usable := params.F(85) - 2*params.I(3) - 2*params.I(2)
	fits := func(n int) bool {
		set, err := txqr.EncodeSet(make([]byte, n), txqr.MaxSymbols, ecc)
		if err != nil {
			return false
		}
		for _, c := range set {
			if c.Size*params.StrokeWidth*scale > usable {
				return false
			}
		}
		return true
	}
	// The search starts at MaxSymbols, not at 1, and that is not an
	// optimisation: EncodeSet REFUSES a payload it cannot split into k
	// non-empty parts, so `fits` is false at the bottom for a reason that has
	// nothing to do with the plate. Doubling up from 1 therefore never leaves
	// the ground and returns 0 -- measured, and it made the refusal state a
	// ceiling of zero bytes.
	lo := txqr.MaxSymbols
	if !fits(lo) {
		return 0
	}
	hi := lo * 2
	for hi < 1<<20 && fits(hi) {
		lo, hi = hi, hi*2
	}
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// buildQRPlates attempts one configuration; ok=false means it does not fit.
//
// A cheap geometry gate runs before any spline is planned: a symbol whose
// module grid plus quiet border cannot fit the usable width at this scale can
// never pass toPlate, and toPlate plans a full engraving to find that out.
func buildQRPlates(params engrave.Params, c txCandidate, symbols int, legendAlone bool, ecc qr.Level, scale int) ([]Plate, []string, bool) {
	tx := c.tx
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
	legend := transactionLegend(c, symbols, ecc)
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
