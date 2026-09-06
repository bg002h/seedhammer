package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/codex32"
	"seedhammer.com/hashlock"
	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// The Hashlock plates flow (H6 §5.2, §6.3): cut a preimage plate for a record
// the payload already carries, WITHOUT a composition.
//
// IT READS ctx.sysw AND NOTHING ELSE. It builds no composition, touches no
// composerState and holds nothing in one, which is what keeps §2.3's "the
// program holds nothing" true for this route. Decision 4 is the whole shape:
// this cuts "without a composition".
//
// A PHRASE RECORD DERIVES ON PICK AND THE RESULT LIVES FOR THE FLOW. The LIST
// draws without deriving -- three records would be a 30 s stall before a list
// could be drawn, and this flow meets MORE phrase records than the composer
// typically does, not fewer -- picking one derives ONCE behind the shipped
// `Deriving` countdown, and the result lives in a LOCAL of this flow so a
// redraw, a page or a Back never re-runs the KDF.

// hashlockPlatesRecord is one payload record this flow can cut, and everything
// the flow learns about it.
//
// preimage and phrase are SECRET. They live for the flow and are scrubbed on
// return; nothing here reaches composerState or survives the function.
type hashlockPlatesRecord struct {
	isPhrase bool
	// pos is the record's 1-based position among the records of its OWN class,
	// which is how §5.1's row labels number them and therefore how the operator
	// reads them off the screen.
	pos      int
	phrase   []byte
	method   hashlockMethod
	preimage [32]byte
	digest   [32]byte
	// derived is false for a phrase record nothing has picked yet. A preimage
	// record arrives derived: DecodeMS1Preimage gave X at list time.
	derived bool
}

// hashlockPlatesRecords lists the payload's preimage and phrase records, in
// payload order, WITHOUT deriving anything.
func hashlockPlatesRecords(s *syswSession) []hashlockPlatesRecord {
	if s == nil || !s.loaded {
		return nil
	}
	var out []hashlockPlatesRecord
	pre, ph := 0, 0
	for _, r := range s.records {
		switch r.class {
		case sysw.ClassPreimage:
			// TrimSpace, as sysw.Classify and composerPayloadPreimages do: the
			// door's count and this list must answer the same question the same
			// way (post-impl review I-1).
			c, err := codex32.New(strings.TrimSpace(r.body))
			if err != nil {
				continue
			}
			x, err := codex32.DecodeMS1Preimage(c)
			if err != nil {
				continue
			}
			pre++
			out = append(out, hashlockPlatesRecord{
				pos: pre, preimage: x, digest: hashlock.Digest(&x), derived: true,
			})
		case sysw.ClassPhrase:
			rec, err := sysw.ParsePhraseRecord(r.body)
			if err != nil {
				continue
			}
			ph++
			out = append(out, hashlockPlatesRecord{
				isPhrase: true, pos: ph,
				phrase: []byte(rec.Phrase), method: hashlockMethodOf(rec.Method),
			})
		}
	}
	return out
}

// hashlockPlatesRows is the list §5.2 draws, in the row forms §5.1 already
// defines -- so the two screens that name a payload record name it the same way.
func hashlockPlatesRows(recs []hashlockPlatesRecord) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		if !r.isPhrase {
			out = append(out, composerHashPreimageRow(r.pos, r.digest))
			continue
		}
		if r.derived {
			d := r.digest
			out = append(out, composerHashPhraseRow(r.pos, &d))
			continue
		}
		out = append(out, composerHashPhraseRow(r.pos, nil))
	}
	return out
}

// hashlockPlatesScrub wipes every phrase and zeroes every preimage this flow
// derived. Called from the flow's own defer, so a Back, a refusal, a ctx.Done
// unwind and a panic are covered by construction -- composerFlowExit's rule,
// applied to a flow that has no composerState to hang it on.
func hashlockPlatesScrub(recs []hashlockPlatesRecord) {
	for i := range recs {
		wipeBytes(recs[i].phrase)
		recs[i].phrase = nil
		recs[i].preimage = [32]byte{}
		recs[i].derived = false
	}
}

// hashlockPlatesDerive runs the KDF ONCE for a phrase record, behind the shipped
// countdown, and keeps the result in the flow's own slice.
//
// NEVER SILENTLY: a 10 s stall with no screen is a hang to the operator, which
// is what hashlockDeriveFlow's countdown exists to prevent. Returns false when
// the operator backed out of the derivation.
func hashlockPlatesDerive(ctx *Context, th *Colors, recs []hashlockPlatesRecord, i int) bool {
	if recs[i].derived {
		return true
	}
	x, ok := hashlockDeriveFlow(ctx, th, recs[i].phrase, recs[i].method)
	if !ok {
		return false
	}
	recs[i].preimage = x
	recs[i].digest = hashlock.Digest(&x)
	recs[i].derived = true
	return true
}

// hashlockPlatesMaterial is the record as §5.3's pick step reads it: a phrase
// record offers the phrase forms, a preimage record does not.
func hashlockPlatesMaterial(r hashlockPlatesRecord) hashlockMaterial {
	m := hashlockMaterial{preimage: r.preimage, method: r.method, provenance: hashlockFromPayload}
	if r.isPhrase {
		m.phrase = r.phrase
	}
	return m
}

// hashlockPlatesStub is §6.3's `mk1 stub` row for THIS flow, and it is the one
// locator row that may be absent.
//
// IT COMES FROM AN md1 RECORD IN THE SAME PAYLOAD, and there is no other
// source: no payload record carries an id, so a preimage or phrase record alone
// cannot name the policy it belongs to. When the payload holds no md1, the
// field is OMITTED rather than filled with a placeholder -- a wrong stub on a
// plate is worse than none, because it names a policy the plate does not
// unlock.
func hashlockPlatesStub(s *syswSession) (stub string, isPolicy bool) {
	if s == nil || !s.loaded {
		return "", false
	}
	var chunks []string
	for _, r := range s.records {
		if r.class == sysw.ClassMDMK && codex32.ValidMD(r.body) {
			chunks = append(chunks, r.body)
		}
	}
	if len(chunks) == 0 {
		return "", false
	}
	b, err := md.FormAwareStubChunks(chunks)
	if err != nil {
		return "", false
	}
	_, kind, err := md.FormAwareIdChunks(chunks)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%x", b), kind == md.WalletIdPolicy
}

// hashlockPlatesMatch is §6.3's `matches hash <i> in the payload` row: the
// 0-based index of the payload hash: record this digest equals, or -1.
func hashlockPlatesMatch(s *syswSession, digest [32]byte) int {
	for i, d := range composerPayloadDigests(s) {
		if d == digest {
			return i
		}
	}
	return -1
}

// hashlockPlatesLocator is §6.3's locator for a plate cut from a PAYLOAD record.
//
// THERE IS NO `path` ROW: this flow builds no composition, so no path exists to
// name. The `hash` row is printed from the DERIVED result and is therefore
// never blank -- a phrase-form plate with no locator at all is a bearer phrase
// in plain text with no digest, no path and no payload position, and it is the
// worst artifact this stage can cut.
func hashlockPlatesLocator(s *syswSession, r hashlockPlatesRecord) []string {
	stub, isPolicy := hashlockPlatesStub(s)
	return hashlockPlateLocator(0, r.digest, stub, isPolicy, hashlockPlatesMatch(s, r.digest))
}

// composerDoorHasPreimage is §5.2's door predicate: the route is offered only
// when the loaded payload holds at least one record it could cut.
//
// A DOOR ROW THAT NAMES A ROUTE IT CANNOT TAKE is the F-437 defect the door
// exists to remove, which is why this is a predicate and not an unconditional
// row. It takes the session rather than the Context for the reason
// composerDoorHasConsumablePolicy does: the door counts, it consumes nothing.
func composerDoorHasPreimage(s *syswSession) bool {
	if s == nil {
		return false
	}
	return s.has(sysw.ClassPreimage) || s.has(sysw.ClassPhrase)
}

// composerHashlockPlatesFlow is §5.2's route: list, pick, choose a form, cut.
func composerHashlockPlatesFlow(ctx *Context, th *Colors) {
	recs := hashlockPlatesRecords(ctx.sysw)
	defer hashlockPlatesScrub(recs)
	if len(recs) == 0 {
		// Unreachable from the door, whose predicate is the same question. It
		// refuses rather than drawing an empty picker, because a list with no
		// rows is a screen an operator cannot leave except by Back.
		showError(ctx, th, "Hashlock plates", composerCopyHashlockPlatesEmpty())
		return
	}
	for !ctx.Done {
		sel, ok := composerPickScreen(ctx, th, "Hashlock plates",
			composerCopyHashlockPlatesLead(len(recs)), hashlockPlatesRows(recs))
		if !ok {
			return
		}
		// DERIVE ON PICK, once. A Back during the countdown returns to the list
		// with nothing derived and nothing cut.
		if !hashlockPlatesDerive(ctx, th, recs, sel) {
			continue
		}
		r := recs[sel]
		m := hashlockPlatesMaterial(r)
		choice := hashlockPlatesFormPick(ctx, th, r, m)
		if choice == hashlockPlateDecline {
			continue
		}
		plate, err := composerHashlockPlateFor(ctx.Platform,
			hashlockPlate{digest: r.digest, material: m, choice: choice},
			hashlockPlatesLocator(ctx.sysw, r))
		if err != nil {
			showError(ctx, th, "Hashlock plates", composerCopyPreimagePlateRefusal())
			continue
		}
		if !NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
			// NEITHER §8.4 ARM, AND NOT BY OVERSIGHT. §8.4a says the phrase
			// "dies with this composition" and §8.4b speaks about a run that
			// also cuts policy plates; this flow builds no composition, and the
			// material it cuts stays in the payload, in flash. Telling the
			// operator a secret is gone when it is still in flash is false in
			// the dangerous direction. Reusing an arm is the obvious
			// implementation, which is why it is refused by name.
			showError(ctx, th, "Hashlock plates", composerCopyHashlockPlatesNotCut())
			continue
		}
	}
}

// hashlockPlatesFormPick is §5.3's pick step, for a record rather than a held
// digest: the same rows, the same masking, the same §8.5 warning.
func hashlockPlatesFormPick(ctx *Context, th *Colors, r hashlockPlatesRecord, m hashlockMaterial) hashlockPlateChoice {
	rows, choices := composerPreimagePlateRows(m)
	lead := composerCopyPreimagePlateLead(hashlockFirst8Last8(r.digest), 0, len(m.phrase), m.method.String())
	for !ctx.Done {
		sel, ok := composerPickScreen(ctx, th, "Preimage plate", lead, rows)
		if !ok {
			return hashlockPlateDecline
		}
		choice := choices[sel]
		if choice == hashlockPlatePhraseQR &&
			!composerConfirmScreen(ctx, th, "Preimage QR",
				composerConfirmBody(composerCopyPreimageQRWarning())) {
			continue
		}
		return choice
	}
	return hashlockPlateDecline
}
