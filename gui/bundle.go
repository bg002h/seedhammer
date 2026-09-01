package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── T5: guided bundle sequencing (multi-card md1/mk1 → confirm → engrave) ───
//
// The bundle gatherer accumulates MULTIPLE DISTINCT cards over NFC — the device
// analogue of host `me bundle`. It wraps the shipped single-card gatherers
// (mk1Gatherer / md1Gatherer) UNCHANGED, but inverts their semantics: where a
// single-card gather treats a foreign chunk_set_id as a rejection, the bundle
// keys gatherers by csid so a NEW csid starts a NEW card (R0-I2). Each card is
// added to the bundle ONLY after its integrity gate passes (full reassembly +
// integrity for chunked; BCH + structural md.Decode for a standalone md1, I-1).
// A single (non-chunked) mk1 is REFUSED (R0-C1, host parity); an ms1 secret is
// REFUSED in the NFC channel (R0-C2).

// bundleCardKind discriminates a key card from a descriptor card.
type bundleCardKind int

const (
	cardMK1 bundleCardKind = iota // an mk1 account-key card
	cardMD1                       // an md1 descriptor card
	cardMS1                       // an ms1 SECRET share card (T6a-2 single-sig engrave only)
)

// bundleCard is one verified card in the accumulated bundle. strings holds the
// verbatim chunk strings (in index order, or the single string for a standalone
// md1) — every engraved plate is one of these strings, unmodified (I-4).
type bundleCard struct {
	kind    bundleCardKind
	label   string   // "mk1 key" / "md1 descriptor", for the review screen
	strings []string // verbatim chunk strings in index order (or [single])
	summary string   // a one-line metadata summary for the review screen

	// device-csid-warning Contract 3: computed ONCE, at offerChunkedMK1 set
	// completion, for a cardMK1 whose wire-declared chunk_set_id disagrees
	// with mk.DerivedChunkSetID of its own decoded content. Every kind but
	// cardMK1 leaves these at their zero value (csidMismatch false). The
	// comparison travels ON the card as data so every downstream consumer —
	// however many hops from the gatherer — reads the SAME evaluation rather
	// than risking a second, possibly-drifted one.
	csidMismatch bool
	declaredCSID uint32
	derivedCSID  uint32
}

// csidMismatchWarningText is the R6 host-verbatim mismatch-warning body (SPEC
// design/SPEC_device_csid_warning.md, Contract 2): byte-identical to
// crates/me-cli/src/csid_warn.rs's chunk_set_id_mismatch_warning(declared,
// derived), ASCII-only (device strings are ASCII-only: a non-ASCII rune
// blanks the WHOLE modal body, gui/font_coverage_test.go — R0 r1 I2), and
// measured to fit the panel with 302 chars of headroom (R0 r1/r2). Shared by
// every csid-mismatch surface: the inspect flow's notice (mk1_inspect.go) and
// the bundle-gatherer flow's set-completion notice (Engrave Bundle / Wallet
// Policy / Build Policy).
func csidMismatchWarningText(declared, derived uint32) string {
	return fmt.Sprintf(
		"warning: this key card's stamped chunk-set id (%05x) was not derived from "+
			"its content, which computes %05x. The card decodes fine, but diagnostics that "+
			"name plates by id will call it %05x. To fix it, re-mint: run mk encode again "+
			"without --chunk-set-id and the id is derived from the key data automatically.",
		declared, derived, declared)
}

// csidMarker is the compact per-card mismatch marker (Contract 3: "the
// marker's compact form ... is the implementer's proposal, frozen at the
// screenshot gate"), appended to a card's line on every review/census/
// inventory/payload surface: "" for a matched (or non-mk1) card, else
// " [csid <declared>!<derived>]" — legible at a glance, ASCII, and identical
// across every list surface so the same card reads the same way everywhere.
func csidMarker(c bundleCard) string {
	if !c.csidMismatch {
		return ""
	}
	return fmt.Sprintf(" [csid %05x!%05x]", c.declaredCSID, c.derivedCSID)
}

// bundleCSIDNote is Contract 3's "line-marker only, NO modal" for the two
// verify-readback surfaces (multisig_verify.go, singlesig_verify.go): one
// line per csid-mismatched mk1 card among cards, appended to the terminal
// PASS/FAIL verdict text those screens already show — never a separate
// modal, because a mid-verify modal would blur the pass/fail reading these
// screens exist to give (R0, stated reason). "" when nothing mismatches.
func bundleCSIDNote(cards []bundleCard) string {
	var lines []string
	for _, c := range cards {
		if c.kind == cardMK1 && c.csidMismatch {
			lines = append(lines, "Note: mk1 key"+csidMarker(c)+
				" -- chunk-set id mismatch; see Inspect key for detail.")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n")
}

// showBundleCSIDMismatchNotices is Contract 3's "notice modal at set
// completion" for the three interactive-gather surfaces that want one
// (Engrave Bundle, Wallet Policy, Build Policy — via bundleFlow,
// walletPolicyFlow and buildMultisigPolicyFlow respectively). Called ONCE,
// right after that surface's gather returns, over the whole completed set:
// one non-blocking showNotice per csid-mismatched mk1 card, same body as the
// inspect flow's (csidMismatchWarningText), same non-blocking semantics
// (every tap ACCEPTED, R1). Engrave Multisig and the two verify readbacks
// deliberately never call this (Contract 3: refused-before-render / line-
// marker-only respectively).
//
// RE-FIRES ON RE-ENTRY, BY DESIGN (whole-diff review M3, accepted). All
// three callers loop back into their own gather on a review Back
// (bundleFlow's `gathered = cards; continue`, walletPolicyFlow's five
// `continue` paths, buildStepGather's "this step can run more than once
// now"), and each re-entry recomputes csidMismatch from scratch and calls
// this again -- so a still-mismatched card's warning re-fires once per
// round trip rather than latching "already shown this session". Warning-
// only and non-blocking, so accepted rather than debounced; pinned live by
// TestBundleFlowNoticeRefiresOnReviewBackReentry (gui/csid_warning_test.go).
func showBundleCSIDMismatchNotices(ctx *Context, th *Colors, title string, cards []bundleCard) {
	for _, c := range cards {
		if c.kind == cardMK1 && c.csidMismatch {
			showNotice(ctx, th, title, csidMismatchWarningText(c.declaredCSID, c.derivedCSID))
		}
	}
}

// scanClass is the per-scan classification (R0-C1/C2), computed BEFORE any
// accumulation so the bundle loop can refuse / route each object explicitly.
type scanClass int

const (
	clsDrop            scanClass = iota // not an md/mk object — drop with a note
	clsMs1Refuse                        // a codex32 secret (HRP ms) — refuse, never NFC
	clsSingleMK1Refuse                  // a single (non-chunked) mk1 — malformed, refuse
	clsStandaloneMD1                    // a single (non-chunked) md1 — a 1-plate card
	clsChunkedMK1                       // a chunk of a chunked mk1 key card
	clsChunkedMD1                       // a chunk of a chunked md1 descriptor card
)

// classify inspects one scanned object and returns its bundle class plus, for a
// chunked card, its chunk_set_id and the verbatim string (R0-C1/C2):
//
//   - codex32.String / HRP-"ms" secret → clsMs1Refuse (ms1 arrives as a
//     codex32.String, NOT mdmkText — scan.go:70-73 — so it would be silently
//     dropped by an mdmkText-only assertion).
//   - mdmkText with an mk1 prefix → mk.ParseHeader: !Chunked → clsSingleMK1Refuse
//     (host parity); chunked → clsChunkedMK1 + ChunkSetID.
//   - mdmkText with an md1 prefix → md.ParseChunkHeader: !Chunked →
//     clsStandaloneMD1; chunked → clsChunkedMD1 + ChunkSetID.
//   - anything else → clsDrop.
func classify(obj any) (scanClass, uint32, string) {
	switch o := obj.(type) {
	case codex32.String:
		// The scanner only yields a codex32.String for a valid codex32 secret
		// (ms-HRP); md1/mk1 use the BCH codec and arrive as mdmkText (scan.go:70-73).
		// A codex32 secret is SECRET material — refused in the NFC/bundle channel
		// regardless of HRP, never silently dropped (R0-C2).
		return clsMs1Refuse, 0, o.String()
	case mdmkText:
		s := string(o)
		switch {
		case hasMKPrefix(s):
			h, err := mk.ParseHeader(s)
			if err != nil {
				return clsDrop, 0, s
			}
			if !h.Chunked {
				return clsSingleMK1Refuse, 0, s
			}
			return clsChunkedMK1, h.ChunkSetID, s
		case hasMDPrefix(s):
			h, err := md.ParseChunkHeader(s)
			if err != nil {
				return clsDrop, 0, s
			}
			if !h.Chunked {
				return clsStandaloneMD1, 0, s
			}
			return clsChunkedMD1, h.ChunkSetID, s
		default:
			return clsDrop, 0, s
		}
	default:
		return clsDrop, 0, ""
	}
}

// bundleOfferStatus is the per-offer outcome the gather flow turns into operator
// feedback.
type bundleOfferStatus int

const (
	bundleDropped          bundleOfferStatus = iota // not an md/mk object
	bundleRefusedMs1                                // an ms1 secret — refused
	bundleRefusedSingleMK1                          // a single (malformed) mk1 — refused
	bundleAddedSingleMD1                            // a standalone md1 card added
	bundleChunkProgress                             // a chunk added; card still incomplete
	bundleCardComplete                              // a chunked card completed + verified
	bundleDuplicate                                 // a chunk/card already captured
)

// bundleGatherer accumulates distinct verified cards. Chunked cards are keyed by
// chunk_set_id so a new csid creates a new sub-gatherer = a new card (R0-I2);
// standalone md1 cards are appended directly (they have a zero-value csid that
// would otherwise collide, R0-C1).
type bundleGatherer struct {
	mkSets map[uint32]*mk1Gatherer // reuse UNCHANGED
	mdSets map[uint32]*md1Gatherer // reuse UNCHANGED
	cards  []bundleCard            // completed + verified, in completion order
}

// offer classifies one scanned object and routes it. A card is added to cards
// only after its integrity gate passes (I-1).
func (g *bundleGatherer) offer(obj any) bundleOfferStatus {
	cls, csid, str := classify(obj)
	switch cls {
	case clsMs1Refuse:
		return bundleRefusedMs1
	case clsSingleMK1Refuse:
		return bundleRefusedSingleMK1
	case clsStandaloneMD1:
		return g.offerStandaloneMD1(str)
	case clsChunkedMK1:
		return g.offerChunkedMK1(csid, str)
	case clsChunkedMD1:
		return g.offerChunkedMD1(csid, str)
	default:
		return bundleDropped
	}
}

// offerStandaloneMD1 validates a single-string md1 via md.Decode (BCH + full
// structural decode = its integrity, R0-C1) and appends it as its own card,
// deduplicating by the verbatim string.
func (g *bundleGatherer) offerStandaloneMD1(str string) bundleOfferStatus {
	if g.hasStandaloneMD1(str) {
		return bundleDuplicate
	}
	tpl, err := md.Decode(str)
	if err != nil {
		return bundleDropped
	}
	g.cards = append(g.cards, bundleCard{
		kind:    cardMD1,
		label:   "md1 descriptor",
		strings: []string{str},
		summary: bundleMD1Summary(tpl),
	})
	return bundleAddedSingleMD1
}

func (g *bundleGatherer) hasStandaloneMD1(str string) bool {
	for _, c := range g.cards {
		if c.kind == cardMD1 && len(c.strings) == 1 && c.strings[0] == str {
			return true
		}
	}
	return false
}

// offerChunkedMK1 routes a chunk to its csid's sub-gatherer (creating a new one
// = a new card on a new csid, R0-I2). On set completion it runs the integrity
// gate (mk.Decode) and appends a verified cardMK1.
func (g *bundleGatherer) offerChunkedMK1(csid uint32, str string) bundleOfferStatus {
	if g.cardCompleteMK(csid) {
		return bundleDuplicate
	}
	if g.mkSets == nil {
		g.mkSets = map[uint32]*mk1Gatherer{}
	}
	sub, ok := g.mkSets[csid]
	if !ok {
		sub = &mk1Gatherer{}
		g.mkSets[csid] = sub
	}
	switch sub.offer(str) {
	case gatherAdded:
		if !sub.complete() {
			return bundleChunkProgress
		}
		collected := sub.collected()
		card, err := mk.Decode(collected)
		if err != nil {
			// Reassembly/integrity failed — the set is not added (I-1). Reset the
			// sub-gatherer so a fresh, correct scan can recover.
			delete(g.mkSets, csid)
			return bundleDropped
		}
		bc := bundleCard{
			kind:    cardMK1,
			label:   "mk1 key",
			strings: collected,
			summary: mk1Summary(card),
		}
		// device-csid-warning Contract 3: the comparison runs ONCE, here, at
		// set completion, and its result travels ON bc so no downstream
		// consumer re-evaluates it. A DerivedChunkSetID error is a defensive
		// no-op (silent): the card just passed mk.Decode, so encodeBytecode
		// re-encoding it should never fail in practice (the parity test
		// proves round-trip on 20/20 clean corpus rows) — this only guards
		// against treating "could not compute" as "mismatched".
		if derived, derr := mk.DerivedChunkSetID(card); derr == nil && derived != csid {
			bc.csidMismatch = true
			bc.declaredCSID = csid
			bc.derivedCSID = derived
		}
		g.cards = append(g.cards, bc)
		return bundleCardComplete
	case gatherDup:
		return bundleDuplicate
	default: // gatherForeign / gatherIgnored — should not occur for a csid-keyed sub.
		return bundleDropped
	}
}

// offerChunkedMD1 mirrors offerChunkedMK1 for md1, gating on md.DecodeChunks.
func (g *bundleGatherer) offerChunkedMD1(csid uint32, str string) bundleOfferStatus {
	if g.cardCompleteMD(csid) {
		return bundleDuplicate
	}
	if g.mdSets == nil {
		g.mdSets = map[uint32]*md1Gatherer{}
	}
	sub, ok := g.mdSets[csid]
	if !ok {
		sub = &md1Gatherer{}
		g.mdSets[csid] = sub
	}
	switch sub.offer(str) {
	case gatherAdded:
		if !sub.complete() {
			return bundleChunkProgress
		}
		collected := sub.collected()
		tpl, err := md.DecodeChunks(collected)
		if err != nil {
			delete(g.mdSets, csid)
			return bundleDropped
		}
		g.cards = append(g.cards, bundleCard{
			kind:    cardMD1,
			label:   "md1 descriptor",
			strings: collected,
			summary: bundleMD1Summary(tpl),
		})
		return bundleCardComplete
	case gatherDup:
		return bundleDuplicate
	default:
		return bundleDropped
	}
}

// cardCompleteMK reports whether a chunked mk1 card of this csid has already
// completed (so re-scanning its chunks is a duplicate, not progress).
func (g *bundleGatherer) cardCompleteMK(csid uint32) bool {
	sub, ok := g.mkSets[csid]
	return ok && sub.complete()
}

func (g *bundleGatherer) cardCompleteMD(csid uint32) bool {
	sub, ok := g.mdSets[csid]
	return ok && sub.complete()
}

// pending reports whether any chunked card is mid-set (primed but not complete)
// — used by the gather flow to warn before "Done adding cards" strands a
// half-scanned card.
func (g *bundleGatherer) pending() bool {
	for _, sub := range g.mkSets {
		if sub.primed && !sub.complete() {
			return true
		}
	}
	for _, sub := range g.mdSets {
		if sub.primed && !sub.complete() {
			return true
		}
	}
	return false
}

// dropPending discards every half-scanned (primed, incomplete) sub-gatherer so a
// partial card is never carried into the engrave. Completed cards (already in
// g.cards) are untouched. The operator may re-scan the dropped card's full
// chunk set to re-add it.
func (g *bundleGatherer) dropPending() {
	for csid, sub := range g.mkSets {
		if sub.primed && !sub.complete() {
			delete(g.mkSets, csid)
		}
	}
	for csid, sub := range g.mdSets {
		if sub.primed && !sub.complete() {
			delete(g.mdSets, csid)
		}
	}
}

// mk1Summary is a one-line review summary for an mk1 card.
//
// F-78: joined with "|", not "·" — "·" has no glyph in ctx.Styles.body
// (poppins.Regular16) and contributes zero pixels.
func mk1Summary(card mk.Card) string {
	fp := card.Fingerprint
	if fp == "" {
		fp = "none"
	}
	return card.Network + " | " + card.Path + " | fp " + fp
}

// bundleMD1Summary is a one-line review summary for an md1 descriptor card,
// reusing the same scriptName + policyLine helpers as the single-card display.
func bundleMD1Summary(tpl md.Template) string {
	return scriptName(tpl) + " " + policyLine(tpl)
}
