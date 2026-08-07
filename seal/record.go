package seal

import (
	"errors"
	"fmt"
	"strings"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/nonstandard"
)

// §10.2.1's classifier allow-list and §6.3's card-set decode. This is what
// stops a seed reaching steel in the clear.
//
// THREE passes, distinct, none substituting for another:
//
//  1. the case check, once per record (§6.4)
//  2. the allow-list, once per record (§10.2.1)
//  3. the decode, once per card GROUP (§6.3)
//
// Any single failure in any of them rejects the WHOLE payload (§6.4). Partial
// acceptance would leave the operator engraving an incomplete wallet backup
// while believing it complete, which is the worst available outcome.

var (
	ErrNotLowercase       = errors.New("seal: record is not lowercase")
	ErrRecordNotPermitted = errors.New("seal: record classification not permitted in this section")
	ErrUndecodableCardSet = errors.New("seal: public records do not form a decodable card set")
)

// Section is where a record was carried. Secrecy is decided by section
// placement, checked against CLASSIFIED CONTENT — never against anything the
// sealer asserts. codex32.New accepts secret shares, so nothing on the wire
// binds what actually engraves; the device must look at the content.
type Section int

const (
	SectionPublic Section = iota
	SectionEncrypted
)

func (s Section) String() string {
	if s == SectionPublic {
		return "public"
	}
	return "encrypted"
}

// Classification is one of gui/scan.go's classifier outcomes.
//
// The list and the ORDER below are taken from Scan (gui/scan.go:28-81) and must
// stay in step with it. Order is load-bearing, not cosmetic: a record that Scan
// would return as a descriptor must not be re-classified here as md1/mk1 and
// admitted, so the branches are evaluated in Scan's sequence.
type Classification int

const (
	ClassUnknown Classification = iota
	ClassDebugCommand
	ClassMnemonic
	ClassDescriptor
	ClassCodex32Secret
	ClassMDMK
	ClassAddress
)

func (c Classification) String() string {
	switch c {
	case ClassDebugCommand:
		return "debug command"
	case ClassMnemonic:
		return "BIP-39 mnemonic"
	case ClassDescriptor:
		return "output descriptor"
	case ClassCodex32Secret:
		return "codex32 secret"
	case ClassMDMK:
		return "md1/mk1 card"
	case ClassAddress:
		return "bitcoin address"
	default:
		return "unknown format"
	}
}

// AdmittedRecord is a record that passed all three passes, with the
// classification that admitted it. Phase B labels each plate by this — never by
// anything the sealer asserted.
type AdmittedRecord struct {
	Record string
	Class  Classification
}

// cmdPrefix mirrors gui/scan.go:56. A decrypted plaintext of
// "command: lock-boot" would reach gui.go:1672 and call Platform.LockBoot,
// which does writeOTPValues -> otp.EnableSecureBoot -> machine.CPUReset
// (cmd/controller/platform_sh2.go:545). This prefix is the only gate, and the
// wire format is normative and public, so the device MUST NOT assume the blob
// was produced by a conforming sealer.
const cmdPrefix = "command: "

// Classify reproduces gui/scan.go's Scan branch order for a single record.
func Classify(s string) Classification {
	if strings.HasPrefix(s, cmdPrefix) {
		return ClassDebugCommand
	}
	if _, err := bip39.Parse([]byte(s)); err == nil {
		return ClassMnemonic
	}
	if _, err := nonstandard.OutputDescriptor([]byte(s)); err == nil {
		return ClassDescriptor
	}
	if _, err := codex32.New(s); err == nil {
		return ClassCodex32Secret
	}
	if codex32.ValidMD(s) || codex32.ValidMK(s) {
		return ClassMDMK
	}
	if _, err := btcaddr.DecodeAddress(s, &chaincfg.MainNetParams); err == nil {
		return ClassAddress
	}
	if _, err := btcaddr.DecodeAddress(s, &chaincfg.TestNet3Params); err == nil {
		return ClassAddress
	}
	return ClassUnknown
}

// permitted is an ALLOW-list, not a deny-list. A deny-list silently admits
// whatever branch Scan grows next, and one of the branches it already has burns
// OTP fuses.
//
//	public    | md1/mk1 only, AND every card group must reassemble and decode
//	encrypted | md1/mk1, a codex32 secret (ms1), or a parsed BIP-39 mnemonic
func permitted(section Section, c Classification) bool {
	if c == ClassMDMK {
		return true
	}
	return section == SectionEncrypted &&
		(c == ClassCodex32Secret || c == ClassMnemonic)
}

// AdmitSection runs all three passes over one section's records. On any failure
// it returns NO records: rejection is whole-payload, and an empty result is
// Phase A's expression of "nothing was engraved".
func AdmitSection(records []string, section Section) ([]AdmittedRecord, error) {
	out := make([]AdmittedRecord, 0, len(records))
	for i, r := range records {
		// Pass 1 — §6.4's all-lowercase rule, BEFORE classification, binding
		// BOTH sections. Measured: ValidMD returns true for a fully uppercased
		// md1, by design, and the device's own keyboard-entry path emits
		// uppercase (gui/codex32_input_test.go:62). Without this the same
		// wallet has two spec-legal encodings and therefore two §6.6 hashes,
		// so the operator sees a mismatch on an UNTAMPERED payload and learns
		// that mismatches are normal.
		if pos := firstUpperASCII(r); pos >= 0 {
			return nil, fmt.Errorf("%w: record %d, byte %d", ErrNotLowercase, i, pos)
		}
		// Pass 2 — §10.2.1's allow-list.
		c := Classify(r)
		if !permitted(section, c) {
			return nil, fmt.Errorf("%w: record %d classifies as %s, which the %s section does not permit",
				ErrRecordNotPermitted, i, c, section)
		}
		out = append(out, AdmittedRecord{Record: r, Class: c})
	}
	// Pass 3 — §6.3's card-set decode, once per GROUP, public section only.
	// ValidMD/ValidMK never open the payload, so classification is not
	// sufficient: without this a defective or third-party sealer can put seed
	// entropy in the cleartext section, where `picotool save` reaches it with
	// no passphrase at all.
	if section == SectionPublic {
		if err := decodePublicSet(records); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// firstUpperASCII returns the byte offset of the first uppercase ASCII letter,
// or -1.
//
// ASCII rather than unicode.IsUpper deliberately. Every record that can pass
// the allow-list is drawn from the bech32/codex32 alphabet, so the two agree on
// everything admissible — and the fork already records that the unicode package
// costs ~55 KB of RAM on this target (gui/scan.go:62), which is real money for
// a check that cannot observe the difference.
func firstUpperASCII(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return i
		}
	}
	return -1
}

// groupKey is a CARD's identity.
//
// (hrp, chunked, csid, uniq) — and `chunked` comes from the parsed header, NOT
// from the chunk-set id. ChunkSetID == 0 is returned for a non-chunked record
// (md/chunk.go:195) AND is a legal value for a chunked one (md/chunk.go:66,
// mk/mk.go:75-76), so keying on csid conflates the two in BOTH directions:
// unrelated single-string cards collide into one group, and a legitimate
// chunked card with csid 0 is split into failing singletons. Rust avoids this
// with Option<u32> (record.rs:113).
//
// uniq is i+1 for a non-chunked record — its own card — and 0 otherwise.
type groupKey struct {
	hrp     byte // 'd' for md1, 'k' for mk1
	chunked bool
	csid    uint32
	uniq    int
}

// groupCards partitions records into card sets, preserving first-seen order so
// the decode below is deterministic.
func groupCards(records []string) ([]groupKey, map[groupKey][]string, error) {
	keys := make([]groupKey, 0, len(records))
	groups := make(map[groupKey][]string, len(records))
	for i, r := range records {
		k, err := cardKey(r, i)
		if err != nil {
			return nil, nil, err
		}
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], r)
	}
	return keys, groups, nil
}

// cardKey reads the card identity off a raw, not-yet-grouped record. Both
// accessors are verified operational on exactly that, which is what §6.3
// requires — and NOT derive_chunk_set_id, whose input is only obtainable after
// a group has already been reassembled and decoded.
func cardKey(s string, i int) (groupKey, error) {
	switch {
	case codex32.ValidMD(s):
		// Two return values: md/chunk.go:185. The live device site branches on
		// .Chunked exactly this way (gui/md1_gather.go:38).
		h, err := md.ParseChunkHeader(s)
		if err != nil {
			return groupKey{}, fmt.Errorf("%w: record %d: %v", ErrUndecodableCardSet, i, err)
		}
		if !h.Chunked {
			return groupKey{hrp: 'd', uniq: i + 1}, nil
		}
		return groupKey{hrp: 'd', chunked: true, csid: h.ChunkSetID}, nil
	case codex32.ValidMK(s):
		// mk/mk.go:56; the live site is gui/mk1_inspect.go:65.
		h, err := mk.ParseHeader(s)
		if err != nil {
			return groupKey{}, fmt.Errorf("%w: record %d: %v", ErrUndecodableCardSet, i, err)
		}
		if !h.Chunked {
			return groupKey{hrp: 'k', uniq: i + 1}, nil
		}
		return groupKey{hrp: 'k', chunked: true, csid: h.ChunkSetID}, nil
	default:
		// Unreachable behind the allow-list, which admits only ClassMDMK into
		// the public section. Fail closed rather than group it with anything.
		return groupKey{}, fmt.Errorf("%w: record %d is not an md1 or mk1 card",
			ErrUndecodableCardSet, i)
	}
}

// decodePublicSet enforces §6.3: every public record belongs to a card set that
// REASSEMBLES AND DECODES, every group must succeed, and no record may be left
// over. Records are chunks, so this is necessarily a whole-set operation — a
// per-record decode rejects every legitimate payload, including vectors D, E
// and G.
//
// Dispatch mirrors record.rs::decode_public_set. Neither path handles both
// forms: md.Reassemble on a non-chunked md1 gives "wire version mismatch" and
// md.Decode on a chunked one gives "chunked md1 not supported" — both measured.
//
// Two of Rust's checks are deliberately NOT ported, because Go satisfies them
// by construction — verified, so the omission reads as intentional rather than
// forgotten. record.rs:78-84 rejects BCH-CORRECTED mk1; codex32.ValidMK and
// mk.Decode do no correction at all. record.rs's first_noncanonical rejects
// interior spaces and hyphens; the codex32 engine's inputChar has no mapping
// for 0x20 or '-', so ValidMD/ValidMK already return false and the allow-list
// refuses them.
func decodePublicSet(records []string) error {
	keys, groups, err := groupCards(records)
	if err != nil {
		return err
	}
	for _, k := range keys {
		set := groups[k]
		var derr error
		switch {
		case k.hrp == 'd' && k.chunked:
			_, derr = md.Reassemble(set)
		case k.hrp == 'd':
			if len(set) != 1 {
				// Unreachable: uniq makes every non-chunked record its own
				// group. Fail closed rather than silently decode set[0].
				return fmt.Errorf("%w: %d non-chunked md1 records share a card", ErrUndecodableCardSet, len(set))
			}
			_, derr = md.Decode(set[0])
		default: // 'k', chunked or not — mk.Decode handles both.
			_, derr = mk.Decode(set)
		}
		if derr != nil {
			return fmt.Errorf("%w: %c-card: %v — a BCH-valid string is not proof of a real wallet card",
				ErrUndecodableCardSet, k.hrp, derr)
		}
	}
	return nil
}
