package codex32

import "strings"

// Edit is one substitution the decoder applied: the full-string rune index
// (== byte index for the ASCII bech32 charset; Phase B indexes []rune(frag))
// and the before/after bech32 characters (for the GUI's per-position confirm
// diff).
type Edit struct {
	Pos      int
	Was, Now byte
}

// CorrectionResult is a unique within-radius (≤4 substitutions) correction that
// RE-VERIFIES as a valid codeword.
type CorrectionResult struct {
	Corrected string
	Edits     []Edit
}

// synCount is the number of BCH syndromes (the codes are BCH(•,•,8), t=4).
const synCount = 8

// bmMaxLen bounds the Berlekamp-Massey connection-polynomial array. Over 8
// syndromes deg(Λ)≤L≤8 (classic BM invariant), so 9 coefficients suffice; a
// resize that would exceed this is treated as uncorrectable (correctable words
// yield deg≤4 and never approach it). See SPEC §2.7 / plan M-3.
const bmMaxLen = synCount + 1

// bchParams selects the per-code BCH machinery. eng is a fresh verifier engine
// (generator + POLYMOD_INIT residue + target) — the SAME constants the
// verifier uses (no second copy; SPEC §2.5). alpha is β (regular) or γ (long).
type bchParams struct {
	eng    *engine
	alpha  gf1024
	jStart uint32
	nSyms  int
}

// paramsForHRP returns the decoder parameters for a fragment, mirroring the
// verifiers' dispatch exactly: ms by total length (New: 48..93 short / 125..127
// long), md regular-only (data ≥13), mk by data-part length (14..93 regular /
// 96..108 long). ok=false for any out-of-bracket / unknown HRP.
func paramsForHRP(hrp string, total, dataLen int) (bchParams, bool) {
	switch strings.ToLower(hrp) {
	case "ms":
		switch {
		case total >= shortCodeMinLength && total <= shortCodeMaxLength:
			return bchParams{newShortChecksum(), betaGf1024, regularJStart, shortChecksumLen}, true
		case total >= longCodeMinLength && total <= longCodeMaxLength:
			return bchParams{newLongChecksum(), gammaGf1024, longJStart, longChecksumLen}, true
		}
	case "md":
		if dataLen >= mdmkShortSyms {
			eng := &engine{
				generator: newShortChecksum().generator,
				residue:   unpackSyms(0, mdmkPolymodInitLo, mdmkShortSyms),
				target:    unpackSyms(mdRegularTargetHi, mdRegularTargetLo, mdmkShortSyms),
			}
			return bchParams{eng, betaGf1024, regularJStart, mdmkShortSyms}, true
		}
	case "mk":
		switch {
		case dataLen >= mkRegularMinLen && dataLen <= mkRegularMaxLen:
			eng := &engine{
				generator: newShortChecksum().generator,
				residue:   unpackSyms(0, mdmkPolymodInitLo, mdmkShortSyms),
				target:    unpackSyms(mkRegularTargetHi, mkRegularTargetLo, mdmkShortSyms),
			}
			return bchParams{eng, betaGf1024, regularJStart, mdmkShortSyms}, true
		case dataLen >= mkLongMinLen && dataLen <= mkLongMaxLen:
			eng := &engine{
				generator: newLongChecksum().generator,
				residue:   unpackSyms(0, mdmkPolymodInitLo, mdmkLongSyms),
				target:    unpackSyms(mkLongTargetHi, mkLongTargetLo, mdmkLongSyms),
			}
			return bchParams{eng, gammaGf1024, longJStart, mdmkLongSyms}, true
		}
	}
	return bchParams{}, false
}

// Correct attempts to error-correct an invalid codex32-family string of the
// given parsed code. Returns (result, true) ONLY for a unique within-radius
// (≤4 substitutions) correction that RE-VERIFIES as a valid codeword; (_, false)
// otherwise (uncorrectable / >radius / re-verify fail). It NEVER guesses and
// NEVER auto-applies — the caller confirms result.Edits against the source card.
func Correct(frag string) (CorrectionResult, bool) {
	hrp, data := splitHRP(frag)
	p, ok := paramsForHRP(hrp, len(frag), len(data))
	if !ok {
		return CorrectionResult{}, false
	}
	if err := p.eng.inputHRP(hrp); err != nil {
		return CorrectionResult{}, false
	}
	if err := p.eng.inputData(data); err != nil {
		return CorrectionResult{}, false
	}
	// Syndrome polynomial = residue ⊕ target, reversed from the engine's
	// MSB-first layout to LSB-first coeffs[i]=coeff of xⁱ (SPEC §2.6).
	n := p.nSyms
	coeffs := make([]fe, n)
	for i := 0; i < n; i++ {
		coeffs[i] = p.eng.residue[n-1-i] ^ p.eng.target[n-1-i]
	}
	positions, mags, ok := decodeErrors(coeffs, len(data), p.alpha, p.jStart)
	if !ok || len(positions) == 0 {
		return CorrectionResult{}, false
	}
	// Apply: the data part begins at full-string index len(hrp)+1 (HRP + the
	// '1' separator). Preserve the fragment's case so the result re-verifies.
	useUpper := fragUsesUpper(frag)
	offset := len(hrp) + 1
	r := []rune(frag)
	edits := make([]Edit, 0, len(positions))
	for i, k := range positions {
		abs := offset + k
		if abs < 0 || abs >= len(r) {
			return CorrectionResult{}, false
		}
		was := r[abs]
		orig, ok := feFromRune(was)
		if !ok {
			return CorrectionResult{}, false
		}
		now := feToByte(orig.Add(mags[i]), useUpper)
		r[abs] = rune(now)
		edits = append(edits, Edit{Pos: abs, Was: byte(was), Now: now})
	}
	corrected := string(r)
	if !reverify(hrp, corrected) {
		return CorrectionResult{}, false // mandatory re-verify (SPEC §2.2)
	}
	return CorrectionResult{Corrected: corrected, Edits: edits}, true
}

// reverify runs the SAME verifier the device uses, dispatched by HRP.
func reverify(hrp, s string) bool {
	switch strings.ToLower(hrp) {
	case "ms":
		_, err := New(s)
		return err == nil
	case "md":
		return ValidMD(s)
	case "mk":
		return ValidMK(s)
	}
	return false
}

// fragUsesUpper reports whether the fragment is upper-cased (no lowercase
// letter). Valid codex32 strings are single-cased; the corrected char matches.
func fragUsesUpper(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return false
		}
	}
	return true
}

// feToByte renders a GF(32) symbol as a bech32 char in the requested case.
func feToByte(v fe, upper bool) byte {
	b := byte(v.rune()) // lowercase
	if upper && b >= 'a' && b <= 'z' {
		b -= 'a' - 'A'
	}
	return b
}

// decodeErrors runs the BCH pipeline over LSB-first GF(32) coeffs (the residue⊕
// target, length nSyms) for a data part of length L. Returns ascending-sorted
// data-part positions and matching GF(32) magnitudes, or ok=false. Port of the
// Rust decode_errors (bch_decode.rs:550).
func decodeErrors(coeffs []fe, L int, alpha gf1024, jStart uint32) ([]int, []fe, bool) {
	syn := computeSyndromes(coeffs, alpha, jStart)
	allZero := true
	for _, s := range syn {
		if !s.isZero() {
			allZero = false
			break
		}
	}
	if allZero {
		return nil, nil, false // already a codeword; nothing to correct
	}
	lam, lamLen, ok := berlekampMassey(syn)
	if !ok {
		return nil, nil, false
	}
	deg := lamLen - 1
	if deg == 0 || deg > 4 {
		return nil, nil, false // >4 errors exceeds t=4 capacity
	}
	degrees, ok := chienSearch(lam, lamLen, L, alpha)
	if !ok || len(degrees) != deg {
		return nil, nil, false
	}
	mags, ok := forney(syn, lam, lamLen, degrees, alpha, jStart)
	if !ok {
		return nil, nil, false
	}
	// Translate polynomial degree d -> data index k = L-1-d, then sort
	// ascending by position (insertion sort: ≤4 elements, TinyGo-friendly).
	pos := make([]int, len(degrees))
	mg := make([]fe, len(degrees))
	for i, d := range degrees {
		if d >= L {
			return nil, nil, false
		}
		k := L - 1 - d
		j := i
		for j > 0 && pos[j-1] > k {
			pos[j] = pos[j-1]
			mg[j] = mg[j-1]
			j--
		}
		pos[j] = k
		mg[j] = mags[i]
	}
	return pos, mg, true
}

// computeSyndromes: S_m = E(α^{jStart+m}) for m=0..7 (bch_decode.rs:286).
func computeSyndromes(coeffs []fe, alpha gf1024, jStart uint32) [synCount]gf1024 {
	var syn [synCount]gf1024
	aj := alpha.pow(jStart)
	for m := 0; m < synCount; m++ {
		syn[m] = horner(coeffs, aj)
		aj = aj.mul(alpha)
	}
	return syn
}

// horner evaluates a GF(32)-coefficient polynomial (coeffs[i]=coeff of xⁱ) at a
// GF(1024) point, high index first.
func horner(coeffs []fe, x gf1024) gf1024 {
	acc := gf1024Zero
	for i := len(coeffs) - 1; i >= 0; i-- {
		acc = acc.mul(x).add(gf1024FromFe(coeffs[i]))
	}
	return acc
}

// hornerExt is horner for GF(1024)-coefficient polynomials.
func hornerExt(coeffs []gf1024, x gf1024) gf1024 {
	acc := gf1024Zero
	for i := len(coeffs) - 1; i >= 0; i-- {
		acc = acc.mul(x).add(coeffs[i])
	}
	return acc
}

// berlekampMassey returns the error-locator Λ (Λ(0)=1) and its length, or
// ok=false on buffer overflow. Fixed-size arrays; no heap (SPEC §2.7). Port of
// bch_decode.rs:324.
func berlekampMassey(syn [synCount]gf1024) ([bmMaxLen]gf1024, int, bool) {
	var lam, prev [bmMaxLen]gf1024
	lam[0] = gf1024One
	prev[0] = gf1024One
	lamLen, prevLen := 1, 1
	l := 0
	m := 1
	b := gf1024One

	for k := 0; k < synCount; k++ {
		d := syn[k]
		for i := 1; i <= l; i++ {
			if i <= k && i < lamLen {
				d = d.add(lam[i].mul(syn[k-i]))
			}
		}
		if d.isZero() {
			m++
			continue
		}
		scale := d.mul(b.inv())
		newLen := lamLen
		if prevLen+m > newLen {
			newLen = prevLen + m
		}
		if newLen > bmMaxLen {
			return lam, 0, false
		}
		if 2*l <= k {
			t := lam // value copy of the whole array
			tLen := lamLen
			lamLen = newLen
			for i := 0; i < prevLen; i++ {
				lam[i+m] = lam[i+m].add(scale.mul(prev[i]))
			}
			l = k + 1 - l
			prev = t
			prevLen = tLen
			b = d
			m = 1
		} else {
			lamLen = newLen
			for i := 0; i < prevLen; i++ {
				lam[i+m] = lam[i+m].add(scale.mul(prev[i]))
			}
			m++
		}
	}
	for lamLen > 1 && lam[lamLen-1].isZero() {
		lamLen--
	}
	return lam, lamLen, true
}

// chienSearch returns the polynomial degrees d∈[0,L) with Λ(α^{-d})=0, or
// ok=false if the root count != deg(Λ) (bch_decode.rs:387).
func chienSearch(lam [bmMaxLen]gf1024, lamLen, L int, alpha gf1024) ([]int, bool) {
	deg := lamLen - 1
	if deg == 0 {
		return nil, true
	}
	degrees := make([]int, 0, deg)
	aInv := alpha.inv()
	cur := gf1024One
	for d := 0; d < L; d++ {
		if hornerExt(lam[:lamLen], cur).isZero() {
			degrees = append(degrees, d)
		}
		cur = cur.mul(aInv)
	}
	if len(degrees) != deg {
		return nil, false
	}
	return degrees, true
}

// forney returns the GF(32) error magnitudes for the located degrees, or
// ok=false on any guard (Λ'(X⁻¹)=0, mag∉GF(32), mag=0). Port of
// bch_decode.rs:421.
func forney(syn [synCount]gf1024, lam [bmMaxLen]gf1024, lamLen int, degrees []int, alpha gf1024, jStart uint32) ([]fe, bool) {
	// Ω(x) = S(x)·Λ(x) mod x⁸.
	var omega [synCount]gf1024
	for i := 0; i < synCount; i++ {
		for j := 0; j < lamLen; j++ {
			if i+j < synCount {
				omega[i+j] = omega[i+j].add(syn[i].mul(lam[j]))
			}
		}
	}
	// Λ'(x): char-2 formal derivative keeps only odd-power terms.
	var lamPrime [synCount]gf1024
	lpLen := lamLen - 1
	if lpLen < 0 {
		lpLen = 0
	}
	for i := 1; i < lamLen; i++ {
		if i%2 == 1 {
			lamPrime[i-1] = lam[i]
		}
	}
	shift := uint32(0)
	if jStart > 0 {
		shift = jStart - 1
	}
	mags := make([]fe, 0, len(degrees))
	for _, d := range degrees {
		xk := alpha.pow(uint32(d))
		xkInv := xk.inv()
		omegaVal := hornerExt(omega[:], xkInv)
		lampVal := hornerExt(lamPrime[:lpLen], xkInv)
		if lampVal.isZero() {
			return nil, false
		}
		xkShift := xkInv.pow(shift) // X_k^{1-jStart}
		mag := xkShift.mul(omegaVal.mul(lampVal.inv()))
		if mag.hi != feQ {
			return nil, false // magnitude must lie in GF(32)
		}
		if mag.lo == feQ {
			return nil, false // zero magnitude ⇒ not a real error
		}
		mags = append(mags, mag.lo)
	}
	return mags, true
}
