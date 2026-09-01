package mk

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

// csidCorpusJSON is a byte-for-byte VENDORED copy of the mnemonic-key
// extension corpus, embedded from mk/testdata/csid_ext_v0.1.json. The
// source of truth is the committed, shipped Rust artifact:
//
//	mnemonic-key/crates/mk-codec/src/test_vectors/csid_ext_v0.1.json
//
// (family_token "mk-codec 0.5", schema 1). This replaces the prior
// hand-transcribed literal table (go-mk-vector-corpus-ingestion followup,
// the F-425 "vendored seam" pattern): the test now reads the ACTUAL Rust
// corpus bytes instead of a manual copy that could silently drift if the
// Rust corpus changed underneath it.
//
// Re-vendoring (copying a newer csid_ext_v0.1.json over this file) is a
// DELIBERATE act: it MUST be paired with updating csidCorpusSHA256 below
// in the same change, or csidVendoredCorpusSHA256 test fails and flags the
// drift instead of silently picking up new content.
//
//go:embed testdata/csid_ext_v0.1.json
var csidCorpusJSON []byte

// csidCorpusSHA256 pins the vendored copy's SHA-256 (computed via
// `sha256sum mk/testdata/csid_ext_v0.1.json`). Any edit to the vendored
// file -- accidental or otherwise -- that isn't paired with updating this
// constant fails csidVendoredCorpusSHA256 below.
const csidCorpusSHA256 = "88bbe056e85dde694353475e774a78a00defe75cb8694654c4be1d2467ad68f9"

// csidCorpusRow mirrors the fields of the Rust corpus's row schema that
// this test needs. Other fields present in the JSON (declared_csid,
// description, warning_text, strings) are intentionally omitted -- Go's
// json.Unmarshal ignores unknown fields, so the struct only needs to
// name what it reads.
type csidCorpusRow struct {
	Name                  string `json:"name"`
	CanonicalBytecodeHex  string `json:"canonical_bytecode_hex"`
	DerivedCSID           string `json:"derived_csid"` // 5 lowercase hex digits, zero-padded ({:05x})
	ExpectMismatchWarning bool   `json:"expect_mismatch_warning"`
}

// csidCorpus mirrors the top-level shape of the corpus JSON.
type csidCorpus struct {
	FamilyToken string          `json:"family_token"`
	Rows        []csidCorpusRow `json:"rows"`
	Schema      int             `json:"schema"`
}

// csidExtCleanRowCount is the corpus's own clean-row count (measured via
// `jq '[.rows[] | select(.expect_mismatch_warning == false)] | length'` on
// the vendored copy: 20 of 21 rows). Asserting the parsed row count
// against this constant, rather than just iterating whatever the corpus
// contains, makes a silently-dropped row (or an unexpectedly grown/shrunk
// corpus) fail the test instead of passing with different coverage.
const csidExtCleanRowCount = 20

// TestVendoredCorpusSHA256 is the SHA gate: it asserts the embedded bytes
// of mk/testdata/csid_ext_v0.1.json match the pinned csidCorpusSHA256, so
// an accidental or silent edit to the vendored copy fails loudly instead
// of quietly changing what TestChunkSetIDDerivationParity asserts.
func TestVendoredCorpusSHA256(t *testing.T) {
	got := fmt.Sprintf("%x", sha256.Sum256(csidCorpusJSON))
	if got != csidCorpusSHA256 {
		t.Fatalf("sha256(testdata/csid_ext_v0.1.json) = %s, want %s (pinned) -- the vendored copy changed; if this is a deliberate re-vendor from mnemonic-key/crates/mk-codec/src/test_vectors/csid_ext_v0.1.json, update csidCorpusSHA256 to match", got, csidCorpusSHA256)
	}
}

// TestChunkSetIDDerivationParity is the R4 cross-language tripwire: it
// guarantees seedhammer.com/mk's top20(bytecode) reproduces the SAME
// chunk_set_id the Rust mk-codec corpus pins, so the two implementations
// can never drift apart silently (the F-212 lesson -- Go and Rust once
// computed different WalletPolicyIds while every fork test stayed green).
//
// Do NOT "fix" a mismatch here by changing top20 in encode.go -- Rust
// leads; a genuine mismatch is a cross-language drift finding to report,
// not a Go-side bug to patch.
func TestChunkSetIDDerivationParity(t *testing.T) {
	var corpus csidCorpus
	if err := json.Unmarshal(csidCorpusJSON, &corpus); err != nil {
		t.Fatalf("json.Unmarshal(embedded corpus): %v", err)
	}

	var cleanRows []csidCorpusRow
	for _, row := range corpus.Rows {
		if !row.ExpectMismatchWarning {
			cleanRows = append(cleanRows, row)
		}
	}
	if len(cleanRows) != csidExtCleanRowCount {
		t.Fatalf("corpus has %d clean rows, want %d -- a row was dropped or the corpus grew", len(cleanRows), csidExtCleanRowCount)
	}

	sawLeadingZero := false
	for _, row := range cleanRows {
		row := row
		t.Run(row.Name, func(t *testing.T) {
			bytecode, err := hex.DecodeString(row.CanonicalBytecodeHex)
			if err != nil {
				t.Fatalf("hex.DecodeString(canonical_bytecode_hex): %v", err)
			}
			got := fmt.Sprintf("%05x", top20(bytecode))
			if got != row.DerivedCSID {
				t.Fatalf("top20(bytecode) = %s, want %s (corpus derived_csid)", got, row.DerivedCSID)
			}
		})
		if len(row.DerivedCSID) == 5 && row.DerivedCSID[0] == '0' {
			// derived id < 0x10000: exercises the {:05x} leading-zero
			// rendering path (r4 L1-I2), not just the raw value.
			sawLeadingZero = true
		}
	}
	if !sawLeadingZero {
		t.Fatalf("no clean row in the corpus has a derived_csid < 0x10000 -- the leading-zero rendering path ({:05x}) is no longer exercised")
	}
}
