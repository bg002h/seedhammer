package sysw

import "testing"

// TestSyswMDMKRunGroupBoundByFirstStringsCount is the Go twin of mnemonic-engrave's
// rv_mdmk_run_group_is_bound_by_the_first_strings_count (S1 §14d): a group is full
// at its FIRST string's count, so a later string that says a smaller count still
// joins. Records: that row's three rewritten md1 strings (counts 3, 3, 2).
func TestSyswMDMKRunGroupBoundByFirstStringsCount(t *testing.T) {
	recs := []string{
		"md1fv9wjpqpqpm6jzzqqvqpdqnf4ztqq4gy99tzyzyzdv7xh9vpdwu3t7dhhesk2tl3",
		"md1fv9wjpqg0yq82l0czvx85ae43vtfd26hsmngjecmqy44k2pgttqh74qwxlawq374",
		"md1fv9wjqsgp2026hh65xpvugtfhd9792zxgunymm0a82pdju6442qscgcgwld9j2ls",
	}
	g := mdmkGroups(recs)
	if len(g) != 1 || len(g[0].idxs) != 3 {
		t.Fatalf("groups %+v, want one group of all three (bound by the first string's count, 3)", g)
	}
}
