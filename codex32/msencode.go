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

// EncodeMS1Preimage encodes a hashlock preimage X as the ms1 kind-0x03 plate
// string: NewSeed("ms", 0, "hash", 's', [0x03‖X]). It is the Go port of
// ms_codec::encode(Tag::HASH, &Payload::Preimage(x)) (SPEC_ms_hashlock §1
// rule 2) and is DOWNSTREAM of it — the Rust side decides the wire form.
//
// The id is the FIXED literal "hash" and there is no parameter for it. That is
// the whole point: NewSeed will mint a kind-0x03 payload under id "entr" quite
// happily, and the Rust encoder refuses that shape outright with
// Error::TagKindMismatch, so the id must not be reachable from a caller here
// either. A mistagged plate satisfies the wide IsPreimage, fails
// IsPreimagePlate, and is therefore an operator's only backup of a spend
// secret, on steel, that no tool will read.
//
// The returned string is SECRET (it embeds the preimage); the caller scrubs.
// DecodeMS1Preimage(New(EncodeMS1Preimage(x))) == x.
func EncodeMS1Preimage(x [32]byte) (string, error) {
	payload := make([]byte, 0, 1+len(x))
	payload = append(payload, msPrefixPreimage)
	payload = append(payload, x[:]...)
	s, err := NewSeed("ms", 0, "hash", 's', payload)
	if err != nil {
		return "", err
	}
	return s.String(), nil
}
