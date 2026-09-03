package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestComposerShapeSignatureMovesExactlyWithSlotNumbering is §7d's rule,
// stated as an equivalence: the signature changes for the wrapper, the path
// count and a key count, and for NOTHING ELSE.
func TestComposerShapeSignatureMovesExactlyWithSlotNumbering(t *testing.T) {
	digest := [32]byte{0x55}
	base := composerTwoPathList()
	sig := composerShapeSignature(base)

	// MOVES numbering.
	wrapper := base
	wrapper.Wrapper = md.ComposeTr
	if composerShapeSignature(wrapper) == sig {
		t.Error("changing the wrapper does not move the signature, but tr extracts an " +
			"internal key as @0 and renumbers every slot (§5)")
	}
	fewer := md.PathList{Wrapper: base.Wrapper, Paths: base.Paths[:1]}
	if composerShapeSignature(fewer) == sig {
		t.Error("removing a path does not move the signature")
	}
	wider := composerTwoPathList()
	wider.Paths[0].Keys = &md.KeySet{K: 2, N: 4, Sorted: true}
	if composerShapeSignature(wider) == sig {
		t.Error("changing a path's key count does not move the signature")
	}

	// DOES NOT move numbering (§7d: assignments are KEPT).
	locked := composerTwoPathList()
	locked.Paths[1].Lock = &md.Lock{Kind: md.LockOlderBlocks, Value: 42}
	if composerShapeSignature(locked) != sig {
		t.Error("adding a lock moved the signature, so assignments would be discarded " +
			"for an edit that renumbers nothing (§7d rules them KEPT)")
	}
	hashed := composerTwoPathList()
	hashed.Paths[1].Hash = &digest
	if composerShapeSignature(hashed) != sig {
		t.Error("adding a hash moved the signature; §7d keeps assignments across it")
	}
	// The THRESHOLD is not the key count: k moves no slot.
	thresh := composerTwoPathList()
	thresh.Paths[0].Keys = &md.KeySet{K: 3, N: 3, Sorted: true}
	if composerShapeSignature(thresh) != sig {
		t.Error("changing k moved the signature; only n changes how many slots exist")
	}
}

// TestComposerDiscardIsSilentWithNothingSeated is §7d's last clause: with no
// slot yet assigned there is nothing to discard and §8j does not fire. A
// warning that fires when nothing is at stake is a warning the operator
// learns to tap through.
func TestComposerDiscardIsSilentWithNothingSeated(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	if composerAnySlotAssigned(st) {
		t.Fatal("a fresh state reports an assignment")
	}
	st.assigned = make([]composerAssignment, 4)
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
	if composerAnySlotAssigned(st) {
		t.Fatal("an all-unassigned slice reports an assignment; src must be -1 for unseated")
	}
	st.assigned[2].src = 0
	if !composerAnySlotAssigned(st) {
		t.Fatal("a seated slot is not detected, so §8j would never fire")
	}
	assertModalBodyFits(t, "the §8j discard confirm", confirmWarningBody,
		composerConfirmBody(composerCopyEditClearsKeys()))
}

// TestComposerDiscardClearsEverySeat: a partial discard is the state that
// seats keys into the wrong slots silently, which is the whole reason §7d
// discards ALL assignments rather than the ones it can prove moved.
func TestComposerDiscardClearsEverySeat(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	st.assigned = []composerAssignment{{src: 0}, {src: 1}, {src: -1}, {src: 2}}
	st.sources = []composerSource{{used: true}, {used: true}, {used: true}}
	composerDiscardAssignments(st)
	for i, a := range st.assigned {
		if a.src != -1 {
			t.Errorf("slot @%d still holds source %d after a discard", i, a.src)
		}
	}
	for i, s := range st.sources {
		if s.used {
			t.Errorf("source %d is still marked used after a discard, so it would never "+
				"be offered again for the slots it no longer fills", i)
		}
	}
}
