// Package sysw reads the SYSTEMWIDE payload container.
//
// It is a behaviour-faithful port of mnemonic-engrave's crates/me-cli/src/sysw,
// which is PRIMARY: per the project's Rust-primary rule this package may never
// lead. A change to normative behaviour lands in Rust with test vectors first,
// and arrives here as a port.
//
// Provenance pin: mnemonic-engrave sysw, branch sysw-container.
//
// There is deliberately NO pack: the device never creates a payload, only reads
// one. Omitting it removes the possibility of the device disagreeing with the
// host about how to build a container it should never build.
//
// Separate from package seal, which reads the SEALED PAYLOAD container at a
// different address with a different magic. The two are frozen apart.
package sysw

// MAGIC is eight bytes, matching MNEMBLOB's width so both containers present a
// same-width discriminator at offset 0.
var MAGIC = [8]byte{'M', 'N', 'E', 'M', 'S', 'Y', 'S', 'W'}

const (
	// RegionAddr is fixed and normative, for the same reason seal.PayloadAddr
	// is: any other value produces a blob the device never looks at. A full
	// megabyte below the Sealed Payload region, so an overrun in either
	// direction hits unprogrammed flash rather than the other feature's data.
	RegionAddr = 0x10D00000
	RegionLen  = 65536

	Version         = 0x01
	KDFPBKDF2SHA256 = 0x01
	AEADAES256GCM   = 0x01

	HeaderLen = 52
	SaltLen   = 16
	IVLen     = 12
	TagLen    = 16

	// MaxSectionLen is EPD §6's cap, inherited unchanged: 8191 rather than
	// 8192 because the scan buffer signals overflow when it is exactly FULL.
	MaxSectionLen = 8191
	MinIterations = 100_000
	MaxIterations = 2_000_000

	// PassphraseMax is over the NORMALISED string, host and device: 8 (the
	// longest BIP-39 English word) x 24 words + 23 separators.
	//
	// NOT passphrase.MaxLen, which is 100 and is by its own comment "a
	// plate-capacity limit chosen for legibility" -- a fact about steel, not
	// about entry. Applying it here would make every long generated passphrase
	// unenterable.
	PassphraseMax = 215

	WordsMin     = 2
	WordsMax     = 24
	WordsDefault = 12
)

type Header struct {
	Iterations uint32
	Salt       [SaltLen]byte
	IV         [IVLen]byte
	PubLen     uint32
	CtLen      uint32
}

func (h Header) Sealed() bool { return h.CtLen > 0 }

// TotalLen is the on-wire length, tag included. Safe only AFTER ParseHeader,
// which is what proves both lengths are bounded and the sum cannot wrap.
func (h Header) TotalLen() int {
	n := HeaderLen + int(h.PubLen) + int(h.CtLen)
	if h.Sealed() {
		n += TagLen
	}
	return n
}
