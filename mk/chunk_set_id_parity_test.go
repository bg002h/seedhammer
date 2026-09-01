package mk

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// csidParityVectors hand-carries the CLEAN rows (expect_mismatch_warning ==
// false) of the mnemonic-key extension corpus:
// mnemonic-key/crates/mk-codec/src/test_vectors/csid_ext_v0.1.json
// (family_token "mk-codec 0.5", schema 1; 20 of 21 rows are clean, the
// remaining row -- SEED_pinned_12345_ef12f -- is the one deliberate
// mismatch row and is intentionally NOT included here).
//
// This is a Rust-primary convergence test (R4 in
// design/SPEC_chunk_set_id_verification.md, contract 8): Rust already
// derives and pins these ids; this test only asserts the Go port's top20
// reproduces them, per the fork's existing hand-carried parityVectors
// pattern in mk_test.go -- the fork has no JSON reader for the corpus, and
// adding one is a separate post-cycle followup
// (go-mk-vector-corpus-ingestion), not this test's job.
//
// Do NOT "fix" a mismatch here by changing top20 in encode.go -- Rust
// leads; a genuine mismatch is a cross-language drift finding to report,
// not a Go-side bug to patch.
var csidParityVectors = []struct {
	name        string
	bytecodeHex string
	derivedCSID string // 5 lowercase hex digits, zero-padded ({:05x})
}{
	{
		name:        "CT1_twin_of_V1_bip48_mainnet_1_stub_with_fp",
		bytecodeHex: "040111223344aabbccdd050488b21e10203001abababababababababababababababababababababababababababababababab031b84c5567b126440995d3ed5aaba0565d71e1834604819ff9c17f5e9d5dd078f",
		derivedCSID: "83bb2",
	},
	{
		name:        "CT2_twin_of_V2_bip84_mainnet_1_stub_with_fp",
		bytecodeHex: "0401c0ffee00deadbeef030488b21e10203002a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8024d4b6cd1361032ca9bd2aeb9d900aa4d45d9ead80ac9423374c451a7254d0766",
		derivedCSID: "f479a",
	},
	{
		name:        "CT3_twin_of_V3_bip48_testnet_1_stub_with_fp",
		bytecodeHex: "0401778899aa1020304015043587cf10203003a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a902531fe6068134503d2723133227c867ac8fa6c83c537e9a44c3c5bdbdcb1fe337",
		derivedCSID: "c8ea7",
	},
	{
		name:        "SEED_plate_a_1b1ba",
		bytecodeHex: "0401001e5b0ea1a1a1a1050488b21e10203014bebebebebebebebebebebebebebebebebebebebebebebebebebebebebebebebe03ff8adab52623bcb2717fc71d7edc6f55e98396e6c234dff01f307a12b2af1c99",
		derivedCSID: "1b1ba",
	},
	{
		name:        "SEED_plate_b_ef12f",
		bytecodeHex: "0401000c7765b2b2b2b2050488b21e10203015bfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbfbf03d793631af7aa0e709439dd47fc001acd0b0727670b6670ea528ac83cb0127f4a",
		derivedCSID: "ef12f",
	},
	{
		name:        "SP01_std_path_0x01",
		bytecodeHex: "04015350000150500001010488b21e10203040eaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaea0250d1203e168650a52be58141df7b5be8e11f9c38e3ef76bffc9a8225039fcb97",
		derivedCSID: "1b147",
	},
	{
		name:        "SP02_std_path_0x02",
		bytecodeHex: "04015350010250500102020488b21e10203041ebebebebebebebebebebebebebebebebebebebebebebebebebebebebebebebeb02eec7245d6b7d2ccb30380bfbe2a3648cd7a942653f5aa340edcea1f283686619",
		derivedCSID: "2f7ac",
	},
	{
		name:        "SP03_std_path_0x03",
		bytecodeHex: "04015350020350500203030488b21e10203042e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e8e80324653eac434488002cc06bbfb7f10fe18991e35f9fe4302dbea6d2353dc0ab1c",
		derivedCSID: "a5465",
	},
	{
		name:        "SP04_std_path_0x04",
		bytecodeHex: "04015350030450500304040488b21e10203043e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9e9027f31ebc5462c1fdce1b737ecff52d37d75dea43ce11c74d25aa297165faa2007",
		derivedCSID: "290dd",
	},
	{
		name:        "SP05_std_path_0x05",
		bytecodeHex: "04015350040550500405050488b21e10203044eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee032c0b7cf95324a07d05398b240174dc0c2be444d96b159aa6c7f7b1e668680991",
		derivedCSID: "24c9d",
	},
	{
		name:        "SP06_std_path_0x06",
		bytecodeHex: "04015350050650500506060488b21e10203045efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef02edabbd16b41c8371b92ef2f04c1185b4f03b6dcd52ba9b78d9d7c89c8f221145",
		derivedCSID: "d2670",
	},
	{
		name:        "SP07_std_path_0x07",
		bytecodeHex: "04015350060750500607070488b21e10203046ecececececececececececececececececececececececececececececececec024bc2a31265153f07e70e0bab08724e6b85e217f8cd628ceb62974247bb493382",
		derivedCSID: "3c614",
	},
	{
		name:        "SP08_std_path_0x11",
		bytecodeHex: "0401535007115050071111043587cf10203047edededededededededededededededededededededededededededededededed021492bc6a132ac91cb8b9f57d2b809dd2bdb8e1a294d3edbb6c6f7fc03bf11cac",
		derivedCSID: "7e66a",
	},
	{
		// The leading-zero row (r4 L1-I2): derived id < 0x10000, so this
		// row proves the {:05x} zero-padded rendering, not just the raw
		// value.
		name:        "SP09_std_path_0x12",
		bytecodeHex: "0401535008125050081212043587cf10203048e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e2e202dbcfa1c73674cba4aa1b6992ebdc6a77008d38f6c6ec068c3c862b9ff6d287f2",
		derivedCSID: "0012f",
	},
	{
		name:        "SP10_std_path_0x13",
		bytecodeHex: "0401535009135050091313043587cf10203049e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3031cf37eb561f80022293855860f47122025c0929a05f6f08c503ed7f2325cafd5",
		derivedCSID: "eb6ff",
	},
	{
		name:        "SP11_std_path_0x14",
		bytecodeHex: "040153500a1450500a1414043587cf1020304ae0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e00339277f08c34fac33c3b15e58a166a366897665419e5c3f214775ee6e4716717e",
		derivedCSID: "8a576",
	},
	{
		name:        "SP12_std_path_0x15",
		bytecodeHex: "040153500b1550500b1515043587cf1020304be1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e103d32b8a8af7e376739f1675707c8b57c6ad9f010c5ba82c60d973bf7a42be577c",
		derivedCSID: "58204",
	},
	{
		name:        "SP13_std_path_0x16",
		bytecodeHex: "040153500c1650500c1616043587cf1020304ce6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e6e602c91d295563aa25d500374244b0428ed9d7978740d8dae8e466b8a16c15945b37",
		derivedCSID: "ae436",
	},
	{
		name:        "SP14_std_path_0x17",
		bytecodeHex: "040153500d1750500d1717043587cf1020304de7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e7e70371102fc86b5c576c72f411e083cc03eb83d1b55065406ba2a483208dbb5074ab",
		derivedCSID: "55429",
	},
	{
		name:        "LZ1_derived_below_0x10000",
		bytecodeHex: "04010000001170717273040488b21e10203022888888888888888888888888888888888888888888888888888888888888888802466d7fcae563e5cb09a0d1870bb580344804617879a14949cf22285f1bae3f27",
		derivedCSID: "0191c",
	},
}

// csidExtCleanRowCount is the corpus's own clean-row count (measured via
// `jq '[.rows[] | select(.expect_mismatch_warning == false)] | length'` on
// mnemonic-key/crates/mk-codec/src/test_vectors/csid_ext_v0.1.json: 20 of
// 21 rows). Asserting the Go table's length against this constant, rather
// than just iterating the table, makes a silently-dropped row fail the
// test instead of passing with reduced coverage.
const csidExtCleanRowCount = 20

// TestChunkSetIDDerivationParity is the R4 cross-language tripwire: it
// guarantees seedhammer.com/mk's top20(bytecode) reproduces the SAME
// chunk_set_id the Rust mk-codec corpus pins, so the two implementations
// can never drift apart silently (the F-212 lesson -- Go and Rust once
// computed different WalletPolicyIds while every fork test stayed green).
func TestChunkSetIDDerivationParity(t *testing.T) {
	if len(csidParityVectors) != csidExtCleanRowCount {
		t.Fatalf("csidParityVectors has %d rows, want %d (corpus's clean-row count) -- a row was dropped or the corpus grew", len(csidParityVectors), csidExtCleanRowCount)
	}
	for _, v := range csidParityVectors {
		t.Run(v.name, func(t *testing.T) {
			bytecode, err := hex.DecodeString(v.bytecodeHex)
			if err != nil {
				t.Fatalf("hex.DecodeString(bytecodeHex): %v", err)
			}
			got := fmt.Sprintf("%05x", top20(bytecode))
			if got != v.derivedCSID {
				t.Fatalf("top20(bytecode) = %s, want %s (corpus derived_csid)", got, v.derivedCSID)
			}
		})
	}
}
