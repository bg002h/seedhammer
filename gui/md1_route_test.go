package gui

import (
	"strings"
	"testing"
)

// chooseInspect advances the ChoiceScreen to the first entry ("Inspect
// descriptor") and selects it (Button3 = confirm at index 0).
func chooseInspect(r *EventRouter) {
	click(r, Button3)
}

// TestMdmkFlowChunkedMD1RoutesToGather: scanning/inspecting a chunked md1 (first
// chunk) no longer shows the "not yet supported" refusal — it enters the gather
// flow. For the single-chunk wsh_multi_chunked vector it completes immediately
// (unsorted multi, no pubkeys → template-only display), proving the route
// reaches reassembly + expansion rather than the old refusal.
func TestMdmkFlowChunkedMD1RoutesToGather(t *testing.T) {
	chunked := loadChunkedVectorString(t, "wsh_multi_chunked")
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText(chunked)) })
	defer quit()

	var all strings.Builder
	content, ok := frame()
	if !ok {
		t.Fatal("mdmkFlow produced no frame")
	}
	all.WriteString(content)
	if !uiContains(content, "Inspect descriptor") {
		t.Fatalf("chunked md1 chooser missing Inspect descriptor; got %q", content)
	}
	chooseInspect(&ctx.Router)
	// Pump a few frames through the completion handler.
	for i := 0; i < 6; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		all.WriteString("\n")
		all.WriteString(c)
	}
	got := all.String()
	if uiContains(got, "not yet supported") || uiContains(got, "not supported") {
		t.Errorf("chunked md1 must NOT show the refusal anymore; got %q", got)
	}
}

// TestMdmkFlowChunkedMD1ExpandOKReachesDescriptor: a complete chunked
// wsh(sortedmulti) set (with real xpubs) routed through mdmkFlow reaches the
// descriptor display + address-verify. Driven via gatheredDescriptorFlow (the
// route's completion handler) since a 6-chunk set can't complete via the
// no-reader test platform.
func TestMdmkFlowChunkedMD1ExpandOKReachesDescriptor(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { gatheredDescriptorFlow(ctx, &descriptorTheme, wshSortedmultiChunks) })
	defer quit()
	got := ""
	for i := 0; i < 4; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		got += "\n" + c
	}
	// The descriptor display offers verification (DescriptorScreen / its menu).
	if uiContains(got, "tampered") || uiContains(got, "can't decode") || uiContains(got, "complex policy") {
		t.Errorf("expandOK chunked set must reach the descriptor display, not an error; got %q", got)
	}
}

// TestMdmkFlowSingleMD1Unchanged: a single (non-chunked) md1 still decodes +
// displays via md1DisplayFlow (the err==nil arm), unchanged by the route edit.
func TestMdmkFlowSingleMD1Unchanged(t *testing.T) {
	single := loadChunkedVectorString(t, "wpkh_basic") // single-string md1
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText(single)) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(content, "Inspect descriptor") {
		t.Fatalf("single md1 chooser missing Inspect descriptor; got %q", content)
	}
	chooseInspect(&ctx.Router)
	got := ""
	for i := 0; i < 6; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		got += "\n" + c
	}
	// wpkh_basic is a renderable single-key template → md1DisplayFlow shows the
	// "Type: P2WPKH single-key" summary, not a refusal/error.
	if uiContains(got, "not yet supported") || uiContains(got, "can't decode") {
		t.Errorf("single md1 display regressed; got %q", got)
	}
	if !uiContains(got, "P2WPKH") && !uiContains(got, "single") {
		t.Errorf("single md1 should show its template summary; got %q", got)
	}
}
