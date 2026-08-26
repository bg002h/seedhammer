// Package mt decodes mt1 (signed-transaction) constellation strings: the
// 11-symbol chunk header, chunk-set reassembly, and the structural
// transaction parse + txid binding that CONFIRMS a set.
//
// PORT, NOT PRIMARY. The wire format is mnemonic-transaction/crates/mt-codec
// (spec: mnemonic-engrave/design/SPEC_mt_v0_1.md); the confirmation semantics
// converge on mnemonic-engrave/crates/me-cli/src/sysw/mt.rs and tx.rs, which
// may never be led by this port. Provenance pin: mt-codec 0.1.0 /
// me-cli sysw::mt as of the exp/tx-brief-driven branch.
//
// mt1 is PUBLIC in the sysw sense — the record exists to be engraved in
// cleartext — but the DECODED set is a BEARER instrument: anyone holding the
// complete transaction can broadcast it. This package handles no secrets and
// derives no keys.
//
// WHY CONFIRMATION NEEDS A TRANSACTION PARSER. Unlike md1/mk1, whose real
// decoders (md.Reassemble, mk.Decode) are semantic arbiters, ANY complete set
// of BCH-valid mt1 strings "reassembles" — the payload is opaque bytes. The
// semantic check mt has is the one the format builds in: the bytes must parse
// as one serialized Bitcoin transaction, and the set's 20-bit chunk_set_id
// must equal the top 20 bits of the display txid (SPEC_mt_v0_1 §10.13 c).
// Without it, 32 bytes of seed entropy wrap into a "confirmed" public record
// — the §5.3.2 smuggling channel, reopened for a new HRP.
package mt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"seedhammer.com/codex32"
)

// Header field widths, in bits — mt-codec consts.rs. Every field is a whole
// number of 5-bit symbols; there is no chunked flag because mt1 is ALWAYS
// chunked (a 1-chunk set is count=1, not a different form).
const (
	mtVersion  = 1
	headerSyms = 11
	wSetID     = 20
	wCount     = 15
	wIndex     = 15
	maxChunks  = 1 << wCount
)

// Decode/reassembly error sentinels.
var (
	errEmptyInput      = errors.New("mt: empty input")
	errNotMT1          = errors.New("mt: not a BCH-valid mt1 string")
	errMixedSets       = errors.New("mt: chunk_set_id mismatch within the set")
	errCountMismatch   = errors.New("mt: chunk count disagrees between chunks")
	errAmbiguousChunk  = errors.New("mt: two different payloads for one chunk index")
	errUnevenChunks    = errors.New("mt: non-final chunks differ in length")
	errTxidBinding     = errors.New("mt: chunk_set_id does not match the transaction's txid")
	errUnsignedInputs  = errors.New("mt: an input carries neither a scriptSig nor a witness")
	errNotATransaction = errors.New("mt: reassembled bytes are not one serialized transaction")
)

// Header is the parsed 11-symbol header of one mt1 string.
type Header struct {
	Version     int
	ChunkSetID  uint32 // top 20 bits of the display txid
	TotalChunks int    // stored on the wire as count-1
	ChunkIndex  int    // 0-based
}

// ParseHeader extracts the header from one BCH-valid mt1 string.
func ParseHeader(s string) (Header, error) {
	syms, err := codex32.MTDataSymbols(s)
	if err != nil {
		return Header{}, errNotMT1
	}
	return parseHeaderSyms(syms)
}

func parseHeaderSyms(syms []byte) (Header, error) {
	if len(syms) < headerSyms {
		return Header{}, errNotMT1 // unreachable: ValidMT enforces the minimum
	}
	var bits uint64
	for _, s := range syms[:headerSyms] {
		bits = bits<<5 | uint64(s&0x1f)
	}
	h := Header{
		Version:     int(bits >> (wSetID + wCount + wIndex)),
		ChunkSetID:  uint32(bits >> (wCount + wIndex) & (1<<wSetID - 1)),
		TotalChunks: int(bits>>wIndex&(1<<wCount-1)) + 1, // count-1 on the wire
		ChunkIndex:  int(bits & (1<<wIndex - 1)),
	}
	if h.Version != mtVersion {
		return Header{}, fmt.Errorf("mt: unsupported version %d", h.Version)
	}
	if h.ChunkIndex >= h.TotalChunks || h.TotalChunks > maxChunks {
		return Header{}, fmt.Errorf("mt: header index %d out of range for count %d",
			h.ChunkIndex, h.TotalChunks)
	}
	return h, nil
}

// payloadBytes repacks the post-header symbols into bytes, TRUNCATING to
// floor(5n/8) — deliberately NOT mk's strict-padding fiveBitToBytes, because
// the primary (mt-codec pipeline.rs decode_chunk, plan=None) truncates rather
// than rejecting non-zero padding, and this port is bound to the primary's
// answers. The encoder always pads with zeros; the difference is only
// reachable by a forged string, whose set then still has to pass the txid
// binding.
func payloadBytes(syms []byte) []byte {
	var acc uint32
	var nbits uint
	out := make([]byte, 0, len(syms)*5/8)
	for _, v := range syms {
		acc = acc<<5 | uint32(v&0x1f)
		nbits += 5
		if nbits >= 8 {
			nbits -= 8
			out = append(out, byte(acc>>nbits))
		}
	}
	return out
}

// Tx is a structurally parsed transaction: what the review screen shows and
// what the QR path engraves.
type Tx struct {
	Raw         []byte
	TxidDisplay string // byte-reversed lowercase hex, the form a user reads
	Inputs      int
	Outputs     int
	SegWit      bool
	// EveryInputSigned: every input carries a non-empty scriptSig or at least
	// one witness item. False means the transaction cannot be broadcast.
	//
	// (G-P3.2) This is the ONLY thing separating a signature-stripped
	// transaction from the honest one it came from: stripping the witness is
	// exactly the operation the txid is defined to ignore, so both have the
	// same txid, the same ChunkSetID, and pass every other check here. The
	// parser previously called skipBytes for the scriptSig and DISCARDED the
	// length, so the device had no signal at all.
	//
	// Rust is primary: crates/me-cli/src/sysw/tx.rs's `every_input_signed`.
	EveryInputSigned bool
	// UnsignedInputs are the INDICES of the inputs carrying neither. Empty
	// exactly when EveryInputSigned is true.
	//
	// Carried rather than recomputed by the caller because the review screen
	// has to say WHICH input: "an input is unsigned" tells an operator holding
	// a 3-input transaction nothing they can act on.
	//
	// CONVERGENCE PORT, not a Go-led change: `unsigned_inputs` landed in
	// crates/me-cli/src/sysw/tx.rs first, with its vectors (G-P3.3).
	UnsignedInputs []int
}

// ChunkSetID returns the top 20 bits of the display txid — the value every
// chunk of this transaction's mt1 set must carry.
func (t Tx) ChunkSetID() uint32 {
	// The first five hex characters, as mt-codec's
	// content_id_from_txid_display reads them.
	var v uint32
	for i := 0; i < 5; i++ {
		c := t.TxidDisplay[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		default:
			d = uint32(c-'a') + 10
		}
		v = v<<4 | d
	}
	return v
}

// Decode reassembles a COMPLETE set of BCH-valid mt1 strings (any order,
// byte-identical duplicates tolerated) and confirms it: the bytes must parse
// as one serialized Bitcoin transaction whose display txid's top 20 bits
// equal the set's chunk_set_id. Anything less is an error — this is the
// decode-confirmation `[mt-decode]` names, and the engrave path refuses an
// unconfirmed set.
//
// STRICTER THAN THE PRIMARY IN TWO NAMED PLACES, both reachable only by
// forged strings and both failing toward "unconfirmed": non-final chunks must
// share one length (the primary trims order-dependently), and every chunk
// must declare the same TotalChunks (the primary reads count from the first
// readable chunk only).
func Decode(in []string) (Tx, error) {
	if len(in) == 0 {
		return Tx{}, errEmptyInput
	}
	type chunk struct {
		h       Header
		payload []byte
	}
	var first *Header
	slots := make(map[int][]byte)
	for _, s := range in {
		syms, err := codex32.MTDataSymbols(s)
		if err != nil {
			return Tx{}, errNotMT1
		}
		h, err := parseHeaderSyms(syms)
		if err != nil {
			return Tx{}, err
		}
		if first == nil {
			hh := h
			first = &hh
		} else {
			if h.ChunkSetID != first.ChunkSetID {
				return Tx{}, errMixedSets
			}
			if h.TotalChunks != first.TotalChunks {
				return Tx{}, errCountMismatch
			}
		}
		p := payloadBytes(syms[headerSyms:])
		if prev, ok := slots[h.ChunkIndex]; ok {
			if !bytes.Equal(prev, p) {
				return Tx{}, errAmbiguousChunk
			}
			continue // an identical duplicate is a well-kept drawer, not an error
		}
		slots[h.ChunkIndex] = p
	}
	count := first.TotalChunks
	var raw []byte
	for i := 0; i < count; i++ {
		p, ok := slots[i]
		if !ok {
			return Tx{}, fmt.Errorf("mt: missing chunk %d of %d", i+1, count)
		}
		if i < count-1 {
			if l, ok := slots[0]; ok && len(p) != len(l) {
				return Tx{}, errUnevenChunks
			}
		}
		raw = append(raw, p...)
	}
	tx, err := ParseTx(raw)
	if err != nil {
		return Tx{}, err
	}
	if tx.ChunkSetID() != first.ChunkSetID {
		return Tx{}, errTxidBinding
	}
	// (G-P3.2) A set carrying an UNSIGNED transaction does not confirm. The
	// binding above cannot see it: stripping the witnesses leaves the txid
	// unchanged, so the set id still matches. Under the 2026-08-25 rulings the
	// consequence is NOT a refusal -- the caller offers the set with the
	// operator's legend REPLACED, exactly as it does for any other set that
	// fails to confirm.
	if !tx.EveryInputSigned {
		return Tx{}, errUnsignedInputs
	}
	return tx, nil
}

// ParseTx structurally parses one serialized Bitcoin transaction and computes
// its txid over the witness-stripped form (BIP-141). Structural ONLY: no
// script validation and no signature VALIDITY check -- that needs prevout
// scripts and amounts an offline device cannot have. It does report whether
// every input carries a signature at all (EveryInputSigned), which is
// structural and is the only signal separating a stripped transaction from the
// honest one it came from. The whole buffer must be
// consumed. Mirrors me-cli's sysw::tx::parse.
func ParseTx(raw []byte) (Tx, error) {
	p := &parser{buf: raw}
	version, err := p.take(4)
	if err != nil {
		return Tx{}, errNotATransaction
	}
	segwit := false
	mark := p.pos
	b, err := p.u8()
	if err != nil {
		return Tx{}, errNotATransaction
	}
	if b == 0x00 {
		flag, err := p.u8()
		if err != nil || flag != 0x01 {
			return Tx{}, errNotATransaction
		}
		segwit = true
	} else {
		p.pos = mark
	}
	coreStart := p.pos
	nIn, err := p.count()
	if err != nil || nIn == 0 {
		return Tx{}, errNotATransaction
	}
	// Per input, not per transaction: a mixed transaction keeps its legacy
	// scriptSigs when the witnesses are stripped, so a whole-transaction test
	// would pass while the segwit inputs were left unsigned.
	signed := make([]bool, nIn)
	for i := 0; i < nIn; i++ {
		if _, err := p.take(36); err != nil { // outpoint
			return Tx{}, errNotATransaction
		}
		n, err := p.bytesLen() // scriptSig
		if err != nil {
			return Tx{}, errNotATransaction
		}
		signed[i] = n > 0
		if _, err := p.take(4); err != nil { // sequence
			return Tx{}, errNotATransaction
		}
	}
	nOut, err := p.count()
	if err != nil || nOut == 0 {
		return Tx{}, errNotATransaction
	}
	for i := 0; i < nOut; i++ {
		if _, err := p.take(8); err != nil { // value
			return Tx{}, errNotATransaction
		}
		if err := p.skipBytes(); err != nil { // scriptPubKey
			return Tx{}, errNotATransaction
		}
	}
	coreEnd := p.pos
	if segwit {
		for i := 0; i < nIn; i++ {
			items, err := p.count()
			if err != nil {
				return Tx{}, errNotATransaction
			}
			if items > 0 {
				signed[i] = true
			}
			for j := 0; j < items; j++ {
				if err := p.skipBytes(); err != nil {
					return Tx{}, errNotATransaction
				}
			}
		}
	}
	locktime, err := p.take(4)
	if err != nil {
		return Tx{}, errNotATransaction
	}
	if p.pos != len(raw) {
		return Tx{}, errNotATransaction
	}

	// PER INPUT, and the list is the answer: EveryInputSigned is defined from
	// it below, so the verdict and the indices the screens print cannot drift.
	// Mirrors sysw::tx::parse's `unsigned`.
	var unsigned []int
	for i, ok := range signed {
		if !ok {
			unsigned = append(unsigned, i)
		}
	}

	h := sha256.New()
	h.Write(version)
	h.Write(raw[coreStart:coreEnd])
	h.Write(locktime)
	d1 := h.Sum(nil)
	d2 := sha256.Sum256(d1)
	// Display form: byte-reversed.
	rev := make([]byte, 32)
	for i, b := range d2 {
		rev[31-i] = b
	}
	return Tx{
		Raw:         raw,
		TxidDisplay: hex.EncodeToString(rev),
		Inputs:      nIn,
		Outputs:     nOut,
		SegWit:      segwit,
		EveryInputSigned: len(unsigned) == 0,
		UnsignedInputs:   unsigned,
	}, nil
}

type parser struct {
	buf []byte
	pos int
}

func (p *parser) take(n int) ([]byte, error) {
	if n > len(p.buf)-p.pos {
		return nil, errNotATransaction
	}
	s := p.buf[p.pos : p.pos+n]
	p.pos += n
	return s, nil
}

func (p *parser) u8() (byte, error) {
	b, err := p.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// varint reads a Bitcoin compactSize, CANONICAL form required — Bitcoin
// Core's ReadCompactSize rejects non-canonical encodings, so accepting one
// here would admit bytes no node would.
func (p *parser) varint() (uint64, error) {
	first, err := p.u8()
	if err != nil {
		return 0, err
	}
	switch first {
	case 0xFD:
		b, err := p.take(2)
		if err != nil {
			return 0, err
		}
		v := uint64(b[0]) | uint64(b[1])<<8
		if v < 0xFD {
			return 0, errNotATransaction
		}
		return v, nil
	case 0xFE:
		b, err := p.take(4)
		if err != nil {
			return 0, err
		}
		v := uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
		if v <= 0xFFFF {
			return 0, errNotATransaction
		}
		return v, nil
	case 0xFF:
		b, err := p.take(8)
		if err != nil {
			return 0, err
		}
		var v uint64
		for i := 7; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
		if v <= 0xFFFFFFFF {
			return 0, errNotATransaction
		}
		return v, nil
	default:
		return uint64(first), nil
	}
}

// count is a loop-driving varint, bounded by what the buffer could hold at
// >= 1 byte per element so a hostile count cannot spin the parser.
func (p *parser) count() (int, error) {
	v, err := p.varint()
	if err != nil {
		return 0, err
	}
	if v > uint64(len(p.buf)-p.pos) {
		return 0, errNotATransaction
	}
	return int(v), nil
}

// bytesLen skips a length-prefixed byte string and RETURNS its length, which
// skipBytes discards. The signature predicate needs to know whether a scriptSig
// was empty, and "no error" cannot say.
func (p *parser) bytesLen() (int, error) {
	n, err := p.varint()
	if err != nil {
		return 0, err
	}
	if n > uint64(len(p.buf)-p.pos) {
		return 0, errNotATransaction
	}
	p.pos += int(n)
	return int(n), nil
}

func (p *parser) skipBytes() error {
	n, err := p.varint()
	if err != nil {
		return err
	}
	if n > uint64(len(p.buf)-p.pos) {
		return errNotATransaction
	}
	p.pos += int(n)
	return nil
}
