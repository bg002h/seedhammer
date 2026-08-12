package sysw

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"seedhammer.com/seal"
)

var (
	ErrPassphraseMissing = errors.New("sysw: this payload is sealed and needs a passphrase")
	ErrNotUTF8           = errors.New("sysw: records are not valid UTF-8")
)

// Payload is a container's records, split the way it stores them.
type Payload struct {
	// Cleartext on the wire, whatever their class. In an UNSEALED payload this
	// is everything -- including secret CLASSES, which decision 6 permits and
	// F1 flags at load.
	Public []string
	// Encrypted. Empty in an unsealed payload.
	Secret []string
}

// Open parses and, if sealed, decrypts.
//
// The AAD is `header || public section`. Binding only the ciphertext's framing
// would let an attacker swap a public record for one encoding THEIR xpub with
// the tag still verifying, and the operator would engrave a steel backup of a
// wallet they do not control.
//
// Crypto comes from package seal unchanged. Two AES-GCM implementations in one
// firmware is two things that can disagree.
func Open(blob []byte, passphrase string) (*Payload, error) {
	h, err := ParseHeader(blob)
	if err != nil {
		return nil, err
	}
	if len(blob) < h.TotalLen() {
		return nil, fmt.Errorf("%w: %d, header declares %d", ErrTooShort, len(blob), h.TotalLen())
	}
	pubEnd := HeaderLen + int(h.PubLen)
	public, err := splitRecords(blob[HeaderLen:pubEnd])
	if err != nil {
		return nil, err
	}
	if !h.Sealed() {
		return &Payload{Public: public}, nil
	}
	if passphrase == "" {
		return nil, ErrPassphraseMissing
	}
	key := seal.DeriveKey(seal.NormalisePassphrase(passphrase), h.Salt[:], int(h.Iterations))
	pt, err := seal.Open(key, h.IV[:], blob[:pubEnd], blob[pubEnd:h.TotalLen()])
	if err != nil {
		return nil, err
	}
	secret, err := splitRecords(pt)
	if err != nil {
		return nil, err
	}
	return &Payload{Public: public, Secret: secret}, nil
}

func splitRecords(b []byte) ([]string, error) {
	if !utf8.Valid(b) {
		return nil, ErrNotUTF8
	}
	if len(b) == 0 {
		return nil, nil
	}
	return strings.Split(string(b), "\n"), nil
}
