package main

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

// Bug 52: when a chunked md1 set that fails to expand is exactly ONE string
// long, probe() falls back to md.Decode, which refuses any chunked string with
// ErrChunkedUnsupported, and reports that sentence in place of the real expand
// error. The code's own comment says a chunk set that lost a chunk must stay an
// expand failure whose error is the chunk-set one.
func TestBughuntSh252OneChunkOfSetKeepsRealError(t *testing.T) {
	full := vectorChunks(t, "keyed_compose_wsh_timelock_hashlock")
	if len(full) < 3 {
		t.Fatalf("fixture has %d chunks, need a real set", len(full))
	}
	// Positive control: with two of the chunks the real error is kept.
	_, _, realErr2 := md.ExpandWalletPolicyChunks(full[:2])
	if realErr2 == nil {
		t.Fatal("control: two of the chunks expanded")
	}
	res := probe(caseIn{ID: "t", Chunks: full[:2], Indices: []uint32{0}})
	if res.Stage != stageExpand || res.Error != realErr2.Error() {
		t.Fatalf("control (2 chunks): stage %q error %q, want expand / %q", res.Stage, res.Error, realErr2)
	}

	// The bug: one chunk of the same set.
	_, _, realErr := md.ExpandWalletPolicyChunks(full[:1])
	if realErr == nil {
		t.Fatal("one chunk of a three-chunk set expanded")
	}
	res = probe(caseIn{ID: "t", Chunks: full[:1], Indices: []uint32{0}})
	if res.OK || res.Stage != stageExpand {
		t.Fatalf("stage %q ok %v, want a failed expand", res.Stage, res.OK)
	}
	if res.Error != realErr.Error() {
		t.Errorf("probe reported %q, the real expand error is %q", res.Error, realErr)
	}
	if strings.Contains(res.Error, md.ErrChunkedUnsupported.Error()) {
		t.Errorf("probe replaced the real error with %q", md.ErrChunkedUnsupported)
	}
}
