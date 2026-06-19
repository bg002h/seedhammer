package codex32

// MDChecksumSymbols computes the 13 regular-BCH checksum symbols for an md1
// string's data part. dataSyms are the 5-bit data symbols (the payload symbols,
// or a chunk header followed by chunk-payload symbols). The returned symbols,
// rendered after "md1" + the rendered data part, complete a BCH-valid md1
// string (ValidMD == true).
//
// CRITICAL (I-7): this is the GENERATE counterpart of verifyMDMK/ValidMD and
// MUST use the md/mk initial residue POLYMOD_INIT (mdmkPolymodInitLo), NOT
// codex32's residue init of 1. The engine is built exactly like ValidMD's
// verify path — regular generator + md regular target + POLYMOD_INIT — then fed
// inputHRP("md") + the rendered data part + inputTarget(); the resulting residue
// IS the checksum. md1 is regular-only (md-codec dropped the long code). The
// round-trip parity test (mdencode_test + md's TestEncodeMD1StringGoldens) is
// the guard. Pure-stdlib; TinyGo-safe (uint64 only). Analogue of
// MKChecksumSymbols (mkencode.go).
func MDChecksumSymbols(dataSyms []byte) []byte {
	n := mdmkShortSyms
	e := &engine{
		generator: newShortChecksum().generator,
		residue:   unpackSyms(0, mdmkPolymodInitLo, n), // POLYMOD_INIT — NOT codex32's 1
		target:    unpackSyms(mdRegularTargetHi, mdRegularTargetLo, n),
	}
	e.inputHRP("md")
	data := make([]byte, len(dataSyms))
	for i, s := range dataSyms {
		data[i] = fe(s).rune()
	}
	if err := e.inputData(string(data)); err != nil {
		// dataSyms are 5-bit values rendered to valid bech32 runes, so inputData
		// never fails here; return nil defensively rather than panic.
		return nil
	}
	e.inputTarget()
	out := make([]byte, n)
	for i, r := range e.residue {
		out[i] = byte(r)
	}
	return out
}

// AssembleMD1 renders an md1 string: "md1" + each data symbol + the 13-symbol
// regular BCH checksum. md1 is regular-only (no long code), so unlike
// assembleMK1 there is no regular/long selection. The per-string codex32.ValidMD
// gate proves the checksum is correct.
func AssembleMD1(dataSyms []byte) string {
	ck := MDChecksumSymbols(dataSyms)
	buf := make([]byte, 0, 3+len(dataSyms)+len(ck))
	buf = append(buf, "md1"...)
	for _, s := range dataSyms {
		buf = append(buf, fe(s).rune())
	}
	for _, s := range ck {
		buf = append(buf, fe(s).rune())
	}
	return string(buf)
}
