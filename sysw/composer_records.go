package sysw

// The wallet-policy composer's three payload record classes -- key:, hash:,
// now: -- ported predicate by predicate from the host's
// crates/me-cli/src/sysw/composer_records.rs (SPEC_wallet_policy_composer.md
// §6a) and measured against the vendored record_class_vectors.json (§12 item 8:
// "classifies identically on the host and on the device").
//
// The device parses and classifies; it prints no §8n line (that is host copy)
// and leaves a malformed record inert. The hex rule is this file's own, like
// the host's, so the port stays one unit rather than inheriting record.go's
// history through a shared helper.

import (
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
)

const (
	KeyPrefix  = "key:"
	HashPrefix = "hash:"
	NowPrefix  = "now:"

	composerMaxHeight  uint64 = 499_999_999
	composerMaxSeconds uint64 = 2_147_483_647
)

var (
	ErrKeyRecord  = errors.New("sysw: key: needs [fingerprint/path]xpub with an origin")
	ErrHashRecord = errors.New("sysw: hash: must be exactly 64 lowercase hex characters")
	ErrNowRecord  = errors.New("sysw: now: must be <seconds>[,<height>] in range")
)

// KeyRecord is a parsed key: record.
type KeyRecord struct {
	Fingerprint [4]byte
	Origin      bip32.Path
	Xpub        string
	Text        string
}

// NowRecord is a parsed now: record -- a LOWER BOUND on the present that the
// device echoes and never encodes (C24).
type NowRecord struct {
	Seconds   uint32
	Height    uint32
	HasHeight bool
}

// IsComposerRecord reports whether the record carries one of the three
// prefixes, well-formed or not (a malformed one is still OURS: refused, never
// passed to the sniffers).
func IsComposerRecord(record string) bool {
	return strings.HasPrefix(record, KeyPrefix) || strings.HasPrefix(record, HashPrefix) || strings.HasPrefix(record, NowPrefix)
}

func classifyComposer(record string) Class {
	switch {
	case strings.HasPrefix(record, KeyPrefix):
		if _, err := ParseKeyRecord(record); err == nil {
			return ClassKey
		}
	case strings.HasPrefix(record, HashPrefix):
		if _, err := ParseHashRecord(record); err == nil {
			return ClassHash
		}
	case strings.HasPrefix(record, NowPrefix):
		if _, err := ParseNowRecord(record); err == nil {
			return ClassNow
		}
	}
	return ClassUnknown
}

// unhexLower is the host's unhex_lower: even length, every character in
// [0-9a-f]. Uppercase is refused, not folded (§6.6 hashes the wire spelling).
func unhexLower(s string) ([]byte, bool) {
	if len(s)%2 != 0 {
		return nil, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, false
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

// ParseHashRecord: exactly 64 lowercase hex characters.
func ParseHashRecord(record string) ([32]byte, error) {
	body, ok := strings.CutPrefix(record, HashPrefix)
	if !ok || len(body) != 64 {
		return [32]byte{}, ErrHashRecord
	}
	b, ok := unhexLower(body)
	if !ok {
		return [32]byte{}, ErrHashRecord
	}
	var h [32]byte
	copy(h[:], b)
	return h, nil
}

// digitsInRange is the host's digits_in_range: ASCII digits only (no sign, no
// blank), at most maxDigits of them, value within [lo, hi].
func digitsInRange(s string, maxDigits int, lo, hi uint64) (uint32, bool) {
	if s == "" || len(s) > maxDigits {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint64(c-'0')
	}
	if v < lo || v > hi {
		return 0, false
	}
	return uint32(v), true
}

// ParseNowRecord: hex of "<seconds>[,<height>]", seconds 1..=2^31-1 (10 digits
// at most), height 1..=499,999,999 (9 digits at most).
func ParseNowRecord(record string) (NowRecord, error) {
	body, ok := strings.CutPrefix(record, NowPrefix)
	if !ok {
		return NowRecord{}, ErrNowRecord
	}
	b, ok := unhexLower(body)
	if !ok || !utf8.Valid(b) {
		return NowRecord{}, ErrNowRecord
	}
	text := string(b)
	secText, heightText, hasHeight := strings.Cut(text, ",")
	secs, ok := digitsInRange(secText, 10, 1, composerMaxSeconds)
	if !ok {
		return NowRecord{}, ErrNowRecord
	}
	out := NowRecord{Seconds: secs}
	if hasHeight {
		h, ok := digitsInRange(heightText, 9, 1, composerMaxHeight)
		if !ok {
			return NowRecord{}, ErrNowRecord
		}
		out.Height, out.HasHeight = h, true
	}
	return out, nil
}

// parseOriginPath is the host's key: path grammar as applied to the text
// between "fp/" and "]": one or more elements, each ASCII digits with an
// optional ' or h hardening marker, no signs, no blanks, no empty element, and
// every index below 2^31. The range check is not decorative: bip32's in-band
// hardening convention spells hardened 0 as 2147483648, so an UNHARDENED
// component written "2147483648" would otherwise be re-read as 0h -- a
// different origin from the one on the record (composer-S2-exec-review-r0 C-1;
// the host refuses it via ChildNumber::from_normal_idx).
func parseOriginPath(s string) (bip32.Path, bool) {
	if s == "" {
		return nil, false
	}
	var out bip32.Path
	for _, el := range strings.Split(s, "/") {
		digits := strings.TrimSuffix(strings.TrimSuffix(el, "'"), "h")
		if len(digits) == len(el) {
			// unhardened: nothing was trimmed
		} else if len(el)-len(digits) != 1 {
			return nil, false
		}
		if digits == "" {
			return nil, false
		}
		var idx uint64
		for i := 0; i < len(digits); i++ {
			if digits[i] < '0' || digits[i] > '9' {
				return nil, false
			}
			idx = idx*10 + uint64(digits[i]-'0')
			if idx >= 1<<31 {
				return nil, false
			}
		}
		v, err := bip32.ParsePathElement(el)
		if err != nil {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// ParseKeyRecord: hex of "[<8 lowercase hex fp>/<path>]<xpub>" where the xpub
// is a public extended key of depth 3 or 4, the path has as many components as
// the xpub's depth, and its last component is the xpub's own child number.
// The fingerprint, account and interior components are DECLARATIONS nothing
// here can verify (F-217); the mapping review says so.
func ParseKeyRecord(record string) (KeyRecord, error) {
	body, ok := strings.CutPrefix(record, KeyPrefix)
	if !ok {
		return KeyRecord{}, ErrKeyRecord
	}
	b, ok := unhexLower(body)
	if !ok || !utf8.Valid(b) {
		return KeyRecord{}, ErrKeyRecord
	}
	text := string(b)
	rest, ok := strings.CutPrefix(text, "[")
	if !ok {
		return KeyRecord{}, ErrKeyRecord
	}
	originText, xpubText, ok := strings.Cut(rest, "]")
	if !ok {
		return KeyRecord{}, ErrKeyRecord
	}
	fpText, pathText, ok := strings.Cut(originText, "/")
	if !ok {
		return KeyRecord{}, ErrKeyRecord
	}
	fpBytes, ok := unhexLower(fpText)
	if !ok || len(fpBytes) != 4 {
		return KeyRecord{}, ErrKeyRecord
	}
	origin, ok := parseOriginPath(pathText)
	if !ok {
		return KeyRecord{}, ErrKeyRecord
	}
	ek, err := hdkeychain.NewKeyFromString(xpubText)
	if err != nil || ek.IsPrivate() {
		return KeyRecord{}, ErrKeyRecord
	}
	depth := int(ek.Depth())
	if depth != 3 && depth != 4 {
		return KeyRecord{}, ErrKeyRecord
	}
	if len(origin) != depth {
		return KeyRecord{}, ErrKeyRecord
	}
	if origin[len(origin)-1] != ek.ChildIndex() {
		return KeyRecord{}, ErrKeyRecord
	}
	var fp [4]byte
	copy(fp[:], fpBytes)
	return KeyRecord{Fingerprint: fp, Origin: origin, Xpub: xpubText, Text: text}, nil
}
