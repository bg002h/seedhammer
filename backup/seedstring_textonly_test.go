package backup

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/codex32"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
)

// EngraveSeedStringTextOnly is the TEXT-ONLY codex32 plate (Refugium plan F7
// §4.4, R0 I-8a): the Refugium build engraves no QR of an ms1 string, and the
// seed-string plate was the one ms1 plate with no text-only variant.
// EngraveSeedString itself is unchanged -- TestCodex32's goldens pin that.

func codex32Plate(t *testing.T, share string) SeedString {
	t.Helper()
	s, err := codex32.New(share)
	if err != nil {
		t.Fatal(err)
	}
	mk, err := hdkeychain.NewMaster(s.Seed(), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	pkey, err := mk.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := s.Split()
	return SeedString{
		Title:             id,
		Seed:              s.String(),
		MasterFingerprint: bip32.Fingerprint(pkey),
		Font:              constant.Font,
	}
}

func TestEngraveSeedStringTextOnlyGolden(t *testing.T) {
	for i, share := range []string{
		"ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln",
		"ms10leetsllhdmn9m42vcsamx24zrxgs3qrl7ahwvhw4fnzrhve25gvezzyq0pgjxpzx0ysaam",
	} {
		p, err := EngraveSeedStringTextOnly(params, codex32Plate(t, share))
		if err != nil {
			t.Fatal(err)
		}
		compareGolden(t, "codex32-"+string(rune('0'+i))+"-text", p)
	}
}

// The text-only plate carries NO QR: its cut is a strict subset of the TEXT +
// QR plate's, missing exactly the code. Measured as the stepper's needle-down
// command count: the two plates share every glyph, so the text-only one must emit
// strictly fewer commands.
func TestEngraveSeedStringTextOnlyHasNoQR(t *testing.T) {
	plate := codex32Plate(t, "ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln")
	withQR, err := EngraveSeedString(params, plate)
	if err != nil {
		t.Fatal(err)
	}
	textOnly, err := EngraveSeedStringTextOnly(params, plate)
	if err != nil {
		t.Fatal(err)
	}
	moves := func(e engrave.Engraving) (n int) {
		for range e {
			n++
		}
		return n
	}
	q, txt := moves(withQR), moves(textOnly)
	if txt == 0 || txt >= q {
		t.Fatalf("text-only plate emits %d commands, TEXT + QR %d: want 0 < text-only < TEXT + QR", txt, q)
	}
}

// Refused exactly where EngraveSeedString refuses for length, so a share the
// QR variant admits is never refused by the text-only one.
func TestEngraveSeedStringTextOnlyRefusesWhatTheQRPlateRefuses(t *testing.T) {
	long := SeedString{Title: "x", Seed: "MS1", Font: constant.Font}
	for n := 0; n < 200; n++ {
		long.Seed += "Q"
	}
	if _, err := EngraveSeedString(params, long); err == nil {
		t.Fatal("precondition: EngraveSeedString accepted a 203-character string")
	}
	if _, err := EngraveSeedStringTextOnly(params, long); err == nil {
		t.Fatal("EngraveSeedStringTextOnly accepted a string EngraveSeedString refuses")
	}
}
