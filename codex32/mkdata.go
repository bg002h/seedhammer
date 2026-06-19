package codex32

import "errors"

// errNotMK1 is returned by MKDataSymbols for any string that is not a
// BCH-valid mk1 string.
var errNotMK1 = errors.New("codex32: not a valid mk1 string")

// MKDataSymbols returns the 5-bit data symbols of a BCH-valid mk1 string —
// the string-layer header symbols followed by the bytes_to_5bit-encoded
// fragment — with the BCH checksum (13 regular / 15 long) stripped. Each
// returned byte is a 5-bit value (0..31). It errors if s is not a BCH-valid
// mk1 string. Pure-stdlib; no key-derivation deps.
//
// Callers (the mk package) parse the string-layer header off the front and
// repack the remaining fragment symbols 5-bit→8-bit with strict padding checks.
func MKDataSymbols(s string) ([]byte, error) {
	if !ValidMK(s) {
		return nil, errNotMK1
	}
	_, data := splitHRP(s)
	// Checksum-symbol count by the same data-part length bracket ValidMK uses.
	var checksum int
	switch n := len(data); {
	case n >= mkRegularMinLen && n <= mkRegularMaxLen:
		checksum = mdmkShortSyms
	case n >= mkLongMinLen && n <= mkLongMaxLen:
		checksum = mdmkLongSyms
	default:
		return nil, errNotMK1 // unreachable: ValidMK already rejected these lengths.
	}
	body := data[:len(data)-checksum]
	syms := make([]byte, 0, len(body))
	for _, c := range body {
		e, ok := feFromRune(c)
		if !ok {
			return nil, errNotMK1 // unreachable: ValidMK already verified the charset.
		}
		syms = append(syms, byte(e))
	}
	return syms, nil
}
