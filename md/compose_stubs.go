package md

// ComposerStubs returns the stubs a key card minted or re-minted by the
// composer carries (SPEC_wallet_policy_composer.md §7c, §7d; §12 item 6): the
// composed TEMPLATE's stub always, and the composed KEYED policy's stub after
// seating, so one card seats into either engraved form. Both come from
// FormAwareStubChunks, which is what the shipped seating compares against
// (gui/key_card_seating.go), so a card stamped here seats there.
//
// keyedChunks may be nil (no keyed policy yet). If the two stubs happen to
// coincide the second is not repeated.
func ComposerStubs(templateChunks, keyedChunks []string) ([][4]byte, error) {
	tmpl, err := FormAwareStubChunks(templateChunks)
	if err != nil {
		return nil, err
	}
	out := [][4]byte{tmpl}
	if len(keyedChunks) > 0 {
		pol, err := FormAwareStubChunks(keyedChunks)
		if err != nil {
			return nil, err
		}
		if pol != tmpl {
			out = append(out, pol)
		}
	}
	return out, nil
}
