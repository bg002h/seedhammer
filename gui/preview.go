//go:build !tinygo

// Plate previews for cmd/plateview.
//
// The plate compositions this exposes -- above all the proof patterns -- are
// unexported package state, and a preview that re-declared them would be a
// SECOND copy that drifts: the whole value of a preview is that it shows what
// the machine will cut, which it can only do by going through the same
// FitBlocks/EngraveFitted path the device does.
//
// The !tinygo constraint is what keeps that honesty free: the firmware is built
// with TinyGo (cmd/controller/main.go is `tinygo && rp`), so this file is
// absent from the image rather than relying on the linker to drop it.
package gui

import (
	"fmt"
	"sort"

	"seedhammer.com/backup"
	"seedhammer.com/bip39"
	"seedhammer.com/engrave"
)

// PreviewOpts are the knobs cmd/plateview exposes. Zero values give each
// plate's default form.
type PreviewOpts struct {
	// Text, Title and Footer apply to the "freetext" plate only; the proof
	// plates carry their own, which is the point of them.
	Text, Title, Footer string
	// QR selects the QR variant. A proof that needs the whole plate has none
	// and ignores this -- see ftProof.NeedsWholePlate.
	QR bool
	// Face is "sh" or "const", for "freetext" only.
	Face string
	// SizeMM pins the rung. Zero auto-fits, which is what the device does
	// unless the operator chose a size.
	SizeMM float32
}

// PreviewRow is one engraved row: the face it is cut in and what it says.
type PreviewRow struct {
	Face string
	Text string
}

// Preview is a rendered plate plus what can be said about it in text.
//
// Rows is empty for plates that are not laid out by the free-text fitter --
// there is no per-row face map to report for a seed plate, and inventing one
// would be a preview of something the device does not do.
type Preview struct {
	Plate  Plate
	SizeMM float32
	Rows   []PreviewRow
	// Title and Footer with the faces they are cut in, when the plate has them.
	Title, TitleFace   string
	Footer, FooterFace string
	HasQR              bool
}

// previewSeed is the canonical BIP-39 all-zeros test vector, published in the
// BIP itself and in every implementation's test suite.
//
// It is HARDCODED, and cmd/plateview deliberately offers no flag to supply a
// different one. A host tool that renders an arbitrary mnemonic to a PNG on
// disk is a way to leak a seed by accident, and it would buy nothing: what a
// preview answers is where the words land on the steel, which any twelve words
// answer equally well.
// UPPERCASE because the shipped wordlist is (bip39/wordlist.go), which is what
// LabelFor returns and what ClosestWord's binary search compares against --
// lowercase sorts after every entry and matches nothing.
var previewSeed = []string{
	"ABANDON", "ABANDON", "ABANDON", "ABANDON", "ABANDON", "ABANDON",
	"ABANDON", "ABANDON", "ABANDON", "ABANDON", "ABANDON", "ABOUT",
}

// previewPassphrase is a sample, for the same reason previewSeed is fixed.
const previewPassphrase = "correct horse battery staple"

// PreviewPlates lists every name BuildPreview accepts.
func PreviewPlates() []string {
	names := make([]string, 0, len(previewBuilders))
	for n := range previewBuilders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var previewBuilders = map[string]func(engrave.Params, PreviewOpts) (Preview, error){
	"textproof":  proofPreview(ftProofTriggerSH),
	"constproof": proofPreview(ftProofTriggerConst),
	"bothproof":  proofPreview(ftProofTriggerBoth),
	"freetext":   freeTextPreview,
	"seed":       seedPreview,
	"passphrase": passphrasePreview,
}

// BuildPreview renders the named plate at params.
func BuildPreview(params engrave.Params, name string, o PreviewOpts) (Preview, error) {
	b, ok := previewBuilders[name]
	if !ok {
		return Preview{}, fmt.Errorf("plateview: unknown plate %q (have %v)", name, PreviewPlates())
	}
	return b(params, o)
}

// proofPreview renders one of the canned proof patterns, through the same
// trigger lookup the text box uses -- so a proof that changes changes here too.
func proofPreview(trigger string) func(engrave.Params, PreviewOpts) (Preview, error) {
	return func(params engrave.Params, o PreviewOpts) (Preview, error) {
		p, ok := ftProofForTrigger(trigger)
		if !ok {
			return Preview{}, fmt.Errorf("plateview: no proof for trigger %q", trigger)
		}
		// A whole-plate proof drops the QR when it loads; mirror that here
		// rather than fitting a plate the device would refuse to build.
		qr := o.QR && !p.NeedsWholePlate()
		// The mixed proof at a chosen rung goes through the SAME drop ladder
		// the device walks, so a preview cannot show content the machine would
		// have trimmed.
		if o.SizeMM != 0 && p.Trigger == ftProofTriggerBoth {
			text, plan, title, footer, err := ftBothAt(params, o.SizeMM, qr)
			if err != nil {
				return Preview{}, err
			}
			return fittedPreviewAt(params, plan, text, title, footer, qr, o.SizeMM)
		}
		return fittedPreviewAt(params, p.Plan, p.For(qr), p.Title, ftProofFooter, qr, o.SizeMM)
	}
}

func freeTextPreview(params engrave.Params, o PreviewOpts) (Preview, error) {
	plan := &ftPlanSH
	switch o.Face {
	case "", "sh":
	case "const":
		plan = &ftPlanConst
	default:
		return Preview{}, fmt.Errorf("plateview: unknown face %q (want sh or const)", o.Face)
	}
	return fittedPreviewAt(params, plan, o.Text, o.Title, o.Footer, o.QR, o.SizeMM)
}

// fittedPreview is the one path every free-text-family plate takes: ONE
// FitBlocks, whose result is both engraved and reported. The rows printed and
// the rows cut cannot disagree because they are the same value.
func fittedPreviewAt(params engrave.Params, plan *ftPlan, text, title, footer string, qr bool, size float32) (Preview, error) {
	blocks := plan.Blocks(text)
	fit := backup.FitBlocks
	if size != 0 {
		fit = func(p engrave.Params, b []backup.Block, t, f string, q bool) (backup.Fitted, error) {
			return backup.FitBlocksAt(p, b, t, f, q, size)
		}
	}
	fitted, err := fit(params, blocks, title, footer, qr)
	if err != nil {
		return Preview{}, err
	}
	p, err := toPlate(backup.EngraveFitted(params, fitted), params)
	if err != nil {
		return Preview{}, err
	}
	pr := Preview{
		Plate:      p,
		SizeMM:     fitted.SizeMM,
		Title:      fitted.Title,
		TitleFace:  previewFaceName(fitted.TitleFace),
		Footer:     fitted.Footer,
		FooterFace: previewFaceName(fitted.FooterFace),
		HasQR:      fitted.QR != nil,
	}
	for i, l := range fitted.Lines {
		pr.Rows = append(pr.Rows, PreviewRow{Face: previewFaceName(fitted.Faces[i]), Text: l})
	}
	return pr, nil
}

func seedPreview(params engrave.Params, o PreviewOpts) (Preview, error) {
	m, err := previewMnemonic()
	if err != nil {
		return Preview{}, err
	}
	p, err := engraveSeed(params, m, 0)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Plate: p, HasQR: true}, nil
}

func passphrasePreview(params engrave.Params, o PreviewOpts) (Preview, error) {
	p, err := ppBuildPlate(params, []byte(previewPassphrase), "", "", o.QR)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Plate: p, HasQR: o.QR}, nil
}

// previewMnemonic resolves previewSeed through the shipped wordlist, so a
// typo in the vector is an error here rather than a plate of blank words.
func previewMnemonic() (bip39.Mnemonic, error) {
	m := make(bip39.Mnemonic, len(previewSeed))
	for i, w := range previewSeed {
		word, ok := bip39.ClosestWord(w)
		if !ok || bip39.LabelFor(word) != w {
			return nil, fmt.Errorf("plateview: %q is not a BIP-39 word", w)
		}
		m[i] = word
	}
	return m, nil
}

// previewFaceName names a face for the row listing. The two shipped faces are
// the only ones a plate can be cut in.
func previewFaceName(f any) string {
	switch f {
	case ftFaceSH.Face:
		return "sh"
	case ftFaceConst.Face:
		return "const"
	}
	return "?"
}
