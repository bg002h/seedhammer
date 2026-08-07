package seal

import (
	"errors"
	"fmt"
)

// §6.4 — the record container. NORMATIVE.
//
// Records separated by a single LF (0x0A). Nothing else. LF is safe as a
// separator because no constellation string contains a newline.
//
//	record[0] LF record[1] LF … LF record[n-1]
//
// Both sections use this encoding, and §6.4's `1..24` cap is over the TOTAL
// across both — which SplitSection cannot see, so the caller checks
// len(public)+len(secret). See open.go.
//
// The PUBLIC section is parsed on UNAUTHENTICATED input: §10.2 step 2 splits
// and classifies it before any tag check, and in the unencrypted shape there is
// no tag at all. Every constraint here binds it identically (§6.5).

var (
	// ErrTooManyRecords is a PREALLOCATED sentinel and the record count comes
	// back out of band, both deliberately. §11.2 requires the too-many-records
	// path to allocate ZERO times, and constructing an error that names the
	// count would allocate 1 (struct) or 3 (fmt.Errorf) — measured — which
	// destroys the only signal that distinguishes a correct pre-split scan
	// from a split-then-count mutant. The CALLER composes the message that
	// names the count and the cap, which is also the only place the
	// cross-section total is known.
	ErrTooManyRecords = errors.New("seal: too many records")
	ErrRecordTooLong  = errors.New("seal: record too long")
	ErrEmptyRecord    = errors.New("seal: empty record")
	ErrCarriageReturn = errors.New("seal: carriage return in payload")
)

// SplitSection splits an LF-joined section into records, applying every §6.4
// bound. It returns the records, the RECORD COUNT, and an error.
//
// The count is a separate return value so that the too-many-records error can
// stay a preallocated sentinel; see ErrTooManyRecords.
//
// The separator scan runs BEFORE any split. A plaintext of 8191 LF bytes
// satisfies ct_len <= 8191, and an implementation that splits first
// materialises ~8192 slice headers — ~98 KB on a 32-bit target, a fifth of the
// free heap, transiently. With ct_len == 0 that is reachable with no passphrase
// and no KDF at all.
func SplitSection(b []byte) ([]string, int, error) {
	// Pass 1: count separators only, and reject on the count. Allocation-free
	// by construction — no bytes.Count, no split, no formatted error.
	n := 1
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if n > MaxRecords {
		return nil, n, ErrTooManyRecords
	}

	// Pass 2: bound each record and reject CR. Every byte of b lies in exactly
	// one record, because LF is the only separator — so scanning the records
	// for 0x0D is "a 0x0D anywhere".
	recs := make([]string, 0, n)
	start := 0
	for i := 0; i <= len(b); i++ {
		if i < len(b) && b[i] != '\n' {
			continue
		}
		rec := b[start:i]
		idx := len(recs)
		if len(rec) == 0 {
			// Independently rejects "\n\n", a leading LF and a trailing LF,
			// which is why §6.4 states it as one rule.
			return nil, n, fmt.Errorf("%w: record %d", ErrEmptyRecord, idx)
		}
		if len(rec) > MaxRecordLen {
			return nil, n, fmt.Errorf("%w: record %d is %d bytes; must be 1..%d",
				ErrRecordTooLong, idx, len(rec), MaxRecordLen)
		}
		for _, c := range rec {
			if c == '\r' {
				return nil, n, fmt.Errorf("%w: record %d — CRLF is rejected, not tolerated",
					ErrCarriageReturn, idx)
			}
		}
		recs = append(recs, string(rec))
		start = i + 1
	}
	return recs, n, nil
}
