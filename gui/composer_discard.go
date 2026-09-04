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
// A LOCK OR HASH EDIT MOVES NO SLOT -- UNDER wsh. Under tr it can, and that
// exception is the whole of the correction below: assignments are kept across
// an edit that leaves the NUMBERING alone, and the stub screen is re-shown
// either way, because the template ID is not shape-invariant even when the
// numbering is (§7c).

// composerShapeSignature captures what slot numbering depends on -- by ASKING
// THE CODEC, not by re-deriving its rule here.
//
// IT USED TO BE A LIST OF STRUCTURAL TERMS: the wrapper, the path count and
// each path's key count, which is §7d's own enumeration and is INCOMPLETE.
// md's lowerTr numbers slots from an internal key it picks with isBareSingle()
// -- Keys.N == 1, no Lock, no Hash (md/compose.go) -- and puts that path's key
// FIRST, before listed order. So under tr a LOCK or a HASH decides which path
// owns slot @0, and two lists these terms called equal could number their
// slots completely differently: a hand-built [2-of-2, 1 key, 1 key] and the
// decaying-multisig preset both signed "w0/2,1,1," while three of their four
// slots served different paths. composerApplyShapeEdit compared the terms, saw
// no move, and kept every seat -- keys spending on paths the operator never
// chose them for, which is the one failure gui/key_card_seating.go:24-27
// refuses to allow anywhere on this device.
//
// The structural terms are KEPT as well as the mapping, because a list the
// codec refuses (empty, lock-only, key-less under tr) has no mapping to
// compare, and two different invalid shapes must still not look equal.
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
	// THE CODEC IS THE AUTHORITY. A list that does not compose contributes no
	// mapping, so an edit into or out of a refused shape always reads as a
	// move -- which discards, the safe direction.
	if c, err := md.Compose(list); err == nil {
		b.WriteByte('|')
		for _, s := range c.Slots() {
			fmt.Fprintf(&b, "%d.%d/", s.Path, s.Ordinal)
		}
	}
	return b.String()
}

// composerShapeField names the one field an editor arm can change, so the
// probe below varies THAT field and nothing else.
type composerShapeField uint8

const (
	composerFieldLock composerShapeField = iota
	composerFieldHash
)

// composerEditCanRenumber reports whether editing path idx's lock -- or its
// hash -- can move slot numbering under this list's wrapper, by asking the
// codec with that ONE field cleared and then set, rather than by naming
// lowerTr's predicate a second time in this package.
//
// It exists so §8j is asked exactly where it is true. Asking it before every
// lock editor told an operator who wanted to change a lock that every key
// would be cleared -- false for the edit they intended, and declining it left
// the lock uneditable at all (§7g classifies a lock edit DEFAULT). Asking it
// before none of them let a tr lock edit hand slot @0 to another path in
// silence.
//
// IT VARIES ONLY THE FIELD THE ARM EDITS, and that is not a detail: the first
// version cleared the HASH in both variants while toggling the lock, so it
// answered a question about a path it had already changed (fold verification
// r2, I-2 and I-3, measured over 14,092 (list, idx) pairs -- 1,200 false
// negatives and 288 false positives, against 0 and 0 for this one). The false
// negatives were key-less paths, where clearing the hash empties the path and
// the list stops composing: the probe called that "no move", the arm asked
// nothing, and composerApplyShapeEdit then discarded every seat in silence.
// The false positives were tr paths carrying a hash, where no lock can affect
// isBareSingle: §8j fired, cleared nothing, and declining it left the lock
// uneditable -- verbatim the failure this function exists to remove.
func composerEditCanRenumber(list md.PathList, idx int, field composerShapeField) bool {
	if idx < 0 || idx >= len(list.Paths) {
		return false
	}
	cleared := composerListWithPaths(list)
	set := composerListWithPaths(list)
	switch field {
	case composerFieldLock:
		cleared.Paths[idx].Lock = nil
		set.Paths[idx].Lock = &md.Lock{Kind: md.LockOlderBlocks, Value: 1}
	case composerFieldHash:
		var probe [32]byte
		for i := range probe {
			probe[i] = 0x01
		}
		cleared.Paths[idx].Hash = nil
		set.Paths[idx].Hash = &probe
	}
	return composerShapeSignature(cleared) != composerShapeSignature(set)
}

// composerListWithPaths copies the path SLICE so a probe can replace a path's
// lock or hash without writing through to the operator's own list. The
// elements are values and the probe only replaces pointers, never writes
// through them.
func composerListWithPaths(list md.PathList) md.PathList {
	out := list
	out.Paths = append([]md.SpendPath(nil), list.Paths...)
	return out
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
