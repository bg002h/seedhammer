package bundle

import "testing"

// FuzzVerify feeds arbitrary string-slice bundles to Verify: it must never
// panic and must only ever return nil or a typed error. (T6a-1 Task 4.)
func FuzzVerify(f *testing.F) {
	f.Add(wpkhMS1, wpkhMK1[0], wpkhMK1[1], wpkhMD1[0], wpkhMD1[1], wpkhMD1[2])
	f.Add("", "", "", "", "", "")
	f.Fuzz(func(t *testing.T, ms1, mk1a, mk1b, md1a, md1b, md1c string) {
		b := Bundle{
			MS1: ms1,
			MK1: []string{mk1a, mk1b},
			MD1: []string{md1a, md1b, md1c},
		}
		// Compare the bundle to itself and to a fixed correct bundle: both must
		// return without panicking (nil or a typed error).
		_ = Verify(b, b)
		_ = Verify(correctBundle(), b)
		_ = Verify(b, correctBundle())
	})
}
