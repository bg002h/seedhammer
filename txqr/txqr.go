// Package txqr encodes a raw signed Bitcoin transaction as QR symbols for
// engraving: one plain byte-mode symbol when the transaction fits, and ISO/IEC
// 18004 Structured Append symbols when it must split.
//
// WHY STRUCTURED APPEND AND NOT A BESPOKE HEADER. The QR carries the RAW
// transaction bytes so that a recoverer with an ORDINARY scanner gets a
// broadcastable transaction with no constellation knowledge (the F-234
// promise, per the measured QR findings). Structured Append is QR's own
// standard for multi-symbol payloads: each symbol carries a 20-bit header
// (mode 0011, 4-bit index, 4-bit count-1, 8-bit parity = XOR of every byte of
// the undivided message), a standard decoder collects symbols in ANY order,
// and the parity byte detects a symbol from a foreign set — the
// drawer-of-plates failure, caught by the format rather than the operator.
//
// THE VENDORED LIBRARY HAS NO STRUCTURED APPEND (verified against
// kortschak-qr v0.3.2: Encode returns one *Code). It does not need it: its
// coding.Plan.Encode is variadic over the exported Encoding interface, so the
// 20-bit header is written by an Encoding implemented HERE, ahead of the
// byte-mode segment — no fork of the vendored module.
//
// LIMITS. Structured Append's count field is 4 bits: AT MOST 16 SYMBOLS, a
// hard bound (the caller discards any plan above it, exactly as it discards
// one that does not fit a plate). And this package deliberately has no
// opinion about plates: it encodes symbols; fitting them is the caller's
// (gui/transaction.go's), because fit depends on engrave.Params this package
// must not know.
package txqr

import (
	"errors"
	"fmt"

	qr "github.com/seedhammer/kortschak-qr"
	"github.com/seedhammer/kortschak-qr/coding"
)

// MaxSymbols is Structured Append's hard cap: the count-1 field is 4 bits.
const MaxSymbols = 16

var errTooManySymbols = errors.New("txqr: more than 16 symbols (Structured Append cap)")

// EncodeSet encodes data as k QR symbols at the given error-correction level:
// plain byte mode for k = 1, Structured Append for 2 ≤ k ≤ 16. Symbols are
// returned in index order; data is split into k nearly-equal parts (the last
// may be shorter), so symbol sizes are balanced the way the plates will be.
func EncodeSet(data []byte, k int, level qr.Level) ([]*qr.Code, error) {
	if len(data) == 0 {
		return nil, errors.New("txqr: empty data")
	}
	if k < 1 || k > MaxSymbols {
		return nil, errTooManySymbols
	}
	if k == 1 {
		// BYTE MODE, EXPLICITLY -- not qr.Encode, which selects numeric, then
		// alphanumeric, then byte, and would therefore make ONE symbol's
		// capacity a fact about the payload's CHARACTER DISTRIBUTION rather
		// than its length. A transaction whose serialization happened to fall
		// inside the alphanumeric charset would fit a symbol its neighbours of
		// identical size would not, and the plate count the operator is shown
		// on the review screen would move for a reason nothing on that screen
		// explains. It also made the package's own "byte mode always" claim
		// false of half its own paths.
		//
		// Found by running capgate over this package (capgate_test.go).
		c, err := encodeByte(data, level)
		if err != nil {
			return nil, err
		}
		return []*qr.Code{c}, nil
	}
	if k > len(data) {
		return nil, fmt.Errorf("txqr: %d symbols for %d bytes", k, len(data))
	}
	// Parity: XOR of every byte of the ORIGINAL undivided message, identical
	// across the set — what lets a decoder detect a foreign symbol.
	var parity byte
	for _, b := range data {
		parity ^= b
	}
	per := (len(data) + k - 1) / k
	out := make([]*qr.Code, 0, k)
	for i := 0; i < k; i++ {
		lo := i * per
		hi := min(lo+per, len(data))
		c, err := encodeSA(data[lo:hi], i, k, parity, level)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// saHeader is the 20-bit Structured Append header, written ahead of the data
// segment through the vendored library's own Encoding seam.
type saHeader struct {
	index  int
	count  int
	parity byte
}

func (s saHeader) String() string {
	return fmt.Sprintf("StructuredAppend(%d of %d)", s.index+1, s.count)
}

func (s saHeader) Check() error {
	if s.index < 0 || s.count < 2 || s.count > MaxSymbols || s.index >= s.count {
		return fmt.Errorf("txqr: bad structured append header %d/%d", s.index, s.count)
	}
	return nil
}

// Bits is the fixed header size: 4 (mode) + 4 (index) + 4 (count-1) + 8
// (parity). Independent of version — Structured Append has no length field.
func (s saHeader) Bits(coding.Version) int { return 20 }

func (s saHeader) Encode(b *coding.Bits, _ coding.Version) {
	b.Write(3, 4) // mode indicator 0011
	b.Write(uint(s.index), 4)
	b.Write(uint(s.count-1), 4)
	b.Write(uint(s.parity), 8)
}

// encodeByte is the vendored qr.Encode's version walk and mask search with the
// mode PINNED to byte, for the single-symbol case. It shares encodeSA's body by
// passing no header.
func encodeByte(data []byte, level qr.Level) (*qr.Code, error) {
	return encodeSegments(coding.String(data), level, nil)
}

// encodeSA mirrors the vendored qr.Encode's version walk and mask search,
// with the Structured Append header prepended. Byte mode always: the payload
// is raw transaction bytes, and a per-part mode choice would make symbol
// boundaries observable in the decoded text.
func encodeSA(part []byte, index, count int, parity byte, level qr.Level) (*qr.Code, error) {
	head := saHeader{index: index, count: count, parity: parity}
	if err := head.Check(); err != nil {
		return nil, err
	}
	return encodeSegments(coding.String(part), level, &head)
}

// encodeSegments is the version walk and mask search both paths share, with the
// byte-mode body and an OPTIONAL Structured Append header ahead of it.
//
// ONE implementation, because two were the drift risk: the capacity a symbol
// has and the capacity the planner assumes it has are computed here and
// nowhere else.
func encodeSegments(body coding.String, level qr.Level, head *saHeader) (*qr.Code, error) {
	l := coding.Level(level)
	headBits := func(v coding.Version) int {
		if head == nil {
			return 0
		}
		return head.Bits(v)
	}
	var v coding.Version
	for v = coding.MinVersion; ; v++ {
		if v > coding.MaxVersion {
			return nil, errors.New("txqr: part too long to encode as QR")
		}
		if headBits(v)+body.Bits(v) <= v.DataBytes(l)*8 {
			break
		}
	}
	var best *coding.Code
	lowPenalty := int(^uint(0) >> 1)
	for m := coding.Mask(0); m <= 7; m++ {
		p, err := coding.NewPlan(v, l, m)
		if err != nil {
			return nil, err
		}
		var cc *coding.Code
		if head == nil {
			cc, err = p.Encode(body)
		} else {
			cc, err = p.Encode(*head, body)
		}
		if err != nil {
			return nil, err
		}
		if pen := cc.Penalty(); pen < lowPenalty {
			best = cc
			lowPenalty = pen
		}
	}
	return &qr.Code{Bitmap: best.Bitmap, Size: best.Size, Stride: best.Stride, Scale: 8}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
