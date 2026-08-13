package gui

import (
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// eofNFC is a reader parked at EOF -- exactly what every one-shot tag source
// becomes once its record has been read, and what the emulator's nfcSource
// hands out for the whole life of a screen after the page's tag is consumed.
type eofNFC struct{ reads atomic.Int64 }

func (r *eofNFC) Read([]byte) (int, error) { r.reads.Add(1); return 0, io.EOF }
func (r *eofNFC) Close() error             { return nil }

// F-126. The scan loop yielded only on scanFailed, so a reader parked at EOF was
// polled as fast as the reader could answer. This asserts THAT, and nothing
// about what it costs on the device or in a browser -- those are F-126's claims
// and are not reproduced here.
//
// The bound is deliberately loose: with the wait this is a handful of reads,
// without it ~198,000 in the same window, so any threshold between the two
// separates them and a tight one would only add flake.
func TestNFCScannerDoesNotSpinAtEOF(t *testing.T) {
	r := new(eofNFC)
	ctx := NewContext(newPlatform())
	_, stop := startScanner(ctx, r)
	time.Sleep(150 * time.Millisecond)
	stop()
	n := r.reads.Load()
	t.Logf("%d reads in 150ms", n)
	if n == 0 {
		t.Fatal("INCONCLUSIVE: the reader was never polled, so this test cannot " +
			"tell a backoff from a scanner that never ran")
	}
	if n > 100 {
		t.Errorf("%d reads in 150ms: the loop is spinning at EOF (F-126)", n)
	}
}

// A reader with something to say is NOT slowed down: the backoff must key on a
// poll that produced nothing, not on every poll.
func TestNFCScannerStillDeliversATag(t *testing.T) {
	const rec = "text:48656c6c6f"
	ctx := NewContext(newPlatform())
	scans, stop := startScanner(ctx, &oneShotNFC{rec: []byte(rec)})
	defer stop()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-scans:
			// The first delivery is the in-progress report (nil object): Scan
			// sees the record with a nil error and only parses on the following
			// EOF. Read on rather than asserting the head of the channel.
			if s.Object == nil {
				continue
			}
			if _, ok := s.Object.(freeTextScan); !ok {
				t.Fatalf("scanned %T (%v), want freeTextScan", s.Object, s.Object)
			}
			return
		case <-deadline:
			t.Fatal("the scanner delivered no object")
		}
	}
}

// oneShotNFC hands out a record once and then reports EOF forever, which is what
// a tag crossing the reader looks like and what cmd/emu's nfcSource models.
type oneShotNFC struct {
	rec  []byte
	done bool
}

func (r *oneShotNFC) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.rec), nil
}

func (r *oneShotNFC) Close() error { return nil }

// Stage 10a. The two record forms §5.3 added were invented for the payload, but
// §2.1 made NFC-for-everything a deliberate capability and §3.1 is NORMATIVE
// that the seam offers Scanned -- so a tag carrying one of them must parse.
// Until now both fell to errScanUnknownFormat.
func TestScanAcceptsTheTwoNewRecordForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  string
		want any
	}{
		// "Hello, World!" -- a space, which EPD §6.4 forbids raw and which is
		// the whole reason the body is hex.
		{"text", "text:48656c6c6f2c20576f726c6421", freeTextScan("Hello, World!")},
		// A newline, the RECORD SEPARATOR itself, inside the body.
		{"text with the separator", "text:610a62", freeTextScan("a\nb")},
		{"pass", "pass:6162616e646f6e2061626f7574", passScan("abandon about")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := new(scanner)
			obj, err := s.Scan(&oneShotNFC{rec: []byte(tc.rec)})
			if err != nil {
				// The first Read returns the record with a nil error, which Scan
				// reports as progress; the second hits EOF and parses.
				if err != errScanInProgress {
					t.Fatalf("first Scan: %v", err)
				}
				obj, err = s.Scan(&eofNFC{})
			}
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			switch want := tc.want.(type) {
			case freeTextScan:
				got, ok := obj.(freeTextScan)
				if !ok || got != want {
					t.Fatalf("scanned %T(%v), want freeTextScan(%q)", obj, obj, string(want))
				}
			case passScan:
				got, ok := obj.(passScan)
				if !ok || string(got) != string(want) {
					t.Fatalf("scanned %T(%v), want passScan(%q)", obj, obj, string(want))
				}
			}
		})
	}
}

// §5.3.1: the prefixes are RESERVED. A record beginning text:/pass: whose body
// is not valid LOWERCASE hex is refused outright -- it must never fall through
// to the sniffers below it and be admitted as some other class, and it must
// never be treated as free text, which would turn a malformed record into an
// engraved plate.
//
// NOT here: a bare "pass:" or "text:". An EMPTY body is valid lowercase hex --
// of zero bytes -- so both codecs accept it and classify it, and refusing it in
// the scanner alone would put a rule in one of the two places §12 exists to
// prevent. What an empty record is worth is a question for the consuming flow.
func TestScanReservesTheTwoPrefixes(t *testing.T) {
	for _, rec := range []string{
		"text:zzzz",       // not hex at all
		"text:48656C6C6F", // UPPERCASE hex: two spellings, two digests
		"pass:abc",        // odd length
		"text:48656c6c6",  // truncated mid-byte: an odd nibble count
	} {
		t.Run(rec, func(t *testing.T) {
			s := new(scanner)
			if _, err := s.Scan(&oneShotNFC{rec: []byte(rec)}); err != errScanInProgress {
				t.Fatalf("first Scan: %v", err)
			}
			obj, err := s.Scan(&eofNFC{})
			if err != errScanUnknownFormat {
				t.Fatalf("Scan(%q) = (%T %v, %v), want errScanUnknownFormat", rec, obj, obj, err)
			}
			if obj != nil {
				t.Errorf("a reserved prefix produced an object: %T(%v)", obj, obj)
			}
		})
	}
}
