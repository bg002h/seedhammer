package codex32

// gf1024 is an element of GF(1024)=GF(32²), represented as lo + hi·ζ with
// the field relation ζ²=ζ+1. Built on the fork's GF(32) (fe.Mul); this is a
// faithful port of the constellation Gf1024 (mk-codec bch_decode.rs), whose
// carryless GF(32) multiply is identical to the fork's log-table fe.Mul
// (pinned by TestForkGf32MatchesCarryless). Pure; no allocation.
type gf1024 struct {
	lo, hi fe
}

var (
	gf1024Zero = gf1024{lo: feQ, hi: feQ} // {0,0}
	gf1024One  = gf1024{lo: feP, hi: feQ} // {1,0}

	// betaGf1024 = β = 8·ζ (feQ + feG·ζ), order 93, the regular code's
	// BCH-defining primitive element. gammaGf1024 = γ = 25 + 6·ζ
	// (feE + feX·ζ), order 1023, the long code's. (bch_decode.rs:204-211.)
	betaGf1024  = gf1024{lo: feQ, hi: feG} // {0,8}
	gammaGf1024 = gf1024{lo: feE, hi: feX} // {25,6}
)

const (
	regularJStart uint32 = 77   // β^{77..84} are the regular generator's roots
	longJStart    uint32 = 1019 // γ^{1019..1026} the long generator's roots
)

func gf1024FromFe(v fe) gf1024 { return gf1024{lo: v, hi: feQ} }

func (a gf1024) add(b gf1024) gf1024 {
	return gf1024{lo: a.lo ^ b.lo, hi: a.hi ^ b.hi}
}

func (a gf1024) isZero() bool { return a.lo == feQ && a.hi == feQ }

// mul multiplies in GF(1024) via the 4-subfield identity (ζ²=ζ+1):
//
//	(lo+hi·ζ)(lo'+hi'·ζ) = (ll+hh) + (lh+hl+hh)·ζ.
func (a gf1024) mul(b gf1024) gf1024 {
	ll := a.lo.Mul(b.lo)
	lh := a.lo.Mul(b.hi)
	hl := a.hi.Mul(b.lo)
	hh := a.hi.Mul(b.hi)
	return gf1024{lo: ll ^ hh, hi: lh ^ hl ^ hh}
}

// pow is square-and-multiply. exp is a small fixed exponent (j_start, 1022,
// position degrees), never attacker-controlled timing-sensitive material.
func (a gf1024) pow(exp uint32) gf1024 {
	base := a
	res := gf1024One
	for exp > 0 {
		if exp&1 == 1 {
			res = res.mul(base)
		}
		base = base.mul(base)
		exp >>= 1
	}
	return res
}

// inv is the Fermat inverse a^(2^10-2)=a^1022. Callers guard against inverting
// zero (zero syndromes / Λ'(X⁻¹)≠0) before reaching here.
func (a gf1024) inv() gf1024 { return a.pow(1022) }
