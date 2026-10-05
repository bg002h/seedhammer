//go:build refugium

package gui

// THE REFUGIUM PROFILE'S BEHAVIOUR (Refugium plan F7 §4.1-§4.4), run only under
// -tags refugium. Each test names the default-build test that is its positive
// control: the same drive, with the opposite verdict, in the untagged suite.
//
// The source-level half (no forbidden literal or call in the Refugium file sets,
// every reader request through nfcReader, every passphrase prompt through
// askBIP39Passphrase, every seed-string plate through the ms1 gate) is
// refugium_build_test.go, which runs in both builds.

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/sysw"
)

// countingTagReader counts every Read, so "nothing was read" is a number and not
// the absence of a screen.
type countingTagReader struct {
	rec    []byte
	reads  atomic.Int32
	closes atomic.Int32
}

func (r *countingTagReader) Read(p []byte) (int, error) {
	r.reads.Add(1)
	return copy(p, r.rec), nil
}

func (r *countingTagReader) Close() error {
	r.closes.Add(1)
	return nil
}

func TestRefugiumProfileIsOn(t *testing.T) {
	if !refugiumProfile || proofTriggersEnabled {
		t.Fatalf("-tags refugium built refugiumProfile=%v proofTriggersEnabled=%v; want true, false",
			refugiumProfile, proofTriggersEnabled)
	}
}

// §4.2, the gui layer. startScanner is handed a LIVE reader and reads nothing.
// Control: TestNFCScannerStillDeliversATag (nfc_scan_test.go).
func TestRefugiumStartScannerReadsNothing(t *testing.T) {
	r := &countingTagReader{rec: []byte("command: lock-boot")}
	ctx := NewContext(newPlatform())
	scans, stop := startScanner(ctx, r)
	select {
	case s := <-scans:
		t.Fatalf("the scanner delivered %+v under the profile", s)
	case <-time.After(300 * time.Millisecond):
	}
	stop()
	if n := r.reads.Load(); n != 0 {
		t.Fatalf("the reader was Read %d times under the profile", n)
	}
}

// §4.2. nfcReader and nfcAvailable never consult the platform, even one that
// has a reader to hand out.
func TestRefugiumNeverAsksThePlatformForAReader(t *testing.T) {
	p := newPlatform()
	var asked atomic.Int32
	p.nfc = func() io.ReadCloser {
		asked.Add(1)
		return &countingTagReader{rec: []byte("x")}
	}
	ctx := NewContext(p)
	if !p.Features().Has(FeatureNFC) {
		t.Fatal("INCONCLUSIVE: the test platform reports no reader, so this proves nothing")
	}
	if r := ctx.nfcReader(); r != nil {
		t.Fatalf("nfcReader() = %T under the profile, want untyped nil", r)
	}
	if ctx.nfcAvailable() {
		t.Fatal("nfcAvailable() is true under the profile")
	}
	if ctx.scanOffered() {
		t.Fatal("scanOffered() is true under the profile")
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("Platform.NFCReader was called %d times", n)
	}
}

// §4.1 + §4.2, end to end on the start screen: a lock-boot tag held over a
// machine whose platform HAS a reader is never read, LockBoot is never called,
// and the start screen stays. Control: TestDebugCommandLockBootCharacterization.
func TestRefugiumStartScreenReadsNoTag(t *testing.T) {
	p := newPlatform()
	r := &countingTagReader{rec: []byte("command: lock-boot")}
	var asked atomic.Int32
	p.nfc = func() io.ReadCloser { asked.Add(1); return r }
	p.lockBoot = func() error {
		t.Error("LockBoot called in the Refugium build")
		return nil
	}
	ctx := NewContext(p)
	returned := false
	frame, quit := runUI(ctx, func() {
		_, returned = new(StartScreen).Flow(ctx, &descriptorTheme)
	})
	defer quit()
	var last string
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		time.Sleep(time.Millisecond)
	}
	if returned {
		t.Fatal("StartScreen.Flow returned: something arrived from the reader")
	}
	if !uiContains(last, "Backup Wallet") {
		t.Fatalf("not on the start screen; got %q", last)
	}
	if n := r.reads.Load() + asked.Load(); n != 0 {
		t.Fatalf("reader touched: %d reads, %d NFCReader calls", r.reads.Load(), asked.Load())
	}
}

// §4.1. The Refugium debug arm recognises nothing, whatever the command.
func TestRefugiumDebugCommandsAreInert(t *testing.T) {
	ctx := NewContext(newPlatform())
	for _, cmd := range []string{"lock-boot", "FOREVERLAURA!", "nope"} {
		res, act := handleDebugCommand(ctx, new(StartScreen), debugCommand{Command: cmd})
		if res != debugStay || act != (startScreenAction{}) {
			t.Errorf("command %q: got (%v, %+v), want (debugStay, zero action)", cmd, res, act)
		}
	}
}

// §4.2, scan offer: seed entry. A platform WITH a reader still draws no
// "Where from?" picker. Control: TestSyswSeedPickerOffersScanWithoutAPayload.
func TestRefugiumSeedEntryOffersNoScan(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag(testSeedPhrase)
	ctx := NewContext(p)
	frame, _, quit := runUITouch(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("seed entry drew nothing")
	}
	if !uiContains(content, "Choose number of words") || uiContains(content, "Where from?") {
		t.Fatalf("seed entry's first screen is not the keyboard's word count; got %q", content)
	}
}

// §4.2, scan offer: verify address. The Scan/Type choice is not drawn; the
// keyboard opens. Control: the default build's "Input method" screen.
func TestRefugiumVerifyAddressOffersNoScan(t *testing.T) {
	p := newPlatform()
	p.nfc = nfcTag("bc1qxyz")
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { verifyAddressFlow(ctx, &descriptorTheme, nil) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("verify address drew nothing")
	}
	if uiContains(content, "Input method") || uiContains(content, "Scan") {
		t.Fatalf("verify address offered a scan; got %q", content)
	}
}

// §4.2, scan offer: the composer door. "Scan cards" is gone; "Build a new
// policy" is still there.
func TestRefugiumComposerDoorOffersNoScan(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag("x")
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("the composer door drew nothing")
	}
	if uiContains(content, "Scan cards") {
		t.Fatalf("the composer door offers Scan cards; got %q", content)
	}
	if !uiContains(content, "Build a new policy") {
		t.Fatalf("the composer door lost its typed route; got %q", content)
	}
}

// §4.2, scan offer: the payload/scan decline picker takes the payload without
// drawing a one-row choice.
func TestRefugiumSyswChooseTakesThePayloadWithoutAPicker(t *testing.T) {
	ctx := NewContext(newPlatform())
	var got bool
	frame, quit := runUI(ctx, func() {
		got = syswChoose(ctx, &descriptorTheme, "Seed", "lead", syswAltScan)
	})
	defer quit()
	if c, ok := frame(); ok {
		t.Fatalf("syswChoose drew a frame for the scan alternative: %q", c)
	}
	if !got {
		t.Fatal("syswChoose declined the payload")
	}
}

// §4.2, review M-2: a single-card chunk gather the payload does not complete
// is refused with one screen and returns -- never a gather loop waiting on a
// reader this build does not have. Control: TestMK1GatherFlowBackNoReader
// (the default build's Back-only gather).
func TestRefugiumIncompleteChunkGathersRefuse(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(ctx *Context) bool
		want string
	}{
		{"md1", func(ctx *Context) bool { return md1GatherFlow(ctx, &descriptorTheme, wshSortedmultiChunks[0]) },
			fmt.Sprintf("Captured 1 of %d.", len(wshSortedmultiChunks))},
		{"mk1", func(ctx *Context) bool { _, ok := mk1GatherFlow(ctx, &descriptorTheme, v1c0); return ok },
			"Captured 1 of 2."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			r := &countingTagReader{rec: []byte(wshSortedmultiChunks[1])}
			p.nfc = func() io.ReadCloser { return r }
			ctx := NewContext(p)
			got := true
			frame, quit := runUI(ctx, func() { got = tc.run(ctx) })
			defer quit()
			c, ok := pumpUntil(frame, "pack the full set", 8)
			if !ok {
				t.Fatalf("no refusal; got %q", c)
			}
			if !uiContains(c, tc.want) || uiContains(c, "Scan the next chunk") {
				t.Fatalf("refusal text: %q", c)
			}
			click(&ctx.Router, Button3)
			if c, more := frame(); more {
				t.Fatalf("the gather went on after the refusal: %q", c)
			}
			if got {
				t.Fatal("an incomplete gather reported success")
			}
			if r.reads.Load() != 0 {
				t.Fatal("the reader was read")
			}
		})
	}
}

// §4.2. Both verify flows refuse the readback up front, before the seed is
// retyped, and record nothing observed.
func TestRefugiumVerifyFlowsRefuseTheReadback(t *testing.T) {
	t.Run("single-sig", func(t *testing.T) {
		ctx := NewContext(newPlatform())
		rec := &verifyRecord{}
		var got = true
		frame, quit := runUI(ctx, func() {
			got = singleSigVerifyFlow(ctx, &descriptorTheme, true, false, false, rec)
		})
		defer quit()
		content, ok := pumpUntil(frame, "Verify on another build", 8)
		if !ok {
			t.Fatalf("no refusal drawn; got %q", content)
		}
		click(&ctx.Router, Button3)
		for {
			if _, ok := frame(); !ok {
				break
			}
		}
		if got {
			t.Fatal("singleSigVerifyFlow reported success")
		}
		if *rec != (verifyRecord{}) {
			t.Fatalf("the verify record was written: %+v", *rec)
		}
	})
}

// §4.3. Every passphrase prompt is a notice drawn under the question's title.
// Acknowledging it is Skip, (0, true); Back on it keeps the question's Back,
// (0, false), so each site steps back exactly as before (review M-1).
func TestRefugiumPassphrasePromptIsANotice(t *testing.T) {
	for _, tc := range []struct {
		b      Button
		wantOK bool
	}{{Button3, true}, {Button1, false}} {
		ctx := NewContext(newPlatform())
		cs := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
		sel, ok := -1, !tc.wantOK
		frame, quit := runUI(ctx, func() { sel, ok = askBIP39Passphrase(ctx, &descriptorTheme, cs) })
		content, seen := pumpUntil(frame, "This build takes no BIP-39 passphrase", 8)
		if !seen {
			quit()
			t.Fatalf("no notice; got %q", content)
		}
		if uiContains(content, "Add passphrase") {
			t.Errorf("the notice still offers Add passphrase: %q", content)
		}
		click(&ctx.Router, tc.b)
		for {
			if _, more := frame(); !more {
				break
			}
		}
		quit()
		if sel != 0 || ok != tc.wantOK {
			t.Errorf("dismissed with %v: got (%d, %v), want (0, %v)", tc.b, sel, ok, tc.wantOK)
		}
	}
}

// review M-1: a session that ends ON the notice is not an acknowledgement.
func TestRefugiumPassphraseNoticeEndedByDoneIsNotOK(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.Done = true
	cs := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := askBIP39Passphrase(ctx, &descriptorTheme, cs); sel != 0 || ok {
		t.Fatalf("with ctx.Done: got (%d, %v), want (0, false)", sel, ok)
	}
}

// review M-1, at the site whose !ok means most: seedPassphraseStep. Back on
// the notice is "not this seed" -- the seed is un-registered and the step
// reports a decline -- and acknowledging it keeps the seed, with no
// passphrase bound.
func TestRefugiumSeedPassphraseStepKeepsItsBack(t *testing.T) {
	m, err := bip39.ParseMnemonic(testSeedPhrase)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		b         Button
		want      bool
		wantSeeds int
	}{{Button3, true, 1}, {Button1, false, 0}} {
		reg := &seedRegistry{}
		id, err := reg.add("@0", m, "", &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		ctx := NewContext(newPlatform())
		got := !tc.want
		frame, quit := runUI(ctx, func() { got = seedPassphraseStep(ctx, &descriptorTheme, reg, id, "@0", "Build") })
		if c, ok := pumpUntil(frame, "This build takes no BIP-39 passphrase", 8); !ok {
			quit()
			t.Fatalf("no notice; got %q", c)
		}
		click(&ctx.Router, tc.b)
		for {
			if _, more := frame(); !more {
				break
			}
		}
		quit()
		if got != tc.want || reg.count() != tc.wantSeeds {
			t.Errorf("%v on the notice: step=%v seeds=%d, want %v and %d", tc.b, got, reg.count(), tc.want, tc.wantSeeds)
		}
		if s, ok := reg.at(0); ok && s.Passphrase != "" {
			t.Errorf("a passphrase was bound: %q", s.Passphrase)
		}
	}
}

// §4.3. A missed prompt still cannot take a passphrase.
func TestRefugiumPassphraseEntryReturnsNothing(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		if s, ok := passphraseFlowTitled(ctx, &descriptorTheme, "Passphrase"); s != "" || ok {
			t.Errorf("passphraseFlowTitled = (%q, %v)", s, ok)
		}
		if s, ok := syswPassphraseFlowTitled(ctx, &descriptorTheme, "Passphrase"); s != "" || ok {
			t.Errorf("syswPassphraseFlowTitled = (%q, %v)", s, ok)
		}
	})
	defer quit()
	if c, ok := frame(); ok {
		t.Fatalf("a passphrase keyboard was drawn: %q", c)
	}
}

// §4.3. SLIP-39: answering yes to the passphrase ends the recovery with the
// refusal; it never recovers a seed with the passphrase silently dropped.
// Control: TestRecoverSLIP39Passphrase (slip39_polish_test.go).
func TestRefugiumSLIP39PassphraseStopsTheRecovery(t *testing.T) {
	first := parseFixtureShare(t, slip39Vec3[0])
	ctx := NewContext(newPlatform())
	driveShare(&ctx.Router, slip39Vec3[1])
	click(&ctx.Router, Down, Button3) // "Enter passphrase"
	var m bip39.Mnemonic
	ok := true
	frame, quit := runUI(ctx, func() { m, ok = recoverSLIP39Flow(ctx, &descriptorTheme, first) })
	defer quit()
	content, seen := pumpUntil(frame, "cannot take a SLIP-39 passphrase", 400)
	if !seen {
		t.Fatalf("no SLIP-39 refusal; got %q", content)
	}
	click(&ctx.Router, Button3)
	for {
		if _, more := frame(); !more {
			break
		}
	}
	if ok || m != nil {
		t.Fatalf("recovery continued: ok=%v mnemonic=%v", ok, m != nil)
	}
}

// §4.3. The passphrase program is not in the carousel: a full lap of Right
// never shows it and the pager has one dot fewer.
func TestRefugiumCarouselHidesThePassphraseProgram(t *testing.T) {
	ctx := NewContext(newPlatform())
	s := new(StartScreen)
	frame, quit := runUI(ctx, func() { s.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("no start screen")
	}
	seen := map[program]bool{}
	for i := 0; i <= int(s.lastNav())+1; i++ {
		seen[s.prog] = true
		c, _ := frame()
		if uiContains(c, "BIP-39 Password") {
			t.Fatalf("the passphrase program is drawn: %q", c)
		}
		click(&ctx.Router, Right)
		frame()
	}
	if seen[engravePassphrase] {
		t.Fatal("the carousel stopped on engravePassphrase")
	}
	for p := program(0); p <= s.lastNav(); p++ {
		if p != engravePassphrase && !seen[p] {
			t.Errorf("program %d was never reached: hiding one skipped another", p)
		}
	}
}

// §4.3. A payload holding a pass: record is refused whole, at load, naming the
// record by position. Control: the chain-pass walk (chain_class_walk_test.go).
func TestRefugiumPayloadWithAPassRecordIsRefused(t *testing.T) {
	p := newPlatform()
	p.sysw = chainRegion(t, chainPayloadNamed(t, "chain-pass"))
	ctx := NewContext(p)
	loaded := true
	frame, quit := runUI(ctx, func() {
		loaded = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true)
	})
	defer quit()
	if c, ok := pumpUntil(frame, "Load it?", 32); !ok {
		t.Fatalf("no load offer; got %q", c)
	}
	click(&ctx.Router, Button3) // LOAD
	c, ok := pumpUntil(frame, "Record 1 of this payload is a BIP-39 passphrase", 64)
	if !ok {
		t.Fatalf("no pass: refusal; got %q", c)
	}
	if strings.Contains(c, "636f7272") || uiContains(c, "correct horse") {
		t.Fatalf("the refusal shows the secret: %q", c)
	}
	click(&ctx.Router, Button3)
	for {
		if _, more := frame(); !more {
			break
		}
	}
	if loaded || ctx.sysw != nil {
		t.Fatalf("payload loaded=%v session=%v", loaded, ctx.sysw != nil)
	}
}

// §4.4. The codex32 producers cut text only for an ms1 string, byte for byte
// backup.EngraveSeedStringTextOnly. Control: the default goldens
// (backup/testdata/codex32-*.bin, unchanged).
func TestRefugiumMS1SeedStringPlateHasNoQR(t *testing.T) {
	const vec = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"
	if _, err := codex32.New(vec); err != nil {
		t.Fatalf("INCONCLUSIVE: the BIP-93 vector does not parse: %v", err)
	}
	params := engraverParams
	s := backup.SeedString{Title: "test", Seed: vec, Font: constant.Font}
	got, err := engraveSeedStringPlate(params, s)
	if err != nil {
		t.Fatal(err)
	}
	want, err := backup.EngraveSeedStringTextOnly(params, s)
	if err != nil {
		t.Fatal(err)
	}
	withQR, err := backup.EngraveSeedString(params, s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(planBytes(t, got), planBytes(t, want)) {
		t.Fatal("the ms1 plate is not the text-only plate")
	}
	if bytes.Equal(planBytes(t, got), planBytes(t, withQR)) {
		t.Fatal("INCONCLUSIVE: the text-only and QR plates are identical")
	}
}

func planBytes(t *testing.T, e engrave.Engraving) []byte {
	t.Helper()
	var b bytes.Buffer
	for c := range e {
		fmt.Fprintf(&b, "%v\n", c)
	}
	return b.Bytes()
}

// §4.4. A bundle ms1 card offers TEXT ONLY; an md1 card keeps its QR variants.
func TestRefugiumMS1CardOffersTextOnly(t *testing.T) {
	const ms1 = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"
	names, _, err := validateMdmkStrings(newPlatform(), []string{ms1}, "MS1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "TEXT ONLY" {
		t.Fatalf("an ms1 card offers %v, want [TEXT ONLY]", names)
	}
}

// §4.4. Engrave Text: the QR step offers only "No QR" for an ms1 composition,
// and the plate builder drops a requested QR.
func TestRefugiumFreeTextMS1HasNoQR(t *testing.T) {
	const ms1 = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"
	ctx := NewContext(newPlatform())
	blocks := []backup.Block{{Text: ms1}}
	frame, quit := runUI(ctx, func() { ftQRChoiceFlow(ctx, &descriptorTheme, true, blocks) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no QR step drawn")
	}
	if !uiContains(c, "never engraves one as a QR") || uiContains(c, "Add QR") {
		t.Fatalf("the QR step for an ms1 text: %q", c)
	}
}

// §4.2, the fault screen: drawn first, takes no input, and nothing advances it.
func TestRefugiumNFCFaultScreenTakesNoInput(t *testing.T) {
	p := &faultPlatform{testPlatform: newPlatform(), fault: io.ErrUnexpectedEOF}
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { uiFlow(ctx, "test") })
	defer quit()
	c, ok := frame()
	if !ok || !uiContains(c, nfcFaultLead) {
		t.Fatalf("the first screen is not the NFC fault; got %q", c)
	}
	for _, b := range []Button{Button1, Button2, Button3, Center, Left, Right, Up, Down} {
		click(&ctx.Router, b)
		c, ok = frame()
		if !ok || !uiContains(c, nfcFaultLead) {
			t.Fatalf("after %v the fault screen was left; got %q", b, c)
		}
	}
}

// faultPlatform is a testPlatform that reports an NFC fault.
type faultPlatform struct {
	*testPlatform
	fault error
}

func (p *faultPlatform) NFCFault() error { return p.fault }

// review M-4: the refusal keys on the `pass:` PREFIX, so a record whose body
// does not decode -- which sysw.Classify calls ClassUnknown -- is refused too.
// It is still a passphrase record, and still secret.
func TestRefugiumAnyPassPrefixedRecordIsFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    sysw.Payload
		want int
	}{
		{"well-formed, public", sysw.Payload{Public: []string{"text:41", "pass:636f7272"}}, 2},
		{"undecodable body", sysw.Payload{Public: []string{"pass:zz-not-hex"}}, 1},
		{"empty body", sysw.Payload{Secret: []string{"pass:"}}, 1},
		{"secret section, after one public", sysw.Payload{Public: []string{"text:41"}, Secret: []string{"pass:nothex"}}, 2},
	} {
		n, ok := syswFirstPassphraseRecord(&tc.p)
		if !ok || n != tc.want {
			t.Errorf("%s: got (%d, %v), want (%d, true)", tc.name, n, ok, tc.want)
		}
	}
	if _, ok := syswFirstPassphraseRecord(&sysw.Payload{Public: []string{"text:70617373"}}); ok {
		t.Error("a text record was taken for a passphrase")
	}
}

// refugiumMS1Vec is BIP-93's test vector: a valid ms1 string, public by
// construction.
const refugiumMS1Vec = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"

// §4.4, review I-1 and M-6: the free-text sink. ftBuildPlate drops a QR the
// caller asked for when the text CONTAINS an ms1 string anywhere -- a label,
// a bracket, a list number, a quote or an NBSP in front changes nothing, the
// QR would still be the secret in one photo. A text that only mentions ms1
// keeps its QR (positive control), so the gate is not "never a QR".
func TestRefugiumFreeTextSinkDropsTheQRForAnEmbeddedMS1(t *testing.T) {
	build := func(t *testing.T, text string) *backup.Fitted {
		t.Helper()
		var got backup.Fitted
		seen := false
		freetextPlateHook = func(f backup.Fitted) { got, seen = f, true }
		defer func() { freetextPlateHook = nil }()
		if _, err := ftBuildPlate(ftParamsAtSpeed(engraverParams, 0), &ftPlanSH, text, "", "", true, 0, 0); err != nil {
			t.Fatalf("ftBuildPlate(%q): %v", text, err)
		}
		if !seen {
			t.Fatal("the plate hook never ran")
		}
		return &got
	}
	for _, text := range []string{
		refugiumMS1Vec,
		"Share A: " + refugiumMS1Vec,
		"(" + refugiumMS1Vec + ")",
		"1. " + refugiumMS1Vec,
		"\"" + refugiumMS1Vec,
		" " + refugiumMS1Vec,
		strings.ToUpper(refugiumMS1Vec),
		"ms10-tests-xxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw",
		"ms10test sxxxx xxxxx xxxxx xxxxx xxxxx xxxx4 nzvca 9cmcz lw",
	} {
		if f := build(t, text); f.QR != nil {
			t.Errorf("%q: the plate carries a QR", text)
		}
	}
	for _, text := range []string{"HELLO WORLD", "see the ms1 card", "ms1 plates are text only"} {
		if f := build(t, text); f.QR == nil {
			t.Errorf("%q: the QR was dropped from a text holding no ms1 string", text)
		}
	}
	// An NBSP (or any other Unicode space) is a separator too. The plate's
	// font has no glyph for it, so this case is the predicate alone: a
	// payload text record can carry one even though no keyboard types it.
	for _, text := range []string{"\u00a0" + refugiumMS1Vec, "Share\u00a0A:\u2009ms10tests\u00a0xxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"} {
		if !noMS1QRText(text) {
			t.Errorf("%q: the free-text gate let it through", text)
		}
	}
}

// §4.4, review M-6: Engrave Text, QR chosen BEFORE an ms1 text is typed. The
// flow drops the QR after the text step and says so.
func TestRefugiumFreeTextForcedOffQRIsSaid(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, true)
	ftSetText(h, "Share A: "+refugiumMS1Vec)
	ftOK(h)
	h.mustReach("ms1secretstring")
	if !uiContains(h.content, "carries no QR") {
		t.Fatalf("the notice does not say the QR is gone: %q", h.content)
	}
	h.tapNav(Button3)
	h.mustReach("Title")
}

// §4.4, review M-6: the sealed-unlock codex32 plate. unlockEngraveCodex32
// cuts unlockCodex32Plan, and under the profile that is byte for byte the
// text-only plate for the SeedString it builds.
func TestRefugiumUnlockCodex32PlateIsTextOnly(t *testing.T) {
	s, err := codex32.New(refugiumMS1Vec)
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := s.Split()
	got, err := unlockCodex32Plan(engraverParams, s)
	if err != nil {
		t.Fatal(err)
	}
	want, err := backup.EngraveSeedStringTextOnly(engraverParams,
		backup.SeedString{Title: id, Seed: s.String(), Font: constant.Font})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(planBytes(t, got), planBytes(t, want)) {
		t.Fatal("the unlock codex32 plate is not the text-only plate")
	}
}
