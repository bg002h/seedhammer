package gui

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"seedhammer.com/hashlock"
	"strings"

	"seedhammer.com/codex32"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
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
//
// THE ELISION IS hashlockFirst8Last8's, not a second copy of it: this used to
// slice h[:8] and h[56:], which is the same string for a 64-hex sha256 digest
// and reads twelve bytes of alloc-gate padding for a 40-hex one.
func composerHashRow(i int, digest *md.HashLock) string {
	// THE KIND REPLACES THE WORD `hash` RATHER THAN JOINING IT, and that is a
	// measurement, not a preference (SPEC_hashlock_kinds §7.5: the row form is
	// re-measured AS A WHOLE, and character count is not the constraint on a
	// proportional face). `hash 10  ripemd160 <elision>  (in payload)` WRAPS TO
	// TWO LINES at the shipped band -- so does the hash256 form -- while
	// `ripemd160 10  <elision>  (in payload)` draws at 383 px in a 411 px band,
	// 28 px clear. The alternative §7.5 sanctions, an abbreviated DISPLAY token
	// (`rmd160`), was declined: it would put a second vocabulary on the device
	// for the one axis this whole cycle exists to make unambiguous.
	//
	// The band's lead is `Which hash?`, so a row that answers with the name of
	// a hash function reads as an answer. The sibling bands keep their own
	// nouns (`preimage`, `phrase`) because those name the record the digest
	// CAME FROM, which is a different question and one the kind does not answer.
	return fmt.Sprintf("%s %d  %s", digest.Kind().Token(), i, hashlockFirst8Last8(digest))
}

// composerPayloadDigests returns every well-formed hash: record, in payload
// order. A malformed one is ClassUnknown and INERT under the shipped contract
// (sysw/descriptor.go:46-48): it reaches no screen, and its only device-side
// signal is the door's not-understood count (§6a).
func composerPayloadDigests(s *syswSession) []*md.HashLock {
	if s == nil || !s.loaded {
		return nil
	}
	var out []*md.HashLock
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

// composerWarnTwentyByteUnseen is SPEC_hashlock_kinds §8's gate: it draws the
// 20-byte warning and reports whether the operator accepted it.
//
// SILENT FOR EVERYTHING §8 SAYS IS SILENT -- the 32-byte kinds, and any digest
// whose preimage this composition holds (composerState.hashlockHeld, which only
// the deriving routes populate). So the phrase route, the payload preimage-plate
// route and the payload phrase-record route all pass through untouched, because
// by the time they assign, the material is held.
//
// IT IS A CONFIRM, NOT A NOTICE. §8 calls it a warning and the operator is about
// to commit a path to a digest nothing here can check; a modal they cannot
// decline would be a notice dressed as a gate. Declining assigns nothing.
func composerWarnTwentyByteUnseen(ctx *Context, th *Colors, st *composerState, h *md.HashLock) bool {
	if h == nil || h.Kind().DigestLen() != 20 {
		return true
	}
	if _, held := st.hashlockHeld[h.MapKey()]; held {
		return true
	}
	// THE PAYLOAD COUNTS AS HAVING SEEN IT (F-572). The warning asserts
	// "nothing on this device has seen a preimage for it", and `hashlockHeld`
	// alone made that a statement about the COMPOSER'S BOOKKEEPING dressed as a
	// statement about the payload: a payload carrying both a `hash160:` record
	// and the preimage plate for it drew the warning while
	// composerPayloadPreimages had already decoded that very preimage to build
	// the screen behind the modal.
	//
	// A warning that is demonstrably false in a payload the host built
	// correctly teaches operators to hold through warnings -- which is the
	// exact hazard this copy's own comment names as the reason not to overstate.
	//
	// IT RE-DERIVES AT THIS LOCK'S KIND rather than comparing the band's stored
	// digest. A preimage PLATE record carries X and no kind, so
	// composerPayloadPreimages builds its digest at sha256 -- the only one that
	// record can claim. Comparing those locks against a hash160 lock would
	// never match, and the suppressor would be dead code that looked like a fix.
	//
	// The question the warning actually asks is "has this device seen a
	// preimage FOR THIS DIGEST", so that is what is computed: X hashed at h's
	// kind, compared to h's bytes.
	for _, p := range composerPayloadPreimages(ctx.sysw) {
		if bytes.Equal(hashlock.DigestOf(h.Kind(), &p.preimage), h.Digest()) {
			return true
		}
	}
	return composerConfirmScreen(ctx, th, "Hash lock",
		composerConfirmBody(composerCopyTwentyByteUnseen(h.Kind())))
}

// composerHashKinds is the kind screen's row order.
//
// THE DEFAULT IS THE CALLER'S SEED, NOT ROW 0 -- corrected after a mutation run
// showed this comment's first version was wrong about its own code.
// composerHashKindPick opens on whichever row holds `initial`, so reordering
// this list does NOT change what an operator who presses straight through gets;
// composerHashEdit's `kind := md.KindSha256` is what does. Row order is a
// reading-order choice, and sha256 leads because it is the kind every wallet
// this device could build before this cycle used, and the kind the three record
// grammars carrying no kind field still mean (§7.1 routes 4, 5).
//
// Order is fixed in a list rather than derived by ranging over the constants so
// that adding a fifth kind is a deliberate edit here -- with a decision about
// where it sits -- and not a silent reshuffle of a screen an operator has
// muscle memory for.
var composerHashKinds = [...]md.HashKind{
	md.KindSha256, md.KindHash256, md.KindRipemd160, md.KindHash160,
}

// composerHashKindRow is one row: the token, then the digest width in hex
// characters, which is what the pad on the next screen will demand.
func composerHashKindRow(k md.HashKind) string {
	// TWO SPACES, NOT %-10s PADDING. The face is proportional (SPEC_hashlock_
	// kinds §7.5's own measurement: `rmd160` and `sha256` are both six
	// characters and differ in width), so padding to a character count cannot
	// align a column -- it only makes the gap ragged in a way that reads as a
	// rendering fault.
	return fmt.Sprintf("%s  %d hex", k.Token(), k.DigestLen()*2)
}

// composerHashKindPick is SPEC_hashlock_kinds §7.1's own screen: the hash KIND,
// which is a different axis from `Which hash?`'s SOURCE and from the phrase
// route's derivation METHOD.
//
// ITS OWN SCREEN, NOT A BAND ON `Which hash?`. Three hand-maintained gates are
// wired to that screen's row counts, and a fifth band would disturb all three
// for no design benefit (§7.1).
//
// IT COMES BEFORE THE MATERIAL ON BOTH ARMS, which is what keeps the KDF out of
// the Back path: nothing is held when the kind is chosen, so Back from here
// costs an operator nothing. It does NOT make the correction free -- an
// operator who reaches the phrase screen, realises the kind is wrong and steps
// back must retype the dropped phrase and pay the KDF again. Only the
// navigation is free.
//
// `initial` is the kind already chosen, so a Back from the pad or the phrase
// screen reopens here on it rather than re-proposing sha256.
func composerHashKindPick(ctx *Context, th *Colors, initial md.HashKind) (md.HashKind, bool) {
	rows := make([]string, 0, len(composerHashKinds))
	sel := 0
	for i, k := range composerHashKinds {
		rows = append(rows, composerHashKindRow(k))
		if k == initial {
			sel = i
		}
	}
	i, ok := composerPickScreenFrom(ctx, th, "Hash function",
		composerCopyHashKindLead(), rows, sel)
	if !ok {
		return initial, false
	}
	return composerHashKinds[i], true
}

// composerHexEntry is the fallback pad: the kind's digest as hex, accepted only
// at exactly md.HashKind.DigestLen()*2 characters (§6c, SPEC_hashlock_kinds
// §7.1).
//
// THE KIND ARRIVES FROM THE SCREEN BEFORE IT, and the bound follows the kind
// rather than a literal 64. That is what preserves the shipped property this
// pad was built on -- "an entry N characters long is N VALID characters by
// construction" -- at a second width: the alphabet is hex alone, and the clamp
// is the kind's own, so a full entry is always a well-formed digest OF THAT
// KIND. A fixed 64 would have refused every ripemd160 digest as too short while
// telling the operator nothing about which of the two numbers was wrong.
//
// IT RETURNS A LOCK OF THE KIND IT WAS GIVEN. This comment used to say it
// returned a sha256 lock "because `Which hash?` offers no kind" -- true until
// §7.1 put a kind screen in front of this pad, in the same commit that added
// the parameter above it. Left as it was, it described the screen it had just
// stopped describing, directly over the signature that contradicts it.
// `draft` is what was typed on a previous visit, restored on re-entry, and the
// second return value hands back whatever is on the pad when Back is pressed
// (F-541).
//
// BACK USED TO DISCARD IT SILENTLY. §7.1 put the kind screen behind Back, so
// "step back and check which kind this is" cost the whole entry -- up to 64
// characters transcribed from paper, thrown away for looking. The phrase arm
// already restores its draft (hashlockPhraseFlow takes `initial` for exactly
// this); the pad did not, which made the two arms disagree about what Back
// means.
func composerHexEntry(ctx *Context, th *Colors, kind md.HashKind, draft string) (*md.HashLock, string, bool) {
	want := kind.DigestLen() * 2
	kbd := NewKeyboard(ctx, composerHexKeys)
	// The draft may have been typed at a WIDER kind: re-entering at ripemd160
	// after typing 64 hex keeps the first 40 rather than discarding them, which
	// is the same clamp the live entry applies and for the same reason.
	//
	// AND IT SPEAKS, exactly as the live clamp does (F-570). This truncation was
	// silent, so a 64-hex sha256 digest re-entered at ripemd160 drew
	// `40 of 40 hex` with the checkmark lit -- the two strongest "you are done"
	// signals the screen has -- over the first 40 characters of the WRONG
	// digest. The lock that produces is one nobody can open.
	//
	// This screen's own shipped promise is "THE CLAMP IS SILENT NO LONGER", and
	// it was silent on exactly the path §7.1 created by putting a kind screen
	// behind Back. `over` is seeded HERE rather than left to the live clamp,
	// which only ever sees keystrokes.
	over := false
	if len(draft) > want {
		draft = draft[:want]
		over = true
	}
	kbd.Fragment = draft
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	// THE CLAMP IS SILENT NO LONGER (journey walk I-3). Dropping keystrokes
	// past the bound while lighting the checkmark and reporting `N of N hex` is
	// how a 64-hex PREIMAGE typed into a 40-hex ripemd160 pad becomes an
	// accepted, unspendable lock: the operator has just been told twice that
	// the preimage is 32 bytes, the word "digest" was last on screen two
	// screens ago, and the only signals that anything was dropped are a readout
	// that stops growing and a last-8 that will not match their paper.
	//
	// The clamp stays -- it is what keeps "an entry N characters long is N
	// VALID characters by construction" true -- but it now says so. `over`
	// clears as soon as the entry is short again, so it describes the entry in
	// hand rather than accusing the operator of a mistake they have corrected --
	// including a truncation carried in from the re-entry clamp above.
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if len(kbd.Fragment) > want {
			kbd.Fragment = kbd.Fragment[:want]
			over = true
		} else if len(kbd.Fragment) < want {
			over = false
		}
		frag := kbd.Fragment
		valid := len(frag) == want
		if backBtn.Clicked(ctx) {
			return nil, frag, false
		}
		clicked := okBtn.Clicked(ctx)
		if valid && clicked {
			raw, err := hex.DecodeString(frag)
			// md.NewHashLock re-checks the width against the KIND rather than
			// against a literal 32, and its false arm joins the decode's:
			// both mean "these are not the bytes of a sha256 digest", and
			// neither pads, truncates, nor returns a zero digest -- a silently
			// zero hashlock is spendable by anyone who knows the preimage of
			// zero. The pad offers hex alone and clamps at 64, so both arms
			// are unreachable from this screen.
			var lock *md.HashLock
			if err == nil {
				var ok bool
				lock, ok = md.NewHashLock(kind, raw)
				if !ok {
					lock = nil
				}
			}
			if lock == nil {
				// Unreachable, as it was before the kind screen: the alphabet
				// is hex and the clamp is the kind's own width, so `valid`
				// cannot be true for bytes NewHashLock rejects. It names the
				// kind anyway -- an unreachable message that hardcodes sha256
				// is a lie waiting for the day it becomes reachable.
				showError(ctx, th, "Hash lock",
					fmt.Sprintf("That is not a %s digest.", kind.Token()))
				continue
			}
			return lock, frag, true
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

		countText := fmt.Sprintf("%d of %d hex", len(frag), want)
		if over {
			countText += " - extra ignored"
		}
		count, csz := widget.Label(&ctx.B, ctx.Styles.body, th.Text, countText)
		countOp := count.Offset(image.Pt((dims.X-csz.X)/2, wordOff.Y+frgSize.Y+8))

		navBtns := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if valid {
			navBtns = append(navBtns, NavButton{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
		// THE TITLE NAMES THE KIND (journey walk I-2). This screen shows a
		// digest being BUILT, for longer than any other screen shows one at
		// all, and it was the one screen in the flow that did not say which
		// hash it belonged to. The width alone cannot answer it: sha256 and
		// hash256 are both 64 hex, ripemd160 and hash160 both 40, so an
		// off-by-one tap on the kind screen lands on a same-width sibling and
		// nothing here would differ. §6's both-or-neither rule engages -- the
		// width is a token an operator will READ as the kind, and it is not one.
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, kind.Token()+" hash")
		ctx.Frame(op.Layer(kbdOp, word, countOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return nil, "", false
}

const composerHashRowPhrase = "Type a hashlock phrase"

// composerHashRowHex is the typed-digest row. Named rather than inline because
// composerCopyHashlockPhraseRule points the operator at it BY LABEL, and the
// two drifting apart would send them looking for a row that is not there.
const composerHashRowHex = "Type a digest"

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
func composerHashInPayloadRow(i int, digest *md.HashLock) string {
	return composerHashRow(i, digest) + "  (in payload)"
}

// composerHashPreimageRow is band 2: a preimage PLATE record in the payload,
// whose digest is computed directly from the record -- no KDF, no countdown.
func composerHashPreimageRow(i int, digest *md.HashLock) string {
	return fmt.Sprintf("preimage %d  %s %s", i, digest.Kind().Token(), hashlockFirst8Last8(digest))
}

// composerHashPhraseRow is band 3, in its two forms (§5.1 Step 1).
//
// d == nil is the row BEFORE derivation, and it says why it cannot show a
// digest rather than showing a blank or a placeholder: deriving to draw the
// list is the "three records would be a 30 s stall before a list could be
// drawn" §5.1 rejects. Once the record has been derived in this composition the
// row carries the digest, read back out of hashlockHeld.
func composerHashPhraseRow(i int, d *md.HashLock) string {
	if d == nil {
		return fmt.Sprintf("phrase record %d (derive to see the digest)", i)
	}
	return fmt.Sprintf("phrase %d  %s %s", i, d.Kind().Token(), hashlockFirst8Last8(d))
}

// hashlockPayloadPreimage is one ClassPreimage record of the loaded payload,
// decoded once at row-build time. Both fields are SECRET; the composition's
// flow-exit defer scrubs whatever reaches hashlockHeld, and nothing here
// outlives the row set.
type hashlockPayloadPreimage struct {
	preimage [32]byte
	// digest is the LOCK the record's X hashes to, not a bare digest: a row
	// built from it is compared against the payload's own hash: records, whose
	// kind is part of what they say (SPEC_hashlock_kinds §5).
	digest *md.HashLock
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
	// F-573: computed once; the payload's hash: records are what name the kind
	// a kind-agnostic preimage belongs to.
	digests := composerPayloadDigests(s)
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
		// F-573: a preimage PLATE record is an ms1 string carrying X and no
		// kind -- and it needs none, a preimage being kind-agnostic. The
		// payload's own `hash:` records say which kind it is for, so this band
		// no longer draws three of the four kinds as sha256.
		out = append(out, hashlockPayloadPreimage{
			preimage: x,
			digest:   hashlockLockOf(hashlockKindFromPayload(&x, digests), &x),
		})
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
func hashlockDerivedDigest(st *composerState, rec sysw.PhraseRecord) *md.HashLock {
	if st == nil {
		return nil
	}
	want := hashlockMethodOf(rec.Method)
	// Ranged over the VALUES: hashlockHeld is keyed by md.HashLock.MapKey()
	// because a HashLock cannot be a Go map key, and the value carries the lock
	// itself so a loop like this one still has the kind (composer_state.go).
	for _, m := range st.hashlockHeld {
		if m.provenance != hashlockFromPayload || m.method != want {
			continue
		}
		if string(m.phrase) != rec.Phrase {
			continue
		}
		return m.lock
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
	digests   []*md.HashLock
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
	// `Type a digest`, NOT `Type 64 hex` (SPEC_hashlock_kinds §7.1). The row
	// used to name the width because the width was fixed; the kind screen now
	// sits between this row and the pad, and a ripemd160 entry is 40. A label
	// promising 64 to an operator who is about to be asked for 40 is the same
	// defect class as §13.2's reconcile screen: a screen stating the axis it
	// does not control. The pad states the real bound, once the kind is known.
	labels = append(labels, composerHashRowHex)
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
func composerHashInPayload(r composerHashRowSet, st *composerState, d *md.HashLock) bool {
	for _, p := range r.preimages {
		if p.digest.Equal(d) {
			return true
		}
	}
	for _, rec := range r.phrases {
		if h := hashlockDerivedDigest(st, rec); h.Equal(d) {
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
			// SPEC §8, band 1: a payload-supplied digest. The device did not
			// derive it, so a 20-byte kind warns here.
			if !composerWarnTwentyByteUnseen(ctx, th, st, rows.digests[sel]) {
				continue // declined -> `Which hash?`, nothing assigned
			}
			st.list.Paths[idx].Hash = rows.digests[sel]
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
		// ─── THE TWO TYPED ARMS, AND THE ONLY TWO THAT ASK FOR A KIND ────────
		//
		// SPEC_hashlock_kinds §7.1. Four of the six entry routes never reach the
		// kind screen and that is not an oversight: a payload `hash:` record
		// carries its kind in the record (§6), a `phrase:` record and a
		// preimage-plate record carry no kind field at all (so sha256), and the
		// preset archetype takes its kind from the preset's own grammar on the
		// host. Only material typed HERE has no other source for the answer.
		//
		// THE KIND SCREEN IS UPSTREAM OF THE MATERIAL on both arms, so `kind`
		// lives outside the inner loop and a Back from the pad or the phrase
		// screen reopens the kind screen ON THE KIND ALREADY CHOSEN. Back from
		// the kind screen itself leaves the inner loop, which lands on `Which
		// hash?` -- nothing was held, so nothing is discarded.
		//
		// `kind` is re-seeded to sha256 on each fresh entry from `Which hash?`,
		// not carried across, so an operator returning to this row later gets
		// the usual proposal rather than a stale one from an abandoned attempt.
		case sel == rows.phraseRow:
			kind := md.KindSha256
			// THE PHRASE LIVES OUT HERE, with the kind, because both survive the
			// same Back (F-571) -- the pad arm below does the same with its
			// draft, for the same reason.
			var phrase []byte
			for {
				k, ok := composerHashKindPick(ctx, th, kind)
				if !ok {
					break // -> `Which hash?`; nothing held, nothing discarded
				}
				kind = k
				outcome, kept := hashlockPhraseRoute(ctx, th, st, idx, rows.digests, kind, phrase)
				phrase = kept
				switch outcome {
				case hashlockAssigned:
					return true
				case hashlockBackToWhichHash:
					// ONE SCREEN EARLIER THAN THE NAME SAYS. H2 §4.6's leg is
					// "Back from the phrase screen -> `Which hash?` (phrase
					// dropped)"; with the kind screen inserted in front it stops
					// here instead, and THE PHRASE IS STILL DROPPED. Back itself
					// costs no derivation because the kind screen is upstream of
					// the KDF -- but retyping the phrase does.
					continue
				}
			}
		case sel == rows.hexRow:
			kind := md.KindSha256
			// THE DRAFT LIVES OUT HERE, with the kind, because both survive the
			// same Back (F-541). Stepping back to check the kind used to cost
			// the whole entry -- up to 64 characters off paper, discarded for
			// looking -- which made the kind screen expensive to consult and so
			// consulted less than it should be.
			draft := ""
			for {
				k, ok := composerHashKindPick(ctx, th, kind)
				if !ok {
					break // -> `Which hash?`, path intact
				}
				kind = k
				d, kept, ok := composerHexEntry(ctx, th, kind, draft)
				draft = kept
				if !ok {
					continue // Back from the pad -> the kind screen, kind AND draft kept
				}
				// SPEC §8: a TYPED digest is never device-derived.
				if !composerWarnTwentyByteUnseen(ctx, th, st, d) {
					continue // declined -> the kind screen, nothing assigned
				}
				st.list.Paths[idx].Hash = d
				return true
			}
		case sel == rows.noneRow:
			st.list.Paths[idx].Hash = nil
			return true
		default:
			panic(fmt.Sprintf("composerHashEdit: pick returned row %d of %d", sel, len(rows.labels)))
		}
	}
}
