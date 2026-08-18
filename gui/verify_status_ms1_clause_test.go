package gui

import (
	"strings"
	"testing"
)

// ─── F-206 (S6b spec §3.3): the ms1 clause is COUNT-FREE ─────────────────────
//
// verifyStatusMS1Clause used to be the fixed singular "The ms1 secret you
// typed matched this seed.", appended whenever passRecord.full is set --
// unconditionally on passRecord.legs. F-206's OWN filed remedy (pluralise over
// legs) is unsound: legs is len(legs), one leg per FILLED SLOT, and "one seed
// fills several slots of a policy that puts it at several accounts"
// (gui/multisig_verify.go:299-300). At 1 seed filling 2 legs, pluralising
// would print "the ms1 secrets you typed" over an operator who typed ONE --
// a new over-claiming falsehood, which R-D forbids.
//
// passRecord CANNOT DISTINGUISH "1 seed / 2 legs" FROM "2 seeds / 2 legs" --
// it carries only {full, legs, suppliedCosigners}, no seed count. So this is a
// FLOW-LEVEL assertion (spec's own words: "as a unit test it silently degrades
// to two cases and stops exercising the middle one, which is the case that
// kills the filed remedy"): it drives the REAL multisigVerifyFlow, typing seeds
// one at a time, and only THAT can make "1 seed filling 2 legs" and "2 seeds
// filling 2 legs" different walks that happen to produce the SAME passRecord
// shape -- which is exactly the case a passRecord-literal unit test cannot
// construct honestly (it would have to assert something about how many times a
// seed was typed, and a literal has no such fact to assert).
//
// ALL THREE CASES SHARE ONE FIXTURE: Trace B, full mode (s5TraceBEngraved(t,
// true)) -- master A at slots @0 and @1 (different origins, same words),
// master B at slot @2. Measured: 2 distinct ms1 plates (buildEngraveTail
// dedupes on the MS1 STRING VALUE, not on registry SeedID, so master A's two
// slot registrations mint only one ms1 plate between them) at ms1Plates[0]=A,
// ms1Plates[1]=B; mk1Plates[i] is slot i's plate.
//
//	1 seed / 1 leg   expectedSlots={0}    type A once             -> legs=1
//	1 seed / 2 legs  expectedSlots={0,1}  type A once (fills both) -> legs=2
//	2 seeds / 2 legs expectedSlots={0,2}  type A, then B           -> legs=2
func TestMS1ClauseIsCountFreeAcrossSeedAndLegCounts(t *testing.T) {
	md1, mk1Plates, ms1Plates := s5TraceBEngraved(t, true)
	if len(mk1Plates) != 3 {
		t.Fatalf("Trace B engraved %d mk1 plate(s), want 3", len(mk1Plates))
	}
	if len(ms1Plates) != 2 {
		t.Fatalf("Trace B (full) minted %d ms1 plate(s), want 2 (the dedupe by MS1 "+
			"string value should fold master A's two slot registrations into one)", len(ms1Plates))
	}

	cases := []struct {
		name     string
		expected []int
		mk1s     [][]string // gathered plates, in expectedSlots order
		seeds    []string   // typed one at a time
		ms1s     []string   // typed once per seed, same order as seeds
		wantLegs int
	}{
		{
			name:     "1 seed 1 leg",
			expected: []int{0},
			mk1s:     [][]string{mk1Plates[0]},
			seeds:    []string{fixtureMasterA},
			ms1s:     []string{ms1Plates[0]},
			wantLegs: 1,
		},
		{
			name:     "1 seed 2 legs",
			expected: []int{0, 1},
			mk1s:     [][]string{mk1Plates[0], mk1Plates[1]},
			seeds:    []string{fixtureMasterA},
			ms1s:     []string{ms1Plates[0]},
			wantLegs: 2,
		},
		{
			name:     "2 seeds 2 legs",
			expected: []int{0, 2},
			mk1s:     [][]string{mk1Plates[0], mk1Plates[2]},
			seeds:    []string{fixtureMasterA, fixtureMasterB},
			ms1s:     []string{ms1Plates[0], ms1Plates[1]},
			wantLegs: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var records []string
			records = append(records, md1...)
			for _, p := range tc.mk1s {
				records = append(records, p...)
			}
			rec := s6bDriveFullMultisigVerify(t, records, tc.expected, md1, tc.seeds, tc.ms1s)
			if rec.pass == nil {
				t.Fatal("the verify did not record a pass")
			}
			if !rec.pass.full {
				t.Fatal("the verify did not record full mode")
			}
			if rec.pass.legs != tc.wantLegs {
				t.Fatalf("recorded %d leg(s), want %d -- this case's premise (%d seed(s), "+
					"%d expected slot(s)) did not produce the shape the case name claims",
					rec.pass.legs, tc.wantLegs, len(tc.seeds), len(tc.expected))
			}

			line := buildVerifyPassLine(*rec.pass)
			if !strings.Contains(line, "The ms1 you typed for each seed matched.") {
				t.Errorf("[%s] pass line does not carry the count-free ms1 clause: %q", tc.name, line)
			}
			// THE OVER-CLAIMING DIRECTION, NAMED EXPLICITLY: the struck singular
			// wording and any naive pluralisation of it must both be absent, in
			// EVERY case -- including "1 seed 2 legs", where a plural would be the
			// new falsehood F-206's own filed remedy would have introduced.
			for _, forbidden := range []string{
				"The ms1 secret you typed matched this seed.",
				"The ms1 secrets you typed matched this seed.",
				"ms1 secrets you typed",
			} {
				if strings.Contains(line, forbidden) {
					t.Errorf("[%s] pass line carries %q, which F-206 (S6b spec §3.3) removes: %q",
						tc.name, forbidden, line)
				}
			}
		})
	}
}

// s6bDriveFullMultisigVerify drives the REAL multisigVerifyFlow in FULL mode,
// typing `seeds[i]` and then `ms1s[i]` for each i in order, with a
// "TYPE THE NEXT SEED" press between seeds. It returns whatever verifyRecord
// the flow wrote.
func s6bDriveFullMultisigVerify(t *testing.T, records []string, expected []int,
	engravedMd1 []string, seeds, ms1s []string,
) verifyRecord {
	t.Helper()
	if len(seeds) != len(ms1s) {
		t.Fatalf("s6bDriveFullMultisigVerify: %d seed(s) but %d ms1(s)", len(seeds), len(ms1s))
	}
	if len(seeds) == 0 {
		t.Fatal("s6bDriveFullMultisigVerify: no seed to type")
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)
	var rec verifyRecord
	done := false
	frame, quit := runUI(ctx, func() {
		multisigVerifyFlow(ctx, &descriptorTheme, true /* FULL */, expected, engravedMd1, &rec)
		done = true
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 32); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	for i, phrase := range seeds {
		if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
			t.Fatalf("seed %d: the flow did not ask for a seed; got %q", i, c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, phrase)
		if c, ok := pumpUntil(frame, "passphrase", 200); !ok {
			t.Fatalf("seed %d: the passphrase prompt was not reached; got %q", i, c)
		}
		click(&ctx.Router, Button3) // Skip -- Trace B's masters carry no passphrase
		frame()
		if c, ok := pumpUntil(frame, "Type ms1", 96); !ok {
			t.Fatalf("seed %d: full mode did not ask for the ms1; got %q", i, c)
		}
		runes(&ctx.Router, strings.ToLower(ms1s[i]))
		click(&ctx.Router, Button3)
		frame()
		if i < len(seeds)-1 {
			if c, ok := pumpUntil(frame, "not checked yet", 96); !ok {
				t.Fatalf("seed %d: did not reach the next-seed offer; got %q", i, c)
			}
			click(&ctx.Router, Button3) // TYPE THE NEXT SEED
			frame()
		}
	}

	last, ok := pumpUntil(frame, "Verify OK", 96)
	if !ok {
		t.Fatalf("the verify did not reach a clean pass; final screen %q", last)
	}
	click(&ctx.Router, Button3) // dismiss the success notice -- showNotice blocks until dismissed
	for i := 0; i < 32 && !done; i++ {
		if _, ok := frame(); !ok {
			break
		}
	}
	if !done {
		t.Fatal("multisigVerifyFlow did not return after the success notice was dismissed")
	}
	return rec
}
