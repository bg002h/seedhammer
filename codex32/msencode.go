package codex32

// EncodeMS1 encodes BIP-39 entropy as an m-format ms1 secret string — the
// net-new inverse of DecodeMS1 (the fork shipped only the decode side). It is
// the exact recipe NewSeed("ms", 0, "entr", 's', [0x00‖entropy]) (T6a-1, C4):
//
//   - hrp "ms", threshold 0 (the unshared secret), share index lowercase 's';
//   - the 4-char id is the FIXED literal "entr" (NOT fingerprint-derived);
//   - the codex32 data payload is the 0x00 entr prefix byte (msPrefixEntr,
//     mspayload.go:5-12) followed by the raw entropy — entr carries NO language
//     byte, so this is English/entr-only this cycle (the 0x02 mnem-prefix
//     language-carrying variant is a follow-on).
//
// entropy must be a valid BIP-39 length (16/20/24/28/32 bytes); any other length
// returns errMSBadLength. The returned string is SECRET (it embeds the seed
// entropy); the caller scrubs. DecodeMS1(New(EncodeMS1(e))) == e.
func EncodeMS1(entropy []byte) (string, error) {
	switch len(entropy) {
	case 16, 20, 24, 28, 32:
	default:
		return "", errMSBadLength
	}
	payload := make([]byte, 0, len(entropy)+1)
	payload = append(payload, msPrefixEntr)
	payload = append(payload, entropy...)
	s, err := NewSeed("ms", 0, "entr", 's', payload)
	if err != nil {
		return "", err
	}
	return s.String(), nil
}
