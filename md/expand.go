package md

import (
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
)

// ─── Exported chunked-decode + wallet-policy expansion (#10b) ────────────────
//
// DecodeChunks is the chunked analog of Decode: it reassembles + integrity-
// checks a multi-chunk md1 set (via Reassemble) and summarizes it into a
// Template. ExpandWalletPolicy surfaces the per-@N data the GUI projection needs
// (the Go analog of Rust canonicalize.rs:420-474 expand_per_at_n): structured
// origin path, use-site, fingerprint, and the 65-byte xpub bytes. Both are
// exported because the underlying *descriptor is unexported (Reassemble returns
// it but package gui cannot name it).

// DecodeChunks decodes a multi-chunk md1 descriptor set into a Template. It runs
// the full Reassemble path (per-chunk BCH verify → header consistency →
// completeness → concat → decode+validate → csid integrity gate), then
// summarizes. The resulting Template equals Decode of the equivalent single
// string. Reassemble errors surface unchanged; ErrChunkSetIDMismatch and
// ErrChunkSetIncomplete are exported so callers can errors.Is-dispatch distinct
// UX (R0-C1).
func DecodeChunks(strs []string) (Template, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return Template{}, err
	}
	return summarize(d), nil
}

// UseSiteAlt is one alternative of a use-site multipath (e.g. the `0` and `1` of
// `<0;1>/*`). Value is the raw child index (no hardening offset); Hardened is
// the per-alt hardening flag.
type UseSiteAlt struct {
	Hardened bool
	Value    uint32
}

// UseSite is the resolved per-@N use-site (suffix) path. HasMultipath
// distinguishes a `<a;b>/*` multipath (HasMultipath=true) from a bare `*`
// (HasMultipath=false). WildcardHardened marks a hardened trailing wildcard
// (`*'`), which is unsupported for public derivation (D5).
type UseSite struct {
	HasMultipath     bool
	Multipath        []UseSiteAlt
	WildcardHardened bool
}

// ExpandedKey is the per-@N expansion record — one per placeholder index in
// 0..n (the Go analog of Rust ExpandedKey, canonicalize.rs:337-350). OriginPath
// is a bip32.Path ([]uint32) with hardening encoded IN-BAND as
// value + hdkeychain.HardenedKeyStart (R0-I2; no parallel []bool). Fingerprint /
// Xpub are valid only when their *Present flag is set.
type ExpandedKey struct {
	Index              uint8
	OriginPath         bip32.Path
	UseSite            UseSite
	Fingerprint        [4]byte
	FingerprintPresent bool
	Xpub               [65]byte // 32B chain code ‖ 33B compressed pubkey
	XpubPresent        bool
}

// ExpandWalletPolicy expands a decoded descriptor into one ExpandedKey per @N in
// 0..n. It is the GUI-facing accessor; callers that already hold a decoded
// result use this form (no second decode). Mirrors Rust expand_per_at_n
// (canonicalize.rs:420-474) with the R0-I1 origin precedence:
//
//	origin = OriginPathOverrides[idx]              (if present)
//	       | path_decl (Divergent[idx] / Shared)   (if non-empty)
//	       | canonicalOrigin(tree)                  (R0-I1 fallback)
//
// The canonicalOrigin fallback is the deliberate Go divergence from Rust
// (which relies on encode-time canonical-fill of path_decl): the Go decoder
// leaves an elided shared path empty, so without this fallback the displayed
// key-origin would serialize depth/childnum 0 — a wrong displayed xpub
// (md.go:1033,1089). Derivation itself ignores depth/childnum, so address
// verification is unaffected either way, but the displayed origin must be right.
//
// Use-site precedence is override > descriptor baseline (canonicalize.rs:457-460).
func ExpandWalletPolicy(d *descriptor) ([]ExpandedKey, error) {
	out := make([]ExpandedKey, 0, d.n)
	for idx := uint8(0); idx < d.n; idx++ {
		out = append(out, ExpandedKey{
			Index:              idx,
			OriginPath:         resolveOriginPath(d, idx),
			UseSite:            resolveUseSite(d, idx),
			Fingerprint:        fingerprintBytesFor(d, idx),
			FingerprintPresent: hasFingerprint(d, idx),
			Xpub:               xpubBytesFor(d, idx),
			XpubPresent:        hasXpub(d, idx),
		})
	}
	return out, nil
}

// ExpandWalletPolicyChunks reassembles a chunk set then expands it (the
// []string-input form). Returns the Template (so the caller need not decode
// twice) alongside the per-@N expansion.
func ExpandWalletPolicyChunks(strs []string) (Template, []ExpandedKey, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return Template{}, nil, err
	}
	keys, err := ExpandWalletPolicy(d)
	if err != nil {
		return Template{}, nil, err
	}
	return summarize(d), keys, nil
}

// resolveOriginPath applies the R0-I1 precedence and converts the resolved
// originPath components to a bip32.Path (in-band hardening).
func resolveOriginPath(d *descriptor, idx uint8) bip32.Path {
	// 1. Per-@N override.
	if d.tlv.originPresent {
		for _, o := range d.tlv.originOverrides {
			if o.idx == idx {
				return componentsToPath(o.path.components)
			}
		}
	}
	// 2. path_decl baseline (Divergent[idx] / Shared), when non-empty.
	var declComps []pathComponent
	if d.pathDecl.divergent != nil {
		if int(idx) < len(d.pathDecl.divergent) {
			declComps = d.pathDecl.divergent[idx].components
		}
	} else if d.pathDecl.shared != nil {
		declComps = d.pathDecl.shared.components
	}
	if len(declComps) != 0 {
		return componentsToPath(declComps)
	}
	// 3. canonicalOrigin(tree) fallback (R0-I1) — for an elided shared path the
	// decoder accepts the implied wrapper path; reuse it so the displayed
	// key-origin serializes the right depth/childnum.
	if co, ok := canonicalOrigin(d.tree); ok {
		return componentsToPath(co.components)
	}
	return nil
}

// componentsToPath converts originPath components to a bip32.Path, encoding
// hardening in-band as value + hdkeychain.HardenedKeyStart (R0-I2).
func componentsToPath(comps []pathComponent) bip32.Path {
	if len(comps) == 0 {
		return nil
	}
	p := make(bip32.Path, len(comps))
	for i, c := range comps {
		v := c.value
		if c.hardened {
			v += hdkeychain.HardenedKeyStart
		}
		p[i] = v
	}
	return p
}

// resolveUseSite applies the override > baseline precedence and converts to the
// exported UseSite shape.
func resolveUseSite(d *descriptor, idx uint8) UseSite {
	us := d.useSite
	if d.tlv.useSitePresent {
		for _, o := range d.tlv.useSiteOverrides {
			if o.idx == idx {
				us = o.path
				break
			}
		}
	}
	out := UseSite{HasMultipath: us.hasMultipath, WildcardHardened: us.wildcardHardened}
	if us.hasMultipath {
		out.Multipath = make([]UseSiteAlt, len(us.multipath))
		for i, a := range us.multipath {
			out.Multipath[i] = UseSiteAlt{Hardened: a.hardened, Value: a.value}
		}
	}
	return out
}

func hasFingerprint(d *descriptor, idx uint8) bool {
	if !d.tlv.fpPresent {
		return false
	}
	for _, fp := range d.tlv.fingerprints {
		if fp.idx == idx {
			return true
		}
	}
	return false
}

func fingerprintBytesFor(d *descriptor, idx uint8) [4]byte {
	if d.tlv.fpPresent {
		for _, fp := range d.tlv.fingerprints {
			if fp.idx == idx {
				return fp.fp
			}
		}
	}
	return [4]byte{}
}

func hasXpub(d *descriptor, idx uint8) bool {
	if !d.tlv.pubPresent {
		return false
	}
	for _, p := range d.tlv.pubkeys {
		if p.idx == idx {
			return true
		}
	}
	return false
}

func xpubBytesFor(d *descriptor, idx uint8) [65]byte {
	if d.tlv.pubPresent {
		for _, p := range d.tlv.pubkeys {
			if p.idx == idx {
				return p.xpub
			}
		}
	}
	return [65]byte{}
}
