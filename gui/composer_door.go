package gui

import (
	"seedhammer.com/sysw"
)

// The Wallet Policy door (SPEC_wallet_policy_composer.md §7a, C6).
//
// BEFORE THIS THERE WAS NO DOOR. walletPolicyFlow offered the payload's md1
// cards (gui/wallet_policy.go:44), then a Descriptor record (:46), and when
// the payload held neither it fell through to the NFC gather at :97 with no
// screen at all -- so an operator with an empty machine met a wait, not a
// choice. §7a makes it a ChoiceScreen in EVERY state, and each choice NAMES
// the route it takes (F-437, the same ruling behind syswAltScan).
//
// THE KEY STATE IS COUNTED WITHOUT THE `compared` GATE, ON PURPOSE.
// syswSession.has exists for exactly this and says so (gui/sysw_session.go
// :178-180): a menu may offer "from payload" before the operator has compared
// anything. The door COUNTS; it consumes nothing. Seating consumes, through
// take/takeAll, and inherits their refusal.

type composerRoute int

const (
	composerRouteScan composerRoute = iota
	composerRouteFromPayload
	composerRouteBuild
)

// composerDoorCounts reports what the loaded payload holds, for §8r.
//
// `inert` is the not-understood count: records the classifier placed in
// ClassUnknown, which under the shipped contract stay in the session, are
// offered to nobody and reach no screen (sysw/descriptor.go:46-48). It is
// the ONE line that covers all three composer classes' malformations, since
// a bad hash: or now: changes no other count (§6a).
func composerDoorCounts(s *syswSession) (keys, seeds, inert int) {
	if s == nil || !s.loaded {
		return 0, 0, 0
	}
	for _, r := range s.records {
		switch r.class {
		case sysw.ClassKey:
			keys++
		case sysw.ClassMnemonic, sysw.ClassCodex32Secret:
			seeds++
		case sysw.ClassUnknown:
			inert++
		}
	}
	return keys, seeds, inert
}

// composerDoorLines is §8r, in §7a's order.
//
// A SEED PRINTS NO COUNT OF SLOTS and a seeds-only payload prints no key
// count: a seed fills any number of slots (C12, §4f), so a slot number beside
// it would answer a question the operator is not asking.
func composerDoorLines(s *syswSession, payloadInFlash bool) []string {
	if s == nil || !s.loaded {
		if payloadInFlash {
			return []string{composerCopyPayloadNotLoaded()}
		}
		return []string{composerCopyNoKeys()}
	}
	keys, seeds, inert := composerDoorCounts(s)
	var lines []string
	switch {
	case keys > 0 && seeds > 0:
		lines = append(lines, composerCopyKeysAndSeeds(keys, seeds))
	case keys > 0:
		lines = append(lines, composerCopyKeysLoaded(keys))
	case seeds > 0:
		lines = append(lines, composerCopySeedOnly())
	default:
		lines = append(lines, composerCopyNoKeys())
	}
	if inert > 0 {
		lines = append(lines, composerCopyNotUnderstood(inert))
	}
	return lines
}

// composerDoorHasConsumablePolicy reports whether "From payload" has anywhere
// to go: a Descriptor record, or an md1/mk1 chunk set.
func composerDoorHasConsumablePolicy(s *syswSession) bool {
	if s == nil {
		return false
	}
	return s.has(sysw.ClassDescriptor) || s.has(sysw.ClassMDMK)
}

// composerDoorFlow draws the door and reports the chosen route.
//
// The lead carries §8r's key-state lines. ChoiceScreen's Lead is drawn with
// widget.Labelw (gui/gui.go:1969) so it WRAPS, which the choice rows do not
// -- which is why the state is a lead and the routes are rows.
func composerDoorFlow(ctx *Context, th *Colors) (composerRoute, bool) {
	inFlash := false
	if r := ctx.Platform.SyswReader(); r != nil && r.Probe() {
		inFlash = true
	}
	lead := ""
	for i, l := range composerDoorLines(ctx.sysw, inFlash) {
		if i > 0 {
			lead += " "
		}
		lead += l
	}
	choices := []string{"Scan cards"}
	routes := []composerRoute{composerRouteScan}
	if composerDoorHasConsumablePolicy(ctx.sysw) {
		choices = append(choices, "From payload")
		routes = append(routes, composerRouteFromPayload)
	}
	choices = append(choices, "Build a new policy")
	routes = append(routes, composerRouteBuild)

	cs := &ChoiceScreen{Title: "Wallet Policy", Lead: lead, Choices: choices}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return composerRouteScan, false
	}
	return routes[sel], true
}
