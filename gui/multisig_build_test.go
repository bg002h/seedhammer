package gui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// TestMultisigScriptChoices: exactly the 3 sortedmulti wrappers, order-locked to
// the MultisigScript enum, wsh first (highlighted by default).
func TestMultisigScriptChoices(t *testing.T) {
	c := multisigScriptChoices()
	if len(c) != 3 {
		t.Fatalf("template choices = %d, want 3", len(c))
	}
	if multisigScriptFor(0) != md.MultisigWsh ||
		multisigScriptFor(1) != md.MultisigShWsh ||
		multisigScriptFor(2) != md.MultisigSh {
		t.Fatalf("template mapping wrong: 0=%v 1=%v 2=%v",
			multisigScriptFor(0), multisigScriptFor(1), multisigScriptFor(2))
	}
}

// TestMultisigNChoices: n picker offers exactly "2".."5" (n in 2..5).
func TestMultisigNChoices(t *testing.T) {
	c := multisigNChoices()
	want := []string{"2", "3", "4", "5"}
	if len(c) != len(want) {
		t.Fatalf("n choices = %v, want %v", c, want)
	}
	for i := range want {
		if c[i] != want[i] {
			t.Fatalf("n choices[%d] = %q, want %q", i, c[i], want[i])
		}
	}
	if multisigNFor(0) != 2 || multisigNFor(3) != 5 {
		t.Fatalf("n mapping wrong: 0=%d 3=%d, want 2 and 5", multisigNFor(0), multisigNFor(3))
	}
}

// TestMultisigKChoices: k picker is built from the chosen n as "1".."n" (k<=n,
// k>=1), so k>n is structurally unreachable.
func TestMultisigKChoices(t *testing.T) {
	for n := 2; n <= 5; n++ {
		c := multisigKChoices(n)
		if len(c) != n {
			t.Fatalf("n=%d: k choices = %v, want %d entries", n, c, n)
		}
		if c[0] != "1" {
			t.Fatalf("n=%d: k choices[0] = %q, want 1", n, c[0])
		}
		if multisigKFor(0) != 1 || multisigKFor(n-1) != n {
			t.Fatalf("n=%d: k mapping wrong: 0=%d last=%d", n, multisigKFor(0), multisigKFor(n-1))
		}
	}
}

// TestMultisigSelfSlotChoices: the self-slot picker offers "@0".."@{n-1}".
func TestMultisigSelfSlotChoices(t *testing.T) {
	for n := 2; n <= 5; n++ {
		c := multisigSelfSlotChoices(n)
		if len(c) != n {
			t.Fatalf("n=%d: self-slot choices = %v, want %d entries", n, c, n)
		}
		if c[0] != "@0" || c[n-1] != ("@"+string(rune('0'+n-1))) {
			t.Fatalf("n=%d: self-slot choices = %v", n, c)
		}
	}
}

// TestMultisigFpChoices: the fp-presence picker offers exactly No / Yes
// (Omit / Include), index 0 == Omit (default).
func TestMultisigFpChoices(t *testing.T) {
	c := multisigFpChoices()
	if len(c) != 2 {
		t.Fatalf("fp choices = %v, want 2", c)
	}
	if multisigIncludeFpFor(0) != false || multisigIncludeFpFor(1) != true {
		t.Fatalf("fp mapping wrong: 0=%v 1=%v, want false,true",
			multisigIncludeFpFor(0), multisigIncludeFpFor(1))
	}
}

// TestMultisigSharedOrigin pins the fixed BIP-48 P2WSH shared origin.
func TestMultisigSharedOrigin(t *testing.T) {
	got := multisigSharedOrigin().String()
	if got != "m/48h/0h/0h/2h" {
		t.Fatalf("shared origin = %q, want m/48h/0h/0h/2h", got)
	}
}

// TestAssembleBuildPolicy_T6bByteMatch is the strongest faithfulness gate (A3,
// R0-M4): reconstruct the T6b 2-of-3 wsh(sortedmulti) fixture's EXACT request
// from the DECODED fixture, drive fp-presence=OMIT (the fixture is fp-ABSENT),
// and assert the assembled md1 is byte-identical to the on-disk fixture with
// stub == 7b716421. (Include would yield a DIFFERENT id — see the next test.)
func TestAssembleBuildPolicy_T6bByteMatch(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("fixture has %d slots, want 3", len(keys))
	}

	// Self slot @1: derive the abandon-about key at the shared origin, exactly as
	// the Build flow does (deriveAccountXpub -> base58 xpub + masterFP).
	self := abandonAboutMnemonic()
	selfXpub, selfMasterFP, err := deriveAccountXpub(self, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub(self): %v", err)
	}

	// Foreign cosigners @0 and @2: rebuild MultisigCosigner DIRECTLY from the
	// decoded 65-byte ExpandedKey.Xpub (the fixture slots carry no base58 xpub).
	foreign := func(k md.ExpandedKey) md.MultisigCosigner {
		var cc [32]byte
		var pk [33]byte
		copy(cc[:], k.Xpub[0:32])
		copy(pk[:], k.Xpub[32:65])
		return md.MultisigCosigner{ChainCode: cc, CompressedPubkey: pk, FpPresent: false}
	}
	selfCC, selfPK, _, err := decodeXpubBytes(selfXpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes(self): %v", err)
	}
	selfCos := md.MultisigCosigner{ChainCode: selfCC, CompressedPubkey: selfPK, FpPresent: false}

	req := md.EncodeMultisigRequest{
		Cosigners:    []md.MultisigCosigner{foreign(keys[0]), selfCos, foreign(keys[2])}, // @0, @1=self, @2
		K:            2,
		Script:       md.MultisigWsh,
		OriginMode:   md.OriginShared,
		SharedOrigin: originComponents(multisigSharedOrigin()),
	}
	out, stub, _, err := md.EncodeMultisig(req)
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	if len(out) != len(chunks) {
		t.Fatalf("assembled %d chunks, want %d", len(out), len(chunks))
	}
	for i := range chunks {
		if out[i] != chunks[i] {
			t.Fatalf("chunk[%d] mismatch (fp-absent T6b replay):\n got %s\nwant %s", i, out[i], chunks[i])
		}
	}
	wantStub := [4]byte{0x7b, 0x71, 0x64, 0x21}
	if stub != wantStub {
		t.Fatalf("stub = %x, want 7b716421", stub)
	}
	// Sanity: the self slot really is the abandon-about key (masterFP 0x73c5da0a).
	if selfMasterFP != 0x73c5da0a {
		t.Fatalf("self masterFP = %08x, want 73c5da0a", selfMasterFP)
	}
}

// TestAssembleBuildPolicy_IncludeFpDiffers asserts the SAME keys/order with
// fp-presence=INCLUDE yields a DIFFERENT id (the real id for the fixture keys
// with foreign fp {1,2,3,4} is ceadba4d; never assert a specific Include id from
// the encoder beyond != 7b716421), confirming the homogeneous fp choice changes
// the WalletPolicyId (M1/M4).
func TestAssembleBuildPolicy_IncludeFpDiffers(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	self := abandonAboutMnemonic()
	selfXpub, selfMasterFP, err := deriveAccountXpub(self, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub: %v", err)
	}
	selfCC, selfPK, _, err := decodeXpubBytes(selfXpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	var selfFP [4]byte
	selfFP[0] = byte(selfMasterFP >> 24)
	selfFP[1] = byte(selfMasterFP >> 16)
	selfFP[2] = byte(selfMasterFP >> 8)
	selfFP[3] = byte(selfMasterFP)
	withFp := func(k md.ExpandedKey) md.MultisigCosigner {
		var cc [32]byte
		var pk [33]byte
		copy(cc[:], k.Xpub[0:32])
		copy(pk[:], k.Xpub[32:65])
		// Foreign slots carry no fp in the fixture; for the INCLUDE homogeneous
		// case synthesize a fingerprint — this test only asserts the id DIFFERS
		// from the fp-absent golden.
		return md.MultisigCosigner{ChainCode: cc, CompressedPubkey: pk, Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true}
	}
	req := md.EncodeMultisigRequest{
		Cosigners:    []md.MultisigCosigner{withFp(keys[0]), {ChainCode: selfCC, CompressedPubkey: selfPK, Fingerprint: selfFP, FpPresent: true}, withFp(keys[2])},
		K:            2,
		Script:       md.MultisigWsh,
		OriginMode:   md.OriginShared,
		SharedOrigin: originComponents(multisigSharedOrigin()),
	}
	_, stub, _, err := md.EncodeMultisig(req)
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	if stub == [4]byte{0x7b, 0x71, 0x64, 0x21} {
		t.Fatal("INCLUDE fp produced the fp-absent stub 7b716421; fp-presence must change the id")
	}
	_ = bytes.Equal // keep the import if unused elsewhere
	_ = mk.Card{}   // keep the mk import referenced
}

// TestAssembleBuildPolicy_Wrapper exercises the high-level assembleBuildPolicy
// (the SOLE EncodeMultisig caller) end-to-end via gathered mk.Cards: 2-of-2,
// self @0, one cosigner @1, fp-omit. The stub it returns must equal
// WalletPolicyIDStubChunks(out) (I-STUB) and a deriveMultisigLeg over `out`
// binds the operator mk1 to that same stub.
func TestAssembleBuildPolicy_Wrapper(t *testing.T) {
	self := abandonAboutMnemonic()
	selfXpub, selfMasterFP, err := deriveAccountXpub(self, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub(self): %v", err)
	}
	// One foreign cosigner as a real base58 xpub: reuse the canonical bip85 master
	// derived at the shared origin (any valid mainnet xpub works).
	other := canonicalBip85Master(t)
	otherXpub, _, err := deriveAccountXpub(other, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub(other): %v", err)
	}
	otherCard := mk.Card{Network: "mainnet", Path: "m/48h/0h/0h/2h", Fingerprint: "", Xpub: otherXpub, Stubs: [][4]byte{{0, 0, 0, 0}}}

	p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlot: 0, IncludeFp: false}
	out, stub, slots, err := assembleBuildPolicy(p, selfXpub, selfMasterFP, []mk.Card{otherCard})
	if err != nil {
		t.Fatalf("assembleBuildPolicy: %v", err)
	}
	gotStub, err := md.WalletPolicyIDStubChunks(out)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if gotStub != stub {
		t.Fatalf("returned stub %x != WalletPolicyIDStubChunks(out) %x (I-STUB)", stub, gotStub)
	}
	if len(slots) != 2 {
		t.Fatalf("slots = %d, want 2", len(slots))
	}
	// Self is @0 (SelfSlot=0); here fp-omit so all FpPresent must be false.
	for i, s := range slots {
		if s.FpPresent {
			t.Fatalf("slot %d FpPresent=true under fp-omit", i)
		}
	}
	// I-STUB downstream: deriveMultisigLeg over `out` binds to the same stub.
	b, err := deriveMultisigLeg(self, "", &chaincfg.MainNetParams, multisigSharedOrigin(), out, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg: %v", err)
	}
	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != stub {
		t.Fatalf("mk1 stub = %v, want [%x] (I-STUB)", card.Stubs, stub)
	}
}

// TestBuildReviewLines: the review reflects the stub, each @N->fp(+present), the
// chosen homogeneous fp-presence, and the M1 note that fp-presence affects the
// policy id. Drive both Omit and Include. The Include stub here is an
// illustrative literal (any 4 bytes); the encoder is never asserted to produce
// it (see the Include-differs test, which only asserts != 7b716421).
func TestBuildReviewLines(t *testing.T) {
	stub := [4]byte{0x7b, 0x71, 0x64, 0x21}
	slotsOmit := []md.SlotInfo{
		{Index: 0, FpPresent: false},
		{Index: 1, FpPresent: false},
		{Index: 2, FpPresent: false},
	}
	lines := buildReviewLines(stub, slotsOmit, false)
	joined := strings.ToLower(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "7b716421") {
		t.Fatalf("review missing stub 7b716421:\n%s", joined)
	}
	if !strings.Contains(joined, "@0") || !strings.Contains(joined, "@2") {
		t.Fatalf("review missing per-slot @N lines:\n%s", joined)
	}
	if !strings.Contains(joined, "fingerprint") {
		t.Fatalf("review missing the fp-presence note:\n%s", joined)
	}

	// Illustrative Include stub (e.g. ceadba4d) — NOT asserted against the encoder.
	slotsInc := []md.SlotInfo{
		{Index: 0, Fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, FpPresent: true},
		{Index: 1, Fingerprint: [4]byte{0x01, 0x02, 0x03, 0x04}, FpPresent: true},
	}
	linesInc := buildReviewLines([4]byte{0xce, 0xad, 0xba, 0x4d}, slotsInc, true)
	joinedInc := strings.ToLower(strings.Join(linesInc, "\n"))
	if !strings.Contains(joinedInc, "ceadba4d") {
		t.Fatalf("include review missing stub ceadba4d:\n%s", joinedInc)
	}
	if !strings.Contains(joinedInc, "73c5da0a") {
		t.Fatalf("include review missing slot @0 fp 73c5da0a:\n%s", joinedInc)
	}
}
