package slip39

import (
	"errors"
	"strings"
)

var (
	errWrongLength                = errors.New("slip39: wrong word count")
	errNotInWordlist              = errors.New("slip39: word not in wordlist")
	errBadChecksum                = errors.New("slip39: bad checksum")
	errBadPadding                 = errors.New("slip39: bad padding")
	errGroupThresholdExceedsCount = errors.New("slip39: group threshold exceeds count")
)

// Share is a parsed SLIP-39 share's header metadata plus its extracted share
// VALUE. Fields are decoded from the share's bit layout; the RS1024 checksum
// has been verified. Mnemonic holds the canonical (uppercase) words; Value
// holds the secret-bearing share-value bytes (16/20/24/28/32) consumed by
// Combine (the verbatim single-share engrave path ignores Value).
type Share struct {
	Mnemonic        []string
	Value           []byte // share-value bytes (secret-bearing; for Combine)
	Identifier      int    // 15-bit
	Extendable      bool   // ext flag (selects the RS1024 customization string)
	IterationExp    int    // 4-bit
	GroupIndex      int    // 4-bit
	GroupThreshold  int    // decoded (stored + 1)
	GroupCount      int    // decoded (stored + 1)
	MemberIndex     int    // 4-bit
	MemberThreshold int    // decoded (stored + 1)
}

// rs1024GEN / rs1024Polymod / rs1024Verify implement the SLIP-0039 RS1024
// checksum over GF(1024) (error-detection only — NOT secret handling).
var rs1024GEN = [10]uint32{
	0xe0e040, 0x1c1c080, 0x3838100, 0x7070200, 0xe0e0009,
	0x1c0c2412, 0x38086c24, 0x3090fc48, 0x21b1f890, 0x3f3f120,
}

func rs1024Polymod(values []int) uint32 {
	chk := uint32(1)
	for _, v := range values {
		b := chk >> 20
		chk = (chk&0xfffff)<<10 ^ uint32(v)
		for i := 0; i < 10; i++ {
			if (b>>uint(i))&1 != 0 {
				chk ^= rs1024GEN[i]
			}
		}
	}
	return chk
}

func rs1024Verify(cs string, data []int) bool {
	vals := make([]int, 0, len(cs)+len(data))
	for _, c := range []byte(cs) {
		vals = append(vals, int(c))
	}
	vals = append(vals, data...)
	return rs1024Polymod(vals) == 1
}

// exactWord returns the Word index for a case-insensitive EXACT wordlist match
// (ClosestWord is a prefix match, so verify LabelFor(w) == upper(word)).
func exactWord(word string) (Word, bool) {
	u := strings.ToUpper(word)
	w, _ := ClosestWord(u)
	if w < 0 || LabelFor(w) != u {
		return -1, false
	}
	return w, true
}

// ParseShare validates a SLIP-39 share mnemonic and returns its decoded header
// plus the extracted share VALUE. Checks: a valid word count ∈ {20,23,27,30,33}
// (master-secret sizes 16/20/24/28/32 B), all words in the wordlist
// (case-insensitive), valid RS1024 checksum (customization string per the ext
// bit), group_count ≥ group_threshold (structural), and leading value-pad bits
// all zero. Returns a classifiable sentinel error on failure.
func ParseShare(mnemonic string) (Share, error) {
	fields := strings.Fields(mnemonic)
	// Accepted word counts (M4 canonical form): 7 metadata words + a value
	// field of {13,16,20,23,26} words → master-secret {16,20,24,28,32} B.
	switch len(fields) {
	case 20, 23, 27, 30, 33:
	default:
		return Share{}, errWrongLength
	}
	indices := make([]int, len(fields))
	canonical := make([]string, len(fields))
	for i, f := range fields {
		w, ok := exactWord(f)
		if !ok {
			return Share{}, errNotInWordlist
		}
		indices[i] = int(w)
		canonical[i] = LabelFor(w)
	}
	// First 4 words = the 40-bit header. uint64 is REQUIRED: on RP2350/TinyGo
	// int is 32-bit and a 40-bit shift would overflow.
	hdr := uint64(indices[0])<<30 | uint64(indices[1])<<20 | uint64(indices[2])<<10 | uint64(indices[3])
	ext := (hdr>>24)&1 == 1
	cs := "shamir"
	if ext {
		cs = "shamir_extendable"
	}
	if !rs1024Verify(cs, indices) {
		return Share{}, errBadChecksum
	}
	groupThreshold := int((hdr>>12)&0xf) + 1
	groupCount := int((hdr>>8)&0xf) + 1
	// Structural check (port share.rs:248-256): reject group_count <
	// group_threshold before extracting the value.
	if groupCount < groupThreshold {
		return Share{}, errGroupThresholdExceedsCount
	}
	// Value field = words [4 : len-3]. valueWords*10 bits, left-padded with
	// padBits zeros; padBits ≤ 8 for all five accepted counts.
	valueWords := len(fields) - 7
	padBits := (10 * valueWords) % 16
	valueBytes := (10*valueWords - padBits) / 8
	val, ok := decodeValue(indices[4:len(indices)-3], padBits, valueBytes)
	if !ok {
		return Share{}, errBadPadding
	}
	return Share{
		Mnemonic:        canonical,
		Value:           val,
		Identifier:      int(hdr >> 25),
		Extendable:      ext,
		IterationExp:    int((hdr >> 20) & 0xf),
		GroupIndex:      int((hdr >> 16) & 0xf),
		GroupThreshold:  groupThreshold,
		GroupCount:      groupCount,
		MemberIndex:     int((hdr >> 4) & 0xf),
		MemberThreshold: int(hdr&0xf) + 1,
	}, nil
}

// decodeValue unpacks value words (10-bit, big-endian, left-padded with
// padBits zeros) into valueBytes bytes; returns false if a leading pad bit
// is set. Byte-oriented (no value-wide accumulator) — TinyGo int is 32-bit.
func decodeValue(valueWords []int, padBits, valueBytes int) ([]byte, bool) {
	getBit := func(i int) byte {
		w := valueWords[i/10] & 0x3ff
		return byte((w >> (9 - i%10)) & 1)
	}
	for i := 0; i < padBits; i++ {
		if getBit(i) != 0 {
			return nil, false
		}
	}
	out := make([]byte, valueBytes)
	for bi := range out {
		var b byte
		for j := 0; j < 8; j++ {
			b = (b << 1) | getBit(padBits+bi*8+j)
		}
		out[bi] = b
	}
	return out, true
}

// Describe returns a short human label for a ParseShare error (for the GUI), or
// "" for nil; unknown errors → "invalid".
func Describe(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errBadChecksum):
		return "bad checksum"
	case errors.Is(err, errNotInWordlist):
		return "unknown word"
	case errors.Is(err, errWrongLength):
		return "wrong length"
	case errors.Is(err, errBadPadding):
		return "bad padding"
	case errors.Is(err, errGroupThresholdExceedsCount):
		return "group threshold exceeds count"
	case errors.Is(err, errIdentifierMismatch):
		return "id mismatch"
	case errors.Is(err, errExtendableMismatch):
		return "extendable mismatch"
	case errors.Is(err, errIterationExponentMismatch):
		return "iteration mismatch"
	case errors.Is(err, errGroupThresholdMismatch):
		return "group threshold mismatch"
	case errors.Is(err, errGroupCountMismatch):
		return "group count mismatch"
	case errors.Is(err, errShareValueLengthMismatch):
		return "value length mismatch"
	case errors.Is(err, errInvalidShareValueLength):
		return "value length mismatch"
	case errors.Is(err, errMemberThresholdMismatch):
		return "member threshold mismatch"
	case errors.Is(err, errDuplicateMemberIndex):
		return "duplicate share"
	case errors.Is(err, errInsufficientShares):
		return "not enough shares"
	case errors.Is(err, errDigestVerificationFailed):
		return "bad share set"
	case errors.Is(err, errEmptyShares):
		return "not enough shares"
	default:
		return "invalid"
	}
}
