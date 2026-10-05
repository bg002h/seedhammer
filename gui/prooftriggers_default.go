//go:build !refugium

package gui

// The typed QA proof triggers and everything that spells one, kept out of the
// Refugium build (Refugium plan F7 §4.1, R0 M-5 and round 3 M-1/M-3). Their
// twin, prooftriggers_refugium.go, defines the same names with no trigger in
// them; the comparisons are additionally gated by proofTriggersEnabled
// (profile_*.go), because a trigger disabled by setting it to "" would fire on
// an empty field.
//
// Moved verbatim from gui/freetext_proof.go and gui/passphrase_passproof.go.

const (
	// ftProofTriggerSH loads the font/sh proof. Kept from the original feature
	// (the followup, the review and the operator's notes all call it
	// TEXTPROOF!), and it needs no face in its name: font/sh is the face this
	// program engraves in unless told otherwise, so TEXTPROOF! proves the TEXT
	// plate as it normally cuts.
	ftProofTriggerSH = "TEXTPROOF!"

	// ftProofTriggerConst loads the font/constant proof. Named for the face and
	// not for the plate, because that is the whole point of it: it borrows the
	// free-text plate as a rig to qualify the face every OTHER plate is cut in.
	// The two triggers are the same length and differ from the first character,
	// so neither is a prefix of the other and a mistyped one matches nothing.
	ftProofTriggerConst = "CONSTPROOF!"

	// ftProofTriggerBoth loads the MIXED-FACE proof: one plate whose top half
	// is cut in font/sh and whose bottom half is cut in font/constant, both at
	// 3.0mm. Named for what it proves rather than for a face, because it proves
	// both.
	//
	// It exists because plates are scarce. A legibility round that has to
	// qualify both faces costs two plates -- TEXTPROOF! and CONSTPROOF! -- and
	// this one costs one. It is not a replacement for either: half a plate
	// holds less than a whole one, so the single-face proofs stay for the
	// rounds where a face needs the deeper read.
	//
	// The three triggers differ from their FIRST character, so none is a prefix
	// of another and a mistyped one matches nothing.
	ftProofTriggerBoth = "BOTHPROOF!"

	// The SIZE LADDER, one trigger per side of one plate. Each side carries the
	// complete 95-character sweep in BOTH faces at every rung it names, so the
	// pair answers what a render cannot: which glyphs survive as the size drops.
	//
	// Two triggers and no bare SIZEPROOF!: the ladder has no default half, and
	// defaulting would let a slip cut the wrong side onto steel already
	// engraved. Neither may ever be marked Sizeable, or SIZEPROOF!FRONT4.4
	// becomes ambiguous against the rung suffix parser -- and the rung the
	// ladder resolves to is 0, which is what both of ftFitAt's routing rules
	// depend on.
	//
	// The slot after the '!' names the SIDE, per LEXICON_proof_triggers.md: the
	// root names an axis and the slot holds one kind. It is a side here and a
	// rung after BOTHPROOF!, which is why this is a second trigger rather than
	// a value in the first one's slot.
	ftProofTriggerSizeFront = "SIZEPROOF!FRONT"
	ftProofTriggerSizeBack  = "SIZEPROOF!BACK"
)

// ftProofs is every proof the free-text program offers: one per engraving face,
// the mixed-face plate, and the two sides of the size ladder.
var ftProofs = []ftProof{
	{
		Trigger: ftProofTriggerSH,
		Plan:    &ftPlanSH,
		Title:   ftProofTitleSH,
		Footer:  ftProofFooter,
		Text:    ftProofTextSH,
		TextQR:  ftProofTextSHQR,
	},
	{
		Trigger: ftProofTriggerConst,
		Plan:    &ftPlanConst,
		Title:   ftProofTitleConst,
		Footer:  ftProofFooter,
		Text:    ftProofTextConst,
		TextQR:  ftProofTextConstQR,
	},
	{
		Trigger:  ftProofTriggerBoth,
		Plan:     &ftPlanBoth,
		Sizeable: true,
		Title:    ftProofTitleBoth,
		Footer:   ftProofFooter,
		Text:     ftProofTextBoth,
		// No QR variant: the pattern needs the whole plate. See
		// NeedsWholePlate.
		TextQR: "",
	},
	{
		Trigger: ftProofTriggerSizeFront,
		Plan:    &ftPlanSizeFront,
		Side:    "FRONT",
		Title:   ftProofTitleSizeFront,
		// No footer: measured, one at the title's 3.8mm rung starts 3.200mm
		// above where this side's body ends and the fit refuses the plate.
		Footer: "",
		Text:   ftProofTextSizeFront,
		// No QR variant either, and here it is structural rather than a
		// capacity judgement: FitSized has no parameter for a code, because the
		// keep-out band is quantised by a single fontSize and a plate that
		// mixes sizes has none. Empty is what makes the prompted drop apply.
		TextQR: "",
	},
	{
		Trigger: ftProofTriggerSizeBack,
		Plan:    &ftPlanSizeBack,
		Side:    "BACK",
		Title:   ftProofTitleSizeBack,
		// 1.600mm short at the title's 3.0mm rung.
		Footer: "",
		Text:   ftProofTextSizeBack,
		TextQR: "",
	},
}

// ppPassProofTrigger is the literal that offers the pattern. The trailing '!'
// mirrors the NFC debugCommand precedent (FOREVERLAURA!, gui/scan.go).
//
// Renamed from FONTPROOF! (2026-08-05). The old root named no axis and was
// distinguished from the free-text program's TEXTPROOF! only by living in
// another program -- "font" and "text" being near-synonyms, while FONTPROOF!
// cut in font/constant, the very face CONSTPROOF! proves. That is not
// theoretical: the operator called the free-text proof "FONTPROOF!" repeatedly,
// and typing it opened the passphrase program instead. PASSPROOF! names its
// program and differs from every other root at the first character. See
// mnemonic-engrave design/LEXICON_proof_triggers.md.
const ppPassProofTrigger = "PASSPROOF!"

// ppIsPassProofTrigger is ppPassProofOffer's comparison, kept beside the
// constant it compares against.
func ppIsPassProofTrigger(typed string) bool {
	return typed == ppPassProofTrigger
}

// The passphrase test pattern's declining-branch sentences spell the trigger,
// so they live here with it. See gui/passphrase_passproof.go for the rest of
// the prompt's wording.
const (
	// The declining branch differs by FIELD, and saying otherwise makes the one
	// prompt in this feature whose entire purpose is honesty tell a small lie.
	// In the passphrase field, "no" really does continue with PASSPROOF! as the
	// passphrase. In either fingerprint field it CANNOT: ValidateFingerprint
	// refuses a non-hex value, so "no" returns to the field with the text still
	// there. Pinned by TestPassProofNoBranchInFingerprintFieldRefuses, which
	// asserts the refusal, and by TestPassProofKeepLineMatchesTheField.
	ppPassProofKeepPassphrase = "Back = no: continue with PASSPROOF! exactly as typed. " +
		"Any text can be a real passphrase, including this one."

	ppPassProofKeepFingerprint = "Back = no: keep PASSPROOF! in this field. " +
		"It is not hex, so this field will ask again for 8 hex digits."
)

// ppPassProofKeep is the declining-branch sentence for a field.
func ppPassProofKeep(isPassphrase bool) string {
	if isPassphrase {
		return ppPassProofKeepPassphrase
	}
	return ppPassProofKeepFingerprint
}
