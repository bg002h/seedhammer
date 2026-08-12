package sysw

import (
	"encoding/hex"
	"errors"
	"strings"
)

// Prefixes are RESERVED. A record beginning with one whose body is not valid
// lowercase hex is ClassUnknown and refused -- never quietly treated as free
// text, which would let a malformed record become an engraved plate.
const (
	TextPrefix = "text:"
	PassPrefix = "pass:"
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
// DESCRIPTOR AND ADDRESS ARE DELIBERATELY ABSENT, matching the Rust primary:
// classifying them needs a descriptor parser and an address decoder. An
// unclassifiable record is ClassUnknown and the caller fails closed.
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
	return classifyConstellation(record)
}
