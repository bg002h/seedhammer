package gui

import "testing"

// ---------------------------------------------------------------------------
// F-87, the residue -- unlockEngraveMnemonic's THIRD early return.
//
// The three early returns (!ss.Confirm, masterFingerprintFor err, engraveSeed
// err) share ONE `defer clear(m)` over bip39.Parse's independent []Word copy of
// the seed. Two of them are pinned in unlock_session_test.go. The
// masterFingerprintFor leg was not, and unlike the other two it has no natural
// trigger: it fires only when hdkeychain.NewMaster rejects the derived key,
// which is a 1-in-2^127 event. No mnemonic, Platform or network argument
// reaches it, so the leg was covered by nothing at all.
//
// Why the leg is worth a test even though deleting the defer outright is
// already caught: the residual risk F-87 names is not deletion, it is a
// "simplification" that replaces the shared defer with explicit clear() calls
// and misses one return. A mutant of that shape is invisible to the other two
// tests and visible to this one.
//
// The seam this needs (masterFingerprintFailHook) is the one place in gui.go
// where a test hook changes behaviour instead of observing it, and it is
// written to fail CLOSED -- see its doc comment.
// ---------------------------------------------------------------------------

// TestUnlockEngraveMnemonicZeroesMOnFingerprintError -- the ":274" leg.
//
// The seed is confirmed on SeedScreen (so ss.Confirm's own validity probe runs
// for real), masterFingerprintFor then reports failure, the flow shows
// "Couldn't derive the fingerprint for this seed." and returns. Every word of
// bip39.Parse's copy must read zero afterwards.
//
// Memory: unlockMnemonicParsedHook (gui/unlock_mnemonic_seam.go) fires
// immediately after `defer clear(m)` is registered and hands over the slice
// value, so the test holds the SAME backing array the defer will zero -- the
// flow neither copies nor reallocates it (bip39.Parse preallocates to cap 24
// precisely so append never orphans). assertMnemonicZeroed refuses to pass on
// a hook that never fired, which is what makes the zero-read mean something.
func TestUnlockEngraveMnemonicZeroesMOnFingerprintError(t *testing.T) {
	p := unlockedPayload(t, "A")
	rec := append([]byte(nil), p.Secret[0].Record...)
	got := watchMnemonicParsed(t)

	fires := 0
	masterFingerprintFailHook = func() bool { fires++; return true }
	t.Cleanup(func() { masterFingerprintFailHook = nil })

	pf := newPlatform()
	pf.display = sh2DisplaySize
	h := runUnlockEngraveMnemonic(t, pf, rec)

	h.mustReach("EngraveSeed")
	h.tapNav(Button3) // confirm the seed -- checksum-valid, so this accepts
	h.mustReach("derive the fingerprint")
	h.tapNav(Button3) // dismiss the error modal (ErrorScreen.ok is bound to Button3)
	// Deliberately not mustReach/h.next(): dismissing the modal makes showModal
	// return with nothing left to draw and unlockEngraveMnemonic's own early
	// return follows immediately -- no further frame is expected, only the flow
	// ending. Same shape as TestUnlockEngraveMnemonicZeroesMOnEngraveSeedError.
	if c, ok := h.frame(); ok {
		h.content = c
	}

	if fires != 1 {
		t.Fatalf("masterFingerprintFailHook fired %d times, want exactly 1 -- the flow did "+
			"not take the fingerprint leg, so this test did not exercise it", fires)
	}
	if !*h.done {
		t.Fatal("unlockEngraveMnemonic never returned after the fingerprint error was shown")
	}
	assertMnemonicZeroed(t, *got)
}
