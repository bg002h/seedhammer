package gui

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image"
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

	// (2) THE PAYLOAD SUPPLIES THE WHOLE COSIGNER SET (S1). §3.3.2 admits
	// ClassMDMK to this program; every such record is fed to the gather through
	// the SAME offer() a scanned card takes, so all of them get identical dedup,
	// chunk assembly and integrity gating.
	//
	// The count of cards on the payload is INDEPENDENT of n (F-173, `0..n`):
	// over-supply is normal and is resolved by selection, under-supply is a named
	// refusal, and the exact-count check stays where it belongs — on the
	// ASSEMBLED set, in buildCosignerCards, below.
	open := p.N - 1
	records, state := buildCosignerSource(ctx)
	supply, incomplete := buildCosignerSupply(records)
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
	mk1s := mk1CosignerCards(cards)
	// Re-classified over what the GATHER produced, not over what the payload
	// held: on reader-equipped hardware the operator may have added more.
	//
	// THE SWITCH IS EXHAUSTIVE ON PURPOSE (fold, N1). `classifyCosignerSupply`
	// is total, but leaving auto-fill as this switch's implicit default made the
	// CALL SITE non-total: a fourth outcome, or any future disagreement between
	// the two classify calls, would have taken the all-cards branch in silence.
	// The ruling that forbids assuming `n-1` deserves a case analysis that says
	// so at both ends.
	var chosen []int
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
	cosigners, ok := buildCosignerCards(picked, open)
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
	origins := buildCosignerOrigins(p.N, p.SelfSlot, chosen)

	// (3) TYPED-ONLY self seed (I-SCRUB). Scrub on EVERY exit.
	mnemonic, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	if buildMultisigSeedHook != nil {
		buildMultisigSeedHook(mnemonic)
	}
	defer func() {
		for i := range mnemonic {
			mnemonic[i] = 0
		}
	}()
	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		// §3.3.2 admits ClassPassphrase to this program, so the payload is
		// offered before the keyboard (plan stage 13b). NOT passphraseFlow: see
		// syswPassphraseFlow for the two normative rules a shared edit inside
		// passphraseFlow would have broken.
		if pass, ok := syswPassphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	// (4) Derive the self key at the LOCKED shared origin (self-origin ==
	// policy-origin by construction). deriveAccountXpub neuters (no xprv) +
	// scrubs the seed/master internally.
	selfXpub, selfMasterFP, err := deriveAccountXpub(mnemonic, passphrase, &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't derive your key from the seed.")
		return
	}

	// (5) Assemble via the SOLE md1 producer md.EncodeMultisig.
	assembledMd1, stub, slots, err := assembleBuildPolicy(p, selfXpub, selfMasterFP, cosigners)
	if err != nil {
		// SPEC §4.1's refusal is NAMED, not folded into the generic failure. The
		// generic text ("Couldn't assemble the wallet policy.") describes a device
		// problem; a duplicate key is the operator's INPUT and there are two
		// things they can do about it, so both are said.
		var dup errBuildDuplicateKey
		if errors.As(err, &dup) {
			showError(ctx, th, "Duplicate key",
				buildDuplicateKeyMessage(dup, p.SelfSlot, origins))
			return
		}
		showError(ctx, th, "Build Policy", "Couldn't assemble the wallet policy.")
		return
	}

	// (6) Review the (stub, slots) ordering handle (I-ORDER), carrying S1's §0.1
	// announcement of which payload cards filled which slots. Back -> abort.
	if !buildReviewFlow(ctx, th, stub, slots, p.IncludeFp,
		buildProvenanceLines(origins, len(mk1s))) {
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
	modeChoice := &ChoiceScreen{Title: "Engrave Mode", Lead: "What to engrave?", Choices: []string{"Full (seed + keys)", "Watch-only (keys)"}}
	modeSel, ok := modeChoice.Choose(ctx, th)
	if !ok {
		return
	}
	full := modeSel == 0

	// (9) Derive the operator's leg over the engrave md1 (full policy OR the
	// stripped template; flows EXACTLY like a supplied md1; deriveMultisigLeg
	// binds mk1.Stubs form-aware — WalletPolicyId for full, WDT-Id for template,
	// C2) and engrave.
	b, err := deriveMultisigLeg(mnemonic, passphrase, &chaincfg.MainNetParams, multisigSharedOrigin(), engraveMd1, full)
	if err != nil {
		showError(ctx, th, "Build Policy", "Couldn't derive the bundle from the seed.")
		return
	}
	cardsOut := multisigEngraveCards(b.MS1, b.MK1, b.MD1, full)
	bundleEngrave(ctx, th, cardsOut)

	// (10) Offer verify-bundle — full policy only. The verify re-derives via the
	// xpub seed-cross-match (findUserSlot), which a KEYLESS template has no xpub
	// to match (D1); the template's binding is the device's own form-aware
	// readback, already established at engrave. So a template engrave skips the
	// cross-match verify offer.
	if !template {
		verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: "Verify the engraved plates?", Choices: []string{"Verify now", "Skip"}}
		if sel, ok := verifyChoice.Choose(ctx, th); ok && sel == 0 {
			multisigVerifyFlow(ctx, th, b, full)
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
		multisigRestoreDocFlow(ctx, th, tpl, keys)
	}
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
func multisigBuildExperimentalWarning(ctx *Context, th *Colors) bool {
	warn := &ConfirmWarningScreen{
		Title: "EXPERIMENTAL",
		Body: "This device-authored multisig policy is NOT validated end-to-end — there is no " +
			"coordinator or hardware round-trip. You MUST verify the assembled descriptor and the " +
			"shown policy stub + per-slot fingerprints against your coordinator/wallet BEFORE funding. " +
			"The fingerprint choice changes the policy id.\n\nHold button to confirm.",
		Icon: assets.IconHammer,
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

// The fp-presence picker (HOMOGENEOUS): Omit (index 0, default) -> no fp TLVs on
// any slot; Include (index 1) -> every slot's master fp.
func multisigFpChoices() []string       { return []string{"No (omit)", "Yes (include)"} }
func multisigIncludeFpFor(idx int) bool { return idx == 1 }

// buildPolicyParams is the assembled shape the operator picked.
type buildPolicyParams struct {
	Script    md.MultisigScript
	N         int
	K         int
	SelfSlot  int  // 0..N-1
	IncludeFp bool // homogeneous fp-presence
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
			sCS := &ChoiceScreen{Title: "Your slot", Lead: "Which slot is your key?", Choices: multisigSelfSlotChoices(p.N)}
			sIdx, ok := sCS.Choose(ctx, th)
			if !ok {
				stage = stageK // Back -> re-pick k.
				continue
			}
			p.SelfSlot = sIdx
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

var errBuildSlotCount = errors.New("multisig build: cosigner count != n-1")

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

// buildSlotProvenance names one slot for a refusal, using what the flow already
// knows: the self-slot the operator picked and the card→slot map
// buildCosignerOrigins built. A refusal that says "slots @0 and @1" and stops
// leaves the operator to work out which of their inputs to change.
func buildSlotProvenance(slot, selfSlot int, origins []cosignerOrigin) string {
	if slot == selfSlot {
		return fmt.Sprintf("slot @%d (your key, from your seed)", slot)
	}
	for _, o := range origins {
		if o.slot == slot {
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
func buildDuplicateKeyMessage(e errBuildDuplicateKey, selfSlot int, origins []cosignerOrigin) string {
	a := buildSlotProvenance(e.SlotA, selfSlot, origins)
	b := buildSlotProvenance(e.SlotB, selfSlot, origins)
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
// the policy stays homogeneous); when false no fp is set. The card's Origin is
// IGNORED (OriginShared mode declares the single shared origin).
func cosignerFromCard(card mk.Card, includeFp bool) (md.MultisigCosigner, error) {
	cc, pk, _, err := decodeXpubBytes(card.Xpub)
	if err != nil {
		return md.MultisigCosigner{}, err
	}
	c := md.MultisigCosigner{ChainCode: cc, CompressedPubkey: pk}
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
// path (I-VERBATIM). It places the self-derived key at p.SelfSlot and the
// gathered cosigners in the REMAINING slots in gather order (ascending slot
// index, skipping SelfSlot), builds the homogeneous-fp []MultisigCosigner, and
// calls md.EncodeMultisig in that exact (caller-owned, order-preserving) order.
func assembleBuildPolicy(p buildPolicyParams, selfXpub string, selfMasterFP uint32, cosigners []mk.Card) (out []string, stub [4]byte, slots []md.SlotInfo, err error) {
	// Defensive bounds: the @S picker is bounded to 0..n-1, but assembleBuildPolicy
	// must never panic on an out-of-range self-slot (fuzz/robustness).
	if p.N < 1 || p.SelfSlot < 0 || p.SelfSlot >= p.N {
		return nil, [4]byte{}, nil, errBuildSlotCount
	}
	if len(cosigners) != p.N-1 {
		return nil, [4]byte{}, nil, errBuildSlotCount
	}
	selfCC, selfPK, _, err := decodeXpubBytes(selfXpub)
	if err != nil {
		return nil, [4]byte{}, nil, err
	}
	self := md.MultisigCosigner{ChainCode: selfCC, CompressedPubkey: selfPK}
	if p.IncludeFp {
		self.Fingerprint = fpBytes(selfMasterFP)
		self.FpPresent = true
	}

	all := make([]md.MultisigCosigner, p.N)
	all[p.SelfSlot] = self
	gi := 0 // gather index into cosigners
	for slot := 0; slot < p.N; slot++ {
		if slot == p.SelfSlot {
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
	// the SOLE md1-bytes producer for this path (I-VERBATIM), so a check here
	// covers every present and future route into a policy; a check at the card
	// picker would not, because the self key does not exist until step (4) of
	// buildMultisigPolicyFlow and the delivered hazard is self-vs-card.
	//
	// It also runs BEFORE buildReviewFlow, which is the point: a duplicate must
	// never reach the review screen. With fp-presence Omit (the default) that
	// screen renders every slot "(no fp)", so a wallet one master can spend alone
	// looks exactly like a wallet three masters share.
	if a, b, dup := duplicateSlotPair(all); dup {
		return nil, [4]byte{}, nil, errBuildDuplicateKey{SlotA: a, SlotB: b}
	}

	req := md.EncodeMultisigRequest{
		Cosigners:    all,
		K:            uint8(p.K),
		Script:       p.Script,
		OriginMode:   md.OriginShared,
		SharedOrigin: originComponents(multisigSharedOrigin()),
	}
	return md.EncodeMultisig(req)
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
func buildReviewLines(stub [4]byte, slots []md.SlotInfo, includeFp bool, provenance []string) []string {
	lines := append([]string{}, provenance...)
	lines = append(lines,
		fmt.Sprintf("Policy stub: %x", stub),
		"Slots:",
	)
	for _, s := range slots {
		if s.FpPresent {
			lines = append(lines, fmt.Sprintf("@%d  fp %x", s.Index, s.Fingerprint))
		} else {
			lines = append(lines, fmt.Sprintf("@%d  (no fp)", s.Index))
		}
	}
	if includeFp {
		lines = append(lines, "Fingerprints INCLUDED on every slot.")
	} else {
		lines = append(lines, "Fingerprints OMITTED on every slot.")
	}
	lines = append(lines, "Fingerprint choice changes the policy id — match your coordinator.")
	return lines
}

// buildReviewFlow displays the read-only (stub, slots) review and lets the
// operator Continue (Button3 -> true) or Back (Button1 -> false). Reuses the
// paged read-only restore-doc screen idiom.
func buildReviewFlow(ctx *Context, th *Colors, stub [4]byte, slots []md.SlotInfo, includeFp bool, provenance []string) bool {
	lines := buildReviewLines(stub, slots, includeFp, provenance)
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
