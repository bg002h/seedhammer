package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// ─── H5 §2 (F-480): provenance is per DIGEST, not one flag per policy ────────
//
// composerState.hashByPhrase was one bool for a whole composition: set at HOLD,
// cleared only when NO path carried a hash at all. Replace a phrase-set hash on
// path 1 with a payload row while path 2 keeps a hex hash and Done still named a
// phrase the composition no longer has -- the operator is told to back up an
// artifact that does not exist, at the one screen whose job is to say what
// spending needs.
//
// The replacement is a value SET of the digests this composition derived from a
// phrase, and a predicate that walks the CURRENT paths. Nothing deletes from the
// set, because a value set cannot go stale: a digest no path carries is simply
// never matched.

// composerFlowShapedState builds composerState EXACTLY as composerFlow does
// (gui/composer_flow.go:34) -- a struct literal that never mentions
// phraseDigests, so the map arrives nil.
//
// THE NIL IS THE POINT. An assignment into a nil map panics, and this is the
// one production construction site, so a helper that did not allocate would
// panic on the machine at the moment the operator holds to confirm a hash that
// gates funds. Every test in this package builds the state the same way.
func composerFlowShapedState(t *testing.T, paths int) *composerState {
	t.Helper()
	st := &composerState{reg: &seedRegistry{}, bound: composerBoundFrom(nil)}
	if st.phraseDigests != nil {
		t.Fatal("this helper exists to reproduce the NIL map composerFlow leaves; it is not nil")
	}
	st.list = md.PathList{Wrapper: md.ComposeWsh, Paths: make([]md.SpendPath, paths)}
	return st
}

// TestComposerPhraseRouteHoldsOnTheZeroValueState is fidelity C-1, executed.
//
// The whole phrase route is driven to HOLD on a state built the way production
// builds it. If composerNotePhraseDigest did not allocate, this panics inside
// the GUI goroutine at the assignment -- it does not merely fail.
//
// MUTATION: drop the nil check from composerNotePhraseDigest (assign straight
// into st.phraseDigests) -> `panic: assignment to entry in nil map`.
func TestComposerPhraseRouteHoldsOnTheZeroValueState(t *testing.T) {
	st := composerFlowShapedState(t, 1)
	var ret bool
	h := runComposerHashEdit(t, st, composerSessionWith(nil, nil), 0, &ret)
	h.mustReach("Type a hashlock phrase")
	h.tapRow(0, 3)
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.mustReach("Hashlock phrase")
	typeOnPassphraseKeyboard(t, h, hashlockAnchorPhrase)
	h.tapNav(Button3)
	h.mustReach("Which method?")
	h.tapRow(1, 2) // sha256: instant
	h.mustReach("brainwallet")
	h.holdConfirm()
	h.mustReach("Write down this phrase")
	h.holdConfirm()
	h.mustReach("run ms hashlock with this phrase")

	want := hashlockMustHex(t, hashlockAnchorSHA_H)
	if _, ok := st.phraseDigests[want]; !ok {
		t.Fatalf("the anchor's digest is not in the phrase set (%d entries)", len(st.phraseDigests))
	}
	if !composerAnyPathByPhrase(st) {
		t.Error("a path carries a phrase-derived digest and the predicate says otherwise")
	}
}

// TestComposerAnyPathByPhraseIsPerDigest is the predicate itself, over the
// compositions the flag got wrong.
//
// MUTATION: report len(st.phraseDigests) > 0 without walking the paths ->
// "the phrase path was edited to a payload row" and "the phrase path was
// removed" both fail.
// MUTATION: compare p.Hash pointers instead of the digest VALUE (`for d := range
// st.phraseDigests { if p.Hash == &d }`) -> every positive row fails.
func TestComposerAnyPathByPhraseIsPerDigest(t *testing.T) {
	phrase := hashlockMustHex(t, hashlockAnchorSHA_H)
	other := hashlockMustHex(t, strings.Repeat("5a", 32))
	// A DIFFERENT POINTER holding the SAME 32 bytes: what every re-entry arm of
	// composerHashEdit writes (the hex pad, a payload row), never the pointer the
	// set was built from. The row below would otherwise be byte-identical to
	// "one phrase path" and exercise nothing (r0 fidelity M-1).
	retyped := phrase

	for _, tc := range []struct {
		name string
		set  [][32]byte
		hash []*[32]byte // one entry per path; nil means no hash
		want bool
	}{
		{"no paths at all", nil, nil, false},
		{"a nil set is read, not written", nil, []*[32]byte{&phrase}, false},
		{"one phrase path", [][32]byte{phrase}, []*[32]byte{&phrase}, true},
		{"the phrase path was edited to a payload row", [][32]byte{phrase}, []*[32]byte{&other}, false},
		{"the phrase path was removed", [][32]byte{phrase}, []*[32]byte{nil}, false},
		{"a mixed wallet: one phrase path, one other", [][32]byte{phrase}, []*[32]byte{&phrase, &other}, true},
		{"two paths share one phrase digest", [][32]byte{phrase}, []*[32]byte{&phrase, &phrase}, true},
		{"the same digest re-typed as 64 hex is still by phrase", [][32]byte{phrase}, []*[32]byte{&retyped}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := composerFlowShapedState(t, len(tc.hash))
			for _, d := range tc.set {
				composerNotePhraseDigest(st, d)
			}
			for i, h := range tc.hash {
				st.list.Paths[i].Hash = h
			}
			if got := composerAnyPathByPhrase(st); got != tc.want {
				t.Errorf("composerAnyPathByPhrase = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestReassigningTheSameDigestStaysByPhrase drives spec §2 item 4's second
// clause through production: a path whose phrase-derived digest is written
// AGAIN, by another route, is still by phrase.
//
// The hex pad is the route §2 item 4 names, and typing 64 characters on it has
// no harness helper; the payload row is the same arm of composerHashEdit
// (composer_hash.go writes a fresh *[32]byte either way) carrying the same 32
// bytes, so it exercises the property the pad would: the pointer in p.Hash is
// NOT the one composerNotePhraseDigest was given, and the predicate compares
// VALUES.
//
// MUTATION: compare p.Hash pointers instead of the digest value in
// composerAnyPathByPhrase -> this fails; so does every positive row of
// TestComposerAnyPathByPhraseIsPerDigest.
func TestReassigningTheSameDigestStaysByPhrase(t *testing.T) {
	st := composerFlowShapedState(t, 1)
	phrase := hashlockMustHex(t, hashlockAnchorSHA_H)
	st.list.Paths[0].Hash = &phrase
	composerNotePhraseDigest(st, phrase)

	var ret bool
	h := runComposerHashEdit(t, st, composerSessionWith([]string{"hash:" + hashlockAnchorSHA_H}, nil), 0, &ret)
	h.mustReach("Which hash?")
	h.tapRow(0, 4) // the payload row, carrying the SAME digest
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.waitDone()
	if !ret {
		t.Fatal("composerHashEdit returned false after a payload row")
	}
	if got := st.list.Paths[0].Hash; got == &phrase {
		t.Fatal("the payload row reused the ORIGINAL pointer, so this test proves nothing " +
			"a pointer comparison would not also pass")
	}
	if got := hashlockHashHex(st.list.Paths[0].Hash); got != hashlockAnchorSHA_H {
		t.Fatalf("path 1 hash = %s, want the same digest %s", got, hashlockAnchorSHA_H)
	}
	if !composerAnyPathByPhrase(st) {
		t.Error("the digest was derived from a phrase in this composition and re-entered " +
			"unchanged; the backup burden is the same and the predicate says otherwise")
	}
}

// TestComposerHashEditToAPayloadRowDropsThePhraseForm is F-480's own scenario,
// driven through the production edit rather than asserted on the predicate.
//
// Path 1's phrase-set hash is replaced by a `hash:` record from the payload
// while path 2 keeps a hex hash. Every path is still hashed, so §8h fires -- and
// under the old bool it fired in the PHRASE form, naming a phrase this
// composition no longer holds.
//
// MUTATION: report len(st.phraseDigests) > 0 in composerAnyPathByPhrase -> the
// banner is the phrase form and this fails.
func TestComposerHashEditToAPayloadRowDropsThePhraseForm(t *testing.T) {
	payloadDigest := strings.Repeat("ab", 32)
	st := composerFlowShapedState(t, 2)
	phrase := hashlockMustHex(t, hashlockAnchorSHA_H)
	hexed := hashlockMustHex(t, strings.Repeat("5a", 32))
	st.list.Paths[0].Hash = &phrase
	composerNotePhraseDigest(st, phrase)
	st.list.Paths[1].Hash = &hexed
	if !composerAnyPathByPhrase(st) {
		t.Fatal("the fixture must START in the phrase form for this test to mean anything")
	}

	var ret bool
	h := runComposerHashEdit(t, st, composerSessionWith([]string{"hash:" + payloadDigest}, nil), 0, &ret)
	h.mustReach("Which hash?")
	h.tapRow(0, 4) // the one payload row, above the phrase / hex / none rows
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.waitDone()
	if !ret {
		t.Fatal("composerHashEdit returned false after a payload row")
	}
	if got := hashlockHashHex(st.list.Paths[0].Hash); got != payloadDigest {
		t.Fatalf("path 1 hash = %s, want the payload row %s", got, payloadDigest)
	}
	if !composerEveryPathHashed(st.list) {
		t.Fatal("this test needs a composition §8h's guard ACCEPTS")
	}
	if composerAnyPathByPhrase(st) {
		t.Error("no path carries a phrase-derived digest and the predicate still says one does")
	}
	if got := composerCopyHashEveryPathFor(st); got != composerCopyHashEveryPath() {
		t.Errorf("§8h names a phrase this composition no longer has:\n%q", got)
	}
}

// TestComposerMixedWalletBannerNamesEveryPhraseAndEveryPlate is journey I-3.
//
// On a wallet with one phrase path and one payload-row path BOTH backups are
// needed -- one per path -- and the shipped sentence offered a choice ("the
// phrase and its method, OR the preimage plate"), which is an undercount at the
// one screen the operator reads to learn what spending needs.
//
// MUTATION: restore "Back up the phrase and its method, or the preimage plate,
// separately." -> both assertions fail.
func TestComposerMixedWalletBannerNamesEveryPhraseAndEveryPlate(t *testing.T) {
	st := composerFlowShapedState(t, 2)
	phrase := hashlockMustHex(t, hashlockAnchorSHA_H)
	fromPlate := hashlockMustHex(t, strings.Repeat("ab", 32))
	st.list.Paths[0].Hash = &phrase
	composerNotePhraseDigest(st, phrase)
	st.list.Paths[1].Hash = &fromPlate

	if !composerEveryPathHashed(st.list) || !composerAnyPathByPhrase(st) {
		t.Fatal("this test needs a MIXED wallet that §8h's guard accepts")
	}
	body := composerCopyHashEveryPathFor(st)
	for _, want := range []string{"every phrase and its method", "every preimage plate"} {
		if !strings.Contains(body, want) {
			t.Errorf("§8h's phrase form does not carry %q:\n%q", want, body)
		}
	}
	if strings.Contains(body, "method, or the") {
		t.Errorf("§8h's phrase form still offers a CHOICE of backups:\n%q", body)
	}
}

// TestTwoPlateWalletBannerCountsEveryPreimage is journey I-2: §8h's PLAIN form
// carries the same count the phrase form does.
//
// H5 §2 item 5 made the phrase form say "every ... and every" because a choice
// is an undercount at the one screen whose job is to say what spending needs.
// Its sibling had the identical defect and would have been left standing: two
// paths, two DIFFERENT digests, two different preimages the operator must hold
// -- and the shipped sentence named one ("Back the preimage up separately").
//
// The fixture is the mixed wallet of the test above with the phrase path
// replaced by a second plate, so §8h fires and the predicate is false.
//
// MUTATION: restore "Back the preimage up separately." -> both assertions fail.
func TestTwoPlateWalletBannerCountsEveryPreimage(t *testing.T) {
	st := composerFlowShapedState(t, 2)
	first := hashlockMustHex(t, strings.Repeat("ab", 32))
	second := hashlockMustHex(t, strings.Repeat("5a", 32))
	st.list.Paths[0].Hash = &first
	st.list.Paths[1].Hash = &second

	if !composerEveryPathHashed(st.list) {
		t.Fatal("this test needs a composition §8h's guard ACCEPTS")
	}
	if composerAnyPathByPhrase(st) {
		t.Fatal("this test needs the PLAIN form: no digest here came from a phrase")
	}
	body := composerCopyHashEveryPathFor(st)
	if !strings.Contains(body, "Back up every preimage separately.") {
		t.Errorf("§8h's plain form does not count the preimages:\n%q", body)
	}
	if strings.Contains(body, "Back the preimage up") {
		t.Errorf("§8h's plain form still names ONE preimage on a two-plate wallet:\n%q", body)
	}
}
