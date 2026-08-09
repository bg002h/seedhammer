package op

// Len reports the buffer's current fill. It exists so a test can assert that a
// long-lived buffer is not growing across frames -- the defect class behind the
// §10.2.4 warning's 228 KB accumulation, which was invisible to every other
// seam because both fields are unexported.
func (b *Buffer) Len() (args, refs int) {
	return len(b.args), len(b.refs)
}
