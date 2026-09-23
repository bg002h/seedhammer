package md

import (
	"encoding/json"
	"strings"
	"testing"
)

// F-449 stage 4: SPEC §7's sibling key-path kind on the Go side, and the
// exported Liana key the device's address deriver reads.

// TestKeyPathNamesTheLianaKind is SPEC §7's Go sibling kind, on both the
// structural walk and the Template the inspect screen reads.
//
// Mutations: keyPathOf mapping InternalKeyLianaUnspendable to KeyPathNUMS fails
// the kind-1 rows; rootKeyPath returning KeyPathNone fails the Template rows.
func TestKeyPathNamesTheLianaKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		want KeyPathKind
	}{
		{"keyed_tr_liana_kofn_recovery", KeyPathLianaUnspendable},
		{"keyed_compose_preset_kofn_recovery", KeyPathNUMS},
	} {
		chunks := vectorChunksFor(t, tc.name)
		shape, err := PolicyShapeChunks(chunks)
		if err != nil {
			t.Fatal(err)
		}
		if !shape.Complete || shape.KeyPath != tc.want {
			t.Fatalf("%s: PolicyShape KeyPath = %d (complete %v), want %d", tc.name, shape.KeyPath, shape.Complete, tc.want)
		}
		tpl, _, err := ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatal(err)
		}
		if tpl.KeyPath != tc.want {
			t.Fatalf("%s: Template.KeyPath = %d, want %d", tc.name, tpl.KeyPath, tc.want)
		}
	}
}

// TestAnUnknownInternalKeyKindIsNotDescribed: a kind keyPathOf has no name for
// makes the whole summary incomplete, never a neighbouring kind (SPEC §7a row
// 1's failure, one layer up). No decoder yields such a kind today -- the
// version set {4, 8} admits three -- so the tree is built by hand.
//
// Mutation: keyPathOf's fall-through returning (KeyPathNUMS, true) fails.
func TestAnUnknownInternalKeyKindIsNotDescribed(t *testing.T) {
	leaf := node{tag: tagPkK, body: keyArgBody{index: 0}}
	tree := node{tag: tagTr, body: trBody{ik: InternalKeyKind(3), tree: &leaf}}
	if s := policyShape(tree); s.Complete {
		t.Fatalf("an unknown internal-key kind was described: %+v", s)
	}
	if kp := rootKeyPath(tree); kp != KeyPathNone {
		t.Fatalf("rootKeyPath named an unknown kind %d", kp)
	}
}

// TestLianaUnspendableKeyForIsTheRecipeOverTheSuppliedLeaves binds the
// exported entry the device's address deriver calls to the recipe the Go leg
// of SPEC §8.1 already proved against Liana's goldens -- over the keyed card
// AND over its key-less template with the same keys supplied (the Wallet
// Policy template + mk1 cards route, R0 I1).
//
// Mutation: returning lianaUnspendableKey(nil) fails the Rust comparison.
// (Visiting the keys in index order instead of occurrence order is inert by
// construction: canonical numbering is first-occurrence -- stage 3's M6 note.)
func TestLianaUnspendableKeyForIsTheRecipeOverTheSuppliedLeaves(t *testing.T) {
	chunks := vectorChunksFor(t, "keyed_tr_liana_kofn_recovery")
	_, keys, err := ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	xpubs := map[uint8][65]byte{}
	for _, k := range keys {
		xpubs[k.Index] = k.Xpub
	}
	got, err := LianaUnspendableKeyFor(chunks, xpubs)
	if err != nil {
		t.Fatal(err)
	}
	// The reference is the Rust primary's own rendering of this wallet's
	// internal key (the vendored record's chain-0 descriptor), not a second
	// Go walk: F-449 stage 4 R1 N3 retired lianaLeafPubkeys so the recipe has
	// ONE key walk.
	var rec keyedConformanceRecord
	if err := json.Unmarshal(vectorRecordFor(t, "keyed_tr_liana_kofn_recovery"), &rec); err != nil {
		t.Fatal(err)
	}
	xkey, _, _ := strings.Cut(strings.TrimPrefix(rec.Chains["0"].Descriptor, "tr("), "/")
	rust, err := parseExtendedKey(xkey)
	if err != nil {
		t.Fatal(err)
	}
	if got != rust.material {
		t.Fatalf("got %x, want the Rust primary's %x", got, rust.material)
	}
	tmpl, err := StripToTemplate(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if fromTemplate, err := LianaUnspendableKeyFor(tmpl, xpubs); err != nil || fromTemplate != got {
		t.Fatalf("over the key-less template with the same keys: %x, %v; want %x", fromTemplate, err, got)
	}
	delete(xpubs, 3)
	if _, err := LianaUnspendableKeyFor(tmpl, xpubs); err == nil {
		t.Fatal("a missing leaf key yielded a Liana key")
	}
	if _, err := LianaUnspendableKeyFor(vectorChunksFor(t, "keyed_compose_preset_kofn_recovery"), nil); err == nil {
		t.Fatal("a kind-0 set yielded a Liana key")
	}
}
