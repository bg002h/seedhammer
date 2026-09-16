package gui

import (
	"testing"

	"seedhammer.com/hashlock"
	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// F-535: the `phrase:` record carries the METHOD axis and NOT the kind axis,
// and F-573: an ms1 preimage plate record carries X and no kind either. Neither
// is a wire defect — a preimage is kind-agnostic by construction, the same 32
// bytes hashing under all four kinds, which is exactly why the record does not
// name one.
//
// What WAS a defect is that the device filled the gap with an assumption:
// `hashlockLockOf(md.KindSha256, &x)` at four sites. On a hash256, ripemd160 or
// hash160 payload that assigned the WRONG KIND to the operator's own material —
// the plate would carry a kind the wallet does not use.
//
// The payload's own `hash:` records are what disambiguate, and they were in
// scope at every one of those sites.
//
// MUTATION: put `md.KindSha256` back at any of the four sites -> the matching
// subtest below fails. Before the fix ALL 1340 gui tests passed, which is why
// this file exists: the assumption was load-bearing and untested.
func TestPayloadMaterialTakesItsKindFromTheHashRecord(t *testing.T) {
	const phrase = "correct horse battery staple"
	x := hashlock.PreimageSHA256([]byte(phrase))

	for _, kind := range []md.HashKind{
		md.KindSha256, md.KindHash256, md.KindRipemd160, md.KindHash160,
	} {
		t.Run(kind.Token(), func(t *testing.T) {
			// The payload states THIS kind's digest of THIS preimage, which is
			// the only thing that can say which kind the material is for.
			want := hashlockLockOf(kind, &x)
			sess := composerSessionWith(
				[]string{sysw.HashRecord(want)},
				[]string{
					composerTestPreimageRecord(t, x),
					composerTestPhraseRecord(sysw.HashlockSHA256, phrase),
				},
			)

			// F-573: the preimage band.
			pres := composerPayloadPreimages(sess)
			if len(pres) != 1 {
				t.Fatalf("payload preimages = %d, want 1", len(pres))
			}
			if got := pres[0].digest.Kind(); got != kind {
				t.Errorf("preimage band kind = %s, want %s (the payload states it)",
					got.Token(), kind.Token())
			}
			if !pres[0].digest.Equal(want) {
				t.Errorf("preimage band digest does not equal the payload's own hash: record")
			}

			// F-573: the Hashlock-plates flow's preimage record.
			recs := hashlockPlatesRecords(sess)
			var sawPreimage bool
			for _, r := range recs {
				if r.derived && r.preimage == x {
					sawPreimage = true
					if got := r.digest.Kind(); got != kind {
						t.Errorf("plates preimage kind = %s, want %s", got.Token(), kind.Token())
					}
				}
			}
			if !sawPreimage {
				t.Fatalf("the plates flow listed no derived preimage record")
			}

			// The helper itself, directly: it is the single place the four
			// sites now agree through.
			if got := hashlockKindFromPayload(&x, composerPayloadDigests(sess)); got != kind {
				t.Errorf("hashlockKindFromPayload = %s, want %s", got.Token(), kind.Token())
			}
		})
	}
}

// The control that keeps the sweep honest: with NO hash: record to match, the
// kind is unknowable and the fallback is sha256 — the prior behaviour, and the
// honest answer, since the confirm modal's relation line already says the
// payload states no matching digest.
func TestPayloadMaterialWithNoHashRecordFallsBackToSha256(t *testing.T) {
	x := hashlock.PreimageSHA256([]byte("a phrase whose digest is in no record"))
	sess := composerSessionWith(nil, []string{composerTestPreimageRecord(t, x)})

	if got := hashlockKindFromPayload(&x, composerPayloadDigests(sess)); got != md.KindSha256 {
		t.Errorf("fallback kind = %s, want sha256", got.Token())
	}
	pres := composerPayloadPreimages(sess)
	if len(pres) != 1 || pres[0].digest.Kind() != md.KindSha256 {
		t.Errorf("a payload with no hash: record should still draw sha256")
	}
}
