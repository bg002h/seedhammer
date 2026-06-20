package gui

import (
	"fmt"

	"seedhammer.com/bip39"
	"seedhammer.com/md"
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
	// Implemented across Tasks 2–5. Task 1 only wires the front-door route; the
	// first user-facing screen is the template picker (Task 2).
	_, ok := multisigTemplatePick(ctx, th)
	if !ok {
		return
	}
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

// buildParamPickFlow runs the bounded pickers in order: template -> n -> k(n) ->
// self-slot @S -> fp-presence. Back from any picker re-shows the previous one
// (or returns ok==false from the first). Every returned param is in-range by
// construction (no free-form widget exists).
func buildParamPickFlow(ctx *Context, th *Colors) (buildPolicyParams, bool) {
	var p buildPolicyParams
	// Stage 1: template.
	script, ok := multisigTemplatePick(ctx, th)
	if !ok {
		return p, false
	}
	p.Script = script
	for {
		// Stage 2: n.
		nCS := &ChoiceScreen{Title: "Cosigners", Lead: "How many keys (n)?", Choices: multisigNChoices()}
		nIdx, ok := nCS.Choose(ctx, th)
		if !ok {
			return p, false // Back from n -> abandon (template already chosen; simplest).
		}
		p.N = multisigNFor(nIdx)
		// Stage 3: k (dependent on n).
		kCS := &ChoiceScreen{Title: "Threshold", Lead: fmt.Sprintf("Required signatures (k of %d)?", p.N), Choices: multisigKChoices(p.N)}
		kIdx, ok := kCS.Choose(ctx, th)
		if !ok {
			continue // Back from k -> re-pick n.
		}
		p.K = multisigKFor(kIdx)
		// Stage 4: self-slot @S.
		sCS := &ChoiceScreen{Title: "Your slot", Lead: "Which slot is your key?", Choices: multisigSelfSlotChoices(p.N)}
		sIdx, ok := sCS.Choose(ctx, th)
		if !ok {
			continue // Back from @S -> re-pick n (and k).
		}
		p.SelfSlot = sIdx
		// Stage 5: fp-presence.
		fpCS := &ChoiceScreen{Title: "Fingerprints", Lead: "Include key fingerprints?", Choices: multisigFpChoices()}
		fpIdx, ok := fpCS.Choose(ctx, th)
		if !ok {
			continue // Back from fp -> re-pick n.
		}
		p.IncludeFp = multisigIncludeFpFor(fpIdx)
		return p, true
	}
}
