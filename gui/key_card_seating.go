package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/bip32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── Seating gathered mk1 key cards onto a keyless template (F-216, D3) ──────
//
// A keyless template names its slots by ORIGIN — `@0` lives at
// `48'/0'/0'/2'` — and carries no keys. An mk1 card carries a key and declares
// the origin it came from. Seating is the join, and the ruling
// (`design/agent-reports/RULING_f216_slot_mapping.md`) fixes how:
//
//	LAYER 1  the card's policy_id_stub must include this template's stub
//	LAYER 2  the card's origin must equal the slot's declared origin, and its
//	         fingerprint the slot's, when the template declares one
//
// GATHER ORDER IS NEVER AN INPUT, and the operator is never asked to assign a
// card to a slot. Both were rejected for the same reason: they are silent when
// wrong. A misassignment does not fail — it derives a different wallet's
// address and shows it to the operator as PROOF, which is worse than showing
// none. Every state this cannot decide is a refusal.
//
// ONE CARD MAY FILL SEVERAL SLOTS. A policy can legitimately seat one master at
// several accounts, and a card whose origin matches two slots fills both. What
// is refused is the reverse: two DIFFERENT cards claiming one slot.

// Typed refusals, so the screen can say which of them happened. "Your cards
// were refused" without a reason is the worst version of a correct refusal.
var (
	// errSeatNotThisPolicy — the card's stubs do not include this template's.
	errSeatNotThisPolicy = errors.New("seat: card does not belong to this policy")
	// errSeatNoSlot — the card belongs to the policy but matches no slot's
	// declared origin.
	errSeatNoSlot = errors.New("seat: card matches no slot in this policy")
	// errSeatSlotUnfilled — a slot has no card.
	errSeatSlotUnfilled = errors.New("seat: a slot has no key card")
	// errSeatSlotContested — two DIFFERENT cards claim one slot. Undecidable,
	// and the one state where guessing would be invisible.
	errSeatSlotContested = errors.New("seat: two different cards claim one slot")
)

// seatKeyCards fills a keyless template's slots from gathered mk1 cards.
//
// Returns the template's `[]md.ExpandedKey` with every slot's xpub populated,
// or a typed refusal. On any error NOTHING is returned to derive from: a
// partial seating is exactly the state that produces a plausible wrong address.
func seatKeyCards(templateMd1 []string, cards []mk.Card) ([]md.ExpandedKey, error) {
	stub, err := md.FormAwareStubChunks(templateMd1)
	if err != nil {
		return nil, err
	}
	_, keys, err := md.ExpandWalletPolicyChunks(templateMd1)
	if err != nil {
		return nil, err
	}

	// LAYER 1 — membership. This cannot map a card to a slot; what it does is
	// make layer 2 exact, by removing the wrong-master card whose origin
	// happens to match. Standard paths like 48'/0'/0'/2' collide across masters
	// constantly, so without this a stranger's card at a common path would seat
	// itself wherever the fingerprint was elided.
	for i, c := range cards {
		if !hasStub(c.Stubs, stub) {
			return nil, fmt.Errorf("%w: card %d (%s)", errSeatNotThisPolicy, i+1, c.Path)
		}
	}

	// LAYER 2 — declaration match.
	filledBy := make([]int, len(keys))
	for i := range filledBy {
		filledBy[i] = -1
	}
	for ci, c := range cards {
		seated := false
		for si := range keys {
			ok, err := slotMatchesCard(keys[si], c)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			seated = true
			if prev := filledBy[si]; prev >= 0 && !sameCardKey(cards[prev], c) {
				return nil, fmt.Errorf("%w: @%d claimed by cards %d and %d",
					errSeatSlotContested, keys[si].Index, prev+1, ci+1)
			}
			filledBy[si] = ci
		}
		if !seated {
			return nil, fmt.Errorf("%w: card %d (%s)", errSeatNoSlot, ci+1, c.Path)
		}
	}

	out := make([]md.ExpandedKey, len(keys))
	copy(out, keys)
	for si := range out {
		ci := filledBy[si]
		if ci < 0 {
			return nil, fmt.Errorf("%w: @%d (%s)", errSeatSlotUnfilled, out[si].Index, out[si].OriginPath)
		}
		cc, pk, _, err := decodeXpubBytes(cards[ci].Xpub)
		if err != nil {
			return nil, err
		}
		copy(out[si].Xpub[0:32], cc[:])
		copy(out[si].Xpub[32:65], pk[:])
		out[si].XpubPresent = true
	}
	return out, nil
}

// slotMatchesCard is layer 2's predicate: same origin, and same fingerprint
// when the template declares one.
//
// THE PATHS ARE COMPARED STRUCTURALLY, NOT AS STRINGS, and this is not a
// stylistic choice. `bip32.Path.String()` renders `m/48h/0h/0h/2h` while
// `mk.Card.Path` carries `m/48'/0'/0'/2'` — the same path in two notations. A
// string comparison matches NEITHER card against ANY slot, and the symptom is
// every card refused, which reads as a corrupt card rather than a formatting
// mismatch. `bip32.ParsePath` accepts both spellings, so parsing is exact.
func slotMatchesCard(slot md.ExpandedKey, c mk.Card) (bool, error) {
	cp, err := bip32.ParsePath(c.Path)
	if err != nil {
		return false, fmt.Errorf("seat: card path %q: %w", c.Path, err)
	}
	if len(cp) != len(slot.OriginPath) {
		return false, nil
	}
	for i := range cp {
		if cp[i] != slot.OriginPath[i] {
			return false, nil
		}
	}
	// The fingerprint is checked only when the TEMPLATE declares one. A
	// template may elide fingerprints; requiring the card to match an absent
	// declaration would refuse every card for a legal template.
	if slot.FingerprintPresent {
		if c.Fingerprint == "" {
			return false, nil
		}
		if !equalFingerprint(slot.Fingerprint, c.Fingerprint) {
			return false, nil
		}
	}
	return true, nil
}

func equalFingerprint(want [4]byte, got string) bool {
	const hexDigits = "0123456789abcdef"
	if len(got) != 8 {
		return false
	}
	for i, b := range want {
		if got[i*2] != hexDigits[b>>4] || got[i*2+1] != hexDigits[b&0xf] {
			return false
		}
	}
	return true
}

// sameCardKey reports whether two cards carry the same key, so that one master
// seated at one slot by two scans of the SAME card is not read as a contest.
func sameCardKey(a, b mk.Card) bool { return a.Xpub == b.Xpub }

func hasStub(stubs [][4]byte, want [4]byte) bool {
	for _, s := range stubs {
		if s == want {
			return true
		}
	}
	return false
}
