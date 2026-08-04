package backup

import (
	"slices"
	"strings"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
)

// plateKnot is one commanded point of an engraving.
type plateKnot struct {
	P bezier.Point
	// Engrave is true for a cutting move, false for a repositioning move.
	Engrave bool
}

func passphraseEngraving(t *testing.T, plate Passphrase) engrave.Engraving {
	t.Helper()
	eng, err := EngravePassphrase(params, plate)
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

// passphrasePoints returns the points the machine is commanded to visit. It
// reads the command stream rather than PlanEngraving's spline, because the
// planner prepends two clamping knots at the origin that are not plate
// geometry; every planned knot is inside the convex hull of these points.
func passphrasePoints(t *testing.T, plate Passphrase) []plateKnot {
	t.Helper()
	var out []plateKnot
	for c := range passphraseEngraving(t, plate) {
		k, ok := c.AsKnot()
		if !ok {
			continue
		}
		out = append(out, plateKnot{P: k.Knot, Engrave: k.Engrave})
	}
	return out
}

// passphrasePlan plans plate and returns every knot the machine will visit,
// including its timing.
func passphrasePlan(t *testing.T, plate Passphrase) []bspline.Knot {
	t.Helper()
	return slices.Collect(engrave.PlanEngraving(conf, passphraseEngraving(t, plate)))
}

// knotBounds is the bounding box of pts.
func knotBounds(pts []plateKnot) bspline.Bounds {
	if len(pts) == 0 {
		return bspline.Bounds{}
	}
	b := bspline.Bounds{Min: pts[0].P, Max: pts[0].P}
	for _, k := range pts[1:] {
		b.Min.X = min(b.Min.X, k.P.X)
		b.Min.Y = min(b.Min.Y, k.P.Y)
		b.Max.X = max(b.Max.X, k.P.X)
		b.Max.Y = max(b.Max.Y, k.P.Y)
	}
	return b
}

// inRect reports whether p is inside [lo, hi] on both axes.
func inRect(p bezier.Point, lo, hi bezier.Point) bool {
	return lo.X <= p.X && p.X <= hi.X && lo.Y <= p.Y && p.Y <= hi.Y
}

// The passphrase must be engraved verbatim. SeedString uppercases its content;
// doing that here would silently destroy the secret.
func TestPassphrasePreservesCase(t *testing.T) {
	got := passphraseGlyphs("Hunter2")
	if got != "Hunter2" {
		t.Errorf("case not preserved: %q", got)
	}
}

// Every space becomes the visible mark; nothing else changes.
func TestPassphraseSpaceSubstitution(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ab", "ab"},
		{"a b", "a\x1fb"},
		{"a  b", "a\x1f\x1fb"},
		{" ab", "\x1fab"},
		{"ab ", "ab\x1f"},
		{"  ", "\x1f\x1f"},
	}
	for _, tc := range tests {
		if got := passphraseGlyphs(tc.in); got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The engraved stream must contain no literal 0x20. A real space would be
// invisible on metal, which is the entire reason the mark exists.
func TestPassphraseEngravesNoLiteralSpace(t *testing.T) {
	if strings.ContainsRune(passphraseGlyphs("a b c"), ' ') {
		t.Error("engraved text contains a literal 0x20")
	}
}

// The 100-character worst case must stay inside the usable area: the plate is
// 85x85mm and the 10mm margin bands are reserved for metadata.
func TestPassphraseLayoutFitsNoQR(t *testing.T) {
	p := Passphrase{Passphrase: strings.Repeat("a", 100), Font: constant.Font}
	pts := passphrasePoints(t, p)
	if len(pts) == 0 {
		t.Fatal("no knots engraved")
	}
	lo := bezier.Pt(params.F(innerMargin), params.F(innerMargin))
	hi := bezier.Pt(params.F(85-innerMargin), params.F(85-innerMargin))
	for _, k := range pts {
		if !inRect(k.P, lo, hi) {
			t.Fatalf("knot %v outside the usable area [%v, %v]", k.P, lo, hi)
		}
	}
	b := knotBounds(pts)
	// The block is the full 10 rows x 10 characters at the pinned em, not a
	// length-scaled one: 40mm wide, 60mm tall.
	if got, want := b.Max.X-b.Min.X, params.F(40); got > want {
		t.Errorf("block width %d > %d", got, want)
	}
	if got, want := b.Max.Y-b.Min.Y, params.F(60); got > want {
		t.Errorf("block height %d > %d", got, want)
	}
}

// The engraver must receive the visible mark, never a real space: 0x20 has an
// advance but an empty spline, so a passphrase of spaces would engrave nothing
// at all.
func TestPassphraseEngravesMarkNotSpace(t *testing.T) {
	spaces := passphrasePoints(t, Passphrase{Passphrase: "   ", Font: constant.Font})
	ink := 0
	for _, k := range spaces {
		if k.Engrave {
			ink++
		}
	}
	if ink == 0 {
		t.Error("a passphrase of spaces engraved no ink: 0x20 reached the stringer")
	}
	// Engraving " " must be indistinguishable from engraving the mark itself.
	// passphraseGlyphs is the identity on a string that already holds the mark,
	// so equality here can only mean the space was translated.
	viaSpace := passphrasePlan(t, Passphrase{Passphrase: "a b c", Font: constant.Font})
	viaMark := passphrasePlan(t, Passphrase{
		Passphrase: "a" + string(SpaceMark) + "b" + string(SpaceMark) + "c",
		Font:       constant.Font,
	})
	if !slices.Equal(viaSpace, viaMark) {
		t.Errorf("space and mark engrave differently: %d vs %d knots", len(viaSpace), len(viaMark))
	}
}
