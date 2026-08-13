// Command journeykeys derives the cosigner set for the operator-journey
// document: N deterministic BIP-39 seeds and the account xpub each one yields
// at m/48'/0'/0'/2'.
//
// It uses the same code path the device does (bip39.MnemonicSeed →
// hdkeychain.NewMaster → bip32.Derive → Neuter), so the xpubs in the journey
// are the xpubs the machine would show.
//
// Scratch tool for building design/journeys artifacts; not part of the firmware.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
)

type key struct {
	Index    int    `json:"index"`
	Mnemonic string `json:"mnemonic"`
	Path     string `json:"path"`
	FP       string `json:"fingerprint"`
	Xpub     string `json:"xpub"`
}

func main() {
	n := 12
	pathStr := "m/48'/0'/0'/2'"
	path, err := bip32.ParsePath(pathStr)
	if err != nil {
		panic(err)
	}
	net := &chaincfg.MainNetParams

	var out []key
	for i := 0; i < n; i++ {
		// Deterministic 128-bit entropy, so the document regenerates identically.
		h := sha256.Sum256([]byte(fmt.Sprintf("shibboleth journey cosigner %d", i)))
		m := bip39.New(h[:16])
		if !m.Valid() {
			panic(fmt.Sprintf("cosigner %d: generated mnemonic is invalid", i))
		}

		seed := bip39.MnemonicSeed(m, "")
		master, err := hdkeychain.NewMaster(seed, net)
		if err != nil {
			panic(err)
		}
		pk, err := master.ECPubKey()
		if err != nil {
			panic(err)
		}
		fp := bip32.Fingerprint(pk)
		acctPriv, err := bip32.Derive(master, path)
		if err != nil {
			panic(err)
		}
		acct, err := acctPriv.Neuter()
		if err != nil {
			panic(err)
		}
		out = append(out, key{
			Index:    i,
			Mnemonic: m.String(),
			Path:     pathStr,
			FP:       fmt.Sprintf("%08x", fp),
			Xpub:     acct.String(),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
