package gui

import "seedhammer.com/sysw"

// syswSession is the systemwide payload the machine currently holds.
//
// SPEC_systemwide_payloads §3.2.1, transcribed. ONE entry, not a list: loading
// a second replaces the first outright. A list would raise "which payload did
// this record come from" at every consumption, and that question has exactly one
// safe answer when there is only ever one.
//
// LIFETIME IS THE PROCESS. Cleared when the firmware restarts and at no other
// time — there is no timer and no idle wipe (decision 2). No flow clears it on
// exit, because a flow that did would silently reintroduce the per-program KDF
// that "once per session" exists to avoid.
//
// This is the NON-WIPING class by the operator's explicit ruling and by EPD §2.2
// item 12 before it. Nothing here claims the records are scrubbed.
type syswSession struct {
	loaded bool

	// identity is sysw.Identity over the region bytes — NOT the EPD §6.6
	// digest, which does not exist when pub_len == 0 and would therefore give
	// every secrets-only payload one shared identity.
	identity [32]byte

	// compared is set per [compared] (§12.2): the operator compared the
	// displayed digest, OR the payload opened. NOT "the operator compared the
	// digest" alone — that is one of two routes, and glossing it as the whole
	// rule is how a second definition gets written.
	compared bool

	sealed bool

	// weak names the [cliff] result for brevity only. [cliff] is a WORD COUNT,
	// so weak == false does not mean strong.
	weak bool

	// records are classified ONCE, at load. Re-sniffing at the point of use
	// would let one byte string be admitted as one class and consumed as
	// another.
	records []syswRecord
}

type syswRecord struct {
	class sysw.Class
	body  string
}

// load replaces whatever was held. Returns the flags the consuming screen must
// show (§3.3.3).
// `compared` is decided by the LOAD FLOW, which is the only place that can see
// both of [compared]'s routes (§12.2): the operator confirming the displayed
// digest, and a successful AEAD open. Passing it in keeps that decision in one
// place instead of letting each caller invent half the rule.
func (s *syswSession) load(p *sysw.Payload, identity [32]byte, sealed, cliffAbove, compared bool) {
	*s = syswSession{loaded: true, identity: identity, sealed: sealed, weak: !cliffAbove, compared: compared}
	for _, r := range p.Public {
		s.records = append(s.records, syswRecord{class: sysw.Classify(r), body: r})
	}
	for _, r := range p.Secret {
		s.records = append(s.records, syswRecord{class: sysw.Classify(r), body: r})
	}
}

// take returns the first record of the wanted class, or false.
//
// It refuses while compared is false: a record may not be handed to a program
// until the payload it came from has been authenticated by one of [compared]'s
// two routes.
func (s *syswSession) take(want sysw.Class) (string, bool) {
	if !s.loaded || !s.compared {
		return "", false
	}
	for _, r := range s.records {
		if r.class == want {
			return r.body, true
		}
	}
	return "", false
}

// has reports whether a class is present, WITHOUT the compared gate, so a menu
// can offer "from payload" before the operator has compared anything.
func (s *syswSession) has(want sysw.Class) bool {
	if !s.loaded {
		return false
	}
	for _, r := range s.records {
		if r.class == want {
			return true
		}
	}
	return false
}

// syswOffer asks whether to take a record of `want` from the loaded payload.
//
// Returns ok=false when there is nothing to offer OR the operator declines, so
// every caller is `if body, ok := syswOffer(...); ok { … }` and otherwise falls
// through to its existing input path. That shape keeps the payload strictly
// ADDITIVE: a machine with no payload behaves exactly as it did before.
//
// The four programs that do not share seedEntryFlow each call this — measured,
// they share no helper at all, which is why the seam covers 4 of 8 programs and
// the other four are wired individually (plan stage 5d).
func syswOffer(ctx *Context, th *Colors, want sysw.Class, lead string) (string, bool) {
	if ctx.sysw == nil || !ctx.sysw.has(want) {
		return "", false
	}
	cs := &ChoiceScreen{Title: "Input", Lead: lead, Choices: []string{"ENTER IT", "FROM PAYLOAD"}}
	choice, ok := cs.Choose(ctx, th)
	if !ok || choice == 0 {
		return "", false
	}
	return ctx.sysw.take(want)
}
