package gui

import (
	"strings"
	"testing"

	"seedhammer.com/seal"
)

// F-474, the flow-level half, and the same argument §10.2.1a's test next door
// makes: seal.RecordNotPermittedError carrying the index and the kind is not
// the requirement -- being TOLD is.
//
// unlockSealedFlow's switch has a `default:` arm that renders every unhandled
// error as "Payload unreadable.", so a refusal with no case of its own is
// invisible: the operator authenticates successfully, waits out the derivation,
// and is then told their intact payload has been tampered with. §2.2 item 4 has
// taught them to read "unreadable" as "someone replaced my payload", so the
// screen sends them chasing a compromise that did not happen. Only a flow-level
// test can see it -- seal's own tests stay green through it.
//
// The vector is the seam corpus's preimage-plate-0x03 row, the same string
// seal/record_test.go and cmd/emu/walk_h0_preimage.js use. Its classification
// is asserted here rather than trusted.
const guiPreimagePlate = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c"

func TestUnlockNamesARefusedPreimageInsteadOfCallingItUnreadable(t *testing.T) {
	if got := seal.Classify([]byte(guiPreimagePlate)); got != seal.ClassUnknown {
		t.Fatalf("H0 keeps a preimage plate ClassUnknown; Classify says %v", got)
	}

	d := sealVector(t, "D")
	// A public section too, so the §6.6 hash screen is drawn and the flow runs
	// exactly as a real both-sections payload would. The refusal happens in the
	// ENCRYPTED section, AFTER the AEAD tag verified -- which is precisely why
	// "unreadable" was the wrong word for it.
	//
	// The plate sits at record 1, not record 0: an arm that hardcoded the index
	// would pass on a single-record section and fail here.
	blob := sealBlobForTest(t, d.Public, []string{d.Secret[0], guiPreimagePlate},
		fixturePassphrase, fixtureIterations)

	h := newUnlockHarness(t, payloadReaderFrom(t, blob))
	h.toPassphrase(true)
	h.typePassphrase(strings.Fields(fixturePassphrase))
	got := h.mustReach("hashlock preimage")

	// MUTATION: delete the errors.As arm from unlockSealedFlow -> the flow falls
	// through to `default:` and this test fails at mustReach("hashlock
	// preimage") with the last frame reading "Payload unreadable.".
	if strings.Contains(got, "unreadable") {
		t.Errorf("the screen still says the payload is unreadable; got %q", got)
	}
	// MUTATION: hardcode `1` -> passes here; that is why unlockNotPermittedBody
	// is table-tested below over several indices.
	if !uiContains(got, "Record 1") {
		t.Errorf("the screen must name WHICH record; got %q", got)
	}
	if !uiContains(got, "not a seed") {
		t.Errorf("the screen must say a preimage is not a seed; got %q", got)
	}
	if !uiContains(got, "Nothing was opened") {
		t.Errorf("the screen must say nothing was opened; got %q", got)
	}
	// H5 §5 (F-488): the refusal says what to do next. Naming the record and
	// the kind leaves the operator holding an intact payload and no route.
	// MUTATION: drop the new sentence -> this fails.
	if !uiContains(got, "Remove that record") {
		t.Errorf("the screen must say what to do next; got %q", got)
	}
	// MUTATION: drop "(records count from 0)" -> this fails. The index is
	// 0-based (seal/record.go:69) and the device said so nowhere; once the
	// number is an instruction to DELETE, a 1-based reading deletes the record
	// above the plate -- in this fixture's own blob, the seed at record 0.
	if !uiContains(got, "records count from 0") {
		t.Errorf("the screen must say the index is 0-based; got %q", got)
	}
	// MUTATION: drop "-- and any others like it --" -> this fails. AdmitSection
	// returns on the FIRST refused record, so a payload with two plates is
	// refused twice and the index MOVES between rounds; an operator applying the
	// second refusal's number to their original listing deletes a record the
	// device never named (r0 journey M-3).
	if !uiContains(got, "and any others like it") {
		t.Errorf("the screen must say there may be more than one; got %q", got)
	}

	// It leaves rather than looping for another passphrase: the payload is
	// refused WHOLE and retrying cannot help.
	h.tapNav(Button3)
	for i := 0; i < 128 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatalf("the refusal did not leave the flow; last frame %q", h.content)
	}
}

// The body as a pure function, over indices and kinds the flow test cannot
// cheaply reach (F-474). The flow test drives ONE record at ONE index, so it
// cannot tell a body that reads its argument from one that hardcodes the case
// it happens to be driven with -- this can.
//
// MUTATIONS:
//   - hardcode the index (`Record 1` always) -> the record-0 and record-7 rows fail.
//   - report the Preimage flag for every class -> the codex32-secret row fails.
//   - ignore Preimage and always use Class.String() -> both preimage rows fail
//     with "unknown format" where "a hashlock preimage" is wanted.
func TestUnlockNotPermittedBodyNamesTheRecordAndTheKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *seal.RecordNotPermittedError
		want []string
		not  []string
	}{
		{
			"a preimage plate at record 1",
			&seal.RecordNotPermittedError{Index: 1, Class: seal.ClassUnknown, Section: seal.SectionEncrypted, Preimage: true},
			[]string{"Record 1", "hashlock preimage", "not a seed", "Nothing was opened",
				"Remove that record -- and any others like it -- (records count from 0) on the host and seal the payload again."},
			[]string{"unknown format", "unreadable"},
		},
		{
			"a preimage plate at record 0 -- records count from 0",
			&seal.RecordNotPermittedError{Index: 0, Class: seal.ClassUnknown, Section: seal.SectionEncrypted, Preimage: true},
			[]string{"Record 0", "hashlock preimage"},
			[]string{"Record 1"},
		},
		{
			"a codex32 secret in the public section",
			&seal.RecordNotPermittedError{Index: 7, Class: seal.ClassCodex32Secret, Section: seal.SectionPublic},
			[]string{"Record 7", "codex32 secret", "Nothing was opened",
				"Remove that record -- and any others like it -- (records count from 0) on the host and seal the payload again."},
			[]string{"hashlock preimage", "unreadable"},
		},
		{
			// H5 §5's fit row: the LONGEST noun this body can carry
			// ("not a format this machine reads") at a two-digit index, so
			// assertModalBodyFits measures the widest arm rather than the
			// first one.
			"the longest noun at a two-digit index",
			&seal.RecordNotPermittedError{Index: 13, Class: seal.ClassUnknown, Section: seal.SectionEncrypted},
			[]string{"Record 13", "not a format this machine reads",
				"Remove that record -- and any others like it -- (records count from 0) on the host and seal the payload again."},
			[]string{"hashlock preimage"},
		},
		{
			"a record this machine does not read at all",
			&seal.RecordNotPermittedError{Index: 2, Class: seal.ClassUnknown, Section: seal.SectionEncrypted},
			[]string{"Record 2", "not a format this machine reads",
				"Remove that record -- and any others like it -- (records count from 0) on the host and seal the payload again."},
			[]string{"hashlock preimage"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := unlockNotPermittedBody(tc.err)
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("body %q does not carry %q", body, w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(body, n) {
					t.Errorf("body %q carries %q and must not", body, n)
				}
			}
			// The record's own bytes never reach a screen.
			if strings.Contains(body, guiPreimagePlate) {
				t.Error("the body carries the record itself")
			}
			// F-185's class check: a body the operator cannot read all of is
			// not a diagnosis. Measured on the renderer showError uses.
			assertModalBodyFits(t, tc.name, errorScreenBody, body)
		})
	}
}
