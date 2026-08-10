package op

import "testing"

// The array a Buffer outgrows is the one Scrub can never reach: append replaces
// it, Scrub sees only the current array, and a mark-sweep collector that does
// not zero on free leaves every rune written into it intact. Measured on a
// rendered 24-word seed frame before this was fixed: thirteen words came back
// verbatim and in index order out of an array Residue() scored 0.
//
// These tests hold their OWN reference to the outgrown array and read its
// memory. They deliberately do not consult Buffer.Zeroed() for the verdict --
// the bookkeeping is exactly what a mutation would leave intact while the bytes
// survived, which is how the previous version of this work reported PASS on a
// live leak.
//
// Mutation this pins: delete `clear(old[:cap(old)])` from appendArgs.
func TestAppendZeroesTheArgsArrayItOutgrows(t *testing.T) {
	var b Buffer
	b.appendArgs(1, 2, 3, 4)
	if cap(b.args) == 0 {
		t.Fatal("no array to outgrow")
	}

	// Our own reference to the current backing array, filled with a canary so
	// "zeroed" is distinguishable from "never written".
	old := b.args[:cap(b.args)]
	for i := range old {
		old[i] = 0xDEADBEEF
	}
	b.args = b.args[:4]
	oldCap := cap(old)

	// Grow until the array is replaced.
	for cap(b.args) == oldCap {
		b.appendArgs(0x11111111)
	}

	for i, v := range old {
		if v != 0 {
			t.Fatalf("outgrown args array still holds data at [%d] = %#x -- "+
				"append replaced the array without zeroing it, and Scrub can never reach it", i, v)
		}
	}
	if arrays, entries := b.Zeroed(); arrays == 0 || entries == 0 {
		t.Errorf("accounting disagrees with memory: Zeroed() = (%d, %d) after an outgrown array", arrays, entries)
	}
}

// The refs array carries interface values, so an unzeroed orphan both leaks the
// reference and PINS the referent: Buffer.Reset clears only the current array,
// precisely so those referents drop.
//
// Mutation this pins: delete `clear(old[:cap(old)])` from appendRefs.
func TestAppendZeroesTheRefsArrayItOutgrows(t *testing.T) {
	var b Buffer
	b.appendRefs("canary", "canary", "canary", "canary")
	if cap(b.refs) == 0 {
		t.Fatal("no array to outgrow")
	}

	old := b.refs[:cap(b.refs)]
	for i := range old {
		old[i] = "canary"
	}
	b.refs = b.refs[:4]
	oldCap := cap(old)

	for cap(b.refs) == oldCap {
		b.appendRefs("filler")
	}

	for i, r := range old {
		if r != nil {
			t.Fatalf("outgrown refs array still holds %v at [%d] -- the reference leaks "+
				"and the referent stays pinned for the Buffer's lifetime", r, i)
		}
	}
}

// Zeroing an outgrown array must not disturb the live one: append has already
// copied the elements across, so the new array carries the full history.
func TestOutgrowingPreservesTheLiveContents(t *testing.T) {
	var b Buffer
	want := make([]uint32, 0, 4096)
	for i := 1; i <= 4096; i++ {
		b.appendArgs(uint32(i))
		want = append(want, uint32(i))
	}
	if arrays, _ := b.Zeroed(); arrays == 0 {
		t.Fatal("INCONCLUSIVE: never reallocated, so nothing was outgrown")
	}
	if len(b.args) != len(want) {
		t.Fatalf("len(args) = %d, want %d", len(b.args), len(want))
	}
	for i, v := range want {
		if b.args[i] != v {
			t.Fatalf("args[%d] = %d, want %d -- zeroing the outgrown array corrupted the live one", i, b.args[i], v)
		}
	}
}
