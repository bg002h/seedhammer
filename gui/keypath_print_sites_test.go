package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// F-449 stage 4: SPEC §7 on the device -- the key-path print sites, the
// class-2 + unlocked-path rulings (and F-644's device half), the inspect
// screen, and §7a.3's engrave half.

func composerTrPreset(t *testing.T, name string) md.PathList {
	t.Helper()
	for _, p := range composerPresets(md.ComposeTr) {
		if p.name == name {
			return p.list
		}
	}
	t.Fatalf("no tr preset %q", name)
	return md.PathList{}
}

func lianaKofnChunks(t *testing.T, kind md.UnspendableKind) []string {
	t.Helper()
	c, err := md.ComposeWithUnspendable(composerTrPreset(t, "kofn-recovery"), make([]*md.SlotOrigin, 4), kind)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	return chunks
}

// TestEveryKeyPathPrintSiteNamesTheLianaKind: the three surfaces that switch on
// the key path with no default (fable M-3) each print a kind-1 line, and the
// consent screen does not show the outside-Liana notice for a shape Liana
// imports.
//
// Mutations: deleting the KeyPathLianaUnspendable arm in composerConsentLinesFor,
// in policySummaryLines, or in md1KeyPathLine each fails its own row.
func TestEveryKeyPathPrintSiteNamesTheLianaKind(t *testing.T) {
	chunks := lianaKofnChunks(t, md.UnspendableLiana)

	consent, err := composerConsentLinesFor(chunks, nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	drawn := normalizeDrawn(strings.Join(consent, "\n"))
	if !strings.Contains(drawn, normalizeDrawn(composerCopyLianaKeyPath())) {
		t.Errorf("consent: no kind-1 key-path line:\n%s", strings.Join(consent, "\n"))
	}
	if strings.Contains(drawn, normalizeDrawn("OUTSIDE LIANA'S MODEL")) {
		t.Errorf("consent: the outside-Liana notice fired on a shape Liana v15.0 imports")
	}

	shape, err := md.PolicyShapeChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if lines := policySummaryLines(shape); len(lines) == 0 || lines[0] != "Key-path: none (Liana key)" {
		t.Errorf("template summary: the key-path line is not first, or not kind 1: %q", lines)
	}

	tpl, _, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range md1Summary(tpl) {
		found = found || l == "Key path: Liana key"
	}
	// md1PolicyFlow hard-chunks every line longer than 20 bytes, mid-word, so
	// each key-path line this stage adds must fit one chunk.
	for _, kp := range []md.KeyPathKind{md.KeyPathNone, md.KeyPathNUMS, md.KeyPathSpendable, md.KeyPathLianaUnspendable} {
		if l, _ := md1KeyPathLine(md.Template{Root: md.ScriptTr, KeyPath: kp}); len(l) > 20 {
			t.Errorf("key-path line %q is %d bytes; md1PolicyFlow hard-chunks at 20", l, len(l))
		}
	}
	if !found {
		t.Errorf("md1Summary names no Liana key path: %q", md1Summary(tpl))
	}
	for kind, want := range map[md.UnspendableKind]string{md.UnspendableNums: "Key path: NUMS"} {
		tpl0, _, err := md.ExpandWalletPolicyChunks(lianaKofnChunks(t, kind))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(md1Summary(tpl0), "|"), want) {
			t.Errorf("md1Summary at kind 0 does not say %q: %q", want, md1Summary(tpl0))
		}
	}
}

// TestComposerLianaClassRulings is SPEC §7's two rulings, stated separately
// because they are separate decisions -- and F-644's device half.
//
// Class 2 is skipped for the Liana kind (it is what the kind is for), and the
// Liana kind is NOT an unlocked path (an unspendable key spends nothing).
// §7's constructed shape -- one timelocked leaf -- is class 7 at kind 1;
// Liana v15.0 refuses it (design/evidence/f449-stage4/liana-probes-out.jsonl,
// `s7-single-timelocked-leaf`).
//
// The four F-644 shapes (md-cli composes them with no warning) each name a
// class here, and Liana v15.0 refuses each with its own recomputed key (same
// evidence file), so the device's choice screen never offers the Liana key
// for any of them.
//
// Mutations: `shape.KeyPath == md.KeyPathSpendable ||
// shape.KeyPath == md.KeyPathLianaUnspendable` in the unlocked count turns the
// §7 row "" (the composer would call the wallet Liana-compatible before
// steel); class 2 as `shape.KeyPath != md.KeyPathSpendable` turns kofn's kind-1
// row into "NUMS key path".
func TestComposerLianaClassRulings(t *testing.T) {
	one := func() md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: 1, N: 1}} }
	multi := func(k, n uint8) md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: k, N: n}} }
	locked := func(p md.SpendPath, kind md.LockKind, v uint32) md.SpendPath {
		p.Lock = &md.Lock{Kind: kind, Value: v}
		return p
	}
	for _, tc := range []struct {
		what string
		list md.PathList
		kind md.UnspendableKind
		want string
	}{
		{"kofn at kind 0: class 2 applies", composerTrPreset(t, "kofn-recovery"), md.UnspendableNums, "NUMS key path"},
		{"kofn at kind 1: class 2 skipped", composerTrPreset(t, "kofn-recovery"), md.UnspendableLiana, ""},
		{"SPEC §7 constructed: one timelocked leaf", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			locked(one(), md.LockOlderBlocks, 26280)}}, md.UnspendableLiana, "no unlocked path"},
		{"F-644 1: two unlocked primaries", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			multi(2, 3), multi(2, 2)}}, md.UnspendableLiana, "no locked path"},
		{"F-644 1b: two unlocked primaries plus a recovery", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			multi(2, 3), multi(2, 2), locked(one(), md.LockOlderBlocks, 26280)}}, md.UnspendableLiana, "a second unlocked path"},
		{"F-644 2: absolute-timelock recovery", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			multi(2, 2), locked(one(), md.LockAfterHeight, 800000)}}, md.UnspendableLiana, "an absolute lock"},
		{"F-644 3: duplicate recovery timelock", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			multi(2, 2), locked(one(), md.LockOlderBlocks, 100), locked(one(), md.LockOlderBlocks, 100)}}, md.UnspendableLiana, "two paths with one lock"},
		{"F-644 4: no recovery path", md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
			multi(2, 3)}}, md.UnspendableLiana, "no locked path"},
	} {
		n, err := md.ValidatePathList(tc.list)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		c, err := md.ComposeWithUnspendable(tc.list, make([]*md.SlotOrigin, n), tc.kind)
		if err != nil {
			t.Fatalf("%s: compose: %v", tc.what, err)
		}
		chunks, err := c.Chunks()
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		shape, err := md.PolicyShapeChunks(chunks)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		if got := composerLianaOutsideModelClass(md.ScriptTr, shape); got != tc.want {
			t.Errorf("%s: class %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestAnUnnamedKeyPathIsRefusedBeforeConsent is SPEC §7a.3's engrave half: a
// keyed tr policy whose key-path kind md could not name is refused, never
// engraved without its proof. No decoder yields one today (md's
// TestAnUnknownInternalKeyKindIsNotDescribed pins that rootKeyPath reports it
// as KeyPathNone), so the Template is constructed.
//
// Mutation: md1KeyPathUnknown returning false fails.
func TestAnUnnamedKeyPathIsRefusedBeforeConsent(t *testing.T) {
	if !md1KeyPathUnknown(md.Template{Root: md.ScriptTr, KeyPath: md.KeyPathNone}) {
		t.Fatal("a tr policy with no named key path is not refused")
	}
	for _, kp := range []md.KeyPathKind{md.KeyPathNUMS, md.KeyPathSpendable, md.KeyPathLianaUnspendable} {
		if md1KeyPathUnknown(md.Template{Root: md.ScriptTr, KeyPath: kp}) {
			t.Errorf("a named key path %d is refused", kp)
		}
	}
	if md1KeyPathUnknown(md.Template{Root: md.ScriptWsh}) {
		t.Error("a wsh policy is refused for having no key path")
	}
}

// TestInspectNamesTheLianaKindAndItsPolicyId is SPEC §7's md1Summary ruling on
// the screen an operator uses a year later: a KEYED kind-1 card read through the
// gather path (gatheredDescriptorFlow -> complexAddressSource -> md1PolicyFlow)
// shows its Policy id AND names the Liana key path. At stage 3 it showed
// neither (stage 3 R0 m2), because the device could not derive kind 1 and so
// fell to "Complex policy - display only".
//
// Mutation: taprootInternalKey's Liana arm returning errUnderivableInternalKey
// sends the card back to "display only" and fails on the Policy id.
func TestInspectNamesTheLianaKindAndItsPolicyId(t *testing.T) {
	chunks := loadVectorChunks(t, "keyed_tr_liana_kofn_recovery")
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() { gatheredDescriptorFlow(ctx, &descriptorTheme, chunks) })
	defer quit()
	var seen strings.Builder
	for page := 0; page < 8; page++ {
		c, ok := frame()
		if !ok {
			break
		}
		seen.WriteString(c)
		if uiContains(seen.String(), "Key path: Liana key") {
			break
		}
		tapNavSlot(t, ctx, drawer(), Button3) // page, BY TOUCH (R0 m2)
	}
	all := seen.String()
	if uiContains(all, "display only") {
		t.Fatalf("a kind-1 keyed card fell to display-only:\n%q", all)
	}
	for _, want := range []string{"Policy id:", "Key path: Liana key"} {
		if !uiContains(all, want) {
			t.Errorf("the inspect screen does not show %q:\n%q", want, all)
		}
	}
}
