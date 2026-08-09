package gui

// §10.2.4's residency seam.
//
// "Resident" is a LIFETIME, not a buffer scan (§10.2.4 as amended 2026-08-09).
// seal.RecordsResident() reads false from the instant a plate is built, while
// the flow still holds codex32.String, the parsed words, and the plate's SPLINE
// CLOSURE -- an iter.Seq over the plaintext, not a rendering (F-83 as
// corrected). The spec therefore FORBIDS that predicate as the timer's key, and
// this guard's lifetime is the key instead.
//
// The bracket is unlockSecretSession's own first and last act, so the window is
// exactly "secrets decrypted and being offered" to "the last secret plate has
// left the screen". The gaps at its edges are frame-free straight-line code.
type wipeGuard struct {
	// job is the engrave job currently cutting a secret plate, nil otherwise.
	// Registered by the two unlock engrave arms around their Engrave call.
	job *engraveJob
}

// wipeNowHook forces a wipe on Run's next tick. Nil in production.
//
// It exists so the UNWIND can be tested on the commit that introduces it:
// §10.2.4's timer arrives a task later, and `wiping` is a local inside
// runWithFlow with no other seam, so without this there is no reachable path
// that sets it and none of the unwind's mutation rows can be run.
var wipeNowHook func() bool

// armed reports whether §10.2.4's timer should be running.
//
// nil receiver -- no secret session open -- is the overwhelmingly common case
// and costs two nil checks per Run tick.
//
// A RUNNING job disarms it: §10.2.4 row 2, never wipe mid-plate with the needle
// down. engraveStopping is listed for completeness rather than because it is
// reachable here -- Engrave CAN return in that state (gui/gui.go:2703), and
// the deferred g.job = nil then disarms anyway. Note what is deliberately NOT
// here: screen visibility. Row 2 as amended keys on the JOB, so the
// hold-to-start and plate-done screens are ARMED -- they are walk-away states
// with secrets still held.
func (g *wipeGuard) armed() bool {
	if g == nil {
		return false
	}
	if j := g.job; j != nil {
		switch j.Status().State {
		case engraveRunning, engraveStopping:
			return false
		}
	}
	return true
}
