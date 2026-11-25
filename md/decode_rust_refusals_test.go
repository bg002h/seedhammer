package md

import (
	"errors"
	"testing"
)

// Bug hunt 2026-11-25 SH#3: four hand-built md1 cards, each with a valid BCH
// checksum, that the Rust primary refuses to decode and this port used to
// accept (deriving @0 from the overridden <2;3>/* in the first). The refusals
// are md-codec's: validate_use_site_overrides_canonical (0.37.0),
// validate_no_empty_origin_overrides (0.42.0) and tlv.rs reject_non_zero_pad
// (0.41.0). Each string's Rust verdict was taken from `md decode` (md-cli
// built from descriptor-mnemonic, md-codec 0.48.7) and is quoted below.
func TestDecodeRefusesWhatRustRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, card string
		want       error
		rust       string
	}{
		{"baseline-override", "md1yq802gggqpsqpvupgtqup0mzd8ztny0l", errBaselineUseSiteOverride,
			"use-site override keyed on baseline @0; @0 cannot be overridden"},
		{"redundant-override", "md1ypfdsssj5qqcy8ppgzcuqqcraaev3s8l3skq", errRedundantUseSiteOverride,
			"redundant use-site override for @1; equals the baseline use-site path"},
		{"empty-origin-override", "md1yq802gggqpsqvuqx24ty777vsm0p", errEmptyOriginOverride,
			"origin-path override for @0 is present but empty (zero components)"},
		{"dirty-pad", "md1yq802gggqpsphn65anvqptgep", errMalformedPayloadPadding,
			"malformed payload padding: 2 trailing pad bit(s) were not all zero"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.card)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Decode(%s) = %v, want %v (Rust: %q)", tc.card, err, tc.want, tc.rust)
			}
		})
	}
	// The same payload with a ZERO pad is a good card in both implementations
	// (Rust: wpkh(@0/<0;1>/*)), so the pad check refuses the bits, not the card.
	if _, err := Decode("md1yq802gggqpsqwgtua24e7ssf3"); err != nil {
		t.Fatalf("the clean-pad twin no longer decodes: %v", err)
	}
}
