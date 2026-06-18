package codex32

import (
	"strings"
	"testing"
)

func TestMStarInWindow(t *testing.T) {
	pad := func(hrp string, dataLen int) string {
		return hrp + "1" + strings.Repeat("q", dataLen)
	}
	// pad("ms",45) = "ms"+"1"+45×"q" = total length 48 (the prefix "xx1" is 3
	// chars, so total = dataLen + 3; ms windows are keyed on TOTAL length, md/mk
	// on the data-part length).
	cases := []struct {
		name string
		frag string
		want bool
	}{
		// ms uses TOTAL length 48..93 / 125..127.
		{"ms below short", pad("ms", 44), false}, // total 47 < 48
		{"ms short min", pad("ms", 45), true},    // total 48
		{"ms short max", pad("ms", 90), true},    // total 93
		{"ms dead zone", pad("ms", 91), false},   // total 94
		{"ms long lo", pad("ms", 122), true},     // total 125
		{"ms long hi", pad("ms", 124), true},     // total 127
		{"ms too long", pad("ms", 125), false},   // total 128
		// md: data ≥13, no upper bound.
		{"md below", pad("md", 12), false},
		{"md min", pad("md", 13), true},
		{"md big", pad("md", 200), true},
		// mk: data 14..93 / 96..108; 94..95 reserved.
		{"mk below reg", pad("mk", 13), false},
		{"mk reg lo", pad("mk", 14), true},
		{"mk reg hi", pad("mk", 93), true},
		{"mk reserved 94", pad("mk", 94), false},
		{"mk reserved 95", pad("mk", 95), false},
		{"mk long lo", pad("mk", 96), true},
		{"mk long hi", pad("mk", 108), true},
		{"mk too long", pad("mk", 109), false},
		// unknown / no separator.
		{"no separator", "msqqqq", false}, // no '1' ⇒ HRP "" ⇒ false
		{"foreign hrp", pad("xx", 60), false},
	}
	for _, c := range cases {
		if got := MStarInWindow(c.frag); got != c.want {
			t.Errorf("%s: MStarInWindow(%q[len %d]) = %v, want %v", c.name, c.frag, len(c.frag), got, c.want)
		}
	}
}
