package gui

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/font/constant"
	"seedhammer.com/seal"
)

// §10.2.2 -- the secret session. Every record that classifies as a secret is
// offered FIRST, consecutively, and each is wiped as its plate leaves the
// screen BY ANY ROUTE: Cut, Skip, Cancel, a failed engrave, an error, ctx.Done.
//
// Why plural is load-bearing: a 2-of-3 wsh-sortedmulti has THREE ms1 cards
// (vector F). Under a singular implementation the operator engraves one of
// three, the plate list then shows only mk1/md1 so nothing looks missing, and
// they store an incomplete backup of a 2-of-3 believing it complete -- §6.4's
// own "worst available outcome".
//
// Why the wipe is keyed on the plate leaving rather than on completion: aborting
// mid-plate to re-seat shifted steel is the machine's most ordinary recovery,
// and keying on completion would leave the seed resident in a state nothing
// guards. Re-cutting then needs a fresh unlock -- twelve words and a ~31 s KDF --
// ONCE THE ENGRAVE SCREEN IS LEFT. A plate merely PAUSED resumes from the
// spline: the record was zeroed before the plate reached the screen, so there is
// nothing left to re-protect.
// That is the price, it is deliberate (operator, 2026-08-07, reaffirmed
// 2026-08-08), and it costs no reboot: the sealed blob is untouched in flash.

// unlockSecretHook is a test-only seam. It observes each stage with the record's
// live bytes, so a test can assert on the BUFFER -- that the record is non-zero
// when offered and zero once its plate has left -- rather than on a return
// value, which cannot tell a wipe from a missing wipe. nil in production.
// Mirrors unlockEngraveHook, the sanctioned in-file seam.
var unlockSecretHook func(stage string, idx int, record []byte)

// unlockSecretLabel names a secret plate by its CLASSIFIED type and its index
// among secrets -- never by anything the sealer asserted, and never by
// rendering the record's contents.
func unlockSecretLabel(c seal.Classification, i, n int) string {
	name := "secret"
	switch c {
	case seal.ClassCodex32Secret:
		name = "ms1"
	case seal.ClassMnemonic:
		name = "seed words"
	}
	if n > 1 {
		return fmt.Sprintf("%s %d/%d", name, i+1, n)
	}
	return name
}

// unlockSecretSession offers every secret record, in order, before the plate
// list is built.
func unlockSecretSession(ctx *Context, th *Colors, p *seal.Payload) {
	at := make([]int, 0, len(p.Secret))
	for i, r := range p.Secret {
		if seal.IsSecret(r.Class) {
			at = append(at, i)
		}
	}
	for n, i := range at {
		unlockSecretPlate(ctx, th, p, i, unlockSecretLabel(p.Secret[i].Class, n, len(at)))
	}
}

// unlockSecretPlate offers ONE secret plate and wipes it on the way out.
//
// The wipe is a defer registered before anything can return. That is what makes
// "by any route" a property of the code rather than of the author remembering
// every branch -- and the branches are not obvious: EngraveScreen.Engrave has
// two returns but five reachable exits, including a panic unwind.
func unlockSecretPlate(ctx *Context, th *Colors, p *seal.Payload, i int, label string) {
	defer func() {
		p.WipeSecretAt(i)
		if unlockSecretHook != nil {
			unlockSecretHook("wiped", i, p.Secret[i].Record)
		}
	}()
	if unlockSecretHook != nil {
		unlockSecretHook("offered", i, p.Secret[i].Record)
	}
	cs := &ChoiceScreen{
		Title:   label,
		Lead:    "SECRET seed material",
		Choices: []string{"Cut this plate", "Skip"},
	}
	// Back and Skip are the same outcome, and both wipe. There is deliberately
	// no third option: §10.2.2 gives the operator Cut or Skip, and a "later"
	// that kept the record resident is the state this section exists to prevent.
	choice, ok := cs.Choose(ctx, th)
	if !ok || choice != 0 {
		return
	}
	switch p.Secret[i].Class {
	case seal.ClassCodex32Secret:
		unlockEngraveCodex32(ctx, th, p.Secret[i].Record)
	case seal.ClassMnemonic:
		unlockEngraveMnemonic(ctx, th, p.Secret[i].Record)
	}
}

// unlockEngraveCodex32 cuts one ms1 record.
//
// It does NOT reuse engraveCodex32, whose codex32Recover branch waits on
// physical NFC shares that a payload-sourced record does not have -- the same
// dead end that made B1 refuse mdmkFlow (F-76). Nor backupSeedStringFlow, whose
// `for { if Engrave { return } }` loop re-presents a CANCELLED plate: under
// §10.2.2 that record is already being wiped, so the retry would offer to cut
// nothing.
//
// HONEST CAVEAT, and it is the same one gui/ms1_decode.go:19-20 already
// carries: codex32.String holds the share as a Go string, and backup.SeedString
// and the Plate derived from it hold further copies. None can be zeroed. What
// this function guarantees is that seal's OWN buffer is zeroed by the caller's
// defer and the derived copies are dropped; TinyGo's GC decides the rest.
func unlockEngraveCodex32(ctx *Context, th *Colors, rec []byte) {
	s, err := codex32.New(string(rec))
	if err != nil {
		// Unreachable behind §10.2.1's allow-list, which admitted this record
		// via codex32.New in the first place. Named rather than assumed.
		showError(ctx, th, unlockTitle, "This record is not a readable codex32 secret.")
		return
	}
	id, _, _ := s.Split()
	params := ctx.Platform.EngraverParams()
	plan, err := backup.EngraveSeedString(params, backup.SeedString{
		Title: id,
		Seed:  s.String(),
		Font:  constant.Font,
	})
	if err != nil {
		showError(ctx, th, unlockTitle, "This record does not fit any plate size.")
		return
	}
	plate, err := toPlate(plan, params)
	if err != nil {
		showError(ctx, th, unlockTitle, "This record does not fit any plate size.")
		return
	}
	// §10.2.2, and it must be HERE rather than after Engrave returns. The plate
	// carries the geometry: newEngraverJob holds plate.Spline
	// (gui/engraver.go:64) and the engrave loop iterates e.spline (:170), so
	// nothing reads these bytes again. Waiting for Engrave would leave the seed
	// resident for the whole ~21-minute cut -- and Back while running does NOT
	// return (gui/gui.go:2651-2656 calls Stop() and keeps rendering), so the
	// abort-mid-plate path §10.2.2 calls the machine's most ordinary recovery
	// would leave it resident indefinitely and let the operator resume without
	// a fresh unlock.
	clear(rec)
	// ONE engrave, then return regardless of the outcome.
	NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme)
}

// unlockEngraveMnemonic cuts one bare-mnemonic record.
//
// Composed rather than delegated to backupWalletFlow, which prompts for a
// BIP-39 passphrase, offers a fingerprint choice, and loops back to re-Confirm
// on a cancelled engrave. The plate produced here is the one backupWalletFlow's
// Skip-passphrase path produces.
func unlockEngraveMnemonic(ctx *Context, th *Colors, rec []byte) {
	m, err := bip39.Parse(rec)
	if err != nil {
		showError(ctx, th, unlockTitle, "This record is not a readable BIP-39 mnemonic.")
		return
	}
	// bip39.Parse returns a SECOND copy of the secret as []Word. seal's copy is
	// zeroed by the caller's defer; this one is this function's to zero, and
	// clear() reaches []Word where wipeBytes ([]byte) does not compile.
	defer clear(m)
	// NoEdit: the edit nav button (Button2, or a tap on its slot) opens the word
	// editor, and editing an authoritative payload seed produces a
	// self-consistent plate that does not restore the payload's wallet. For a
	// TYPED seed editing a word is a typo fix; for this one it is corruption.
	ss := &SeedScreen{NoEdit: true}
	if !ss.Confirm(ctx, th, m) {
		return
	}
	// The BARE fingerprint. A payload-sourced seed gets no passphrase prompt:
	// §8's twelve words are the payload's passphrase and are NEVER seed
	// entropy, and offering a second, different passphrase here is exactly the
	// confusion §8 says the UI must not create.
	mfp, err := masterFingerprintFor(m, &chaincfg.MainNetParams, "")
	if err != nil {
		showError(ctx, th, unlockTitle, "Couldn't derive the fingerprint for this seed.")
		return
	}
	params := ctx.Platform.EngraverParams()
	plate, err := engraveSeed(params, m, mfp)
	if err != nil {
		showError(ctx, th, unlockTitle, "This seed does not fit any plate size.")
		return
	}
	// §10.2.2 — see unlockEngraveCodex32 for why this is before Engrave and not
	// after. m is zeroed by this function's own defer; rec is seal's buffer.
	clear(rec)
	NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme)
}
