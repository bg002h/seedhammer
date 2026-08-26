package txqr

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
	"github.com/seedhammer/kortschak-qr/coding"
)

// The pinned "even" transaction (mt-codec vector corpus): real, signed, 222 B.
const evenRawHex = "020000000001017c8da925af70e49a12b0cea7b639df5037c87b7fa61f262b86ac32c47aa3ba1a0000000000fdffffff02404b4c0000000000160014c1de0dd435d1d4ad97ed1f51d63f91c800cc4eab3ea1b92901000000160014751097c299d6354fbb2c5a84512dd708f2902f5e0247304402207debc7d89984c7717940b622504318d2c184966a618b32cf8b700d0f125b3ffa02206ef875f9c0b5931e0ea1cf0c109bdb8512835c8e51526f99b3419929a2ea7259012103718f5fd45b926226357e2b0400574b41a32d0bf0ae69a02eebea5fbc542ff52060000000"

func evenRaw(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString(evenRawHex)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The 20-bit header, bit for bit: mode 0011, index, count-1, parity — ISO
// 18004 via the measured QR findings. Asserted at the byte level so a field
// swap cannot hide behind a decoder that makes the same mistake.
func TestStructuredAppendHeaderBits(t *testing.T) {
	var b coding.Bits
	saHeader{index: 2, count: 6, parity: 0xA7}.Encode(&b, coding.Version(10))
	if b.Bits() != 20 {
		t.Fatalf("header is %d bits, want 20", b.Bits())
	}
	// Bits.Bytes() panics on a fractional byte, so pad to a boundary the same
	// way the plan does and check the header bits verbatim.
	b.Write(0, 4)
	got := b.Bytes()
	// 0011 0010 | 0101 1010 | 0111 0000 (mode 3, index 2, count-1 5, parity a7)
	want := []byte{0x32, 0x5A, 0x70}
	if !bytes.Equal(got, want) {
		t.Errorf("header bytes % x, want % x", got, want)
	}
}

func TestSingleSymbolMatchesThePlainEncoder(t *testing.T) {
	data := evenRaw(t)
	set, err := EncodeSet(data, 1, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := qr.Encode(string(data), qr.M)
	if err != nil {
		t.Fatal(err)
	}
	if set[0].Size != plain.Size || !bytes.Equal(set[0].Bitmap, plain.Bitmap) {
		t.Error("a 1-symbol set must be the plain byte-mode encoding")
	}
}

func TestEncodeSetBounds(t *testing.T) {
	data := evenRaw(t)
	if _, err := EncodeSet(data, 17, qr.M); err == nil {
		t.Error("17 symbols must be refused: the count-1 field is 4 bits")
	}
	if _, err := EncodeSet(nil, 1, qr.M); err == nil {
		t.Error("empty data must be refused")
	}
	if _, err := EncodeSet([]byte{1}, 2, qr.M); err == nil {
		t.Error("more symbols than bytes must be refused")
	}
	set, err := EncodeSet(data, 3, qr.M)
	if err != nil || len(set) != 3 {
		t.Fatalf("3-way split: %v", err)
	}
	// Balanced split: symbol versions should be equal-ish (same part size).
	if set[0].Size != set[1].Size {
		t.Errorf("parts 0 and 1 differ in size: %d vs %d", set[0].Size, set[1].Size)
	}
}

// The proof that matters: an ORDINARY scanner reads the engraved symbols back
// to the exact transaction bytes, with no constellation knowledge, in ANY
// order. ZXing (ZXingReader, the zxing-cpp CLI) is an independent mainstream
// implementation that decodes Structured Append and MERGES the sequence
// itself; its merged result must be byte-identical to the transaction.
// (zbar was tried first and cannot decode even qrencode's own reference SA
// symbols, so it is no oracle for this.)
func TestZxingMergesTheSetBackToTheTransaction(t *testing.T) {
	zxing, err := exec.LookPath("ZXingReader")
	if err != nil {
		t.Skip("ZXingReader not installed; the scanner round-trip needs it")
	}
	data := evenRaw(t)
	for _, k := range []int{1, 2, 3, 6} {
		set, err := EncodeSet(data, k, qr.M)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		// REVERSE order on purpose: scan order is irrelevant to a standard
		// decoder — the order lives inside each symbol, never on the plate —
		// and this is the cheapest place to prove it.
		args := []string{}
		for i := len(set) - 1; i >= 0; i-- {
			p := filepath.Join(dir, fmt.Sprintf("sym-%d.png", i))
			if err := os.WriteFile(p, set[i].PNG(), 0o600); err != nil {
				t.Fatal(err)
			}
			args = append(args, p)
		}
		out, _ := exec.Command(zxing, args...).Output()
		merged := mergedBytes(t, string(out), k)
		if !bytes.Equal(merged, data) {
			t.Errorf("k=%d: merged result diverged: got %d bytes, want %d",
				k, len(merged), len(data))
		}
	}
}

// mergedBytes extracts the Bytes: line of the result block whose Structured
// Append line reports a merged result of k symbols (or, for k = 1, the sole
// result's bytes).
func mergedBytes(t *testing.T, out string, k int) []byte {
	t.Helper()
	blocks := strings.Split(out, "Text:")
	for _, b := range blocks {
		if k > 1 && !strings.Contains(b, fmt.Sprintf("merged result from %d symbols", k)) {
			continue
		}
		i := strings.Index(b, "Bytes:")
		if i < 0 {
			continue
		}
		line := b[i+len("Bytes:"):]
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[:j]
		}
		h := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		raw, err := hex.DecodeString(strings.ToLower(h))
		if err != nil {
			t.Fatalf("k=%d: unparseable Bytes line: %v", k, err)
		}
		return raw
	}
	t.Fatalf("k=%d: no merged result in ZXing output:\n%s", k, out)
	return nil
}

// P5 M-1 — EncodeSet panicked instead of refusing, for a k that cannot split
// the data into k non-empty parts.
//
// The only guard was `k > len(data)`, which does not catch it: with per =
// ceil(len/k), the last few symbols get lo = i*per beyond len(data) while hi is
// clamped to len(data), so `data[lo:hi]` has lo > hi. Executed arithmetic:
// len=113, k=16 -> per=8, lo=120, hi=113 -> data[120:113].
//
// gui/transaction.go's comment already asserted the contract this test pins:
// "EncodeSet REFUSES a payload it cannot split into k non-empty parts." That
// was false; now it is true.
func TestEncodeSetRefusesAKThatCannotSplitIntoNonEmptyParts(t *testing.T) {
	for _, tc := range []struct{ n, k int }{
		{113, 16},
		{60, 14},
	} {
		data := make([]byte, tc.n)
		for i := range data {
			data[i] = byte(i)
		}
		got, err := EncodeSet(data, tc.k, qr.M)
		if err == nil {
			t.Fatalf("n=%d k=%d: expected a refusal, got %d symbols", tc.n, tc.k, len(got))
		}
	}
}

// THE CONTROL: a k that CAN split must still encode. Without this, refusing
// everything would satisfy the test above.
func TestEncodeSetStillEncodesASplittableSet(t *testing.T) {
	data := make([]byte, 113)
	for i := range data {
		data[i] = byte(i)
	}
	got, err := EncodeSet(data, 15, qr.M)
	if err != nil {
		t.Fatalf("n=113 k=15 must encode: %v", err)
	}
	if len(got) != 15 {
		t.Fatalf("want 15 symbols, got %d", len(got))
	}
}
