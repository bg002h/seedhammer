package sysw

import (
	"encoding/hex"
	"errors"
	"strings"

	"seedhammer.com/mt"
)

// Prefixes are RESERVED. A record beginning with one whose body is not valid
// lowercase hex is ClassUnknown and refused -- never quietly treated as free
// text, which would let a malformed record become an engraved plate.
const (
	TextPrefix = "text:"
	PassPrefix = "pass:"
	// TxPrefix carries a raw signed Bitcoin transaction as lowercase hex, for
	// QR engraving. Reserved like the other two; classification additionally
	// requires the bytes to PARSE as one transaction (see Classify), so the
	// prefix cannot smuggle arbitrary bytes into a non-secret class.
	TxPrefix = "tx:"
)

type Class int

const (
	ClassUnknown Class = iota
	ClassMnemonic
	ClassCodex32Secret
	ClassPassphrase
	ClassFreeText
	ClassDescriptor
	ClassMDMK
	ClassAddress
	// ClassMt is one chunk of an mt1 signed-transaction set. NOT secret: the
	// record exists to be engraved in cleartext, so flash holds nothing the
	// steel will not. An UNCONFIRMED one still reads as secret through
	// MTUnconfirmed, exactly as ClassMDMK does through MDMKUnconfirmed.
	ClassMt
	// ClassTx is a tx:-prefixed raw signed transaction, for the QR engraving
	// path. Classification already proved it parses; no confirmation walk
	// exists for it. Same secrecy reasoning as ClassMt.
	ClassTx
	// ClassKey, ClassHash, ClassNow are the wallet-policy COMPOSER's records
	// (SPEC_wallet_policy_composer.md §6a; SPEC_systemwide_payloads.md 5.3):
	// a cosigner's [fingerprint/path]xpub, a sha256 digest for a hashlock, and
	// the pack time with an optional height. None is secret or bearer. Body
	// rules and prefixes live in composer_records.go, ported as one unit from
	// the host's composer_records.rs and pinned row-for-row by the vendored
	// record_class_vectors.json (§12 item 8). A malformed one is ClassUnknown
	// and the device leaves it inert; the §8n line is the HOST's.
	ClassKey
	ClassHash
	ClassNow
)

// IsSecret extends the shipped predicate (seal/session.go:17, which is
// ClassCodex32Secret || ClassMnemonic) with ClassPassphrase.
//
// ClassFreeText is deliberately NOT secret even though an operator may put
// anything in it: a class states what the format guarantees, not what a human
// might do. A class claiming secrecy it cannot enforce is the over-claim F-123
// was filed against.
func (c Class) IsSecret() bool {
	return c == ClassMnemonic || c == ClassCodex32Secret || c == ClassPassphrase
}

var ErrBadHex = errors.New("sysw: reserved prefix with a body that is not lowercase hex")

// DecodeBody decodes a text: or pass: record's body.
//
// WHY THE BODY IS ENCODED AT ALL: EPD §6.4 requires every record to be the
// canonical, unbroken string -- no interior spaces -- and uses LF as the record
// separator. "Hello, World!" has a space and Engrave Text's keyboard has a
// newline key, so both new classes break both clauses. Encoding keeps §6.4
// intact rather than weakening it for two classes, and lowercase hex is the only
// common encoding that survives §6.6's lowercase canonicalisation unchanged.
func DecodeBody(record string) ([]byte, error) {
	var body string
	switch {
	case strings.HasPrefix(record, TextPrefix):
		body = record[len(TextPrefix):]
	case strings.HasPrefix(record, PassPrefix):
		body = record[len(PassPrefix):]
	case strings.HasPrefix(record, TxPrefix):
		body = record[len(TxPrefix):]
	default:
		return nil, errors.New("sysw: not an encoded record")
	}
	// Strictly lowercase. Uppercase is REJECTED rather than accepted: §6.6
	// hashes the record as it appears on the wire, so two spellings of one body
	// would be two different digests.
	if strings.ToLower(body) != body {
		return nil, ErrBadHex
	}
	b, err := hex.DecodeString(body)
	if err != nil {
		return nil, ErrBadHex
	}
	return b, nil
}

// Classify places a record.
//
// ADDRESS IS DELIBERATELY ABSENT, matching the Rust primary: classifying one
// needs an address decoder, which neither side has. DESCRIPTOR was absent for
// the same shape of reason until S2 -- see classifyConstellation's last arm and
// sysw/descriptor.go, which port SPEC_descriptor_input.md §5.2's predicate
// rather than reusing the scan door's wider one. An unclassifiable record is
// ClassUnknown and the caller fails closed.
func Classify(record string) Class {
	if strings.HasPrefix(record, PassPrefix) {
		if _, err := DecodeBody(record); err == nil {
			return ClassPassphrase
		}
		return ClassUnknown
	}
	if strings.HasPrefix(record, TextPrefix) {
		if _, err := DecodeBody(record); err == nil {
			return ClassFreeText
		}
		return ClassUnknown
	}
	if strings.HasPrefix(record, TxPrefix) {
		// Hex AND a structural transaction parse -- mirroring the Rust
		// primary's classify. A body that decodes but does not parse is
		// Unknown, never quietly some other class.
		if b, err := DecodeBody(record); err == nil {
			if _, err := mt.ParseTx(b); err == nil {
				return ClassTx
			}
		}
		return ClassUnknown
	}
	if IsComposerRecord(record) {
		return classifyComposer(record)
	}
	return classifyConstellation(record)
}
