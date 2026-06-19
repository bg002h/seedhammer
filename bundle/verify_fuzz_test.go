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

// FuzzVerifyWatchOnly (T6a-2 Task 8) targets the watch-only ms1-presence
// branches added to Verify: arbitrary MS1 strings on either side (incl. empty,
// one-sided, and garbage) over the consistent wpkh golden public legs. Verify
// must NEVER panic — the both-empty skip, the one-sided presence mismatch, and
// the entropy-decode error paths are all total.
func FuzzVerifyWatchOnly(f *testing.F) {
	f.Add("", "")
	f.Add(wpkhMS1, wpkhMS1)
	f.Add(wpkhMS1, "")
	f.Add("", wpkhMS1)
	f.Add("ms1garbage", "ms1garbage")
	f.Add("not-an-ms1", wpkhMS1)

	f.Fuzz(func(t *testing.T, dMS1, rMS1 string) {
		d := correctBundle()
		r := correctBundle()
		d.MS1 = dMS1
		r.MS1 = rMS1
		_ = Verify(d, r)
	})
}
