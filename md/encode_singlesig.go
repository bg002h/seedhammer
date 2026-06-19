package md

import "errors"

// ─── EncodeSingleSig (T6a-1) — a byte-faithful single-sig WALLET-POLICY md1 ───
//
// EncodeSingleSig builds a wallet-policy *descriptor (n=1; pubkeys + fingerprints
// TLV; an EXPLICIT path_decl.Shared origin) for one of the 4 single-sig script
// shapes and emits the CHUNKED md1 strings via the shipped split (the canonical
// payload is ~81 bytes / >320 bits, so it always chunks — ~3 strings; the
// single-string encodeMD1String path is NOT used, R0-I1). It is the headless
// wire core of T6a; the GUI caller (T6a-2) supplies the parsed key material so
// no private bytes pass through here. Mirrors the toolkit's single-sig synth
// (synthesize.rs:140-155 / cell_7_wpkh_full, wallet_policy.rs:190-204).

// PathComponent is one RAW BIP-32 origin path step: a Hardened flag plus the
// bare child Value (e.g. {Hardened:true, Value:84} for 84'). This is the
// encoder's component form — NOT the in-band +HardenedKeyStart bip32.Path
// convention used by the expand/display accessor (R0-M5: do not conflate).
type PathComponent struct {
	Hardened bool
	Value    uint32
}

var (
	errSingleSigEmptyOrigin = errors.New("md: EncodeSingleSig requires an explicit origin")
	errSingleSigBadScript   = errors.New("md: EncodeSingleSig unknown script kind")
)

// EncodeSingleSig encodes a single-sig wallet-policy md1 over the given account
// key (chainCode ‖ compressedPubkey form the 65-byte Pubkeys TLV entry), master
// fingerprint fp, explicit BIP origin (e.g. m/84'/0'/0'), and script shape. It
// returns the chunked codex32 md1 strings (>=2). An empty origin is rejected
// (the explicit origin is mandatory — BIP-49 sh(wpkh) requires it on the wire,
// and the toolkit emits it for every shape for determinism).
func EncodeSingleSig(chainCode [32]byte, compressedPubkey [33]byte, fp [4]byte, origin []PathComponent, script ScriptKind) ([]string, error) {
	if len(origin) == 0 {
		return nil, errSingleSigEmptyOrigin
	}

	// Explicit shared origin from the raw components.
	comps := make([]pathComponent, len(origin))
	for i, c := range origin {
		comps[i] = pathComponent{hardened: c.Hardened, value: c.Value}
	}
	sharedOrigin := originPath{components: comps}

	// 65-byte Pubkeys TLV payload = chainCode ‖ compressedPubkey.
	var xpub [65]byte
	copy(xpub[:32], chainCode[:])
	copy(xpub[32:], compressedPubkey[:])

	// The single-sig tree per script shape (R0-C2): 4 DISTINCT bodies.
	tree, err := singleSigTree(script)
	if err != nil {
		return nil, err
	}

	d := &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &sharedOrigin},
		// useSite = <0;1>/* — hasMultipath, alts {0},{1}, unhardened wildcard.
		useSite: useSitePath{
			hasMultipath: true,
			multipath: []alternative{
				{hardened: false, value: 0},
				{hardened: false, value: 1},
			},
			wildcardHardened: false,
		},
		tree: tree,
		tlv: tlvSection{
			pubPresent:   true,
			pubkeys:      []idxPub{{idx: 0, xpub: xpub}},
			fpPresent:    true,
			fingerprints: []idxFP{{idx: 0, fp: fp}},
		},
	}

	// Route through split, which runs encodePayload -> canonicalize (a no-op at
	// n=1, but NOT bypassed) and chunks the >320-bit payload.
	return split(d)
}

// singleSigTree returns the wallet-policy tree node for the 4 single-sig shapes,
// each referencing placeholder @0 (R0-C2):
//
//	ScriptPkh    -> node{tagPkh,  keyArgBody{0}}
//	ScriptWpkh   -> node{tagWpkh, keyArgBody{0}}
//	ScriptTr     -> node{tagTr,   trBody{isNums:false, keyIndex:0, tree:nil}}  (NOT keyArgBody)
//	ScriptShWpkh -> node{tagSh,   childrenBody{[node{tagWpkh, keyArgBody{0}}]}}
func singleSigTree(script ScriptKind) (node, error) {
	switch script {
	case ScriptPkh:
		return node{tag: tagPkh, body: keyArgBody{index: 0}}, nil
	case ScriptWpkh:
		return node{tag: tagWpkh, body: keyArgBody{index: 0}}, nil
	case ScriptTr:
		return node{tag: tagTr, body: trBody{isNums: false, keyIndex: 0, tree: nil}}, nil
	case ScriptShWpkh:
		inner := node{tag: tagWpkh, body: keyArgBody{index: 0}}
		return node{tag: tagSh, body: childrenBody{children: []node{inner}}}, nil
	default:
		return node{}, errSingleSigBadScript
	}
}
