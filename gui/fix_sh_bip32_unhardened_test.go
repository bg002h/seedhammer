package gui

import (
	"errors"
	"testing"

	"seedhammer.com/bip380"
)

// FIX-sh-bip32-unhardened (bh-sh-M6). The grade's scanned text: an origin
// element 2147483648 with no hardened marker. At the NFC scan door it is an
// unknown format (main screen subtitle "Unknown format"), not a descriptor
// carried as origin m/84h/0h/0h.
const fixShBip32K = "tpubD6NzVbkrYhZ4Wgf68pWWfTRzdXMoepBvepfiAKtNoE7RvbAbWVfWzQxH1jbkfNk3iJ9zR65Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ"

func TestFixShBip32UnhardenedScanDoorUnknownFormat(t *testing.T) {
	ok := "wpkh([d34db33f/84h/0h/2147483647]" + fixShBip32K + "/0/*)"
	if obj, err := scanOnce(t, ok); err != nil {
		t.Fatalf("control scan %q: %v", ok, err)
	} else if _, isDesc := obj.(*bip380.Descriptor); !isDesc {
		t.Fatalf("control scan %q = %T, want *bip380.Descriptor", ok, obj)
	}
	for _, s := range []string{
		"wpkh([d34db33f/84h/0h/2147483648]" + fixShBip32K + "/0/*)",
		"wpkh([d34db33f/84h/0h/0h]" + fixShBip32K + "/2147483648/*)",
	} {
		obj, err := scanOnce(t, s)
		if !errors.Is(err, errScanUnknownFormat) {
			if d, isDesc := obj.(*bip380.Descriptor); isDesc {
				t.Errorf("scan %q = descriptor with origin %v children %+v, err %v; want errScanUnknownFormat",
					s, d.Keys[0].DerivationPath, d.Keys[0].Children, err)
			} else {
				t.Errorf("scan %q = %T, err %v; want errScanUnknownFormat", s, obj, err)
			}
		}
	}
}
