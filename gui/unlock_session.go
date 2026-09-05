package gui

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/font/constant"
	"seedhammer.com/gui/assets"
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

// unlockMnemonicHook observes bip39.Parse's []Word copy at the moment the plate
// is handed to Engrave. It exists because that copy is a local: no test could
// reach it, which is why a seed sat live through the whole cut with the suite
// green. Asserting on the BUFFER at the instant the screen comes up is the only
// assertion that distinguishes an early wipe from a deferred one. nil in
// production.
var unlockMnemonicHook func(m bip39.Mnemonic)

// unlockSecretLabel names a secret plate by its CLASSIFIED type and its index
// WITHIN THAT CLASS -- never across classes, never by anything the sealer
// asserted, and never by rendering the record's contents.
//
// (The pass-3 fold prepended this paragraph without removing the one it
// replaced, so the godoc carried "its index among secrets" -- the exact
// behaviour the fold removed as a defect -- as the sentence `go doc` shows
// FIRST. Deleted; a stale record beside a fixed behaviour is how the defect
// comes back.)
//
// i and n count secrets of THIS class only. Numbering across all secrets while
// naming per class renders "ms1 1/2" then "seed words 2/2" on a mixed payload,
// which tells the operator there are two ms1 cards and they hold the second.
// No canonical vector mixes the classes (A 0/1, B 0/1, C 0/6, D 5/1, E 5/0,
// F 0/15, G 12/3 — all-ms1 or all-mnemonic), so nothing caught it.
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
	// prev, not nil, on the way out -- see unlockPassphraseFlow's own bracket
	// (§10.2.4 row 4, task9-r0.md M2): these two guards never nest today (this
	// function always runs after unlockPassphraseFlow has already returned and
	// uninstalled its own), and save-and-restore makes that a structural fact
	// rather than an accident of call order.
	prev := ctx.wipe
	g := &wipeGuard{}
	ctx.wipe = g
	defer func() {
		ctx.wipe = prev
		// F-107: the seed is RENDERED here, and op.Glyph encodes every drawn rune
		// into ctx.B.args as a uint32. Buffer.Reset (per frame) only TRUNCATES
		// args, so without this the twelve words come back verbatim and in order
		// from the backing array on a NORMAL exit -- read your words, press back.
		//
		// Only the §10.2.4 wipe scrubbed before this (gui/run_flow.go:245), and
		// the wipe is the RARE path: on hardware runWithFlow never returns, so a
		// "normal exit" is the flow walking back to the start screen with the
		// SAME Context and the same Buffer.
		//
		// Safe here: the defer runs strictly between frames, so no op still to be
		// drawn is zeroed (R0 round 0 §(a)).
		ctx.B.Scrub()
	}()
	at := make([]int, 0, len(p.Secret))
	for i, r := range p.Secret {
		if seal.IsSecret(r.Class) {
			at = append(at, i)
		}
	}
	// Count PER CLASS, not across all secrets — see unlockSecretLabel. total is
	// how many of each class exist; seen is how many have been offered so far.
	total := make(map[seal.Classification]int, 2)
	for _, i := range at {
		total[p.Secret[i].Class]++
	}
	seen := make(map[seal.Classification]int, 2)
	for _, i := range at {
		c := p.Secret[i].Class
		unlockSecretPlate(ctx, th, p, i, unlockSecretLabel(c, seen[c], total[c]))
		seen[c]++
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
		// Guarded on the same bound WipeSecretAt fails closed on, three lines up:
		// an out-of-range index is a no-op there and a panic here, and a panic on
		// a watchdog-less device is a brick.
		if unlockSecretHook != nil && i >= 0 && i < len(p.Secret) {
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
		// IconDiscard, not the default IconBack. Back here is NOT "step back one
		// screen": it takes the Skip path and the deferred WipeSecretAt then
		// destroys a decrypted SEED record, recoverable only with twelve words
		// and a ~31 s KDF. unlockPlateListFlow made this same call for a
		// strictly smaller loss -- a payload rather than a seed -- and these two
		// screens are one tap apart.
		BackIcon: assets.IconDiscard,
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
// It does NOT reuse engraveCodex32, whose codex32Recover branch prompts for
// the REST of a share set -- shares a payload-sourced record does not carry,
// the same dead end that made B1 refuse mdmkFlow (F-76). (Those shares are
// typed on the codex32 keypad, via inputCodex32Flow; there is no NFC on that
// path. This comment said "physical NFC shares" until the post-implementation
// review measured it -- M-3.) Nor backupSeedStringFlow, whose
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
	if codex32.IsPreimage(s) {
		// Unreachable behind seal.Classify's H0 guard, which never admits a
		// preimage plate as ClassCodex32Secret. Named rather than assumed:
		// this is the one call on the sealed path that cuts metal.
		showError(ctx, th, unlockTitle, "This record is a hashlock preimage, not a seed. It is not engraved as one.")
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
	// nothing reads these bytes again. (NewEngraveScreen also reads plate.id,
	// which is an opaque number and is 0 on this path -- see Plate.id. The
	// sentence above is about rec's BYTES, and stays true.) Waiting for Engrave
	// would leave the seed
	// resident for the whole ~21-minute cut -- and Back while running does NOT
	// return (gui/gui.go:2651-2656 calls Stop() and keeps rendering), so the
	// abort-mid-plate path §10.2.2 calls the machine's most ordinary recovery
	// would leave it resident indefinitely and let the operator resume without
	// a fresh unlock.
	clear(rec)
	// ONE engrave, then return regardless of the outcome.
	scr := NewEngraveScreen(ctx, plate)
	if g := ctx.wipe; g != nil {
		g.job = scr.job
		defer func() { g.job = nil }()
	}
	scr.Engrave(ctx, &engraveTheme)
}

// unlockEngraveMnemonic cuts one bare-mnemonic record.
//
// Composed rather than delegated to backupWalletFlow, which prompts for a
// BIP-39 passphrase, offers a fingerprint choice, and loops back to re-Confirm
// on a cancelled engrave. The plate produced here is the one backupWalletFlow's
// Skip-passphrase path produces.
//
// HONEST CAVEAT — the seed-equivalent copies this path makes. This list has been
// wrong TWICE: it first named only `m`, then claimed to be the "full inventory"
// and omitted four more (B2a-ii lens 1, I1 then D1). It is therefore written as
// what is KNOWN, not as exhaustive — if you add a derivation here, add a row.
//
//	ZEROED  rec         seal's []byte, cleared before Engrave (see below)
//	ZEROED  m           bip39.Parse's []Word, cleared beside it
//	ZEROED  the 64-byte BIP-39 seed, in deriveMasterKey
//	ZEROED  the BIP-32 master private key, in masterFingerprintFor and at the
//	        seed-entry validity probe
//	LIVE    sentence []byte inside bip39.MnemonicSeed — the PLAINTEXT MNEMONIC,
//	        never wiped, plus whatever append() reallocation orphans. Not
//	        reachable from this package. F-88.
//	LIVE    the []byte behind string(seedqr.QR(m)), and qr.Code.Bitmap. F-88.
//	LIVE    engraveSeed's words []string. The STRINGS are not secret — LabelFor
//	        returns substrings of the public wordlist — but their SELECTION and
//	        ORDER are the seed, and clear(words) would destroy that for free.
//	        F-88.
//	LIVE    plate.Spline, for the duration of the cut. It IS the seed rendered
//	        as geometry and must exist while the needle moves. F-83, accepted.
//
// The ZEROED rows are defence in depth with the usual TinyGo caveat. `m`,
// `rec`, the 64-byte seed and the BIP-32 master key are ALL pinned by tests as
// of the post-merge polish phase (F-94 and F-87, closed 2026-08-11).
//
// This comment used to say the seed and master key were NOT pinned, "and the
// reason is scheduling, not impossibility" — itself a correction of a still
// earlier version claiming they "cannot be without unsafe". Both readings are
// now moot: `deriveSeedHook` and `deriveMasterKeyHook` (`gui/gui.go:257,266`)
// are exactly the ordinary package vars that precedent predicted, and
// `gui/master_key_residue_test.go` reads the same backing arrays after the
// wipe to prove it.
//
// The history is kept rather than tidied away, because it is the lesson. A
// comment licensing a coverage gap on seed-equivalent material outlived the gap
// and overstated its obstacle twice before anyone measured it.
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
	// F-87: fires immediately after the defer above is registered, so a test
	// holds the SAME backing array the defer will zero and can assert it on
	// every early return below -- not just the success path unlockMnemonicHook
	// already covers. See unlock_mnemonic_seam.go.
	if unlockMnemonicParsedHook != nil {
		unlockMnemonicParsedHook(m)
	}
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
	// after. BOTH copies go here, and that is the whole point: rec is seal's
	// buffer, and m is bip39.Parse's INDEPENDENT []Word copy of the same seed.
	//
	// An earlier version zeroed m only on the defer below, which fires when this
	// function RETURNS -- i.e. after Engrave. That left a full copy of the seed
	// live for the entire ~21-minute cut and indefinitely on the paused or failed
	// engrave screen, which is precisely the residency the lines above exist to
	// remove. Worse, nothing else could reach it: §10.2.4's RecordsResident()
	// scans p.Secret only, and p.Wipe() loops p.Secret and p.Public -- neither
	// reaches a local -- so the timer condition reads FALSE while the seed is
	// live. Found by the B2a-ii whole-diff review, lens 1.
	//
	// The defer stays: it covers the three early returns above, where no plate was
	// ever built. clear is idempotent, so the double-zero is free.
	clear(rec)
	clear(m)
	if unlockMnemonicHook != nil {
		unlockMnemonicHook(m)
	}
	scr := NewEngraveScreen(ctx, plate)
	if g := ctx.wipe; g != nil {
		g.job = scr.job
		defer func() { g.job = nil }()
	}
	scr.Engrave(ctx, &engraveTheme)
}
