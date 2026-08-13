package gui

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"seedhammer.com/bip39"
)

// The WORD-PLATE verify — SPEC_systemwide_payloads §7, which had no owner and no
// implementation until plan stage 12.
//
// THE FILE NAME IS LOAD-BEARING. `sysw_verify_test.go` (spec test 16) scans
// every `*_verify.go` file for the `seedEntryFlow` identifier, so this flow sits
// under that structural guarantee from its first line: a verify that could name
// the payload source would compare the engrave's own secret against itself and
// pass unconditionally, certifying a WRONG PLATE as good (§7.4).
//
// SCOPE, per §7.2's 2026-08-12 note: this menu binds the plate whose engraved
// content is the WORDS THEMSELVES — Backup Wallet's mnemonic plates. The bundle
// verifies (singleSigVerifyFlow, multisigVerifyFlow) RE-DERIVE, and a
// re-derivation needs every word, so their full re-entry is arithmetic rather
// than a depth this menu could relax. They are untouched.

// verifyProvenance is §7.1.1, NORMATIVE. Two of §7.2's options produce a result
// the DEVICE DID NOT COMPUTE, so the outcome carries its provenance exactly as a
// record carries its source (§3.2).
type verifyProvenance int

const (
	provDeviceComparedAll verifyProvenance = iota
	provDeviceComparedSubset
	provOperatorAsserted
	provNotVerified
)

// verifyProvenanceLine renders §7.1.1's four strings.
//
// NOTHING MAY RENDER ANY OF THESE AS THE BARE WORD "verified". An operator
// assertion and a device comparison are different facts, and collapsing them is
// the over-claim F-123 was filed against — this time about the plate rather than
// the wipe. (`not verified` contains the word because that is the string §7.1.1
// gives; what is forbidden is a rendering that says "verified" and means one of
// the other three.)
//
// It takes n and total because the enum cannot carry them: `device-compared
// (N of M)` is one outcome with as many renderings as there are subset sizes.
func verifyProvenanceLine(p verifyProvenance, n, total int) string {
	switch p {
	case provDeviceComparedAll:
		return "device-compared (every word)"
	case provDeviceComparedSubset:
		return fmt.Sprintf("device-compared (%d of %d)", n, total)
	case provOperatorAsserted:
		return "operator-asserted"
	default:
		return "not verified"
	}
}

// §7.2's menu. The labels are the spec's, verbatim, and the order is the
// spec's. `even words` and `odd words` are two ROWS here although §7.2's table
// gives them one line: they are two different selections (§7.2.1 defines each
// separately), and a single row meaning both is not something an operator can
// choose.
//
// `skip` is NORMATIVE and R1-I1 is why: §7.1 calls bypass "a menu option, not a
// hidden escape", and without a row for it §7.1.1's `not verified` outcome is
// unreachable by any path.
const (
	depthEvery = iota
	depthEvenWords
	depthOddWords
	depthSix
	depthThree
	depthReadOnly
	depthSkip
)

var verifyMenu = []string{
	"every word",
	"even words",
	"odd words",
	"6 words",
	"3 words",
	"read only",
	"skip",
}

// verifyPositions is §7.2.1, NORMATIVE. It returns 0-INDEXED positions into the
// mnemonic; the spec indexes from 1 and the conversion happens here, once.
//
//   - even/odd are 1-INDEXED over the engraved word list: even is words
//     2, 4, 6 …, odd is 1, 3, 5 …. Stated in the spec because "even" is
//     ambiguous between the two indexings, and an implementer picking the other
//     one produces a verify that silently checks the COMPLEMENT of its label.
//   - 6 words / 3 words are drawn without replacement, uniformly, from all
//     positions, FRESH for every attempt — a fixed set would let a second
//     attempt pass by reciting the same positions, which is a memory test rather
//     than a plate check.
func verifyPositions(depth, total int) []int {
	switch depth {
	case depthEvery:
		pos := make([]int, total)
		for i := range pos {
			pos[i] = i
		}
		return pos
	case depthEvenWords:
		// 1-indexed 2,4,6… -> 0-indexed 1,3,5…
		var pos []int
		for i := 1; i < total; i += 2 {
			pos = append(pos, i)
		}
		return pos
	case depthOddWords:
		// 1-indexed 1,3,5… -> 0-indexed 0,2,4…
		var pos []int
		for i := 0; i < total; i += 2 {
			pos = append(pos, i)
		}
		return pos
	case depthSix:
		return drawPositions(total, 6)
	case depthThree:
		return drawPositions(total, 3)
	}
	return nil
}

// drawPositions draws k distinct positions from [0,total), uniformly and without
// replacement, and returns them in ascending order so the operator reads the
// plate forwards.
//
// THE RNG IS THE DEVICE'S CSPRNG (§7.2.1) — the same source seed entropy comes
// from, never a counter, a tick, or a hash of the mnemonic. A predictable draw
// lets an attacker who controls the plate know which positions go unchecked.
func drawPositions(total, k int) []int {
	if k > total {
		k = total
	}
	perm := make([]int, total)
	for i := range perm {
		perm[i] = i
	}
	// Partial Fisher-Yates: k swaps, each choosing uniformly from what is left.
	for i := 0; i < k; i++ {
		j := i + randIntn(total-i)
		perm[i], perm[j] = perm[j], perm[i]
	}
	out := perm[:k]
	sort.Ints(out)
	return out
}

// randIntn returns a uniform value in [0,n) from crypto/rand.
//
// REJECTION SAMPLING, not `% n`. Taking a 32-bit value modulo n biases the low
// positions whenever n does not divide 2^32, and for the draws here that bias
// falls on exactly the positions an attacker would want left unchecked.
func randIntn(n int) int {
	if n <= 1 {
		return 0
	}
	m := uint64(n)
	limit := (uint64(1) << 32) - ((uint64(1) << 32) % m)
	var b [4]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			// crypto/rand is the same source seed entropy comes from; if it
			// cannot answer, the machine has no business drawing positions and
			// pretending it did.
			panic("gui: crypto/rand: " + err.Error())
		}
		v := uint64(binary.BigEndian.Uint32(b[:]))
		if v < limit {
			return int(v % m)
		}
	}
}

// plateVerifyFlow offers §7.2's menu and returns the outcome with its
// provenance (§7.1.1), having shown it.
//
// THE MENU IS ALWAYS OFFERED AND NEVER FORCED (§7.1, operator ruling
// 2026-08-11). There is no gate, because there is no gate the device can
// honestly evaluate: §5.2 says the machine has no idea what the operator wrote
// down, it sees a button press. Bypass is a menu ROW, not a hidden escape, and
// "read only" is an operator ASSERTION the device records without pretending to
// have confirmed it.
//
// THE BASELINE IS `m` — the just-engraved mnemonic, passed in by the caller and
// still in scope there. Nothing here reads the session: §7.4 forbids the cache
// from answering a verification prompt on the operator's behalf, and this file
// having no way to NAME the session is the mechanism rather than a comment
// saying it does not.
func plateVerifyFlow(ctx *Context, th *Colors, m bip39.Mnemonic) verifyProvenance {
	cs := &ChoiceScreen{Title: "Verify Plate", Lead: "Check the engraved words?", Choices: verifyMenu}
	for !ctx.Done {
		depth, ok := cs.Choose(ctx, th)
		if !ok {
			// Back is the same outcome as `skip`, and is recorded as such:
			// §7.1.1 has no fifth provenance for "the operator left".
			depth = depthSkip
		}
		switch depth {
		case depthSkip:
			return plateVerifyOutcome(ctx, th, provNotVerified, 0, len(m))
		case depthReadOnly:
			if !plateVerifyAssertFlow(ctx, th, m) {
				continue // declined the assertion -> back to the menu
			}
			return plateVerifyOutcome(ctx, th, provOperatorAsserted, 0, len(m))
		}
		for !ctx.Done {
			// FRESH PER ATTEMPT (§7.2.1). The draw is inside the retry loop for
			// that reason and no other; hoisting it would turn a retry into a
			// second recital of the same positions.
			pos := verifyPositions(depth, len(m))
			wrong, done := plateVerifyTypeFlow(ctx, th, m, pos)
			if !done {
				break // Back out of entry -> re-offer the depths
			}
			if len(wrong) == 0 {
				if len(pos) == len(m) {
					return plateVerifyOutcome(ctx, th, provDeviceComparedAll, len(pos), len(m))
				}
				return plateVerifyOutcome(ctx, th, provDeviceComparedSubset, len(pos), len(m))
			}
			if !plateVerifyRetryFlow(ctx, th, wrong) {
				break
			}
		}
	}
	return provNotVerified
}

// plateVerifyTypeFlow prompts the drawn positions on the word keyboard and
// returns the positions that did not match, plus whether entry TERMINATED
// rather than being abandoned.
//
// EVERY POSITION NOT UNDER TEST IS PRE-FILLED, which is what makes the prompt
// name the PLATE's position. inputWordsFlow steps to the next EMPTY slot, and
// draws that slot's own index — so with only the drawn positions empty, the
// operator is asked for "Word 17 of 24" and types word 17, instead of being
// asked for word 3 of 3 and having to be told elsewhere which word that is.
//
// THE CHECKSUM GATE IS OFF, and not only because a subset has no checksum. With
// it on, the keyboard MASKS the last word to the checksum-valid candidates —
// which would stop the operator typing the wrong last word off a mis-cut plate,
// hiding the exact defect this flow exists to find.
func plateVerifyTypeFlow(ctx *Context, th *Colors, m bip39.Mnemonic, pos []int) ([]int, bool) {
	if len(pos) == 0 {
		return nil, false
	}
	typed := emptyBIP39Mnemonic(len(m))
	// The operator's keystrokes off the plate ARE the seed. Scrubbed on every
	// exit, the way deriveXpubFlow scrubs its own copy.
	defer func() {
		for i := range typed {
			typed[i] = 0
		}
	}()
	drawn := make(map[int]bool, len(pos))
	for _, p := range pos {
		drawn[p] = true
	}
	for i := range typed {
		if !drawn[i] {
			typed[i] = 0 // any real word: it reads as FILLED, and is never compared
		}
	}
	if _, done := inputWordsFlow(ctx, th, typed, pos[0], "",
		wordEntryOpts{checksumGate: false}); !done {
		return nil, false
	}
	var wrong []int
	for _, p := range pos {
		if typed[p] != m[p] {
			wrong = append(wrong, p)
		}
	}
	return wrong, true
}

// plateVerifyAssertFlow is §7.2's `read only` row: the device shows the words it
// engraved, the operator compares them to the steel BY EYE, and DECLARES the
// result. The device records the declaration and does not pretend to have
// confirmed it — which is the whole reason §7.1.1 exists.
func plateVerifyAssertFlow(ctx *Context, th *Colors, m bip39.Mnemonic) bool {
	// NoEdit: this is a read-only comparison. An edit affordance here would let
	// the operator change the words after the plate was cut, which is not a typo
	// fix, and nothing downstream re-engraves from it.
	ss := &SeedScreen{NoEdit: true}
	if !ss.Confirm(ctx, th, m) {
		return false
	}
	cs := &ChoiceScreen{
		Title:   "Verify Plate",
		Lead:    "Did the plate match, by eye? The device did not check it.",
		Choices: []string{"NO", "IT MATCHED"},
	}
	choice, ok := cs.Choose(ctx, th)
	return ok && choice == 1
}

// plateVerifyRetryFlow names the mismatched positions and offers a retry, which
// RE-DRAWS (§7.2.1). Returns true to try again.
func plateVerifyRetryFlow(ctx *Context, th *Colors, wrong []int) bool {
	labels := make([]string, len(wrong))
	for i, p := range wrong {
		labels[i] = fmt.Sprintf("%d", p+1) // §7.2.1 indexes from 1
	}
	noun := "Words"
	if len(wrong) == 1 {
		noun = "Word"
	}
	cs := &ChoiceScreen{
		Title:   "Verify Plate",
		Lead:    fmt.Sprintf("%s %s did not match the plate.", noun, strings.Join(labels, ", ")),
		Choices: []string{"GIVE UP", "RETRY"},
	}
	choice, ok := cs.Choose(ctx, th)
	return ok && choice == 1
}

// plateVerifyOutcome shows the result AS ITS PROVENANCE and returns it. The
// screen renders §7.1.1's string and nothing else claims a verification: a
// screen that said "Verified" over an operator assertion would be the collapse
// §7.1.1 forbids.
func plateVerifyOutcome(ctx *Context, th *Colors, p verifyProvenance, n, total int) verifyProvenance {
	showNotice(ctx, th, "Verify Plate", verifyProvenanceLine(p, n, total))
	return p
}
