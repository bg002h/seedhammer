package codex32

import "errors"

// m-format ms1 payload prefix bytes (ms-codec consts.rs:17,39). The prefix is
// the FIRST byte of the codex32 data payload (Seed()[0]) — NOT the 4-char
// id/Tag, which is "entr" for both entr and mnem secrets.
const (
	msPrefixEntr     = 0x00 // RESERVED_PREFIX: payload = [0x00][entropy]
	msPrefixMnem     = 0x02 // MNEM_PREFIX:     payload = [0x02][language][entropy]
	msPrefixPreimage = 0x03 // PREIMAGE_PREFIX: payload = [0x03][32-byte hashlock preimage] (SPEC_ms_hashlock §1)
	msMaxLanguage    = 9    // MNEM_LANGUAGE_NAMES indices 0..9
)

var (
	errMSBadPrefix   = errors.New("codex32: not an m-format secret payload")
	errMSBadLanguage = errors.New("codex32: invalid mnem wordlist language")
	errMSBadLength   = errors.New("codex32: invalid entropy length")
)

// MSLanguageNames are the BIP-39 wordlist names indexed by the mnem language
// byte (ms-codec consts.rs:47-58).
var MSLanguageNames = [10]string{
	"English", "Japanese", "Korean", "Spanish",
	"Chinese (Simplified)", "Chinese (Traditional)",
	"French", "Italian", "Czech", "Portuguese",
}

// DecodeMS1 decodes the m-format ms1 secret payload from a New-valid, UNSHARED
// codex32 string: prefix = Seed()[0] (msPrefixEntr/msPrefixMnem); for mnem,
// language = Seed()[1] (0..9); entropy = the remaining 16/20/24/28/32 bytes.
// language is 0 for entr. Deterministic; the returned entropy is SECRET (caller
// scrubs). Callers MUST pass only the unshared secret — a K-of-N share carries
// an SSS-evaluated point, not an m-format payload, and yields errMSBadPrefix/Length.
func DecodeMS1(s String) (prefix, language int, entropy []byte, err error) {
	data := s.Seed()
	if len(data) < 2 {
		return 0, 0, nil, errMSBadPrefix
	}
	switch data[0] {
	case msPrefixEntr:
		prefix, language, entropy = msPrefixEntr, 0, data[1:]
	case msPrefixMnem:
		if len(data) < 3 {
			return 0, 0, nil, errMSBadLength
		}
		language = int(data[1])
		if language > msMaxLanguage {
			return 0, 0, nil, errMSBadLanguage
		}
		prefix, entropy = msPrefixMnem, data[2:]
	default:
		return 0, 0, nil, errMSBadPrefix
	}
	switch len(entropy) {
	case 16, 20, 24, 28, 32:
	default:
		return 0, 0, nil, errMSBadLength
	}
	return prefix, language, entropy, nil
}

// IsPreimage reports whether a New-valid string carries the m-format HASHLOCK
// PREIMAGE kind (SPEC_ms_hashlock §1: payload = [0x03][32 bytes], id `hash`).
//
// H0 (SPEC_ms_hashlock §9): such a string is INERT on this device — never a
// codex32 SECRET and no class of its own — because every path that admits
// ClassCodex32Secret ends at backup.EngraveSeedString, and a hashlock
// preimage is not a seed: engraved as one it exposes a spend secret as a
// backup. DecodeMS1 is deliberately unchanged and still refuses the prefix;
// the device learns to USE a preimage in stage H2, not here.
//
// The question is "is this a preimage SINGLE", not "does some byte equal 3":
// the check is singles-only (§1 rule 2 -- a share's data part is an SSS
// point, and its first byte is whatever the polynomial gave it), and the
// preimage payload is exactly [0x03][32 bytes]. The id is NOT consulted: the
// kind is the prefix byte (§1), a 0x03 single under any other id is a
// mismatch the host refuses, and it is not a seed either way.
//
// THE COLLISION, stated plainly (post-implementation review I-1). A plain
// BIP-93 33-byte seed that begins 0x03 is indistinguishable from a preimage
// plate -- same width, same prefix byte, and the id is not consulted -- and
// IS REFUSED. Roughly 1 in 256 of 33-byte seeds. The 16-, 20-, 24-, 28- and
// 32-byte seeds are untouched, and so is every share. That is accepted, not
// overlooked: the constellation profile pins a 33-byte payload to a kind
// byte, `me` refuses the identical string (ms-codec 0.7 at the prefix gate,
// 0.8 as a TagKindMismatch), so this is CONVERGENCE with the Rust primary
// rather than a device narrowing; and the alternative -- keying on the id
// `hash` -- would engrave a MISTAGGED REAL PREIMAGE as a seed. A refusal
// costs a re-encode; a wrong cut exposes a spend secret. Pinned by the seam
// corpus row bip93-plain-33-byte-payload-0x03 in both repos.
//
// Reads the prefix fields and one payload byte; nothing new is retained.
func IsPreimage(s String) bool {
	f, err := ParsePrefix(s.String())
	if err != nil || !f.Unshared {
		return false
	}
	d := s.Seed()
	return len(d) == 33 && d[0] == msPrefixPreimage
}

// DecodeMS1Preimage decodes the m-format HASHLOCK PREIMAGE kind (SPEC_ms_hashlock
// §1: payload = [0x03][32 bytes]) from a New-valid string: ONLY an unshared
// single whose data is exactly 33 bytes beginning 0x03 -- the shape IsPreimage
// tests. Every other input is refused with the same errors DecodeMS1 uses:
// errMSBadPrefix for a wrong first byte or a SHARE, errMSBadLength for an
// unshared 0x03 payload that is not 33 bytes.
//
// DecodeMS1 is deliberately NOT taught this kind (r2 review C-2): its five callers
// all treat the result as a SEED. The returned preimage is SECRET; the caller
// scrubs. No screen calls this in stage H2.
func DecodeMS1Preimage(s String) (preimage [32]byte, err error) {
	f, perr := ParsePrefix(s.String())
	if perr != nil || !f.Unshared {
		return preimage, errMSBadPrefix
	}
	d := s.Seed()
	if len(d) == 0 || d[0] != msPrefixPreimage {
		return preimage, errMSBadPrefix
	}
	if len(d) != 1+32 {
		return preimage, errMSBadLength
	}
	copy(preimage[:], d[1:])
	return preimage, nil
}
