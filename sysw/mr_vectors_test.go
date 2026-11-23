package sysw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The vendored S1 vectors (SPEC_systemwide_payloads.md section 14i): the Go
// port of the Rust primary's string (14b), chunk-set (14c) and kit (14d) rules
// is pinned to them row for row.

const (
	mrVectorsFile       = "testdata/sysw_mr_v1_vectors.json"
	mrVectorsProvenance = "testdata/sysw_mr_v1_vectors.provenance.json"
)

type mrDetailJSON struct {
	Begins     string `json:"begins"`
	Record     *int   `json:"record"`
	Index      *int   `json:"index"`
	Count      *int   `json:"count"`
	FirstCount *int   `json:"first_count"`
	Records    []int  `json:"records"`
	Kind       string `json:"kind"`
	Class      string `json:"class"`
}

type mrCardJSON struct {
	Kind    string `json:"kind"`
	Strings int    `json:"strings"`
	Begins  string `json:"begins"`
	Ends    string `json:"ends"`
}

type mrVectorFile struct {
	StringVectors []struct {
		Name   string  `json:"name"`
		Record string  `json:"record"`
		Class  string  `json:"class"`
		Reason *string `json:"reason"`
	} `json:"string_vectors"`
	SetVectors []struct {
		Name    string        `json:"name"`
		Records []string      `json:"records"`
		Pack    string        `json:"pack"`
		Reason  *string       `json:"reason"`
		Detail  *mrDetailJSON `json:"detail"`
	} `json:"set_vectors"`
	KitVectors []struct {
		Name    string        `json:"name"`
		Records []string      `json:"records"`
		Kit     string        `json:"kit"`
		Reason  *string       `json:"reason"`
		Detail  *mrDetailJSON `json:"detail"`
		Cards   []mrCardJSON  `json:"cards"`
	} `json:"kit_vectors"`
}

func loadMrVectors(t *testing.T) mrVectorFile {
	t.Helper()
	raw, err := os.ReadFile(mrVectorsFile)
	if err != nil {
		t.Fatalf("reading %s: %v", mrVectorsFile, err)
	}
	var f mrVectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parsing %s: %v", mrVectorsFile, err)
	}
	return f
}

// detailMatches compares a returned MrDetail with a vector's detail: every
// field the vector names must equal, and the vector naming a field is what
// makes the field part of the contract.
func detailMatches(t *testing.T, name string, got MrDetail, want *mrDetailJSON) {
	t.Helper()
	if want == nil {
		return
	}
	eq := func(field string, g int, w *int) {
		if w != nil && g != *w {
			t.Errorf("%s: detail %s = %d, want %d", name, field, g, *w)
		}
	}
	if want.Begins != "" && got.Begins != want.Begins {
		t.Errorf("%s: detail begins = %q, want %q", name, got.Begins, want.Begins)
	}
	eq("record", got.Record, want.Record)
	eq("index", got.Index, want.Index)
	eq("count", got.Count, want.Count)
	eq("first_count", got.FirstCount, want.FirstCount)
	if want.Records != nil && (len(want.Records) != 2 || got.Records != [2]int{want.Records[0], want.Records[1]}) {
		t.Errorf("%s: detail records = %v, want %v", name, got.Records, want.Records)
	}
	if want.Kind != "" && got.Kind != want.Kind {
		t.Errorf("%s: detail kind = %q, want %q", name, got.Kind, want.Kind)
	}
	if want.Class != "" && mrClassVectorName(got.Class) != want.Class {
		t.Errorf("%s: detail class = %q, want %q", name, mrClassVectorName(got.Class), want.Class)
	}
}

// mrClassVectorName is the vector file's spelling of a class (the `class` of a
// KIT_OTHER_RECORD detail); the vectors only carry the classes below.
func mrClassVectorName(c Class) string {
	switch c {
	case ClassFreeText:
		return "free text"
	case ClassCodex32Secret:
		return "codex32 secret"
	case ClassMnemonic:
		return "BIP-39 mnemonic"
	}
	return "?"
}

func TestSyswMrVectorsProvenance(t *testing.T) {
	rawPin, err := os.ReadFile(mrVectorsProvenance)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin at %s: %v", mrVectorsProvenance, err)
	}
	var pin struct {
		Repo, Path, Commit, SHA256 string
		Bytes, Vectors             int
	}
	if err := json.Unmarshal(rawPin, &pin); err != nil {
		t.Fatalf("parsing %s: %v", mrVectorsProvenance, err)
	}
	if pin.Repo != "mnemonic-engrave" || strings.TrimSpace(pin.Commit) == "" || strings.TrimSpace(pin.Path) == "" {
		t.Fatalf("INCONCLUSIVE: the pin must name the mnemonic-engrave commit and path: %+v", pin)
	}
	raw, err := os.ReadFile(mrVectorsFile)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != pin.SHA256 {
		t.Fatalf("%s sha256 = %s, but the pin records %s: the vendored file was edited or the pin was not re-recorded", mrVectorsFile, got, pin.SHA256)
	}
	if len(raw) != pin.Bytes {
		t.Fatalf("%s is %d bytes, the pin records %d", mrVectorsFile, len(raw), pin.Bytes)
	}
	f := loadMrVectors(t)
	if n := len(f.StringVectors) + len(f.SetVectors) + len(f.KitVectors); n != pin.Vectors {
		t.Fatalf("the file holds %d vectors, the pin records %d", n, pin.Vectors)
	}
}

// TestSyswMrStringVectors is S1 test 24: every string vector classifies as its
// class, and a refused one reports its reason.
func TestSyswMrStringVectors(t *testing.T) {
	f := loadMrVectors(t)
	if len(f.StringVectors) != 13 {
		t.Fatalf("expected 13 string vectors, file holds %d", len(f.StringVectors))
	}
	for _, v := range f.StringVectors {
		wantClass := ClassUnknown
		if v.Class == "Mr" {
			wantClass = ClassMr
		}
		if got := Classify(v.Record); got != wantClass {
			t.Errorf("%s: Classify = %v, want %v", v.Name, got, wantClass)
		}
		if !IsMrCandidate(v.Record) {
			t.Errorf("%s: not an mr1 candidate", v.Name)
		}
		want := ""
		if v.Reason != nil {
			want = *v.Reason
		}
		if got := MrStringReason(v.Record); got != want {
			t.Errorf("%s: MrStringReason = %q, want %q", v.Name, got, want)
		}
	}
	if ClassMr.IsSecret() {
		t.Error("ClassMr must not be secret")
	}
	// A trimmed record: the trim every constellation arm applies.
	if got := Classify("  " + f.StringVectors[0].Record + "\n"); got != ClassMr {
		t.Errorf("a padded single-card classifies %v, want ClassMr", got)
	}
	if IsMrCandidate("mt1abc") || IsMrCandidate("mr") {
		t.Error("IsMrCandidate accepted a non-mr1 prefix")
	}
}

// TestSyswMrSetVectors is S1 test 25: verdict, reason and detail per set
// vector.
func TestSyswMrSetVectors(t *testing.T) {
	f := loadMrVectors(t)
	if len(f.SetVectors) != 12 {
		t.Fatalf("expected 12 set vectors, file holds %d", len(f.SetVectors))
	}
	for _, v := range f.SetVectors {
		ref, refused := MrSetProblem(v.Records)
		if v.Pack == "ok" {
			if refused {
				t.Errorf("%s: refused %s, want pass", v.Name, ref.Reason)
			}
			continue
		}
		if !refused {
			t.Errorf("%s: passed, want %s", v.Name, *v.Reason)
			continue
		}
		if ref.Reason != *v.Reason {
			t.Errorf("%s: reason %s, want %s", v.Name, ref.Reason, *v.Reason)
		}
		detailMatches(t, v.Name, ref.Detail, v.Detail)
	}
}

// TestSyswMrKitVectors is S1 test 26: verdict, reason, detail and cards per kit
// vector.
func TestSyswMrKitVectors(t *testing.T) {
	f := loadMrVectors(t)
	if len(f.KitVectors) != 10 {
		t.Fatalf("expected 10 kit vectors, file holds %d", len(f.KitVectors))
	}
	for _, v := range f.KitVectors {
		kit := EvaluateKit(v.Records)
		switch v.Kit {
		case "none":
			if kit.Offers {
				t.Errorf("%s: offers a kit, want none", v.Name)
			}
			continue
		case "eligible", "refused":
			if !kit.Offers {
				t.Errorf("%s: offers no kit, want %s", v.Name, v.Kit)
				continue
			}
		default:
			t.Fatalf("%s: unknown verdict %q", v.Name, v.Kit)
		}
		if v.Kit == "refused" {
			if kit.Eligible {
				t.Errorf("%s: eligible, want %s", v.Name, *v.Reason)
				continue
			}
			if kit.Refusal.Reason != *v.Reason {
				t.Errorf("%s: reason %s, want %s", v.Name, kit.Refusal.Reason, *v.Reason)
			}
			detailMatches(t, v.Name, kit.Refusal.Detail, v.Detail)
			continue
		}
		if !kit.Eligible {
			t.Errorf("%s: refused %s %+v, want eligible", v.Name, kit.Refusal.Reason, kit.Refusal.Detail)
			continue
		}
		if len(kit.Cards) != len(v.Cards) {
			t.Errorf("%s: %d cards, want %d", v.Name, len(kit.Cards), len(v.Cards))
			continue
		}
		for i, c := range v.Cards {
			g := kit.Cards[i]
			if g.Kind != c.Kind || len(g.Strings) != c.Strings || g.Begins != c.Begins || g.Ends != c.Ends {
				t.Errorf("%s: card %d = {%s %d %s %s}, want {%s %d %s %s}", v.Name, i+1,
					g.Kind, len(g.Strings), g.Begins, g.Ends, c.Kind, c.Strings, c.Begins, c.Ends)
			}
		}
	}
	// kit-2of3 is 6 cards, kit-single 1, the two-copies kits 4.
	want := map[string]int{"kit-2of3": 6, "kit-single": 1, "kit-two-copies": 4, "kit-two-copies-mk1": 4}
	for _, v := range f.KitVectors {
		if n, ok := want[v.Name]; ok {
			if got := len(EvaluateKit(v.Records).Cards); got != n {
				t.Errorf("%s: %d cards, want %d", v.Name, got, n)
			}
		}
	}
}

// TestSyswMDMKGroupsByRuns is S1 section 14d's grouping rule: a record joins
// the latest group of its (hrp, chunk-set id) while that group holds fewer
// strings than its count, so two complete copies of one mk1 card each confirm.
func TestSyswMDMKGroupsByRuns(t *testing.T) {
	f := loadMrVectors(t)
	var kit []string
	for _, v := range f.KitVectors {
		if v.Name == "kit-2of3" {
			kit = v.Records
		}
	}
	if len(kit) != 12 {
		t.Fatal("kit-2of3 missing")
	}
	// dupmk: the two strings of the first mk1 plate, given twice.
	dupmk := []string{kit[2], kit[3], kit[2], kit[3]}
	if got := MDMKUnconfirmed(dupmk); len(got) != 0 {
		t.Errorf("dupmk: unconfirmed %v, want none (two complete groups)", got)
	}
	// A copy must be complete before the next starts: A1 A1 A2 A2 is a
	// duplicate then a short group, so everything stays unconfirmed.
	if got := MDMKUnconfirmed([]string{kit[2], kit[2], kit[3], kit[3]}); len(got) != 4 {
		t.Errorf("A1 A1 A2 A2: unconfirmed %v, want all four", got)
	}
	// The plain card is unchanged.
	if got := MDMKUnconfirmed(kit[2:4]); len(got) != 0 {
		t.Errorf("one mk1 card: unconfirmed %v, want none", got)
	}
}
