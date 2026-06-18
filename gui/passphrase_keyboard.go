package gui

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// (R0 C-1/C-2: NO "fmt" — Layout uses widget.Labelf, not stdlib fmt; NO
// "seedhammer.com/gui/layout" — this widget references no layout.* symbol.)

// Passphrase keyboard pages (case-preserving, printable-ASCII).
const (
	ppPageLower   = "qwertyuiop\nasdfghjkl\nzxcvbnm"
	ppPageUpper   = "QWERTYUIOP\nASDFGHJKL\nZXCVBNM"
	ppPageSymbols = "1234567890\n-/:;()&$@\"\n.,?!'+=_#"
)

var ppPages = [3]string{ppPageLower, ppPageUpper, ppPageSymbols}

// ppPageCycleLabel[p] is the cap shown on page p (it names the NEXT page).
var ppPageCycleLabel = [3]string{"ABC", "?123", "abc"}

type ppAction int

const (
	ppRune      ppAction = iota // commit k.r (space is ppRune with r==' ')
	ppPageCycle                 // page = (page+1)%3
	ppReveal                    // toggle revealed
	ppBackspace                 // delete last rune
)

type ppKey struct {
	r      rune   // literal char for ppRune (case as-stored); 0 otherwise
	label  string // cap for special keys (page-cycle/space); "" → render %c r
	action ppAction
	pos    image.Point // top-left within the page grid
	size   image.Point // cell glyph extent (per-key; function row varies)
	clk    Clickable
}

type PassphraseKeyboard struct {
	Fragment string
	page     int
	revealed bool

	pages [3][][]ppKey
	size  [3]image.Point

	row, col int
	inp      InputTracker
}

// NewPassphraseKeyboard builds the 3 page grids (each = the page's letter/symbol
// rows + a shared-shape function row) with per-key positions, adapting
// NewKeyboard's cell-sizing + row-centering.
func NewPassphraseKeyboard(ctx *Context) *PassphraseKeyboard {
	k := new(PassphraseKeyboard)
	style := ctx.Styles.keyboard
	cell := style.Measure(math.MaxInt, "W") // uniform letter-cell glyph extent
	const margin = 2
	letterW := cell.X + 2*keyPadX + margin
	rowH := cell.Y + 2*keyPadY + margin

	for p := 0; p < 3; p++ {
		var rows [][]ppKey
		for _, line := range strings.Split(ppPages[p], "\n") {
			var row []ppKey
			for _, r := range line {
				row = append(row, ppKey{r: r, action: ppRune, size: cell})
			}
			rows = append(rows, row)
		}
		// Function row: page-cycle, space (a ppRune with r==' '), reveal, backspace.
		fr := []ppKey{
			{label: ppPageCycleLabel[p], action: ppPageCycle},
			{r: ' ', label: "space", action: ppRune},
			{label: "show", action: ppReveal}, // label re-derived from revealed in Layout
			{action: ppBackspace},
		}
		for i := range fr {
			fr[i].size = ppKeyExtent(ctx, fr[i], cell)
		}
		rows = append(rows, fr)

		// Position: letter rows use the uniform letterW; the function row uses
		// per-key widths. Each row is horizontally centered in maxw.
		maxw := 0
		for _, row := range rows[:len(rows)-1] {
			if w := len(row) * letterW; w > maxw {
				maxw = w
			}
		}
		if w := ppRowWidth(fr, margin); w > maxw {
			maxw = w
		}
		y := 0
		for ri, row := range rows {
			if ri < len(rows)-1 { // letter row (uniform letterW)
				w := len(row) * letterW
				x := (maxw - w) / 2
				for j := range row {
					row[j].pos = image.Pt(x+j*letterW+keyPadX, y+keyPadY)
					row[j].size = cell
				}
			} else { // function row (per-key widths)
				w := ppRowWidth(row, margin)
				x := (maxw - w) / 2
				for j := range row {
					cw := row[j].size.X + 2*keyPadX + margin
					row[j].pos = image.Pt(x+keyPadX, y+keyPadY)
					x += cw
				}
			}
			y += rowH
		}
		k.pages[p] = rows
		k.size[p] = image.Pt(maxw, y-margin)
	}
	k.Clear()
	return k
}

// ppKeyExtent measures a function key's glyph extent (label, or the backspace icon).
func ppKeyExtent(ctx *Context, key ppKey, cell image.Point) image.Point {
	switch key.action {
	case ppBackspace:
		b := assets.KeyBackspace.Bounds()
		return image.Pt(b.Min.X*2+b.Dx(), cell.Y) // R0 I-1: include the Min.X margin (matches NewKeyboard gui.go:868)
	default:
		lbl := key.label
		if lbl == "" {
			lbl = string(key.r)
		}
		return image.Pt(ctx.Styles.keyboard.Measure(math.MaxInt, "%s", lbl).X, cell.Y)
	}
}

func ppRowWidth(row []ppKey, margin int) int {
	w := 0
	for _, key := range row {
		w += key.size.X + 2*keyPadX + margin
	}
	return w
}

func (k *PassphraseKeyboard) Clear() {
	k.Fragment = ""
	k.page = 0
	k.revealed = false
	rows := k.pages[k.page]
	k.row = len(rows) / 2
	k.col = len(rows[k.row]) / 2
}

// Valid: backspace valid iff Fragment non-empty; everything else always.
func (k *PassphraseKeyboard) Valid(key ppKey) bool {
	if key.action == ppBackspace {
		return k.Fragment != ""
	}
	return true
}

// commit applies a key's action.
func (k *PassphraseKeyboard) commit(key ppKey) {
	switch key.action {
	case ppRune:
		k.Fragment += string(key.r) // NO ToUpper — case preserved
	case ppBackspace:
		if k.Fragment != "" {
			_, n := utf8.DecodeLastRuneInString(k.Fragment)
			k.Fragment = k.Fragment[:len(k.Fragment)-n]
		}
	case ppPageCycle:
		k.page = (k.page + 1) % 3
		rows := k.pages[k.page]
		k.row = len(rows) / 2
		k.col = len(rows[k.row]) / 2
	case ppReveal:
		k.revealed = !k.revealed
	}
}

func (k *PassphraseKeyboard) keys() [][]ppKey { return k.pages[k.page] }

func (k *PassphraseKeyboard) Update(ctx *Context) bool {
	k.adjust()
	cur := k.keys()
	for i, row := range cur {
		for j := range row {
			key := &row[j]
			if k.Valid(*key) && key.clk.Clicked(ctx) {
				k.row, k.col = i, j
				k.commit(*key)
				return true
			}
		}
	}
	for {
		e, ok := k.inp.Next(ctx, ButtonFilter(Left), ButtonFilter(Right), ButtonFilter(Up), ButtonFilter(Down), ButtonFilter(Center), RuneFilter())
		if !ok {
			break
		}
		if e, ok := e.AsButton(); ok {
			if !e.Pressed {
				continue
			}
			cur = k.keys()
			switch e.Button {
			case Left:
				k.moveCol(-1)
			case Right:
				k.moveCol(+1)
			case Up:
				k.moveRow(-1)
			case Down:
				k.moveRow(+1)
			case Center:
				k.commit(cur[k.row][k.col])
				return true
			}
		}
		if e, ok := e.AsRune(); ok {
			// Cross-page, case-sensitive, no page switch.
			for _, page := range k.pages {
				for _, row := range page {
					for _, key := range row {
						if key.action == ppRune && key.r == e.Rune {
							k.commit(key)
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (k *PassphraseKeyboard) moveCol(d int) {
	row := k.keys()[k.row]
	next := k.col
	for {
		next = (next + d + len(row)) % len(row)
		if k.Valid(row[next]) {
			k.col = next
			k.adjust()
			return
		}
		if next == k.col { // full loop, none valid (shouldn't happen)
			return
		}
	}
}

func (k *PassphraseKeyboard) moveRow(d int) {
	rows := k.keys()
	n := len(rows)
	next := k.row
	for {
		next = (next + d + n) % n
		if k.adjustCol(next) {
			k.adjust()
			return
		}
		if next == k.row {
			return
		}
	}
}

// adjust / adjustCol: same nearest-valid-key logic as Keyboard (gui.go:1157-1209),
// over the ACTIVE page's grid (k.keys()), using ppKey.pos. No allowBackspace param
// (R0 I-2): Valid already excludes an empty-Fragment backspace, so the shared
// keyboard's allowBackspace plumbing is vestigial here.
func (k *PassphraseKeyboard) adjust() {
	rows := k.keys()
	dist := int(1e6)
	current := rows[k.row][k.col].pos
	found := false
	for i, row := range rows {
		for j, key := range row {
			if !k.Valid(key) {
				continue
			}
			d := key.pos.Sub(current)
			if d2 := d.X*d.X + d.Y*d.Y; d2 < dist {
				dist = d2
				k.row, k.col = i, j
				found = true
			}
		}
	}
	if !found {
		k.row = len(rows) - 1
		k.col = len(rows[k.row]) - 1
	}
}

func (k *PassphraseKeyboard) adjustCol(row int) bool {
	rows := k.keys()
	dist := int(1e6)
	found := false
	x := rows[k.row][k.col].pos.X
	for i, key := range rows[row] {
		if !k.Valid(key) {
			continue
		}
		found = true
		k.row = row
		d := rows[row][i].pos.X - x
		if d < 0 {
			d = -d
		}
		if d < dist {
			dist = d
			k.col = i
		}
	}
	return found
}

// Layout renders the masked/revealed readout above the active page's key grid.
// The returned image.Point is the COMBINED extent (readout + grid).
func (k *PassphraseKeyboard) Layout(ctx *Context, th *Colors) (op.Op, image.Point) {
	// Readout: masked '*'×len (default) or cleartext.
	shown := k.Fragment
	if !k.revealed {
		shown = strings.Repeat("*", utf8.RuneCountInString(k.Fragment))
	}
	readoutOp, readoutSz := widget.Labelw(&ctx.B, ctx.Styles.word, math.MaxInt, th.Text, shown)
	const readoutGap = 8

	gridY := readoutSz.Y + readoutGap
	var content op.Op
	rows := k.keys()
	for i, row := range rows {
		for j, key := range row {
			valid := k.Valid(key)
			bgcol := th.Text
			col := th.Text
			active := false
			switch {
			case !valid:
				bgcol = mulAlpha(bgcol, theme.inactiveMask)
				col = bgcol
			case i == k.row && j == k.col:
				active = true
				col = th.Background
			}
			bgsz := key.size
			bgr := image.Rectangle{Max: bgsz}
			inpOp := op.Input(&ctx.B, &k.pages[k.page][i][j].clk).Clip(bgr)
			var keyOp op.Op
			var sz image.Point
			switch {
			case key.action == ppBackspace:
				icn := assets.KeyBackspace
				sz = image.Pt(bgsz.X, icn.Bounds().Dy())
				keyOp = op.Compose(op.Color(&ctx.B, col), op.Mask(&ctx.B, icn))
			case key.label != "" && key.action == ppReveal:
				lbl := "show"
				if k.revealed {
					lbl = "hide"
				}
				keyOp, sz = widget.Labelf(&ctx.B, ctx.Styles.keyboard, col, "%s", lbl)
			case key.label != "":
				keyOp, sz = widget.Labelf(&ctx.B, ctx.Styles.keyboard, col, "%s", key.label)
			default:
				keyOp, sz = widget.Labelf(&ctx.B, ctx.Styles.keyboard, col, "%c", key.r) // NO ToUpper
			}
			keyOp = keyOp.Offset(bgsz.Sub(sz).Div(2))
			bgr.Min.X -= keyPadX
			bgr.Max.X += keyPadX
			bgr.Min.Y -= keyPadY
			bgr.Max.Y += keyPadY
			bgOp := op.Color(&ctx.B, bgcol)
			var mask op.MaskOp
			if active {
				mask = op.RoundedRect2(&ctx.B, bgr, keyCornerRadius)
			} else {
				mask = op.RoundedOutline2(&ctx.B, bgr, keyCornerRadius, keyLineWidth)
			}
			btnOp := op.Layer(inpOp, keyOp, op.Compose(bgOp, mask)).Offset(key.pos.Add(image.Pt(0, gridY)))
			content = op.Layer(content, btnOp)
		}
	}
	combined := image.Pt(max(readoutSz.X, k.size[k.page].X), gridY+k.size[k.page].Y)
	full := op.Layer(content, readoutOp.Offset(image.Pt((combined.X-readoutSz.X)/2, 0)))
	return full, combined
}
