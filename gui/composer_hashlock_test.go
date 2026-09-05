package gui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/hashlock"
	"seedhammer.com/md"
)

// The anchor phrase and the corpus digests (hashlock/testdata/hashlock-v0.8.json,
// derivation row 0) -- typed on the real keyboard through the real flow.
const (
	hashlockAnchorPhrase = "correct horse battery staple"
	hashlockAnchorHardH  = "3cf5d421caf2a9c8eb9de1d400866ea7d475e6ba978861bb0167a37cb70a4c12"
	hashlockAnchorSHA_H  = "b867db875479bcc0287352cdaa4a1755689b8338777d0915e9acd9f6edbc96cb"
	hashlockMixedPhrase  = "Correct Horse Battery Staple"
)

// composerStateForTest is an empty policy shape with one path being added --
// the minimal state runComposerAddPath's callers need before a path exists.
//
// Wrapper is wsh: a key-less path is wsh-only (composer_shape.go:250, spec
// §4b/C16) and md.ComposeWrapper's zero value is ComposeTr, which REFUSES a
// key-less path outright ("This build will not put a key-less path in
// taproot") -- the very screen these tests drive through never appears under
// the zero-value state.
func composerStateForTest(t *testing.T) *composerState {
	t.Helper()
	return &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
}

// hashlockKbdFor captures the *PassphraseKeyboard hashlockPhraseFlow registers
// via hookPPWidget, keyed by the harness that is driving it.
//
// sessionHarness (gui/unlock_session_test.go) carries no widgets map of its
// own, and this file does not modify that struct -- so the capture lives here,
// alongside tapPassphraseKey, the only place that reads it.
var hashlockKbdFor = map[*sessionHarness]*PassphraseKeyboard{}

// runComposerAddPath drives composerAddPath (the CREATION entry point, where a
// false from composerHashEdit deletes the path -- spec §4.6) on the touch harness.
//
// p.display is set to sh2DisplaySize: the default 240x240 test display is
// narrower than the passphrase keyboard (340 px), which pushes q/p/a/l off the
// canvas -- reachable by a hit test, unreachable by a finger (the rule
// passphrase_flow_test.go states for exactly this reason).
func runComposerAddPath(t *testing.T, st *composerState, s *syswSession) *sessionHarness {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = s
	returned := false
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	passphraseWidgetHook = func(name string, w any) {
		if name != "kbd" {
			return
		}
		if k, ok := w.(*PassphraseKeyboard); ok {
			hashlockKbdFor[h] = k
		}
	}
	frame, drawer, quit := runUITouch(ctx, func() {
		composerAddPath(ctx, &descriptorTheme, st)
		returned = true
	})
	h.frame, h.drawer = frame, drawer
	t.Cleanup(func() {
		quit()
		passphraseWidgetHook = nil
		delete(hashlockKbdFor, h)
	})
	return h
}

// runComposerHashEdit drives composerHashEdit ALONE on the touch harness, at an
// existing path, so the row switch can be exercised per row without walking the
// whole add-path flow first. ret receives composerHashEdit's return value.
//
// Same platform setup as runComposerAddPath (sh2DisplaySize, the passphrase
// keyboard hook), because the phrase row leads to the same keyboard.
func runComposerHashEdit(t *testing.T, st *composerState, sess *syswSession, idx int, ret *bool) *sessionHarness {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sess
	returned := false
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	passphraseWidgetHook = func(name string, w any) {
		if name != "kbd" {
			return
		}
		if k, ok := w.(*PassphraseKeyboard); ok {
			hashlockKbdFor[h] = k
		}
	}
	frame, drawer, quit := runUITouch(ctx, func() {
		*ret = composerHashEdit(ctx, &descriptorTheme, st, idx)
		returned = true
	})
	h.frame, h.drawer = frame, drawer
	t.Cleanup(func() {
		quit()
		passphraseWidgetHook = nil
		delete(hashlockKbdFor, h)
	})
	return h
}

// composerStateWithPaths is composerStateForTest with n paths already present,
// each key-less and un-hashed -- the shape composerHashEdit edits in place.
func composerStateWithPaths(t *testing.T, n int) *composerState {
	t.Helper()
	st := composerStateForTest(t)
	st.list.Paths = make([]md.SpendPath, n)
	return st
}

// tapRow selects row i of an n-row composerPickScreen page by touch (the
// zero-Button Clickables plateHitPoints already knows how to find,
// unlock_platelist_test.go) and takes it -- the same "tap selects, Button3
// takes" contract composer_pick_touch_test.go exercises directly. A row
// count that does not match what is actually drawn fails loudly rather than
// silently tapping the wrong target.
func (h *sessionHarness) tapRow(i, n int) {
	h.t.Helper()
	pts := plateHitPoints(h.ctx, h.drawer())
	if len(pts) != n {
		h.t.Fatalf("tapRow(%d, %d): the screen drew %d touch targets, not %d", i, n, len(pts), n)
	}
	tap(&h.ctx.Router, h.drawer(), pts[i])
	h.next("after selecting the row")
	h.tapNav(Button3)
}

// holdConfirm holds Button3 (the ConfirmWarningScreen hold gesture) past
// confirmDelay, then RELEASES.
//
// It cannot reuse wipe_guard_test.go's sessionHarness.hold verbatim: that
// helper never sends a release, which is fine for its own callers (one hold
// per test). This route holds SEVERAL ConfirmWarningScreens in sequence (the
// key-less consent, a method warning, the final Hash lock confirm), and
// EventRouter.Events (gui/event.go) tracks exactly ONE pointer contact
// GLOBALLY: while `pointer.pressed` is true it reuses the STALE
// `pointer.pressedTag` instead of hit-testing the current frame. Measured:
// two sequential holds with no release in between "succeeded" (a frame kept
// coming back) but the second one never left 0% progress, because its press
// event was routed to the FIRST screen's now-defunct Clickable, which nobody
// still polls. The release resets `pointer.pressed`, so the NEXT hold's press
// gets a fresh hit test against the CURRENT screen.
//
// Tolerant of the flow ending here (mirrors unlock_session_test.go's
// holdDiscardConfirm): the LAST hold in several of these tests is the one
// that assigns the digest and lets composerAddPath return, so no further
// frame legitimately follows it.
func (h *sessionHarness) holdConfirm() {
	h.t.Helper()
	dims := h.ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	pos := image.Pt(dims.X-sz.X/2, ys[int(Button3-Button1)]+sz.Y/2)
	d := h.drawer()
	tag, _, hit := d.Hit(pos)
	if !hit {
		h.t.Fatalf("holdConfirm: no touch target at %v", pos)
	}
	if c, ok := tag.(*Clickable); !ok || (c.Button != Button3 && c.AltButton != Button3) {
		h.t.Fatalf("holdConfirm: the target at %v is %v", pos, tag)
	}
	h.ctx.Router.Events(d, PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event())
	h.next("hold press")
	time.Sleep(confirmDelay)
	if c, ok := h.frame(); ok {
		h.content = c
	}
	h.ctx.Router.Events(d, PointerEvent{Pressed: false, Entered: true, Pos: pos}.Event())
	if c, ok := h.frame(); ok {
		h.content = c
	}
}

// waitDone pumps frames until composerAddPath has returned (the *done flag
// runComposerAddPath set is flipped synchronously, before the underlying
// goroutine exits, so the pump's final ok==false confirms it rather than
// racing it).
func (h *sessionHarness) waitDone() {
	h.t.Helper()
	for i := 0; i < 256; i++ {
		if _, ok := h.frame(); !ok {
			if !*h.done {
				h.t.Fatalf("the session ended without composerAddPath returning")
			}
			return
		}
		if *h.done {
			return
		}
	}
	h.t.Fatalf("composerAddPath never returned after 256 frames")
}

// tapPassphraseKey types one character on the harness's registered
// PassphraseKeyboard, cycling pages by touch until the character's page is up
// -- modelled on ppHarness.typeRune (passphrase_flow_test.go), adapted to
// sessionHarness because that is the type runComposerAddPath returns.
func (h *sessionHarness) tapPassphraseKey(r rune) {
	h.t.Helper()
	kbd, ok := hashlockKbdFor[h]
	if !ok {
		h.t.Fatal("no *PassphraseKeyboard was registered for this harness")
	}
	for range len(ppPages) + 1 {
		if tag := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppRune && k.r == r }); tag != nil {
			h.tapAt(h.point(tag, "key "+string(r)))
			h.next("after typing a character")
			return
		}
		cyc := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppPageCycle })
		if cyc == nil {
			h.t.Fatalf("no page-cycle key on keyboard page %d", kbd.page)
		}
		h.tapAt(h.point(cyc, "page-cycle key"))
		h.next("after cycling the keyboard page")
	}
	h.t.Fatalf("%q is not typeable on any keyboard page", string(r))
}

// tapAt and point mirror ppHarness's (passphrase_flow_test.go), adapted to
// sessionHarness: a tap aimed at the centre of a hit area actually drawn,
// failing loudly if that area is undrawn, off-panel, or covered.
func (h *sessionHarness) tapAt(pos image.Point) {
	h.t.Helper()
	tap(&h.ctx.Router, h.drawer(), pos)
}

func (h *sessionHarness) point(tag op.Tag, what string) image.Point {
	h.t.Helper()
	d := h.drawer()
	b, ok := d.TagBounds(tag)
	if !ok {
		h.t.Fatalf("%s: no hit area was drawn -- unreachable by touch", what)
	}
	c := b.Min.Add(b.Max).Div(2)
	screen := image.Rectangle{Max: h.ctx.Platform.DisplaySize()}
	if !c.In(screen) {
		h.t.Fatalf("%s: hit area %v lies off the %v panel -- unreachable by a finger", what, b, screen)
	}
	if hit, _, ok := d.Hit(c); !ok || hit != tag {
		h.t.Fatalf("%s: hit area %v is covered by another target (%v)", what, b, hit)
	}
	return c
}

// typeOnPassphraseKeyboard taps each character of s on the four-page printable
// keyboard.
func typeOnPassphraseKeyboard(t *testing.T, h *sessionHarness, s string) {
	t.Helper()
	for _, r := range s {
		h.tapPassphraseKey(r)
	}
}

func hashlockHashHex(h *[32]byte) string { return hex.EncodeToString(h[:]) }

// groupBy inserts a space every n runes -- the corpus's own "grouped" refusal
// shape (hashlock.IsMS1Shaped strips it right back out).
func groupBy(s string, n int) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%n == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hashlockCorpusRow is the one field these GUI tests read back from the
// vendored corpus: the derivation row for a given (untouched) phrase.
type hashlockCorpusForGUI struct {
	rows map[string]struct{ SHA256H string }
}

func (c hashlockCorpusForGUI) row(t *testing.T, phrase string) struct{ SHA256H string } {
	t.Helper()
	r, ok := c.rows[phrase]
	if !ok {
		t.Fatalf("the corpus has no derivation row for %q", phrase)
	}
	return r
}

// loadHashlockCorpusForGUI reads hashlock/testdata via a path RELATIVE TO THIS
// PACKAGE (`go test` runs with gui/ as its working directory, and hashlock/ is
// a sibling of it), not a duplicate copy: the hashlock package already owns
// the vendored corpus and its provenance pin (Task 1), and this file reads the
// SAME bytes rather than re-vendoring them.
func loadHashlockCorpusForGUI(t *testing.T) hashlockCorpusForGUI {
	t.Helper()
	raw, err := os.ReadFile("../hashlock/testdata/hashlock-v0.8.json")
	if err != nil {
		t.Fatalf("reading the vendored hashlock corpus: %v", err)
	}
	var doc struct {
		Derivation []struct {
			Phrase  string `json:"phrase"`
			SHA256H string `json:"sha256_h"`
		} `json:"derivation"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the vendored hashlock corpus: %v", err)
	}
	c := hashlockCorpusForGUI{rows: map[string]struct{ SHA256H string }{}}
	for _, r := range doc.Derivation {
		c.rows[r.Phrase] = struct{ SHA256H string }{r.SHA256H}
	}
	return c
}

// Both methods, from the creation entry point, land the corpus digest on the path.
func TestHashlockPhraseRouteSetsTheCorpusDigest(t *testing.T) {
	for _, tc := range []struct {
		name, phrase, method, want string
		methodRow                  int
	}{
		{"hardened anchor", hashlockAnchorPhrase, "hardened", hashlockAnchorHardH, 0},
		{"sha256 anchor", hashlockAnchorPhrase, "sha256", hashlockAnchorSHA_H, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := composerStateForTest(t) // an empty policy shape with one path being added
			h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
			h.mustReach("What can spend on this path?")
			h.choose(1) // A hash, no keys
			h.mustReach("EXPERIMENTAL")
			h.holdConfirm() // key-less path consent (§8a)
			h.mustReach("Type a hashlock phrase")
			h.tapRow(0, 3)               // Type a hashlock phrase (no payload digests)
			h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
			h.tapNav(Button3)
			h.mustReach("Hashlock phrase")
			typeOnPassphraseKeyboard(t, h, tc.phrase)
			h.tapNav(Button3) // OK
			h.mustReach("Which method?")
			h.tapRow(tc.methodRow, 2)
			if tc.method == "sha256" {
				h.mustReach("brainwallet")
				h.holdConfirm()
			} else {
				// 28 characters: no hardened warning.
				h.mustReach("Deriving")
			}
			body := h.mustReach("Write down this phrase")
			// Post-impl I-1: the two spec 4.5-normative tokens are produced by
			// production code (hashlockFirst8Last8 and len(phrase)) and were
			// asserted by nothing -- both survived mutation of the whole suite.
			// MUTATION: hashlockFirst8Last8 returning s[:8]+".."+s[:8] -> the
			// first assertion fails; len(phrase)+1 at the call site -> the second.
			wantTok := "hash " + tc.want[:8] + ".." + tc.want[56:]
			if !strings.Contains(normalizeDrawn(body), normalizeDrawn(wantTok)) {
				t.Errorf("the confirm modal drew %q, want it to contain %q", normalizeDrawn(body), wantTok)
			}
			wantChars := fmt.Sprintf("chars: %d", len(tc.phrase))
			if !strings.Contains(normalizeDrawn(body), normalizeDrawn(wantChars)) {
				t.Errorf("the confirm modal drew %q, want it to contain %q", normalizeDrawn(body), wantChars)
			}
			h.holdConfirm()
			if got := st.list.Paths[len(st.list.Paths)-1].Hash; got == nil || hashlockHashHex(got) != tc.want {
				t.Fatalf("path hash = %v, want %s", got, tc.want)
			}
		})
	}
}

// The three non-fixed-point rows, typed exactly, derive the corpus digests -- a
// screen-layer fold (case, whitespace, separators) fails here (spec §2, §7.2).
func TestHashlockPhraseRouteDoesNotNormalise(t *testing.T) {
	c := loadHashlockCorpusForGUI(t) // reads hashlock/testdata via the package path
	for _, phrase := range []string{hashlockMixedPhrase, "  a  b ", "correct-horse,battery staple"} {
		row := c.row(t, phrase)
		st := composerStateForTest(t)
		h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
		h.mustReach("What can spend on this path?")
		h.choose(1) // A hash, no keys
		h.mustReach("EXPERIMENTAL")
		h.holdConfirm()
		h.mustReach("Type a hashlock phrase")
		h.tapRow(0, 3)
		h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
		h.tapNav(Button3)
		h.mustReach("Hashlock phrase")
		typeOnPassphraseKeyboard(t, h, phrase)
		h.tapNav(Button3)
		h.mustReach("Which method?")
		h.tapRow(1, 2) // sha256: instant
		h.mustReach("brainwallet")
		h.holdConfirm()
		h.mustReach("Write down this phrase")
		h.holdConfirm()
		if got := st.list.Paths[len(st.list.Paths)-1].Hash; got == nil || hashlockHashHex(got) != row.SHA256H {
			t.Fatalf("%q: path hash = %v, want %s", phrase, got, row.SHA256H)
		}
	}
}

// Spec §4.6: Back at every inner step keeps the phrase and never deletes the
// path; only Back at `Which hash?` returns false (and deletes it at creation).
func TestHashlockBackContractKeepsThePath(t *testing.T) {
	st := composerStateForTest(t)
	h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
	h.mustReach("What can spend on this path?")
	h.choose(1) // A hash, no keys
	h.mustReach("EXPERIMENTAL")
	h.holdConfirm()
	h.mustReach("Type a hashlock phrase")
	h.tapRow(0, 3)
	h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
	h.tapNav(Button3)
	h.mustReach("Hashlock phrase")
	typeOnPassphraseKeyboard(t, h, hashlockAnchorPhrase)
	h.tapNav(Button3)
	h.mustReach("Which method?")
	h.tapNav(Button1) // Back -> phrase screen, phrase intact
	h.mustReach("Hashlock phrase")
	h.mustReach("28/100")
	h.tapNav(Button3)
	h.mustReach("Which method?")
	h.tapRow(1, 2)
	h.mustReach("brainwallet")
	h.tapNav(Button1) // decline -> method pick, phrase intact
	h.mustReach("Which method?")
	h.tapRow(0, 2)
	h.mustReach("Deriving")
	h.tapNav(Button1) // Back during derivation -> method pick
	h.mustReach("Which method?")
	h.tapRow(1, 2)
	h.mustReach("brainwallet")
	h.holdConfirm()
	h.mustReach("Write down this phrase")
	h.tapNav(Button1) // Back on the confirm -> method pick, nothing assigned
	h.mustReach("Which method?")
	if n := len(st.list.Paths); n != 1 {
		t.Fatalf("path deleted by an inner Back: %d paths", n)
	}
	if st.list.Paths[0].Hash != nil {
		t.Fatalf("hash assigned before HOLD")
	}
	h.tapNav(Button1) // Back at method pick -> phrase screen
	h.mustReach("Hashlock phrase")
	h.tapNav(Button1) // Back at phrase screen -> Which hash?, phrase dropped
	h.mustReach("Type a hashlock phrase")
	if n := len(st.list.Paths); n != 1 {
		t.Fatalf("path deleted by Back to Which hash?: %d paths", n)
	}
	h.tapNav(Button1) // Back at Which hash? -> false -> creation deletes the path
	h.waitDone()
	if n := len(st.list.Paths); n != 0 {
		t.Fatalf("Back at Which hash? at creation must delete the path: %d paths", n)
	}
}

// Declined SHA-256, then Hardened, with the phrase typed ONCE (spec §7.2).
func TestHashlockDeclineThenHardenedTypesOnce(t *testing.T) {
	st := composerStateForTest(t)
	h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
	h.mustReach("What can spend on this path?")
	h.choose(1) // A hash, no keys
	h.mustReach("EXPERIMENTAL")
	h.holdConfirm()
	h.mustReach("Type a hashlock phrase")
	h.tapRow(0, 3)
	h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
	h.tapNav(Button3)
	h.mustReach("Hashlock phrase")
	typeOnPassphraseKeyboard(t, h, hashlockAnchorPhrase)
	h.tapNav(Button3)
	h.mustReach("Which method?")
	h.tapRow(1, 2)
	h.mustReach("brainwallet")
	h.tapNav(Button1)
	h.mustReach("Which method?")
	h.tapRow(0, 2)
	h.mustReach("Deriving")
	h.mustReach("Write down this phrase")
	h.holdConfirm()
	if got := st.list.Paths[0].Hash; got == nil || hashlockHashHex(got) != hashlockAnchorHardH {
		t.Fatalf("hash = %v, want hardened anchor", got)
	}
}

// The §2 refusals through the screen: 101/100 visible and refused; 64 hex; an
// ms1 plate grouped and ungrouped (spec §7.2).
func TestHashlockPhraseRefusalsOnScreen(t *testing.T) {
	const plate = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c"
	for _, tc := range []struct{ name, typed, needle string }{
		{"101 characters", strings.Repeat("k", 101), "at most 100 characters"},
		{"64 hex", hashlockAnchorHardH, "Use the Type 64 hex row"},
		{"plate ungrouped", plate, "preimage plate, not a phrase"},
		{"plate grouped by 5", groupBy(plate, 5), "preimage plate, not a phrase"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := composerStateForTest(t)
			h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
			h.mustReach("What can spend on this path?")
			h.choose(1) // A hash, no keys
			h.mustReach("EXPERIMENTAL")
			h.holdConfirm()
			h.mustReach("Type a hashlock phrase")
			h.tapRow(0, 3)
			h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
			h.tapNav(Button3)
			h.mustReach("Hashlock phrase")
			typeOnPassphraseKeyboard(t, h, tc.typed)
			if tc.name == "101 characters" {
				h.mustReach("101/100")
			}
			h.tapNav(Button3)
			h.mustReach(tc.needle)
		})
	}
}

// The method modals fire on their condition and not otherwise (19 vs 20 chars;
// sha256 always).
func TestHashlockMethodModalsFireOnCondition(t *testing.T) {
	for _, tc := range []struct {
		phrase string
		method int
		warns  bool
	}{
		{"nineteen-characters", 0, true},   // 19 chars, hardened -> 72-days modal
		{"twenty--characters!!", 0, false}, // 20 chars, hardened -> no modal
		{"twenty--characters!!", 1, true},  // sha256 -> always
	} {
		st := composerStateForTest(t)
		h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
		h.mustReach("What can spend on this path?")
		h.choose(1) // A hash, no keys
		h.mustReach("EXPERIMENTAL")
		h.holdConfirm()
		h.mustReach("Type a hashlock phrase")
		h.tapRow(0, 3)
		h.mustReach("32-byte value") // the §8i rule modal (composerCopyHashRule)
		h.tapNav(Button3)
		h.mustReach("Hashlock phrase")
		typeOnPassphraseKeyboard(t, h, tc.phrase)
		h.tapNav(Button3)
		h.mustReach("Which method?")
		h.tapRow(tc.method, 2)
		if tc.warns {
			h.mustReach("Continue?")
		} else {
			h.mustReach("Deriving")
		}
	}
}

// The relation line, parameterised (r0 fidelity I-5). Round 0 had ONE case whose
// matching record sat at index 0, so `match := 0` in place of `match := -1` was
// indistinguishable from correct code: the loop found a real match at 0 either
// way. Three cases close it.
//
// MUTATIONS:
//   - `match := 0` -> the "neither record matches" case reports `matches hash 1
//     in the payload` instead of the no-match line, and fails.
//   - report `match` rather than `match+1` (1-based off-by-one) -> the "second
//     record matches" case fails, because it is the only one whose answer is not
//     also 1 under the mutation.
//   - `if len(payload) > 0` -> `if true` in hashlockRelationLine -> the "no
//     records at all" case fails on the unwanted no-match line.
func TestHashlockConfirmRelationLine(t *testing.T) {
	const otherDigest = "abababababababababababababababababababababababababababababababab"
	for _, tc := range []struct {
		name     string
		records  []string
		want     string
		unwanted string
	}{
		{
			"the SECOND record matches -- pins the 1-based index",
			[]string{"hash:" + otherDigest, "hash:" + hashlockAnchorSHA_H},
			"matches hash 2 in the payload", "matches hash 1",
		},
		{
			"records are loaded and NEITHER matches",
			[]string{"hash:" + otherDigest, "hash:" + strings.Repeat("cd", 32)},
			"no hash: record in the payload has this digest", "matches hash",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := composerStateForTest(t)
			h := runComposerAddPath(t, st, composerSessionWith(tc.records, nil))
			h.mustReach("What can spend on this path?")
			h.choose(1) // A hash, no keys
			h.mustReach("EXPERIMENTAL")
			h.holdConfirm()
			h.mustReach("Which hash?")
			h.tapRow(len(tc.records), len(tc.records)+3) // the phrase row sits after the payload rows
			h.mustReach("32-byte value")                 // the §8i rule modal (composerCopyHashRule)
			h.tapNav(Button3)
			h.mustReach("Hashlock phrase")
			typeOnPassphraseKeyboard(t, h, hashlockAnchorPhrase)
			h.tapNav(Button3)
			h.mustReach("Which method?")
			h.tapRow(1, 2)
			h.mustReach("brainwallet")
			h.holdConfirm()
			body := h.mustReach(tc.want)
			if uiContains(body, tc.unwanted) {
				t.Errorf("the confirm modal also drew %q: %q", tc.unwanted, body)
			}
		})
	}

	// With NO hash: records loaded, neither line is drawn at all -- the arm the
	// two cases above cannot reach.
	if got := hashlockRelationLine(nil, hashlockMustHex(t, hashlockAnchorSHA_H)); got != "" {
		t.Errorf("no payload records drew the relation line %q", got)
	}
}

// composerHashEdit dispatches BY LABEL, driven through the screen with two
// payload digests loaded -- the shape that can tell a correct switch from a
// surgical reversion to index arithmetic (r0 fidelity I-1, refined by tests I-2).
//
// With 2 digests the rows are payload 0, payload 1, phrase (2), hex (3), none
// (4). MUTATION: replace the switch's phrase/hex/none arms with
// `case sel == len(rows.digests): // phrase` + `default: st.list.Paths[idx].Hash
// = nil` -- the reversion the plan's own C-4 comment describes. The phrase row
// still lands correctly (it IS len(digests)), so every test that runs with 0
// payload digests still passes; the "hex row opens hex entry" subtest below is
// what fails, because the hex row falls into the clearing arm and
// composerHashEdit returns true with Hash nil instead of drawing the pad.
func TestComposerHashEditDispatchesByRowLabel(t *testing.T) {
	const digestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const digestB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sessionOf := func() *syswSession {
		return composerSessionWith([]string{"hash:" + digestA, "hash:" + digestB}, nil)
	}

	t.Run("payload row 2 assigns payload digest 2", func(t *testing.T) {
		st := composerStateWithPaths(t, 1)
		var ret bool
		h := runComposerHashEdit(t, st, sessionOf(), 0, &ret)
		h.mustReach("Which hash?")
		h.tapRow(1, 5)
		h.mustReach("32-byte value") // the §8i rule modal: a payload row TAKES a hash
		h.tapNav(Button3)
		h.waitDone()
		if !ret {
			t.Fatal("composerHashEdit returned false after a payload row was taken")
		}
		if got := st.list.Paths[0].Hash; got == nil || hashlockHashHex(got) != digestB {
			t.Fatalf("hash = %v, want payload digest 2", got)
		}
	})

	t.Run("hex row opens hex entry and does not clear", func(t *testing.T) {
		st := composerStateWithPaths(t, 1)
		var ret bool
		h := runComposerHashEdit(t, st, sessionOf(), 0, &ret)
		h.mustReach("Which hash?")
		h.tapRow(3, 5)
		h.mustReach("32-byte value")
		h.tapNav(Button3)
		// The hex pad, NOT a cleared lock and a returned composerHashEdit.
		h.mustReach("0 of 64 hex")
		if *h.done {
			t.Fatal("composerHashEdit returned instead of opening hex entry")
		}
		h.tapNav(Button1) // Back at the pad -> `Which hash?`, nothing assigned
		h.mustReach("Which hash?")
		if st.list.Paths[0].Hash != nil {
			t.Fatal("Back at the hex pad assigned a hash")
		}
	})

	t.Run("phrase row opens the phrase screen", func(t *testing.T) {
		st := composerStateWithPaths(t, 1)
		var ret bool
		h := runComposerHashEdit(t, st, sessionOf(), 0, &ret)
		h.mustReach("Which hash?")
		h.tapRow(2, 5)
		h.mustReach("32-byte value")
		h.tapNav(Button3)
		h.mustReach("Hashlock phrase")
	})

	t.Run("none row clears without the rule modal", func(t *testing.T) {
		st := composerStateWithPaths(t, 1)
		var preset [32]byte
		preset[0] = 0x11
		st.list.Paths[0].Hash = &preset
		st.hashByPhrase = true
		var ret bool
		h := runComposerHashEdit(t, st, sessionOf(), 0, &ret)
		h.mustReach("Which hash?")
		h.tapRow(4, 5)
		h.waitDone()
		if !ret {
			t.Fatal("composerHashEdit returned false after `No hash lock`")
		}
		if st.list.Paths[0].Hash != nil {
			t.Fatal("`No hash lock` did not clear the hash")
		}
		// r0 adversarial I-2: the provenance flag is dropped once no path
		// carries a hash at all. MUTATION: delete the composerHashByPhraseSync
		// call in composerHashEdit's noneRow arm -> this fails.
		if st.hashByPhrase {
			t.Fatal("st.hashByPhrase survived the last hash being cleared")
		}
	})
}

// Spec §4.6 through the CREATION entry point for the row this plan CHANGED:
// `Type 64 hex`'s Back used to propagate out of composerHashEdit and delete the
// path (composer_shape.go:269-272 at the fork baseline c4a64fc); under §4.6 it
// returns to `Which hash?` with
// the path intact. Round 0 claimed "Task 4's harness tests do" cover this and
// none did (r0 adversarial I-3 = fidelity I-4 = journey I-4).
//
// MUTATION: `return false` in place of `continue` in composerHashEdit's hex arm
// -> measured: `never reached "Type a hashlock phrase"; last frame
// "0123456789ABCDEF0of64hexHashlock"`. It fails EARLIER than at the path count,
// because composerHashEdit's false unwinds composerAddPath, which deletes the
// path and leaves the screen -- so `Which hash?` never comes back at all. The
// path-count assertion below is what states the device consequence.
func TestHashlockHexRowBackKeepsThePath(t *testing.T) {
	st := composerStateForTest(t)
	h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
	h.mustReach("What can spend on this path?")
	h.choose(1) // A hash, no keys
	h.mustReach("EXPERIMENTAL")
	h.holdConfirm()
	h.mustReach("Type a hashlock phrase")
	h.tapRow(1, 3) // Type 64 hex (no payload digests: phrase 0, hex 1, none 2)
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.mustReach("0 of 64 hex")
	h.tapNav(Button1) // Back at the pad
	h.mustReach("Type a hashlock phrase")
	if n := len(st.list.Paths); n != 1 {
		t.Fatalf("Back at the hex pad deleted the path: %d paths", n)
	}
	if st.list.Paths[0].Hash != nil {
		t.Fatal("Back at the hex pad assigned a hash")
	}
}

// hashlockDerivingLead is §4.4's lead, as a pure function (r0 adversarial I-4).
//
// The guard itself is not what round 0 got wrong -- `done > 0 && elapsed > 0`
// around the estimate and `done <= 0 || elapsed <= 0` around the zero state are
// the same predicate. What was wrong is WHERE it was evaluated: only inside
// DeriveHardened's callback, whose first call arrives at done = 501, so the zero
// state could never be chosen. The hoisted zero-state FRAME in hashlockDeriveFlow
// is the fix, and TestHashlockDeriveKeepsAwakeUnderTheScreensaver asserts the
// lead is drawn on frame 0.
//
// MUTATION for THIS test: drop the guard and return the estimate unconditionally
// -> the three zero-state rows below fail with
// `= "About -9223372036 seconds left."` (done = 0 divides into the estimate).
func TestHashlockDerivingLead(t *testing.T) {
	zero := composerCopyHashlockDerivingLead()
	for _, tc := range []struct {
		name        string
		done, total int
		elapsed     time.Duration
		want        string
	}{
		{"the zero-state frame", 0, hashlock.Iterations, 0, zero},
		{"zero done, time already passed", 0, hashlock.Iterations, 2 * time.Second, zero},
		{"no elapsed time yet", 500, hashlock.Iterations, 0, zero},
		{"halfway, five seconds in", 50000, 100000, 5 * time.Second, "About 5 seconds left."},
		{"a tenth in, one second", 10000, 100000, time.Second, "About 9 seconds left."},
	} {
		if got := hashlockDerivingLead(tc.done, tc.total, tc.elapsed); got != tc.want {
			t.Errorf("%s: hashlockDerivingLead(%d, %d, %v) = %q, want %q",
				tc.name, tc.done, tc.total, tc.elapsed, got, tc.want)
		}
	}
}

// The hardened derivation must not be parked by the screensaver, and its
// zero-state lead must actually be drawn (r0 adversarial C-1 and I-4).
//
// This is the fork's own F-93 regression shape (run_flow_test.go:671's
// TestRunKeepAwakeDuringDerivationDoesNotParkUnderTheScreensaver), pointed at
// hashlockDeriveFlow BY NAME: that test drives unlockDerive and cannot see this
// screen, and the touch harness the rest of this file uses is structurally blind
// to the class, because runUITouch sets ctx.FrameCallback directly and never
// runs Run's idle loop at all.
//
// The arithmetic: hashlock.Iterations = 100,000 in Step(500) slices is 200
// progress calls, and at p.tickFloor = 1s that is 200 s of bubble time against
// idleTimeout's 180 s (gui/gui.go:3584) -- the crossing happens inside the
// derivation, with margin. The floor is load-bearing for the same reason
// deadlinePlatform documents: with ctx.WakeupAt(time.Now()) every deadline is
// already expired, so without a floor the bubble clock never advances and the
// mutant would pass too.
//
// MUTATIONS, both measured:
//   - delete `ctx.KeepAwake()` from hashlockDeriveFlow's frame closure -> the
//     screensaver activates at 180 s and its branch `continue`s without
//     returning control, so ctx.Frame never returns and mustFinish reports
//     "Run exceeded 100000 ticks without terminating -- flow is probably parked
//     (screensaver?). 180 frames drawn, last = 89%About21secondsleft.Deriving".
//   - delete `ctx.WakeupAt(time.Now())` and keep KeepAwake -> the saver never
//     fires (KeepAwake refreshes a.idle.start every tick), so the PARK check
//     above stays green; what breaks is the CLOCK. Every AppendEvents then waits
//     out Run's own ctx.WakeupAt(idleWakeup) -- three minutes -- so a 10-second
//     derivation takes ten hours and the countdown freezes between slices. The
//     elapsed-time assertion below is what sees it: 9h57m1s against the 201 s a
//     1 s tick floor costs. Measured, and the reason this test asserts on device
//     time and not only on completion.
//   - delete `frame(0, hashlock.Iterations)` (the zero-state frame) -> the lead
//     assertion below fails; the derivation itself still completes.
func TestHashlockDeriveKeepsAwakeUnderTheScreensaver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		p.tickFloor = 1 * time.Second
		var got [32]byte
		var ok bool
		flow := func(ctx *Context, version string) {
			got, ok = hashlockDeriveFlow(ctx, &descriptorTheme, []byte(hashlockAnchorPhrase), hashlockHardened)
			ctx.Done = true
		}
		start := time.Now()
		drawn := mustFinish(t, p, flow, nil)
		elapsed := time.Since(start)
		if !ok {
			t.Fatal("hashlockDeriveFlow returned ok=false -- abandoned or never finished")
		}
		// 201 frames at a 1 s tick floor is 201 s of bubble time. A frame that
		// does not ask to be woken waits out Run's idle deadline instead, which
		// is 3 minutes EACH -- two orders of magnitude, so the bound does not
		// need to be tight to be decisive.
		if elapsed > 10*time.Minute {
			t.Errorf("the derivation took %v of device time; at a %v tick floor and %d frames "+
				"it should take about %v. A frame that omits ctx.WakeupAt(time.Now()) waits out "+
				"Run's idle deadline (3 min) instead of the next 500-iteration slice",
				elapsed, p.tickFloor, len(drawn), time.Duration(len(drawn))*p.tickFloor)
		}
		if want := hashlock.PreimageHardened([]byte(hashlockAnchorPhrase)); got != want {
			t.Error("the derived preimage is not PreimageHardened's (bytes deliberately not logged)")
		}
		if len(drawn) < 200 {
			t.Errorf("only %d frames drawn; 100,000 iterations in 500-step slices is 201", len(drawn))
		}
		if !uiContains(drawn[0], "This takes about 10 seconds") {
			t.Errorf("the first frame is %q, not §4.4's zero-state lead", drawn[0])
		}
		if !uiContains(drawn[len(drawn)-1], "seconds left") {
			t.Errorf("the last frame is %q, not the countdown estimate", drawn[len(drawn)-1])
		}
	})
}

// The reconciliation line is reachable for EVERY policy that has a phrase-set
// hash, including the ordinary mixed one (r0 adversarial I-1 = fidelity I-2 =
// journey I-3). §4.5's drop-order step 2 had moved the line into the §8h form at
// Done, which composerEveryPathHashed guards -- false the moment one path is
// keyed, so on this shape the line was drawn nowhere.
//
// The state here IS that shape: path 0 already carries a hash of its own, and
// path 1 (the one being edited) gets the phrase route. composerEveryPathHashed
// is asserted below, so the test fails loudly if a future edit makes the §8h
// guard true here and the case stops being the one it was written for.
//
// MUTATION: delete the showError(..., composerCopyHashlockReconcile()) call from
// hashlockPhraseRoute -> `never reached "run ms hashlock with this phrase"`.
func TestHashlockReconcileScreenIsReachableOnAMixedPolicy(t *testing.T) {
	st := composerStateWithPaths(t, 2)
	var other [32]byte
	other[0] = 0x11
	st.list.Paths[0].Hash = &other
	st.list.Paths[1].Hash = nil
	if composerEveryPathHashed(st.list) {
		t.Fatal("this test needs a policy §8h's guard REJECTS; it no longer is one")
	}
	var ret bool
	h := runComposerHashEdit(t, st, composerSessionWith(nil, nil), 1, &ret)
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
	// The other path's hash differs, so §4.5's second relation line fires too
	// (r0 journey I-1). MUTATION: return "" from hashlockOtherPathLine ->
	// `never reached "back up every phrase"`.
	h.mustReach("back up every phrase")
	h.holdConfirm()
	h.mustReach("run ms hashlock with this phrase")
	if got := st.list.Paths[1].Hash; got == nil || hashlockHashHex(got) != hashlockAnchorSHA_H {
		t.Fatalf("path 2 hash = %v, want the anchor's sha256 digest", got)
	}
	// r0 tests I-4: the flag's real assignment, driven through the route rather
	// than built as a struct literal. MUTATION: delete `st.hashByPhrase = true`
	// from hashlockPhraseRoute -> this fails.
	if !st.hashByPhrase {
		t.Fatal("the phrase route did not record that this hash was set by phrase")
	}
}

// The confirm modal's SECOND relation line stays silent when the other path
// carries the SAME digest -- one phrase, not two (r0 journey I-1's other half).
//
// MUTATION: drop the `*p.Hash != h` comparison from hashlockOtherPathLine (warn
// whenever any other path has any hash) -> this fails at the unwanted-text check.
func TestHashlockOtherPathLineIsSilentOnAnEqualHash(t *testing.T) {
	same := hashlockMustHex(t, hashlockAnchorSHA_H)
	st := composerStateWithPaths(t, 2)
	st.list.Paths[0].Hash = &same
	if got := hashlockOtherPathLine(st, 1, same); got != "" {
		t.Errorf("an EQUAL hash on another path drew %q, want silence", got)
	}
	if got := hashlockOtherPathLine(st, 0, same); got != "" {
		t.Errorf("the path being edited must not warn about itself: %q", got)
	}
	var different [32]byte
	different[0] = 0x11
	if got := hashlockOtherPathLine(st, 1, different); got != composerCopyHashlockOtherPath() {
		t.Errorf("a DIFFERENT hash on another path drew %q, want the warning", got)
	}
	if got := hashlockOtherPathLine(composerStateWithPaths(t, 2), 1, different); got != "" {
		t.Errorf("no other path carries a hash at all; drew %q", got)
	}
	// Post-impl e2e I-1: with THREE other paths carrying three different hashes
	// (the reasonably complex wallet's shape) the line must still be the same
	// warning and must carry no count -- "two phrases" was wrong here.
	// MUTATION: put a number back into composerCopyHashlockOtherPath -> fails.
	many := composerStateWithPaths(t, 4)
	for i := 0; i < 3; i++ {
		var d [32]byte
		d[0] = byte(0x20 + i)
		many.list.Paths[i].Hash = &d
	}
	if got := hashlockOtherPathLine(many, 3, different); got != composerCopyHashlockOtherPath() {
		t.Errorf("three other differing hashes drew %q, want the warning", got)
	}
	if strings.ContainsAny(composerCopyHashlockOtherPath(), "0123456789") || strings.Contains(composerCopyHashlockOtherPath(), "two") {
		t.Errorf("the other-path line carries a count: %q", composerCopyHashlockOtherPath())
	}
}

func hashlockMustHex(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad 32-byte hex %q: %v", s, err)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

// Post-impl I-2 (F-481): the phrase screen must DRAW its readout. Before the
// fix an 8 px CutBottom left the passphrase keyboard's readout budget at 11 px
// (one line needs 19), so PassphraseKeyboard.Layout dropped every rune: no
// asterisks masked, nothing on reveal, while the `show` key stayed live -- the
// dead-control shape the fork removed the gear key for.
// MUTATION: restore `content, _ = content.CutBottom(8)` in hashlockPhraseFlow
// -> zero asterisks in the frame and this test fails.
func TestHashlockPhraseScreenDrawsTheMaskedReadout(t *testing.T) {
	st := composerStateForTest(t)
	h := runComposerAddPath(t, st, composerSessionWith(nil, nil))
	h.mustReach("What can spend on this path?")
	h.choose(1) // A hash, no keys
	h.mustReach("EXPERIMENTAL")
	h.holdConfirm()
	h.mustReach("Type a hashlock phrase")
	h.tapRow(0, 3)
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.mustReach("Hashlock phrase")
	typeOnPassphraseKeyboard(t, h, "abcdefghij")
	frame := h.mustReach("10/100")
	if n := strings.Count(frame, "*"); n < 10 {
		t.Fatalf("the phrase screen drew %d asterisks for 10 typed characters; the readout is not drawn (F-481). frame: %q", n, normalizeDrawn(frame))
	}
}

// Post-impl N-1: every sentinel hashlock.ValidatePhrase can return has a copy
// arm; the err.Error() fallthrough (a Go error with a package prefix) must be
// unreachable. MUTATION: delete the ErrHex64 case -> this test names it.
func TestHashlockRefusalCopyCoversEverySentinel(t *testing.T) {
	for _, err := range []error{hashlock.ErrEmpty, hashlock.ErrNotPrintableASCII, hashlock.ErrMS1Shaped, hashlock.ErrTooLong, hashlock.ErrHex64} {
		if got := composerCopyHashlockRefusal(err); got == "" || got == err.Error() {
			t.Errorf("composerCopyHashlockRefusal(%v) = %q: fell through to the Go error", err, got)
		}
	}
}

// Post-impl interruption M-1: "Remove path" is the other event after which no
// phrase-set hash can remain in the composition, and it spliced the slice
// without re-syncing st.hashByPhrase -- so §8h at Done would name a phrase the
// composition no longer has.
// MUTATION: delete the composerHashByPhraseSync call in composerPathEdit's
// Remove arm -> the flag stays true and this fails.
func TestRemovePathReSyncsHashByPhrase(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		st := &composerState{list: composerTwoPathList(), reg: &seedRegistry{}}
		composerSizeAssignments(st) // nothing seated: the shape guard stays silent
		d := hashlockMustHex(t, hashlockAnchorSHA_H)
		st.list.Paths[0].Hash = &d
		st.hashByPhrase = true // path 1's hash came from a phrase; path 2 has none
		frame, quit := runUI(ctx, func() { composerPathEdit(ctx, &descriptorTheme, st, 0) })
		defer quit()
		pumpUntil(frame, "Path 1:", 16)
		click(&ctx.Router, Down, Down, Down) // Keys, Time lock, Hash lock -> Remove path
		click(&ctx.Router, Button3)
		for i := 0; i < 8 && len(st.list.Paths) == 2; i++ {
			frame()
		}
		if len(st.list.Paths) != 1 {
			t.Fatalf("Remove path did not remove the path (len %d)", len(st.list.Paths))
		}
		if st.hashByPhrase {
			t.Fatal("the only phrase-set hash was removed and st.hashByPhrase is still true")
		}
	})
}
