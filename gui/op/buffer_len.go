package op

// Len reports the buffer's current fill. It exists so a test can assert that a
// long-lived buffer is not growing across frames -- the defect class behind the
// §10.2.4 warning's 228 KB accumulation, which was invisible to every other
// seam because both fields are unexported.
func (b *Buffer) Len() (args, refs int) {
	return len(b.args), len(b.refs)
}

// Scrub ZEROES the buffer's backing arrays, where Reset merely truncates.
//
// Reset is the per-frame path and is deliberately cheap: `b.args = b.args[:0]`
// is a truncation, and only `refs` is cleared. That is fine while a buffer is
// being reused -- later frames overwrite `args` -- and WRONG for a buffer being
// ABANDONED with seed-derived ops still in it.
//
// op.Glyph encodes every rendered rune into `args` as a uint32, so after Reset
// the plaintext comes back verbatim and in order from the backing array.
// Measured against a rendered SeedScreen: refs scrubbed 511/511, args
// recovered all twelve words. §10.2.4's wipe abandons its Context, so it must
// call this rather than relying on Reset.
func (b *Buffer) Scrub() {
	clear(b.args[:cap(b.args)])
	clear(b.refs[:cap(b.refs)])
	// Arrays this Buffer OUTGREW were already zeroed by appendArgs/appendRefs at
	// the instant they were outgrown, so there is nothing left here for Scrub to
	// reach -- see Buffer.zeroedArrays. Before that existed, every reallocation
	// left a complete copy of everything written before it, and Scrub zeroed
	// only the current array.
	b.args = b.args[:0]
	b.refs = b.refs[:0]
}

// Residue counts what a truncation left behind: non-zero words and non-nil
// refs anywhere in the BACKING ARRAYS, including past the current length.
//
// It exists because Len cannot distinguish Reset from Scrub -- both leave
// len 0 -- so without it "the abandoned buffer is scrubbed" is an assertion no
// test can fail. After Scrub this is (0, 0); after Reset alone it is not.
func (b *Buffer) Residue() (args, refs int) {
	for _, v := range b.args[:cap(b.args)] {
		if v != 0 {
			args++
		}
	}
	for _, r := range b.refs[:cap(b.refs)] {
		if r != nil {
			refs++
		}
	}
	return args, refs
}

// Zeroed reports how many outgrown backing arrays this Buffer has zeroed, and
// how many entries they held. It exists so a test can prove the zeroing RAN:
// an array that was correctly zeroed and an array that was never created are
// indistinguishable by inspection, so without this an orphan test passes
// vacuously on any frame too small to reallocate.
func (b *Buffer) Zeroed() (arrays, entries int) {
	return b.zeroedArrays, b.zeroedEntries
}

// Caps reports the current backing arrays' capacity, so a measurement can state
// retention against the frame that produced it.
func (b *Buffer) Caps() (args, refs int) {
	return cap(b.args), cap(b.refs)
}
