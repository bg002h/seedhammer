package codex32

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
)

const mrAlphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// mrEncode builds an mr1 string from data symbols with an independent copy of
// BIP 93's ms32_polymod (init 0x23181b3, mr's residue 0x085d21404aee15059),
// so the verifier under test never checks its own output.
func mrEncode(data []int) string {
	gen := []*big.Int{}
	for _, g := range []string{"19DC500CE73FDE210", "1BFAE00DEF77FE529", "1FBD920FFFE7BEE52", "1739640BDEEE3FDAD", "07729A039CFC75F5A"} {
		v, _ := new(big.Int).SetString(g, 16)
		gen = append(gen, v)
	}
	mask60 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 60), big.NewInt(1))
	polymod := func(vals []int) *big.Int {
		r := big.NewInt(0x23181b3)
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
		return r
	}
	vals := []int{'m' >> 5, 'r' >> 5, 0, 'm' & 31, 'r' & 31}
	vals = append(vals, data...)
	vals = append(vals, make([]int, 13)...)
	target, _ := new(big.Int).SetString("085D21404AEE15059", 16)
	res := polymod(vals)
	res.Xor(res, target)
	var sb strings.Builder
	sb.WriteString("mr1")
	for _, d := range data {
		sb.WriteByte(mrAlphabet[d])
	}
	for i := 0; i < 13; i++ {
		sym := new(big.Int).Rsh(res, uint(5*(12-i))).Uint64() & 31
		sb.WriteByte(mrAlphabet[sym])
	}
	return sb.String()
}

func TestValidMR(t *testing.T) {
	raw, err := os.ReadFile("../sysw/testdata/sysw_mr_v1_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		StringVectors []struct{ Name, Record string } `json:"string_vectors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, v := range f.StringVectors {
		want := v.Name == "single-card" || v.Name == "chunk-0" || v.Name == "chunk-1"
		// Pure BCH verification: names whose only fault is case, an alphabet
		// character, a length or a reserved symbol are judged by the sysw rule
		// order, so only the three valid rows and `checksum` are pinned here.
		switch v.Name {
		case "single-card", "chunk-0", "chunk-1", "checksum":
			if got := ValidMR(v.Record); got != want {
				t.Errorf("ValidMR(%s) = %v, want %v", v.Name, got, want)
			}
		}
	}
	// A 96-character (93 data symbols) string is valid, 94 symbols is not.
	data := make([]int, 80)
	for i := range data {
		data[i] = (i * 7) % 32
	}
	s93 := mrEncode(data)
	if len(s93) != 96 || !ValidMR(s93) {
		t.Errorf("93-symbol string: len %d valid %v, want 96 true", len(s93), ValidMR(s93))
	}
	s94 := mrEncode(append(data, 5))
	if len(s94) != 97 || ValidMR(s94) {
		t.Errorf("94-symbol string: len %d valid %v, want 97 false", len(s94), ValidMR(s94))
	}
	// 14 symbols is the shortest; 13 is not.
	if s := mrEncode([]int{1}); !ValidMR(s) {
		t.Errorf("14-symbol string %s refused", s)
	}
	if s := mrEncode(nil); ValidMR(s) {
		t.Errorf("13-symbol string %s accepted", s)
	}
	// The mt1 target must not validate an mr1 string and vice versa.
	if ValidMT(s93) || ValidMD(s93) || ValidMK(s93) {
		t.Error("an mr1 string validated under another hrp")
	}
}
