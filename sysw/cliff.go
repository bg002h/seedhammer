package sysw

import (
	"strings"

	"seedhammer.com/bip39"
)

// CliffAbove reports whether a NORMALISED passphrase is at or above the
// threshold: five or more whitespace-separated tokens, every one a BIP-39
// English wordlist entry.
//
// A pure function of the normalised string, so host and device agree with no
// shared state, no header field and nothing attacker-controlled. That is the
// whole reason it is a word count: an entropy threshold would have to be
// recorded somewhere the device could read, and any such field is
// attacker-controlled.
//
// A SPEED BUMP, NOT A STRENGTH MEASURE. "abandon" five times is five wordlist
// tokens, zero entropy, and above it. Deliberate: these programs are the
// lower-assurance branch.
func CliffAbove(normalised string) bool {
	n := 0
	for _, tok := range strings.Fields(normalised) {
		w, ok := bip39.ClosestWord(strings.ToUpper(tok))
		if !ok || !strings.EqualFold(bip39.LabelFor(w), tok) {
			return false
		}
		n++
	}
	return n >= 5
}
