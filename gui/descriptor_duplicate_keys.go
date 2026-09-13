package gui

import (
	"seedhammer.com/address"
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

// descriptorRepeatsAKey reports the first two positions in desc.Keys that put
// the same public key at two seats of the script.
//
// IT ASKS address.DerivesSameKey RATHER THAN COMPARING FIELDS. This function
// used to compare KeyData, ChainCode and the Children STRUCT, and review C-1
// broke it with a one-token edit: `A` and `A/<0;1>/*` are the same key written
// two ways, derivePubKey normalises the first into the second, and the struct
// comparison called them different -- so wsh(sortedmulti(2,A,A/<0;1>/*,B))
// walked past the refusal and paid out the byte-identical address the refused
// fixture produces. Three more spellings did the same.
//
// The lesson is placement, not arithmetic: the normalisations belong to the
// deriver, so the equality question does too. Anything here can only ever be a
// copy of them, and a copy is what drifted.
//
// Returns the two indices ascending, so the answer does not depend on iteration
// order.
func descriptorRepeatsAKey(desc *bip380.Descriptor) (int, int, bool) {
	if desc == nil {
		return 0, 0, false
	}
	// O(n^2), which is fine: BIP-388 caps a multisig at 20 key expressions and
	// this device's limit is lower. A map keyed on a serialised expression
	// would allocate, and DescriptorScreen.Draw is on the 0-alloc benchmarked
	// path (TestAllocs).
	for i := range desc.Keys {
		for j := i + 1; j < len(desc.Keys); j++ {
			if address.DerivesSameKey(desc.Keys[i], desc.Keys[j]) {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// maxTitleDrawn bounds the descriptor Title this device DRAWS.
//
// The value is a display budget, not a format rule: DescriptorScreen neither
// scrolls nor clips, so every character of an unbounded scanned Title displaces
// a character of something the device chose to say. 48 leaves the Type and
// Script lines intact at the SH2's 480x320 while still showing a name long
// enough to recognise a wallet by.
const maxTitleDrawn = 48

// truncateForDisplay shortens s to at most n runes, marking the cut.
//
// RUNES, NOT BYTES, so a multi-byte character is never split into invalid UTF-8
// on its way to the rasteriser. ASCII "..." rather than an ellipsis: the body
// face lacks that glyph, and a missing rune blanks the whole line rather than
// degrading one character (gui/font_coverage_test.go).
func truncateForDisplay(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
