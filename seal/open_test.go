package seal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// The headless pipeline, driven end to end from testdata/vectors.json.

// countingKDF wraps DeriveKey and records how many times it ran. It is the
// instrument every "no KDF ran" assertion below rests on — a return-value
// assertion cannot see the difference between a bound check that ran before the
// KDF and one that ran after it, only ~31 s of wall clock can, and on a
// watchdog-less firmware that difference is a refusal versus a hang.
type countingKDF struct{ calls int }

func (c *countingKDF) derive(passphrase string, salt []byte, iterations int) []byte {
	c.calls++
	return DeriveKey(passphrase, salt, iterations)
}

func openerWithCounter() (Opener, *countingKDF) {
	c := &countingKDF{}
	return Opener{KDF: c.derive}, c
}

// ---------------------------------------------------------------------------
// A TEST-ONLY sealer. Production deliberately has no seal path — a device that
// can seal is a device that can be made to emit a payload — but the hostile
// payloads below (a 25-record split, a reordered public section) do not exist
// in the vector file and have to be built.
//
// TestSealForTestReproducesEveryVector pins it against Plan A's own bytes, so
// a defect in the harness shows up as a failing vector rather than as a
// mysteriously-passing negative.
// ---------------------------------------------------------------------------

func sealForTest(t *testing.T, public, secret []string, passphrase string, salt, iv []byte, iterations uint32) []byte {
	t.Helper()
	pub := strings.Join(public, "\n")
	sec := strings.Join(secret, "\n")
	h := Header{PubLen: uint32(len(pub))}
	if len(secret) > 0 {
		h.Iterations = iterations
		h.CtLen = uint32(len(sec))
		copy(h.Salt[:], salt)
		copy(h.IV[:], iv)
	}
	hdr := h.Encode()
	blob := append(append([]byte(nil), hdr[:]...), pub...)
	if len(secret) == 0 {
		return blob
	}
	key := DeriveKey(NormalisePassphrase(passphrase), h.Salt[:], int(h.Iterations))
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return gcm.Seal(blob, h.IV[:], []byte(sec), blob)
}

func TestSealForTestReproducesEveryVector(t *testing.T) {
	for _, v := range loadVectors(t) {
		pass := ""
		if v.Passphrase != nil {
			pass = *v.Passphrase
		}
		got := sealForTest(t, v.Public, v.Secret, pass,
			mustHex(t, v.SaltHex), mustHex(t, v.IVHex), v.Iterations)
		if h := hex.EncodeToString(got); h != v.BlobHex {
			t.Errorf("vector %s: the test sealer does not reproduce Plan A's bytes\n got %s\nwant %s",
				v.Name, h, v.BlobHex)
		}
	}
}

// ---------------------------------------------------------------------------

// Every vector, end to end.
func TestOpenDrivesEveryVector(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			o, kdf := openerWithCounter()
			pass := ""
			if v.Passphrase != nil {
				pass = *v.Passphrase
			}
			p, err := o.Open(v.Blob(t), pass)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if p.Header.PubLen != v.PubLen || p.Header.CtLen != v.CtLen {
				t.Errorf("header pub=%d ct=%d, vector declares %d/%d",
					p.Header.PubLen, p.Header.CtLen, v.PubLen, v.CtLen)
			}
			if len(p.Public) != len(v.Public) {
				t.Fatalf("%d public records, vector declares %d", len(p.Public), len(v.Public))
			}
			for i, a := range p.Public {
				if a.Record != v.Public[i] {
					t.Errorf("public record %d:\n got %q\nwant %q", i, a.Record, v.Public[i])
				}
			}
			if len(p.Secret) != len(v.Secret) {
				t.Fatalf("%d secret records, vector declares %d", len(p.Secret), len(v.Secret))
			}
			for i, a := range p.Secret {
				if a.Record != v.Secret[i] {
					t.Errorf("secret record %d:\n got %q\nwant %q", i, a.Record, v.Secret[i])
				}
			}
			// §10.2 step 3: a hash iff pub_len > 0.
			if p.HasHash != (v.PubLen > 0) {
				t.Errorf("HasHash = %v with pub_len = %d", p.HasHash, v.PubLen)
			}
			if p.HasHash {
				want := *v.PubhashUnsealed
				if v.CtLen > 0 {
					want = *v.PubhashSealed
				}
				if got := hex.EncodeToString(p.Hash[:]); got != want {
					t.Errorf("hash = %s, vector declares %s", got, want)
				}
			}
			// The KDF runs exactly once when something is encrypted, and not
			// at all when nothing is.
			wantCalls := 0
			if v.CtLen > 0 {
				wantCalls = 1
			}
			if kdf.calls != wantCalls {
				t.Errorf("KDF ran %d times, want %d", kdf.calls, wantCalls)
			}
		})
	}
}

// §11.2: vector C decrypts to 6 canonical records classified IN ORDER as ms1,
// mk1, mk1, md1, md1, md1. A bundle that split correctly but classified nothing
// would otherwise pass.
func TestVectorCClassificationSequence(t *testing.T) {
	v := vectorNamed(t, "C")
	o, _ := openerWithCounter()
	p, err := o.Open(v.Blob(t), *v.Passphrase)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := []Classification{
		ClassCodex32Secret, // ms1
		ClassMDMK,          // mk1
		ClassMDMK,          // mk1
		ClassMDMK,          // md1
		ClassMDMK,          // md1
		ClassMDMK,          // md1
	}
	if len(p.Secret) != len(want) {
		t.Fatalf("%d records, want %d", len(p.Secret), len(want))
	}
	for i := range want {
		if p.Secret[i].Class != want[i] {
			t.Errorf("record %d classified %v, want %v", i, p.Secret[i].Class, want[i])
		}
	}
	// The HRPs are also checkable, and pin the sequence to real content.
	for i, prefix := range []string{"ms1", "mk1", "mk1", "md1", "md1", "md1"} {
		if !strings.HasPrefix(p.Secret[i].Record, prefix) {
			t.Errorf("record %d does not start with %s", i, prefix)
		}
	}
}

// Without this, the mutant "move the bound checks after DeriveKey" passes every
// other test in the plan, just ~31 s slower — and on a watchdog-less firmware
// that is the difference between a refusal and a hang.
func TestBadHeaderNeverReachesTheKDF(t *testing.T) {
	a := vectorNamed(t, "A")
	e := vectorNamed(t, "E")

	mutate := func(src vector, f func([]byte)) []byte {
		b := append([]byte(nil), src.Blob(t)...)
		f(b)
		return b
	}
	putU32 := func(off int, v uint32) func([]byte) {
		return func(b []byte) { binary.BigEndian.PutUint32(b[off:off+4], v) }
	}
	putByte := func(off int, v byte) func([]byte) {
		return func(b []byte) { b[off] = v }
	}

	cases := []struct {
		name string
		blob []byte
		want error
	}{
		{"bad magic", mutate(a, putByte(0, 'X')), ErrBadMagic},
		{"bad version", mutate(a, putByte(8, 0x02)), ErrUnknownVersion},
		{"reserved not zero", mutate(a, putByte(11, 0x01)), ErrReservedNotZero},
		{"bad kdf id", mutate(a, putByte(9, 0x02)), ErrUnknownKDF},
		{"bad aead id", mutate(a, putByte(10, 0x02)), ErrUnknownAEAD},
		{"iterations 0", mutate(a, putU32(12, 0)), ErrIterations},
		{"iterations below the floor", mutate(a, putU32(12, MinIterations-1)), ErrIterations},
		{"iterations above the ceiling", mutate(a, putU32(12, MaxIterations+1)), ErrIterations},
		{"iterations at 2^32-1", mutate(a, putU32(12, ^uint32(0))), ErrIterations},
		{"pub_len over the cap", mutate(a, putU32(44, MaxSectionLen+1)), ErrPubLen},
		{"ct_len over the cap", mutate(a, putU32(48, MaxSectionLen+1)), ErrCtLen},
		// §11.2 names this one explicitly: a 32-bit native-int region-fit check
		// wraps it negative and accepts.
		{"ct_len 0xFFFFFFF0", mutate(a, putU32(48, 0xFFFFFFF0)), ErrCtLen},
		{"pub_len 0xFFFFFFF0", mutate(a, putU32(44, 0xFFFFFFF0)), ErrPubLen},
		{"empty payload", mutate(a, putU32(48, 0)), ErrEmpty},
		{"short buffer", a.Blob(t)[:HeaderLen-1], ErrTooShort},
		// §6.2's unsealed shape: with ct_len == 0 every crypto field must be
		// zero, or an attacker can stage a downgrade a later version honours.
		{"unsealed with kdf_id", mutate(e, putByte(9, 0x01)), ErrUnsealedFieldNotZero},
		{"unsealed with aead_id", mutate(e, putByte(10, 0x01)), ErrUnsealedFieldNotZero},
		{"unsealed with iterations", mutate(e, putByte(15, 0x01)), ErrUnsealedFieldNotZero},
		{"unsealed with salt", mutate(e, putByte(16, 0x01)), ErrUnsealedFieldNotZero},
		{"unsealed with iv", mutate(e, putByte(32, 0x01)), ErrUnsealedFieldNotZero},
	}
	for _, c := range cases {
		o, kdf := openerWithCounter()
		p, err := o.Open(c.blob, *a.Passphrase)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
		if kdf.calls != 0 {
			t.Errorf("%s: KDF ran %d times before the bound check", c.name, kdf.calls)
		}
		if p != nil {
			t.Errorf("%s: a rejected payload must yield nothing", c.name)
		}
	}
}

// §6.4's 1..24 cap is over the TOTAL across BOTH sections. Task 5's 25-record
// test exercises only the single-section path, so with the caller-side check
// deleted this split passes everything else.
func TestTotalRecordCapSpansBothSections(t *testing.T) {
	d := vectorNamed(t, "D")
	g := vectorNamed(t, "G")

	// 20 public records that genuinely DECODE — 9 distinct cards. An earlier
	// Rust version of this test used 20 copies of one md1 chunk, which dies in
	// the card-set decode with "chunk set incomplete" long before the cap is
	// reached, and deleting the combined-cap check left the whole suite green.
	public := append([]string(nil), g.Public...) // 12 records, 4 cards
	public = append(public, d.Public...)         // +5 records, 2 cards (distinct csids)
	public = append(public, singleMD1a, singleMD1b, "md1yqpqqxqsqgprhfjpjaz6d")
	if len(public) != 20 {
		t.Fatalf("the public half must be 20 records, got %d", len(public))
	}
	// Prove the premise: 20 public records alone are legal and decodable.
	if _, err := AdmitSection(public, SectionPublic); err != nil {
		t.Fatalf("the 20 public records must decode, or this test passes for the wrong reason: %v", err)
	}

	secret := []string{d.Secret[0], d.Secret[0], d.Secret[0], d.Secret[0], d.Secret[0]}
	blob := sealForTest(t, public, secret, *d.Passphrase,
		mustHex(t, d.SaltHex), mustHex(t, d.IVHex), d.Iterations)

	o, _ := openerWithCounter()
	p, err := o.Open(blob, *d.Passphrase)
	if !errors.Is(err, ErrTooManyRecords) {
		t.Fatalf("20 + 5 = 25 records must be refused, got %v", err)
	}
	if p != nil {
		t.Error("a rejected payload must yield nothing")
	}
	// §6.4: "The device MUST distinguish 'too many records (N, max 24)' from
	// 'payload unreadable'." record_count is authenticated plaintext, so naming
	// it leaks nothing — and conflating a too-large wallet with an attack would
	// send the operator chasing a compromise that did not happen.
	msg := err.Error()
	for _, want := range []string{"25", "24"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the error must name the count and the cap; %q lacks %q", msg, want)
		}
	}
	// 24 total is legal: drop one secret record.
	blob24 := sealForTest(t, public, secret[:4], *d.Passphrase,
		mustHex(t, d.SaltHex), mustHex(t, d.IVHex), d.Iterations)
	if _, err := o.Open(blob24, *d.Passphrase); err != nil {
		t.Errorf("24 records across both sections is legal: %v", err)
	}
}

// §10.2 step 4 / §10.2's closing rule: the passphrase prompt is conditional on
// ct_len > 0 and NOTHING else. Vector E must reach the plate list with no
// passphrase at all and no KDF — asserted on the instrument, not on the return
// value, because a scripted platform will happily feed twelve words into a
// prompt that should not exist and still reach the plate list.
func TestPublicOnlyPayloadNeedsNoPassphrase(t *testing.T) {
	e := vectorNamed(t, "E")
	for _, pass := range []string{"", "whatever the caller happens to pass"} {
		o, kdf := openerWithCounter()
		p, err := o.Open(e.Blob(t), pass)
		if err != nil {
			t.Fatalf("vector E must open with passphrase %q: %v", pass, err)
		}
		if kdf.calls != 0 {
			t.Errorf("passphrase %q: KDF ran %d times on an unsealed payload", pass, kdf.calls)
		}
		if p.NeedsPassphrase() {
			t.Error("an unsealed payload must not report that it needs a passphrase")
		}
		if len(p.Secret) != 0 {
			t.Errorf("%d secret records on an unsealed payload", len(p.Secret))
		}
		if !p.HasHash {
			t.Error("pub_len > 0 must produce a hash")
		}
	}
}

// §10.2 step 3: with pub_len == 0 NOTHING is displayed. The digest of an empty
// record set is a constant, and showing the same number on every
// fully-encrypted payload would teach the operator it is furniture.
func TestFullyEncryptedPayloadProducesNoHash(t *testing.T) {
	for _, name := range []string{"A", "B", "C", "F"} {
		v := vectorNamed(t, name)
		if v.PubLen != 0 {
			t.Fatalf("vector %s must be fully encrypted", name)
		}
		o, _ := openerWithCounter()
		p, err := o.Open(v.Blob(t), *v.Passphrase)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p.HasHash {
			t.Errorf("%s: pub_len == 0 must produce no hash, got %x", name, p.Hash)
		}
		if p.Hash != [16]byte{} {
			t.Errorf("%s: the hash field must stay zero when there is nothing to hash", name)
		}
	}
}

// A tampered public payload fails at the TAG, and the tamper is one that
// SURVIVES the allow-list. Do NOT flip a byte: a flipped byte breaks BCH, the
// record is refused at the allow-list before any key is derived, and a test
// asserting only "an error came back" then passes under the very mutant it
// exists to kill — AAD dropping the public section.
//
// Reordering keeps every record BCH-valid and every group decodable, so the
// pipeline reaches step 8 and the failure has to come from the tag.
func TestReorderedPublicSectionFailsAtTheTag(t *testing.T) {
	d := vectorNamed(t, "D")
	blob := d.Blob(t)
	// D's public section is mk1 chunk 0, mk1 chunk 1, md1 chunks 0-2. Move the
	// md1 card in front of the mk1 card: intra-card order is untouched, so
	// every group still reassembles.
	reordered := append(append([]string(nil), d.Public[2:]...), d.Public[:2]...)
	if _, err := AdmitSection(reordered, SectionPublic); err != nil {
		t.Fatalf("the reordered section must still be admissible, or this test "+
			"fires at the wrong layer: %v", err)
	}
	joined := []byte(strings.Join(reordered, "\n"))
	if len(joined) != int(d.PubLen) {
		t.Fatalf("reordering must preserve pub_len: %d vs %d", len(joined), d.PubLen)
	}
	if bytes.Equal(joined, blob[HeaderLen:HeaderLen+int(d.PubLen)]) {
		t.Fatal("the reorder did not change the section bytes")
	}
	tampered := append([]byte(nil), blob...)
	copy(tampered[HeaderLen:], joined)

	o, kdf := openerWithCounter()
	p, err := o.Open(tampered, *d.Passphrase)
	// Specifically the AUTHENTICATION error, not merely non-nil: Phase A must
	// give Phase B an error that lets it say "wrong passphrase, OR this payload
	// has been altered". Reporting only "wrong passphrase" loses the one signal
	// §2.2 item 4 exists to raise.
	if !errors.Is(err, ErrAuthentication) {
		t.Errorf("a reordered public section must fail the tag, got %v", err)
	}
	if p != nil {
		t.Error("a rejected payload must yield nothing")
	}
	if kdf.calls != 1 {
		t.Errorf("the KDF must have run exactly once to reach the tag, ran %d", kdf.calls)
	}
	// The untampered blob still opens, so the failure above is the reorder and
	// not a broken fixture.
	o2, _ := openerWithCounter()
	if _, err := o2.Open(blob, *d.Passphrase); err != nil {
		t.Errorf("the untampered vector must still open: %v", err)
	}
}

func TestWrongPassphraseFailsAtTheTag(t *testing.T) {
	a := vectorNamed(t, "A")
	o, _ := openerWithCounter()
	_, err := o.Open(a.Blob(t), "abandon abandon abandon abandon abandon abandon "+
		"abandon abandon abandon abandon abandon about")
	if !errors.Is(err, ErrAuthentication) {
		t.Errorf("a wrong passphrase must fail the tag, got %v", err)
	}
}

// A space-grouped record — `mnemonic bundle`'s DISPLAY form — rejects the whole
// bundle. codex32's inputChar has no mapping for 0x20, so ValidMD/ValidMK
// return false and the allow-list refuses it. This is the exact defect that
// shipped in an earlier draft of vector C.
func TestSpaceGroupedRecordsRejectTheBundle(t *testing.T) {
	d := vectorNamed(t, "D")
	for _, bad := range []string{
		"md1fv9w jpqpqpm6jzzqqvqpdqnf4ztqq4gy99tzyzyzdv7xh9vpdwu3t7dhhesk2tl3",
		"md1fv9w-jpqpqpm6jzzqqvqpdqnf4ztqq4gy99tzyzyzdv7xh9vpdwu3t7dhhesk2tl3",
	} {
		if Classify(bad) != ClassUnknown {
			t.Errorf("%q must not classify as a card", bad)
		}
		blob := sealForTest(t, replaceAt(d.Public, 2, bad), d.Secret, *d.Passphrase,
			mustHex(t, d.SaltHex), mustHex(t, d.IVHex), d.Iterations)
		o, _ := openerWithCounter()
		p, err := o.Open(blob, *d.Passphrase)
		if !errors.Is(err, ErrRecordNotPermitted) {
			t.Errorf("%q must reject the bundle, got %v", bad, err)
		}
		if p != nil {
			t.Error("a rejected payload must yield nothing")
		}
	}
}

// A `command: lock-boot` record in position 3 of 6, inside the ENCRYPTED
// section — authenticated plaintext, which is not the same as safe.
func TestEncryptedDebugCommandRejectsTheBundle(t *testing.T) {
	c := vectorNamed(t, "C")
	recs := replaceAt(c.Secret, 2, "command: lock-boot")
	blob := sealForTest(t, nil, recs, *c.Passphrase,
		mustHex(t, c.SaltHex), mustHex(t, c.IVHex), c.Iterations)
	o, _ := openerWithCounter()
	p, err := o.Open(blob, *c.Passphrase)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("a debug command in the plaintext must reject the bundle, got %v", err)
	}
	if p != nil {
		t.Error("a rejected payload must yield nothing — nothing may be engraved")
	}
}

// §6.4's container rules bind the decrypted section too.
func TestMalformedEncryptedContainerRejectsTheBundle(t *testing.T) {
	c := vectorNamed(t, "C")
	base := strings.Join(c.Secret, "\n")
	for _, m := range []struct {
		name string
		body string
		want error
	}{
		{"trailing LF", base + "\n", ErrEmptyRecord},
		{"leading LF", "\n" + base, ErrEmptyRecord},
		{"empty record", strings.Replace(base, "\n", "\n\n", 1), ErrEmptyRecord},
		{"carriage return", strings.Replace(base, "\n", "\r\n", 1), ErrCarriageReturn},
	} {
		blob := sealForTest(t, nil, []string{m.body}, *c.Passphrase,
			mustHex(t, c.SaltHex), mustHex(t, c.IVHex), c.Iterations)
		o, _ := openerWithCounter()
		p, err := o.Open(blob, *c.Passphrase)
		if !errors.Is(err, m.want) {
			t.Errorf("%s: got %v, want %v", m.name, err, m.want)
		}
		if p != nil {
			t.Errorf("%s: a rejected payload must yield nothing", m.name)
		}
	}
}

// §8.1: lowercase, single-space separated, no leading or trailing space. Host
// and device MUST produce byte-identical KDF input.
func TestNormalisePassphrase(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"  BEEF   beef\tbeef  ", "beef beef beef"},
		{"beef beef", "beef beef"},
		{"\nBEEF\n", "beef"},
		{"", ""},
	} {
		if got := NormalisePassphrase(c.in); got != c.want {
			t.Errorf("NormalisePassphrase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// It reaches the KDF: a passphrase differing only in case and spacing must
	// open vector A.
	a := vectorNamed(t, "A")
	o, _ := openerWithCounter()
	if _, err := o.Open(a.Blob(t), "  "+strings.ToUpper(*a.Passphrase)+"  "); err != nil {
		t.Errorf("a de-normalised passphrase must still open the payload: %v", err)
	}
}

// The Reader seam and the pipeline compose: bytes off "flash" straight into
// Open.
func TestOpenFromAReader(t *testing.T) {
	d := vectorNamed(t, "D")
	region := append(append([]byte(nil), d.Blob(t)...), bytes.Repeat([]byte{0xFF}, 256)...)
	r := FileReader{Path: writeRegion(t, region)}
	raw, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	o, _ := openerWithCounter()
	p, err := o.Open(raw, *d.Passphrase)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := FormatHash(p.Hash); got != "a26e d22b b747 dfd0 2367 06ad 14c1 9679" {
		t.Errorf("displayed hash = %q", got)
	}
}

// A nil KDF must fall back to the real one rather than panic or silently derive
// nothing — Phase B constructs an Opener{} with no seam.
func TestZeroOpenerUsesTheRealKDF(t *testing.T) {
	a := vectorNamed(t, "A")
	p, err := Opener{}.Open(a.Blob(t), *a.Passphrase)
	if err != nil {
		t.Fatalf("Opener{}.Open: %v", err)
	}
	if len(p.Secret) != 1 || p.Secret[0].Record != a.Secret[0] {
		t.Error("the zero Opener must decrypt exactly as the instrumented one does")
	}
}
