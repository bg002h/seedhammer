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

// unlockPassphraseWordsHook hands over the []Word buffer the operator types
// into, at the moment it is ALLOCATED, so a test holds the same backing array
// through every exit and can read it afterwards.
//
// §11.2 requires the passphrase buffer to be asserted zeroed ON THE BUFFER
// ITSELF, and this buffer is otherwise unreachable: the flow returns nil on the
// partial-entry exit, so there is no return value to read. One seam covers both
// wipes, because unlockPassphraseFlow returns this exact slice on success -- so
// it is also the `m` unlockSealedFlow zeroes after each attempt. nil in
// production; same in-file style as unlockSecretHook and unlockMnemonicHook.
var unlockPassphraseWordsHook func(m bip39.Mnemonic)

// unlockKeyHook hands over the DERIVED KEY at the moment unlockAttemptOnce takes
// ownership of it, for the same reason and under the same §11.2 clause.
//
// The newDeriver seam cannot reach this array: unlockDerive returns d.Key(),
// which is a FRESH copy (seal/pbkdf2.go's Key), so the Deriver a test holds and
// the buffer the flow zeroes are different allocations. nil in production.
var unlockKeyHook func(key []byte)

// unlockPassphraseNotice is §10.2 step 5's "these words are not a seed", split
// out from unlockPassphraseFlow so unlockSealedFlow can show it ONCE per
// session rather than once per attempt.
//
// unlockSealedFlow calls unlockPassphraseFlow afresh for every retry, so with
// the notice inside it a wrong passphrase cost the operator two dismissals --
// the §10.2 step 8 error carrying the §6.6 hash, and then this -- before the
// keyboard came back. It also put a screen BETWEEN the hash and the retry, on
// the one path where the hash is the signal the operator is meant to act on.
func unlockPassphraseNotice(ctx *Context, th *Colors) {
	showNotice(ctx, th, unlockTitle,
		"Enter the 12-word passphrase for this payload.\n\n"+
			"These words are the payload's passphrase. They are NOT a seed and no "+
			"wallet is derived from them.")
}

// unlockPassphraseFlow takes §8's twelve words. It returns ok == false only
// when the operator backs out; a checksum-invalid entry is reported and
// re-prompted here, because that is a typo and not a decision.
//
// It does NOT reuse seedEntryFlow, which opens with a 12/24 word-count picker.
// §8 says the passphrase is twelve words; there is no choice to offer. What it
// does reuse unmodified is inputWordsFlow, whose length is len(mnemonic) and
// whose title parameter is a documented additive seam.
//
// The notice is the CALLER's (unlockSealedFlow), once per session -- see
// unlockPassphraseNotice.
func unlockPassphraseFlow(ctx *Context, th *Colors) (bip39.Mnemonic, bool) {
	if unlockPassphraseHook != nil {
		unlockPassphraseHook()
	}
	// The screen's identity is established by unlockPassphraseNotice, before
	// entry, and the title passed to inputWordsFlow stays "" (R0 round 0, M4).
	// The title parameter is an either/or: gui/gui.go:765-770 renders
	// `layoutTitlef("Word %d of %d")` when it is empty and `layoutTitle(title)`
	// when it is not. Passing "Passphrase" would REPLACE the only per-word
	// progress on the screen, so the operator would type twelve words with no
	// idea how many remain, on the screen that gates a ~31 s KDF. Empty is also
	// what makes §8's "reuses the existing 12-word seed-entry flow unmodified"
	// literally true, and it keeps the existing
	// `uiContains(content, "Word 1 of")` negative assertion working.
	for !ctx.Done {
		m := emptyBIP39Mnemonic(12)
		if unlockPassphraseWordsHook != nil {
			unlockPassphraseWordsHook(m)
		}
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
	// seconds left."
	//
	// The product cannot overflow int64. `total-done` is bounded by §6.2's
	// ceiling at 1,999,999, so overflow needs
	// elapsed > (2^63-1)/1_999_999 = 4,611,688,324,271 ns -- 4611.7 s, about
	// 76.9 minutes of wall time on a screen the operator is watching. (An
	// earlier revision of this comment said "~10^10 ns", which is 10 seconds
	// and understates the headroom by ~461x; the figure above is computed, per
	// this project's rule that numbers in comments are measured and not
	// estimated.)
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
	// derived accumulates the time spent INSIDE d.Step and nothing else.
	//
	// §7.1 is closed by Task 9.3 reading the log line below off the real
	// machine and re-deriving the iteration count from it, so the line must
	// report the DERIVATION, not the wall clock. Wall time here also covers
	// ~600 full-panel repaints, and -- the part that actually bites -- any
	// interval in which Run's screensaver has parked this loop, which is
	// unbounded (see the frame-deadline note below and F-93). §7.1's own
	// history is a rate estimate wrong by 1.54x that set the default to
	// 450,000; a parked wall-clock reading would repeat that error larger, and
	// this time with the number "measured on the real part".
	//
	// Both are logged: the rate comes from `derived`, and `wall` is what §7.1
	// separately asks for -- "the number the operator actually experiences".
	var derived time.Duration
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		stepStart := time.Now()
		done := d.Step(kdfStepIterations)
		derived += time.Since(stepStart)
		if done {
			// §7.1 still owes an in-situ rate on RP2350B silicon. Logging it
			// here makes the operator's real unlock that measurement, in the
			// real call path, rather than a benchmark's idealised loop. The
			// iteration count travels in the header and is public, so this
			// leaks nothing.
			log.Printf("seal: kdf %d iterations in %s derived (%s wall)",
				d.Total(), derived, time.Since(start))
			return d.Key(), true
		}
		dims := ctx.Platform.DisplaySize()
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, unlockTitle)
		// d.Done()*100 is computed in int, which is 32 bits on this target, and
		// it is safe only because of seal.MaxIterations. Done() <= Total() <=
		// 2,000,000, so the product peaks at 200,000,000 against int32's
		// 2,147,483,647 -- 10.7x of headroom, but headroom that is a silent
		// consequence of a constant in ANOTHER package. The largest
		// seal.MaxIterations this expression tolerates is 21,474,836
		// (x100 = 2,147,483,600); at 21,474,837 it wraps and the screen renders
		// a NEGATIVE percentage during the one operation §10.2 step 7 exists to
		// keep legible. Pinned at the bound by
		// TestUnlockDerivesAtTheMaximumIterationCount. If MaxIterations is ever
		// raised, widen this to int64 rather than re-deriving the margin.
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
		//
		// WHAT THIS ORDER DOES NOT FIX, so nobody reads it as fixed: Run refreshes
		// a.idle.start only on `len(evts) > 0`, and a derivation produces no
		// events, so a derivation longer than idleTimeout still trips the saver --
		// whose branch `continue`s without breaking, so ctx.Frame does not return
		// and the KDF stops until a touch. At §7.1's measured 9,715 it/s that is
		// iterations >= 180 * 9,715 = 1,748,700, against §6.2's ceiling of
		// 2,000,000 (205.9 s): the top 13.2% of the LEGAL range, reachable with a
		// conforming blob. The default is 300,000 (30.9 s), nowhere near it.
		// Closing it needs a Run-side change and must be reconciled with §10.2.4's
		// residency timer, which is F-89's territory -- so it is B2b's, filed as
		// F-93. The log line above is the half that could be fixed here.
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
	if unlockKeyHook != nil {
		unlockKeyHook(key)
	}
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
	// ONCE per session, above the retry loop. Inside it the operator dismissed
	// this notice again after every wrong passphrase, with the §6.6 hash they
	// are meant to be comparing pushed one screen further back each time.
	unlockPassphraseNotice(ctx, th)
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
