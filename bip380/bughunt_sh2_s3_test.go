package bip380

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

const (
	bhKeyA = "[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*"
	bhKeyB = "[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/0/*"
)

// bhDepth0Xpub returns the xpub of a depth-0 (master) key made from a fixed
// test seed: depth 0, parent fingerprint 0, child number 0.
func bhDepth0Xpub(t *testing.T, fill byte) string {
	t.Helper()
	mk, err := hdkeychain.NewMaster(bytes.Repeat([]byte{fill}, 32), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := mk.Neuter()
	if err != nil {
		t.Fatal(err)
	}
	return pub.String()
}

// Bug 1: ParseKey throws away the xpub's depth and child number, and
// Key.ExtendedKey rebuilds them from DerivationPath, so a different xpub string
// is re-encoded from the one that was entered.
func TestBughuntSh201XpubDepthChildRewritten(t *testing.T) {
	x1 := bhDepth0Xpub(t, 0x11)
	x2 := bhDepth0Xpub(t, 0x22)
	cases := []struct{ name, desc string }{
		{"no origin", fmt.Sprintf("wsh(sortedmulti(1,%s,%s))", x1, x2)},
		{"origin of the wrong length", fmt.Sprintf("wsh(sortedmulti(1,[aabbccdd/48h/0h/0h/2h]%s,[11223344/48h/0h/0h/2h]%s))", x1, x2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.desc)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(d.Keys) != 2 {
				t.Fatalf("got %d keys, want 2", len(d.Keys))
			}
			for i, want := range []string{x1, x2} {
				if got := d.Keys[i].String(); got != want {
					t.Errorf("key %d re-encodes to a different xpub:\n got  %s\n want %s", i, got, want)
				}
				if enc := d.EncodeNoChecksum(); !strings.Contains(enc, want) {
					t.Errorf("EncodeNoChecksum() does not contain the xpub that was entered (%s):\n%s", want, enc)
				}
			}
		})
	}
}

// Bug 6a: Parse accepts a multisig threshold of 0, a negative number, or more
// than the key count.
func TestBughuntSh206aParseThreshold(t *testing.T) {
	// Positive control: a sane threshold parses.
	if _, err := Parse(fmt.Sprintf("wsh(sortedmulti(2,%s,%s))", bhKeyA, bhKeyB)); err != nil {
		t.Fatalf("control 2-of-2 refused: %v", err)
	}
	for _, k := range []string{"5", "3", "0", "-1"} {
		desc := fmt.Sprintf("wsh(sortedmulti(%s,%s,%s))", k, bhKeyA, bhKeyB)
		d, err := Parse(desc)
		if err == nil {
			t.Errorf("threshold %s of 2 keys accepted: Threshold=%d, %d keys", k, d.Threshold, len(d.Keys))
		}
	}
}

// Bug 6 ("Also"): Parse accepts sortedmulti under pkh, wpkh and tr, and wsh
// with a single bare key.
func TestBughuntSh206cScriptTypeMismatch(t *testing.T) {
	// Positive controls: the legal shapes parse.
	for _, ok := range []string{
		fmt.Sprintf("wpkh(%s)", bhKeyA),
		fmt.Sprintf("pkh(%s)", bhKeyA),
		fmt.Sprintf("sh(sortedmulti(1,%s,%s))", bhKeyA, bhKeyB),
		fmt.Sprintf("wsh(sortedmulti(1,%s,%s))", bhKeyA, bhKeyB),
		fmt.Sprintf("sh(wsh(sortedmulti(1,%s,%s)))", bhKeyA, bhKeyB),
	} {
		if _, err := Parse(ok); err != nil {
			t.Fatalf("control %q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		fmt.Sprintf("wpkh(sortedmulti(1,%s,%s))", bhKeyA, bhKeyB),
		fmt.Sprintf("pkh(sortedmulti(1,%s,%s))", bhKeyA, bhKeyB),
		fmt.Sprintf("tr(sortedmulti(1,%s,%s))", bhKeyA, bhKeyB),
		fmt.Sprintf("wsh(%s)", bhKeyA),
	} {
		if d, err := Parse(bad); err == nil {
			t.Errorf("%.40s... accepted: Script=%v Type=%v keys=%d", bad, d.Script, d.Type, len(d.Keys))
		}
	}
}
