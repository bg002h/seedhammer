package slip39

import (
	"encoding/hex"
	"testing"
)

func hexEq(b []byte) string { return hex.EncodeToString(b) }

func TestCombineBasic2of3(t *testing.T) {
	shares := vectorShares(t, 3) // all mnemonics of official vector idx 3
	parsed := make([]Share, len(shares))
	for i, m := range shares {
		s, err := ParseShare(m)
		if err != nil {
			t.Fatalf("share %d: %v", i, err)
		}
		parsed[i] = s
	}
	got, err := Combine(parsed[:2], []byte("TREZOR")) // any 2 of 3
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if hexEq(got) != "b43ceb7e57a0ea8766221624d01b0864" {
		t.Errorf("recovered %x want b43c…0864", got)
	}
}
