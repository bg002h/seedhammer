package gui

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── T6c Phase B: the on-device "Build policy" authoring path ────────────────
//
// buildMultisigPolicyFlow assembles a sortedmulti k-of-n wallet-policy md1 ON
// the device (the device is the AUTHORITATIVE creator — there is no coordinator
// to match), then engraves it through the UNCHANGED T6b machinery. It is reached
// only from the engraveMultisigFlow front-door ("Build policy"); the existing
// "Supply policy (md1)" path is supplyMultisigPolicyFlow (the verbatim T6b body).
//
// The assembled md1 is built by the SOLE md1-bytes producer md.EncodeMultisig
// (via assembleBuildPolicy); every downstream consumer takes those strings
// VERBATIM (I-VERBATIM). The operator MUST acknowledge an unskippable
// EXPERIMENTAL warning before any engrave (I-WARN); this path is hardware-
// UNvalidated.

// buildMultisigSeedHook is a test-only seam to observe the typed mnemonic (to
// assert it is scrubbed on exit). nil in production. Mirrors multisigSeedHook.
var buildMultisigSeedHook func(bip39.Mnemonic)

func buildMultisigPolicyFlow(ctx *Context, th *Colors) {
	// (1) Bounded param pickers (template/n/k/@S/fp).
	p, ok := buildParamPickFlow(ctx, th)
	if !ok {
		return
	}
	// The @S picker's FIRST screen is a mandatory pick, so this cannot fire today.
	// (It used to read "the @S picker always sets exactly one held slot", which
	// stopped being true at 4b10319 when the picker became a multi-select in this
	// same branch -- and that stale premise is exactly what hid I-6: an operator
	// CAN now hold every slot, so `open` can be 0.)
	//
	// It is here because every use of p.SelfSlots below would otherwise index an
	// empty slice, and a panic partway through a flow that cuts steel is a worse
	// outcome than a named refusal. buildEngraveTail's errBuildNoHeldSlot is the
	// same guard at the other end of the flow.
	if len(p.SelfSlots) == 0 {
		showError(ctx, th, "Build Policy", "This build holds no key of its own, so there "+
			"is nothing for this device to engrave. Start again and choose your slot.")
		return
	}

	// (2) THE PAYLOAD SUPPLIES THE WHOLE COSIGNER SET (S1). §3.3.2 admits
	// ClassMDMK to this program; every such record is fed to the gather through
	// the SAME offer() a scanned card takes, so all of them get identical dedup,
	// chunk assembly and integrity gating.
	//
	// The count of cards on the payload is INDEPENDENT of n (F-173, `0..n`):
	// over-supply is normal and is resolved by selection, under-supply is a named
	// refusal, and the exact-count check stays where it belongs — on the
	// ASSEMBLED set, in buildCosignerCards, below.
	records, state := buildCosignerSource(ctx)
	supply, incomplete := buildCosignerSupply(records)

	// (2a) S4's ONE slot-source question (SPEC 4.3): is the operator's own key on
	// a payload card as well as in their seed? That is the `both` assignment, and
	// it is the only shape that triggers the gate.
	//
	// ASKED ONLY WHEN `both` IS ACHIEVABLE, which is this package's own rule
	// rather than a new one: a picker with one possible answer is a tap that
	// teaches nothing (syswSeedPicker says so in those words, and
	// buildCosignerPickFlow short-circuits on the same principle). `both` puts a
	// card in the operator's slot too, so it needs N cards where `derived` needs
	// N-1; a payload that cannot supply N cannot offer the choice, and asking
	// anyway would walk the operator into an under-supply refusal they caused by
	// answering a question the device already knew the answer to.
	selfSource := slotFromSeed
	if state == cosignerSourceLoaded && len(supply) >= p.N {
		src, ok := buildSelfSourceFlow(ctx, th, p.SelfSlots)
		if !ok {
			return
		}
		selfSource = src
	}
	p.SelfFromCard = selfSource == slotFromBoth

	open := p.N - len(p.SelfSlots)
	if p.SelfFromCard {
		open = p.N
	}

	// A BUILD THAT DEMANDS NOTHING TALKS TO NO PAYLOAD (I-6). An operator holding
	// every slot of a 2-of-2 and typing both seeds on the keyboard needs a payload
	// for nothing, and every screen below this point is about resolving one:
	// the under-supply refusal, the gather, the payload review, the card picker
	// and the card decode. Running them for a demand of zero is what produced a
	// refusal saying "this policy needs no cosigner key cards" and "load the
	// payload" in one sentence.
	//
	// SKIPPING THE GATHER IS PART OF THE FIX, NOT A FLOURISH. Letting the flow
	// through classifyCosignerSupply alone lands it in bundleGatherFlow, which
	// needs one complete card to proceed and answers Done on zero with "No
	// complete cards. Pack them on the host with `me sysw pack` and load the
	// payload again." -- the same dead end, one screen later.
	//
	// Pre-S5 this branch was unreachable (`open` was always p.N-1 >= 1); S5's
	// multi-select @S picker is what made it a shape an operator can build.
	var (
		cosigners []mk.Card
		origins   []cosignerOrigin
		mk1s      []bundleCard
		// chosen stays declared out here because buildSlotSources reads it at (4).
		// A zero-demand build leaves it nil, which that projection already handles:
		// open == 0 means every slot is held and !p.SelfFromCard (a `both` build
		// sets open = p.N), so every slot resolves to slotFromSeed with Card = -1.
		chosen []int
	)
	if open > 0 {
		if classifyCosignerSupply(state, len(supply), open) == cosignerRefuse {
			// Refuse BEFORE the gather. Walking the operator into a gather that
			// cannot succeed and dead-ending them there is the defect this stage
			// removes; the message names the only route this hardware has.
			showError(ctx, th, "Build Policy",
				buildSupplyRefusal(state, len(supply), open, incomplete))
			return
		}
		ctx.syswBundleSeeds = records
		cards, ok := bundleGatherFlow(ctx, th, buildCosignerGatherTitle)
		if !ok {
			return
		}
		// md1 records ride along on a systemwide payload and must not fail the build
		// (spec P0 item 3); buildCosignerCards refuses on one, so they are dropped
		// here rather than there.
		mk1s = mk1CosignerCards(cards)
		// Re-classified over what the GATHER produced, not over what the payload
		// held: on reader-equipped hardware the operator may have added more.
		//
		// THE SWITCH IS EXHAUSTIVE ON PURPOSE (fold, N1). `classifyCosignerSupply`
		// is total, but leaving auto-fill as this switch's implicit default made the
		// CALL SITE non-total: a fourth outcome, or any future disagreement between
		// the two classify calls, would have taken the all-cards branch in silence.
		// The ruling that forbids assuming `n-1` deserves a case analysis that says
		// so at both ends.
		outcome := classifyCosignerSupply(cosignerSourceLoaded, len(mk1s), open)
		switch outcome {
		case cosignerRefuse:
			showError(ctx, th, "Build Policy",
				buildSupplyRefusal(cosignerSourceLoaded, len(mk1s), open, false))
			return
		case cosignerAutoFill:
			// Exactly enough: every card fills a slot, in payload record order.
			chosen = make([]int, len(mk1s))
			for i := range chosen {
				chosen[i] = i
			}
		case cosignerSelect:
			// SPEC P0 item 6's review runs on BOTH arms (below); what is bounded to
			// over-supply is the CHOICE. A picker with one possible answer is a tap
			// that teaches nothing.
			if !buildPayloadReviewFlow(ctx, th, mk1s, open, true) {
				return
			}
			chosen, ok = buildCosignerPickFlow(ctx, th, mk1s, open)
			if !ok {
				return
			}
		default:
			showError(ctx, th, "Build Policy",
				"Couldn't work out how the payload's cards fit this policy.")
			return
		}
		if outcome == cosignerAutoFill {
			// SPEC P0 item 6, "ruled here, not deferred": the operator sees a review
			// of what the payload supplied on this arm too. Before the fold their
			// only such screen was the shared gather's "Scan a card, or Done." — a
			// count and an instruction this hardware cannot perform.
			if !buildPayloadReviewFlow(ctx, th, mk1s, open, false) {
				return
			}
		}
		picked := make([]bundleCard, 0, len(chosen))
		for _, i := range chosen {
			picked = append(picked, mk1s[i])
		}
		cosigners, ok = buildCosignerCards(picked, open)
		if !ok {
			// NOT an under-supply refusal (fold, N2). `picked` is all-mk1 and its
			// length equals `open` on both arms, so the count arm of
			// buildCosignerCards cannot fire here and printing "holds 2, needs 2"
			// with a rewrite-the-payload remedy would be two equal counts and a
			// wrong instruction. What is left is a card that passed mk.Decode in the
			// gatherer and failed it here, which is a read failure, so say that.
			showError(ctx, th, "Build Policy",
				"Couldn't read the cosigner key cards from the payload.")
			return
		}
		origins = buildCosignerOrigins(p, chosen)
	}

	// (3) The self seed(s), through the ONE seam, into the REGISTRY -- ONE PER HELD
	// SLOT, each screen naming its slot (SPEC 4.1: the passphrase prompt is per
	// seed, and an unlabelled second prompt is indistinguishable from a repeat of
	// the first).
	//
	// It is NOT keyboard-only, and on this flow that is not a theoretical point:
	// seedEntryFlow is the source picker, and cmd/emu/walk_build_policy.js takes
	// the self seed FROM THE PAYLOAD (the cards blob carries a ClassMnemonic for
	// master A). A comment here claiming "typed-only, never a scan" was already
	// contradicted by a shipped walk when S3 deleted it on 2026-08-15.
	//
	// THE SCRUB IS THE REGISTRY'S, AND IT IS ONE SITE (SPEC 4.2). The `defer` is
	// installed here, before any seed exists, so every exit below -- a Back, a
	// gate FAIL screen, the tail abort, a ctx.Done unwind, a panic -- is covered
	// by construction. buildSeedForSlot registers before it returns, so no seed is
	// ever live and unowned.
	reg := &seedRegistry{}
	defer reg.scrub()
	seedIDs := make([]int, 0, len(p.SelfSlots))
	for _, slot := range p.SelfSlots {
		id, ok := buildSeedForSlot(ctx, th, reg, slot)
		if !ok {
			return
		}
		seedIDs = append(seedIDs, id)
	}

	// (4) THE SEED<->KEY GATE (SPEC 4.3), at construction time and before
	// assembly. On a `both` self slot it derives from the seed at the CARD's own
	// declared origin and compares chain code + pubkey, through findUserSlot.
	sources := buildSlotSources(p, seedIDs, chosen, reg)
	notices, gerr := buildSlotGate(sources, p.Script, reg, cosigners, &chaincfg.MainNetParams)
	if gerr != nil {
		var mm errBuildSeedKeyMismatch
		if errors.As(gerr, &mm) {
			showError(ctx, th, "Key does not match seed",
				buildSeedKeyMismatchMessage(mm, origins))
			return
		}
		var fc errBuildFingerprintContradicts
		if errors.As(gerr, &fc) {
			showError(ctx, th, "Card contradicts seed",
				buildFingerprintContradictsMessage(fc, origins))
			return
		}
		showError(ctx, th, "Build Policy", "Couldn't check your key against your seed.")
		return
	}

	// (4a) SPEC 4.3's review of the assignment, BEFORE assembly. Back abandons.
	if !buildSlotSourceReviewFlow(ctx, th, sources, origins, reg, notices) {
		return
	}

	// (4b) Derive each held slot's key AT THAT SLOT'S OWN ORIGIN (§0.1a's
	// template-aware BIP-48 path, at the slot's own account). deriveAccountXpub
	// neuters (no xprv) + scrubs the seed/master internally.
	//
	// SKIPPED on a `both` slot: M-B makes the CARD authoritative there, and the
	// gate above has already proved the two agree, so deriving a second copy of
	// the same key would only create a second thing that could be wrong.
	self, derr := buildSelfKeys(sources, p.Script, reg, &chaincfg.MainNetParams)
	if derr != nil {
		showError(ctx, th, "Build Policy", "Couldn't derive your key from the seed.")
		return
	}

	// (5) Assemble via the SOLE md1 producer md.EncodeMultisig.
	assembledMd1, stub, slots, err := assembleBuildPolicy(p, self, cosigners)
	if err != nil {
		// SPEC §4.1's refusal is NAMED, not folded into the generic failure. The
		// generic text ("Couldn't assemble the wallet policy.") describes a device
		// problem; a duplicate key is the operator's INPUT and there are two
		// things they can do about it, so both are said.
		var dup errBuildDuplicateKey
		if errors.As(err, &dup) {
			showError(ctx, th, "Duplicate key",
				buildDuplicateKeyMessage(dup, p, origins))
			return
		}
		// SPEC M-1's refusal is NAMED for the same reason: a depth-0 cosigner card
		// is the operator's input and the generic text sends them nowhere.
		var eo errBuildEmptyOrigin
		if errors.As(err, &eo) {
			showError(ctx, th, "Key origin missing",
				buildEmptyOriginMessage(eo, origins))
			return
		}
		showError(ctx, th, "Build Policy", "Couldn't assemble the wallet policy.")
		return
	}

	// (6) Review the (stub, slots) ordering handle (I-ORDER), carrying S1's §0.1
	// announcement of which payload cards filled which slots. Back -> abort.
	//
	// AND THE KEYS, from S5. Before this the screen showed "@0 (no fp)" on the
	// default path, so the operator confirmed a wallet policy whose contents they
	// had never seen and the EXPERIMENTAL warning two screens later asked them to
	// compare it against a coordinator with nothing on the device to compare.
	//
	// A policy whose keys cannot be shown is NOT engraved. §0.1 clause 2 is the
	// rule: permissiveness stops where a wrong assumption would be invisible in
	// every artifact, and a review screen that silently omits a slot's key is that
	// invisibility manufactured on purpose.
	slotKeys, slotOrigins, kerr := buildSlotKeyStrings(assembledMd1, self, cosigners)
	if kerr != nil {
		showError(ctx, th, "Build Policy",
			"Couldn't show the keys this policy holds, so it was not engraved. Build "+
				"again; if it happens twice, rewrite the payload on the host with `me sysw pack`.")
		return
	}
	if !buildReviewFlow(ctx, th, p.Script, stub, slots, slotKeys, p.IncludeFp,
		buildProvenanceLines(origins, len(mk1s)), heldSlotOrigins(p.SelfSlots, slotOrigins)) {
		return
	}

	// (6b) Wallet-policy form: default FULL policy (recommended); opt-in
	// TEMPLATE-only behind the per-shape consent + recovery estimate (DD5/S4/S6).
	// On template, STRIP the assembled md1 to keyless; deriveMultisigLeg then
	// auto-binds the self mk1 to the WDT-Id (form-aware, C2). The supply
	// seed-cross-match flow is left untouched (D1).
	engraveMd1 := assembledMd1
	template := false
	formChoice := &ChoiceScreen{
		Title:   "Engrave wallet policy",
		Lead:    "Which md1?",
		Choices: []string{"Full policy md1", "Template-only md1"},
	}
	if sel, ok := formChoice.Choose(ctx, th); ok && sel == 1 {
		// Refuse render-gap shapes BEFORE engrave (C4). The build path only
		// authors sortedmulti (admitted), but the guard is the single gate.
		if gerr := md.TemplateEngraveShapeGuardChunks(assembledMd1); gerr != nil {
			showError(ctx, th, "Build Policy", "This policy shape can't be safely engraved as a template (unrecoverable with the shipped toolkit). Use the full policy.")
			return
		}
		tmplMd1, terr := md.StripToTemplate(assembledMd1)
		if terr != nil {
			showError(ctx, th, "Build Policy", "Couldn't build the template bundle.")
			return
		}
		if !templateConsentFlow(ctx, th, tmplMd1) {
			return
		}
		engraveMd1 = tmplMd1
		template = true
	}

	// (7) The MANDATORY unskippable EXPERIMENTAL warning (I-WARN). Abort the
	// engrave on Back/ConfirmNo.
	if !multisigBuildExperimentalWarning(ctx, th) {
		return
	}

	// (8) Full vs watch-only.
	//
	// THE "FULL" ROW NAMES WHAT IT LEAVES OUT (S5). A BIP-39 passphrase is a
	// required spending factor and is never engraved, so on a build that used one,
	// "Full (seed + keys)" tells the operator that a partial backup is a complete
	// one. The label is ASKED OF THE REGISTRY rather than tracked beside it: SPEC
	// 4.1 makes the passphrase per-seed, so the honest question is about the set
	// of pairs this flow actually holds, and one passphrased leg among three is
	// enough.
	usedPassphrase := reg.usesPassphrase()
	modeChoice := &ChoiceScreen{Title: "Engrave Mode", Lead: "What to engrave?", Choices: []string{buildFullModeLabel(usedPassphrase), "Watch-only (keys)"}}
	modeSel, ok := modeChoice.Choose(ctx, th)
	if !ok {
		return
	}
	full := modeSel == 0

	// (9) Derive the operator's leg PER HELD SLOT, at that slot's own origin, over
	// the engrave md1 (full policy OR the stripped template; flows EXACTLY like a
	// supplied md1; deriveMultisigLeg binds mk1.Stubs form-aware — WalletPolicyId
	// for full, WDT-Id for template, C2) and engrave.
	legs, engravedSlots, cardsOut, terr := buildEngraveTail(sources, p.Script, reg,
		&chaincfg.MainNetParams, cosigners, engraveMd1, full)
	if terr != nil {
		showError(ctx, th, "Build Policy", "Couldn't derive the bundle from the seed.")
		return
	}

	// (9a) HOW MANY PLATES, BEFORE THE FIRST ONE. Trace B cuts 6-9 plates over
	// hours and the operator committed to that with no count at all. Back here
	// aborts before anything is cut, which is the last moment that is free.
	if !confirmReviewScreen(ctx, th, "Plate Count", buildPlateCensusLines(cardsOut)) {
		return
	}
	// AN ABORT ENDS THE PROGRAM HERE (I-12). Both surfaces below vouch for a
	// COMPLETE set -- a verify over plates that were never all cut, and a restore
	// document headed "This backup is N plates ... If any of them is missing, this
	// backup is incomplete." An operator who just read "Bundle Incomplete: this
	// set is not a usable backup yet" must not be shown either.
	if bundleEngrave(ctx, th, "Build Policy", cardsOut) != bundleEngraveDone {
		return
	}

	// (10) Offer verify-bundle — full policy only. The verify re-derives via the
	// xpub seed-cross-match (findUserSlot), which a KEYLESS template has no xpub
	// to match (D1); the template's binding is the device's own form-aware
	// readback, already established at engrave. So a template engrave skips the
	// cross-match verify offer.
	//
	// PER LEG, from S5. multisigVerifyFlow reads back EVERY engraved key plate,
	// re-derives every leg the operator's seeds account for, and requires the
	// pairing to be a bijection -- so a three-plate build cannot report Verify OK
	// having compared one. `full` is the mode, which is what says whether an ms1
	// must be hand-typed; the legs themselves are re-derived from a RE-TYPED seed
	// rather than handed over from here (§7.4: comparing the engrave source
	// against itself passes unconditionally).
	//
	// engravedSlots IS THE OBLIGATION LIST, and it is passed rather than
	// recomputed. The verify's derive loop asks a re-typed seed which slots it
	// FILLS; that set is not the set the engrave cut a plate for, and the
	// difference is not hypothetical -- one seed can fill several slots at
	// distinct origins with distinct keys. Handing over the tail's own held-slot
	// indices is what keeps "every leg must find its plate" a statement about
	// this run's steel instead of about the policy.
	//
	// engraveMd1 IS THE OTHER HALF OF IT. Slot indices alone are re-based onto
	// whatever policy the readback supplies, so a byte-valid plate set from a
	// DIFFERENT wallet -- the same cosigners at another threshold, say, from the
	// generation this run supersedes -- satisfies the same integers and reports
	// "Verify OK" while this run's steel is never read. This flow has held the
	// closing datum all along; it hands it over now. It is the md1 that was
	// ENGRAVED, not assembledMd1, because the plate the operator will present is
	// the one that came off this machine.
	//
	// AND THE OFFER LOOPS (I-4), for the reason spelled out at the supply path's
	// own offer: the incomplete screen prescribed a re-run this device could not
	// perform. Only verifyComplete falls through to the restore document.
	//
	// IT DISPATCHES THROUGH multisigVerifyFn, the in-file test seam, for the same
	// reason the supply path does: without it this loop is unreachable from any
	// executing test and its row-to-index mapping is unpinned (B4).
	//
	// ONE RECORD FOR THE WHOLE LOOP, AND IT IS DECLARED OUTSIDE THE GATE ABOVE as
	// well as outside the loop: the restore document below is drawn on runs that
	// never entered the verify at all (a template, or a build holding no leg of
	// its own), and on those the zero value is the truthful status. Sticky, for
	// the supply path's reason -- an earlier attempt that found a bad plate is not
	// erased by a later one that passed.
	var rec verifyRecord
	if !template && len(legs) > 0 {
		lead, choices := "Verify the engraved plates?", []string{"Verify now", "Skip"}
		for {
			verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: lead, Choices: choices}
			sel, ok := verifyChoice.Choose(ctx, th)
			if !ok || sel != 0 {
				break
			}
			res := multisigVerifyFn(ctx, th, full, engravedSlots, engraveMd1, &rec)
			if res != verifyIncomplete && res != verifyFailed {
				break
			}
			lead = multisigVerifyRetryLead
			choices = []string{"VERIFY AGAIN", "CONTINUE"}
		}
	}

	// (11) Restore doc (display-only, PUBLIC). A full policy expands to per-key
	// origins; a keyless template has no xpubs to render, so the restore doc is
	// skipped for the template form (the template-id consent already shown).
	if !template {
		tpl, keys, err := md.ExpandWalletPolicyChunks(assembledMd1)
		if err != nil {
			showError(ctx, th, "Build Policy", "Couldn't decode the assembled policy.")
			return
		}
		// (11a) AND WHAT THE SET IS, afterwards. The restore doc is read years
		// later, alone, often by someone who was not the operator, and a plate
		// count is the one fact that tells them whether they are holding all of it.
		//
		// PER SEED, not the label's single bit (I-5). The mode label asks "is this
		// set short of a factor" and a bool answers it; the document has to say
		// WHICH seed, because a build holding three slots can carry three different
		// passphrases and "keep the passphrase somewhere separate" has one referent.
		//
		// THE STATUS IS WHAT THE LOOP ABOVE RECORDED. A run that skipped the verify,
		// or never reached it, leaves `rec` at its zero value and this document
		// claims nothing; an empty string would render as silence, and silence is
		// what reads as a pass.
		multisigRestoreDocFlow(ctx, th, tpl, keys,
			buildVerifyStatusLine(rec),
			buildPlateInventoryLines(cardsOut, reg.passphraseFacts(), seedCapacityMany))
	}
}

// buildSlotSources projects the flow's parameters onto SPEC 4.3's per-slot
// source model.
//
// It is a pure function of what the operator already chose, so the model and the
// assembly cannot disagree about which slot holds what: `chosen` is the same
// card list assembleBuildPolicy fills from, walked in the same order, and the
// self slot is included or skipped by the same p.SelfFromCard.
//
// `seedIDs[i]` is the seed the operator entered for `p.SelfSlots[i]`, so the two
// slices are index-aligned by construction of the caller's loop.
//
// THE ACCOUNT IS ASSIGNED HERE, AND IT IS KEYED ON THE MASTER, NOT ON THE SEED
// ID. A held slot's account is its ordinal among the held slots that share a
// master fingerprint: the first slot a master fills gets account 0, the second
// account 1, and so on. Keying on the seedID instead would mint the SAME key
// twice whenever one master was registered twice -- which is exactly what the
// per-slot entry screens do when the operator types one seed for two held slots
// -- and §4.1's duplicate-key refusal would then reject a legitimate
// multi-account wallet. Trace B is that wallet: @0 = A account 0, @1 = A account
// 1, @2 = B account 0.
func buildSlotSources(p buildPolicyParams, seedIDs []int, chosen []int, reg *seedRegistry) []slotSource {
	out := make([]slotSource, p.N)
	held := map[int]int{}
	for i, s := range p.SelfSlots {
		held[s] = i
	}
	// masterFP -> how many held slots that master already fills.
	accounts := map[uint32]uint32{}
	nextAccount := func(seedID int) uint32 {
		seed, ok := reg.at(seedID)
		if !ok {
			return 0
		}
		a := accounts[seed.MasterFP]
		accounts[seed.MasterFP] = a + 1
		return a
	}
	gi := 0
	for slot := 0; slot < p.N; slot++ {
		hi, isHeld := held[slot]
		seedID := -1
		if isHeld && hi < len(seedIDs) {
			seedID = seedIDs[hi]
		}
		if isHeld && !p.SelfFromCard {
			out[slot] = slotSource{
				Kind: slotFromSeed, SeedID: seedID, Account: nextAccount(seedID), Card: -1,
			}
			continue
		}
		kind := slotFromCard
		id := -1
		if isHeld {
			kind = slotFromBoth
			id = seedID
		}
		card := -1
		if gi < len(chosen) {
			card = gi
		}
		out[slot] = slotSource{Kind: kind, SeedID: id, Card: card}
		gi++
	}
	return out
}

// heldSlotKey is one slot the operator fills FROM A SEED: the key derived for it
// and the origin it was derived AT.
//
// The origin travels with the key rather than being recomputed at assembly, so
// the path the device DERIVED at and the path it DECLARES on the plate cannot
// come apart. A `both` slot has no entry here: SPEC M-B makes the card
// authoritative there, so the slot is filled from the card like any other.
type heldSlotKey struct {
	Slot     int
	Xpub     string
	MasterFP uint32
	Origin   bip32.Path
}

// buildSelfKeys derives one key per DERIVED held slot, at that slot's own origin
// (§0.1a's template-aware BIP-48 path, at the slot's own account).
//
// SKIPPED ON A `both` SLOT, which is step (4b)'s pre-S5 rule unchanged: M-B makes
// the card authoritative there and the gate has already proved the two agree, so
// deriving a second copy of the same key would only create a second thing that
// could be wrong.
func buildSelfKeys(sources []slotSource, script md.MultisigScript, reg *seedRegistry, net *chaincfg.Params) ([]heldSlotKey, error) {
	var out []heldSlotKey
	for slot, s := range sources {
		if s.Kind != slotFromSeed {
			continue
		}
		seed, ok := reg.at(s.SeedID)
		if !ok {
			return nil, errBuildSlotAssignment{Slot: slot}
		}
		origin := derivedSlotOrigin(script, s.Account)
		x, fp, err := deriveAccountXpub(seed.Mnemonic, seed.Passphrase, net, origin)
		if err != nil {
			return nil, err
		}
		out = append(out, heldSlotKey{Slot: slot, Xpub: x, MasterFP: fp, Origin: origin})
	}
	return out, nil
}

// buildSeedForSlot is seed entry PLUS its own passphrase, registered before it
// returns.
//
// TWO SPEC CLAUSES MEET HERE.
//
// 4.1: "The passphrase prompt is PER SEED, asked at that seed's entry, and the
// (seed, passphrase) pair is the derivation unit everywhere this spec says
// seed." One flow-global passphrase applied to N seeds would mint keys the
// operator can only re-derive with a pairing they never chose, and for a
// new-seed slot there is no card to cross-check, so no row of 4.3 could catch
// it. The passphrase is therefore asked HERE, inside the per-seed entry, and not
// once for the flow.
//
// And the screens NAME THE SLOT. With several seeds in one flow an unlabelled
// second prompt is indistinguishable from a repeat of the first, and a
// passphrase entered against the wrong slot mints a key no 4.3 row can catch.
func buildSeedForSlot(ctx *Context, th *Colors, reg *seedRegistry, slot int) (int, bool) {
	label := fmt.Sprintf("@%d", slot)
	mnemonic, ok := seedEntryFlowTitled(ctx, th, "Seed for "+label, label)
	if !ok {
		return 0, false
	}
	if buildMultisigSeedHook != nil {
		buildMultisigSeedHook(mnemonic)
	}
	// Registered IMMEDIATELY, before the passphrase screens can return early:
	// from this line on the registry's deferred scrub owns these words. The
	// passphrase is then bound to the same entry.
	seedID, err := reg.add("your seed for "+label, mnemonic, "", &chaincfg.MainNetParams)
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't read that seed.")
		return 0, false
	}
	ppChoice := &ChoiceScreen{
		Title:   "Passphrase " + label,
		Lead:    "Add a BIP-39 passphrase?",
		Choices: []string{"Skip", "Add passphrase"},
	}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		// §3.3.2 admits ClassPassphrase to this program, so the payload is
		// offered before the keyboard (plan stage 13b). NOT passphraseFlow: see
		// syswPassphraseFlow for the two normative rules a shared edit inside
		// passphraseFlow would have broken.
		if pass, ok := syswPassphraseFlowTitled(ctx, th, "Passphrase "+label); ok {
			if err := reg.bindPassphrase(seedID, pass, &chaincfg.MainNetParams); err != nil {
				showError(ctx, th, "Build Policy", "Couldn't apply that passphrase.")
				return 0, false
			}
		}
	}
	return seedID, true
}

// templateConsentFlow shows the per-shape consent surface (classifiable k-of-N
// OR honest-minimal complex + depth-≥2 experimental gate) for a stripped
// template md1, then the loud warning. Returns false on Back/abort (fall back to
// full policy / cancel). It classifies the template via md.DecodeChunks +
// md.TapTreeDepthChunks and roots the displayed template-id on the WDT-Id stub.
func templateConsentFlow(ctx *Context, th *Colors, tmplMd1 []string) bool {
	tmpl, err := md.DecodeChunks(tmplMd1)
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't classify the template policy.")
		return false
	}
	depth, err := md.TapTreeDepthChunks(tmplMd1)
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't classify the template policy.")
		return false
	}
	stub, err := md.FormAwareStubChunks(tmplMd1)
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't compute the template id.")
		return false
	}
	return confirmReviewScreen(ctx, th, "Template-only md1", templateConsentLines(tmpl, stub, depth))
}

// multisigBuildExperimentalWarning is the MANDATORY, unskippable, operator-
// acknowledged warning shown immediately before any Build-path engrave (I-WARN):
// the device-authored policy is NOT validated end-to-end (no coordinator /
// hardware round-trip), so the operator MUST verify the assembled descriptor +
// the shown stub/per-slot fingerprints against their coordinator BEFORE funding.
// Hold to confirm; Back/ConfirmNo returns false and the caller ABORTS the
// engrave. There is no skip/setting path. Mirrors childSeedWarning.
//
// The BODY is a separate function so the F-185 class check can render it without
// driving the whole flow (gui/modal_fits_test.go).
func multisigBuildExperimentalWarning(ctx *Context, th *Colors) bool {
	warn := &ConfirmWarningScreen{
		Title: "EXPERIMENTAL",
		Body:  multisigBuildExperimentalWarningBody(),
		Icon:  assets.IconHammer,
	}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, res := warn.Layout(ctx, th, dims)
		switch res {
		case ConfirmNo:
			return false
		case ConfirmYes:
			return true
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// multisigBuildExperimentalWarningBody is the warning's text.
//
// IT USED TO TEACH A CHECK THAT CANNOT CHECK. The shipped body said "verify the
// assembled descriptor and the shown policy stub + per-slot fingerprints against
// your coordinator/wallet BEFORE funding", and the fingerprint half of that is
// void twice over:
//
//   - fingerprints are OMITTED BY DEFAULT. buildFingerprintChoice offers
//     Omit first, so on the default path every slot on the review screen reads
//     "no fingerprint" and there is nothing to compare at all.
//   - present, they are CARD-SELF-DECLARED AND UNBOUND TO THE KEY (mk/mk.go).
//     cosignerFromCard reads Fingerprint off the card string; nothing derives it
//     from the xpub beside it. An attacker who substitutes a key writes the
//     matching fingerprint next to it at no cost, and the operator's comparison
//     passes on the forged card.
//
// A ritual that cannot fail is worse than silence: it spends the operator's
// attention and hands back a false negative. So this asks for a comparison that
// CAN fail -- the keys or the descriptor, against a wallet this device did not
// author -- states that a matching fingerprint is not that comparison and why,
// and names the check that actually settles it. The fingerprint choice's effect
// on the policy id has moved to the review screen, which is where the choice is
// shown and where an operator can still act on it.
//
// NO EM-DASH. This body carried one, and S2's whole-walk raster floor measured
// what that cost: 4973 ink pixels against a 5482 px title-only frame, i.e. the
// BODY DID NOT DRAW AT ALL on the one screen that is the operator's last chance
// to stop. F-78's "zero-pixel glyph" understates it: the glyph takes its whole
// line with it.
//
// ITS LENGTH IS GATED, not judged (F-185). A modal's body scrolls and nothing on
// the frame says so, and this machine has no button that scrolls it, so a
// sentence past the fold is a sentence the operator is never told exists.
// gui/modal_fits_test.go renders this body and compares the DRAWN FRAME against
// this string, with 80 characters of headroom to spare.
func multisigBuildExperimentalWarningBody() string {
	return "Nothing outside this device has checked this policy. Before you fund it, " +
		"compare the keys you just reviewed, or the descriptor on the restore doc, " +
		"against the same wallet in your coordinator.\n" +
		"A matching fingerprint is not that check: a card states its own, and nothing " +
		"binds it to the key.\n" +
		"What settles it is restoring these plates in your coordinator and seeing your " +
		"own first receive address.\n\nHold button to confirm."
}

// buildCosignerCards filters the gathered cards down to EXACTLY `want` cosigner
// mk1 cards (cardMK1), decoding each to an mk.Card. It refuses (ok=false) when
// the count != want or any md1/ms1 card is present (the Build path gathers KEYS,
// not a descriptor). Order is gather order (I-ORDER fills remaining slots in this
// order).
func buildCosignerCards(cards []bundleCard, want int) ([]mk.Card, bool) {
	var out []mk.Card
	for _, c := range cards {
		switch c.kind {
		case cardMK1:
			card, err := mk.Decode(c.strings)
			if err != nil {
				return nil, false
			}
			out = append(out, card)
		case cardMD1, cardMS1:
			return nil, false // the Build path gathers cosigner KEYS only.
		}
	}
	if len(out) != want {
		return nil, false
	}
	return out, true
}

// multisigScriptChoices is the bounded template picker's list (LOCKED: all three
// sortedmulti wrappers; wsh highlighted by being index 0 / the default choice).
func multisigScriptChoices() []string {
	return []string{
		"wsh (native segwit)",
		"sh(wsh) (nested segwit)",
		"sh (legacy)",
	}
}

// multisigScriptFor maps a template-picker index to the shipped MultisigScript
// enum (1:1, order-locked with multisigScriptChoices).
func multisigScriptFor(idx int) md.MultisigScript {
	switch idx {
	case 0:
		return md.MultisigWsh
	case 1:
		return md.MultisigShWsh
	default:
		return md.MultisigSh
	}
}

// multisigTemplatePick shows the bounded template ChoiceScreen and returns the
// chosen MultisigScript. ok==false on Back.
func multisigTemplatePick(ctx *Context, th *Colors) (md.MultisigScript, bool) {
	cs := &ChoiceScreen{Title: "Template", Lead: "Choose policy type", Choices: multisigScriptChoices()}
	idx, ok := cs.Choose(ctx, th)
	if !ok {
		return md.MultisigWsh, false
	}
	return multisigScriptFor(idx), true
}

// n ∈ 2..5 (LOCKED). The encoder guards n<=32 regardless; this cap is a UX/plate
// ceiling. multisigNChoices/multisigNFor are index-aligned.
func multisigNChoices() []string { return []string{"2", "3", "4", "5"} }
func multisigNFor(idx int) int   { return idx + 2 }

// k ∈ 1..n (LOCKED), built from the chosen n so k>n is structurally unreachable.
func multisigKChoices(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("%d", i+1)
	}
	return out
}
func multisigKFor(idx int) int { return idx + 1 }

// The self-slot @S picker: "@0".."@{n-1}". The chosen index IS the slot.
func multisigSelfSlotChoices(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("@%d", i)
	}
	return out
}

// multisigRemainingSlotChoices lists the slots NOT yet held, with the slot each
// row maps to. The two slices are index-aligned, so a picker's chosen row can
// never be read as a slot number directly -- which it is not, once @0 has been
// taken and the first row means @1.
func multisigRemainingSlotChoices(n int, held []int) (labels []string, slots []int) {
	got := make(map[int]bool, len(held))
	for _, s := range held {
		got[s] = true
	}
	for i := 0; i < n; i++ {
		if got[i] {
			continue
		}
		labels = append(labels, fmt.Sprintf("@%d", i))
		slots = append(slots, i)
	}
	return labels, slots
}

// multisigSelfSlotPickFlow is the @S picker as a SET, which is what lets the
// operator hold SEVERAL of a policy's slots.
//
// WHY IT IS COMPOSED OUT OF ChoiceScreens RATHER THAN A NEW WIDGET. There is no
// multi-select screen in this package -- measured, zero hits for
// MultiSelect/Checkbox/toggle-list over gui/*.go -- and the shipped idiom for
// "produce a subset as []int" is buildCosignerPickFlow's loop of bounded
// ChoiceScreens. A toggle-list would need a selection MARKER, and a marker is
// one more glyph that can raster to nothing on the one screen that decides
// which keys are the operator's (F-78/F-151 is that failure twice already). So
// every screen here is a widget that already ships and is already drawn
// somewhere else in this flow.
//
// THE FIRST SCREEN IS THE SHIPPED ONE, CHARACTER FOR CHARACTER. "Which slot is
// your key?" is a pinned walk needle with exactly one production site
// (cmd/emu/needle_test.go), FOUR walk drivers anchor on it -- walk_build_policy,
// walk_s3_nested, walk_s4_gate and walk_trace_b, each declaring its own
// NEEDLE_SLOT -- and its default row is @0. So accepting every default produces
// {@0}: the pre-S5 single-select behaviour unchanged, which is what keeps every
// existing test and walk meaning what it meant. (This said "three", and "three"
// was TRUE when it was written on 2026-08-15; walk_trace_b landed the next day
// and nothing counted again. S6a step 8's re-sweep did. A comment that states a
// count over a growing set goes stale by other people's work, not by its own
// author's mistake, which is why the drivers are now NAMED rather than tallied.)
//
// BACK IS NEVER A WAY TO CONFIRM. Back at any of the three surfaces returns
// ok=false, which buildParamPickFlow reads as "step back one stage" -- the rule
// it applies to every stage above the first. An operator who has picked @0, been
// asked about a second slot and pressed Back has not said "just @0"; the screen
// has a row that says that.
//
// The returned set is ASCENDING and distinct, which is what buildPolicyParams
// documents and what buildSlotSources' account numbering walks.
func multisigSelfSlotPickFlow(ctx *Context, th *Colors, n int) ([]int, bool) {
	first := &ChoiceScreen{
		Title:   "Your slot",
		Lead:    "Which slot is your key?",
		Choices: multisigSelfSlotChoices(n),
	}
	idx, ok := first.Choose(ctx, th)
	if !ok {
		return nil, false
	}
	held := []int{idx}
	for len(held) < n {
		more := &ChoiceScreen{
			Title:   "Your slots",
			Lead:    "Do you hold another slot?",
			Choices: []string{"NO, THAT IS ALL", "YES, ONE MORE"},
		}
		sel, ok := more.Choose(ctx, th)
		if !ok {
			return nil, false
		}
		if sel != 1 {
			break
		}
		labels, slots := multisigRemainingSlotChoices(n, held)
		if len(slots) == 1 {
			// A PICKER WITH ONE POSSIBLE ANSWER IS A TAP THAT TEACHES NOTHING.
			// This package's own rule (syswSeedPicker says so in those words;
			// buildCosignerPickFlow short-circuits on it), and it is not merely
			// tidying: a screen offering one row invites the operator to believe
			// they chose between alternatives that were never there.
			held = append(held, slots[0])
			continue
		}
		pick := &ChoiceScreen{
			Title:   "Your slots",
			Lead:    "Which other slot is yours?",
			Choices: labels,
		}
		j, ok := pick.Choose(ctx, th)
		if !ok {
			return nil, false
		}
		held = append(held, slots[j])
	}
	slices.Sort(held)
	return held, true
}

// The fp-presence picker (HOMOGENEOUS): Omit (index 0, default) -> no fp TLVs on
// any slot; Include (index 1) -> every slot's master fp.
func multisigFpChoices() []string       { return []string{"No (omit)", "Yes (include)"} }
func multisigIncludeFpFor(idx int) bool { return idx == 1 }

// buildPolicyParams is the assembled shape the operator picked.
type buildPolicyParams struct {
	Script md.MultisigScript
	N      int
	K      int
	// SelfSlots is the SET of slots the operator HOLDS -- S5's multi-slot self,
	// where before S5 this was a single `SelfSlot int`. Entries are slot indices
	// in 0..N-1, ascending and distinct; the picker produces exactly one today
	// and the model accepts several (Trace B holds @0, @1 and @2).
	//
	// EACH HELD SLOT GETS ITS OWN BIP-48 ACCOUNT, assigned by buildSlotSources as
	// the slot's ordinal among the held slots sharing a MASTER. That is what keeps
	// two slots held from one seed off §4.1's duplicate-key refusal: same master,
	// different account, different key.
	SelfSlots []int
	IncludeFp bool // homogeneous fp-presence
	// SelfFromCard is S4's `both` assignment on the operator's own slot(s): their
	// key arrives on a payload CARD, and the seed they supply is what the gate
	// checks it against rather than what the slot is filled from. It applies to
	// EVERY entry of SelfSlots, because the flow asks the question once; the
	// per-slot model (slotSource) can express a mixture and assembleBuildPolicy
	// reads the mixture off the held-key set rather than off this flag.
	//
	// SPEC M-B is why it changes the fill and not only the check: "in a `both`
	// slot the card's declared origin and key are AUTHORITATIVE". The gate has
	// already proved the two agree, so the choice is not about which key ends up
	// in the policy -- it is about which one the artifacts describe.
	//
	// THE ZERO VALUE IS THE SHIPPED BEHAVIOUR. false means @SelfSlot is filled
	// from the derived self key and `cosigners` holds N-1 cards, exactly as
	// before S4, so every existing caller and test literal is unaffected by
	// construction.
	SelfFromCard bool
}

// Picker stage indices for buildParamPickFlow's stage-loop. Back from any stage
// > stageTemplate steps back ONE stage; Back from stageTemplate abandons.
const (
	stageTemplate = iota // template (script kind)
	stageN               // cosigner count n
	stageK               // threshold k (range depends on n)
	stageSelfSlot        // self-slot @S (range depends on n)
	stageFp              // fingerprint presence
	stageDone            // all picked
)

// buildParamPickFlow runs the bounded pickers in order: template -> n -> k(n) ->
// self-slot @S -> fp-presence. Back navigates back ONE stage through the full
// sequence; Back from the FIRST stage (template) abandons the Build flow
// (ok==false). k's and @S's ranges depend on n and are re-derived whenever those
// stages are (re-)entered, so changing n upstream correctly re-bounds them. Every
// returned param is in-range by construction (no free-form widget exists).
func buildParamPickFlow(ctx *Context, th *Colors) (buildPolicyParams, bool) {
	var p buildPolicyParams
	stage := stageTemplate
	for stage != stageDone {
		switch stage {
		case stageTemplate:
			script, ok := multisigTemplatePick(ctx, th)
			if !ok {
				return p, false // Back from the first stage -> abandon the Build flow.
			}
			p.Script = script
			stage = stageN
		case stageN:
			nCS := &ChoiceScreen{Title: "Cosigners", Lead: "How many keys (n)?", Choices: multisigNChoices()}
			nIdx, ok := nCS.Choose(ctx, th)
			if !ok {
				stage = stageTemplate // Back -> re-pick template.
				continue
			}
			p.N = multisigNFor(nIdx)
			stage = stageK
		case stageK:
			kCS := &ChoiceScreen{Title: "Threshold", Lead: fmt.Sprintf("Required signatures (k of %d)?", p.N), Choices: multisigKChoices(p.N)}
			kIdx, ok := kCS.Choose(ctx, th)
			if !ok {
				stage = stageN // Back -> re-pick n (which re-bounds k/@S).
				continue
			}
			p.K = multisigKFor(kIdx)
			stage = stageSelfSlot
		case stageSelfSlot:
			// A SET, from S5's multi-select picker. It cannot be empty (the first
			// screen's pick is mandatory) and it cannot repeat a slot (each round
			// picks from what is left), so the emptiness guard at the top of
			// buildMultisigPolicyFlow and assembleBuildPolicy's duplicate-slot
			// check are both backstops rather than reachable arms.
			slots, ok := multisigSelfSlotPickFlow(ctx, th, p.N)
			if !ok {
				stage = stageK // Back -> re-pick k.
				continue
			}
			p.SelfSlots = slots
			stage = stageFp
		case stageFp:
			fpCS := &ChoiceScreen{Title: "Fingerprints", Lead: "Include key fingerprints?", Choices: multisigFpChoices()}
			fpIdx, ok := fpCS.Choose(ctx, th)
			if !ok {
				stage = stageSelfSlot // Back -> re-pick @S.
				continue
			}
			p.IncludeFp = multisigIncludeFpFor(fpIdx)
			stage = stageDone
		}
	}
	return p, true
}

// buildCosignerGatherTitle is D-4's fix: the Build path's cosigner gather names
// what the operator is doing instead of naming a different program.
//
// It is a CONSTANT, and a single-site one, because cmd/emu/needle_test.go pins
// the walk's anchors to strings with exactly one production site — and a walk
// that could finally identify this screen is the second thing D-4 buys. Before
// the fix this gather and the bundle program's gather were identical character
// for character (measured 2026-08-14), so no walk could tell them apart.
//
// (This comment deliberately does not quote the OLD title. needle_test.go counts
// a needle's production sites by blunt substring match over gui's source, so a
// comment carrying a literal is counted as a flow that draws it.)
const buildCosignerGatherTitle = "Cosigner Keys"

// errBuildSlotCount is a structurally impossible slot set: a held slot outside
// 0..n-1, one slot held twice, or a cosigner count that is not exactly the
// number of slots no held key fills. Before S5 it read "cosigner count != n-1",
// which was the same condition when the operator could hold only one slot.
var errBuildSlotCount = errors.New("multisig build: the slot set and the cosigner cards do not fit")

// errBuildDuplicateKey is SPEC §4.1's refusal: two slots of the assembled policy
// hold the same key. It carries BOTH slot indices so the flow can name them with
// the provenance it already holds — "your key" vs "payload card N" — instead of
// printing a generic failure the operator cannot act on.
type errBuildDuplicateKey struct{ SlotA, SlotB int }

func (e errBuildDuplicateKey) Error() string {
	return fmt.Sprintf("multisig build: slots @%d and @%d hold the same key", e.SlotA, e.SlotB)
}

// duplicateSlotPair reports the first pair of slots carrying an IDENTICAL 65-byte
// chain code ‖ compressed pubkey, which is SPEC §4.1's comparison verbatim.
//
// THE BASIS IS THE PART MOST EASILY GOT WRONG, so it is written down. Both
// rejected alternatives are worse in opposite directions:
//
//   - MASTER FINGERPRINT identifies a MASTER, not a key. It would refuse the
//     legitimate multi-account wallet — one master contributing account 0 and
//     account 1 as two distinct cosigners, which is exactly the shape the
//     delivered payload carries as cards A@0 and A@1. It is also absent by
//     default (fp-presence Omit is index 0), so it would usually not be there
//     to compare.
//   - BASE58 XPUB carries parent-fingerprint/depth metadata that legitimately
//     differs between two sources of the SAME key, and md/expand.go drops it
//     anyway. Comparing it would miss a real duplicate that arrived by two
//     routes.
//
// cc‖pk is exact in both directions: identical xpubs derive identical child keys
// at every address index, and differing chain codes derive differing children
// even under an equal parent pubkey. No missed duplicate, no refused legitimate
// setup. Machine-checked both ways in multisig_build_dupkey_test.go.
//
// [32]byte and [33]byte are comparable, so `==` here IS the byte comparison.
func duplicateSlotPair(all []md.MultisigCosigner) (int, int, bool) {
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[i].ChainCode == all[j].ChainCode &&
				all[i].CompressedPubkey == all[j].CompressedPubkey {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// errBuildEmptyOrigin is spec M-1's refusal: a cosigner card declares an origin
// with NO components -- a depth-0 card, spelt "m". It carries the slot and the
// card's own spelling, so the refusal can quote the operator's input back rather
// than paraphrase it.
//
// It is NOT S2's foreign-origin refusal under a new name. That one refused every
// card whose origin was not the shared one and S5 deletes it; this one refuses
// the single shape md itself cannot encode, and it exists so the operator meets
// a screen naming their card rather than "Couldn't assemble the wallet policy".
type errBuildEmptyOrigin struct {
	Slot     int
	Declared string
}

func (e errBuildEmptyOrigin) Error() string {
	return fmt.Sprintf("multisig build: slot @%d declares origin %q, which has no path components",
		e.Slot, e.Declared)
}

// buildEmptyOriginMessage is the operator-facing refusal. It says which slot,
// what the card claims, that nothing was cut, and the routes that exist on this
// hardware (§0.1b -- the payload and the keyboard, never "scan a card").
//
// NO EM-DASH: it is a zero-pixel glyph in the body face and a line containing
// one does not draw at all (F-78/F-151).
func buildEmptyOriginMessage(e errBuildEmptyOrigin, origins []cosignerOrigin) string {
	who := ""
	for _, o := range origins {
		if o.slot == e.Slot {
			who = fmt.Sprintf(" (payload card %d)", o.card)
			break
		}
	}
	declared := e.Declared
	if declared == "" {
		declared = "m"
	}
	return fmt.Sprintf("The key card for slot @%d%s says its key was derived at %q, "+
		"the master key itself, with no derivation path at all. A wallet policy has to "+
		"record where every key came from, so this card cannot be placed in one. "+
		"Nothing was engraved.\n\n"+
		"Use a card derived at an account path such as %s, or rewrite the payload on "+
		"the host with `me sysw pack`.",
		e.Slot, who, declared, multisigSharedOrigin().String())
}

// buildSlotProvenance names one slot for a refusal, using what the flow already
// knows: the self-slot the operator picked and the card→slot map
// buildCosignerOrigins built. A refusal that says "slots @0 and @1" and stops
// leaves the operator to work out which of their inputs to change.
// `p.SelfFromCard` is S4's `both` assignment: the operator's own slot is filled
// from a payload card too, so "your key, from your seed" would be a wrong claim
// about where it came from and would send them to change the wrong input.
//
// It takes the whole `p` from S5 because the question is now set membership --
// "is this one of the slots the operator holds" -- and a single index cannot ask
// it.
func buildSlotProvenance(slot int, p buildPolicyParams, origins []cosignerOrigin) string {
	held := false
	for _, s := range p.SelfSlots {
		if s == slot {
			held = true
			break
		}
	}
	if held && !p.SelfFromCard {
		return fmt.Sprintf("slot @%d (your key, from your seed)", slot)
	}
	for _, o := range origins {
		if o.slot == slot {
			if held {
				return fmt.Sprintf("slot @%d (your key, payload card %d)", slot, o.card)
			}
			return fmt.Sprintf("slot @%d (payload card %d)", slot, o.card)
		}
	}
	// Unreachable while every non-self slot comes from a payload card; kept so a
	// future source that forgets to register provenance degrades to a slot
	// number rather than to a wrong claim about where the key came from.
	return fmt.Sprintf("slot @%d", slot)
}

// buildDuplicateKeyMessage is the operator-facing refusal. Every sentence is
// load-bearing: WHICH slots, WHY it is harm, that NOTHING was cut, and both
// routes that exist on this hardware (§0.1b — the payload and the keyboard, never
// "scan a card").
//
// No em-dash: it is a zero-pixel glyph in poppins.Regular16, the body face (F-78,
// re-measured for this text). The "Duplicate key" heading is the modal TITLE.
func buildDuplicateKeyMessage(e errBuildDuplicateKey, p buildPolicyParams, origins []cosignerOrigin) string {
	a := buildSlotProvenance(e.SlotA, p, origins)
	b := buildSlotProvenance(e.SlotB, p, origins)
	return fmt.Sprintf("%s and %s hold the SAME key. A policy that repeats a key can be "+
		"spent by fewer different keys than its k-of-n says. Nothing was engraved. "+
		"Build again and choose different cards, or use a different seed; if the "+
		"payload has no other cards, rewrite it on the host with `me sysw pack`.",
		strings.ToUpper(a[:1])+a[1:], b)
}

// multisigSharedOrigin is the LOCKED shared origin for OriginShared mode: the
// BIP-48 P2WSH multisig account path m/48'/0'/0'/2' (matches T6b / pathPickerFlow
// BIP-48). Self and every cosigner declare this single shared origin.
func multisigSharedOrigin() bip32.Path {
	const h = hdkeychain.HardenedKeyStart
	return bip32.Path{48 | h, 0 | h, 0 | h, 2 | h}
}

// fpBytes converts a uint32 master fingerprint to the 4-byte big-endian form the
// encoder's MultisigCosigner.Fingerprint expects.
func fpBytes(fp uint32) [4]byte {
	return [4]byte{byte(fp >> 24), byte(fp >> 16), byte(fp >> 8), byte(fp)}
}

// cosignerFromCard parses ONE gathered cosigner mk.Card into a MultisigCosigner.
// includeFp drives HOMOGENEOUS fp-presence: when true the card's 8-hex
// Fingerprint is decoded to 4 bytes (a missing fp under Include is an error so
// the policy stays homogeneous); when false no fp is set.
//
// THE CARD'S DECLARED ORIGIN IS CARRIED, from S5 (spec R-3). Before S5 it was
// DISCARDED, because OriginShared mode declares one origin for the whole policy;
// that was correct for a card actually minted at m/48'/0'/0'/2' and a lie for
// any other, so S2 refused every other card outright rather than stamp a path
// the card disagreed with. S5 gives each slot its own origin, which makes the
// card's own answer the one the policy records.
//
// PERMISSIVE ON SPELLING, STRICT ON VALUE (§0.1): the origin is carried as
// PARSED COMPONENTS, so m/48'/0'/0'/2' and m/48h/0h/0h/2h are the same origin
// and neither notation changes the assembled bytes. A path that does not PARSE
// is refused here rather than stamped over: an origin the device cannot read is
// one it cannot honour.
func cosignerFromCard(card mk.Card, includeFp bool) (md.MultisigCosigner, error) {
	cc, pk, _, err := decodeXpubBytes(card.Xpub)
	if err != nil {
		return md.MultisigCosigner{}, err
	}
	origin, err := bip32.ParsePath(card.Path)
	if err != nil {
		return md.MultisigCosigner{}, err
	}
	c := md.MultisigCosigner{
		ChainCode:        cc,
		CompressedPubkey: pk,
		Origin:           originComponents(origin),
	}
	if includeFp {
		if card.Fingerprint == "" {
			return md.MultisigCosigner{}, errors.New("multisig build: Include selected but a cosigner card has no fingerprint")
		}
		raw, err := hex.DecodeString(card.Fingerprint)
		if err != nil || len(raw) != 4 {
			return md.MultisigCosigner{}, errors.New("multisig build: bad cosigner fingerprint")
		}
		var fp [4]byte
		copy(fp[:], raw)
		c.Fingerprint = fp
		c.FpPresent = true
	}
	return c, nil
}

// assembleBuildPolicy is the SOLE md1-bytes producer call site for the Build
// path (I-VERBATIM). It places each held key at its own slot and the gathered
// cosigners in the REMAINING slots in gather order (ascending slot index),
// builds the homogeneous-fp []MultisigCosigner, and calls md.EncodeMultisig in
// that exact (caller-owned, order-preserving) order.
// `self` carries one entry per slot filled FROM A SEED, each with the origin it
// was derived at; every remaining slot is filled from `cosigners` in gather
// order, ascending. Before S5 this was a single (selfXpub, selfMasterFP) pair at
// p.SelfSlot -- a shape that cannot express Trace B, where the operator holds
// three slots across two masters.
func assembleBuildPolicy(p buildPolicyParams, self []heldSlotKey, cosigners []mk.Card) (out []string, stub [4]byte, slots []md.SlotInfo, err error) {
	if p.N < 1 {
		return nil, [4]byte{}, nil, errBuildSlotCount
	}
	// Defensive bounds: the @S picker is bounded to 0..n-1, but assembleBuildPolicy
	// must never panic on an out-of-range or repeated held slot (fuzz/robustness).
	bySlot := make(map[int]heldSlotKey, len(self))
	for _, h := range self {
		if h.Slot < 0 || h.Slot >= p.N {
			return nil, [4]byte{}, nil, errBuildSlotCount
		}
		if _, dup := bySlot[h.Slot]; dup {
			return nil, [4]byte{}, nil, errBuildSlotCount
		}
		bySlot[h.Slot] = h
	}
	// S4: a `both` self slot is filled from a card like every other slot, so the
	// set the operator chose is N long rather than N-1. S5 generalises that to
	// "every slot no held key fills". Both counts are exact and neither is a floor
	// -- the ruling that forbids assuming n-1 (F-173) is about the PAYLOAD's
	// supply, never about the assembled set.
	if len(cosigners) != p.N-len(self) {
		return nil, [4]byte{}, nil, errBuildSlotCount
	}

	all := make([]md.MultisigCosigner, p.N)
	gi := 0 // gather index into cosigners
	for slot := 0; slot < p.N; slot++ {
		if h, held := bySlot[slot]; held {
			selfCC, selfPK, _, derr := decodeXpubBytes(h.Xpub)
			if derr != nil {
				return nil, [4]byte{}, nil, derr
			}
			c := md.MultisigCosigner{
				ChainCode:        selfCC,
				CompressedPubkey: selfPK,
				Origin:           originComponents(h.Origin),
			}
			if p.IncludeFp {
				c.Fingerprint = fpBytes(h.MasterFP)
				c.FpPresent = true
			}
			all[slot] = c
			continue
		}
		c, cerr := cosignerFromCard(cosigners[gi], p.IncludeFp)
		if cerr != nil {
			return nil, [4]byte{}, nil, cerr
		}
		all[slot] = c
		gi++
	}

	// SPEC §4.1, ON THE ASSEMBLED SET AND NOWHERE ELSE. assembleBuildPolicy is
	// the SOLE md1-bytes producer for the BUILD path (I-VERBATIM), so a check
	// here covers every present and future route into a DEVICE-AUTHORED policy;
	// a check at the card picker would not, because the self key does not exist
	// until step (4) of buildMultisigPolicyFlow and the delivered hazard is
	// self-vs-card.
	//
	// SCOPE, said plainly because "sole md1 producer" reads wider than it is:
	// this does NOT check an md1 the operator SUPPLIES. supplyMultisigPolicyFlow
	// engraves a descriptor someone else authored, repeated keys and all, and
	// that is §4.1's scoping rather than a gap — the device is not the author
	// there, and refusing another coordinator's policy at engrave time would be
	// this device overruling a wallet it did not create. Every md1 the DEVICE
	// mints passes here; not every md1 the device engraves.
	//
	// It also runs BEFORE buildReviewFlow, which is the point: a duplicate must
	// never reach the review screen. With fp-presence Omit (the default) that
	// screen renders every slot "(no fp)", so a wallet one master can spend alone
	// looks exactly like a wallet three masters share.
	if a, b, dup := duplicateSlotPair(all); dup {
		return nil, [4]byte{}, nil, errBuildDuplicateKey{SlotA: a, SlotB: b}
	}

	// S5 REPLACES S2's INTERIM FOREIGN-ORIGIN REFUSAL, which stood exactly here.
	// Until this stage every slot had to declare the one shared origin, because
	// the alternative was not "permit" but "stamp m/48'/0'/0'/2' over a card that
	// says otherwise", which is the device overruling an answer the operator
	// already gave. Per-slot origins make the card's own answer recordable, so the
	// shape is now ACCEPTED. §4.1's duplicate check above is the one that OUTLIVES
	// the stage and it still runs FIRST, which was S2's ruling and is not reversed
	// here: on the delivered payload one build can be both a duplicate and a
	// divergent-origin set, and the duplicate is the graver harm.
	//
	// SHARED WHEN THE ORIGINS AGREE, DIVERGENT WHEN THEY DO NOT. The two are
	// different WIRE FORMS of the same paths, so a policy that switched to
	// divergent unconditionally would re-mint every id the shared form ever
	// produced -- including S2's committed byte-identity golden and every wallet
	// already matched against a coordinator. The comparison is over PARSED
	// COMPONENTS, so it is a comparison of paths and never of notation.
	req := md.EncodeMultisigRequest{
		Cosigners: all,
		K:         uint8(p.K),
		Script:    p.Script,
	}
	if shared, ok := commonOrigin(all); ok {
		req.OriginMode = md.OriginShared
		req.SharedOrigin = shared
	} else {
		req.OriginMode = md.OriginDivergent
	}
	out, stub, slots, err = md.EncodeMultisig(req)
	if err != nil {
		// SPEC M-1, NAMED RATHER THAN GENERIC. A depth-0 card ("m") parses to a
		// ZERO-component origin, which md refuses in either mode
		// (md/encode_multisig.go:96-106). The encoder is left to be the authority
		// on that -- it refuses first, and this only attributes the refusal to the
		// slot whose card caused it, because "Couldn't assemble the wallet policy"
		// describes a device problem and a depth-0 card is the operator's input.
		if slot, ok := emptyOriginSlot(all); ok {
			declared := ""
			gi := 0
			for s := 0; s < p.N; s++ {
				if _, held := bySlot[s]; held {
					continue
				}
				if s == slot {
					declared = cosigners[gi].Path
					break
				}
				gi++
			}
			return nil, [4]byte{}, nil, errBuildEmptyOrigin{Slot: slot, Declared: declared}
		}
		return nil, [4]byte{}, nil, err
	}
	return out, stub, slots, nil
}

// commonOrigin reports the ONE origin every cosigner declares, or ok=false when
// they differ. An empty origin is never a shared origin: md refuses it, and
// naming the slot that carries it is worth more than a mode switch.
func commonOrigin(all []md.MultisigCosigner) ([]md.PathComponent, bool) {
	if len(all) == 0 {
		return nil, false
	}
	first := all[0].Origin
	if len(first) == 0 {
		return nil, false
	}
	for _, c := range all[1:] {
		if len(c.Origin) != len(first) {
			return nil, false
		}
		for i := range first {
			if c.Origin[i] != first[i] {
				return nil, false
			}
		}
	}
	return first, true
}

// emptyOriginSlot reports the first slot declaring a ZERO-component origin,
// which is what a depth-0 ("m") card parses to.
func emptyOriginSlot(all []md.MultisigCosigner) (int, bool) {
	for i, c := range all {
		if len(c.Origin) == 0 {
			return i, true
		}
	}
	return 0, false
}

// buildReviewLines renders the (stub, slots) ordering-verification handle
// (I-ORDER): the 4-byte policy stub, each slot @N -> fingerprint (or "no fp"
// under the homogeneous Omit choice), and the M1 note that the fp-presence
// choice changes the WalletPolicyId — so the operator records/matches the right
// id against their coordinator BEFORE funding.
// `provenance` carries S1's §0.1 announcement (which payload cards filled which
// slots) and is placed FIRST, above the stub: it is the one line on this screen
// whose subject is an assumption the device made rather than a parameter the
// operator picked, and §0.1 clause 3 puts such an announcement on the
// confirmation surface itself, not below the fold. Empty for a set that reached
// here by some other route.
//
// `script` drives §0.1a's ORIGIN announcement, which is the second assumption on
// this screen and the one nobody owned until 2026-08-15. It sits with the
// provenance line, above the stub, for the same clause-3 reason.
//
// `slotKeys` IS THE POLICY'S CONTENTS, and until S5 this screen did not show
// them. It printed "@0 fp xxxxxxxx", or on the DEFAULT path -- fingerprints are
// omitted by default -- "@0 (no fp)", and nothing else. So the last screen before
// an irreversible engrave asked the operator to confirm a wallet policy whose
// keys they had never seen, and the EXPERIMENTAL warning one screen later told
// them to compare it against their coordinator with nothing on the device to
// compare. The keys are printed IN FULL, base58, the way an mk1 card and a
// coordinator both spell them: a truncated or re-encoded form is a comparison
// the operator cannot finish.
//
// THE INSTRUCTION IS PREPENDED, not appended, and that is the census NOTE's
// precedent rather than a preference. confirmReviewScreen is confirmable from
// ANY page (Button3 continues wherever the pager is), so a line added at the
// bottom can be committed past unread; the plate census puts its note first for
// exactly this reason.
func buildReviewLines(script md.MultisigScript, stub [4]byte, slots []md.SlotInfo, slotKeys []string, includeFp bool, provenance []string, held []heldSlotOrigin) []string {
	lines := []string{
		"Check each key below against your coordinator, or against the card it came " +
			"from, before you continue.",
	}
	lines = append(lines, provenance...)
	lines = append(lines, buildOriginAnnouncement(script, held)...)
	lines = append(lines,
		fmt.Sprintf("Policy stub: %x", stub),
		"Slots:",
	)
	for i, s := range slots {
		label := fmt.Sprintf("@%d, no fingerprint", s.Index)
		if s.FpPresent {
			label = fmt.Sprintf("@%d, fingerprint %x", s.Index, s.Fingerprint)
		}
		key := ""
		if i < len(slotKeys) {
			key = slotKeys[i]
		}
		if key == "" {
			// No key to show. The production flow cannot reach this -- it refuses
			// before the review when buildSlotKeyStrings cannot map a slot -- but a
			// caller that passes none gets a slot list, not a slot list that looks
			// like it carried a key and lost it.
			lines = append(lines, label)
			continue
		}
		lines = append(lines, label+":")
		lines = append(lines, chunkString(key, 20)...)
	}
	if includeFp {
		lines = append(lines, "Fingerprints INCLUDED on every slot.")
	} else {
		lines = append(lines, "Fingerprints OMITTED on every slot.")
	}
	lines = append(lines, "Fingerprint choice changes the policy id, so match your coordinator.")
	return lines
}

// buildSlotKeyStrings maps every slot of the ASSEMBLED policy back to the base58
// xpub string the operator holds for it, so the review screen states what the
// bytes carry rather than what the flow intended to put in them.
//
// IT READS THE ASSEMBLED md1, deliberately, rather than re-walking
// assembleBuildPolicy's slot loop. A second copy of "which key fills which slot"
// is how a review screen starts describing a different wallet from the one being
// engraved: the two copies agree until one of them is edited. Here the slot order
// comes from the md1's own expansion and each key is matched by its 65 bytes
// (chain code || compressed pubkey), so the screen cannot disagree with the
// plate unless the plate disagrees with itself.
//
// A SLOT IT CANNOT MAP IS AN ERROR, not a blank line. The whole point of the
// screen is that the operator sees the policy's contents before confirming it; a
// slot rendered empty would be the original defect with extra steps, so the flow
// refuses instead.
func buildSlotKeyStrings(assembled []string, self []heldSlotKey, cosigners []mk.Card) ([]string, []string, error) {
	_, keys, err := md.ExpandWalletPolicyChunks(assembled)
	if err != nil {
		return nil, nil, err
	}
	type candidate struct {
		cc  [32]byte
		pk  [33]byte
		str string
	}
	cands := make([]candidate, 0, len(self)+len(cosigners))
	add := func(x string) {
		cc, pk, _, derr := decodeXpubBytes(x)
		if derr != nil {
			return
		}
		cands = append(cands, candidate{cc: cc, pk: pk, str: x})
	}
	for _, h := range self {
		add(h.Xpub)
	}
	for _, c := range cosigners {
		add(c.Xpub)
	}
	out := make([]string, len(keys))
	// AND THE ORIGINS, OFF THE SAME EXPANSION. §0.1a's announcement has to state
	// the path this build stamped on EVERY slot, and the assembled bytes are the
	// only source that cannot disagree with the plate. Re-deriving them from the
	// slot sources would be a second copy of the tail's origin rule (derived ->
	// derivedSlotOrigin, both -> the card's declared path), and two copies of that
	// rule agree until one of them is edited.
	origins := make([]string, len(keys))
	for i, k := range keys {
		if !k.XpubPresent {
			return nil, nil, fmt.Errorf("multisig build: slot @%d carries no key to show", i)
		}
		origins[i] = k.OriginPath.String()
		found := false
		for _, c := range cands {
			if slices.Equal(c.cc[:], k.Xpub[0:32]) && slices.Equal(c.pk[:], k.Xpub[32:65]) {
				out[i] = c.str
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("multisig build: slot @%d holds a key none of this "+
				"build's inputs accounts for", i)
		}
	}
	return out, origins, nil
}

// buildOriginAnnouncement is §0.1a: the device says WHICH derivation path it
// stamped on every slot, and by WHOSE authority.
//
// The assumption exists because the operator picks a script type and the device
// picks a derivation path from it: BIP-48 assigns 2' to native segwit, 1' to
// nested segwit, and NOTHING to legacy P2SH. That is a default nobody asked for.
//
// S5 CHANGED WHAT IS ANNOUNCED, because S5 changed the default (§0.1a): sh(wsh)
// now derives at 1', the path BIP-48 actually assigns it, so the sentence
// stopped being "this build uses 2' until per-card origins land" and became a
// plain statement of the assignment. Legacy sh is unchanged and still has no
// authority to cite.
//
// It describes the slots THIS DEVICE DERIVES. A payload card's own origin is an
// answer the operator already gave, and §0.1's corollary says a tool that cries
// DEFAULT when the operator chose is a tool whose warnings get ignored.
//
// ANNOUNCE, DO NOT REFUSE, and §0.1 clause 2 is what decides it: the origin is
// printed with the xpub, carried in every mk1 and shown on the restore doc, so a
// wrong assumption is detectable by reading the artifacts. That makes it eligible
// to assume, and having assumed it the device owes the operator the sentence.
//
// Three different sentences, because the three cases are genuinely different and
// a shared one would be false for two of them:
//
//   - wsh, the BIP's own recommended default. Cite BIP-48 and stop.
//   - sh(wsh), where BIP-48 assigns 1' and this build uses 2' until S5's
//     per-slot origins land. Say both paths; this is the one case where the
//     device is knowingly off the assignment it is citing.
//   - sh, where NO BIP assigns anything. There is no authority to cite, so the
//     device owns the choice out loud. Citing BIP-48 here would be a false claim
//     of authority, which is worse than saying nothing.
//
// NO EM-DASHES. Measured 2026-08-15: a body line containing one rasters at
// 2652 px against 7419 for the same text with a hyphen — the line does not draw
// at all, which is F-151's shape and exactly what test 5's raster floor is for.
func buildOriginAnnouncement(script md.MultisigScript, held []heldSlotOrigin) []string {
	// THE ORIGINS ARE THE ONES THIS BUILD ACTUALLY USED (I-7). This function
	// hardcoded derivedSlotOrigin(script, 0), and buildReviewLines prints no
	// per-slot origin, so a build holding @0 and @1 from one master -- which
	// derives at m/48h/0h/0h/2h and m/48h/0h/1h/2h -- put NO correct statement of
	// where @1's key lives on the last confirmation screen before the EXPERIMENTAL
	// warning and the engrave. The very next screen then told the operator to
	// "compare the keys you just reviewed against the same wallet in your
	// coordinator"; one who took the announced origin at face value entered
	// m/48'/0'/0'/2' for both, derived the wrong key for @1, and either blamed the
	// device or mis-registered the wallet at a path the plate does not carry.
	//
	// §0.1a's requirement is that the device says which path it stamped on EVERY
	// slot, so when the held slots diverge they are ENUMERATED. When they share
	// one origin -- the ordinary single-slot build, and a multi-slot build across
	// distinct masters at account 0 -- the sentence is the shipped scalar one,
	// unchanged.
	base, enumerated := heldOriginSummary(script, held)
	if enumerated != "" {
		base = enumerated
	}
	switch script {
	case md.MultisigShWsh:
		return []string{
			fmt.Sprintf("Your key origins: %s.", base),
			fmt.Sprintf("Key origins follow BIP-48 for nested segwit (script type 1h), "+
				"so this build derives your keys at %s. A payload card keeps the origin "+
				"it declares.", base),
		}
	case md.MultisigSh:
		return []string{
			fmt.Sprintf("Your key origins: %s.", base),
			fmt.Sprintf("No BIP assigns a derivation path for legacy P2SH multisig, "+
				"so this is this device's convention, %s. It is recorded on every "+
				"artifact.", base),
		}
	default:
		return []string{
			fmt.Sprintf("Your key origins: %s, the BIP-48 path for native segwit.", base),
		}
	}
}

// heldSlotOrigin is one slot the operator holds, and the origin the ASSEMBLED
// POLICY declares for it.
//
// The origin comes off the assembled bytes rather than from a second walk of the
// slot sources, so the sentence on the review screen and the path on the plate
// cannot come apart.
type heldSlotOrigin struct {
	Slot int
	Path string
}

// heldSlotOrigins pairs the held slot indices with their assembled origins.
// A slot index outside the assembled set is skipped rather than indexed: the
// production flow cannot produce one (assembleBuildPolicy fills every slot), and
// a panic on the last screen before an engrave is a worse outcome than a
// sentence with one fewer slot in it.
func heldSlotOrigins(held []int, slotOrigins []string) []heldSlotOrigin {
	out := make([]heldSlotOrigin, 0, len(held))
	for _, s := range held {
		if s < 0 || s >= len(slotOrigins) || slotOrigins[s] == "" {
			continue
		}
		out = append(out, heldSlotOrigin{Slot: s, Path: slotOrigins[s]})
	}
	return out
}

// heldOriginSummary reduces the held slots to what the announcement has to say.
//
// It returns (scalar, "") when every held slot shares ONE origin -- the shipped
// sentence, unchanged, for the ordinary build -- and ("", enumeration) when they
// diverge. An EMPTY held set falls back to derivedSlotOrigin(script, 0), which is
// the account-0 default this build would use: a caller that supplies no slots is
// a unit test of the wording rather than a flow, and the flow supplies them.
func heldOriginSummary(script md.MultisigScript, held []heldSlotOrigin) (scalar, enumerated string) {
	if len(held) == 0 {
		return derivedSlotOrigin(script, 0).String(), ""
	}
	same := true
	for _, h := range held[1:] {
		if h.Path != held[0].Path {
			same = false
			break
		}
	}
	if same {
		return held[0].Path, ""
	}
	parts := make([]string, len(held))
	for i, h := range held {
		parts[i] = fmt.Sprintf("@%d at %s", h.Slot, h.Path)
	}
	return "", joinAnd(parts)
}

// buildReviewFlow displays the read-only (stub, slots) review and lets the
// operator Continue (Button3 -> true) or Back (Button1 -> false). Reuses the
// paged read-only restore-doc screen idiom.
func buildReviewFlow(ctx *Context, th *Colors, script md.MultisigScript, stub [4]byte, slots []md.SlotInfo, slotKeys []string, includeFp bool, provenance []string, held []heldSlotOrigin) bool {
	lines := buildReviewLines(script, stub, slots, slotKeys, includeFp, provenance, held)
	return confirmReviewScreen(ctx, th, "Policy Review", lines)
}

// confirmReviewScreen is a paged, read-only confirm screen: Button3 -> true
// (continue), Button1 -> false (back), Button2 pages. Mirrors bundleReviewFlow.
func confirmReviewScreen(ctx *Context, th *Colors, title string, lines []string) bool {
	backBtn := &Clickable{Button: Button1}
	contBtn := &Clickable{Button: Button3, AltButton: Center}
	pageBtn := &Clickable{Button: Button2}
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	contentTop := leadingSize + 8
	contentBottom := dims.Y - leadingSize
	start := 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if contBtn.Clicked(ctx) {
			return true
		}
		shown := 0
		y := contentTop
		body := make([]op.Op, 0, len(lines))
		for i := start; i < len(lines); i++ {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, lines[i])
			if i > start && y+sz.Y > contentBottom {
				break
			}
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
			shown++
			if y > contentBottom {
				break
			}
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(lines) {
				start += shown
			} else {
				start = 0
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
		// The pager is drawn ONLY when there is a second page. It used to be
		// unconditional, so a four-line screen -- the payload digest, which is
		// the screen an operator meets first -- showed a right arrow that did
		// nothing when pressed. A control that is present and inert teaches the
		// operator that controls here may be inert, which is expensive on a
		// device whose other buttons cut steel.
		//
		// `shown` is the count this frame's loop actually laid out, so the test
		// is exact rather than a guess at how many lines fit.
		navs := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}
		if start > 0 || shown < len(lines) {
			navs = append(navs, NavButton{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		navs = append(navs, NavButton{Clickable: contBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navs...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}
