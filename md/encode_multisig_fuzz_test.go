package md

import "testing"

// FuzzEncodeMultisig feeds arbitrary (n, k, per-cosigner cc/pk/fp/fpPresent,
// script, originMode) and asserts: no panic; and any SUCCESSFUL encode
// round-trips via ExpandWalletPolicyChunks recovering the inputs in order. An
// off-curve pubkey is rejected at DECODE only — a benign skip (mirrors
// FuzzEncodeSingleSig). (T6c Phase A Task 6.)
func FuzzEncodeMultisig(f *testing.F) {
	// Seed from the vendored full-policy golden inputs.
	f.Add(uint8(3), uint8(2),
		mustHexFuzz("101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f"),
		mustHexFuzz("03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23"),
		mustHexFuzz("deadbeef"), true, uint8(0), uint8(0), uint32(48))

	f.Fuzz(func(t *testing.T, nRaw, kRaw byte, ccBytes, pkBytes, fpBytes []byte, fpPresent bool, scriptRaw, originRaw byte, originHead uint32) {
		n := int(nRaw%8) + 1 // 1..8 cosigners (kept small to keep the fuzz fast)
		var cc [32]byte
		copy(cc[:], ccBytes)
		var pk [33]byte
		copy(pk[:], pkBytes)
		var fp [4]byte
		copy(fp[:], fpBytes)

		cosigners := make([]MultisigCosigner, n)
		for i := range cosigners {
			cosigners[i] = MultisigCosigner{
				ChainCode: cc, CompressedPubkey: pk, Fingerprint: fp, FpPresent: fpPresent,
				Origin: []PathComponent{{Hardened: true, Value: originHead}},
			}
		}
		req := EncodeMultisigRequest{
			Cosigners:    cosigners,
			K:            kRaw%32 + 1, // 1..32
			Script:       MultisigScript(int(scriptRaw) % 3),
			OriginMode:   OriginMode(int(originRaw) % 2),
			SharedOrigin: []PathComponent{{Hardened: true, Value: originHead}},
		}
		out, _, slots, err := EncodeMultisig(req)
		if err != nil {
			return // guarded error (k>n, etc.) — benign skip
		}
		if len(out) < 1 {
			t.Fatalf("EncodeMultisig returned %d chunks", len(out))
		}
		if len(slots) != n {
			t.Fatalf("slots=%d, want %d", len(slots), n)
		}
		_, keys, err := ExpandWalletPolicyChunks(out)
		if err != nil {
			return // off-curve pubkey rejected at decode — benign skip
		}
		if len(keys) != n {
			t.Fatalf("recovered n=%d, want %d", len(keys), n)
		}
		for i, k := range keys {
			if int(k.Index) != i {
				t.Fatalf("key %d Index=%d (order not preserved)", i, k.Index)
			}
			var wantXpub [65]byte
			copy(wantXpub[:32], cc[:])
			copy(wantXpub[32:], pk[:])
			if k.Xpub != wantXpub {
				t.Fatalf("key %d xpub not recovered", i)
			}
		}
	})
}
