package md

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
	"seedhammer.com/bip380"
)

// keyedConformanceRecord mirrors `<name>.conformance.json`, emitted by
// `md vectors` in the primary Rust repo (descriptor-mnemonic).
type keyedConformanceRecord struct {
	Name                       string  `json:"name"`
	Template                   string  `json:"template"`
	Path                       *string `json:"path"` // retained: names the elided-origin vectors F-212 was about
	Md1EncodingID              string  `json:"md1_encoding_id"`
	WalletDescriptorTemplateID string  `json:"wallet_descriptor_template_id"`
	WalletPolicyID             string  `json:"wallet_policy_id"`
	Chains                     map[string]struct {
		Descriptor string   `json:"descriptor"`
		Addresses  []string `json:"addresses"`
	} `json:"chains"`
	// Keys is the primary's per-slot account xpub, carrying the REAL parent
	// fingerprint (the descriptor's carry zero). F-630's gate binds the two
	// spellings over the 65 bytes they share, never over the base58 string.
	Keys []struct {
		Index uint8  `json:"index"`
		Xpub  string `json:"xpub"`
	} `json:"keys"`
}

// TestKeyedConformanceAgreesWithRust is the CROSS-LANGUAGE gate R3 exists for.
//
// Until these vectors landed, every entry in the primary's MANIFEST was
// keyless, so this port could agree with Rust about every byte on the wire and
// still compute a different wallet id — and nothing would have said so. The
// records carry real xpubs, so the identities below are key-DEPENDENT and a
// divergence in key handling shows up here rather than on someone's steel.
//
// The keys are BIP-39's published test mnemonic ("abandon … about"); never put
// funds behind them.
//
// SCOPE, stated rather than implied: this checks the identities the Go port
// computes today. Address derivation for taproot script trees is NOT checked
// because this port cannot do it yet (address/address.go derives SortedMulti
// and Singlesig only) — that is Stage 3, and the sub-test below records which
// shapes are waiting rather than passing over them in silence.
func TestKeyedConformanceAgreesWithRust(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "vectors", "keyed_*.conformance.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no keyed_*.conformance.json vendored — the cross-language gate is checking NOTHING")
	}

	checked := 0
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".conformance.json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			var rec keyedConformanceRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				t.Fatalf("parse %s: %v", p, err)
			}

			chunks := loadPhraseChunks(t, name)
			if len(chunks) == 0 {
				t.Fatalf("%s: no md1 chunks in the vendored phrase", name)
			}

			// The card must decode at all. A keyed full-policy card is chunked
			// (real xpubs exceed codex32's single-string cap), so this also
			// exercises reassembly.
			if _, err := DecodeChunks(chunks); err != nil {
				t.Fatalf("%s: DecodeChunks: %v", name, err)
			}

			// WalletPolicyId is KEY-DEPENDENT — the whole reason a keyless
			// corpus could not gate it.
			got, err := WalletPolicyIdChunks(chunks)
			if err != nil {
				t.Fatalf("%s: WalletPolicyIdChunks: %v", name, err)
			}
			gotHex := hex.EncodeToString(got[:])

			// F-212 IS CLOSED (2026-08-20): every vector must agree, elided
			// origin or not.
			//
			// This arm used to pin a GAP. Go and Rust disagreed here whenever the
			// origin was elided -- rust c79039c5…, go 260f334a… for one wallet --
			// because Rust canonical-fills an empty origin before hashing and this
			// port hashed it as-is. The omission cited "R0-I2" as a deliberate
			// divergence; R0-I2 is a different ruling (OriginPath's type shape),
			// and R0-I1 REQUIRES the fallback. The port converged, per the
			// Rust-primary rule.
			//
			// The pinned-gap arm was written to fire when the divergence was
			// FIXED, and it did. That is why it is gone rather than forgotten.
			if gotHex != rec.WalletPolicyID {
				t.Errorf("%s: wallet_policy_id\n  go:   %s\n  rust: %s",
					name, gotHex, rec.WalletPolicyID)
			}

			// And the template id, which is key-STABLE: the two must not be
			// equal, or a consumer comparing the wrong one would still appear
			// to match.
			d, err := Reassemble(chunks)
			if err != nil {
				t.Fatalf("%s: Reassemble: %v", name, err)
			}
			tid, err := WalletDescriptorTemplateId(d)
			if err != nil {
				t.Fatalf("%s: WalletDescriptorTemplateId: %v", name, err)
			}
			if want := rec.WalletDescriptorTemplateID; hex.EncodeToString(tid[:]) != want {
				t.Errorf("%s: wallet_descriptor_template_id\n  go:   %s\n  rust: %s",
					name, hex.EncodeToString(tid[:]), want)
			}
			if rec.WalletPolicyID == rec.WalletDescriptorTemplateID {
				t.Errorf("%s: the two ids are EQUAL in the record — comparing the "+
					"wrong one against a coordinator would silently appear to match", name)
			}
			checked++
		})
	}
	if checked == 0 {
		t.Fatal("every keyed vector was skipped; the gate asserted nothing")
	}
}

// loadPhraseChunks reads a vendored `.phrase.txt`, dropping the `chunk-set-id:`
// header a chunked card carries and stripping the display separators an
// operator's re-typed card would not have.
func loadPhraseChunks(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "phrase.txt"))
	if err != nil {
		t.Fatalf("read %s.phrase.txt: %v", name, err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	return out
}

// ─── F-630: the rendered descriptor, gated ───────────────────────────────────
//
// TestKeyedConformanceDescriptorsAgreeWithTheirTemplates is the gate F-630 was
// filed to produce. The sibling test above reads a record's IDs and ADDRESSES;
// nothing in this repository had ever parsed a `chains[].descriptor`, so
// md-codec 0.44.0's xpub-header defect — every rendered key serialised at
// depth 0 / child 0 under a four-component origin — sat in the vendored corpus
// with the whole suite green, and a corpus-wide derivation-suffix regression
// across 92 of 92 chain descriptors would have been just as silent.
//
// Six clauses, each closing a measured mutation:
//
//	D1   the HEADER: the xpub base58-decodes with a valid checksum, its depth
//	     equals the number of origin components, its child number is the
//	     terminal origin component (hardened-encoded; 0 for an empty path),
//	     and its parent fingerprint is zero (the parent point is not on the
//	     md1 wire).
//	D1'  the TEMPLATE: each descriptor reduces back to the record's own
//	     `template` — replace every `[fp/origin]xkey` with `@N/origin`, where N
//	     is the slot whose 65 bytes match, and compare STRICTLY against
//	     `template` with `<0;1>` resolved to the chain. D1 stops at the key, so
//	     everything after it (the derivation suffix, operand order, threshold,
//	     script wrapper) was parsed by nobody. This compares the primary's
//	     hand-authored input literal against the primary's rendering of it.
//	D1"  the CHECKSUM, which the reduction strips and D1 never reaches.
//	D2a  the record's two spellings agree: every descriptor key's 65 bytes are
//	     some `keys[]` entry's. A uniformly wrong corpus agrees with itself —
//	     that is exactly how 0.44.0 survived — so this binds them.
//	D2b  and they agree with the GO PORT's expansion of the same card, with
//	     every Go slot carrying an xpub appearing at least once. That clause is
//	     what makes this cross-language rather than a JSON self-consistency
//	     check.
//	D2c  the origin bracket's FINGERPRINT equals the Go port's. In the Rust
//	     function 0.44.0 rewrote, the fingerprint is assembled on the same line
//	     as the path, so a regression there emits a uniformly wrong fingerprint
//	     that agrees with itself; addresses do not depend on it and
//	     wallet_policy_id is computed from the card, so no other test can see it.
//	D3   the origin bracket's PATH equals the Go port's origin path, with a
//	     two-sided allowlist for the deliberate R0-I1 elided-origin divergence.
//
// Hardening is spelled differently on the two sides (the fork renders `48h`,
// the record carries `48'`), so every path comparison goes through
// bip32.Path.String() rather than comparing raw text.
//
// SCOPE, stated rather than implied: an xprv forged with the rendered xpub's
// header passes every clause. It is unreachable from the primary — the md1
// wire carries no private material — and it is a secret-handling defect, which
// per the operator's 2026-08-27 ruling is logged rather than gated.

// descriptorKeyRe matches one `[fingerprint/origin]xkey` in a rendered
// descriptor. The key runs to the first non-base58 byte, which is the `/` of
// the derivation suffix, a `,` or a `)` — never part of the key.
//
// A key this regexp FAILS to match cannot hide: the reduction below would
// leave the raw `[fp/path]xpub` in place where the template carries `@N`, so
// D1' reds. The regexp is not load-bearing on its own.
var descriptorKeyRe = regexp.MustCompile(`\[([0-9a-fA-F]{8})((?:/[0-9]+['hH]?)*)\]([1-9A-HJ-NP-Za-km-z]+)`)

// forkbuiltRecordPins is the EXACT set of vector names allowed to carry a
// fork-side conformance record in md/testdata/forkbuilt/ (D5g).
//
// A pinned record is NOT a vendored one: it is a fork-maintained fixture
// preserving a policy the primary DROPPED (descriptor-mnemonic b2c5d693
// replaced the reuse-bearing policy under each of these names with a
// reuse-free one), so no corrected rendering of it exists or can be obtained.
// Its value is its addresses and ids, which 0.44.0 did not move.
//
// THE MEMBERSHIP IS ASSERTED, NOT INFERRED FROM A FILE'S EXISTENCE. Selecting
// the relaxed tier by "does forkbuilt/<name>.conformance.json exist" would make
// the shortest path from a red gate to a green one "pin the regression": import
// a genuine header regression for any vector, copy the defective record into
// forkbuilt/, and the whole suite goes green precisely because that record
// carries the legacy shape the relaxed arm accepts. A fourth pinned record is a
// deliberate, visible act or it is a bug.
var forkbuiltRecordPins = []string{
	"keyed_tr_multi_a",
	"keyed_tr_sortedmulti_a",
	"keyed_wsh_timelock_hashlock",
}

// elidedOriginDivergence pins BOTH SIDES of the deliberate R0-I1 divergence
// (D3). For these two the Rust record emits a bare-fingerprint origin (depth 0,
// which is already right) while the Go port canonical-fills from
// canonicalOrigin(tree) and renders three components. Both satisfy D1
// independently.
//
// Pinning only the Go half is not enough, and that was measured: a record
// re-pointed to account 9h — consistently, header and bracket together — passed
// every other clause and would have shipped a wrong-account descriptor on the
// two plainest single-key vectors in the corpus. So the record's expected
// bracket is pinned in full, fingerprint included.
var elidedOriginDivergence = map[string]struct {
	goPath  string // what canonicalOrigin(tree) fills in
	bracket string // the record's origin bracket, verbatim and complete
}{
	"keyed_wpkh":       {goPath: "m/84h/0h/0h", bracket: "[73c5da0a]"},
	"keyed_tr_keyonly": {goPath: "m/86h/0h/0h", bracket: "[73c5da0a]"},
}

func TestKeyedConformanceDescriptorsAgreeWithTheirTemplates(t *testing.T) {
	// D5g's membership assertion, as a t.Errorf and NEVER a t.Fatalf: at T1 of
	// this cycle the pinned set is legitimately empty, and a fatal assertion
	// here aborts before a single vector is examined — hiding the whole
	// acceptance measurement behind one unrelated line.
	pinned := forkbuiltPinnedRecordSet(t)
	wantPinned := map[string]bool{}
	for _, n := range forkbuiltRecordPins {
		wantPinned[n] = true
		if !pinned[n] {
			t.Errorf("no fork-side record at testdata/forkbuilt/%s.conformance.json; "+
				"the pinned tier names it but nothing is preserving it", n)
		}
	}
	for n := range pinned {
		if !wantPinned[n] {
			t.Errorf("testdata/forkbuilt/%s.conformance.json is pinned but is not one of "+
				"the %d names this gate allows a relaxed header for. A pinned record is "+
				"exempt from D1's header clause, so adding one is how a genuine header "+
				"regression would be made green -- name it here deliberately or delete it",
				n, len(forkbuiltRecordPins))
		}
	}

	paths, err := filepath.Glob(filepath.Join("testdata", "vectors", "keyed_*.conformance.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no keyed_*.conformance.json vendored — the descriptor gate is checking NOTHING")
	}

	passed, failed, legacyPassed := 0, 0, 0
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".conformance.json")
		legacy := pinned[name]
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		ok := t.Run(name, func(t *testing.T) {
			var rec keyedConformanceRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				t.Fatalf("parse %s: %v", p, err)
			}
			assertDescriptorsAgree(t, name, rec, loadPhraseChunks(t, name), legacy)
		})
		switch {
		case !ok:
			failed++
		case legacy:
			passed++
			legacyPassed++
		default:
			passed++
		}
	}
	if passed+failed == 0 {
		t.Fatal("every keyed vector was skipped; the gate asserted nothing")
	}
	t.Logf("descriptor gate: %d of %d vectors pass (%d correct-header, %d pinned-legacy), %d fail",
		passed, passed+failed, passed-legacyPassed, legacyPassed, failed)
}

// assertDescriptorsAgree runs D1, D1', D1", D2a, D2b, D2c and D3 over every
// chain descriptor of one record. `legacy` selects D5g's pinned-tier header
// shape in place of D1's; NOTHING ELSE relaxes, because these three records are
// the fork's own fixtures and no upstream re-vendor will ever correct a defect
// in them.
func assertDescriptorsAgree(t *testing.T, name string, rec keyedConformanceRecord, chunks []string, legacy bool) {
	t.Helper()

	recByIndex := map[uint8][65]byte{}
	for _, k := range rec.Keys {
		parsed, err := parseExtendedKey(k.Xpub)
		if err != nil {
			t.Fatalf("keys[@%d] %q: %v", k.Index, k.Xpub, err)
		}
		recByIndex[k.Index] = parsed.material
	}
	if len(recByIndex) == 0 {
		t.Fatalf("%s: the record carries no keys[] — D2a would assert nothing", name)
	}
	recSlotOf := slotsByMaterial(t, "the record's keys[]", recByIndex)

	// The SAME card, expanded by the Go port (D2b). This is the clause that
	// makes the gate cross-language: without it the whole test is one JSON
	// file agreeing with itself.
	_, keys, err := ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("%s: ExpandWalletPolicyChunks: %v", name, err)
	}
	goByIndex := map[uint8][65]byte{}
	goKeys := map[uint8]ExpandedKey{}
	for _, k := range keys {
		goKeys[k.Index] = k
		if k.XpubPresent {
			goByIndex[k.Index] = k.Xpub
		}
	}
	if len(goByIndex) == 0 {
		t.Fatalf("%s: the Go expansion carries no xpub in any slot — D2b would assert nothing", name)
	}
	goSlotOf := slotsByMaterial(t, "the Go port's expansion", goByIndex)

	seenGoSlot := map[uint8]bool{}
	chains := make([]string, 0, len(rec.Chains))
	for c := range rec.Chains {
		chains = append(chains, c)
	}
	slices.Sort(chains)

	for _, chain := range chains {
		desc := rec.Chains[chain].Descriptor
		if desc == "" {
			t.Errorf("chain %s: the record carries no descriptor", chain)
			continue
		}

		// D1": the checksum, which the reduction below strips and the header
		// clause never reaches.
		body, sum, hasSum := strings.Cut(desc, "#")
		switch {
		case !hasSum:
			t.Errorf("chain %s: the descriptor carries no BIP-380 checksum", chain)
		case !bip380.ValidChecksum(body, sum):
			t.Errorf("chain %s: BIP-380 checksum #%s is wrong for the descriptor body", chain, sum)
		}

		matches := descriptorKeyRe.FindAllStringSubmatchIndex(body, -1)
		if len(matches) == 0 {
			t.Errorf("chain %s: no [origin]xkey in the descriptor — a keyed vector renders at "+
				"least one, so this gate would be asserting nothing about it", chain)
			continue
		}

		var reduced strings.Builder
		prev := 0
		for _, m := range matches {
			whole := body[m[0]:m[1]]
			fp := strings.ToLower(body[m[2]:m[3]])
			originText := body[m[4]:m[5]]
			xkey := body[m[6]:m[7]]

			reduced.WriteString(body[prev:m[0]])
			prev = m[1]

			parsed, err := parseExtendedKey(xkey)
			if err != nil {
				t.Errorf("chain %s: %s…: %v", chain, whole[:min(len(whole), 24)], err)
				reduced.WriteString(whole)
				continue
			}
			comps, err := bracketComponents(originText)
			if err != nil {
				t.Errorf("chain %s: origin %q: %v", chain, originText, err)
				reduced.WriteString(whole)
				continue
			}

			// D1 / D5g: the header.
			if legacy {
				// The pinned tier's legacy shape asserted EXACTLY. A record whose
				// header is neither this nor the correct one fails, so a third
				// staleness cannot slip in; one that becomes correct fails too,
				// which tells us the primary has re-shipped the policy.
				if parsed.depth != 0 || parsed.child != 0 || len(comps) == 0 {
					t.Errorf("chain %s: pinned record's header is depth %d child %d under %d origin "+
						"component(s); the fork-maintained fixture preserves md-codec's pre-0.44.0 "+
						"shape, which is depth 0 / child 0 under a NON-EMPTY origin",
						chain, parsed.depth, parsed.child, len(comps))
				}
			} else {
				if int(parsed.depth) != len(comps) {
					t.Errorf("chain %s: [%s%s] has %d origin component(s) but the xpub serialises at "+
						"depth %d", chain, fp, originText, len(comps), parsed.depth)
				}
				wantChild := uint32(0)
				if len(comps) > 0 {
					wantChild = comps[len(comps)-1]
				}
				if parsed.child != wantChild {
					t.Errorf("chain %s: [%s%s] ends at child %d but the xpub serialises child %d",
						chain, fp, originText, wantChild, parsed.child)
				}
			}
			if parsed.parentFP != 0 {
				t.Errorf("chain %s: [%s%s] xpub carries parent fingerprint %08x, want 0 — the parent "+
					"point is not on the md1 wire", chain, fp, originText, parsed.parentFP)
			}

			// D2a: the record's own two spellings.
			slot, inRecord := recSlotOf[parsed.material]
			if !inRecord {
				t.Errorf("chain %s: [%s%s]'s key material is in no keys[] entry of the same record",
					chain, fp, originText)
				reduced.WriteString(whole)
				continue
			}

			// D2b/D2c/D3: the Go port's expansion of the same card.
			if goSlot, inGo := goSlotOf[parsed.material]; !inGo {
				t.Errorf("chain %s: [%s%s]'s key material is in no slot of the Go port's expansion "+
					"of the same card", chain, fp, originText)
			} else {
				seenGoSlot[goSlot] = true
				gk := goKeys[goSlot]
				if !gk.FingerprintPresent {
					t.Errorf("chain %s: the Go expansion's @%d declares no fingerprint, so the "+
						"record's [%s…] is bound to nothing", chain, goSlot, fp)
				} else if got := hex.EncodeToString(gk.Fingerprint[:]); got != fp {
					t.Errorf("chain %s: origin bracket fingerprint\n  record: %s\n  go:     %s",
						chain, fp, got)
				}
				bracketPath := bip32.Path(comps).String()
				goPath := gk.OriginPath.String()
				if d, allowed := elidedOriginDivergence[name]; allowed {
					if goPath != d.goPath {
						t.Errorf("chain %s: the elided-origin divergence is PINNED for this vector: the "+
							"Go port canonical-fills %s, but it now fills %s", chain, d.goPath, goPath)
					}
					if got := "[" + fp + originText + "]"; got != d.bracket {
						t.Errorf("chain %s: the record's origin bracket is pinned at %s for this vector "+
							"and now reads %s — a bracket re-pointed CONSISTENTLY with its header "+
							"passes every other clause", chain, d.bracket, got)
					}
					if goPath == bracketPath {
						t.Errorf("chain %s: %s no longer diverges (both sides say %s); delete its "+
							"elidedOriginDivergence entry rather than leaving a stale allowlist",
							chain, name, goPath)
					}
				} else if bracketPath != goPath {
					t.Errorf("chain %s: origin path for @%d\n  record: %s\n  go:     %s\n"+
						"(only keyed_wpkh and keyed_tr_keyonly are allowed to diverge here, and both "+
						"sides of that divergence are pinned)", chain, goSlot, bracketPath, goPath)
				}
			}

			fmt.Fprintf(&reduced, "@%d%s", slot, originText)
		}
		reduced.WriteString(body[prev:])

		// D1': the reduction, compared STRICTLY. D3 normalises hardening
		// because it compares PATHS; this compares whole strings, both
		// spellings are exact across the corpus, so strict is free and
		// additionally catches a bracket spelling flip.
		want := strings.ReplaceAll(rec.Template, "<0;1>", chain)
		if strings.ContainsAny(want, "<>") {
			t.Errorf("chain %s: the template carries a multipath spelling this gate does not "+
				"resolve: %s", chain, rec.Template)
		}
		if got := reduced.String(); got != want {
			t.Errorf("chain %s: the descriptor does not reduce to the record's own template\n"+
				"  got  %s\n  want %s", chain, got, want)
		}
	}

	// D2b's second half: every Go slot carrying an xpub must appear. Without
	// it a descriptor that simply DROPPED a key would satisfy every clause
	// above, since each clause quantifies over what the descriptor contains.
	missing := make([]int, 0, len(goByIndex))
	for idx := range goByIndex {
		if !seenGoSlot[idx] {
			missing = append(missing, int(idx))
		}
	}
	slices.Sort(missing)
	for _, idx := range missing {
		t.Errorf("the Go port's @%d carries an xpub that appears in no chain descriptor of the record", idx)
	}
}

// parsedExtendedKey is the part of a serialised BIP-32 extended key this gate
// asserts over. material is (chain code ‖ compressed pubkey), which is what the
// record's keys[] and the rendered descriptor SHARE — the base58 strings differ,
// because keys[] carries the real parent fingerprint and the descriptor's
// carries zero.
type parsedExtendedKey struct {
	depth    uint8
	parentFP uint32
	child    uint32
	material [65]byte
}

// parseExtendedKey base58-decodes and CHECKSUM-VALIDATES an extended key
// (hdkeychain.NewKeyFromString rejects a bad checksum, a wrong length and a
// pubkey off the curve).
func parseExtendedKey(s string) (parsedExtendedKey, error) {
	k, err := hdkeychain.NewKeyFromString(s)
	if err != nil {
		return parsedExtendedKey{}, err
	}
	pub, err := k.ECPubKey()
	if err != nil {
		return parsedExtendedKey{}, err
	}
	out := parsedExtendedKey{depth: k.Depth(), parentFP: k.ParentFingerprint(), child: k.ChildIndex()}
	cc := k.ChainCode()
	if len(cc) != 32 {
		return parsedExtendedKey{}, fmt.Errorf("chain code is %d bytes, want 32", len(cc))
	}
	copy(out.material[:32], cc)
	copy(out.material[32:], pub.SerializeCompressed())
	return out, nil
}

// slotsByMaterial inverts slot → material into material → slot, and FAILS
// LOUDLY when that inversion is not single-valued.
//
// "The slot whose 65 bytes match" has no answer when two slots carry the same
// material. A plain map-based inversion silently keeps one of them and then
// produces a FALSE RED everywhere the template names the other — a gate
// reporting a defect that is not there is worse than one reporting nothing.
// Measured 0 of 46 records at descriptor-mnemonic b2c5d693, so this costs
// nothing today; it exists so that the day it costs something, it says so.
func slotsByMaterial(t *testing.T, what string, byIndex map[uint8][65]byte) map[[65]byte]uint8 {
	t.Helper()
	group := map[[65]byte][]int{}
	for idx, mat := range byIndex {
		group[mat] = append(group[mat], int(idx))
	}
	out := make(map[[65]byte]uint8, len(group))
	for mat, idxs := range group {
		if len(idxs) != 1 {
			slices.Sort(idxs)
			t.Fatalf("ambiguous slot material in %s: slots %v carry identical "+
				"(chain code ‖ compressed pubkey) bytes, so \"the slot whose 65 bytes match\" "+
				"has no single answer; this gate refuses to guess", what, idxs)
		}
		out[mat] = uint8(idxs[0])
	}
	return out
}

// bracketComponents parses the path part of an origin bracket — the text after
// the 8-hex fingerprint, e.g. "/48'/0'/0'/2'" — into hardened-in-band uint32s,
// the same encoding md.ExpandedKey.OriginPath uses (R0-I2). An empty string is
// a bare-fingerprint origin and yields no components.
func bracketComponents(s string) ([]uint32, error) {
	if s == "" {
		return nil, nil
	}
	var out []uint32
	for _, part := range strings.Split(strings.TrimPrefix(s, "/"), "/") {
		hardened := strings.HasSuffix(part, "'") || strings.HasSuffix(part, "h") || strings.HasSuffix(part, "H")
		digits := part
		if hardened {
			digits = part[:len(part)-1]
		}
		if digits == "" {
			return nil, fmt.Errorf("empty component in %q", s)
		}
		var v uint64
		for _, c := range digits {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("non-numeric component %q in %q", part, s)
			}
			v = v*10 + uint64(c-'0')
			if v >= hdkeychain.HardenedKeyStart {
				return nil, fmt.Errorf("component %q in %q is out of range", part, s)
			}
		}
		if hardened {
			v += hdkeychain.HardenedKeyStart
		}
		out = append(out, uint32(v))
	}
	return out, nil
}

// forkbuiltPinnedRecordSet enumerates the fork-side conformance record pins
// actually on disk. The set is COMPARED against forkbuiltRecordPins above; it
// never decides membership on its own.
func forkbuiltPinnedRecordSet(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "forkbuilt", "*.conformance.json"))
	if err != nil {
		t.Fatalf("glob testdata/forkbuilt: %v", err)
	}
	out := map[string]bool{}
	for _, p := range paths {
		out[strings.TrimSuffix(filepath.Base(p), ".conformance.json")] = true
	}
	return out
}
