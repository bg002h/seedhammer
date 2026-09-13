package gui

import (
	"bytes"

	"seedhammer.com/bip380"
)

// The duplicate-key rule, expressed over a *bip380.Descriptor (F-530).
//
// WHY A SECOND PREDICATE AND NOT md.DuplicateKeySlot. That one walks a decoded
// md1 TREE and answers in terms of @N slots. Two of descriptorFlow's three
// callers have no md1 at all -- a scanned descriptor (gui/gui.go) and a payload
// record via nonstandard.OutputDescriptor (gui/wallet_policy.go) -- so there is
// no tree to walk and no slot to name. Threading chunks into the third caller
// would have closed a third of the surface and left the two that matter silent
// under a suite that then looks complete, which is exactly how the first
// surface stayed hidden through a whole review round.
//
// IT IS ALSO THE MORE VALUABLE RULE, because it catches a descriptor an
// operator pasted or scanned from somewhere else -- the case md1 cannot reach
// by construction.

// descriptorRepeatsAKey reports the first two positions in desc.Keys that are
// the same KEY EXPRESSION, and so put the same public key at two seats of the
// script.
//
// WHAT IS COMPARED, AND WHY EACH CHOICE. A multisig script holds derived
// pubkeys and nothing else, so two entries collide exactly when they derive the
// same pubkey at every index:
//
//   - KeyData and ChainCode: the xpub itself. Same pair, same CKDpub output.
//   - Children: the derivation applied at the use site. INCLUDED, and this is
//     the load-bearing one -- X/0/* and X/1/* are the SAME xpub and derive
//     DIFFERENT keys, BIP 388 permits exactly that on one placeholder, and a
//     predicate ignoring Children would refuse a legal wallet. Refusing a good
//     wallet is the expensive direction of wrong here, because the operator's
//     recourse is to stop using this device.
//
// DELIBERATELY NOT COMPARED: MasterFingerprint and DerivationPath. Those record
// where the xpub came from, they are metadata that never reaches the script,
// and two entries differing only there still push identical bytes. Comparing
// them would be a predicate that misses the defect whenever the two seats were
// labelled with different origins -- which is what a coordinator bug producing
// this shape would plausibly do.
//
// Returns the two indices ascending, so the answer does not depend on iteration
// order.
func descriptorRepeatsAKey(desc *bip380.Descriptor) (int, int, bool) {
	if desc == nil {
		return 0, 0, false
	}
	for i := range desc.Keys {
		for j := i + 1; j < len(desc.Keys); j++ {
			if sameKeyExpression(desc.Keys[i], desc.Keys[j]) {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// sameKeyExpression reports whether two descriptor keys derive the same public
// key at every index.
//
// O(n^2) over the key list above, which is fine: BIP-388 caps a multisig at 20
// key expressions and this device's own limit is lower. A map keyed on the
// serialized expression would allocate, and DescriptorScreen.Draw is on the
// 0-alloc benchmarked path.
func sameKeyExpression(a, b bip380.Key) bool {
	if !bytes.Equal(a.KeyData, b.KeyData) || !bytes.Equal(a.ChainCode, b.ChainCode) {
		return false
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		x, y := a.Children[i], b.Children[i]
		if x.Type != y.Type || x.Index != y.Index || x.End != y.End || x.Hardened != y.Hardened {
			return false
		}
	}
	return true
}
