package gui

import "testing"

// skipUnderRefugium is how a DEFAULT-build test steps aside under -tags
// refugium (Refugium plan F7 §4.5, R0 M-4). It skips only when the profile is
// on, so the default suite runs every one of these tests as before; and the
// skip message always starts "refugium profile:", which is what
// scripts/refugium-tagged-test.sh accepts and counts -- any other SKIP fails
// that run.
//
// A test skips for one of the reasons below, each a behaviour the Refugium
// build changes on purpose. The Refugium side of each is pinned by
// refugium_profile_test.go.
func skipUnderRefugium(t testing.TB, why string) {
	t.Helper()
	if refugiumProfile {
		t.Skip("refugium profile: " + why)
	}
}

const (
	// §4.3: the BIP-39 passphrase program is out of the carousel, so every
	// program after it sits one Right earlier than these tests count.
	refugiumSkipCarousel = "the passphrase program is hidden, so carousel positions shift (F7 §4.3)"
	// §4.2: no tag is read, and the scan rows/offers are gone, so a test that
	// presents a tag or selects a row by its index is driving another build.
	refugiumSkipNFC = "no tag is read and no scan is offered (F7 §4.2)"
	// §4.2: both verify flows refuse the readback before the seed is retyped.
	refugiumSkipVerify = "verify refuses the readback: this build reads no card (F7 §4.2)"
	// §4.3: the passphrase question is a notice, passphrase entry returns
	// nothing, and a pass: payload is refused at load.
	refugiumSkipPassphrase = "the BIP-39 passphrase prompt is a notice and no passphrase is taken (F7 §4.3)"
	// §4.3: a SLIP-39 passphrase ends the recovery.
	refugiumSkipSLIP39 = "a SLIP-39 passphrase ends the recovery (F7 §4.3)"
)
