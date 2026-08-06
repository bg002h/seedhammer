package backup

import (
	"fmt"
	"math"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
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

// lineLayout is how the screw-hole bands and an optional QR narrow the lines of
// one text block. It exists as a named type rather than a closure so the n < 1
// clamp can be tested directly: no golden reaches it, because no descriptor
// plate carries a QR wide enough to exhaust a line's budget, yet the clamp is
// what stands between a wide QR and a panic in WrapText's width assertion.
type lineLayout struct {
	charPerLine   int // budget of an unobstructed line
	charPerQRLine int // budget of a line running beside the QR
	holeChars     int // characters lost to a screw-hole band, per side
	// qrTop and qrBottom are the code's keep-out band, [qrTop, qrBottom), in
	// PLATE-ABSOLUTE device units -- copied from the one qrPlacement the plate
	// resolved, never counted in rows off this layout's own baseY. A row index
	// is relative to whatever block the layout belongs to, and the moment two
	// blocks on one plate start at different y that index stops naming the same
	// row as the code's. An empty band (both zero) is a plate with no code.
	qrTop, qrBottom int
	charWidth       int
	fontSize        int
	baseY           int // top of the block, device units
	plateHeight     int
	innerMargin     int
}

// at returns the character budget and left inset of output line i, reproducing
// the arithmetic descriptor plates have always used -- including the clamp.
func (l lineLayout) at(i int) (n, offx int) {
	n = l.charPerLine
	// Both predicates are now asked in the same currency -- absolute y -- so
	// there is no index in this function that means one thing to the band and
	// another to the code.
	y := l.baseY + i*l.fontSize
	isQRLine := l.qrTop <= y && y < l.qrBottom
	if isQRLine {
		n = l.charPerQRLine
	}
	// Avoid screw holes on the smaller plates on the first and last lines.
	holeLine := y < l.innerMargin ||
		l.baseY+(i+1)*l.fontSize > l.plateHeight-l.innerMargin
	if holeLine {
		if !isQRLine {
			// End of line.
			n -= l.holeChars
		}
		// Beginning of line.
		n -= l.holeChars
		offx = l.holeChars * l.charWidth
	}
	if n < 1 {
		// A line always carries at least one character. Without this the
		// budget goes non-positive beside a wide QR, and WrapText's
		// widthAt >= 1 assertion turns a cramped plate into a panic.
		n = 1
	}
	return n, offx
}

// qrPlacement is where a code sits, in DEVICE units, and the band of y the text
// must keep out of. It is computed ONCE per plate (free text) or per paragraph
// (descriptor) and is read by BOTH the layout that narrows the lines and the
// engraver that draws the code, so the two cannot drift.
//
// fontSize is the size the band is quantised to. A plate that MIXES sizes has
// no single value here and carries no QR; see Fitted.Mixed.
type qrPlacement struct {
	Top, Bottom int // the narrowed band, [Top, Bottom), PLATE-ABSOLUTE
	X, Y        int // the code's own top-left corner
	Size        int // the code's side
	KeepOutX    int // horizontal space the code denies a line: Size + 2*qrBorder
}

// qrPlaceAt resolves a code's placement against a plate whose text starts at
// anchorY, in device units.
//
// The ANCHOR is the caller's decision and the reason this takes one rather than
// reading a baseY off a layout: the free-text plate's code is one object on one
// plate and is anchored at the plate's top margin whatever block a row belongs
// to, while the descriptor's code belongs to a paragraph and moves with it.
//
// KeepOutX is carried rather than re-derived because charPerQRLine is
// (width - 2*qrBorder - qrsz) / charWidth, and once a caller holds a placement
// instead of a (*qr.Code, qrScale) pair, qrBorder is recoverable only by
// inverting X. That it happens to equal params.I(2) today is a coincidence, not
// a definition.
func qrPlaceAt(params engrave.Params, qrc *qr.Code, qrScale, fontSize, anchorY int) qrPlacement {
	margin := params.I(outerMargin)
	inner := params.I(innerMargin)
	qrBorder := params.I(2)
	qrsz := qrc.Size * params.StrokeWidth * qrScale
	holeLines := int(math.Ceil(float64(inner-margin) / float64(fontSize)))
	qrLines := (qrsz + 2*qrBorder + fontSize - 1) / fontSize
	top := anchorY + holeLines*fontSize
	return qrPlacement{
		Top:      top,
		Bottom:   top + qrLines*fontSize,
		X:        params.F(plateSize) - qrsz - margin - qrBorder,
		Y:        top + (qrLines*fontSize-qrsz)/2,
		Size:     qrsz,
		KeepOutX: qrsz + 2*qrBorder,
	}
}

// textLayout builds the per-line character grid of a text block at fontSize,
// starting at baseY. It is the ONE place the screw-hole band and the QR
// narrowing are computed, so the fit check, the confirm screen and the engraver
// cannot drift apart by a character.
//
// qrp is the plate's ALREADY RESOLVED code placement, nil when there is no
// code. It is taken rather than a (*qr.Code, qrScale) pair so that this
// function cannot choose an anchor: the anchor is the caller's decision (see
// qrPlaceAt) and the band this layout narrows against is the very band the
// engraver draws the code in.
func textLayout(params engrave.Params, fnt *vector.Face, fontSize, baseY int, qrp *qrPlacement) lineLayout {
	charWidth := fixedCharWidth(fnt, fontSize)
	margin := params.I(outerMargin)
	inner := params.I(innerMargin)
	width := params.F(plateSize) - 2*margin
	l := lineLayout{
		charPerLine: width / charWidth,
		holeChars:   int(math.Ceil(float64(inner-margin) / float64(charWidth))),
		charWidth:   charWidth,
		fontSize:    fontSize,
		baseY:       baseY,
		plateHeight: params.F(plateSize),
		innerMargin: inner,
	}
	if qrp != nil {
		// KeepOutX is the code plus both its borders -- the whole width the
		// code denies a line -- which is exactly the 2*qrBorder + qrsz this
		// subtracted when it held the code itself.
		l.charPerQRLine = (width - qrp.KeepOutX) / charWidth
		l.qrTop, l.qrBottom = qrp.Top, qrp.Bottom
	}
	return l
}
