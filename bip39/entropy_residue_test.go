package bip39

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// ---------------------------------------------------------------------------
// F-104 item 2 -- the entropy residue splitMnemonic and its callers leave
// behind. `entBytes` is the one ZEROABLE []byte in that group; the math/big nat
// beside it is not zeroable and is deliberately out of scope here (it stays
// filed).
//
// Each buffer is a local of a function that returns something else -- a bool, a
// Mnemonic, or a differently-allocated slice -- so nothing outside could ever
// observe it, which is exactly why the clear() beside it would otherwise be
// deletable with the whole suite green. entropyResidueHook hands over the slice
// VALUE while it is still populated, and a Go slice value carries the pointer
// to its backing array: the read after the call therefore observes the SAME
// allocation, not a fresh one.
//
// Every test below establishes the buffer really held the entropy -- by
// comparing against the published BIP-39 vector, not by inspection -- before
// any all-zero read is allowed to mean anything.
// ---------------------------------------------------------------------------

// BIP-39 English test vector 2, taken from this package's own corpus
// (testVectors[1]). Its entropy has no leading zero byte, so big.Int.Bytes()
// returns it unshortened and `splitMnemonic.raw` must equal it exactly.
const (
	residueMnemonic = "legal winner thank year wave sausage worth useful legal winner thank yellow"
	residueEntropy  = "7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f"
)

// residueRec records every entropy buffer the hook hands over: the live slice
// (same backing array) and a snapshot of its contents at hand-over time.
type residueRec struct {
	where string
	buf   []byte
	live  []byte
}

func watchEntropyResidue(t *testing.T) *[]residueRec {
	t.Helper()
	got := new([]residueRec)
	entropyResidueHook = func(where string, ent []byte) {
		*got = append(*got, residueRec{where, ent, append([]byte(nil), ent...)})
	}
	t.Cleanup(func() { entropyResidueHook = nil })
	return got
}

func nonZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return true
		}
	}
	return false
}

// assertAllZeroed is the shared post-condition: every buffer handed over must
// read all-zero now, and at least one of them must have been non-zero when it
// was handed over -- otherwise the whole assertion is vacuous.
func assertAllZeroed(t *testing.T, recs []residueRec) {
	t.Helper()
	if len(recs) == 0 {
		t.Fatal("entropyResidueHook never fired -- this test asserted nothing")
	}
	sawLive := 0
	for _, r := range recs {
		if nonZero(r.live) {
			sawLive++
		}
	}
	if sawLive == 0 {
		t.Fatal("every buffer handed over was already all-zero when it was handed over; " +
			"the assertions below cannot distinguish a wipe from an empty buffer")
	}
	for i, r := range recs {
		if nonZero(r.buf) {
			t.Fatalf("buffer %d (%s) still holds %x after the call returned: the clear() "+
				"beside it did not run, and the seed entropy is live as unreachable heap "+
				"garbage (F-104 item 2)", i, r.where, r.buf)
		}
	}
}

// TestValidZeroesTheDiscardedEntropy pins the clear() in Valid and the one in
// splitMnemonic. Valid returns a bool; both entropy buffers it produced are
// discarded, and both must read zero once it returns.
func TestValidZeroesTheDiscardedEntropy(t *testing.T) {
	m, err := ParseMnemonic(residueMnemonic)
	if err != nil {
		t.Fatal(err)
	}
	want, err := hex.DecodeString(residueEntropy)
	if err != nil {
		t.Fatal(err)
	}

	recs := watchEntropyResidue(t)
	if !m.Valid() {
		t.Fatal("the vector mnemonic did not validate; wrong fixture")
	}

	byWhere := map[string]residueRec{}
	for _, r := range *recs {
		byWhere[r.where] = r
	}
	// POSITIVE CONTROL: what the hook handed over really was this mnemonic's
	// entropy, so the zero-read below is evidence of a wipe rather than of the
	// test watching the wrong buffer.
	if r, ok := byWhere["Valid.ent"]; !ok {
		t.Fatal("Valid.ent was never handed over")
	} else if !bytes.Equal(r.live, want) {
		t.Fatalf("Valid.ent held %x while live, want the vector entropy %x", r.live, want)
	}
	if r, ok := byWhere["splitMnemonic.raw"]; !ok {
		t.Fatal("splitMnemonic.raw was never handed over")
	} else if !bytes.Equal(r.live, want) {
		t.Fatalf("splitMnemonic.raw held %x while live, want the vector entropy %x "+
			"(this vector has no leading zero byte, so big.Int.Bytes() is the entropy "+
			"unshortened)", r.live, want)
	}

	assertAllZeroed(t, *recs)
}

// TestFixChecksumZeroesTheDiscardedEntropy pins the clear() in FixChecksum.
// Only the checksum WORD survives that call; the entropy it was computed from
// must not.
func TestFixChecksumZeroesTheDiscardedEntropy(t *testing.T) {
	m, err := ParseMnemonic(residueMnemonic)
	if err != nil {
		t.Fatal(err)
	}
	recs := watchEntropyResidue(t)
	fixed := m.FixChecksum()
	if !fixed.Valid() {
		t.Fatal("FixChecksum produced an invalid mnemonic")
	}

	seen := false
	for _, r := range *recs {
		if r.where == "FixChecksum.ent" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("FixChecksum.ent was never handed over")
	}
	assertAllZeroed(t, *recs)
}

// TestLastWordCandidatesLeavesNoEntropyResidue is the count that makes F-104
// item 2 concrete: the last-word screen scans every candidate word, so the
// operator's 11-word prefix is turned into entropy 2,048 times per screen. The
// expected fire count is derived from NumWords rather than written down, and
// EVERY one of those buffers must be zero when the scan ends.
func TestLastWordCandidatesLeavesNoEntropyResidue(t *testing.T) {
	m, err := ParseMnemonic(residueMnemonic)
	if err != nil {
		t.Fatal(err)
	}
	recs := watchEntropyResidue(t)
	cands := LastWordCandidates(m)
	if len(cands) != 128 {
		t.Fatalf("LastWordCandidates returned %d candidates for a 12-word prefix, want 128",
			len(cands))
	}
	// Two buffers per Valid() call -- splitMnemonic.raw and Valid.ent -- and one
	// Valid() per candidate word.
	if want := 2 * int(NumWords); len(*recs) != want {
		t.Fatalf("the hook fired %d times, want %d (2 buffers x %d candidate words)",
			len(*recs), want, NumWords)
	}
	assertAllZeroed(t, *recs)
}

// TestSplitMnemonicReturnedEntropyIsNotWiped is the POSITIVE CONTROL for the
// instrument: splitMnemonic's RETURN value is the caller's, and nothing zeroes
// it. Reading it after the call must therefore come back NON-zero.
//
// Without this, a green run above would be consistent with the tests observing
// a fresh allocation on the second read rather than the array they captured --
// the subtle way a residue test reports success while the secret is still
// resident. This test fails if that is what is happening.
func TestSplitMnemonicReturnedEntropyIsNotWiped(t *testing.T) {
	m, err := ParseMnemonic(residueMnemonic)
	if err != nil {
		t.Fatal(err)
	}
	want, err := hex.DecodeString(residueEntropy)
	if err != nil {
		t.Fatal(err)
	}
	ent, _ := splitMnemonic(m)
	if !bytes.Equal(ent, want) {
		t.Fatalf("splitMnemonic returned %x, want %x -- the padding rewrite changed what "+
			"this function computes", ent, want)
	}
	if !nonZero(ent) {
		t.Fatal("splitMnemonic's returned entropy read back all-zero: the clear(raw) " +
			"beside it is reaching the RETURNED buffer, which would silently corrupt " +
			"Entropy() and every caller of it")
	}
	clear(ent)
}

// TestSplitMnemonicPaddingIsUnchanged guards the one behavioural risk in the
// F-104 fix: `entBytes = append(padding, entBytes...)` was split into two names
// so the pre-padding copy could be zeroed, and that must not have changed the
// left-padding it produces. Vector 1's entropy is all zero bytes, so
// big.Int.Bytes() returns an EMPTY slice for it -- the maximal-padding case,
// and the one a rewrite would break first.
func TestSplitMnemonicPaddingIsUnchanged(t *testing.T) {
	for _, v := range testVectors {
		m, err := ParseMnemonic(v.mnemonic)
		if err != nil {
			t.Fatal(err)
		}
		want, err := hex.DecodeString(v.entropy)
		if err != nil {
			t.Fatal(err)
		}
		ent, check := splitMnemonic(m)
		if !bytes.Equal(ent, want) {
			t.Fatalf("%q: splitMnemonic = %x, want %x", v.mnemonic, ent, want)
		}
		if got := checksum(want); got != check {
			t.Fatalf("%q: checksum byte = %d, want %d", v.mnemonic, check, got)
		}
	}
}
