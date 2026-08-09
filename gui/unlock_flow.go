package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/seal"
)

// §10 device side, Plan B Phase B1 — the UNSEALED path.
//
// B1 never derives a key, never decrypts and never holds a secret record, so
// none of §10.2.2's session lifecycle or §10.2.4's idle wipe lives here. What
// guarantees the public section carries nothing secret is §10.2.1's allow-list,
// which is Phase A code (seal.AdmitSection) and already vector-tested.

// unlockTitle is the operator-facing name of the whole feature, and the same
// string the menu entry carries.
const unlockTitle = "Sealed Payload"

// unlockPayloadFlow is the unlockPayload program (§10.2). r is the payload
// region's reader, PROBED once at GUI start (§10.1) and READ here — F-79:
// startup retains the reader, never the 65,536 bytes.
func unlockPayloadFlow(ctx *Context, th *Colors, r seal.Reader) {
	// Structurally unreachable: lastNav() returns bip85Derive when no payload
	// was probed, and both carousel wrap sites bound m.prog by it, so the
	// unlockPayload case cannot be dispatched with a nil reader. Guarded anyway,
	// for the reason crypto.go guards a wrong-length IV and pbkdf2.go a nil mac
	// -- a panic on a watchdog-less device is a brick, and this costs one
	// comparison. Before F-79 the same mistake was harmless: the parameter was a
	// nil []byte and Inspect reported it as unreadable.
	if r == nil {
		showError(ctx, th, unlockTitle, "Payload unreadable.")
		return
	}
	blob, err := r.Read()
	if err != nil {
		// ErrNoPayload here means the region was erased between the startup
		// probe and now, which takes a picotool run and a reboot. Report it the
		// same as any unreadable region rather than inventing a third message.
		showError(ctx, th, unlockTitle, "Payload unreadable.")
		return
	}
	// F-79. The region is this flow's for the duration of the session and
	// nobody else's. clear() zeroes it rather than merely dropping it: the AAD
	// and the ciphertext both live in here, and TinyGo will not necessarily
	// collect it before the engrave that follows needs the heap.
	//
	// A CLOSURE, not `defer clear(blob)` (R0 round 0, I1). Deferred call
	// arguments are evaluated when the defer STATEMENT runs, so `defer
	// clear(blob)` would capture the slice header here and pin all 65,536 bytes
	// for the whole flow -- B2a-ii's `blob = nil` would rebind the local and
	// release nothing, and F-79 would be reported closed while unfixed in
	// exactly the payload-present-plus-running-engrave configuration B2a-ii's
	// Task 9.5 exists to test. The closure reads `blob` at EXIT instead.
	defer func() { clear(blob) }()
	// Steps 1-3, headless. Do NOT re-implement them here: two code paths that
	// must agree on the public record set is the divergence one entry point
	// exists to eliminate, and §6.6 is the only control an unsealed payload
	// has.
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		// §6.2/§6.4: every violation reads as "payload unreadable" to the
		// operator, EXCEPT too-many-records, which §6.4 requires be
		// distinguishable. "Unreadable" is what the operator has been taught to
		// read as "someone replaced my payload"; conflating a too-large wallet
		// with an attack would send them chasing a compromise that did not
		// happen.
		if errors.Is(err, seal.ErrTooManyRecords) {
			showError(ctx, th, unlockTitle,
				"This payload declares more records than the machine accepts.")
			return
		}
		showError(ctx, th, unlockTitle, "Payload unreadable.")
		return
	}
	// §10.2 step 10 / §10.2.2: "Lock, Back, an error, ctx.Done" are ONE exit,
	// not four. Registered the moment there is anything to wipe, so every route
	// out of this flow -- including a panic unwind -- zeroes every record in
	// BOTH sections. §10.2.2's per-record wipe is finer-grained and runs first;
	// this is the backstop that catches whatever it did not reach.
	defer p.Wipe()

	// Step 3 — the hash, shown ONLY when the payload has a public section.
	// HasHash is false exactly when pub_len == 0, and the digest of an empty
	// record set is a CONSTANT: showing the same number on every fully
	// encrypted payload would teach the operator it is furniture.
	if p.HasHash {
		showNotice(ctx, th, "Public Data Hash", unlockHashBody(p))
	}

	// §10.2 steps 5-9. On failure or cancellation this returns WITHOUT reaching
	// the plate list -- see unlockSealedFlow's contract.
	if p.Header.Sealed() {
		if !unlockSealedFlow(ctx, th, blob, p) {
			return
		}
		// F-79: the blob's last use was UnlockWithKey's AAD and ciphertext. Zero
		// and drop it BEFORE the session, so the engrave that follows does not
		// run with the region still on the heap. This releases it only because
		// the deferred clear above is a CLOSURE reading `blob` at exit; a
		// `defer clear(blob)` would still hold the array (I1).
		//
		// Safe because AdmitSection COPIES every record
		// (seal/record.go:207, `append([]byte(nil), r...)`), so p.Public and
		// p.Secret do not alias this buffer.
		clear(blob)
		blob = nil
		// §10.2.2 -- every secret record offered FIRST, consecutively, each
		// wiped as its plate leaves the screen by any route.
		unlockSecretSession(ctx, th, p)
		unlockPlateListFlow(ctx, th, unlockPlates(p))
		return
	}

	// Step 4 — ct_len == 0. No passphrase is prompted; the §10.2.3 warning is
	// shown instead and must be explicitly confirmed.
	if !unlockWarnUnauthenticated(ctx, th, p) {
		return
	}
	// Both paths build the list the same way, which is also what stops the
	// unsealed path from silently keeping the old labels.
	unlockPlateListFlow(ctx, th, unlockPlates(p))
}

// unlockHashBody renders §10.2 step 3: the §6.6 digest, the PUBLIC record count
// and the sealed/unsealed shape.
//
// The count is len(p.Public) and NEVER len(p.Public)+len(p.Secret). §6.6 hashes
// the public record count while §6.4's 1..24 cap counts both sections — vector
// D is 5 public of 6 total, and the two counts produce DIFFERENT digests. A
// screen displaying the total beside a hash computed over the public count
// teaches the operator that mismatches are normal, which disarms the only
// control an unsealed payload has.
//
// The digest goes through seal.FormatHash and is never regrouped locally: the
// eight-groups-of-four shape is what makes it comparable against what the
// operator wrote down.
func unlockHashBody(p *seal.Payload) string {
	return fmt.Sprintf("Public data hash (%d records, %s):\n\n%s\n\nCompare this with the value you recorded.",
		len(p.Public), unlockShape(p), seal.FormatHash(p.Hash))
}

func unlockShape(p *seal.Payload) string {
	if p.Header.Sealed() {
		return "SEALED"
	}
	return "UNSEALED"
}

// unlockWarnUnauthenticated is §10.2.3, shown when and only when ct_len == 0.
// It returns true only on an explicit confirmation.
//
// The copy is NORMATIVE and reproduced from §10.2.3. It must keep saying that
// the hash is something the OPERATOR compares, and must never imply the device
// verified anything: there is no key here, so there is nothing to verify with.
//
// ConfirmWarningScreen is existing machinery -- scrollable body, cancel, and a
// hold-to-confirm that is the "explicit confirmation" §10.2 step 4 requires.
func unlockWarnUnauthenticated(ctx *Context, th *Colors, p *seal.Payload) bool {
	warn := &ConfirmWarningScreen{
		Title: "Not Authenticated",
		Body: "THIS PAYLOAD IS NOT AUTHENTICATED\n\n" +
			"It carries no encrypted data, so there is no key and nothing proves it is " +
			"the payload you sent. Anyone with physical access could have replaced it.\n\n" +
			fmt.Sprintf("Public data hash (%d records, UNSEALED):\n\n%s\n\n", len(p.Public), seal.FormatHash(p.Hash)) +
			"Compare this with the value you recorded.\n\n" +
			"If you sealed this payload with a passphrase, the encrypted part has been " +
			"REMOVED. Do not continue.",
		Icon: assets.IconCheckmark,
	}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, res := warn.Layout(ctx, th, dims)
		switch res {
		case ConfirmNo:
			return false
		case ConfirmYes:
			return true
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
	return false
}
