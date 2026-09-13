package sysw

import (
	"testing"

	"seedhammer.com/bip380"
	"seedhammer.com/nonstandard"
)

// The four inputs §4 measured as the descriptor arm's hazards, asserted here
// as CLASSIFICATIONS rather than as parser verdicts -- because the classifier
// is what decides whether a program is ever handed the string.
//
// Two of them used to CRASH rather than refuse, and a crash inside
// sysw.Classify is a payload that will not load: it runs over every record of
// every loaded payload (gui/sysw_session.go). The other two are the pair the
// vector file cannot instrument, one per cascade path.

// keys reused below. SKXP is the fixture xpub; SKYL is its ypub twin and SKTP a
// testnet key -- the same material the seam vectors carry, so a divergence here
// and a divergence there have the same cause.
const (
	skXP = "xpub6C9j4wAxxkWN4cq8G4N2mkV6NrGGhnLFCGdh8GsYY1xreEveW5YEXJMjDZWLAcnZ26xqVft5FmgBxPixdMGoVQZMdtEJRRADxrn4facoGnx"
	skYL = "ypub6WyzNbqt7S3quv2F6R9eyqabYpQieQKk7P9uufmRv2LjhLjskjho9N1sEmTvAXSURk5eF9UdiS2jqgLXM3gpHeExWDvj1KyiEaqi47h3Ef1"
	skTP = "tpubDCXMbAzeg2TpLR1yiFM7yfpThyMvhAqJjuDzUpvgsvikPXbMaJPKfk2ZTbb7h7jnp1Vk7FPwnsWEeaDa2D83Nr1ehUyc6wpTYpNURb6Qt26"
)

// A ONE-LINE record with a two-character fingerprint header. Before
// nonstandard/parse.go's `len(fp) != 4` guard this PANICKED inside
// binary.BigEndian.Uint32 (§4.2 defect 4), which the classifier reaches on
// every record of every payload -- so the panic was a payload that would not
// load, from one line of text.
func TestShortFingerprintHeaderClassifiesUnknownWithoutPanicking(t *testing.T) {
	if got := Classify("ab: " + skXP); got != ClassUnknown {
		t.Errorf("Classify(short-fingerprint header) = %v, want ClassUnknown", got)
	}
}

// The titled zero-key BlueWallet shape, on one line. `parseBlueWalletDescriptor`
// ACCEPTS it -- Title is non-empty and 0 declared keys equals 0 parsed keys --
// and the descriptor it produces PANICS Descriptor.Encode (§4.2 defect 1: the
// zero Script has no arm in encode's switch). DescriptorScreen encodes, so the
// only thing between this line and a crashed screen is admission refusing it.
//
// Measured which conjunct refuses it: the SHAPE conjunct, because Script is the
// zero value and is neither a single-key script nor a multisig slot. §4.7's
// threshold conjunct would also refuse it (0 keys, threshold 0), which is why
// the two are stated as a conjunction and not as a cascade of one.
func TestTitledZeroKeyDescriptorClassifiesUnknown(t *testing.T) {
	if got := Classify("Name: my wallet"); got != ClassUnknown {
		t.Errorf("Classify(titled zero-key) = %v, want ClassUnknown", got)
	}
	if admitDescriptor(titledZeroKey(t)) {
		t.Error("§4.7 admitted a descriptor whose Encode panics")
	}
	if descriptorShapeOK(titledZeroKey(t)) {
		t.Error("conjunct 1 must refuse the zero Script")
	}
	if thresholdOK(titledZeroKey(t)) {
		t.Error("conjunct 2 must refuse threshold 0 over 0 keys")
	}
}

// A BARE TPUB. §4.5 is NORMATIVE and a RULING: `me` refuses tpub promotion
// entirely, because a testnet key whose only claim to being a wallet is a
// version byte mapping to the MAINNET path 44'/0'/0' is an inference the host
// declines to make. This device's parser stays WIDER and promotes it, so a
// conjuncts-only port would classify it -- every §4.7 conjunct passes, `tpub`
// being admitted for a key inside a descriptor. It is the one row that proves
// the cascade's narrowings had to be ported too
// (promotion/15-bare-tpub-host-refused).
func TestBareTpubClassifiesUnknown(t *testing.T) {
	if got := Classify(skTP); got != ClassUnknown {
		t.Errorf("Classify(bare tpub) = %v, want ClassUnknown -- §4.5 refuses tpub "+
			"promotion entirely, and this parser does not", got)
	}
	// The control: the same shape with an admitted mainnet version IS a
	// descriptor, so the refusal above is the version byte and nothing else.
	if got := Classify(skXP); got != ClassDescriptor {
		t.Errorf("Classify(bare xpub) = %v, want ClassDescriptor -- the control for "+
			"the tpub case must be admitted", got)
	}
}

// A BARE YPUB, the bare-key half of §4.3's version check.
//
// THIS TEST CAN FAIL, and the reason it can is P3.4: F-426's `ypubVer` arm
// widens THIS device's parser to accept `ypub` (bip380/bip380.go:447), which is
// the seam-safe direction. `me`'s admission is UNCHANGED in S2 -- five versions,
// and `ypub` is not one, measured rc 3 on this exact string -- so a classifier
// that inherited the parser's new answer would place a record the host refuses.
// The version cannot be a conjunct: ParseExtendedKey normalises it to `xpub`
// before a bip380.Key exists, so the check has to read the record's own bytes.
//
// No vector row can see this. §7's ypub row is a WRAPPED descriptor
// (version-gap/full-origin-ypub), so it exercises the embedded path; this is
// the promotion path, and the two reach the version check by different routes.
func TestBareYpubClassifiesUnknown(t *testing.T) {
	if got := Classify(skYL); got != ClassUnknown {
		t.Errorf("Classify(bare ypub) = %v, want ClassUnknown -- §4.3 admits five "+
			"versions and ypub is not one, even though P3.4's ypubVer arm makes "+
			"this device's own parser accept it", got)
	}
}

// The embedded half of the same check, so the sentence "on BOTH the
// descriptor-embedded and bare-key paths" is falsifiable rather than asserted.
func TestFullOriginYpubDescriptorClassifiesUnknown(t *testing.T) {
	rec := "sh(wpkh([4bbaa801/49h/0h/0h]" + skYL + "/<0;1>/*))"
	if got := Classify(rec); got != ClassUnknown {
		t.Errorf("Classify(full-origin ypub) = %v, want ClassUnknown", got)
	}
	// The control, from §4.3's own measured pair: the xpub twin of the same
	// wallet is admitted, so what refuses above is the version byte.
	twin := "sh(wpkh([4bbaa801/49h/0h/0h]" + skXP + "/<0;1>/*))"
	if got := Classify(twin); got != ClassDescriptor {
		t.Errorf("Classify(xpub twin) = %v, want ClassDescriptor", got)
	}
}

// titledZeroKey is `Name: my wallet` as the cascade parses it, so the conjunct
// assertions above run against the real object rather than a hand-built one.
func titledZeroKey(t *testing.T) *bip380.Descriptor {
	t.Helper()
	d, err := nonstandard.OutputDescriptor([]byte("Name: my wallet"))
	if err != nil {
		t.Fatalf("the BlueWallet branch no longer accepts a titled zero-key file: %v.\n"+
			"That is a behaviour change in nonstandard/parse.go, not a test bug -- "+
			"re-read §4.2 defect 1 before editing this test.", err)
	}
	return d
}

// ─── F-530 review C-2: conjunct 8(b) compared spelling ───────────────────────
//
// keyIdentityOK read slices.Equal(a.Children, b.Children), and
// address.derivePubKey normalises an ABSENT derivation into <0;1>/* before
// deriving. So the same wallet written two ways was admitted as a two-key
// multisig while deriving one key at both seats -- and the gui gate added for
// F-530 fired only on descriptors admission had ALREADY rejected, leaving this
// route with nothing it actually caught.
//
// CONVERGENCE PORT. The primary carried the identical defect at
// me-cli/src/descriptor/admit.rs conjunct 8(b); it was fixed there first, with
// the vector gate/duplicate-key-implicit-use-site, and this followed.
func TestAdmissionRefusesEverySpellingOfADuplicateKey(t *testing.T) {
	const (
		xpubA = "xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan"
		oriA  = "[dc567276/48h/0h/0h/2h]"
	)
	for _, tc := range []struct {
		name  string
		desc  string
		admit bool
		why   string
	}{
		{"explicit twice", "wsh(sortedmulti(2," + oriA + xpubA + "/<0;1>/*," + oriA + xpubA + "/<0;1>/*))",
			false, "the spelling-equal case, refused before this fix too"},
		{"implicit twice", "wsh(sortedmulti(2," + oriA + xpubA + "," + oriA + xpubA + "))",
			false, "also spelling-equal: both absent"},
		{"implicit vs explicit", "wsh(sortedmulti(2," + oriA + xpubA + "," + oriA + xpubA + "/<0;1>/*))",
			false, "THE BYPASS: an absent path IS <0;1>/*, so these are one key twice"},
		{"child vs range on receive", "wsh(sortedmulti(2," + oriA + xpubA + "/0/*," + oriA + xpubA + "/<0;1>/*))",
			false, "a range resolves to Index on the receive chain -- the chain that gets funded"},
		{"disjoint multipath", "wsh(sortedmulti(2," + oriA + xpubA + "/<0;1>/*," + oriA + xpubA + "/<2;3>/*))",
			true, "BIP 388 permits one key at DISJOINT multipath sets, and this conjunct's own comment says so"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := nonstandard.OutputDescriptor([]byte(tc.desc))
			if err != nil {
				t.Fatalf("OutputDescriptor: %v", err)
			}
			if got := admitDescriptor(d); got != tc.admit {
				verb := "admits"
				if tc.admit {
					verb = "refuses"
				}
				t.Errorf("§4.7 admission %s this descriptor; %s", verb, tc.why)
			}
		})
	}
}
