package codex32

// MKChecksumSymbols computes the BCH checksum symbols for an mk1 string's data
// part. dataSyms are the 5-bit data symbols (the string-layer header symbols
// followed by the bytes_to_5bit-encoded fragment); long selects the long code
// (15-symbol checksum) over the regular code (13-symbol checksum). The returned
// symbols, rendered after the rendered data part, complete a BCH-valid mk1
// string (ValidMK == true).
//
// CRITICAL (C-1): this is the GENERATE counterpart of verifyMDMK and MUST use
// the md/mk initial residue POLYMOD_INIT (mdmkPolymodInitLo), NOT codex32's
// residue init of 1. The engine is built exactly like verifyMDMK — mk1
// generator + mk1 target + POLYMOD_INIT — then fed inputHRP("mk") + the rendered
// data part + inputTarget(); the resulting residue IS the checksum. Cloning
// NewSeed's checksum step verbatim would use codex32's residue init and emit
// strings that silently fail ValidMK. The round-trip parity test (mkencode_test
// and mk's TestEncode*) is the guard. Pure-stdlib; TinyGo-safe (uint64 only).
func MKChecksumSymbols(dataSyms []byte, long bool) []byte {
	var generator []fe
	var targetHi, targetLo uint64
	var n int
	if long {
		generator = newLongChecksum().generator
		targetHi, targetLo = mkLongTargetHi, mkLongTargetLo
		n = mdmkLongSyms
	} else {
		generator = newShortChecksum().generator
		targetHi, targetLo = mkRegularTargetHi, mkRegularTargetLo
		n = mdmkShortSyms
	}
	e := &engine{
		generator: generator,
		residue:   unpackSyms(0, mdmkPolymodInitLo, n), // POLYMOD_INIT — NOT codex32's 1
		target:    unpackSyms(targetHi, targetLo, n),
	}
	// Build the rendered (lowercase) data part and feed it through the engine.
	// The engine decodes runes internally, so the data part is passed as a
	// string, exactly like the verify path.
	e.inputHRP("mk")
	data := make([]byte, len(dataSyms))
	for i, s := range dataSyms {
		data[i] = fe(s).rune()
	}
	if err := e.inputData(string(data)); err != nil {
		// dataSyms are 5-bit values (0..31) rendered to valid bech32 runes, so
		// inputData never fails here; return nil defensively rather than panic.
		return nil
	}
	e.inputTarget()
	out := make([]byte, n)
	for i, r := range e.residue {
		out[i] = byte(r)
	}
	return out
}
