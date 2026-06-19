// package address derives recieve and change addresses from
// output descriptors.
package address

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"seedhammer.com/bip380"
)

func Change(desc *bip380.Descriptor, index uint32) (string, error) {
	return addressAt(desc, index, true)
}

func Receive(desc *bip380.Descriptor, index uint32) (string, error) {
	return addressAt(desc, index, false)
}

func Supported(desc *bip380.Descriptor) bool {
	_, err := Receive(desc, 0)
	return !errors.Is(err, errUnsupported)
}

var errUnsupported = errors.New("unsupported descriptor")

// Errors returned by Find. ErrUnsupported is the exported counterpart to the
// internal errUnsupported, returned for a keyless descriptor (which addressAt
// would otherwise panic on).
var (
	ErrUnsupported      = errors.New("address: unsupported descriptor")
	ErrAddrUnparseable  = errors.New("address: candidate is not a valid address")
	ErrAddrWrongNetwork = errors.New("address: candidate is for a different network")
)

// addrFindMaxGap bounds the per-chain gap scan (mirrors gui.addrMaxIndex+1;
// defined here because package address cannot import package gui).
const addrFindMaxGap uint32 = 50

// Find scans the descriptor's receive then change ranges [0,gap) for an address
// equal to candidate. chain is 0 (receive) or 1 (change). Panic-safe / total:
// returns a typed error (never panics) for a keyless/unsupported descriptor, an
// unparseable candidate, or a wrong-network candidate; propagates any per-index
// derivation error rather than masking it as a non-match.
func Find(desc *bip380.Descriptor, candidate string, gap uint32) (chain int, index uint32, found bool, err error) {
	if len(desc.Keys) == 0 { // R0-I1: guard before desc.Keys[0]/Supported (both panic on keyless).
		return 0, 0, false, ErrUnsupported
	}
	if gap == 0 || gap > addrFindMaxGap {
		gap = addrFindMaxGap
	}
	net := desc.Keys[0].Network
	// NOTE: within package `address`, the btcd parser github.com/btcsuite/btcd/address/v2
	// is imported under its own name `address`, so this is `address.DecodeAddress`
	// (a bare `DecodeAddress` is undefined here — R1-M1).
	want, derr := address.DecodeAddress(candidate, net)
	if derr != nil {
		return 0, 0, false, ErrAddrUnparseable
	}
	if !want.IsForNet(net) {
		return 0, 0, false, ErrAddrWrongNetwork
	}
	wantStr := want.String()
	for i := uint32(0); i < gap; i++ {
		got, e := Receive(desc, i) // R0-I2: propagate, don't compare "" silently.
		if e != nil {
			return 0, 0, false, e
		}
		if got == wantStr {
			return 0, i, true, nil
		}
	}
	for i := uint32(0); i < gap; i++ {
		got, e := Change(desc, i)
		if e != nil {
			return 0, 0, false, e
		}
		if got == wantStr {
			return 1, i, true, nil
		}
	}
	return 0, 0, false, nil
}

func addressAt(desc *bip380.Descriptor, index uint32, change bool) (string, error) {
	var addr address.Address
	var network *chaincfg.Params
	switch desc.Type {
	case bip380.SortedMulti:
		var keys []*address.AddressPubKey
		for _, k := range desc.Keys {
			pub, err := derivePubKey(k, index, change)
			if err != nil {
				return "", fmt.Errorf("address: %w", err)
			}
			if network != nil && k.Network != network {
				return "", fmt.Errorf("address: multisig descriptor mixes networks: %w", errUnsupported)
			}
			network = k.Network
			addrPub, err := address.NewAddressPubKey(pub.SerializeCompressed(), network)
			if err != nil {
				return "", fmt.Errorf("address: %w", err)
			}
			keys = append(keys, addrPub)
		}
		slices.SortFunc(keys, func(addr1, addr2 *address.AddressPubKey) int {
			return bytes.Compare(addr1.PubKey().SerializeCompressed(), addr2.PubKey().SerializeCompressed())
		})
		script, err := txscript.MultiSigScript(keys, desc.Threshold)
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
		switch desc.Script {
		case bip380.P2SH:
			addr, err = address.NewAddressScriptHash(script, network)
		case bip380.P2WSH, bip380.P2SH_P2WSH:
			hash := sha256.Sum256(script)
			addr, err = address.NewAddressWitnessScriptHash(hash[:], network)
		default:
			return "", fmt.Errorf("address: multisig script: %s: %w", desc.Script, errUnsupported)
		}
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
	case bip380.Singlesig:
		k := desc.Keys[0]
		network = k.Network
		pub, err := derivePubKey(k, index, change)
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
		switch desc.Script {
		case bip380.P2PKH:
			pkHash := address.Hash160(pub.SerializeCompressed())
			addr, err = address.NewAddressPubKeyHash(pkHash, network)
		case bip380.P2WPKH, bip380.P2SH_P2WPKH:
			pkHash := address.Hash160(pub.SerializeCompressed())
			addr, err = address.NewAddressWitnessPubKeyHash(pkHash, network)
		case bip380.P2TR:
			tkey := txscript.ComputeTaprootKeyNoScript(pub)
			addr, err = address.NewAddressTaproot(schnorr.SerializePubKey(tkey), network)
		default:
			return "", fmt.Errorf("address: singlesig script: %s: %w", desc.Script, errUnsupported)
		}
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
	default:
		return "", fmt.Errorf("address: descriptor: %w", errUnsupported)
	}
	// Derive wrapped address types.
	switch desc.Script {
	case bip380.P2SH_P2WPKH, bip380.P2SH_P2WSH:
		script, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
		addr, err = address.NewAddressScriptHash(script, network)
		if err != nil {
			return "", fmt.Errorf("address: %w", err)
		}
	}
	return addr.String(), nil
}

func derivePubKey(k bip380.Key, index uint32, change bool) (*secp256k1.PublicKey, error) {
	children := k.Children
	if len(children) == 0 {
		// Default to <0;1>/*.
		children = append(children,
			bip380.Derivation{
				Type:  bip380.RangeDerivation,
				Index: 0,
				End:   1,
			},
			bip380.Derivation{
				Type: bip380.WildcardDerivation,
			},
		)
	}
	xpub := k.ExtendedKey()
	for _, c := range children {
		var id uint32
		switch c.Type {
		case bip380.ChildDerivation:
			id = c.Index
		case bip380.RangeDerivation:
			if c.End != c.Index+1 {
				return nil, errors.New("unsupported range path element")
			}
			id = c.Index
			if change {
				id = c.End
			}
		case bip380.WildcardDerivation:
			id = index
		default:
			return nil, errors.New("unsupported path element")
		}
		child, err := xpub.Derive(id)
		if err != nil {
			return nil, err
		}
		xpub = child
	}
	return xpub.ECPubKey()
}
