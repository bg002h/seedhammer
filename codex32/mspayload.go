package codex32

import "errors"

// m-format ms1 payload prefix bytes (ms-codec consts.rs:17,39). The prefix is
// the FIRST byte of the codex32 data payload (Seed()[0]) — NOT the 4-char
// id/Tag, which is "entr" for both entr and mnem secrets.
const (
	msPrefixEntr  = 0x00 // RESERVED_PREFIX: payload = [0x00][entropy]
	msPrefixMnem  = 0x02 // MNEM_PREFIX:     payload = [0x02][language][entropy]
	msMaxLanguage = 9    // MNEM_LANGUAGE_NAMES indices 0..9
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
