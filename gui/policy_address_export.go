package gui

import "seedhammer.com/md"

// PolicyAddressAt exposes policyAddressAt to cmd/policyprobe, the device leg of
// the policy differential harness.
//
// IT CARRIES NO LOGIC. The body is one call and must stay one call: the harness
// exists to compare what THIS DEVICE derives against the Rust primary and
// Bitcoin Core, so anything this wrapper decided for itself — a network, a
// refusal, a retry — would be a fourth implementation quietly standing in for
// the device's. The alternative, copying policyAddressAt into cmd/, would make
// the harness measure the copy and report agreement while the shipped screen
// drifted; that is the one failure this whole leg is built to prevent.
//
// WHY THE ROUTER AND NOT complexAddressSource. It wrapped complexAddressSource
// until 2026-09-13, which is only ONE of the two routes policyAddressAt tries.
// The flat *bip380.Descriptor route runs first and serves every single-key and
// plain-multisig shape, so the probe reported `wpkh`, `tr` key-path-only and
// every `sh(...)` vector as "the device declined this policy shape" — eight of
// them in the vendored corpus — when a real device derives some of those and
// refuses others for entirely different reasons. A harness that names the wrong
// branch as the refusal is worse than one that cannot derive at all, because
// its answer looks like a device finding.
//
// Contract is policyAddressAt's, unchanged: the collected md1 chunks, the
// Template and the ExpandedKeys from md.ExpandWalletPolicyChunks in, a deriver
// func(index uint32, change bool) (string, error) out, and !ok when this device
// cannot derive the shape by EITHER route. !ok is an ANSWER, not an error — the
// harness counts it. Mainnet only (policy_address.go, D1).
func PolicyAddressAt(collected []string, tpl md.Template, keys []md.ExpandedKey) (func(uint32, bool) (string, error), bool) {
	return policyAddressAt(collected, tpl, keys)
}
