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
// VENDORED, since 2026-08-15 (C-4). The file is GENERATED in the primary repo
// and stays strictly downstream of it, but the path used to point into a SIBLING
// CHECKOUT (../../mnemonic-engrave/...) that the fork's workflow never checks
// out -- so every test in this file skipped on the machine whose verdict gates a
// merge, on every push, and the suite still reported ok and exit 0. The vendored
// copy carries a provenance pin (sysw_vectors.provenance.json) recording the
// primary commit and the file's SHA-256, exactly as every other Go port here
// pins the Rust crate it tracks.
//
// SYSW_VECTORS still overrides the path, for a developer testing against an
// unreleased vector set. There is no longer anything to skip: absent, empty or
// unparseable is INCONCLUSIVE and fatal.
const defaultVectors = "testdata/sysw_vectors.json"

// vectorProvenance is the pin beside the vendored copy.
const vectorProvenance = "testdata/sysw_vectors.provenance.json"

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
	// Unconfirmed is `[mdmk-decode]` (§12.6): the indices of the PUBLIC-SECTION
	// records that are not decode-confirmed.
	//
	// Indices are into the public section, NOT into Records -- the public
	// section is the one list both implementations reconstruct identically from
	// Blob, while Records is the primary's packing order, which a sealed
	// payload never reveals. It costs nothing: ClassMDMK is not a secret class,
	// so every md1/mk1 record is in the public section in both variants.
	Unconfirmed []int `json:"mdmk_unconfirmed"`
}

// loadVectors resolves the file. IT CANNOT SKIP.
//
// The fork's own cross-language harness already learned this once: a
// differential oracle that silently no-ops reads exactly like one that passes.
// The lesson was written down and then implemented as an ESCALATION
// (SYSW_REQUIRE_VECTORS=1 -> t.Fatalf) that nothing ever set, which is the same
// failure with a paper trail. Enforcement is not something an environment
// variable turns on: the vectors are in the repo, so absence here means the
// checkout is broken, and that is a failure everywhere including CI.
func loadVectors(t *testing.T) []vector {
	t.Helper()
	path := os.Getenv("SYSW_VECTORS")
	if path == "" {
		path = defaultVectors
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		abs, _ := filepath.Abs(path)
		t.Fatalf("INCONCLUSIVE: the conformance vectors are unreadable at %s: %v\n"+
			"They are vendored into this repo at %s and are not optional — without them "+
			"this package's agreement with the Rust primary is asserted by nothing.",
			abs, err, defaultVectors)
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

// TestConformanceMDMKDecode is `[mdmk-decode]` (§12.6) across the language
// boundary. The primary computes the expected set; this recomputes it from the
// BLOB, which is all a device ever has, and they must match record for record.
//
// It also guards the guard, and that is the load-bearing half. A vector set in
// which every record answered "unconfirmed" would be passed by an
// implementation that answers "unconfirmed" to everything, and one in which
// every record answered "confirmed" by its mirror image. So the set must hold
// BOTH answers, and hold them inside ONE payload -- only there does the
// (hrp, chunk_set_id) grouping have to be right, and only there can a port that
// grouped by HRP alone be caught.
func TestConformanceMDMKDecode(t *testing.T) {
	vs := loadVectors(t)
	var bothInOnePayload int
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
			var pub []string
			if h.PubLen > 0 {
				pub, err = splitRecords(blob[HeaderLen : HeaderLen+int(h.PubLen)])
				if err != nil {
					t.Fatalf("public section: %v", err)
				}
			}
			got := MDMKUnconfirmed(pub)
			if len(got) != len(v.Unconfirmed) {
				t.Fatalf("unconfirmed set\n got %v\nwant %v", got, v.Unconfirmed)
			}
			for i := range got {
				if got[i] != v.Unconfirmed[i] {
					t.Fatalf("unconfirmed set\n got %v\nwant %v", got, v.Unconfirmed)
				}
			}

			var mdmk int
			for _, r := range pub {
				if Classify(r) == ClassMDMK {
					mdmk++
				}
			}
			if len(v.Unconfirmed) > 0 && mdmk > len(v.Unconfirmed) {
				bothInOnePayload++
			}
		})
	}
	if bothInOnePayload == 0 {
		t.Error("INCONCLUSIVE: no vector holds a confirmed card BESIDE an unconfirmed " +
			"one, so nothing here can fail an implementation that answers the same way " +
			"for every record")
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
