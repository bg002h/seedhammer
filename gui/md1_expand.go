package gui

import (
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
)

// expandStatus is the outcome of projecting an expanded md1 wallet-policy onto a
// *bip380.Descriptor.
type expandStatus int

const (
	// expandOK: a *bip380.Descriptor was built and is safe to display + verify.
	expandOK expandStatus = iota
	// expandTemplateOnly: the md1 carries no xpubs (D3) — show the template
	// read-only; no descriptor, no address-verify.
	expandTemplateOnly
	// expandUnsupported: a faithfully-expressible bip380 descriptor cannot be
	// built (unsorted multi / multi_a / sortedmulti_a / taptree / complex, D2;
	// or a hardened wildcard / hardened multipath alt / exotic range, D5/R0-I2).
	// Display-only; NEVER build a descriptor or verify an address.
	expandUnsupported
)

// expandedToDescriptor projects a decoded md1 Template + its per-@N expansion
// onto a *bip380.Descriptor, applying the faithful-or-refuse policy (D2) and
// the locked decisions D1 (mainnet-only), D3 (no-pubkeys → template-only),
// D5/R0-I2 (hardened/exotic use-site → unsupported), R0-C2 (sh nesting
// discriminant). On expandTemplateOnly / expandUnsupported it returns a nil
// descriptor — the caller must NOT verify an address.
func expandedToDescriptor(tpl md.Template, keys []md.ExpandedKey) (*bip380.Descriptor, expandStatus) {
	// Map the renderable shape → (Script, MultisigType). A non-expressible
	// shape (unsorted multi, multi_a, sortedmulti_a, taptree, complex) refuses
	// here, BEFORE any xpub work (D2).
	script, msType, ok := scriptForTemplate(tpl)
	if !ok {
		return nil, expandUnsupported
	}

	// D3: no xpubs to expand → template-only (no descriptor / no verify).
	if len(keys) == 0 {
		return nil, expandTemplateOnly
	}
	for _, k := range keys {
		if !k.XpubPresent {
			return nil, expandTemplateOnly
		}
	}

	bkeys := make([]bip380.Key, 0, len(keys))
	for _, k := range keys {
		children, cok := useSiteToChildren(k.UseSite)
		if !cok {
			// Hardened wildcard / hardened multipath alt / exotic range (D5,
			// R0-I2): reject early so it fails display-only, never late at verify
			// against a wrong address.
			return nil, expandUnsupported
		}
		bkeys = append(bkeys, bip380.Key{
			Network:           &chaincfg.MainNetParams, // D1: mainnet-only.
			MasterFingerprint: fpFromBytes(k.Fingerprint),
			DerivationPath:    k.OriginPath, // bip32.Path, in-band hardening (R0-I2).
			Children:          children,
			KeyData:           append([]byte(nil), k.Xpub[32:65]...), // compressed pubkey.
			ChainCode:         append([]byte(nil), k.Xpub[0:32]...),  // 32-byte chain code.
			ParentFingerprint: 0,
		})
	}

	desc := &bip380.Descriptor{
		Script:    script,
		Type:      msType,
		Threshold: tpl.K, // 0 for singlesig (unused by address derivation).
		Keys:      bkeys,
	}
	return desc, expandOK
}

// scriptForTemplate maps the renderable Template shape to a bip380 Script +
// MultisigType, or reports !ok for a non-bip380-expressible shape (D2, R0-C2).
func scriptForTemplate(tpl md.Template) (bip380.Script, bip380.MultisigType, bool) {
	if !tpl.Renderable {
		return 0, 0, false
	}
	switch tpl.Policy {
	case md.PolicySingle:
		switch tpl.Root {
		case md.ScriptWpkh:
			return bip380.P2WPKH, bip380.Singlesig, true
		case md.ScriptPkh:
			return bip380.P2PKH, bip380.Singlesig, true
		case md.ScriptTr:
			return bip380.P2TR, bip380.Singlesig, true
		case md.ScriptSh:
			// sh(wpkh) — BIP-49 P2SH-P2WPKH single-sig. Keyed on the InnerWpkh
			// discriminant, symmetric with the InnerWsh sorted-multi sh arm below.
			// Disjoint from PolicySortedMulti, so it can never collide with the
			// P2SH_P2WSH / bare-P2SH multisig arms.
			if tpl.InnerWpkh {
				return bip380.P2SH_P2WPKH, bip380.Singlesig, true
			}
		}
	case md.PolicySortedMulti:
		switch tpl.Root {
		case md.ScriptWsh:
			return bip380.P2WSH, bip380.SortedMulti, true
		case md.ScriptSh:
			// R0-C2: the nesting discriminant decides. sh(wsh(sortedmulti)) is a
			// nested-segwit P2SH_P2WSH; a bare sh(sortedmulti) is a legacy P2SH.
			// They hash to DIFFERENT addresses — never collapse to one.
			if tpl.InnerWsh {
				return bip380.P2SH_P2WSH, bip380.SortedMulti, true
			}
			return bip380.P2SH, bip380.SortedMulti, true
		}
	}
	// Unsorted multi / multi_a / sortedmulti_a / taptree / any other shape:
	// not bip380-expressible (D2) — display-only, never verified.
	return 0, 0, false
}

// useSiteToChildren maps a resolved use-site to a []bip380.Derivation
// (mirroring to_miniscript.rs:116-131): <a;b>/* → [RangeDerivation{a,b},
// WildcardDerivation]; bare * → [WildcardDerivation]. Reports !ok for a
// hardened wildcard, a hardened multipath alt (D5), or a multipath whose range
// is not exactly <a;b> with b==a+1 (R0-I2 — address.derivePubKey supports only
// End==Index+1, address/address.go:196-198).
func useSiteToChildren(us md.UseSite) ([]bip380.Derivation, bool) {
	if us.WildcardHardened {
		return nil, false // D5: hardened trailing wildcard.
	}
	if !us.HasMultipath {
		// Bare * — a single trailing wildcard. address.derivePubKey defaults to
		// <0;1>/* only when Children is EMPTY, so be explicit here.
		return []bip380.Derivation{{Type: bip380.WildcardDerivation}}, true
	}
	// A standard <a;b>/* multipath: exactly two alts, neither hardened, b==a+1.
	if len(us.Multipath) != 2 {
		return nil, false // exotic alt-count (≠2) — not address-supported.
	}
	if us.Multipath[0].Hardened || us.Multipath[1].Hardened {
		return nil, false // D5: hardened multipath alt.
	}
	idx := us.Multipath[0].Value
	end := us.Multipath[1].Value
	if end != idx+1 {
		return nil, false // R0-I2: address.derivePubKey only supports End==Index+1.
	}
	return []bip380.Derivation{
		{Type: bip380.RangeDerivation, Index: idx, End: end},
		{Type: bip380.WildcardDerivation},
	}, true
}

// fpFromBytes packs a 4-byte big-endian master fingerprint into the uint32 that
// bip380.Key.MasterFingerprint expects.
func fpFromBytes(fp [4]byte) uint32 {
	return uint32(fp[0])<<24 | uint32(fp[1])<<16 | uint32(fp[2])<<8 | uint32(fp[3])
}
