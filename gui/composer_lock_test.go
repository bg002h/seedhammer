package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// TestComposerDaysToUnitsMatchesTheSpecWorkedExample pins §6b's arithmetic to
// the number §8c prints: 90 days is 15188 units of 512 seconds.
func TestComposerDaysToUnitsMatchesTheSpecWorkedExample(t *testing.T) {
	if got := composerDaysToUnits(90); got != 15188 {
		t.Errorf("composerDaysToUnits(90) = %d, want 15188 (ceil(90*86400/512))", got)
	}
	if got := composerDaysToUnits(1); got != 169 {
		t.Errorf("composerDaysToUnits(1) = %d, want 169 (ceil(86400/512))", got)
	}
	// The CEILING never rounds down: a day that rounded down would encode a
	// lock shorter than the operator asked for.
	for d := uint32(1); d <= 388; d++ {
		u := composerDaysToUnits(d)
		if uint64(u)*512 < uint64(d)*86400 {
			t.Fatalf("%d days encodes %d units = %d s, short of %d s", d, u, uint64(u)*512, uint64(d)*86400)
		}
		if u == 0 || u > 65535 {
			t.Fatalf("%d days encodes %d units, outside §4c's 1..=65535", d, u)
		}
	}
}

// TestComposerLockCheckRefusesEverySection4cBoundary is §12 item 7: every
// boundary value in and out, per kind, against the DEVICE's gate.
func TestComposerLockCheckRefusesEverySection4cBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		l    md.Lock
		ok   bool
	}{
		{"blocks, one", md.Lock{Kind: md.LockOlderBlocks, Value: 1}, true},
		{"blocks, max", md.Lock{Kind: md.LockOlderBlocks, Value: 65535}, true},
		{"blocks, zero", md.Lock{Kind: md.LockOlderBlocks, Value: 0}, false},
		{"blocks, over", md.Lock{Kind: md.LockOlderBlocks, Value: 65536}, false},
		{"units, one", md.Lock{Kind: md.LockOlderUnits, Value: 1}, true},
		{"units, max", md.Lock{Kind: md.LockOlderUnits, Value: 65535}, true},
		// ZERO UNITS is the one md itself still accepts (older(0x400000), the
		// filed md-older-zero-time-units-not-refused), and BIP-68 line 46 says
		// zero units is NO LOCK. §4c makes the device refuse it independently,
		// which is the whole point of §12 item 7.
		{"units, zero", md.Lock{Kind: md.LockOlderUnits, Value: 0}, false},
		{"units, over", md.Lock{Kind: md.LockOlderUnits, Value: 65536}, false},
		{"height, one", md.Lock{Kind: md.LockAfterHeight, Value: 1}, true},
		{"height, max", md.Lock{Kind: md.LockAfterHeight, Value: 499_999_999}, true},
		{"height, zero", md.Lock{Kind: md.LockAfterHeight, Value: 0}, false},
		{"height, over", md.Lock{Kind: md.LockAfterHeight, Value: 500_000_000}, false},
		{"time, floor", md.Lock{Kind: md.LockAfterTime, Value: 500_000_000}, true},
		{"time, max", md.Lock{Kind: md.LockAfterTime, Value: 2_147_483_647}, true},
		{"time, under", md.Lock{Kind: md.LockAfterTime, Value: 499_999_999}, false},
		{"time, over", md.Lock{Kind: md.LockAfterTime, Value: 2_147_483_648}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.l.Check()
			if (err == nil) != tc.ok {
				t.Errorf("Lock{%v, %d}.Check() = %v, want ok=%v", tc.l.Kind, tc.l.Value, err, tc.ok)
			}
		})
	}
}

// TestComposerDateEntryRefusesImpossibleAndPre2009Dates is §6b's entry band
// and §8t.
func TestComposerDateEntryRefusesImpossibleAndPre2009Dates(t *testing.T) {
	for _, tc := range []struct {
		digits string
		ok     bool
		why    string
	}{
		{"20270301", true, "the §8c worked example"},
		{"20090103", true, "the floor itself is admitted"},
		{"20090102", false, "one day below the floor (§8t)"},
		{"20081231", false, "before 2009 (§8t)"},
		{"20270231", false, "2027-02-31 does not exist; time.Date would normalise it to March"},
		{"20271301", false, "month 13"},
		{"20270000", false, "day and month zero"},
		{"20380120", false, "past 2038-01-19, the Unix-time ceiling §4c's time row stops at"},
	} {
		t.Run(tc.digits, func(t *testing.T) {
			y, m, d, ok := composerParseDateDigits(tc.digits)
			if !ok {
				if tc.ok {
					t.Fatalf("%s: the digits did not parse but should have (%s)", tc.digits, tc.why)
				}
				return
			}
			unix, inBand := composerDateToUnix(y, m, d)
			if inBand != tc.ok {
				t.Fatalf("%s -> %04d-%02d-%02d, in band = %v, want %v (%s)",
					tc.digits, y, m, d, inBand, tc.ok, tc.why)
			}
			if !inBand {
				return
			}
			if err := (md.Lock{Kind: md.LockAfterTime, Value: unix}).Check(); err != nil {
				t.Errorf("%s produced %d, which the device's own §4c gate refuses: %v",
					tc.digits, unix, err)
			}
		})
	}
}

// TestComposerLockEchoesAreTheSpecStrings is §8c under the verbatim rule and
// §12 item 5's condition test for the bound and no-bound lines.
func TestComposerLockEchoesAreTheSpecStrings(t *testing.T) {
	// 1788220800 is 2026-09-01 00:00:00 UTC and 1803859200 is 2027-03-01,
	// both MEASURED (`python3 -c "import datetime; ..."`) rather than
	// transcribed: an epoch off by a day reads exactly like a correct one.
	packed := composerBound{seconds: 1788220800, hasBound: true}
	withHeight := composerBound{seconds: 1788220800, height: 905000, hasBound: true, hasHeight: true}
	none := composerBound{}

	got := strings.Join(composerLockEcho(md.Lock{Kind: md.LockOlderUnits, Value: 15188}, none), " ")
	if !strings.Contains(got, "90 days = 15188 units of 512 s (90.0 days)") {
		t.Errorf("the relative-time echo is not §8c's: %q", got)
	}
	// A RELATIVE lock is bounded by nothing: `now:` bounds dates and heights,
	// which are absolute. The bare disclaimer must not appear on it either --
	// there is nothing about the present for it to disclaim.
	if strings.Contains(got, "cannot tell the time") {
		t.Errorf("a relative lock carries a present-tense disclaimer it has no use for: %q", got)
	}

	got = strings.Join(composerLockEcho(md.Lock{Kind: md.LockAfterTime, Value: 1803859200}, packed), " ")
	if !strings.Contains(got, "2027-03-01 00:00 UTC") {
		t.Errorf("the date echo is not §8c's: %q", got)
	}
	if !strings.Contains(got, composerCopyPackedDateBound("2026-09-01")) {
		t.Errorf("the date echo does not carry the packed-date bound line: %q", got)
	}

	got = strings.Join(composerLockEcho(md.Lock{Kind: md.LockAfterHeight, Value: 905001}, withHeight), " ")
	if !strings.Contains(got, "Block 905001") {
		t.Errorf("the height echo is not §8c's: %q", got)
	}
	if !strings.Contains(got, composerCopyPackedHeightBound(905000)) {
		t.Errorf("the height echo does not carry the packed-height bound line: %q", got)
	}

	// NO now: FIELD FOR THIS KIND -> the bare disclaimer, never silence.
	got = strings.Join(composerLockEcho(md.Lock{Kind: md.LockAfterHeight, Value: 905001}, packed), " ")
	if !strings.Contains(got, composerCopyNoBound()) {
		t.Errorf("a height with a seconds-only bound must carry the BARE disclaimer: %q", got)
	}
}

// TestComposerBelowBoundRefusals is §6b's refusal and §8o, both directions,
// plus §12 item 5's fits assertions for the four refusal bodies.
func TestComposerBelowBoundRefusals(t *testing.T) {
	b := composerBound{seconds: 1788220800, height: 905000, hasBound: true, hasHeight: true}
	if composerLockBelowBound(md.Lock{Kind: md.LockAfterTime, Value: 1788220799}, b) == "" {
		t.Error("a date one second before the pack time is not refused")
	}
	if got := composerLockBelowBound(md.Lock{Kind: md.LockAfterTime, Value: 1788220801}, b); got != "" {
		t.Errorf("a date after the pack time is refused with %q", got)
	}
	if composerLockBelowBound(md.Lock{Kind: md.LockAfterHeight, Value: 904999}, b) == "" {
		t.Error("a height below the packed height is not refused")
	}
	// A field that is ABSENT bounds nothing (§6b).
	seconds := composerBound{seconds: 1788220800, hasBound: true}
	if got := composerLockBelowBound(md.Lock{Kind: md.LockAfterHeight, Value: 1}, seconds); got != "" {
		t.Errorf("a height was refused against a bound with no height field: %q", got)
	}
	for _, tc := range []struct {
		what string
		body string
	}{
		{"the §8o below-bound date refusal", composerCopyBelowBoundDate()},
		{"the §8o below-bound height refusal", composerCopyBelowBoundHeight()},
		{"the §8t date floor refusal", composerCopyDateFloor()},
		{"the §8u relative ceiling refusal", composerCopyRelativeCeiling()},
	} {
		assertModalBodyFits(t, tc.what, errorScreenBody, tc.body)
	}
}
