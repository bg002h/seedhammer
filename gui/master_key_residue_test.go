package gui

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// ---------------------------------------------------------------------------
// F-94 -- the 64-byte BIP-39 seed and the BIP-32 master private key are ZEROED
// on every path and NOTHING PINNED IT. All three wipes added by 3c477b9
// (`defer wipeBytes(seed)` in deriveMasterKey, `defer mk.Zero()` in
// masterFingerprintFor, and `mk.Zero()` at the SeedScreen validity probe) could
// be deleted with the whole suite green -- "deletable green", the exact shape
// of the Critical this thread began with.
//
// A test that passes today is worth nothing here unless it is guaranteed to
// FAIL tomorrow, so every test below is written to be killed by deleting the
// one line it pins, and each one first establishes -- by comparison against an
// authoritative value, not by inspection -- that it captured REAL secret
// material rather than an empty or already-zero buffer. A test that reads an
// unrelated allocation and reports success is the failure mode this file
// exists to avoid.
// ---------------------------------------------------------------------------

// BIP-39 English test vector 1 (github.com/trezor/python-mnemonic
// vectors.json, the vector this repo's bip39 corpus is drawn from), used here
// as the POSITIVE CONTROL: entropy 00000000000000000000000000000000 ->
// "abandon ... about", and with passphrase "TREZOR" the 64-byte seed below.
// Recomputed on this tree before it was written down.
const (
	abandonMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	abandonSeedHex  = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e534955" +
		"31f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"
)

// TestDeriveMasterKeyZeroesTheBIP39Seed pins `defer wipeBytes(seed)`
// (gui/gui.go, deriveMasterKey).
//
// HOW IT KNOWS IT IS LOOKING AT THE RIGHT MEMORY. deriveSeedHook hands over the
// seed slice VALUE, and a Go slice value carries the pointer to its backing
// array -- so `captured` and deriveMasterKey's own `seed` name the same array,
// and wipeBytes writes through that array. There is no copy anywhere on this
// path and no reallocation: bip39.MnemonicSeed returns pbkdf2.Key's single
// 64-byte result and nothing appends to it. The read after the call therefore
// observes the SAME allocation the wipe wrote to, not a fresh one.
//
// POSITIVE CONTROL. Before asserting zero, the test asserts the snapshot taken
// while the buffer was live equals the published BIP-39 seed for this mnemonic
// and passphrase. An all-zero read is only evidence of a wipe if the same bytes
// were demonstrably the seed a moment earlier; without that, a test that
// captured the wrong (or an empty) buffer would report success. See
// TestDeriveSeedPinFailsWhenTheSeedIsNotWiped for the negative half.
func TestDeriveMasterKeyZeroesTheBIP39Seed(t *testing.T) {
	mn := bip39FromWords(t, abandonMnemonic)

	var captured []byte
	var snapshot []byte
	deriveSeedHook = func(seed []byte) {
		captured = seed
		snapshot = append([]byte(nil), seed...)
	}
	t.Cleanup(func() { deriveSeedHook = nil })

	mk, ok := deriveMasterKey(mn, &chaincfg.MainNetParams, "TREZOR")
	if !ok {
		t.Fatal("deriveMasterKey failed on BIP-39 test vector 1")
	}
	mk.Zero() // the returned key is the caller's to scrub; this test is the caller

	if captured == nil {
		t.Fatal("deriveSeedHook never fired -- this test asserted nothing")
	}
	if len(captured) != 64 {
		t.Fatalf("captured %d bytes, want the 64-byte BIP-39 seed", len(captured))
	}
	want, err := hex.DecodeString(abandonSeedHex)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot, want) {
		t.Fatalf("the hook did not hand over the real BIP-39 seed:\n got %x\nwant %x\n"+
			"the rest of this test would be asserting zero on the wrong buffer", snapshot, want)
	}
	for i, b := range captured {
		if b != 0 {
			t.Fatalf("byte %d of the 64-byte BIP-39 seed is still %#02x after deriveMasterKey "+
				"returned: `defer wipeBytes(seed)` did not run, and a full seed-equivalent "+
				"buffer is live on the heap (F-94)", i, b)
		}
	}
}

// TestDeriveSeedPinFailsWhenTheSeedIsNotWiped is the POSITIVE CONTROL for the
// instrument above: an identical capture-then-read, over a buffer nothing
// wipes. It asserts the read comes back NON-zero.
//
// Its job is to prove the "is it zero" check can distinguish a wiped buffer
// from a live one at all. If Go handed the test a fresh allocation on the
// second read -- the subtle failure the F-94 brief warns about -- this test
// would see zeroes and fail, so a green run here is evidence the pin above is
// reading the array it captured and not a different one.
func TestDeriveSeedPinFailsWhenTheSeedIsNotWiped(t *testing.T) {
	mn := bip39FromWords(t, abandonMnemonic)

	var captured []byte
	unwiped := func() {
		// deriveMasterKey without the wipe: the same call, same allocation,
		// minus the one line under test.
		seed := bip39.MnemonicSeed(mn, "TREZOR")
		captured = seed
		if _, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams); err != nil {
			t.Fatalf("NewMaster: %v", err)
		}
	}
	unwiped()

	if len(captured) != 64 {
		t.Fatalf("captured %d bytes, want 64", len(captured))
	}
	if allZeroBytes(captured) {
		t.Fatal("the unwiped seed read back all-zero: the test is observing a different " +
			"allocation than the one it captured, so every 'it is zero' assertion in this " +
			"file would be vacuous")
	}
	want, err := hex.DecodeString(abandonSeedHex)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(captured, want) {
		t.Fatalf("unwiped seed = %x, want %x", captured, want)
	}
	clear(captured)
}

// TestMasterFingerprintForZeroesTheMasterPrivateKey pins `defer mk.Zero()`
// (gui/gui.go, masterFingerprintFor).
//
// HOW IT KNOWS IT IS LOOKING AT THE RIGHT MEMORY. hdkeychain.ExtendedKey.Zero
// zeroes k.key, k.pubKey, k.chainCode and k.parentFP in place and only then
// nils k.version and k.key. k.chainCode is NOT nilled, so the exported
// ChainCode() accessor keeps reading the SAME backing array before and after --
// it copies out of it on each call. Reading it twice through the captured
// *ExtendedKey is therefore a genuine before/after of one allocation, not two
// separate observations.
//
// The private key bytes themselves have no exported accessor, so the test also
// asserts IsPrivate() flipped to false: within hdkeychain's Zero that
// assignment happens strictly after zero(k.key), so it cannot be observed
// unless the key bytes were wiped first. The two together mean "Zero ran on
// this object".
//
// POSITIVE CONTROL: the chain code read inside the hook must be a non-zero
// 32-byte value and the key must report IsPrivate() -- i.e. the test held a
// live master PRIVATE key -- before the post-return read is allowed to mean
// anything.
func TestMasterFingerprintForZeroesTheMasterPrivateKey(t *testing.T) {
	mn := bip39FromWords(t, abandonMnemonic)

	var captured *hdkeychain.ExtendedKey
	var liveChainCode []byte
	var livePrivate bool
	deriveMasterKeyHook = func(k *hdkeychain.ExtendedKey) {
		captured = k
		liveChainCode = k.ChainCode()
		livePrivate = k.IsPrivate()
	}
	t.Cleanup(func() { deriveMasterKeyHook = nil })

	if _, err := masterFingerprintFor(mn, &chaincfg.MainNetParams, ""); err != nil {
		t.Fatalf("masterFingerprintFor: %v", err)
	}

	if captured == nil {
		t.Fatal("deriveMasterKeyHook never fired -- this test asserted nothing")
	}
	if !livePrivate {
		t.Fatal("the captured key was not a PRIVATE key while live; this test is not " +
			"watching the master private key at all")
	}
	if len(liveChainCode) != 32 || allZeroBytes(liveChainCode) {
		t.Fatalf("the captured key's chain code was %d bytes and all-zero=%v while live; "+
			"the post-return read below would prove nothing",
			len(liveChainCode), allZeroBytes(liveChainCode))
	}

	post := captured.ChainCode()
	if len(post) != 32 {
		t.Fatalf("ChainCode() returned %d bytes after the call, want 32", len(post))
	}
	if !allZeroBytes(post) {
		t.Fatalf("the master key's chain code is still %x after masterFingerprintFor "+
			"returned: `defer mk.Zero()` did not run and the BIP-32 master private key "+
			"is live on the heap (F-94)", post)
	}
	if captured.IsPrivate() {
		t.Fatal("the master key still reports IsPrivate() after masterFingerprintFor " +
			"returned: hdkeychain.Zero clears that flag only after zeroing the key bytes, " +
			"so `defer mk.Zero()` did not run (F-94)")
	}
}

// TestSeedScreenProbeZeroesTheDiscardedMasterKey pins the `mk.Zero()` beside
// the SeedScreen validity probe (gui/gui.go, SeedScreen.Confirm).
//
// That probe wants only the `ok`; the live master private key it also gets is
// discarded, which before 3c477b9 left seed-equivalent material unscrubbed on
// the ORDINARY seed-entry path -- the one every typed seed takes, not just the
// sealed-payload one.
//
// Memory and control as in TestMasterFingerprintForZeroesTheMasterPrivateKey.
// SeedScreen.Confirm is the only caller of deriveMasterKey in this flow
// (masterFingerprintFor runs later, in Confirm's CALLER), so the hook fires
// exactly once and the key it hands over is the probe's.
func TestSeedScreenProbeZeroesTheDiscardedMasterKey(t *testing.T) {
	m := validMnemonic(12)

	var captured *hdkeychain.ExtendedKey
	var liveChainCode []byte
	var livePrivate bool
	fires := 0
	deriveMasterKeyHook = func(k *hdkeychain.ExtendedKey) {
		fires++
		captured = k
		liveChainCode = k.ChainCode()
		livePrivate = k.IsPrivate()
	}
	t.Cleanup(func() { deriveMasterKeyHook = nil })

	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	ss := &SeedScreen{}
	confirmed, returned := false, false
	frame, drawer, quit := runUITouch(ctx, func() {
		confirmed = ss.Confirm(ctx, &descriptorTheme, m)
		returned = true
	})
	defer quit()

	if _, ok := frame(); !ok {
		t.Fatal("SeedScreen produced no frame")
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	// Confirm returns straight out of the probe with no further ctx.Frame, so
	// the next pull ends the iterator rather than yielding a frame.
	for range 16 {
		if _, ok := frame(); !ok {
			break
		}
	}
	if !returned {
		t.Fatal("SeedScreen.Confirm never returned after the seed was confirmed")
	}
	if !confirmed {
		t.Fatal("SeedScreen.Confirm rejected a checksum-valid mnemonic")
	}

	if fires != 1 {
		t.Fatalf("deriveMasterKeyHook fired %d times, want exactly 1 (the validity probe); "+
			"the assertions below may be reading a different key", fires)
	}
	if !livePrivate {
		t.Fatal("the probe's key was not a PRIVATE key while live")
	}
	if len(liveChainCode) != 32 || allZeroBytes(liveChainCode) {
		t.Fatalf("the probe's chain code was %d bytes and all-zero=%v while live",
			len(liveChainCode), allZeroBytes(liveChainCode))
	}

	post := captured.ChainCode()
	if !allZeroBytes(post) {
		t.Fatalf("the probe's discarded master key still has chain code %x after "+
			"SeedScreen.Confirm returned: the `mk.Zero()` beside the validity probe did "+
			"not run (F-94)", post)
	}
	if captured.IsPrivate() {
		t.Fatal("the probe's discarded master key still reports IsPrivate() after " +
			"SeedScreen.Confirm returned: `mk.Zero()` did not run (F-94)")
	}
}
