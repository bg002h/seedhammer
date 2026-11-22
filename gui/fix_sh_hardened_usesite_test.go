package gui

import (
	"testing"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
)

// FIX-sh-address-hardened-usesite, the scan-door half (sweep-1122 EN-Q7): a
// scanned descriptor reaches the descriptor screen (engraveObjectFlow, the only
// consumer of a scanned *bip380.Descriptor) only if it passes the same use-site
// conjunct as the payload door (sysw.UseSitesOK, SPEC_descriptor_input.md §4.7
// conjunct 7). A refused one returns false, which the main screen reads as
// "Unknown format" (gui.go scanUnknownFormat), as sysw makes it ClassUnknown.
//
// Back is queued first, so a descriptor that wrongly opens the screen backs out
// and returns true instead of hanging the test.

const fixShScanK = "tpubD6NzVbkrYhZ4Wgf68pWWfTRzdXMoepBvepfiAKtNoE7RvbAbWVfWzQxH1jbkfNk3iJ9zR65Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ"

func fixShScanForms(useSite string) map[string]string {
	k := fixShScanK + useSite
	return map[string]string{
		"wpkh":     "wpkh(" + k + ")",
		"multisig": "wsh(sortedmulti(1," + k + "," + fixShScanK + "/9/*))",
	}
}

// fixShScanned runs the real scanner over the descriptor text (so the object is
// what an NFC tag produces), then hands it to the engrave dispatch.
func fixShScanned(t *testing.T, d string) (opened bool) {
	t.Helper()
	obj, err := scanOnce(t, d)
	desc, ok := obj.(*bip380.Descriptor)
	if !ok || err != nil {
		t.Fatalf("scan %s = %T %v, want *bip380.Descriptor (the scanner is unchanged)", d, obj, err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1)
	return engraveObjectFlow(ctx, &descriptorTheme, desc)
}

func TestFixShScanDoorRefusesUseSite(t *testing.T) {
	for _, us := range []string{
		"/0h/*", "/0'/*", "/*h", "/*'", "/0/*h", "/5h", // hardened (the defect)
		"/5", "/0/1", "/0/1/*", "/<0;2>/*", "/<0;1>/5", // outside the closed set
	} {
		for form, d := range fixShScanForms(us) {
			if fixShScanned(t, d) {
				t.Errorf("use-site %q (%s): engraveObjectFlow = true (descriptor screen opened), want false (Unknown format)", us, form)
			}
		}
	}
}

func TestFixShScanDoorAdmitsClosedSet(t *testing.T) {
	for _, us := range []string{"", "/*", "/0/*", "/<0;1>", "/<0;1>/*"} {
		for form, d := range fixShScanForms(us) {
			if !fixShScanned(t, d) {
				t.Errorf("use-site %q (%s): engraveObjectFlow = false, want true (descriptor screen opened, Back)", us, form)
			}
		}
	}
}

// Moved out of TestEverySpellingOfTheSameKeyIsCaught: a hardened spelling no
// longer derives, so it no longer repeats a key, and the device shows no address
// for it at all (address.Supported false), which is the stronger refusal.
func TestFixShHardenedSpellingDerivesNothing(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{"hardened vs not", f530XpubA + "/0h/*", f530XpubA + "/0/*"},
		{"hardened wildcard vs not", f530XpubA + "/*h", f530XpubA + "/*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desc := loadTestDesc(t, "wsh(sortedmulti(2,"+tc.a+","+tc.b+","+f530XpubB+"))")
			if _, _, dup := descriptorRepeatsAKey(desc); dup {
				t.Errorf("descriptorRepeatsAKey = true, want false: the hardened seat derives no key")
			}
			if address.Supported(desc) {
				t.Errorf("address.Supported = true, want false: a hardened use-site step cannot be derived from an xpub")
			}
		})
	}
}
