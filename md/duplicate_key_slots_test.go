package md

import "testing"

// F-218 convergence port. One key seated twice at the same use-site derives an
// identical child at every index, so the policy is satisfiable by fewer parties
// than it names.
//
// The use-site half is what makes this correct rather than merely strict: the
// same xpub at two DIFFERENT multipath branches derives a different child at
// every index — two wallets, not a duplicate — and a check on the key alone
// would refuse it.

func key(pub byte, site UseSite) ExpandedKey {
	k := ExpandedKey{XpubPresent: true, UseSite: site}
	k.Xpub[0] = pub
	k.Xpub[64] = pub
	return k
}

func site(alts ...uint32) UseSite {
	u := UseSite{HasMultipath: len(alts) > 0}
	for _, a := range alts {
		u.Multipath = append(u.Multipath, UseSiteAlt{Value: a})
	}
	return u
}

func TestSameKeySameUseSiteIsADuplicate(t *testing.T) {
	a, b, dup := DuplicateKeySlots([]ExpandedKey{
		{Index: 0, XpubPresent: true, UseSite: site(0, 1)},
		{Index: 1, XpubPresent: true, UseSite: site(0, 1)},
	})
	if !dup || a != 0 || b != 1 {
		t.Fatalf("two identical slots were not reported: a=%d b=%d dup=%v", a, b, dup)
	}
}

func TestSameKeyDifferentUseSiteIsNotADuplicate(t *testing.T) {
	k0 := key(7, site(0, 1))
	k1 := key(7, site(2, 3))
	k0.Index, k1.Index = 0, 1
	if _, _, dup := DuplicateKeySlots([]ExpandedKey{k0, k1}); dup {
		t.Fatal("one key at two DIFFERENT branches was called a duplicate; they derive different children")
	}
}

func TestDifferentKeysAreNotADuplicate(t *testing.T) {
	k0, k1 := key(1, site(0, 1)), key(2, site(0, 1))
	k0.Index, k1.Index = 0, 1
	if _, _, dup := DuplicateKeySlots([]ExpandedKey{k0, k1}); dup {
		t.Fatal("two distinct keys were called a duplicate")
	}
}

// A slot with no xpub has no key to duplicate.
func TestSlotsWithoutKeysAreSkipped(t *testing.T) {
	k0 := key(1, site(0, 1))
	k1 := ExpandedKey{Index: 1, XpubPresent: false, UseSite: site(0, 1)}
	k0.Index = 0
	if _, _, dup := DuplicateKeySlots([]ExpandedKey{k0, k1}); dup {
		t.Fatal("a keyless slot was reported as a duplicate")
	}
}
