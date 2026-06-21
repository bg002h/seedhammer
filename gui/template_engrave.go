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
func templateConsentLines(tmpl md.Template, templateID [4]byte, tapDepth int) []string {
	var lines []string
	if tmpl.Renderable && tmpl.Policy != md.PolicyComplex {
		lines = append(lines,
			"TEMPLATE-ONLY md1 (advanced)",
			fmt.Sprintf("Policy: %s", policyTypeLabel(tmpl)),
			fmt.Sprintf("Key slots: %d", tmpl.N),
			fmt.Sprintf("Template-ID: %x", templateID),
		)
	} else {
		// C3 honest-minimal: no k-of-N computable on-device.
		lines = append(lines,
			"COMPLEX POLICY (advanced)",
			"Cannot fully display on-device.",
			fmt.Sprintf("Script: %s", complexScriptFamily(tmpl, tapDepth)),
			fmt.Sprintf("Key slots: %d", tmpl.N),
			fmt.Sprintf("Template-ID: %x", templateID),
			"VERIFY against your coordinator /",
			"toolkit BEFORE funding.",
		)
	}
	// The shared loud warning + estimate.
	lines = append(lines, templateWarningLines()...)
	// The depth-≥2 taproot EXPERIMENTAL gate (S5).
	if tapDepth >= 2 {
		lines = append(lines,
			"EXPERIMENTAL: taproot depth >= 2",
			"The shipped toolkit CANNOT reconstruct",
			"this taptree (rust-miniscript PR #953).",
			"Recovery needs an UNRELEASED",
			"rust-miniscript >13.1.0.",
			"DO NOT use for real funds until that ships.",
		)
	}
	return lines
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
