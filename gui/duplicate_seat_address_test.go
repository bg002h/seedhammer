package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/address"
	"seedhammer.com/md"
)

// ─── F-531: one slot in two seats, and the address that came out wrong ───────
//
// expandedToDescriptor projected a repeated-seat multisig onto a flat
// bip380.Descriptor with ONE KEY PER SLOT while carrying tpl.K over unchanged.
// For wsh(sortedmulti(1,@0,@0,@1)) that is a 1-of-2 -- a different script, and
// so a different address -- shown under a screen labelled "1-of-3".
//
// MEASURED AGAINST BITCOIN CORE 25.0.0, not reasoned about. A throwaway regtest
// datadir, getdescriptorinfo + deriveaddresses, keys version-swapped to tpub;
// the witness programs below are Core's, transcribed from bcrt1 to bc1 by the
// emitter that produced the identical program:
//
//	wsh(sortedmulti(1,A,A,B))  bcrt1qvljqpugpaf8w3t898gw2c26yta833xz4gcuznjhuzl5g8j2dxezqqqkckr
//	wsh(sortedmulti(1,A,B))    bcrt1q2gu6t4m7ax8vzl37th5jyjjwj9l3zy84h0yrwaredxy6fxdvl87s0ufenu
//	wsh(sortedmulti(2,A,A,B))  bcrt1qaej2r8zv5l00dndh7u6ch5w5pn8lrv70vayz3xu99j3y2emqdghqs9h244
//	wsh(sortedmulti(2,A,B))    bcrt1qsl0stsxz6630afvzkamm9x9jj0mgsf3dcesnr2x967m62hnukwxqqz65am
//
// Core ACCEPTS all four -- the repeat is no obstacle to importing or funding
// it, which is exactly why showing the wrong one was dangerous. Row 1 is what
// the emitter derives; row 2 is what the flat route derived and showed. The
// flat route was not approximately right, it was answering a different
// question.
//
// THE DEVICE NOW DERIVES NO ADDRESS AT ALL for this shape. That is the
// operator's standing ruling (2026-08-30, "bad ideas can be valid, but we don't
// want to support BIP forbidden wallets"), whose authority is BIP 388's
// pairwise-distinctness rule and its footnote on miniscript pubkey-reuse
// insecurity -- one key that signs for two seats signs two messages, and that
// is a key-recovery hazard, not merely a redundancy the label overstates. The
// Rust primary reaches the same place by the same route: `md address` refuses
// the shape while `md decode` reads the card and warns.
//
// WHY THIS IS NOT F-514 REVERSED. F-514 chose warn-over-refuse because
// "refusing on-device would strand a card that may already be engraved". The
// card is not stranded: it still decodes, still displays, still verifies, and
// still warns. What it no longer does is hand the operator an address to fund.
// That is the primary's disposition split exactly -- read surfaces warn and
// proceed, derive surfaces refuse.

// forkBuiltChunks loads a fork-native md1 fixture from md/testdata/forkbuilt/.
// Its generator, and the proof it has not drifted, are in
// md/dup_seat_fixture_test.go.
func forkBuiltChunks(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "forkbuilt", name+".md1.txt"))
	if err != nil {
		t.Fatalf("read %s.md1.txt: %v", name, err)
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.ReplaceAll(strings.TrimSpace(l), " ", ""); strings.HasPrefix(l, "md1") {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s carries no md1 strings", name)
	}
	return out
}

var dupSeatFixtures = []struct {
	name string
	// coreAddress is Bitcoin Core's answer for the FAITHFUL three-seat
	// descriptor, witness program identical to the bcrt1 row in the block
	// above. The emitter agrees with it; the flat route did not.
	coreAddress string
	// projectedAddress is what the flat route showed before F-531 -- Core's
	// answer for the two-seat descriptor the projection silently built.
	projectedAddress string
}{
	{
		name:             "dup_seat_wsh_sortedmulti_k1",
		coreAddress:      "bc1qvljqpugpaf8w3t898gw2c26yta833xz4gcuznjhuzl5g8j2dxezq6323ek",
		projectedAddress: "bc1q2gu6t4m7ax8vzl37th5jyjjwj9l3zy84h0yrwaredxy6fxdvl87s4d4suf",
	},
	{
		name:             "dup_seat_wsh_sortedmulti_k2",
		coreAddress:      "bc1qaej2r8zv5l00dndh7u6ch5w5pn8lrv70vayz3xu99j3y2emqdghq25tr6q",
		projectedAddress: "bc1qsl0stsxz6630afvzkamm9x9jj0mgsf3dcesnr2x967m62hnukwxq6nxajw",
	},
}

// TestFlatRouteRefusesARepeatedSeatMultisig is the funds-critical arm: the
// projection that produced the wrong address must not produce a descriptor at
// all.
//
// It asserts expandUnsupported rather than "the descriptor is right", because
// a *bip380.Descriptor cannot carry three seats over two slots and be built
// from a per-slot key list -- the refusal IS the faithful-or-refuse contract
// this file's projection has always claimed (D2).
func TestFlatRouteRefusesARepeatedSeatMultisig(t *testing.T) {
	for _, f := range dupSeatFixtures {
		t.Run(f.name, func(t *testing.T) {
			chunks := forkBuiltChunks(t, f.name)
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			desc, status := expandedToDescriptor(tpl, keys)
			if status != expandUnsupported || desc != nil {
				var got string
				if desc != nil {
					a, _ := address.Receive(desc, 0)
					got = "\nit derives " + a + "\nCore's answer for this policy is " + f.coreAddress
				}
				t.Fatalf("expandedToDescriptor returned status=%v for a %d-seat policy over "+
					"%d key slots, want expandUnsupported.%s\n"+
					"A per-slot key list cannot carry a repeated seat: keeping K while "+
					"dropping the repeat builds a DIFFERENT script, and the address it "+
					"hashes to is a different address (F-531)", status, tpl.M, tpl.N, got)
			}
		})
	}
}

// TestNoAddressSurfaceDerivesForARepeatedSeatPolicy walks every entry point
// that can put an address on the screen and asserts none of them does.
//
// BOTH ROUTES, because closing only the flat one would leave the emitter
// deriving the shape -- correctly, which is worse than it sounds: a correct
// address for a wallet the operator has been told not to use is still an
// invitation to fund it.
func TestNoAddressSurfaceDerivesForARepeatedSeatPolicy(t *testing.T) {
	for _, f := range dupSeatFixtures {
		t.Run(f.name, func(t *testing.T) {
			chunks := forkBuiltChunks(t, f.name)
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			if at, ok := policyAddressAt(chunks, tpl, keys); ok {
				a, _ := at(0, false)
				t.Errorf("policyAddressAt derives %s for a repeated-seat policy; want no "+
					"deriver at all", a)
			}
			if at, ok := complexAddressSource(chunks, keys); ok {
				a, _ := at(0, false)
				t.Errorf("complexAddressSource derives %s for a repeated-seat policy; want "+
					"no deriver at all", a)
			}
			lines := walletPolicyAddressLines(chunks, tpl, keys)
			joined := strings.Join(lines, "\n")
			for _, addr := range []string{f.coreAddress, f.projectedAddress} {
				if strings.Contains(joined, addr) {
					t.Errorf("the Engrave Wallet Policy address block shows %s for a "+
						"repeated-seat policy:\n%s", addr, joined)
				}
			}
			restore, hasAddr, err := multisigRestoreLines(tpl, keys)
			if err != nil {
				t.Fatalf("multisigRestoreLines: %v", err)
			}
			if hasAddr {
				t.Errorf("the restore doc claims addresses for a repeated-seat policy:\n%s",
					strings.Join(restore, "\n"))
			}
		})
	}
}

// TestRepeatedSeatRefusalStillWarns is the other half, and it is the half a
// refusal quietly breaks: the F-514 duplicate-key warning lives on the branch
// that HAS addresses, so refusing to derive would have dropped the operator on
// a silent screen.
//
// Silence is the failure mode F-514 exists to prevent. A screen that shows no
// address and says nothing is indistinguishable from a policy this device
// simply cannot render, and the operator would go looking for a better tool
// rather than learning that their wallet reuses a key.
func TestRepeatedSeatRefusalStillWarns(t *testing.T) {
	for _, f := range dupSeatFixtures {
		t.Run(f.name, func(t *testing.T) {
			chunks := forkBuiltChunks(t, f.name)
			slot, kind, err := md.DuplicateKeySlotChunks(chunks)
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			want := composerCopyDuplicateKeys(slot, kind)
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}

			t.Run("wallet policy address block", func(t *testing.T) {
				assertCarriesWarning(t, walletPolicyAddressLines(chunks, tpl, keys), want,
					"the Engrave Wallet Policy address block")
			})

			t.Run("composer consent", func(t *testing.T) {
				lines, err := composerConsentLinesFor(chunks, nil, 0)
				if err != nil {
					t.Fatalf("composerConsentLinesFor: %v", err)
				}
				assertCarriesWarning(t, lines, want, "the composer consent screen")
			})

			t.Run("inspect descriptor", func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					p := newPlatform()
					p.display = sh2DisplaySize
					ctx := NewContext(p)
					frame, quit := runUI(ctx, func() {
						gatheredDescriptorFlow(ctx, &descriptorTheme, chunks)
					})
					defer quit()
					// THE OPENING OF THE COPY, TAKEN FROM THE COPY. Three
					// tests in this cycle broke on wording edits because they
					// hardcoded the sentence; and the screen pages, so the tail
					// would prove the warning exists somewhere rather than that
					// it is seen.
					if got, ok := pumpUntil(frame, want[:40], 24); !ok {
						t.Errorf("the Inspect-descriptor screen refuses to derive an address "+
							"for a repeated-seat policy and never says why.\nLast frame: %q", got)
					}
				})
			})
		})
	}
}

// TestTheTwoAddressRoutesNeverDisagree is the durable half of this file: the
// fixtures above pin ONE shape, this pins the RULE.
//
// Wherever both routes can derive, they must derive the same address. They are
// two readings of one card -- the flat one projects the summarized template,
// the emitter walks the encoded tree -- and a card whose two readings differ is
// a card one of whose readings is wrong.
//
// WHAT IT WOULD AND WOULD NOT HAVE CAUGHT. Not F-531 as it stood: no VENDORED
// vector repeats a seat, so this loop would have been green on the day the flat
// projection was written. It is stated plainly because the tempting claim --
// "the invariant that would have caught it" -- is false, and a test whose
// comment oversells it is how the next lossy projection gets waved through.
// What it does catch is the NEXT one: any future shape the flat route
// summarizes lossily while the emitter reads it faithfully fails here rather
// than on a plate.
//
// The fork-built repeated-seat fixtures are in the loop too. Both routes refuse
// them today, so both skip -- and that is the assertion: re-admitting the shape
// on one route without the other puts this test back in play, and it will then
// demand they agree.
func TestTheTwoAddressRoutesNeverDisagree(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "md", "testdata", "vectors"))
	if err != nil {
		t.Fatalf("read vectors dir: %v", err)
	}
	type source struct {
		name string
		load func(*testing.T) []string
	}
	var sources []source
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".phrase.txt")
		if !ok {
			continue
		}
		sources = append(sources, source{name, func(t *testing.T) []string {
			return loadVectorChunks(t, name)
		}})
	}
	for _, f := range dupSeatFixtures {
		sources = append(sources, source{f.name, func(t *testing.T) []string {
			return forkBuiltChunks(t, f.name)
		}})
	}
	compared := 0
	for _, src := range sources {
		name := src.name
		t.Run(name, func(t *testing.T) {
			chunks := src.load(t)
			if len(chunks) == 0 {
				t.Skip("not an md1 phrase vector")
			}
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Skip("not a wallet policy")
			}
			desc, status := expandedToDescriptor(tpl, keys)
			if status != expandOK {
				t.Skip("flat route does not derive this shape")
			}
			emit, ok := complexAddressDeriver(chunks, keys)
			if !ok {
				t.Skip("emitter does not derive this shape")
			}
			flat, err := address.Receive(desc, 0)
			if err != nil {
				t.Fatalf("flat route: %v", err)
			}
			got, err := emit(0, false)
			if err != nil {
				t.Fatalf("emitter: %v", err)
			}
			compared++
			if flat != got {
				t.Errorf("the two address routes disagree for %s:\n"+
					"  flat (expandedToDescriptor -> address.Receive): %s\n"+
					"  emitter (the encoded tree):                     %s\n"+
					"One of them is reading a script the card does not encode. The emitter "+
					"walks the tree the plates reconstruct, so it is the one to trust "+
					"(F-531, measured against Bitcoin Core 25.0.0)", name, flat, got)
			}
		})
	}
	if compared == 0 {
		t.Error("no vector exercised BOTH routes, so this test compared nothing and " +
			"would pass with either route arbitrarily broken")
	}
}

// TestEmitterMatchesBitcoinCoreOnARepeatedSeatPolicy keeps the measurement that
// decided F-531 alive in the repo.
//
// The device refuses this shape now, so the refusal sits in front of the
// deriver -- which means nothing would otherwise record WHICH route had been
// right, and the next reader would have to boot Core again to find out. It
// calls the deriver beneath the gate for exactly that reason.
//
// If this row ever fails, the emitter has drifted from Core and the refusal
// above is the only thing standing between that drift and a plate.
func TestEmitterMatchesBitcoinCoreOnARepeatedSeatPolicy(t *testing.T) {
	for _, f := range dupSeatFixtures {
		t.Run(f.name, func(t *testing.T) {
			chunks := forkBuiltChunks(t, f.name)
			_, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			at, ok := complexAddressDeriver(chunks, keys)
			if !ok {
				t.Fatal("the emitter can no longer derive the repeated-seat shape, so the " +
					"Core measurement this test carries is no longer checked against anything")
			}
			got, err := at(0, false)
			if err != nil {
				t.Fatalf("emitter: %v", err)
			}
			if got != f.coreAddress {
				t.Errorf("the emitter derives %s; Bitcoin Core 25.0.0 derives %s for the "+
					"faithful three-seat descriptor", got, f.coreAddress)
			}
			if got == f.projectedAddress {
				t.Error("the emitter now agrees with the PROJECTED two-seat address, which " +
					"means it has acquired the F-531 defect itself")
			}
		})
	}
}

// ─── review I-2: the KEYLESS form of the same shape ──────────────────────────
//
// The warning was an ARM of noAddressLines, placed after the two keyless ones,
// so it never fired for a repeated-seat template carrying no keys. That card is
// not a curiosity: md.TemplateEngraveShapeGuardChunks admits it, the Engrave
// Wallet Policy consent is one confirm and one bundle review from bundleEngrave,
// and the screen said "Template has no keys - no addresses." and nothing else.
// The operator reads "P2WSH 1-of-3 multisig (sorted)", counts the two slots
// listed under it, and cuts steel.
//
// Reuse is a fact about the CARD and keylessness is a fact about what can be
// derived from it, so the sentence that names the reuse now LEADS both.
const dupSeatKeyless = "dup_seat_wsh_sortedmulti_k1_keyless"

func TestKeylessRepeatedSeatTemplateIsNotSilent(t *testing.T) {
	chunks := forkBuiltChunks(t, dupSeatKeyless)
	slot, kind, err := md.DuplicateKeySlotChunks(chunks)
	if err != nil {
		t.Fatalf("DuplicateKeySlotChunks: %v", err)
	}
	if kind == md.DuplicateNone {
		t.Fatal("the keyless fixture carries no duplicate, so this test asserts nothing")
	}
	want := composerCopyDuplicateKeys(slot, kind)

	// THE ENGRAVE CONSENT ITSELF, not the address block beneath it. This is the
	// screen the plate is cut from, and walletPolicyConsentLines is what it
	// renders; asserting one layer down would have passed while the screen an
	// operator actually reads stayed silent.
	lines, err := walletPolicyConsentLines(chunks, nil)
	if err != nil {
		t.Fatalf("walletPolicyConsentLines: %v", err)
	}
	assertCarriesWarning(t, lines, want, "the Engrave Wallet Policy consent")
	assertShowsNoAddress(t, lines, "the Engrave Wallet Policy consent")

	consent, err := composerConsentLinesFor(chunks, nil, 0)
	if err != nil {
		t.Fatalf("composerConsentLinesFor: %v", err)
	}
	assertCarriesWarning(t, consent, want, "the composer consent screen")

	// AND THE REASON FOR THE ABSENCE IS THE KEYLESSNESS, not the refusal. Both
	// sentences are true of this card; only one of them is why there is no
	// address, and naming the other would send the operator to fix the wrong
	// thing.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "no addresses") {
		t.Errorf("the consent never says there are no addresses:\n%s", joined)
	}
	if strings.Contains(joined, composerCopyNoAddressesDuplicateKeys()) {
		t.Errorf("the consent blames the refusal for a keyless template's missing "+
			"addresses. There was nothing to derive from; the reuse is a separate "+
			"fact and is stated separately:\n%s", joined)
	}

	t.Run("inspect descriptor", func(t *testing.T) {
		// expandTemplateOnly, the arm that had no modal at all: a keyless card
		// went straight to md1DisplayFlow. This is why the announcement was
		// hoisted above the routing rather than added to one more arm.
		synctest.Test(t, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			frame, quit := runUI(ctx, func() {
				gatheredDescriptorFlow(ctx, &descriptorTheme, chunks)
			})
			defer quit()
			if got, ok := pumpUntil(frame, want[:40], 24); !ok {
				t.Errorf("the Inspect-descriptor screen shows a keyless repeated-seat "+
					"template and never mentions the reuse.\nLast frame: %q", got)
			}
		})
	})
}

// TestTheRestoreDocNamesTheReuse is review M-3: the sentence added to the
// restore document had no test, and deleting it left 1312/1312 green.
//
// It matters more than most copy. §4.4 calls the restore document "the one that
// matters most", and it is what a reader holds in five years with no device
// behind it -- "Addresses unavailable for this policy shape" tells that reader
// the shape was exotic, when the truth is that their wallet reuses a key.
func TestTheRestoreDocNamesTheReuse(t *testing.T) {
	for _, name := range []string{"dup_seat_wsh_sortedmulti_k1", "dup_seat_wsh_sortedmulti_k2"} {
		t.Run(name, func(t *testing.T) {
			chunks := forkBuiltChunks(t, name)
			tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			lines, hasAddr, err := multisigRestoreLines(tpl, keys)
			if err != nil {
				t.Fatalf("multisigRestoreLines: %v", err)
			}
			if hasAddr {
				t.Fatal("the restore doc derives addresses for a repeated-seat policy")
			}
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "seats one key") {
				t.Errorf("the restore document does not name the reuse, so a reader is told "+
					"the SHAPE was unsupported when the fact is that their wallet reuses a "+
					"key:\n%s", joined)
			}
			if strings.Contains(joined, "Addresses unavailable for this policy shape.") {
				t.Errorf("the restore document fell back to the generic sentence:\n%s", joined)
			}
		})
	}
}
