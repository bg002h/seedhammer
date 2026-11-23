package main

import "testing"

func TestFixShBiptoolSurplusShares(t *testing.T) {
	in := []byte("ms13casha320zyxwvutsrqpnmlkjhgfedca2a8d0zehn8a0t\n" +
		"ms13cashcacdefghjklmnpqrstuvwxyz023949xq35my48dr\n" +
		"ms13cashd0wsedstcdcts64cd7wvy4m90lm28w4ffupqs7rm\n" +
		"ms13casheqqqqqqqqqqqqqqqqqqqqqqqqqqa8zxete8lyl02\n")
	for _, cmd := range []string{"interpolate -idx s", "derive seed"} {
		out, err := execErr(in, cmd)
		if len(out) != 0 {
			t.Errorf("%s: stdout %q, want empty", cmd, out)
		}
		if err == nil || err.Error() != "too many shares" {
			t.Errorf("%s: err = %v, want too many shares", cmd, err)
		}
	}
}
