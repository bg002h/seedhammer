package gui

import (
	"errors"
	"fmt"
	"image"
	"log"
	"time"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
)

// §10.2 steps 5-8: twelve words, the checksum gate, the ~31 s KDF with a real
// progress indicator, and the retry loop that keeps the §6.6 hash on screen.

// kdfStepIterations is how much of the derivation runs between two frames.
//
// §7.1 measured 9,715 iterations/sec on RP2350 silicon, so 500 iterations is
// ~51 ms of work per frame -- about 19 fps, which reads as motion, and well
// under the ~250 ms at which a touch starts to feel ignored. At the 300,000
// default that is 600 frames.
const kdfStepIterations = 500

var (
	// errUnlockChecksum is §10.2 step 6: the words are not a valid BIP-39
	// mnemonic, so NO KDF is run. Distinct from a tag failure because the
	// operator's next action differs -- retype, versus suspect the payload.
	errUnlockChecksum = errors.New("unlock: passphrase is not checksum-valid")
	// errUnlockCancelled is the operator leaving, at any point.
	errUnlockCancelled = errors.New("unlock: cancelled")
)

// newDeriver is the KDF seam, and it is NOT optional (R0 round 0, I2).
//
// §11.2 requires "BIP-39 checksum rejection happens without invoking the KDF"
// and §11.3 makes both the "checksum check removed" and "KDF run before the
// checksum gate" mutants mandatory -- each asserted by INSTRUMENTATION, because
// both orders return the identical error and a return-value assertion is a
// guaranteed false PASS over exactly the defect.
//
// The existing instrument (Opener.KDF, counted by countingKDF in
// seal/open_test.go:14-20) is no longer in the path: B2a derives through
// seal.NewDeriver and opens through UnlockWithKey, neither of which consults
// Opener.KDF. This variable is the replacement, in the same in-file style as
// unlockEngraveHook and unlockSecretHook. Production is always seal.NewDeriver;
// a test swaps it and counts.
var newDeriver = seal.NewDeriver

// unlockPassphraseHook fires when the word-entry screen is ENTERED, and exists
// for one required negative: §11.2's "Vector E reaches the plate list with the
// keyboard flow NEVER ENTERED -- asserted by instrumenting the prompt entry
// point, not by return value. A scripted fake platform will happily feed twelve
// words into a prompt that should not exist and still reach the plate list, so
// a return-value assertion reports PASS over exactly the defect." nil in
// production.
var unlockPassphraseHook func()

// unlockPassphraseFlow takes §8's twelve words. It returns ok == false only
// when the operator backs out; a checksum-invalid entry is reported and
// re-prompted here, because that is a typo and not a decision.
//
// It does NOT reuse seedEntryFlow, which opens with a 12/24 word-count picker.
// §8 says the passphrase is twelve words; there is no choice to offer. What it
// does reuse unmodified is inputWordsFlow, whose length is len(mnemonic) and
// whose title parameter is a documented additive seam.
func unlockPassphraseFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	if unlockPassphraseHook != nil {
		unlockPassphraseHook()
	}
	// The screen's identity is established HERE, before entry, and the title
	// passed to inputWordsFlow stays "" (R0 round 0, M4). The title parameter is
	// an either/or: gui/gui.go:765-770 renders `layoutTitlef("Word %d of %d")`
	// when it is empty and `layoutTitle(title)` when it is not. Passing
	// "Passphrase" would REPLACE the only per-word progress on the screen, so
	// the operator would type twelve words with no idea how many remain, on the
	// screen that gates a ~31 s KDF. Empty is also what makes §8's "reuses the
	// existing 12-word seed-entry flow unmodified" literally true, and it keeps
	// the existing `uiContains(content, "Word 1 of")` negative assertion working.
	showNotice(ctx, th, unlockTitle,
		"Enter the 12-word passphrase for this payload.\n\n"+
			"These words are the payload's passphrase. They are NOT a seed and no "+
			"wallet is derived from them.")
	for !ctx.Done {
		m := emptyBIP39Mnemonic(12)
		inputWordsFlow(ctx, th, m, 0, "")
		// inputWordsFlow returns on Back with whatever has been typed, so an
		// incomplete mnemonic is the ordinary shape of "the operator left".
		// Treating it as an error would report a typo they did not make.
		if !isMnemonicComplete(m) {
			clear(m)
			return nil, false
		}
		if !m.Valid() {
			// Structurally hard to reach: LastWordCandidates restricts the
			// final slot to the 128 checksum-valid words. Not impossible, and
			// the cost of being wrong is a 31 s wait ending in a message that
			// blames the payload.
			clear(m)
			showError(ctx, th, unlockTitle, "Not a valid passphrase, check the words.")
			continue
		}
		return m, true
	}
	return nil, false
}

// passphraseBytes builds §8.1's normalised form -- twelve lowercase words,
// single-space separated, no trailing space -- into a buffer the CALLER owns
// and can zero.
//
// Mnemonic.String() produces byte-identical output (measured:
// seal.NormalisePassphrase(m.String()) == m.String()), but it produces a Go
// STRING, which cannot be zeroed. That is the whole reason this exists.
//
// The capacity is fixed so append never regrows: a regrow would leave a stale
// copy of the first half of the passphrase in an orphaned array that nothing
// can reach to wipe. Twelve words of at most eight letters plus eleven
// separators is 107 bytes.
func passphraseBytes(m bip39.Mnemonic) []byte {
	b := make([]byte, 0, 128)
	for i, w := range m {
		if i > 0 {
			b = append(b, ' ')
		}
		start := len(b)
		b = append(b, bip39.LabelFor(w)...)
		// The wordlist is stored uppercase; lowercase in place rather than via
		// bytes.ToLower, which would allocate a temporary per word.
		for j := start; j < len(b); j++ {
			if c := b[j]; c >= 'A' && c <= 'Z' {
				b[j] = c + ('a' - 'A')
			}
		}
	}
	return b
}

// unlockKDFLead is §10.2 step 7's "the screen must say how long it will take".
//
// The estimate is measured from THIS derivation rather than read off §7.1's
// table, so it stays right on a part whose rate differs from the RP2350A the
// table was measured on -- which is precisely the residual caveat §7.1 still
// owes.
func unlockKDFLead(done, total int, elapsed time.Duration) string {
	if done <= 0 || elapsed <= 0 {
		return "Unlocking. This takes about 30 seconds."
	}
	// Multiply BEFORE dividing: `elapsed/done` truncates to whole nanoseconds
	// first, and on a fast host that rounds to 0 and the screen reads "About 0
	// seconds left." int64 overflows only past ~10^10 ns of elapsed time.
	left := time.Duration(int64(elapsed) * int64(total-done) / int64(done))
	return fmt.Sprintf("Unlocking. About %d seconds left.", int(left.Seconds()+0.5))
}

// unlockDerive runs the KDF a slice at a time, drawing a frame between slices.
// It returns the derived key, which the CALLER must zero, or ok == false if the
// operator left.
func unlockDerive(ctx *Context, th *Colors, h seal.Header, pass []byte) ([]byte, bool) {
	d := newDeriver(pass, h.Salt[:], int(h.Iterations))
	// Registered before anything can return. Key() hands back a copy, so this
	// does not zero the result out from under the caller.
	defer d.Wipe()
	backBtn := &Clickable{Button: Button1}
	start := time.Now()
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		if d.Step(kdfStepIterations) {
			// §7.1 still owes an in-situ rate on RP2350B silicon. Logging it
			// here makes the operator's real unlock that measurement, in the
			// real call path, rather than a benchmark's idealised loop. The
			// iteration count travels in the header and is public, so this
			// leaks nothing.
			log.Printf("seal: kdf %d iterations in %s", d.Total(), time.Since(start))
			return d.Key(), true
		}
		dims := ctx.Platform.DisplaySize()
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, unlockTitle)
		pctOp, pctSz := widget.Label(&ctx.B, ctx.Styles.progress, th.Text,
			fmt.Sprintf("%d%%", d.Done()*100/d.Total()))
		leadOp, leadSz := widget.Labelw(&ctx.B, ctx.Styles.lead, dims.X-2*8, th.Text,
			unlockKDFLead(d.Done(), d.Total(), time.Since(start)))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconDiscard},
		}...)
		// BEFORE ctx.Frame, and the order is load-bearing. ctx.Frame IS the
		// yield, and Run reads the deadline for the frame it has just been
		// handed (`wakeup := ctx.Wakeup`, gui/gui.go:2972) BEFORE its own
		// ctx.Reset(). So a WakeupAt placed AFTER Frame governs the NEXT frame,
		// never this one -- and frame 1 then inherits whatever the preceding
		// screen left, which is Run's own ctx.WakeupAt(idleWakeup): the twelfth
		// word's OK release plus THREE MINUTES. The derivation parks at
		// 500/300,000 iterations, the screensaver takes the screen, and
		// ctx.Frame never returns.
		//
		// That is worse than the blocking pbkdf2.Key this design replaced, which
		// at least finished in ~31 s, and it defeats the operator decision this
		// whole screen exists to satisfy. EngraveScreen.Engrave has the correct
		// order (gui/gui.go:2733 before :2741); this now matches it.
		ctx.WakeupAt(time.Now())
		ctx.Frame(op.Layer(
			nav,
			titleOp,
			pctOp.Offset(image.Pt((dims.X-pctSz.X)/2, (dims.Y-pctSz.Y)/2-leadSz.Y)),
			leadOp.Offset(image.Pt((dims.X-leadSz.X)/2, (dims.Y+pctSz.Y)/2)),
			op.Color(&ctx.B, th.Background),
		))
	}
	return nil, false
}

// unlockAttemptOnce is §10.2 steps 6-9 for ONE attempt.
//
// It takes the mnemonic rather than reading it, which is the only way to test
// the checksum gate: LastWordCandidates restricts the keyboard's final slot to
// the 128 checksum-valid words, so a test cannot TYPE `beef` x11 + `bacon`.
// §11.4 requires that case to be rejected with no KDF run, asserted by
// instrumenting the KDF and not by return value.
func unlockAttemptOnce(ctx *Context, th *Colors, blob []byte, p *seal.Payload, m bip39.Mnemonic) error {
	// §10.2 step 6, BEFORE the ~31 s KDF. Both halves matter: Valid() alone is
	// false on a partial mnemonic too, but NormalisePassphrase silently repairs
	// a partial into a well-formed WRONG passphrase, and the operator then
	// waits 31 s for a message that blames the payload.
	if !isMnemonicComplete(m) || !m.Valid() {
		return errUnlockChecksum
	}
	pass := passphraseBytes(m)
	defer clear(pass)
	key, ok := unlockDerive(ctx, th, p.Header, pass)
	if !ok {
		return errUnlockCancelled
	}
	// §10.2 step 10: the derived key is zeroed on every exit path.
	defer clear(key)
	var o seal.Opener
	return o.UnlockWithKey(blob, p, key)
}

// unlockRetryBody is §10.2 step 8's message. It MUST offer both readings and
// keep the §6.6 hash on screen: the tag also authenticates the public section
// (§6.1a), so a tampered public card fails here too -- ~31 s after the hash was
// displayed. Reporting only "wrong passphrase" invites the operator to retype
// three times and conclude the blob is corrupt, losing the one signal §2.2
// item 4 exists to raise.
func unlockRetryBody(p *seal.Payload) string {
	msg := "Wrong passphrase, or this payload has been altered.\n\n"
	if !p.HasHash {
		// pub_len == 0: there is no hash to compare, and inventing one would be
		// the empty-record-set constant -- furniture, per §10.2 step 3.
		return msg + "Check the words and try again."
	}
	return msg + fmt.Sprintf("Public data hash (%d records, %s):\n\n%s\n\n"+
		"Compare this against the value you recorded.",
		len(p.Public), unlockShape(p), seal.FormatHash(p.Hash))
}

// unlockSealedFlow is §10.2 steps 5-9 with the retry loop. It returns true only
// when p.Secret has been populated.
//
// A false return MUST NOT fall through to the plate list: p.Public on a sealed
// payload is a legitimate record set, and engraving it while dropping the
// encrypted half is §6.4's incomplete-backup-believed-complete, the worst
// available outcome. That is the same rule B1's Task 6 enforced with a terminal
// screen, and it does not relax now that unlocking exists.
func unlockSealedFlow(ctx *Context, th *Colors, blob []byte, p *seal.Payload) bool {
	for !ctx.Done {
		m, ok := unlockPassphraseFlow(ctx, th)
		if !ok {
			return false
		}
		err := unlockAttemptOnce(ctx, th, blob, p, m)
		// The mnemonic is []Word, so clear() reaches it; wipeBytes takes []byte
		// and does not compile against it.
		clear(m)
		switch {
		case err == nil:
			return true
		case errors.Is(err, errUnlockCancelled):
			return false
		case errors.Is(err, errUnlockChecksum):
			showError(ctx, th, unlockTitle, "Not a valid passphrase, check the words.")
		case errors.Is(err, seal.ErrAuthentication):
			showError(ctx, th, unlockTitle, unlockRetryBody(p))
		case errors.Is(err, seal.ErrTooManyRecords):
			// §6.4 requires this be distinguishable from "unreadable": the
			// count is authenticated plaintext, so naming it leaks nothing, and
			// conflating a too-large wallet with an attack would send the
			// operator chasing a compromise that did not happen.
			showError(ctx, th, unlockTitle,
				"This payload declares more records than the machine accepts.")
			return false
		default:
			showError(ctx, th, unlockTitle, "Payload unreadable.")
			return false
		}
	}
	return false
}
