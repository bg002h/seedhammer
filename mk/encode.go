package mk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
	"seedhammer.com/codex32"
)

// Encode errors.
var (
	errEncodeStubCount = errors.New("mk: encode requires stub_count >= 1")
	errEncodeXpub      = errors.New("mk: invalid account xpub")
	errEncodeInvariant = errors.New("mk: xpub depth/child does not match path")
	errEncodeFP        = errors.New("mk: invalid fingerprint")
	errEncodePath      = errors.New("mk: invalid path")
	errEncodeCompact   = errors.New("mk: compact-73 length mismatch")
)

const (
	// CHUNKED_FRAGMENT_LONG_BYTES — the max bytes per chunk fragment of the
	// cross-chunk stream (mk-codec chunk.rs). The stream is always > 53 bytes for
	// a 1-stub card (~84 B), so a T4 card always splits into >= 2 chunks.
	chunkedFragmentBytes = 53
)

// Encode is the inverse of Decode: it builds a complete, BCH-valid set of mk1
// chunk strings (>= 2) from a Card. It is deterministic — same Card yields
// byte-identical strings (the chunk_set_id is derived from the bytecode, never
// from randomness). The round-trip Decode(Encode(card)) == card and per-chunk
// codex32.ValidMK are the load-bearing correctness gates.
//
// Encode handles ONLY public material: it parses the account xpub and never
// touches private keys.
func Encode(card Card) ([]string, error) {
	bytecode, err := encodeBytecode(card)
	if err != nil {
		return nil, err
	}
	return encodeChunks(bytecode), nil
}

// encodeBytecode builds the bytecode body (the inverse of decodeBytecode):
//
//	header(1) | stub_count(1) | stubs(4*N) | [fp(4) iff header&0x04] | path | compact73
func encodeBytecode(card Card) ([]byte, error) {
	if len(card.Stubs) == 0 {
		return nil, errEncodeStubCount
	}
	if len(card.Stubs) > 0xff {
		return nil, errEncodeStubCount
	}

	// Resolve the path string to raw components (hardened bit included).
	comps, err := resolvePath(card.Path)
	if err != nil {
		return nil, err
	}

	// Build compact-73 from the xpub, validating the depth/child invariant.
	compact, err := compactFromXpub(card.Xpub, comps)
	if err != nil {
		return nil, err
	}

	// Header byte: version nibble 0; fingerprint flag (bit 2) iff a fingerprint
	// is present. All other bits (reservedMask) are zero.
	var fp []byte
	hdr := byte(0x00)
	if card.Fingerprint != "" {
		fp, err = hex.DecodeString(card.Fingerprint)
		if err != nil || len(fp) != 4 {
			return nil, errEncodeFP
		}
		hdr |= fingerprintFlagMask
	}

	pathBytes, err := encodePath(comps)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, 2+4*len(card.Stubs)+len(fp)+len(pathBytes)+xpubCompactBytes)
	out = append(out, hdr)
	out = append(out, byte(len(card.Stubs)))
	for _, s := range card.Stubs {
		out = append(out, s[:]...)
	}
	out = append(out, fp...) // empty when no fingerprint
	out = append(out, pathBytes...)
	out = append(out, compact...)
	return out, nil
}

// resolvePath parses a path string ("m/84'/0'/0'") into raw uint32 components
// with the hardened bit set as appropriate. "m" (depth 0) yields an empty slice.
func resolvePath(path string) ([]uint32, error) {
	p, err := bip32.ParsePath(path)
	if err != nil {
		return nil, errEncodePath
	}
	if len(p) > maxPathComponents {
		return nil, errPathTooDeep
	}
	return []uint32(p), nil
}

// compactFromXpub parses the base58 account xpub and serializes the 73-byte
// compact form: version(4) | parentFP(4) | chainCode(32) | compressedPubKey(33).
// It enforces the encode invariant: the xpub's depth equals len(comps) AND the
// xpub's child number equals the terminal path component (raw, hardened bit
// included). Public-only — never touches a private key.
func compactFromXpub(xpub string, comps []uint32) ([]byte, error) {
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, errEncodeXpub
	}
	if key.IsPrivate() {
		// Defensive: Encode must never serialize private material.
		return nil, errEncodeXpub
	}
	// Depth/child invariant.
	if int(key.Depth()) != len(comps) {
		return nil, errEncodeInvariant
	}
	if len(comps) > 0 {
		if key.ChildIndex() != comps[len(comps)-1] {
			return nil, errEncodeInvariant
		}
	} else if key.ChildIndex() != 0 {
		return nil, errEncodeInvariant
	}

	version := key.Version()
	if len(version) != 4 {
		return nil, errEncodeXpub
	}
	// Only mainnet/testnet public versions are representable on the wire.
	switch hex.EncodeToString(version) {
	case "0488b21e", "043587cf":
	default:
		return nil, errEncodeXpub
	}
	chainCode := key.ChainCode() // returns a copy
	if len(chainCode) != 32 {
		return nil, errEncodeXpub
	}
	pub, err := key.ECPubKey()
	if err != nil {
		return nil, errEncodeXpub
	}
	pubBytes := pub.SerializeCompressed()
	if len(pubBytes) != 33 {
		return nil, errEncodeXpub
	}

	var parentFP [4]byte
	parentFP[0] = byte(key.ParentFingerprint() >> 24)
	parentFP[1] = byte(key.ParentFingerprint() >> 16)
	parentFP[2] = byte(key.ParentFingerprint() >> 8)
	parentFP[3] = byte(key.ParentFingerprint())

	out := make([]byte, 0, xpubCompactBytes)
	out = append(out, version...)
	out = append(out, parentFP[:]...)
	out = append(out, chainCode...)
	out = append(out, pubBytes...)
	if len(out) != xpubCompactBytes {
		return nil, errEncodeCompact
	}
	return out, nil
}

// encodePath is the inverse of decodePath: a 1-byte standard-table indicator
// for one of the 14 standard paths, else 0xFE + count + LEB128-per-component.
func encodePath(comps []uint32) ([]byte, error) {
	if ind, ok := standardPathIndicator(comps); ok {
		return []byte{ind}, nil
	}
	if len(comps) > maxPathComponents {
		return nil, errPathTooDeep
	}
	out := make([]byte, 0, 2+len(comps)*5)
	out = append(out, explicitPathIndicator)
	out = append(out, byte(len(comps)))
	for _, c := range comps {
		out = appendLEB128(out, c)
	}
	return out, nil
}

// standardPathIndicator returns the standard-table indicator byte for comps if
// it matches one of the 14 standard paths, mirroring the decoder's table.
func standardPathIndicator(comps []uint32) (byte, bool) {
	for ind, p := range standardPaths {
		if len(p) != len(comps) {
			continue
		}
		match := true
		for i := range p {
			if p[i] != comps[i] {
				match = false
				break
			}
		}
		if match {
			return ind, true
		}
	}
	return 0, false
}

// appendLEB128 appends the unsigned LEB128 encoding of v (the inverse of
// readLEB128).
func appendLEB128(dst []byte, v uint32) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if v == 0 {
			return dst
		}
	}
}

// encodeChunks splits the bytecode (plus its 4-byte integrity hash) into >= 2
// chunk strings, each with an 8-symbol chunked header and a per-chunk BCH
// checksum. The chunk_set_id is derived deterministically from the bytecode.
func encodeChunks(bytecode []byte) []string {
	csid := top20(bytecode)
	sum := sha256.Sum256(bytecode)
	stream := make([]byte, 0, len(bytecode)+crossChunkHashBytes)
	stream = append(stream, bytecode...)
	stream = append(stream, sum[:crossChunkHashBytes]...)

	// Split the stream into <= 53-byte fragments.
	var frags [][]byte
	for off := 0; off < len(stream); off += chunkedFragmentBytes {
		end := off + chunkedFragmentBytes
		if end > len(stream) {
			end = len(stream)
		}
		frags = append(frags, stream[off:end])
	}
	total := len(frags)

	out := make([]string, total)
	for i, frag := range frags {
		hdr := []byte{
			mkVersionV01,
			typeChunked,
			byte(csid >> 15 & 0x1f),
			byte(csid >> 10 & 0x1f),
			byte(csid >> 5 & 0x1f),
			byte(csid & 0x1f),
			byte(total - 1), // value-1 on the wire
			byte(i),         // chunk index, verbatim 0-based
		}
		dataSyms := append(hdr, bytesToFiveBit(frag)...)
		out[i] = assembleMK1(dataSyms)
	}
	return out
}

// bytesToFiveBit repacks bytes into 5-bit symbols, MSB-first, zero-padding the
// trailing partial group (the inverse of fiveBitToBytes).
func bytesToFiveBit(b []byte) []byte {
	out := make([]byte, 0, (len(b)*8+4)/5)
	var acc uint32
	var bits uint
	for _, v := range b {
		acc = acc<<8 | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out = append(out, byte(acc>>bits&0x1f))
		}
	}
	if bits > 0 {
		// Zero-pad the final partial group on the low bits.
		out = append(out, byte(acc<<(5-bits)&0x1f))
	}
	return out
}

// assembleMK1 renders an mk1 string: "mk1" + each data symbol + the BCH
// checksum symbols. It selects the regular (13-symbol) or long (15-symbol) code
// so the resulting data-part length lands in ValidMK's bracket (regular
// [14,93], long [96,108]); the per-chunk codex32.ValidMK gate proves the
// selection is correct.
func assembleMK1(dataSyms []byte) string {
	// Decide regular vs long by the resulting data-part length (data syms +
	// checksum syms after "mk1").
	long := len(dataSyms)+mdmkShortSyms > mkRegularMaxLen
	ck := codex32.MKChecksumSymbols(dataSyms, long)

	buf := make([]byte, 0, 3+len(dataSyms)+len(ck))
	buf = append(buf, "mk1"...)
	for _, s := range dataSyms {
		buf = append(buf, symRune(s))
	}
	for _, s := range ck {
		buf = append(buf, symRune(s))
	}
	return string(buf)
}

// top20 derives the deterministic 20-bit chunk_set_id from the bytecode:
// the top 20 bits of SHA-256(bytecode). NO CSPRNG.
func top20(bytecode []byte) uint32 {
	h := sha256.Sum256(bytecode)
	return uint32(h[0])<<12 | uint32(h[1])<<4 | uint32(h[2])>>4
}

const (
	mkRegularMaxLen = 93 // ValidMK regular bracket upper bound (data-part chars)
	mdmkShortSyms   = 13 // regular checksum symbol count
)

// symRune renders a 5-bit symbol (0..31) to its lowercase bech32 character.
const bech32Alphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func symRune(s byte) byte {
	return bech32Alphabet[s&0x1f]
}
