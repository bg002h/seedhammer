package md

import (
	"os"
	"regexp"
	"testing"
)

// F-612: countKeySlots had no `trBody` arm, so a `tr` NESTED INSIDE A TAPLEAF
// hid its internal key and its whole subtree — the policy reported
// DuplicateNone however many times a slot repeated inside it.
//
// Found by the F-533 whole-diff review with a constructed case, and graded
// Minor rather than Important because it is unreachable: `emitFragment` has no
// `tagTr` case, so no deriving policy can carry a nested `tr`.
//
// MUTATION: delete the trBody arm from countKeySlots -> this fails with
// DuplicateNone, which is the pre-fix behaviour.
func TestNestedTaprootInternalKeyIsCounted(t *testing.T) {
	// tr(@0, { tr(@0, pk(@1)) }) — the inner internal key repeats the outer's
	// slot. Before the fix the inner tr contributed nothing at all.
	key := func(slot uint8) node {
		return node{tag: tagPkK, body: keyArgBody{index: slot}}
	}
	inner := node{tag: tagTr, body: trBody{keyIndex: 0, tree: ptr(key(1))}}
	outer := node{tag: tagTr, body: trBody{keyIndex: 0, tree: ptr(inner)}}

	slot, kind := DuplicateKeySlot(outer)
	if kind == DuplicateNone {
		t.Fatal("a nested tr's internal key is invisible to the key-slot walk: " +
			"the policy repeats slot @0 at two internal keys and reports DuplicateNone")
	}
	if slot != 0 {
		t.Errorf("reported slot @%d, want @0", slot)
	}
}

// A NUMS internal key is the H-point, not a reference to a slot, so it must not
// be counted — the same rule F-533 applies at the top level (SPEC §7). Without
// this the arm above would invent a repeat of slot 0 for every nested NUMS tr.
func TestNestedNumsInternalKeyIsNotCounted(t *testing.T) {
	key := func(slot uint8) node {
		return node{tag: tagPkK, body: keyArgBody{index: slot}}
	}
	inner := node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: ptr(key(0))}}
	outer := node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: ptr(inner)}}

	if _, kind := DuplicateKeySlot(outer); kind != DuplicateNone {
		t.Errorf("two NUMS internal keys were counted as a repeat of slot 0: %v", kind)
	}
}

// THE TRIPWIRE, and it is the point of this file.
//
// F-612 is safe today only because `emitFragment` has no `tagTr` case, which is
// a property of a DIFFERENT function in a different file. Nothing connected the
// two, so teaching emitFragment taproot would have made the nested-tr gap live
// silently — and the review that found the gap said exactly that.
//
// This is that connection. If someone adds taproot to emitFragment, this fails
// and points them at the walk.
//
// `tagTrue` CONTAINS "tagTr" as a substring, so the match is on a word
// boundary. A plain strings.Contains would report a taproot case that is not
// there and this gate would fail on every tree, which is the failure mode that
// gets a gate deleted rather than read.
func TestEmitFragmentHasNoTaprootCase(t *testing.T) {
	src, err := os.ReadFile("script_emit.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`\btagTrue\b`).Match(src) {
		t.Fatal("script_emit.go mentions no tagTrue: this gate is reading the wrong " +
			"file and would pass on anything")
	}
	if regexp.MustCompile(`\btagTr\b`).Match(src) {
		t.Fatal("script_emit.go now handles tagTr, so a `tr` can be nested inside a " +
			"tapleaf and REACH a deriving policy. F-612's countKeySlots arm is no " +
			"longer merely defensive: re-read it, and re-read DuplicateKeySlot's " +
			"taproot arm, before removing this gate.")
	}
}
