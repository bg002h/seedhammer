package gui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
)

// TestWshWitnessScriptHashesToRustsAddress is the gate for the segwit-v0 script
// emitter: the script this device builds must hash to the SAME P2WSH address
// the Rust primary derives.
//
// This is a stronger check than comparing scripts would be. A P2WSH address is
// sha256(witnessScript) in bech32 — so agreeing on the address means agreeing
// on every byte of the script, including the `v:` wrapper's VERIFY merging,
// script-number minimal encoding, and key order. One wrong opcode moves the
// address entirely.
func TestWshWitnessScriptHashesToRustsAddress(t *testing.T) {
	// EVERY vendored wsh vector the emitter can handle, not one. A single
	// fixture leaves whole rules untested: with only the timelock/hashlock
	// vector, dropping `sortedmulti`'s BIP-67 key sort changed nothing, because
	// no vector reached that branch.
	paths, err := filepath.Glob(filepath.Join("..", "md", "testdata", "vectors", "keyed_wsh_*.conformance.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no keyed_wsh_* vectors vendored — this gate checks NOTHING")
	}
	total, refused := 0, 0
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".conformance.json")
		n := checkWshVector(t, name, p)
		if n < 0 {
			refused++
			continue
		}
		total += n
	}
	if total == 0 {
		t.Fatalf("no addresses compared (%d vectors refused) — the gate asserted nothing", refused)
	}
	t.Logf("cross-checked %d wsh addresses against Rust across %d vector(s), %d refused",
		total, len(paths)-refused, refused)
}

// checkWshVector returns the number of addresses compared, or -1 if the emitter
// refused the shape (which is a legitimate outcome, not a failure).
func checkWshVector(t *testing.T, name, p string) int {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var rec struct {
		Chains map[string]struct {
			Addresses []string `json:"addresses"`
		} `json:"chains"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse: %v", err)
	}

	chunks := loadVectorChunks(t, name)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicy: %v", err)
	}

	checked := 0
	for chainStr, chain := range rec.Chains {
		change := chainStr == "1"
		for i, want := range chain.Addresses {
			derived := map[uint8][]byte{}
			for _, k := range keys {
				children, ok := useSiteToChildren(k.UseSite)
				if !ok {
					t.Fatalf("@%d unsupported use-site", k.Index)
				}
				bk := bip380.Key{
					Network:           &chaincfg.MainNetParams,
					MasterFingerprint: fpFromBytes(k.Fingerprint),
					DerivationPath:    k.OriginPath,
					Children:          children,
					KeyData:           append([]byte(nil), k.Xpub[32:65]...),
					ChainCode:         append([]byte(nil), k.Xpub[0:32]...),
				}
				pk, err := address.DeriveChild(bk, uint32(i), change)
				if err != nil {
					t.Fatalf("@%d derive: %v", k.Index, err)
				}
				derived[k.Index] = pk.SerializeCompressed()
			}

			script, err := md.EmitWitnessScriptChunks(chunks, derived)
			if errors.Is(err, md.ErrScriptUnsupported) {
				return -1 // a shape this emitter does not build yet
			}
			if err != nil {
				t.Fatalf("%s: emit witness script: %v", name, err)
			}
			got, err := address.WitnessScriptAddress(script, &chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("address: %v", err)
			}
			if got != want {
				t.Errorf("%s chain %s index %d:\n  go:   %s\n  rust: %s\n  script: %x",
					name, chainStr, i, got, want, script)
			}
			checked++
		}
	}
	return checked
}
