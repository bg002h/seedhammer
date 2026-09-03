package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/md"
)

// The mapping review (SPEC §7d), the last screen before consent and the only
// one that shows slot, fingerprint and origin together.
//
// THE ORIGINS ARE PRINTED VERBATIM, with the note that the device cannot
// confirm the key was derived there. A key: record's origin proves the
// xpub's DEPTH and its LAST COMPONENT against the declared path and nothing
// else; the account and every interior component are declarations neither
// this device nor mk can verify (F-217). Printing them without that sentence
// would imply a check that was never run.

// composerOriginKey renders an origin for comparison. Structural, not
// textual: bip32.Path.String renders `m/48h/...` while an mk1 card carries
// `m/48'/...`, and a string comparison would match neither
// (gui/key_card_seating.go:130-137 states the same lesson).
func composerOriginKey(o []md.PathComponent) string {
	var b strings.Builder
	for _, c := range o {
		fmt.Fprintf(&b, "%d", c.Value)
		if c.Hardened {
			b.WriteByte('h')
		}
		b.WriteByte('/')
	}
	return b.String()
}

// composerInvariantViolation is §4f's pairwise-distinguishability invariant:
// no two slots may declare the same origin unless BOTH declare a fingerprint
// and those fingerprints DIFFER.
//
// The asymmetric case -- one fingerprint present, one absent -- is the
// dangerous one and is why the rule is not simply "two fingerprints".
// slotMatchesCard skips the fingerprint test when the template declares none
// (gui/key_card_seating.go:151-159), so one card fills BOTH slots and the
// operator is shown a mis-seated key as reviewed.
func composerInvariantViolation(st *composerState) bool {
	type seat struct {
		fp        [4]byte
		fpPresent bool
	}
	byOrigin := map[string][]seat{}
	for _, a := range st.assigned {
		if a.src < 0 {
			// AN UNSEATED SLOT IS NOT A COLLISION. Its origin is nil here, so
			// every unseated slot hashed to "" and two of them looked like two
			// keys at one origin with no fingerprints -- refusing §7f's
			// partially seated form and C26's key-less template with §8v, a
			// body about keys that are not there. The origins those slots will
			// DECLARE are §4f's lowest-free accounts, assigned by the codec
			// when the template is emitted, and the real invariant is checked
			// on the decoded md1 by composerSelfCheck.
			continue
		}
		k := composerOriginKey(a.origin)
		byOrigin[k] = append(byOrigin[k], seat{a.fingerprint, a.fpPresent})
	}
	for _, seats := range byOrigin {
		if len(seats) < 2 {
			continue
		}
		for i := 0; i < len(seats); i++ {
			for j := i + 1; j < len(seats); j++ {
				if !seats[i].fpPresent || !seats[j].fpPresent {
					return true
				}
				if seats[i].fp == seats[j].fp {
					return true
				}
			}
		}
	}
	return false
}

// composerDuplicateXpub is §7d's same-xpub refusal (BIP-388 line 193's
// pairwise-distinct rule). md refuses it only at ENCODE, so catching it here
// is the difference between a review that names both slots and a codec error
// the operator cannot act on.
func composerDuplicateXpub(st *composerState) (uint8, uint8, bool) {
	seen := map[string]int{}
	for i, a := range st.assigned {
		if a.xpub == "" {
			continue
		}
		if j, ok := seen[a.xpub]; ok {
			return uint8(j), uint8(i), true
		}
		seen[a.xpub] = i
	}
	return 0, 0, false
}

// composerSharedSeed is one C29 finding: the slots INSIDE one path that share
// a fingerprint, with that path's threshold.
type composerSharedSeed struct {
	slots []uint8
	k, n  int
}

// composerSharedSeedInPath finds C29's case: one seed (one fingerprint) at
// two or more slots INSIDE ONE path. Across paths is C5's NORMAL case and
// gets an informational line instead (§7g).
func composerSharedSeedInPath(st *composerState) []composerSharedSeed {
	order := composerSlotOrder(st.list)
	byPath := map[int]map[[4]byte][]uint8{}
	for i, a := range st.assigned {
		if a.src < 0 || i >= len(order) || !a.fpPresent {
			continue
		}
		p := order[i].path
		if byPath[p] == nil {
			byPath[p] = map[[4]byte][]uint8{}
		}
		byPath[p][a.fingerprint] = append(byPath[p][a.fingerprint], uint8(i))
	}
	var out []composerSharedSeed
	for i, p := range st.list.Paths {
		if p.Keys == nil {
			continue
		}
		for _, slots := range byPath[i+1] {
			if len(slots) < 2 {
				continue
			}
			out = append(out, composerSharedSeed{slots: slots, k: int(p.Keys.K), n: int(p.Keys.N)})
		}
	}
	return out
}

// composerSharedSeedBody picks between §8g's two bodies: the FIRST when the
// shared slots REACH the threshold (one person can satisfy the path alone),
// the second otherwise (they hold some of what it needs).
func composerSharedSeedBody(c composerSharedSeed) string {
	if len(c.slots) >= c.k {
		return composerCopySameSeedThreshold(c.slots, c.k, c.n)
	}
	return composerCopySameSeedBelow(c.slots, c.k)
}

// composerPersonInTwoPaths is C5's normal case: one fingerprint seated in two
// DIFFERENT paths. It earns §8k's informational line, never a warning.
func composerPersonInTwoPaths(st *composerState) bool {
	order := composerSlotOrder(st.list)
	paths := map[[4]byte]map[int]bool{}
	for i, a := range st.assigned {
		if a.src < 0 || i >= len(order) || !a.fpPresent {
			continue
		}
		if paths[a.fingerprint] == nil {
			paths[a.fingerprint] = map[int]bool{}
		}
		paths[a.fingerprint][order[i].path] = true
		if len(paths[a.fingerprint]) > 1 {
			return true
		}
	}
	return false
}

// composerMappingLines is the review body: slot, fingerprint, origin VERBATIM,
// then what the device did NOT check.
func composerMappingLines(st *composerState) []string {
	var lines []string
	for i, a := range st.assigned {
		if a.src < 0 {
			lines = append(lines, fmt.Sprintf("@%d: unseated", i))
			continue
		}
		fp := "no fingerprint"
		if a.fpPresent {
			fp = fmt.Sprintf("%x", a.fingerprint)
		}
		lines = append(lines, fmt.Sprintf("@%d: %s %s", i, fp, composerOriginText(a.origin)))
	}
	// F-217, said plainly. The xpub's DEPTH and its LAST component are checked
	// against the declared path; the account and every interior component are
	// declarations neither this device nor mk can verify. Printing the origin
	// without this sentence would imply a check that was never run.
	lines = append(lines, "", "This device cannot confirm a key was derived at the origin it declares.")
	for _, c := range composerSharedSeedInPath(st) {
		lines = append(lines, "", composerSharedSeedBody(c))
	}
	if composerPersonInTwoPaths(st) {
		lines = append(lines, "", composerCopyPersonInTwoPaths())
	}
	return lines
}

// composerOriginText renders an origin the way a card writes it, with `'` for
// hardening -- the spelling mk.Card.Path carries (gui/key_card_seating.go
// :130-137 names the two notations and why they are compared structurally).
func composerOriginText(o []md.PathComponent) string {
	var b strings.Builder
	b.WriteByte('m')
	for _, c := range o {
		fmt.Fprintf(&b, "/%d", c.Value)
		if c.Hardened {
			b.WriteByte('\'')
		}
	}
	return b.String()
}

// composerMappingReview refuses first, then shows. Back KEEPS assignments
// (§7d): of everything Back can discard on this path, a seating the operator
// has just worked through is among the most expensive.
func composerMappingReview(ctx *Context, th *Colors, st *composerState) bool {
	if composerInvariantViolation(st) {
		showError(ctx, th, "Key mapping", composerCopySameOriginFewFingerprints())
		return false
	}
	if a, b, dup := composerDuplicateXpub(st); dup {
		showError(ctx, th, "Key mapping", fmt.Sprintf(
			"Slots @%d and @%d hold the same key. Every slot needs a different key.", a, b))
		return false
	}
	return composerReadScreen(ctx, th, "Key mapping", composerMappingLines(st))
}
