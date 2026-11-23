package sysw

import (
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
)

// mrEnc builds an mr1 string from data symbols with an independent copy of
// BIP 93's ms32_polymod and mr's residue 0x085d21404aee15059.
func mrEnc(data []int) string {
	var gen []*big.Int
	for _, g := range []string{"19DC500CE73FDE210", "1BFAE00DEF77FE529", "1FBD920FFFE7BEE52", "1739640BDEEE3FDAD", "07729A039CFC75F5A"} {
		v, _ := new(big.Int).SetString(g, 16)
		gen = append(gen, v)
	}
	mask60 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 60), big.NewInt(1))
	r := big.NewInt(0x23181b3)
	vals := append([]int{'m' >> 5, 'r' >> 5, 0, 'm' & 31, 'r' & 31}, data...)
	vals = append(vals, make([]int, 13)...)
	for _, v := range vals {
		b := new(big.Int).Rsh(r, 60).Uint64()
		r = new(big.Int).And(r, mask60)
		r.Lsh(r, 5)
		r.Xor(r, big.NewInt(int64(v)))
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				r.Xor(r, gen[i])
			}
		}
	}
	target, _ := new(big.Int).SetString("085D21404AEE15059", 16)
	r.Xor(r, target)
	var sb strings.Builder
	sb.WriteString("mr1")
	for _, d := range data {
		sb.WriteByte(mrAlphabet[d])
	}
	for i := 0; i < 13; i++ {
		sb.WriteByte(mrAlphabet[new(big.Int).Rsh(r, uint(5*(12-i))).Uint64()&31])
	}
	return sb.String()
}

// mrChunkStr builds one chunk: tag l, 20-bit set id, count-1, index, slice.
func mrChunkStr(csid uint32, count, index int, slice []byte) string {
	var bits []int
	push := func(v uint32, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, int(v>>uint(i))&1)
		}
	}
	push(31, 5)
	push(csid, 20)
	push(uint32(count-1), 6)
	push(uint32(index), 6)
	for _, b := range slice {
		push(uint32(b), 8)
	}
	for len(bits)%5 != 0 {
		bits = append(bits, 0)
	}
	var data []int
	for i := 0; i < len(bits); i += 5 {
		s := 0
		for j := 0; j < 5; j++ {
			s = s<<1 | bits[i+j]
		}
		data = append(data, s)
	}
	return mrEnc(data)
}

// mrCard cuts content the way mr does: count = max(2, ceil(len/45)).
func mrCard(content []byte) []string {
	h := sha256.Sum256(append([]byte(mrChunkDomain), content...))
	csid := uint32(h[0])<<12 | uint32(h[1])<<4 | uint32(h[2])>>4
	n := (len(content) + 44) / 45
	if n < 2 {
		n = 2
	}
	each := (len(content) + n - 1) / n
	var out []string
	for k := 0; k < n; k++ {
		lo, hi := k*each, (k+1)*each
		if hi > len(content) || k == n-1 {
			hi = len(content)
		}
		out = append(out, mrChunkStr(csid, n, k, content[lo:hi]))
	}
	return out
}

// 14b's edges: L = 14 and 93 symbols after mr1 are the shortest and longest
// mr1 strings; the three reserved first symbols; chunk data lengths 8 and 13
// (0 and 4 padding bits at the extremes).
func TestMrStringEdgesAndOrder(t *testing.T) {
	zeros := func(n int) []int { return make([]int, n) }
	ok := map[string]string{
		"L=14 (shortest)":         mrEnc([]int{1}),
		"L=93 (longest)":          mrEnc(append([]int{1}, zeros(79)...)),
		"chunk of 8 data symbols": mrEnc(append([]int{31}, zeros(7)...)),
		"chunk with 4 pad bits":   mrEnc(append([]int{31}, zeros(12)...)),
	}
	for name, rec := range ok {
		if got := MrStringReason(rec); got != "" || Classify(rec) != ClassMr {
			t.Errorf("%s: %q reason %q class %v, want ClassMr", name, rec, got, Classify(rec))
		}
	}
	bad := map[string]string{
		"L=13":                    MrLength,
		"L=94":                    MrLength,
		"reserved u":              MrReservedSymbol,
		"reserved a":              MrReservedSymbol,
		"reserved 7":              MrReservedSymbol,
		"chunk of 7 data symbols": MrChunkShort,
		"bad char and bad length": MrCharacter,
		"capital and bad char":    MrCase,
	}
	rec := map[string]string{
		"L=13":                    mrEnc(nil),
		"L=94":                    mrEnc(append([]int{1}, zeros(80)...)),
		"reserved u":              mrEnc([]int{28, 0}),
		"reserved a":              mrEnc([]int{29, 0}),
		"reserved 7":              mrEnc([]int{30, 0}),
		"chunk of 7 data symbols": mrEnc(append([]int{31}, zeros(6)...)),
		"bad char and bad length": "mr1b",
		"capital and bad char":    "Mr1b",
	}
	for name, want := range bad {
		if got := MrStringReason(rec[name]); got != want {
			t.Errorf("%s: %q reason %q, want %q", name, rec[name], got, want)
		}
	}
}

// 14c's chunk count rule at its edges: count = max(2, ceil(len/45)) bytes.
func TestMrSetSliceRuleEdges(t *testing.T) {
	for _, n := range []int{2, 10, 45, 46, 89, 90, 91, 135, 136} {
		content := make([]byte, n)
		for i := range content {
			content[i] = byte(i*7 + 3)
		}
		card := mrCard(content)
		if ref, bad := MrSetProblem(card); bad {
			t.Errorf("%d bytes in %d chunks: refused %s", n, len(card), ref.Reason)
		}
	}
	// the rule's count is exact: 90 bytes in 3 chunks, 10 bytes in 3 chunks
	for _, tc := range []struct{ n, chunks int }{{90, 3}, {10, 3}} {
		content := make([]byte, tc.n)
		h := sha256.Sum256(append([]byte(mrChunkDomain), content...))
		csid := uint32(h[0])<<12 | uint32(h[1])<<4 | uint32(h[2])>>4
		each := (tc.n + tc.chunks - 1) / tc.chunks
		var card []string
		for k := 0; k < tc.chunks; k++ {
			lo, hi := k*each, (k+1)*each
			if hi > tc.n || k == tc.chunks-1 {
				hi = tc.n
			}
			if lo > tc.n {
				lo = tc.n
			}
			card = append(card, mrChunkStr(csid, tc.chunks, k, content[lo:hi]))
		}
		ref, bad := MrSetProblem(card)
		if !bad || ref.Reason != SetSlices {
			t.Errorf("%d bytes in %d chunks: got %v %q, want SET_SLICES", tc.n, tc.chunks, bad, ref.Reason)
		}
	}
}
