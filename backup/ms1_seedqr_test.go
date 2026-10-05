package backup

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	qr "github.com/seedhammer/kortschak-qr"
	"github.com/seedhammer/kortschak-qr/coding"
	"seedhammer.com/bezier"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/seedqr"
)

// ms1SeedQRVector is one row of testdata/ms1_seedqr_vectors.json. Every column
// is produced by the Rust generator in testdata/gen (ms-codec for ms1,
// rust-bip39 for entropy and word indices, the qrcode crate for the width), or
// copied verbatim from SeedSigner's spec, never by the Go code tested here.
// Sources, revisions and the command are in ms1_seedqr_vectors.provenance.json.
type ms1SeedQRVector struct {
	Name     string `json:"name"`
	Words    string `json:"words"`
	Entropy  string `json:"entropy"`
	MS1      string `json:"ms1"`
	SeedQR   string `json:"seedqr"`
	QRWidthM int    `json:"qr_width_m"`
}

func loadMS1SeedQRVectors(t *testing.T) []ms1SeedQRVector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "ms1_seedqr_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Vectors []ms1SeedQRVector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	// 10 SeedSigner rows plus the 16 12- and 24-word English rows of the
	// Trezor corpus. A shrunken file must not pass as a smaller green.
	if len(f.Vectors) != 26 {
		t.Fatalf("%d vectors, want 26", len(f.Vectors))
	}
	return f.Vectors
}

func vectorByName(t *testing.T, name string) ms1SeedQRVector {
	t.Helper()
	for _, v := range loadMS1SeedQRVectors(t) {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("no vector %q", name)
	return ms1SeedQRVector{}
}

func (v ms1SeedQRVector) mnemonic(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic(v.Words)
	if err != nil {
		t.Fatalf("%s: %v", v.Name, err)
	}
	return m
}

// independentFingerprint derives the master fingerprint the way TestCodex32
// does, straight from hdkeychain, not through the helper the function under
// test uses.
func independentFingerprint(t *testing.T, m bip39.Mnemonic) uint32 {
	t.Helper()
	mk, err := hdkeychain.NewMaster(bip39.MnemonicSeed(m, ""), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := mk.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	return bip32.Fingerprint(pk)
}

func TestMS1SeedQRVectors(t *testing.T) {
	words12, words24 := 0, 0
	for _, v := range loadMS1SeedQRVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			m := v.mnemonic(t)
			switch len(m) {
			case 12:
				words12++
			case 24:
				words24++
			default:
				t.Fatalf("%d words; the corpus is 12- and 24-word rows only", len(m))
			}
			if got := string(seedqr.QR(m)); got != v.SeedQR {
				t.Errorf("seedqr.QR = %s\nwant       %s", got, v.SeedQR)
			}
			back, ok := seedqr.Parse([]byte(v.SeedQR))
			if !ok || back.String() != m.String() {
				t.Errorf("seedqr.Parse(digits) = %v %v, want %q", back, ok, m.String())
			}
			ent, err := hex.DecodeString(v.Entropy)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(m.Entropy()); got != v.Entropy {
				t.Errorf("entropy = %s, want %s", got, v.Entropy)
			}
			ms1, err := codex32.EncodeMS1(ent)
			if err != nil {
				t.Fatal(err)
			}
			if ms1 != v.MS1 {
				t.Errorf("EncodeMS1 = %s\nwant        %s (ms-codec)", ms1, v.MS1)
			}
			code, err := qr.Encode(v.SeedQR, seedQRLevel)
			if err != nil {
				t.Fatal(err)
			}
			if code.Size != v.QRWidthM {
				t.Errorf("QR width at M = %d, want %d", code.Size, v.QRWidthM)
			}
			if code.Size > seedQRMaxSize {
				t.Errorf("QR width %d above the seed-plate cap %d", code.Size, seedQRMaxSize)
			}
		})
	}
	if words12 == 0 || words24 == 0 {
		t.Fatalf("12-word rows %d, 24-word rows %d; both lengths must be covered", words12, words24)
	}
}

// seedQRPlate builds the plate for a vector row with its true fingerprint.
func seedQRPlate(t *testing.T, v ms1SeedQRVector) (SeedString, bip39.Mnemonic) {
	t.Helper()
	m := v.mnemonic(t)
	cx, err := codex32.New(v.MS1)
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := cx.Split()
	return SeedString{
		Title:             id,
		Seed:              v.MS1,
		MasterFingerprint: independentFingerprint(t, m),
		Font:              constant.Font,
	}, m
}

func TestEngraveSeedStringSeedQRGolden(t *testing.T) {
	for _, tc := range []struct{ golden, row string }{
		{"ms1-seedqr-24", "seedsigner-tv1"},
		{"ms1-seedqr-12", "seedsigner-tv4"},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			plate, m := seedQRPlate(t, vectorByName(t, tc.row))
			if plate.MasterFingerprint == 0 {
				t.Fatal("the golden must carry a non-zero fingerprint")
			}
			p, err := EngraveSeedStringSeedQR(params, plate, m)
			if err != nil {
				t.Fatal(err)
			}
			compareGolden(t, tc.golden, p)
		})
	}
}

// seedQRGrid recovers the QR module grid from the knots the plate engraves
// inside the QR's box, which engraveSeedString places centred at x = 60 mm and
// centred vertically. ConstantQR engraves every black module and only black
// modules, so a module is black exactly when something lands on it.
func seedQRGrid(t *testing.T, p engrave.Engraving, dim int) qrGrid {
	t.Helper()
	sw := params.StrokeWidth
	pitch := sw * 3 // engraveSeedString's qrScale
	qrsz := dim * pitch
	lo := bezier.Pt(params.I(60)-qrsz/2, (params.I(85)-qrsz)/2)
	hi := bezier.Pt(lo.X+qrsz, lo.Y+qrsz)
	var pts []bezier.Point
	for c := range p {
		k, ok := c.AsKnot()
		if ok && k.Engrave && inRect(k.Knot, lo, hi) {
			pts = append(pts, k.Knot)
		}
	}
	if len(pts) == 0 {
		t.Fatal("nothing engraved inside the QR box")
	}
	// As passphraseQRGrid: module (0,0) is centred one stroke width plus a
	// radius in from the QR's top-left corner (engrave.centerOf).
	minX, minY := lo.X+sw+sw/2, lo.Y+sw+sw/2
	g := make(qrGrid, dim)
	for i := range g {
		g[i] = make([]bool, dim)
	}
	for _, q := range pts {
		x, okx := qrModuleIndex(q.X-minX, pitch, sw)
		y, oky := qrModuleIndex(q.Y-minY, pitch, sw)
		if !okx || !oky || x < 0 || x >= dim || y < 0 || y >= dim {
			t.Fatalf("engraved knot %v inside the QR box is not on a module centre", q)
		}
		g[y][x] = true
	}
	return g
}

// TestEngraveSeedStringSeedQRCarriesTheDigits reads the QR back off the
// engraving: the plate must hand a scanner the Standard SeedQR digits, and
// never the ms1 string (UI spec §4.4).
func TestEngraveSeedStringSeedQRCarriesTheDigits(t *testing.T) {
	for _, row := range []string{"seedsigner-tv1", "seedsigner-tv4", "trezor-english-00-12", "trezor-english-11-24"} {
		t.Run(row, func(t *testing.T) {
			v := vectorByName(t, row)
			plate, m := seedQRPlate(t, v)
			p, err := EngraveSeedStringSeedQR(params, plate, m)
			if err != nil {
				t.Fatal(err)
			}
			got := decodeQRAt(t, seedQRGrid(t, p, v.QRWidthM), coding.M)
			if got != v.SeedQR {
				t.Fatalf("plate QR = %q, want the SeedQR digits %q", got, v.SeedQR)
			}
			if strings.Contains(strings.ToLower(got), "ms1") {
				t.Fatalf("plate QR carries the ms1 string: %q", got)
			}
		})
	}
}

// TestEngraveSeedStringSeedQRAcceptsUppercase: the plate engraves the string
// in uppercase anyway, so a caller holding it in either case gets the plate.
func TestEngraveSeedStringSeedQRAcceptsUppercase(t *testing.T) {
	plate, m := seedQRPlate(t, vectorByName(t, "seedsigner-tv4"))
	plate.Seed = strings.ToUpper(plate.Seed)
	if _, err := EngraveSeedStringSeedQR(params, plate, m); err != nil {
		t.Fatal(err)
	}
	plate.MasterFingerprint = 0 // zero: no fingerprint row, nothing to check
	if _, err := EngraveSeedStringSeedQR(params, plate, m); err != nil {
		t.Fatal(err)
	}
}

func TestEngraveSeedStringSeedQRRefusals(t *testing.T) {
	v := vectorByName(t, "seedsigner-tv1")
	good, m := seedQRPlate(t, v)

	// One word changed, checksum repaired so the words are a VALID mnemonic of
	// another seed: only the ms1-vs-words comparison can catch it.
	other := append(bip39.Mnemonic(nil), m...)
	other[0] = (other[0] + 1) % bip39.NumWords
	other = other.FixChecksum()
	if !other.Valid() || other.String() == m.String() {
		t.Fatal("control: the changed mnemonic must be valid and different")
	}

	// An invalid checksum: the last word bumped until the checksum fails.
	bad := append(bip39.Mnemonic(nil), m...)
	for bad.Valid() {
		bad[len(bad)-1] = (bad[len(bad)-1] + 1) % bip39.NumWords
	}

	wrongFP := good
	wrongFP.MasterFingerprint ^= 1

	outOfRange := append(bip39.Mnemonic(nil), m...)
	outOfRange[3] = bip39.NumWords

	threeWords := bip39.Mnemonic{0, 0, 0}

	cases := []struct {
		name  string
		plate SeedString
		m     bip39.Mnemonic
		want  error
	}{
		{"word-changed", good, other, errSeedQRDisagree},
		{"wrong-fingerprint", wrongFP, m, errSeedQRFingerprint},
		{"bad-checksum", good, bad, errSeedQRInvalidMnemonic},
		{"word-out-of-range", good, outOfRange, errSeedQRInvalidMnemonic},
		{"three-words", good, threeWords, errSeedQRInvalidMnemonic},
		{"no-words", good, nil, errSeedQRInvalidMnemonic},
		{"string-of-another-seed", seedQRPlateFor(t, "seedsigner-tv2"), m, errSeedQRDisagree},
		{"not-an-ms1-string", SeedString{Seed: "ms10leetsllhdmn9m42vcsamx24zrxgs3qrl7ahwvhw4fnzrhve25gvezzyq0pgjxpzx0ysaam", Font: constant.Font}, m, errSeedQRDisagree},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e engrave.Engraving
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v; want a returned error", r)
					}
				}()
				e, err = EngraveSeedStringSeedQR(params, tc.plate, tc.m)
			}()
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if e != nil {
				t.Fatal("a refused plate returned an engraving")
			}
		})
	}
}

func seedQRPlateFor(t *testing.T, row string) SeedString {
	t.Helper()
	p, _ := seedQRPlate(t, vectorByName(t, row))
	return p
}

// TestEngraveSeedStringSeedQRWipesTheEntropy pins the wipe of the entropy
// buffer m.Entropy() returns. The hook hands over that slice itself; its
// snapshot must equal the vector's entropy first, or reading zero would
// prove nothing.
func TestEngraveSeedStringSeedQRWipesTheEntropy(t *testing.T) {
	v := vectorByName(t, "seedsigner-tv1")
	plate, m := seedQRPlate(t, v)
	var captured []byte
	var snapshot string
	seedQREntropyHook = func(ent []byte) {
		captured = ent
		snapshot = hex.EncodeToString(ent)
	}
	t.Cleanup(func() { seedQREntropyHook = nil })
	if _, err := EngraveSeedStringSeedQR(params, plate, m); err != nil {
		t.Fatal(err)
	}
	if snapshot != v.Entropy {
		t.Fatalf("hook saw %s, want the entropy %s", snapshot, v.Entropy)
	}
	for i, b := range captured {
		if b != 0 {
			t.Fatalf("entropy byte %d is still %#02x after the call returned", i, b)
		}
	}
}
