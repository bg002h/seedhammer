package codex32

import "testing"

const (
	surplusA   = "ms13casha320zyxwvutsrqpnmlkjhgfedca2a8d0zehn8a0t"
	surplusC   = "ms13cashcacdefghjklmnpqrstuvwxyz023949xq35my48dr"
	surplusD   = "ms13cashd0wsedstcdcts64cd7wvy4m90lm28w4ffupqs7rm"
	surplusE   = "ms13casheekgpemxzshcrmqhaydlp6yhms3ws7320xyxsar9"
	surplusX   = "ms13casheqqqqqqqqqqqqqqqqqqqqqqqqqqa8zxete8lyl02"
	surplusN   = "ms12namea320zyxwvutsrqpnmlkjhgfedcaxrpp870hkkqrm"
	surplusT0a = "ms10testsqqqqqqqqqqqqqqqqqqqqqqqqqqs75svv7jal8p5"
	surplusT0b = "ms10testsqyqqqqqqqqqqqqqqqqqqqqqqqqmwawx2dktvjhg"
	surplusS   = "ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln"
)

func surplusSet(t *testing.T, ss ...string) []String {
	t.Helper()
	var out []String
	for _, s := range ss {
		out = append(out, mustFromString(t, s))
	}
	return out
}

func surplusRefused(t *testing.T, name string, target rune, ss ...string) {
	t.Helper()
	got, err := Interpolate(surplusSet(t, ss...), target)
	if got != (String{}) {
		t.Errorf("%s: returned %q, want zero String", name, got.String())
	}
	if err == nil || err.Error() != "too many shares" {
		t.Errorf("%s: err = %v, want too many shares", name, err)
	}
}

func TestFixShCodex32SurplusInconsistent(t *testing.T) {
	surplusRefused(t, "A C D X", 'S', surplusA, surplusC, surplusD, surplusX)
	surplusRefused(t, "X A C D", 'S', surplusX, surplusA, surplusC, surplusD)
}

func TestFixShCodex32SurplusConsistent(t *testing.T) {
	surplusRefused(t, "A C D E", 'S', surplusA, surplusC, surplusD, surplusE)
}

func TestFixShCodex32SurplusTargetIsInput(t *testing.T) {
	surplusRefused(t, "A C D X -> E", 'E', surplusA, surplusC, surplusD, surplusX)
	surplusRefused(t, "A C D X -> F", 'F', surplusA, surplusC, surplusD, surplusX)
}

func TestFixShCodex32SurplusRepeatedIndex(t *testing.T) {
	surplusRefused(t, "A C D D", 'S', surplusA, surplusC, surplusD, surplusD)
}

func TestFixShCodex32SurplusMismatchFirst(t *testing.T) {
	got, err := Interpolate(surplusSet(t, surplusA, surplusC, surplusD, surplusN), 'S')
	if got != (String{}) {
		t.Errorf("returned %q, want zero String", got.String())
	}
	if err == nil || err.Error() != "mismatched threshold" {
		t.Errorf("err = %v, want mismatched threshold", err)
	}
}

func TestFixShCodex32ExactKUnchanged(t *testing.T) {
	got, err := Interpolate(surplusSet(t, surplusA, surplusC, surplusD), 'S')
	if err != nil || got.String() != surplusS {
		t.Errorf("A C D -> S: %q, %v", got.String(), err)
	}
	got, err = Interpolate(surplusSet(t, surplusA, surplusC), 'S')
	if err == nil || err.Error() != "insufficient shares" || got != (String{}) {
		t.Errorf("A C -> S: %q, %v", got.String(), err)
	}
	got, err = Interpolate(surplusSet(t, surplusA, surplusC, surplusD), 'A')
	if err != nil || got.String() != surplusA {
		t.Errorf("A C D -> A: %q, %v", got.String(), err)
	}
}

func TestFixShCodex32Threshold0(t *testing.T) {
	got, err := Interpolate(surplusSet(t, surplusT0a), 'S')
	if err != nil || got.String() != surplusT0a {
		t.Errorf("T0a alone: %q, %v", got.String(), err)
	}
	surplusRefused(t, "T0a T0b", 'S', surplusT0a, surplusT0b)
	surplusRefused(t, "T0b T0a", 'S', surplusT0b, surplusT0a)
	surplusRefused(t, "T0a T0a", 'S', surplusT0a, surplusT0a)
}

func TestFixShCodex32DescribeTooMany(t *testing.T) {
	_, err := Interpolate(surplusSet(t, surplusA, surplusC, surplusD, surplusE), 'S')
	if got := Describe(err); got != "too many shares" {
		t.Errorf("Describe = %q, want too many shares", got)
	}
}
