package md

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	btcaddr "github.com/btcsuite/btcd/address/v2"
)

// Witness-script emission for segwit v0 (Stage 3).
//
// WHY THIS LIVES IN `md`. Emitting a script is a walk of the decoded AST, and
// `md` is the only package that has one — `Template` is a flat summary and the
// tree is unexported. The alternative was leaking the AST, which would make
// every consumer a second implementation of these rules.
//
// IT STILL DOES NO KEY DERIVATION. The caller passes DERIVED public keys, one
// per `@N`. Derivation needs the use-site path, which lives in the address
// layer; doing it here would put that rule in two places, and a use-site applied
// twice or not at all is a wrong address.
//
// SCOPE IS A SUBSET, AND SAYS SO. The fragments below are exactly those the
// pathological journey's own wsh() policy uses — or_i, and_v, the `v:` wrapper,
// after, older, sha256, multi — plus the key forms. Anything else returns
// ErrScriptUnsupported rather than an approximation: a script that is nearly
// right commits funds to an address nobody can spend from.

// Opcodes used by the emitted subset.
const (
	opDUP            = 0x76
	opEQUAL          = 0x87
	opEQUALVERIFY    = 0x88
	opVERIFY         = 0x69
	opIF             = 0x63
	opELSE           = 0x67
	opENDIF          = 0x68
	opSIZE           = 0x82
	opSHA256         = 0xa8
	opHASH160        = 0xa9
	opHASH256        = 0xaa
	opRIPEMD160      = 0xa6
	opCHECKSIG       = 0xac
	opCHECKSIGVERIFY = 0xad
	opCHECKMULTISIG  = 0xae
	// BIP-342 tapscript. OP_CHECKSIGADD reuses OP_SUCCESS186's slot, which is
	// why it exists only under taproot; OP_NUMEQUAL is ordinary but is the
	// terminator multi_a uses in place of CHECKMULTISIG's implicit compare.
	opCHECKSIGADD      = 0xba
	opNUMEQUAL         = 0x9c
	opCHECKMULTISIGVER = 0xaf
	opCSV              = 0xb2 // OP_CHECKSEQUENCEVERIFY
	opCLTV             = 0xb1 // OP_CHECKLOCKTIMEVERIFY
	opDROP             = 0x75
	opBOOLAND          = 0x9a
	opBOOLOR           = 0x9b
	opNOTIF            = 0x64
	opIFDUP            = 0x73
	opTOALTSTACK       = 0x6b
	opFROMALTSTACK     = 0x6c
	opADD              = 0x93
	op0NOTEQUAL        = 0x92
	opSWAP             = 0x7c
	op1Negate          = 0x4f
	op0                = 0x00
)

// ErrScriptUnsupported marks a fragment this emitter cannot build, so a caller
// can say "not supported yet" rather than "invalid policy".
var ErrScriptUnsupported = errors.New("md: script fragment not supported for emission")

// emitEnv is the emission context: the derived keys, and WHICH SCRIPT LANGUAGE
// is being emitted.
//
// One fragment walker serves both, because segwit-v0 witness scripts and
// tapscript leaves are the same miniscript over different primitives. Only two
// things actually differ, and both are in `tap`:
//
//   - KEY WIDTH. `keys` holds 33-byte compressed points for segwit v0 and
//     32-byte x-only points for tapscript. The caller derives, so this walker
//     just pushes what it is given — but the two must never be mixed, since a
//     33-byte push in a tapscript leaf is a valid script for the wrong key.
//   - THE MULTISIG OPCODE. BIP-342 DISABLED `OP_CHECKMULTISIG` in tapscript and
//     replaced it with `multi_a`'s CHECKSIGADD accumulation. So `multi` is
//     segwit-v0-only and `multi_a` is tapscript-only, and each is REFUSED in
//     the other context rather than silently translated: a policy asking for
//     CHECKMULTISIG under taproot is not a policy this device can honour, and
//     emitting the "equivalent" would commit funds to a script nobody wrote.
type emitEnv struct {
	keys map[uint8][]byte
	tap  bool
}

// EmitWitnessScriptChunks decodes an md1 chunk set and emits the segwit-v0
// witness script for its `wsh(...)` body, using the supplied derived keys.
//
// `keys` maps each `@N` to its DERIVED 33-byte compressed public key.
func EmitWitnessScriptChunks(strs []string, keys map[uint8][]byte) ([]byte, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return nil, err
	}
	if d.tree.tag != tagWsh {
		return nil, ErrScriptUnsupported
	}
	b, ok := d.tree.body.(childrenBody)
	if !ok || len(b.children) != 1 {
		return nil, ErrScriptUnsupported
	}
	var out []byte
	if err := emitFragment(b.children[0], emitEnv{keys: keys}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func pushData(out *[]byte, data []byte) {
	switch n := len(data); {
	case n < 0x4c:
		*out = append(*out, byte(n))
	case n <= 0xff:
		*out = append(*out, 0x4c, byte(n))
	default:
		*out = append(*out, 0x4d, byte(n), byte(n>>8))
	}
	*out = append(*out, data...)
}

// pushNumber emits a script number the way miniscript does: small values as
// OP_1..OP_16, otherwise a minimally-encoded little-endian push.
func pushNumber(out *[]byte, v uint32) {
	if v == 0 {
		*out = append(*out, op0)
		return
	}
	if v <= 16 {
		*out = append(*out, byte(0x50+v))
		return
	}
	var buf [5]byte
	binary.LittleEndian.PutUint32(buf[:4], v)
	n := 4
	for n > 0 && buf[n-1] == 0 {
		n--
	}
	// A leading byte with the high bit set would read as negative, so pad.
	if n > 0 && buf[n-1]&0x80 != 0 {
		buf[n] = 0
		n++
	}
	pushData(out, buf[:n])
}

// emitFragment appends one fragment's script.
func emitFragment(n node, e emitEnv, out *[]byte) error {
	switch n.tag {
	case tagPkK:
		// A BARE `PkK` CARRIES AN IMPLICIT `c:` — it emits OP_CHECKSIG too.
		//
		// The md1 wire deliberately stores a bare key at every key-check
		// position and reserves `Tag::Check` for wrapping NON-key children
		// (SPEC §5.1 Q12); the renderer reconstructs the `pk(K)` shorthand from
		// it. So the wire's `PkK` is miniscript's `c:pk_k`, and emitting only
		// the push produces a script that is missing its signature check.
		//
		// This was invisible until a vector contained a bare `pk()`: the
		// timelock/hashlock vector is all `multi` and hash/timelock fragments,
		// so it passed with the push-only version. `keyed_wsh_or_b` caught it.
		b, ok := n.body.(keyArgBody)
		if !ok {
			return ErrScriptUnsupported
		}
		k, ok := e.keys[b.index]
		if !ok {
			return ErrScriptUnsupported
		}
		pushData(out, k)
		*out = append(*out, opCHECKSIG)
		return nil

	case tagPkH:
		// `pkh(K)` on the wire is miniscript's `c:pk_h(K)`, the same implicit
		// `c:` as PkK above (SPEC §5.1 Q12), so the arm emits the whole
		// DUP HASH160 <hash160(K)> EQUALVERIFY CHECKSIG. The key is hashed AS
		// SUPPLIED: 33-byte compressed in segwit-v0, 32-byte x-only under tap
		// (BIP-342's pk_h hashes the x-only key), exactly as PkK pushes it.
		//
		// The composer is the first producer of this fragment on this device
		// (Multisig Build only ever wrote sortedmulti); md/compose_pkh_emit_test.go
		// pins it to the Rust primary's addresses for five vectors.
		b, ok := n.body.(keyArgBody)
		if !ok {
			return ErrScriptUnsupported
		}
		k, ok := e.keys[b.index]
		if !ok {
			return ErrScriptUnsupported
		}
		*out = append(*out, opDUP, opHASH160)
		pushData(out, btcaddr.Hash160(k))
		*out = append(*out, opEQUALVERIFY, opCHECKSIG)
		return nil

	case tagCheck:
		// `c:X` — X then OP_CHECKSIG.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opCHECKSIG)
		return nil

	case tagVerify:
		// `v:X` — THE SUBTLE ONE. It does not append OP_VERIFY blindly: if the
		// wrapped fragment ends in a verifiable opcode, that opcode is REPLACED
		// by its VERIFY form. Appending instead produces a longer script that
		// still parses and hashes to a different address.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		before := len(*out)
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		if len(*out) == before {
			return ErrScriptUnsupported
		}
		switch (*out)[len(*out)-1] {
		case opCHECKSIG:
			(*out)[len(*out)-1] = opCHECKSIGVERIFY
		case opCHECKMULTISIG:
			(*out)[len(*out)-1] = opCHECKMULTISIGVER
		case opEQUAL:
			(*out)[len(*out)-1] = opEQUALVERIFY
		default:
			*out = append(*out, opVERIFY)
		}
		return nil

	case tagAndV:
		// `and_v(X,Y)` — concatenation, nothing more.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		return emitFragment(c.children[1], e, out)

	case tagOrI:
		// `or_i(X,Y)` — OP_IF X OP_ELSE Y OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opIF)
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opELSE)
		if err := emitFragment(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagAndB:
		// `and_b(X,Y)` — X Y OP_BOOLAND.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		if err := emitFragment(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opBOOLAND)
		return nil

	case tagOrB:
		// `or_b(X,Z)` — X Z OP_BOOLOR.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		if err := emitFragment(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opBOOLOR)
		return nil

	case tagOrC:
		// `or_c(X,Z)` — X OP_NOTIF Z OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opNOTIF)
		if err := emitFragment(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagOrD:
		// `or_d(X,Z)` — X OP_IFDUP OP_NOTIF Z OP_ENDIF.
		//
		// The OP_IFDUP is what distinguishes it from or_c and is easy to drop:
		// without it the script still parses and hashes elsewhere. It is the
		// degrading-multisig idiom's fragment, so it earns its own vector.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opIFDUP, opNOTIF)
		if err := emitFragment(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagThresh:
		// `thresh(k,X1,...,Xn)` — X1 (Xi OP_ADD)... <k> OP_EQUAL.
		//
		// The threshold is over SUB-POLICIES, not keys, which is why this is
		// not multi: each branch contributes 0 or 1 and the sum is compared.
		b, ok := n.body.(variableBody)
		if !ok || len(b.children) == 0 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(b.children[0], e, out); err != nil {
			return err
		}
		for _, c := range b.children[1:] {
			if err := emitFragment(c, e, out); err != nil {
				return err
			}
			*out = append(*out, opADD)
		}
		pushNumber(out, uint32(b.k))
		*out = append(*out, opEQUAL)
		return nil

	case tagSwap:
		// `s:X` — OP_SWAP X.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opSWAP)
		return emitFragment(c.children[0], e, out)

	case tagAlt:
		// `a:X` — OP_TOALTSTACK X OP_FROMALTSTACK.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opTOALTSTACK)
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opFROMALTSTACK)
		return nil

	case tagDupIf:
		// `d:X` — OP_DUP OP_IF X OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opDUP, opIF)
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagZeroNotEqual:
		// `n:X` — X OP_0NOTEQUAL.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, op0NOTEQUAL)
		return nil

	case tagNonZero:
		// `j:X` — OP_SIZE OP_0NOTEQUAL OP_IF X OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opSIZE, op0NOTEQUAL, opIF)
		if err := emitFragment(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagTrue:
		*out = append(*out, 0x51) // OP_1
		return nil

	case tagFalse:
		*out = append(*out, op0)
		return nil

	case tagOlder:
		v, ok := n.body.(timelockBody)
		if !ok {
			return ErrScriptUnsupported
		}
		pushNumber(out, uint32(v))
		*out = append(*out, opCSV)
		return nil

	case tagAfter:
		v, ok := n.body.(timelockBody)
		if !ok {
			return ErrScriptUnsupported
		}
		pushNumber(out, uint32(v))
		*out = append(*out, opCLTV)
		return nil

	case tagSha256, tagHash256:
		h, ok := n.body.(hash256Body)
		if !ok {
			return ErrScriptUnsupported
		}
		op := byte(opSHA256)
		if n.tag == tagHash256 {
			op = opHASH256
		}
		// OP_SIZE <32> OP_EQUALVERIFY <hashop> <h> OP_EQUAL
		*out = append(*out, opSIZE)
		pushNumber(out, 32)
		*out = append(*out, opEQUALVERIFY, op)
		pushData(out, h[:])
		*out = append(*out, opEQUAL)
		return nil

	case tagRipemd160, tagHash160:
		h, ok := n.body.(hash160Body)
		if !ok {
			return ErrScriptUnsupported
		}
		op := byte(opRIPEMD160)
		if n.tag == tagHash160 {
			op = opHASH160
		}
		*out = append(*out, opSIZE)
		pushNumber(out, 32)
		*out = append(*out, opEQUALVERIFY, op)
		pushData(out, h[:])
		*out = append(*out, opEQUAL)
		return nil

	case tagMulti, tagSortedMulti:
		// BIP-342 disabled OP_CHECKMULTISIG in tapscript. A `multi()` inside a
		// tap leaf has no emission, and inventing the multi_a equivalent would
		// change the policy.
		if e.tap {
			return ErrScriptUnsupported
		}
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return ErrScriptUnsupported
		}
		ks := make([][]byte, 0, len(b.indices))
		for _, i := range b.indices {
			k, ok := e.keys[i]
			if !ok {
				return ErrScriptUnsupported
			}
			ks = append(ks, k)
		}
		if n.tag == tagSortedMulti {
			sortByteSlices(ks)
		}
		pushNumber(out, uint32(b.k))
		for _, k := range ks {
			pushData(out, k)
		}
		pushNumber(out, uint32(len(ks)))
		*out = append(*out, opCHECKMULTISIG)
		return nil

	case tagMultiA, tagSortedMultiA:
		// `multi_a(k, ...)` — BIP-342's replacement for CHECKMULTISIG:
		//
		//	<k0> OP_CHECKSIG <k1> OP_CHECKSIGADD … <kn-1> OP_CHECKSIGADD <k> OP_NUMEQUAL
		//
		// The first key CHECKSIGs to seed the accumulator; every later one adds
		// to it. Tapscript-only, for the same reason `multi` is not.
		if !e.tap {
			return ErrScriptUnsupported
		}
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return ErrScriptUnsupported
		}
		ks := make([][]byte, 0, len(b.indices))
		for _, i := range b.indices {
			k, ok := e.keys[i]
			if !ok {
				return ErrScriptUnsupported
			}
			ks = append(ks, k)
		}
		// sortedmulti_a sorts the DERIVED keys, so the order depends on the
		// address index — it cannot be precomputed from the template.
		if n.tag == tagSortedMultiA {
			sortByteSlices(ks)
		}
		for i, k := range ks {
			pushData(out, k)
			if i == 0 {
				*out = append(*out, opCHECKSIG)
			} else {
				*out = append(*out, opCHECKSIGADD)
			}
		}
		pushNumber(out, uint32(b.k))
		*out = append(*out, opNUMEQUAL)
		return nil

	default:
		return ErrScriptUnsupported
	}
}

// sortByteSlices applies BIP-67 lexicographic ordering to serialized keys.
func sortByteSlices(ks [][]byte) {
	sort.Slice(ks, func(i, j int) bool { return bytes.Compare(ks[i], ks[j]) < 0 })
}
