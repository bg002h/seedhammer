package gui

// Lens-1 (funds safety and policy correctness) tests written for the composer
// fable review r0, adopted from the reviewer's preserved harness
// (`/scratch/code/shibboleth/.tmp/fable-funds-work/harness/`, three
// `zz_fable_funds_*_test.go` files, package gui).
//
// The three RED-at-tip assertions are kept verbatim in substance -- each one
// was the counterexample for a finding -- and the reviewer's FABLE_OUT-gated
// evidence drivers (JSON dumps for the host and Bitcoin Core legs) are not
// adopted: they assert nothing and skip unless an env var is set. The helpers
// they shared (`fableSeeds`, the two preimages, `fableHash`) are kept because
// the assertions use them.
//
// C-1 additionally asserts AGAINST THE HOST ORACLE: `md compose --wrapper wsh
// --experimental` is the Rust primary, and the Go port must admit exactly the
// lists it admits. The oracle leg skips when `md` is not on PATH; the literal
// table beside it does not, so the rule is pinned with or without the CLI.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

var fableFundsSeeds = []string{
	"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
	"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
}

var fableFundsPreimage = sha256.Sum256([]byte("fable-funds-preimage-1"))
var fableFundsPreimage2 = sha256.Sum256([]byte("fable-funds-preimage-2"))

func fableFundsHash(t *testing.T, pre [32]byte) *md.HashLock {
	t.Helper()
	d := sha256.Sum256(pre[:])
	h, ok := md.NewHashLock(md.KindSha256, d[:])
	if !ok {
		t.Fatal("md.NewHashLock refused a 32-byte sha256 digest")
	}
	return h
}

// ─── C-1: at most ONE key-less path per policy ──────────────────────────────

// fableKeylessCase is one path list, named the way `md compose --path` names
// it, so the oracle leg can be built from the same row.
type fableKeylessCase struct {
	what  string
	args  []string // the --path arguments, in listed order
	paths []md.SpendPath
	admit bool // what `md compose --wrapper wsh --experimental` does
}

func fableKeylessCases(t *testing.T) []fableKeylessCase {
	t.Helper()
	H := fableFundsHash(t, fableFundsPreimage)
	H2 := fableFundsHash(t, fableFundsPreimage2)
	h1 := fmt.Sprintf("sha256=%x", H.Digest())
	h2 := fmt.Sprintf("sha256=%x", H2.Digest())
	keyed := func(k, n uint8) md.SpendPath {
		return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}}
	}
	older := func(n uint32) *md.Lock { return &md.Lock{Kind: md.LockOlderBlocks, Value: n} }
	after := func(h uint32) *md.Lock { return &md.Lock{Kind: md.LockAfterHeight, Value: h} }
	return []fableKeylessCase{
		// REFUSED: two or more key-less paths, whatever locks they carry and
		// wherever they sit. Measured against md 0.16.2.
		{"[keyed, K, K]", []string{"1of1", "keyless," + h1, "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, {Hash: H2}}, false},
		{"[keyed, K, K+older(5)]", []string{"1of1", "keyless," + h1, "keyless," + h2 + ",older=5"},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, {Hash: H2, Lock: older(5)}}, false},
		{"[K, K+older(5), keyed]", []string{"keyless," + h1, "keyless," + h2 + ",older=5", "1of1"},
			[]md.SpendPath{{Hash: H}, {Hash: H2, Lock: older(5)}, keyed(1, 1)}, false},
		{"[keyed, K+older(5), K+after(200)]", []string{"1of1", "keyless," + h1 + ",older=5", "keyless," + h2 + ",after=200"},
			[]md.SpendPath{keyed(1, 1), {Hash: H, Lock: older(5)}, {Hash: H2, Lock: after(200)}}, false},
		{"[K+older(5), keyed, K+after(200)]", []string{"keyless," + h1 + ",older=5", "1of1", "keyless," + h2 + ",after=200"},
			[]md.SpendPath{{Hash: H, Lock: older(5)}, keyed(1, 1), {Hash: H2, Lock: after(200)}}, false},
		{"[keyed, 2of2, K, K]", []string{"1of1", "2of2", "keyless," + h1, "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), keyed(2, 2), {Hash: H}, {Hash: H2}}, false},
		{"[keyed, K, 2of2, K]", []string{"1of1", "keyless," + h1, "2of2", "keyless," + h2},
			[]md.SpendPath{keyed(1, 1), {Hash: H}, keyed(2, 2), {Hash: H2}}, false},
		// ADMITTED: at most one key-less path.
		{"[K+older(5), 2of3]", []string{"keyless," + h1 + ",older=5", "2of3"},
			[]md.SpendPath{{Hash: H, Lock: older(5)}, keyed(2, 3)}, true},
		{"[keyed, 2of2, K]", []string{"1of1", "2of2", "keyless," + h1},
			[]md.SpendPath{keyed(1, 1), keyed(2, 2), {Hash: H}}, true},
		{"[keyed, K]", []string{"1of1", "keyless," + h1},
			[]md.SpendPath{keyed(1, 1), {Hash: H}}, true},
		{"[K, keyed]", []string{"keyless," + h1, "1of1"},
			[]md.SpendPath{{Hash: H}, keyed(1, 1)}, true},
		{"[keyed, K+older(5)]", []string{"1of1", "keyless," + h1 + ",older=5"},
			[]md.SpendPath{keyed(1, 1), {Hash: H, Lock: older(5)}}, true},
	}
}

// TestFableRedTwoKeylessPathsAreRefused is lens-1 C-1.
//
// `or_i(l, r)` is non-malleable only if one arm is `safe` (needs a signature),
// and a key-less path is never safe -- so two of them anywhere in the list put
// two unsafe arms under one `or_i` in §5's right-leaning chain, whatever locks
// they carry. The primary catches it by re-parsing its own lowering through
// `md encode`; this port has no post-lowering parse (md/compose.go:1-22
// "It emits no text"), so the rule is stated in ValidatePathList instead.
func TestFableRedTwoKeylessPathsAreRefused(t *testing.T) {
	for _, c := range fableKeylessCases(t) {
		list := md.PathList{Wrapper: md.ComposeWsh, Paths: c.paths}
		_, err := md.ValidatePathList(list)
		switch {
		case c.admit && err != nil:
			t.Errorf("ValidatePathList refused %s (%v); md compose --experimental admits it", c.what, err)
		case !c.admit && err == nil:
			t.Errorf("ValidatePathList admitted %s; md compose --experimental refuses it as malleable", c.what)
		case !c.admit && !errors.Is(err, md.ErrComposeTwoKeylessPaths):
			t.Errorf("ValidatePathList refused %s with %v, want ErrComposeTwoKeylessPaths", c.what, err)
		}
	}
}

// TestFableTwoKeylessPathsAgreeWithTheHostOracle runs the SAME lists through
// `md compose` and requires the Go port's answer to match arm for arm.
//
// The Rust primary is normative (CLAUDE.md, Rust-primary rule), so the table
// above is only as good as its agreement with the CLI. Skipped where `md` is
// absent -- the table still runs.
func TestFableTwoKeylessPathsAgreeWithTheHostOracle(t *testing.T) {
	bin, err := exec.LookPath("md")
	if err != nil {
		t.Skip("md is not on PATH; the literal table in TestFableRedTwoKeylessPathsAreRefused still runs")
	}
	for _, c := range fableKeylessCases(t) {
		args := []string{"compose", "--wrapper", "wsh", "--experimental"}
		for _, p := range c.args {
			args = append(args, "--path", p)
		}
		out, cerr := exec.Command(bin, args...).CombinedOutput()
		oracleAdmits := cerr == nil
		if oracleAdmits != c.admit {
			t.Fatalf("the table says md compose admits=%v for %s, the CLI says %v:\n%s",
				c.admit, c.what, oracleAdmits, out)
		}
		_, gerr := md.ValidatePathList(md.PathList{Wrapper: md.ComposeWsh, Paths: c.paths})
		if (gerr == nil) != oracleAdmits {
			t.Errorf("%s: md compose admits=%v, md.ValidatePathList admits=%v (%v)",
				c.what, oracleAdmits, gerr == nil, gerr)
		}
	}
}

// ─── I-2: the same KEY at two slots, whatever its serialisation ─────────────

// TestFableRedSameKeyReserializedIsRefused is lens-1 I-2.
//
// §7d's same-xpub refusal compared xpub STRINGS. The same key re-serialised
// with a zero parent fingerprint -- the form `md descriptor` itself prints
// (F-611), so an operator copying an xpub off host output produces exactly
// this -- is one key on the wire and two strings on the screen, and the
// "2-of-3" it built was spent by one signer signing twice (Core v31.1,
// `sig1 == sig2`).
func TestFableRedSameKeyReserializedIsRefused(t *testing.T) {
	st := fableSameKeyTwoSlots(t)
	if _, _, dup := composerDuplicateXpub(st); !dup {
		t.Fatalf("composerDuplicateXpub did not refuse the same key at @0 and @1 (xpub strings %q vs %q)",
			st.assigned[0].xpub, st.assigned[1].xpub)
	}
}

// TestFableSameKeyReserializedIsRefusedAtTheMint is the other half of I-2:
// the device must not MINT what `md encode` refuses. composerArtifactsFor is
// the mint path, and it binds the 65-byte chain-code||point, so the two
// strings collapse to one key there.
func TestFableSameKeyReserializedIsRefusedAtTheMint(t *testing.T) {
	st := fableSameKeyTwoSlots(t)
	_, keyed, err := composerArtifactsFor(st)
	if err == nil {
		t.Fatalf("composerArtifactsFor minted a policy that repeats a key at @0 and @1: %v", keyed)
	}
	if !errors.Is(err, md.ErrComposeRepeatedKeyMaterial) {
		t.Fatalf("composerArtifactsFor refused with %v, want md.ErrComposeRepeatedKeyMaterial", err)
	}
	// And §8m draws the §7d body for it rather than the codec's own text.
	body, ok := composerRefusalBody(err)
	if !ok || body != composerCopySameXpub(0, 1) {
		t.Fatalf("composerRefusalBody(%v) = %q, %v; want the §7d same-key body", err, body, ok)
	}
}

// fableSameKeyTwoSlots seats @0 from a seed, @1 from a `key:`-shaped source
// carrying THE SAME KEY re-serialised (zero parent fingerprint, depth 4,
// child 2') under a different declared account, and @2 from a second seed.
func fableSameKeyTwoSlots(t *testing.T) *composerState {
	t.Helper()
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}}}}
	st := &composerState{list: list, reg: &seedRegistry{}}
	for i := 0; i < 2; i++ {
		m, err := bip39.ParseMnemonic(fableFundsSeeds[i])
		if err != nil {
			t.Fatal(err)
		}
		id, err := st.reg.add("seed", m, "", &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		seed, _ := st.reg.at(id)
		var fp [4]byte
		binary.BigEndian.PutUint32(fp[:], seed.MasterFP)
		st.sources = append(st.sources, composerSource{kind: composerSourceSeed, fingerprint: fp, fpPresent: true, seedID: id})
	}
	composerSizeAssignments(st)
	a0, err := composerSeedDerive(st, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	st.assigned[0] = a0
	ek, err := hdkeychain.NewKeyFromString(a0.xpub)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ek.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	re := hdkeychain.NewExtendedKey(chaincfg.MainNetParams.HDPublicKeyID[:],
		pub.SerializeCompressed(), ek.ChainCode(), []byte{0, 0, 0, 0},
		4, 2+hdkeychain.HardenedKeyStart, false)
	if re.String() == a0.xpub {
		t.Fatal("the re-serialisation is byte-identical, so the test cannot see a string comparison")
	}
	src := composerSource{kind: composerSourceKey, fingerprint: a0.fingerprint, fpPresent: true,
		origin: md.DefaultOrigin(md.ComposeWsh, 1), xpub: re.String(), seedID: -1}
	st.sources = append(st.sources, src)
	st.assigned[1] = composerAssignment{src: 2, origin: src.origin, fingerprint: src.fingerprint, fpPresent: true, xpub: src.xpub}
	a2, err := composerSeedDerive(st, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.assigned[2] = a2
	return st
}

// ─── I-3: the consent must name k-of-n of a locked or hashed multi ──────────

// TestFableRedConsentNamesThresholdOfLockedMulti is lens-1 I-3.
//
// §7e: the consent "MUST name, per path in listed order: its k-of-n or single
// key". md.PolicyShape set Branch.K/N only for a PLAIN threshold, so every
// multi-key path carrying a lock or a hash printed `N key(s), custom` -- and
// composerSelfCheck then compared the key COUNT alone, so a mis-tapped k was
// invisible on both the promise and the check.
func TestFableRedConsentNamesThresholdOfLockedMulti(t *testing.T) {
	for _, tc := range []struct {
		what  string
		paths []md.SpendPath
		want  []string
	}{
		{
			"tiered-recovery: [2-of-2], [1-of-2 + older(5)]",
			[]md.SpendPath{
				{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
				{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}},
			[]string{"Path 1: 2-of-2", "Path 2: 1-of-2"},
		},
		{
			"9-of-9 and 1-of-9 behind locks print identically at tip",
			[]md.SpendPath{
				{Keys: &md.KeySet{K: 5, N: 9, Sorted: true}},
				{Keys: &md.KeySet{K: 9, N: 9, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}},
				{Keys: &md.KeySet{K: 1, N: 9, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 6}}},
			[]string{"Path 1: 5-of-9", "Path 2: 9-of-9", "Path 3: 1-of-9"},
		},
		{
			"a hashed multi: [2-of-3 + sha256 + older(5)], [1 key]",
			[]md.SpendPath{
				{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}, Hash: fableFundsHash(t, fableFundsPreimage), Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}},
				{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}}},
			[]string{"Path 1: 2-of-3"},
		},
	} {
		list := md.PathList{Wrapper: md.ComposeWsh, Paths: tc.paths}
		c, err := md.Compose(list)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		chunks, err := c.Chunks()
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		listed, kp := composerListedPaths(list)
		lines, err := composerConsentLinesFor(chunks, listed, kp)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		joined := strings.Join(lines, "\n")
		for _, want := range tc.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: the consent does not name %q:\n%s", tc.what, want, joined)
			}
		}
		if strings.Contains(joined, "key(s), custom") {
			t.Errorf("%s: the consent still prints the count-only form:\n%s", tc.what, joined)
		}
	}
}

// TestFableSelfCheckComparesTheThresholdOfALockedMulti is I-3's second half:
// the self-check must compare k, not only n, where the path carries a lock.
func TestFableSelfCheckComparesTheThresholdOfALockedMulti(t *testing.T) {
	built := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		{Keys: &md.KeySet{K: 1, N: 2, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}}}
	// The artifact the device would cut if k were mis-lowered on path 2.
	wrong := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}},
		{Keys: &md.KeySet{K: 2, N: 2, Sorted: true}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}}}
	c, err := md.Compose(wrong)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	st := &composerState{list: built, reg: &seedRegistry{}}
	if err := composerSelfCheck(st, chunks); err == nil {
		t.Fatal("composerSelfCheck accepted an artifact whose locked path is 2-of-2 for a built 1-of-2")
	}
}

// ─── L1 I-1 = L3 I-1: one key-less path un-imports the WHOLE wallet ─────────

// TestFableKeylessPathNamesTheImportConsequence is the one finding two lenses
// reached independently.
//
// §8a warned about the preimage and said nothing about import, and the
// consent's `KEY-LESS (EXPERIMENTAL)` row restated no more. Measured by both
// reviewers: Bitcoin Core v25.0 AND v31.1 refuse the descriptor at
// `getdescriptorinfo` ("witnesses without signature exist") so
// `importdescriptors` is never reached and no checksum is issued; libnunchuk
// 2.1.1 refuses; Liana refuses any hashlock path. The KEYED path's owner
// cannot watch or spend through any of them either -- the refusal is of the
// whole descriptor. The Rust primary ADMITS the shape, so nothing upstream
// says it, and funding it is discovered at restore.
func TestFableKeylessPathNamesTheImportConsequence(t *testing.T) {
	body := composerCopyKeylessPath()
	for _, want := range []string{"Bitcoin Core", "Nunchuk", "Liana"} {
		if !strings.Contains(body, want) {
			t.Errorf("the §8a key-less body does not name %s:\n%s", want, body)
		}
	}
	// AND IT IS RESTATED AT CONSENT, the screen §7e calls the promise.
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}},
		{Hash: fableFundsHash(t, fableFundsPreimage), Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}}}
	c, err := md.Compose(list)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	listed, kp := composerListedPaths(list)
	lines, err := composerConsentLinesFor(chunks, listed, kp)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "KEY-LESS (EXPERIMENTAL)") {
		t.Fatalf("INCONCLUSIVE: the fixture's consent has no key-less path:\n%s", joined)
	}
	if !strings.Contains(normalizeDrawn(joined), normalizeDrawn(body)) {
		t.Errorf("the consent does not restate §8a's key-less body:\n%s", joined)
	}
	// AND A WALLET WITH NO KEY-LESS PATH MUST NOT CARRY IT -- a warning shown
	// on every policy is a warning nobody reads.
	plain := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}}}}
	pc, err := md.Compose(plain)
	if err != nil {
		t.Fatal(err)
	}
	pchunks, err := pc.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	plisted, pkp := composerListedPaths(plain)
	plines, err := composerConsentLinesFor(pchunks, plisted, pkp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(normalizeDrawn(strings.Join(plines, "\n")), normalizeDrawn(body)) {
		t.Error("a 2-of-3 with no key-less path carries §8a's key-less body at consent")
	}
}

// ─── L2 I-1: §8f's NUMS note is FALSE for Nunchuk ───────────────────────────

// TestFableNUMSNoteDoesNotPromiseNunchuk is lens-2 I-1.
//
// §8f claimed "Bitcoin Core and Nunchuk import this form". Core: true, 7/7.
// Nunchuk: FALSE, 0 of 7, measured by RUNNING libnunchuk 2.1.1's
// `Utils::ParseWalletDescriptor` -- the function the desktop calls -- over
// every NUMS-keyed shape the composer emits, in both the multipath and the
// single-chain spellings (14/14 refused, code=-1017). Two mechanisms:
// `sortedmulti_a` is not a miniscript fragment for its template validator,
// and every other NUMS shape is stamped DISABLE_KEY_PATH, which re-renders
// the key path as an unspendable XPUB, so the round-trip string check fails.
//
// AND THE NOTE MUST NOT SEND A NUNCHUK USER TO THE UNSPENDABLE-XPUB FORM.
// That form IS the one Nunchuk accepts, and it is a DIFFERENT WALLET:
// measured, `preset-kofn-recovery-tr` derives bc1pm0udr8a... through the xpub
// form and bc1pac935qv... through the raw-H form the device cuts, because the
// xpub form derives per-index children of H and the raw form does not. The
// existing F-449 sentence points LIANA there and that stays; the finding is
// that a Nunchuk user must be sent to wsh, or to a tr policy whose first path
// is a single key, and never there.
func TestFableNUMSNoteDoesNotPromiseNunchuk(t *testing.T) {
	body := composerCopyNUMS()
	norm := normalizeDrawn(body)
	if strings.Contains(norm, normalizeDrawn("Bitcoin Core and Nunchuk import this form")) {
		t.Errorf("§8f still claims Nunchuk imports the NUMS form; libnunchuk 2.1.1 refuses 7 of 7:\n%s", body)
	}
	if !strings.Contains(norm, normalizeDrawn("Nunchuk")) {
		t.Errorf("§8f does not mention Nunchuk at all, so an operator who wants it is told nothing:\n%s", body)
	}
	if !strings.Contains(norm, normalizeDrawn("Bitcoin Core")) {
		t.Errorf("§8f no longer names the wallet that DOES import this form:\n%s", body)
	}
	// The way out must be named, and it must not be the unspendable xpub.
	if !strings.Contains(norm, normalizeDrawn("use wsh")) {
		t.Errorf("§8f does not name the wrapper a Nunchuk operator should choose instead:\n%s", body)
	}
	for _, sent := range []string{"Nunchuk and BIP-388 signers need an unspendable xpub",
		"Nunchuk needs an unspendable xpub"} {
		if strings.Contains(norm, normalizeDrawn(sent)) {
			t.Errorf("§8f sends a Nunchuk user to the unspendable-xpub form, which is a "+
				"DIFFERENT wallet with different addresses:\n%s", body)
		}
	}
	assertModalBodyFits(t, "the §8f NUMS note", errorScreenBody, body)
}

// ─── L2 I-2: a wsh policy mixing lock BASES is refused by Nunchuk ───────────

// TestFableMixedLockBasesUnderWshAreNoticed is lens-2 I-2.
//
// libnunchuk 2.1.1's MiniscriptTimeline walks the WHOLE wsh script and throws
// "Timelock mixing" on the first lock whose base -- TIME vs HEIGHT -- differs
// from any earlier one, regardless of relative/absolute and regardless of
// which `or` branch it sits in. Bitcoin Core imports the same descriptor
// (miniscript's own rule only forbids mixing inside ONE satisfaction), and
// §4c admits both bases with nothing anywhere saying a coordinator will
// refuse the combination.
//
// THE BASES ARE THE AXIS, NOT relative-vs-absolute: older(blocks) and
// after(height) are both HEIGHT, which is why preset-decaying-multisig-wsh
// mixes them and imports. older(units) and after(time) are both TIME.
//
// Under tr the tapscript route validates each leaf separately and a composer
// path carries at most one lock, so no leaf can mix and the notice must not
// fire there.
func TestFableMixedLockBasesUnderWshAreNoticed(t *testing.T) {
	single := func() md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}} }
	locked := func(k md.LockKind, v uint32) md.SpendPath {
		p := single()
		p.Lock = &md.Lock{Kind: k, Value: v}
		return p
	}
	body := composerCopyMixedLockBases()
	for _, tc := range []struct {
		what   string
		list   md.PathList
		notice bool
	}{
		{"wsh: [1 key], [older 100 units], [after height 1000000] -- the lens's own case",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				single(), locked(md.LockOlderUnits, 100), locked(md.LockAfterHeight, 1_000_000)}}, true},
		{"wsh: [1 key], [after time 1893456000], [older 5 blocks]",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				single(), locked(md.LockAfterTime, 1_893_456_000), locked(md.LockOlderBlocks, 5)}}, true},
		{"wsh: decaying-multisig's shape -- older(blocks) with after(height), both HEIGHT",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				single(), locked(md.LockOlderBlocks, 5), locked(md.LockAfterHeight, 150)}}, false},
		{"wsh: older(units) with after(time), both TIME",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				single(), locked(md.LockOlderUnits, 169), locked(md.LockAfterTime, 1_893_456_000)}}, false},
		{"wsh: no locks at all",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}}}}, false},
		{"tr: the same mixed bases -- each leaf validates separately",
			md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
				single(), locked(md.LockOlderUnits, 100), locked(md.LockAfterHeight, 1_000_000)}}, false},
	} {
		c, err := md.Compose(tc.list)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		chunks, err := c.Chunks()
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		listed, kp := composerListedPaths(tc.list)
		lines, err := composerConsentLinesFor(chunks, listed, kp)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		got := strings.Contains(normalizeDrawn(strings.Join(lines, "\n")), normalizeDrawn(body))
		if got != tc.notice {
			t.Errorf("%s: mixed-lock-bases notice shown=%v, want %v:\n%s",
				tc.what, got, tc.notice, strings.Join(lines, "\n"))
		}
	}
	assertModalBodyFits(t, "the mixed-lock-bases notice", errorScreenBody, body)
}

// ─── L3 I-3: the restore document must not deny addresses the consent showed ─

// TestFableRestoreDocDerivesEveryShapeTheConsentDerives is lens-3 I-3.
//
// composerRestoreDoc hands the keyed policy to multisigRestoreDocFlow, whose
// multisigRestoreLines knew only the flat expandedToDescriptor route and
// printed "Addresses unavailable for this policy shape." for everything else
// -- 13 of the reviewer's 22 keyed shapes. The consent screen one screen
// earlier had derived and displayed four addresses for each of them, through
// policyAddressAt's complex route. The document states as fact something the
// same device disproved a screen ago, and a restorer reading it in five years
// is told not to try.
//
// THE CHECK IS EQUALITY WITH THE CONSENT, not merely "an address appears":
// two routes that both produce an address and disagree about WHICH is the
// worse failure, and it is the one a "does it say unavailable" assertion
// cannot see.
func TestFableRestoreDocDerivesEveryShapeTheConsentDerives(t *testing.T) {
	single := func() md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}} }
	kofn := func(k, n uint8) md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}} }
	older := func(n uint32) *md.Lock { return &md.Lock{Kind: md.LockOlderBlocks, Value: n} }
	locked := func(p md.SpendPath, l *md.Lock) md.SpendPath { p.Lock = l; return p }
	for _, tc := range []struct {
		what string
		list md.PathList
	}{
		{"flat sortedmulti (the shape that already worked)",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{kofn(2, 3)}}},
		{"or_d: 2-of-3 head, single-key recovery behind older(26280)",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{kofn(2, 3), locked(single(), older(26280))}}},
		{"or_i: single head, single behind a lock",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{single(), locked(single(), older(5))}}},
		{"three paths, two locks",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				kofn(2, 2), locked(single(), older(5)), locked(single(), older(6))}}},
		{"tiered recovery: 2-of-2 then 1-of-2 behind a lock",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				kofn(2, 2), locked(kofn(1, 2), older(5))}}},
		{"hashlock-gated",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}, Hash: fableFundsHash(t, fableFundsPreimage)},
				locked(single(), older(5))}}},
		{"tr with an extracted internal key",
			md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{single(), locked(kofn(2, 3), older(5))}}},
		{"tr NUMS, three leaves",
			md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
				kofn(2, 2), locked(single(), older(5)), locked(single(), older(6))}}},
	} {
		chunks := fableKeyedBuild(t, tc.list)
		tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		at, consentOK := policyAddressAt(chunks, tpl, keys)
		if !consentOK {
			t.Fatalf("INCONCLUSIVE: %s -- the consent route derives no address either", tc.what)
		}
		wantRecv, err := at(0, false)
		if err != nil {
			t.Fatalf("%s: consent receive 0: %v", tc.what, err)
		}
		wantChange, err := at(0, true)
		if err != nil {
			t.Fatalf("%s: consent change 0: %v", tc.what, err)
		}
		lines, hasAddr, err := multisigRestoreLines(chunks, tpl, keys)
		if err != nil {
			t.Fatalf("%s: multisigRestoreLines: %v", tc.what, err)
		}
		joined := strings.Join(lines, "\n")
		if !hasAddr {
			t.Errorf("%s: the restore document says it has no address for a shape the consent "+
				"screen just derived %s for:\n%s", tc.what, wantRecv, joined)
			continue
		}
		if !strings.Contains(joined, wantRecv) {
			t.Errorf("%s: the document's first receive is not the consent's %s:\n%s",
				tc.what, wantRecv, joined)
		}
		if !strings.Contains(joined, wantChange) {
			t.Errorf("%s: the document's first change is not the consent's %s:\n%s",
				tc.what, wantChange, joined)
		}
	}
}

// fableKeyedBuild seats every slot of `list` from the two demo seeds, in
// rotation, through the PRODUCTION §4f account rule (composerSeedDerive), and
// returns the keyed md1 chunks the device would cut.
//
// It is not composerHonestBuildFor: that one builds a TEMPLATE (declared
// origins, no xpubs), and a template has no addresses on either route, so a
// test about addresses built on it is INCONCLUSIVE by construction.
func fableKeyedBuild(t *testing.T, list md.PathList) []string {
	t.Helper()
	st := &composerState{list: list, reg: &seedRegistry{}}
	for i := range fableFundsSeeds {
		m, err := bip39.ParseMnemonic(fableFundsSeeds[i])
		if err != nil {
			t.Fatal(err)
		}
		id, err := st.reg.add(fmt.Sprintf("seed %d", i+1), m, "", &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		seed, _ := st.reg.at(id)
		var fp [4]byte
		binary.BigEndian.PutUint32(fp[:], seed.MasterFP)
		st.sources = append(st.sources, composerSource{kind: composerSourceSeed,
			label: fmt.Sprintf("seed %d", i+1), fingerprint: fp, fpPresent: true, seedID: id})
	}
	composerSizeAssignments(st)
	for i := range st.assigned {
		a, err := composerSeedDerive(st, uint8(i), i%len(fableFundsSeeds))
		if err != nil {
			t.Fatalf("composerSeedDerive(@%d): %v", i, err)
		}
		st.assigned[i] = a
	}
	_, keyed, err := composerArtifactsFor(st)
	if err != nil {
		t.Fatalf("composerArtifactsFor: %v", err)
	}
	if len(keyed) == 0 {
		t.Fatal("INCONCLUSIVE: the build produced no keyed chunks")
	}
	return keyed
}

// ─── r1 review I-1 and M-2: the creation-time key-less guard ────────────────

// TestFableKeylessCreationGuardYieldsToTheStructuralRefusals is the
// operator-visible half of round-1 review I-1, plus M-2.
//
// composerAddPath refuses a SECOND key-less path at creation, which is §4e's
// "REFUSE at the picker" half. But the cap's remedy -- "fold them into one
// path" -- does not cure the structural refusals the codec now orders AHEAD
// of it, so the guard must not claim it where it cannot work:
//
//   - under sh / sh-wsh the blocker is the one-sorted-multisig rule, and
//     folding leaves two paths, still refused. The body whose remedy DOES
//     work is one arm away.
//   - the emptied path of M-5 (no key, no hash) is not a key-less path at
//     all: the primary's own vector says key-less is `keys == null` AND
//     `hash != null`, and the codec refuses an empty path earlier with
//     LockOnlyPathError. Counting it told the operator they already hold a
//     key-less path when they hold an empty one.
func TestFableKeylessCreationGuardYieldsToTheStructuralRefusals(t *testing.T) {
	H := fableFundsHash(t, fableFundsPreimage)
	keyed := func(k, n uint8) md.SpendPath {
		return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}}
	}
	// M-2: the COUNT itself. An empty path is not a key-less path.
	for _, tc := range []struct {
		what  string
		paths []md.SpendPath
		want  int
	}{
		{"a real key-less path", []md.SpendPath{keyed(2, 3), {Hash: H}}, 1},
		{"an EMPTY path (no key, no hash) -- M-5's shape",
			[]md.SpendPath{keyed(2, 3), {}}, 0},
		{"an empty path carrying only a time lock",
			[]md.SpendPath{keyed(2, 3), {Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}}, 0},
		{"a key-less path with a lock", []md.SpendPath{keyed(2, 3),
			{Hash: H, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 5}}}, 1},
	} {
		list := md.PathList{Wrapper: md.ComposeWsh, Paths: tc.paths}
		if got := composerKeylessPathCount(list, -1); got != tc.want {
			t.Errorf("%s: composerKeylessPathCount = %d, want %d", tc.what, got, tc.want)
		}
	}
	// I-1: the BODY the creation guard shows, by wrapper.
	for _, tc := range []struct {
		what    string
		wrapper md.ComposeWrapper
		want    string
	}{
		{"wsh -- the cap is the blocker and folding cures it",
			md.ComposeWsh, composerCopyRefuseTwoKeylessPaths()},
		{"sh -- the blocker is the one-sorted-multisig rule; folding leaves two paths",
			md.ComposeSh, composerCopyRefuseLegacyShape()},
		{"sh-wsh -- the same",
			md.ComposeShWsh, composerCopyRefuseLegacyShape()},
	} {
		list := md.PathList{Wrapper: tc.wrapper, Paths: []md.SpendPath{
			keyed(2, 3), {Hash: H}, {Hash: H}}}
		body, ok := composerAddPathKeylessRefusal(list, 2)
		if !ok {
			t.Errorf("%s: the creation guard admits a second key-less path", tc.what)
			continue
		}
		if body != tc.want {
			t.Errorf("%s: the creation guard shows %q, want %q", tc.what, body, tc.want)
		}
	}
	// And a FIRST key-less path under wsh is not refused at creation at all.
	first := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{keyed(2, 3), {Hash: H}}}
	if body, ok := composerAddPathKeylessRefusal(first, 1); ok {
		t.Errorf("the creation guard refused the FIRST key-less path with %q", body)
	}
}

// TestFableValidatePathListOrdersTheCapLast is round-1 review I-1 at the
// codec, stated as the four orderings rather than read off the vector -- so
// the rule is pinned here too if the vector is ever re-shaped.
func TestFableValidatePathListOrdersTheCapLast(t *testing.T) {
	H := fableFundsHash(t, fableFundsPreimage)
	H2 := fableFundsHash(t, fableFundsPreimage2)
	keyed := func(k, n uint8) md.SpendPath {
		return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}}
	}
	for _, tc := range []struct {
		what string
		list md.PathList
		want error
	}{
		{"tr: the tr rule is the one in force there",
			md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{keyed(2, 3), {Hash: H}, {Hash: H2}}},
			md.ErrComposeKeylessUnderTr},
		{"no keyed path at all (BIP-388 l.191)",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{Hash: H}, {Hash: H2}}},
			md.ErrComposeNoKeyedPath},
		{"36 slots: folding two key-less paths into one sheds no slot",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				keyed(9, 9), keyed(9, 9), keyed(9, 9), keyed(9, 9), {Hash: H}, {Hash: H2}}},
			md.ErrComposeTooManySlots},
		{"sh: folding leaves two paths, still not one sorted multisig",
			md.PathList{Wrapper: md.ComposeSh, Paths: []md.SpendPath{keyed(2, 3), {Hash: H}, {Hash: H2}}},
			md.ErrComposeLegacyWrapperShape},
	} {
		_, err := md.ValidatePathList(tc.list)
		if err == nil {
			t.Errorf("%s: admitted", tc.what)
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: refused with %v, want %v -- the cap's remedy does not cure this",
				tc.what, err, tc.want)
		}
	}
}

// ─── r1 review M-1: the mk1-card door is the other testnet entrance ─────────

// TestFableTestnetCardIsNotOfferedAsASource is round-1 review M-1.
//
// r0's M-6 closed the `key:` record door and left the mk1-card door open.
// mk.Card.Network is "mainnet" | "testnet" by design, mk/encode.go admits the
// testnet public version, and the composer's card door reaches the material
// through decodeXpubBytes -- which parses and never asks the network. So a
// testnet card was offered in the seating pick-list, the slot bound its
// chain-code||point, and the consent derived mainnet bc1q... for material
// declared under coin type 1'. The door count does not flag it either: an mk1
// card is ClassMDMK, so it never reaches composerCopyNotUnderstood.
//
// §4f is "mainnet-only by construction" at BOTH entrances or at neither.
func TestFableTestnetCardIsNotOfferedAsASource(t *testing.T) {
	m, err := bip39.ParseMnemonic(fableFundsSeeds[0])
	if err != nil {
		t.Fatal(err)
	}
	const hard = 0x80000000
	tpub, tfp, err := deriveAccountXpub(m, "", &chaincfg.TestNet3Params,
		[]uint32{48 | hard, 1 | hard, 0 | hard, 2 | hard})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tpub, "tpub") {
		t.Fatalf("derived %s, not a tpub", tpub[:8])
	}
	xpub, xfp, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams,
		[]uint32{48 | hard, 0 | hard, 0 | hard, 2 | hard})
	if err != nil {
		t.Fatal(err)
	}
	card := func(net, path, xp string, fp uint32) []string {
		t.Helper()
		strs, err := mk.Encode(mk.Card{Network: net, Path: path,
			Fingerprint: fmt.Sprintf("%08x", fp), Xpub: xp, Stubs: [][4]byte{{1, 2, 3, 4}}})
		if err != nil {
			t.Fatalf("mk.Encode(%s): %v", net, err)
		}
		return strs
	}
	testnet := card("testnet", "m/48'/1'/0'/2'", tpub, tfp)
	mainnet := card("mainnet", "m/48'/0'/0'/2'", xpub, xfp)

	// THE CONTROL FIRST, so a door that offered nothing at all would fail.
	ctx := NewContext(newPlatform())
	ctx.sysw = composerSessionWith(mainnet, nil)
	if got := composerCardSources(ctx); len(got) != 1 {
		t.Fatalf("INCONCLUSIVE: the mainnet control yields %d card sources, want 1", len(got))
	}
	ctx = NewContext(newPlatform())
	ctx.sysw = composerSessionWith(testnet, nil)
	if got := composerCardSources(ctx); len(got) != 0 {
		t.Errorf("the composer offers a testnet mk1 card as a source (%d): label %q xpub %s... "+
			"-- the consent would derive mainnet bc1q addresses for material declared under "+
			"coin type 1h, the same sentence sysw.ParseKeyRecord now prevents on the other door",
			len(got), got[0].label, got[0].xpub[:12])
	}
	// BOTH IN ONE PAYLOAD: the testnet card must not take the mainnet one
	// down with it.
	ctx = NewContext(newPlatform())
	ctx.sysw = composerSessionWith(append(append([]string{}, testnet...), mainnet...), nil)
	got := composerCardSources(ctx)
	if len(got) != 1 {
		t.Fatalf("a payload with one testnet and one mainnet card yields %d sources, want 1", len(got))
	}
	if got[0].xpub != xpub {
		t.Errorf("the surviving source is %s..., want the mainnet card %s...",
			got[0].xpub[:12], xpub[:12])
	}
}

// ─── L5 I-1/I-2: one consent line naming why a policy is outside Liana's model ──

// TestFableOutsideLianaModelNamesTheFirstClass is lens-5 I-1 (17 of 56
// composable shapes import into Liana 8.0; the device named two of the other
// nine refusal classes before this fold) and I-2 (a second unlocked path
// after a multi-key one is silently absorbed as an extra key, not refused --
// named the same as an outright second-unlocked refusal, because both leave
// the operator with a wallet Liana will not show as built).
//
// NINE CLASSES, ONE PER ROW, IN LIANA'S OWN ORDER OF REFUSAL -- the wrapper
// and the taproot internal key are checked before Liana ever walks the
// policy tree, so they are named ahead of a policy-content reason even when
// a shape is outside the model for more than one reason at once. Row 10 is
// that overlap, pinned directly: the hashlock-gated preset under tr carries
// BOTH a NUMS internal key and a keyed hash path, and the notice must name
// the NUMS class, not the hash one, or a reorder of the two checks would
// pass every other row and still be wrong.
//
// The three negatives are the tr-with-real-key preset (its "unlocked path"
// is the taproot key path itself, not a Branch -- md.KeyPathSpendable) and
// two of the three wsh presets Liana accepts unedited (report runbook §4);
// row 4 covers the third (hashlock-gated fires, not accepted, but exercises
// the same preset table). Together with row 3 (plain-multisig, the demo
// payload's own shape) and row 5 (decaying-multisig), all six shipped
// presets are exercised under at least one wrapper.
func TestFableOutsideLianaModelNamesTheFirstClass(t *testing.T) {
	single := func() md.SpendPath { return md.SpendPath{Keys: &md.KeySet{K: 1, N: 1, Sorted: true}} }
	multiKeys := func(k, n uint8) md.SpendPath {
		return md.SpendPath{Keys: &md.KeySet{K: k, N: n, Sorted: true}}
	}
	lockedAt := func(p md.SpendPath, kind md.LockKind, v uint32) md.SpendPath {
		p.Lock = &md.Lock{Kind: kind, Value: v}
		return p
	}
	preset := func(w md.ComposeWrapper, name string) md.PathList {
		t.Helper()
		for _, p := range composerPresets(w) {
			if p.name == name {
				return p.list
			}
		}
		t.Fatalf("no preset %q under wrapper %v", name, w)
		return md.PathList{}
	}

	for _, tc := range []struct {
		what      string
		list      md.PathList
		wantClass string // "" means the notice must not fire
	}{
		// ─── one row per class, in Liana's own order ───────────────────────
		{"1. sh: plain-multisig -- Liana takes wsh or tr only",
			preset(md.ComposeSh, "plain-multisig"), "legacy wrapper"},
		{"2. tr: plain-multisig -- a bare k-of-n under tr has a NUMS internal key",
			preset(md.ComposeTr, "plain-multisig"), "NUMS key path"},
		{"3. wsh: plain-multisig -- the demo payload's own wallet, no recovery path",
			preset(md.ComposeWsh, "plain-multisig"), "no locked path"},
		{"4. wsh: hashlock-gated -- a keyed hash path, no timelock recognised",
			preset(md.ComposeWsh, "hashlock-gated"), "a hash lock"},
		{"5. wsh: decaying-multisig -- carries after(1000000)",
			preset(md.ComposeWsh, "decaying-multisig"), "an absolute lock"},
		{"6. wsh: [1 key], [1 key, older 100 UNITS]",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				single(), lockedAt(single(), md.LockOlderUnits, 100)}}, "a lock in time units"},
		{"7. wsh: three paths, all locked by older(blocks), all different values",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				lockedAt(multiKeys(2, 2), md.LockOlderBlocks, 100),
				lockedAt(single(), md.LockOlderBlocks, 200),
				lockedAt(single(), md.LockOlderBlocks, 300)}}, "no unlocked path"},
		{"8. wsh: 2-of-3 unlocked, two recovery paths at the SAME older(100)",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				multiKeys(2, 3),
				lockedAt(single(), md.LockOlderBlocks, 100),
				lockedAt(single(), md.LockOlderBlocks, 100)}}, "two paths with one lock"},
		{"9. wsh: 2-of-3 unlocked, 1 key unlocked, 1 key after older(100) -- lens 5 I-2's X24",
			md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
				multiKeys(2, 3),
				single(),
				lockedAt(single(), md.LockOlderBlocks, 100)}}, "a second unlocked path"},

		// ─── order priority: two classes true at once, first must win ─────
		{"10. tr: hashlock-gated -- NUMS AND a keyed hash path both apply; NUMS is first",
			preset(md.ComposeTr, "hashlock-gated"), "NUMS key path"},

		// ─── negatives: Liana imports these unedited (report runbook §4) ──
		{"11. tr: simple-timelocked-inheritance -- the primary IS the key path, not a Branch",
			preset(md.ComposeTr, "simple-timelocked-inheritance"), ""},
		{"12. wsh: kofn-recovery -- one unlocked 2-of-3, one older(26280) recovery",
			preset(md.ComposeWsh, "kofn-recovery"), ""},
		{"13. wsh: simple-timelocked-inheritance -- one unlocked, one older(26280)",
			preset(md.ComposeWsh, "simple-timelocked-inheritance"), ""},
		{"14. wsh: tiered-recovery -- one unlocked 2-of-2, one older(26280) recovery",
			preset(md.ComposeWsh, "tiered-recovery"), ""},
	} {
		c, err := md.Compose(tc.list)
		if err != nil {
			t.Fatalf("%s: compose: %v", tc.what, err)
		}
		chunks, err := c.Chunks()
		if err != nil {
			t.Fatalf("%s: chunks: %v", tc.what, err)
		}
		listed, kp := composerListedPaths(tc.list)
		lines, err := composerConsentLinesFor(chunks, listed, kp)
		if err != nil {
			t.Fatalf("%s: consent: %v", tc.what, err)
		}
		drawn := normalizeDrawn(strings.Join(lines, "\n"))
		if tc.wantClass == "" {
			if strings.Contains(drawn, normalizeDrawn("OUTSIDE LIANA'S MODEL")) {
				t.Errorf("%s: the outside-Liana notice fired, want it silent:\n%s",
					tc.what, strings.Join(lines, "\n"))
			}
			continue
		}
		want := composerCopyOutsideLianaModel(tc.wantClass)
		if !strings.Contains(drawn, normalizeDrawn(want)) {
			t.Errorf("%s: outside-Liana notice for class %q not found:\n%s",
				tc.what, tc.wantClass, strings.Join(lines, "\n"))
		}
	}
	assertModalBodyFits(t, "the outside-Liana-model notice, longest class line",
		errorScreenBody, composerCopyOutsideLianaModel("two paths with one lock"))
}
