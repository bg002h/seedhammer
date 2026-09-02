package mk

// AppendStubs returns a copy of card carrying its existing stubs, in order,
// followed by each given stub it does not already carry (SPEC_wallet_policy_composer.md
// §7d: a seated card "is later cut as a RE-MINTED mk1 carrying BOTH the composed
// template's stub and the composed policy's stub APPENDED to its existing
// stubs", so it stays indexed to the wallets it already belonged to). The input
// is not mutated; Encode's stub_count bound (<= 255) is enforced by Encode.
func AppendStubs(card Card, stubs ...[4]byte) Card {
	out := card
	out.Stubs = make([][4]byte, 0, len(card.Stubs)+len(stubs))
	out.Stubs = append(out.Stubs, card.Stubs...)
	for _, s := range stubs {
		dup := false
		for _, have := range out.Stubs {
			if have == s {
				dup = true
				break
			}
		}
		if !dup {
			out.Stubs = append(out.Stubs, s)
		}
	}
	return out
}
