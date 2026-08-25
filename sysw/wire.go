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

	// MaxSectionLen -- RAISED from 8191 to 32,734, converging on the Rust
	// primary (crates/me-cli/src/sysw/wire.rs).
	//
	// 8191 was the NFC SCAN BUFFER minus one: gui/scan.go allocates 8*1024 and
	// signals overflow when the buffer is exactly FULL. `sysw` inherited it
	// unchanged, so the FLASH path was capped at an eighth of its own region
	// for a reason belonging to a transport it never uses -- a sysw container
	// reaches the device by picotool at 0x10D00000, never on a tag. A RECORD
	// on a tag is still bound by the scan buffer, and that is a different
	// limit on a different thing.
	//
	// **THE PORT MOVES WITH THE PRIMARY OR THE DEVICE REFUSES WHAT THE HOST
	// EMITS.** `me` raised its constant; leaving this at 8191 would make every
	// container between 8,192 and 32,734 bytes of section load on the host and
	// fail at ParseHeader on the machine -- with "malformed container", which
	// is the wrong sentence for a payload that is exactly right.
	//
	// The formula preserves the property boundBlob's no-wrap argument rests
	// on:
	//
	//	(RegionLen - HeaderLen - TagLen) / 2 = (65536 - 52 - 16) / 2 = 32734
	//
	// so TWO maxed sections plus header plus tag still fit the region. A round
	// 32,768 breaks it by 34 bytes. `seal`'s own cap stays 8191 and stays
	// FROZEN -- EPD's container really is scanned.
	MaxSectionLen = (RegionLen - HeaderLen - TagLen) / 2
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

// The property MaxSectionLen's formula exists to preserve, at COMPILE time:
// two maxed sections plus header plus tag still fit the region, which is what
// boundBlob's 32-bit no-wrap reasoning rests on. A test would fail late; a
// negative array length fails the build.
var _ [RegionLen - (HeaderLen + 2*MaxSectionLen + TagLen)]struct{}

// ...and a round 32,768 would NOT fit -- by 34 bytes. This is why the cap is an
// ugly number, pinned so nobody "tidies" it into a power of two.
var _ [MaxSectionLen - 32734]struct{}

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
