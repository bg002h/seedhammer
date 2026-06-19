// Package mk decodes mk1 (account-xpub) constellation strings into the
// account metadata they carry: network, derivation path, origin fingerprint,
// policy-id stubs, and the BIP-32 account xpub. mk1 is PUBLIC; this package
// performs no secret handling. Wire format: mnemonic-key/crates/mk-codec
// (family_token "mk-codec 0.2").
package mk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/codex32"
)

// Decode/reassembly error sentinels.
var (
	errEmptyInput             = errors.New("mk: empty input")
	errUnexpectedEnd          = errors.New("mk: unexpected end of data")
	errTrailingBytes          = errors.New("mk: trailing bytes after xpub")
	errReservedBits           = errors.New("mk: reserved header bits set")
	errStubCount              = errors.New("mk: stub_count must be >= 1")
	errMalformedPadding       = errors.New("mk: malformed payload padding")
	errChunkedHeaderMalformed = errors.New("mk: chunked header malformed")
	errMixedHeaderTypes       = errors.New("mk: mixed header types in chunk set")
	errChunkSetIDMismatch     = errors.New("mk: chunk_set_id mismatch")
	errDuplicateChunk         = errors.New("mk: duplicate chunk index")
	errCrossChunkHash         = errors.New("mk: cross-chunk integrity hash mismatch")
	errPathTooDeep            = errors.New("mk: path too deep")
	errPathComponent          = errors.New("mk: invalid path component")
)

const (
	mkVersionV01      = 0x00
	typeSingle        = 0x00
	typeChunked       = 0x01
	singleHeaderSyms  = 2
	chunkedHeaderSyms = 8
	maxChunks         = 32
)

// Header is a parsed string-layer header for one mk1 string.
type Header struct {
	Chunked     bool
	ChunkSetID  uint32
	TotalChunks int // 1 for single-string; >=2 in practice.
	ChunkIndex  int // 0-based.
}

// ParseHeader extracts the string-layer header from one BCH-valid mk1 string.
func ParseHeader(s string) (Header, error) {
	syms, err := codex32.MKDataSymbols(s)
	if err != nil {
		return Header{}, err
	}
	h, _, err := parseHeaderSyms(syms)
	return h, err
}

// parseHeaderSyms reads the string-layer header off the front of syms and
// returns it plus the number of symbols consumed (2 single / 8 chunked).
func parseHeaderSyms(syms []byte) (Header, int, error) {
	if len(syms) < singleHeaderSyms {
		return Header{}, 0, errUnexpectedEnd
	}
	if version := syms[0] & 0x1f; version != mkVersionV01 {
		return Header{}, 0, fmt.Errorf("mk: unsupported version: 0x%02x", version)
	}
	switch syms[1] & 0x1f {
	case typeSingle:
		return Header{Chunked: false, TotalChunks: 1, ChunkIndex: 0}, singleHeaderSyms, nil
	case typeChunked:
		if len(syms) < chunkedHeaderSyms {
			return Header{}, 0, errUnexpectedEnd
		}
		csid := uint32(syms[2]&0x1f)<<15 | uint32(syms[3]&0x1f)<<10 |
			uint32(syms[4]&0x1f)<<5 | uint32(syms[5]&0x1f)
		total := int(syms[6]&0x1f) + 1 // value-1 on the wire.
		index := int(syms[7] & 0x1f)   // verbatim, 0-based — NOT value-1 (R0-C1).
		if total > maxChunks || index >= total {
			return Header{}, 0, errChunkedHeaderMalformed
		}
		return Header{Chunked: true, ChunkSetID: csid, TotalChunks: total, ChunkIndex: index}, chunkedHeaderSyms, nil
	default:
		return Header{}, 0, fmt.Errorf("mk: unsupported card type: 0x%02x", syms[1]&0x1f)
	}
}

// fiveBitToBytes repacks 5-bit symbols into bytes, rejecting any symbol >= 32,
// a leftover group of >= 5 bits, or non-zero trailing padding bits (mk-codec
// string_layer/bch.rs:78-100). Unlike codex32's parts.data() it never panics
// and never silently drops a partial byte.
func fiveBitToBytes(syms []byte) ([]byte, error) {
	var acc uint32
	var bits uint
	out := make([]byte, 0, len(syms)*5/8)
	for _, v := range syms {
		if v >= 32 {
			return nil, errMalformedPadding
		}
		acc = acc<<5 | uint32(v)
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>bits&0xff))
		}
	}
	if bits >= 5 {
		return nil, errMalformedPadding
	}
	if acc&(1<<bits-1) != 0 {
		return nil, errMalformedPadding
	}
	return out, nil
}

const (
	fingerprintFlagMask   = 0x04
	reservedMask          = 0x0b // bits 0,1,3
	explicitPathIndicator = 0xFE
	maxPathComponents     = 10
	xpubCompactBytes      = 73
	crossChunkHashBytes   = 4
	hardenedBit           = 0x80000000
)

// Card is the decoded account metadata carried by an mk1 string set.
type Card struct {
	Network     string    // "mainnet" | "testnet"
	Path        string    // e.g. "m/48'/0'/0'/2'" ("m" for depth-0)
	Fingerprint string    // 8 lowercase hex, or "" if absent
	Stubs       [][4]byte // policy-id stubs (len >= 1)
	Xpub        string    // base58 "xpub…"/"tpub…"
}

type chunkFrag struct {
	header   Header
	fragment []byte
}

// Decode reassembles a complete set of BCH-valid mk1 chunk strings (any order)
// and decodes to a Card.
func Decode(in []string) (Card, error) {
	if len(in) == 0 {
		return Card{}, errEmptyInput
	}
	frags := make([]chunkFrag, 0, len(in))
	for _, s := range in {
		syms, err := codex32.MKDataSymbols(s)
		if err != nil {
			return Card{}, err
		}
		h, n, err := parseHeaderSyms(syms)
		if err != nil {
			return Card{}, err
		}
		frag, err := fiveBitToBytes(syms[n:])
		if err != nil {
			return Card{}, err
		}
		frags = append(frags, chunkFrag{header: h, fragment: frag})
	}
	bytecode, err := reassemble(frags)
	if err != nil {
		return Card{}, err
	}
	return decodeBytecode(bytecode)
}

func reassemble(frags []chunkFrag) ([]byte, error) {
	first := frags[0].header
	if !first.Chunked {
		if len(frags) != 1 {
			return nil, errChunkedHeaderMalformed
		}
		return frags[0].fragment, nil // single-string fragment IS the bytecode (no hash).
	}
	total := first.TotalChunks
	if len(frags) != total {
		return nil, fmt.Errorf("mk: received %d chunks, header declares %d", len(frags), total)
	}
	slots := make([][]byte, total)
	for _, f := range frags {
		if !f.header.Chunked {
			return nil, errMixedHeaderTypes
		}
		if f.header.ChunkSetID != first.ChunkSetID {
			return nil, errChunkSetIDMismatch
		}
		if f.header.TotalChunks != total {
			return nil, errChunkedHeaderMalformed
		}
		idx := f.header.ChunkIndex
		if idx >= total {
			return nil, errChunkedHeaderMalformed
		}
		if slots[idx] != nil {
			return nil, errDuplicateChunk
		}
		slots[idx] = f.fragment
	}
	var stream []byte
	for i, frag := range slots {
		if frag == nil {
			return nil, fmt.Errorf("mk: missing chunk %d", i)
		}
		stream = append(stream, frag...)
	}
	if len(stream) < crossChunkHashBytes {
		return nil, errCrossChunkHash
	}
	split := len(stream) - crossChunkHashBytes
	bytecode := stream[:split]
	sum := sha256.Sum256(bytecode)
	if !bytes.Equal(sum[:crossChunkHashBytes], stream[split:]) {
		return nil, errCrossChunkHash
	}
	return bytecode, nil
}

func decodeBytecode(b []byte) (Card, error) {
	cur := 0
	read := func(n int) ([]byte, error) {
		if cur+n > len(b) {
			return nil, errUnexpectedEnd
		}
		out := b[cur : cur+n]
		cur += n
		return out, nil
	}
	hdr, err := read(1)
	if err != nil {
		return Card{}, err
	}
	if hdr[0]>>4 != 0 {
		return Card{}, fmt.Errorf("mk: unsupported bytecode version: %d", hdr[0]>>4)
	}
	if hdr[0]&reservedMask != 0 {
		return Card{}, errReservedBits
	}
	fpPresent := hdr[0]&fingerprintFlagMask != 0
	scb, err := read(1)
	if err != nil {
		return Card{}, err
	}
	stubCount := int(scb[0])
	if stubCount == 0 {
		return Card{}, errStubCount
	}
	stubs := make([][4]byte, stubCount)
	for i := range stubs {
		sb, err := read(4)
		if err != nil {
			return Card{}, err
		}
		copy(stubs[i][:], sb)
	}
	fp := ""
	if fpPresent {
		fpb, err := read(4)
		if err != nil {
			return Card{}, err
		}
		fp = hex.EncodeToString(fpb)
	}
	comps, err := decodePath(read)
	if err != nil {
		return Card{}, err
	}
	compact, err := read(xpubCompactBytes)
	if err != nil {
		return Card{}, err
	}
	if cur != len(b) {
		return Card{}, errTrailingBytes
	}
	xpub, network, err := reconstructXpub(compact, comps)
	if err != nil {
		return Card{}, err
	}
	return Card{Network: network, Path: pathString(comps), Fingerprint: fp, Stubs: stubs, Xpub: xpub}, nil
}

func h(i uint32) uint32 { return i | hardenedBit }

var standardPaths = map[byte][]uint32{
	0x01: {h(44), h(0), h(0)},
	0x02: {h(49), h(0), h(0)},
	0x03: {h(84), h(0), h(0)},
	0x04: {h(86), h(0), h(0)},
	0x05: {h(48), h(0), h(0), h(2)},
	0x06: {h(48), h(0), h(0), h(1)},
	0x07: {h(87), h(0), h(0)},
	0x11: {h(44), h(1), h(0)},
	0x12: {h(49), h(1), h(0)},
	0x13: {h(84), h(1), h(0)},
	0x14: {h(86), h(1), h(0)},
	0x15: {h(48), h(1), h(0), h(2)},
	0x16: {h(48), h(1), h(0), h(1)},
	0x17: {h(87), h(1), h(0)},
}

func decodePath(read func(int) ([]byte, error)) ([]uint32, error) {
	ib, err := read(1)
	if err != nil {
		return nil, err
	}
	ind := ib[0]
	if ind == explicitPathIndicator {
		cb, err := read(1)
		if err != nil {
			return nil, err
		}
		count := int(cb[0])
		if count > maxPathComponents {
			return nil, errPathTooDeep
		}
		comps := make([]uint32, 0, count)
		for i := 0; i < count; i++ {
			v, err := readLEB128(read)
			if err != nil {
				return nil, err
			}
			comps = append(comps, v)
		}
		return comps, nil
	}
	if p, ok := standardPaths[ind]; ok {
		out := make([]uint32, len(p))
		copy(out, p)
		return out, nil
	}
	return nil, fmt.Errorf("mk: invalid path indicator byte: 0x%02x", ind)
}

func readLEB128(read func(int) ([]byte, error)) (uint32, error) {
	var result uint64
	var shift uint
	for {
		bb, err := read(1)
		if err != nil {
			return 0, err
		}
		result |= uint64(bb[0]&0x7f) << shift
		if bb[0]&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, errPathComponent
		}
	}
	if result > 0xffffffff {
		return 0, errPathComponent
	}
	return uint32(result), nil
}

func pathString(comps []uint32) string {
	var b strings.Builder
	b.WriteString("m")
	for _, c := range comps {
		b.WriteByte('/')
		if c&hardenedBit != 0 {
			fmt.Fprintf(&b, "%d'", c&^uint32(hardenedBit))
		} else {
			fmt.Fprintf(&b, "%d", c)
		}
	}
	return b.String()
}

func reconstructXpub(compact []byte, comps []uint32) (xpub, network string, err error) {
	if len(compact) != xpubCompactBytes {
		return "", "", errUnexpectedEnd
	}
	version := compact[0:4]
	parentFP := compact[4:8]
	chainCode := compact[8:40]
	pubKey := compact[40:73]
	switch hex.EncodeToString(version) {
	case "0488b21e":
		network = "mainnet"
	case "043587cf":
		network = "testnet"
	default:
		return "", "", fmt.Errorf("mk: invalid xpub version: %x", version)
	}
	if _, err := btcec.ParsePubKey(pubKey); err != nil {
		return "", "", fmt.Errorf("mk: invalid xpub public key: %w", err)
	}
	depth := uint8(len(comps))
	childNum := uint32(0)
	if len(comps) > 0 {
		childNum = comps[len(comps)-1] // raw, hardened bit included (R0-M1).
	}
	key := hdkeychain.NewExtendedKey(version, pubKey, chainCode, parentFP, depth, childNum, false)
	return key.String(), network, nil
}
