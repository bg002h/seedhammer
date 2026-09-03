package gui

import (
	"fmt"
	"strconv"
	"time"

	"seedhammer.com/md"
)

// Lock entry (SPEC §6b, C11, C24, C25): kind, then unit, then digits, then an
// echo the operator can check.
//
// THE OPERATOR NEVER TYPES A RAW OPERAND. They type a count of blocks, a
// count of days, a block height, or eight date digits; the encoding is
// computed here. A raw `older(4210836)` on a screen is a number no one can
// check, and §4c's four bands overlap in the operand space -- 26280 is
// equally `older(26280)` and `after(26280)` -- so the KIND has to come from
// the operator's choice rather than from the number.
//
// THE DEVICE ENFORCES §4c ITSELF (§4c, §9 item 3, §12 item 7). md today
// accepts `older(0x400000)` -- zero time units, which BIP-68 line 46 defines
// as no lock at all (filed md-older-zero-time-units-not-refused) -- so every
// value produced here goes through md.Lock.Check before it is stored, and the
// entry bounds are the same bands where the operator meets them.

// composerDaysToUnits is §6b's days-to-units conversion, rounding UP: a day
// that rounded down would encode a lock shorter than the operator asked for.
func composerDaysToUnits(days uint32) uint32 {
	return uint32((uint64(days)*86400 + 511) / 512)
}

// The date-entry band (§6b). The floor is NOT §4c's operand floor: any date
// whose 00:00 UTC value is below 500,000,000 encodes as a block HEIGHT rather
// than a time (1985-11-05 00:00 UTC is 499,996,800), so the entry stops at
// 2009-01-03 and says so with §8t. The ceiling is the Unix-time ceiling
// §4c's time row stops at.
const (
	composerDateFloorUnix   uint32 = 1230940800 // 2009-01-03 00:00:00 UTC
	composerDateCeilingUnix uint32 = 2147472000 // 2038-01-19 00:00:00 UTC
)

// composerParseDateDigits splits a YYYYMMDD field. The pad types no
// separators (§6b), so the field is fixed-width and its shape is checked here
// rather than by a parser that would accept "2027-3-1".
func composerParseDateDigits(s string) (y, m, d int, ok bool) {
	if len(s) != 8 {
		return 0, 0, 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, 0, 0, false
		}
	}
	y, _ = strconv.Atoi(s[0:4])
	m, _ = strconv.Atoi(s[4:6])
	d, _ = strconv.Atoi(s[6:8])
	return y, m, d, true
}

// composerDateExists reports whether a calendar date is real.
//
// IT IS SEPARATE FROM THE BAND CHECK, and that separation is the fix for a
// live defect: composerDateToUnix returns (0, false) for EVERY failure --
// impossible date, below the floor, above the ceiling alike -- so a dispatch
// that tried to tell them apart by reading its uint32 result (`y > 2038 ||
// u == 0`) was a tautology, true on every failure, and "that date does not
// exist" was dead code. 2027-02-31 got the CEILING message, advising a block
// height for a date that will never exist at any height (F-458).
//
// The round trip is the test rather than a month-length table, for the reason
// composerDateToUnix states: time.Date NORMALISES, so 2027-02-31 becomes
// 2027-03-03 and would otherwise be silently accepted as a different date.
func composerDateExists(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == m && t.Day() == d
}

// composerDateToUnix converts a calendar date to its 00:00:00 UTC Unix time
// and reports whether it is inside the entry band.
//
// IMPOSSIBLE DATES ARE CAUGHT BY THE ROUND TRIP, not by a month-length table.
// time.Date NORMALISES: 2027-02-31 becomes 2027-03-03 and would otherwise be
// silently accepted as a different date than the one typed. Comparing the
// components back is exact and needs no leap-year rule of its own.
func composerDateToUnix(y, m, d int) (uint32, bool) {
	if !composerDateExists(y, m, d) {
		return 0, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	u := t.Unix()
	if u < int64(composerDateFloorUnix) || u > int64(composerDateCeilingUnix) {
		return 0, false
	}
	return uint32(u), true
}

// composerLockBoundLine is §6b's bound line: the pack date or height when the
// relevant now: field is present, the bare disclaimer when it is not, and
// NOTHING for a relative lock, which nothing about the present bounds.
//
// The copy never says "now" and never withdraws the disclaimer, because a
// stale now: record can only weaken the below-bound refusal, never invent
// one.
func composerLockBoundLine(b composerBound, kind md.LockKind) string {
	switch kind {
	case md.LockOlderBlocks, md.LockOlderUnits:
		return ""
	case md.LockAfterTime:
		if b.hasBound {
			return composerCopyPackedDateBound(b.packDate())
		}
	case md.LockAfterHeight:
		if b.hasHeight {
			return composerCopyPackedHeightBound(b.height)
		}
	}
	return composerCopyNoBound()
}

// composerLockEcho is what the operator reads back: §8c's echo for the kind,
// then the bound line.
func composerLockEcho(l md.Lock, b composerBound) []string {
	var head string
	switch l.Kind {
	case md.LockOlderBlocks:
		head = composerCopyLockEchoBlocks(l.Value)
	case md.LockOlderUnits:
		head = composerCopyLockEchoDays(composerUnitsToDays(l.Value), l.Value)
	case md.LockAfterHeight:
		head = composerCopyLockEchoHeight(l.Value)
	case md.LockAfterTime:
		t := time.Unix(int64(l.Value), 0).UTC()
		head = composerCopyLockEchoDate(t.Year(), int(t.Month()), t.Day())
	}
	out := []string{head}
	if line := composerLockBoundLine(b, l.Kind); line != "" {
		out = append(out, line)
	}
	return out
}

// composerLockBelowBound returns the §8o body when the lock is below the
// payload's bound, or "" when it is not. A field that is ABSENT bounds
// nothing (§6b), which is why each arm checks its own presence flag.
func composerLockBelowBound(l md.Lock, b composerBound) string {
	switch l.Kind {
	case md.LockAfterTime:
		if b.hasBound && l.Value < b.seconds {
			return composerCopyBelowBoundDate()
		}
	case md.LockAfterHeight:
		if b.hasHeight && l.Value < b.height {
			return composerCopyBelowBoundHeight()
		}
	}
	return ""
}

// composerLockAccept is the ONE gate every entered lock passes: §4c through
// md.Lock.Check, then the payload bound. Both refusals name what to do
// instead and print no encoding (§11).
func composerLockAccept(ctx *Context, th *Colors, l md.Lock, b composerBound) bool {
	if err := l.Check(); err != nil {
		// Reached only by a bound this file and md disagree about, which is a
		// defect rather than an operator error -- so it says so instead of
		// showing a §8 line that would misdescribe it.
		showError(ctx, th, "Time lock", "This device will not write that lock value.")
		return false
	}
	if body := composerLockBelowBound(l, b); body != "" {
		showError(ctx, th, "Time lock", body)
		return false
	}
	return true
}

// composerLockEdit is §6b's kind, unit, digits, echo.
func composerLockEdit(ctx *Context, th *Colors, st *composerState, idx int) bool {
	title := fmt.Sprintf("Path %d lock", idx+1)
	kindCS := &ChoiceScreen{
		Title:   title,
		Lead:    "What kind of time lock?",
		Choices: []string{"None", "After a wait", "After a date or height"},
	}
	kindSel, ok := kindCS.Choose(ctx, th)
	if !ok {
		return false
	}
	if kindSel == 0 {
		st.list.Paths[idx].Lock = nil
		return true
	}

	var lock md.Lock
	if kindSel == 1 {
		unitCS := &ChoiceScreen{Title: title, Lead: "Measured how?", Choices: []string{"Blocks", "Days"}}
		unitSel, ok := unitCS.Choose(ctx, th)
		if !ok {
			return false
		}
		if unitSel == 0 {
			frag, ok := composerDigitEntry(ctx, th, title, "How many blocks?", 5, func(s string) (string, bool) {
				// AN EMPTY FIELD IS NOT A CEILING BREACH. Returning §8u for
				// every unparseable fragment meant the operator read
				// "Relative locks reach at most 455 days..." before typing a
				// digit -- a refusal for a limit they had not approached. The
				// date and height pads already say what to type first.
				return composerBlocksBandEcho(s)
			})
			if !ok {
				return false
			}
			n, _ := strconv.ParseUint(frag, 10, 32)
			lock = md.Lock{Kind: md.LockOlderBlocks, Value: uint32(n)}
		} else {
			frag, ok := composerDigitEntry(ctx, th, title, "How many days?", 3, func(s string) (string, bool) {
				return composerDaysBandEcho(s)
			})
			if !ok {
				return false
			}
			n, _ := strconv.ParseUint(frag, 10, 32)
			lock = md.Lock{Kind: md.LockOlderUnits, Value: composerDaysToUnits(uint32(n))}
		}
	} else {
		absCS := &ChoiceScreen{Title: title, Lead: "Named how?", Choices: []string{"A date", "A block height"}}
		absSel, ok := absCS.Choose(ctx, th)
		if !ok {
			return false
		}
		if absSel == 0 {
			frag, ok := composerDigitEntry(ctx, th, title, "Date as YYYYMMDD", 8, func(s string) (string, bool) {
				y, m, d, parsed := composerParseDateDigits(s)
				if !parsed {
					return "eight digits, YYYYMMDD", false
				}
				_, inBand := composerDateToUnix(y, m, d)
				if !inBand {
					// THE THREE FAILURES ARE TOLD APART BY WHAT THEY ARE, not
					// by reading the returned operand. `u` is 0 on every
					// failure path, so `y > 2038 || u == 0` was a tautology and
					// "that date does not exist" was dead code -- 2027-02-31
					// got the CEILING message, advising a block height for a
					// date that no height makes real (F-458).
					if !composerDateExists(y, m, d) {
						return "that date does not exist", false
					}
					if y < 2009 {
						return composerCopyDateFloor(), false
					}
					// A DATE PAST THE CEILING EXISTS; the build will not write
					// it as a TIME lock. §4d's first archetype,
					// simple-timelocked-inheritance, is exactly where a
					// twenty-year date is the ordinary case, so the operator is
					// told the limit and the alternative rather than being left
					// to retype.
					return composerCopyDateCeiling(), false
				}
				// THE RAW OPERAND IS NOT SHOWN. §6b's premise is that the
				// operator never types one, and the echo screen after this
				// prints §8c's clean form, so the number added nothing anyone
				// could check.
				return composerCopyLockEchoDate(y, m, d), true
			})
			if !ok {
				return false
			}
			y, m, d, _ := composerParseDateDigits(frag)
			u, _ := composerDateToUnix(y, m, d)
			lock = md.Lock{Kind: md.LockAfterTime, Value: u}
		} else {
			frag, ok := composerDigitEntry(ctx, th, title, "Block height", 9, func(s string) (string, bool) {
				return composerHeightBandEcho(s)
			})
			if !ok {
				return false
			}
			n, _ := strconv.ParseUint(frag, 10, 64)
			lock = md.Lock{Kind: md.LockAfterHeight, Value: uint32(n)}
		}
	}

	if !composerLockAccept(ctx, th, lock, st.bound) {
		return false
	}
	// THE ECHO IS A CONFIRM, not a notice: §6b's "kind, unit, digits, echo"
	// ends with the operator reading it back, and Back here discards the lock
	// rather than storing an operand nobody agreed to.
	if !composerReadScreen(ctx, th, title, composerLockEcho(lock, st.bound)) {
		return false
	}
	st.list.Paths[idx].Lock = &lock
	return true
}

// The three RELATIVE/ABSOLUTE entry bands, named rather than written inline in
// composerLockEdit's closures (review r0 M-1).
//
// They bound what the operator can TYPE. md.Lock.Check is the emitter's gate
// and catches an out-of-band value either way (§12 item 7), so a drift here
// costs no wrong plate -- it costs the COPY: composerLockAccept would draw
// "This device will not write that lock value.", the builder-defect line,
// instead of §8u or the band hint, at an operator who typed one too many. As
// closures they had no caller a test could reach, so widening any of them by
// one left the whole suite green.
//
// AN EMPTY FIELD SAYS WHAT TO TYPE, not what is too much (§8u, journey M-2):
// echoing the ceiling at someone who has typed nothing answers a question they
// have not asked.

func composerBlocksBandEcho(s string) (string, bool) {
	if s == "" {
		return "1 to 65535 blocks", false
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || n < 1 || n > 65535 {
		return composerCopyRelativeCeiling(), false
	}
	return composerCopyLockEchoBlocks(uint32(n)), true
}

func composerDaysBandEcho(s string) (string, bool) {
	if s == "" {
		return "1 to 388 days", false
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || n < 1 || n > 388 {
		return composerCopyRelativeCeiling(), false
	}
	return composerCopyLockEchoDays(uint32(n), composerDaysToUnits(uint32(n))), true
}

func composerHeightBandEcho(s string) (string, bool) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n < 1 || n > 499_999_999 {
		return "1 to 499999999", false
	}
	return composerCopyLockEchoHeight(uint32(n)), true
}
