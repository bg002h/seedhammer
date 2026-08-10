package gui

import (
	"testing"
)

// TestMeasureSeedFrameOrphans is a MEASUREMENT, not a gate. It prints what a
// real 24-word SeedScreen frame actually does to op.Buffer's backing arrays, so
// the residency design can quote a number instead of projecting one from Go's
// growth series.
//
// The projection is not good enough. The single-element doubling series predicts
// cap 3072 for a frame of this size, but the real buffer appends several
// elements at a time (ParamImageMask and encodeOp each append a payload and then
// a header word), so its caps skip steps. And a series over `args` alone omits
// `refs` entirely, which is a third of the bytes.
func TestMeasureSeedFrameOrphans(t *testing.T) {
	for _, n := range []int{12, 24} {
		m := validMnemonic(n)
		pf := newPlatform()
		pf.display = sh2DisplaySize
		ctx := NewContext(pf)
		ss := &SeedScreen{}
		frame, _, quit := runUITouch(ctx, func() {
			ss.Confirm(ctx, &descriptorTheme, m)
		})
		if _, ok := frame(); !ok {
			quit()
			t.Fatalf("%d-word SeedScreen produced no frame", n)
		}
		lenArgs, lenRefs := ctx.B.Len()
		capArgs, capRefs := ctx.B.Caps()
		zArrays, zEntries := ctx.B.Zeroed()
		resArgs, resRefs := ctx.B.Residue()
		// Device sizing: uint32 = 4 B, any = 8 B on the 32-bit RP2350.
		// zEntries mixes both kinds, so 4 B is the floor and 8 B the ceiling.
		curKB := float64(capArgs*4+capRefs*8) / 1024
		t.Logf("%2d-word seed frame: len args=%d refs=%d | cap args=%d refs=%d (current retention %.1f KB)",
			n, lenArgs, lenRefs, capArgs, capRefs, curKB)
		t.Logf("%2d-word    outgrown-and-zeroed: %d arrays, %d entries (%.1f-%.1f KB, freed not retained)",
			n, zArrays, zEntries, float64(zEntries*4)/1024, float64(zEntries*8)/1024)
		t.Logf("%2d-word    residue before Scrub: args=%d refs=%d", n, resArgs, resRefs)
		ctx.B.Scrub()
		resArgs, resRefs = ctx.B.Residue()
		t.Logf("%2d-word    residue after  Scrub: args=%d refs=%d", n, resArgs, resRefs)
		quit()
	}
}

// TestSeedFrameReachesTheOutgrownArrayClass pins the half a unit test cannot
// reach: that a REAL 24-word seed frame actually outgrows its backing arrays,
// so op's zeroing is on the seed path rather than a hypothetical one.
//
// It deliberately does NOT claim to verify the zeroing. Measured by mutation:
// with `clear(old[:cap(old)])` deleted from appendArgs this test still PASSES,
// because once nothing retains an outgrown array `Residue()` cannot see it --
// the assertion is structurally blind to the class. The zeroing is pinned in
// the op package by TestAppendZeroesThe{Args,Refs}ArrayItOutgrows, which hold
// their own reference and read the memory, and the routing is pinned by
// TestBufferGrowthIsFunnelled.
//
// Named for what it pins, because the first draft of it was called
// TestOutgrownArraysAreZeroedAtReallocation and asserted no such thing.
func TestSeedFrameReachesTheOutgrownArrayClass(t *testing.T) {
	m := validMnemonic(24)
	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	ss := &SeedScreen{}
	frame, _, quit := runUITouch(ctx, func() {
		ss.Confirm(ctx, &descriptorTheme, m)
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("SeedScreen produced no frame")
	}

	zArrays, zEntries := ctx.B.Zeroed()
	if zArrays == 0 {
		t.Fatalf("INCONCLUSIVE: the seed frame never outgrew a backing array, so "+
			"this test cannot witness the class it exists for (zeroed %d arrays/%d entries)",
			zArrays, zEntries)
	}

	// The current arrays still hold the frame -- that is Scrub's job, not
	// appendArgs's. What must already be zero is everything the buffer outgrew,
	// and Residue only sees the current array, so scrub it and assert the total.
	ctx.B.Scrub()
	args, refs := ctx.B.Residue()
	if args != 0 || refs != 0 {
		t.Errorf("after Scrub the buffer still holds %d non-zero args and %d non-nil refs", args, refs)
	}

	t.Logf("seed frame outgrew and zeroed %d arrays holding %d entries", zArrays, zEntries)
}
