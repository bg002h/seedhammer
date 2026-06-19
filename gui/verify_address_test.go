package gui

import (
	"strings"
	"testing"
)

func TestRunVerifyResult(t *testing.T) {
	desc := loadTestDesc(t, descWPKH) // address_polish_test.go helper + const
	// Drive runVerify directly with a candidate (bypasses NFC, NFCReader()==nil).
	cases := []struct{ name, cand, want string }{
		{"match receive", "bc1qkwl5qpx6k93cqmnygn6kgucgka8q3z4kur2nm8", "Receive"},
		{"not found", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", "Not found"},
		{"invalid", "not-an-address", "Invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			frame, quit := runUI(ctx, func() { runVerify(ctx, &descriptorTheme, desc, c.cand) })
			defer quit() // abandons the result loop; do NOT click Back per-frame (it would dismiss before the result renders — R0-C1)
			var all strings.Builder
			for i := 0; i < 4; i++ { // frame 1 = "Verifying…", frame 2+ = result (loops until Back)
				content, ok := frame()
				if !ok {
					break
				}
				all.WriteString(content)
			}
			if !uiContains(all.String(), c.want) {
				t.Errorf("verify(%q): want %q; got %q", c.cand, c.want, all.String())
			}
		})
	}
}
