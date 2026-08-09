package seal

import (
	"errors"
	"fmt"
	"strings"
)

// §10.2 steps 1-3 and 8-9, headless. One entry point Phase B calls: bytes in,
// parsed header + public records + the §6.6 hash out, and — given a passphrase,
// and only when something is encrypted — the decrypted records too.
//
// No UI, no globals, no lifetime management. Phase A holds decrypted records in
// memory and hands them back; §10.2.2's session lifecycle and §10.2.4's idle
// wipe belong to Phase B, which owns the lifetime.

// KDF is the key-derivation seam. It exists so a test can observe that NO KDF
// ran — the mutant "move the §6.2 bound checks after DeriveKey" passes every
// return-value assertion in this package, just ~31 s slower, and on a
// watchdog-less firmware that is the difference between a refusal and a hang.
type KDF func(passphrase string, salt []byte, iterations int) []byte

// Opener runs the pipeline. The zero value is valid and uses the real KDF.
type Opener struct {
	KDF KDF
}

// Payload is what Phase B renders. Records carry the classification that
// admitted them, so the plate list can label each entry by CLASSIFIED type —
// never by anything the sealer asserted.
type Payload struct {
	Header Header
	Public []AdmittedRecord
	Secret []AdmittedRecord

	// Hash is the §6.6 public-data hash, meaningful only when HasHash is true.
	// HasHash is false exactly when pub_len == 0: the digest of an empty record
	// set is a constant, and showing the same number on every fully-encrypted
	// payload would teach the operator it is furniture (§10.2 step 3).
	Hash    [16]byte
	HasHash bool

	// nPub carries the public record count from Inspect to Unlock, where the
	// §6.4 cross-section total is checked. Unexported: it is plumbing, not API.
	nPub int
}

// Wipe zeroes every admitted record's bytes. Phase B MUST call it on every exit
// path from a session (§10.2 step 10, §10.2.2). It is why AdmittedRecord.Record
// is []byte and not string: a Go string cannot be zeroed, so a secret returned
// as one is unwipeable by construction and §10.2.2's "each record is wiped as
// its plate leaves the screen" would have no implementation at all.
//
// Same caveat as the rest of the firmware: TinyGo's GC may copy or retain, so
// this is defence in depth, not a guarantee.
func (p *Payload) Wipe() {
	for _, r := range p.Secret {
		clear(r.Record)
	}
	for _, r := range p.Public {
		clear(r.Record)
	}
}

// NeedsPassphrase reports whether the payload carries an encrypted section.
// The passphrase prompt is conditional on this and NOTHING else: a public-only
// payload must never ask for one, because there is no key to derive and
// prompting would train the operator to type twelve words at a screen that
// cannot check them.
func (p *Payload) NeedsPassphrase() bool { return p.Header.Sealed() }

// NormalisePassphrase applies §8.1: lowercase, single-space separated, no
// leading or trailing space. Host and device MUST produce byte-identical KDF
// input, and a stray trailing space would otherwise buy a ~31 s KDF and an
// indistinguishable tag failure.
func NormalisePassphrase(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Open runs the pipeline over a payload region.
//
// On ANY failure it returns nil: rejection is whole-payload (§6.4). Partial
// acceptance would leave the operator engraving an incomplete wallet backup
// while believing it complete, which is the worst available outcome.
//
// passphrase is used only when the header says something is encrypted; when
// ct_len == 0 it is ignored entirely and no KDF runs.
func (o Opener) Open(blob []byte, passphrase string) (*Payload, error) {
	p, err := o.Inspect(blob)
	if err != nil {
		return nil, err
	}
	if !p.NeedsPassphrase() {
		return p, nil
	}
	if err := o.Unlock(blob, p, passphrase); err != nil {
		return nil, err
	}
	return p, nil
}

// Inspect runs §10.2 steps 1-3 ONLY: parse, bound-check, split, allow-list and
// card-set-decode the public section, and compute the §6.6 hash. It needs NO
// passphrase and runs NO KDF.
//
// This exists because §10.2 step 3 requires the hash displayed BEFORE word
// entry (step 5), and step 8 requires it kept on screen through the retry loop.
// A single Open that returns nil on a tag mismatch cannot deliver either: the
// hash would only ever be reachable AFTER a correct passphrase and a ~31 s KDF.
// Phase B calls Inspect, shows the hash, takes the words, then calls Unlock —
// and still holds the Payload, with its hash, when Unlock fails.
//
// Do NOT re-implement steps 1-3 in Phase B. Two code paths that must agree on
// the public record set is exactly the divergence one headless entry point
// exists to eliminate, and §6.6 is the only control an unsealed payload has.
func (o Opener) Inspect(blob []byte) (*Payload, error) {
	// Step 1 — parse and bound-check. EVERY §6.2 bound is checked here, before
	// any allocation or KDF work.
	h, err := ParseHeader(blob)
	if err != nil {
		return nil, err
	}
	// The header's lengths are authoritative (§6.2): a UF2 block carries a
	// fixed 256-byte payload, so the written region is the blob followed by
	// padding and then undefined sector bytes. Bound each section by its
	// declared length; never infer length from region contents.
	end := HeaderLen + int(h.PubLen) + int(h.CtLen)
	if h.Sealed() {
		end += TagLen
	}
	if len(blob) < end {
		return nil, fmt.Errorf("%w: region holds %d bytes, the header declares %d",
			ErrTooShort, len(blob), end)
	}
	split := HeaderLen + int(h.PubLen)

	p := &Payload{Header: h}

	// Step 2 — split, allow-list and card-set-decode the PUBLIC section. This
	// runs on UNAUTHENTICATED input: in the unencrypted shape there is no tag
	// at all, and in the sealed shape the tag has not been checked yet.
	var nPub int
	if h.PubLen > 0 {
		recs, n, err := SplitSection(blob[HeaderLen:split])
		if err != nil {
			return nil, describeRecordCount(err, n, 0)
		}
		nPub = n
		admitted, err := AdmitSection(recs, SectionPublic)
		if err != nil {
			return nil, err
		}
		p.Public = admitted

		// Step 3 — the §6.6 hash, computed from the records JUST PARSED and
		// never read from the payload. There is deliberately no hash field on
		// the wire: an attacker who rewrites the records rewrites the hash
		// beside them.
		// Public records only — cleartext by definition, so the copy is harmless.
		strs := make([]string, len(recs))
		for i, r := range recs {
			strs[i] = string(r)
		}
		p.Hash = PublicDataHash(strs, h.Sealed())
		p.HasHash = true
	}

	p.nPub = nPub
	return p, nil
}

// Unlock runs §10.2 steps 5-9 against a Payload that Inspect already produced.
// On failure p is left intact — its Header, Public records and Hash remain
// valid, which is what lets Phase B keep the hash on screen through the retry
// loop (§10.2 step 8).
//
// It is the derivation plus UnlockWithKey, and nothing else: there is ONE
// implementation of the open-split-allow-list pipeline in this package. The
// split exists because §10.2 step 7 needs a seam to draw frames in across the
// ~31 s derivation, and a single call that derives and opens has none.
func (o Opener) Unlock(blob []byte, p *Payload, passphrase string) error {
	h := p.Header
	end := HeaderLen + int(h.PubLen) + int(h.CtLen)
	if h.Sealed() {
		end += TagLen
	}

	// Unlock re-derives its offsets from p.Header, which came from a DIFFERENT
	// call (Inspect). Nothing forces the caller to pass the same blob back, so
	// bound-check the one actually handed to us before slicing it. crypto.go's
	// rule applies here too: a panic on a device is a brick, and DeriveKey and
	// Open both fail closed rather than panicking.
	//
	// Redundant with UnlockWithKey's own guard, deliberately: Unlock remains a
	// public entry point and bound-checks its own argument.
	if len(blob) < end {
		return fmt.Errorf("%w: region holds %d bytes, the header declares %d",
			ErrTooShort, len(blob), end)
	}

	// Step 4 — nothing encrypted: stop here. No passphrase, no KDF.
	//
	// This stays even though UnlockWithKey rejects the same shape, because the
	// two contracts DIFFER: Unlock's is "a payload with nothing encrypted is
	// not an error", UnlockWithKey's is "you gave me a key for a payload with
	// no ciphertext".
	if !h.Sealed() {
		return nil
	}

	// Steps 7-8 — derive, then open over AAD = header ‖ public section (§6.1a).
	// The AAD is taken from the blob's own bytes, so it binds version,
	// algorithm ids, iteration count, salt, IV, both lengths AND every public
	// record: altering one byte of a public card fails the tag exactly as
	// altering ciphertext does.
	derive := o.KDF
	if derive == nil {
		derive = DeriveKey
	}
	key := derive(NormalisePassphrase(passphrase), h.Salt[:], int(h.Iterations))
	// §10.2 step 10: wipe the derived key on EVERY exit path. Same honest caveat
	// as the rest of the firmware — TinyGo's GC may copy or retain, so this is
	// defence in depth, not a guarantee.
	defer clear(key)
	return o.UnlockWithKey(blob, p, key)
}

// describeRecordCount turns SplitSection's preallocated sentinel into the
// message §6.4 requires. The sentinel carries no count precisely so the
// allocation-free reject path stays allocation-free; naming the count is the
// caller's job, and this is the caller.
func describeRecordCount(err error, nPub, nSec int) error {
	if !errors.Is(err, ErrTooManyRecords) {
		return err
	}
	return recordCountError(nPub+nSec, nPub, nSec)
}

// recordCountError names the count and the cap.
//
// §6.4: "The device MUST distinguish 'too many records (N, max 24)' from
// 'payload unreadable'." record_count is authenticated plaintext, so naming it
// leaks nothing to anyone without the passphrase — and §6.2 uses "payload
// unreadable" for a corrupt or TAMPERED blob, which §2.2 item 4 has taught the
// operator to read as "someone replaced my payload". Conflating a too-large
// wallet with an attack would send them chasing a compromise that did not
// happen.
func recordCountError(total, nPub, nSec int) error {
	return fmt.Errorf("%w: %d records (%d public + %d encrypted); the cap is %d across both sections "+
		"(a 2-of-3 multisig bundle is 15)", ErrTooManyRecords, total, nPub, nSec, MaxRecords)
}
