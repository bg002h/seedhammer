package gui

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
)

// Address derivation for wallet policies a *bip380.Descriptor cannot express.
//
// WHAT THIS CLOSES. expandedToDescriptor projects an md1 onto the flat
// bip380.Descriptor the shipped address code derives from, and reports
// expandUnsupported for every shape that projection loses: a taproot script
// tree, multi_a/sortedmulti_a, and wsh miniscript. Those all landed on one
// screen reading "Complex policy - display only", which was true when the only
// deriver was addressAt's two-case switch.
//
// It is no longer true. md.TapLeavesChunks + address.TaprootScriptPath derive a
// taproot script path, and md.EmitWitnessScriptChunks +
// address.WitnessScriptAddress derive a wsh miniscript address. This file is the
// seam that lets an operator SEE that, because a capability reachable only from
// a Go test is not a capability the device has.
//
// WHY IT IS NOT A SECOND ADDRESS PATH. It shares expandedKeysToBip380 with the
// flat route, so the use-site translation (which multipath child, which range)
// exists once. Package address states the hazard directly: "a use-site applied
// twice or not at all is a wrong address" — and a wrong address here is a backup
// that verifies against funds nobody can spend.

// complexAddressSource returns a deriver for a wallet policy that has real
// xpubs but no faithful bip380.Descriptor, or !ok when this device still cannot
// derive the shape.
//
// PROBED, NOT PREDICTED. Rather than deciding from the template which shapes
// are derivable — a claim that goes stale the moment the emitter grows a
// fragment — it derives index 0 and reports ok only if that succeeded. Same
// contract as address.Supported, which is `Receive(desc, 0)` without an error,
// and it cannot drift from what the emitters actually accept.
func complexAddressSource(collected []string, keys []md.ExpandedKey) (func(uint32, bool) (string, error), bool) {
	if len(keys) == 0 {
		return nil, false // template-only (D3): nothing to derive from.
	}
	for _, k := range keys {
		if !k.XpubPresent {
			return nil, false
		}
	}
	bkeys, ok := expandedKeysToBip380(keys)
	if !ok {
		return nil, false // hardened wildcard / exotic use-site (D5, R0-I2).
	}
	byIndex := make(map[uint8]bip380.Key, len(bkeys))
	for i, k := range keys {
		byIndex[k.Index] = bkeys[i]
	}
	network := &chaincfg.MainNetParams // D1: mainnet-only.

	var src func(uint32, bool) (string, error)
	// PROBE WITH THE FUNCTION THAT DOES THE WORK. EmitTapLeavesChunks returns
	// the internal-key facts itself, so this asks it once with a throwaway key
	// map purely to learn "is this a taproot script tree, and what is its
	// internal key".
	//
	// It used to read those facts from TapLeavesChunks instead, whose error path
	// returns `0, false, nil, err` — so for every shape the DESCRIBER could not
	// name, the caller silently got isNUMS=false and index 0 and derived the
	// internal key from @0. The addresses were well-formed and wrong. Caught
	// only by comparing against Rust: `multi_a` alone matched, and all three
	// `and_v(v:…, multi_a(…))` shapes did not — the difference being exactly
	// which of them the describer could name. Never consume values from a call
	// that returned an error.
	probe := make(map[uint8][]byte, len(byIndex))
	for i := range byIndex {
		probe[i] = make([]byte, 32)
	}
	if ikIndex, isNUMS, _, err := md.EmitTapLeavesChunks(collected, probe); err == nil {
		src = func(index uint32, change bool) (string, error) {
			xonly := make(map[uint8][]byte, len(byIndex))
			for i, k := range byIndex {
				pk, err := address.DeriveChild(k, index, change)
				if err != nil {
					return "", err
				}
				// X-ONLY, not compressed: BIP-341 keys carry no parity byte, and
				// a 33-byte push builds a valid script for a different key.
				xonly[i] = schnorr.SerializePubKey(pk)
			}
			_, _, leaves, err := md.EmitTapLeavesChunks(collected, xonly)
			if err != nil {
				return "", err
			}
			scripts := make([]address.LeafScript, 0, len(leaves))
			for _, l := range leaves {
				scripts = append(scripts, address.LeafScript{Depth: l.Depth, Script: l.Script})
			}
			var ikey *secp256k1.PublicKey
			if isNUMS {
				ikey, err = address.NUMSInternalKey()
			} else {
				internal, iok := byIndex[ikIndex]
				if !iok {
					return "", errors.New("gui: taproot internal key has no @N entry")
				}
				ikey, err = address.DeriveChild(internal, index, change)
			}
			if err != nil {
				return "", err
			}
			addr, err := address.TaprootScriptPathAddress(ikey, scripts, network)
			if err != nil {
				return "", err
			}
			return addr.String(), nil
		}
	} else {
		src = func(index uint32, change bool) (string, error) {
			derived := make(map[uint8][]byte, len(byIndex))
			for i, k := range byIndex {
				pk, err := address.DeriveChild(k, index, change)
				if err != nil {
					return "", err
				}
				derived[i] = pk.SerializeCompressed()
			}
			script, err := md.EmitWitnessScriptChunks(collected, derived)
			if err != nil {
				return "", err
			}
			return address.WitnessScriptAddress(script, network)
		}
	}
	if _, err := src(0, false); err != nil {
		return nil, false
	}
	return src, true
}

// tapLeafSpecs translates md's leaf descriptions into the address package's,
// resolving each @N to its key.
//
// The two TapLeafKind enums are declared independently — package address holds
// its own copy so it never imports the codec — so this maps them with an
// explicit switch and refuses an unrecognized kind. A numeric conversion would
// compile, and would keep compiling after either enum gained or reordered a
// constant, at which point a sortedmulti_a leaf would silently emit a multi_a
// script: a valid address for the wrong policy.
func tapLeafSpecs(leaves []md.TapLeaf, byIndex map[uint8]bip380.Key) ([]address.TapLeafSpec, bool) {
	specs := make([]address.TapLeafSpec, 0, len(leaves))
	for _, l := range leaves {
		var kind address.TapLeafKind
		switch l.Kind {
		case md.TapLeafPK:
			kind = address.TapLeafPK
		case md.TapLeafMultiA:
			kind = address.TapLeafMultiA
		case md.TapLeafSortedMultiA:
			kind = address.TapLeafSortedMultiA
		default:
			return nil, false
		}
		lkeys := make([]bip380.Key, 0, len(l.KeyIndices))
		for _, i := range l.KeyIndices {
			k, ok := byIndex[i]
			if !ok {
				return nil, false
			}
			lkeys = append(lkeys, k)
		}
		specs = append(specs, address.TapLeafSpec{
			Depth: l.Depth,
			Kind:  kind,
			Keys:  lkeys,
			K:     l.K,
		})
	}
	return specs, true
}
