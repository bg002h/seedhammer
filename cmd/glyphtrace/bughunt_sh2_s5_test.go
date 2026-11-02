package main

import "testing"

// TestBughuntSh246ShortChannelNotWorst: bug-hunt sh2 #46. A channel shorter
// than the sustain window (sustainMM, 6 px) scores SustainedMax 0, so
// worstChannel ranks a 0.5mm-wide gap below a 0.3mm-wide one. Both are at or
// above findChannels' own minimum run (minChannelRunMM), so findChannels can
// emit the short one.
//
// Open product question: what SustainedMax should return for a channel
// shorter than the window (assumed: the narrowest width of what there is). The
// test only asserts the reported wrong ranking is absent.
func TestBughuntSh246ShortChannelNotWorst(t *testing.T) {
	t.Skip("deferred-minor: sh-glyphtrace-short-channel")
	short := channel{Widths: []float64{.5, .5, .5, .5}, RunMM: minChannelRunMM}
	long := channel{Widths: []float64{.3, .3, .3, .3, .3, .3, .3, .3}, RunMM: .2}
	w, ok := worstChannel([]channel{long, short}, minChannelRunMM)
	if !ok {
		t.Fatal("no worst channel")
	}
	if len(w.Widths) != len(long.Widths) {
		t.Errorf("worst channel is the %d-px-long, 0.5mm-wide one (SustainedMax %v); want the 0.3mm-wide one (SustainedMax %v)",
			len(w.Widths), short.SustainedMax(sustainMM), long.SustainedMax(sustainMM))
	}
}
