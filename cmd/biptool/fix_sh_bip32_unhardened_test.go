package main

import "testing"

// FIX-sh-bip32-unhardened (bh-sh-M6): `derive -path` refuses an unmarked
// element of 2^31 or more instead of deriving the hardened child it wraps to.
func TestFixShBip32UnhardenedBiptoolPath(t *testing.T) {
	code, stdout, stderr := fixArgsRun(t, []byte(fixArgsXprv), "derive -path m/83696968h/32h/2147483648 xprv")
	const want = "biptool: bip32: path element out of range: \"2147483648\"\n"
	if code != 2 || stdout != "" || stderr != want {
		t.Errorf("exit %d stdout %q stderr %q; want exit 2, no stdout, stderr %q", code, stdout, stderr, want)
	}
}
