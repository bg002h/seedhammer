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

// unlockPayloadFlow is the unlockPayload program (§10.2). blob is the payload
// region as read ONCE at GUI start (§10.1).
func unlockPayloadFlow(ctx *Context, th *Colors, blob []byte) {
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

	// Step 3 — the hash, shown ONLY when the payload has a public section.
	// HasHash is false exactly when pub_len == 0, and the digest of an empty
	// record set is a CONSTANT: showing the same number on every fully
	// encrypted payload would teach the operator it is furniture.
	if p.HasHash {
		showNotice(ctx, th, "Public Data Hash", unlockHashBody(p))
	}

	// B1 stops here for a sealed payload (§10.2 steps 5-9 are B2). It must not
	// fall through to the plate list: p.Public on a sealed payload is a
	// legitimate record set, and engraving it while silently dropping the
	// encrypted half is §6.4's incomplete-backup-believed-complete, the worst
	// available outcome.
	if p.Header.Sealed() {
		showError(ctx, th, unlockTitle,
			"This payload is sealed with a passphrase.\n\n"+
				"Unlocking is not available in this build, so none of it can be engraved yet.")
		return
	}

	// Step 4 — ct_len == 0. No passphrase is prompted; the §10.2.3 warning is
	// shown instead and must be explicitly confirmed.
	if !unlockWarnUnauthenticated(ctx, th, p) {
		return
	}
	unlockPlateListFlow(ctx, th, p.Public)
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
