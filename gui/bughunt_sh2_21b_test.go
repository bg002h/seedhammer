package gui

import (
	"testing"

	"seedhammer.com/md"
)

// Bug 21 (second case): with TWO unseated slots the codec gives them the two
// lowest free accounts (§4f, md resolveOrigins: "no other slot (declared or
// filled earlier) holds"), so the self-check must count an earlier unseated
// slot's origin as taken, or it refuses an honest build.
func TestBughuntSh221SelfCheckTwoUnseatedSlots(t *testing.T) {
	list := md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3, Sorted: true}},
	}}
	st := &composerState{list: list, reg: &seedRegistry{}}
	st.sources = []composerSource{{kind: composerSourceKey, seedID: -1}}
	fp := [4]byte{0x73, 0xc5, 0xda, 0x00}
	origin := composerTestOrigin(2, 1)
	st.assigned = []composerAssignment{
		{src: 0, account: 1, origin: origin, fingerprint: fp, fpPresent: true},
		{src: -1}, {src: -1},
	}
	declared := []*md.SlotOrigin{{Origin: origin, Fingerprint: fp, FpPresent: true}, nil, nil}
	c, err := md.ComposeWith(list, declared)
	if err != nil {
		t.Fatalf("md.ComposeWith: %v", err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	if err := composerSelfCheck(st, chunks); err != nil {
		t.Errorf("the self-check REFUSED an honest build with two unseated slots: %v", err)
	}
}
