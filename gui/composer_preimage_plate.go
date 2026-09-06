package gui

import (
	"encoding/hex"
	"fmt"
	"sort"

	"seedhammer.com/backup"
	"seedhammer.com/codex32"
	"seedhammer.com/font/constant"
	"seedhammer.com/hashlock"
	"seedhammer.com/md"
)

// The preimage plates a composition may cut (H6 §5.3, §5.4, §6.3, §8.3-§8.5).
//
// THE DONE REVIEW IS TWO STEPS, AND THE FIRST ONE HAD TO BE A NEW SCREEN. The
// per-plate decisions do NOT live on confirmReviewScreen and cannot: that
// screen (gui/multisig_build.go:1895) is paged and READ-ONLY, every input is
// already bound (backBtn = Button1, contBtn = Button3 + Center, pageBtn =
// Button2), its body lines are widget.Labelw ops and not Clickables, and it is
// SHARED with the Policy Review. W-2 measured that exact failure once already:
// 205 taps that moved nothing, on a screen whose rows were not targets. So step
// (A) is one composerPickScreen per held digest and step (B) REPORTS what (A)
// decided.
//
// STEP (B) TAKES A VALUE, NOT THE STATE. composerCensusLines gains the decided
// plate list as a parameter rather than reading hashlockHeld, so the census
// reports a decision instead of recomputing one -- two answers to "what is
// being cut" is precisely the drift the census exists to remove.

// hashlockPlateChoice is what step (A) decided for one held digest (§5.3).
type hashlockPlateChoice int

const (
	// hashlockPlateDecline is Button1 and the `do not cut this preimage` row
	// alike. Declining does NOT abort the run: a preimage plate is not part of
	// the policy set, so bundleEngrave's set-level "a partial bundle can't be
	// used" reasoning (gui/bundle_flow.go:625-631) does not reach it.
	hashlockPlateDecline hashlockPlateChoice = iota
	// hashlockPlateString cuts the ms1 kind-0x03 plate string.
	hashlockPlateString
	// hashlockPlatePhrase cuts the phrase and its method, text only.
	hashlockPlatePhrase
	// hashlockPlatePhraseQR adds §8.6's QR, and taking it fires §8.5's warning.
	hashlockPlatePhraseQR
)

// hashlockPlate is one HELD digest and what this run decided about it.
//
// path is the 1-based index of the FIRST current path carrying the digest, or
// 0 for a digest no path carries. A retained preimage no current path carries
// is LISTED, never cut (§5.3 item 6), and NOTHING is deleted from hashlockHeld
// to achieve that (§2.2 item 2) -- the zero is what carries the difference.
type hashlockPlate struct {
	digest   [32]byte
	material hashlockMaterial
	path     int
	choice   hashlockPlateChoice
}

// composerPreimagePlates is every digest this composition HOLDS, in a
// DETERMINISTIC order: the paths first, in path order, then the digests no path
// carries, in digest order.
//
// THE ORDER IS NOT MAP ORDER, and that is load-bearing rather than tidy. Go
// randomises map iteration, so a census built by ranging hashlockHeld would
// list the same composition's plates in a different order on every frame -- on
// the screen whose whole job is to be read against the plates on the bench.
//
// A DIGEST TWO PATHS SHARE IS ONE PLATE. The plate carries the preimage, not
// the path; cutting it twice would double the bearer plates for one secret.
func composerPreimagePlates(st *composerState) []hashlockPlate {
	if st == nil || len(st.hashlockHeld) == 0 {
		return nil
	}
	var out []hashlockPlate
	seen := map[[32]byte]bool{}
	for i, p := range st.list.Paths {
		if p.Hash == nil || seen[*p.Hash] {
			continue
		}
		m, held := st.hashlockHeld[*p.Hash]
		if !held {
			continue
		}
		seen[*p.Hash] = true
		out = append(out, hashlockPlate{digest: *p.Hash, material: m, path: i + 1})
	}
	var rest [][32]byte
	for h := range st.hashlockHeld {
		if !seen[h] {
			rest = append(rest, h)
		}
	}
	// sort.Slice, AND THE ALTERNATIVE WAS MEASURED. A hand-rolled insertion
	// sort was tried here on the assumption that TinyGo's reflection-based
	// sort.Slice would be the expensive one; it measured 1,644,120 B of flash
	// against 1,643,580 for this line, i.e. 540 B WORSE. The reflect machinery
	// is already resident, so the call is free and the loop was not.
	sort.Slice(rest, func(i, j int) bool {
		return string(rest[i][:]) < string(rest[j][:])
	})
	for _, h := range rest {
		out = append(out, hashlockPlate{digest: h, material: st.hashlockHeld[h]})
	}
	return out
}

// composerPreimagePlateRows is §5.3's row set for one held digest, and the
// choice each row stands for.
//
// THE TWO PHRASE ROWS ARE OFFERED ONLY WHEN THE PHRASE IS HELD (decision 1). A
// payload-delivered preimage record carries X and no phrase at all, so a
// `phrase + method` plate for it would have nothing to engrave.
func composerPreimagePlateRows(m hashlockMaterial) ([]string, []hashlockPlateChoice) {
	rows := []string{"preimage string"}
	choices := []hashlockPlateChoice{hashlockPlateString}
	if len(m.phrase) > 0 {
		rows = append(rows, "phrase + method", "phrase + method + QR")
		choices = append(choices, hashlockPlatePhrase, hashlockPlatePhraseQR)
	}
	rows = append(rows, "do not cut this preimage")
	choices = append(choices, hashlockPlateDecline)
	return rows, choices
}

// composerPreimagePlatePick is step (A) for ONE digest.
//
// A PHRASE IS MASKED AND NOT REVEALED HERE (§5.3 item 7). The lead prints
// `phrase: <n> characters` and the method and shows no character at all. The
// shipped reveal affordance is `{label: "show", action: ppReveal}`
// (gui/passphrase_keyboard.go:141) -- a KEY ON A KEYBOARD GRID that LATCHES
// (k.revealed = !k.revealed, :221) -- and neither a pick screen nor a paged
// confirm has a key grid to put it on. NO NEW CONTROL IS INVENTED for a screen
// with no spare input. It applies to BOTH provenances: a payload-delivered
// phrase is a secret the operator never typed, may not own, and did not ask to
// see, in whatever room the machine lives in.
//
// BACK IS `do not cut` FOR THE HIGHLIGHTED PLATE, matching composerPickScreen's
// shipped decline arm. Backing out of the STEP is not offered here: §2.2 item 4
// keeps the composition intact through composerFlow's own loop, and a Back that
// unwound the whole step would leave the operator no way to answer the question
// for the remaining digests.
func composerPreimagePlatePick(ctx *Context, th *Colors, st *composerState, h [32]byte) hashlockPlateChoice {
	m, held := st.hashlockHeld[h]
	if !held {
		return hashlockPlateDecline
	}
	path := 0
	for i, p := range st.list.Paths {
		if p.Hash != nil && *p.Hash == h {
			path = i + 1
			break
		}
	}
	rows, choices := composerPreimagePlateRows(m)
	lead := composerCopyPreimagePlateLead(hashlockFirst8Last8(h), path, len(m.phrase), m.method.String())
	for !ctx.Done {
		sel, ok := composerPickScreen(ctx, th, "Preimage plate", lead, rows)
		if !ok {
			return hashlockPlateDecline
		}
		choice := choices[sel]
		if choice == hashlockPlatePhraseQR &&
			!composerConfirmScreen(ctx, th, "Preimage QR",
				composerConfirmBody(composerCopyPreimageQRWarning())) {
			continue // §8.5 declined -> back to the rows, nothing decided
		}
		return choice
	}
	return hashlockPlateDecline
}

// composerPreimagePlateStep runs step (A) over every held digest a CURRENT PATH
// carries, and returns the whole decided list -- the unused entries included,
// with their zero choice, because §8.3 REPORTS them.
//
// A DIGEST NO PATH CARRIES IS NOT OFFERED. There is no plate to decide about:
// §5.3 item 6 says it is listed and not cut, and asking the operator to choose
// a form for something that will not be cut is a question with no answer.
func composerPreimagePlateStep(ctx *Context, th *Colors, st *composerState) []hashlockPlate {
	plates := composerPreimagePlates(st)
	for i := range plates {
		if plates[i].path == 0 {
			continue
		}
		plates[i].choice = composerPreimagePlatePick(ctx, th, st, plates[i].digest)
	}
	return plates
}

// composerAcceptedPreimagePlates is the sub-list §5.4 cuts, in census order.
func composerAcceptedPreimagePlates(plates []hashlockPlate) []hashlockPlate {
	var out []hashlockPlate
	for _, p := range plates {
		if p.choice != hashlockPlateDecline {
			out = append(out, p)
		}
	}
	return out
}

// hashlockPlateFormWords is §8.3's form column: `phrase, hardened, QR`,
// `phrase, sha256`, `preimage string`.
func hashlockPlateFormWords(p hashlockPlate) string {
	switch p.choice {
	case hashlockPlatePhraseQR:
		return "phrase, " + p.material.method.String() + ", QR"
	case hashlockPlatePhrase:
		return "phrase, " + p.material.method.String()
	case hashlockPlateString:
		return "preimage string"
	}
	return ""
}

// hashlockPlateLocator is §6.3's locator rows, in order, pre-formatted for
// backup.Hashlock -- which takes no dependency on md or hashlock, exactly as
// backup.Passphrase takes none.
//
// THE `hash` ROW IS ALWAYS PRESENT, and that is the row standing between the
// operator and the worst artifact this stage can cut: a phrase-form plate with
// no locator at all is a bearer phrase in plain text with no digest, no path
// and no payload position.
//
// stub is the 8 hex characters of the mk1 stub, or "" when none can be
// computed; stubIsPolicy chooses between composer_stub.go:54's and :67's own
// literals, so the plate and the screen the operator copied into their notebook
// use the same words. payloadMatch is the 0-based index of the payload hash:
// record this digest equals, or -1.
func hashlockPlateLocator(path int, digest [32]byte, stub string, stubIsPolicy bool, payloadMatch int) []string {
	var out []string
	if path > 0 {
		out = append(out, fmt.Sprintf("path %d", path))
	}
	out = append(out, "hash  "+hashlockFirst8Last8(digest))
	if stub != "" {
		label := "mk1 stub (template): "
		if stubIsPolicy {
			label = "mk1 stub (policy): "
		}
		out = append(out, label+stub)
	}
	if payloadMatch >= 0 {
		out = append(out, fmt.Sprintf("matches hash %d in the payload", payloadMatch+1))
	}
	return out
}

// composerHashlockLocator is hashlockPlateLocator for a COMPOSER-NATIVE plate:
// the path is known, and the stub is the policy's when every slot is seated and
// the template's otherwise. No payload position is printed -- §6.3 scopes that
// row to the Hashlock plates flow, where the record's own index is the only
// locator there is.
func composerHashlockLocator(p hashlockPlate, template, keyed []string) []string {
	chunks, isPolicy := template, false
	if len(keyed) > 0 {
		chunks, isPolicy = keyed, true
	}
	stub := ""
	if len(chunks) > 0 {
		if s, err := md.FormAwareStubChunks(chunks); err == nil {
			stub = fmt.Sprintf("%x", s)
		}
	}
	return hashlockPlateLocator(p.path, p.digest, stub, isPolicy, -1)
}

// composerBuildHashlockPlate turns one decision into the plate backup engraves.
//
// THE ms1 STRING IS BUILT HERE AND NOWHERE ELSE: codex32.EncodeMS1Preimage
// refuses any id but `hash`, so a mistagged plate -- one that satisfies the wide
// IsPreimage, fails IsPreimagePlate and is therefore a spend secret on steel
// that no tool will read -- cannot be produced from this path.
func composerBuildHashlockPlate(p hashlockPlate, locator []string) (backup.Hashlock, error) {
	plate := backup.Hashlock{Locator: locator, Font: constant.Font}
	switch p.choice {
	case hashlockPlateString:
		s, err := codex32.EncodeMS1Preimage(p.material.preimage)
		if err != nil {
			return backup.Hashlock{}, err
		}
		plate.Form = backup.HashlockString
		plate.MS1 = s
	case hashlockPlatePhrase, hashlockPlatePhraseQR:
		hardened := p.material.method == hashlockHardened
		plate.Form = backup.HashlockPhrase
		plate.Phrase = string(p.material.phrase)
		plate.Method = hashlock.MethodLine(hardened)
		plate.QR = p.choice == hashlockPlatePhraseQR
		plate.QRText = hashlock.QRText(hardened, plate.Phrase)
	default:
		return backup.Hashlock{}, errHashlockPlateDeclined
	}
	return plate, nil
}

var errHashlockPlateDeclined = fmt.Errorf("gui: a declined preimage plate has no form to build")

// composerHashlockPlateBuiltHook records the plate description as it is built,
// with the locator the caller passed. nil in production; the sanctioned in-file
// seam (freetextEngraveHook, unlockEngraveHook, composerPlateCutHook).
//
// IT EXISTS FOR ONE ASSERTION AND NOTHING ELSE: §5.2 step 3 item 4's "the
// locator's `hash` row is printed from THAT result", i.e. from the DERIVED
// digest. Nothing at the screen layer can see a locator -- it is engraved, not
// drawn -- so a flow that built the locator BEFORE deriving would draw exactly
// the same screens and cut a bearer plate whose only locator row reads
// `hash  00000000..00000000`. That is the worst artifact this stage can cut,
// and until this seam existed no test in the package could fail on it.
var composerHashlockPlateBuiltHook func(desc backup.Hashlock)

func composerNoteHashlockPlateBuilt(desc backup.Hashlock) {
	if composerHashlockPlateBuiltHook != nil {
		composerHashlockPlateBuiltHook(desc)
	}
}

// composerHashlockPlateFor builds the engraver-ready Plate.
func composerHashlockPlateFor(pl Platform, p hashlockPlate, locator []string) (Plate, error) {
	desc, err := composerBuildHashlockPlate(p, locator)
	if err != nil {
		return Plate{}, err
	}
	composerNoteHashlockPlateBuilt(desc)
	params := pl.EngraverParams()
	side, err := backup.EngraveHashlock(params, desc)
	if err != nil {
		return Plate{}, err
	}
	return toPlate(side, params)
}

// composerPlateCutHook records each plate as the flow hands it to the engraver,
// in ORDER. nil in production; the sanctioned in-file seam (freetextEngraveHook,
// unlockEngraveHook).
//
// IT EXISTS FOR ONE ASSERTION AND NOTHING ELSE: §5.4's order. Nothing at the
// screen layer can see that the preimage plates were cut BEFORE the policy set
// -- both runs draw the same screens in the same shapes -- and the window the
// order removes is the one in which the md1 plates exist and the preimage does
// not.
var composerPlateCutHook func(kind string)

func composerNotePlateCut(kind string) {
	if composerPlateCutHook != nil {
		composerPlateCutHook(kind)
	}
}

// composerAbortNoPreimage is §8.4a: a preimage plate's own Engrave returned
// false and NONE has been cut.
//
// IT SAYS "dies with this composition", NOT "is now gone", and that is a
// correction rather than a style. An abort here returns false from
// composerEngraveStep and composerFlow LOOPS BACK with the state intact
// (gui/composer_flow.go:47-131) -- the phrase is still held. Saying it is gone
// would be false on the screen whose job is to stop a funding decision.
func composerAbortNoPreimage(ctx *Context, th *Colors) bool {
	showError(ctx, th, "Wallet Policy", composerCopyAbortNoPreimage())
	return false
}

// composerAbortPreimageCut is §8.4b: at least one preimage plate was cut and
// the run then ended.
//
// THIS IS THE WINDOW THE ORDER CREATES, and its danger is specific.
// bundleEngrave's set-level copy tells the operator a partial bundle cannot be
// used, whose natural response is to run the composition again; §5.3 sets no
// cap, so the second run cuts a SECOND bearer plate for the same secret, one of
// which the operator has no record of -- while §8.3's own line tells them to
// store them apart from each other.
func composerAbortPreimageCut(ctx *Context, th *Colors) bool {
	showError(ctx, th, "Wallet Policy", composerCopyAbortPreimageCut())
	return false
}

// composerPreimageCensusLines is §8.3's block: the heading, one row per
// accepted plate, the two not-cut forms, and the apart-storage line.
//
// A DECLINED PLATE IS DROPPED FROM THE PLAN AND NAMED. Silence would leave the
// operator reading a census that is right about what it lists and says nothing
// about the decision they just made.
func composerPreimageCensusLines(plates []hashlockPlate) []string {
	if len(plates) == 0 {
		return nil
	}
	accepted := composerAcceptedPreimagePlates(plates)
	// §8.3's STAND-ALONE NOTICE FORM: a review whose only entry is an unused
	// preimage. The terse row says what happened; this says what to do about it,
	// and on a review with nothing else to read there is room for it.
	if len(plates) == 1 && plates[0].path == 0 {
		return []string{"", composerCopyPreimageOnlyNotice()}
	}
	out := []string{""}
	if len(accepted) > 0 {
		out = append(out, composerCopyPreimagePlateHeading(len(accepted)))
		for _, p := range accepted {
			out = append(out, composerCopyPreimagePlateRow(p.path,
				hashlockFirst8Last8(p.digest), hashlockPlateFormWords(p)))
		}
	}
	for _, p := range plates {
		switch {
		case p.path == 0:
			out = append(out, composerCopyPreimageNotOnAnyPath(hashlockFirst8Last8(p.digest)))
		case p.choice == hashlockPlateDecline:
			out = append(out, composerCopyPreimageDeclined(hashlockFirst8Last8(p.digest)))
		}
	}
	if len(accepted) > 0 {
		out = append(out, composerCopyPreimageKeepApart())
	}
	return out
}

// composerEveryHashedPathHeld is §10.1's third-arm predicate: every hashed path's
// digest has material this composition holds.
//
// PRESENT TENSE, NOT FUTURE (§10.1). The banner's one call site
// (gui/composer_shape.go) runs at composerFlow's line 75 while the accept or
// decline happens at line 125, so "this run cuts a plate for each one" would be
// read by an operator who then declines one -- and be left with a backup
// instruction that was false about what the run did.
func composerEveryHashedPathHeld(st *composerState) bool {
	if st == nil || len(st.hashlockHeld) == 0 {
		return false
	}
	any := false
	for _, p := range st.list.Paths {
		if p.Hash == nil {
			continue
		}
		any = true
		if _, held := st.hashlockHeld[*p.Hash]; !held {
			return false
		}
	}
	return any
}

// composerEveryHeldPathHasAPhrase chooses §10.1's FOURTH arm, whose body says
// this composition "holds the phrase and method for each one".
//
// EVERY, NOT AT LEAST ONE, and the body is what forces it: on a mixed policy
// where one path's material is a payload preimage record with no phrase at all,
// "for each one" would be false. The plan's own wording ("at least one of those
// came from a phrase typed here") also excludes a payload-delivered phrase the
// device does hold -- and the operator's backup burden is identical either way.
func composerEveryHeldPathHasAPhrase(st *composerState) bool {
	if !composerEveryHashedPathHeld(st) {
		return false
	}
	for _, p := range st.list.Paths {
		if p.Hash == nil {
			continue
		}
		if len(st.hashlockHeld[*p.Hash].phrase) == 0 {
			return false
		}
	}
	return true
}

// composerAnyPathHashed is §10.3's predicate for the plate marking.
func composerAnyPathHashed(st *composerState) bool {
	for _, p := range st.list.Paths {
		if p.Hash != nil {
			return true
		}
	}
	return false
}

// composerPreimageMarkTitle is §10.3: `PREIMAGE REQUIRED` on the md1 AND the
// mk1 key cards when any path carries a hash.
//
// singleSigPlateMark's mechanism unchanged (gui/singlesig.go:365-374, whose
// "PASSWORD REQUIRED" is also 17 characters against MaxTitleLen = 18).
// bundlePlateMark (gui/bundle_flow.go:573-578) excludes exactly one kind --
// cardMS1 -- so a run-level markTitle stamps every mk1 key card too. THAT IS
// TAKEN DELIBERATELY: it is singleSigPlateMark's own precedent and what F-132
// asks for, since a cosigner holding a key card is exactly the year-later
// reader who otherwise has nothing telling them a preimage exists. Marking per
// card kind would suppress the message on the only artifact that travels.
func composerPreimageMarkTitle(st *composerState) string {
	if composerAnyPathHashed(st) {
		return "PREIMAGE REQUIRED"
	}
	return ""
}

// hashlockDigestHex is the census's own spelling of a digest, for tests and for
// the locator alike.
func hashlockDigestHex(h [32]byte) string { return hex.EncodeToString(h[:]) }
