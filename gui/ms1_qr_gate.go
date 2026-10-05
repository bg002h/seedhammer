package gui

import (
	"strings"
	"unicode"

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

// containsMS1String is the FREE-TEXT predicate (review I-1, recheck m-1): after
// every rune that is not a letter or a digit is dropped -- spaces (NBSP
// included), line breaks and any punctuation a share might be grouped with
// ('-', ',', '.', '/', ':', '_', ...) -- the text contains, ANYWHERE and in
// either case, the HRP-and-separator `ms1` followed by at least
// ms1MinDataChars bech32 characters.
//
// Known limit: a share broken up by WORDS (a label per line, "Line 2: ...")
// is not caught when no run of 16 data characters survives; a word with a
// non-bech32 letter (b, i, o) ends the run.
//
// isMS1String only looks at the start, which is exact for the verbatim codec
// strings the bundle and codex32 producers handle, and wrong for free text: a
// label, a bracket, a list number or a quote in front of a share leaves the
// secret just as readable in the QR. Erring wide here only costs a QR on text
// that happens to hold "ms1" and sixteen bech32 characters in a row.
func containsMS1String(s string) bool {
	var b strings.Builder
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	t := b.String()
	for i := 0; ; {
		j := strings.Index(t[i:], "ms1")
		if j < 0 {
			return false
		}
		k := i + j + len("ms1")
		n := 0
		for k+n < len(t) && strings.IndexByte(bech32Charset, t[k+n]) >= 0 {
			n++
		}
		if n >= ms1MinDataChars {
			return true
		}
		i = i + j + 1
	}
}

// bech32Charset is BIP-173's data alphabet, lowercase.
const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// ms1MinDataChars is how many bech32 characters after `ms1` make free text an
// ms1 string for containsMS1String. Every codex32 string has far more (BIP-93's
// shortest is 48 characters in all); sixteen is enough to stop a QR of one.
const ms1MinDataChars = 16

// noMS1QRText reports whether free text must not be engraved as a QR in this
// build: Engrave Text's QR step, its forced-off notice, and ftBuildPlate.
func noMS1QRText(s string) bool {
	return refugiumProfile && containsMS1String(s)
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
