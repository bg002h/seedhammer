package md

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── F-531: the repeated-SEAT fixture, and its generator ─────────────────────
//
// No vendored vector carries this shape, and F-529 says a re-vendor from the
// primary would remove even the three key-reuse vectors that do exist. So the
// fixture is fork-native: the bytes live in md/testdata/forkbuilt/, and the
// test below REGENERATES them from the tree and fails if they have drifted.
//
// THE GENERATOR IS THE POINT. A fixture whose reproduction path has rotted is a
// fixture nobody can extend -- and this one cannot be produced by any shipped
// encoder, because EncodeMultisig emits seats [0..n-1] and never repeats one.
// Reaching the shape at all needs `split` over a hand-built tree, which is
// package-internal, which is why the artifact is a file and not a helper the
// gui tests could call.
//
// WHAT THE SHAPE IS: wsh(sortedmulti(1,@0,@0,@1)) -- THREE seats over TWO key
// slots. That is "one slot in two seats", the distinct sibling of F-218's "one
// key in two slots" (two slots that happen to declare the same xpub). The
// device admits it on the supply path; the flat address route projected it to
// one key per slot while keeping K, and so derived a 1-of-2's address under a
// 1-of-3's label (F-531).

const dupSeatFixtureDir = "forkbuilt"

// dupSeatVectors are the fork-built repeated-seat policies, by file base name.
var dupSeatVectors = []struct {
	name    string
	k       uint8
	indices []uint8
	// keyless drops the Pubkeys TLV, leaving a TEMPLATE that declares the
	// slots and carries a key for none of them. Review I-2: this form passes
	// TemplateEngraveShapeGuardChunks and reaches the consent screen one
	// confirm from steel, so it needs a fixture of its own -- the keyed one
	// exercises a different arm of every screen.
	keyless bool
	why     string
}{
	{
		name: "dup_seat_wsh_sortedmulti_k1", k: 1, indices: []uint8{0, 0, 1},
		why: "1-of-3 over two slots: the label says 1-of-3, the projection said 1-of-2",
	},
	{
		name: "dup_seat_wsh_sortedmulti_k2", k: 2, indices: []uint8{0, 0, 1},
		why: "2-of-3 over two slots: k >= 2, where the fewer-keys sentence is true",
	},
	{
		name: "dup_seat_wsh_sortedmulti_k1_keyless", k: 1, indices: []uint8{0, 0, 1},
		keyless: true,
		why:     "the same shape with no keys: engravable as a template, and silent about the reuse until review I-2",
	},
}

// dupSeatDescriptor builds the repeated-seat descriptor for one vector.
//
// The two cosigner keys are the encode_multisig test constants, so the fixture
// shares its key material with the rest of this package rather than inventing
// more of it.
func dupSeatDescriptor(t *testing.T, k uint8, indices []uint8, keyless bool) *descriptor {
	t.Helper()
	cc, pk := mkXpub65(t,
		"101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f",
		"03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23")
	cc2, pk2 := mkXpub65(t,
		"303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f",
		"02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	base, _, _, err := EncodeMultisig(EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{ChainCode: cc, CompressedPubkey: pk, Fingerprint: [4]byte{0xde, 0xad, 0xbe, 0xef}, FpPresent: true},
			{ChainCode: cc2, CompressedPubkey: pk2, Fingerprint: [4]byte{0xfe, 0xed, 0xfa, 0xce}, FpPresent: true},
		},
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   OriginShared,
		SharedOrigin: sharedOrigin4828(),
	})
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	d, err := Reassemble(base)
	if err != nil {
		t.Fatalf("Reassemble: %v", err)
	}
	// The repeat is installed HERE, after a legitimate encode, because no
	// encoder in the constellation will emit it.
	d.tree = node{tag: tagWsh, body: childrenBody{children: []node{{
		tag: tagSortedMulti, body: multiKeysBody{k: k, indices: indices},
	}}}}
	if keyless {
		d.tlv.pubPresent = false
		d.tlv.pubkeys = nil
	}
	return d
}

func dupSeatPath(name string) string {
	return filepath.Join("testdata", dupSeatFixtureDir, name+".md1.txt")
}

// TestDupSeatFixturesMatchTheirGenerator is the anti-decay gate. It rebuilds
// every fork-built repeated-seat fixture and compares it to the checked-in
// bytes, so the file can never quietly stop being what the tree produces.
//
// Set MD_REGEN_DUP_SEAT=1 to rewrite the files after an intentional change.
func TestDupSeatFixturesMatchTheirGenerator(t *testing.T) {
	regen := os.Getenv("MD_REGEN_DUP_SEAT") == "1"
	for _, v := range dupSeatVectors {
		t.Run(v.name, func(t *testing.T) {
			d := dupSeatDescriptor(t, v.k, v.indices, v.keyless)
			chunks, err := split(d)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			want := strings.Join(chunks, "\n") + "\n"
			if regen {
				if err := os.WriteFile(dupSeatPath(v.name), []byte(want), 0o644); err != nil {
					t.Fatalf("regen: %v", err)
				}
				t.Logf("regenerated %s", dupSeatPath(v.name))
				return
			}
			got, err := os.ReadFile(dupSeatPath(v.name))
			if err != nil {
				t.Fatalf("read %s: %v\nrun with MD_REGEN_DUP_SEAT=1 to create it", dupSeatPath(v.name), err)
			}
			if string(got) != want {
				t.Fatalf("%s has drifted from its generator (%s).\nwant:\n%s\ngot:\n%s\n"+
					"If the change is intended, rerun with MD_REGEN_DUP_SEAT=1",
					dupSeatPath(v.name), v.why, want, got)
			}
		})
	}
}

// TestDupSeatFixturesAreTheShapeTheyClaim measures the fixture rather than
// trusting its name: THREE seats, TWO slots, renderable as a sorted multi, and
// reported by the duplicate predicate as the fewer-keys kind.
func TestDupSeatFixturesAreTheShapeTheyClaim(t *testing.T) {
	for _, v := range dupSeatVectors {
		t.Run(v.name, func(t *testing.T) {
			raw, err := os.ReadFile(dupSeatPath(v.name))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var chunks []string
			for _, l := range strings.Split(string(raw), "\n") {
				if l = strings.TrimSpace(l); strings.HasPrefix(l, "md1") {
					chunks = append(chunks, l)
				}
			}
			tpl, keys, err := ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			for _, k := range keys {
				if k.XpubPresent == v.keyless {
					t.Fatalf("keyless=%v but slot @%d XpubPresent=%v; the fixture is not "+
						"the form it claims and the screens it exercises take the other arm",
						v.keyless, k.Index, k.XpubPresent)
				}
			}
			if v.keyless {
				// The whole point of the keyless form: the device will ENGRAVE
				// it. If the shape guard ever refuses it, review I-2's screen is
				// unreachable and its test is testing nothing.
				if err := TemplateEngraveShapeGuardChunks(chunks); err != nil {
					t.Fatalf("the keyless repeated-seat template is refused for engrave (%v), "+
						"so the consent screen it goes silent on is no longer reachable", err)
				}
			}
			if tpl.M != len(v.indices) {
				t.Errorf("M=%d, want %d seats", tpl.M, len(v.indices))
			}
			if tpl.N != 2 || len(keys) != 2 {
				t.Errorf("N=%d keys=%d, want 2 slots", tpl.N, len(keys))
			}
			if tpl.Policy != PolicySortedMulti || !tpl.Renderable {
				t.Errorf("policy=%v renderable=%v, want a renderable sorted multi",
					tpl.Policy, tpl.Renderable)
			}
			if int(v.k) != tpl.K {
				t.Errorf("K=%d, want %d", tpl.K, v.k)
			}
			slot, kind, err := DuplicateKeySlotChunks(chunks)
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if kind != DuplicateFewerKeys || slot != 0 {
				t.Errorf("duplicate predicate says %v @%d, want DuplicateFewerKeys @0",
					kind, slot)
			}
		})
	}
}
