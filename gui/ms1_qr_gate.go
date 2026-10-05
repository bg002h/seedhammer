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
// variants, and Engrave Text's "Add QR" -- each offer text only there. The
// default build is unchanged.

// isMS1String is the ONE predicate every producer's gate uses: after the
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

// ftMS1NoQRNotice is what Engrave Text says when it drops a QR the operator
// asked for because the text is an ms1 string.
const ftMS1NoQRNotice = "This text is an ms1 secret string. This build never engraves an ms1 string as a QR, so the plate carries no QR."

// ftQRLeadMS1 is the QR step's lead when the composition is already an ms1
// string (a payload text record, or Back over typed text).
const ftQRLeadMS1 = "This text is an ms1 secret string. This build never engraves one as a QR."
