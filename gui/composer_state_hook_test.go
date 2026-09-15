package gui

import (
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
)

// ─── H5 §4 (F-485): the composition-state seam ──────────────────────────────
//
// cmd/emu's walk is the gate this seam exists for, and a walk cannot run in CI.
// So the properties it depends on are gated HERE: the hook is installed while a
// composition runs and nil otherwise, it reports p.Hash per path, and what it
// hands back cannot be written through.

// TestComposerStateHookIsInstalledOnlyWhileAFlowRuns is the lifetime property.
//
// A hook left installed after composerFlow returns would answer with the LAST
// composition's digests, and a walk asserting "path 1 holds no hash yet" would
// pass on a previous run's cleared state -- the stale-answer failure that is
// worse than no answer at all.
//
// MUTATION: delete `defer clearComposerStateHook()` from composerFlow -> the
// after-the-flow assertion fails.
// MUTATION: delete `setComposerStateHook(st)` from composerFlow -> the
// during-the-flow assertion fails (ComposerPathHashes returns nil).
func TestComposerStateHookIsInstalledOnlyWhileAFlowRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if got := ComposerPathHashes(); got != nil {
			t.Fatalf("the hook is installed before any composition ran: %v", got)
		}
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith(nil, nil)

		done := false
		frame, quit := runUI(ctx, func() {
			composerFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		if got, ok := pumpUntil(frame, "Which script?", 24); !ok {
			t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
		}
		// Inside the flow: a composition with no paths yet is an EMPTY slice,
		// which is not the same answer as "no composition is running".
		hashes := ComposerPathHashes()
		if hashes == nil {
			t.Fatal("the hook is not installed while composerFlow is running")
		}
		if len(hashes) != 0 {
			t.Fatalf("a composition with no paths reports %d hash(es)", len(hashes))
		}

		// Back out of the wrapper picker: composerFlow returns and the deferred
		// clear runs.
		click(&ctx.Router, Button1)
		for i := 0; i < 64 && !done; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if !done {
			t.Fatal("composerFlow never returned, so the clear was never reached")
		}
		if got := ComposerPathHashes(); got != nil {
			t.Fatalf("the hook survived the composition it was installed for: %v", got)
		}
	})
}

// TestComposerStateHookReportsEachPathAndHandsOutCopies is the read contract.
//
// The copy half is the one that matters: the consumer is JavaScript on a page,
// and a *[32]byte into an md.SpendPath would let a walk WRITE the policy it
// exists to observe -- a reading seam that can drive is not a reading seam.
//
// MUTATION: return st's own pointers (`out[i] = p.Hash`) -> the write-through
// assertion fails.
// MUTATION: report only the paths that carry a hash (skip the nil entries
// instead of leaving a hole) -> the index alignment assertion fails, and the
// walk's "path 0 holds nothing yet" read would silently become "some path".
func TestComposerStateHookReportsEachPathAndHandsOutCopies(t *testing.T) {
	var raw [32]byte
	for i := range raw {
		raw[i] = byte(i)
	}
	d := composerTestLock(raw)
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{}, {Hash: d},
	}}}
	setComposerStateHook(st)
	t.Cleanup(clearComposerStateHook)

	got := ComposerPathHashes()
	if len(got) != 2 {
		t.Fatalf("the hook reports %d entries for a 2-path composition", len(got))
	}
	if got[0] != nil {
		t.Errorf("path 1 carries no hash and the hook reports %s", hashlockHashHex(got[0]))
	}
	if !got[1].Equal(d) {
		t.Fatalf("path 2's hash is %v, want %s", got[1], hashlockHashHex(d))
	}
	// THE WRITE-THROUGH IS NOW A COMPILE ERROR RATHER THAN AN ASSERTION, and
	// that is the stronger form of the same guarantee. This block used to do
	// `got[1][0] ^= 0xff` and check the policy had not moved; a digest now
	// lives in md.HashLock's unexported field, with no setter, so no caller
	// outside package md can write one at all and that statement does not
	// build. What the named mutation (`out[i] = p.Hash`) actually changes is
	// the pointer IDENTITY, and that is still observable here.
	//
	// MUTATION: `out[i] = p.Hash` in setComposerStateHook -> this fails.
	if got[1] == st.list.Paths[1].Hash {
		t.Error("the hook handed out the POLICY's own pointer rather than a copy of the lock")
	}
	if !st.list.Paths[1].Hash.Equal(d) {
		t.Errorf("the policy's digest moved: %s, want %s",
			hashlockHashHex(st.list.Paths[1].Hash), hashlockHashHex(d))
	}
}
