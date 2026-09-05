package seal

import (
	"bytes"
	"errors"
	"fmt"

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
	// ErrCodex32TooLong is §10.2.1a's refusal, and it is a SEPARATE sentinel
	// from ErrRecordNotPermitted on purpose. §6.4 renders every other admission
	// failure to the operator as "Payload unreadable"; this record is not
	// unreadable. It is intact and too long to cut, and the operator has been
	// taught to read "unreadable" as "someone replaced my payload". Conflating
	// a plate-geometry limit with an attack sends them chasing a compromise
	// that did not happen -- exactly the argument ErrTooManyRecords already
	// won. The length and the classification are authenticated plaintext, so
	// naming them leaks nothing.
	ErrCodex32TooLong = errors.New("seal: codex32 secret is too long to engrave")
)

// RecordNotPermittedError is ErrRecordNotPermitted with the two facts a SCREEN
// needs: WHICH record the allow-list refused, and WHAT that record is (F-474).
//
// The sentinel already named both in its MESSAGE, and that was not enough.
// gui/unlock_kdf.go reaches the error through errors.Is and cannot take a
// message apart, so its `default:` arm rendered every allow-list refusal as
// "Payload unreadable." -- after a successful authentication and a ~31 s
// derivation, on a payload that is intact. That is the same false diagnosis
// ErrCodex32TooLong and ErrTooManyRecords already exist to remove, and §6.4
// requires the machine to distinguish it.
//
// It carries NO record bytes, and that line is not incidental. The index, the
// classification and the section are authenticated plaintext, so naming them
// leaks nothing -- exactly the argument §6.4 won for the record count -- while
// the record itself may be seed material.
//
// Preimage is a FIELD rather than a Classification, because H0 considered a
// class of its own for the kind and rejected one: a preimage plate stays
// ClassUnknown and inert on every classifier, and the reason it was refused
// here is that it is not on the allow-list, not that it is special. This flag
// only lets the screen say WHICH unknown it was, which is the difference
// between "unknown format" and "a hashlock preimage, not a seed".
type RecordNotPermittedError struct {
	Index    int            // 0-based, as `me` counts records
	Class    Classification // Classify's verdict on the refused record
	Section  Section
	Preimage bool // the record is an unshared kind-0x03 hashlock preimage plate (H0)
}

// Error names the CLASSIFICATION and, when there is one, the kind -- never the
// kind INSTEAD of the classification. The allow-list refused this record on its
// class, and H0's own TestAdmitSectionRefusesAPreimagePlateAsUnknown pins that
// a preimage plate stays "unknown format" here; a message that said only
// "hashlock preimage plate" would erase the fact that test exists to hold.
func (e *RecordNotPermittedError) Error() string {
	what := e.Class.String()
	if e.Preimage {
		what += " (a hashlock preimage plate)"
	}
	return fmt.Sprintf("%s: record %d classifies as %s, which the %s section does not permit",
		ErrRecordNotPermitted.Error(), e.Index, what, e.Section)
}

// Unwrap keeps every existing errors.Is(err, ErrRecordNotPermitted) call site
// working: the type is ADDITIVE, and TestRecordNotPermittedErrorStillMatchesTheSentinel
// is what says so.
func (e *RecordNotPermittedError) Unwrap() error { return ErrRecordNotPermitted }

// MaxEngraveableCodex32Len is §10.2.1a's limit: the longest codex32 secret the
// seed plate's QR can hold.
//
// Derived from backup.EngraveSeedString, which builds the QR from the
// UPPERCASED share at error-correction level M and refuses qrc.Size > 33.
// Measured against the real encoder the boundary is exact and sits between 90
// and 91:
//
//	 63-90  -> QR size 33 -> cuts
//	 91-122 -> QR size 37 -> refused
//	123-127 -> QR size 41 -> refused
//
// codex32.New admits two bands -- short 48-93 and long 125-127 -- so BOTH
// overhang the engraver: 91-93 and every long code. That is why this is stated
// as one length rather than as "reject long codes".
//
// IT IS PINNED, NOT TRUSTED. TestEngraveableLimitIsDerivedFromTheRealQREncoder
// (backup/engraveable_test.go) re-derives this number from qr.Encode and fails
// if it is no longer 90/91. Two things can move it independently: the
// qrc.Size > 33 cap and the error-correction level. Measured: at qr.Q instead
// of qr.M the limit drops to 67 -- below EncodeMS1's ordinary output -- which
// would silently reopen this defect AND reject ordinary seeds. (qrScale is NOT
// among them: the boundary is decided by qr.Encode before qrScale is read.)
//
// Characters, and len() is the right measure: a record only reaches this check
// once Classify has returned ClassCodex32Secret, i.e. once codex32.New has
// parsed it, so every byte is drawn from the bech32 charset and is one
// character.
const MaxEngraveableCodex32Len = 90

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
	// Record is []byte, not string, so Payload.Wipe can zero it. A Go string
	// cannot be zeroed, which would make §10.2 step 10 unimplementable for the
	// one thing that most needs it.
	Record []byte
	Class  Classification

	// HRP is 'd' (md1) or 'k' (mk1) for a ClassMDMK record in EITHER section,
	// and 0 for every other record. §6.3 admits md1/mk1 into the encrypted
	// section explicitly, and vector F's secret set is ms1 x3 / mk1 x6 / md1 x6,
	// so twelve of its fifteen secret records ARE cards. Grouping the encrypted
	// section is LABEL-ONLY (labelEncryptedCards): no decode runs there and no
	// grouping failure rejects a payload. F-77, closed in B2a.
	//
	// It comes from the §6.3 card grouping seal already performs -- the UI must
	// never re-derive it, or the plate list and the decode can disagree about
	// what a record is. Classification cannot stand in: ClassMDMK is one value
	// covering both formats, so it cannot even tell md1 from mk1.
	HRP byte
	// CardIndex/CardTotal identify which (HRP, chunk_set_id) card this record
	// belongs to, 1-based, among cards of the SAME HRP. PlateIndex/PlateTotal
	// identify this record within that card.
	//
	// Indexing cards WITHIN an HRP rather than across the section is what keeps
	// a 2-of-3's three mk1 cosigner cards distinguishable; a flat mk1 1/6..6/6
	// conflates them, which is §6.4's incomplete-backup-believed-complete
	// hazard wearing a label.
	CardIndex, CardTotal   int
	PlateIndex, PlateTotal int
}

// cmdPrefix mirrors gui/scan.go:56. A decrypted plaintext of
// "command: lock-boot" would reach gui.go:1672 and call Platform.LockBoot,
// which does writeOTPValues -> otp.EnableSecureBoot -> machine.CPUReset
// (cmd/controller/platform_sh2.go:545). This prefix is the only gate, and the
// wire format is normative and public, so the device MUST NOT assume the blob
// was produced by a conforming sealer.
const cmdPrefix = "command: "

// Classify reproduces gui/scan.go's Scan branch order for a single record.
//
// HONEST CAVEAT about the string copy, CORRECTED (lens 3 M1). An earlier version
// of this comment said "the copy's lifetime is this function". That is false: a
// Go string is a heap allocation that outlives the frame until the GC collects
// it, is never zeroed, and — the fork's own standing caveat — TinyGo's GC may
// copy or retain. Classify runs from AdmitSection(recs, SectionEncrypted), i.e.
// from inside UnlockWithKey, once per decrypted record, so these copies live
// from unlock to power-off rather than for the duration of one plate.
//
// What is done about it: the conversion is made as LATE as possible, below the
// three branches that take []byte. A command record, a bare mnemonic and a
// descriptor are therefore never stringified at all — which removes the
// ClassMnemonic case entirely, and that is the one that is seed material. An
// ms1 still gets a string copy because codex32.New needs one; that residue is
// F-88's, and it is the caveat gui/ms1_decode.go:19-20 already carries. Stated
// rather than claimed away: §2.2 item 9's "after that RAM holds public records
// only" does not hold for it.
//
// The branch ORDER is normative and unchanged — moving the conversion moves no
// test, because nothing above the new site consults s.
func Classify(b []byte) Classification {
	if bytes.HasPrefix(b, []byte(cmdPrefix)) {
		return ClassDebugCommand
	}
	if m, err := bip39.Parse(b); err == nil {
		// Parse returns a full, WIPEABLE []Word copy of the record. Every other
		// copy this function makes is an immutable string it cannot zero; this
		// one it can, so it does. (Parse now also zeroes its own accumulator on
		// each ERROR exit, so the reject path leaves nothing behind either.)
		clear(m)
		return ClassMnemonic
	}
	if _, err := nonstandard.OutputDescriptor(b); err == nil {
		return ClassDescriptor
	}
	// From here down the engines need a string. Nothing above this line touches
	// s, so the two classes that carry seed material never allocate one.
	s := string(b)
	if c, err := codex32.New(s); err == nil && !codex32.IsPreimage(c) {
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

// isPreimageRecord is H0's own predicate, asked of a record the allow-list has
// already refused, so a screen can say "a hashlock preimage, not a seed"
// instead of "unknown format" (F-474).
//
// It is deliberately not part of Classify: H0 rejected a Classification of its
// own for the kind, and adding one here would put a preimage on a code path
// that classifies rather than one that refuses.
func isPreimageRecord(r []byte) bool {
	c, err := codex32.New(string(r))
	return err == nil && codex32.IsPreimage(c)
}

// AdmitSection runs all three passes over one section's records. On any failure
// it returns NO records: rejection is whole-payload, and an empty result is
// Phase A's expression of "nothing was engraved".
func AdmitSection(records [][]byte, section Section) ([]AdmittedRecord, error) {
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
			wipe(out)
			return nil, fmt.Errorf("%w: record %d, byte %d", ErrNotLowercase, i, pos)
		}
		// Pass 2 — §10.2.1's allow-list.
		c := Classify(r)
		if !permitted(section, c) {
			wipe(out)
			// isPreimageRecord runs ONLY here, on the refusal path that returns
			// immediately, so the happy path pays nothing for it (F-474).
			return nil, &RecordNotPermittedError{
				Index: i, Class: c, Section: section, Preimage: isPreimageRecord(r),
			}
		}
		// Pass 2a — §10.2.1a. An ms1 the seed plate cannot hold is refused HERE,
		// in the per-record pass, and never in the post-loop section block: there
		// it would leak every ms1 already copied into `out`, unreachable to both
		// Payload.Wipe and RecordsResident(). So it wipes what it copied and
		// returns nothing, exactly as the two allow-list failure paths above do.
		//
		// AFTER the allow-list, not before it, and the order is load-bearing. An
		// over-length codex32 secret in the PUBLIC section is a secret shipped in
		// the clear -- a far more serious finding than a plate that does not fit
		// -- and the operator must be told the serious one. Checking first turns
		// ErrRecordNotPermitted into ErrCodex32TooLong for exactly that case.
		//
		// ms1 ONLY. mdmkText and mnemonics are deliberately not covered: a
		// descriptor too long for one record CHUNKS across records that
		// reassemble by (HRP, chunk_set_id), and md/mk plates have VARIANTS
		// (backup.EngraveText via engrave.QR, with "TEXT + QR" and "QR ONLY"
		// among the shapes gui/gui.go offers). A seed share has neither -- it is
		// atomic, and backup.EngraveSeedString has exactly one shape.
		if c == ClassCodex32Secret && len(r) > MaxEngraveableCodex32Len {
			wipe(out)
			return nil, fmt.Errorf(
				"%w: record %d is a codex32 secret of %d characters; this machine can engrave at most %d",
				ErrCodex32TooLong, i, len(r), MaxEngraveableCodex32Len)
		}
		// COPY, not a slice into the caller's buffer. Unlock does
		// `defer clear(plaintext)` and SplitSection slices into that plaintext, so
		// retaining r would hand Phase B records that zero themselves the moment
		// Unlock returns. The copy is also what Payload.Wipe owns.
		out = append(out, AdmittedRecord{Record: append([]byte(nil), r...), Class: c})
	}
	// Pass 3 — §6.3's card-set decode, once per GROUP, public section only.
	// ValidMD/ValidMK never open the payload, so classification is not
	// sufficient: without this a defective or third-party sealer can put seed
	// entropy in the cleartext section, where `picotool save` reaches it with
	// no passphrase at all.
	if section == SectionPublic {
		// Public records are cleartext by definition, so stringifying them
		// copies nothing that needs wiping.
		strs := make([]string, len(records))
		for i, r := range records {
			strs[i] = string(r)
		}
		// The grouping is computed ONCE and both consumers read the same
		// object. Two groupings is two chances to disagree about what a card
		// is, and the decode and the plate list disagreeing is precisely the
		// divergence one headless entry point exists to eliminate.
		//
		// It is computed HERE, after the pass-1/pass-2 loop above, and never
		// before it. cardKey fails closed for anything that is not an md1/mk1
		// card, so grouping first would send a record the allow-list is about
		// to refuse into cardKey and change the rejection sentinel --
		// TestPublicSectionRefusesASecret asserts ErrRecordNotPermitted, and
		// the wrong order turns that into ErrUndecodableCardSet.
		g, err := groupRecords(strs)
		if err != nil {
			return nil, err
		}
		if err := decodePublicSet(g); err != nil {
			return nil, err
		}
		// Backfill §10.2.2's plate identity onto records already admitted.
		labelCards(out, g)
	}
	// F-77 — pass 3's grouping, published for the encrypted section too. It is
	// LABEL-ONLY: see labelEncryptedCards. No decode, and no new rejection.
	if section == SectionEncrypted {
		labelEncryptedCards(out)
	}
	return out, nil
}

// labelCards publishes the §6.3 grouping on the admitted records, in record
// order. It ADDS NO §6.3 LOGIC: every value here comes out of the partition
// groupRecords already built for the decode.
func labelCards(out []AdmittedRecord, g grouping) {
	// Cards of each HRP, in first-seen order. cardTotal is complete only after
	// this loop, which is why the assignment below is a second pass.
	cardIdx := make(map[groupKey]int, len(g.keys))
	cardTotal := make(map[byte]int, 2)
	for _, k := range g.keys {
		cardTotal[k.hrp]++
		cardIdx[k] = cardTotal[k.hrp]
	}
	plate := make(map[groupKey]int, len(g.keys))
	for i, k := range g.perRecord {
		plate[k]++
		out[i].HRP = k.hrp
		out[i].CardIndex = cardIdx[k]
		out[i].CardTotal = cardTotal[k.hrp]
		out[i].PlateIndex = plate[k]
		out[i].PlateTotal = len(g.groups[k])
	}
}

// firstUpperASCII returns the byte offset of the first uppercase ASCII letter,
// or -1.
//
// ASCII rather than unicode.IsUpper deliberately. Every record that can pass
// the allow-list is drawn from the bech32/codex32 alphabet, so the two agree on
// everything admissible — and the fork already records that the unicode package
// costs ~55 KB of RAM on this target (gui/scan.go:62), which is real money for
// a check that cannot observe the difference.
func firstUpperASCII(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
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

// grouping is the §6.3 card partition of one section's records, computed ONCE
// and read by both the decode and §10.2.2's plate labels.
//
// perRecord is each record's card identity IN RECORD ORDER. keys and groups
// alone cannot give it back: two records of one card are indistinguishable
// inside groups[k], so without this the labels would need a second cardKey
// pass over the same input.
type grouping struct {
	keys      []groupKey
	groups    map[groupKey][]string
	perRecord []groupKey
}

// groupRecords partitions records into card sets, preserving first-seen order
// so the decode below is deterministic.
func groupRecords(records []string) (grouping, error) {
	g := grouping{
		keys:      make([]groupKey, 0, len(records)),
		groups:    make(map[groupKey][]string, len(records)),
		perRecord: make([]groupKey, len(records)),
	}
	for i, r := range records {
		k, err := cardKey(r, i)
		if err != nil {
			return grouping{}, err
		}
		if _, seen := g.groups[k]; !seen {
			g.keys = append(g.keys, k)
		}
		g.groups[k] = append(g.groups[k], r)
		g.perRecord[i] = k
	}
	return g, nil
}

// groupCards is groupRecords' cards-only view: the partition without the
// per-record index. It is what §6.3 is ABOUT -- which records form a card --
// and the two grouping tests assert against it directly.
func groupCards(records []string) ([]groupKey, map[groupKey][]string, error) {
	g, err := groupRecords(records)
	if err != nil {
		return nil, nil, err
	}
	return g.keys, g.groups, nil
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
// It takes the grouping rather than computing one: AdmitSection already built
// it, and the plate labels read the same object.
func decodePublicSet(g grouping) error {
	for _, k := range g.keys {
		set := g.groups[k]
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

// wipe zeroes records already copied into a partial result that is about to be
// discarded.
//
// ITS CALL SITES ARE NOT REGRESSION-TESTED, and cannot cheaply be: `out` is
// never returned on the paths that call this, so no test can observe those
// bytes through the public API without `unsafe`. Deleting any of the THREE
// calls — the two allow-list paths and §10.2.1a's engraveability check — leaves
// the whole suite green. Re-measured 2026-08-10 when the third was added, by
// deleting it: `go test ./seal/ ./backup/ ./gui/` stayed green.
// TestWipeZeroesAPartialResult below covers this function's own behaviour, so a
// no-op `wipe` is caught; a REMOVED call is not. Saying so beats a contorted
// test that pretends otherwise.
//
// What IS testable about placement is WHICH record a section stops at, and
// TestTooLongSecretIsCaughtInThePerRecordPass uses it: an over-length secret at
// index 0 ahead of an allow-list failure at index 1 must report
// ErrCodex32TooLong, which only a check inside the loop can do.
//
// Whole-payload rejection means the caller never sees these records, but they
// are copies of possibly-secret bytes and clearing them costs nothing.
func wipe(recs []AdmittedRecord) {
	for _, r := range recs {
		clear(r.Record)
	}
}
