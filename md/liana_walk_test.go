package md

import (
	"encoding/json"
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
		chunks := vectorChunksFor(t, name)
		d, err := Reassemble(chunks)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var occ []uint8
		collectKeyOccurrences(*d.tree.body.(trBody).tree, &occ)
		if len(occ) != 4 {
			t.Fatalf("%s: %d leaf-key occurrences, want 4", name, len(occ))
		}
		k, err := lianaKeyOverOwnKeys(chunks, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		keys = append(keys, k)
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
	chunks := vectorChunksFor(t, name)
	body, _, _ := strings.Cut(rec.Chains["0"].Descriptor, "#")
	if _, err := reduceLianaInternalKey("0", body, chunks); err != nil {
		t.Fatalf("control: the genuine record is refused: %v", err)
	}
	// The same four keys REVERSED: each key moves to the mirrored placeholder,
	// so the one key walk (@0..@3, one occurrence each) feeds the recipe the
	// reversed leaf list.
	near, err := lianaKeyOverOwnKeys(chunks, func(i uint8) uint8 { return 3 - i })
	if err != nil {
		t.Fatal(err)
	}
	nearKey := hdkeychain.NewExtendedKey(chaincfg.MainNetParams.HDPublicKeyID[:],
		near[32:], near[:32], []byte{0, 0, 0, 0}, 0, 0, false)
	rest := strings.TrimPrefix(body, "tr(")
	_, after, _ := strings.Cut(rest, "/")
	forged := "tr(" + nearKey.String() + "/" + after
	if _, err := reduceLianaInternalKey("0", forged, chunks); err == nil {
		t.Fatal("a recipe output over a DIFFERENT leaf order was accepted as this wallet's internal key")
	}
}

// lianaKeyOverOwnKeys is LianaUnspendableKeyFor over a keyed card's own
// Pubkeys-TLV keys, read out by ExpandWalletPolicyChunks -- the route the
// device's keyed-card consent takes. move, when non-nil, seats the key of
// placeholder move(i) at @i, which builds a near-miss over the same keys.
func lianaKeyOverOwnKeys(chunks []string, move func(uint8) uint8) ([65]byte, error) {
	_, keys, err := ExpandWalletPolicyChunks(chunks)
	if err != nil {
		return [65]byte{}, err
	}
	own := map[uint8][65]byte{}
	for _, k := range keys {
		if k.XpubPresent {
			own[k.Index] = k.Xpub
		}
	}
	xpubs := own
	if move != nil {
		xpubs = map[uint8][65]byte{}
		for i := range own {
			xpubs[i] = own[move(i)]
		}
	}
	return LianaUnspendableKeyFor(chunks, xpubs)
}
