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
