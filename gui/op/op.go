package op

import (
	"image"
	"image/color"
	"math"
	"slices"

	"golang.org/x/image/draw"
	"seedhammer.com/font/bitmap"
	"seedhammer.com/image/rgb565"
)

type ImageHandle struct {
	scratch [2]ParameterizedImage
}

type Tag any

type Op struct {
	op
}

type MaskOp struct {
	op
}

type Buffer struct {
	args []uint32
	refs []any
	// zeroedArrays/zeroedEntries count the backing arrays this Buffer has
	// OUTGROWN and zeroed at the moment it outgrew them.
	//
	// append REPLACES the array on reallocation, and the old one -- still
	// holding every rune written into it -- becomes garbage that Scrub can
	// never reach, because Scrub only ever sees the CURRENT array. Measured on
	// a rendered 24-word seed frame before this existed: thirteen words came
	// back verbatim and in index order out of an array Residue() scored 0.
	//
	// Zeroing at the reallocation rather than recording the array for a later
	// Scrub is what keeps the fix free: append has already copied the elements
	// out, no op aliases an outgrown array (ops carry INDICES, and Drawer.draw
	// snapshots the current arrays at entry -- nothing appends during a draw),
	// so the old array is dead the instant append returns. Retaining it
	// instead would hold 35.5 KB per 24-word frame, which is both a real cost
	// on a 283 K-free device and indistinguishable from F-109's open ~35 K.
	//
	// These two counters exist so a test can prove the zeroing RAN. Without
	// them the property is invisible: a correctly-zeroed orphan and an orphan
	// that was never created look identical from outside.
	zeroedArrays  int
	zeroedEntries int
}

// appendArgs is the ONLY route by which b.args grows, and appendRefs the only
// route for b.refs. Everything in this file goes through them; `git grep
// '\.args = append'` outside these two functions must return nothing.
//
// They detect reallocation AFTER the fact rather than predicting it. A
// predict-ahead form (`if cap-len < n`) has to be told how many elements are
// coming, and is silently wrong wherever a site appends TWICE -- ParamImageMask
// and encodeOp both append the payload and THEN a header word, so a prediction
// of len(args) lets the first append fit exactly and the SECOND one reallocate
// unrecorded, orphaning the array that just received the runes. Comparing caps
// across the append needs no count, holds for any number of appends and any
// branch, and cannot be got wrong per-site.
func (b *Buffer) appendArgs(vals ...uint32) {
	old := b.args
	b.args = append(b.args, vals...)
	if cap(b.args) != cap(old) && cap(old) > 0 {
		b.zeroedArrays++
		b.zeroedEntries += cap(old)
		clear(old[:cap(old)])
	}
}

func (b *Buffer) appendRefs(vals ...any) {
	old := b.refs
	b.refs = append(b.refs, vals...)
	if cap(b.refs) != cap(old) && cap(old) > 0 {
		b.zeroedArrays++
		b.zeroedEntries += cap(old)
		clear(old[:cap(old)])
	}
}

type Drawer struct {
	maskStack    []frameOp
	jumpStack    []ops
	inputs       []inputOp
	skipInputOps bool
	// text is non-nil for ExtractText to collect
	// all runes from a frame.
	text []rune
}

type op struct {
	r    ops
	buf  *Buffer
	nops int
}

type ops struct {
	start, end int
	refs       int
}

type opType uint32

const (
	opJump opType = iota
	opCompose
	opImage
	opMask
	opInput
	opOffset
	opClip
)

func Color(b *Buffer, col color.RGBA) Op {
	if b == nil {
		return Op{}
	}
	nrgba := uint32(col.R)<<24 | uint32(col.G)<<16 | uint32(col.B)<<8 | uint32(col.A)
	return Op{encodeOp(b, opImage, 1, []any{uniformImage}, nrgba)}
}

func Image(b *Buffer, img image.Image) Op {
	if b == nil {
		return Op{}
	}
	return Op{encodeOp(b, opImage, 1, []any{img})}
}

func Mask(b *Buffer, img image.Image) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	return MaskOp{encodeOp(b, opMask, 0, []any{img})}
}

func _Alpha(b *Buffer, alpha byte) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	nrgba := uint32(alpha)
	return MaskOp{encodeOp(b, opMask, 0, []any{uniformImage}, nrgba)}
}

func RoundedRect2(b *Buffer, bounds image.Rectangle, cornerRadius int) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	r := cornerRadius * px
	sz := bounds.Size()
	return MaskOp{encodeOp(b, opMask, 0, []any{roundedRectImage},
		uint32(sz.X),
		uint32(sz.Y),
		uint32(r),
	)}.Offset(bounds.Min)
}

func RoundedOutline2(b *Buffer, bounds image.Rectangle, cornerRadius, lineWidth int) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	r := cornerRadius * px
	lw := (lineWidth - 1) * px
	sz := bounds.Size()
	return MaskOp{encodeOp(b, opMask, 0, []any{roundedOutlineImage},
		uint32(sz.X),
		uint32(sz.Y),
		uint32(r),
		uint32(lw),
	)}.Offset(bounds.Min)
}

func Glyph(b *Buffer, face *bitmap.Face, r rune) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	_, _, ok := face.Glyph(r)
	if !ok {
		return MaskOp{}
	}
	return MaskOp{encodeOp(b, opMask, 0, []any{glyphImage, face}, uint32(r))}
}

func ParamImageMask(b *Buffer, img *ImageHandle, refs []any, args []uint32) MaskOp {
	if b == nil {
		return MaskOp{}
	}
	r := ops{
		start: len(b.args),
		end:   len(b.args) + 1 + len(args),
		refs:  len(b.refs) + 1 + len(refs),
	}
	b.appendArgs(args...)
	b.appendArgs(encodeHeader(opMask, len(args), 1+len(refs)))
	b.appendRefs(img)
	b.appendRefs(refs...)
	return MaskOp{op{
		buf: b,
		r:   r,
	}}
}

func Input(b *Buffer, tag Tag) Op {
	if b == nil {
		return Op{}
	}
	return Op{encodeOp(b, opInput, 1, []any{tag})}
}

func Layer(ops ...Op) Op {
	var g group
	for _, m := range ops {
		g.add(m.op)
	}
	return Op{g.Op()}
}

func Compose(op Op, masks ...MaskOp) Op {
	if op.buf == nil {
		return Op{}
	}
	o := newCompose(op.op)
	var g group
	g.add(o)
	for _, m := range masks {
		g.add(m.op)
	}
	return Op{g.Op()}
}

func (op Op) Offset(off image.Point) Op {
	if op.buf == nil {
		return Op{}
	}
	return Op{pair(ensureLatest(op.op), offsetOp(op.buf, off))}
}

func ensureLatest(o op) op {
	o = newCompose(o)
	if o.r.end == len(o.buf.args) {
		return o
	}
	start := len(o.buf.args)
	o.buf.appendArgs(uint32(o.r.start),
		uint32(o.r.end),
		uint32(o.r.refs),
		encodeHeader(opJump, 3, 0),
	)
	return op{
		buf:  o.buf,
		nops: o.nops,
		r: ops{
			start: start,
			end:   len(o.buf.args),
			refs:  len(o.buf.refs),
		},
	}
}

func pair(o1, o2 op) op {
	return op{
		buf:  o1.buf,
		nops: o1.nops,
		r: ops{
			start: o1.r.start,
			end:   o2.r.end,
			refs:  o2.r.refs,
		},
	}
}

func (op Op) Clip(r image.Rectangle) Op {
	if op.buf == nil {
		return Op{}
	}
	return Op{pair(ensureLatest(op.op), clipOp(op.buf, r))}
}

func (op MaskOp) Offset(off image.Point) MaskOp {
	if op.buf == nil {
		return MaskOp{}
	}
	return MaskOp{pair(ensureLatest(op.op), offsetOp(op.buf, off))}
}

func (op MaskOp) Clip(r image.Rectangle) MaskOp {
	if op.buf == nil {
		return MaskOp{}
	}
	return MaskOp{pair(ensureLatest(op.op), clipOp(op.buf, r))}
}

func (d *Drawer) Draw(dst, maskfb draw.Image, op Op) {
	if op.buf == nil {
		return
	}
	// clear to CAP, not just truncate. The backing array outlives the slice,
	// and a mark-sweep collector scans whole allocated objects -- so a stale
	// frameOp from the PREVIOUS frame keeps that frame's mask source, and its
	// Buffer, alive indefinitely. When the previous frame belonged to a
	// session that has since been wiped, that is the whole leak.
	//
	// Here rather than only in Release: this makes the property structural, so
	// a future caller who abandons a Buffer without calling Release cannot
	// reintroduce it. Costs one clear of <= cap entries per frame, against a
	// full-screen redraw.
	//
	// The recursion's own restore at the end of draw() must NOT do this: those
	// entries alias the buffer being drawn right now.
	clear(d.maskStack[:cap(d.maskStack)])
	d.maskStack = d.maskStack[:0]
	d.jumpStack = append(d.jumpStack[:0], op.op.r)
	d.draw(dst, maskfb, op.op.buf, drawState{clip: image.Rect(-1e9, -1e9, 1e9, 1e9)}, math.MaxInt)
	// o.inputs has been populated, skip the work for subsequent frames.
	d.skipInputOps = true
}

func (d *Drawer) Reset() {
	d.inputs = d.inputs[:0]
	d.skipInputOps = false
}

// Release drops every reference this Drawer still holds into buffers it has
// drawn, so a Buffer abandoned while its Drawer lives on can be collected.
//
// Truncation does not do it. maskStack and inputs are re-sliced to length 0
// between frames, but the backing arrays keep their stale entries, and a
// mark-sweep collector scans whole allocated objects rather than up to len.
// Each stale frameOp holds an imageOp whose `refs` and `args` ALIAS the
// Buffer's own arrays, and whose `src` is an interface-value COPY living here
// rather than in the Buffer -- so Buffer.Scrub, which zeroes that Buffer's
// arrays, cannot reach it.
//
// Reslicing to cap before clear() is the whole point: clear(d.maskStack) with
// len 0 zeroes nothing and leaves exactly the entries that leak.
//
// Production calls this from the wipe path (gui/run_flow.go), the only place a
// Buffer is abandoned while this Drawer survives. Draw clears as it goes
// besides, so a missed call cannot reintroduce the leak.
func (d *Drawer) Release() {
	clear(d.maskStack[:cap(d.maskStack)])
	clear(d.inputs[:cap(d.inputs)])
	d.maskStack = d.maskStack[:0]
	d.inputs = d.inputs[:0]
	// jumpStack is []ops -- three ints, no pointers -- so truncation suffices.
	// Listed rather than omitted so the audit is total.
	d.jumpStack = d.jumpStack[:0]
	d.text = nil
	d.skipInputOps = false
}

func (d *Drawer) draw(dst draw.Image, maskfb draw.Image, buf *Buffer, state drawState, nops int) {
	origMaskStackLen := len(d.maskStack)
	orig := state
	args := buf.args
	refs := buf.refs
	for len(d.jumpStack) > 0 && nops > 0 {
		r := &d.jumpStack[len(d.jumpStack)-1]
		if r.end == r.start {
			d.jumpStack = d.jumpStack[:len(d.jumpStack)-1]
			continue
		}
		typnargs := args[r.end-1]
		typ := opType(typnargs & 0xf)
		nrefs := int((typnargs >> 8) & 0xf)
		nargs := int(typnargs >> 16)
		oargs := args[r.end-1-nargs : r.end-1]
		rargs := refs[r.refs-nrefs : r.refs]
		r.end -= 1 + nargs
		r.refs -= nrefs
		switch typ {
		case opOffset:
			off := image.Point{X: int(int32(oargs[0])), Y: int(int32(oargs[1]))}
			state.pos = state.pos.Add(image.Point(off))
			continue
		case opClip:
			r := image.Rectangle{
				Min: image.Point{X: int(int32(oargs[0])), Y: int(int32(oargs[1]))},
				Max: image.Point{X: int(int32(oargs[2])), Y: int(int32(oargs[3]))},
			}.Add(state.pos)
			state.clip = state.clip.Intersect(r)
			continue
		case opJump:
			d.jumpStack = append(d.jumpStack, ops{
				start: int(oargs[0]),
				end:   int(oargs[1]),
				refs:  int(oargs[2]),
			})
			continue
		case opCompose:
			lops := int(oargs[0])
			d.draw(dst, maskfb, buf, state, lops)
		case opInput:
			if d.skipInputOps {
				break
			}
			iop := inputOp{
				tag:    rargs[0],
				bounds: state.clip,
			}
			d.inputs = append(d.inputs, iop)
		case opImage, opMask:
			iop := imageOp{src: rargs[0], args: oargs, refs: rargs[1:]}
			r := iop.materialize(0).Bounds().Add(state.pos)
			state.clip = state.clip.Intersect(r)
			fop := frameOp{pos: state.pos, op: iop}
			if typ != opImage {
				d.maskStack = append(d.maskStack, fop)
				continue
			}
			clip := state.clip.Intersect(dst.Bounds())
			if clip.Empty() {
				break
			}
			var maskSrc image.Image
			var maskPos image.Point
			maskStack := d.maskStack
			if d.text != nil {
				for _, m := range maskStack {
					switch img := m.op.materialize(0).(type) {
					case *glyph:
						d.text = append(d.text, img.r)
					}
				}
			}
			switch len(maskStack) {
			case 0:
			case 1:
				mask := maskStack[len(maskStack)-1]
				maskSrc = mask.op.materialize(1)
				maskPos = mask.pos
			default:
				// Blend the masks into maskfb.
				maskSrc = maskfb
				// First operation initializes maskfb content.
				op := draw.Src
				for len(maskStack) > 0 {
					if len(maskStack) > 1 {
						// Draw 2 masks at a time.
						m1, m2 := maskStack[0], maskStack[1]
						maskStack = maskStack[2:]
						drawMask(maskfb, clip,
							m2.op.materialize(0), clip.Min.Sub(m2.pos),
							m1.op.materialize(1), clip.Min.Sub(m1.pos), op)
					} else {
						m := maskStack[0]
						maskStack = maskStack[1:]
						drawMask(maskfb, clip,
							m.op.materialize(0), clip.Min.Sub(m.pos),
							nil, image.Point{}, op)
					}
					// Subsequent operations blend into maskfb.
					op = draw.Over
				}
			}
			src := fop.op.materialize(0)
			drawMask(dst, clip, src, clip.Min.Sub(fop.pos), maskSrc, clip.Min.Sub(maskPos), draw.Over)
		}
		state = orig
		d.maskStack = d.maskStack[:origMaskStackLen]
		nops--
	}
}

func (b *Buffer) Reset() {
	b.args = b.args[:0]
	clear(b.refs)
	b.refs = b.refs[:0]
}

func encodeOp(b *Buffer, cmd opType, nops int, refs []any, args ...uint32) op {
	r := ops{
		start: len(b.args),
		end:   len(b.args) + 1 + len(args),
		refs:  len(b.refs) + len(refs),
	}
	b.appendArgs(args...)
	b.appendArgs(encodeHeader(cmd, len(args), len(refs)))
	b.appendRefs(refs...)
	return op{
		buf:  b,
		r:    r,
		nops: nops,
	}
}

func offsetOp(b *Buffer, off image.Point) op {
	return encodeOp(b, opOffset, 0, nil,
		uint32(off.X), uint32(off.Y),
	)
}

func clipOp(b *Buffer, r image.Rectangle) op {
	return encodeOp(b, opClip, 0, nil,
		uint32(r.Min.X), uint32(r.Min.Y),
		uint32(r.Max.X), uint32(r.Max.Y),
	)
}

func newCompose(op op) op {
	if op.nops <= 1 {
		return op
	}
	if op.r.end != len(op.buf.args) {
		start := len(op.buf.args)
		op.buf.appendArgs(uint32(op.r.start),
			uint32(op.r.end),
			uint32(op.r.refs),
			encodeHeader(opJump, 3, 0),
		)
		op.r.start = start
	}
	encodeOp(op.buf, opCompose, 0, nil, uint32(op.nops))
	op.r.end = len(op.buf.args)
	op.r.refs = len(op.buf.refs)
	op.nops = 1
	return op
}

type group struct {
	op
	discontinuous bool
}

func (g *group) add(op op) {
	if op.buf == nil {
		return
	}
	if g.buf == nil {
		g.op = op
		return
	}
	if op.buf != g.buf {
		panic("TODO")
	}
	next := op.r
	if g.r.end != next.start {
		if !g.discontinuous {
			g.discontinuous = true
			start := g.r.start
			g.r.start = len(g.buf.args)
			g.buf.appendArgs(uint32(start))
		}
		g.buf.appendArgs(uint32(g.r.end),
			uint32(g.r.refs),
			encodeHeader(opJump, 3, 0), uint32(next.start),
		)
	}
	g.nops += op.nops
	g.r.end = next.end
	g.r.refs = next.refs
}

func (g *group) Op() op {
	if g.discontinuous {
		g.buf.appendArgs(uint32(g.r.end), uint32(g.r.refs), encodeHeader(opJump, 3, 0))
		g.r.end = len(g.buf.args)
	}
	return g.op
}

func encodeHeader(cmd opType, nargs, nrefs int) uint32 {
	return uint32(nargs)<<16 | uint32(nrefs)<<8 | uint32(cmd)
}

func (t opType) String() string {
	switch t {
	case opClip:
		return "clip"
	case opImage:
		return "image"
	case opInput:
		return "input"
	case opJump:
		return "jump"
	case opCompose:
		return "compose"
	case opMask:
		return "mask"
	case opOffset:
		return "offset"
	default:
		panic("invalid opType")
	}
}

func RegisterParameterizedImage(factory func() ParameterizedImage) *ImageHandle {
	img := new(ImageHandle)
	for i := range img.scratch {
		img.scratch[i] = factory()
	}
	return img
}

type ParameterizedImage func(args []uint32, refs []any) image.Image

type inputOp struct {
	bounds image.Rectangle
	tag    Tag
}

type frameOp struct {
	pos image.Point
	op  imageOp
}

type drawState struct {
	pos  image.Point
	clip image.Rectangle
}

func (d *Drawer) ExtractText(dst image.Rectangle, o Op) string {
	// Instruct Draw to collect runes.
	d.text = []rune{}
	fb := rgb565.New(dst)
	maskfb := image.NewAlpha(dst)
	d.Draw(fb, maskfb, o)
	// Text is added in reverse order.
	slices.Reverse(d.text)
	txt := string(d.text)
	d.text = nil
	return txt
}

func (d *Drawer) TagBounds(t Tag) (image.Rectangle, bool) {
	for _, inp := range d.inputs {
		if t == inp.tag {
			return inp.bounds, true
		}
	}
	return image.Rectangle{}, false
}

func (d *Drawer) Hit(p image.Point) (Tag, image.Rectangle, bool) {
	for _, inp := range d.inputs {
		if p.In(inp.bounds) {
			return inp.tag, inp.bounds, true
		}
	}
	return nil, image.Rectangle{}, false
}

func (op imageOp) materialize(slot int) image.Image {
	switch src := op.src.(type) {
	case *ImageHandle:
		return src.materialize(slot, op.args, op.refs)
	case image.Image:
		return src
	case nil:
		return nil
	default:
		panic("invalid source")
	}
}

func (img *ImageHandle) materialize(slot int, args []uint32, refs []any) image.Image {
	return img.scratch[slot](args, refs)
}

type imageOp struct {
	src  any
	refs []any
	args []uint32
}
