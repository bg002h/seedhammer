package gui

// Bug-hunt triage, sh2, slice S2 (gui: md1 gather, composer, transaction,
// display). One test per bug in the hunt report
// (design/agent-reports/bughunt-2026-10-31-sh2.md): bugs 5, 21, 22, 23, 24, 45
// and 51. Each asserts the CORRECT behaviour, so it FAILS on unmodified code
// when the bug is real.

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
)

// bughuntSh2MD1Chunk crafts a codex32 md1 string whose 37-bit chunk header
// (md/chunk.go readChunkHeader: version 4 bits, chunked 1, csid 20, count-1 6,
// index 6) declares the given count and index. The body is zero padding to 100
// bits; the checksum is the real md1 BCH one (codex32.AssembleMD1).
func bughuntSh2MD1Chunk(count, index int) string {
	type field struct {
		v uint64
		n int
	}
	fields := []field{{4, 4}, {1, 1}, {0x12345, 20}, {uint64(count - 1), 6}, {uint64(index), 6}}
	var bits []byte
	for _, f := range fields {
		for i := f.n - 1; i >= 0; i-- {
			bits = append(bits, byte(f.v>>uint(i))&1)
		}
	}
	for len(bits) < 100 {
		bits = append(bits, 0)
	}
	var syms []byte
	for i := 0; i < len(bits); i += 5 {
		var s byte
		for j := 0; j < 5; j++ {
			s = s<<1 | bits[i+j]
		}
		syms = append(syms, s)
	}
	return codex32.AssembleMD1(syms)
}

// Bug 5: gui/md1_gather.go offer accepts a chunk whose index is >= the declared
// total, so complete() can be true with chunk 1 never gathered.
//
// Open product question: which status a refused chunk reports (gatherForeign
// "Different descriptor", or gatherIgnored "Not an md1 descriptor chunk"). This
// test pins only that it is NOT added.
func TestBughuntSh205Md1GatherIndexPastTotal(t *testing.T) {
	t.Skip("deferred-minor: sh-md1-gather-index-past-total")
	c0 := bughuntSh2MD1Chunk(2, 0)
	bad := bughuntSh2MD1Chunk(2, 5)
	c1 := bughuntSh2MD1Chunk(2, 1)
	// Premise: the crafted strings are well-formed md1 chunks that the header
	// parser reads as the report says. (A header-level fix may refuse `bad`
	// here; that is also correct, so an error is allowed.)
	h0, err := md.ParseChunkHeader(c0)
	if err != nil || h0.TotalChunks != 2 || h0.ChunkIndex != 0 {
		t.Fatalf("INCONCLUSIVE: c0 header %+v err %v", h0, err)
	}
	if hb, err := md.ParseChunkHeader(bad); err == nil && (hb.TotalChunks != 2 || hb.ChunkIndex != 5) {
		t.Fatalf("INCONCLUSIVE: bad header %+v", hb)
	}

	g := &md1Gatherer{}
	if st := g.offer(c0); st != gatherAdded {
		t.Fatalf("INCONCLUSIVE: chunk 0 not added: %v", st)
	}
	st := g.offer(bad)
	if st == gatherAdded {
		t.Errorf("offer accepted chunk index 5 of a 2-chunk set (status gatherAdded)")
	}
	if g.complete() {
		t.Errorf("complete() is true with chunk 1 never gathered; collected() = %q", g.collected())
	}
	// The operator can still finish the set with the real chunk 1.
	if st := g.offer(c1); st != gatherAdded {
		t.Errorf("the real chunk 1 was refused after the bad one: status %v", st)
	}
	if !g.complete() {
		t.Errorf("the set {0, 1} is not complete after the real chunk 1")
	}
	for i, s := range g.collected() {
		if s == "" {
			t.Errorf("collected()[%d] is the empty string: a gap handed to the reassembler", i)
		}
	}
}

// bughuntSh2UnseatedFixture is a 2-of-3 wsh with slots 0 and 1 seated at
// accounts 0 and 1 and slot 2 UNSEATED (src -1). declared2 is what the chunk set
// declares for slot 2 (nil = the codec's §4f default, which is the honest build).
func bughuntSh2UnseatedFixture(t *testing.T, declared2 *md.SlotOrigin) (*composerState, []string) {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	st := &composerState{list: list, reg: &seedRegistry{}}
	st.sources = []composerSource{
		{kind: composerSourceKey, seedID: -1}, {kind: composerSourceKey, seedID: -1},
	}
	declared := make([]*md.SlotOrigin, 3)
	st.assigned = make([]composerAssignment, 3)
	for i := 0; i < 2; i++ {
		fp := [4]byte{0x73, 0xc5, 0xda, byte(i)}
		origin := composerTestOrigin(2, uint32(i))
		st.assigned[i] = composerAssignment{
			src: i, account: uint32(i), origin: origin,
			fingerprint: fp, fpPresent: true,
		}
		declared[i] = &md.SlotOrigin{Origin: origin, Fingerprint: fp, FpPresent: true}
	}
	st.assigned[2] = composerAssignment{src: -1}
	declared[2] = declared2
	c, err := md.ComposeWith(list, declared)
	if err != nil {
		t.Fatalf("md.ComposeWith: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	return st, chunks
}

// Bug 21: gui/composer_selfcheck.go checks only the FINGERPRINT of an unseated
// slot, though its comment says "Both are checked". An unseated slot must
// declare §4f's default origin (the wrapper's m/48'/0'/<account>'/<script>' at
// the lowest free account); a chunk set that declares anything else on it must
// be refused by the self-check.
//
// Open product question: the report states no wording for the refusal; this
// test asserts only that the self-check refuses.
func TestBughuntSh221SelfCheckUnseatedSlotOrigin(t *testing.T) {
	// Premise: the honest build (slot 2 left to the codec) passes, so a
	// refusal below can only come from the origin on slot 2.
	st, chunks := bughuntSh2UnseatedFixture(t, nil)
	if err := composerSelfCheck(st, chunks); err != nil {
		t.Fatalf("INCONCLUSIVE: the self-check refuses the honest unseated build: %v", err)
	}
	for _, tc := range []struct {
		name   string
		origin []md.PathComponent
	}{
		{"a different derivation altogether (m/84'/0'/0')", []md.PathComponent{
			{Hardened: true, Value: 84}, {Hardened: true, Value: 0}, {Hardened: true, Value: 0}}},
		{"§4f's shape at a account that is not the lowest free one (m/48'/0'/7'/2')",
			md.DefaultOrigin(md.ComposeWsh, 7)},
	} {
		st, chunks := bughuntSh2UnseatedFixture(t, &md.SlotOrigin{Origin: tc.origin})
		// The fixture really declares that origin on the unseated slot.
		_, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil || len(keys) != 3 || composerOriginKey(originComponents(keys[2].OriginPath)) != composerOriginKey(tc.origin) {
			t.Fatalf("INCONCLUSIVE (%s): slot 2 does not carry the origin: %v", tc.name, err)
		}
		if err := composerSelfCheck(st, chunks); err == nil {
			t.Errorf("the self-check ACCEPTED an unseated slot that declares %s; "+
				"the comment says its origin is checked", tc.name)
		}
	}
}

// Bug 22: composerPickScreenFrom, after the pager wraps to page 0, clamps the
// cursor with the OLD page's `shown`. When page 0 shows fewer rows than the
// last page did, the cursor stays on a row page 0 does not draw, and Button3
// returns that hidden row.
//
// Rows 0 and 1 are tall (wrap onto several lines) so page 0 holds two rows;
// rows 2..6 are one line each, so the last page (opened at row 2) holds five.
func TestBughuntSh222PickScreenWrapHidesCursor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tall := strings.Repeat("tall row wraps onto several lines ", 6)
		rows := []string{
			"tall row 00 " + tall, "tall row 01 " + tall,
			"short row marker 02", "short row marker 03", "short row marker 04",
			"short row marker 05", "short row marker 06",
		}
		// Premise, measured on the real layout: page 0 shows 2 rows and the
		// page opened at row 2 shows all 5 remaining rows.
		mp := newPlatform()
		mp.display = sh2DisplaySize
		mctx := NewContext(mp)
		_, drawn0, _ := composerPickPageLayout(mctx, &descriptorTheme, sh2DisplaySize, "Choose one", rows, -1)
		_, drawn2, _ := composerPickPageLayout(mctx, &descriptorTheme, sh2DisplaySize, "Choose one", rows[2:], -1)
		shown0, shown2 := drawn0-composerPickRowBase, drawn2-composerPickRowBase
		if shown0 != 2 || shown2 != 5 {
			t.Fatalf("INCONCLUSIVE: page 0 shows %d rows (want 2), the page at row 2 shows %d (want 5)", shown0, shown2)
		}

		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var got int
		var ok bool
		frame, quit := runUI(ctx, func() {
			got, ok = composerPickScreen(ctx, &descriptorTheme, "Pick", "Choose one", rows)
		})
		defer quit()
		frame()
		// Down twice: the cursor leaves page 0 (2 rows) and the page re-homes
		// onto row 2, which is the last page.
		for i := 0; i < 2; i++ {
			click(&ctx.Router, Down)
			frame()
		}
		content, _ := frame()
		if !uiContains(content, "short row marker 02") {
			t.Fatalf("INCONCLUSIVE: the page did not re-home onto row 2.\nFrame: %q", content)
		}
		// The pager wraps to page 0.
		click(&ctx.Router, Button2)
		frame()
		content, _ = frame()
		if !uiContains(content, "tall row 00") {
			t.Fatalf("INCONCLUSIVE: the pager did not wrap to page 0.\nFrame: %q", content)
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 4; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if !ok {
			t.Fatal("the pick screen returned no selection")
		}
		if got >= shown0 {
			t.Errorf("Button3 returned row %d, but the page on screen draws only rows 0..%d "+
				"(the cursor was left on a row page 0 does not show).\nFrame: %q",
				got, shown0-1, content)
		}
	})
}

// Bug 23: gui/transaction.go transactionJobTime never carries 60 minutes into
// the hours, so 7,199 s reads "1 h 60 min". Wording of the correct text is an
// open product question ("2 h 0 min" vs "2 h"); this test pins only that the
// minutes field is below 60 and the total is right.
func TestBughuntSh223JobTimeCarriesMinutesIntoHours(t *testing.T) {
	t.Skip("deferred-minor: sh-jobtime-60-min")
	const tps = 1000
	bad := 0
	firstBad := ""
	for sec := uint64(3600); sec <= 3*3600; sec++ {
		got := transactionJobTime([]Plate{{Duration: sec * tps}}, tps)
		var h, m int
		if n, err := fmt.Sscanf(got, "about %d h %d min of cutting", &h, &m); n != 2 || err != nil {
			t.Fatalf("%d s: unparsed job time %q (%v)", sec, got, err)
		}
		wantTotal := int((sec + 59) / 60)
		if m >= 60 || h*60+m != wantTotal {
			bad++
			if firstBad == "" {
				firstBad = fmt.Sprintf("%d s -> %q", sec, got)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of the seconds in 1 h..3 h print a minutes field of 60 or a wrong total; first: %s", bad, firstBad)
	}
	if got := transactionJobTime([]Plate{{Duration: 7199 * tps}}, tps); strings.Contains(got, "60 min") {
		t.Errorf("7,199 s reads %q", got)
	}
}

// Bug 24: gui/descriptor_duplicate_keys.go truncateForDisplay documents "at
// most n runes" but returns n runes plus the 3-rune marker. Open product
// question: the contract. gui/descriptor_duplicate_keys_test.go:281 pins n+3
// (TestTheDisplayedTitleIsBounded), so the doc may be the wrong half. This test
// asserts the DOCUMENTED contract, which is the reported wrong behaviour's
// absence.
func TestBughuntSh224TruncateForDisplayAtMostN(t *testing.T) {
	t.Skip("deferred-minor: sh-truncate-doc")
	for _, n := range []int{10, 48} {
		got := truncateForDisplay(strings.Repeat("a", n+12), n)
		if r := len([]rune(got)); r > n {
			t.Errorf("truncateForDisplay(%d a's, %d) returned %d runes (%q), the doc says at most %d",
				n+12, n, r, got, n)
		}
	}
}

// Bug 45: composerAddPathKeylessRefusal returns "not refused" for the FIRST
// key-less path under a legacy sh / sh-wsh wrapper, so the operator is taken
// through the §8a bearer-access confirm and the hash entry before Done refuses
// the list (md.ErrComposeLegacyWrapperShape). The creation guard's own comment
// says it exists so nobody types a digest for a path the codec will not admit.
func TestBughuntSh245LegacyFirstKeylessPathRefusedAtCreation(t *testing.T) {
	t.Skip("deferred-minor: sh-legacy-keyless-path-at-creation")
	for _, w := range []md.ComposeWrapper{md.ComposeSh, md.ComposeShWsh} {
		list := md.PathList{Wrapper: w, Paths: []md.SpendPath{
			{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
		}}
		// Premise: the codec does refuse this list once the key-less path is
		// hashed (so a creation refusal is not a false alarm).
		digest, okDigest := md.NewHashLock(md.KindSha256, make([]byte, 32))
		if !okDigest {
			t.Fatal("INCONCLUSIVE: no test digest")
		}
		withHash := md.PathList{Wrapper: w, Paths: []md.SpendPath{
			list.Paths[0], {Hash: digest},
		}}
		if _, err := md.ValidatePathList(withHash); err == nil {
			t.Fatalf("INCONCLUSIVE (wrapper %v): the codec admits the list", w)
		}

		st := &composerState{list: list}
		h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
		h.mustReach("What can spend on this path?")
		h.choose(1) // A hash, no keys
		sawConfirm := false
		sawRefusal := uiContains(h.content, composerCopyRefuseLegacyShape())
		for i := 0; i < 8 && !sawRefusal; i++ {
			c, ok := h.frame()
			if !ok {
				break
			}
			h.content = c
			if uiContains(c, "EXPERIMENTAL") {
				sawConfirm = true
				break
			}
			sawRefusal = uiContains(c, composerCopyRefuseLegacyShape())
		}
		if sawConfirm || !sawRefusal {
			t.Errorf("wrapper %v: choosing 'A hash, no keys' on path 2 was not refused at "+
				"creation (reached the EXPERIMENTAL confirm: %v).\nLast frame: %q", w, sawConfirm, h.content)
		}
	}
}

// Bug 51: payloadTransactions merges a tx: record into a confirmed mt1 set on
// the txid alone, though its doc says only a BYTE-IDENTICAL record merges. The
// txid ignores witness data, so an unsigned copy, or a re-signed copy, of the
// same transaction has the same txid and different bytes, and is dropped.
//
// Open product question: whether the differing record should be offered as its
// own candidate (the doc's contract, asserted here) or the doc should say
// "same txid". The report grades it as lossless for the engraving.
func TestBughuntSh251PayloadMergeNeedsIdenticalBytes(t *testing.T) {
	t.Skip("deferred-minor: sh-payload-merge-same-txid")
	raw := evenTx(t).Raw
	resigned := append([]byte(nil), raw...)
	// A byte inside the 71-byte ECDSA signature: 4 (locktime) + 33 (pubkey) +
	// 1 (pubkey length) + 35 from the end of the witness block.
	resigned[len(resigned)-4-33-1-35] ^= 0x01
	for _, tc := range []struct {
		name string
		rec  string
	}{
		{"an UNSIGNED copy (witness stripped)", "tx:" + txStrippedHex},
		{"a RE-SIGNED copy (one signature byte differs)", "tx:" + fmt.Sprintf("%x", resigned)},
	} {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionWith(append(append([]string{}, txEven...), tc.rec)...)
		cands, _ := payloadTransactions(ctx)
		// Premise: the record parses to the SAME txid with DIFFERENT bytes.
		if len(cands) == 0 || cands[0].tx.TxidDisplay != txEvenTxid || string(cands[0].tx.Raw) != string(raw) {
			t.Fatalf("INCONCLUSIVE (%s): first candidate is not the confirmed set: %+v", tc.name, cands)
		}
		if tc.rec == "tx:"+rawHexOf(t, evenTx(t)) {
			t.Fatalf("INCONCLUSIVE (%s): the record is byte-identical", tc.name)
		}
		if len(cands) != 2 {
			t.Errorf("%s: payloadTransactions returned %d candidates, want 2 (the confirmed set and "+
				"the differing record): the differing record was silently merged away", tc.name, len(cands))
		}
	}
}
