package backup

import (
	"fmt"
	"strings"
	"testing"

	"github.com/seedhammer/kortschak-qr/coding"
)

// This file is a QR *decoder*, used only by tests. It exists because the
// highest-leverage assertion in the passphrase plate is that the engraved QR
// hands a scanner the passphrase byte for byte -- and nothing in the tree could
// read a QR back before now.
//
// It is a decoder, not an inverted encoder: it is handed a module grid
// recovered from the engraving geometry, recovers the mask from the grid's own
// format modules, unmasks, reads the codewords in the standard interleaved
// order and parses the segment header. It deliberately does NOT apply error
// correction -- the grid comes straight off the engraving plan, so damage must
// surface as a decode failure rather than be silently repaired.

// qrGrid is a module grid; g[y][x] is true when the module is black.
type qrGrid [][]bool

// decodeQR recovers the string encoded in g.
func decodeQR(t *testing.T, g qrGrid) string {
	t.Helper()
	size := len(g)
	if size < 21 || (size-17)%4 != 0 {
		t.Fatalf("qr: %d is not a valid module count", size)
	}
	for y, row := range g {
		if len(row) != size {
			t.Fatalf("qr: row %d has %d modules, want %d", y, len(row), size)
		}
	}
	v := coding.Version((size - 17) / 4)
	if v > 5 {
		// Versions above 5 split the data across more than one
		// error-correction block, which this decoder does not de-interleave.
		// ECC-L is pinned (spec 4.2) and ConstantQR refuses dims above 37, so
		// the passphrase plate can never reach here; fail loudly rather than
		// return something plausible.
		t.Fatalf("qr: version %d is beyond the pinned ECC-L range", v)
	}
	// Recover the mask: exactly one of the eight reproduces the grid's
	// function patterns, because the format modules encode the mask number.
	var plan *coding.Plan
	for m := coding.Mask(0); m <= 7; m++ {
		p, err := coding.NewPlan(v, coding.L, m)
		if err != nil {
			t.Fatal(err)
		}
		if qrFixedModulesMatch(p, g) {
			plan = p
			break
		}
	}
	if plan == nil {
		t.Fatal("qr: no ECC-L mask reproduces the grid's function patterns")
	}
	if plan.Blocks != 1 {
		t.Fatalf("qr: %d error-correction blocks; this decoder does not de-interleave", plan.Blocks)
	}
	raw := make([]byte, plan.DataBytes+plan.CheckBytes)
	for y, row := range plan.Pixel {
		for x, pix := range row {
			switch pix.Role() {
			case coding.Data, coding.Check:
			default:
				continue
			}
			// The plan pixel carries the mask; the grid carries mask XOR data.
			if g[y][x] != (pix&coding.Black != 0) {
				o := pix.Offset()
				raw[o/8] |= 1 << (7 - o%8)
			}
		}
	}
	return qrSegments(t, raw[:plan.DataBytes])
}

// qrFixedModulesMatch reports whether every module of p that is not a data or
// check bit -- finders, timing, alignment, format, the remainder bits -- has
// the colour g has.
func qrFixedModulesMatch(p *coding.Plan, g qrGrid) bool {
	for y, row := range p.Pixel {
		for x, pix := range row {
			switch pix.Role() {
			case coding.Data, coding.Check:
				continue
			}
			if (pix&coding.Black != 0) != g[y][x] {
				return false
			}
		}
	}
	return true
}

// qrAlphanumeric is the QR alphanumeric-mode character set, in code order.
const qrAlphanumeric = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// qrSegments parses the data codewords of a version 1-9 code.
func qrSegments(t *testing.T, data []byte) string {
	t.Helper()
	r := &qrBits{t: t, b: data}
	var out strings.Builder
	for r.left() >= 4 {
		switch mode := r.read(4); mode {
		case 0: // terminator
			return out.String()
		case 1: // numeric
			n := int(r.read(10))
			for ; n >= 3; n -= 3 {
				fmt.Fprintf(&out, "%03d", r.read(10))
			}
			switch n {
			case 2:
				fmt.Fprintf(&out, "%02d", r.read(7))
			case 1:
				fmt.Fprintf(&out, "%d", r.read(4))
			}
		case 2: // alphanumeric
			n := int(r.read(9))
			for ; n >= 2; n -= 2 {
				v := int(r.read(11))
				out.WriteByte(qrAlphanumeric[v/45])
				out.WriteByte(qrAlphanumeric[v%45])
			}
			if n == 1 {
				out.WriteByte(qrAlphanumeric[r.read(6)])
			}
		case 4: // byte
			n := int(r.read(8))
			for range n {
				out.WriteByte(byte(r.read(8)))
			}
		default:
			t.Fatalf("qr: unsupported segment mode %d", mode)
		}
	}
	return out.String()
}

// qrBits reads big-endian bit fields out of a codeword stream.
type qrBits struct {
	t   *testing.T
	b   []byte
	pos int
}

func (r *qrBits) left() int { return len(r.b)*8 - r.pos }

func (r *qrBits) read(n int) uint {
	r.t.Helper()
	if n > r.left() {
		r.t.Fatalf("qr: truncated stream: want %d bits, have %d", n, r.left())
	}
	var v uint
	for range n {
		bit := r.b[r.pos/8] >> (7 - r.pos%8) & 1
		v = v<<1 | uint(bit)
		r.pos++
	}
	return v
}
