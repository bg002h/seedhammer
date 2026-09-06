package gui

import (
	"encoding/hex"
	"fmt"
	"image"
	"strings"

	"seedhammer.com/codex32"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/hashlock"
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
// THE COMPOSER HOLDS A PREIMAGE FOR THE LIFE OF ONE COMPOSITION (H6 §2.2) AND
// CAN CUT IT ONTO A PLATE OF ITS OWN (§5.3, §6). This record used to read "THE
// COMPOSER DERIVES A PREIMAGE IN RAM FOR ONE SCREEN (H2) AND NEVER STORES,
// SHOWS OR ENGRAVES IT", which was H2's literal reading of ruling L7. H6 lifts
// three of L7's four verbs -- store, show, engrave -- and leaves the fourth
// refused: a preimage plate presented to a seed flow is still not a seed
// (H0; codex32.IsPreimage). The material lives in composerState.hashlockHeld
// (gui/composer_state.go:81) and is scrubbed by composerFlowExit; what goes in
// the SCRIPT is still only the digest.

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

// composerHashInPayloadRow is §5.1 Step 2's annotated form of the band-1 row.
//
// WHY A HASH ROW NEEDS AN ANNOTATION AT ALL. When a hash: record and a
// preimage record in the SAME payload carry the same digest, bands 1 and 2 draw
// identical digest text, adjacent, differing by one word -- and the
// consequences differ completely: band 1 assigns the digest and holds NO
// material, so no plate is offered at Done, while band 2 holds the preimage and
// offers a plate. The annotation is what makes the two rows readable as
// different choices rather than as a duplicate.
//
// `(in payload)` and NOT `(preimage in payload)`: measured at sh2DisplaySize,
// the longer wording is 50 characters and wraps to two lines in
// composerPageLines' band, which a picker row cannot spend
// (TestWhichHashRowsDrawOnOneLine).
func composerHashInPayloadRow(i int, digest [32]byte) string {
	return composerHashRow(i, digest) + "  (in payload)"
}

// composerHashPreimageRow is band 2: a preimage PLATE record in the payload,
// whose digest is computed directly from the record -- no KDF, no countdown.
func composerHashPreimageRow(i int, digest [32]byte) string {
	h := hex.EncodeToString(digest[:])
	return fmt.Sprintf("preimage %d  %s..%s", i, h[:8], h[56:])
}

// composerHashPhraseRow is band 3, in its two forms (§5.1 Step 1).
//
// d == nil is the row BEFORE derivation, and it says why it cannot show a
// digest rather than showing a blank or a placeholder: deriving to draw the
// list is the "three records would be a 30 s stall before a list could be
// drawn" §5.1 rejects. Once the record has been derived in this composition the
// row carries the digest, read back out of hashlockHeld.
func composerHashPhraseRow(i int, d *[32]byte) string {
	if d == nil {
		return fmt.Sprintf("phrase record %d (derive to see the digest)", i)
	}
	h := hex.EncodeToString(d[:])
	return fmt.Sprintf("phrase %d  %s..%s", i, h[:8], h[56:])
}

// hashlockPayloadPreimage is one ClassPreimage record of the loaded payload,
// decoded once at row-build time. Both fields are SECRET; the composition's
// flow-exit defer scrubs whatever reaches hashlockHeld, and nothing here
// outlives the row set.
type hashlockPayloadPreimage struct {
	preimage [32]byte
	digest   [32]byte
}

// composerPayloadPreimages returns every well-formed preimage PLATE record, in
// payload order.
//
// IT DECODES THROUGH codex32.DecodeMS1Preimage rather than trusting the class.
// Classification already ran isPreimagePlateRecord, so the decode cannot fail
// here; the arm exists so no value is consumed from a call that errored, which
// is composerPayloadDigests' own rule on the sibling band.
func composerPayloadPreimages(s *syswSession) []hashlockPayloadPreimage {
	if s == nil || !s.loaded {
		return nil
	}
	var out []hashlockPayloadPreimage
	for _, r := range s.records {
		if r.class != sysw.ClassPreimage {
			continue
		}
		c, err := codex32.New(strings.TrimSpace(r.body))
		if err != nil {
			continue
		}
		x, err := codex32.DecodeMS1Preimage(c)
		if err != nil {
			continue
		}
		out = append(out, hashlockPayloadPreimage{preimage: x, digest: hashlock.Digest(&x)})
	}
	return out
}

// composerPayloadPhrases returns every well-formed phrase: record, in payload
// order. NOTHING IS DERIVED HERE (§5.1 Step 3).
func composerPayloadPhrases(s *syswSession) []sysw.PhraseRecord {
	if s == nil || !s.loaded {
		return nil
	}
	var out []sysw.PhraseRecord
	for _, r := range s.records {
		if r.class != sysw.ClassPhrase {
			continue
		}
		rec, err := sysw.ParsePhraseRecord(r.body)
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// hashlockMethodOf maps the WIRE selector a phrase: record carries onto the
// screen's method. Two enumerations, deliberately: sysw owns the record grammar
// and gui owns the derivation, and a shared type would make the wire format
// depend on a screen.
func hashlockMethodOf(m sysw.HashlockMethod) hashlockMethod {
	if m == sysw.HashlockSHA256 {
		return hashlockSHA256
	}
	return hashlockHardened
}

// hashlockDerivedDigest reports the digest THIS COMPOSITION already derived for
// a phrase: record, or nil.
//
// IT READS hashlockHeld RATHER THAN A SECOND MAP. Task 8a's map is keyed by
// digest and carries the phrase, the method and the provenance, so "has this
// record been derived here" is answerable from it exactly -- and a second,
// index-keyed map would be a second answer to the same question, which is the
// drift composerHashRowSet's own label-keying exists to prevent. It also
// survives a Back out to `Which hash?` and back in, so the KDF runs once per
// record per composition.
func hashlockDerivedDigest(st *composerState, rec sysw.PhraseRecord) *[32]byte {
	if st == nil {
		return nil
	}
	want := hashlockMethodOf(rec.Method)
	for h, m := range st.hashlockHeld {
		if m.provenance != hashlockFromPayload || m.method != want {
			continue
		}
		if string(m.phrase) != rec.Phrase {
			continue
		}
		d := h
		return &d
	}
	return nil
}

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
	preimages []hashlockPayloadPreimage
	phrases   []sysw.PhraseRecord
	// The first row index of each band. A band with no records still records
	// its start, which equals the next band's start -- so `sel >= start &&
	// sel < start+len(band)` is empty for it and no arm can be entered by an
	// index that belongs to another band.
	preimageRow  int
	phraseRecRow int
	phraseRow    int
	hexRow       int
	noneRow      int
}

// composerHashRows builds §5.1's SIX bands, in order, and records where each
// begins.
//
// IT TAKES THE COMPOSITION STATE because band 3's row form depends on what this
// composition has already derived (§5.1 Step 1). st may be nil, which is the
// row set as a fresh composition first draws it.
func composerHashRows(s *syswSession, st *composerState) composerHashRowSet {
	digests := composerPayloadDigests(s)
	preimages := composerPayloadPreimages(s)
	phrases := composerPayloadPhrases(s)
	r := composerHashRowSet{
		digests: digests, preimages: preimages, phrases: phrases, lead: "Which hash?",
	}
	labels := make([]string, 0, len(digests)+len(preimages)+len(phrases)+3)
	// Band 1: the payload's hash: digests, annotated when the SAME payload also
	// carries the material for one (§5.1 Step 2).
	for i, d := range digests {
		if composerHashInPayload(r, st, d) {
			labels = append(labels, composerHashInPayloadRow(i+1, d))
			continue
		}
		labels = append(labels, composerHashRow(i+1, d))
	}
	// Band 2: the payload's preimage plate records.
	r.preimageRow = len(labels)
	for i, p := range preimages {
		labels = append(labels, composerHashPreimageRow(i+1, p.digest))
	}
	// Band 3: the payload's phrase: records, UNDERIVED unless this composition
	// has already derived them.
	r.phraseRecRow = len(labels)
	for i, rec := range phrases {
		labels = append(labels, composerHashPhraseRow(i+1, hashlockDerivedDigest(st, rec)))
	}
	r.phraseRow = len(labels)
	labels = append(labels, composerHashRowPhrase)
	r.hexRow = len(labels)
	labels = append(labels, "Type 64 hex")
	r.noneRow = len(labels)
	labels = append(labels, "No hash lock")
	r.labels = labels
	if len(digests) == 0 && len(preimages) == 0 && len(phrases) == 0 {
		r.lead = composerCopyHashlockNoPayloadLead()
	}
	return r
}

// composerHashInPayload reports whether the SAME payload carries the material
// for this digest -- a preimage record with that digest, or a phrase: record
// this composition has already derived to it.
//
// AN UNDERIVED phrase: RECORD CANNOT BE COUNTED HERE, and that is a property of
// §5.1 rather than an omission: its digest is not knowable without running the
// KDF, which is exactly what Step 3 forbids at row-build time. Its hash: row
// gains the annotation the moment the record is derived, because the row set is
// rebuilt on every pass of composerHashEdit's loop.
func composerHashInPayload(r composerHashRowSet, st *composerState, d [32]byte) bool {
	for _, p := range r.preimages {
		if p.digest == d {
			return true
		}
	}
	for _, rec := range r.phrases {
		if h := hashlockDerivedDigest(st, rec); h != nil && *h == d {
			return true
		}
	}
	return false
}

// composerHashEdit sets or clears one path's hashlock.
//
// NO PROVENANCE BOOKKEEPING LIVES HERE, and its absence is H5 §2's fix rather
// than an omission. composerHashByPhraseSync used to run in the noneRow arm and
// in composerPathEdit's Remove arm to keep a composition-wide bool honest; with
// provenance held per digest (composerAnyPathByPhrase) every arm below simply
// writes p.Hash, and the predicate reads it.
func composerHashEdit(ctx *Context, th *Colors, st *composerState, idx int) bool {
	title := fmt.Sprintf("Path %d hash", idx+1)
	for {
		rows := composerHashRows(ctx.sysw, st)
		sel, ok := composerPickScreen(ctx, th, title, rows.lead, rows.labels)
		if !ok {
			return false // Back at `Which hash?` -- the ONLY false this function returns (spec §4.6)
		}
		// The §8i rule fires when the operator is TAKING a hash, which is every
		// row of the six bands EXCEPT `No hash lock` (H6 §5.1 Step 4).
		//
		// STATED AS THE ONE ROW IT IS NOT, rather than as a disjunction over the
		// rows it is. The shipped predicate enumerated the taking rows
		// (`sel < len(rows.digests) || sel == rows.phraseRow || sel == rows.hexRow`),
		// and adding a band to a screen would then silently leave the 32-byte
		// rule unstated for it -- the failure being that the operator takes a
		// hash without ever being told what a preimage must be. A predicate
		// keyed on the single clearing row cannot go stale that way.
		taking := sel != rows.noneRow
		if taking {
			showError(ctx, th, title, composerCopyHashRule())
		}
		switch {
		case sel < len(rows.digests):
			d := rows.digests[sel]
			st.list.Paths[idx].Hash = &d
			return true
		case sel >= rows.preimageRow && sel < rows.preimageRow+len(rows.preimages):
			// §5.1: a preimage RECORD takes the phrase route's shape without
			// the KDF -- DecodeMS1Preimage gave X at row-build time, so there
			// is no countdown and no phrase.
			switch hashlockPreimageRecordRoute(ctx, th, st, idx, rows.preimages[sel-rows.preimageRow], rows.digests) {
			case hashlockAssigned:
				return true
			case hashlockBackToWhichHash:
				continue
			}
		case sel >= rows.phraseRecRow && sel < rows.phraseRecRow+len(rows.phrases):
			// DERIVATION IS LAZY and the payload path is a DIFFERENT FUNCTION
			// (§5.1 Step 3): hashlockPhraseRoute picks a method and ends on the
			// reconciliation screen, and a payload phrase must do neither.
			switch hashlockPayloadRoute(ctx, th, st, idx, rows.phrases[sel-rows.phraseRecRow], rows.digests) {
			case hashlockAssigned:
				return true
			case hashlockBackToWhichHash:
				continue
			}
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
			return true
		default:
			panic(fmt.Sprintf("composerHashEdit: pick returned row %d of %d", sel, len(rows.labels)))
		}
	}
}
