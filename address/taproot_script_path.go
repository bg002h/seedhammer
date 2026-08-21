package address

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"seedhammer.com/bip380"
)

// Taproot SCRIPT-PATH address derivation (Stage 3).
//
// Until this existed, `address.go` derived taproot key-path only, via
// `ComputeTaprootKeyNoScript` — so any `tr()` carrying a script tree got a typed
// `errUnsupported` and the device could show no receive or change address for
// it. That is the gap that made the project's own proof model ("derived
// addresses + wallet id") unreachable for exactly the policies this cycle is
// about.
//
// THE DELTA REALLY IS SMALL, and the plan said so: `ComputeTaprootKeyNoScript`
// is a one-line wrapper over the fully general
// `ComputeTaprootOutputKey(internalKey, scriptRoot)`. Everything else here is
// building a correct `scriptRoot`.
//
// THE TREE SHAPE IS LOAD-BEARING. {{A,B},C} and {A,{B,C}} hold the same leaves
// and commit to DIFFERENT output keys, so leaves arrive with their depths and
// are rebuilt in that exact shape. `txscript.AssembleTaprootScriptTree` is
// deliberately NOT used: it builds a Huffman layout of its own choosing, which
// would silently produce a different — and wrong — address for our trees.

// LeafScript is one tapscript leaf: its depth in the tree, and its script bytes.
type LeafScript struct {
	Depth  int
	Script []byte
}

// TapLeafKind mirrors md.TapLeafKind without creating a dependency edge: this
// package derives keys and must not import the codec.
type TapLeafKind uint8

const (
	// TapLeafPK — `pk(K)`.
	TapLeafPK TapLeafKind = iota
	// TapLeafMultiA — `multi_a(k,...)`, written key order preserved.
	TapLeafMultiA
	// TapLeafSortedMultiA — `sortedmulti_a(k,...)`, keys sorted by DERIVED key.
	TapLeafSortedMultiA
)

// TapLeafSpec is one leaf described by the keys it references, before
// derivation. The caller supplies `bip380.Key`s because THIS package owns child
// derivation — handing it pre-derived keys would put the use-site path in two
// places, and a use-site applied twice or not at all is a wrong address.
type TapLeafSpec struct {
	Depth int
	Kind  TapLeafKind
	Keys  []bip380.Key
	K     int
}

// TaprootScriptPath derives the P2TR address for a taproot policy WITH a script
// tree, at one chain and index.
//
// This is the entry point Stage 3 adds. `Receive`/`Change` above cover
// key-path-only taproot via the flat `bip380.Descriptor`, which cannot express
// a tree at all.
func TaprootScriptPath(
	internal bip380.Key,
	leaves []TapLeafSpec,
	index uint32,
	change bool,
	network *chaincfg.Params,
) (string, error) {
	ikey, err := derivePubKey(internal, index, change)
	if err != nil {
		return "", err
	}
	return taprootScriptPathAt(ikey, leaves, index, change, network)
}

// NUMSInternalKey is BIP-341's "no known discrete logarithm" point H, used as
// the internal key by a taproot policy with NO key-path spend. md1 carries it as
// a single `is_nums` bit rather than 32 bytes (SPEC §7), so the point itself has
// to live on this side.
//
// The x coordinate is BIP-341's, and the even-Y lift is what `lift_x` means
// there. Getting this wrong does not fail loudly: it yields a perfectly
// well-formed address for a different, WRONG output key.
var numsXOnly = [32]byte{
	0x50, 0x92, 0x9b, 0x74, 0xc1, 0xa0, 0x49, 0x54,
	0xb7, 0x8b, 0x4b, 0x60, 0x35, 0xe9, 0x7a, 0x5e,
	0x07, 0x8a, 0x5a, 0x0f, 0x28, 0xec, 0x96, 0xd5,
	0x47, 0xbf, 0xee, 0x9a, 0xce, 0x80, 0x3a, 0xc0,
}

// NUMSInternalKey returns the BIP-341 unspendable internal key H.
func NUMSInternalKey() (*secp256k1.PublicKey, error) {
	return schnorr.ParsePubKey(numsXOnly[:])
}

// TaprootScriptPathNUMS derives the P2TR address for a taproot policy whose
// internal key is the NUMS point — script paths only, no key-path spend.
//
// Separate from TaprootScriptPath because there is no `bip380.Key` to derive
// from: the internal key is a fixed constant and does NOT move with the address
// index. The leaves still do, which is why index/change are still parameters.
func TaprootScriptPathNUMS(
	leaves []TapLeafSpec,
	index uint32,
	change bool,
	network *chaincfg.Params,
) (string, error) {
	ikey, err := NUMSInternalKey()
	if err != nil {
		return "", err
	}
	return taprootScriptPathAt(ikey, leaves, index, change, network)
}

// taprootScriptPathAt emits every leaf script at (index, change) and wraps the
// internal key around the resulting tree. Shared by the keyed and NUMS entry
// points so the leaf-emission rules — including sortedmulti_a's sort on DERIVED
// keys — exist once.
func taprootScriptPathAt(
	ikey *secp256k1.PublicKey,
	leaves []TapLeafSpec,
	index uint32,
	change bool,
	network *chaincfg.Params,
) (string, error) {
	scripts := make([]LeafScript, 0, len(leaves))
	for _, l := range leaves {
		derived := make([]*secp256k1.PublicKey, 0, len(l.Keys))
		for _, k := range l.Keys {
			pk, err := derivePubKey(k, index, change)
			if err != nil {
				return "", err
			}
			derived = append(derived, pk)
		}
		var script []byte
		var err error
		switch l.Kind {
		case TapLeafPK:
			if len(derived) != 1 {
				return "", errors.New("address: pk leaf must reference exactly one key")
			}
			script, err = PkLeafScript(derived[0])
		case TapLeafMultiA:
			script, err = MultiALeafScript(l.K, derived, false)
		case TapLeafSortedMultiA:
			script, err = MultiALeafScript(l.K, derived, true)
		default:
			return "", errors.New("address: unsupported tap leaf kind")
		}
		if err != nil {
			return "", err
		}
		scripts = append(scripts, LeafScript{Depth: l.Depth, Script: script})
	}
	addr, err := TaprootScriptPathAddress(ikey, scripts, network)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

var errTapTreeUnbalanced = errors.New("address: tap leaf depths do not form a binary tree")

// TaprootScriptPathAddress computes the P2TR address for an internal key
// committed to a script tree.
//
// `leaves` must be in DEPTH-FIRST order with their depths — the representation
// md.TapLeavesChunks returns.
func TaprootScriptPathAddress(
	internal *secp256k1.PublicKey,
	leaves []LeafScript,
	network *chaincfg.Params,
) (address.Address, error) {
	if len(leaves) == 0 {
		return nil, errors.New("address: taproot script path with no leaves")
	}
	root, err := tapTreeRoot(leaves)
	if err != nil {
		return nil, err
	}
	out := txscript.ComputeTaprootOutputKey(internal, root[:])
	return address.NewAddressTaproot(schnorr.SerializePubKey(out), network)
}

// tapTreeRoot rebuilds the tree from (depth, script) pairs and returns its root
// hash.
//
// THE ALGORITHM IS A STACK OF (depth, node), and it is the same one the md1
// wire format's own tree reader uses: push each leaf, then while the top two
// entries share a depth, combine them into a branch one level shallower. A
// well-formed binary tree collapses to exactly one node at depth 0.
//
// Anything that does NOT collapse is refused rather than patched up — an
// unbalanced list means the caller handed us something that is not the tree it
// thinks it is, and guessing a shape here would produce a plausible address for
// the wrong script.
func tapTreeRoot(leaves []LeafScript) (chainhash.Hash, error) {
	type entry struct {
		depth int
		node  txscript.TapNode
	}
	var stack []entry
	for _, l := range leaves {
		stack = append(stack, entry{
			depth: l.Depth,
			node:  txscript.NewBaseTapLeaf(l.Script),
		})
		for len(stack) >= 2 {
			a, b := stack[len(stack)-2], stack[len(stack)-1]
			if a.depth != b.depth {
				break
			}
			branch := txscript.NewTapBranch(a.node, b.node)
			stack = stack[:len(stack)-2]
			stack = append(stack, entry{depth: a.depth - 1, node: branch})
		}
	}
	if len(stack) != 1 {
		return chainhash.Hash{}, errTapTreeUnbalanced
	}
	// A single leaf sits at depth 0 (the wire's single-leaf form); a built tree
	// collapses to depth 0 too. Any other residue is the same shape error.
	if stack[0].depth != 0 {
		return chainhash.Hash{}, errTapTreeUnbalanced
	}
	return stack[0].node.TapHash(), nil
}

// PkLeafScript emits `pk(K)` as a tapscript: <32-byte x-only key> OP_CHECKSIG.
func PkLeafScript(key *secp256k1.PublicKey) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(key)).
		AddOp(txscript.OP_CHECKSIG).
		Script()
}

// MultiALeafScript emits `multi_a(k, keys...)` / `sortedmulti_a(k, keys...)` as
// the tapscript CHECKSIGADD form:
//
//	<key0> OP_CHECKSIG <key1> OP_CHECKSIGADD ... <keyN> OP_CHECKSIGADD <k> OP_NUMEQUAL
//
// This is the builder the plan named as the one genuine blocker — 2 of 36 tags
// (`multi_a`, `sortedmulti_a`) could not be emitted without it.
//
// `sorted` applies BIP-387's lexicographic ordering to the SERIALIZED x-only
// keys. It must happen on the derived child keys, which is why it is here and
// not in the codec: sorting the `@N` placeholders would sort the wrong things.
func MultiALeafScript(k int, keys []*secp256k1.PublicKey, sorted bool) ([]byte, error) {
	if k <= 0 || k > len(keys) {
		return nil, errors.New("address: multi_a threshold out of range")
	}
	ser := make([][]byte, len(keys))
	for i, key := range keys {
		ser[i] = schnorr.SerializePubKey(key)
	}
	if sorted {
		sort.Slice(ser, func(i, j int) bool { return bytes.Compare(ser[i], ser[j]) < 0 })
	}
	b := txscript.NewScriptBuilder()
	for i, s := range ser {
		b.AddData(s)
		if i == 0 {
			b.AddOp(txscript.OP_CHECKSIG)
		} else {
			b.AddOp(txscript.OP_CHECKSIGADD)
		}
	}
	b.AddInt64(int64(k))
	b.AddOp(txscript.OP_NUMEQUAL)
	return b.Script()
}

// DeriveChild exposes this package's child-key derivation.
//
// It is exported because the segwit-v0 witness-script emitter lives in package
// `md` (the only package holding the decoded AST) and needs DERIVED keys handed
// to it. The alternative — deriving inside the codec — would put the use-site
// path in two places, and a use-site applied twice or not at all is a wrong
// address. One rule, one implementation, called from wherever it is needed.
func DeriveChild(k bip380.Key, index uint32, change bool) (*secp256k1.PublicKey, error) {
	return derivePubKey(k, index, change)
}

// WitnessScriptAddress wraps a segwit-v0 witness script into its P2WSH address.
//
// Pairing this with `md.EmitWitnessScriptChunks` makes the address a FUNCTION of
// the script bytes: P2WSH commits to sha256(script), so agreeing with another
// implementation on the address means agreeing on every opcode. That is why the
// emitter's tests assert addresses rather than script hex — a script diff can be
// argued about, an address cannot.
func WitnessScriptAddress(script []byte, network *chaincfg.Params) (string, error) {
	h := sha256.Sum256(script)
	addr, err := address.NewAddressWitnessScriptHash(h[:], network)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}
