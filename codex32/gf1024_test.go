package codex32

import "testing"

// carrylessGf32Mul is the Rust reference GF(32) multiply (mk-codec
// bch_decode.rs gf32_mul): carryless multiply mod α⁵+α³+1 (mask 0b0_1001).
// Used ONLY to cross-check that the fork's log-table fe.Mul is the SAME
// field, which is what licenses building GF(1024) on fe.Mul (SPEC §3.1).
func carrylessGf32Mul(a, b uint8) uint8 {
	const reduce = 0b0_1001
	var result uint8
	for i := 0; i < 5; i++ {
		if (b>>uint(i))&1 != 0 {
			result ^= a
		}
		carry := (a >> 4) & 1
		a = (a << 1) & 0x1f
		if carry != 0 {
			a ^= reduce
		}
	}
	return result
}

func TestForkGf32MatchesCarryless(t *testing.T) {
	for a := 0; a < 32; a++ {
		for b := 0; b < 32; b++ {
			got := fe(a).Mul(fe(b))
			want := carrylessGf32Mul(uint8(a), uint8(b))
			if uint8(got) != want {
				t.Fatalf("fe(%d).Mul(fe(%d)) = %d, carryless = %d", a, b, got, want)
			}
		}
	}
}

func TestAlphaPowersMatchInvLogTable(t *testing.T) {
	// Powers of α=feZ(=2) via fe.Mul must reproduce invLogTbl, pinning the
	// fork field to the standard codex32 GF(32) the Rust cross-checks.
	a := fe(1)
	for i := 0; i < 31; i++ {
		if a != invLogTbl[i] {
			t.Fatalf("α^%d = %d, want %d", i, a, invLogTbl[i])
		}
		a = a.Mul(feZ)
	}
	if a != fe(1) {
		t.Fatalf("α^31 = %d, want 1", a)
	}
}

func TestZetaCubeRoot(t *testing.T) {
	zeta := gf1024{lo: feQ, hi: feP} // {0,1}
	if got := zeta.mul(zeta); got != zeta.add(gf1024One) {
		t.Fatalf("ζ² = %+v, want ζ+1 = %+v", got, zeta.add(gf1024One))
	}
	if got := zeta.mul(zeta).mul(zeta); got != gf1024One {
		t.Fatalf("ζ³ = %+v, want 1", got)
	}
}

func TestBetaOrder93(t *testing.T) {
	p := gf1024One
	for j := 1; j <= 93; j++ {
		p = p.mul(betaGf1024)
		if p == gf1024One && j != 93 {
			t.Fatalf("β returned to 1 prematurely at exponent %d", j)
		}
	}
	if p != gf1024One {
		t.Fatalf("β^93 = %+v, want 1", p)
	}
}

func TestGammaOrder1023(t *testing.T) {
	for _, q := range []uint32{341, 93, 33} { // 1023/{3,11,31}
		if gammaGf1024.pow(q) == gf1024One {
			t.Fatalf("γ^%d = 1 (γ not order 1023)", q)
		}
	}
	if gammaGf1024.pow(1023) != gf1024One {
		t.Fatalf("γ^1023 != 1")
	}
}

func TestGeneratorConsecutiveRoots(t *testing.T) {
	// g(x) = x^n + Σ generator[j]·x^{n-1-j} (generator MSB-first, monic
	// leading implied). Its 8 consecutive defining roots are β^{77..84}
	// (regular) and γ^{1019..1026} (long); verify g evaluates to zero there.
	evalGen := func(gen []fe, x gf1024) gf1024 {
		n := len(gen)
		acc := x.pow(uint32(n)) // x^n (leading monic term)
		for j := 0; j < n; j++ {
			acc = acc.add(gf1024FromFe(gen[j]).mul(x.pow(uint32(n - 1 - j))))
		}
		return acc
	}
	gReg := newShortChecksum().generator
	for j := uint32(77); j <= 84; j++ {
		if !evalGen(gReg, betaGf1024.pow(j)).isZero() {
			t.Fatalf("g_regular(β^%d) != 0", j)
		}
	}
	gLong := newLongChecksum().generator
	for j := uint32(1019); j <= 1026; j++ {
		if !evalGen(gLong, gammaGf1024.pow(j)).isZero() {
			t.Fatalf("g_long(γ^%d) != 0", j)
		}
	}
}
