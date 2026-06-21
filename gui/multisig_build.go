package gui

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image"

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

	// (2) Gather the n-1 cosigner mk1 cards over NFC (PUBLIC; ms1 refused at
	// classify). Decode each to an mk.Card.
	cards, ok := bundleGatherFlow(ctx, th)
	if !ok {
		return
	}
	cosigners, ok := buildCosignerCards(cards, p.N-1)
	if !ok {
		showError(ctx, th, "Build Policy", fmt.Sprintf("Gather exactly %d cosigner key cards (and no md1).", p.N-1))
		return
	}

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
		if pass, ok := passphraseFlow(ctx, th); ok {
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
		showError(ctx, th, "Build Policy", "Couldn't assemble the wallet policy.")
		return
	}

	// (6) Review the (stub, slots) ordering handle (I-ORDER). Back -> abort.
	if !buildReviewFlow(ctx, th, stub, slots, p.IncludeFp) {
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

var errBuildSlotCount = errors.New("multisig build: cosigner count != n-1")

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
func buildReviewLines(stub [4]byte, slots []md.SlotInfo, includeFp bool) []string {
	lines := []string{
		fmt.Sprintf("Policy stub: %x", stub),
		"Slots:",
	}
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
func buildReviewFlow(ctx *Context, th *Colors, stub [4]byte, slots []md.SlotInfo, includeFp bool) bool {
	lines := buildReviewLines(stub, slots, includeFp)
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
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight},
			{Clickable: contBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}
