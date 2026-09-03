package gui

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// The composer's payload fixtures: the smallest records each class has, in
// the wire form `me sysw pack` writes (SPEC §6a: a reserved prefix and a
// lowercase-hex body).
//
// NOTHING SECRET IS COMMITTED. The mnemonic is BIP-39's published "abandon"
// vector, the same one the Rust compose corpus uses; the key records are
// derived from it. The key record's shape is the host's own worked example
// (crates/me-cli/src/sysw/composer_records.rs:284).
//
// THEY ARE BUILT BY THE SAME ENCODING RULE THE HOST APPLIES, not pasted as
// opaque hex, so a reader can see what each record says. The lockstep that
// the DEVICE agrees with the HOST about these bytes is S2's, gated by
// sysw/composer_records_test.go against the vendored 45-row fixture; these
// are for driving screens.
func composerRecord(prefix, text string) string {
	return prefix + hex.EncodeToString([]byte(text))
}

var (
	composerTestKeyRecord  = composerRecord("key:", "[73c5da0a/48'/0'/0'/2']"+composerTestXpubA)
	composerTestKeyRecord2 = composerRecord("key:", "[73c5da0a/48'/0'/1'/2']"+composerTestXpubB)
	composerTestHashRecord = "hash:" + strings.Repeat("ab", 32)
	// 1788220800 is 2026-09-01 00:00:00 UTC, measured, not transcribed.
	composerTestNowRecord = composerRecord("now:", "1788220800,905000")
	// A seed record is the mnemonic itself: ClassMnemonic is SNIFFED, not
	// prefixed (sysw/record.go's classifyConstellation), so no encoding here.
	composerTestMnemonicRecord = "abandon abandon abandon abandon abandon abandon " +
		"abandon abandon abandon abandon abandon about"
	composerTestDescriptorRecord = composerTestDescriptor
)

// The three constants the fixtures above are built from, MEASURED rather than
// transcribed. Each carries the command that produced it, and the commands
// are the plan's, run at plan time -- so a reader can re-run them instead of
// trusting this file.
//
//	ABANDON="abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
//	~/.cargo/bin/ms derive --template bip48-p2wsh --account 0 --phrase "$ABANDON"
//	~/.cargo/bin/ms derive --template bip48-p2wsh --account 1 --phrase "$ABANDON"
//
// Both report master_fingerprint 73c5da0a, at m/48'/0'/0'/2' and
// m/48'/0'/1'/2': ONE master at TWO accounts, which is C5's normal case and
// exactly what the door's "two keys share a fingerprint" row and the mapping
// review's label rule need a fixture for. INVOKE ms BY PATH: in this
// operator's shell a bare `md` is aliased to `mkdir -p`, which exits 0 and
// creates a directory, and a fixture step that reports success while
// producing nothing is the failure this rule exists to prevent.
const (
	composerTestXpubA = "xpub6DkFAXWQ2dHxq2vatrt9qyA3bXYU4ToWQwCHbf5XB2mSTexcHZCeKS1VZYcPoBd5X8yVcbXFHJR9R8UCVpt82VX1VhR28mCyxUFL4r6KFrf"
	composerTestXpubB = "xpub6DzhyrnFFYQ1HimDiM388xHnDiRPNdZJFBmmxge3Y1WWcHLtMJLfRuhRHqnQCPbTj3fGKTuKFLHzzwpJkp5Dtc3UtLKZKaVZe1yqMBXd6Vk"

	// The record the SHIPPED walk already pins, read out of
	// gui/testdata/s2_descriptor_payload.bin (the container `me sysw pack
	// --as descriptor` wrote; gui/wallet_policy_descriptor_walk_test.go:63-93
	// opens it through the firmware's own sysw.Open). Reused rather than
	// minted a second time, so the composer's door tests and the shipped
	// descriptor walk agree about what a Descriptor record is.
	composerTestDescriptor = "wsh(sortedmulti(2," +
		"[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/*," +
		"[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/*," +
		"[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*))#ud8uyjz3"
)

// composerTestPath is a distinct origin per index, for the pick-list
// measurement: 32 rows that are not all the same width.
func composerTestPath(i int) bip32.Path {
	const h = hdkeychain.HardenedKeyStart
	return bip32.Path{48 | h, 0 | h, uint32(i) | h, 2 | h}
}

// composerTestOrigin is §4f's origin for a wrapper and account, built through
// md.DefaultOrigin so the fixture and the production table cannot disagree.
func composerTestOrigin(scriptType, account uint32) []md.PathComponent {
	w := md.ComposeWsh
	switch scriptType {
	case 1:
		w = md.ComposeShWsh
	case 3:
		w = md.ComposeTr
	}
	return md.DefaultOrigin(w, account)
}

// composerPayloadWith wraps records as the session's load takes them.
func composerPayloadWith(public, secret []string) *sysw.Payload {
	return &sysw.Payload{Public: public, Secret: secret}
}

// composerTwoPathList is the four-slot shape most seating tests use: a 2-of-3
// then a single key, under wsh, so slots @0..@2 are path 1 and @3 is path 2.
func composerTwoPathList() md.PathList {
	return md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 1}},
	}}
}

// composerTestMnemonic is the published "abandon" vector as a bip39.Mnemonic.
func composerTestMnemonic(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic(composerTestMnemonicRecord)
	if err != nil {
		t.Fatalf("the fixture mnemonic does not parse: %v", err)
	}
	return m
}

func composerMainNet() *chaincfg.Params { return &chaincfg.MainNetParams }

// TestComposerFixturesClassifyAsTheClassesTheyClaim is the control every
// composer screen test stands on. A fixture that classifies as ClassUnknown
// would make a "the door shows no keys" assertion pass for the wrong reason.
func TestComposerFixturesClassifyAsTheClassesTheyClaim(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record string
		want   sysw.Class
	}{
		{"key", composerTestKeyRecord, sysw.ClassKey},
		{"key 2", composerTestKeyRecord2, sysw.ClassKey},
		{"hash", composerTestHashRecord, sysw.ClassHash},
		{"now", composerTestNowRecord, sysw.ClassNow},
		{"mnemonic", composerTestMnemonicRecord, sysw.ClassMnemonic},
		{"descriptor", composerTestDescriptorRecord, sysw.ClassDescriptor},
		{"malformed key", "key:zz", sysw.ClassUnknown},
		{"malformed hash", "hash:00", sysw.ClassUnknown},
	} {
		if got := sysw.Classify(tc.record); got != tc.want {
			t.Errorf("%s: Classify = %v, want %v -- every test that reads this fixture "+
				"is measuring something else until this passes", tc.name, got, tc.want)
		}
	}
}
