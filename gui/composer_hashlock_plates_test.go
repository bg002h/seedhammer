package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/backup"
	"seedhammer.com/hashlock"
	"seedhammer.com/sysw"
)

// ─── H6 Task 10: the Hashlock plates flow (spec §5.2, §6.3) ──────────────────

// composerH6PlatesSession is §12 item 3's payload for this route: one preimage
// record and one phrase record, and nothing else.
func composerH6PlatesSession(t *testing.T) *syswSession {
	t.Helper()
	x := hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
	return composerSessionWith(nil, []string{
		composerTestPreimageRecord(t, x),
		composerTestPhraseRecord(sysw.HashlockSHA256, "a second hashlock phrase"),
	})
}

// TestComposerDoorOffersHashlockPlatesOnlyForAPayloadThatHasOne is §5.2 step 1.
//
// MUTATION: offer the route when the payload holds neither class -> the
// keys-only row fails, and the door names a route with nothing to take -- the
// F-437 defect the door exists to remove.
func TestComposerDoorOffersHashlockPlatesOnlyForAPayloadThatHasOne(t *testing.T) {
	x := hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
	for _, tc := range []struct {
		name    string
		session *syswSession
		want    bool
	}{
		{"key records only", composerSessionWith([]string{composerTestKeyRecord}, nil), false},
		{"a hash: record but no material", composerSessionWith([]string{composerTestHashRecord}, nil), false},
		{"a preimage record", composerSessionWith(nil, []string{composerTestPreimageRecord(t, x)}), true},
		{"a phrase record", composerSessionWith(nil,
			[]string{composerTestPhraseRecord(sysw.HashlockSHA256, hashlockAnchorPhrase)}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := composerDoorHasPreimage(tc.session); got != tc.want {
				t.Errorf("composerDoorHasPreimage = %v, want %v", got, tc.want)
			}
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				ctx.sysw = tc.session
				frame, quit := runUI(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
				defer quit()
				content, ok := pumpUntil(frame, "Build a new policy", 16)
				if !ok {
					t.Fatalf("the door never drew.\nLast frame: %q", content)
				}
				if got := uiContains(content, "Hashlock plates"); got != tc.want {
					t.Errorf("Hashlock plates offered = %v, want %v.\nFrame: %q", got, tc.want, content)
				}
				assertChoiceLabelFits(t, "Hashlock plates")
			})
		})
	}
}

// TestComposerDoorLeadNamesTheRecordsTheFourthRouteActsOn is §5.2 step 2, and
// §12 item 3's payload is the one that read wrongest: composerDoorCounts knew
// nothing about these classes, so the door drew "No keys loaded" directly above
// a route offered for exactly them.
//
// MUTATION: drop the preimage count from composerDoorCounts -> the lead says
// only that there are no keys, and the route it offers is unexplained.
func TestComposerDoorLeadNamesTheRecordsTheFourthRouteActsOn(t *testing.T) {
	s := composerH6PlatesSession(t)
	keys, seeds, preimages, inert := composerDoorCounts(s)
	if keys != 0 || seeds != 0 || preimages != 2 || inert != 0 {
		t.Fatalf("counts = %d/%d/%d/%d, want 0/0/2/0", keys, seeds, preimages, inert)
	}
	got := strings.Join(composerDoorLines(s, false), " | ")
	if !strings.Contains(got, "2 preimage or phrase records loaded.") {
		t.Errorf("the door's lead does not name the records the fourth route acts on.\nLines: %s", got)
	}
	// A payload with no such record says nothing about them.
	plain := strings.Join(composerDoorLines(composerSessionWith([]string{composerTestKeyRecord}, nil), false), " | ")
	if strings.Contains(plain, "preimage or phrase") {
		t.Errorf("the door counts preimage records that are not there.\nLines: %s", plain)
	}
}

// TestHashlockPlatesListDrawsWithoutDeriving is §5.2 step 3 item 1, measured
// against a real hardened derivation rather than asserted structurally.
//
// MUTATION: derive per frame (or at list time) -> the list costs one KDF per
// phrase record, which on the SH2 is about 10 s each, and this fails.
func TestHashlockPlatesListDrawsWithoutDeriving(t *testing.T) {
	s := composerSessionWith(nil, []string{
		composerTestPhraseRecord(sysw.HashlockHardened, hashlockAnchorPhrase),
		composerTestPhraseRecord(sysw.HashlockHardened, "a second hashlock phrase"),
		composerTestPhraseRecord(sysw.HashlockHardened, "a third hashlock phrase"),
	})
	t0 := time.Now()
	hashlock.PreimageHardened([]byte(hashlockAnchorPhrase))
	kdf := time.Since(t0)
	t1 := time.Now()
	recs := hashlockPlatesRecords(s)
	rows := hashlockPlatesRows(recs)
	build := time.Since(t1)
	t.Logf("one hardened derivation %v; the list %v", kdf, build)
	if len(recs) != 3 {
		t.Fatalf("%d records, want 3", len(recs))
	}
	for i, r := range rows {
		want := composerHashPhraseRow(i+1, nil)
		if r != want {
			t.Errorf("row %d is %q, want %q", i, r, want)
		}
	}
	if build > kdf/4 {
		t.Errorf("the list took %v against a %v derivation: three records would be a "+
			"30 s stall before a list could be drawn, which §5.1 rejects", build, kdf)
	}
}

// TestHashlockPlatesDerivesOncePerPick is §5.2 step 3 items 2 and 3: picking
// derives ONCE and the result lives in a LOCAL of the flow, so a redraw, a page
// or a Back does not re-run the KDF.
//
// MUTATION: drop the `if recs[i].derived` guard -> the second call re-derives
// and the timing assertion fails.
func TestHashlockPlatesDerivesOncePerPick(t *testing.T) {
	s := composerSessionWith(nil, []string{
		composerTestPhraseRecord(sysw.HashlockHardened, hashlockAnchorPhrase),
	})
	recs := hashlockPlatesRecords(s)
	if recs[0].derived {
		t.Fatal("a phrase record arrives derived")
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	// The derivation runs on the flow's own goroutine here: the countdown draws
	// through ctx.Frame, which this harness pumps.
	synctest.Test(t, func(t *testing.T) {
		p2 := newPlatform()
		p2.display = sh2DisplaySize
		ctx2 := NewContext(p2)
		ok := false
		frame, quit := runUI(ctx2, func() { ok = hashlockPlatesDerive(ctx2, &descriptorTheme, recs, 0) })
		defer quit()
		for i := 0; i < 4096 && !ok; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if !ok {
			t.Fatal("the derivation never completed")
		}
	})
	if !recs[0].derived {
		t.Fatal("the derivation did not reach the flow's own slice")
	}
	want := hashlock.PreimageHardened([]byte(hashlockAnchorPhrase))
	if recs[0].preimage != want || !recs[0].digest.Equal(composerTestLock(hashlock.DigestSHA256(&want))) {
		t.Error("the derived preimage is not the record's")
	}
	// The SECOND call must not re-run the KDF: it returns immediately, without
	// a frame, which is why it can be called outside the harness at all.
	t0 := time.Now()
	if !hashlockPlatesDerive(ctx, &descriptorTheme, recs, 0) {
		t.Fatal("the second call did not report the record derived")
	}
	again := time.Since(t0)
	t.Logf("the second pick took %v", again)
	if again > time.Millisecond {
		t.Errorf("the second pick took %v: the KDF ran again", again)
	}
	// And the row now carries the digest.
	if got := hashlockPlatesRows(recs)[0]; !strings.HasPrefix(got, "phrase 1  ") {
		t.Errorf("the derived row still reads %q", got)
	}
	// The flow's scrub leaves nothing behind.
	hashlockPlatesScrub(recs)
	if recs[0].derived || recs[0].preimage != [32]byte{} || len(recs[0].phrase) != 0 {
		t.Error("the flow's scrub left material in its own slice")
	}
}

// TestHashlockPlatesLocatorAlwaysCarriesTheHashRow is §5.2 step 3 item 4 and
// §6.3, over the three payload shapes that change the locator.
//
// MUTATION: omit the locator's `hash` row for a phrase record -> the non-empty
// assertion fails, which is the row standing between the operator and a bearer
// plate with no locator at all.
// MUTATION: print a `path` row here -> this flow builds no composition and the
// number would name a path that does not exist.
func TestHashlockPlatesLocatorAlwaysCarriesTheHashRow(t *testing.T) {
	x := hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
	h := composerTestLock(hashlock.DigestSHA256(&x))
	digestRow := "hash  " + hashlockFirst8Last8(h)

	t.Run("no md1 and no matching hash: record", func(t *testing.T) {
		s := composerSessionWith(nil, []string{composerTestPreimageRecord(t, x)})
		loc := hashlockPlatesLocator(s, hashlockPlatesRecords(s)[0])
		if len(loc) != 1 || loc[0] != digestRow {
			t.Fatalf("locator = %q, want exactly %q", loc, []string{digestRow})
		}
	})

	t.Run("a matching hash: record in the same payload", func(t *testing.T) {
		s := composerSessionWith([]string{composerTestHashRecord, "hash:" + hashlockDigestHex(h)},
			[]string{composerTestPreimageRecord(t, x)})
		loc := hashlockPlatesLocator(s, hashlockPlatesRecords(s)[0])
		if !containsLine(loc, digestRow) {
			t.Errorf("locator %q has no hash row", loc)
		}
		if !containsLine(loc, "matches hash 2 in the payload") {
			t.Errorf("locator %q does not name the payload position", loc)
		}
	})

	t.Run("a phrase record, derived", func(t *testing.T) {
		// THE CARRIER THE RULE IS WRITTEN FOR. A preimage record arrives
		// derived -- hashlockPlatesRecords decodes X at list time -- so a
		// locator test built only from one cannot see the phrase branch at all.
		const phrase = "a hardened payload hashlock phrase"
		px := hashlock.PreimageHardened([]byte(phrase))
		ph := composerTestLock(hashlock.DigestSHA256(&px))
		s := composerSessionWith(nil,
			[]string{composerTestPhraseRecord(sysw.HashlockHardened, phrase)})
		r := hashlockPlatesRecords(s)[0]
		if !r.isPhrase || r.derived {
			t.Fatalf("the fixture is not an underived phrase record: isPhrase=%v derived=%v",
				r.isPhrase, r.derived)
		}
		// What hashlockPlatesDerive leaves behind, set here so the ROW is what
		// is under test and not the countdown.
		r.preimage, r.digest, r.derived = px, ph, true
		loc := hashlockPlatesLocator(s, r)
		want := "hash  " + hashlockFirst8Last8(ph)
		if !containsLine(loc, want) {
			t.Fatalf("locator = %q, want a %q row: a phrase-form plate with no locator "+
				"at all is the worst artifact this stage can cut", loc, want)
		}
	})

	t.Run("an md1 record supplies the stub", func(t *testing.T) {
		st := composerH6PlateState(t, "anchor a")
		chunks, err := composerTemplateChunksFor(st)
		if err != nil {
			t.Fatal(err)
		}
		s := composerSessionWith(chunks, []string{composerTestPreimageRecord(t, x)})
		loc := hashlockPlatesLocator(s, hashlockPlatesRecords(s)[0])
		stub := ""
		for _, r := range loc {
			if strings.HasPrefix(r, "mk1 stub (") {
				stub = r
			}
		}
		if stub == "" {
			t.Fatalf("locator %q has no mk1 stub row, though the payload holds an md1", loc)
		}
		// The label is composer_stub.go's own literal, so the plate and the
		// screen the operator copied into their notebook use the same words.
		if !strings.HasPrefix(stub, "mk1 stub (template): ") {
			t.Errorf("the stub row is %q; this payload's md1 is a TEMPLATE", stub)
		}
		if !containsLine(loc, digestRow) {
			t.Errorf("locator %q has no hash row", loc)
		}
		for _, r := range loc {
			if strings.HasPrefix(r, "path ") {
				t.Errorf("the locator names %q, but this flow builds no composition", r)
			}
		}
	})
}

// TestHashlockPlatesFlowLocatorCarriesTheDerivedDigest is the OTHER HALF of
// §5.2 step 3 item 4, and the half nothing in this package could fail on until
// the seam below existed: the locator's `hash` row is printed from THAT
// RESULT -- the derived digest -- not merely present.
//
// A UNIT CALL CANNOT SEE THIS. hashlockPlateLocator always appends a `hash`
// row, so "NON-EMPTY" is true by construction; what carries the meaning is that
// the digest on the plate is the one the HOST answers for this phrase, which
// only holds if the locator is built AFTER hashlockPlatesDerive. The want value
// below is computed in this test from hashlock.PreimageSHA256, never read back
// out of the flow.
//
// MUTATION: omit the locator's `hash` row when the record is a phrase -> the
// row assertion fails, which is the row standing between the operator and a
// bearer plate with no locator at all.
// MUTATION: build the locator one statement earlier, before hashlockPlatesDerive
// -> the record's digest is still nil at that point and hashlockFirst8Last8
// panics on the nil lock. It used to read `hash  00000000..00000000` and fail
// the digest assertion instead; a nil *md.HashLock is what replaced the zero
// [32]byte, and it makes the same mutation LOUDER rather than quieter.
func TestHashlockPlatesFlowLocatorCarriesTheDerivedDigest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const phrase = "a payload hashlock phrase"
		x := hashlock.PreimageSHA256([]byte(phrase))
		want := "hash  " + hashlockFirst8Last8(composerTestLock(hashlock.DigestSHA256(&x)))
		s := composerSessionWith(nil,
			[]string{composerTestPhraseRecord(sysw.HashlockSHA256, phrase)})

		var built []backup.Hashlock
		composerHashlockPlateBuiltHook = func(d backup.Hashlock) { built = append(built, d) }
		t.Cleanup(func() { composerHashlockPlateBuiltHook = nil })

		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = s
		frame, drawer, quit := runUITouch(ctx, func() { composerHashlockPlatesFlow(ctx, &descriptorTheme) })
		defer quit()
		if c, ok := pumpUntil(frame, "derive to see the digest", 24); !ok {
			t.Fatalf("the list never drew.\nLast frame: %q", c)
		}
		pts := plateHitPoints(ctx, drawer())
		if len(pts) != 1 {
			t.Fatalf("the list drew %d rows, want 1", len(pts))
		}
		tap(&ctx.Router, drawer(), pts[0])
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "do not cut this preimage", 24); !ok {
			t.Fatalf("the form pick never drew.\nLast frame: %q", c)
		}
		pts = plateHitPoints(ctx, drawer())
		if len(pts) != 4 {
			t.Fatalf("the form pick drew %d rows, want 4", len(pts))
		}
		tap(&ctx.Router, drawer(), pts[1]) // `phrase + method`
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave Plate", 32); !ok {
			t.Fatalf("the engrave screen never drew.\nLast frame: %q", c)
		}
		if len(built) != 1 {
			t.Fatalf("the flow built %d plates, want 1", len(built))
		}
		desc := built[0]
		if desc.Form != backup.HashlockPhrase || desc.Phrase != phrase {
			t.Fatalf("the flow built form=%v phrase=%d chars, want the phrase form",
				desc.Form, len(desc.Phrase))
		}
		if !containsLine(desc.Locator, want) {
			t.Fatalf("the plate's locator = %q, want a %q row. A locator built before the "+
				"derive reads hash  00000000..00000000 and names nothing: a bearer phrase "+
				"in plain text with no digest, no path and no payload position",
				desc.Locator, want)
		}
		click(&ctx.Router, Button1) // abort the cut, so the flow returns
	})
}

// TestHashlockPlatesAbortDrawsNeitherSection84Arm is §5.2 step 5, and the arm
// it refuses by name is the obvious implementation.
//
// MUTATION: reuse §8.4a here -> the assertion below fails, because the material
// is still in flash and the body would say it is gone.
func TestHashlockPlatesAbortDrawsNeitherSection84Arm(t *testing.T) {
	body := composerCopyHashlockPlatesNotCut()
	for _, banned := range []string{
		"dies with this composition", "Do not fund this wallet",
		"A PREIMAGE PLATE WAS CUT", "NO PREIMAGE PLATE WAS CUT",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("this flow's abort borrows §8.4's wording %q, which is false here: "+
				"the flow builds no composition and the material stays in the payload, "+
				"in flash", banned)
		}
	}
	if !strings.Contains(body, "still in this payload") {
		t.Errorf("this flow's abort does not say the record survives: %q", body)
	}
	synctest.Test(t, func(t *testing.T) {
		s := composerH6PlatesSession(t)
		p := newEngravedAwarePlatform()
		p.engraver = newEngraver()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = s
		var seen []string
		frame, drawer, quit := runUITouch(ctx, func() { composerHashlockPlatesFlow(ctx, &descriptorTheme) })
		defer quit()
		record := func() (string, bool) {
			c, ok := frame()
			if ok {
				seen = append(seen, c)
			}
			return c, ok
		}
		c, ok := pumpUntil(record, "can be cut as preimage plates", 24)
		if !ok {
			t.Fatalf("the list never drew.\nLast frame: %q", c)
		}
		pts := plateHitPoints(ctx, drawer())
		if len(pts) != 2 {
			t.Fatalf("the list drew %d rows, want 2", len(pts))
		}
		tap(&ctx.Router, drawer(), pts[0]) // the preimage record
		record()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(record, "do not cut this preimage", 24); !ok {
			t.Fatalf("the form pick never drew.\nLast frame: %q", c)
		}
		pts = plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[0]) // preimage string
		record()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(record, "Engrave Plate", 32); !ok {
			t.Fatalf("the engrave screen never drew.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button1) // abort the cut
		c, ok = pumpUntil(record, "still in this payload", 32)
		if !ok {
			t.Fatalf("this flow's own abort never drew.\nLast frame: %q", c)
		}
		for _, f := range seen {
			if uiContains(f, "dies with this composition") || uiContains(f, "A PREIMAGE PLATE WAS CUT") {
				t.Fatalf("an §8.4 arm was drawn by a flow that builds no composition.\nFrame: %q", f)
			}
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(record, "can be cut as preimage plates", 32); !ok {
			t.Fatalf("the abort did not return to the list.\nLast frame: %q", c)
		}
	})
}

// TestHashlockPlatesFlowReadsNothingFromAComposition is decision 4: this route
// cuts "without a composition".
//
// It is an AST assertion for the same reason the census's is: at runtime a flow
// that read composerState would look identical until the one payload that made
// it differ.
//
// MUTATION: read a digest out of the composition-state hook -> this fails.
func TestHashlockPlatesFlowReadsNothingFromAComposition(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "composer_hashlock_plates.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing composer_hashlock_plates.go: %v", err)
	}
	// Parsed with no comment mode, so the file's own PROSE about composerState
	// is not the thing under test -- an identifier is.
	banned := map[string]bool{
		"composerState": true, "hashlockHeld": true,
		"clearComposerStateHook": true, "setComposerStateHook": true,
		"composerHoldHashlockMaterial": true, "composerPreimagePlates": true,
	}
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && banned[id.Name] {
			t.Errorf("composer_hashlock_plates.go references %s; decision 4 scopes this "+
				"route to ctx.sysw alone, and §2.3's \"the program holds nothing\" "+
				"depends on it", id.Name)
		}
		return true
	})
}

// readSourceFile reads one file of THIS package, so a structural assertion can
// be made about the source rather than about a behaviour that has no observable
// difference until the payload that makes it matter.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(b)
}

// TestHashlockPlatesListsAPaddedPreimageRecordLikeTheDoorDoes is the H6
// post-impl review's I-1. `sysw.Classify` and `Which hash?`'s band 2 both
// TrimSpace a record before decoding it, so a preimage plate that arrived with
// a stray space or CR (a host file with trailing whitespace survives `me sysw
// pack`) is ClassPreimage at the door and selectable in a composition. The §5.2
// flow's own list must offer it too, or the door names a route the flow then
// refuses -- with the empty-payload screen whose comment says it is unreachable
// from the door (spec §5.2, the F-437 shape).
//
// MUTATION: drop the strings.TrimSpace in hashlockPlatesRecords -> the three
// padded cases list 0 records against a door count of 1, and this fails.
func TestHashlockPlatesListsAPaddedPreimageRecordLikeTheDoorDoes(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"bare", guiPreimagePlate},
		{"trailing space", guiPreimagePlate + " "},
		{"leading space", " " + guiPreimagePlate},
		{"trailing CR", guiPreimagePlate + "\r"},
	} {
		s := composerSessionWith(nil, []string{tc.body})
		_, _, preimages, _ := composerDoorCounts(s)
		if preimages != 1 {
			t.Fatalf("%s: the door counts %d preimage records, want 1 -- the fixture no longer classifies", tc.name, preimages)
		}
		recs := hashlockPlatesRecords(s)
		if len(recs) != preimages {
			t.Errorf("%s: the Hashlock plates list holds %d records but the door counts %d -- the door offers a route the flow refuses", tc.name, len(recs), preimages)
		}
	}
}

// TestHashlockPlatesStubReadsAPaddedMD1LikeTheDoorDoes is the H6 post-impl
// delta review's M-3, one row down the same file as I-1: `sysw.Classify` trims
// an md1 record before classifying it ClassMDMK, so the door counts a padded
// md1 -- and `hashlockPlatesStub` must read the same record the same way, or
// the plate silently loses its `mk1 stub` locator row (§6.3) while the door
// and the census both say the payload holds a policy.
//
// MUTATION: drop the strings.TrimSpace in hashlockPlatesStub -> the three
// padded sessions return an empty stub against the bare session's, and this
// fails.
func TestHashlockPlatesStubReadsAPaddedMD1LikeTheDoorDoes(t *testing.T) {
	x := hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
	st := composerH6PlateState(t, "anchor a")
	chunks, err := composerTemplateChunksFor(st)
	if err != nil {
		t.Fatal(err)
	}
	bare := composerSessionWith(chunks, []string{composerTestPreimageRecord(t, x)})
	wantStub, wantPolicy := hashlockPlatesStub(bare)
	if wantStub == "" {
		t.Fatalf("the bare fixture yields no stub -- the fixture is wrong")
	}
	for _, tc := range []struct{ name, before, after string }{
		{"trailing space", "", " "},
		{"leading space", " ", ""},
		{"trailing CR", "", "\r"},
	} {
		padded := make([]string, len(chunks))
		for i, c := range chunks {
			padded[i] = tc.before + c + tc.after
		}
		s := composerSessionWith(padded, []string{composerTestPreimageRecord(t, x)})
		_, _, _, inert := composerDoorCounts(s)
		if inert != 0 {
			t.Fatalf("%s: the door counts %d inert records -- the padded md1 no longer classifies", tc.name, inert)
		}
		stub, isPolicy := hashlockPlatesStub(s)
		if stub != wantStub || isPolicy != wantPolicy {
			t.Errorf("%s: stub (%q, %v), want (%q, %v) -- the plate would lose its mk1 stub row on a payload the door counts as holding a policy",
				tc.name, stub, isPolicy, wantStub, wantPolicy)
		}
	}
}
