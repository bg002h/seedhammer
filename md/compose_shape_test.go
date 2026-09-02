package md

import (
	"reflect"
	"testing"
)

func shapeOf(t *testing.T, name string) PolicyShape {
	t.Helper()
	s, err := PolicyShapeChunks(loadPhraseChunks(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !s.Complete {
		t.Fatalf("%s: shape reported INCOMPLETE", name)
	}
	return s
}

func lk(kind LockKind, v uint32) []Lock { return []Lock{{Kind: kind, Value: v}} }

func TestPolicyShapeSplitsAlternativesIntoBranches(t *testing.T) {
	h := [32]byte{}
	for i := range h {
		h[i] = 0xa8
	}
	for _, tc := range []struct {
		vector string
		want   []Branch
	}{
		// or_d(multi(2,@0,@1,@2), and_v(v:pkh(@3), older(26280)))
		{"keyed_compose_wsh_two_path_or_d", []Branch{
			{K: 2, N: 3, Keys: 3},
			{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 26280)},
		}},
		// or_i(pkh(@0), and_v(v:pkh(@1), older(4209492))) -- 0x400000 + 15188 units
		{"keyed_compose_wsh_single_head_or_i", []Branch{
			{Keys: 1},
			{Keys: 1, Timelock: true, Locks: lk(LockOlderUnits, 15188)},
		}},
		// or_i(pkh(@0), or_i(and_v(v:pkh(@1),older(4032)), and_v(v:pkh(@2),after(1000000))))
		{"keyed_compose_wsh_three_paths", []Branch{
			{Keys: 1},
			{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 4032)},
			{Keys: 1, Timelock: true, Locks: lk(LockAfterHeight, 1_000_000)},
		}},
		// or_i(pkh(@0), and_v(v:multi(2,@1,@2), and_v(v:sha256(H), after(1893456000))))
		{"keyed_compose_wsh_hash_and_time", []Branch{
			{Keys: 1},
			{Keys: 2, Timelock: true, Hashlock: true, Locks: lk(LockAfterTime, 1_893_456_000), Sha256Digests: [][32]byte{h}},
		}},
		// or_i(and_v(v:multi(2,@0,@1), after(905000)), pkh(@2))
		{"keyed_compose_wsh_locked_head_or_i", []Branch{
			{Keys: 2, Timelock: true, Locks: lk(LockAfterHeight, 905_000)},
			{Keys: 1},
		}},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			s := shapeOf(t, tc.vector)
			if s.KeyPath != KeyPathNone {
				t.Errorf("KeyPath = %v, want none for wsh", s.KeyPath)
			}
			if !reflect.DeepEqual(s.Branches, tc.want) {
				t.Fatalf("branches\n got %+v\nwant %+v", s.Branches, tc.want)
			}
		})
	}
}

// The SHIPPED cards with alternatives (fidelity I-3): these are what the
// template-engrave consent screen (gui/template_engrave.go policySummaryLines)
// will now show as several spend paths instead of one. Pinned so that change
// is a recorded decision, not a side effect.
func TestPolicyShapeSplitsTheShippedOrCards(t *testing.T) {
	var hh [32]byte
	for i, b := range []byte{0xa8, 0x4d, 0xce, 0x40, 0x97, 0x57, 0x27, 0xc3, 0x98, 0x02, 0x3c, 0xfb, 0xd5, 0x0d, 0x5d, 0xb3, 0xb9, 0x66, 0x23, 0x75, 0x52, 0x1d, 0x0f, 0x1a, 0xc6, 0x2d, 0xbd, 0x82, 0x9b, 0x9a, 0x08, 0xad} {
		hh[i] = b
	}
	for _, tc := range []struct {
		vector string
		want   []Branch
	}{
		// wsh(or_b(pk(@0), s:pk(@1)))
		{"keyed_wsh_or_b", []Branch{{Keys: 1}, {Keys: 1}}},
		// wsh(or_d(multi(2,@0,@1), and_v(v:older(65535), pk(@2))))
		{"keyed_wsh_or_d_degrading", []Branch{
			{K: 2, N: 2, Keys: 2},
			{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 65535)},
		}},
		// wsh(or_i(and_v(v:after(1000000), and_v(v:sha256(H), multi(2,@0,@1,@2))), and_v(v:older(65535), multi(1,@1,@2))))
		{"keyed_wsh_timelock_hashlock", []Branch{
			{Keys: 3, Timelock: true, Hashlock: true, Locks: lk(LockAfterHeight, 1_000_000), Sha256Digests: [][32]byte{hh}},
			{Keys: 2, Timelock: true, Locks: lk(LockOlderBlocks, 65535)},
		}},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			s := shapeOf(t, tc.vector)
			if !reflect.DeepEqual(s.Branches, tc.want) {
				t.Fatalf("branches\n got %+v\nwant %+v", s.Branches, tc.want)
			}
		})
	}
}

// Fidelity C-1 made concrete: the same operand under older and under after is
// two different wallets, and the summary must say which.
func TestPolicyShapeDistinguishesOlderFromAfterAtTheSameOperand(t *testing.T) {
	a, err := Compose(cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compose(cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), afterHeight(26280))))
	if err != nil {
		t.Fatal(err)
	}
	sa, sb := policyShape(a.d.tree), policyShape(b.d.tree)
	if reflect.DeepEqual(sa.Branches, sb.Branches) {
		t.Fatalf("older(26280) and after(26280) summarize identically: %+v", sa.Branches)
	}
	if sa.Branches[1].Locks[0].Kind != LockOlderBlocks || sb.Branches[1].Locks[0].Kind != LockAfterHeight {
		t.Fatalf("kinds: %+v / %+v", sa.Branches[1].Locks, sb.Branches[1].Locks)
	}
}

// Fidelity I-1: sortedmulti and multi at the same k-of-n differ in Sorted, so
// §7e's "unsorted where sorted was legal" mark comes from the decoded md1.
func TestPolicyShapeCarriesSortedForThresholds(t *testing.T) {
	sorted := shapeOf(t, "keyed_compose_wsh_sole_sortedmulti")
	unsorted := shapeOf(t, "keyed_compose_wsh_unsorted_sole")
	if !reflect.DeepEqual(sorted.Branches, []Branch{{K: 2, N: 3, Keys: 3, Sorted: true}}) {
		t.Fatalf("sorted: %+v", sorted.Branches)
	}
	if !reflect.DeepEqual(unsorted.Branches, []Branch{{K: 2, N: 3, Keys: 3, Sorted: false}}) {
		t.Fatalf("unsorted: %+v", unsorted.Branches)
	}
	tap := shapeOf(t, "keyed_compose_tr_sole_sortedmulti_a")
	if len(tap.Branches) != 1 || !tap.Branches[0].Sorted || tap.Branches[0].K != 2 {
		t.Fatalf("sortedmulti_a leaf: %+v", tap.Branches)
	}
}

// Eight or_i-chained paths: eight branches, each carrying its own lock.
func TestPolicyShapeWalksAnEightPathChain(t *testing.T) {
	s := shapeOf(t, "compose_wsh_eight_paths")
	if len(s.Branches) != 8 {
		t.Fatalf("branches = %d, want 8 (%+v)", len(s.Branches), s.Branches)
	}
	for i, b := range s.Branches {
		if b.Keys != 1 || !b.Timelock || len(b.Locks) != 1 || b.Locks[0] != (Lock{Kind: LockOlderBlocks, Value: uint32(100 + i)}) {
			t.Errorf("branch %d = %+v, want 1 key, older(%d) blocks", i, b, 100+i)
		}
	}
}

// A taproot leaf list is unchanged by the split (one leaf, one branch) but
// now carries locks: tr(NUMS,{and_v(v:pk(@0),older(1)),{and_v(v:pk(@1),older(2)),and_v(v:multi_a(2,@2,@3),after(2))}}).
func TestPolicyShapeTapLeavesCarryLocks(t *testing.T) {
	s := shapeOf(t, "keyed_compose_tr_nums_three_leaves")
	want := []Branch{
		{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 1), Depth: 1},
		{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 2), Depth: 2},
		// K/N stay ZERO here: the multi_a sits under and_v with the lock, and
		// plainMulti reports a threshold only for a BARE one (the existing
		// TestPolicyShapeNeverClaimsAPlainThresholdItCannotSee rule).
		{Keys: 2, Timelock: true, Locks: lk(LockAfterHeight, 2), Depth: 2},
	}
	if s.KeyPath != KeyPathNUMS || s.TapDepth != 2 {
		t.Fatalf("KeyPath=%v TapDepth=%d, want NUMS depth 2", s.KeyPath, s.TapDepth)
	}
	if !reflect.DeepEqual(s.Branches, want) {
		t.Fatalf("branches\n got %+v\nwant %+v", s.Branches, want)
	}
}

// The keyless-wsh no-corpus shape: the second alternative has NO key, and the
// summary must say so (Keys 0) rather than refuse -- it is a legal EXPERIMENTAL
// wallet the operator asked for, and hiding the bearer path would be the lie.
func TestPolicyShapeReportsAKeylessAlternativeHonestly(t *testing.T) {
	s, err := PolicyShapeChunks(noCorpusChunks["compose_wsh_keyless_hash_path"])
	if err != nil {
		t.Fatal(err)
	}
	if !s.Complete || len(s.Branches) != 2 {
		t.Fatalf("Complete=%v branches=%d", s.Complete, len(s.Branches))
	}
	if b := s.Branches[1]; b.Keys != 0 || !b.Hashlock || !b.Timelock || len(b.Sha256Digests) != 1 || !reflect.DeepEqual(b.Locks, lk(LockAfterHeight, 1_383_520)) {
		t.Fatalf("keyless branch = %+v", b)
	}
}

// andor(X,Y,Z) is (X and Y) or Z: two branches, hand-built because no vector
// carries one.
func TestPolicyShapeSplitsAndOr(t *testing.T) {
	x := node{tag: tagPkK, body: keyArgBody{index: 0}}
	y := node{tag: tagOlder, body: timelockBody(144)}
	z := node{tag: tagPkH, body: keyArgBody{index: 1}}
	tree := node{tag: tagWsh, body: childrenBody{children: []node{{tag: tagAndOr, body: childrenBody{children: []node{x, y, z}}}}}}
	s := policyShape(tree)
	want := []Branch{{Keys: 1, Timelock: true, Locks: lk(LockOlderBlocks, 144)}, {Keys: 1}}
	if !s.Complete || !reflect.DeepEqual(s.Branches, want) {
		t.Fatalf("Complete=%v branches\n got %+v\nwant %+v", s.Complete, s.Branches, want)
	}
}
