package codex32

import "errors"

// errNotMD1 is returned by MDDataSymbols for any string that is not a
// BCH-valid md1 string.
var errNotMD1 = errors.New("codex32: not a valid md1 string")

// MDDataSymbols returns the 5-bit data symbols of a BCH-valid md1 string
// (string-layer header + payload) with the 13-symbol regular checksum stripped.
// Each byte is a 5-bit value (0..31). md1 is regular-code only. Pure-stdlib.
//
// The caller (the md package) checks symbols[0]&1 for the chunked flag, then
// repacks the symbols 5-bit→8-bit (MSB-first) into the payload byte stream.
func MDDataSymbols(s string) ([]byte, error) {
	if !ValidMD(s) {
		return nil, errNotMD1
	}
	_, data := splitHRP(s)
	if len(data) < mdmkShortSyms {
		return nil, errNotMD1 // unreachable: ValidMD requires >= 13 data symbols.
	}
	body := data[:len(data)-mdmkShortSyms]
	syms := make([]byte, 0, len(body))
	for _, c := range body {
		e, ok := feFromRune(c)
		if !ok {
			return nil, errNotMD1 // unreachable: ValidMD verified the charset.
		}
		syms = append(syms, byte(e))
	}
	return syms, nil
}
