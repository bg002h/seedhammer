package gui

import (
	"seedhammer.com/md"
)

// Slot-directed seating (SPEC §7d).
//
// IT WALKS SLOTS, NOT SOURCES, and that is C8's model: the operator is asked,
// per emitted slot, which key goes in it, rather than being handed a pile of
// cards to place. The pile version is what seatKeyCards does for a template
// that already declares its origins, and it is why that function refuses to
// guess when two cards claim one slot (gui/key_card_seating.go:24-27): a
// misassignment does not fail, it derives a different wallet's address and
// shows it to the operator as proof.
//
// BACK STEPS BACK ONE SLOT rather than abandoning seating, the same directive
// gatherSlotSeeds follows (gui/multisig_build.go:725-737) and for the same
// measured reason: it previously returned on any Back, so mistyping the
// SECOND slot's seed also discarded the first.

// composerAssignableSlots is §7d's counting rule: records and cards are used
// AT MOST ONCE, and a SEED is a source of as many slots as the operator
// assigns. So the count is of SEATS, not of sources, and a payload holding
// one seed can fill every slot.
func composerAssignableSlots(st *composerState) int {
	slots := composerSlotCount(st.list)
	single := 0
	for _, s := range st.sources {
		if s.kind == composerSourceSeed {
			return slots
		}
		single++
	}
	if single > slots {
		return slots
	}
	return single
}

// composerUnfilledSlots names the slots §8p has to list.
func composerUnfilledSlots(st *composerState) []uint8 {
	var out []uint8
	for i, a := range st.assigned {
		if a.src < 0 {
			out = append(out, uint8(i))
		}
	}
	return out
}

// composerReleaseSeat drops slot i's assignment and RELEASES the source it
// held, reporting whether there was anything to release.
//
// The release is the half that was missing. composerSeatFlow marks a consumed
// key:/mk1 source `used` and the pick list filters `used` sources out, so an
// assignment dropped without releasing its source takes that key out of every
// later pick list while nothing holds it -- which is how a Back at the mapping
// review left an operator with two key: records being offered only "Type a
// seed" and "Leave unseated" (review r0 C-1).
//
// A SEED source is never marked used, because one seed fills any number of
// slots (C12, §4f), so there is nothing to release for it.
func composerReleaseSeat(st *composerState, i int) bool {
	if i < 0 || i >= len(st.assigned) {
		return false
	}
	src := st.assigned[i].src
	if src < 0 {
		return false
	}
	if src < len(st.sources) && st.sources[src].kind != composerSourceSeed {
		st.sources[src].used = false
	}
	st.assigned[i] = composerAssignment{src: -1}
	return true
}

// composerReleaseLastSeat releases the HIGHEST-indexed seated slot, so a Back
// one level up lands where the operator last was.
func composerReleaseLastSeat(st *composerState) bool {
	for i := len(st.assigned) - 1; i >= 0; i-- {
		if composerReleaseSeat(st, i) {
			return true
		}
	}
	return false
}

// composerSeatFlow asks for every UNSEATED slot in emitted order.
//
// IT RESUMES, IT DOES NOT RE-ASK (review r0 C-1). Re-entering seating after any
// Back past this step used to restart at slot @0 and overwrite every
// assignment, while the pick list filtered out every source those assignments
// still held -- so §7d's "Back keeps assignments", which the spec states twice,
// was unmet and the policy the operator had just reviewed became unreachable.
// A slot that already holds a source is skipped; releasing a slot is what makes
// it askable again, and that is the one place a source goes back on the list.
//
// Returns false when the operator backs out of the FIRST slot it asks, which is
// the directive's rule wherever Back is the way out of an opening screen.
func composerSeatFlow(ctx *Context, th *Colors, st *composerState) bool {
	n := composerSlotCount(st.list)
	if len(st.assigned) != n {
		st.assigned = make([]composerAssignment, n)
		for i := range st.assigned {
			st.assigned[i].src = -1
		}
	}
	for i := 0; i < n && !ctx.Done; {
		if st.assigned[i].src >= 0 {
			// Already seated -- resume past it rather than re-asking.
			i++
			continue
		}
		slot := uint8(i)
		var rows []string
		var srcIdx []int
		for j, src := range st.sources {
			if src.used {
				continue
			}
			rows = append(rows, composerSourceRow(src))
			srcIdx = append(srcIdx, j)
		}
		rows = append(rows, "Type a seed", "Leave unseated")
		sel, ok := composerPickScreen(ctx, th, "Seat keys", composerSeatPrompt(st, slot), rows)
		if !ok {
			if i == 0 {
				return false
			}
			// Step BACK one slot, releasing what it held so the source it
			// holds returns to the next pick list.
			i--
			composerReleaseSeat(st, i)
			continue
		}
		switch {
		case sel < len(srcIdx):
			j := srcIdx[sel]
			if st.sources[j].kind == composerSourceSeed {
				a, err := composerSeedDerive(st, slot, j)
				if err != nil {
					showError(ctx, th, "Seat keys", "Couldn't derive a key from that seed.")
					continue
				}
				st.assigned[i] = a
			} else {
				src := st.sources[j]
				st.sources[j].used = true
				st.assigned[i] = composerAssignment{
					src: j, origin: src.origin, fingerprint: src.fingerprint,
					fpPresent: src.fpPresent, xpub: src.xpub,
				}
			}
			i++
		case sel == len(srcIdx):
			src, ok := composerSeedSource(ctx, th, st)
			if !ok {
				continue
			}
			st.sources = append(st.sources, src)
		default:
			// Left unseated on purpose: the shortfall screen below is what
			// decides whether that is allowed, so this is not a refusal here.
			st.assigned[i] = composerAssignment{src: -1}
			i++
		}
	}
	return true
}

// composerSeatingComplete reports whether every slot holds a key.
func composerSeatingComplete(st *composerState) bool {
	return len(composerUnfilledSlots(st)) == 0
}

// composerShortfall is §7d's all-or-nothing transition: fewer assignable
// slots than slots REFUSES, naming the counts and the unfilled slots.
//
// NO CAUSE IS GUESSED. §8p states two facts and stops; the C5 lesson is
// taught at the shape step by §8k, and a second explanation on the screen
// that refuses is one more thing that can be wrong.
//
// Returns true when the operator chooses to engrave a key-less template
// anyway (§7f's partially seated form).
func composerShortfall(ctx *Context, th *Colors, st *composerState) bool {
	unfilled := composerUnfilledSlots(st)
	showError(ctx, th, "Seat keys", composerCopyShortfall(
		composerSlotCount(st.list), composerAssignableSlots(st), unfilled))
	cs := &ChoiceScreen{
		Title:   "Seat keys",
		Lead:    "What now?",
		Choices: []string{"Back to the paths", "Engrave a key-less template"},
	}
	sel, ok := cs.Choose(ctx, th)
	return ok && sel == 1
}

var _ = md.ComposeMaxSlots
