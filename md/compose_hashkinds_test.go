package md

import (
	"bytes"
	"testing"
)

// The Go twin of descriptor-mnemonic's crates/md-codec/tests/compose_hashkinds.rs.
//
// WHY IT EXISTS. The cross-language review found that no committed Go test
// called Compose() with a hash256, ripemd160 or hash160 lock: this package's
// compose family has exactly one hash-bearing row and its digest is composeH,
// hardcoded to KindSha256. Everything else that covered the new kinds came in
// through the DECODE side (the vendored conformance corpus), so the BUILDER --
// the path the device actually drives when an operator composes a wallet -- was
// exercised at one kind out of four, in the cycle that added the other three.
//
// The corpus is the stronger check where it reaches, because it is Rust's own
// bytes. This is what it does not reach.

// composeHashKindLock is a digest of `b` repeated to the kind's own width, so a
// fixture cannot be the wrong length by construction.
func composeHashKindLock(t *testing.T, k HashKind, b byte) *HashLock {
	t.Helper()
	h, ok := NewHashLock(k, bytes.Repeat([]byte{b}, k.DigestLen()))
	if !ok {
		t.Fatalf("%s rejected %d bytes, which is its own width", k.Token(), k.DigestLen())
	}
	return h
}

// TestComposeRoundTripsEveryHashKind: a policy built at each kind survives
// Compose -> Chunks -> PolicyShapeChunks carrying the SAME kind and digest.
//
// MUTATION: make HashKind.tag() return tagSha256 for every kind -> the three
// new kinds decode as sha256 and the kind assertion fails.
// MUTATION: make the lowering emit a fixed 32-byte body -> the 20-byte kinds
// fail on digest length.
func TestComposeRoundTripsEveryHashKind(t *testing.T) {
	for _, k := range []HashKind{KindSha256, KindHash256, KindRipemd160, KindHash160} {
		t.Run(k.Token(), func(t *testing.T) {
			lock := composeHashKindLock(t, k, 0xa8)
			list := PathList{Wrapper: ComposeWsh, Paths: []SpendPath{
				{Keys: &KeySet{K: 2, N: 3, Sorted: true}, Hash: lock},
			}}
			c, err := Compose(list)
			if err != nil {
				t.Fatalf("Compose at %s: %v", k.Token(), err)
			}
			chunks, err := c.Chunks()
			if err != nil {
				t.Fatalf("Chunks at %s: %v", k.Token(), err)
			}
			shape, err := PolicyShapeChunks(chunks)
			if err != nil {
				t.Fatalf("PolicyShapeChunks at %s: %v", k.Token(), err)
			}
			var got []*HashLock
			for _, b := range shape.Branches {
				got = append(got, b.Hashlocks...)
			}
			if len(got) != 1 {
				t.Fatalf("%s: the decoded shape carries %d hashlocks, want 1", k.Token(), len(got))
			}
			if got[0].Kind() != k {
				t.Errorf("composed %s and decoded %s -- the kind did not survive the wire",
					k.Token(), got[0].Kind().Token())
			}
			if !bytes.Equal(got[0].Digest(), lock.Digest()) {
				t.Errorf("%s: digest %x, want %x", k.Token(), got[0].Digest(), lock.Digest())
			}
			if len(got[0].Digest()) != k.DigestLen() {
				t.Errorf("%s: decoded %d digest bytes, want %d -- a width that survives Equal "+
					"can still be wrong on the wire", k.Token(), len(got[0].Digest()), k.DigestLen())
			}
		})
	}
}

// TestEveryHashKindTakesItsOwnWireTag: no two kinds share a tag.
//
// A shared tag is the worst outcome this cycle can produce -- a plate engraved
// by one tool decoding as a different hash function in another -- and it is the
// one defect a per-kind round-trip alone would NOT catch, because each kind
// round-trips through its own encoder and decoder consistently.
func TestEveryHashKindTakesItsOwnWireTag(t *testing.T) {
	seen := map[tag]HashKind{}
	for _, k := range []HashKind{KindSha256, KindHash256, KindRipemd160, KindHash160} {
		tg := k.tag()
		if other, dup := seen[tg]; dup {
			t.Errorf("%s and %s share wire tag %v: a plate cut by one would decode as the other",
				k.Token(), other.Token(), tg)
		}
		seen[tg] = k
	}
	if len(seen) != 4 {
		t.Errorf("four kinds mapped to %d distinct tags", len(seen))
	}
}
