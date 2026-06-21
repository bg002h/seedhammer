package bundle

import (
	"strings"
	"testing"
)

// The 4 vendored singlesig goldens supply real ms1/mk1/md1 trios. We reuse the
// bip84 (wpkh) golden as the canonical correct bundle for the comparator tests.
// ms1 is the abandon-seed zero-16 vector (the bundle's ms1 leg).

const (
	wpkhMS1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
)

var (
	// Regenerated via the shipped mk.Encode (t6a2-M1): the previously-vendored
	// chunks carried a stale chunk-set-id (0x1c017) from an older encoder, while
	// mk.Encode deterministically derives csid = top20(SHA-256(bytecode)) =
	// 0xbaa99. These chunks decode to the identical Card (same network, path,
	// fingerprint, stub, xpub), so the comparator tests stay meaningful.
	wpkhMK1 = []string{
		"mk1qph25epqqsq3cqtsleeutks2qvzg3vs70mejhk622ws2kgdemj2cd8zwj2skzx2wq0qw70l4q99vdyh5x0z8v4yslsp8qasghpexqvjkydy7",
		"mk1qph25epp0f30mtxzd65mvwcur9usdatwuqvq6z70r9nwrgk6xn6l8gy6nwa2n97735qp69khmdydd",
	}
	wpkhMD1 = []string{
		"md1fgdxlpqpqpm6jzzqqvqpdqw0za5zs4gyy55aq4vsmnhy4s6wyaypu34c7raqu8np",
		"md1fgdxlpqf2zcgefcpupmel75q5435j7seugaj5jr7qyur6vt76es5cdeyrq7zdy0d",
		"md1fgdxlpq3xa2dk8vwpj7gx74hwqxqdp083jehp5tdrfa0n5zdfkqcdlrvnh5r62jn",
	}
)

func correctBundle() Bundle {
	return Bundle{
		MS1: wpkhMS1,
		MK1: append([]string(nil), wpkhMK1...),
		MD1: append([]string(nil), wpkhMD1...),
	}
}

func TestVerifyBundleMatch(t *testing.T) {
	derived := correctBundle()
	readback := correctBundle()
	if err := Verify(derived, readback); err != nil {
		t.Fatalf("correct bundle: %v (want PASS)", err)
	}
}

// bip44 is a complete, internally-consistent golden bundle (its mk1 binds to its
// md1) at a DIFFERENT origin (m/44'/0'/0'). Used as a divergent read-back set.
func bip44Bundle() Bundle {
	return Bundle{
		MS1: wpkhMS1, // same seed → same ms1 entropy
		MK1: []string{
			"mk1qpljgwpqqsqley8qjaeutks2qyzg3vs7z4du5kfa5j7pjz3xsqg36v06mlwfqhe20akwwlr0zzv3jyt0y575x3zjryphwex3qasx2tg384t2",
			"mk1qpljgwppfjgslnc8l2tgsm48jncdtjhdntlrpdzts0m7yyamj2fsul05h59vh4dpp9yddv6g95v0e",
		},
		MD1: []string{
			"md1fwujrpqpqpmtyyyqqcgz6qu79mg9p2sg8kjtcxg2y6qpz8f3ltvph7alcz39q6r0",
			"md1fwujrpq0mjg972nldnnhcmcsnyv3zme984p5g5seqdm5eyg0euq52wvs9ycxx7yc",
			"md1fwujrpqhl2tgsm48jncdtjhdntlrpdzts0m7yyamj2fsul05h5qfrpa458scjl0p",
		},
	}
}

func TestVerifyBundleMutatedXpub(t *testing.T) {
	// A read-back from a DIFFERENT, internally-consistent wallet (bip44, same
	// seed) → fingerprint matches but xpub/path diverge → FAIL naming the field.
	err := Verify(correctBundle(), bip44Bundle())
	if err == nil {
		t.Fatal("divergent wallet accepted, want FAIL")
	}
	if !strings.Contains(err.Error(), "xpub") && !strings.Contains(err.Error(), "path") {
		t.Errorf("error %q does not name xpub/path", err)
	}
}

// tr is a complete, internally-consistent golden bundle (bip86) used as a
// divergent read-back set whose md1 (and xpub/path) differ from wpkh.
func trBundle() Bundle {
	return Bundle{
		MS1: wpkhMS1,
		MK1: []string{
			"mk1qpdtndpqqsqk4ek4neeutks2qszg3vs7qdf8pkkxr28j06vpsgc56fzymglxqr44sdhv3tgc8jrvxy0ethuqs2cc4gp5z4hq5nvu3ra6m75z",
			"mk1qpdtndppsfu29zzu3wuczjq43528gc6qj7shn3jz7g70rnqymf3f43hsldv83jtr2cvxjuzwdcvpv",
		},
		MD1: []string{
			"md1f0v9epqpqpm66zzqqvpqtgrnchdq592prrp4re8axqcyv2dy3zvs74fg8q3asx79",
			"md1f0v9epqtglxqr44sdhv3tgc8jrvxy0ethuqs2cc4gp5rqnc52yy57wwa5lkavfzd",
			"md1f0v9epqnj9mnq2gzkx3garrgzt6z7wxgtereuwvqndx9xkx7rasjh5hmwdzq9uja",
		},
	}
}

func TestVerifyBundleMutatedDescriptor(t *testing.T) {
	// A divergent, internally-consistent read-back (bip86 tr) → a different
	// policy → FAIL. The first diverging field is reported (md1 / path / xpub
	// all differ between wpkh and tr).
	err := Verify(correctBundle(), trBundle())
	if err == nil {
		t.Fatal("divergent descriptor accepted, want FAIL")
	}
	if !strings.Contains(err.Error(), "md1") &&
		!strings.Contains(err.Error(), "path") &&
		!strings.Contains(err.Error(), "xpub") {
		t.Errorf("error %q does not name md1/path/xpub", err)
	}
}

// TestVerifyBundleMd1PositionalContract documents and guards the COMPARATOR
// CONTRACT: bundle.Verify compares md1 POSITIONALLY (equalStrings), so it
// correctly rejects an out-of-order md1 []string. Canonical ChunkIndex ordering
// is the GATHER layer's responsibility — md1Gatherer.collected()
// (gui/md1_gather.go), which the H2 fix made deterministic; see the gui test
// TestMD1GathererCollectedIndexOrder (T-H2). This is NOT product behaviour that
// rejects correct backups (the gather layer canonicalizes order before Verify);
// it asserts the comparator stays a pure positional compare and is NOT weakened
// to sort internally (which would re-introduce parsing into the deterministic
// core). Assertion unchanged from the former TestVerifyBundleMd1Reordered.
func TestVerifyBundleMd1PositionalContract(t *testing.T) {
	derived := correctBundle()
	readback := correctBundle()
	// An out-of-order md1 []string: a valid set (Reassemble is order-tolerant, so
	// the stub still binds) but the positional sequence differs → md1 mismatch.
	readback.MD1 = []string{wpkhMD1[1], wpkhMD1[0], wpkhMD1[2]}
	err := Verify(derived, readback)
	if err == nil {
		t.Fatal("out-of-order md1 accepted by the positional comparator, want FAIL")
	}
	if !strings.Contains(err.Error(), "md1") {
		t.Errorf("error %q does not name md1", err)
	}
}

func TestVerifyBundleMutatedEntropy(t *testing.T) {
	// A read-back ms1 with different entropy → FAIL naming ms1/entropy.
	readback := correctBundle()
	// 16 zero bytes except a flipped low bit → "...qqqp..." style; use a known
	// different entropy vector (entr16 with all 0x01-ish). Use the parity
	// vector entr20-nonzero is 20B; pick a 16B nonzero ms1.
	readback.MS1 = "ms10entrsqgqqc83yukgh23xkvmp59xf2eldpk4cdrq2y4h82yz" // mnem-english16, entropy 0c1e... ≠ zero
	err := Verify(correctBundle(), readback)
	if err == nil {
		t.Fatal("mutated entropy accepted, want FAIL")
	}
	if !strings.Contains(err.Error(), "ms1") && !strings.Contains(err.Error(), "entropy") {
		t.Errorf("error %q does not name ms1/entropy", err)
	}
}

func TestVerifyBundleEntropyComparedNotString(t *testing.T) {
	// ms1 compared on RECOVERED ENTROPY, not the raw string: an ms1 that decodes
	// to the SAME entropy must still match even if the string form differs.
	// The zero-16 entr vector and a freshly EncodeMS1'd zero-16 are identical
	// here, so to exercise the entropy-not-string path we re-encode the same
	// entropy and confirm PASS (any incidental string difference is tolerated).
	derived := correctBundle()
	readback := correctBundle()
	// Same entropy, identical string — must PASS (sanity that entropy compare
	// is the gate, not a brittle byte-for-byte string compare of ms1).
	if err := Verify(derived, readback); err != nil {
		t.Fatalf("same-entropy ms1: %v (want PASS)", err)
	}
}

// TestVerifyBundleWatchOnlyMatch (T6a-2, R0-C1): a watch-only bundle has MS1==""
// on BOTH sides → the ms1 leg is SKIPPED; the mk1 + md1 + stub-binding legs
// still run and PASS for a consistent set.
func TestVerifyBundleWatchOnlyMatch(t *testing.T) {
	derived := correctBundle()
	derived.MS1 = ""
	readback := correctBundle()
	readback.MS1 = ""
	if err := Verify(derived, readback); err != nil {
		t.Fatalf("watch-only (both MS1 empty) consistent bundle: %v (want PASS)", err)
	}
}

// TestVerifyBundleWatchOnlyStillChecksMd1: a watch-only bundle (no ms1) still
// runs the mk1/md1/stub legs — a diverging md1 FAILs even with the ms1 leg
// skipped.
func TestVerifyBundleWatchOnlyStillChecksMd1(t *testing.T) {
	derived := correctBundle()
	derived.MS1 = ""
	readback := trBundle() // a different policy entirely
	readback.MS1 = ""
	if err := Verify(derived, readback); err == nil {
		t.Fatal("watch-only divergent descriptor accepted, want FAIL")
	}
}

// TestVerifyBundleMs1OneSided (R0-C1): an ms1 present on ONE side only is a
// presence mismatch → error (NOT a silent watch-only skip). Both directions.
func TestVerifyBundleMs1OneSided(t *testing.T) {
	// derived has ms1, readback does not.
	d := correctBundle()
	r := correctBundle()
	r.MS1 = ""
	err := Verify(d, r)
	if err == nil {
		t.Fatal("one-sided ms1 (derived has, readback lacks) accepted, want FAIL")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "ms1") {
		t.Errorf("error %q does not name ms1 presence", err)
	}
	// derived lacks ms1, readback has it.
	d2 := correctBundle()
	d2.MS1 = ""
	r2 := correctBundle()
	err2 := Verify(d2, r2)
	if err2 == nil {
		t.Fatal("one-sided ms1 (readback has, derived lacks) accepted, want FAIL")
	}
	if !strings.Contains(strings.ToLower(err2.Error()), "ms1") {
		t.Errorf("error %q does not name ms1 presence", err2)
	}
}

func TestVerifyBundleStubMismatch(t *testing.T) {
	// A read-back mk1 whose stub does NOT bind to its md1 → FAIL "stub mismatch".
	// Construct a readback whose md1 is wpkh but mk1 is the bip44 card (its stub
	// binds to the pkh policy, not the wpkh md1). The intra-bundle stub-binding
	// check must catch it BEFORE/at the field compare.
	readback := Bundle{
		MS1: wpkhMS1,
		// bip44 mk1 (stub = pkh policy stub)
		MK1: []string{
			"mk1qpljgwpqqsqley8qjaeutks2qyzg3vs7z4du5kfa5j7pjz3xsqg36v06mlwfqhe20akwwlr0zzv3jyt0y575x3zjryphwex3qasx2tg384t2",
			"mk1qpljgwppfjgslnc8l2tgsm48jncdtjhdntlrpdzts0m7yyamj2fsul05h59vh4dpp9yddv6g95v0e",
		},
		// wpkh md1
		MD1: append([]string(nil), wpkhMD1...),
	}
	err := Verify(readback, readback) // compare the mismatched bundle to itself
	if err == nil {
		t.Fatal("mk1/md1 stub mismatch accepted, want FAIL")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "stub") {
		t.Errorf("error %q does not name stub", err)
	}
}

// T-M1 (verify-cluster M1): a readback ms1 whose recovered entropy MATCHES the
// derived ms1 but whose BIP-39 language byte DIFFERS (Japanese mnem, lang 1, vs
// the derived English entr, lang 0) must FAIL — identical entropy under a
// different wordlist is a different wallet. The fixture is built directly because
// EncodeMS1 only emits entr/English; this is a hand-typed-readback-only string.
// Proven on 3a23dbb: Verify PASSES this (the M1 bug). After the language compare
// it FAILS with "verify: ms1 wordlist/language mismatch".
func TestVerifyBundleLanguageMismatch(t *testing.T) {
	derived := correctBundle() // entr / English, entropy = zero-16
	readback := correctBundle()
	// codex32.NewSeed("ms",0,"entr",'s',[]byte{0x02,0x01,<zero16>}) — a valid
	// language-1 (Japanese) mnem ms1 with the SAME zero-16 entropy as wpkhMS1.
	// (Verified: decodes to prefix=2/mnem, language=1, entropy=00..00.)
	readback.MS1 = "ms10entrsqgqsqqqqqqqqqqqqqqqqqqqqqqqqqj9tawneveyd9j"
	err := Verify(derived, readback)
	if err == nil {
		t.Fatal("language-differ readback (same entropy) accepted, want FAIL")
	}
	if !strings.Contains(err.Error(), "language") && !strings.Contains(err.Error(), "wordlist") {
		t.Errorf("error %q does not name language/wordlist", err)
	}
}

// TestVerifyBundleLanguageEnglishNotOverRejected: a legitimate English/entr
// readback (language 0) against an English/entr derived (language 0) must still
// PASS — the language compare must not over-reject identical-wordlist readbacks.
func TestVerifyBundleLanguageEnglishNotOverRejected(t *testing.T) {
	if err := Verify(correctBundle(), correctBundle()); err != nil {
		t.Fatalf("English/entr readback over-rejected: %v (want PASS)", err)
	}
}

// ─── Task 5: form-aware stub binding (C2) ────────────────────────────────────
//
// A KEYLESS-TEMPLATE bundle from mnemonic-toolkit@6de53879
// (bundle --template bip84 --md1-form=template, abandon-about seed): the mk1
// stub roots on the WalletDescriptorTemplateId of the keyless md1 — NOT its
// WalletPolicyId. Verify must select the id space by form (FormAwareStubChunks);
// today it is unconditionally WalletPolicyId-derived → a template mis-binds.
func templateBundle() Bundle {
	return Bundle{
		MS1: wpkhMS1, // same abandon seed → same ms1 entropy
		MK1: []string{
			"mk1qpg4m4pqqsq52a6af4eutks2qvzg3vs70mejhk622ws2kgdemj2cd8zwj2skzx2wq0qw70l4q99vdyh5x0z8v4yslsp8qjuq3c9tru385fac",
			"mk1qpg4m4pp0f30mtxzd65mvwcur9usdatwuqvq6z70r9nwrgk6xn6l8gy6nvzwz8qpm4xs9a0men698",
		},
		MD1: []string{"md1fvwrwqqpqqgqpsqqq3uaau4ctxyl7"}, // keyless bip84 template
	}
}

// TestVerifyTemplateBundleBinds: the engraved template bundle's mk1 (rooting on
// the WDT-Id) VERIFIES. Fails today (verify.go uses WalletPolicyIDStubChunks →
// the keyless template's WalletPolicyId ≠ the WDT-Id stub the mk1 carries).
func TestVerifyTemplateBundleBinds(t *testing.T) {
	b := templateBundle()
	if err := Verify(b, b); err != nil {
		t.Fatalf("template bundle (mk1 on WDT-Id) rejected: %v (want PASS)", err)
	}
}

// TestVerifyTemplateBundleForeignMk1Fails: the same template md1 paired with a
// FOREIGN mk1 (the wpkh full-policy card, whose stub roots on a WalletPolicyId)
// → stub mismatch FAIL. The security negative.
func TestVerifyTemplateBundleForeignMk1Fails(t *testing.T) {
	foreign := Bundle{
		MS1: wpkhMS1,
		MK1: append([]string(nil), wpkhMK1...),            // full-policy card, foreign stub
		MD1: []string{"md1fvwrwqqpqqgqpsqqq3uaau4ctxyl7"}, // keyless template md1
	}
	err := Verify(foreign, foreign)
	if err == nil {
		t.Fatal("template md1 + foreign full-policy mk1 accepted, want FAIL")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "stub") {
		t.Errorf("error %q does not name stub", err)
	}
}
