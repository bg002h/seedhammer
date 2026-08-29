package bip380

import (
	"encoding/hex"
	"strings"
	"testing"
)

// F-426, THE DEVICE HALF: `ypub` had a version constant and a case in the
// NORMALISATION switch, and no case in the CLASSIFICATION switch -- so it fell
// to `default` and this parser refused the commonest non-`xpub` key there is,
// even with a full explicit origin. `ParseExtendedKey`'s `ypubVer` arm closes
// that, and these two tests are the direction pair it needs.
//
// THE HOST'S ADMISSION IS UNCHANGED IN S2, and that is not an omission.
// SPEC_descriptor_input.md §4.3 is NORMATIVE that `me` admits exactly five
// versions -- xpub, tpub, zpub, Ypub, Zpub -- and `ypub` is not one; measured
// against the S2 `me`, both spellings of this very key refuse at rc 3
// ("`me` admits exactly `xpub`, `tpub`, `zpub`, `Ypub`, `Zpub`. This key is
// `ypub`, whose equivalent is `xpub`: ..."), bare and inside a descriptor.
// The convergence widening of the host is F-426's own LATER cycle.
//
// (The quote was "the device admits exactly ..." when P3.4 landed, and it was
// true then. P3.5's fold re-subjected the message -- the sentence is about what
// `me` admits, not about the device -- and falsified this comment from the
// other repo, without its diff touching this file. Re-measured against the
// engrave tree at P3.5, and this is what `me` prints now.)
//
// So this arm widens the SCAN DOOR only, and that is the seam-SAFE direction:
// the file's invariant is host_admits(input) => device_admits(canonical), and
// a device that accepts MORE can never be handed a payload it cannot read.
// Where Rust-primary binds -- the sysw CLASSIFIER, which must answer §5.2's
// predicate exactly -- parity is held by a string-level version check that
// refuses `ypub` on both the bare-key and descriptor-embedded paths
// (sysw/descriptor.go; its bare-key negative is TestBareYpubClassifiesUnknown,
// which is the one place that case lives).

// The same key material in both spellings: `ypub` and `xpub` differ in the four
// version bytes and nothing else, which is what makes the normalisation
// assertion below an equality rather than a resemblance.
const (
	ypubFixture = "ypub6WyzNbqt7S3quv2F6R9eyqabYpQieQKk7P9uufmRv2LjhLjskjho9N1sEmTvAXSURk5eF9UdiS2jqgLXM3gpHeExWDvj1KyiEaqi47h3Ef1"
	xpubTwin    = "xpub6C9j4wAxxkWN4cq8G4N2mkV6NrGGhnLFCGdh8GsYY1xreEveW5YEXJMjDZWLAcnZ26xqVft5FmgBxPixdMGoVQZMdtEJRRADxrn4facoGnx"
	// The same 78 bytes again under `upub`'s version (044a5262), re-checksummed
	// -- the neighbour the switch must keep refusing.
	upubTwin = "upub5Dew9wADWhsvWjFmkz1A9VCarwpvsvMkSw52n6BtPzqDUwUxk73Yf7PK9wdaAtpnoBcRFF6PsncYJXtGUG2m6hWZ2s92fghm9gb8Voj5yXL"
	// A DIFFERENT key: the seam file's promotion/04-bare-Ypub-refused row.
	capYpubFixture = "Ypub6ht5VqaKgPcDLVBd35cdouvQGcSyrm1LReoapw2yHoB9KXJnX965EUso3URPixfNfD9d7jUkbeRExqxHeGqmS8MdLh38QjSi8K7ae5rcihQ"
)

// Direction one: it CLASSIFIES. `ypub` is SLIP-132 for BIP-49, so its implied
// script is P2SH_P2WPKH -- the `sh(wpkh(...))` form, and not P2SH_P2WSH, which
// is `Ypub`'s. The two spellings differ by one capital letter and the two
// scripts differ by a whole wallet, so the arm asserts WHICH script it returns
// and not merely that it returned one.
func TestParseExtendedKeyClassifiesYpub(t *testing.T) {
	script, xpub, err := ParseExtendedKey(ypubFixture)
	if err != nil {
		t.Fatalf("ParseExtendedKey(ypub) = %v -- F-426's ypubVer case is gone", err)
	}
	if script != P2SH_P2WPKH {
		t.Errorf("ypub implies %v, want P2SH_P2WPKH (BIP-49). Ypub's P2SH_P2WSH is a "+
			"DIFFERENT wallet reached by one capital letter.", script)
	}
	if xpub == nil {
		t.Fatal("ParseExtendedKey returned no key")
	}
	// The near neighbour, kept adjacent so a future edit cannot merge the two
	// arms: capital `Ypub` was already classified and must stay where it was.
	if s, _, err := ParseExtendedKey(capYpubFixture); err != nil || s != P2SH_P2WSH {
		t.Errorf("Ypub classifies as %v (err %v), want P2SH_P2WSH -- unchanged by F-426", s, err)
	}
	// And the version this switch must still REFUSE, so the widening is known
	// to be ONE case and not a door. `upub` is testnet BIP-49 -- the nearest
	// unclassified neighbour, and the same 78 bytes as the ypub fixture with
	// only the four version bytes changed, so what refuses it is the switch.
	if _, _, err := ParseExtendedKey(upubTwin); err == nil {
		t.Error("ParseExtendedKey(upub) now ACCEPTS -- F-426 is one case, not a door")
	}
}

// Direction two: it NORMALISES to `xpub`. The switch that does this already
// listed `ypubVer`, and was dead for that version because classification
// returned an error first. Now it runs -- so the Key that reaches a descriptor
// carries mainnet version bytes and re-serialises to the xpub twin, which is
// exactly why sysw's §4.3 check cannot be a conjunct over the parsed value.
func TestParseExtendedKeyNormalisesYpubToXpub(t *testing.T) {
	_, xpub, err := ParseExtendedKey(ypubFixture)
	if err != nil {
		t.Fatalf("ParseExtendedKey(ypub): %v", err)
	}
	if got := hex.EncodeToString(xpub.Version()); got != "0488b21e" {
		t.Errorf("version after normalisation = %s, want 0488b21e (xpub)", got)
	}
	if got := xpub.String(); got != xpubTwin {
		t.Errorf("normalised key = %s, want %s -- same material, xpub spelling", got, xpubTwin)
	}
	// The control, which is what makes the equality above mean "normalised" and
	// not "happens to match": the two fixtures really are different strings.
	if ypubFixture == xpubTwin || !strings.HasPrefix(ypubFixture, "ypub") {
		t.Fatal("the fixtures are not a ypub/xpub pair")
	}
}

// The whole-key-expression path, because ParseKey is what a descriptor reaches
// and it adds the SLIP-132 fallback: a `ypub` with no explicit origin now
// implies 49'/0'/0'. Before F-426 this errored before the fallback was reached.
func TestParseKeyAcceptsABareYpubAndImpliesBIP49(t *testing.T) {
	k, err := ParseKey(nil, []byte(ypubFixture))
	if err != nil {
		t.Fatalf("ParseKey(bare ypub): %v", err)
	}
	want := P2SH_P2WPKH.DerivationPath()
	if len(k.DerivationPath) != len(want) {
		t.Fatalf("implied path %v, want %v", k.DerivationPath, want)
	}
	for i := range want {
		if k.DerivationPath[i] != want[i] {
			t.Fatalf("implied path %v, want %v", k.DerivationPath, want)
		}
	}
	if got := k.String(); got != xpubTwin {
		t.Errorf("ParseKey normalised to %s, want %s", got, xpubTwin)
	}
}
