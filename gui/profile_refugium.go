//go:build refugium

package gui

// The Refugium build profile (Refugium plan F7, decision D1): a Go build tag,
// because -ldflags -X cannot remove code and this build's guarantees are about
// what is ABSENT from the binary.
//
//   - NFC is off for the whole power cycle (D3): no reader is ever read
//     (startScanner, nfcAvailable) and no screen offers or asks for a scan.
//   - The lock-boot OTP writer and the FOREVERLAURA! QA command are not in the
//     build (debugcmd_refugium.go; cmd/controller's lockboot_refugium.go), and
//     neither are the typed QA proof triggers (prooftriggers_refugium.go).
//   - No BIP-39 passphrase is taken (D4): every prompt is a notice, a payload
//     holding a pass: record is refused at load, and the passphrase program is
//     hidden.
//   - No QR of an ms1 string is engraved (§4.4).
//
// gui/refugium_build_test.go polices every *_refugium.go file: each has a
// `X && !refugium` twin and carries none of the forbidden literals.

const refugiumProfile = true

// proofTriggersEnabled gates every proof-trigger comparison. It is a separate
// switch rather than a disabled trigger value because a trigger set to "" would
// FIRE on an empty passphrase or an empty text field.
const proofTriggersEnabled = false
