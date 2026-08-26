package gui

import (
	"path/filepath"
	"testing"

	"seedhammer.com/bspline"
	"seedhammer.com/internal/golden"
)

// ═══ THE PLATES, PINNED ══════════════════════════════════════════════════════
//
// Every other transaction test asserts on STRINGS: that a legend contains a
// txid, that a title names a plate. None of them can see what is actually cut.
// A legend can carry the right words and be laid out on top of the QR symbol,
// clipped at the plate edge, or drawn at a size the steel cannot hold, and the
// string assertions all pass.
//
// These four goldens are the artifacts themselves -- the b-spline the engraver
// executes, through the real planner and toPlate -- for the four plates this
// feature can produce:
//
//	tx-qr          a signed transaction as one QR plate with its legend
//	tx-text        a confirmed 6-string set as one TEXT plate
//	tx-unconfirmed an incomplete set, legend REPLACED (ruling 2026-08-25)
//	tx-unsigned-qr an unsigned transaction, legend REPLACED (2026-08-25b)
//
// The last two are the point. The rulings put a warning into STEEL, and steel
// is the one output that cannot be recut cheaply; a substitution that is right
// in a string and wrong on the plate is exactly the failure the rulings exist
// to prevent, and it is invisible to every assertion above.
//
// THE CONTRACT IS freetext_sizeproof_golden_test.go's, not backup/testdata's:
// these exist in order to MOVE. What they buy is that a movement is SEEN and
// attributed. When one fails, do not reach for -update first. Run
//
//	go test ./gui -run TestTransactionPlateGoldens -artifacts -outputdir=DIR
//
// which writes tx-*.bin.svg (what the code draws now) and tx-*.bin.orig.svg
// (what the golden holds) at the production stroke; diff the pair. Only once
// that diff is the edit you meant:
//
//	go test ./gui -run TestTransactionPlateGoldens -update
//
// SCOPE -update WITH -run, as that file says: a bare `go test ./... -update`
// also rewrites backup's sixteen frozen goldens.
func TestTransactionPlateGoldens(t *testing.T) {
	cases := []struct {
		name    string
		records []string
		qr      bool
	}{
		{"tx-qr", []string{"tx:" + rawHexOfEven(t)}, true},
		{"tx-text", txEven, false},
		{"tx-unconfirmed", txEven[:3], false},
		{"tx-unsigned-qr", []string{"tx:" + txStrippedHex}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pl := newPlatform()
			ctx := NewContext(pl)
			ctx.sysw = sessionWith(tc.records...)
			cands, _ := payloadTransactions(ctx)
			if len(cands) != 1 {
				t.Fatalf("%d candidates", len(cands))
			}
			c := cands[0]

			// PLANNED THE WAY THE PROGRAM PLANS IT, through the choice the
			// operator would be offered -- so a golden can never be recorded
			// for a plate the flow would not produce.
			kinds := transactionPlateKinds(c)
			want := "TEXT PLATES"
			if tc.qr {
				want = "QR PLATES"
			}
			if !contains(kinds, want) {
				t.Fatalf("the flow does not offer %s for this payload (offers %v)", want, kinds)
			}
			var plates []Plate
			var err error
			if tc.qr {
				plates, _, _, err = planTransactionQRPlates(pl, c)
			} else {
				plates, _, err = planTransactionTextPlates(pl, c)
			}
			if err != nil {
				t.Fatalf("planning: %v", err)
			}
			if len(plates) == 0 {
				t.Fatal("no plates")
			}
			// A golden over an EMPTY engraving passes forever, and -update
			// would bake that in silently. Measure unions only ENGRAVED
			// segments, so this asks about ink and not about travel moves.
			for i, p := range plates {
				if bspline.Measure(p.Spline).Bounds.Empty() {
					t.Fatalf("plate %d cuts nothing", i)
				}
			}

			P := pl.EngraverParams()
			bounds := bspline.Bounds{Max: SquarePlate.Dims(P.Millimeter)}
			dir := t.ArtifactDir()
			for i, p := range plates {
				name := tc.name
				if len(plates) > 1 {
					name = tc.name + "-" + string(rune('1'+i))
				}
				err := golden.CompareBSpline(filepath.Join("testdata", name+".bin"),
					*update, dir, P.StrokeWidth, bounds, p.Spline)
				if err != nil {
					t.Fatalf("%s moved: %v\n\n"+
						"The PLATE has changed, not just a string. Re-run with\n"+
						"  -artifacts -outputdir=DIR\n"+
						"and diff\n"+
						"  %s   what the code draws NOW\n"+
						"  %s   what this golden holds\n"+
						"Only once that diff is the edit you meant, re-record with\n"+
						"  go test ./gui -run TestTransactionPlateGoldens -update\n"+
						"NEVER with a bare `go test ./... -update`.",
						name, err,
						filepath.Join(dir, name+".bin.svg"),
						filepath.Join(dir, name+".bin.orig.svg"))
				}
			}
		})
	}
}

