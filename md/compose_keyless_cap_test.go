package md

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The primary's CONFORMANCE VECTOR for spec §4e's key-less cap (md-codec
// 0.45.0, composer fable review r0 lens 1 C-1): a policy holds at most one
// key-less spend path.
//
// PINNED SEPARATELY from the rest of the compose corpus, and
// md/testdata/compose_refusal_vectors.provenance.json carries the measured
// reason: the commit this vector arrives on also carries md-codec 0.44.0,
// which rewrote the xpubs in all 33 keyed conformance records, and a full
// re-vendor to reach one file leaves `go test ./md/` GREEN while importing
// that unported rule -- the conformance gate parses .chains[].descriptor into
// a field it never asserts.
//
// IT IS THE FIRST REFUSAL VECTOR IN THE CORPUS, so it has a shape of its own:
// `md vectors` exports ADMITTED policies (a template, a card, addresses) and
// a refusal has none of those, so the primary maintains this file by hand and
// it carries ONE file rather than four or five.
const composeRefusalProvenance = "testdata/compose_refusal_vectors.provenance.json"

type composeRefusalPin struct {
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Files  []struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
	Vectors int `json:"vectors"`
}

var composeRefusalOnce struct {
	sync.Once
	names map[string]bool
}

// composeRefusalPinnedNames is the set of files the refusal pin covers. It is
// what keeps the whole-corpus directory scan honest about the exclusion.
func composeRefusalPinnedNames() map[string]bool {
	composeRefusalOnce.Do(func() {
		composeRefusalOnce.names = map[string]bool{}
		raw, err := os.ReadFile(composeRefusalProvenance)
		if err != nil {
			return
		}
		var p composeRefusalPin
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		for _, f := range p.Files {
			composeRefusalOnce.names[f.Name] = true
		}
	})
	return composeRefusalOnce.names
}

func TestComposeRefusalVectorsMatchTheirProvenancePin(t *testing.T) {
	raw, err := os.ReadFile(composeRefusalProvenance)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin at %s: %v", composeRefusalProvenance, err)
	}
	var p composeRefusalPin
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("parsing %s: %v", composeRefusalProvenance, err)
	}
	if strings.TrimSpace(p.Commit) == "" || strings.TrimSpace(p.Path) == "" {
		t.Fatalf("INCONCLUSIVE: %s names no primary commit and path", composeRefusalProvenance)
	}
	if len(p.Files) != p.Vectors || len(p.Files) == 0 {
		t.Fatalf("pin lists %d files for %d vectors", len(p.Files), p.Vectors)
	}
	for _, f := range p.Files {
		body, err := os.ReadFile(filepath.Join("testdata", "vectors", f.Name))
		if err != nil {
			t.Fatalf("pinned file missing: %v", err)
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
			t.Errorf("%s: sha256 %s, pin says %s", f.Name, got, f.SHA256)
		}
	}
	// The DIRECTORY, the other way round: a compose_refusal_* file in the tree
	// that this pin does not list.
	entries, err := os.ReadDir(filepath.Join("testdata", "vectors"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "compose_refusal_") && !composeRefusalPinnedNames()[e.Name()] {
			t.Errorf("%s: in testdata/vectors but in neither pin -- re-run "+
				"scripts/vendor-compose-refusal-vectors.sh or remove it", e.Name())
		}
	}
}

// ─── the vector itself ──────────────────────────────────────────────────────

type keylessCapVector struct {
	Rule        string `json:"rule"`
	RefusedWhen string `json:"refused_when"`
	Error       string `json:"error"`
	Cases       []struct {
		Name    string `json:"name"`
		Wrapper string `json:"wrapper"`
		Paths   []struct {
			Keys *struct {
				K      uint8 `json:"k"`
				N      uint8 `json:"n"`
				Sorted bool  `json:"sorted"`
			} `json:"keys"`
			Hash *struct {
				Kind   string `json:"kind"`
				Digest string `json:"digest"`
			} `json:"hash"`
			Lock *struct {
				Kind  string `json:"kind"`
				Value uint32 `json:"value"`
			} `json:"lock"`
		} `json:"paths"`
		Expect string `json:"expect"`
		Error  *struct {
			Kind   string `json:"kind"`
			First  int    `json:"first"`
			Second int    `json:"second"`
		} `json:"error"`
		Slots int `json:"slots"`
	} `json:"cases"`
}

// TestComposeKeylessCapAgreesWithTheRustVector is the conformance gate for the
// rule md/compose.go ports from md_codec::compose::validate().
//
// The Rust implementer re-measured 859 wsh path lists of two to four paths
// over five keyed and four key-less atoms: all 504 with two or more key-less
// paths refused, all 355 with at most one emitted, zero counterexamples. So
// the rule is exactly "more than one key-less path, refuse", at any positions,
// whatever locks or hashes they carry -- and the vector's third case is a
// NON-ADJACENT pair, which is the case a port that only looked at neighbours
// would pass three of four on.
func TestComposeKeylessCapAgreesWithTheRustVector(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", "compose_refusal_keyless_cap.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %v", err)
	}
	var v keylessCapVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parsing the vector: %v", err)
	}
	if len(v.Cases) == 0 {
		t.Fatal("INCONCLUSIVE: the vector carries no cases")
	}
	var refused, admitted int
	for _, c := range v.Cases {
		list := PathList{}
		switch c.Wrapper {
		case "wsh":
			list.Wrapper = ComposeWsh
		case "tr":
			list.Wrapper = ComposeTr
		case "sh":
			list.Wrapper = ComposeSh
		case "sh-wsh":
			list.Wrapper = ComposeShWsh
		default:
			t.Fatalf("%s: unknown wrapper %q", c.Name, c.Wrapper)
		}
		for i, p := range c.Paths {
			var sp SpendPath
			if p.Keys != nil {
				sp.Keys = &KeySet{K: p.Keys.K, N: p.Keys.N, Sorted: p.Keys.Sorted}
			}
			if p.Hash != nil {
				d, err := hex.DecodeString(p.Hash.Digest)
				if err != nil {
					t.Fatalf("%s path %d: %v", c.Name, i+1, err)
				}
				var kind HashKind
				switch p.Hash.Kind {
				case "sha256":
					kind = KindSha256
				case "hash256":
					kind = KindHash256
				case "ripemd160":
					kind = KindRipemd160
				case "hash160":
					kind = KindHash160
				default:
					t.Fatalf("%s path %d: unknown hash kind %q", c.Name, i+1, p.Hash.Kind)
				}
				h, ok := NewHashLock(kind, d)
				if !ok {
					t.Fatalf("%s path %d: NewHashLock refused the vector's digest", c.Name, i+1)
				}
				sp.Hash = h
			}
			if p.Lock != nil {
				var kind LockKind
				switch p.Lock.Kind {
				case "older_blocks":
					kind = LockOlderBlocks
				case "older_units":
					kind = LockOlderUnits
				case "after_height":
					kind = LockAfterHeight
				case "after_time":
					kind = LockAfterTime
				default:
					t.Fatalf("%s path %d: unknown lock kind %q", c.Name, i+1, p.Lock.Kind)
				}
				sp.Lock = &Lock{Kind: kind, Value: p.Lock.Value}
			}
			list.Paths = append(list.Paths, sp)
		}
		slots, err := ValidatePathList(list)
		switch c.Expect {
		case "refused":
			refused++
			if err == nil {
				t.Errorf("%s: ValidatePathList admitted a list the primary refuses (%s)", c.Name, v.RefusedWhen)
				continue
			}
			if c.Error == nil || c.Error.Kind != v.Error {
				t.Fatalf("%s: the vector's case names error %+v, the file's rule names %q",
					c.Name, c.Error, v.Error)
			}
			var got TwoKeylessPathsError
			if !errors.As(err, &got) {
				t.Errorf("%s: refused with %v, want %s", c.Name, err, v.Error)
				continue
			}
			if got.First != c.Error.First || got.Second != c.Error.Second {
				t.Errorf("%s: %s names paths %d and %d (0-based), the vector says %d and %d",
					c.Name, v.Error, got.First, got.Second, c.Error.First, c.Error.Second)
			}
		case "admitted":
			admitted++
			if err != nil {
				t.Errorf("%s: ValidatePathList refused the control case (%v) -- a port that "+
					"refused EVERY key-less path would pass the three refusals above and "+
					"delete the EXPERIMENTAL feature", c.Name, err)
				continue
			}
			if c.Slots != 0 && slots != c.Slots {
				t.Errorf("%s: %d slots, the vector says %d", c.Name, slots, c.Slots)
			}
		default:
			t.Fatalf("%s: unknown expect %q", c.Name, c.Expect)
		}
	}
	if refused == 0 || admitted == 0 {
		t.Fatalf("INCONCLUSIVE: %d refusals and %d admissions -- the vector must carry both "+
			"or one direction of the rule is untested", refused, admitted)
	}
	t.Logf("%s: %d refused, %d admitted", v.Rule, refused, admitted)
}
