package backup

import (
	"fmt"
	"strings"
)

// WrapText lays out s into engraved lines at a fixed pitch.
//
// widthAt is indexed by OUTPUT line (0 = the first line WrapText emits), NOT by
// plate row. The caller supplies the plate-row offset inside the closure: the
// free-text plate passes i+1 when a title occupies row 0; the descriptor callers
// pass their paragraph's own base. Getting this wrong puts the first text line
// on the title's row, or makes the fit check and the engraving disagree by one.
// widthAt MUST return >= 1 for every output line 0 <= i < maxLines; WrapText
// asserts this.
// Returns ok=false if the composition needs more than maxLines lines; callers
// MUST NOT engrave a false result.
//
// The screen renders these lines, the engraver engraves these lines, and the
// fit check counts these lines -- one function called three times, never three
// implementations that ought to agree.
//
// Returned lines contain no '\n': each is engraved by its own engrave.String
// call at its own (offx, offy).
func WrapText(s string, widthAt func(line int) int, maxLines int) (lines []string, ok bool) {
	// widthAt is asserted where it is used rather than up front for all
	// maxLines: the descriptor path is deliberately unbounded, so an eager
	// sweep would run forever, and a width that is only wrong on a line the
	// text never reaches is not a defect.
	width := func(line int) int {
		w := widthAt(line)
		if w < 1 {
			// A zero width consumes nothing and appends forever. On a device
			// with no OOM killer that is a hang mid-flow, not a crash.
			panic(fmt.Errorf("backup: widthAt(%d) = %d, must be >= 1", line, w))
		}
		return w
	}

	// Rule 1: split into blocks on '\n'. A block boundary is a wrap-time
	// concept only -- it never becomes a backup.Paragraph, whose blank-line
	// advance is 1mm rather than a full row.
	for _, block := range strings.Split(s, "\n") {
		if block == "" {
			// An empty block emits one empty line occupying a full row.
			if len(lines) >= maxLines {
				return lines, false
			}
			lines = append(lines, "")
			continue
		}
		for pos := 0; pos < len(block); {
			if len(lines) >= maxLines {
				return lines, false
			}
			w := width(len(lines))
			// end is what this line keeps, next is where the following line
			// resumes. They differ by exactly the space run a break consumes.
			end, next := pos, pos
			committed := false
			for cur := pos; cur < len(block); {
				sp := cur
				for sp < len(block) && block[sp] == ' ' {
					sp++
				}
				wd := sp
				for wd < len(block) && block[wd] != ' ' {
					wd++
				}
				if wd == sp {
					// A space run with no word after it: end of block. Keep it
					// so rule (c) can strip it, and finish the line.
					end, next = wd, wd
					break
				}
				if wd-pos <= w {
					// Rule 3: greedy fill. The run of spaces before this word
					// is interior, so rule (b) preserves it verbatim.
					end, next = wd, wd
					committed = true
					cur = wd
					continue
				}
				if !committed {
					// Rule 4: the token is alone at the start of the line and
					// does not fit, so break at EXACTLY w. This is also what
					// makes the descriptor path (no spaces at all) reduce to
					// the txt[:n] slice EngraveText used to do.
					end, next = pos+w, pos+w
					break
				}
				// Rule 5(a): break here, consuming exactly the run of spaces at
				// the break point -- block[cur:sp].
				next = sp
				break
			}
			// Rule 5(c): trailing spaces are stripped from every emitted line,
			// which wins over 5(b) at end of line. A line whose whole content
			// is a space run is emitted empty.
			lines = append(lines, strings.TrimRight(block[pos:end], " "))
			pos = next
		}
	}
	return lines, true
}
