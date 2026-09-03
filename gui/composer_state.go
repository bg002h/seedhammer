package gui

import (
	"fmt"
	"time"

	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/sysw"
)

// composerState is everything one composition holds, from the door to the
// last plate.
//
// ONE STRUCT FOR BOTH PARTS OF THE FLOW, declared here rather than grown
// field by field, because the seating half reads what the shape half wrote
// and a state split across two types is a state with two answers to "how
// many slots are there". The shape half writes `list` and `bound`; the
// seating half writes `sources`, `assigned` and `reg`.
//
// WHAT IT IS NOT: the source of anything shown at consent. §7e derives the
// consent surface from the DECODED md1 the device is about to engrave, never
// from this struct, and §8q's self-check compares the two. A field here that
// the consent screen read directly would defeat the check that exists to
// catch a builder defect.
type composerState struct {
	// list is the operator's ordered spend-path list under one wrapper: the
	// value md.Compose lowers. The GUI never builds a descriptor itself.
	list md.PathList

	// bound is the payload's now: record (§6a, C24): a LOWER bound on the
	// present, affecting echoes and refusals only, never an encoded operand.
	bound composerBound

	// NO CONFIRM MEMO LIVES HERE, and its absence is the fix rather than an
	// omission. §8a and §8b were memoised by the operator's path INDEX, and an
	// index is not an identity: "Remove path" splices the slice and left the
	// map, so a new key-less path at a reused index skipped an unskippable
	// confirm (C16). §8a now fires where a key-less path is CREATED, which
	// happens exactly once per path by construction, and §8b fires at the
	// transition out of the shape, where the sole-path condition it depends on
	// is final.

	// sources are the seatable keys: key: records, mk1 cards and seeds
	// (§7d). Filled by the seating half.
	sources []composerSource

	// assigned[i] is the source seated at EMITTED slot index i (§5's
	// first-appearance numbering), or the zero value with src < 0 for an
	// unseated slot. Discarded wholesale by any edit that moves slot
	// numbering (§7d, §8j).
	assigned []composerAssignment

	// reg holds every seed entered in this flow. C14: scrub on exit as
	// Multisig Build does -- and as there, the `defer reg.scrub()` is
	// installed at the FLOW's entry, before any seed exists, so every exit is
	// covered by construction (gui/multisig_build.go:290-291).
	reg *seedRegistry
}

// composerBound is the payload's now: record, decoded (§6a, §6b).
//
// A FIELD THAT IS ABSENT BOUNDS NOTHING. `now:` may carry seconds alone or
// seconds and a height; the height bounds heights and the seconds bound
// dates, and the echo for a kind whose field is missing carries the bare
// disclaimer (§6b). The copy never says "now": a stale record can only weaken
// the below-bound refusal, never invent one.
type composerBound struct {
	seconds   uint32
	height    uint32
	hasBound  bool
	hasHeight bool
}

// packDate renders the bound's seconds as the YYYY-MM-DD the §8c body prints.
func (b composerBound) packDate() string {
	return time.Unix(int64(b.seconds), 0).UTC().Format("2006-01-02")
}

// composerBoundFrom reads the loaded payload's single now: record.
//
// AT MOST ONE, enforced at the two sites that see the whole payload: the host
// `pack_with` and syswSession.load (§6a). Two operator-supplied records are a
// host refusal; if two ever reach the device they both go inert, so this
// takes the first ClassNow record and a second changes nothing.
func composerBoundFrom(s *syswSession) composerBound {
	if s == nil || !s.loaded {
		return composerBound{}
	}
	for _, r := range s.records {
		if r.class != sysw.ClassNow {
			continue
		}
		n, err := sysw.ParseNowRecord(r.body)
		if err != nil {
			// Unreachable: a record that does not parse is not ClassNow. The
			// arm exists because consuming a value from a call that returned an
			// error is the defect gui/policy_address.go:63-75 documents.
			return composerBound{}
		}
		return composerBound{
			seconds: n.Seconds, height: n.Height,
			hasBound: true, hasHeight: n.HasHeight,
		}
	}
	return composerBound{}
}

// composerSourceKind names where a seatable key comes from (§7d).
type composerSourceKind uint8

const (
	// composerSourceKey is a key: record: fingerprint, origin and xpub, all
	// DECLARED. The device can check the xpub's depth and last component
	// against the path and nothing else (F-217).
	composerSourceKey composerSourceKind = iota
	// composerSourceCard is an mk1 card from the payload. Its stubs are
	// IGNORED at seating -- the policy does not exist yet -- and both stubs
	// are appended when it is re-minted (§7d).
	composerSourceCard
	// composerSourceSeed is a BIP-39 or ms1 seed. Unlike the other two it is
	// not used up: one seed fills as many slots as the operator assigns, each
	// at its own hardened account by ordinal (§4f, C12).
	composerSourceSeed
)

// composerSource is one seatable key on the pick list.
type composerSource struct {
	kind  composerSourceKind
	label string
	// used is "consumed" for a key: record and an mk1 card (C8's "remaining"),
	// and always false for a seed.
	used bool

	fingerprint [4]byte
	fpPresent   bool
	origin      []md.PathComponent
	// xpub is the base58 account key. mk.Card.Xpub is a string
	// (mk/mk.go:138), and deriveAccountXpub returns one, so this is the form
	// every source has in common; decodeXpubBytes converts for md.Bind.
	xpub string
	// card is the payload mk1 this source came from, for re-minting.
	card mk.Card
	// seedID indexes composerState.reg for a seed source, and is -1 otherwise.
	seedID int
}

// composerAssignment is what fills one emitted slot.
type composerAssignment struct {
	// src indexes composerState.sources, or -1 for an unseated slot.
	src int
	// account is the BIP-48 account component for a SEED-derived slot: the
	// ordinal among the slots that master fills, in ascending emitted slot
	// index (§4f). Zero and meaningless for the other two kinds, which carry
	// the origin their record or card DECLARES, verbatim.
	account uint32
	// origin and fingerprint are the resolved declaration for this slot, as
	// the mapping review prints them and as md.ComposeWith receives them.
	origin      []md.PathComponent
	fingerprint [4]byte
	fpPresent   bool
	xpub        string
}

// composerAnySlotAssigned reports whether any emitted slot holds a source.
//
// It lives with the STATE rather than with the discard rule because both
// halves of the flow ask it: the shape half to decide whether §8j has
// anything to warn about, and the seating half to decide whether the
// shortfall check applies. An unseated slot is src == -1, never a zero value,
// so a freshly-made slice must be initialised rather than left at zero -- a
// zero src would read as "seated from source 0".
func composerAnySlotAssigned(st *composerState) bool {
	for _, a := range st.assigned {
		if a.src >= 0 {
			return true
		}
	}
	return false
}

// composerSlotCount is the policy's TOTAL slot count: the number the wire's
// 5-bit path_decl.n caps at 32 (md/md.go:215-221).
func composerSlotCount(list md.PathList) int {
	n := 0
	for _, p := range list.Paths {
		if p.Keys != nil {
			n += int(p.Keys.N)
		}
	}
	return n
}

// composerMaxKeysForPath is the picker's own bound (§4e: "the picker does not
// offer the value"): a path may hold up to 9 keys, and never more than the
// 32-slot budget the rest of the policy leaves.
func composerMaxKeysForPath(st *composerState, pathIdx int) int {
	used := 0
	for i, p := range st.list.Paths {
		if i == pathIdx || p.Keys == nil {
			continue
		}
		used += int(p.Keys.N)
	}
	free := md.ComposeMaxSlots - used
	if free < 0 {
		free = 0
	}
	if free > md.ComposeMaxKeysPerPath {
		free = md.ComposeMaxKeysPerPath
	}
	return free
}

// composerSortedIsLegal reports whether §5's key-set rule admits a SORTED
// form for this path -- which is the only place the §8b confirm may fire.
//
// §5's rule: SOLE path, unlocked, unhashed, n >= 2 lowers to sortedmulti (or
// sortedmulti_a under tr) by BIP-383/388's sole-child rule; ANY other
// multi-key path is necessarily unsorted, because nested sortedmulti is
// refused by md and by the BIPs. So on those paths the operator declined
// nothing and §8b must stay silent (§5a).
//
// The legacy wrappers are sorted-only (§4a, §4e), so no choice is offered
// under them either.
func composerSortedIsLegal(list md.PathList, idx int) bool {
	if list.Wrapper == md.ComposeSh || list.Wrapper == md.ComposeShWsh {
		return false
	}
	if len(list.Paths) != 1 || idx != 0 {
		return false
	}
	p := list.Paths[0]
	return p.Keys != nil && p.Keys.N >= 2 && p.Lock == nil && p.Hash == nil
}

// composerEveryPathHashed is §8h's condition: every way to spend this wallet
// needs a preimage that is not on this device and not on these plates.
func composerEveryPathHashed(list md.PathList) bool {
	if len(list.Paths) == 0 {
		return false
	}
	for _, p := range list.Paths {
		if p.Hash == nil {
			return false
		}
	}
	return true
}

// composerPathLine is one row of the path-list screen (§7b's "Path 2: 2-of-3
// + 90 days"). idx is the OPERATOR's zero-based index; the label counts from
// one, as every "Path N" prompt in §7d and §8s does.
func composerPathLine(p md.SpendPath, idx int) string {
	body := "hash only"
	if p.Keys == nil && p.Hash == nil {
		// NEITHER element. It used to read "hash only", so a path left empty
		// by a decline was described as something it was not and then refused
		// at Done with a body about a lock nobody set -- with no way to see
		// what was wrong with it.
		body = "empty"
	}
	if p.Keys != nil {
		if p.Keys.N == 1 {
			body = "1 key"
		} else {
			body = fmt.Sprintf("%d-of-%d", p.Keys.K, p.Keys.N)
		}
	}
	if p.Hash != nil && p.Keys != nil {
		body += " + hash"
	}
	if p.Lock != nil {
		body += " + " + composerLockShort(*p.Lock)
	}
	return fmt.Sprintf("Path %d: %s", idx+1, body)
}

// composerLockShort is the lock as a path-list row shows it: short enough for
// one line, in the operator's own units. The full echo (§8c) is what the lock
// entry screen and the consent screen print.
func composerLockShort(l md.Lock) string {
	switch l.Kind {
	case md.LockOlderBlocks:
		return fmt.Sprintf("%d blocks", l.Value)
	case md.LockOlderUnits:
		return fmt.Sprintf("%d days", composerUnitsToDays(l.Value))
	case md.LockAfterHeight:
		return fmt.Sprintf("block %d", l.Value)
	case md.LockAfterTime:
		return time.Unix(int64(l.Value), 0).UTC().Format("2006-01-02")
	}
	return "lock"
}

// composerUnitsToDays converts 512-second units back to whole days.
//
// IT FLOORS, and the direction is not arbitrary. Days-to-units rounds UP (a
// lock must never be shorter than the operator asked for), so the encoded
// value always covers at least the days typed and a bit more: 90 days is
// 15188 units, which is 90.0029 days. Flooring recovers the number the
// operator TYPED; rounding up would print 91 days back at someone who typed
// 90, on the screen whose whole job is to read the value back to them.
func composerUnitsToDays(units uint32) uint32 {
	return uint32(uint64(units) * 512 / 86400)
}

// composerSlotsKeysLine is §7b's live line.
//
// A SEED IS NOT A COUNT. §7d rules that "keys available" counts records plus
// cards plus, for each seed, "any slots", so a payload with two records and a
// seed reads `keys available: 2 + seed` rather than 3 -- which would promise
// a third distinct key that does not exist.
func composerSlotsKeysLine(st *composerState) string {
	if len(st.sources) == 0 {
		// §7b scopes this line to "whenever a payload is loaded". With none it
		// read `keys available: 0` on a build the door has already described
		// as key-less -- a count that answers a question nobody asked.
		return fmt.Sprintf("slots: %d", composerSlotCount(st.list))
	}
	records, seeds := 0, 0
	for _, s := range st.sources {
		if s.kind == composerSourceSeed {
			seeds++
		} else {
			records++
		}
	}
	line := fmt.Sprintf("slots: %d / keys available: %d", composerSlotCount(st.list), records)
	switch {
	case seeds == 1:
		line += " + seed"
	case seeds > 1:
		line += " + seeds"
	}
	return line
}
