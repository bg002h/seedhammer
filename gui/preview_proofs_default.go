//go:build !tinygo && !refugium

package gui

// The proof plates' previewBuilders entries, kept with the trigger constants
// they name (Refugium plan F7 §4.1, round 3 M-3): the Refugium build has no
// proof triggers, so cmd/plateview offers no proof plate there.
func init() {
	previewBuilders["textproof"] = proofPreview(ftProofTriggerSH)
	previewBuilders["constproof"] = proofPreview(ftProofTriggerConst)
	previewBuilders["bothproof"] = proofPreview(ftProofTriggerBoth)
	// One entry per SIDE. The two sides are two independent plate programs and
	// an operator flip; rendering them as one image would invent a relationship
	// the firmware does not have.
	previewBuilders["sizeproof-front"] = proofPreview(ftProofTriggerSizeFront)
	previewBuilders["sizeproof-back"] = proofPreview(ftProofTriggerSizeBack)
}
