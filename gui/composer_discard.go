package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/md"
)

// Discard-on-numbering-change (SPEC §7d, §8j).
//
// WHY ALL ASSIGNMENTS AND NOT THE ONES THAT MOVED. §5 numbers slots by FIRST
// APPEARANCE in the emitted text, and that text is a function of the wrapper
// as well as of the path list -- tr extracts an internal key as @0 and wsh
// does not. A carried assignment would seat keys silently into the wrong
// slots, which is the one failure gui/key_card_seating.go:24-27 refuses to
// allow anywhere on this device: a misassignment does not fail, it derives a
// different wallet's address and shows it to the operator as proof.
//
// A LOCK OR HASH EDIT MOVES NO SLOT. Assignments are kept across it and the
// stub screen is re-shown, because the template ID is not shape-invariant
// even when the numbering is (§7c).

// composerShapeSignature captures exactly what slot numbering depends on: the
// wrapper, the number of paths, and each path's KEY COUNT. Not k, which
// changes no slot; not the lock; not the digest.
func composerShapeSignature(list md.PathList) string {
	var b strings.Builder
	fmt.Fprintf(&b, "w%d/", list.Wrapper)
	for _, p := range list.Paths {
		n := 0
		if p.Keys != nil {
			n = int(p.Keys.N)
		}
		fmt.Fprintf(&b, "%d,", n)
	}
	return b.String()
}

// composerDiscardAssignments clears every seat and releases every source it
// held. Both halves, because a source left marked `used` would never be
// offered again for the slot it no longer fills.
func composerDiscardAssignments(st *composerState) {
	for i := range st.assigned {
		st.assigned[i] = composerAssignment{src: -1}
	}
	for i := range st.sources {
		st.sources[i].used = false
	}
}

// composerApplyShapeEdit runs an edit and discards the seats if, and only if,
// the numbering moved.
func composerApplyShapeEdit(st *composerState, edit func()) bool {
	before := composerShapeSignature(st.list)
	edit()
	if composerShapeSignature(st.list) == before {
		return false
	}
	composerDiscardAssignments(st)
	st.assigned = make([]composerAssignment, composerSlotCount(st.list))
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
	return true
}
