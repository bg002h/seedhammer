package md

// ─── Template identity (WalletDescriptorTemplateId) + form-aware stub ────────
//
// This file ports the keyless-template identity machinery from Rust md-codec:
//   - isWalletPolicy            (encode.rs is_wallet_policy: Some-AND-non-empty)
//   - WalletDescriptorTemplateId (identity.rs:71-104 compute_…)
//   - the form-aware stub selector that routes every mk1-stub mint through the
//     right id space (WalletPolicyId for a keyed policy, WDT-Id for a template).

// isWalletPolicy mirrors Rust md-codec encode.rs is_wallet_policy: the pubkeys
// TLV is present AND non-empty. A keyless template (pubkeys:null) is NOT a
// wallet policy. Testing len() as well as the presence flag guards the I1 bug
// class: a strip that nulled the slice but left pubPresent set must not slip
// through as a wallet-policy (which would also trip errEmptyTLVEncode on
// re-emit).
func isWalletPolicy(d *descriptor) bool {
	return d.tlv.pubPresent && len(d.tlv.pubkeys) > 0
}

// WalletDescriptorTemplateId is the 128-bit BIP-388 wallet-descriptor-template
// identifier (spec §8.1) — a PORT of Rust compute_wallet_descriptor_template_id
// (identity.rs:71-104). It hashes ONLY the template content:
//
//	SHA-256( useSitePath ‖ writeNode(tree) ‖ [UseSitePathOverrides-TLV] )[0:16]
//
// Key-independent and origin-invariant (no keys/fingerprints/origin enter the
// preimage). Distinct per (script family, k, N, use-site).
//
// Deliberately does NOT canonicalize (unlike WalletPolicyId): Rust hashes the
// template content AS-STORED, relying on the decode-side canonical invariant
// (validatePlaceholderUsage + canonical decode form). A future AUTHOR-BUILT
// (non-decoded) AST must be canonicalized before reaching this function.
//
// Because WDT-Id bypasses encodePayload (where the errPathDeclNMismatch guard
// normally lives, encode.go:401), the kiw/n consistency guard is carried INSIDE
// this function: kiw is computed from d.n, and a desynced pathDecl.n is
// rejected (a kiw mismatch would silently corrupt the template-id bitstream).
func WalletDescriptorTemplateId(d *descriptor) ([16]byte, error) {
	// Guard the kiw/n invariant inside the function (WDT-Id bypasses the
	// encodePayload guard at encode.go:401). kiw is derived from d.n below.
	if d.pathDecl.n != d.n {
		return [16]byte{}, errPathDeclNMismatch
	}
	width := kiw(d.n)

	var w bitWriter
	// (a) use-site path bits (identity.rs:77).
	if err := writeUseSitePath(&w, d.useSite); err != nil {
		return [16]byte{}, err
	}
	// (b) tree bits (identity.rs:78).
	if err := writeNode(&w, d.tree, width); err != nil {
		return [16]byte{}, err
	}
	// (c) the UseSitePathOverrides TLV ENTRY, iff present (identity.rs:79-98).
	// Re-encode the entry directly: build the override payload into a sub-writer
	// (idx at kiw bits ‖ writeUseSitePath, per override, ascending), then emit
	// tag(5b) ‖ varint(payload-bit-len) ‖ payload — the use-site branch of
	// writeTLVSection, byte-for-byte vs identity.rs:82-97.
	if d.tlv.useSitePresent {
		if len(d.tlv.useSiteOverrides) == 0 {
			return [16]byte{}, errEmptyTLVEncode
		}
		var sub bitWriter
		var last uint8
		haveLast := false
		for _, e := range d.tlv.useSiteOverrides {
			if haveLast && e.idx <= last {
				return [16]byte{}, errOverrideOrder
			}
			haveLast = true
			last = e.idx
			sub.write(uint64(e.idx), int(width))
			if err := writeUseSitePath(&sub, e.path); err != nil {
				return [16]byte{}, err
			}
		}
		bitLen := sub.bitLen()
		payload := sub.intoBytes()
		w.write(uint64(tlvUseSitePathOverrides), 5)
		if err := writeVarint(&w, uint32(bitLen)); err != nil {
			return [16]byte{}, err
		}
		if err := reEmitBits(&w, payload, bitLen); err != nil {
			return [16]byte{}, err
		}
	}

	return sha256First16(w.intoBytes()), nil
}

// WalletDescriptorTemplateIdStub is the top-4 bytes of WalletDescriptorTemplateId
// — the mk1 KEY card's policy_id_stub source for a KEYLESS template (the
// template-form analogue of WalletPolicyIDStub).
func WalletDescriptorTemplateIdStub(d *descriptor) ([4]byte, error) {
	id, err := WalletDescriptorTemplateId(d)
	if err != nil {
		return [4]byte{}, err
	}
	var stub [4]byte
	copy(stub[:], id[:4])
	return stub, nil
}

// FormAwareStub mints the mk1 policy_id_stub for a descriptor, selecting the id
// space by FORM (port of Rust mk-cli derive_stub_from_md1, mod.rs:72-82): a
// keyed wallet-policy roots on WalletPolicyId; a keyless template roots on
// WalletDescriptorTemplateId. Routed through every stub-mint + verify site so a
// template binds (and the device's own readback verifies) and a keyed policy is
// byte-identical to the prior WalletPolicyId-only behaviour.
func FormAwareStub(d *descriptor) ([4]byte, error) {
	if isWalletPolicy(d) {
		return WalletPolicyIDStub(d)
	}
	return WalletDescriptorTemplateIdStub(d)
}

// FormAwareStubChunks is the chunked-md1-input form of FormAwareStub: it
// reassembles the wire strings (the same Reassemble decode WalletPolicyIDStubChunks
// uses) and selects by form. Reassemble errors surface unchanged.
func FormAwareStubChunks(strs []string) ([4]byte, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return [4]byte{}, err
	}
	return FormAwareStub(d)
}
