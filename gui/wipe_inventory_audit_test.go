package gui

// Wipe-inventory audit (2026-08-09): drive the REAL uiFlow at Run level through
// the REAL §10.2.4 idle wipe (3:00 idle + 30 s warning), holding the live
// backing arrays of every buffer the code claims to zero, and assert ON THE
// BUFFERS that each one is zero once the wipe has completed and the session
// has restarted.
//
// Why this exists when unlock_wipe_test.go and run_reentry_test.go already do:
// the flow-level wipe tests drive unlockPayloadFlow through runUITouch, which
// BYPASSES Run -- so none of them exercises the actual §10.2.4 trigger, the
// unwind through a parked screen, or ctx.B.Scrub() on the abandoned Context.
// The Run-level reentry test exercises all of that but asserts only LIVENESS
// (the restarted flow keeps drawing), not residency. This file is the missing
// intersection: real trigger, real unwind, residency asserted on the arrays.
//
// Two parks, chosen for what is resident at wipe time:
//
//   - vector A parked on SeedScreen.Confirm: the words are on screen, rec is
//     still non-zero in p.Secret, and bip39.Parse's []Word copy `m` is live in
//     a LOCAL whose only wipe on this path is the deferred clear(m) -- the
//     exact F-89 shape: the wipe MUST unwind the flow or that copy survives
//     with every predicate reading clean. No other test drives the wipe
//     through this state.
//   - vector F parked on the first ms1 Cut/Skip: the DEFAULT arm (six of seven
//     vectors carry ms1 secrets), record still non-zero, nothing else built.

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/op"
)

// wipeCaptures holds the live backing arrays the hooks hand out, plus a
// sanity bit per capture proving the instrument saw non-zero content BEFORE
// the wipe -- an all-zero capture that was never non-zero asserts nothing.
type wipeCaptures struct {
	// records[i] is p.Secret[i].Record as handed to unlockSecretHook at
	// "offered" -- the SAME slice WipeSecretAt and Payload.Wipe clear.
	records map[int][]byte
	// recordsNonZero[i] records whether the slice held a non-zero byte when
	// it was captured.
	recordsNonZero map[int]bool
	// passWords is every typed-passphrase buffer unlockPassphraseFlow
	// allocated (one per attempt).
	passWords []bip39.Mnemonic
	// keys is every derived key unlockAttemptOnce took ownership of.
	keys        [][]byte
	keysNonZero []bool
	// parsed is bip39.Parse's []Word copy inside unlockEngraveMnemonic,
	// captured the instant its deferred clear(m) was registered.
	parsed        []bip39.Mnemonic
	parsedNonZero []bool
}

func installWipeCaptureHooks(t *testing.T) *wipeCaptures {
	t.Helper()
	c := &wipeCaptures{
		records:        make(map[int][]byte),
		recordsNonZero: make(map[int]bool),
	}
	unlockSecretHook = func(stage string, idx int, rec []byte) {
		if stage != "offered" {
			return
		}
		if _, seen := c.records[idx]; seen {
			return
		}
		c.records[idx] = rec
		c.recordsNonZero[idx] = !allZeroBytes(rec)
	}
	unlockPassphraseWordsHook = func(m bip39.Mnemonic) {
		c.passWords = append(c.passWords, m)
	}
	unlockKeyHook = func(key []byte) {
		c.keys = append(c.keys, key)
		c.keysNonZero = append(c.keysNonZero, !allZeroBytes(key))
	}
	unlockMnemonicParsedHook = func(m bip39.Mnemonic) {
		c.parsed = append(c.parsed, m)
		c.parsedNonZero = append(c.parsedNonZero, !allZeroWords(m))
	}
	t.Cleanup(func() {
		unlockSecretHook = nil
		unlockPassphraseWordsHook = nil
		unlockKeyHook = nil
		unlockMnemonicParsedHook = nil
	})
	return c
}

// allZeroBytes and allZeroWords already exist in this package's test files
// (unlock_session_test.go:27, unlock_wipe_test.go:37) and are reused here.

// driveWipeResidency runs the whole scenario: unlock `vector`, walk to the
// park via the steps `parkFor` builds against the live driver, let the REAL
// §10.2.4 timer warn and wipe, and return the captures plus the abandoned
// first session's Context.
func driveWipeResidency(t *testing.T, vector string, parkFor func(drv *reentryDriver) []reentryStep) (*wipeCaptures, *Context) {
	t.Helper()
	c := installWipeCaptureHooks(t)
	var firstCtx *Context

	synctest.Test(t, func(t *testing.T) {
		p := newHitPlatform()
		// 1 s floor: idleTimeout is crossed in 180 ticks instead of 18,000.
		p.tickFloor = time.Second
		v := sealVector(t, vector)
		p.payload = payloadReaderFrom(t, v.blob(t))

		drv := &reentryDriver{t: t, p: p, dims: sh2DisplaySize, reentryTap: -1}

		// Session 1: carousel to Sealed Payload, unlock, walk to the park.
		drv.addCarouselSteps("s1", func() { drv.tapNav(Button3) })
		if v.PubLen > 0 {
			drv.steps = append(drv.steps,
				reentryStep{"s1: dismiss the hash notice", "Public data hash", func() { drv.tapNav(Button3) }})
		}
		drv.steps = append(drv.steps,
			reentryStep{"s1: dismiss the passphrase notice", "Enter the 12-word passphrase", func() { drv.tapNav(Button3) }})
		for i, w := range vectorPassphrase(t, v) {
			title := fmt.Sprintf("Word %d of 12", i+1)
			for _, r := range strings.ToLower(w) {
				drv.steps = append(drv.steps, reentryStep{
					name: fmt.Sprintf("s1: word %d key %q", i+1, string(r)),
					wait: title,
					act:  func() { drv.tapKey(r) },
				})
			}
			drv.steps = append(drv.steps, reentryStep{
				name: fmt.Sprintf("s1: word %d OK", i+1),
				wait: title,
				act:  func() { drv.tapNav(Button3) },
			})
		}
		drv.steps = append(drv.steps, parkFor(drv)...)
		// The REAL §10.2.4 sequence: the warning must be observed before the
		// wipe, so a forced-wipe shortcut cannot silently substitute.
		drv.steps = append(drv.steps,
			reentryStep{"s1: §10.2.4 warning observed", "erased in", func() {}})

		sessions := 0
		flow := func(ctx *Context, version string) {
			sessions++
			if sessions > 1 {
				return // session 2: end the run; the wipe already happened
			}
			firstCtx = ctx
			uiFlow(ctx, version)
		}

		r := image.Rectangle{Max: sh2DisplaySize}
		observe := func(o op.Op) {
			d := new(op.Drawer)
			txt := d.ExtractText(r, o)
			drv.d, drv.text = d, txt
			if drv.dead || drv.step >= len(drv.steps) {
				return
			}
			// Never act while queued events are still undelivered (see
			// run_reentry_test.go's observe).
			if len(p.queued) > 0 {
				return
			}
			if st := drv.steps[drv.step]; uiContains(txt, st.wait) {
				st.act()
				drv.step++
			}
		}
		ticks := 0
		for range runWithFlow(p, "test", flow, observe) {
			ticks++
			if ticks > reentryTickCap {
				break
			}
		}
		if drv.step != len(drv.steps) {
			t.Fatalf("driver stalled at step %q (%d/%d) after %d ticks -- the wipe was never "+
				"reached, so the residency assertions would prove nothing",
				drv.steps[drv.step].name, drv.step, len(drv.steps), ticks)
		}
		if sessions < 2 {
			t.Fatalf("the wipe never restarted the session (sessions=%d)", sessions)
		}
	})
	return c, firstCtx
}

// TestWipeZeroesEveryPinnedBufferAtRunLevel is the audit's fidelity
// measurement: after the REAL idle wipe, every buffer the code claims to zero
// is asserted zero ON ITS BACKING ARRAY, and the abandoned Context's frame
// buffer is asserted residue-free with real seed words having been rendered
// into it.
func TestWipeZeroesEveryPinnedBufferAtRunLevel(t *testing.T) {
	t.Run("vectorA-parked-on-seed-screen", func(t *testing.T) {
		c, firstCtx := driveWipeResidency(t, "A", func(drv *reentryDriver) []reentryStep {
			return []reentryStep{
				// "seed words" ChoiceScreen: choice 0 ("Cut this plate") is
				// the default; the checkmark takes it.
				{"s1: Cut on the secret offer", "SECRET seed material", func() { drv.tapNav(Button3) }},
				// SeedScreen.Confirm draws the words; park here with the seed
				// on screen and bip39.Parse's copy live in a local.
				{"s1: parked on the seed words", "BACON", func() {}},
			}
		})

		if len(c.records) == 0 {
			t.Fatal("unlockSecretHook never fired; nothing was measured")
		}
		for i, rec := range c.records {
			if !c.recordsNonZero[i] {
				t.Errorf("record %d was already zero when offered; instrument broken", i)
			}
			if !allZeroBytes(rec) {
				t.Errorf("record %d still holds non-zero bytes after the §10.2.4 wipe", i)
			}
		}
		if len(c.parsed) == 0 {
			t.Fatal("unlockMnemonicParsedHook never fired; the park never reached unlockEngraveMnemonic")
		}
		for i, m := range c.parsed {
			if !c.parsedNonZero[i] {
				t.Errorf("parsed mnemonic %d was already zero at capture; instrument broken", i)
			}
			if !allZeroWords(m) {
				t.Errorf("bip39.Parse's []Word copy %d SURVIVED the wipe -- the unwind did not run "+
					"unlockEngraveMnemonic's deferred clear(m) (F-89's exact shape)", i)
			}
		}
		assertCommonWipeResidency(t, c, firstCtx)
	})

	t.Run("vectorF-parked-on-ms1-cutskip", func(t *testing.T) {
		c, firstCtx := driveWipeResidency(t, "F", func(drv *reentryDriver) []reentryStep {
			return []reentryStep{
				// Park on the FIRST ms1 offer; the record is live in p.Secret.
				{"s1: parked on the ms1 offer", "SECRET seed material", func() {}},
			}
		})

		// Vector F has THREE ms1 secrets. Only the first is offered before
		// the park; the other two are offered during the unwind sweep (their
		// hooks fire as unlockSecretPlate enters and their deferred wipes run
		// immediately), so all three arrive here.
		if got := len(c.records); got != 3 {
			t.Fatalf("captured %d secret records, want vector F's 3 ms1", got)
		}
		for i, rec := range c.records {
			if !c.recordsNonZero[i] {
				t.Errorf("ms1 record %d was already zero when offered; instrument broken", i)
			}
			if !allZeroBytes(rec) {
				t.Errorf("ms1 record %d still holds non-zero bytes after the §10.2.4 wipe", i)
			}
		}
		assertCommonWipeResidency(t, c, firstCtx)
	})
}

// assertCommonWipeResidency asserts the vector-independent half: the typed
// passphrase, the derived key, and the abandoned frame buffer.
func assertCommonWipeResidency(t *testing.T, c *wipeCaptures, firstCtx *Context) {
	t.Helper()
	if len(c.passWords) == 0 {
		t.Fatal("unlockPassphraseWordsHook never fired; the passphrase was never typed")
	}
	for i, m := range c.passWords {
		// The buffer starts as -1 sentinels; after typing it holds words and
		// after the attempt it must be all zero. A buffer still holding -1 or
		// words means either the script never typed or a wipe was missed.
		if !allZeroWords(m) {
			t.Errorf("typed-passphrase buffer %d still holds non-zero words after the wipe", i)
		}
	}
	if len(c.keys) == 0 {
		t.Fatal("unlockKeyHook never fired; no key was derived")
	}
	for i, k := range c.keys {
		if !c.keysNonZero[i] {
			t.Errorf("derived key %d was already zero at capture; instrument broken", i)
		}
		if !allZeroBytes(k) {
			t.Errorf("derived key %d still holds non-zero bytes after the wipe", i)
		}
	}
	if firstCtx == nil {
		t.Fatal("the first session's Context was never captured")
	}
	args, refs := firstCtx.B.Residue()
	if args != 0 || refs != 0 {
		t.Errorf("the abandoned session's frame buffer holds residue after the wipe: "+
			"%d non-zero args, %d non-nil refs -- the park screen's content is recoverable "+
			"from the backing array", args, refs)
	}
}
