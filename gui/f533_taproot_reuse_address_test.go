package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// f533Vectors are the two cards F-533 refuses: tr(@0, multi_a/sortedmulti_a
// (2,@0,@1)) -- one slot at the taproot key path AND inside a leaf. They are
// loaded through the fork-side pin (F-529).
var f533Vectors = []string{"keyed_tr_multi_a", "keyed_tr_sortedmulti_a"}

// TestF533RefusalHappensAtTheBranchTheDeviceTakes is the anti-false-GREEN gate,
// and it is the reason this file exists at all.
//
// cmd/policyprobe -- the corpus harness that measures this change -- wraps
// policyAddressAt, the ROUTER. Its own header warns that wrapping the wrong
// branch "manufactures device findings", and the inverse holds here: a corpus
// result that moved for some other reason would look exactly like this gate
// working. So each claim is made separately.
//
//  1. The flat *bip380.Descriptor route does NOT admit these, so the router
//     falls through to complexAddressSource. If it ever did admit them, the
//     F-531/F-533 gate would be bypassed and the device would derive.
//  2. complexAddressDeriver -- the body BELOW the gate -- DOES derive them.
//     Without this the refusal could be an emitter that cannot do the work,
//     and the gate would be asserting nothing. This is the same reason
//     complexAddressDeriver is callable at all.
//  3. complexAddressSource refuses.
//  4. policyAddressAt, the router the inspect screen and the probe both call,
//     refuses.
//
// MUTATION: disable the gate line in complexAddressSource (`&& false`) and
// steps 3 and 4 fail -- printing the mainnet address the device would have
// shown -- while 1 and 2 stay green. That is what says the missing address is
// the GATE's doing and not the shape's.
//
// AND ONE THAT DOES NOT REACH STEPS 3 AND 4, stated because assuming it would
// is how a mutation note goes stale: dropping the internal-key term from
// md.DuplicateKeySlot fails this test at its own PRECONDITION instead, naming
// the fixture as no longer carrying the kind. The precondition is doing its
// job there -- without it the test would quietly pass for a policy F-533 is
// not about -- so the predicate's own mutation is measured in md, not here.
func TestF533RefusalHappensAtTheBranchTheDeviceTakes(t *testing.T) {
	for _, name := range f533Vectors {
		t.Run(name, func(t *testing.T) {
			chunks := loadVectorChunks(t, name)
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			slot, kind, err := md.DuplicateKeySlotChunks(chunks)
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if kind != md.DuplicateTaprootInternalKey {
				t.Fatalf("the fixture reports %v (@%d), not the kind F-533 is about; "+
					"this test would be measuring a different rule", kind, slot)
			}

			if _, status := expandedToDescriptor(tpl, keys); status == expandOK {
				t.Fatalf("the FLAT address route admits this policy, so the gate in " +
					"complexAddressSource is not on the path the device takes and the " +
					"corpus result comes from somewhere else")
			}

			if _, ok := complexAddressDeriver(chunks, keys); !ok {
				t.Fatalf("the emitter below the gate cannot derive this shape, so the " +
					"refusal proves nothing: the address would be missing either way")
			}

			if _, ok := complexAddressSource(chunks, keys); ok {
				t.Errorf("complexAddressSource derives an address for a policy that puts " +
					"slot @0 at the taproot key path AND in a leaf; BIP 388 forbids it " +
					"and the Rust primary refuses it (F-533)")
			}

			if at, ok := policyAddressAt(chunks, tpl, keys); ok {
				a, _ := at(0, false)
				t.Errorf("policyAddressAt -- the router the inspect screen and "+
					"cmd/policyprobe both call -- derives %s for the same policy", a)
			}
		})
	}
}

// TestF533SurfacesSayWhyThereIsNoAddress is the F-531 sibling claim for the new
// kind: every surface that would have shown an address carries the sentence.
//
// A silent refusal is the worse screen. The two sentences are separate on
// purpose -- the warning is a fact about the CARD, the refusal a fact about
// this device -- and both are owed here because these cards carry real xpubs.
func TestF533SurfacesSayWhyThereIsNoAddress(t *testing.T) {
	for _, name := range f533Vectors {
		t.Run(name, func(t *testing.T) {
			chunks := loadVectorChunks(t, name)
			slot, kind, err := md.DuplicateKeySlotChunks(chunks)
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			want := composerCopyDuplicateKeys(slot, kind)
			if !strings.Contains(want, "BIP 388") {
				t.Fatalf("the sentence for %v does not name the rule it rests on: %q",
					kind, want)
			}
			// And NOT the sentence this kind would have inherited from the
			// fallthrough, which this repo measured FALSE for these two cards:
			// Core 31.1 imports both.
			if strings.Contains(want, "Bitcoin Core refuses") {
				t.Fatalf("the operator is told Bitcoin Core refuses a descriptor Core "+
					"IMPORTS: %q", want)
			}

			lines, err := composerConsentLinesFor(chunks, nil, 0, false)
			if err != nil {
				t.Fatalf("composerConsentLinesFor: %v", err)
			}
			assertCarriesWarning(t, lines, want, "the composer consent screen")
			assertShowsNoAddress(t, lines, "the composer consent screen")

			consent, err := walletPolicyConsentLines(chunks, nil)
			if err != nil {
				t.Fatalf("walletPolicyConsentLines: %v", err)
			}
			assertCarriesWarning(t, consent, want, "the Engrave Wallet Policy consent")
			assertShowsNoAddress(t, consent, "the Engrave Wallet Policy consent")

			// The Inspect-descriptor route, which is the screen F-514 was
			// measured on and the one F-531's review found carrying nothing.
			keysFor := func() []md.ExpandedKey {
				_, keys, err := md.ExpandWalletPolicyChunks(chunks)
				if err != nil {
					t.Fatalf("ExpandWalletPolicyChunks: %v", err)
				}
				return keys
			}
			body, ok := duplicateRefusalBody(chunks, keysFor())
			if !ok {
				t.Fatal("the Inspect screen shows no modal for this card")
			}
			if !strings.Contains(body, want) {
				t.Errorf("the Inspect modal does not carry the warning:\n%s", body)
			}
			if !strings.Contains(body, composerCopyNoAddressesDuplicateKeys()) {
				t.Errorf("the Inspect modal never says why there is no address:\n%s", body)
			}
		})
	}
}
