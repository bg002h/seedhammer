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
