// Command plateview renders a plate to SVG or PNG without a machine or a
// browser, at the parameters the SeedHammer II actually engraves at.
//
// It is a PREVIEW OF THE TOOLPATH, not a drawing of it: the geometry comes
// from the same FitBlocks/EngraveFitted/PlanEngraving calls the firmware makes,
// stroked at the production 0.3mm cut width. What you see is the cut.
//
//	go run ./cmd/plateview -plate bothproof -o /tmp/plate.png
//	go run ./cmd/plateview -plate freetext -text "$(cat note.txt)" -title NOTE -o out.svg
//	go run ./cmd/plateview -list
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"seedhammer.com/bspline"
	"seedhammer.com/gui"
	"seedhammer.com/internal/golden"
	"seedhammer.com/internal/sh2"
)

// The machine's engraver parameters, pinned against
// cmd/controller/platform_sh2.go by internal/sh2's own test.
var engraverParams = sh2.Params()

func main() {
	var (
		plate  = flag.String("plate", "bothproof", "which plate to render; -list to see them")
		out    = flag.String("o", "plate.svg", "output file; .png converts via rsvg-convert")
		text   = flag.String("text", "", "body text (freetext plate)")
		title  = flag.String("title", "", "title row (freetext plate)")
		footer = flag.String("footer", "", "footer row (freetext plate)")
		face   = flag.String("face", "sh", "engraving face for the freetext plate: sh or const")
		qr     = flag.Bool("qr", false, "include a QR code")
		px     = flag.Int("px", 1600, "PNG width and height in pixels")
		list   = flag.Bool("list", false, "list the plates and exit")
	)
	flag.Parse()

	if *list {
		for _, n := range gui.PreviewPlates() {
			fmt.Println(n)
		}
		return
	}
	if err := run(*plate, *out, *px, gui.PreviewOpts{
		Text: *text, Title: *title, Footer: *footer, Face: *face, QR: *qr,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "plateview: %v\n", err)
		os.Exit(1)
	}
}

func run(plate, out string, px int, o gui.PreviewOpts) error {
	p, err := gui.BuildPreview(engraverParams, plate, o)
	if err != nil {
		return err
	}
	describe(os.Stderr, plate, p)

	svg, err := render(p)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(out), ".png") {
		return writePNG(out, svg, px)
	}
	return os.WriteFile(out, svg, 0o644)
}

// describe reports the layout in text, from the SAME Preview that is drawn --
// so the listing cannot describe a plate other than the one rendered.
func describe(w *os.File, plate string, p gui.Preview) {
	fmt.Fprintf(w, "%s: %s, %d rows, ~%s to engrave\n",
		plate, sizeLabel(p), len(p.Rows), engraveDuration(p.Plate.Duration))
	if p.Title != "" {
		fmt.Fprintf(w, "  title  [%-5s] %s\n", p.TitleFace, p.Title)
	}
	for i, r := range p.Rows {
		fmt.Fprintf(w, "  row %2d [%-5s] %2d  %s\n", i+1, r.Face, len([]rune(r.Text)), r.Text)
	}
	if p.Footer != "" {
		fmt.Fprintf(w, "  footer [%-5s] %s\n", p.FooterFace, p.Footer)
	}
	if p.HasQR {
		fmt.Fprintln(w, "  (carries a QR code)")
	}
}

func sizeLabel(p gui.Preview) string {
	if p.SizeMM == 0 {
		return "fixed layout"
	}
	return fmt.Sprintf("%.1fmm", p.SizeMM)
}

// engraveDuration turns the planner's tick count into wall clock exactly as
// the device's own progress screen does -- divide by TicksPerSecond, round up
// (gui.go's engraveRunning case). A preview that estimated differently from
// the machine would be a second answer to a question with one.
func engraveDuration(ticks uint) string {
	tps := uint(engraverParams.TicksPerSecond)
	secs := (ticks + tps - 1) / tps
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}

// plateChrome is the steel and the two screw-hole keep-out bands.
//
// The bands are backup's innerMargin, which is ALL the firmware knows: it
// reserves a strip at each end that no glyph may enter, and never the hole
// centres. Drawing circles would be inventing geometry this repo does not have.
const innerMargin = 10 // mm; backup.innerMargin

func render(p gui.Preview) ([]byte, error) {
	dims := gui.SquarePlate.Dims(engraverParams.Millimeter)
	var buf bytes.Buffer
	if err := golden.Vectorize(&buf, engraverParams.StrokeWidth,
		bspline.Bounds{Max: dims}, p.Plate.Spline); err != nil {
		return nil, err
	}
	w, band := dims.X, innerMargin*engraverParams.Millimeter
	chrome := fmt.Sprintf(`
<rect x="0" y="0" width="%d" height="%d" rx="%d" fill="#d8d8d4" stroke="#8a8a86" stroke-width="%d"/>
<rect x="0" y="0" width="%d" height="%d" fill="#c4c4bf"/>
<rect x="0" y="%d" width="%d" height="%d" fill="#c4c4bf"/>
`,
		w, dims.Y, 2*engraverParams.Millimeter, engraverParams.Millimeter/4,
		w, band,
		dims.Y-band, w, band)

	svg := buf.String()
	const anchor = `<path class="spline"`
	i := strings.Index(svg, anchor)
	if i < 0 {
		// Vectorize's output shape is the one thing this depends on; a silent
		// miss would ship a plate with no steel behind it and look merely odd.
		return nil, fmt.Errorf("no toolpath in the vectorized plate")
	}
	return []byte(svg[:i] + chrome + svg[i:]), nil
}

func writePNG(out string, svg []byte, px int) error {
	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		return fmt.Errorf("PNG output needs rsvg-convert on PATH (%v); write .svg instead", err)
	}
	cmd := exec.Command("rsvg-convert",
		"-w", fmt.Sprint(px), "-h", fmt.Sprint(px), "-b", "white", "-o", out)
	cmd.Stdin = bytes.NewReader(svg)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
