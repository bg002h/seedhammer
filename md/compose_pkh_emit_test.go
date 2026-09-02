package md

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
)

// The keyed compose vectors whose wsh body contains pkh(@i) -- the shape
// Multisig Build never produced, so the emitter never had to know it.
var pkhWshVectors = []string{
	"keyed_compose_wsh_two_path_or_d",
	"keyed_compose_wsh_single_head_or_i",
	"keyed_compose_wsh_locked_head_or_i",
	"keyed_compose_wsh_hash_and_time",
	"keyed_compose_wsh_three_paths",
}

type composeConformanceKeys struct {
	Keys []struct {
		Index uint8  `json:"index"`
		Xpub  string `json:"xpub"`
	} `json:"keys"`
	Chains map[string]struct {
		Addresses []string `json:"addresses"`
	} `json:"chains"`
}

func loadComposeConformance(t *testing.T, name string) composeConformanceKeys {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "conformance.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var rec composeConformanceKeys
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rec
}

// derivedKeys derives every slot's compressed pubkey at <chain>/<index> from
// the record's account xpubs (use-site <0;1>/*).
func derivedKeys(t *testing.T, rec composeConformanceKeys, chain, index uint32) map[uint8][]byte {
	t.Helper()
	keys := map[uint8][]byte{}
	for _, k := range rec.Keys {
		ek, err := hdkeychain.NewKeyFromString(k.Xpub)
		if err != nil {
			t.Fatalf("@%d xpub: %v", k.Index, err)
		}
		c, err := ek.Derive(chain)
		if err != nil {
			t.Fatalf("@%d/%d: %v", k.Index, chain, err)
		}
		c, err = c.Derive(index)
		if err != nil {
			t.Fatalf("@%d/%d/%d: %v", k.Index, chain, index, err)
		}
		pub, err := c.ECPubKey()
		if err != nil {
			t.Fatalf("@%d pubkey: %v", k.Index, err)
		}
		keys[k.Index] = pub.SerializeCompressed()
	}
	return keys
}

func p2wshAddress(t *testing.T, script []byte) string {
	t.Helper()
	h := sha256.Sum256(script)
	a, err := btcaddr.NewAddressWitnessScriptHash(h[:], &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("p2wsh: %v", err)
	}
	return a.EncodeAddress()
}

// TestPkhWitnessScriptsReproduceRustsAddresses: for each pkh-bearing vector,
// the emitted witness script's P2WSH address at receive and change index 0
// and 1 equals what the Rust primary derived for the same descriptor and keys.
func TestPkhWitnessScriptsReproduceRustsAddresses(t *testing.T) {
	for _, name := range pkhWshVectors {
		t.Run(name, func(t *testing.T) {
			rec := loadComposeConformance(t, name)
			chunks := loadPhraseChunks(t, name)
			// Receive (chain 0) AND change (chain 1), indices 0 and 1 each: §12
			// item 1's "receive 0..1, change 0..1" (fidelity M-2).
			for chain := uint32(0); chain < 2; chain++ {
				want := rec.Chains[strconv.Itoa(int(chain))].Addresses
				if len(want) < 2 {
					t.Fatalf("record has %d chain-%d addresses, want >= 2", len(want), chain)
				}
				for i := uint32(0); i < 2; i++ {
					script, err := EmitWitnessScriptChunks(chunks, derivedKeys(t, rec, chain, i))
					if err != nil {
						t.Fatalf("EmitWitnessScriptChunks(chain %d index %d): %v", chain, i, err)
					}
					if got := p2wshAddress(t, script); got != want[i] {
						t.Errorf("chain %d index %d:\n  go:   %s\n  rust: %s", chain, i, got, want[i])
					}
				}
			}
		})
	}
}

// The key enters the script through its hash: a different key at the pkh slot
// moves the address. (The manual mutation in Step 5 -- swap OP_EQUALVERIFY for
// OP_EQUAL -- is the one that proves the OPCODES are checked; this one proves
// the HASH is of the supplied key and not a constant.)
func TestPkhScriptDependsOnTheKey(t *testing.T) {
	name := "keyed_compose_wsh_single_head_or_i" // or_i(pkh(@0), and_v(v:pkh(@1), older(..)))
	rec := loadComposeConformance(t, name)
	chunks := loadPhraseChunks(t, name)
	keys := derivedKeys(t, rec, 0, 0)
	base, err := EmitWitnessScriptChunks(chunks, keys)
	if err != nil {
		t.Fatal(err)
	}
	flipped := map[uint8][]byte{}
	for k, v := range keys {
		flipped[k] = append([]byte(nil), v...)
	}
	flipped[0][32] ^= 0x01
	mut, err := EmitWitnessScriptChunks(chunks, flipped)
	if err != nil {
		t.Fatal(err)
	}
	if p2wshAddress(t, base) == p2wshAddress(t, mut) {
		t.Fatal("changing slot @0's key did not change the address: the pkh arm is not hashing the key")
	}
	if len(base) != len(mut) {
		t.Fatalf("a key change altered the script LENGTH (%d vs %d); pkh must push a fixed 20-byte hash", len(base), len(mut))
	}
}

// v:multi_a must fold OP_NUMEQUAL into OP_NUMEQUALVERIFY (0x9d), never append
// OP_VERIFY after it: the two scripts differ, so the leaf hash and the address
// differ. keyed_compose_tr_nums_three_leaves carries the first verify-wrapped
// multi_a in this repo; before the fold arm existed it derived a wrong address
// (composer-S2-implementation-report F-1; the gui address gate is the oracle,
// this is the byte-level pin).
func TestVerifyWrappedMultiAFoldsIntoNumEqualVerify(t *testing.T) {
	chunks := loadPhraseChunks(t, "keyed_compose_tr_nums_three_leaves")
	keys := map[uint8][]byte{}
	for i := uint8(0); i < 4; i++ {
		k := make([]byte, 32)
		for j := range k {
			k[j] = byte(i + 1)
		}
		keys[i] = k
	}
	_, isNUMS, leaves, err := EmitTapLeavesChunks(chunks, keys)
	if err != nil {
		t.Fatalf("EmitTapLeavesChunks: %v", err)
	}
	if !isNUMS || len(leaves) != 3 {
		t.Fatalf("isNUMS=%v leaves=%d, want NUMS with three leaves", isNUMS, len(leaves))
	}
	folded := 0
	for i, l := range leaves {
		s := l.Script
		for j := 0; j+1 < len(s); j++ {
			if s[j] == opNUMEQUAL && s[j+1] == opVERIFY {
				t.Errorf("leaf %d emits OP_NUMEQUAL OP_VERIFY at %d; miniscript folds to OP_NUMEQUALVERIFY", i, j)
			}
		}
		if bytesContain(s, opNUMEQUALVERIFY) {
			folded++
		}
	}
	if folded != 1 {
		t.Fatalf("%d leaves carry OP_NUMEQUALVERIFY, want exactly one (the v:multi_a leaf)", folded)
	}
}

func bytesContain(s []byte, b byte) bool {
	for _, x := range s {
		if x == b {
			return true
		}
	}
	return false
}

// Tapscript context: the composer never emits pkh under tr (path_body uses pk /
// multi_a there), but §9 item 2 asks for the arm in both contexts. A hand-built
// tr(NUMS, pkh(@0)) leaf must emit DUP HASH160 <hash160(xonly)> EQUALVERIFY
// CHECKSIG with the 32-byte key the caller supplied.
func TestPkhTapLeafEmitsTheHash160Form(t *testing.T) {
	leaf := node{tag: tagPkH, body: keyArgBody{index: 0}}
	shared := originPath{components: toComponents(DefaultOrigin(ComposeTr, 0))}
	d := &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &shared},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{hardened: false, value: 0}, {hardened: false, value: 1}},
		},
		tree: node{tag: tagTr, body: trBody{isNums: true, keyIndex: 0, tree: &leaf}},
	}
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	xonly := make([]byte, 32)
	for i := range xonly {
		xonly[i] = byte(i + 1)
	}
	_, isNUMS, leaves, err := EmitTapLeavesChunks(chunks, map[uint8][]byte{0: xonly})
	if err != nil {
		t.Fatalf("EmitTapLeavesChunks: %v", err)
	}
	if !isNUMS || len(leaves) != 1 {
		t.Fatalf("isNUMS=%v leaves=%d, want NUMS with one leaf", isNUMS, len(leaves))
	}
	want := append([]byte{opDUP, opHASH160, 0x14}, btcaddr.Hash160(xonly)...)
	want = append(want, opEQUALVERIFY, opCHECKSIG)
	if got := leaves[0].Script; string(got) != string(want) {
		t.Fatalf("leaf script\n got %x\nwant %x", got, want)
	}
}
