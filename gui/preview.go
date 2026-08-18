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
	"slices"
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
	// Sizes is the size each row is cut at, parallel to Rows.
	//
	// It is carried beside SizeMM rather than derived from it because a plate
	// that MIXES sizes has no valid SizeMM -- it is 0 -- and a preview that
	// printed that would say "0.0mm" for a size ladder, which is the very
	// defect the ladder was added to look for.
	Sizes []float32
	Rows  []PreviewRow
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
	// One entry per SIDE. The two sides are two independent plate programs and
	// an operator flip; rendering them as one image would invent a relationship
	// the firmware does not have.
	"sizeproof-front": proofPreview(ftProofTriggerSizeFront),
	"sizeproof-back":  proofPreview(ftProofTriggerSizeBack),
	"freetext":        freeTextPreview,
	"seed":            seedPreview,
	"passphrase":      passphrasePreview,
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
		p, _, ok := ftProofForTrigger(trigger)
		if !ok {
			return Preview{}, fmt.Errorf("plateview: no proof for trigger %q", trigger)
		}
		// A whole-plate proof drops the QR when it loads; mirror that here
		// rather than fitting a plate the device would refuse to build.
		qr := o.QR && !p.NeedsWholePlate()
		// Through the SAME resolver the prompt and the loader use, never a
		// third derivation of the four fields. It is what walks the mixed
		// proof's drop ladder at a chosen rung, so a preview cannot show
		// content the machine would have trimmed -- and it is what returns the
		// per-proof footer, so a preview cannot put a footer on a plate the
		// device engraves without one.
		//
		// The rung is only ever a CHOICE for a Sizeable proof. -size is a flag
		// and reaches this for every plate name.
		var rung float32
		if p.Sizeable {
			rung = o.SizeMM
		}
		out := ftProofOutcomeFor(params, p, rung, qr)
		// o.SizeMM rather than out.SizeMM: a non-Sizeable proof still honours
		// -size by being fitted at that rung, exactly as it did before -- and a
		// SIZE LADDER is refused there rather than flattened, because ftFitAt
		// makes a rung beside per-block sizes an error.
		return fittedPreviewAt(params, out.Plan, out.Text, out.Title, out.Footer, qr, o.SizeMM)
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

// fittedPreview is the one path every free-text-family plate takes: ONE fit,
// whose result is both engraved and reported. The rows printed and the rows cut
// cannot disagree because they are the same value.
//
// It goes through ftFitAt, the device's OWN router, rather than calling
// FitBlocks or FitBlocksAt. Both of those ignore Block.SizeMM, so a preview
// wired straight to them fitted a size ladder UNIFORMLY: FitBlocks succeeded at
// some single rung, the listing printed one size, and the preview of a
// permanent-steel plate showed a plate the device will not cut. The tool exists
// to check the plate before it is cut, which makes that worse than a wrong
// render, not better.
func fittedPreviewAt(params engrave.Params, plan *ftPlan, text, title, footer string, qr bool, size float32) (Preview, error) {
	fitted, err := ftFitAt(params, plan.Blocks(text), title, footer, qr, size)
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
		Sizes:      slices.Clone(fitted.Sizes),
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
	p, err := ppBuildPlate(params, []byte(previewPassphrase), "", "", o.QR, "", false)
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
