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

// The load-bearing regression (R1-exec defect): measure-and-advance paging must
// never silently drop an index off-screen. Page forward across enough pages to
// cover indices 0..7 and assert EVERY address appears — for both a single-sig
// fixture (more fit/page) and a long-address P2WSH fixture (fewer fit/page).
func TestDescriptorAddressFlowNoSkippedIndices(t *testing.T) {
	for _, descStr := range []string{descWPKH, descCustomChildren} {
		d := loadTestDesc(t, descStr)
		seen := make(map[string]bool)
		for i := uint32(0); i < 8; i++ {
			a, err := address.Receive(d, i)
			if err != nil {
				t.Fatalf("%s: Receive(%d): %v", descStr, i, err)
			}
			seen[a] = false
		}
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() { descriptorAddressFlow(ctx, &descriptorTheme, d) })
		// Observe the entry page BEFORE advancing (else index 0 is paged over
		// before frame 0 renders). Advance one page per observed frame; 60 frames
		// covers idx 0..7 even at the worst case of 1 address/page.
		for i := 0; i < 60; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			for a := range seen {
				if uiContains(c, a) {
					seen[a] = true
				}
			}
			click(&ctx.Router, Button3) // advance AFTER observing this page
		}
		quit()
		for a, ok := range seen {
			if !ok {
				t.Errorf("%s: address %q was never viewable — paging skipped it", descStr, a)
			}
		}
	}
}

// On a supported descriptor, Button2 opens a Show/Verify choice; selecting "Show
// addresses" (choice 0, confirmed via Button3) opens the address view. On an
// unsupported descriptor, Button2 is inert.
func TestDescriptorConfirmAddressAffordance(t *testing.T) {
	d := loadTestDesc(t, descWPKH) // supported
	if !address.Supported(d) {
		t.Fatal("fixture must be address-supported")
	}
	ds := &DescriptorScreen{Descriptor: d}
	want0, _ := address.Receive(d, 0)
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // open the Show/Verify choice from the confirm screen
	click(&ctx.Router, Button3) // select choice 0 ("Show addresses") → address view
	frame, quit := runUI(ctx, func() { ds.Confirm(ctx, &descriptorTheme) })
	defer quit()
	var content string
	saw := false
	for i := 0; i < 10; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		content = c
		if uiContains(content, want0) {
			saw = true
			break
		}
	}
	_ = content
	if !saw {
		t.Fatal("Button2 did not open the address view on a supported descriptor")
	}
}

// R0 MINOR-2 fold: on a descriptor address.Supported reports false for, the
// Button2 affordance is inert — the address view never opens, the confirm screen
// stays, no crash. The descriptor is the supported wpkh fixture with its Script
// mutated to an unsupported value, so derivation still succeeds but the script
// switch returns errUnsupported (the Supported==false branch / StyleNone path).
func TestDescriptorConfirmAddressAffordanceUnsupported(t *testing.T) {
	d := loadTestDesc(t, descWPKH)
	d.Script = bip380.Script(99) // unsupported singlesig script
	if address.Supported(d) {
		t.Fatal("fixture must be address-unsupported for this test")
	}
	ds := &DescriptorScreen{Descriptor: d}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // must be inert: no address view
	frame, quit := runUI(ctx, func() { ds.Confirm(ctx, &descriptorTheme) })
	defer quit()
	// Drive several frames; the confirm screen must keep rendering (descriptor info,
	// e.g. "Script") and must never show the address view's "Receive addresses".
	sawConfirm := false
	for i := 0; i < 8; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		if uiContains(c, "Receive addresses") || uiContains(c, "Change addresses") {
			t.Fatal("Button2 opened the address view on an unsupported descriptor")
		}
		if uiContains(c, "Script") {
			sawConfirm = true
		}
	}
	if !sawConfirm {
		t.Fatal("confirm screen did not keep rendering after inert Button2")
	}
}
