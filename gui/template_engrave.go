package gui

import (
	"fmt"

	"seedhammer.com/bundle"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── Template-engrave (opt-in): strip a device-BUILT bundle to keyless ───────
//
// The engrave flow defaults to the full-policy md1 (DD5). When the user opts
// into "Template-only" the device-built bundle is transformed to a keyless
// template: the md1 is stripped (md.StripToTemplate), and every mk1 card's
// policy_id_stub is re-minted on the template's WalletDescriptorTemplateId
// (form-aware, C2) so the engraved template bundle binds and passes the device's
// own readback verify. The ms1 secret leg is untouched.

// templateizeBundle converts a device-built FULL bundle into its keyless
// TEMPLATE form: strip the md1 + re-mint the single mk1 stub on the template id.
// (Single-sig: exactly one mk1 card. The N-cosigner multisig form is handled by
// templateizeMultisigBundle in Task 7.)
func templateizeBundle(b bundle.Bundle) (bundle.Bundle, error) {
	tmplMD1, err := md.StripToTemplate(b.MD1)
	if err != nil {
		return bundle.Bundle{}, err
	}
	stub, err := md.FormAwareStubChunks(tmplMD1)
	if err != nil {
		return bundle.Bundle{}, err
	}
	mk1, err := reStubMk1(b.MK1, stub)
	if err != nil {
		return bundle.Bundle{}, err
	}
	return bundle.Bundle{MS1: b.MS1, MK1: mk1, MD1: tmplMD1}, nil
}

// reStubMk1 re-encodes an mk1 card carrying the SAME xpub/path/fingerprint but a
// new (template-id) policy_id_stub. The xpub-bearing card is otherwise verbatim.
func reStubMk1(mk1 []string, stub [4]byte) ([]string, error) {
	card, err := mk.Decode(mk1)
	if err != nil {
		return nil, err
	}
	card.Stubs = [][4]byte{stub}
	return mk.Encode(card)
}

// templateConsentLines builds the per-shape consent surface shown before a
// multisig/general template engrave (S4/S5/C3/DD7):
//   - a CLASSIFIABLE shape (single/multi/sortedmulti) → full type + k-of-N + the
//     N-slot count + template-id, then the loud warning.
//   - a PolicyComplex shape (general miniscript / multi-leaf taptree) → the
//     HONEST-MINIMAL consent {script family, key-slot count N, template-id}: the
//     device cannot break it down; verify against the off-device toolkit (C3).
//   - a depth-≥2 taptree → additionally the EXPERIMENTAL warning naming the
//     unreleased rust-miniscript >13.1.0 / PR #953 (S5).
//
// templateID is the 4-byte WDT-Id stub of the (stripped) template; tapDepth is
// md.TapTreeDepthChunks of the template.
func templateConsentLines(tmpl md.Template, templateID [4]byte, tapDepth int, shape md.PolicyShape) []string {
	var lines []string
	if tmpl.Renderable && tmpl.Policy != md.PolicyComplex {
		lines = append(lines,
			"TEMPLATE-ONLY md1 (advanced)",
			fmt.Sprintf("Policy: %s", policyTypeLabel(tmpl)),
			fmt.Sprintf("Key slots: %d", tmpl.N),
			fmt.Sprintf("Template-ID: %x", templateID),
		)
	} else {
		// C3 honest-minimal, PLUS a structural summary when one can be given
		// completely (Stage 2).
		lines = append(lines,
			"COMPLEX POLICY (advanced)",
			fmt.Sprintf("Script: %s", complexScriptFamily(tmpl, tapDepth)),
			fmt.Sprintf("Key slots: %d", tmpl.N),
			fmt.Sprintf("Template-ID: %x", templateID),
		)
		// THE CONTRACT: shape.Complete is the ONLY thing that admits a summary.
		// A partial one is worse than none — SPEC §4.2/C3's objection is exactly
		// that describing some spend paths and not others leaves the operator
		// believing they have seen the policy. So an incomplete walk keeps the
		// original "cannot display" wording and adds nothing.
		if summary := policySummaryLines(shape); len(summary) > 0 {
			lines = append(lines, summary...)
		} else {
			lines = append(lines, "Cannot fully display on-device.")
		}
		lines = append(lines,
			"VERIFY against your coordinator /",
			"toolkit BEFORE funding.",
		)
	}
	// The shared loud warning + estimate.
	lines = append(lines, templateWarningLines()...)
	// The depth-≥2 taproot gate (S5). REWORDED 2026-08-20: the blocker it named
	// is FIXED, and a warning that names a fixed blocker is worse than none --
	// it teaches the operator to discount the next one.
	//
	// What it used to say, and why each line had to go:
	//
	//   "The shipped toolkit CANNOT reconstruct this taptree"
	//       False now. mnemonic-toolkit 0.97.0 restores depth->=2 taproot; the
	//       refusal gate was lifted with the miniscript pin.
	//   "Recovery needs an UNRELEASED rust-miniscript >13.1.0"
	//       Misleading. PR #953 is merged but 13.1.0 was cut from a maintenance
	//       line, so it is NEWER than the merge and will never contain it.
	//       Waiting for a release was never a plan with a date.
	//   "DO NOT use for real funds until that ships"
	//       Conditioned on an event that will not happen.
	//
	// What remains TRUE, and is what an operator actually needs: recovery
	// requires a build carrying #953. An older md or toolkit still cannot
	// reconstruct this taptree, so the caveat becomes a MINIMUM VERSION rather
	// than disappearing -- an operator's backup outlives the build that made it.
	if tapDepth >= 2 {
		lines = append(lines,
			"Taproot depth >= 2",
			"Recovery needs md 0.13+ / toolkit 0.97+",
			"(rust-miniscript PR #953). OLDER builds",
			"CANNOT reconstruct this taptree.",
			"PROVE RECOVERY BEFORE FUNDING.",
		)
	}
	return lines
}

// policySummaryLines renders the STRUCTURAL summary, or nothing at all.
//
// It returns empty for an incomplete walk, and the caller then shows the
// honest-minimal screen. Every line here is a structural fact read off the
// decoded tree — no fragment is named, because naming one is a rendering, and a
// rendering this device cannot re-parse is the thing the cycle's invariant
// forbids.
//
// THE KEY-PATH LINE COMES FIRST AND IS NEVER OMITTED. A spendable taproot
// internal key can move the funds without satisfying ANY leaf, so a summary that
// listed leaves and stayed quiet about it would describe the least likely spend
// path and hide the most direct one.
func policySummaryLines(shape md.PolicyShape) []string {
	if !shape.Complete {
		return nil
	}
	if shape.KeyPath == md.KeyPathNone && len(shape.Branches) == 0 {
		return nil
	}
	var out []string
	switch shape.KeyPath {
	case md.KeyPathSpendable:
		out = append(out, "Key-path: A KEY CAN SPEND ALONE")
	case md.KeyPathNUMS:
		out = append(out, "Key-path: none (script paths only)")
	}
	if n := len(shape.Branches); n > 0 {
		word := "paths"
		if n == 1 {
			word = "path"
		}
		if shape.TapDepth > 0 {
			out = append(out, fmt.Sprintf("Spend %s: %d (tree depth %d)", word, n, shape.TapDepth))
		} else {
			out = append(out, fmt.Sprintf("Spend %s: %d", word, n))
		}
	}
	for i, b := range shape.Branches {
		var desc string
		if b.N > 0 {
			desc = fmt.Sprintf("%d-of-%d", b.K, b.N)
		} else {
			// NOT a plain threshold. Say how many keys it involves rather than
			// inventing a k-of-N that would misdescribe the conditions.
			desc = fmt.Sprintf("%d key(s), custom", b.Keys)
		}
		if b.Timelock {
			desc += " +timelock"
		}
		if b.Hashlock {
			desc += " +hashlock"
		}
		out = append(out, fmt.Sprintf("  %d: %s", i+1, desc))
	}
	return out
}

// policyTypeLabel renders a short k-of-N label for a classifiable multisig/
// single-sig template.
func policyTypeLabel(tmpl md.Template) string {
	switch tmpl.Policy {
	case md.PolicySingle:
		return "single-sig"
	case md.PolicyMulti:
		return fmt.Sprintf("multi %d-of-%d", tmpl.K, tmpl.M)
	case md.PolicySortedMulti:
		return fmt.Sprintf("sortedmulti %d-of-%d", tmpl.K, tmpl.M)
	default:
		return "wallet policy"
	}
}

// complexScriptFamily names the script family for a non-classifiable shape
// (honest-minimal — no breakdown).
func complexScriptFamily(tmpl md.Template, tapDepth int) string {
	if tapDepth >= 1 {
		return fmt.Sprintf("tr + script tree (depth %d)", tapDepth)
	}
	return "general miniscript"
}

// templateWarningLines are the loud opt-in warning + recovery-time estimate
// shown before a template engrave (S4 mockup + the S6 estimate). They are
// load-bearing consent strings (asserted by the flow tests).
func templateWarningLines() []string {
	return []string{
		"TEMPLATE-ONLY md1 (advanced)",
		"Omits keys: ~1 plate (vs ~2-3).",
		"The md1 ALONE cannot rebuild your wallet:",
		"you ALSO need the cosigner key cards (mk1),",
		"and recovery may need an off-device key search.",
		"Recovery search (off-device, toolkit):",
		"  sortedmulti (usual): NONE (order-invariant)",
		"  ordered multi / N!:  N=5 ~0.8ms",
		"  N=9 ~2.5s   N=12 ~55min  (1 thread)",
		"github.com/bg002h/mnemonic-toolkit",
	}
}
