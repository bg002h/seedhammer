package gui

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/md"
)

// abandonSlotXpub derives the abandon-about seed at the given origin and packs
// its canonical (chainCode||compressedPubkey) into the 65-byte ExpandedKey.Xpub
// layout (cc = [0:32], pk = [32:65]).
func abandonSlotXpub(t *testing.T, origin bip32.Path) [65]byte {
	t.Helper()
	m := abandonAboutMnemonic()
	xpub, _, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, origin)
	if err != nil {
		t.Fatalf("deriveAccountXpub: %v", err)
	}
	cc, pk, _, err := decodeXpubBytes(xpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	var out [65]byte
	copy(out[0:32], cc[:])
	copy(out[32:65], pk[:])
	return out
}

// foreignXpub is a structurally-valid 65-byte Xpub that the abandon seed never
// derives to (a fixed non-zero pattern). Used for non-cosigner slots.
func foreignXpub() [65]byte {
	var out [65]byte
	for i := range out {
		out[i] = byte(0x40 + i)
	}
	return out
}

func msPath(comps ...uint32) bip32.Path {
	p := make(bip32.Path, len(comps))
	copy(p, comps)
	return p
}

const hard32 = 0x80000000

// TestFindUserSlot exercises the D14 cross-match (I-1): match on the canonical
// (cc,pk) pair via bytes.Equal over the full 32+33 bytes, derive at each slot's
// OWN origin, refuse on zero matches, first-by-index + reused notice on >=2.
func TestFindUserSlot(t *testing.T) {
	net := &chaincfg.MainNetParams
	m := abandonAboutMnemonic()
	origin0 := msPath(hard32+48, hard32+0, hard32+0, hard32+2) // @0 origin
	origin1 := msPath(hard32+48, hard32+0, hard32+1, hard32+2) // @1 origin (distinct)
	origin2 := msPath(hard32+48, hard32+0, hard32+2, hard32+2) // @2 origin (distinct)

	t.Run("match at @1, foreign @0/@2", func(t *testing.T) {
		keys := []md.ExpandedKey{
			{Index: 0, OriginPath: origin0, Xpub: foreignXpub(), XpubPresent: true},
			{Index: 1, OriginPath: origin1, Xpub: abandonSlotXpub(t, origin1), XpubPresent: true},
			{Index: 2, OriginPath: origin2, Xpub: foreignXpub(), XpubPresent: true},
		}
		idx, origin, reused, ok := findUserSlot(m, "", net, keys)
		if !ok {
			t.Fatal("ok=false, want a match at @1")
		}
		if idx != 1 {
			t.Fatalf("slot index = %d, want 1", idx)
		}
		if origin.String() != origin1.String() {
			t.Fatalf("origin = %s, want %s", origin.String(), origin1.String())
		}
		if len(reused) != 0 {
			t.Fatalf("reused = %v, want empty (single match)", reused)
		}
	})

	t.Run("non-cosigner -> refuse", func(t *testing.T) {
		keys := []md.ExpandedKey{
			{Index: 0, OriginPath: origin0, Xpub: foreignXpub(), XpubPresent: true},
			{Index: 1, OriginPath: origin1, Xpub: foreignXpub(), XpubPresent: true},
		}
		if _, _, _, ok := findUserSlot(m, "", net, keys); ok {
			t.Fatal("ok=true for a non-cosigner seed, want false (refuse)")
		}
	})

	t.Run("ambiguous @0 and @2 -> first-by-index + notice", func(t *testing.T) {
		// The SAME seed at two DISTINCT origins (legitimate reused key).
		keys := []md.ExpandedKey{
			{Index: 0, OriginPath: origin0, Xpub: abandonSlotXpub(t, origin0), XpubPresent: true},
			{Index: 1, OriginPath: origin1, Xpub: foreignXpub(), XpubPresent: true},
			{Index: 2, OriginPath: origin2, Xpub: abandonSlotXpub(t, origin2), XpubPresent: true},
		}
		idx, origin, reused, ok := findUserSlot(m, "", net, keys)
		if !ok {
			t.Fatal("ok=false, want first-by-index match")
		}
		if idx != 0 {
			t.Fatalf("slot index = %d, want 0 (first-by-index)", idx)
		}
		if origin.String() != origin0.String() {
			t.Fatalf("origin = %s, want @0 origin %s", origin.String(), origin0.String())
		}
		if len(reused) != 2 || reused[0] != 0 || reused[1] != 2 {
			t.Fatalf("reused = %v, want [0 2]", reused)
		}
	})

	t.Run("XpubPresent=false slot is skipped", func(t *testing.T) {
		keys := []md.ExpandedKey{
			{Index: 0, OriginPath: origin1, Xpub: abandonSlotXpub(t, origin1), XpubPresent: false}, // skipped
			{Index: 1, OriginPath: origin1, Xpub: abandonSlotXpub(t, origin1), XpubPresent: true},  // matches
		}
		idx, _, _, ok := findUserSlot(m, "", net, keys)
		if !ok || idx != 1 {
			t.Fatalf("idx=%d ok=%v, want match at @1 (the present slot)", idx, ok)
		}
	})
}

// TestFormatSlotList (t6b-M1): the reused-key notice must name EVERY reused
// slot, not just the first two — so a 3+-match notice lists all of them.
func TestFormatSlotList(t *testing.T) {
	cases := []struct {
		in   []int
		want string
	}{
		{nil, ""},
		{[]int{}, ""},
		{[]int{2}, "@2"},
		{[]int{0, 2}, "@0 and @2"},
		{[]int{0, 1, 3}, "@0, @1 and @3"},
		{[]int{0, 1, 2, 4}, "@0, @1, @2 and @4"},
	}
	for _, c := range cases {
		if got := formatSlotList(c.in); got != c.want {
			t.Errorf("formatSlotList(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
