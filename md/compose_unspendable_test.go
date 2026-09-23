package md

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

// F-449 stage 4: the Go composer's Liana request, bound to the Rust primary's
// kind-1 vectors (stage 3 vendored them), and SPEC §6's mint refusals.

// lianaComposeRows are the two keyed kind-1 vectors the Rust primary minted
// (descriptor-mnemonic test_vectors.rs, F-449 stage 3 Task 1) as COMPOSER path
// lists. Both use the tr default origins (48'/0'/i'/3') with every slot
// unseated, which is what those MANIFEST entries declare.
func lianaComposeRows() []struct {
	name string
	list PathList
} {
	return []struct {
		name string
		list PathList
	}{
		{"keyed_tr_liana_kofn_recovery", cpl(ComposeTr, cu(2, 3), clk(ck(1, 1), olderBlocks(26280)))},
		{"keyed_tr_liana_nested_two_recoveries", cpl(ComposeTr, cu(2, 2), clk(ck(1, 1), olderBlocks(26280)), clk(ck(1, 1), olderBlocks(52560)))},
	}
}

// TestComposeLianaReproducesTheRustVectors is the Go composer's Rust-primary
// binding: composing the preset with the Liana request, then binding the
// vector's keys, yields the primary's payload bytes and chunk strings.
//
// Mutation: lowerTr's `case UnspendableLiana:` arm yielding InternalKeyNUMS
// fails here (the bytes are the kind-0 twin's).
func TestComposeLianaReproducesTheRustVectors(t *testing.T) {
	for _, row := range lianaComposeRows() {
		t.Run(row.name, func(t *testing.T) {
			want := loadDescriptor(t, row.name)
			c, err := ComposeWithUnspendable(row.list, make([]*SlotOrigin, want.n), UnspendableLiana)
			if err != nil {
				t.Fatalf("ComposeWithUnspendable: %v", err)
			}
			if !reflect.DeepEqual(c.d.tree, want.tree) {
				t.Fatalf("tree differs from the vendored descriptor.json:\n got %+v\nwant %+v", c.d.tree, want.tree)
			}
			if err := c.Bind(composeTlvPubkeys(want), composeTlvFingerprints(want)); err != nil {
				t.Fatalf("Bind: %v", err)
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
			if c.UnspendableRequestUnmet() {
				t.Fatal("a met Liana request reports itself unmet")
			}
		})
	}
}

// TestComposeWithIsTheNumsRequest pins that every pre-F-449 caller composes
// what it always composed: ComposeWith == ComposeWithUnspendable(…, Nums).
func TestComposeWithIsTheNumsRequest(t *testing.T) {
	list := lianaComposeRows()[0].list
	a, err := ComposeWith(list, make([]*SlotOrigin, 4))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComposeWithUnspendable(list, make([]*SlotOrigin, 4), UnspendableNums)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.d.tree, b.d.tree) || a.internalKey() != InternalKeyNUMS {
		t.Fatalf("ComposeWith is not the NUMS request: %+v vs %+v", a.d.tree, b.d.tree)
	}
}

// TestALianaRequestOverARealKeyIsUnmet is SPEC §6 row 3 in Go: the first bare
// single key becomes the internal key, the request has nothing to select, and
// the composition says so instead of silently ignoring it.
//
// Mutation: UnspendableRequestUnmet returning false fails the first row;
// returning `c.requested == UnspendableLiana` fails the kofn row.
func TestALianaRequestOverARealKeyIsUnmet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		list  PathList
		kind  UnspendableKind
		unmet bool
		ik    InternalKeyKind
	}{
		{"real key, liana", cpl(ComposeTr, ck(1, 1), clk(ck(1, 1), olderBlocks(26280))), UnspendableLiana, true, InternalKeySlot},
		{"real key, nums", cpl(ComposeTr, ck(1, 1), clk(ck(1, 1), olderBlocks(26280))), UnspendableNums, false, InternalKeySlot},
		{"kofn, liana", cpl(ComposeTr, cu(2, 3), clk(ck(1, 1), olderBlocks(26280))), UnspendableLiana, false, InternalKeyLianaUnspendable},
		{"wsh, liana", cpl(ComposeWsh, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))), UnspendableLiana, true, InternalKeySlot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, err := ValidatePathList(tc.list)
			if err != nil {
				t.Fatal(err)
			}
			c, err := ComposeWithUnspendable(tc.list, make([]*SlotOrigin, n), tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.UnspendableRequestUnmet(); got != tc.unmet {
				t.Fatalf("UnspendableRequestUnmet = %v, want %v", got, tc.unmet)
			}
			if got := c.internalKey(); got != tc.ik {
				t.Fatalf("built internal key = %d, want %d", got, tc.ik)
			}
		})
	}
}

// TestValidateUnspendableShapeRefusesSpecSixRows is the port of md-codec's
// validate_unspendable_shape, row by row. Row 1 is reachable through the
// composer's API (a sole k-of-n under tr lowers to sortedmulti_a); rows 2 and
// 4 are reached here by editing the built descriptor, because the composer
// cannot produce them -- see TestTheComposerCannotReachSpecSixRowsTwoAndFour.
//
// Mutations: deleting each `return Err…` in validateUnspendableShape fails the
// matching row; isStandardMultipath returning true fails both use-site rows.
func TestValidateUnspendableShapeRefusesSpecSixRows(t *testing.T) {
	compose := func(t *testing.T, list PathList, kind UnspendableKind) Composed {
		t.Helper()
		n, err := ValidatePathList(list)
		if err != nil {
			t.Fatal(err)
		}
		c, err := ComposeWithUnspendable(list, make([]*SlotOrigin, n), kind)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	kofn := cpl(ComposeTr, cu(2, 3), clk(ck(1, 1), olderBlocks(26280)))
	if err := compose(t, kofn, UnspendableLiana).ValidateUnspendableShape(); err != nil {
		t.Fatalf("kofn-recovery at kind 1 refused: %v", err)
	}
	if err := compose(t, cpl(ComposeTr, ck(2, 3)), UnspendableNums).ValidateUnspendableShape(); err != nil {
		t.Fatalf("a kind-0 sortedmulti_a refused: %v (the rows bind kind 1 only)", err)
	}
	if err := compose(t, cpl(ComposeTr, ck(2, 3)), UnspendableLiana).ValidateUnspendableShape(); !errors.Is(err, ErrUnspendableSortedMultiA) {
		t.Fatalf("row 1: got %v, want ErrUnspendableSortedMultiA", err)
	}

	c := compose(t, kofn, UnspendableLiana)
	c.d.useSite.multipath = []alternative{{value: 2}, {value: 3}}
	if err := c.ValidateUnspendableShape(); !errors.Is(err, ErrUnspendableUseSite) {
		t.Fatalf("row 2 (shared <2;3>): got %v", err)
	}
	c = compose(t, kofn, UnspendableLiana)
	c.d.tlv.useSiteOverrides = []idxUseSite{{idx: 1, path: useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}, wildcardHardened: true}}}
	if err := c.ValidateUnspendableShape(); !errors.Is(err, ErrUnspendableUseSite) {
		t.Fatalf("row 2 (override <0;1>/*h): got %v", err)
	}

	c = compose(t, kofn, UnspendableLiana)
	c.d.tree = node{tag: tagWsh, body: childrenBody{children: []node{c.d.tree}}}
	if err := c.ValidateUnspendableShape(); !errors.Is(err, ErrUnspendableNotRootTr) {
		t.Fatalf("row 4 (wsh(tr(liana))): got %v", err)
	}
}

// TestTheComposerCannotReachSpecSixRowsTwoAndFour pins WHY rows 2 and 4 are
// refusals the composer never meets rather than dead guards: finishComposed
// writes the <0;1>/* use-site and no override, and every lowering puts tr at
// the root. If either ever changes, this fails and the rows become reachable.
//
// Mutation: finishComposed writing {0,1} with wildcardHardened: true fails.
func TestTheComposerCannotReachSpecSixRowsTwoAndFour(t *testing.T) {
	for _, list := range []PathList{
		cpl(ComposeTr, cu(2, 3), clk(ck(1, 1), olderBlocks(26280))),
		cpl(ComposeTr, cu(2, 2), clk(cu(1, 2), olderBlocks(26280))),
		cpl(ComposeTr, ck(2, 3)),
	} {
		n, err := ValidatePathList(list)
		if err != nil {
			t.Fatal(err)
		}
		c, err := ComposeWithUnspendable(list, make([]*SlotOrigin, n), UnspendableLiana)
		if err != nil {
			t.Fatal(err)
		}
		if !isStandardMultipath(c.d.useSite) || c.d.tlv.useSiteOverrides != nil {
			t.Fatalf("the composer wrote a use-site other than <0;1>/*: %+v", c.d.useSite)
		}
		if c.d.tree.tag != tagTr || rejectNestedUnspendable(c.d.tree, true) != nil {
			t.Fatalf("the composer nested a Liana key: %+v", c.d.tree)
		}
	}
}

// TestComposeLianaTemplateEqualsTheHost pins the UNSEATED kofn-recovery Liana
// template -- the chunk the device's stub screen is built from and the plate a
// key-less composition cuts -- to what md-cli 0.19.0 prints for
//
//	md compose --wrapper tr --preset kofn-recovery,2of3,older=26280 --unspendable liana
//	  | md encode --force-chunked --group-size 0
//
// at descriptor-mnemonic cf35d61a (the noCorpusChunks convention: a literal the
// primary printed, dated). Its Template-ID is f99cc42e1ff68bae546c4d1070e5963f,
// the keyed vector's, as SPEC §3e requires (keys do not enter the template id).
//
// Mutation: lowerTr choosing NUMS for the Liana request fails (md1frk8k...).
func TestComposeLianaTemplateEqualsTheHost(t *testing.T) {
	c, err := ComposeWithUnspendable(cpl(ComposeTr, ck(2, 3), clk(ck(1, 1), olderBlocks(26280))), make([]*SlotOrigin, 4), UnspendableLiana)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"md13ls8aqqxq6tvyyykjmpprj6tvyy495kcgfwtsqrq0zjqgsexd9dcqqqv65q3cm0m0nz2h7w9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks differ from md-cli 0.19.0's:\n got %v\nwant %v", got, want)
	}
	id, err := c.TemplateID()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(id[:]) != "f99cc42e1ff68bae546c4d1070e5963f" {
		t.Fatalf("Template-ID %x", id)
	}
}
