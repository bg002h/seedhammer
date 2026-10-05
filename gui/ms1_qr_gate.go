package gui

import (
	"strings"

	"seedhammer.com/backup"
	"seedhammer.com/engrave"
)

// THE REFUGIUM BUILD ENGRAVES NO QR OF AN ms1 STRING (Refugium plan F7 §4.4,
// R0 I-8a; UI spec §4.4: ms1 plates carry a Standard SeedQR of the words,
// "never a QR of the ms1 string"). The producers -- the codex32 engrave flow,
// the sealed-unlock codex32 plate, a bundle ms1 card's TEXT + QR and QR ONLY
// variants -- each offer text only there; Engrave Text goes further and
// offers no QR at all (noFreeTextQR). The default build is unchanged.

// isMS1String is the ONE predicate every ms1 producer's gate uses: after the
// display separators (whitespace, '-', ',') are stripped, the string begins
// with the HRP `ms` and its separator `1`, in either case.
//
// No minimum length and no character-set check, so it is STRICTER than
// hashlock.IsMS1Shaped (48 characters, bech32 only): anything that so much as
// starts like an ms1 string is kept off a QR. A card's strings are verbatim
// codec strings, so for them this is exactly "the HRP is ms".
func isMS1String(s string) bool {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f', '-', ',':
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 3 {
			break
		}
	}
	return strings.EqualFold(b.String(), "ms1")
}

// noFreeTextQR reports whether Engrave Text may cut no QR in this build: under
// the Refugium profile it never does, for any text (F7 §4.4; closing fold
// recheck m-1). Neither the UI spec nor the plan gives this build a use for a
// free-text QR, and a predicate that tries to recognise an ms1 string inside
// free text can always be dodged by formatting -- a label per line, words
// between the groups -- so the profile removes the QR rather than guessing.
// Used by Engrave Text's QR step, its forced-off notice and ftBuildPlate.
func noFreeTextQR() bool {
	return refugiumProfile
}

// noMS1QR reports whether s must not be engraved as a QR in this build.
func noMS1QR(s string) bool {
	return refugiumProfile && isMS1String(s)
}

// engraveSeedStringPlate is the seed-string plate the codex32 producers cut:
// TEXT + QR in the default build, text only for an ms1 string under the
// profile (backup.EngraveSeedStringTextOnly, with its own goldens).
func engraveSeedStringPlate(params engrave.Params, s backup.SeedString) (engrave.Engraving, error) {
	if noMS1QR(s.Seed) {
		return backup.EngraveSeedStringTextOnly(params, s)
	}
	return backup.EngraveSeedString(params, s)
}

// ftNoQRNotice is what Engrave Text says if it ever has to drop a QR the
// operator asked for. Under the profile the QR step offers only "No QR", so
// this is the defensive arm of the same rule.
const ftNoQRNotice = "This build engraves text without a QR, so the plate carries no QR."

// ftQRLeadNoQR is the QR step's lead under the profile: one answer, the state.
const ftQRLeadNoQR = "This build engraves text without a QR."
