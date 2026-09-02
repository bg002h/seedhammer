package md

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ─── the primary's family(), mirrored ─────────────────────────────────────────
//
// Each row is (name, path list, tags) exactly as descriptor-mnemonic
// crates/md-codec/tests/compose_support.rs::family() has it at 66bdf2f4. The
// 26 corpus names are vendored (Task 1); the two `no-corpus` rows are pinned
// by the chunk-set literals below, because the primary's exporter refuses a
// signature-free path and cannot write them to MANIFEST.

var composeH = func() *[32]byte {
	var h [32]byte
	for i := range h {
		h[i] = 0xa8
	}
	return &h
}()

func ck(k, n uint8) SpendPath                           { return SpendPath{Keys: &KeySet{K: k, N: n, Sorted: true}} }
func cu(k, n uint8) SpendPath                           { return SpendPath{Keys: &KeySet{K: k, N: n, Sorted: false}} }
func clk(p SpendPath, l Lock) SpendPath                 { l2 := l; p.Lock = &l2; return p }
func chs(p SpendPath) SpendPath                         { p.Hash = composeH; return p }
func ckl(l *Lock) SpendPath                             { return SpendPath{Hash: composeH, Lock: l} }
func cpl(w ComposeWrapper, paths ...SpendPath) PathList { return PathList{Wrapper: w, Paths: paths} }

func olderBlocks(n uint32) Lock { return Lock{Kind: LockOlderBlocks, Value: n} }
func olderUnits(n uint32) Lock  { return Lock{Kind: LockOlderUnits, Value: n} }
func afterHeight(n uint32) Lock { return Lock{Kind: LockAfterHeight, Value: n} }
func afterTime(n uint32) Lock   { return Lock{Kind: LockAfterTime, Value: n} }

type composeFamilyRow struct {
	name string
	list PathList
	tags []string
}

func composeFamily() []composeFamilyRow {
	eight := func(mk func(i uint32) SpendPath) []SpendPath {
		out := make([]SpendPath, 8)
		for i := range out {
			out[i] = mk(uint32(i))
		}
		return out
	}
	tr32 := eight(func(uint32) SpendPath { return ck(4, 4) })
	return []composeFamilyRow{
		{"keyed_compose_wsh_sole_sortedmulti", cpl(ComposeWsh, ck(2, 3)),
			[]string{"w:wsh", "paths:1", "head:bare-multi", "lock:none", "sorted", "ik:none", "fp:one-seed-one-path", "origins:default-wsh"}},
		{"keyed_compose_wsh_two_path_or_d", cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))),
			[]string{"w:wsh", "paths:2", "head:bare-multi", "lock:blocks", "ik:none", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-wsh"}},
		{"keyed_compose_wsh_two_path_distinct_fingerprints", cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))),
			[]string{"w:wsh", "paths:2", "head:bare-multi", "lock:blocks", "ik:none", "fp:distinct", "origins:default-wsh"}},
		{"keyed_compose_wsh_single_head_or_i", cpl(ComposeWsh, ck(1, 1), clk(ck(1, 1), olderUnits(15188))),
			[]string{"w:wsh", "paths:2", "head:single", "lock:units", "ik:none", "fp:one-seed-two-paths", "origins:default-wsh"}},
		{"keyed_compose_wsh_locked_head_or_i", cpl(ComposeWsh, clk(ck(2, 2), afterHeight(905_000)), ck(1, 1)),
			[]string{"w:wsh", "paths:2", "head:locked", "lock:height", "ik:none", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-wsh"}},
		{"keyed_compose_wsh_hash_and_time", cpl(ComposeWsh, ck(1, 1), clk(chs(ck(2, 2)), afterTime(1_893_456_000))),
			[]string{"w:wsh", "paths:2", "head:single", "lock:time", "hash", "ik:none", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-wsh"}},
		{"keyed_compose_wsh_three_paths", cpl(ComposeWsh, ck(1, 1), clk(ck(1, 1), olderBlocks(4032)), clk(ck(1, 1), afterHeight(1_000_000))),
			[]string{"w:wsh", "paths:3", "head:single", "lock:blocks", "lock:height", "ik:none", "fp:one-seed-two-paths", "origins:default-wsh"}},
		{"keyed_compose_wsh_unsorted_sole", cpl(ComposeWsh, cu(2, 3)),
			[]string{"w:wsh", "paths:1", "head:bare-multi", "lock:none", "unsorted", "ik:none", "fp:one-seed-one-path", "origins:default-wsh"}},
		{"keyed_compose_sh_wsh_sole", cpl(ComposeShWsh, ck(2, 3)),
			[]string{"w:sh-wsh", "paths:1", "head:bare-multi", "lock:none", "sorted", "ik:none", "fp:one-seed-one-path", "origins:default-sh-wsh"}},
		{"keyed_compose_sh_wsh_one_of_two", cpl(ComposeShWsh, ck(1, 2)),
			[]string{"w:sh-wsh", "paths:1", "head:bare-multi", "lock:none", "sorted", "ik:none", "fp:one-seed-one-path", "origins:default-sh-wsh"}},
		{"keyed_compose_sh_sole", cpl(ComposeSh, ck(2, 2)),
			[]string{"w:sh", "paths:1", "head:bare-multi", "lock:none", "sorted", "ik:none", "fp:one-seed-one-path", "origins:default-sh"}},
		{"keyed_compose_sh_two_of_four", cpl(ComposeSh, ck(2, 4)),
			[]string{"w:sh", "paths:1", "head:bare-multi", "lock:none", "sorted", "ik:none", "fp:one-seed-one-path", "origins:default-sh"}},
		{"keyed_compose_tr_two_path_nums", cpl(ComposeTr, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))),
			[]string{"w:tr", "paths:2", "ik:nums", "spine:2", "lock:blocks", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"keyed_compose_tr_two_path_distinct_fingerprints", cpl(ComposeTr, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))),
			[]string{"w:tr", "paths:2", "ik:nums", "spine:2", "lock:blocks", "fp:distinct", "origins:default-tr"}},
		{"keyed_compose_tr_extracted_first", cpl(ComposeTr, ck(1, 1), clk(ck(1, 1), olderBlocks(65535))),
			[]string{"w:tr", "paths:2", "ik:extracted-first", "spine:1", "lock:blocks", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"keyed_compose_tr_extracted_later_four_paths", cpl(ComposeTr, clk(ck(1, 1), olderBlocks(10)), clk(ck(1, 1), afterHeight(1_000_000)), ck(1, 1), clk(ck(1, 1), olderUnits(100))),
			[]string{"w:tr", "paths:4", "ik:extracted-later", "spine:3", "lock:blocks", "lock:height", "lock:units", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"keyed_compose_tr_three_paths_extracted_later", cpl(ComposeTr, clk(ck(1, 1), olderBlocks(10)), ck(1, 1), clk(ck(1, 1), olderUnits(5))),
			[]string{"w:tr", "paths:3", "ik:extracted-later", "spine:2", "lock:blocks", "lock:units", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"keyed_compose_tr_nums_three_leaves", cpl(ComposeTr, clk(ck(1, 1), olderBlocks(1)), clk(ck(1, 1), olderBlocks(2)), clk(ck(2, 2), afterHeight(2))),
			[]string{"w:tr", "paths:3", "ik:nums", "spine:3", "lock:blocks", "lock:height", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"keyed_compose_tr_sole_sortedmulti_a", cpl(ComposeTr, ck(2, 3)),
			[]string{"w:tr", "paths:1", "ik:nums", "spine:1", "lock:none", "sorted", "fp:one-seed-one-path", "origins:default-tr"}},
		{"keyed_compose_tr_key_path_only", cpl(ComposeTr, ck(1, 1)),
			[]string{"w:tr", "paths:1", "ik:extracted-first", "spine:0", "lock:none", "origins:default-tr"}},
		{"keyed_compose_tr_unsorted_sole_leaf", cpl(ComposeTr, cu(2, 2)),
			[]string{"w:tr", "paths:1", "ik:nums", "spine:1", "lock:none", "unsorted", "fp:one-seed-one-path", "origins:default-tr"}},
		{"keyed_compose_tr_hash_leaf", cpl(ComposeTr, ck(2, 2), clk(chs(ck(1, 1)), afterTime(1_893_456_000))),
			[]string{"w:tr", "paths:2", "ik:nums", "spine:2", "hash", "lock:time", "fp:one-seed-one-path", "fp:one-seed-two-paths", "origins:default-tr"}},
		{"compose_wsh_keyless_hash_path", cpl(ComposeWsh, ck(2, 3), ckl(&Lock{Kind: LockAfterHeight, Value: 1_383_520})),
			[]string{"w:wsh", "paths:2", "head:bare-multi", "keyless-wsh", "hash", "lock:height", "ik:none", "fp:none", "origins:default-wsh", "no-corpus"}},
		{"compose_wsh_keyless_hash_only", cpl(ComposeWsh, ck(1, 1), ckl(nil)),
			[]string{"w:wsh", "paths:2", "head:single", "keyless-wsh", "hash", "lock:none", "ik:none", "fp:none", "origins:default-wsh", "no-corpus"}},
		{"compose_wsh_eight_paths", cpl(ComposeWsh, eight(func(i uint32) SpendPath { return clk(ck(1, 1), olderBlocks(100+i)) })...),
			[]string{"w:wsh", "paths:8", "head:locked", "lock:blocks", "ik:none", "fp:none", "origins:default-wsh"}},
		{"compose_tr_seven_leaves", cpl(ComposeTr, eight(func(i uint32) SpendPath {
			if i == 0 {
				return ck(1, 1)
			}
			return clk(ck(1, 1), olderBlocks(100+i))
		})...),
			[]string{"w:tr", "paths:8", "ik:extracted-first", "spine:7", "lock:blocks", "fp:none", "origins:default-tr"}},
		{"compose_wsh_thirty_two_slots", cpl(ComposeWsh, ck(9, 9), ck(9, 9), ck(9, 9), ck(5, 5)),
			[]string{"w:wsh", "paths:4", "slots:32", "head:bare-multi", "lock:none", "ik:none", "fp:none", "origins:default-wsh"}},
		{"compose_tr_thirty_two_slots", cpl(ComposeTr, tr32...),
			[]string{"w:tr", "paths:8", "slots:32", "ik:nums", "spine:7", "lock:none", "fp:none", "origins:default-tr"}},
	}
}

// The two no-corpus entries, as `md encode --experimental --force-chunked`
// printed them at descriptor-mnemonic 66bdf2f4 (2026-09-02).
var noCorpusChunks = map[string][]string{
	"compose_wsh_keyless_hash_path": {
		"md1f8mjcqs9qjtvyyy5jmpprjjtvyy49gqpsfsxpzrye4m29g4z52329g4q6xvdgtdqavtat",
		"md1f8mjcqs252329g4z52329g4z52329g4z52329g4z52329gdsq9guvq2q8uaha9yndk0",
	},
	"compose_wsh_keyless_hash_only": {
		"md1f8kl5qspqztvyyy4qqxpxfdm29g4z52329g4z52329g559vylcxqps8u",
		"md1f8kl5qsf29g4z52329g4z52329g4z52329g4z52sq3yc9v383ler7a",
	},
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func composeTlvPubkeys(d *descriptor) map[uint8][65]byte {
	out := map[uint8][65]byte{}
	for _, p := range d.tlv.pubkeys {
		out[p.idx] = p.xpub
	}
	return out
}

func composeTlvFingerprints(d *descriptor) map[uint8][4]byte {
	out := map[uint8][4]byte{}
	for _, f := range d.tlv.fingerprints {
		out[f.idx] = f.fp
	}
	return out
}

// TestComposeReproducesEveryVectorByteForByte is §12 item 1's Go half: the
// UNSEATED builder output equals the vendored tree, path declaration and
// use-site; after binding the vector's keys and fingerprints (what the MANIFEST
// binding did in Rust) the payload BYTES and the CHUNK STRINGS are identical.
func TestComposeReproducesEveryVectorByteForByte(t *testing.T) {
	for _, row := range composeFamily() {
		t.Run(row.name, func(t *testing.T) {
			c, err := Compose(row.list)
			if err != nil {
				t.Fatalf("Compose: %v", err)
			}
			if hasTag(row.tags, "no-corpus") {
				chunks := noCorpusChunks[row.name]
				want, err := Reassemble(chunks)
				if err != nil {
					t.Fatalf("Reassemble(no-corpus literal): %v", err)
				}
				if !reflect.DeepEqual(c.d.tree, want.tree) {
					t.Fatalf("tree differs from the primary's:\n got %+v\nwant %+v", c.d.tree, want.tree)
				}
				if !reflect.DeepEqual(c.d.pathDecl, want.pathDecl) {
					t.Fatalf("pathDecl differs: got %+v want %+v", c.d.pathDecl, want.pathDecl)
				}
				got, err := c.Chunks()
				if err != nil {
					t.Fatalf("Chunks: %v", err)
				}
				if !reflect.DeepEqual(got, chunks) {
					t.Fatalf("chunks differ:\n got %v\nwant %v", got, chunks)
				}
				return
			}
			want := loadDescriptor(t, row.name)
			if !reflect.DeepEqual(c.d.tree, want.tree) {
				t.Fatalf("tree differs from the vendored descriptor.json:\n got %+v\nwant %+v", c.d.tree, want.tree)
			}
			if !reflect.DeepEqual(c.d.pathDecl, want.pathDecl) {
				t.Fatalf("pathDecl differs:\n got %+v\nwant %+v", c.d.pathDecl, want.pathDecl)
			}
			if !reflect.DeepEqual(c.d.useSite, want.useSite) || c.d.n != want.n {
				t.Fatalf("useSite/n differ: got %+v/%d want %+v/%d", c.d.useSite, c.d.n, want.useSite, want.n)
			}
			if strings.HasPrefix(row.name, "keyed_") {
				if err := c.Bind(composeTlvPubkeys(want), composeTlvFingerprints(want)); err != nil {
					t.Fatalf("Bind: %v", err)
				}
			} else if len(want.tlv.pubkeys) != 0 || len(want.tlv.fingerprints) != 0 {
				t.Fatalf("an unkeyed vector carries keys or fingerprints")
			}
			gotBytes, _, err := encodePayload(c.d)
			if err != nil {
				t.Fatalf("encodePayload: %v", err)
			}
			if wantBytes := loadBytesHex(t, row.name); !bytes.Equal(gotBytes, wantBytes) {
				t.Fatalf("payload bytes differ:\n got %x\nwant %x", gotBytes, wantBytes)
			}
			gotChunks, err := c.Chunks()
			if err != nil {
				t.Fatalf("Chunks: %v", err)
			}
			if wantChunks := loadPhraseChunks(t, row.name); !reflect.DeepEqual(gotChunks, wantChunks) {
				t.Fatalf("chunks differ:\n got %v\nwant %v", gotChunks, wantChunks)
			}
		})
	}
}

// Every tag appears in at least two vectors, except the ones with exactly one
// legal shape (spec §12 item 1; primary's SINGULAR_TAGS = {"spine:0"}).
func TestComposeFamilyTagsAreCoveredTwice(t *testing.T) {
	count := map[string]int{}
	for _, row := range composeFamily() {
		for _, tag := range row.tags {
			count[tag]++
		}
	}
	for tag, n := range count {
		if tag == "spine:0" {
			// The primary's SINGULAR_TAGS: exactly one legal shape, so exactly one row.
			if n != 1 {
				t.Errorf("singular tag %q appears in %d vectors, want exactly 1", tag, n)
			}
			continue
		}
		if n < 2 {
			t.Errorf("tag %q appears in %d vector(s); the two-vector rule wants 2", tag, n)
		}
	}
	if len(composeFamily()) != 28 {
		t.Errorf("family has %d rows, the primary has 28", len(composeFamily()))
	}
}

// Slot numbering is first-appearance in the EMITTED text (§5): the taproot
// internal key's path is numbered first even when listed later.
func TestComposeNumbersSlotsByFirstAppearance(t *testing.T) {
	c, err := Compose(cpl(ComposeTr, clk(ck(1, 1), olderBlocks(10)), ck(1, 1), clk(ck(1, 1), olderUnits(5))))
	if err != nil {
		t.Fatal(err)
	}
	ik, ok := c.InternalKeyPath()
	if !ok || ik != 1 {
		t.Fatalf("internal key path = %d,%v; want path 1", ik, ok)
	}
	want := []ComposeSlot{{Index: 0, Path: 1, Ordinal: 0}, {Index: 1, Path: 0, Ordinal: 0}, {Index: 2, Path: 2, Ordinal: 0}}
	if !reflect.DeepEqual(c.Slots(), want) {
		t.Fatalf("slots = %+v, want %+v", c.Slots(), want)
	}
	// wsh: listed order IS emitted order.
	c, err = Compose(cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))))
	if err != nil {
		t.Fatal(err)
	}
	want = []ComposeSlot{{0, 0, 0}, {1, 0, 1}, {2, 0, 2}, {3, 1, 0}}
	if !reflect.DeepEqual(c.Slots(), want) {
		t.Fatalf("slots = %+v, want %+v", c.Slots(), want)
	}
	if _, ok := c.InternalKeyPath(); ok {
		t.Fatal("a wsh policy reported an internal key path")
	}
}

// §4f: unseated slots take the wrapper's default origin at the LOWEST account
// no other slot holds; a declared slot's origin is respected and skipped over.
func TestComposeWithFillsTheLowestFreeAccount(t *testing.T) {
	list := cpl(ComposeWsh, ck(2, 3))
	acct1 := DefaultOrigin(ComposeWsh, 1)
	c, err := ComposeWith(list, []*SlotOrigin{nil, {Origin: acct1, Fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, FpPresent: true}, nil})
	if err != nil {
		t.Fatal(err)
	}
	got := c.d.pathDecl.divergent
	want := []originPath{
		{components: toComponents(DefaultOrigin(ComposeWsh, 0))},
		{components: toComponents(acct1)},
		{components: toComponents(DefaultOrigin(ComposeWsh, 2))},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins = %+v, want %+v", got, want)
	}
	if !c.d.tlv.fpPresent || len(c.d.tlv.fingerprints) != 1 || c.d.tlv.fingerprints[0].idx != 1 {
		t.Fatalf("fingerprints = %+v, want exactly slot 1's", c.d.tlv.fingerprints)
	}
	// All slots at one declared origin, all with distinct fingerprints: legal,
	// and the path declaration collapses to SHARED.
	shared := DefaultOrigin(ComposeWsh, 0)
	c, err = ComposeWith(list, []*SlotOrigin{
		{Origin: shared, Fingerprint: [4]byte{1}, FpPresent: true},
		{Origin: shared, Fingerprint: [4]byte{2}, FpPresent: true},
		{Origin: shared, Fingerprint: [4]byte{3}, FpPresent: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.d.pathDecl.shared == nil || c.d.pathDecl.divergent != nil {
		t.Fatalf("expected a shared path declaration, got %+v", c.d.pathDecl)
	}
}

func TestComposeDefaultOriginsPerWrapper(t *testing.T) {
	h := func(v uint32) PathComponent { return PathComponent{Hardened: true, Value: v} }
	for _, tc := range []struct {
		w    ComposeWrapper
		want []PathComponent
	}{
		{ComposeWsh, []PathComponent{h(48), h(0), h(5), h(2)}},
		{ComposeSh, []PathComponent{h(48), h(0), h(5), h(2)}},
		{ComposeShWsh, []PathComponent{h(48), h(0), h(5), h(1)}},
		{ComposeTr, []PathComponent{h(48), h(0), h(5), h(3)}},
	} {
		if got := DefaultOrigin(tc.w, 5); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("DefaultOrigin(%v,5) = %+v, want %+v", tc.w, got, tc.want)
		}
	}
	if ComposeTr.ScriptType() != 3 || ComposeShWsh.ScriptType() != 1 || ComposeWsh.ScriptType() != 2 || ComposeSh.ScriptType() != 2 {
		t.Fatal("script-type components do not match §4f")
	}
}

// Every refusal in the primary's validate() refuses here, by sentinel.
func TestComposeRefusesWhatThePrimaryRefuses(t *testing.T) {
	nine := func() []SpendPath {
		out := make([]SpendPath, 9)
		for i := range out {
			out[i] = ck(1, 1)
		}
		return out
	}()
	for _, tc := range []struct {
		name string
		list PathList
		want error
	}{
		{"no paths", cpl(ComposeWsh), ErrComposeNoPaths},
		{"nine paths", cpl(ComposeWsh, nine...), ErrComposeTooManyPaths},
		{"k zero", cpl(ComposeWsh, ck(0, 2)), ErrComposeBadThreshold},
		{"k above n", cpl(ComposeWsh, ck(3, 2)), ErrComposeBadThreshold},
		{"ten keys in a path", cpl(ComposeWsh, ck(1, 10)), ErrComposeBadThreshold},
		{"lock-only path", cpl(ComposeWsh, ck(1, 1), SpendPath{Lock: &Lock{Kind: LockOlderBlocks, Value: 1}}), ErrComposeLockOnlyPath},
		{"keyless under tr", cpl(ComposeTr, ck(1, 1), ckl(nil)), ErrComposeKeylessUnderTr},
		{"no keyed path", cpl(ComposeWsh, ckl(nil)), ErrComposeNoKeyedPath},
		{"33 slots", cpl(ComposeWsh, ck(9, 9), ck(9, 9), ck(9, 9), ck(6, 6)), ErrComposeTooManySlots},
		{"sh with two paths", cpl(ComposeSh, ck(2, 2), ck(1, 1)), ErrComposeLegacyWrapperShape},
		{"sh-wsh unsorted", cpl(ComposeShWsh, cu(2, 2)), ErrComposeLegacyWrapperShape},
		{"sh single key", cpl(ComposeSh, ck(1, 1)), ErrComposeLegacyWrapperShape},
		{"older zero blocks", cpl(ComposeWsh, clk(ck(1, 1), olderBlocks(0))), ErrComposeLockOutOfRange},
		{"older 65536 blocks", cpl(ComposeWsh, clk(ck(1, 1), olderBlocks(65536))), ErrComposeLockOutOfRange},
		{"older zero units", cpl(ComposeWsh, clk(ck(1, 1), olderUnits(0))), ErrComposeLockOutOfRange},
		{"after height at the time threshold", cpl(ComposeWsh, clk(ck(1, 1), afterHeight(500_000_000))), ErrComposeLockOutOfRange},
		{"after time below the threshold", cpl(ComposeWsh, clk(ck(1, 1), afterTime(499_999_999))), ErrComposeLockOutOfRange},
		{"after time above 2^31-1", cpl(ComposeWsh, clk(ck(1, 1), afterTime(2_147_483_648))), ErrComposeLockOutOfRange},
	} {
		_, err := Compose(tc.list)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	// Declared slot count must match.
	if _, err := ComposeWith(cpl(ComposeWsh, ck(2, 3)), []*SlotOrigin{nil, nil}); !errors.Is(err, ErrComposeWrongSlotCount) {
		t.Errorf("wrong slot count: %v", err)
	}
	// Two slots at one origin without two distinct fingerprints (§4f, §8v).
	o := DefaultOrigin(ComposeWsh, 0)
	for _, decl := range [][]*SlotOrigin{
		{{Origin: o}, {Origin: o}, nil},
		{{Origin: o, Fingerprint: [4]byte{1}, FpPresent: true}, {Origin: o}, nil},
		{{Origin: o, Fingerprint: [4]byte{1}, FpPresent: true}, {Origin: o, Fingerprint: [4]byte{1}, FpPresent: true}, nil},
	} {
		if _, err := ComposeWith(cpl(ComposeWsh, ck(2, 3)), decl); !errors.Is(err, ErrComposeIndistinguishableSlots) {
			t.Errorf("indistinguishable slots accepted: %v", err)
		}
	}
}

// §12 item 7: the device-side lock range check, every boundary in and out.
func TestLockCheckIsTheDeviceSideRangeGate(t *testing.T) {
	ok := []Lock{olderBlocks(1), olderBlocks(65535), olderUnits(1), olderUnits(65535), afterHeight(1), afterHeight(499_999_999), afterTime(500_000_000), afterTime(2_147_483_647)}
	bad := []Lock{olderBlocks(0), olderBlocks(65536), olderUnits(0), olderUnits(65536), afterHeight(0), afterHeight(500_000_000), afterTime(499_999_999), afterTime(2_147_483_648), {Kind: LockKind(9), Value: 1}}
	for _, l := range ok {
		if err := l.Check(); err != nil {
			t.Errorf("%+v: %v", l, err)
		}
	}
	for _, l := range bad {
		if err := l.Check(); err == nil {
			t.Errorf("%+v accepted", l)
		}
	}
	// The operand the wire carries: units get the 0x400000 type flag.
	if tag, v, err := olderUnits(15188).operand(); err != nil || tag != tagOlder || v != 4209492 {
		t.Fatalf("older units operand = %v %d %v", tag, v, err)
	}
	// And back: every in-range lock survives operand -> lockFromWire unchanged,
	// so a decoded card names the same kind and value the operator entered.
	for _, l := range ok {
		tag, v, err := l.operand()
		if err != nil {
			t.Fatal(err)
		}
		if got := lockFromWire(tag, v); got != l {
			t.Errorf("%+v -> wire %v %d -> %+v", l, tag, v, got)
		}
	}
}

// Experimental marks mirror the primary's `experimental()`: a keyless path
// always; unsorted keys only where sorted would have been legal (the sole
// bare-multi path).
func TestComposeExperimentalMarks(t *testing.T) {
	c, err := Compose(cpl(ComposeWsh, cu(2, 3)))
	if err != nil {
		t.Fatal(err)
	}
	if want := []ComposeExperimental{{Kind: ExperimentalUnsortedKeys, Path: 0}}; !reflect.DeepEqual(c.Experimental(), want) {
		t.Fatalf("marks = %+v, want %+v", c.Experimental(), want)
	}
	c, err = Compose(cpl(ComposeWsh, cu(2, 3), clk(ck(1, 1), olderBlocks(1))))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Experimental()) != 0 {
		t.Fatalf("multi under or_d is the only legal spelling there; marks = %+v", c.Experimental())
	}
	c, err = Compose(cpl(ComposeWsh, ck(1, 1), ckl(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if want := []ComposeExperimental{{Kind: ExperimentalKeylessPath, Path: 1}}; !reflect.DeepEqual(c.Experimental(), want) {
		t.Fatalf("marks = %+v, want %+v", c.Experimental(), want)
	}
}

// Stub and template id come from the same descriptor the chunks encode, so a
// consumer comparing a card's stub against the composed template agrees with
// the shipped FormAwareStubChunks on the emitted chunks.
func TestComposedStubMatchesTheChunks(t *testing.T) {
	c, err := Compose(cpl(ComposeTr, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	fromChunks, err := FormAwareStubChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	stub, err := c.Stub()
	if err != nil {
		t.Fatal(err)
	}
	if stub != fromChunks {
		t.Fatalf("Stub %x != FormAwareStubChunks %x", stub, fromChunks)
	}
	tid, err := c.TemplateID()
	if err != nil {
		t.Fatal(err)
	}
	if [4]byte(tid[:4]) != stub {
		t.Fatalf("a keyless template's stub is its template id's first four bytes: %x vs %x", tid, stub)
	}
}
