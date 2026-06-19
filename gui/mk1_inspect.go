package gui

import (
	"strings"

	"seedhammer.com/mk"
)

func hasMKPrefix(s string) bool {
	return strings.HasPrefix(s, "mk1") || strings.HasPrefix(s, "MK1")
}

// chunkString splits s into substrings of at most n runes (ASCII here), so the
// long base58 xpub renders as short non-wrapping display lines.
func chunkString(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

type gatherStatus int

const (
	gatherIgnored gatherStatus = iota // not an mk1 chunk / parse failed
	gatherForeign                     // valid mk1 but a different chunk set
	gatherDup                         // chunk index already captured
	gatherAdded                       // new chunk added
)

// mk1Gatherer accumulates mk1 chunk strings toward a complete set. Pure (no
// GUI/NFC) so it is unit-tested directly; mk1GatherFlow is a thin NFC shell.
type mk1Gatherer struct {
	set    map[int]string
	total  int
	setID  uint32
	primed bool
}

func (g *mk1Gatherer) offer(s string) gatherStatus {
	h, err := mk.ParseHeader(s)
	if err != nil {
		return gatherIgnored
	}
	if !g.primed {
		g.set = map[int]string{}
		g.total = h.TotalChunks
		g.setID = h.ChunkSetID
		g.primed = true
	} else if !h.Chunked || h.ChunkSetID != g.setID || h.TotalChunks != g.total {
		return gatherForeign
	}
	if _, ok := g.set[h.ChunkIndex]; ok {
		return gatherDup
	}
	g.set[h.ChunkIndex] = s
	return gatherAdded
}

func (g *mk1Gatherer) complete() bool { return g.primed && len(g.set) == g.total }

func (g *mk1Gatherer) collected() []string {
	out := make([]string, 0, len(g.set))
	for _, s := range g.set {
		out = append(out, s)
	}
	return out
}
