package gui

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bspline"
	"seedhammer.com/internal/golden"
	"seedhammer.com/sysw"
)

// ═══ THE PAYLOAD CHAIN, END TO END ═══════════════════════════════════════════
//
// Four links, and until this file only two of them were joined:
//
//	(1) CLI creation   `me sysw pack` writes a container
//	(2) device ingest  the firmware reads that container out of flash
//	(3) the walk       load -> review -> plate kind -> plan -> cut -> post-cut
//	(4) the plate      a rendered artifact of the toolpath
//
// gui/transaction_walk_test.go covers (2)+(3) beautifully and starts from a Go
// literal: `newTxWalk(t, "tx:"+rawHexOfEven(t))` builds the session in Go and
// never touches a container. So the walks prove the FLOW and say nothing about
// whether what the producer emits is what the device accepts -- a `me` that
// wrote a byte-different record, or a record class the device classifies
// differently, would leave all sixteen of them green.
//
// These walks start from BYTES `me` WROTE. gui/testdata/chain/chain_payloads.json
// holds containers produced by running the real CLI (scripts/gen-chain-fixtures.sh),
// and the region they are padded into is read through sysw.FileReader --
// the host stand-in for the device's XIP reader -- and opened by syswLoadFlow,
// the same function boot calls. Nothing about the ingest is faked except the
// address the bytes come from.
//
// WHAT THIS CHAIN DOES NOT PROVE, said plainly, because a chain that overstates
// itself is worse than a shorter honest one:
//
//   - NO HARDWARE. testEngraver accepts the stepper stream and digests it.
//     Nothing here says a motor turned or that steel was cut.
//   - THE SCREENS ARE TEXT, NOT PIXELS. op.Drawer.ExtractText walks the
//     firmware's op tree, so these frames show WHAT the device says and not how
//     it LOOKS. A legend that renders off the panel passes here.
//   - THE PLATE ARTIFACT IS A RENDER OF THE PLAN, not a capture from inside the
//     engrave loop. It is bound to the walk by the plate TITLE and the plate
//     count the walk's own screens asserted, and by nothing stronger -- see
//     chainGoldenPlate's comment.
//   - THE FIXTURE IS PINNED, NOT LIVE. The committed bytes are `me` output, but
//     this test does not run `me`; chain_fixture_live_test.go (build tag
//     `oraclelive`) does, and is what catches drift.
//   - ONE RECORD KIND. Tx and Mt only. The other six packable classes reach no
//     engrave walk from a payload at all; see design/agent-reports for the
//     matrix.

// chainPayload is one entry of gui/testdata/chain/chain_payloads.json.
type chainPayload struct {
	Name    string   `json:"name"`
	Note    string   `json:"note"`
	Command []string `json:"command"`
	Records []string `json:"records"`
	Blob    string   `json:"blob"`
	Bytes   int      `json:"bytes"`
	SHA256  string   `json:"sha256"`
	// Digest is what `me sysw show` printed for these bytes. The device
	// recomputes it from the container; the walk asserts the Payload Digest
	// screen shows THIS string. That equality is the whole host/device binding
	// -- an operator standing at the machine compares exactly these two.
	Digest string `json:"digest"`
}

func chainPayloadNamed(t *testing.T, name string) chainPayload {
	t.Helper()
	path := filepath.Join("testdata", "chain", "chain_payloads.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		// FATAL, never a skip. The file is in the repo; its absence means the
		// checkout is broken, and a test that answers "I could not tell" by
		// reporting success is the default failure mode in this tree.
		t.Fatalf("INCONCLUSIVE: %s is unreadable: %v\n"+
			"Regenerate it with ./scripts/gen-chain-fixtures.sh", path, err)
	}
	var doc struct {
		MeVersion string         `json:"me_version"`
		Payloads  []chainPayload `json:"payloads"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, p := range doc.Payloads {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no payload named %q in %s", name, path)
	return chainPayload{}
}

// chainRegion pads a CLI-built container out to a full region and hands back a
// reader over it -- byte-for-byte what `me sysw pack --region` writes, which is
// what gets flashed. The 0xFF tail is the ERASED state of NOR flash, so this is
// the sector with only the container written.
//
// Padded here rather than committed at 64 KiB: the pad is mechanical and
// asserting it in Go is cheaper than storing 64 KiB of 0xFF three times.
// scripts/gen-chain-fixtures.sh's own `--region` output was checked against
// this construction when the fixtures were made.
func chainRegion(t *testing.T, p chainPayload) sysw.Reader {
	t.Helper()
	b, err := hex.DecodeString(p.Blob)
	if err != nil {
		t.Fatalf("payload %s: blob is not hex: %v", p.Name, err)
	}
	if len(b) != p.Bytes {
		t.Fatalf("payload %s: blob is %d bytes, the fixture says %d", p.Name, len(b), p.Bytes)
	}
	region := make([]byte, sysw.RegionLen)
	for i := range region {
		region[i] = 0xFF
	}
	copy(region, b)
	f := filepath.Join(t.TempDir(), "region.bin")
	if err := os.WriteFile(f, region, 0o600); err != nil {
		t.Fatal(err)
	}
	return sysw.FileReader{Path: f}
}

// chainWalk is txWalk with the first two links attached: the session is not
// built in Go, it is READ from a region holding a container `me` wrote, through
// syswLoadFlow's real screens.
type chainWalk struct {
	t     *testing.T
	pay   chainPayload
	ctx   *Context
	eng   *testEngraver
	pl    *engravedAwarePlatform
	frame func() (string, bool)
	quit  func()
	last  string
}

func newChainWalk(t *testing.T, name string) *chainWalk {
	t.Helper()
	pay := chainPayloadNamed(t, name)
	e := newEngraver()
	p := newEngravedAwarePlatform()
	p.engraver = e
	p.display = sh2DisplaySize
	p.sysw = chainRegion(t, pay)
	ctx := NewContext(p)
	w := &chainWalk{t: t, pay: pay, ctx: ctx, eng: e, pl: p}
	w.frame, w.quit = runUI(ctx, func() {
		// atBoot=true: the offer screen, the digest comparison and the warning
		// summary, exactly as gui.go:2031 calls it at start-up. ctx.sysw is
		// assigned by this call and by nothing in the test.
		if !syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true) {
			return
		}
		engraveTransactionFlow(ctx, &descriptorTheme)
	})
	return w
}

func (w *chainWalk) until(want string) string {
	w.t.Helper()
	got, ok := pumpUntil(w.frame, want, 64)
	w.last = got
	if !ok {
		w.t.Fatalf("the chain never reached a screen saying %q.\nLast frame: %q", want, got)
	}
	return got
}

func (w *chainWalk) confirm() { click(&w.ctx.Router, Button3) }

func (w *chainWalk) paged(want string) string {
	w.t.Helper()
	for page := 0; page < 6; page++ {
		got, ok := pumpUntil(w.frame, want, 24)
		w.last = got
		if ok {
			return got
		}
		click(&w.ctx.Router, Button2)
	}
	w.t.Fatalf("%q is on no page the operator can reach.\nLast frame: %q", want, w.last)
	return ""
}

// ingest drives links (2) and (3)'s opening: LOAD the payload, then compare the
// digest. It asserts the digest screen carries the number `me sysw show`
// printed for these exact bytes.
//
// THE COMPARISON IS ON THE DIGITS, NOT THE GROUPING, and that is a limit of the
// harness rather than a choice. op.Drawer.ExtractText concatenates text ops with
// no separators, so the screen's `e7e5 152f …` arrives as `e7e5152f…`: the
// spaces are inter-op gaps, not characters, and no assertion made through this
// extractor can see them. uiContains strips spaces from the wanted string,
// which is exactly the right comparison here and exactly the wrong one to read
// as "the screen is grouped correctly".
func (w *chainWalk) ingest() {
	w.t.Helper()
	w.until("A systemwide payload is present")
	w.confirm() // LOAD is choice 0

	got := w.until("Payload Digest")
	if !uiContains(got, w.pay.Digest) {
		w.t.Fatalf("the device's digest screen does not show what `me sysw show` printed.\n"+
			"  me sysw show: %q\n  device screen: %q\n"+
			"These are the two numbers an operator compares by hand; if they differ, "+
			"the ceremony the screen asks for cannot be completed.", w.pay.Digest, got)
	}
	// The digits must be a full 32 nibbles: a truncated render would still
	// contain a prefix of the pinned value and pass a `Contains` on it.
	if n := len(strings.ReplaceAll(w.pay.Digest, " ", "")); n != 32 {
		w.t.Fatalf("the fixture's digest is %d hex digits, not 32: %q", n, w.pay.Digest)
	}
	if !uiContains(got, "me sysw show") {
		w.t.Errorf("the screen must name the command that prints the other number: %q", got)
	}
	w.confirm() // the comparison IS route 2 of [compared]
}

// ─── THE CHAIN: a me-packed payload, loaded, walked, cut, rendered ──────────

// TestChainFromAMePackedPayloadToACutQRPlate is the complete chain for the Tx
// record class, the kind with the most existing support.
//
// The payload holds the pinned "even" transaction delivered BOTH ways -- the
// six-chunk mt1 set and a tx: record of the same 222 bytes -- because that is
// what an operator who ran `mt encode` and `mt encode --qr` actually packs.
func TestChainFromAMePackedPayloadToACutQRPlate(t *testing.T) {
	var words int
	var digest uint64
	var art string
	synctest.Test(t, func(t *testing.T) {
		w := newChainWalk(t, "chain-tx")
		defer w.quit()
		w.eng.keepWordsIn(t)

		// (1)+(2) the container `me` wrote, read out of the region.
		w.ingest()

		// (3) THE WALK. One candidate: the tx: record merged into the confirmed
		// set on the shared txid, so no picker -- and the merged candidate
		// carries BOTH strings and bytes, so both plate kinds are offered.
		got := w.until("Engrave this transaction?")
		if !uiContains(got, txEvenTxid[:16]) {
			t.Errorf("the review screen must carry the txid: %q", got)
		}
		if !uiContains(got, "BEARER") {
			t.Errorf("...and the bearer warning, on the page it is confirmed from: %q", got)
		}
		w.confirm()

		got = w.until("Choose plate kind")
		if !uiContains(got, "QR PLATES") || !uiContains(got, "TEXT PLATES") {
			t.Fatalf("the merged candidate has strings AND bytes, so both kinds "+
				"must be offered: %q", got)
		}
		// TEXT PLATES is choice 0 and QR PLATES is choice 1, so the QR walk has
		// to move the selection. Down, not Button1 -- Button1 is Back on a
		// ChoiceScreen (gui.go:1869) and taking it here ends the flow silently.
		click(&w.ctx.Router, Down)
		w.confirm()

		got = w.until("Engrave?")
		for _, want := range []string{"plate(s)", "QR", "ECC", "modules", "cutting"} {
			if !uiContains(got, want) {
				t.Errorf("the plan-confirm screen never says %q: %q", want, got)
			}
		}
		w.confirm()

		got = w.until("Engrave this plate")
		if !uiContains(got, "TX 2DCF2B97 1/1") {
			t.Errorf("the plate title must name the transaction and its place: %q", got)
		}
		chainEngraveOnePlate(t, w)

		synctest.Wait()
		click(&w.ctx.Router, Button3) // accept the finished plate
		synctest.Wait()
		w.until("TEST THEM NOW")
		w.paged("mt inspect")
		w.paged("no camera")

		words, digest = w.eng.engraved()
		if words == 0 {
			t.Fatal("the chain completed having written ZERO stepper words: " +
				"nothing was cut, and every screen assertion above would still pass")
		}
		if spill := w.eng.engravedWordsFile(); spill != "" {
			if fi, err := os.Stat(spill); err != nil || fi.Size() != int64(4*words) {
				t.Errorf("the raw cut is not recoverable from the spill: %v", err)
			}
		}

		// (4) THE PLATE. Rendered from the plan the flow builds for the very
		// candidate this walk engraved, read back out of the session the load
		// flow filled -- and compared against the committed golden.
		art = chainGoldenPlate(t, w, true, "tx-qr", "TX 2DCF2B97 1/1")
	})
	t.Logf("chain-tx cut %d stepper words, digest %#x; plate artifact %s",
		words, digest, art)
}

// chainEngraveOnePlate is txWalk.engraveOnePlate, on a chainWalk. Kept as its
// own function rather than made generic: the two harnesses differ in what they
// build, not in how a plate is driven, and a shared interface here would be
// indirection for one call site.
func chainEngraveOnePlate(t *testing.T, w *chainWalk) {
	t.Helper()
	w.confirm() // "ENGRAVE" on the per-plate choice screen
	click(&w.ctx.Router, Button3, Button3, Button3)
	press(&w.ctx.Router, Button3)
	w.frame()
	time.Sleep(confirmDelay)
	for {
		w.frame()
		select {
		case <-w.eng.closes:
			return
		case <-w.pl.wakeups:
		}
	}
}

// chainGoldenPlate is link (4): it produces the plate artifact AND compares it
// against the committed golden that TestTransactionPlateGoldens pins.
//
// THAT COMPARISON IS THE POINT, and it is worth more than a bare render. The
// goldens were recorded from a session built in Go -- `sessionWith(txEven...)`,
// a literal -- so matching one proves the plate reached from `me sysw pack`
// bytes is knot-for-knot the plate the Go-literal path produces. A producer
// that emitted a subtly different record, or a device that classified it into a
// different shape, would move the spline and fail here. Nothing in the sixteen
// existing walks could see that.
//
// IT IS STILL A RENDER OF THE PLAN, NOT A CAPTURE OF THE CUT. The Plate the
// walk engraved is a local inside transactionReviewAndEngrave and never escapes
// it. This recomputes the plan from the candidate the SAME session yields --
// deterministic in the candidate and the engraver params -- and binds it to the
// walk by the two observables the walk already asserted: the plate TITLE and
// the plate count.
//
// cmd/plateview cannot do this: its eight named plates are the proof patterns,
// freetext, seed and passphrase, so a transaction plate has no plateview name
// (gui/preview.go's previewBuilders map). golden.CompareBSpline is the same
// renderer underneath -- Vectorize at the production stroke -- reached without
// inventing a ninth builder.
//
// CompareBSpline writes <name>.bin.svg into dumpDir unconditionally, so the
// artifact exists whether or not the comparison passes. dumpDir is t.TempDir(),
// and CHAIN_PLATE_OUT copies it out -- the shape TX_JOURNEY_OUT uses, for the
// same reason: a capture nobody can look at is not evidence, and a file left in
// the worktree is litter.
func chainGoldenPlate(t *testing.T, w *chainWalk, qr bool, goldenName, wantTitle string) string {
	t.Helper()
	cands, _ := payloadTransactions(w.ctx)
	if len(cands) != 1 {
		t.Fatalf("the session yields %d candidates; the walk went through one", len(cands))
	}
	var plates []Plate
	var titles []string
	var note string
	var err error
	if qr {
		plates, titles, note, err = planTransactionQRPlates(w.ctx.Platform, cands[0])
	} else {
		plates, titles, err = planTransactionTextPlates(w.ctx.Platform, cands[0])
	}
	if err != nil {
		t.Fatalf("re-planning the plates: %v", err)
	}
	if len(plates) != 1 {
		t.Fatalf("re-planned %d plates; the walk cut one (%s)", len(plates), note)
	}
	if titles[0] != wantTitle {
		t.Fatalf("the re-planned plate is titled %q, not %q -- so this is not the "+
			"plate the walk's own plate screen named", titles[0], wantTitle)
	}
	// A golden over an EMPTY engraving passes forever. Measure unions only
	// ENGRAVED segments, so this asks about ink and not about travel moves.
	if bspline.Measure(plates[0].Spline).Bounds.Empty() {
		t.Fatal("the plate cuts nothing")
	}
	P := w.ctx.Platform.EngraverParams()
	bounds := bspline.Bounds{Max: SquarePlate.Dims(P.Millimeter)}
	dir := t.TempDir()
	// update is ALWAYS false here, whatever -update was passed. This test
	// asserts an EQUALITY between two routes to one plate; letting it re-record
	// would let the CLI route silently redefine the golden the Go route is
	// measured against, which is the one thing it exists to prevent.
	if err := golden.CompareBSpline(filepath.Join("testdata", goldenName+".bin"),
		false, dir, P.StrokeWidth, bounds, plates[0].Spline); err != nil {
		t.Fatalf("the plate reached from a `me sysw pack` payload is NOT the plate "+
			"%s pins: %v\n\n"+
			"That golden was recorded from a Go-built session. A difference means "+
			"the CLI route and the Go-literal route produce different steel -- diff\n"+
			"  %s   what the CLI route draws\n"+
			"  %s   what the golden holds\n"+
			"Do NOT re-record from here; fix the divergence or re-record through "+
			"TestTransactionPlateGoldens.",
			goldenName, err,
			filepath.Join(dir, goldenName+".bin.svg"),
			filepath.Join(dir, goldenName+".bin.orig.svg"))
	}
	art := filepath.Join(dir, goldenName+".bin.svg")
	fi, err := os.Stat(art)
	if err != nil {
		t.Fatalf("no plate artifact was written: %v", err)
	}
	t.Logf("plate artifact: %s (%d bytes) — matches golden testdata/%s.bin; %s",
		art, fi.Size(), goldenName, note)
	if out := os.Getenv("CHAIN_PLATE_OUT"); out != "" {
		b, err := os.ReadFile(art)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatalf("CHAIN_PLATE_OUT=%s: %v", out, err)
		}
		t.Logf("also wrote %s", out)
	}
	return art
}

// ─── the TEXT half of the same payload ──────────────────────────────────────

// The same CLI-built container, cut the other way. It is a separate test rather
// than a second loop in the one above because the walk is the subject: a flow
// that could only reach the plate kind it was steered to first would pass a
// combined test and fail this one.
func TestChainFromAMePackedPayloadToACutTextPlate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newChainWalk(t, "chain-tx")
		defer w.quit()
		w.ingest()

		w.until("Engrave this transaction?")
		w.confirm()
		w.until("Choose plate kind")
		w.confirm() // TEXT PLATES is choice 0

		got := w.until("Engrave?")
		if !uiContains(got, "6 string(s)") {
			t.Errorf("the plan must state how many strings are being cut -- the "+
				"payload holds six mt1 chunks: %q", got)
		}
		w.confirm()

		got = w.until("Engrave this plate")
		if !uiContains(got, "TX 2DCF2B97") {
			t.Errorf("plate title: %q", got)
		}
		chainEngraveOnePlate(t, w)
		synctest.Wait()
		click(&w.ctx.Router, Button3)
		synctest.Wait()

		w.until("TEST THEM NOW")
		got = w.paged("mt verify")
		if uiContains(got, "mt inspect") {
			t.Errorf("a text plate is verified string by string, not as raw bytes: %q", got)
		}
		if words, _ := w.eng.engraved(); words == 0 {
			t.Fatal("the text chain cut nothing")
		}
		chainGoldenPlate(t, w, false, "tx-text", "TX 2DCF2B97 1/1")
	})
}

// ─── the QR-only payload ────────────────────────────────────────────────────

// A payload holding ONLY a tx: record. It proves the withholding half of the
// plate-kind rule from CLI-built bytes: with no mt1 strings there is nothing to
// engrave as text, and TEXT PLATES must not be offered.
func TestChainATxOnlyPayloadOffersNoTextPlates(t *testing.T) {
	w := newChainWalk(t, "chain-txonly")
	defer w.quit()
	w.ingest()

	w.until("Engrave this transaction?")
	w.confirm()
	got := w.until("Choose plate kind")
	if !uiContains(got, "QR PLATES") {
		t.Fatalf("QR was not offered for a tx: record: %q", got)
	}
	if uiContains(got, "TEXT PLATES") {
		t.Errorf("a tx: record carries no mt1 strings: %q", got)
	}
}

// ─── G-P3.10, AS IT MANIFESTS IN THE CHAIN ──────────────────────────────────

// G-P3.10 (gui/transaction.go:467) merges transaction candidates on the DERIVED
// TXID, never on the bytes -- so a tx: record byte-different from an existing
// candidate but sharing its txid is SILENTLY DROPPED. Not merged, not flagged,
// not a second picker row.
//
// TestTheMergeIsKeyedOnTheTxidNotOnTheBytes already measures that at the
// payloadTransactions level from a Go-built session. This measures it ALONG THE
// WHOLE CHAIN, which is where the cost is visible: `me sysw pack` REFUSED this
// record until the operator passed --allow-unsigned-inputs, printed a four-line
// warning naming the txid and the risk, and wrote SEVEN records; `me sysw show`
// lists all seven and describes the seventh at length. The operator therefore
// has every reason to believe the device holds it. It does -- the record is
// there, it classifies ClassTx, the session carries it -- and the walk offers
// ONE transaction with no mention that a second record was discarded.
//
// THE BEHAVIOUR IS NOT CHANGED HERE. The operator has ruled "engrave both"; the
// fix is separate work. This is the record of how the defect presents to
// someone standing at the machine.
func TestChainGP310SilentlyDropsAByteDifferentTwin(t *testing.T) {
	w := newChainWalk(t, "chain-gp310")
	defer w.quit()

	// The payload the CLI built really does hold seven records, two of which
	// are transaction-bearing -- so the loss below is the device's, not the
	// fixture's.
	if len(w.pay.Records) != 7 {
		t.Fatalf("fixture holds %d records, want 7", len(w.pay.Records))
	}
	w.ingest()

	// PUMP TO THE REVIEW SCREEN FIRST. ctx.sysw is assigned inside syswLoadFlow,
	// which runs on the UI side of iter.Pull -- a click only queues an event, so
	// the session does not exist until frames have carried the flow past the
	// load. Reading it straight after ingest() nil-panicked.
	got := w.until("Engrave this transaction?")

	var mt, tx int
	for _, r := range w.ctx.sysw.records {
		switch r.class {
		case sysw.ClassMt:
			mt++
		case sysw.ClassTx:
			tx++
		}
	}
	if mt != 6 || tx != 1 {
		t.Fatalf("the loaded session holds %d mt1 chunks and %d tx: records, want 6 and 1: "+
			"the drop measured below must be the MERGE, not a classification failure", mt, tx)
	}

	cands, _ := payloadTransactions(w.ctx)
	if len(cands) != 1 {
		t.Fatalf("MEASURED BEHAVIOUR CHANGED: %d candidates. If the merge is now on "+
			"bytes or content, G-P3.10 is closed and this test should be rewritten "+
			"to assert the new answer.", len(cands))
	}
	if len(cands[0].tx.Raw) != 222 {
		t.Errorf("the surviving candidate is %d bytes, want the 222-byte signed form",
			len(cands[0].tx.Raw))
	}

	// And the operator is never told. The picker does not appear (one
	// candidate), and the review screen names one transaction.
	if uiContains(got, "Which transaction?") {
		t.Error("a picker appeared; the drop would at least be visible")
	}
	for _, unsaid := range []string{"113", "discard", "dropped", "ignored", "second record"} {
		if uiContains(got, unsaid) {
			t.Errorf("the screen mentions %q -- the drop may no longer be silent, "+
				"which would be an improvement this test should be updated for: %q",
				unsaid, got)
		}
	}
	t.Log("G-P3.10 confirmed along the chain: `me` packed 7 records including a " +
		"113-byte signature-stripped twin it warned about by name; the device loaded " +
		"all 7, classified them 6 Mt + 1 Tx, and offered ONE transaction with no " +
		"mention that the twin was dropped.")
}
