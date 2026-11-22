package address

import (
	"errors"
	"testing"

	"seedhammer.com/bip380"
)

// FIX-sh-address-hardened-usesite (bh-sh-C2, sweep-1122 row 20, ruling T4).
// A hardened use-site step below an extended PUBLIC key cannot be derived
// (BIP-32 CKDpub has no hardened case); BIP-380 L82 says `/*h` means hardened
// children. Deriving the unhardened child instead shows a wrong address.

const fixShK = "tpubD6NzVbkrYhZ4Wgf68pWWfTRzdXMoepBvepfiAKtNoE7RvbAbWVfWzQxH1jbkfNk3iJ9zR65Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ"

// The address the device showed for wpkh(K/0h/*) at index 0 before the fix: it
// is the /0/* receive address, so Find "verified" it as the hardened wallet's.
const fixShK00 = "tb1qnskw4kf6qevrcyjjd7cnn39uemz9j2cnaxa4u3"

const fixShHardenedErr = "address: hardened use-site path element: unsupported descriptor"

func fixShParse(t *testing.T, s string) *bip380.Descriptor {
	t.Helper()
	d, err := bip380.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%s): %v", s, err)
	}
	return d
}

func TestFixShHardenedUseSiteRefused(t *testing.T) {
	for _, tc := range []struct{ name, s string }{
		{"wpkh 0h", "wpkh(" + fixShK + "/0h/*)"},
		{"wpkh 0 apostrophe", "wpkh(" + fixShK + "/0'/*)"},
		{"wpkh wildcard h", "wpkh(" + fixShK + "/*h)"},
		{"wpkh wildcard apostrophe", "wpkh(" + fixShK + "/*')"},
		{"wpkh 0 then wildcard h", "wpkh(" + fixShK + "/0/*h)"},
		{"wpkh fixed 5h", "wpkh(" + fixShK + "/5h)"},
		{"pkh 0h", "pkh(" + fixShK + "/0h/*)"},
		{"sh-wpkh wildcard h", "sh(wpkh(" + fixShK + "/*h))"},
		{"tr 0h", "tr(" + fixShK + "/0h/*)"},
		{"wsh sortedmulti one hardened seat", "wsh(sortedmulti(1," + fixShK + "/1/*," + fixShK + "/0h/*))"},
		{"wpkh non-consecutive range then wildcard h", "wpkh(" + fixShK + "/<0;2>/*h)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixShParse(t, tc.s)
			for _, change := range []bool{false, true} {
				a, err := addressAt(d, 0, change)
				if err == nil {
					t.Errorf("change=%v: derived %q, want refusal", change, a)
					continue
				}
				if !errors.Is(err, errUnsupported) || err.Error() != fixShHardenedErr {
					t.Errorf("change=%v: err %q (Is errUnsupported=%v), want %q",
						change, err, errors.Is(err, errUnsupported), fixShHardenedErr)
				}
			}
			if Supported(d) {
				t.Errorf("Supported = true, want false")
			}
			if c, i, found, err := Find(d, fixShK00, 20); found || !errors.Is(err, errUnsupported) {
				t.Errorf("Find = chain %d index %d found %v err %v, want not found and errUnsupported", c, i, found, err)
			}
		})
	}
}

// Controls: the closed admitted set still derives, with the literal addresses.
func TestFixShUnhardenedUseSiteUnchanged(t *testing.T) {
	for _, tc := range []struct{ d, want string }{
		{"wpkh(" + fixShK + "/0/*)", fixShK00},
		{"wpkh(" + fixShK + "/*)", "tb1qhdgyak30llluz36r6lk5ywup5uq0jz0ejkztjl"},
	} {
		d := fixShParse(t, tc.d)
		if a, err := Receive(d, 0); err != nil || a != tc.want {
			t.Errorf("%s: Receive(0) = %q %v, want %q", tc.d, a, err, tc.want)
		}
		if !Supported(d) {
			t.Errorf("%s: Supported = false, want true", tc.d)
		}
	}
}

// DeriveChild is the exported door into derivePubKey (the md witness-script and
// taproot script-path emitters use it); it refuses the same steps.
func TestFixShDeriveChildRefusesHardened(t *testing.T) {
	for _, s := range []string{"/0h/*", "/*h", "/5h"} {
		k := fixShParse(t, "wpkh("+fixShK+s+")").Keys[0]
		for _, change := range []bool{false, true} {
			pub, err := DeriveChild(k, 0, change)
			if pub != nil || !errors.Is(err, errUnsupported) {
				t.Errorf("%s change=%v: DeriveChild = %v %v, want nil and errUnsupported", s, change, pub != nil, err)
			}
		}
	}
}
