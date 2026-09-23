package md

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
)

// ─── SPEC §8.6 Go leg: structure independence over the vendored kind-1 cards ─

// kofn_recovery ({multi_a(2,A,B,C), pk(D)&older}) and nested_two_recoveries
// ({multi_a(2,A,B), {pk(C)&older, pk(D)&older}}) hold the SAME four keys in the
// SAME order, so they MUST derive one internal key -- and Rust rendered the
// same xpub for both (measured: xpub661MyMwAqRbcEbgXQvrr…). A test pins it so
// nobody "fixes" it into a tree-dependent hash.
func TestLianaKeyDependsOnTheLeafKeysNotTheTree(t *testing.T) {
	var keys [][65]byte
	for _, name := range []string{"keyed_tr_liana_kofn_recovery", "keyed_tr_liana_nested_two_recoveries"} {
		d, err := Reassemble(vectorChunksFor(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		leaves, err := lianaLeafPubkeys(d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(leaves) != 4 {
			t.Fatalf("%s: %d leaf-key occurrences, want 4", name, len(leaves))
		}
		keys = append(keys, lianaUnspendableKey(leaves))
	}
	if keys[0] != keys[1] {
		t.Errorf("the two trees derive different internal keys:\n  %x\n  %x", keys[0], keys[1])
	}
}

// ─── SPEC §8.10: the near-miss the positive corpus cannot supply ─────────────

// A valid recipe output over a DIFFERENT leaf set (here: the same four keys
// reversed) is a perfectly formed Liana key -- H pubkey, depth 0, mainnet --
// for a different wallet. reduceLianaInternalKey must refuse it; a check
// weakened to "the pubkey is H" accepts it and relabels a stranger's key as
// this wallet's own.
func TestLianaReductionRefusesANearMiss(t *testing.T) {
	name := "keyed_tr_liana_kofn_recovery"
	var rec keyedConformanceRecord
	if err := json.Unmarshal(vectorRecordFor(t, name), &rec); err != nil {
		t.Fatal(err)
	}
	d, err := Reassemble(vectorChunksFor(t, name))
	if err != nil {
		t.Fatal(err)
	}
	body, _, _ := strings.Cut(rec.Chains["0"].Descriptor, "#")
	if _, err := reduceLianaInternalKey("0", body, d); err != nil {
		t.Fatalf("control: the genuine record is refused: %v", err)
	}
	leaves, err := lianaLeafPubkeys(d)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(leaves)
	near := lianaUnspendableKey(leaves)
	nearKey := hdkeychain.NewExtendedKey(chaincfg.MainNetParams.HDPublicKeyID[:],
		near[32:], near[:32], []byte{0, 0, 0, 0}, 0, 0, false)
	rest := strings.TrimPrefix(body, "tr(")
	_, after, _ := strings.Cut(rest, "/")
	forged := "tr(" + nearKey.String() + "/" + after
	if _, err := reduceLianaInternalKey("0", forged, d); err == nil {
		t.Fatal("a recipe output over a DIFFERENT leaf order was accepted as this wallet's internal key")
	}
}
