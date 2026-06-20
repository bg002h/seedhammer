package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// The 6 chunks of a wsh(sortedmulti(2,3)) md1 with real xpubs + fingerprints
// (generated from md.split of the hand-built descriptor; see the md-package
// generator). They reassemble → expand → expandOK, so a complete set drives the
// gather-completion handler into the descriptor display + address verify.
var wshSortedmultiChunks = []string{
	"md1f9k2szspqjtvyyy4qqxppcgsc97v95zqyudm486mm4xav6hqptc0rd7sr9mfc8yrzcx7sju0ra3jh8llnx",
	"md1f9k2szsguxj4ln63stmuuq6kgrvtxn9uedgysqk5mrsqw5njj30rf8ejcf6w954djz5pse9uf467htrhv9",
	"md1f9k2szskmd6duvfx8px8yg8ygdjvlt5pter2r3mlhkwekkmg35c9z9n9exphw5vzmqsus7s2utcp5k43cp",
	"md1f9k2szsl3agyaty5qzh0rwwq3vq3pj6kuyh77z607jtnsw6g9jypzwhftgrxfsamw42us0cckgptgujwrh",
	"md1f9k2sz3pts25xzd0f9ftv6fzwfhlf03ccs9k5jmk477z5tkyhsytp44jklx3ecnvtf7mslf2jj0tkr9f6c",
	"md1f9k2sz3g5p0kujs0lpqmkaf9xgmzlztk3npxzke82yxuuch2qemkslrh3u95mgqske0kz57em36",
}

// loadChunkedVectorString returns the chunk-format md1 string from a vendored
// vector's .phrase.txt (the LAST non-empty, non-header line). wsh_multi_chunked
// is a single-chunk chunked-format string (unsorted multi, no pubkeys).
func loadChunkedVectorString(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "vectors", name+".phrase.txt"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var last string
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "chunk-set-id:") {
			continue
		}
		last = ln
	}
	if last == "" {
		t.Fatalf("%s: no md1 string", name)
	}
	return last
}

// TestMD1Gatherer: offer primes/dup/foreign/added exactly like mk1Gatherer.
func TestMD1Gatherer(t *testing.T) {
	g := &md1Gatherer{}
	// Prime + add first chunk.
	if st := g.offer(wshSortedmultiChunks[0]); st != gatherAdded {
		t.Fatalf("offer c0: status %v", st)
	}
	if g.complete() {
		t.Fatal("complete after 1 of 6")
	}
	// Duplicate.
	if st := g.offer(wshSortedmultiChunks[0]); st != gatherDup {
		t.Fatalf("offer dup: status %v", st)
	}
	// A foreign chunk: wsh_multi_chunked has a DIFFERENT csid/total.
	foreign := loadChunkedVectorString(t, "wsh_multi_chunked")
	if st := g.offer(foreign); st != gatherForeign {
		t.Fatalf("offer foreign: status %v", st)
	}
	// Garbage / non-chunk.
	if st := g.offer("not an md1 chunk"); st != gatherIgnored {
		t.Fatalf("offer garbage: status %v", st)
	}
	// A single (non-chunked) md1 string: ParseChunkHeader returns Chunked=false →
	// foreign once primed.
	single := loadChunkedVectorString(t, "wpkh_basic")
	if st := g.offer(single); st != gatherForeign {
		t.Fatalf("offer single md1: status %v", st)
	}
	// Add the rest.
	for i := 1; i < len(wshSortedmultiChunks); i++ {
		if st := g.offer(wshSortedmultiChunks[i]); st != gatherAdded {
			t.Fatalf("offer c%d: status %v", i, st)
		}
	}
	if !g.complete() {
		t.Fatalf("not complete after %d chunks", len(wshSortedmultiChunks))
	}
	if len(g.collected()) != len(wshSortedmultiChunks) {
		t.Fatalf("collected %d, want %d", len(g.collected()), len(wshSortedmultiChunks))
	}
}

// TestMD1GathererPrimeFromFirst: priming from an out-of-order chunk still
// completes when all chunks arrive.
func TestMD1GathererPrimeFromFirst(t *testing.T) {
	g := &md1Gatherer{}
	g.offer(wshSortedmultiChunks[3]) // out-of-order prime
	for i, c := range wshSortedmultiChunks {
		if i == 3 {
			continue
		}
		g.offer(c)
	}
	if !g.complete() {
		t.Fatal("not complete after all chunks (out-of-order prime)")
	}
}

// TestGatheredDescriptorFlowExpandOK: a complete wsh(sortedmulti) set with real
// xpubs → the completion handler reassembles, expands, builds the descriptor,
// and enters the descriptor display (offering address verification).
func TestGatheredDescriptorFlowExpandOK(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { gatheredDescriptorFlow(ctx, &descriptorTheme, wshSortedmultiChunks) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("no frame from gatheredDescriptorFlow (expandOK)")
	}
	// DescriptorScreen shows the descriptor; it must NOT be the csid-mismatch or
	// generic decode error, and not the "complex policy" refusal.
	if uiContains(content, "tampered") || uiContains(content, "can't decode") || uiContains(content, "complex policy") {
		t.Errorf("expandOK set showed an error/refusal frame; got %q", content)
	}
}

// TestGatheredDescriptorFlowUnsupported: a complete chunked unsorted-multi set
// (wsh_multi_chunked, no pubkeys) → routes to the display-only path, never a
// descriptor/verify. (It has no pubkeys, so template-only; either way no
// verify, and no crash.)
func TestGatheredDescriptorFlowUnsupported(t *testing.T) {
	ctx := NewContext(newPlatform())
	chunk := loadChunkedVectorString(t, "wsh_multi_chunked")
	frame, quit := runUI(ctx, func() { gatheredDescriptorFlow(ctx, &descriptorTheme, []string{chunk}) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("no frame from gatheredDescriptorFlow (unsupported/template-only)")
	}
}

// tamperedCSIDChunks is the same 6-chunk wsh(sortedmulti) set but stamped with a
// consistent-but-WRONG chunk-set-id (header csid 0xce33a vs the real 0x2d950).
// The exact value is irrelevant — only that it is internally consistent across
// all chunks yet differs from the csid re-derived from the descriptor. It passes
// per-chunk BCH and the version/csid/count consistency check, so reassembly
// reaches the integrity gate where the re-derived csid won't match →
// ErrChunkSetIDMismatch.
var tamperedCSIDChunks = []string{
	"md1fece6zspqjtvyyy4qqxppcgsc27rcm05qew6wpeqckph5wrf2leagc9a7wqdtypsc5kzzkknukj7h",
	"md1fece6zsw9nfj7vk5zgqt2d3cq82fefgh35nuevya8z62kep2q7md6duvfx8px8ygr8gkvum08ttrf",
	"md1fece6zss8ygdjvlt5pter2r3mlhkwekkmg35c9z9n9exphw5vzmqsulr6sf6kfgqtwlt5p83kxpst",
	"md1fece6zsc9w7xuupzcpzr94dcf0au95layh8qa5stygzyawjksxvnpmka24e9wp2s8z4s6jvc5vtw0",
	"md1fece6z3qcf4ay49dnfyfexla978rzqk6jtw6hmc23wcj7q3vxkk2mu688zd3d8mggj5z6yp2j5qas",
	"md1fece6z3w9qtahy5rlcgxah2ffjxchcja5vcfs4kf63ph8x96sxwa58cau0pdx6wxuyffd5pmezz",
}

// TestGatheredDescriptorFlowCSIDMismatch: a tampered (consistent-wrong-csid) set
// surfaces a DISTINCT error message (R0-C1 via errors.Is ErrChunkSetIDMismatch),
// not the generic decode failure.
func TestGatheredDescriptorFlowCSIDMismatch(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { gatheredDescriptorFlow(ctx, &descriptorTheme, tamperedCSIDChunks) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("no frame from gatheredDescriptorFlow (csid mismatch)")
	}
	if !uiContains(content, "match") && !uiContains(content, "tampered") {
		t.Errorf("csid-mismatch set must show a distinct 'chunks don't match' message; got %q", content)
	}
}

// T-H2 (verify-cluster H2): collected() must return chunks in ChunkIndex order
// regardless of arrival order. The gatherer keys by parsed ChunkIndex, so we
// vary ARRIVAL order; the canonical wshSortedmultiChunks slice IS index-ordered.
// Proven on 3a23dbb: collected() ranges the Go map (random order) → non-index
// order on 10/10 shuffled trials (FALSE-FAIL at the positional comparator).
// After the index-walk fix it is index-ordered deterministically every time.
func TestMD1GathererCollectedIndexOrder(t *testing.T) {
	orders := [][]int{
		{5, 0, 3, 1, 4, 2},
		{2, 1, 0, 5, 4, 3},
		{0, 1, 2, 3, 4, 5},
		{3, 5, 1, 0, 2, 4},
	}
	for _, order := range orders {
		// Repeat to defeat Go's randomized map iteration (a single run could
		// coincidentally agree; 10 runs makes a map-order regression observable).
		for trial := 0; trial < 10; trial++ {
			g := &md1Gatherer{}
			for _, i := range order {
				if st := g.offer(wshSortedmultiChunks[i]); st != gatherAdded {
					t.Fatalf("order %v: offer chunk %d status %v", order, i, st)
				}
			}
			if !g.complete() {
				t.Fatalf("order %v: not complete", order)
			}
			got := g.collected()
			if len(got) != len(wshSortedmultiChunks) {
				t.Fatalf("order %v: collected len %d, want %d", order, len(got), len(wshSortedmultiChunks))
			}
			for i := range wshSortedmultiChunks {
				if got[i] != wshSortedmultiChunks[i] {
					t.Fatalf("order %v trial %d: collected()[%d]=%q, want index order %q",
						order, trial, i, got[i], wshSortedmultiChunks[i])
				}
			}
		}
	}
}

// TestMD1GathererShuffledGatherExpands (end-to-end flavour): a complete
// multi-chunk md1 gathered in shuffled order must reassemble + expand the SAME
// descriptor as the canonical index-ordered set (collected() → the production
// gather-completion consumer), confirming the ordering fix reaches the real
// gather→consume path, not just collected() in isolation.
func TestMD1GathererShuffledGatherExpands(t *testing.T) {
	g := &md1Gatherer{}
	for _, i := range []int{5, 0, 3, 1, 4, 2} {
		g.offer(wshSortedmultiChunks[i])
	}
	if !g.complete() {
		t.Fatal("not complete after shuffled gather")
	}
	tpl, keys, err := md.ExpandWalletPolicyChunks(g.collected())
	if err != nil {
		t.Fatalf("expand shuffled-gather collected(): %v", err)
	}
	tplC, keysC, err := md.ExpandWalletPolicyChunks(wshSortedmultiChunks)
	if err != nil {
		t.Fatalf("expand canonical: %v", err)
	}
	if tpl.Root != tplC.Root || tpl.Policy != tplC.Policy || tpl.K != tplC.K || tpl.N != tplC.N {
		t.Fatalf("shuffled-gather template %v/%v/%d-of-%d != canonical %v/%v/%d-of-%d",
			tpl.Root, tpl.Policy, tpl.K, tpl.N, tplC.Root, tplC.Policy, tplC.K, tplC.N)
	}
	if len(keys) != len(keysC) {
		t.Fatalf("shuffled-gather %d keys != canonical %d", len(keys), len(keysC))
	}
}
