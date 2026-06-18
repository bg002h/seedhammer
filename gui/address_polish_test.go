package gui

import (
	"testing"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/nonstandard"
)

const tvXpub = "xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan"

// descWPKH: supported single-sig (default <0;1>/* children).
// descCustomChildren: wsh sortedmulti with explicit /1234/<5;6>/* so receive(branch5) ≠ change(branch6).
const (
	descWPKH           = "wpkh(" + tvXpub + ")"
	descCustomChildren = "wsh(sortedmulti(1," + tvXpub + "/1234/<5;6>/*))"
)

func loadTestDesc(t *testing.T, descStr string) *bip380.Descriptor {
	t.Helper()
	d, err := nonstandard.OutputDescriptor([]byte(descStr))
	if err != nil {
		t.Fatalf("OutputDescriptor(%q): %v", descStr, err)
	}
	return d
}

// frameUntil drives a runUI frame iterator up to n frames, returning true once the
// rendered content contains sub.
func frameUntil(frame func() (string, bool), sub string, n int) bool {
	for i := 0; i < n; i++ {
		c, ok := frame()
		if !ok {
			return false
		}
		if uiContains(c, sub) {
			return true
		}
	}
	return false
}

func TestDescriptorAddressFlowRendersReceive(t *testing.T) {
	d := loadTestDesc(t, descWPKH)
	want0, err := address.Receive(d, 0)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { descriptorAddressFlow(ctx, &descriptorTheme, d) })
	defer quit()
	if !frameUntil(frame, want0, 8) {
		t.Fatalf("address list did not render receive[0] %q", want0)
	}
}

func TestDescriptorAddressFlowToggleChange(t *testing.T) {
	d := loadTestDesc(t, descCustomChildren)
	wantChange0, err := address.Change(d, 0)
	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	wantRecv0, _ := address.Receive(d, 0)
	if wantChange0 == wantRecv0 {
		t.Fatal("fixture must distinguish receive from change")
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // toggle receive→change
	frame, quit := runUI(ctx, func() { descriptorAddressFlow(ctx, &descriptorTheme, d) })
	defer quit()
	if !frameUntil(frame, wantChange0, 8) {
		t.Fatalf("toggle did not render change[0] %q", wantChange0)
	}
}

func TestDescriptorAddressFlowBackExits(t *testing.T) {
	d := loadTestDesc(t, descWPKH)
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back → the flow should return
	frame, quit := runUI(ctx, func() { descriptorAddressFlow(ctx, &descriptorTheme, d) })
	defer quit()
	// The flow returns on Back; the iterator must end within a few frames.
	ended := false
	for i := 0; i < 6; i++ {
		if _, ok := frame(); !ok {
			ended = true
			break
		}
	}
	if !ended {
		t.Fatal("Back did not exit descriptorAddressFlow")
	}
}
