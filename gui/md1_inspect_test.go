package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

func TestMD1DisplayFlowPaging(t *testing.T) {
	ctx := NewContext(newPlatform())
	tpl := md.Template{N: 2, Root: md.ScriptWsh, Policy: md.PolicyMulti, K: 2, M: 2, Renderable: true,
		Keys: []md.KeyOrigin{{Index: 0, Fingerprint: "deadbeef", OriginPath: "m/48'/0'/0'/2'", UseSite: "<0;1>/*"},
			{Index: 1, Fingerprint: "cafebabe", OriginPath: "m/48'/0'/0'/2'", UseSite: "<0;1>/*"}}}
	frame, quit := runUI(ctx, func() { md1DisplayFlow(ctx, &descriptorTheme, tpl) })
	defer quit()
	var all strings.Builder
	for i := 0; i < 16; i++ {
		content, ok := frame()
		if !ok {
			break
		}
		all.WriteString(content)
		click(&ctx.Router, Button3)
	}
	got := all.String()
	if !uiContains(got, "multisig") || !uiContains(got, "deadbeef") || !uiContains(got, "cafebabe") {
		t.Errorf("summary missing fields; got %q", got)
	}
}

func TestMD1DisplayFlowComplexRefuses(t *testing.T) {
	ctx := NewContext(newPlatform())
	tpl := md.Template{N: 1, Root: md.ScriptWsh, Policy: md.PolicyComplex, Renderable: false,
		Keys: []md.KeyOrigin{{Index: 0, OriginPath: "m/0'", UseSite: "<0;1>/*"}}}
	frame, quit := runUI(ctx, func() { md1DisplayFlow(ctx, &descriptorTheme, tpl) })
	defer quit()
	content, _ := frame()
	if !uiContains(content, "cannot display") && !uiContains(content, "complex") {
		t.Errorf("complex policy must refuse; got %q", content)
	}
}

func TestHasMDPrefix(t *testing.T) {
	if !hasMDPrefix("md1abc") || !hasMDPrefix("MD1ABC") {
		t.Fatal("md1 not detected")
	}
	if hasMDPrefix("mk1abc") {
		t.Fatal("mk1 misdetected")
	}
}

func TestMdmkFlowMD1ShowsInspect(t *testing.T) {
	p := newPlatform()
	p.engraver = newEngraver()
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { mdmkFlow(ctx, &descriptorTheme, mdmkText("md1yqpqqxqq8xtwhw4xwn4qh")) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(content, "Inspect descriptor") {
		t.Errorf("md1 chooser must offer Inspect descriptor; got %q", content)
	}
}
