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
	// dupIdx lists the chunk indices, 0-BASED and ascending, at which the set
	// held more than one distinct string. `strs` holds the LAST of each
	// (operator ruling 2026-08-26: last wins) -- these are the slots where the
	// others were dropped, and the review screen names them so the operator
	// can re-pack in another order and cut a different copy.
	//
	// It is carried on the candidate rather than recomputed at the screen
	// because `strs` is already deduped by the time a screen sees it: the
	// evidence does not survive orderByIndex.
	dupIdx []int
	// dupTotal is the set's DECLARED chunk count, from the header. Never
	// len(strs), which is the deduped number.
	dupTotal int
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
		choices = append(choices, transactionChoiceRow(c))
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

// transactionChoiceRow is one row of the picker.
//
// IT PANICKED. Every row read `c.tx.TxidDisplay[:8]`, and an UNCONFIRMED
// candidate carries the ZERO-VALUE mt.Tx -- the set never reassembled, so
// there is no txid to slice:
//
//	panic: runtime error: slice bounds out of range [:8] with length 0
//
// The rows are built for ALL candidates before `len(choices) > 1` decides
// whether to show the screen, so a payload holding ONE incomplete mt1 set
// crashed the program with no picker ever displayed -- and that payload is the
// ordinary one ruling 2026-08-25 exists for: an operator who packed the
// strings they had.
//
// So the row is derived from what the candidate ACTUALLY has, and the three
// shapes are labelled apart. A picker where a spendable transaction and one
// that can never be broadcast look alike is the R10 hazard in miniature.
func transactionChoiceRow(c txCandidate) string {
	what := "raw bytes"
	if len(c.strs) > 0 {
		what = fmt.Sprintf("%d strings", len(c.strs))
	}
	if !c.hasBytes() {
		// No txid exists. The set id is the only identifier there is, and
		// saying "TX" over an empty string is how this crashed.
		return fmt.Sprintf("SET %05X | UNCONFIRMED | %s", c.csid, what)
	}
	label := "TX"
	if !c.confirmed {
		label = "UNSIGNED"
	}
	return fmt.Sprintf("%s %s | %d B | %s", label,
		strings.ToUpper(c.tx.TxidDisplay[:8]), len(c.tx.Raw), what)
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
		// BEFORE the dedup is consumed: orderByIndex keeps the last string per
		// index and the rest are gone, so which indices collided is only
		// knowable from `set`.
		dupIdx, dupTotal := duplicateChunkIndices(set)
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
				dupIdx: dupIdx, dupTotal: dupTotal,
			})
			continue
		}
		cands = append(cands, txCandidate{
			tx: tx, strs: ordered, src: srcPayload, csid: csid,
			confirmed: true, dupIdx: dupIdx, dupTotal: dupTotal,
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

// duplicateChunkIndices names every chunk index for which the set holds two
// strings that are NOT the same string -- the indices orderByIndex thins,
// keeping the last -- together with the set's DECLARED chunk count.
//
// IT EXISTS BECAUSE THE THINNING USED TO BE SILENT (P5 I-2). A set can hold two
// different strings for one index without any collision luck: sign the same
// PSBT twice and the second ceremony differs only in witness bytes, which the
// txid ignores, so the chunk_set_id, the count and the lengths are all
// identical. mt.Decode refuses such a set and the legend is substituted -- but
// the review screen then reported the DEDUPED count and nothing named the
// strings that would never be cut.
//
// OPERATOR RULING 2026-08-26: "last wins is fine but message to user that this
// is the rule is required so they can try again in different order." So the
// dedup is unchanged and this is what the message is built from.
//
// EQUALITY IS CASE-BLIND, deliberately. mt.Decode compares PAYLOAD BYTES and
// tolerates an exact duplicate ("a well-kept drawer, not an error"), and a
// codex32 string is consistent-case, so an all-uppercase copy of a chunk
// carries the identical payload. Reporting that would send the operator away
// to re-pack a payload that lost nothing.
//
// total is the DECLARED count, from the first parseable header -- the same
// field mt.Decode reads the count from, and pointedly NOT len(set) or
// len(orderByIndex(set)). The deduped length is the number the screen was
// already showing while the drop went unnamed, so using it here would put the
// defect inside its own warning.
//
// B-3 (fold review 2026-08-26): this comment used to claim the named number can
// never exceed the total, on the grounds that mt.ParseHeader refuses an
// out-of-range index. THAT CLAIM WAS FALSE and is deleted rather than softened.
// ParseHeader bounds ChunkIndex against THAT STRING'S OWN TotalChunks
// (mt/mt.go: `h.ChunkIndex >= h.TotalChunks`) and nothing bounds it against the
// first string's. Mixed declared counts inside one csid group are a case the
// rest of the code explicitly ANTICIPATES -- setIsComplete rejects them just
// above, and mt.Decode returns a count mismatch -- so the assumption was
// contradicted two functions away.
//
// The consequence is cosmetic and needs a spliced set: >=3 strings and >=2
// distinct declared counts, first-parsed smallest. It renders as "Duplicate
// string 5 of 2 found" -- a number that cannot exist, weird enough that nobody
// would act on it. Left as-is deliberately; the FALSE COMMENT was the hazard,
// because a future reader would have believed the invariant and relied on it.
func duplicateChunkIndices(set []string) (idx []int, total int) {
	distinct := map[int][]string{}
	for _, s := range set {
		h, err := mt.ParseHeader(s)
		if err != nil {
			continue // not engraved in place either; orderByIndex skips it too
		}
		if total == 0 {
			total = h.TotalChunks
		}
		seen := false
		for _, have := range distinct[h.ChunkIndex] {
			if strings.EqualFold(have, s) {
				seen = true
				break
			}
		}
		if !seen {
			distinct[h.ChunkIndex] = append(distinct[h.ChunkIndex], s)
		}
	}
	for i, ss := range distinct {
		if len(ss) > 1 {
			idx = append(idx, i)
		}
	}
	sort.Ints(idx) // map order is random; a warning that reshuffles is not one
	return idx, total
}

// namedChunks is "string 4 of 6" / "strings 4 and 5 of 6" /
// "strings 1, 4 and 5 of 6", for an operator. Mirrors namedInputs.
//
// IT COUNTS FROM ONE, and the shape is the HOST's, not a new one. me-cli's
// describe_set_problem prints "MISSING string(s) 4 and 5 of 6" over the same
// grouping, under a comment that settles the question outright: "Chunk numbers
// are 1-BASED here and everywhere an operator reads them ... (SPEC_mt SS1.1:
// the wire index is 0-based and appears in no message). Printing the wire index
// would send someone counting the strings on their desk and finding the wrong
// one." The gather agrees ("String %d of %d." at h.ChunkIndex+1), and so does
// mt.Decode's own "missing chunk %d of %d".
//
// "string" AND NOT "plate", though the ruling said plate: a TEXT plate carries
// AS MANY STRINGS AS FIT (planTransactionTextPlates packs six onto one), so
// plate numbering is a different, coarser count and "plate 4 of 6" would send
// the operator to a plate that does not exist. The strings are engraved in
// index order, so string 4 is the fourth one reading down the set.
func namedChunks(idx []int, total int) string {
	n := make([]string, 0, len(idx))
	for _, i := range idx {
		n = append(n, fmt.Sprintf("%d", i+1))
	}
	switch len(n) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("string %s of %d", n[0], total)
	default:
		return fmt.Sprintf("strings %s and %s of %d",
			strings.Join(n[:len(n)-1], ", "), n[len(n)-1], total)
	}
}

// transactionDuplicateLines is the message operator ruling 2026-08-26
// requires: the rule stated, every colliding index named, and the one action
// that changes the outcome.
//
// IT NAMES INDICES AND NEVER STRINGS. An mt1 string is a chunk of a signed
// transaction -- bearer material -- so which slot collided is the message and
// the string body is not.
func transactionDuplicateLines(idx []int, total int, src syswSource) []string {
	if len(idx) == 0 {
		return nil
	}
	// THE REMEDY MUST BE ONE THIS OPERATOR HAS (fold review B-1). The first
	// version said "re-pack the payload" unconditionally -- advice for a
	// channel an NFC operator does not have. They scanned these strings; there
	// is no payload of theirs to re-pack, and the action that changes the
	// outcome is to scan the copy they want LAST.
	remedy := []string{
		"engraved. Re-pack the payload",
		"in a different order to cut a",
		"different copy.",
	}
	if src == srcNFC {
		remedy = []string{
			"engraved. Scan the copy you",
			"want kept LAST, then retry.",
		}
	}
	return append([]string{
		"DUPLICATE STRINGS - LAST WINS",
		"",
		fmt.Sprintf("Duplicate %s found, last wins.", namedChunks(idx, total)),
		"",
		"The earlier copies are NOT",
	}, remedy...)
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
		dupIdx, dupTotal := duplicateChunkIndices(set)
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
				dupIdx: dupIdx, dupTotal: dupTotal,
			}, true
		}
		return txCandidate{tx: tx, strs: orderByIndex(set), src: srcNFC, csid: h.ChunkSetID,
			confirmed: true, dupIdx: dupIdx, dupTotal: dupTotal}, true
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
		lines := []string{
			"UNCONFIRMED SET", "",
			fmt.Sprintf("Set %05x, %d string(s).", c.csid, len(c.strs)),
			"",
		}
		// RULING 2026-08-26, and it goes HERE -- third block, above the
		// legend -- for the reason G-P3.20 found for the bearer warning:
		// confirmReviewScreen PAGES, and on the shipped 480x320 panel page one
		// of this screen holds about seven lines. A warning below the legend
		// block is a warning the operator can press Continue past without ever
		// seeing, and this one is the only place the DEDUPED count above it is
		// explained. `%d string(s)` counts what will be cut; these lines say
		// what will not.
		lines = append(lines, transactionDuplicateLines(c.dupIdx, c.dupTotal, c.src)...)
		if len(c.dupIdx) > 0 {
			lines = append(lines, "")
		}
		return append(lines,
			// RULED 2026-08-26 (operator): say what was OBSERVED, not why.
			// The previous wording asserted a cause, and for an UNSIGNED set
			// both halves were false -- it DOES reassemble and it DOES decode,
			// which contradicted this file's own comment at substitutionFor.
			// "did not confirm" is exactly the condition tested (!confirmed)
			// and is true for all three causes: incomplete, undecodable, and
			// unsigned. The closing caveat is the operator's: this device does
			// not parse every transaction, so the operator must know their own.
			"This did not confirm as a",
			"transaction on this device.",
			"The strings are engraveable",
			"and each is valid.",
			// The earlier ruling here narrowed this to "incomplete/does not
			// decode", which fixed the two-run payload case and left the
			// UNSIGNED one still wrong. Naming no cause at all covers both,
			// and the legend below still distinguishes them.
			"",
			"This device does not parse",
			"every transaction. Know what",
			"you are engraving.",
			"",
			"The plate legend WILL be",
			"replaced with:",
			c.subst,
			"",
			"QR plates are unavailable:",
			"there are no transaction bytes.",
		)
	}
	// THE BEARER WARNING IS ABOVE THE TXID, and the order is the finding.
	// confirmReviewScreen PAGES: with the warning last, page 1 held the
	// question and the four txid lines and nothing else, so an operator who
	// pressed Continue -- the ordinary thing to do on a screen showing the
	// number they came to check -- confirmed the cut WITHOUT EVER SEEING IT.
	// Found by walking the flow (G-P3.20); no assertion on the wording could
	// see it, because the words were there.
	lines := []string{
		"Engrave this transaction?",
		"",
		"BEARER: anyone holding the",
		"plates can broadcast it.",
		"",
	}
	lines = append(lines, chunkString(c.tx.TxidDisplay, 16)...)
	lines = append(lines, "",
		fmt.Sprintf("%d bytes, %d in, %d out", len(c.tx.Raw), c.tx.Inputs, c.tx.Outputs),
		"Source: "+syswSourceName(c.src))
	// ON A CONFIRMED CANDIDATE THIS BLOCK IS ALL BUT UNREACHABLE, and it is
	// emitted anyway rather than argued away. mt.Decode compares payload
	// BYTES, so a set it confirmed cannot hold two indices whose payloads
	// disagree; the only strings that are distinct here and identical there
	// differ in the FINAL chunk's padding bits, which payloadBytes truncates
	// (mt.go: the primary truncates rather than rejecting), so it takes a
	// forged string to reach. Nothing of value is dropped in that case, which
	// is why the block goes LAST here and third from the top on the
	// unconfirmed screen: page one of the confirmed review is the bearer
	// warning's, ruled by G-P3.20, and a near-unreachable notice must not
	// displace it.
	return append(lines, transactionDuplicateLines(c.dupIdx, c.dupTotal, c.src)...)
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
		// many, at what protection, AND HOW LONG. This comment used to claim
		// plate count and ECC were "the two numbers the operator budgeted
		// blanks and time by", while no time appeared anywhere: at ~21 minutes
		// a plate, a four-plate job is most of an afternoon, and the screen
		// that asks for the commitment is the only place to say so. Stopping
		// mid-set costs a blank (G-P3.13), so the estimate is not a nicety.
		lines := []string{note, transactionJobTime(plates, ctx.Platform.EngraverParams().TicksPerSecond), "", "Engrave?"}
		if !confirmReviewScreen(ctx, th, "Plates", lines) {
			continue
		}
		if engraveTransactionPlates(ctx, th, plates, titles) {
			// G-P3.17(b) — the per-JOB instruction, once, after the last plate.
			transactionPostCutFlow(ctx, th, c, choices[i] == "QR PLATES", len(plates))
			return
		}
	}
}

// transactionJobTime is how long the whole job takes, from the PLAN rather
// than from a rule of thumb: Plate.Duration is the tick count the engraver will
// actually spend, and TicksPerSecond is the same divisor the live remaining-time
// readout uses. Two clocks for one machine would disagree in front of the
// operator.
func transactionJobTime(plates []Plate, ticksPerSecond uint) string {
	tps := uint64(ticksPerSecond)
	if tps == 0 {
		// A zero-tick machine is a programming error, not an input. Say
		// nothing rather than divide by zero on a confirm screen.
		return "cut time unknown"
	}
	var ticks uint64
	for _, p := range plates {
		ticks += p.Duration
	}
	sec := (ticks + tps - 1) / tps
	switch {
	case sec < 60:
		return fmt.Sprintf("about %d s of cutting", sec)
	case sec < 3600:
		return fmt.Sprintf("about %d min of cutting", (sec+59)/60)
	default:
		return fmt.Sprintf("about %d h %d min of cutting", sec/3600, (sec%3600+59)/60)
	}
}

// transactionPostCutFlow is the PER-JOB instruction (§4.3a, G-P3.17b): shown
// ONCE, after the last plate, naming what to do with the set as a whole.
//
// IT IS THE ONLY PLACE THE DEVICE CAN SAY "TEST THE PLATE", and it has to,
// because it never tests one itself: the SH2 has no camera, so nothing on this
// machine can ever read a plate back. If the operator does not check the
// artifact now, nobody checks it until the day it is needed.
//
// It names ONE command per plate kind, and names it once -- the per-plate
// legends carry the scan-order fact, so repeating it here would be the
// job-level instruction the gate exists to unpick.
func transactionPostCutFlow(ctx *Context, th *Colors, c txCandidate, qr bool, plates int) {
	confirmReviewScreen(ctx, th, "Plates Cut", transactionPostCutLines(c, qr, plates))
}

// transactionPostCutLines is the screen's text, as LINES.
//
// PAGED, NOT A MODAL, and that is not a style choice. The first draft used
// showNotice, and ErrorScreen does not page: the body was cut off mid-sentence
// at "Order does not matter - it is inside", so the operator never reached the
// two lines that matter most -- what to check the txid against, and that this
// machine can never read a plate back. Three assertions on the wording passed
// while the words were unreachable, which is F-151's shape one step along:
// there, text was submitted and not drawn; here, drawn and not shown.
func transactionPostCutLines(c txCandidate, qr bool, plates int) []string {
	lines := []string{
		fmt.Sprintf("%d plate(s) cut.", plates),
		"",
		"TEST THEM NOW, before you",
		"file them.",
		"",
	}
	if qr {
		lines = append(lines,
			"Scan every QR with a phone,",
			"join the hex, and run",
			"`mt inspect` on it.")
	} else {
		lines = append(lines,
			"Type one string into",
			"`mt verify`, then `mt decode`",
			"the whole set.")
	}
	lines = append(lines, "",
		"Order does not matter.",
		"")
	// B-2 (fold review 2026-08-26): discriminate by CAUSE, not by channel.
	// This read `c.subst != "" && len(c.strs) == 0` — "arrived as a payload
	// rather than as loose strings" — so an unsigned SET, which has strings,
	// fell through to the branch predicting failure. Both paths already set
	// legendUnsigned for this cause (the set path via substitutionFor, the
	// payload path directly). It is NOT enough on its own: a `tx:` record carries
	// BYTES and will decode whatever legend was substituted, which the legend
	// test alone would have broken -- caught by
	// TestPostCutDoesNotPredictFailureForARecordThatWillDecode, not by reasoning.
	//
	// So the question is "will this decode there", and it has two independent
	// yeses:
	//   bytes present            -- a tx: record, any subst
	//   legendUnsigned           -- an unsigned SET, whose bytes Go's decoder
	//                               discards (F-262) but which DID decode
	// and one no: an unconfirmed set with neither, where the decode genuinely
	// fails and the original prediction is correct.
	// c.subst != "" keeps CONFIRMED candidates out: they have Raw too, and they
	// belong in the else branch that names their txid. Dropping it swallowed them
	// -- caught by TestThePostCutScreenNamesOneCommandAndSaysOrderDoesNotMatter.
	if c.subst != "" && (len(c.tx.Raw) > 0 || c.subst == legendUnsigned) {
		// P5 M-4 — a tx: record admitted with --allow-unsigned-inputs carries a
		// substituted legend AND engraves QR plates, but its bytes DO decode:
		// `mt inspect` on the scan-back succeeds and prints the transaction.
		// Telling the operator to "expect it to fail there too" predicts an
		// outcome that will not happen, and an instruction that mispredicts is
		// one they stop trusting. What did not hold here is the SIGNATURE
		// check, not the decode.
		lines = append(lines,
			"This WILL decode there.",
			"What did not hold here is",
			"that every input is signed.",
			"The plate legend says so",
			"permanently.")
	} else if c.subst != "" {
		lines = append(lines,
			"This set did NOT confirm",
			"here, so expect it to fail",
			"there too. The plate legend",
			"says so permanently.")
	} else {
		lines = append(lines,
			"Check the txid it reports",
			"against TX "+strings.ToUpper(c.tx.TxidDisplay[:8])+".")
	}
	return append(lines, "",
		"This machine has no camera",
		"and can never read a plate",
		"back.")
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
		plan, err := backup.EngraveText(params, plate)
		if err != nil {
			return Plate{}, err
		}
		return toPlate(plan, params)
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

// plateHasQR distinguishes the two plates a legend can land on. §4.3a: THE
// PER-PLATE INSTRUCTION IS A FUNCTION OF WHAT IS ON THAT PLATE. A legend-only
// plate carries no symbol at all, and it was telling the operator to "scan all
// qr, any order" -- an instruction about plates that are not this one, cut into
// the one plate that has nothing to scan.
func transactionLegend(c txCandidate, symbols int, ecc qr.Level, plateHasQR bool) string {
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
	// What to DO with it, said about the plate it is cut into.
	verb := "then broadcast"
	if c.subst != "" {
		// Nothing here is broadcastable, and the plate must not say it is.
		verb = "to read it back"
	}
	switch {
	case !plateHasQR:
		// The legend plate. The symbols are elsewhere, and saying so is the
		// whole content of this plate's instruction.
		lines = append(lines, fmt.Sprintf("the %d qr are on the other plates", symbols),
			"scan them all, any order, "+verb)
	case symbols > 1:
		lines = append(lines, "scan all qr, any order, "+verb)
	default:
		lines = append(lines, "scan this qr, "+verb)
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
	// P5 N-2 — the remedy must be one this candidate HAS. A tx:-record-only
	// candidate carries no mt1 strings, so "Use TEXT plates" is impossible for
	// it; the operator's route is to re-pack the transaction as an mt1 set.
	// P5 N-3 — the ceiling below is a MODULE-FIT bound. `buildQRPlates`
	// additionally requires the legend/title assembly to fit, so the true
	// acceptance threshold is at or below this number. Said plainly rather
	// than quoted as exact, because a refusal that overstates its own ceiling
	// sends the operator to shave bytes that will not be enough.
	remedy := "Use TEXT plates."
	if len(c.strs) == 0 {
		remedy = "This arrived as a tx: record, so there are no TEXT plates " +
			"to fall back to. Re-pack the transaction as an mt1 set."
	}
	return nil, nil, "", fmt.Errorf("%d bytes is too large for QR plates.\n\n"+
		"At %s modules - the smallest this machine cuts - %d Structured Append "+
		"symbols at ECC %s hold at most about %d bytes (module fit; the legend "+
		"and title take a little more).\n\n%s",
		len(c.tx.Raw), moduleLabel(floorScale), txqr.MaxSymbols, eccName[qr.M],
		qrCeilingBytes(params, qr.M, floorScale), remedy)
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
		plan, err := backup.EngraveText(params, txt)
		if err != nil {
			return false
		}
		p, err := toPlate(plan, params)
		if err != nil {
			return false
		}
		plates = append(plates, p)
		titles = append(titles, title)
		return true
	}
	plateNo := 1
	if legendAlone {
		if !build(backup.Text{Paragraphs: []backup.Paragraph{
			{Text: transactionLegend(c, symbols, ecc, false)},
		}}, transactionPlateTitle(tx, plateNo, plateCount)) {
			return nil, nil, false
		}
		plateNo++
	}
	for i, sym := range set {
		paras := []backup.Paragraph{}
		if i == 0 && !legendAlone {
			paras = append(paras, backup.Paragraph{
				Text: transactionLegend(c, symbols, ecc, true), QR: sym, QRScale: scale,
			})
		} else {
			paras = append(paras, backup.Paragraph{QR: sym, QRScale: scale})
		}
		if !build(backup.Text{Paragraphs: paras},
			transactionPlateTitle(tx, plateNo, plateCount)) {
			return nil, nil, false
		}
		plateNo++
	}
	return plates, titles, true
}
