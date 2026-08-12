package sysw

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The vectors are the CONTRACT. This package and the Rust primary never compare
// to each other, only to this file -- so a case the file omits is one both
// implementations can be wrong about identically and neither will notice.
//
// The file lives in the primary repo because that is where it is generated.
// SYSW_VECTORS overrides the path; the default assumes the usual side-by-side
// checkout.
const defaultVectors = "../../mnemonic-engrave/crates/me-cli/testdata/sysw_vectors.json"

type vector struct {
	Name       string   `json:"name"`
	Note       string   `json:"note"`
	Records    []string `json:"records"`
	Passphrase *string  `json:"passphrase"`
	Blob       string   `json:"blob"`
	PubLen     uint32   `json:"pub_len"`
	CtLen      uint32   `json:"ct_len"`
	Sealed     bool     `json:"sealed"`
	Digest     *string  `json:"digest"`
	Identity   string   `json:"identity"`
}

// loadVectors resolves the file, and FAILS rather than skipping when
// SYSW_REQUIRE_VECTORS=1.
//
// The fork's own cross-language harness already learned this: a differential
// oracle that silently no-ops reads exactly like one that passes. CI sets the
// variable; a local checkout without the sibling repo skips with a note.
func loadVectors(t *testing.T) []vector {
	t.Helper()
	path := os.Getenv("SYSW_VECTORS")
	if path == "" {
		path = defaultVectors
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		abs, _ := filepath.Abs(path)
		if os.Getenv("SYSW_REQUIRE_VECTORS") == "1" {
			t.Fatalf("SYSW_REQUIRE_VECTORS=1 and the vectors are unreadable at %s: %v", abs, err)
		}
		t.Skipf("vectors not found at %s (%v); set SYSW_VECTORS or SYSW_REQUIRE_VECTORS=1", abs, err)
	}
	var vs []vector
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if len(vs) == 0 {
		t.Fatalf("INCONCLUSIVE: %s holds no vectors, so this test checks nothing", path)
	}
	return vs
}

// TestConformance is the whole reason this package is a port rather than a
// reimplementation: every vector the Rust primary pins must open here to the
// same records, with the same digest and the same identity.
func TestConformance(t *testing.T) {
	vs := loadVectors(t)
	for _, v := range vs {
		t.Run(v.Name, func(t *testing.T) {
			blob, err := hex.DecodeString(v.Blob)
			if err != nil {
				t.Fatalf("vector blob is not hex: %v", err)
			}

			h, err := ParseHeader(blob)
			if err != nil {
				t.Fatalf("ParseHeader: %v", err)
			}
			if h.PubLen != v.PubLen || h.CtLen != v.CtLen || h.Sealed() != v.Sealed {
				t.Errorf("header disagrees with the vector: got pub=%d ct=%d sealed=%v, want pub=%d ct=%d sealed=%v",
					h.PubLen, h.CtLen, h.Sealed(), v.PubLen, v.CtLen, v.Sealed)
			}

			if got := hex.EncodeToString(idBytes(Identity(blob))); got != v.Identity {
				t.Errorf("identity\n got %s\nwant %s", got, v.Identity)
			}

			// The digest exists exactly when a public section does -- EPD §6.6's
			// own rule, and the case R0-C2 was raised about. A port that showed
			// the empty-set constant here would encode the bug.
			if v.PubLen == 0 {
				if v.Digest != nil {
					t.Fatalf("INCONCLUSIVE: the vector records a digest for pub_len == 0")
				}
			} else {
				pub, err := splitRecords(blob[HeaderLen : HeaderLen+int(h.PubLen)])
				if err != nil {
					t.Fatalf("public section: %v", err)
				}
				got := hex.EncodeToString(hashBytes(PublicDataHash(pub, h.Sealed())))
				if v.Digest == nil {
					t.Fatalf("INCONCLUSIVE: the vector records no digest for pub_len=%d", v.PubLen)
				}
				if got != *v.Digest {
					t.Errorf("digest\n got %s\nwant %s", got, *v.Digest)
				}
			}

			pass := ""
			if v.Passphrase != nil {
				pass = *v.Passphrase
			}
			p, err := Open(blob, pass)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if n := len(p.Public) + len(p.Secret); n != len(v.Records) {
				t.Fatalf("recovered %d records, the vector has %d", n, len(v.Records))
			}
			// Every record must come back, wherever it was stored.
			for _, want := range v.Records {
				if !contains(p.Public, want) && !contains(p.Secret, want) {
					t.Errorf("record missing after Open: %.40q", want)
				}
			}
		})
	}
}

// TestTheVectorSetIsMeaningful guards the guard: a conformance run over a set
// that happens to contain only easy cases proves very little.
func TestTheVectorSetIsMeaningful(t *testing.T) {
	vs := loadVectors(t)
	var sealed, secretsOnly, plaintext, encoded bool
	for _, v := range vs {
		sealed = sealed || v.Sealed
		secretsOnly = secretsOnly || (v.Sealed && v.PubLen == 0)
		plaintext = plaintext || !v.Sealed
		for _, r := range v.Records {
			if Classify(r) == ClassFreeText {
				encoded = true
			}
		}
	}
	if !sealed || !plaintext {
		t.Error("the set must cover both container variants")
	}
	if !secretsOnly {
		t.Error("the set must contain a secrets-only sealed payload — the pub_len == 0 case")
	}
	if !encoded {
		t.Error("the set must contain an encoded text: record")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func idBytes(a [32]byte) []byte   { return a[:] }
func hashBytes(a [16]byte) []byte { return a[:] }

// TestPaddedRegionMatchesTheBareVector is I5 from the pre-flash conformance
// review. What actually gets written to 0x10D00000 is a 65536-byte REGION:
// the container at offset 0, then 0xFF to the end (`me sysw pack --region`).
// Every vector is a bare blob, so nothing pinned that both implementations trim
// to the header's declared total before hashing.
//
// They agree today — measured during the review. This is what keeps them
// agreeing. If it ever fails, the device and the host disagree about every
// payload ever flashed, and the operator's on-screen digest comparison silently
// stops meaning anything.
//
// The mirror of this test lives in the Rust primary
// (sysw::vectors::tests::padding_a_vector_to_a_full_region_changes_no_identity_and_no_digest),
// so the property is pinned on both sides rather than assumed on one.
func TestPaddedRegionMatchesTheBareVector(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			blob, err := hex.DecodeString(v.Blob)
			if err != nil {
				t.Fatalf("vector blob is not hex: %v", err)
			}
			region := make([]byte, RegionLen)
			for i := range region {
				region[i] = 0xFF
			}
			copy(region, blob)

			h, err := ParseHeader(region)
			if err != nil {
				t.Fatalf("a padded region must still parse: %v", err)
			}
			if got := hex.EncodeToString(idBytes(Identity(region[:h.TotalLen()]))); got != v.Identity {
				t.Errorf("padding to a full region moved the identity\n got %s\nwant %s",
					got, v.Identity)
			}
		})
	}
}
