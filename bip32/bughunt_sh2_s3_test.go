package bip32

import "testing"

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

// Bug 35: an unhardened path element of 2^31 or more is accepted and silently
// becomes a hardened index. BIP-32: "The normal child keys use indices 0
// through 2^31-1. The hardened child keys use indices 2^31 through 2^32-1" and a
// hardened step is written i' / ih; BIP-380: "/NUM or /NUMh path elements to
// indicate the unhardened or hardened derivation steps", so an unmarked NUM is
// unhardened and must be below 2^31.
func TestBughuntSh235ParsePathUnhardenedOverflow(t *testing.T) {
	t.Skip("deferred-minor: sh-parsepath-unhardened-overflow")
	// Positive controls: the boundary values parse to the right indices.
	for _, c := range []struct {
		in   string
		want uint32
	}{
		{"m/2147483647", 2147483647},
		{"m/0h", 0x80000000},
		{"m/0'", 0x80000000},
		{"m/2147483647h", 0xFFFFFFFF},
	} {
		p, err := ParsePath(c.in)
		if err != nil || len(p) != 1 || p[0] != c.want {
			t.Fatalf("control ParsePath(%q) = %v, %v; want [%d]", c.in, p, err, c.want)
		}
	}
	// Existing behaviour that must stay: the hardened overflow is refused.
	if p, err := ParsePath("m/2147483648h"); err == nil {
		t.Fatalf("control ParsePath(m/2147483648h) accepted: %v", p)
	}
	for _, in := range []string{"m/2147483648", "m/4294967295", "m/48h/2147483648/0"} {
		if p, err := ParsePath(in); err == nil {
			t.Errorf("ParsePath(%q) accepted as %v (re-encodes as %q)", in, p, p.Encode())
		}
	}
	for _, in := range []string{"2147483648", "4294967295"} {
		if v, err := ParsePathElement(in); err == nil {
			t.Errorf("ParsePathElement(%q) accepted as %#x", in, v)
		}
	}
}
