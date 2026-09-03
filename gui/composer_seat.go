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

// composerSeatFlow asks for every slot in emitted order.
//
// Returns false when the operator backs out of slot @0, which is the
// directive's rule wherever Back is the way out of an opening screen.
func composerSeatFlow(ctx *Context, th *Colors, st *composerState) bool {
	n := composerSlotCount(st.list)
	if len(st.assigned) != n {
		st.assigned = make([]composerAssignment, n)
		for i := range st.assigned {
			st.assigned[i].src = -1
		}
	}
	for i := 0; i < n && !ctx.Done; {
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
			// Step BACK one slot, releasing what it held.
			i--
			if prev := st.assigned[i].src; prev >= 0 && st.sources[prev].kind != composerSourceSeed {
				st.sources[prev].used = false
			}
			st.assigned[i] = composerAssignment{src: -1}
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
