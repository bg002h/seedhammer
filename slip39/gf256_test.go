package slip39

import "testing"

func TestGF256MulInvDiv(t *testing.T) {
	// Known GF(256) Rijndael (0x11b, generator 3) identities.
	if got := gfMul(0x53, 0xCA); got != 0x01 { // 0x53 and 0xCA are AES-inverse pair
		t.Errorf("gfMul(0x53,0xCA)=%#x want 0x01", got)
	}
	if got := gfMul(3, 0); got != 0 {
		t.Errorf("gfMul(_,0)=%#x want 0", got)
	}
	for a := 1; a < 256; a++ {
		if gfMul(byte(a), gfInv(byte(a))) != 1 {
			t.Fatalf("a*inv(a)!=1 for a=%d", a)
		}
		if gfDiv(byte(a), byte(a)) != 1 {
			t.Fatalf("a/a!=1 for a=%d", a)
		}
	}
	if gfAdd(0xAA, 0x55) != 0xFF {
		t.Errorf("gfAdd is XOR")
	}
}
