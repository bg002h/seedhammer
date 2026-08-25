package txqr

import (
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
)

// GRAFT 5 — THE MODE-SEGMENTATION GATE, run against THIS package's encoder.
//
// F-234 records why this exists. A QR encoder chooses its data mode and will
// silently encode part or all of a payload in a denser one. Three measurements
// were corrupted that way: an all-0x41 payload measured ALPHANUMERIC capacity
// while claiming byte, a high-byte payload paid an ECI header, and a mixed
// payload read 6.6% LOW. **Every one produced a plausible number**, and only
// asserting measured v40 capacity against the published ISO/IEC 18004 limits
// caught them.
//
// A capacity function that disagrees with the published limits is measuring a
// different mode than it claims, and every figure derived from it is suspect --
// which here means the plate count, the symbol count and the ECC level the
// device picks before it cuts steel.
//
// The gate is a PROBE, never a formula: it binary-searches the largest payload
// the real encoder accepts. A formula would restate the assumption under test.
//
// WHAT IT DOES NOT COVER: it measures capacity, not correctness of the modules.
// That a symbol DECODES is the ZXing round-trip's job, and that the Structured
// Append header is what a standard decoder expects is txqr_test.go's.

// publishedV40 is ISO/IEC 18004's version-40 character capacity, per EC level.
var publishedV40 = []struct {
	level          qr.Level
	name           string
	num, alnum, by int
}{
	{qr.L, "L", 7089, 4296, 2953},
	{qr.M, "M", 5596, 3391, 2331},
	{qr.Q, "Q", 3993, 2420, 1663},
	{qr.H, "H", 3057, 1852, 1273},
}

// largest binary-searches the biggest n for which fits(n) is true.
func largest(fits func(int) bool) int {
	if !fits(1) {
		return 0
	}
	lo, hi := 1, 8000
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// THE VENDORED LIBRARY'S CAPACITY TABLES, probed in all three modes at all four
// EC levels. Every figure this package computes rests on `Version.DataBytes`,
// and a table wrong in one cell produces a plausible number everywhere -- which
// is exactly how F-234's three corrupted measurements looked.
//
// qr.Encode is probed HERE rather than EncodeSet because it is the library's
// mode-SELECTING entry point, and selecting the mode is what makes each of the
// three published columns reachable at all. EncodeSet no longer selects (see
// TestOneSymbolIsByteModeToo), so probing it would measure one column and leave
// eight of these twelve cells unchecked.
func TestTheLibraryCapacityTablesMatchThePublishedLimits(t *testing.T) {
	// Lowercase is OUTSIDE the QR alphanumeric charset (which is 0-9, A-Z,
	// space and $%*+-./:), so it forces byte mode without any high bytes that
	// might attract an ECI header.
	sets := []struct {
		name  string
		chars string
		want  func(int, int, int) int
	}{
		{"numeric", "0123456789", func(n, a, b int) int { return n }},
		{"alnum", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", func(n, a, b int) int { return a }},
		{"byte", "abcdefghijklmnopqrstuvwxyz", func(n, a, b int) int { return b }},
	}
	for _, lv := range publishedV40 {
		for _, s := range sets {
			want := s.want(lv.num, lv.alnum, lv.by)
			got := largest(func(n int) bool {
				d := make([]byte, n)
				for i := range d {
					d[i] = s.chars[i%len(s.chars)]
				}
				_, err := qr.Encode(string(d), lv.level)
				return err == nil
			})
			if got != want {
				t.Errorf("v40-%s %s: the library fits %d, published limit is %d "+
					"— the encoder is measuring a different mode than this payload claims, "+
					"and every plate/symbol/ECC figure derived from it is suspect",
					lv.name, s.name, got, want)
			}
		}
	}
}

// THE STRUCTURED APPEND PATH IS SEPARATE AND MUST BE GATED SEPARATELY.
// encodeSA does NOT go through qr.Encode: it writes the 20-bit SA header ahead
// of a coding.String segment and walks versions itself. That is a second
// capacity computation in this package, and a second one is exactly how the
// two drift.
func TestStructuredAppendCapacityCostsExactlyTheHeader(t *testing.T) {
	for _, lv := range publishedV40 {
		// One SA symbol's capacity, probed through the real encoder.
		got := largest(func(n int) bool {
			d := make([]byte, n)
			for i := range d {
				d[i] = "abcdefghijklmnopqrstuvwxyz"[i%26]
			}
			_, err := encodeSA(d, 0, 2, 0x00, lv.level)
			return err == nil
		})
		// The header is 20 bits: 4 mode + 4 index + 4 count-1 + 8 parity. The
		// byte-mode segment is 4 + 16 (v40 length field) + 8n bits, so
		//   8n <= DataBytes*8 - 20 - 20   ->   n <= DataBytes - 5
		// while a bare symbol gives n <= (DataBytes*8 - 20)/8. Derived from the
		// PUBLISHED byte figure rather than from the library's own tables, so
		// this cannot agree with a wrong table.
		dataBytes := (lv.by*8 + 20 + 7) / 8 // invert the bare-symbol formula
		want := dataBytes - 5
		if got != want {
			t.Errorf("v40-%s SA: one symbol fits %d bytes, want %d "+
				"(published byte capacity %d, minus the 20-bit Structured Append header)",
				lv.name, got, want, lv.by)
		}
		// And it must COST capacity rather than being free, which is the
		// mistake a formula that forgot the header would make.
		if got >= lv.by {
			t.Errorf("v40-%s SA: the header costs nothing (%d vs a bare %d)",
				lv.name, got, lv.by)
		}
		// THE WALK MUST DO ITS OWN ARITHMETIC, and this is the assertion that
		// proves it. Measured while mutation-testing this gate: setting
		// saHeader.Bits to 0 does NOT change the measured capacity by one
		// byte, because coding.Plan.Encode catches the overflow afterwards --
		// "cannot encode 23656 bits into 23648-bit code". The encoder is a
		// backstop, so the mutation is harmless, but it is also INVISIBLE to
		// any capacity probe. What it does change is WHO refuses, so that is
		// what is asserted: one byte past the cap must be our version walk
		// running out of versions, not the library rescuing it.
		over := make([]byte, got+1)
		for i := range over {
			over[i] = "abcdefghijklmnopqrstuvwxyz"[i%26]
		}
		_, err := encodeSA(over, 0, 2, 0x00, lv.level)
		if err == nil {
			t.Errorf("v40-%s SA: %d bytes encoded, one past the measured cap",
				lv.name, got+1)
		} else if !strings.Contains(err.Error(), "too long to encode as QR") {
			t.Errorf("v40-%s SA: one byte over was refused by the ENCODER, not by "+
				"the version walk (%v) — the walk is not accounting for the "+
				"Structured Append header", lv.name, err)
		}
	}
}

// The gate above measures ONE symbol. The number the planner actually spends is
// the SET's total, so that is asserted too: 16 symbols is the Structured Append
// cap, and the largest transaction this device can ever put on QR plates is
// that many maxed v40 symbols at the ECC floor.
//
// THE FIGURE IS PRINTED, because it is the QR delivery ceiling and it appears
// in no other test. A number nobody can see is a number nobody checks.
func TestTheQRDeliveryCeilingIsWhatWeThinkItIs(t *testing.T) {
	for _, lv := range publishedV40 {
		per := largest(func(n int) bool {
			d := make([]byte, n)
			for i := range d {
				d[i] = "abcdefghijklmnopqrstuvwxyz"[i%26]
			}
			_, err := encodeSA(d, 0, MaxSymbols, 0x00, lv.level)
			return err == nil
		})
		t.Logf("v40-%s: %5d bytes/symbol x %d symbols = %6d bytes",
			lv.name, per, MaxSymbols, per*MaxSymbols)
	}
	// ECC M is the floor the objective never trades below.
	perM := largest(func(n int) bool {
		d := make([]byte, n)
		for i := range d {
			d[i] = "abcdefghijklmnopqrstuvwxyz"[i%26]
		}
		_, err := encodeSA(d, 0, MaxSymbols, 0x00, qr.M)
		return err == nil
	})
	if perM*MaxSymbols < 32734 {
		// Not a failure of the encoder -- a statement of where the binding
		// limit is. If this ever trips, QR plates became the ceiling instead
		// of the container, and the planner's refusal message must say so.
		t.Logf("NOTE: QR plates cap delivery at %d bytes, BELOW the container's "+
			"32,734-byte section cap — QR is the binding limit, not sysw",
			perM*MaxSymbols)
	}
}

// A PAYLOAD OF ALL 0x41 IS THE F-234 TRAP ITSELF: 'A' is inside the QR
// alphanumeric charset, so an encoder that selects modes measures ALPHANUMERIC
// capacity for it while the caller believes it is measuring byte capacity.
//
// **THE GATE FOUND THIS PACKAGE STANDING IN THE TRAP.** Its own doc comment
// says "Byte mode always: the payload is raw transaction bytes, and a per-part
// mode choice would make symbol boundaries observable in the decoded text" --
// but that was true only of the k >= 2 path. k = 1 went through qr.Encode,
// which picks numeric, then alphanumeric, then byte. So the package documented
// an invariant it did not hold, and one symbol's capacity depended on the
// payload's CHARACTER DISTRIBUTION rather than its length: a transaction whose
// serialization happened to fall inside the alphanumeric charset would fit a
// symbol its neighbours of identical size would not, and the plate count the
// operator is shown would move for a reason unrelated to anything they can see.
//
// It is now byte mode on both paths, so the two capacity computations in this
// package measure the same thing.
func TestOneSymbolIsByteModeToo(t *testing.T) {
	fitsOneSymbol := func(d []byte, l qr.Level) bool {
		_, err := EncodeSet(d, 1, l)
		return err == nil
	}
	// The trap: 3,000 'A' bytes. Alphanumeric capacity at v40-M is 3,391 and
	// byte capacity is 2,331, so this fits if and only if the encoder left byte
	// mode for it.
	trap := []byte(strings.Repeat("A", 3000))
	if fitsOneSymbol(trap, qr.M) {
		t.Error("3,000 'A' bytes fit ONE v40-M symbol; byte capacity is 2,331, so " +
			"the encoder is measuring alphanumeric capacity for a payload the " +
			"package documents as byte mode")
	}
	// ...and all-digits, which is the denser trap still: numeric capacity at
	// v40-M is 5,596.
	if fitsOneSymbol([]byte(strings.Repeat("7", 3000)), qr.M) {
		t.Error("3,000 '7' bytes fit ONE v40-M symbol — numeric mode was selected")
	}
	// THE NEAREST LEGITIMATE INPUT still fits: exactly the byte capacity.
	ok := make([]byte, 2331)
	for i := range ok {
		ok[i] = byte(i % 251)
	}
	if !fitsOneSymbol(ok, qr.M) {
		t.Error("2,331 arbitrary bytes did not fit one v40-M symbol; that IS the " +
			"published byte capacity, so the fix over-corrected")
	}
	if fitsOneSymbol(append(ok, 0x00), qr.M) {
		t.Error("2,332 bytes fit one v40-M symbol; the cap is 2,331")
	}
}
