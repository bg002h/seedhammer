package md

import (
	"testing"
)

// FuzzEncodeSingleSig feeds arbitrary (chainCode, pubkey, fp, origin, script)
// bytes: a successful EncodeSingleSig must produce chunks that round-trip via
// DecodeChunks/ExpandWalletPolicyChunks recovering the inputs, with no panic.
// (T6a-1 Task 4.) The encoder does NOT validate the pubkey on-curve, so the
// decode leg may reject an off-curve point — that is a benign skip.
func FuzzEncodeSingleSig(f *testing.F) {
	// Seed with the 4 vendored shapes' inputs (derived from the goldens).
	seeds := []struct {
		cc, pk, fp string
		depth      int
		script     uint8
	}{
		{"4a53a0ab21b9dc95869c4e92a161194e03c0ef3ff5014ac692f433c4765490fc", "02707a62fdacc26ea9b63b1c197906f56ee0180d0bcf1966e1a2da34f5f3a09a9b", "73c5da0a", 3, uint8(ScriptWpkh)},
	}
	for _, s := range seeds {
		cc := mustHexFuzz(s.cc)
		pk := mustHexFuzz(s.pk)
		fp := mustHexFuzz(s.fp)
		f.Add(cc, pk, fp, uint32(84), s.script)
	}

	f.Fuzz(func(t *testing.T, ccBytes, pkBytes, fpBytes []byte, originHead uint32, scriptRaw uint8) {
		var cc [32]byte
		copy(cc[:], ccBytes)
		var pk [33]byte
		copy(pk[:], pkBytes)
		var fp [4]byte
		copy(fp[:], fpBytes)
		script := ScriptKind(scriptRaw % 6) // 6 ScriptKind values incl. ScriptShWpkh

		// A single-component origin (depth 1) is enough to satisfy the explicit
		// origin requirement and exercises the path-varint.
		origin := []PathComponent{{Hardened: true, Value: originHead}}

		chunks, err := EncodeSingleSig(cc, pk, fp, origin, script)
		if err != nil {
			return // unsupported script kind or other guarded error — benign skip
		}
		if len(chunks) < 1 {
			t.Fatalf("EncodeSingleSig returned %d chunks", len(chunks))
		}
		// Round-trip: decode + expand. Decode runs validateXpubBytes (on-curve),
		// which may reject the random pubkey — that is a benign skip.
		_, keys, err := ExpandWalletPolicyChunks(chunks)
		if err != nil {
			return
		}
		if len(keys) != 1 {
			t.Fatalf("recovered n=%d, want 1", len(keys))
		}
		k := keys[0]
		if k.Xpub != [65]byte(append(append([]byte(nil), cc[:]...), pk[:]...)) {
			t.Fatalf("xpub not recovered: got %x", k.Xpub)
		}
		if k.Fingerprint != fp {
			t.Fatalf("fp not recovered: got %x want %x", k.Fingerprint, fp)
		}
	})
}

func mustHexFuzz(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi := hexNibble(s[2*i])
		lo := hexNibble(s[2*i+1])
		out[i] = hi<<4 | lo
	}
	return out
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// FuzzWalletPolicyId feeds arbitrary bytes as a candidate canonical payload: if
// they decode to a valid descriptor, WalletPolicyId must not panic, must be
// deterministic, and must differ from computeEncodingID's preimage hash in
// general (they are distinct preimages; we only assert no-panic + determinism
// here, since a collision, while astronomically unlikely, is not a bug).
func FuzzWalletPolicyId(f *testing.F) {
	for _, name := range byteParityVectorNames {
		if raw, err := readFileBytes(name); err == nil {
			f.Add(raw)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := decodePayloadValidated(b, len(b)*8)
		if err != nil {
			return
		}
		id1, err := WalletPolicyId(d)
		if err != nil {
			t.Fatalf("WalletPolicyId of decoded descriptor failed: %v", err)
		}
		id2, err := WalletPolicyId(d)
		if err != nil {
			t.Fatalf("WalletPolicyId second call failed: %v", err)
		}
		if id1 != id2 {
			t.Fatalf("WalletPolicyId non-deterministic: %x vs %x", id1, id2)
		}
		// computeEncodingID must also not panic on the same descriptor.
		if _, err := computeEncodingID(d); err != nil {
			t.Fatalf("computeEncodingID failed: %v", err)
		}
	})
}
