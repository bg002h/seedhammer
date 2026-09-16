package gui

import (
	"encoding/hex"
	"strings"
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

// TestComposerStateHookHandsOutNoPadding is SPEC_hashlock_kinds §7.3's gate,
// and §7.3 exists because without it the walk's central assertion COULD NOT
// FAIL.
//
// The seam used to hand out *[32]byte -- the stored array. A 20-byte kind keeps
// its digest in the first 20 bytes of that array and twelve alloc-gate zeros
// after it, so hex.EncodeToString over the array returns 64 characters for a
// ripemd160 lock: 40 real and 24 of padding. A walk asserting "path 1 holds
// digest D" would then pass identically for sha256(D) and for any 20-byte kind
// sharing D's first 20 bytes, because both sides of the comparison were drawn
// from the same padded source.
//
// So this asserts the WIDTH and the ABSENCE OF THE PADDING, not just equality:
// an Equal check passes even while the hex a walk reads is wrong, since Equal
// slices to the kind's width and the emulator bridge is a different call.
//
// MUTATION: make md.HashLock.Digest return the whole array (`h.digest[:]`) ->
// the width assertion fails at 64 != 40 and the suffix assertion names the
// padding it found (measured: ...b0b1b2b3 followed by 24 zeros).
//
// AND THE ABBREVIATION ASSERTION AT THE BOTTOM STAYS GREEN UNDER THAT MUTATION.
// That is not a flaw in it -- it is this test's whole subject, demonstrated on
// itself. Both sides of that comparison read through Digest, so padding moves
// them together and the check cannot fail. Only the WIDTH and the SUFFIX are
// independent of the mutated source, which is why they are asserted separately
// rather than folded into one "the abbreviation matches" line.
func TestComposerStateHookHandsOutNoPadding(t *testing.T) {
	// A ripemd160 digest whose real bytes END in a non-zero, so padding is
	// distinguishable from a digest that merely happens to trail off.
	var d20 [20]byte
	for i := range d20 {
		d20[i] = byte(0xa0 + i)
	}
	lock, ok := md.NewHashLock(md.KindRipemd160, d20[:])
	if !ok {
		t.Fatal("20 bytes is ripemd160's width")
	}
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Hash: lock},
	}}}
	setComposerStateHook(st)
	t.Cleanup(clearComposerStateHook)

	got := ComposerPathHashes()
	if len(got) != 1 {
		t.Fatalf("the hook reports %d entries for a 1-path composition", len(got))
	}
	// This is the exact expression cmd/emu/composer_js.go puts in the entry's
	// `digest` field. The bridge reports {kind, digest} since §12 item 2; the
	// digest half is still built exactly here.
	h := hex.EncodeToString(got[0].Digest())
	if len(h) != 40 {
		t.Errorf("the seam reports %d hex characters for a ripemd160 lock, want 40. "+
			"A walk comparing this against a displayed abbreviation is comparing "+
			"two views of the same padded array:\n%s", len(h), h)
	}
	if strings.HasSuffix(h, "00") {
		t.Errorf("the seam's hex ends in alloc-gate padding, so it is the stored "+
			"ARRAY and not the digest:\n%s", h)
	}
	if want := hex.EncodeToString(d20[:]); h != want {
		t.Errorf("the seam reports %s, want %s", h, want)
	}
	// And the abbreviation the walk builds from it is the one the screen draws.
	// short8 in walk_hashlock_phrase.js slices [0:8] and [-8:]; the Go side must
	// agree at a width that is not 64, or the two disagree only for 20-byte
	// kinds -- the case no sha256 fixture can reach.
	if tok, want := hashlockFirst8Last8(got[0]), h[:8]+".."+h[len(h)-8:]; tok != want {
		t.Errorf("the drawn abbreviation %q is not what the walk's short8 builds "+
			"from the seam's hex (%q)", tok, want)
	}
}
