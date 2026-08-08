package gui

// §10 device side, Plan B Phase B1 — the UNSEALED path.
//
// B1 never derives a key, never decrypts, and never holds a secret record, so
// none of §10.2.2's session lifecycle or §10.2.4's idle wipe lives here. What
// guarantees the public section carries nothing secret is §10.2.1's allow-list,
// which is Phase A code (seal.AdmitSection) and already vector-tested.

// unlockPayloadFlow is the unlockPayload program (§10.2). blob is the payload
// region as read once at GUI start (§10.1).
//
// Task 1 wires the conditional menu entry; the body is Task 2's.
func unlockPayloadFlow(ctx *Context, th *Colors, blob []byte) {
	showNotice(ctx, th, "Sealed Payload", "Reading the payload is not wired up yet.")
}
