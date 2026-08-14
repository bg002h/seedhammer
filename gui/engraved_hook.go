//go:build !tinygo

// The engraved-text seam: which md1/mk1/ms1 string each finished plate carried.
// The third build-tagged pair in this package; plate_hook.go states the general
// argument for the split and frame_hook.go the measurement discipline around
// it.
//
// WHAT IT IS FOR. §4.5 makes an emulator walk the closing gate of every stage,
// and the gate is a BYTE COMPARISON: the strings that came out must be the
// strings the inputs require. Nothing above this line could produce them. The
// toolpath recorder decodes geometry, gui.PlateAware hands over geometry, and
// no exported call turns an arbitrary md1 back into a plate -- gui.BuildPreview
// takes only the named canned plates. So the walk could count plates and could
// not say what was on them.
//
// WHY IT IS TWO EVENTS AND AN ID, WHICH IS THE WHOLE DESIGN.
//
// The obvious shape -- put the string on [Plate] -- IS A FUNDS-SAFETY
// REGRESSION, and specifically it breaks §10.2.2. gui/unlock_session.go clears
// the decrypted record BEFORE the engrave screen is built, and says why in
// full: the plate carries the GEOMETRY, so nothing reads those bytes again and
// the seed need not stay resident for a ~21-minute cut. A string field would
// hold that same material for the whole cut, as a Go string, which cannot be
// zeroed at all. The comment defending that clear would still be there and
// would silently have become false.
//
// So the string never enters Plate. What enters Plate is an opaque id -- a
// number, carrying nothing -- and the string travels separately, on a hook the
// firmware does not contain. PlateText announces "these ids were built from
// this string" at the one place every constellation engrave passes through
// (validateMdmk); PlateEngraved announces "this id finished" from the one place
// an engrave completes.
//
// THE ID IS WHAT MAKES THE CENSUS HONEST, and it is not ceremony. Without it a
// consumer would have to assume the last string it heard about belongs to the
// next plate that finishes -- and that assumption is false in a way that
// INFLATES the census: validate an md1, back out of the picker, later finish an
// unrelated seed plate, and the md1 is recorded as engraved when no such plate
// exists. A gate that reports a plate nobody cut is worse than no gate.
//
// Plates that never went through validateMdmk carry id 0 and are never
// announced, so a seed, a passphrase or a free-text plate cannot enter the
// count by accident.
package gui

// EngravedAware is an OPTIONAL interface a [Platform] may implement to learn
// which md1/mk1/ms1 string each engraved plate carried.
//
// It is on the Platform for [FrameAware]'s reason rather than [PlateAware]'s:
// the two events are separated by an operator choosing a variant and holding a
// button, so no per-job object is alive across both. The Platform is.
//
// The contract is deliberately weak in one direction and strict in the other. A
// PlateText may never be followed by a PlateEngraved -- the operator backs out,
// which is an ordinary thing to do and not an error. A PlateEngraved for an id
// that was never announced means the plate did not come from validateMdmk, and
// an implementation must ignore it rather than guess.
type EngravedAware interface {
	// PlateText reports that every id in ids renders text -- one id per
	// engraving variant of the same string, since the operator has yet to
	// choose between TEXT+QR, TEXT ONLY and QR ONLY and any of them may be the
	// one that gets cut.
	PlateText(ids []uint64, text string)
	// PlateEngraved reports that the plate with this id finished and the
	// operator confirmed it. Only then has anything been engraved: a job that
	// runs and is aborted never reaches here.
	PlateEngraved(id uint64)
}

// notifyPlateText offers the plates' ids to pl, if pl wants them.
//
// It takes the PLATES and builds the id slice itself, rather than taking a
// slice validateMdmk built. Two reasons, and the second is the load-bearing
// one: Plate.id is unexported, so a consumer outside this package cannot read
// it and the ids have to be lifted out somewhere -- and doing it HERE means the
// allocation lives in a file the firmware does not compile. Built at the call
// site instead, the device allocated a []uint64 per validated string that
// nothing on the device ever read. It is also skipped entirely when no consumer
// implements the interface, which on the emulator's own non-walk runs is most
// of the time.
func notifyPlateText(pl Platform, plates []Plate, text string) {
	ea, ok := pl.(EngravedAware)
	if !ok {
		return
	}
	ids := make([]uint64, len(plates))
	for i, p := range plates {
		ids[i] = p.id
	}
	ea.PlateText(ids, text)
}

func notifyPlateEngraved(pl Platform, id uint64) {
	if ea, ok := pl.(EngravedAware); ok {
		ea.PlateEngraved(id)
	}
}
