package seal

import (
	"errors"
	"fmt"
)

// ErrNotSealed is UnlockWithKey called on a payload that carries no encrypted
// section. It is a programming error, not an operator-visible condition: §10.2
// step 4 stops before the passphrase when ct_len == 0, so reaching here means a
// caller skipped that check.
var ErrNotSealed = errors.New("seal: payload carries no encrypted section")

// unlockPlaintextHook hands over the DECRYPTED RECORD CONTAINER at the moment
// UnlockWithKey takes ownership of it. nil in production.
//
// §11.2 requires the plaintext record buffer be asserted zeroed ON THE BUFFER
// ITSELF, and this one is unreachable without a seam: it is gcm.Open's own
// allocation, AdmitSection copies out of it (seal/record.go), and no handle
// escapes this function. It is also the widest such buffer in the firmware --
// every ms1 and every bare mnemonic in the payload in ONE array that neither
// Payload.Wipe, WipeSecretAt nor SecretsResident can reach -- so a deleted
// `defer clear(plaintext)` leaves a full plaintext copy of the seed live for the
// rest of the power cycle with nothing able to notice.
var unlockPlaintextHook func(plaintext []byte)

// UnlockWithKey is §10.2 steps 8-9 against a key the caller already derived.
//
// It exists because §10.2 step 7 requires a progress indicator over a ~31 s
// derivation, and a single call that derives and opens leaves no seam to draw
// a frame in. Unlock is now this function plus the derivation, so there is one
// implementation of the open-split-allow-list pipeline and not two.
//
// On failure p is left INTACT — Header, Public and Hash stay valid — which is
// what lets Phase B keep the §6.6 hash on screen through the retry loop
// (§10.2 step 8).
//
// The key is the caller's to wipe. This function neither zeroes nor retains it.
func (o Opener) UnlockWithKey(blob []byte, p *Payload, key []byte) error {
	h := p.Header
	if !h.Sealed() {
		return ErrNotSealed
	}
	// Bound the HEADER before deriving offsets from it, not just the blob
	// afterwards. p.Header is an exported struct with exported uint32 lengths
	// and this is an exported entry point, so the only thing standing between a
	// caller-supplied Header and the slice expressions below is a check made in
	// a DIFFERENT function (ParseHeader). §6.2 requires the length arithmetic be
	// done in unsigned arithmetic wider than 32 bits or be otherwise
	// overflow-checked, and `int(uint32)` on this target REINTERPRETS rather
	// than widens: TinyGo's int is 32-bit on RP2350, so pub_len = 0x80000000
	// makes `end` NEGATIVE, `len(blob) < end` false, and `blob[:split]` a panic
	// -- a brick on a watchdog-less device.
	//
	// Measured under GOARCH=386, without this clause:
	//	int is 32 bits; split=-2147483596 end=-2147483480
	//	len(blob) < end ? false
	//	PANIC: runtime error: slice bounds out of range [:-2147483596]
	//
	// NOT reachable from a hostile payload: Inspect is the only in-tree producer
	// of a Payload, ParseHeader caps both lengths at MaxSectionLen in uint64
	// arithmetic before the struct is built, and nothing assigns p.Header.
	// This closes the gap between that fact and this function's own contract.
	if h.PubLen > MaxSectionLen || h.CtLen > MaxSectionLen {
		return fmt.Errorf("%w: the header declares pub_len=%d ct_len=%d, the cap is %d",
			ErrTooLarge, h.PubLen, h.CtLen, MaxSectionLen)
	}
	end := HeaderLen + int(h.PubLen) + int(h.CtLen) + TagLen
	split := HeaderLen + int(h.PubLen)
	// The offsets come from p.Header, which came from a DIFFERENT call
	// (Inspect). Nothing forces the caller to hand back the same blob, so
	// bound-check the one actually passed before slicing it: on a device a
	// panic is a brick.
	if len(blob) < end {
		return fmt.Errorf("%w: region holds %d bytes, the header declares %d",
			ErrTooShort, len(blob), end)
	}
	// AAD = header || public section (§6.1a), taken from the blob's own bytes,
	// so it binds version, algorithm ids, iteration count, salt, IV, both
	// lengths AND every public record.
	plaintext, err := Open(key, h.IV[:], blob[:split], blob[split:end])
	if err != nil {
		// Fail closed. ErrAuthentication, and Phase B must offer BOTH readings.
		return err
	}
	// The plaintext buffer is ours; the records copied out of it are wiped by
	// Payload.Wipe, which Phase B owns.
	defer clear(plaintext)
	if unlockPlaintextHook != nil {
		unlockPlaintextHook(plaintext)
	}
	recs, nSec, err := SplitSection(plaintext)
	if err != nil {
		return describeRecordCount(err, p.nPub, nSec)
	}
	// §6.4's 1..24 cap is over the TOTAL across BOTH sections, which
	// SplitSection cannot see — this is the only place the cross-section total
	// is known.
	if total := p.nPub + nSec; total > MaxRecords {
		return recordCountError(total, p.nPub, nSec)
	}
	admitted, err := AdmitSection(recs, SectionEncrypted)
	if err != nil {
		return err
	}
	// Wipe any secrets a PREVIOUS unlock left here before dropping the
	// reference. Overwriting p.Secret makes those bytes unreachable, so Phase B
	// calling p.Wipe() faithfully at session end would still miss them.
	for _, r := range p.Secret {
		clear(r.Record)
	}
	p.Secret = admitted
	return nil
}
