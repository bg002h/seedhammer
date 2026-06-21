package gui

import (
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
