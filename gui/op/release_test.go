package op

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"seedhammer.com/image/rgb565"
)

// The retention path fix D closes.
//
// imageOp holds `refs []any` and `args []uint32` -- slice headers ALIASING a
// Buffer's backing arrays -- and `src any`, an interface-value COPY that lives
// in the Drawer's own array and that Buffer.Scrub therefore cannot reach.
// Drawer.Draw truncates maskStack with [:0], which leaves every stale frameOp
// in the backing array, and a mark-sweep collector scans whole allocated
// objects rather than up to len. So a Drawer that outlives a Buffer pins it.
//
// On the device that Drawer is `d := new(op.Drawer)` at gui/run_flow.go:42,
// deliberately allocated OUTSIDE the session loop so a wipe does not
// reallocate the mask -- which makes the wipe path the one place in production
// where a Buffer is abandoned while its Drawer lives on.

// canaryMask is an image whose collection a test can observe. A distinct
// pointer type because runtime.SetFinalizer needs one, and because it must be
// an op.Mask SOURCE: op.go's draw appends to maskStack only when the op is not
// an opImage, so a canary passed through op.Image or op.Color never enters the
// structure under test at all.
type canaryMask struct {
	img *image.Alpha
}

func newCanaryMask() *canaryMask {
	return &canaryMask{img: image.NewAlpha(image.Rect(0, 0, 4, 4))}
}

func (c *canaryMask) ColorModel() color.Model      { return c.img.ColorModel() }
func (c *canaryMask) Bounds() image.Rectangle      { return c.img.Bounds() }
func (c *canaryMask) At(x, y int) color.Color      { return c.img.At(x, y) }
func (c *canaryMask) AlphaAt(x, y int) color.Alpha { return c.img.AlphaAt(x, y) }

// collected forces collection and reports whether the finalizer ran.
//
// TWO GC calls plus a timeout, not one: SetFinalizer only queues the finalizer,
// which then needs the finalizer goroutine to be scheduled. A single
// runtime.GC() makes this flaky-red rather than deterministic.
//
// EVERY caller must runtime.KeepAlive(d) AFTER its assertion. Go's liveness is
// precise: a Drawer is dead the instant the code stops using it, so without
// KeepAlive the collector reclaims the DRAWER, the canary goes with it, and the
// test reports "released" no matter what the code under test does. Measured --
// with the canary provably still in maskStack[2], collected() returned true.
func collected(done <-chan struct{}) bool {
	for range 2 {
		runtime.GC()
		select {
		case <-done:
			return true
		case <-time.After(200 * time.Millisecond):
		}
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// drawCanary draws one frame carrying the canary as the LAST of nmasks masks,
// scrubs the buffer as the session tail does, and returns with no reference of
// its own left. Everything it allocated is unreachable on return except
// through d.
//
// The canary goes FIRST in the Compose list, and nmasks is a parameter, for the
// reason round 0's I4 gives: Draw re-appends to maskStack from index 0, so a
// later frame with at least as many masks OVERWRITES the canary's slot and
// collects it whatever the code under test does.
//
// FIRST, not last, and the difference is measured rather than reasoned:
// Compose's masks land on maskStack in REVERSE, so a canary passed last sits at
// index 0 and is the FIRST slot any later frame overwrites. Passed first with
// nmasks=3 it sits at index 2, and a one-mask frame cannot reach it. The
// earlier cut of this test put it last, and passed on unfixed code.
func drawCanary(d *Drawer, done chan<- struct{}, nmasks int) {
	buf := new(Buffer)
	c := newCanaryMask()
	runtime.SetFinalizer(c, func(*canaryMask) { close(done) })
	fb := rgb565.New(image.Rect(0, 0, 4, 4))
	mask := image.NewAlpha(image.Rect(0, 0, 4, 4))
	masks := make([]MaskOp, 0, nmasks)
	masks = append(masks, Mask(buf, c))
	for range nmasks - 1 {
		masks = append(masks, Mask(buf, image.NewAlpha(image.Rect(0, 0, 4, 4))))
	}
	d.Reset()
	d.Draw(fb, mask, Compose(Color(buf, color.RGBA{A: 0xff}), masks...))
	// What the session tail does before abandoning the Context. It zeroes the
	// buffer's own arrays -- including every refs slot -- so after this the
	// ONLY thing that can still reach the canary is the Drawer.
	buf.Scrub()
}

// TestDrawerReleaseFreesTheBufferItDrew is the reachability property, and it is
// the one that fails before fix D. Everything else here is white-box detail.
func TestDrawerReleaseFreesTheBufferItDrew(t *testing.T) {
	d := new(Drawer)
	done := make(chan struct{})
	drawCanary(d, done, 3)

	if collected(done) {
		t.Fatal("the canary was collected BEFORE Release -- this test proves nothing: " +
			"either the mask never reached maskStack, or something else already dropped it")
	}
	d.Release()
	if !collected(done) {
		t.Error("after Release the Drawer still reaches the abandoned buffer's mask source; " +
			"the retention path is open")
	}
	runtime.KeepAlive(d)
}

// TestDrawerReleaseClearsToCapacity kills the clear(d.maskStack) mutant -- the
// one without [:cap(...)], which is the natural way to write this wrong.
// At the assertion point len is 0, so a clear bounded by len zeroes NOTHING and
// every stale entry survives.
func TestDrawerReleaseClearsToCapacity(t *testing.T) {
	d := new(Drawer)
	done := make(chan struct{})
	drawCanary(d, done, 3)

	// Vacuous unless the structure under test was actually populated: a frame
	// with no mask ops and no input ops leaves both caps at 0, [:0] is empty,
	// and "every element is the zero value" passes on an untouched Drawer.
	if cap(d.maskStack) == 0 {
		t.Fatal("cap(maskStack) is 0 -- the frame drew no mask ops, so this test is vacuous")
	}
	d.Release()

	for i, f := range d.maskStack[:cap(d.maskStack)] {
		if f.op.src != nil || f.op.refs != nil || f.op.args != nil {
			t.Errorf("maskStack[%d] still holds a reference after Release: %+v", i, f.op)
		}
	}
	for i, in := range d.inputs[:cap(d.inputs)] {
		if in.tag != nil {
			t.Errorf("inputs[%d] still holds tag %v after Release", i, in.tag)
		}
	}
	runtime.KeepAlive(d)
}

// TestDrawerDrawDoesNotStrandTheFrameItReplaces is the ALTITUDE property (round
// 0 M6): Draw is self-maintaining, so a Drawer reused across buffers cannot
// strand the previous one even if nobody calls Release. That is what makes the
// leak structurally unreachable for a future caller rather than a rule someone
// must remember.
func TestDrawerDrawDoesNotStrandTheFrameItReplaces(t *testing.T) {
	d := new(Drawer)
	done := make(chan struct{})
	// THREE masks, canary deepest.
	drawCanary(d, done, 3)

	// A SECOND frame, from a different buffer, with ONE mask and no canary.
	// Shallower on purpose: it refills maskStack[0] only, so the canary's slot
	// at index 2 is untouched and survives unless Draw clears as it goes. With
	// equal depth this test would pass on the unfixed code -- measured, that is
	// exactly what it did before the depth was made explicit.
	buf2 := new(Buffer)
	fb := rgb565.New(image.Rect(0, 0, 4, 4))
	mask := image.NewAlpha(image.Rect(0, 0, 4, 4))
	other := image.NewAlpha(image.Rect(0, 0, 4, 4))
	d.Reset()
	d.Draw(fb, mask, Compose(Color(buf2, color.RGBA{A: 0xff}), Mask(buf2, other)))

	if !collected(done) {
		t.Error("drawing a new frame left the PREVIOUS buffer's mask source reachable; " +
			"Draw is not self-maintaining and the leak survives any missing Release call")
	}
	runtime.KeepAlive(d)
}
