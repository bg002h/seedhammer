package slip39

import (
	"bytes"
	"testing"
)

func TestFeistelSaltAndIters(t *testing.T) {
	// itersPerRound = (10000<<e)/4
	if got := itersPerRound(0); got != 2500 {
		t.Errorf("e=0 -> %d want 2500", got)
	}
	if got := itersPerRound(1); got != 5000 {
		t.Errorf("e=1 -> %d want 5000", got)
	}
	if !bytes.Equal(feistelSalt(0x1234, false), append([]byte("shamir"), 0x12, 0x34)) {
		t.Errorf("non-extendable salt = shamir||be16(id)")
	}
	if len(feistelSalt(0x1234, true)) != 0 {
		t.Errorf("extendable salt is empty")
	}
}
