package gui

import "seedhammer.com/md"

// PolicyAddressSource exposes complexAddressSource to cmd/policyprobe, the
// device leg of the policy differential harness.
//
// IT CARRIES NO LOGIC. The body is one call and must stay one call: the harness
// exists to compare what THIS DEVICE derives against the Rust primary and
// Bitcoin Core, so anything this wrapper decided for itself — a network, a
// refusal, a retry — would be a fourth implementation quietly standing in for
// the device's. The alternative, copying complexAddressSource into cmd/, would
// make the harness measure the copy and report agreement while the shipped
// screen drifted; that is the one failure this whole leg is built to prevent.
//
// Contract is complexAddressSource's, unchanged: collected md1 chunks plus the
// ExpandedKeys from md.ExpandWalletPolicyChunks in, a deriver
// func(index uint32, change bool) (string, error) out, and !ok when this device
// cannot derive the shape at all. !ok is an ANSWER, not an error — the harness
// counts it. Mainnet only (policy_address.go, D1).
func PolicyAddressSource(collected []string, keys []md.ExpandedKey) (func(uint32, bool) (string, error), bool) {
	return complexAddressSource(collected, keys)
}
