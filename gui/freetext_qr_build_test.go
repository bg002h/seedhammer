//go:build !refugium

package gui

import (
	"testing"

	"seedhammer.com/backup"
)

// TestFreeTextQRStaysOnInTheDefaultBuild is the default build's control for
// the Refugium profile's "Engrave Text cuts no QR" (F7 §4.4): here a QR the
// operator asks for is cut, an ms1 string included -- the profile, not the
// text, is what removes it.
func TestFreeTextQRStaysOnInTheDefaultBuild(t *testing.T) {
	for _, text := range []string{
		"HELLO WORLD",
		"ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw",
		"Line 1: ms10testsxxxx\nLine 2: xxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw",
	} {
		var got backup.Fitted
		seen := false
		freetextPlateHook = func(f backup.Fitted) { got, seen = f, true }
		_, err := ftBuildPlate(ftParamsAtSpeed(engraverParams, 0), &ftPlanSH, text, "", "", true, 0, 0)
		freetextPlateHook = nil
		if err != nil {
			t.Fatalf("ftBuildPlate(%q): %v", text, err)
		}
		if !seen || got.QR == nil {
			t.Errorf("%q: the default build dropped a QR the operator asked for", text)
		}
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		ftQRChoiceFlow(ctx, &descriptorTheme, false, []backup.Block{{Text: "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"}})
	})
	defer quit()
	if c, ok := frame(); !ok || !uiContains(c, "Add QR") {
		t.Errorf("the default QR step does not offer a QR: %q", c)
	}
}
