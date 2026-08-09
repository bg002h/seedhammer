package gui

import (
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/seal"
)

// §11.2's wipe clause, on the buffers it names that are NOT records:
//
//	"After leaving a bundle session by EACH exit -- Lock, Back, an error path,
//	and ctx.Done -- the plaintext record buffer, THE DERIVED KEY, and THE
//	PASSPHRASE BUFFER MUST read as zeroed. Asserted ON THE BUFFERS THEMSELVES."
//
// The record buffers were pinned by unlock_session_test.go. These are the other
// three, and until this file existed every one of them could be deleted with the
// whole suite green (B2a-ii whole-diff review, lens 2 I3 / lens 3 I1):
//
//	defer clear(pass)   unlockAttemptOnce      the §8.1 passphrase bytes
//	defer clear(key)    unlockAttemptOnce      the AES-256 key
//	defer d.Wipe()      unlockDerive           the PBKDF2 accumulator
//	clear(m)            unlockSealedFlow       the typed []Word, per attempt
//	clear(m)            unlockPassphraseFlow   the typed []Word, partial exit
//
// EVERY assertion here reads the buffer AFTER the function that owns it has
// returned. A return value cannot tell a wipe from a missing wipe, which is what
// the seams exist for.

// allZeroWords is allZeroBytes for a []Word.
//
// The distinction is load-bearing: emptyBIP39Mnemonic fills with -1
// (gui/gui.go:625-631), so an UNTOUCHED passphrase buffer reads all -1 and a
// half-typed one reads a mix of -1 and word indices. Only clear() makes it read
// all zero, so "all zero" is not a state any other path can produce.
func allZeroWords(m bip39.Mnemonic) bool {
	for _, w := range m {
		if w != 0 {
			return false
		}
	}
	return true
}

// TestUnlockAttemptZeroesThePassphraseBuffer — `defer clear(pass)`.
//
// No new seam: newDeriver is already handed the very buffer unlockAttemptOnce
// clears, so the test holds the same backing array the KDF read.
func TestUnlockAttemptZeroesThePassphraseBuffer(t *testing.T) {
	var heldPass []byte
	var handedOver string
	prev := newDeriver
	newDeriver = func(passphrase, salt []byte, iterations int) *seal.Deriver {
		heldPass = passphrase
		// A COPY, taken at handover, so the assertion below has a positive
		// control: it proves this buffer really carried §8.1's normalised
		// passphrase and not something already blank.
		handedOver = string(passphrase)
		return seal.NewDeriver(passphrase, salt, iterations)
	}
	t.Cleanup(func() { newDeriver = prev })

	v := sealVector(t, "D")
	blob := v.blob(t)
	p := inspected(t, blob)
	if err := runUnlockAttempt(t, blob, p, mnemonicOf(t, vectorPassphrase(t, v)...)); err != nil {
		t.Fatalf("the vector's own passphrase failed to unlock: %v", err)
	}
	if heldPass == nil {
		t.Fatal("the KDF was never handed a passphrase; this test asserted nothing")
	}
	if handedOver != *v.Passphrase {
		t.Fatalf("the KDF was handed %q, the vector's passphrase is %q -- this test is "+
			"watching the wrong buffer", handedOver, *v.Passphrase)
	}
	if !allZeroBytes(heldPass) {
		t.Errorf("the passphrase buffer survived unlockAttemptOnce: %q\n"+
			"§11.2 requires it read as zeroed, asserted on the buffer itself -- these "+
			"twelve words are the ENTIRE strength of §2.2a's security argument",
			heldPass)
	}
}

// TestUnlockAttemptZeroesTheDerivedKey — `defer clear(key)`.
//
// This one needs unlockKeyHook: unlockDerive returns d.Key(), which is a FRESH
// copy (seal/pbkdf2.go), so the Deriver the newDeriver seam holds is a different
// allocation from the buffer unlockAttemptOnce owns and zeroes.
func TestUnlockAttemptZeroesTheDerivedKey(t *testing.T) {
	var held []byte
	var liveAtHandover bool
	unlockKeyHook = func(key []byte) {
		held = key
		liveAtHandover = !allZeroBytes(key)
	}
	t.Cleanup(func() { unlockKeyHook = nil })

	v := sealVector(t, "D")
	blob := v.blob(t)
	p := inspected(t, blob)
	if err := runUnlockAttempt(t, blob, p, mnemonicOf(t, vectorPassphrase(t, v)...)); err != nil {
		t.Fatalf("the vector's own passphrase failed to unlock: %v", err)
	}
	if held == nil {
		t.Fatal("unlockKeyHook never fired; this test asserted nothing")
	}
	if len(held) != seal.KeyLen {
		t.Fatalf("the key handed over is %d bytes, want %d", len(held), seal.KeyLen)
	}
	// The positive control. An all-zero key is a VALID AES key, so a test that
	// only asked "is it zero now" would pass on a derivation that produced
	// nothing -- the exact failure seal/pbkdf2.go's Key contract exists to
	// prevent.
	if !liveAtHandover {
		t.Fatal("the derived key was already all-zero when unlockAttemptOnce took it; " +
			"this test cannot distinguish a wipe from a dead derivation")
	}
	if !allZeroBytes(held) {
		t.Errorf("the derived key survived unlockAttemptOnce: %x\n"+
			"§10.2 step 10 zeroes it on every exit path", held)
	}
}

// TestUnlockDeriveWipesTheDeriverItOwns — `defer d.Wipe()`.
//
// Wipe zeroes u and acc and resets done, and a post-Wipe Key() therefore returns
// nil rather than 32 zero bytes (seal/pbkdf2.go) -- which is the observable this
// rests on. It only discriminates because the unlock SUCCEEDED: a derivation
// that never completed would return nil from Key() too, so the error check above
// the assertion is doing real work.
func TestUnlockDeriveWipesTheDeriverItOwns(t *testing.T) {
	var heldD *seal.Deriver
	prev := newDeriver
	newDeriver = func(passphrase, salt []byte, iterations int) *seal.Deriver {
		heldD = seal.NewDeriver(passphrase, salt, iterations)
		return heldD
	}
	t.Cleanup(func() { newDeriver = prev })

	v := sealVector(t, "D")
	blob := v.blob(t)
	p := inspected(t, blob)
	if err := runUnlockAttempt(t, blob, p, mnemonicOf(t, vectorPassphrase(t, v)...)); err != nil {
		t.Fatalf("the vector's own passphrase failed to unlock: %v", err)
	}
	if heldD == nil {
		t.Fatal("newDeriver was never called; this test asserted nothing")
	}
	if k := heldD.Key(); k != nil {
		t.Errorf("the Deriver was not wiped; Key() still returns %x\n"+
			"the PBKDF2 accumulator IS the key, and §10.2 step 10 covers it", k)
	}
}

// TestUnlockZeroesTheTypedPassphraseAfterTheAttempt — unlockSealedFlow's
// `clear(m)`.
//
// The reading is taken while §10.2.2's session is ON SCREEN, not after the flow
// returns. That is the whole point: the words must be gone the instant
// unlockAttemptOnce hands back, not whenever the flow happens to exit, because
// between those two moments the machine cuts plates.
func TestUnlockZeroesTheTypedPassphraseAfterTheAttempt(t *testing.T) {
	var typed []bip39.Mnemonic
	unlockPassphraseWordsHook = func(m bip39.Mnemonic) { typed = append(typed, m) }
	t.Cleanup(func() { unlockPassphraseWordsHook = nil })

	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	h.typePassphrase(vectorPassphrase(t, v))
	h.mustReach("SECRET seed material")

	if len(typed) != 1 {
		t.Fatalf("the word buffer was handed over %d times, want exactly 1", len(typed))
	}
	if !allZeroWords(typed[0]) {
		t.Errorf("the typed passphrase is STILL RESIDENT while the secret session is up: %v\n"+
			"§11.2 requires the passphrase buffer read as zeroed; unlockSealedFlow's "+
			"clear(m) must fire the instant unlockAttemptOnce returns", typed[0])
	}
}

// TestUnlockZeroesAPartialPassphraseOnTheWayOut — unlockPassphraseFlow's
// `clear(m)` on the partial-entry exit.
//
// inputWordsFlow returns on Back with whatever has been typed (gui/gui.go:704),
// so a part-typed passphrase is the ORDINARY shape of "the operator left" and
// not an error case. Nothing downstream ever sees this buffer -- the flow returns
// nil -- so without the seam there is no return value to assert on, which is how
// the deletion shipped green.
func TestUnlockZeroesAPartialPassphraseOnTheWayOut(t *testing.T) {
	var typed []bip39.Mnemonic
	unlockPassphraseWordsHook = func(m bip39.Mnemonic) { typed = append(typed, m) }
	t.Cleanup(func() { unlockPassphraseWordsHook = nil })

	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	// One word of twelve, then Back.
	h.typeWord("beef")
	h.tapNav(Button1)
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("backing out of word entry did not return to the menu")
	}
	if len(typed) != 1 {
		t.Fatalf("the word buffer was handed over %d times, want exactly 1", len(typed))
	}
	// The control: a buffer that was never typed into reads all -1, and this one
	// must have carried a real word before it was zeroed.
	if len(typed[0]) != 12 {
		t.Fatalf("the word buffer is %d words, §8 says twelve", len(typed[0]))
	}
	if !allZeroWords(typed[0]) {
		t.Errorf("a partly-typed passphrase survived the exit: %v\n"+
			"§11.2's wipe covers EVERY exit, and Back with a word typed is the most "+
			"ordinary one there is", typed[0])
	}
}
