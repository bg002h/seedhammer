package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"slices"
	"testing"

	"golang.org/x/image/math/fixed"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/text"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
)

// ─── seedhammer I-1: "any k of n" on the device (DESIGN_thresh_summary §3) ───
//
// The device text is fork-native: md-codec prints no spend-path summary, so
// these goldens have no Rust counterpart. The SHAPES they are printed from do:
// md/policy_shape_cases_test.go proves the Go walk gives Rust's shape for every
// vendored case. Each golden here is the whole policySummaryLines output for
// the case's card.

const policyShapeCasesPath = "../md/testdata/policy_shape_cases.json"

type summaryCase struct {
	Name  string   `json:"name"`
	MD1   []string `json:"md1"`
	Shape struct {
		Complete bool   `json:"complete"`
		KeyPath  string `json:"key_path"`
		TapDepth int    `json:"tap_depth"`
		Branches []struct {
			KeyPathEntry bool `json:"key_path_entry"`
			AnyOf        *struct {
				K int `json:"k"`
				N int `json:"n"`
			} `json:"any_of"`
			K     int   `json:"k"`
			N     int   `json:"n"`
			Slots []int `json:"slots"`
			Locks []struct {
				Kind  string `json:"kind"`
				Value uint32 `json:"value"`
			} `json:"locks"`
			Hashlocks int `json:"hashlocks"`
			Depth     int `json:"depth"`
		} `json:"branches"`
	} `json:"shape"`
}

// summaryCases reads the vendored file and checks it against the pin md's own
// test checks, so this package cannot pass against a copy md would refuse.
func summaryCases(t *testing.T) []summaryCase {
	t.Helper()
	raw, err := os.ReadFile(policyShapeCasesPath)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %v -- run scripts/vendor-policy-shape-cases.sh", err)
	}
	pinRaw, err := os.ReadFile("../md/testdata/policy_shape_cases.provenance.json")
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin: %v", err)
	}
	var pin struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(pinRaw, &pin); err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != pin.SHA256 {
		t.Fatal("policy_shape_cases.json disagrees with its pin")
	}
	var doc struct {
		Cases []summaryCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("no cases")
	}
	return doc.Cases
}

// shapeOfCase is the device's shape for a case: decoded from its card, or, for
// the one case no card can carry (no placeholder), built from Rust's shape.
// md/policy_shape_cases_test.go proves the walk gives that shape from the
// case's tree, so this is the shape the device would hold.
func shapeOfCase(t *testing.T, c summaryCase) md.PolicyShape {
	t.Helper()
	if c.MD1 != nil {
		s, err := md.PolicyShapeChunks(c.MD1)
		if err != nil {
			t.Fatalf("%s: decode: %v", c.Name, err)
		}
		return s
	}
	kinds := map[string]md.LockKind{
		"OlderBlocks": md.LockOlderBlocks, "OlderUnits": md.LockOlderUnits,
		"AfterHeight": md.LockAfterHeight, "AfterTime": md.LockAfterTime,
	}
	paths := map[string]md.KeyPathKind{
		"NotTaproot": md.KeyPathNone, "Xpub": md.KeyPathSpendable,
		"Nums": md.KeyPathNUMS, "LianaUnspendable": md.KeyPathLianaUnspendable,
	}
	s := md.PolicyShape{Complete: c.Shape.Complete, KeyPath: paths[c.Shape.KeyPath], TapDepth: c.Shape.TapDepth}
	if !s.Complete {
		return md.PolicyShape{}
	}
	for _, rb := range c.Shape.Branches {
		if rb.KeyPathEntry {
			continue
		}
		b := md.Branch{K: rb.K, N: rb.N, Keys: len(rb.Slots), Depth: rb.Depth, Timelock: len(rb.Locks) > 0}
		if rb.AnyOf != nil {
			b.AnyK, b.AnyN = rb.AnyOf.K, rb.AnyOf.N
		}
		for _, l := range rb.Locks {
			k, ok := kinds[l.Kind]
			if !ok {
				t.Fatalf("%s: unknown lock kind %q", c.Name, l.Kind)
			}
			b.Locks = append(b.Locks, md.Lock{Kind: k, Value: l.Value})
		}
		if rb.Hashlocks > 0 {
			t.Fatalf("%s: a hand-built shape with hashlocks is not representable here", c.Name)
		}
		s.Branches = append(s.Branches, b)
	}
	return s
}

// summaryGoldens: the whole policySummaryLines output for every COMPLETE
// vendored case, keyed by case name. DESIGN_thresh_summary §3/§6 "Device
// entry" column plus the header lines.
var summaryGoldens = map[string][]string{
	"thresh_bip388_3of4_lock":   {"Spend path: 1", "  1: any 3 of these 4: 3 keys, 1 timelock; at least 2 keys"},
	"thresh_compiler_3of4_lock": {"Spend path: 1", "  1: any 3 of these 4: 3 keys, 1 timelock; at least 2 keys"},
	"thresh_decay_2of3_lock":    {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_after_2of3":         {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_pkh_children":       {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_lock_first":         {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_alt_children":       {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_un_lock":            {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	// older(0x400000): a units lock of zero, inside the allowlist's
	// 1 <= v < 2^31 operand bound. Still a timelock condition.
	"thresh_zero_delay_units_lock": {"Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_1of3_keys":             {"Spend path: 1", "  1: any 1 of these 3 keys"},
	"thresh_2of3_keys":             {"Spend path: 1", "  1: any 2 of these 3 keys"},
	"thresh_sh_wsh":                {"Spend path: 1", "  1: any 2 of these 3 keys"},
	"thresh_check_wrapped_keys":    {"Spend path: 1", "  1: any 2 of these 3 keys"},
	"thresh_under_or_d":            {"Spend paths: 2", "  1: 1 key(s), custom", "  2: any 2 of these 3 keys"},
	"thresh_under_or_b":            {"Spend paths: 2", "  1: 1 key(s), custom", "  2: any 2 of these 3 keys"},
	"thresh_under_or_i":            {"Spend paths: 2", "  1: 1 key(s), custom", "  2: any 2 of these 3 keys"},
	"thresh_andor_z":               {"Spend paths: 2", "  1: 2 key(s), custom", "  2: any 2 of these 3 keys"},
	// The device lists LEAVES: the internal key is the key-path line, not a
	// numbered entry, and one leaf has tree depth 0 (design review I-2).
	"thresh_tr_leaf":       {"Key-path: A KEY CAN SPEND ALONE", "Spend path: 1", "  1: any 2 of these 3 keys"},
	"thresh_tr_two_leaves": {"Key-path: A KEY CAN SPEND ALONE", "Spend paths: 2 (tree depth 1)", "  1: 1 key(s), custom", "  2: any 2 of these 3 keys"},
	"thresh_tr_nums_leaf":  {"Key-path: none (script paths only)", "Spend path: 1", "  1: any 2 of these 3: 2 keys, 1 timelock; at least 1 key"},
	"thresh_keyless_1of2":  {"Spend path: 1", "  1: any 1 of these 2: 1 key, 1 timelock; NO KEY NEEDED"},
	// A height and a time cannot both be met by one transaction, so a key IS
	// needed; the text errs toward warning (design §1).
	"thresh_mixed_after_kinds": {"Spend path: 1", "  1: any 2 of these 3: 1 key, 2 timelocks; NO KEY NEEDED"},
	"thresh_locks_only_1of2":   {"Spend path: 1", "  1: any 1 of these 2: 2 timelocks; NO KEY NEEDED"},
	// k == n stays a conjunction, unchanged.
	"thresh_kn_2of2":      {"Spend path: 1", "  1: 2 key(s), custom"},
	"thresh_kn_3of3_lock": {"Spend path: 1", "  1: 2 key(s), custom +timelock"},
	"thresh_kn_under_and": {"Spend path: 1", "  1: 3 key(s), custom"},
	// Four chained 20-of-20 multis over the same 20 keys (md review r2 m-2):
	// every key signs, and four thresholds have no single k-of-n, so the
	// count is honest and the threshold is "custom". Its five-multi twin is
	// over the 100-witness-item limit, so it is incomplete and prints nothing.
	"typed_witness_items_84": {"Spend path: 1", "  1: 20 key(s), custom"},
}

// TestPolicySummaryLinesForEveryVendoredCase: every complete case prints its
// golden, a case with an any-of entry and no golden fails, and every incomplete
// case prints NOTHING (the caller then shows "Cannot fully display on-device").
func TestPolicySummaryLinesForEveryVendoredCase(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range summaryCases(t) {
		shape := shapeOfCase(t, c)
		got := policySummaryLines(shape)
		if !c.Shape.Complete {
			if got != nil {
				t.Errorf("%s: incomplete shape printed %q", c.Name, got)
			}
			if _, ok := summaryGoldens[c.Name]; ok {
				t.Errorf("%s: golden for an incomplete case", c.Name)
			}
			continue
		}
		want, ok := summaryGoldens[c.Name]
		if !ok {
			t.Errorf("%s: complete case with no golden (any-of entries must each have one)", c.Name)
			continue
		}
		seen[c.Name] = true
		if !slices.Equal(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", c.Name, got, want)
		}
	}
	for name := range summaryGoldens {
		if !seen[name] {
			t.Errorf("golden %s names no complete vendored case", name)
		}
	}
}

// TestAnyOfEntryWording pins the format's edges the vendored cases do not
// reach: plural and singular of each count, and the floor at zero keys.
func TestAnyOfEntryWording(t *testing.T) {
	for _, tc := range []struct {
		k, n, keys int
		want       string
	}{
		{1, 2, 2, "any 1 of these 2 keys"},
		{31, 32, 32, "any 31 of these 32 keys"},
		{2, 3, 1, "any 2 of these 3: 1 key, 2 timelocks; NO KEY NEEDED"},
		{3, 4, 2, "any 3 of these 4: 2 keys, 2 timelocks; at least 1 key"},
		{4, 5, 4, "any 4 of these 5: 4 keys, 1 timelock; at least 3 keys"},
		{1, 3, 0, "any 1 of these 3: 3 timelocks; NO KEY NEEDED"},
		{30, 32, 22, "any 30 of these 32: 22 keys, 10 timelocks; at least 20 keys"},
	} {
		b := md.Branch{AnyK: tc.k, AnyN: tc.n, Keys: tc.keys, Timelock: tc.keys < tc.n}
		if got := anyOfEntry(b); got != tc.want {
			t.Errorf("k=%d n=%d keys=%d: got %q, want %q", tc.k, tc.n, tc.keys, got, tc.want)
		}
	}
}

// TestAnyOfEntryFitsTheReviewScreen is DESIGN_thresh_summary §3's exhaustive
// fit test: every entry the wire allows -- each (k, keys, timelocks) with
// 2 <= n <= 32 and 1 <= k < n, 11,408 entries -- is ONE line under entry
// numbers 1 to 9 and at most TWO under 10 to 999. A label that wraps stays one
// label (the pager never splits one across pages), so two lines is harmless;
// three is refused here.
//
// EVERY ENTRY NUMBER IS COVERED, BUT NOT EVERY ONE IS LAID OUT. Laying out
// 11,408 x 999 labels takes many minutes, so entry numbers are grouped by the
// exact fixed-point advance of their prefix "  N: " under the real layout
// engine (kerning included), and one member of each group is measured on
// every entry. The grouping is sound because text.Layout breaks on the
// cumulative advance alone, and the prefix sits wholly on the first line ahead
// of the entry's first break: two prefixes with the same advance give the same
// breaks. It is also CHECKED: a second member of each group is measured on a
// spread of entries and must agree.
func TestAnyOfEntryFitsTheReviewScreen(t *testing.T) {
	m := newSummaryMeasure()
	var entries []string
	for n := 2; n <= 32; n++ {
		for keys := 0; keys <= n; keys++ {
			for k := 1; k < n; k++ {
				b := md.Branch{AnyK: k, AnyN: n, Keys: keys, Timelock: keys < n}
				entries = append(entries, anyOfEntry(b))
			}
		}
	}
	if len(entries) != 11408 {
		t.Fatalf("enumerated %d entries, design §3 counts 11,408", len(entries))
	}
	type group struct{ ords []int }
	groups := map[fixed.Int26_6]*group{}
	var keys []fixed.Int26_6
	for ord := 1; ord <= 999; ord++ {
		adv := m.advance(fmt.Sprintf("  %d: ", ord))
		g, ok := groups[adv]
		if !ok {
			g = &group{}
			groups[adv] = g
			keys = append(keys, adv)
		}
		g.ords = append(g.ords, ord)
	}
	check := func(ord int, e string) int {
		s := fmt.Sprintf("  %d: %s", ord, e)
		lines := m.lines(t, s)
		if ord <= 9 && lines != 1 {
			t.Fatalf("%q is %d lines; entry numbers 1-9 must fit one", s, lines)
		}
		if lines > 2 {
			t.Fatalf("%q is %d lines; at most two", s, lines)
		}
		return lines
	}
	for _, adv := range keys {
		g := groups[adv]
		// Entry numbers 1-9 carry the stricter bound, so each is measured
		// itself, not through a group representative.
		measured := map[int]bool{}
		for _, ord := range g.ords {
			if ord > 9 && len(measured) > 0 {
				break
			}
			if ord <= 9 || len(measured) == 0 {
				for _, e := range entries {
					check(ord, e)
				}
				measured[ord] = true
			}
		}
		rep := g.ords[0]
		for _, other := range g.ords[1:] {
			if measured[other] {
				continue
			}
			for i := 0; i < len(entries); i += 37 {
				if a, b := check(rep, entries[i]), check(other, entries[i]); a != b {
					t.Fatalf("entry numbers %d and %d share a prefix advance but %q wraps to %d and %d lines",
						rep, other, entries[i], a, b)
				}
			}
			break
		}
	}
	t.Logf("%d entries x 999 entry numbers, %d prefix-advance groups", len(entries), len(keys))
}

// summaryMeasure counts the lines confirmReviewScreen draws for one summary
// line: the same face and style (ctx.Styles.body) and the same wrap width
// (multisig_build.go `lineWidth := dims.X - 2*8`) on the SH2 panel. Labelw's
// height is ascent + (lines-1)*lineHeight + descent, so the count is exact,
// not a rounding of the height.
type summaryMeasure struct {
	buf   op.Buffer
	style text.Style
	width int
	one   int
	lh    int
}

func newSummaryMeasure() *summaryMeasure {
	m := &summaryMeasure{style: NewStyles().body, width: sh2DisplaySize.X - 2*8}
	m.lh = m.style.LineHeight()
	_, sz := widget.Labelw(&m.buf, m.style, m.width, color.RGBA{}, "X")
	m.one = sz.Y
	return m
}

// advance is the exact fixed-point advance of s under the real layout engine.
func (m *summaryMeasure) advance(s string) fixed.Int26_6 {
	l := &text.Layout{MaxWidth: math.MaxInt, Style: m.style}
	var end fixed.Int26_6
	for {
		g, ok := l.Next("%s", s)
		if !ok {
			return end
		}
		if g.Rune != '\n' {
			end = g.Dot + g.Advance
		}
	}
}

func (m *summaryMeasure) lines(t *testing.T, s string) int {
	m.buf.Reset()
	_, sz := widget.Labelw(&m.buf, m.style, m.width, color.RGBA{}, s)
	extra := sz.Y - m.one
	if extra < 0 || extra%m.lh != 0 {
		t.Fatalf("%q: height %d is not %d + a whole number of %d-px lines", s, sz.Y, m.one, m.lh)
	}
	return 1 + extra/m.lh
}
