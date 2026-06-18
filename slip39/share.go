package slip39

import (
	"errors"
	"strings"
)

var (
	errWrongLength     = errors.New("slip39: wrong word count")
	errUnsupportedSize = errors.New("slip39: 256-bit shares not supported")
	errNotInWordlist   = errors.New("slip39: word not in wordlist")
	errBadChecksum     = errors.New("slip39: bad checksum")
)

// Share is a parsed SLIP-39 share's header metadata (Tier 1: no secret value
// reconstruction). Fields are decoded from the share's bit layout; the RS1024
// checksum has been verified. Mnemonic holds the canonical (uppercase) words.
type Share struct {
	Mnemonic        []string
	Identifier      int  // 15-bit
	Extendable      bool // ext flag (selects the RS1024 customization string)
	IterationExp    int  // 4-bit
	GroupIndex      int  // 4-bit
	GroupThreshold  int  // decoded (stored + 1)
	GroupCount      int  // decoded (stored + 1)
	MemberIndex     int  // 4-bit
	MemberThreshold int  // decoded (stored + 1)
}

const (
	wordsShort = 20 // 128-bit
	wordsLong  = 33 // 256-bit (unsupported in Tier 1)
)

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

// ParseShare validates a SLIP-39 share mnemonic (Tier 1, 128-bit/20-word only)
// and returns its decoded header. Checks: exactly 20 words, all in the wordlist
// (case-insensitive), valid RS1024 checksum (customization string per the ext
// bit), nothing else (no secret reconstruction). A 33-word (256-bit) share is
// rejected as unsupported. Returns a classifiable sentinel error on failure.
func ParseShare(mnemonic string) (Share, error) {
	fields := strings.Fields(mnemonic)
	switch len(fields) {
	case wordsShort:
	case wordsLong:
		return Share{}, errUnsupportedSize
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
	return Share{
		Mnemonic:        canonical,
		Identifier:      int(hdr >> 25),
		Extendable:      ext,
		IterationExp:    int((hdr >> 20) & 0xf),
		GroupIndex:      int((hdr >> 16) & 0xf),
		GroupThreshold:  int((hdr>>12)&0xf) + 1,
		GroupCount:      int((hdr>>8)&0xf) + 1,
		MemberIndex:     int((hdr >> 4) & 0xf),
		MemberThreshold: int(hdr&0xf) + 1,
	}, nil
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
	case errors.Is(err, errUnsupportedSize):
		return "256-bit not supported"
	case errors.Is(err, errWrongLength):
		return "wrong length"
	default:
		return "invalid"
	}
}
