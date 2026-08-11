package gui

import (
	"errors"
	"fmt"
	"image"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
)

// §10.2 steps 5-9 at the UI: twelve words, the checksum gate BEFORE the ~31 s
// KDF, the chunked derivation with a real progress indicator, and the retry
// loop that keeps the §6.6 hash on screen.
//
// Every test here drives the flow by PointerEvent at sh2DisplaySize. The SH2
// has no directional buttons and emits only gui.PointerEvent
// (cmd/controller/platform_sh2.go:413), and a 240x240 default pushes keyboard
// keys off-canvas -- so a button-driven test of a screen with a keyboard proves
// nothing about the machine.

// ---------------------------------------------------------------------------
// instruments
// ---------------------------------------------------------------------------

// kdfCounter replaces the newDeriver seam and counts derivations.
//
// It is NOT optional (R0 round 0, I2). §11.2 requires "BIP-39 checksum
// rejection happens without invoking the KDF", and §11.3 makes both the
// "checksum check removed" and "KDF run before the checksum gate" mutants
// mandatory -- each asserted by INSTRUMENTATION, because both orders return the
// identical error and a return-value assertion is a guaranteed false PASS over
// exactly the defect.
type kdfCounter struct {
	calls      int
	iterations []int
}

func installKDFCounter(t *testing.T) *kdfCounter {
	t.Helper()
	c := &kdfCounter{}
	prev := newDeriver
	newDeriver = func(passphrase, salt []byte, iterations int) *seal.Deriver {
		c.calls++
		c.iterations = append(c.iterations, iterations)
		return seal.NewDeriver(passphrase, salt, iterations)
	}
	t.Cleanup(func() { newDeriver = prev })
	return c
}

// mnemonicOf builds a Mnemonic from words that need not be checksum-valid --
// which is the whole point, since the discriminating case for the gate is one
// the keyboard cannot produce.
func mnemonicOf(t *testing.T, words ...string) bip39.Mnemonic {
	t.Helper()
	m := make(bip39.Mnemonic, len(words))
	for i, w := range words {
		up := strings.ToUpper(w)
		c, ok := bip39.ClosestWord(up)
		if !ok || bip39.LabelFor(c) != up {
			t.Fatalf("%q is not a BIP-39 word", w)
		}
		m[i] = c
	}
	return m
}

// vectorPassphrase reads a vector's twelve words from the vector FILE.
// seal/testdata/README.md:19-24 forbids retyping them into a Go test.
func vectorPassphrase(t *testing.T, v sealTestVector) []string {
	t.Helper()
	if v.Passphrase == nil {
		t.Fatalf("vector %s carries no passphrase", v.Name)
	}
	w := strings.Fields(*v.Passphrase)
	if len(w) != 12 {
		t.Fatalf("vector %s's passphrase is %d words; §8 says twelve", v.Name, len(w))
	}
	return w
}

// runUnlockAttempt drives ONE unlockAttemptOnce to completion under a frame
// pump and returns the error it produced. The pump is generous because
// unlockDerive draws one frame per kdfStepIterations: a 100,000-iteration
// derivation is 200 Step calls and 199 drawn frames, so the house
// pumpUntil(..., 32) idiom would look like a hang.
func runUnlockAttempt(t *testing.T, blob []byte, p *seal.Payload, m bip39.Mnemonic) error {
	t.Helper()
	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	var err error
	frame, _, quit := runUITouch(ctx, func() {
		err = unlockAttemptOnce(ctx, &descriptorTheme, blob, p, m)
	})
	defer quit()
	const budget = 1024
	for i := 0; i < budget; i++ {
		if _, ok := frame(); !ok {
			return err
		}
	}
	t.Fatalf("unlockAttemptOnce drew %d frames without returning", budget)
	return nil
}

// inspected runs §10.2 steps 1-3 headlessly, which is the state the flow is in
// when it asks for words.
func inspected(t *testing.T, blob []byte) *seal.Payload {
	t.Helper()
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// the touch harness
// ---------------------------------------------------------------------------

// unlockHarness is runUnlock's touch-driven sibling. ppHarness is the closest
// existing shape but it hooks passphraseWidgetHook, which the WORD keyboard
// does not use, so this is a clone rather than a bend.
type unlockHarness struct {
	t       *testing.T
	ctx     *Context
	frame   func() (string, bool)
	drawer  func() *op.Drawer
	done    *bool
	content string
	keys    map[rune]image.Point
	// quit stops the coroutine, which makes yield return false and so sets
	// ctx.Done -- the §11.2 exit no test drove before. It is also registered
	// with t.Cleanup, and iter.Pull's stop is idempotent.
	quit func()
}

func newUnlockHarness(t *testing.T, r seal.Reader) *unlockHarness {
	t.Helper()
	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	returned := false
	h := &unlockHarness{t: t, ctx: ctx, done: &returned}
	frame, drawer, quit := runUITouch(ctx, func() {
		unlockPayloadFlow(ctx, &descriptorTheme, r)
		returned = true
	})
	h.frame, h.drawer, h.quit = frame, drawer, quit
	t.Cleanup(quit)
	return h
}

// next pumps one frame and records it.
func (h *unlockHarness) next(what string) string {
	h.t.Helper()
	c, ok := h.frame()
	if !ok {
		h.t.Fatalf("the flow produced no frame (%s); last frame %q", what, h.content)
	}
	h.content = c
	return c
}

// pump advances up to n frames, stopping as soon as want appears. It sends NO
// input: a test that expects a screen to CHANGE must drive it.
func (h *unlockHarness) pump(n int, want string) bool {
	h.t.Helper()
	for i := 0; i < n; i++ {
		c, ok := h.frame()
		if !ok {
			return false
		}
		h.content = c
		if uiContains(c, want) {
			return true
		}
	}
	return false
}

// mustReach pumps until want appears. The budget is 512 rather than the house
// 32 because a real derivation is 199 frames on its own.
func (h *unlockHarness) mustReach(want string) string {
	h.t.Helper()
	if !h.pump(512, want) {
		h.t.Fatalf("never reached %q; last frame %q", want, h.content)
	}
	return h.content
}

// tapNav taps a nav slot by button, asserting a target bound to that button is
// really drawn there.
func (h *unlockHarness) tapNav(b Button) {
	h.t.Helper()
	tapNavSlot(h.t, h.ctx, h.drawer(), b)
}

// keyPoints locates every key of the word keyboard by HIT-TESTING the frame
// that was actually drawn.
//
// Nothing here duplicates Keyboard's layout arithmetic. The targets come out of
// the drawn frame, are grouped into rows by their own drawn Y and ordered left
// to right, so the only assumption is wordKeys' own three literal rows -- and
// the row lengths are asserted, which IS the "the keys are on canvas" property
// that sh2DisplaySize exists to prove. Every key carries an op.Input whether or
// not it is currently valid (gui/gui.go:1355), so no row ever has a gap.
func (h *unlockHarness) keyPoints() map[rune]image.Point {
	h.t.Helper()
	if h.keys != nil {
		return h.keys
	}
	dims := h.ctx.Platform.DisplaySize()
	d := h.drawer()
	type target struct {
		tag  op.Tag
		rect image.Rectangle
	}
	var found []target
	seen := make(map[op.Tag]bool)
	for y := 0; y < dims.Y; y += 2 {
		for x := 0; x < dims.X; x += 2 {
			tag, rect, ok := d.Hit(image.Pt(x, y))
			if !ok {
				continue
			}
			// The three nav slots carry Button1..Button3; a keyboard key's
			// Clickable is the zero value, so its Button is None. Same
			// discriminator plateHitPoints uses.
			c, isClk := tag.(*Clickable)
			if !isClk || c.Button != None {
				continue
			}
			if seen[tag] {
				continue
			}
			seen[tag] = true
			found = append(found, target{tag, rect})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].rect.Min.Y != found[j].rect.Min.Y {
			return found[i].rect.Min.Y < found[j].rect.Min.Y
		}
		return found[i].rect.Min.X < found[j].rect.Min.X
	})
	var rows [][]target
	for _, f := range found {
		if n := len(rows); n > 0 && rows[n-1][0].rect.Min.Y == f.rect.Min.Y {
			rows[n-1] = append(rows[n-1], f)
			continue
		}
		rows = append(rows, []target{f})
	}
	// NewKeyboard appends the backspace key to the last row (gui/gui.go:985).
	want := strings.Split(wordKeys, "\n")
	want[len(want)-1] += "⌫"
	if len(rows) != len(want) {
		h.t.Fatalf("the word keyboard drew %d rows of touch targets, want %d; last frame %q",
			len(rows), len(want), h.content)
	}
	screen := image.Rectangle{Max: dims}
	out := make(map[rune]image.Point)
	for i, rr := range want {
		rs := []rune(rr)
		if len(rows[i]) != len(rs) {
			h.t.Fatalf("keyboard row %d drew %d touch targets, want %d (%q) -- keys are "+
				"undrawn or off the %v panel", i, len(rows[i]), len(rs), rr, dims)
		}
		for j, r := range rs {
			rect := rows[i][j].rect
			c := rect.Min.Add(rect.Max).Div(2)
			if !c.In(screen) {
				h.t.Fatalf("key %q centres at %v, off the %v panel -- unreachable by a finger",
					string(r), c, dims)
			}
			out[r] = c
		}
	}
	h.keys = out
	return out
}

// typeWord taps one word out on the keyboard and accepts it.
func (h *unlockHarness) typeWord(w string) {
	h.t.Helper()
	pts := h.keyPoints()
	for _, r := range strings.ToLower(w) {
		pt, ok := pts[r]
		if !ok {
			h.t.Fatalf("the word keyboard has no %q key", string(r))
		}
		// Re-verify the cached point still resolves to the same target, so a
		// keyboard that moved cannot be silently mis-tapped.
		if _, _, hit := h.drawer().Hit(pt); !hit {
			h.t.Fatalf("the %q key's touch target vanished at %v", string(r), pt)
		}
		tap(&h.ctx.Router, h.drawer(), pt)
		h.next("after tapping " + string(r))
	}
	// OK commits the word and advances to the next slot.
	h.tapNav(Button3)
	h.next("after accepting " + w)
}

// typePassphrase types §8's twelve words. The flow leaves word entry on the
// twelfth OK, so no frame is pumped past it here.
func (h *unlockHarness) typePassphrase(words []string) {
	h.t.Helper()
	if len(words) != 12 {
		h.t.Fatalf("§8's passphrase is twelve words, got %d", len(words))
	}
	h.mustReach("Word 1 of 12")
	for _, w := range words {
		h.typeWord(w)
	}
}

// toPassphrase drives a SEALED payload from the flow's entry to the first word.
func (h *unlockHarness) toPassphrase(hasHash bool) {
	h.t.Helper()
	if hasHash {
		h.mustReach("Public data hash")
		h.tapNav(Button3)
	}
	h.mustReach("Enter the 12-word passphrase")
	h.tapNav(Button3)
	h.mustReach("Word 1 of 12")
}

// ---------------------------------------------------------------------------
// Task 4 — the checksum gate, BEFORE the KDF
// ---------------------------------------------------------------------------

// TestUnlockChecksumGateRunsNoKDF — §11.2: "BIP-39 checksum rejection happens
// without invoking the KDF."
//
// The assertion is on the COUNTER and not on the returned error, because a gate
// placed after unlockDerive returns the identical errUnlockChecksum ~31 s later
// on the device. A return-value assertion is a guaranteed false PASS over
// exactly that defect.
//
// beef x11 + bacon cannot be TYPED -- LastWordCandidates restricts the final
// slot to the 128 checksum-valid words -- so it is driven through
// unlockAttemptOnce, which takes the mnemonic rather than reading it for
// exactly this reason.
func TestUnlockChecksumGateRunsNoKDF(t *testing.T) {
	v := sealVector(t, "D")
	blob := v.blob(t)
	p := inspected(t, blob)

	bad := mnemonicOf(t, "beef", "beef", "beef", "beef", "beef", "beef",
		"beef", "beef", "beef", "beef", "beef", "bacon")
	if bad.Valid() {
		t.Fatal("premise broken: beef x11 + bacon must NOT be checksum-valid")
	}
	if !isMnemonicComplete(bad) {
		t.Fatal("premise broken: beef x11 + bacon is a complete twelve-word mnemonic")
	}

	c := installKDFCounter(t)
	if err := runUnlockAttempt(t, blob, p, bad); !errors.Is(err, errUnlockChecksum) {
		t.Errorf("a checksum-invalid passphrase returned %v, want errUnlockChecksum", err)
	}
	if c.calls != 0 {
		t.Errorf("the checksum-invalid passphrase ran the KDF %d times; §11.2 requires "+
			"the rejection happen WITHOUT invoking it", c.calls)
	}
	if p.Secret != nil {
		t.Errorf("a refused passphrase populated %d secret records", len(p.Secret))
	}

	// THE POSITIVE CONTROL, and it is what stops "the KDF was deleted" from
	// passing the assertion above. A checksum-VALID passphrase must run it
	// exactly once, at the header's own iteration count.
	good := mnemonicOf(t, vectorPassphrase(t, v)...)
	if !good.Valid() {
		t.Fatal("premise broken: the vector's own passphrase must be checksum-valid")
	}
	if err := runUnlockAttempt(t, blob, p, good); err != nil {
		t.Errorf("the vector's own passphrase failed to unlock: %v", err)
	}
	if c.calls != 1 {
		t.Errorf("a valid passphrase ran the KDF %d times, want exactly 1", c.calls)
	}
	if len(c.iterations) != 1 || c.iterations[0] != int(v.Iterations) {
		t.Errorf("the KDF ran at %v iterations; the header says %d -- the count must "+
			"come from the header and never from a constant", c.iterations, v.Iterations)
	}
	if len(p.Secret) != len(v.Secret) {
		t.Errorf("the unlock admitted %d secret records, the vector has %d",
			len(p.Secret), len(v.Secret))
	}
}

// TestUnlockRejectsAPartialPassphraseWithoutAKDF — the OTHER half of the gate,
// and the half NormalisePassphrase makes dangerous.
//
// inputWordsFlow returns on Back with whatever has been typed (gui/gui.go:692),
// so an 11x-1 mnemonic is reachable. Valid() is false on it, but String()
// returns "abandon           " which NormalisePassphrase silently repairs to
// "abandon" -- a well-formed, WRONG, one-word KDF input, ~31 s of waiting, and
// a tag failure indistinguishable from a wrong passphrase.
func TestUnlockRejectsAPartialPassphraseWithoutAKDF(t *testing.T) {
	v := sealVector(t, "D")
	blob := v.blob(t)
	p := inspected(t, blob)

	partial := emptyBIP39Mnemonic(12)
	partial[0] = mnemonicOf(t, "abandon")[0]
	if isMnemonicComplete(partial) {
		t.Fatal("premise broken: a one-word entry must not read as complete")
	}
	// The trap this guards, measured rather than argued.
	if got := seal.NormalisePassphrase(partial.String()); got != "abandon" {
		t.Fatalf("premise broken: NormalisePassphrase repaired the partial to %q", got)
	}

	c := installKDFCounter(t)
	if err := runUnlockAttempt(t, blob, p, partial); !errors.Is(err, errUnlockChecksum) {
		t.Errorf("a partial passphrase returned %v, want errUnlockChecksum", err)
	}
	if c.calls != 0 {
		t.Errorf("a partial passphrase ran the KDF %d times; §10.2 step 6 rejects it "+
			"BEFORE any derivation", c.calls)
	}
	// WHAT THIS TEST DOES NOT PIN, said plainly so the row is not read as
	// covered (lens 2 M1). Its message used to claim "the isMnemonicComplete
	// half of the gate is missing", and that claim was false: the mutant
	// `!isMnemonicComplete(m) || !m.Valid()` -> `!m.Valid()` SURVIVES, because
	// emptyBIP39Mnemonic fills with -1 and Mnemonic.Valid() ends
	// `ChecksumWord(ent) == last`, which can never equal -1. So !m.Valid()
	// alone already rejects every partial this flow can produce, and no input
	// reachable here discriminates the two halves.
	//
	// The report that found this suggested asserting panic-safety instead --
	// that isMnemonicComplete keeps Valid()/splitMnemonic off -1 input, where
	// bytes.Repeat with a negative count would panic. MEASURED, and that is
	// also false: Valid() on a fully -1 mnemonic (12 and 24) and on a
	// half-typed one returns false with NO panic. Both claims are recorded as
	// refuted so the next reader does not re-derive either.
	//
	// isMnemonicComplete stays as defence in depth -- it states the intent
	// independently of Valid()'s implementation -- but nothing in this test
	// stands behind it, and pretending otherwise is what this comment prevents.
}

// TestPassphraseBytesIsSection81sNormalisedForm. The buffer exists so the
// caller can zero it -- Mnemonic.String() produces byte-identical output but as
// a Go STRING, which cannot be zeroed.
func TestPassphraseBytesIsSection81sNormalisedForm(t *testing.T) {
	v := sealVector(t, "D")
	m := mnemonicOf(t, vectorPassphrase(t, v)...)
	got := passphraseBytes(m)
	if want := *v.Passphrase; string(got) != want {
		t.Errorf("passphraseBytes = %q, want the vector's own passphrase %q", got, want)
	}
	if want := seal.NormalisePassphrase(m.String()); string(got) != want {
		t.Errorf("passphraseBytes = %q, want §8.1's normalised form %q", got, want)
	}
	// Lowercase, single-space separated, no leading or trailing space.
	if strings.ToLower(string(got)) != string(got) {
		t.Errorf("passphraseBytes is not lowercase: %q", got)
	}
	if strings.Contains(string(got), "  ") || strings.TrimSpace(string(got)) != string(got) {
		t.Errorf("passphraseBytes is not single-space separated and trimmed: %q", got)
	}
	// The capacity is FIXED so append never regrows: a regrow leaves a stale
	// copy of the first half of the passphrase in an orphaned array that
	// nothing can reach to wipe. The widest legal input is twelve eight-letter
	// words plus eleven separators = 107 bytes.
	if cap(got) < 107 {
		t.Errorf("passphraseBytes' capacity is %d; a twelve-word passphrase needs 107 "+
			"without regrowing", cap(got))
	}
	widest := make(bip39.Mnemonic, 12)
	for i := range widest {
		widest[i] = mnemonicOf(t, "abandon")[0]
	}
	if b := passphraseBytes(widest); cap(b) < len(b) || len(b) > cap(got) {
		t.Errorf("a twelve-word passphrase is %d bytes in a %d-byte buffer", len(b), cap(got))
	}
	// And it is the CALLER's buffer: clear() must reach it.
	clear(got)
	for _, c := range got {
		if c != 0 {
			t.Fatalf("clear() did not reach passphraseBytes' buffer: %q", got)
		}
	}
}

// TestUnlockKDFLeadEstimatesFromThisDerivation — §10.2 step 7's "the screen
// must say how long it will take", measured from THIS derivation rather than
// read off §7.1's table, so it stays right on a part whose rate differs from
// the RP2350A the table was measured on.
//
// ON THE MULTIPLY-BEFORE-DIVIDE ORDER, recorded rather than asserted. The
// divide-first variant `(elapsed/done)*(total-done)` truncates to whole
// nanoseconds, and the resulting error is bounded by (total-done) ns. Measured
// over §6.2's whole legal iteration range [100,000, 2,000,000] and elapsed
// times from 1 ns to 1,000 s, the WORST gap is 1,995,004 ns -- 2 milliseconds,
// which can never change a whole-second reading. So the order is strictly more
// accurate and has no downside, but it is NOT observable through the rendered
// string, and a test claiming to kill that mutant would be a false PASS. It is
// named here instead.
func TestUnlockKDFLeadEstimatesFromThisDerivation(t *testing.T) {
	// Before any progress there is nothing to estimate from.
	if got := unlockKDFLead(0, 100000, 0); !strings.Contains(got, "30 seconds") {
		t.Errorf("the pre-progress lead is %q; §10.2 step 7 requires it say how long", got)
	}
	if got := unlockKDFLead(1000, 100000, 0); !strings.Contains(got, "30 seconds") {
		t.Errorf("a zero elapsed must not be divided by: %q", got)
	}
	// 1% done in 310 ms => 30.69 s remaining, which rounds to 31.
	if got := unlockKDFLead(1000, 100000, 310*time.Millisecond); !strings.Contains(got, "31 seconds") {
		t.Errorf("unlockKDFLead(1000/100000, 310ms) = %q, want ~31 seconds", got)
	}
	// §7.1's own device rate, 9,715 it/s: 100,000 iterations is ~10.3 s, so at
	// the first frame the screen must promise ~10 seconds and not ~31.
	if got := unlockKDFLead(500, 100000, 51*time.Millisecond); !strings.Contains(got, "10 seconds") {
		t.Errorf("at §7.1's measured rate the lead reads %q, want ~10 seconds -- the "+
			"estimate is not coming from THIS derivation", got)
	}
	// And it DECREASES: an estimate that grew or stood still would read as a
	// machine that is not making progress, which is what step 7 exists to
	// prevent.
	prev := 1 << 30
	for _, done := range []int{1000, 25000, 50000, 75000, 99000} {
		got := unlockKDFLead(done, 100000, time.Duration(done)*3*time.Microsecond)
		var secs int
		if _, err := fmt.Sscanf(got, "Unlocking. About %d seconds left.", &secs); err != nil {
			t.Fatalf("unlockKDFLead(%d) = %q, which does not parse: %v", done, got, err)
		}
		if secs > prev {
			t.Errorf("at a constant rate the estimate rose from %d s to %d s at %d/100000",
				prev, secs, done)
		}
		prev = secs
	}
	if prev != 0 {
		t.Errorf("at 99%% the estimate is still %d seconds", prev)
	}
}

// ---------------------------------------------------------------------------
// Task 5 — the flow
// ---------------------------------------------------------------------------

// TestUnlockNeverPromptsWhenNothingIsEncrypted — §11.3's "passphrase prompted
// when ct_len == 0" mutant, and §11.2 names the assertion: "Vector E reaches
// the plate list with the keyboard flow NEVER ENTERED -- asserted by
// instrumenting the prompt entry point, not by return value. A scripted fake
// platform will happily feed twelve words into a prompt that should not exist
// and still reach the plate list, so a return-value assertion reports PASS over
// exactly the defect."
func TestUnlockNeverPromptsWhenNothingIsEncrypted(t *testing.T) {
	entered := 0
	unlockPassphraseHook = func() { entered++ }
	t.Cleanup(func() { unlockPassphraseHook = nil })

	v := sealVector(t, "E")
	if v.CtLen != 0 {
		t.Fatalf("premise broken: vector E must be unsealed, ct_len = %d", v.CtLen)
	}
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.mustReach("Public data hash")
	h.tapNav(Button3)
	h.mustReach("NOT AUTHENTICATED")
	// Hold-to-confirm is the §10.2 step 4 "explicit confirmation". The panel
	// reports a press and a release as separate pointer events, so the hold is
	// a press with no release plus elapsed time.
	pos := navSlotPoint(h.ctx, Button3)
	h.ctx.Router.Events(h.drawer(), PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event())
	h.next("after pressing confirm")
	deadline := time.Now().Add(2 * confirmDelay)
	for time.Now().Before(deadline) {
		if uiContains(h.content, "mk1 1/2") {
			break
		}
		h.next("holding confirm")
	}
	h.mustReach("mk1 1/2")
	if entered != 0 {
		t.Fatalf("an UNSEALED payload entered the passphrase prompt %d times; §10.2 step 4 "+
			"stops before the passphrase when ct_len == 0", entered)
	}
	// The keyboard itself must never have been drawn either.
	if uiContains(h.content, "Word 1 of") {
		t.Fatalf("an unsealed payload reached word entry: %q", h.content)
	}
}

// TestUnlockPromptsOnASealedPayload is the POSITIVE control for the hook above.
// Without it, deleting the whole sealed branch would leave the negative green.
func TestUnlockPromptsOnASealedPayload(t *testing.T) {
	entered := 0
	unlockPassphraseHook = func() { entered++ }
	t.Cleanup(func() { unlockPassphraseHook = nil })

	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	if entered != 1 {
		t.Errorf("a SEALED payload entered the passphrase prompt %d times, want 1", entered)
	}
}

// TestUnlockRetryKeepsTheHashOnScreen — §10.2 step 8.
//
// The message MUST offer both readings: the tag also authenticates the public
// section (§6.1a), so a tampered public card fails here too, ~31 s after the
// hash was displayed. Reporting only "wrong passphrase" invites the operator to
// retype three times and conclude the blob is corrupt, losing the one signal
// §2.2 item 4 exists to raise.
func TestUnlockRetryKeepsTheHashOnScreen(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	// A checksum-VALID but WRONG passphrase, so the failure is the tag and not
	// the gate.
	wrong := []string{"abandon", "abandon", "abandon", "abandon", "abandon", "abandon",
		"abandon", "abandon", "abandon", "abandon", "abandon", "about"}
	if !mnemonicOf(t, wrong...).Valid() {
		t.Fatal("premise broken: the wrong passphrase must still be checksum-valid")
	}
	if strings.Join(wrong, " ") == *v.Passphrase {
		t.Fatal("premise broken: the wrong passphrase must differ from the vector's")
	}
	h.typePassphrase(wrong)
	content := h.mustReach("has been altered")

	if !uiContains(content, "Wrong passphrase") {
		t.Errorf("the retry message does not offer the wrong-passphrase reading: %q", content)
	}
	if !uiContains(content, *v.PubhashSealed) {
		t.Errorf("the retry message dropped the §6.6 hash %s: %q", *v.PubhashSealed, content)
	}
	// ANCHORED: "SEALED" is a SUBSTRING of "UNSEALED" and uiContains is
	// strings.Contains, so a bare "SEALED" needle is satisfied by a screen
	// reading UNSEALED. unlockRetryBody emits "(%d records, %s):".
	if !uiContains(content, ", SEALED):") {
		t.Errorf("the retry message does not carry the SEALED shape: %q", content)
	}
	if uiContains(content, ", UNSEALED):") {
		t.Errorf("the retry message calls a sealed payload UNSEALED: %q", content)
	}
	if !uiContains(content, "5 records") {
		t.Errorf("the retry message does not carry the PUBLIC record count: %q", content)
	}
	// And it RETRIES rather than leaving: dismissing returns STRAIGHT to word
	// entry, with the flow still running.
	//
	// "Straight" is itself the assertion (lens 4 NIT 7). The "These words are
	// NOT a seed" notice belongs to the SESSION, not to the attempt:
	// unlockSealedFlow calls unlockPassphraseFlow afresh per retry, so with the
	// notice inside it a wrong passphrase cost the operator TWO dismissals and
	// put a screen between the §6.6 hash and the keyboard -- on the one path
	// where that hash is the signal they are meant to act on.
	h.tapNav(Button3)
	reachedEntry := false
	for i := 0; i < 64; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
		if uiContains(c, "Enter the 12-word passphrase") {
			t.Fatalf("the passphrase notice was re-shown on the retry: %q\n"+
				"it is shown ONCE per session, above unlockSealedFlow's retry loop", c)
		}
		if uiContains(c, "Word 1 of 12") {
			reachedEntry = true
			break
		}
	}
	if !reachedEntry {
		t.Fatalf("dismissing the retry message did not return to word entry; last frame %q",
			h.content)
	}
	if *h.done {
		t.Fatal("a wrong passphrase ended the flow instead of re-prompting")
	}
}

// TestUnlockCancelNeverReachesThePlateList. A false return from
// unlockSealedFlow MUST NOT fall through: p.Public on a sealed payload is a
// legitimate record set, and engraving it while dropping the encrypted half is
// §6.4's incomplete-backup-believed-complete, the worst available outcome.
func TestUnlockCancelNeverReachesThePlateList(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	// Back out of word entry with nothing typed.
	h.tapNav(Button1)

	labels := plateRecords(t, "D")
	for i := 0; i < 64 && !*h.done; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		for j := range labels {
			if uiContains(c, plateLabel(labels[j], j)) {
				t.Fatalf("cancelling the passphrase still reached the plate list: %q", c)
			}
		}
	}
	if !*h.done {
		t.Fatal("cancelling the passphrase did not return to the menu")
	}
}

// TestUnlockTooManyRecordsIsNotReportedAsAWrongPassphrase — §6.4 requires it be
// distinguishable, and it is the mutant "the ErrAuthentication arm made the
// default" that this kills: the count is authenticated plaintext so naming it
// leaks nothing, and conflating a too-large wallet with an attack would send
// the operator chasing a compromise that did not happen.
func TestUnlockTooManyRecordsIsNotReportedAsAWrongPassphrase(t *testing.T) {
	d := sealVector(t, "D")
	// Five public records plus twenty encrypted ones is 25, one past §6.4's
	// cap of 24 across BOTH sections -- which is the only place the
	// cross-section total is known.
	secret := make([]string, 20)
	for i := range secret {
		secret[i] = "md1qqq"
	}
	words := vectorPassphrase(t, d)
	blob := sealBlobForTest(t, d.Public, secret, strings.Join(words, " "), fixtureIterations)

	h := newUnlockHarness(t, payloadReaderFrom(t, blob))
	h.toPassphrase(true)
	h.typePassphrase(words)
	content := h.mustReach("more records than the machine accepts")
	if uiContains(content, "Wrong passphrase") {
		t.Errorf("a too-large payload was reported as a wrong passphrase: %q", content)
	}
	if uiContains(content, "Payload unreadable") {
		t.Errorf("a too-large payload was reported as unreadable: %q", content)
	}
}

// progressPct reads the percentage off a drawn progress frame. ExtractText
// emits no spaces, so the number sits between the title and the lead line.
// The "%?" is F-86: Boldprogress45 now carries a "%" glyph, so it is drawn
// (and therefore extracted) between the digits and "Unlocking" where it
// used to render zero pixels and vanish from the extracted stream entirely.
var progressPct = regexp.MustCompile(`(?i)sealedpayload(\d+)%?unlocking`)

// TestUnlockDerivesWithARealProgressScreen — §10.2 step 7. pbkdf2.Key is one
// blocking call: the frame loop stops for ~31 s and the screen freezes, which
// is exactly the "machine has hung" reading step 7 exists to prevent.
//
// So the assertion is that the percentage ADVANCES across frames, not merely
// that a screen appeared -- and that the FRAME COUNT is the predicted one,
// which is what says the derivation really is chunked at kdfStepIterations
// rather than run in one blocking call with a progress bar drawn around it.
func TestUnlockDerivesWithARealProgressScreen(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	h.typePassphrase(vectorPassphrase(t, v))

	// NewDeriver performs U_1 itself, so done starts at 1 and each Step covers
	// kdfStepIterations more; the Step that completes the derivation draws no
	// frame. Predicted for 100,000 at 500/frame: 200 Step calls, 199 frames.
	steps := (int(v.Iterations) - 1 + kdfStepIterations - 1) / kdfStepIterations
	wantFrames := steps - 1

	var pcts []int
	frames := 0
	// typePassphrase's last OK already consumed the first progress frame, so
	// count that one too.
	for c := h.content; ; {
		m := progressPct.FindStringSubmatch(c)
		if m == nil {
			break
		}
		frames++
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unreadable progress reading %q", m[1])
		}
		pcts = append(pcts, n)
		next, ok := h.frame()
		if !ok {
			// A successful unlock ends this build's flow (§10.2.2's session is
			// Task 6), so the frame after the last progress frame is the end.
			break
		}
		h.content, c = next, next
		if frames > 512 {
			t.Fatalf("the derivation drew more than 512 frames")
		}
	}
	if len(pcts) == 0 {
		t.Fatalf("no progress frame was drawn at all; last frame %q", h.content)
	}
	t.Logf("the derivation drew %d progress frames at %d iterations, %d per frame "+
		"(predicted %d); percentage ran %d%% -> %d%%",
		frames, v.Iterations, kdfStepIterations, wantFrames, pcts[0], pcts[len(pcts)-1])

	if frames != wantFrames {
		t.Errorf("the derivation drew %d progress frames, predicted %d", frames, wantFrames)
	}
	if frames < 2 {
		t.Fatalf("the derivation drew %d progress frames; a blocking KDF is what §10.2 "+
			"step 7 exists to prevent", frames)
	}
	// It ADVANCES, monotonically, and covers the range.
	distinct := make(map[int]bool)
	for i, p := range pcts {
		distinct[p] = true
		if i > 0 && p < pcts[i-1] {
			t.Errorf("the progress reading went backwards: %d%% after %d%%", p, pcts[i-1])
		}
		if p < 0 || p > 100 {
			t.Errorf("the progress reading is %d%%", p)
		}
	}
	if len(distinct) < 50 {
		t.Errorf("the progress reading took only %d distinct values over %d frames; it "+
			"must ADVANCE, not sit still", len(distinct), frames)
	}
	if pcts[0] > 5 || pcts[len(pcts)-1] < 95 {
		t.Errorf("the progress ran %d%% -> %d%%, which does not span the derivation",
			pcts[0], pcts[len(pcts)-1])
	}
}

// TestProgressStyleRendersPercentSign is F-86's regression guard, superseding
// TestProgressStyleRendersNoPercentSign (which pinned the DEFECT: "%" used to
// contribute zero pixels in Styles.progress because Boldprogress45 was
// generated with -alphabet "0123456789:", missing "%"). font/poppins/gen.go
// now generates it with "0123456789:%%" (F-86), and
// font/poppins/boldprogress45_test.go's TestBoldprogress45HasPercentGlyph
// pins the glyph directly on the RASTER. This test pins the consequence at
// the call-site style level: unlockDerive formats the reading as "%d%%" in
// Styles.progress, and it must now measure wider with "%" than without, the
// same way Styles.lead always has.
func TestProgressStyleRendersPercentSign(t *testing.T) {
	ctx := NewContext(newPlatform())
	w := func(style, s string) int {
		st := ctx.Styles.progress
		if style == "lead" {
			st = ctx.Styles.lead
		}
		_, sz := widget.Label(&ctx.B, st, descriptorTheme.Text, s)
		return sz.X
	}
	prog, progPct := w("progress", "50"), w("progress", "50%")
	lead, leadPct := w("lead", "50"), w("lead", "50%")
	t.Logf("progress: width(\"50\")=%d width(\"50%%\")=%d (delta %d)", prog, progPct, progPct-prog)
	t.Logf("lead:     width(\"50\")=%d width(\"50%%\")=%d (delta %d)", lead, leadPct, leadPct-lead)

	if leadPct <= lead {
		t.Errorf("Styles.lead does not render \"%%\" (delta %d); setup invalid -- the positive "+
			"control face must already carry the glyph", leadPct-lead)
	}
	if progPct <= prog {
		t.Errorf("Styles.progress still does not render \"%%\" (delta %d px): the operator "+
			"sees a bare number where the code says a percentage (F-86)", progPct-prog)
	}
}

// navSlotPoint is the centre of a nav slot, computed the way layoutNavigation
// places it (gui/gui.go:1918-1932).
func navSlotPoint(ctx *Context, b Button) image.Point {
	dims := ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	return image.Pt(dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
}

// The KDF progress loop must ask for its next frame BEFORE submitting one.
//
// ctx.Frame is the yield, and Run reads the deadline for the frame it has just
// been handed (`wakeup := ctx.Wakeup`) BEFORE its own ctx.Reset(). A WakeupAt
// placed after Frame therefore governs the NEXT frame, and frame 1 inherits the
// preceding screen's deadline — Run's ctx.WakeupAt(idleWakeup), i.e. three
// minutes. The derivation parks at 500/300,000 iterations and the screensaver
// takes the screen.
//
// A frame-COUNT assertion is a guaranteed false PASS here: the count is 199
// either way. The property is the DEADLINE, so this test reproduces Run's own
// read-then-Reset ordering and asserts every progress frame is submitted with an
// already-expired one. (B2a-ii whole-diff review, concurrency lens.)
func TestKDFProgressFramesAreSubmittedWithAnExpiredDeadline(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)

	var future int
	var frames int
	ctx.FrameCallback = func(op.Op) {
		// Read the deadline for THIS frame exactly as Run does, before Reset.
		if !ctx.Wakeup.IsZero() && ctx.Wakeup.After(time.Now()) {
			future++
		}
		frames++
	}
	// Seed the deadline the way Run leaves it for a newly-entered screen: the
	// idle timeout, three minutes out. Without this the test cannot see the bug.
	ctx.WakeupAt(time.Now().Add(idleTimeout))

	h := seal.Header{Iterations: 2000}
	for i := range h.Salt {
		h.Salt[i] = byte(i)
	}
	pass := []byte("beef beef beef beef beef beef beef beef beef beef beef beef")
	if key, ok := unlockDerive(ctx, &descriptorTheme, h, pass); !ok || key == nil {
		t.Fatal("the derivation did not complete")
	}
	if frames == 0 {
		t.Fatal("no progress frames were drawn; the test asserted nothing")
	}
	if future != 0 {
		t.Errorf("%d of %d progress frames were submitted with a FUTURE deadline; "+
			"the derivation parks instead of running. WakeupAt must precede ctx.Frame",
			future, frames)
	}
}

// TestVectorBIterationsComeFromTheHeader — §11.3's mandatory row "iteration
// count read as a constant | §11.4 vector B — nothing else sees it", and §11.2's
// "Vector B is not optional — it is the only test that catches a hardcoded
// iteration count."
//
// TestUnlockChecksumGateRunsNoKDF above already asserts the count came from the
// header. It cannot fail: every vector a gui test reached was at 100,000 —
// §6.2's FLOOR and by far the most likely hardcoded value — so the assertion
// compared 100000 against 100000 and passed under
// `newDeriver(pass, h.Salt[:], 100000)`. Vector B, at 100,001, is the only input
// that discriminates, and `grep '"B"' gui/*_test.go` returned nothing before
// this test: B was used in seal/ only, and unlockDerive is a SEPARATE call site
// from seal.Unlock's.
//
// What the mutant costs in the field: §7.1's default is 300,000, so every real
// payload would fail its tag ~10 s in and the device would report "Wrong
// passphrase, or this payload has been altered" — teaching the operator to read
// a false tamper alarm as normal, which is precisely the signal §2.2 item 4
// exists to raise.
func TestVectorBIterationsComeFromTheHeader(t *testing.T) {
	v := sealVector(t, "B")
	if v.Iterations == fixtureIterations {
		t.Fatalf("premise broken: vector B must differ from §6.2's floor of %d, which is "+
			"the value a hardcoded count would most likely be; it reads %d",
			fixtureIterations, v.Iterations)
	}
	blob := v.blob(t)
	p := inspected(t, blob)
	if p.Header.Iterations != v.Iterations {
		t.Fatalf("Inspect read %d iterations, the vector says %d",
			p.Header.Iterations, v.Iterations)
	}

	c := installKDFCounter(t)
	if err := runUnlockAttempt(t, blob, p, mnemonicOf(t, vectorPassphrase(t, v)...)); err != nil {
		t.Fatalf("vector B failed to unlock: %v -- a KDF run at any count but the "+
			"header's %d derives the wrong key, and the tag fails ~31 s in on the device",
			err, v.Iterations)
	}
	if len(c.iterations) != 1 || c.iterations[0] != int(v.Iterations) {
		t.Errorf("the KDF ran at %v iterations; vector B's header says %d -- the count "+
			"must come from the header and never from a constant", c.iterations, v.Iterations)
	}
	if len(p.Secret) != len(v.Secret) {
		t.Errorf("the unlock admitted %d secret records, vector B has %d",
			len(p.Secret), len(v.Secret))
	}
}

// TestUnlockDerivesAtTheMaximumIterationCount — §6.2's CEILING, which nothing
// had ever run (B2a-ii whole-diff review, lens 8 M4; it also pins lens 6 N2's
// coupling at the bound).
//
// Every vector is at 100,000 or 100,001 and fixtureIterations is 100,000, so
// the top 95% of the legal range had never been through the progress screen.
// That matters for one specific reason: the iteration count is
// ATTACKER-CONTROLLED (§2.2 item 4 concedes the region is writable) and it is
// the only header field this screen consumes arithmetically.
//
// Two properties, both invisible at the floor:
//
//  1. d.Done()*100/d.Total() is computed in a 32-bit int on target. At the
//     ceiling the product is 200,000,000 against int32's 2,147,483,647. The
//     percentage must stay in 0..99 and never run backwards -- a wrapped
//     product renders NEGATIVE, on the one screen §10.2 step 7 exists to keep
//     legible.
//  2. The frame count follows the count from the HEADER, so the screen is
//     driven by the payload rather than by a constant.
func TestUnlockDerivesAtTheMaximumIterationCount(t *testing.T) {
	if seal.MaxIterations != 2_000_000 {
		t.Fatalf("premise: this test's arithmetic assumes MaxIterations == 2,000,000, got %d",
			seal.MaxIterations)
	}
	// The largest MaxIterations d.Done()*100 tolerates in a 32-bit int.
	const safeCeiling = 21_474_836
	if seal.MaxIterations > safeCeiling {
		t.Fatalf("seal.MaxIterations = %d exceeds %d: d.Done()*100 wraps a 32-bit int and "+
			"the progress screen renders a negative percentage",
			seal.MaxIterations, safeCeiling)
	}

	d := sealVector(t, "D")
	words := vectorPassphrase(t, d)
	blob := sealBlobForTest(t, d.Public, d.Secret, strings.Join(words, " "), seal.MaxIterations)

	h := newUnlockHarness(t, payloadReaderFrom(t, blob))
	h.toPassphrase(true)
	h.typePassphrase(words)

	// (MaxIterations-1)/kdfStepIterations steps, one frame between each.
	wantFrames := (seal.MaxIterations-1)/kdfStepIterations - 1
	frames, last := 0, -1
	minPct, maxPct := 100, -1
	for i := 0; i < wantFrames+512; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
		m := progressPct.FindStringSubmatch(c)
		if m == nil {
			if frames > 0 {
				break // the derivation finished and the screen moved on
			}
			continue
		}
		pct, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unreadable progress reading %q", m[1])
		}
		if pct < 0 || pct > 99 {
			t.Fatalf("frame %d read %d%%: at %d iterations d.Done()*100 is %d, and a "+
				"32-bit int holds %d -- a reading outside 0..99 is the wrap",
				frames, pct, seal.MaxIterations, seal.MaxIterations*100, math.MaxInt32)
		}
		if pct < last {
			t.Fatalf("frame %d read %d%% after %d%%: the progress ran BACKWARDS", frames, pct, last)
		}
		last = pct
		if pct < minPct {
			minPct = pct
		}
		if pct > maxPct {
			maxPct = pct
		}
		frames++
	}
	if frames != wantFrames {
		t.Errorf("the derivation drew %d progress frames at %d iterations, want %d\n"+
			"the frame count follows the count from the HEADER; a mismatch means the "+
			"screen is driven by a constant", frames, seal.MaxIterations, wantFrames)
	}
	if minPct != 0 || maxPct != 99 {
		t.Errorf("the percentage spanned %d..%d, want 0..99", minPct, maxPct)
	}
	// It must actually SUCCEED at the ceiling, not merely draw.
	h.mustReach("SECRET seed material")
}

// TestUnlockRetryOnAPayloadWithNoPublicSectionShowsNoDigest — §10.2 step 3's
// "furniture" rule on the RETRY screen (B2a-ii whole-diff review, lens 2 M3).
//
// A, B, C and F all carry pub_len == 0, and the retry test uses D. So no test
// drove a wrong passphrase against a payload with no public section, and the
// mutant `if !p.HasHash` -> `if false` survived. Under it a wrong passphrase on
// vector C displays
//
//	Public data hash (0 records, SEALED): 0000 0000 ... 0000
//
// -- the digest of the EMPTY record set, a constant shown on every fully
// encrypted payload, which §10.2 step 3 and §6.6 say "would teach the operator
// it is furniture", on the one screen that exists to raise a tamper signal.
func TestUnlockRetryOnAPayloadWithNoPublicSectionShowsNoDigest(t *testing.T) {
	v := sealVector(t, "C")
	p := unlockedPayload(t, "C")
	if len(p.Public) != 0 {
		t.Fatalf("premise broken: vector C must have no public section, got %d records",
			len(p.Public))
	}
	// The TWO furniture constants a broken branch can print, both COMPUTED and
	// never retyped. They differ, and knowing which is which is the point:
	// p.Hash is never POPULATED when pub_len == 0, so the branch this test's
	// mutant removes prints the ZERO-VALUE array; a different mistake --
	// hashing the empty record set rather than declining to -- prints the
	// digest of no records. Measured:
	//	unpopulated  "0000 0000 0000 0000 0000 0000 0000 0000"
	//	empty set    "9c37 4420 ca19 4e28 8469 bc81 e9e1 b64b"
	// Both are constants on every fully encrypted payload, which is exactly
	// what §10.2 step 3 forbids, so both are excluded.
	var zeroHash [16]byte
	unpopulatedDigest := seal.FormatHash(zeroHash)
	emptySetDigest := seal.FormatHash(seal.PublicDataHash(nil, true))
	if unpopulatedDigest == emptySetDigest {
		t.Fatal("premise broken: the two furniture constants must differ, or this test " +
			"excludes only one of them")
	}

	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	// pub_len == 0, so §10.2 step 3 shows no hash notice at all on the way in.
	h.toPassphrase(false)
	wrong := []string{"abandon", "abandon", "abandon", "abandon", "abandon", "abandon",
		"abandon", "abandon", "abandon", "abandon", "abandon", "about"}
	if !mnemonicOf(t, wrong...).Valid() {
		t.Fatal("premise broken: the wrong passphrase must still be checksum-valid")
	}
	h.typePassphrase(wrong)
	content := h.mustReach("has been altered")

	if !uiContains(content, "Wrong passphrase") {
		t.Errorf("the retry message does not offer the wrong-passphrase reading: %q", content)
	}
	for _, c := range []struct{ what, digest string }{
		{"UNPOPULATED (zero-value p.Hash)", unpopulatedDigest},
		{"EMPTY-RECORD-SET", emptySetDigest},
	} {
		if uiContains(content, c.digest) {
			t.Errorf("the retry message printed the %s digest %s: %q\n"+
				"it is a constant on every pub_len == 0 payload; §10.2 step 3 says showing "+
				"it teaches the operator the digest is furniture, on the one screen that "+
				"exists to raise a tamper signal", c.what, c.digest, content)
		}
	}
	if uiContains(content, "Public data hash") {
		t.Errorf("the retry message offered a public data hash on a payload with no "+
			"public section: %q", content)
	}
	if uiContains(content, "0 records") {
		t.Errorf("the retry message advertised a 0-record hash: %q", content)
	}
	// It must still say what to DO, rather than trailing off.
	if !uiContains(content, "Check the words and try again") {
		t.Errorf("the pub_len == 0 retry message gives the operator no next step: %q", content)
	}
}
