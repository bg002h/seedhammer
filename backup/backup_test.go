package backup

import (
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/bezier"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/bspline"
	"seedhammer.com/codex32"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/internal/golden"
	"seedhammer.com/seedqr"
	slip39words "seedhammer.com/slip39"
)

var (
	update = flag.Bool("update", false, "update golden files")
)

const (
	mm             = 6400
	speed          = 30 * mm
	engravingSpeed = 8 * mm
	accel          = 250 * mm
	jerk           = 2600 * mm
)

var (
	conf = engrave.StepperConfig{
		Speed:          speed,
		EngravingSpeed: engravingSpeed,
		Acceleration:   accel,
		Jerk:           jerk,
		TicksPerSecond: speed,
	}
	params = engrave.Params{
		Millimeter:    mm,
		StrokeWidth:   mm / 3,
		StepperConfig: conf,
	}
)

func BenchmarkEngraving(b *testing.B) {
	const (
		singlesig        = "wpkh([dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*)#ap6v6zth"
		compactSinglesig = "wpkh(xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*)"
		multisig         = "wsh(sortedmulti(2,[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/1/*,[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/0h/*,[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*h))#qjs07xve"
		compactMultisig  = "wsh(sortedmulti(2,xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/1/*,xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/0h/*,xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*h))"
	)

	seed := func(n int) func() engrave.Engraving {
		return func() engrave.Engraving {
			s := genSeed(b, "Satoshi Stash", n)
			p, err := EngraveSeed(params, s)
			if err != nil {
				b.Fatal(err)
			}
			return p
		}
	}
	benchmarks := []struct {
		name string
		plan func() engrave.Engraving
	}{
		{
			"singlesig-descriptor-with-qr",
			func() engrave.Engraving {
				return EngraveText(
					params,
					Text{
						Paragraphs: []Paragraph{{Text: singlesig, QR: QR(b, compactSinglesig), QRScale: 3}},
						Font:       sh.Font,
					},
				)
			},
		},
		{
			"large-descriptor-no-qr",
			func() engrave.Engraving {
				return EngraveText(
					params,
					Text{
						Paragraphs: []Paragraph{{Text: multisig}},
						Font:       sh.Font,
					},
				)
			},
		},
		{
			"large-qr",
			func() engrave.Engraving {
				return EngraveText(
					params,
					Text{
						Paragraphs: []Paragraph{{QR: QR(b, compactMultisig), QRScale: 3}},
						Font:       sh.Font,
					},
				)
			},
		},
		{
			"12-words",
			seed(12),
		},
		{
			"24-words",
			seed(24),
		},
	}
	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			var dur time.Duration
			for b.Loop() {
				dur += engrave.TimePlan(conf, bench.plan())
			}
			b.ReportMetric(dur.Minutes()/float64(b.N), "min/op")
		})
	}
}

func textAndQR(t *testing.T, s string) Paragraph {
	t.Helper()
	qrc, err := qr.Encode(s, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	return Paragraph{Text: s, QR: qrc}
}

func QR(t testing.TB, s string) *qr.Code {
	qrc, err := qr.Encode(s, qr.L)
	if err != nil {
		t.Fatal(err)
	}
	return qrc
}

func TestText(t *testing.T) {
	const (
		singlesig        = "wpkh([dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*)#ap6v6zth"
		compactSinglesig = "wpkh(xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*)"
		multisig         = "wsh(sortedmulti(2,[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/1/*,[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/0h/*,[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*h))#qjs07xve"
		compactMultisig  = "wsh(sortedmulti(2,xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/1/*,xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/0h/*,xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*h))"
	)

	tests := []struct {
		data []Paragraph
	}{
		{[]Paragraph{textAndQR(t, "UR:CRYPTO-OUTPUT/1-2/LPADAOCFADFXCYDAPRLRMSHDOETAADMHTAADMETAADMSOEADAOAOLSTAADDLOXAXHDCLAOLBAOTTVYCXLRCXFLATSAKBMUVWLUOTOSRDOTRSHYZMJNADIELPTBCSPMAOFZPABNAAHDCXHTRDDAOYRYSGUYHLIDHGDMAAGEKIRFRTJZLOFSSRONUYIOJTKOMKTLSBCMIALBTIAMTAADDYOEADLOCSDYYKAEYKAEYKADYKAOCYCFWYAAPAAYCYWYAYDRTBTAADDLOXAXHDCLAXSKURKKMDRFRNIYSFLRDAAYJOOXCKKNEESNEETEHSOYMYECENKGRHJYMYJPINCPAOAAHDCXUTWNSFKGIHUY")}},
		// Standalone large descriptor.
		{[]Paragraph{{Text: multisig}}},
		// Descriptor QR.
		{[]Paragraph{{QR: QR(t, compactMultisig), QRScale: 3}}},
	}
	for i, test := range tests {
		name := fmt.Sprintf("%d-shards-%d", i, len(test.data))
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			txt := Text{
				Paragraphs: test.data,
				Font:       sh.Font,
			}
			compareGolden(t, "text-"+name, EngraveText(params, txt))
		})
	}
}

func TestSeed(t *testing.T) {
	tests := []struct {
		seedLen int
	}{
		{24},
		{12},
	}
	for i, test := range tests {
		name := fmt.Sprintf("%d-words-%d", i, test.seedLen)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			seedDesc := genSeed(t, "Satoshi Stash", test.seedLen)
			p, err := EngraveSeed(params, seedDesc)
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, "seed-"+name, p)
		})
	}
}

func TestSLIP39(t *testing.T) {
	tests := []string{
		"duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard",
	}
	for i, test := range tests {
		name := fmt.Sprintf("%d", i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := strings.Split(test, " ")
			seedDesc := Seed{
				Mnemonic:     w,
				ShortestWord: slip39words.ShortestWord,
				LongestWord:  slip39words.LongestWord,
				Title:        "7945 #1 1/1",
				Font:         constant.Font,
			}
			seedSide, err := EngraveSeed(params, seedDesc)
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, "slip39-"+name, seedSide)
		})
	}
}

func TestConstantSeedTiming(t *testing.T) {
	tests := []string{
		"duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard",
		"shadow pistol academic always adequate wildlife fancy gross oasis cylinder mustang wrist rescue view short owner flip making coding armed",
	}
	var prevProf engrave.Profile
	for i, test := range tests {
		w := strings.Split(test, " ")
		seedDesc := Seed{
			Mnemonic:     w,
			ShortestWord: slip39words.ShortestWord,
			LongestWord:  slip39words.LongestWord,
			Font:         constant.Font,
		}
		seedSide, err := EngraveSeed(params, seedDesc)
		if err != nil {
			t.Fatal(err)
		}
		prof := engrave.ProfileSpline(engrave.PlanEngraving(params.StepperConfig, seedSide))
		if i > 0 && !prof.Equal(prevProf) {
			t.Errorf("seed %q has profile\n%+v\nexpected\n%+v", test, prof, prevProf)
		}
		prevProf = prof
	}
}

func TestConstantStringTiming(t *testing.T) {
	tests := []string{
		"MS10LEETSLLHDMN9M42VCSAMX24ZRXGS3QRL7AHWVHW4FNZRHVE25GVEZZYQWCNRWPMLKMT9DT",
		"MS10LEETSLLHDMN9M42VCSAMX24ZRXGS3QRL7AHWVHW4FNZRHVE25GVEZZYQ0PGJXPZX0YSAAM",
	}
	var prevProf engrave.Profile
	for i, test := range tests {
		scan, err := codex32.New(test)
		if err != nil {
			t.Fatalf("%s: %v", test, err)
		}
		id, _, _ := scan.Split()
		desc := SeedString{
			Title: id,
			Seed:  scan.String(),
			Font:  constant.Font,
		}
		seedSide, err := EngraveSeedString(params, desc)
		if err != nil {
			t.Fatal(err)
		}
		prof := engrave.ProfileSpline(engrave.PlanEngraving(params.StepperConfig, seedSide))
		if i > 0 && !prof.Equal(prevProf) {
			t.Errorf("seed %q has profile\n%+v\nexpected\n%+v", test, prof, prevProf)
		}
		prevProf = prof
	}
}

func TestCodex32(t *testing.T) {
	tests := []string{
		"ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln",
		"ms10leetsllhdmn9m42vcsamx24zrxgs3qrl7ahwvhw4fnzrhve25gvezzyq0pgjxpzx0ysaam",
	}
	for i, test := range tests {
		name := fmt.Sprintf("%d", i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			test, err := codex32.New(test)
			if err != nil {
				t.Fatal(err)
			}
			network := &chaincfg.MainNetParams
			mk, err := hdkeychain.NewMaster(test.Seed(), network)
			if err != nil {
				t.Fatal(err)
			}
			pkey, err := mk.ECPubKey()
			if err != nil {
				t.Fatal(err)
			}
			mfp := bip32.Fingerprint(pkey)
			id, _, _ := test.Split()
			s := SeedString{
				Title:             id,
				Seed:              test.String(),
				MasterFingerprint: mfp,
				Font:              constant.Font,
			}
			p, err := EngraveSeedString(params, s)
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, "codex32-"+name, p)
		})
	}
}

func TestTitleString(t *testing.T) {
	tests := []struct {
		test  string
		title string
	}{
		{"Satoshi's Wallet", "SATOSHI'S WALLET"},
		{"Anø de:Æby09 . asd asd asd as das d asd asdf sdf s fd", "AN DE:BY09 . ASD A"},
		{"Æg", "G"},
		{"🤡 💩", " "},
		{"$€#,", "#,"},
	}
	for _, test := range tests {
		s := TitleString(constant.Font, test.test)
		if s != test.title {
			t.Fatalf("got %q, wanted %q", s, test.title)
		}
	}
}

func genSeed(t testing.TB, title string, seedlen int) Seed {
	m := make(bip39.Mnemonic, seedlen)
	for j := range m {
		m[j] = bip39.Word(j)
	}
	m = m.FixChecksum()
	seed := bip39.MnemonicSeed(m, "")
	network := &chaincfg.MainNetParams
	mk, err := hdkeychain.NewMaster(seed, network)
	if err != nil {
		t.Fatal(err)
	}
	pkey, err := mk.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	mfp := bip32.Fingerprint(pkey)
	qrc, err := qr.Encode(string(seedqr.QR(m)), qr.M)
	if err != nil {
		t.Fatal(err)
	}
	words := make([]string, len(m))
	for i, w := range m {
		words[i] = bip39.LabelFor(w)
	}
	return Seed{
		Title:             title,
		Mnemonic:          words,
		ShortestWord:      bip39.ShortestWord,
		LongestWord:       bip39.LongestWord,
		QR:                qrc,
		MasterFingerprint: mfp,
		Font:              constant.Font,
	}
}

func compareGolden(t testing.TB, name string, plan engrave.Engraving) {
	t.Helper()
	p := filepath.Join("testdata", name+".bin")
	spline := engrave.PlanEngraving(conf, plan)
	bounds := bspline.Bounds{
		Max: bezier.Point{
			X: 85 * mm,
			Y: 85 * mm,
		},
	}
	sw := params.StrokeWidth
	if err := golden.CompareBSpline(p, *update, t.ArtifactDir(), sw, bounds, spline); err != nil {
		t.Fatal(err)
	}
}

// engraveStringRecovered runs EngraveSeedString under a recover so a panic on
// current code is reported as a test failure instead of crashing the run.
func engraveStringRecovered(t *testing.T, raw string) (err error, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	cx, e := codex32.New(raw)
	if e != nil {
		t.Fatalf("codex32.New(%q): %v", raw, e)
	}
	id, _, _ := cx.Split()
	s := SeedString{Title: id, Seed: cx.String(), Font: constant.Font}
	_, err = EngraveSeedString(params, s)
	return err, false
}

func TestEngraveSeedStringTooLong(t *testing.T) {
	cases := []struct{ name, s string }{
		// 93-char short code -> QR dim 37 (V5, unsupported).
		{"dim37", "ms10testsqqrsu9guyv4rzwplgex4gkmzd9c8wl593jfe4gdg47mtm3xt6tv7pelw7h7qxzs3rq0jvtg3ye6xggmhcl92"},
		// 127-char BIP-93 long-code reference -> QR dim 41 (V6, unsupported).
		{"dim41", "ms100c8vsm32zxfguhpchtlupzry9x8gf2tvdw0s3jn54khce6mua7lqpzygsfjd6an074rxvcemlh8wu3tk925acdefghjklmnpqrstuvwxy06fhpv80undvarhrak"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err, panicked := engraveStringRecovered(t, tc.s)
			if panicked {
				t.Fatalf("%s: EngraveSeedString panicked; want a returned error", tc.name)
			}
			if err == nil {
				t.Fatalf("%s: want non-nil error, got nil", tc.name)
			}
		})
	}
}

func TestEngraveSeedStringHappy(t *testing.T) {
	// 74-char codex32 short code -> QR dim 33 (V4, supported).
	const happy = "ms10leetsllhdmn9m42vcsamx24zrxgs3qrl7ahwvhw4fnzrhve25gvezzyq0pgjxpzx0ysaam"
	cx, err := codex32.New(happy)
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := cx.Split()
	s := SeedString{Title: id, Seed: cx.String(), Font: constant.Font}
	e, err := EngraveSeedString(params, s)
	if err != nil {
		t.Fatalf("happy path returned error: %v", err)
	}
	if e == nil {
		t.Fatal("happy path returned nil Engraving")
	}
}

// slip39Words builds an N-word slice of real SLIP-39 words for layout tests.
// frontSideSeed does not validate SLIP-39 checksums, so any valid-wordlist
// strings exercise the geometry.
func slip39Words(n int) []string {
	base := strings.Fields("duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard shadow pistol always adequate wildlife fancy gross oasis cylinder mustang wrist rescue view")
	out := make([]string, n)
	for i := range out {
		out[i] = base[i%len(base)]
	}
	return out
}

// seedLayout mirrors frontSideSeed's large-N (N>24) anchor computation so the
// test can assert the chosen geometry explicitly (pfsN and the no-overlap
// relation), independent of the byte-exact golden.
func seedLayout(n int) (pfs, col1Rows, col2Rows, col1Bot, col2Bot int) {
	full := params.F(4.1)
	col1Rows = (n + 1) / 2 // ceil(N/2)
	col2Rows = n / 2       // floor(N/2)
	pfs = full
	if alt := 16 * full / col1Rows; alt < pfs {
		pfs = alt
	}
	col1H := pfs * col1Rows
	plateY := params.F(85)
	top := (plateY - col1H) / 2
	col1Bot = top + pfs*col1Rows
	col2Bot = top + pfs*col2Rows
	return
}

func TestSLIP39Large(t *testing.T) {
	seedDesc := Seed{
		Mnemonic:     slip39Words(33),
		ShortestWord: slip39words.ShortestWord,
		LongestWord:  slip39words.LongestWord,
		Title:        "7945 #1 1/1",
		Font:         constant.Font,
	}
	side, err := EngraveSeed(params, seedDesc)
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "slip39-33-words", side)
}

func TestSLIP39LargeGeometry(t *testing.T) {
	pfs, col1Rows, col2Rows, col1Bot, col2Bot := seedLayout(33)
	// Pin the chosen adaptive scale (3.859mm), not just "some" non-overlap layout.
	if pfs != 24696 {
		t.Errorf("pfsN = %d, want 24696", pfs)
	}
	if col1Rows != 17 || col2Rows != 16 {
		t.Errorf("rows = %d/%d, want 17/16", col1Rows, col2Rows)
	}
	// No overlap: column 2's bottom edge must not fall below column 1's bottom.
	if col2Bot > col1Bot {
		t.Errorf("col2 bottom %d > col1 bottom %d (overlap)", col2Bot, col1Bot)
	}
	// Within plate bounds on the Y axis.
	plateY := params.F(85)
	if col1Bot > plateY || col2Bot > plateY {
		t.Errorf("off-plate: col1Bot=%d col2Bot=%d plateY=%d", col1Bot, col2Bot, plateY)
	}
}

func TestSLIP39_23WordPin(t *testing.T) {
	seedDesc := Seed{
		Mnemonic:     slip39Words(23),
		ShortestWord: slip39words.ShortestWord,
		LongestWord:  slip39words.LongestWord,
		Title:        "7945 #1 1/1",
		Font:         constant.Font,
	}
	side, err := EngraveSeed(params, seedDesc)
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "slip39-23-words", side)
}

func TestSLIP39_30WordInBounds(t *testing.T) {
	// N=30 takes the rebalanced N>24 path but stays at the full 4.1mm font.
	pfs, col1Rows, col2Rows, col1Bot, col2Bot := seedLayout(30)
	if pfs != params.F(4.1) {
		t.Errorf("N=30 pfsN = %d, want full font %d (4.1mm, no shrink)", pfs, params.F(4.1))
	}
	if col1Rows != 15 || col2Rows != 15 {
		t.Errorf("N=30 rows = %d/%d, want 15/15", col1Rows, col2Rows)
	}
	if col2Bot > col1Bot {
		t.Errorf("N=30 col2 bottom %d > col1 bottom %d (overlap)", col2Bot, col1Bot)
	}

	// And the actual engraved plan stays within [0, F(85)] on both axes.
	seedDesc := Seed{
		Mnemonic:     slip39Words(30),
		ShortestWord: slip39words.ShortestWord,
		LongestWord:  slip39words.LongestWord,
		Title:        "7945 #1 1/1",
		Font:         constant.Font,
	}
	side, err := EngraveSeed(params, seedDesc)
	if err != nil {
		t.Fatal(err)
	}
	plateMax := params.F(85)
	for _, k := range slices.Collect(engrave.PlanEngraving(conf, side)) {
		if k.Ctrl.X < 0 || k.Ctrl.Y < 0 || k.Ctrl.X > plateMax || k.Ctrl.Y > plateMax {
			t.Fatalf("N=30 knot %v out of plate bounds [0,%d]", k.Ctrl, plateMax)
		}
	}
}
