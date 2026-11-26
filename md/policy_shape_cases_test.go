package md

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ─── seedhammer I-1: the Rust primary's shared policy-shape vectors ──────────
//
// md/testdata/policy_shape_cases.json is descriptor-mnemonic's
// crates/md-codec/tests/fixtures/policy_shape/cases.json, vendored by
// scripts/vendor-policy-shape-cases.sh and pinned by sha256 in
// policy_shape_cases.provenance.json. Every case must give the shape Rust
// gives, under DESIGN_thresh_summary section 6's comparison rule: Go has no
// slot list and no key-path entry, so the rule drops Rust's key-path entry and
// compares key COUNTS, the timelock flag, the locks, and the any-of counts.

type shapeCaseLock struct {
	Kind  string `json:"kind"`
	Value uint32 `json:"value"`
}

type shapeCaseBranch struct {
	KeyPathEntry bool `json:"key_path_entry"`
	AnyOf        *struct {
		K int `json:"k"`
		N int `json:"n"`
	} `json:"any_of"`
	K         int             `json:"k"`
	N         int             `json:"n"`
	Sorted    bool            `json:"sorted"`
	Slots     []int           `json:"slots"`
	Locks     []shapeCaseLock `json:"locks"`
	Hashlocks int             `json:"hashlocks"`
	Depth     int             `json:"depth"`
	MinKeys   *int            `json:"min_keys"`
}

type shapeCase struct {
	Name      string   `json:"name"`
	Template  *string  `json:"template"`
	Tree      jsonNode `json:"tree"`
	MD1       []string `json:"md1"`
	MD1Absent *string  `json:"md1_absent"`
	// Typed is Rust's policy_type_checks answer (THE TYPE RULE, md review r2
	// I-1). A pointer so a file that lost the field fails instead of reading
	// as false.
	Typed     *bool `json:"typed"`
	ShapeRust struct {
		Complete bool              `json:"complete"`
		KeyPath  string            `json:"key_path"`
		TapDepth int               `json:"tap_depth"`
		Branches []shapeCaseBranch `json:"branches"`
	} `json:"shape"`
}

// policyShapeCaseCount is the number of cases at the pinned primary commit
// (descriptor-mnemonic 1dac0e10). A count, not ">0": a truncated copy that kept
// one case would otherwise pass.
const policyShapeCaseCount = 106

func policyShapeCases(t *testing.T) []shapeCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "policy_shape_cases.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %v -- run scripts/vendor-policy-shape-cases.sh", err)
	}
	pinRaw, err := os.ReadFile(filepath.Join("testdata", "policy_shape_cases.provenance.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin: %v", err)
	}
	var pin struct {
		Commit string `json:"commit"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(pinRaw, &pin); err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != pin.SHA256 || pin.Commit == "" {
		t.Fatalf("policy_shape_cases.json disagrees with its pin (or the pin names no commit)")
	}
	var doc struct {
		Format int         `json:"format"`
		Cases  []shapeCase `json:"cases"`
	}
	// An unknown field fails: a field the primary adds is a rule this reader
	// does not yet compare, and silently ignoring it would pass a port that
	// disagrees with it.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("policy_shape_cases.json: %v", err)
	}
	if doc.Format != 1 {
		t.Fatalf("policy_shape_cases.json is format %d; this reader knows format 1", doc.Format)
	}
	if len(doc.Cases) != policyShapeCaseCount {
		t.Fatalf("policy_shape_cases.json carries %d cases, want %d", len(doc.Cases), policyShapeCaseCount)
	}
	for _, c := range doc.Cases {
		if c.Typed == nil {
			t.Fatalf("case %s carries no typed answer", c.Name)
		}
		// THE TYPE RULE: complete = walk && typed. A file that says an
		// untyped tree is complete contradicts its own rule.
		if !*c.Typed && c.ShapeRust.Complete {
			t.Fatalf("case %s: typed false but shape complete", c.Name)
		}
	}
	return doc.Cases
}

var shapeCaseKeyPath = map[string]KeyPathKind{
	"NotTaproot":       KeyPathNone,
	"Xpub":             KeyPathSpendable,
	"Nums":             KeyPathNUMS,
	"LianaUnspendable": KeyPathLianaUnspendable,
}

var shapeCaseLockKind = map[string]LockKind{
	"OlderBlocks": LockOlderBlocks,
	"OlderUnits":  LockOlderUnits,
	"AfterHeight": LockAfterHeight,
	"AfterTime":   LockAfterTime,
}

// shapeAgrees applies DESIGN_thresh_summary section 6's comparison rule and
// returns the first disagreement, or "".
func shapeAgrees(c shapeCase, got PolicyShape) string {
	want := c.ShapeRust
	// 1. Complete. When false nothing else is compared: both walks may hold
	// partial entries for a taproot tree, and no caller may read them.
	if got.Complete != want.Complete {
		return fmt.Sprintf("Complete = %v, Rust says %v", got.Complete, want.Complete)
	}
	if !want.Complete {
		return ""
	}
	// 2. Key path and tap depth.
	kp, ok := shapeCaseKeyPath[want.KeyPath]
	if !ok {
		return fmt.Sprintf("unknown Rust key_path %q", want.KeyPath)
	}
	if got.KeyPath != kp {
		return fmt.Sprintf("KeyPath = %v, Rust says %s", got.KeyPath, want.KeyPath)
	}
	if got.TapDepth != want.TapDepth {
		return fmt.Sprintf("TapDepth = %d, Rust says %d", got.TapDepth, want.TapDepth)
	}
	// 3. Drop Rust's key-path entry; the rest match in number and order.
	var rb []shapeCaseBranch
	for _, b := range want.Branches {
		if !b.KeyPathEntry {
			rb = append(rb, b)
		}
	}
	if len(got.Branches) != len(rb) {
		return fmt.Sprintf("%d branches, Rust has %d (key-path entry dropped)", len(got.Branches), len(rb))
	}
	// 4. Per entry.
	for i, w := range rb {
		g := got.Branches[i]
		wk, wn := 0, 0
		if w.AnyOf != nil {
			wk, wn = w.AnyOf.K, w.AnyOf.N
		}
		if g.AnyK != wk || g.AnyN != wn {
			return fmt.Sprintf("branch %d: AnyK/AnyN = %d/%d, Rust any_of %d/%d", i+1, g.AnyK, g.AnyN, wk, wn)
		}
		if g.K != w.K || g.N != w.N || g.Sorted != w.Sorted {
			return fmt.Sprintf("branch %d: K/N/Sorted = %d/%d/%v, Rust %d/%d/%v", i+1, g.K, g.N, g.Sorted, w.K, w.N, w.Sorted)
		}
		if g.Keys != len(w.Slots) {
			return fmt.Sprintf("branch %d: Keys = %d, Rust has %d slots", i+1, g.Keys, len(w.Slots))
		}
		if g.Timelock != (len(w.Locks) > 0) {
			return fmt.Sprintf("branch %d: Timelock = %v, Rust has %d locks", i+1, g.Timelock, len(w.Locks))
		}
		if len(g.Locks) != len(w.Locks) {
			return fmt.Sprintf("branch %d: %d locks, Rust has %d", i+1, len(g.Locks), len(w.Locks))
		}
		for j, wl := range w.Locks {
			kind, ok := shapeCaseLockKind[wl.Kind]
			if !ok {
				return fmt.Sprintf("branch %d: unknown Rust lock kind %q", i+1, wl.Kind)
			}
			if g.Locks[j] != (Lock{Kind: kind, Value: wl.Value}) {
				return fmt.Sprintf("branch %d lock %d: %+v, Rust %s %d", i+1, j+1, g.Locks[j], wl.Kind, wl.Value)
			}
		}
		if g.Hashlock != (w.Hashlocks > 0) || len(g.Hashlocks) != w.Hashlocks {
			return fmt.Sprintf("branch %d: Hashlock %v with %d hashlocks, Rust has %d", i+1, g.Hashlock, len(g.Hashlocks), w.Hashlocks)
		}
		if g.Depth != w.Depth {
			return fmt.Sprintf("branch %d: Depth = %d, Rust %d", i+1, g.Depth, w.Depth)
		}
		if w.AnyOf != nil {
			if w.MinKeys == nil {
				return fmt.Sprintf("branch %d: Rust any-of entry carries no min_keys", i+1)
			}
			if g.AnyOfMinKeys() != *w.MinKeys {
				return fmt.Sprintf("branch %d: AnyOfMinKeys = %d, Rust %d", i+1, g.AnyOfMinKeys(), *w.MinKeys)
			}
		} else if w.MinKeys != nil {
			return fmt.Sprintf("branch %d: Rust conjunction carries min_keys %d", i+1, *w.MinKeys)
		}
	}
	return ""
}

// TestPolicyShapeAgreesWithRustCases: every vendored case, once from its tree
// (policyShape) and once from its card (PolicyShapeChunks) where it has one.
func TestPolicyShapeAgreesWithRustCases(t *testing.T) {
	cases := policyShapeCases(t)
	fromCard := 0
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			if (c.MD1 == nil) == (c.MD1Absent == nil) {
				t.Fatalf("case must carry exactly one of md1 and md1_absent")
			}
			tree := buildNode(t, c.Tree)
			if d := shapeAgrees(c, policyShape(tree)); d != "" {
				t.Errorf("from tree: %s", d)
			}
			if c.MD1 == nil {
				return
			}
			fromCard++
			shape, err := PolicyShapeChunks(c.MD1)
			if err != nil {
				t.Fatalf("from md1: decode: %v", err)
			}
			if d := shapeAgrees(c, shape); d != "" {
				t.Errorf("from md1: %s", d)
			}
		})
	}
	// 102 of the 106 carry a card; md1_absent names why the other four cannot
	// (k = 0, k > n, no placeholder, multi in a tap leaf).
	if fromCard != 102 {
		t.Errorf("%d cases compared from a card, want 102", fromCard)
	}
}

// TestPolicyTypeChecksAgreesWithRustCases compares THE TYPE RULE on its own,
// case by case, against Rust's recorded `typed` -- including the cases whose
// walk is already incomplete, where the shape comparison alone could not tell a
// wrong type checker from a right one.
func TestPolicyTypeChecksAgreesWithRustCases(t *testing.T) {
	for _, c := range policyShapeCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			tree := buildNode(t, c.Tree)
			if got := treeTypeChecks(tree); got != *c.Typed {
				t.Errorf("from tree: treeTypeChecks = %v, Rust typed = %v", got, *c.Typed)
			}
			if c.MD1 == nil {
				return
			}
			d, err := Reassemble(c.MD1)
			if err != nil {
				t.Fatalf("from md1: decode: %v", err)
			}
			if got := treeTypeChecks(d.tree); got != *c.Typed {
				t.Errorf("from md1: treeTypeChecks = %v, Rust typed = %v", got, *c.Typed)
			}
		})
	}
}

// TestPolicyShapeCasesCoverEveryVerdict guards the vendored file against a
// regeneration that quietly dropped a class: it must hold incomplete shapes,
// conjunctions over a thresh, and any-of entries, both all-key and mixed and
// keyless.
func TestPolicyShapeCasesCoverEveryVerdict(t *testing.T) {
	var incomplete, conj, allKeys, mixed, keyless, untypedWalkComplete int
	for _, c := range policyShapeCases(t) {
		// The type rule must be load-bearing in the file: some untyped tree
		// must be one the walk alone would summarize (or_b(pk, thresh) is
		// the review's example), or the rule's tests prove nothing.
		if !*c.Typed && walkShape(buildNode(t, c.Tree)).Complete {
			untypedWalkComplete++
		}
		if !c.ShapeRust.Complete {
			incomplete++
			continue
		}
		for _, b := range c.ShapeRust.Branches {
			switch {
			case b.AnyOf == nil:
				conj++
			case len(b.Locks) == 0:
				allKeys++
			case *b.MinKeys == 0:
				keyless++
			default:
				mixed++
			}
		}
	}
	if incomplete == 0 || conj == 0 || allKeys == 0 || mixed == 0 || keyless == 0 || untypedWalkComplete == 0 {
		t.Fatalf("vendored cases lost a class: incomplete %d, conjunction %d, all-key %d, mixed %d, keyless %d, untyped-but-walk-complete %d",
			incomplete, conj, allKeys, mixed, keyless, untypedWalkComplete)
	}
}

// TestWitnessItemLimitVectors pins the two Rust vectors for Segwitv0's
// 100-witness-item limit (review r2 m-2; descriptor-mnemonic 1dac0e10): five
// chained 20-of-20 multis need 105 items with the script and are refused by
// that limit alone (3415 bytes, 105 ops, both under their limits), while four
// need 84 and type-check. The walk accepts both, so only the type rule tells
// them apart, from the tree and from the card.
func TestWitnessItemLimitVectors(t *testing.T) {
	want := map[string]bool{"untyped_witness_items_over_100": false, "typed_witness_items_84": true}
	for _, c := range policyShapeCases(t) {
		typed, ok := want[c.Name]
		if !ok {
			continue
		}
		delete(want, c.Name)
		t.Run(c.Name, func(t *testing.T) {
			if *c.Typed != typed {
				t.Fatalf("Rust typed = %v, want %v", *c.Typed, typed)
			}
			if c.MD1 == nil {
				t.Fatal("no card")
			}
			d, err := Reassemble(c.MD1)
			if err != nil {
				t.Fatal(err)
			}
			if !walkShape(d.tree).Complete {
				t.Fatal("walk refused; the vector no longer isolates the limit")
			}
			if got := treeTypeChecks(d.tree); got != typed {
				t.Errorf("treeTypeChecks = %v, want %v", got, typed)
			}
			shape, err := PolicyShapeChunks(c.MD1)
			if err != nil {
				t.Fatal(err)
			}
			if shape.Complete != typed {
				t.Errorf("PolicyShapeChunks Complete = %v, want %v", shape.Complete, typed)
			}
		})
	}
	for name := range want {
		t.Errorf("vendored file has no case %q", name)
	}
}
