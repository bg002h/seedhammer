package gui

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"
)

// F-544 / §13.5: `restoreDoc` occurred ZERO times in gui/composer*.go. An
// operator who BUILT a wallet with the composer walked away with steel and no
// document, while the same operator coming through Multisig Build got one.
// That widens F-132's open half — a backup that omits a required factor
// without saying so.
//
// THE EXISTING SUITE WAS SILENT ON THIS. All 1342 gui tests passed with the
// screen wired in, because no test drives composerEngraveStep to
// bundleEngraveDone — the branch the document hangs off. A blocking screen
// nothing reaches would have passed exactly the same way, which is the shape
// that once cost a 3-minute on-device block under 1213 green tests.
//
// So this file asserts the two things the suite could not: the document DRAWS
// and DISMISSES, and it is actually CALLED.
func TestComposerRestoreDocDrawsAndDismisses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keyed []string
		want  []string
	}{
		{
			// Unseated: `keyed` is nil (composerArtifactsFor returns it only
			// when every slot is seated), so there is no descriptor and no
			// address — but there is still a pile of steel to inventory.
			// Returning silently here would reproduce the very silence F-544
			// is about, one case narrower.
			name:  "key-less template",
			keyed: nil,
			want:  []string{"key-less TEMPLATE", "mk1 key cards"},
		},
		{
			// A keyed card that does not expand: say so, and still give the
			// inventory rather than dropping the document.
			name:  "a keyed card that will not read back",
			keyed: []string{"md1qqqqqqqqnotavalidcard"},
			want:  []string{"could not read the policy back"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				done := false
				frame, quit := runUI(ctx, func() {
					composerRestoreDoc(ctx, &descriptorTheme, tc.keyed,
						[]string{"Plates To Cut", "plate 1  md1 policy"})
					done = true
				})
				t.Cleanup(quit)

				content, ok := pumpUntil(frame, "not fully checked", 32)
				if !ok {
					t.Fatalf("the restore doc never drew.\nLast frame: %q", content)
				}
				for _, w := range tc.want {
					if !uiContains(content, w) {
						// Page forward: the screen pages, and the sentence may
						// be below the fold.
						content, ok = composerPageUntil(t, ctx, frame, w, 12)
						if !ok {
							t.Errorf("the document never says %q.\nLast frame: %q", w, content)
						}
					}
				}

				// IT MUST RETURN. A document that cannot be dismissed is worse
				// than none: the flow ends here, and an operator holding steel
				// would be stuck on it.
				for i := 0; i < 64 && !done; i++ {
					click(&ctx.Router, Button3)
					frame()
				}
				if !done {
					t.Fatal("the restore doc never returned; the composer would hang on it")
				}
			})
		})
	}
}

// CAN A USER ACTUALLY REACH IT. The screen above could draw perfectly and
// never be called — a test exercising dead code passes just as green. The
// production call site is asserted structurally, because no gui test drives
// composerEngraveStep as far as bundleEngraveDone.
//
// MUTATION: delete the call in composerEngraveStep -> this fails.
func TestComposerRestoreDocIsActuallyCalled(t *testing.T) {
	src, err := os.ReadFile("composer_flow.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "composerRestoreDoc(ctx, th, keyed,")
	if i < 0 {
		t.Fatal("composerEngraveStep does not call composerRestoreDoc: the document " +
			"is unreachable and F-544 is not closed")
	}
	// And it must hang off the SUCCESS branch: a document shown after an
	// aborted run would describe plates that were never cut.
	head := s[:i]
	if !strings.Contains(head[max(0, len(head)-200):], "if done {") {
		t.Errorf("composerRestoreDoc is not called under `if done`; an aborted run " +
			"would be handed a document describing plates it never cut")
	}
}
