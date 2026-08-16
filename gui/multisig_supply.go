package gui

import "seedhammer.com/md"

// ─── T6b: single-md1 supply filter + full-policy gate ────────────────────────
//
// T6b gathers a SUPPLIED multisig/miniscript wallet-policy md1 over NFC via the
// shipped bundleGatherFlow, then filters it down to EXACTLY ONE descriptor card.
// singleSigReadbackCards is NOT reused (it requires BOTH an mk1 AND an md1,
// gui/singlesig_verify.go:38 — the opposite of a one-md1/zero-mk1 supply, R0-I1).

// extractSuppliedMd1 returns the verbatim chunk strings of EXACTLY one cardMD1
// in the gathered card set (I-1/I-11). It refuses (ok=false) when: there is no
// md1, there are >=2 md1 (ambiguous supply), or any cardMK1/cardMS1 is present
// (polluted supply). The cardMS1 clause is DEFENSIVE — the gather path never
// produces a cardMS1 (ms1 is refused upstream at classify, n-1) — but a stray
// key/secret card must never be silently tolerated alongside the wallet policy.
func extractSuppliedMd1(cards []bundleCard) ([]string, bool) {
	var md1 []string
	count := 0
	for _, c := range cards {
		switch c.kind {
		case cardMD1:
			count++
			md1 = c.strings
		case cardMK1, cardMS1:
			return nil, false // a stray key/secret card pollutes the supply.
		}
	}
	if count != 1 {
		return nil, false // 0 md1 (nothing to engrave) or >=2 (ambiguous).
	}
	return md1, true
}

// extractReadbackMd1AndMk1s is the verify-bundle readback filter (H1): ONE
// wallet-policy md1 and EVERY operator key plate that came back with it, in
// gather order. The read-back mk1s are the operator's ENGRAVED plates, compared
// against the re-derived legs inside verifyMultisigLegs -- NEVER a re-derived
// value against itself. Modeled on singleSigReadbackCards
// (gui/singlesig_verify.go:23).
//
// IT REPLACES extractSuppliedMd1AndMk1, which required EXACTLY ONE mk1 and
// refused a second as ambiguous. That was right while a multisig build could
// engrave only one leg, and it is what made the verify structurally single-leg:
// a build holding three slots cuts three key plates, and a readback filter that
// refuses the second and third guarantees two of them are never checked. Several
// plates is now the normal case, so it is admitted and the BIJECTION in
// verifyMultisigLegs is what resolves them -- each plate to the leg whose key it
// carries, with nothing left over on either side.
//
// This is a distinct helper from extractSuppliedMd1 (the supply/engrave filter,
// which refuses any key card); do NOT widen that one -- it has a second caller
// in the live engrave flow (gui/multisig.go:71).
//
// The md1 stays EXACTLY ONE. Two descriptors is two wallets, and there is no
// answer to "which one did you engrave" that a device may pick for the operator.
// A stray cardMS1 still refuses: the ms1 is hand-typed for verify and never
// scanned (§7.4), so one arriving over the reader means the operator put a
// SECRET plate on a tag, and that is worth stopping for rather than absorbing.
func extractReadbackMd1AndMk1s(cards []bundleCard) (md1 []string, mk1s [][]string, ok bool) {
	for _, c := range cards {
		switch c.kind {
		case cardMD1:
			if md1 != nil {
				return nil, nil, false // more than one descriptor — ambiguous
			}
			md1 = c.strings
		case cardMK1:
			mk1s = append(mk1s, c.strings)
		case cardMS1:
			return nil, nil, false // a stray secret card pollutes the readback.
		}
	}
	if len(md1) == 0 || len(mk1s) == 0 {
		return nil, nil, false
	}
	return md1, mk1s, true
}

// allSlotsHaveXpub is the full-policy gate (I-3): the supplied md1 must be a
// FULL wallet policy — every expanded slot must carry an xpub, else there is no
// public key to cross-match the typed seed against. A template-only md1 (no
// pubkeys) or any-slot-missing-xpub refuses. An empty key set refuses.
func allSlotsHaveXpub(keys []md.ExpandedKey) bool {
	if len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		if !k.XpubPresent {
			return false
		}
	}
	return true
}
