package slip39

// GF(2^8) Rijndael field for SLIP-0039 Shamir. Reduction polynomial
// x^8+x^4+x^3+x+1 (0x11b); multiplicative generator 3. Tables built once.
// Port of mnemonic_toolkit::slip39::gf256. NO math/big.

const gf256ReductionPoly = 0x11b

var gf256Exp [256]byte
var gf256Log [256]byte

func init() {
	x := uint16(1)
	for i := 0; i < 255; i++ {
		gf256Exp[i] = byte(x)
		gf256Log[x] = byte(i)
		x = (x << 1) ^ x // multiply by generator 3 = (x+1) in GF(2^8)
		if x&0x100 != 0 {
			x ^= gf256ReductionPoly
		}
	}
	gf256Exp[255] = 1 // cyclic: exp[255]==exp[0]
}

func gfAdd(a, b byte) byte { return a ^ b }

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	s := uint16(gf256Log[a]) + uint16(gf256Log[b])
	if s >= 255 {
		s -= 255
	}
	return gf256Exp[s]
}

// gfInv: multiplicative inverse. Precondition a != 0 (panics otherwise —
// unreachable in the combine path, see combine.go / SPEC §4.4).
func gfInv(a byte) byte {
	if a == 0 {
		panic("slip39: gfInv(0)")
	}
	return gf256Exp[(255-uint16(gf256Log[a]))%255]
}

// gfDiv: a/b = a*inv(b). Precondition b != 0.
func gfDiv(a, b byte) byte { return gfMul(a, gfInv(b)) }
