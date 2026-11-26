package mk

import (
	"errors"
	"fmt"
	"testing"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
)

// Bug hunt 2026-11-25 SH#4: the key-origin rules mk-codec 0.6.0 added to
// encode (crates/mk-codec/src/bytecode/encode.rs check_key_rules, mnemonic-key
// 113d8b6). Encode used to mint every one of these refused cards.

func keyRulesNeuter(t *testing.T, s string) *hdkeychain.ExtendedKey {
	t.Helper()
	k, err := hdkeychain.NewKeyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	if k.IsPrivate() {
		if k, err = k.Neuter(); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

func keyFP(t *testing.T, k *hdkeychain.ExtendedKey) string {
	t.Helper()
	pub, err := k.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", btcaddr.Hash160(pub.SerializeCompressed())[:4])
}

func keyRulesCard(path, fp, xpub string) Card {
	return Card{Path: path, Fingerprint: fp, Stubs: [][4]byte{{1, 2, 3, 4}}, Xpub: xpub}
}

func TestEncodeRefusesWhatMkCodec060Refuses(t *testing.T) {
	master := keyRulesNeuter(t, deriveTestXpub(t, "m", &chaincfg.MainNetParams))
	child := keyRulesNeuter(t, deriveTestXpub(t, "m/0'", &chaincfg.MainNetParams))
	masterFP := keyFP(t, master)
	parentFP := fmt.Sprintf("%08x", child.ParentFingerprint())
	if parentFP != masterFP {
		t.Fatalf("INCONCLUSIVE: child's parent fingerprint %s is not the master's %s", parentFP, masterFP)
	}
	// A depth-0 key whose parent fingerprint is not zero (BIP 32 test vector 5's
	// shape), built from the master's own material.
	pub, err := master.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	badParent := hdkeychain.NewExtendedKey(chaincfg.MainNetParams.HDPublicKeyID[:],
		pub.SerializeCompressed(), master.ChainCode(), []byte{1, 2, 3, 4}, 0, 0, false)
	// A depth-0 key with an all-zero chain code: R3's one exemption.
	zeroCC := hdkeychain.NewExtendedKey(chaincfg.MainNetParams.HDPublicKeyID[:],
		pub.SerializeCompressed(), make([]byte, 32), []byte{0, 0, 0, 0}, 0, 0, false)

	for _, tc := range []struct {
		name string
		card Card
		want error // nil: Encode must succeed
	}{
		{"R2 depth-0 parent fp nonzero", keyRulesCard("m", "", badParent.String()), errEncodeMasterParentFP},
		{"R3 depth-0 origin fp not the key's", keyRulesCard("m", "deadbeef", master.String()), errEncodeMasterFP},
		{"R3 depth-0 origin fp 00000000, real chain code", keyRulesCard("m", "00000000", master.String()), errEncodeMasterFP},
		{"R4 depth-1 origin fp not the parent's", keyRulesCard("m/0'", "deadbeef", child.String()), errEncodeOriginParentFP},
		{"R4 depth-1 origin fp is the CHILD's own", keyRulesCard("m/0'", keyFP(t, child), child.String()), errEncodeOriginParentFP},
		{"depth-0 own fp", keyRulesCard("m", masterFP, master.String()), nil},
		{"depth-0 no fp", keyRulesCard("m", "", master.String()), nil},
		{"depth-0 zero chain code, fp 00000000", keyRulesCard("m", "00000000", zeroCC.String()), nil},
		{"depth-0 zero chain code, own fp", keyRulesCard("m", keyFP(t, zeroCC), zeroCC.String()), nil},
		{"depth-1 parent fp", keyRulesCard("m/0'", masterFP, child.String()), nil},
		{"depth-1 no fp", keyRulesCard("m/0'", "", child.String()), nil},
		// Depth 2 and deeper carry no rule (the origin is the master, which the
		// key does not name).
		{"depth-3 any fp", keyRulesCard("m/84'/0'/0'", "deadbeef",
			deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strs, err := Encode(tc.card)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Encode: %v, want success", err)
				}
				return
			}
			if !errors.Is(err, tc.want) || strs != nil {
				t.Fatalf("Encode = (%d strings, %v), want refusal %v", len(strs), err, tc.want)
			}
		})
	}
}

// A card minted before the rules still decodes (mk-codec 0.6.0 "Decoders:
// unaffected"), and DerivedChunkSetID -- the canonical re-encode of a DECODED
// card, Rust's encode_canonical_payload, which skips the key rules -- still
// answers for it, so the csid warning keeps working on a legacy card.
func TestKeyRulesLeaveLegacyCardsReadable(t *testing.T) {
	child := keyRulesNeuter(t, deriveTestXpub(t, "m/0'", &chaincfg.MainNetParams))
	legacy := keyRulesCard("m/0'", "deadbeef", child.String())
	bytecode, err := encodeBytecode(legacy)
	if err != nil {
		t.Fatalf("encodeBytecode (the canonical payload) refused a legacy card: %v", err)
	}
	strs, err := encodeChunks(bytecode)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(strs)
	if err != nil || got.Fingerprint != "deadbeef" {
		t.Fatalf("legacy card: Decode = (%+v, %v)", got, err)
	}
	if _, err := DerivedChunkSetID(got); err != nil {
		t.Fatalf("DerivedChunkSetID on a decoded legacy card: %v", err)
	}
}
