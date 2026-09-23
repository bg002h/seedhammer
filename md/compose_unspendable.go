package md

// The composer's unspendable-key REQUEST and SPEC §6's kind-1 mint refusals
// (F-449 stage 4, F-654). A port of md-codec 0.47.0: compose::UnspendableKind
// and compose_with's third argument (compose/mod.rs, compose/tr.rs), and
// validate::validate_unspendable_shape (validate.rs:585-640), which md-cli's
// `md compose --unspendable liana` calls before it emits anything
// (cmd/compose.rs liana_refuse_or_warn).
//
// A REQUEST, NOT A WIRE CONCEPT. InternalKeyKind is what lands on the wire;
// UnspendableKind is what the operator asked for. The Rust primary keeps the
// two apart on purpose (stage 2 plan, Type consistency) and so does this port:
// under a tr whose first bare single-key path becomes a REAL internal key, a
// Liana request has nothing to select, and the built tree says InternalKeySlot.
//
// THE REFUSALS ARE NOT IN encodePayload, AND MUST NEVER MOVE THERE. Reassemble
// re-encodes every decoded card to check its chunk-set id (Reassemble ->
// computeEncodingID -> encodePayload), so a refusal there would make this
// device reject cards the Rust primary decodes; Rust keeps them mint-only for
// the same reason (encode.rs:242-253). They live here, on the composer's
// mint path, and the composer calls them before it builds a chunk.

import (
	"errors"
	"fmt"
)

// UnspendableKind selects the taproot internal key a tr composition uses when
// no path supplies a real one. UnspendableNums is the zero value and the only
// kind the composer built before F-449 stage 4.
type UnspendableKind uint8

const (
	// UnspendableNums is BIP-341's raw H point (wire kind 0).
	UnspendableNums UnspendableKind = iota
	// UnspendableLiana is Liana's unspendable xpub over the composed leaf set
	// (SPEC §2, wire kind 1).
	UnspendableLiana
)

// ComposeWithUnspendable is ComposeWith with the internal-key request md-codec's
// compose_with takes as its third argument. ComposeWith is this with
// UnspendableNums, so every caller that predates F-449 composes what it always
// composed.
func ComposeWithUnspendable(list PathList, declared []*SlotOrigin, unspendable UnspendableKind) (Composed, error) {
	slots, err := ValidatePathList(list)
	if err != nil {
		return Composed{}, err
	}
	if len(declared) != slots {
		return Composed{}, fmt.Errorf("%w: %d given, policy has %d", ErrComposeWrongSlotCount, len(declared), slots)
	}
	c, err := lowerPathList(list, declared, unspendable)
	if err != nil {
		return Composed{}, err
	}
	c.requested = unspendable
	return c, nil
}

// UnspendableRequestUnmet is md-codec's Composed::unspendable_request_unmet: a
// Liana key was requested and the built tree does not carry one. Under tr that
// means a real internal key was extracted (SPEC §6 row 3); under wsh/sh there
// is no internal key at all. Never true for UnspendableNums.
func (c Composed) UnspendableRequestUnmet() bool {
	return c.requested == UnspendableLiana && c.internalKey() != InternalKeyLianaUnspendable
}

// internalKey is the built tree's root taproot internal key, or InternalKeySlot
// for a non-tr root (there is no unspendable key to report).
func (c Composed) internalKey() InternalKeyKind {
	if c.d == nil || c.d.tree.tag != tagTr {
		return InternalKeySlot
	}
	b, ok := c.d.tree.body.(trBody)
	if !ok {
		return InternalKeySlot
	}
	return b.ik
}

var (
	// ErrUnspendableSortedMultiA is SPEC §6 row 1: a belt against a port that
	// feeds MultiALeafScript's derived-key-sorted list into the recipe.
	ErrUnspendableSortedMultiA = errors.New("md: a Liana unspendable key cannot sit over a sortedmulti_a leaf")
	// ErrUnspendableUseSite is SPEC §6 row 2: Liana derives the internal key at
	// 0/i and 1/i, so any other use-site would give the device and Liana two
	// different wallets.
	ErrUnspendableUseSite = errors.New("md: a Liana unspendable key needs the <0;1>/* use-site on every key")
	// ErrUnspendableNotRootTr is SPEC §6 row 4.
	ErrUnspendableNotRootTr = errors.New("md: a Liana unspendable key is allowed only on the root tr()")
)

// ValidateUnspendableShape is md-codec's validate_unspendable_shape over this
// composition: SPEC §6 rows 1, 2 and 4. nil when the tree holds no Liana key.
func (c Composed) ValidateUnspendableShape() error {
	if c.d == nil {
		return nil
	}
	return validateUnspendableShape(c.d)
}

func validateUnspendableShape(d *descriptor) error {
	if err := rejectNestedUnspendable(d.tree, true); err != nil {
		return err
	}
	b, ok := d.tree.body.(trBody)
	if d.tree.tag != tagTr || !ok || b.ik != InternalKeyLianaUnspendable {
		return nil
	}
	if b.tree != nil && containsSortedMultiA(*b.tree) {
		return ErrUnspendableSortedMultiA
	}
	if !isStandardMultipath(d.useSite) {
		return ErrUnspendableUseSite
	}
	for _, o := range d.tlv.useSiteOverrides {
		if !isStandardMultipath(o.path) {
			return ErrUnspendableUseSite
		}
	}
	return nil
}

// isStandardMultipath is md-codec's UseSitePath::standard_multipath(): <0;1>/*
// with no hardening anywhere.
func isStandardMultipath(u useSitePath) bool {
	return u.hasMultipath && !u.wildcardHardened && len(u.multipath) == 2 &&
		u.multipath[0] == (alternative{hardened: false, value: 0}) &&
		u.multipath[1] == (alternative{hardened: false, value: 1})
}

// rejectNestedUnspendable refuses a Liana key on any tr() other than the root,
// recursing through every body that holds children -- the Rust walk's shape.
func rejectNestedUnspendable(n node, isRoot bool) error {
	switch b := n.body.(type) {
	case trBody:
		if b.ik == InternalKeyLianaUnspendable && !isRoot {
			return ErrUnspendableNotRootTr
		}
		if b.tree != nil {
			return rejectNestedUnspendable(*b.tree, false)
		}
	case childrenBody:
		for _, c := range b.children {
			if err := rejectNestedUnspendable(c, false); err != nil {
				return err
			}
		}
	case variableBody:
		for _, c := range b.children {
			if err := rejectNestedUnspendable(c, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// containsSortedMultiA reports a sortedmulti_a node anywhere in n, recursing
// into nested tr() trees as validate.rs does (its fix round 2, M1).
func containsSortedMultiA(n node) bool {
	if n.tag == tagSortedMultiA {
		return true
	}
	switch b := n.body.(type) {
	case trBody:
		return b.tree != nil && containsSortedMultiA(*b.tree)
	case childrenBody:
		for _, c := range b.children {
			if containsSortedMultiA(c) {
				return true
			}
		}
	case variableBody:
		for _, c := range b.children {
			if containsSortedMultiA(c) {
				return true
			}
		}
	}
	return false
}
