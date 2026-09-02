package gui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S4: the slot-assignment model, and the seed<->key gate ──────────────────
//
// SPEC 4.3 rules the model here rather than leaving it to a stage, and the
// reason is recorded in the spec itself: round 0 said the gate "fires on
// assignment" while no stage built a step in which a slot could hold BOTH a seed
// and a key, so a literal implementation would have shipped a gate that never
// fires -- the operator's requirement silently unmet, with tests still green
// against synthetic assignments. The model is what makes the gate reachable.
//
// Every slot @0..@{n-1} carries exactly one SOURCE:
//
//	payloadKey(record)            a cosigner mk1 from the payload
//	derived(seedID, account)      derived from a seed supplied in this flow
//	both(seedID, account, record) the operator asserts the payload key at this
//	                              slot IS theirs, derived from that seed
//
// `both` is the only shape that triggers the gate, and it exists because it is
// the case the operator asked about: a payload carrying their seed AND their own
// key card.
//
// WHAT THE FLOW ACTUALLY REACHES, said plainly rather than implied.
//
// S5 widened the MODEL, the TAIL and the @S PICKER to multi-slot self:
// buildSlotSources accepts a SET of held slots, numbers their BIP-48 accounts
// off the master fingerprint, and buildEngraveTail derives one leg per held slot
// at that slot's own origin.
//
// THE PICKER IS A MULTI-SELECT SINCE 4b10319, IN THIS SAME BRANCH. This paragraph
// used to end "What has NOT moved is the @S PICKER, which is still single-select,
// so a build driven through the screens produces exactly one held slot and the
// multi-slot shapes are exercised by tests driving the model directly." Every
// clause of that was false by the time it was read, and it cost two findings.
// It licensed hand-built multi-slot fixtures as the only way to reach these
// shapes -- which is how C-2's only guard came to be a registry the flow cannot
// construct -- and its sibling sentence in gui/multisig_build.go was the premise
// that made I-6's `open == 0` look unreachable. A reachability claim in a comment
// is a fact a reviewer inherits; this one is now stated as what the screens do.
//
// S5 also removed S2's interim foreign-origin refusal, which is why S5 test 8
// (TestGateStillFiresAfterOriginsDiverge) re-proves this gate through the REAL
// flow: during S4 a divergent-origin input could not reach the gate at all, so
// every S4 gate test below is necessarily synthetic, and without that re-proof
// S4's proof would have expired silently.

// slotSourceKind is SPEC 4.3's source table as a closed enum.
type slotSourceKind int

const (
	// slotFromCard is `payloadKey(record)`: the key came off a cosigner card and
	// no seed in this flow claims it. NOTHING is derived against it. That is the
	// false-positive SPEC 4.3 forbids by name: "a payload carrying the operator's
	// seed alongside other people's cosigner cards is normal and MUST NOT fail."
	slotFromCard slotSourceKind = iota
	// slotFromSeed is `derived(seedID, account)`.
	slotFromSeed
	// slotFromBoth is `both(seedID, account, record)`, and the only kind the gate
	// fires on.
	slotFromBoth
)

// slotSource is one slot's assignment. Index in the enclosing slice IS the slot
// number, so a source cannot disagree with its own @N.
//
// THE TWO BINDINGS ARE SPEC M-B's, ruled there so an implementer cannot pick
// differently:
//
//   - In a `both` slot the CARD's declared origin and key are AUTHORITATIVE. The
//     gate derives at that origin. `Account` is display/bookkeeping only and is
//     an input to nothing -- deriving at m/48'/0'/Account'/2' instead would FAIL
//     LOUDLY on a genuinely-theirs card whose account the operator misremembered.
//   - In a `derived` slot, `Account` occupies the BIP-48 ACCOUNT component:
//     m/48'/0'/Account'/2'.
type slotSource struct {
	Kind slotSourceKind
	// SeedID indexes the flow's seed registry. Meaningless for slotFromCard.
	SeedID int
	// Account is the BIP-48 account component for slotFromSeed, and bookkeeping
	// only for slotFromBoth (see the M-B binding above).
	Account uint32
	// Card indexes the flow's chosen cosigner-card slice. Meaningless for
	// slotFromSeed.
	Card int
}

// derivedSlotOrigin is M-B's second binding as one function: a `derived` slot's
// account number IS the BIP-48 account component. It exists so the account ->
// path mapping has exactly one site, and so a test can name it.
//
// FROM S5 IT IS ALSO TEMPLATE-AWARE (plan §0.1a). BIP-48 assigns the SCRIPT TYPE
// component: 2' to native segwit and 1' to nested segwit, and NOTHING to legacy
// P2SH. Before S5 this returned 2' unconditionally, so an sh(wsh) build stamped
// the native-segwit path onto steel and a BIP-48-aware coordinator reading that
// plate derived at the wrong script-type path. §0.1a rules that S5 is where the
// default becomes template-aware -- not earlier, because making it so at S2 or S3
// would have stranded S3's walk on S2's interim foreign-origin refusal.
func derivedSlotOrigin(script md.MultisigScript, account uint32) bip32.Path {
	const h = hdkeychain.HardenedKeyStart
	return bip32.Path{48 | h, 0 | h, account | h, multisigScriptTypeComponent(script) | h}
}

// multisigScriptTypeComponent is BIP-48's script-type component for a template,
// and the ONE site that decides it.
//
// wsh -> 2', the BIP's own recommended default for native segwit.
// sh(wsh) -> 1', the BIP's assignment for nested segwit (S5; plan §0.1a).
// sh -> 2', because NO BIP assigns legacy P2SH multisig a path at all. That one
// is this device's convention rather than an authority's, which is exactly why
// §0.1a requires it to be ANNOUNCED loudly rather than merely applied
// (buildOriginAnnouncement).
//
// The wallet-policy composer's md.ComposeWrapper.ScriptType() is the same table
// extended by tr = 3'; gui/composer_origin_test.go keeps the two in step.
func multisigScriptTypeComponent(script md.MultisigScript) uint32 {
	if script == md.MultisigShWsh {
		return 1
	}
	return 2
}

// ─── The seed registry ───────────────────────────────────────────────────────

// registeredSeed is one (seed, passphrase) pair supplied in this flow. SPEC 4.1
// makes the pair the derivation unit everywhere the spec says "seed": the
// passphrase prompt is PER SEED, asked at that seed's entry, because one
// flow-global passphrase applied to N seeds would mint keys the operator can only
// re-derive with a pairing they never chose -- and for a new-seed slot there is
// no card to cross-check, so no row of 4.3 could catch it.
type registeredSeed struct {
	// Label is what the screens call this seed. Operator-facing, never a seedID.
	Label      string
	Mnemonic   bip39.Mnemonic
	Passphrase string
	// MasterFP is the master-key fingerprint of THIS (seed, passphrase) pair,
	// captured once at registration so the gate's fingerprint row costs no extra
	// PBKDF2 per slot. It is a fingerprint of the pair, not of the words: a
	// passphrase changes it, which is exactly why the row can catch a
	// contradiction at all.
	MasterFP uint32
}

// seedRegistry holds every seed supplied in this flow, keyed by the seedID a
// slotSource carries.
//
// SPEC 4.2 governs the flow's WORKING COPIES -- these bip39.Mnemonic buffers and
// the derivation intermediates -- and not the systemwide session's stored
// record, whose lifetime SYSW 3.2.1 rules separately and deliberately does not
// scrub.
//
// ONE SCRUB SITE, DEFERRED AT CREATION, IS THE WHOLE DESIGN. The registry is
// made at the top of the build flow and `defer reg.scrub()` is installed there,
// before any seed can be added, so every exit -- a picker Back, a slot-review
// Back, a gate FAIL screen, the tail abort, a ctx.Done unwind, a panic -- is
// covered by construction rather than by an implementer remembering to add a
// scrub to a new return. The plan's warning is exact: "A registry missing one
// exit leaves N masters' seeds in RAM and nothing else would notice." The way
// not to miss one is to have only one.
//
// enter() is why that holds: it registers the seed BEFORE returning its id, so
// there is no window in which a live seed is owned by nobody.
type seedRegistry struct {
	seeds []registeredSeed
}

// add registers a (seed, passphrase) pair and returns its seedID. It derives the
// pair's master fingerprint on the way in; a seed whose master key cannot be
// built at all is refused rather than registered half-formed.
func (r *seedRegistry) add(label string, m bip39.Mnemonic, passphrase string, net *chaincfg.Params) (int, error) {
	_, fp, err := deriveAccountXpub(m, passphrase, net, nil)
	if err != nil {
		return 0, err
	}
	r.seeds = append(r.seeds, registeredSeed{
		Label: label, Mnemonic: m, Passphrase: passphrase, MasterFP: fp,
	})
	return len(r.seeds) - 1, nil
}

// bindPassphrase attaches a passphrase to an already-registered seed and
// re-captures the pair's master fingerprint.
//
// It exists so the seed can be registered the instant it is entered -- before
// the passphrase screens, which have their own Back -- without the registry ever
// holding a pair whose fingerprint belongs to a different pairing. SPEC 4.1
// makes (seed, passphrase) the derivation unit, so a stale MasterFP here would
// feed the gate's fingerprint row a fingerprint from the wrong wallet.
func (r *seedRegistry) bindPassphrase(id int, passphrase string, net *chaincfg.Params) error {
	if id < 0 || id >= len(r.seeds) {
		return fmt.Errorf("multisig build: no seed %d to bind a passphrase to", id)
	}
	_, fp, err := deriveAccountXpub(r.seeds[id].Mnemonic, passphrase, net, nil)
	if err != nil {
		return err
	}
	r.seeds[id].Passphrase = passphrase
	r.seeds[id].MasterFP = fp
	return nil
}

// at returns the registered seed for a seedID.
func (r *seedRegistry) at(id int) (registeredSeed, bool) {
	if id < 0 || id >= len(r.seeds) {
		return registeredSeed{}, false
	}
	return r.seeds[id], true
}

// count reports how many seeds are registered.
func (r *seedRegistry) count() int { return len(r.seeds) }

// usesPassphrase reports whether ANY registered pair carries a passphrase.
//
// It is asked because the backup has to say what is NOT in it. A BIP-39
// passphrase is a REQUIRED SPENDING FACTOR and is NEVER ENGRAVED -- ms1 encodes
// the WORDS, the passphrase is not in that entropy, and no plate this device cuts
// can be made to yield it. A set labelled "Full (seed + keys)" that silently
// omits a spending factor is F-132's shape: a backup that is both wrong and
// trusted.
//
// ANY, not ALL, and SPEC 4.1 is why: the passphrase prompt is PER SEED, so a
// three-master build can have one passphrased leg and two bare ones. One leg the
// plates cannot reconstruct is enough to make the whole set incomplete.
//
// An EXPLICITLY BOUND EMPTY passphrase is no passphrase. syswPassphraseFlowTitled
// can return ("", true), and a build that engraves a plain seed must not be
// labelled as though a factor were missing.
func (r *seedRegistry) usesPassphrase() bool {
	for _, s := range r.seeds {
		if s.Passphrase != "" {
			return true
		}
	}
	return false
}

// passphraseFacts is the registry's per-seed answer, and it is what the restore
// document takes instead of usesPassphrase()'s single bit.
//
// SPEC 4.1 makes the (seed, passphrase) pair the derivation unit and asks the
// passphrase PER SEED, so an any() collapses N per-seed facts into one bool and
// a second required passphrase is deduped out of existence on the one artifact
// that outlives the operator. usesPassphrase() SURVIVES for the engrave-mode
// LABEL, which is a different question with a genuinely boolean answer: is this
// set short of a spending factor, yes or no, on a row that does not wrap.
//
// IT CARRIES NO SECRET. Only the label, the pair's master fingerprint and a
// bool cross into the display path; the mnemonic and the passphrase text stay in
// the registry, which is the one thing that scrubs them.
//
// ONE FACT PER SECRET, NOT PER REGISTRY ENTRY, and that is B1. The registry holds
// one entry per HELD SLOT -- buildMultisigPolicyFlow calls buildSeedForSlot once
// per held slot and buildSeedForSlot calls reg.add unconditionally -- so an
// operator holding @0 and @1 from one master, typing the same passphrase at both
// prompts, produced TWO facts for ONE secret. The document then printed the same
// fingerprint on two lines, each ending "if more than one is listed here they may
// be DIFFERENT passphrases", three steps after the Key-sources gate had correctly
// said "Slots @0 and @1 all come from your seed". A reader who cannot find the
// second passphrase has to decide whether a fully recoverable backup is
// unrecoverable.
//
// THE KEY IS (Mnemonic, Passphrase) -- THE DERIVATION UNIT -- AND DELIBERATELY
// NOT MasterFP. The difference is which way the key FAILS. This dedupe SUPPRESSES
// a sentence, so a false merge drops a required passphrase off the one artifact
// that outlives the operator, and a 4-byte fingerprint collision between two
// unrelated seeds would do exactly that. buildSlotGate keys on MasterFP because
// there a collision only ADDS a spurious notice. Same class of identity rule,
// opposite failure direction, so the two sites key differently ON PURPOSE. The
// registry holds both halves of the pair, so this key is exact rather than a
// surrogate for it.
//
// THE LABELS ARE JOINED RATHER THAN DROPPED. The merged fact still names every
// held slot the secret covers, because losing one would hide a SLOT -- the same
// failure direction the merge exists to avoid -- and the reader needs to know
// which plates the one passphrase reaches.
func (r *seedRegistry) passphraseFacts() []seedPassphraseFact {
	type group struct {
		mnemonic   bip39.Mnemonic
		passphrase string
		masterFP   uint32
		labels     []string
	}
	groups := make([]group, 0, len(r.seeds))
	for _, s := range r.seeds {
		merged := false
		for i := range groups {
			g := &groups[i]
			if g.passphrase == s.Passphrase && slices.Equal(g.mnemonic, s.Mnemonic) {
				g.labels = append(g.labels, s.Label)
				merged = true
				break
			}
		}
		if !merged {
			groups = append(groups, group{
				mnemonic: s.Mnemonic, passphrase: s.Passphrase, masterFP: s.MasterFP,
				labels: []string{s.Label},
			})
		}
	}
	out := make([]seedPassphraseFact, 0, len(groups))
	for _, g := range groups {
		out = append(out, seedPassphraseFact{
			Label: joinAnd(g.labels), MasterFP: g.masterFP, Uses: g.passphrase != "",
		})
	}
	return out
}

// scrub zeroes every registered mnemonic. THE ONE SCRUB SITE (SPEC 4.2).
//
// The Passphrase field is dropped rather than overwritten, and the difference is
// stated rather than papered over: a Go string's backing bytes are immutable, so
// nothing can zero them in place and the only honest thing to do is release the
// reference and let the collector reclaim it. That is exactly the property the
// shipped flow has had all along -- this changes nothing and hides nothing.
func (r *seedRegistry) scrub() {
	for i := range r.seeds {
		for j := range r.seeds[i].Mnemonic {
			r.seeds[i].Mnemonic[j] = 0
		}
		r.seeds[i].Passphrase = ""
		r.seeds[i].MasterFP = 0
	}
}

// ─── The gate ────────────────────────────────────────────────────────────────

// errBuildSeedKeyMismatch is SPEC 4.3's second row: a `both` slot whose payload
// key does not derive from the seed the operator bound to it. It names the SLOT,
// because "the key and the seed disagree" with no slot number is a message the
// operator cannot act on when n is 5.
//
// Declared is the CARD's own spelling of its origin, quoted back rather than
// paraphrased -- and deliberately not the path the gate derived at, so that a
// gate deriving at the wrong origin cannot launder the mistake into its own
// error message.
type errBuildSeedKeyMismatch struct {
	Slot     int
	Declared string
}

func (e errBuildSeedKeyMismatch) Error() string {
	return fmt.Sprintf("multisig build: slot @%d's card key does not derive from its seed at %s",
		e.Slot, e.Declared)
}

// errBuildFingerprintContradicts is SPEC 4.3's third row: a fingerprint IS
// present and contradicts the derivation.
//
// It is a SEPARATE row from the mismatch above and not a stronger form of it,
// because mk1's Fingerprint is card-self-declared and unbound to the key
// (mk/mk.go): a card can carry the right key and the wrong fingerprint. That
// combination passes the derivation row and is still a disagreement the operator
// wrote down somewhere, so it is said out loud.
type errBuildFingerprintContradicts struct {
	Slot     int
	Declared string // the card's own 8-hex fingerprint
	Derived  string // the fingerprint of the (seed, passphrase) pair bound here
}

func (e errBuildFingerprintContradicts) Error() string {
	return fmt.Sprintf("multisig build: slot @%d's card declares fingerprint %s, but its seed derives %s",
		e.Slot, e.Declared, e.Derived)
}

// errBuildSlotAssignment is a structurally impossible assignment reaching the
// gate -- a seedID or card index the flow never created. It is not an operator
// error and has no operator remedy, so it is reported as a device failure. Kept
// because the gate is a pure function over a struct and a future caller can
// build one wrongly; falling through would mean skipping a check.
type errBuildSlotAssignment struct{ Slot int }

func (e errBuildSlotAssignment) Error() string {
	return fmt.Sprintf("multisig build: slot @%d has an unresolvable source", e.Slot)
}

// errBuildUnreadableCard is a `both` slot whose card will not decode into a key
// and an origin. buildCosignerCards already decoded it once, so this is a device
// failure rather than an operator one.
type errBuildUnreadableCard struct{ Slot int }

func (e errBuildUnreadableCard) Error() string {
	return fmt.Sprintf("multisig build: slot @%d's card key could not be read", e.Slot)
}

// bothSlotKey turns a `both` slot's card into the ONE-element []md.ExpandedKey
// findUserSlot compares against.
//
// THIS IS THE ORIGIN BINDING, and it is one line. M-B makes the card's DECLARED
// origin authoritative, so OriginPath comes from card.Path and from nothing else
// -- not from multisigSharedOrigin(), and not from the tuple's Account. A gate
// wrapper that hardcoded the shared origin would pass every other S4 test,
// because S2's interim foreign-origin refusal makes the two values
// indistinguishable for the whole of S4; TestGateDerivesAtTheCardsOwnOrigin is
// the mutation that separates them, on a card declaring m/48'/0'/1'/2'.
func bothSlotKey(card mk.Card) (md.ExpandedKey, error) {
	cc, pk, _, err := decodeXpubBytes(card.Xpub)
	if err != nil {
		return md.ExpandedKey{}, err
	}
	origin, err := bip32.ParsePath(card.Path)
	if err != nil {
		return md.ExpandedKey{}, err
	}
	var x [65]byte
	copy(x[0:32], cc[:])
	copy(x[32:65], pk[:])
	return md.ExpandedKey{OriginPath: origin, Xpub: x, XpubPresent: true}, nil
}

// buildSlotGate is SPEC 4.3's consistency gate, run at construction time over
// the ASSIGNED slot sources and before assembly.
//
// MECHANISM: DERIVATION, NEVER FINGERPRINTS (4.3). It derives from the seed plus
// its own passphrase at the KEY's declared origin and compares chain code ‖
// pubkey -- which is findUserSlot's existing comparison, REUSED rather than
// reimplemented (gui/multisig_match.go). Fingerprints cannot be the mechanism:
// a serialized xpub carries a PARENT fingerprint, the constellation drops even
// that at encode (md/expand.go: ExpandedKey.Xpub is 32B chain code ‖ 33B pubkey),
// and this flow omits the master fingerprint by default, so a fingerprint-keyed
// gate would be silent on most real payloads.
//
// It returns NOTICES, not warnings-that-block: the one legitimate multi-slot
// shape (one seed at >=2 slots under DISTINCT origins) proceeds and says so.
// Round 0 of the spec refused it; that refusal is what SPEC 4.1 and this plan's
// TestGateAcceptsSameSeedAtDistinctOrigins exist to keep out.
//
// WHAT IT DOES NOT DO, on purpose: it never derives against material no slot
// claims. A payload holding the operator's seed and two unrelated cosigner cards
// is NORMAL, and the slotFromCard arm below is a `continue` for exactly that
// reason. Deriving across unassigned material is what R-2 forbids and what
// TestGateIgnoresUnassignedCosigners pins.
//
// The duplicate-key row of 4.3's table is NOT here. It lives in
// assembleBuildPolicy, over the FINAL SLOT SET, where S2 landed it permanently
// -- source-independent and therefore subsuming every arrival shape. S4 does not
// replace that check; it feeds it more sources.
// `script` reaches the gate because derivedSlotOrigin is template-aware from S5
// (§0.1a) and the gate's distinctness key is a derived slot's ORIGIN. Passing it
// keeps derivedSlotOrigin the single account -> path site rather than growing a
// second, script-blind copy of the mapping inside the gate.
func buildSlotGate(sources []slotSource, script md.MultisigScript, reg *seedRegistry, cards []mk.Card, net *chaincfg.Params) ([]string, error) {
	type binding struct {
		slot   int
		origin string
		label  string
	}
	// THE GROUPING KEY IS THE MASTER FINGERPRINT, NOT THE SeedID, and the
	// difference is a Critical this gate shipped once.
	//
	// buildMultisigPolicyFlow calls buildSeedForSlot once PER HELD SLOT
	// (gui/multisig_build.go:196-202) and buildSeedForSlot calls reg.add()
	// unconditionally (:554), so two slots the operator fills from the SAME words
	// carry two DIFFERENT SeedIDs. A SeedID-keyed grouping therefore put every
	// binding in a group of one, `len(bs) < 2` skipped all of them, and SPEC 4.3
	// row 5's notice -- the ONLY surface that tells an operator two of their slots
	// share one secret -- could never fire for the shape the product actually
	// builds. Trace B is that shape. The harm is not cosmetic: the tail correctly
	// cuts ONE ms1 for the shared master, so losing that plate loses BOTH slots,
	// and an operator who read two independent-looking key sources believes one of
	// two keys survives.
	//
	// It is the SAME identity mistake the sibling function buildEngraveTail already
	// shipped and fixed (gui/multisig_build_tail.go:62-84 records the diagnosis);
	// the fix was applied there and never swept into the gate.
	//
	// MasterFP is a fingerprint of the (seed, passphrase) PAIR, which is the right
	// identity here for SPEC 4.1's reason: the same words under two passphrases are
	// two different masters holding two different keys, and grouping them would
	// announce a shared secret that does not exist. It also fails SAFE in the only
	// direction that matters -- a 4-byte collision emits a SPURIOUS notice about
	// two unrelated seeds, never suppresses a true one.
	bound := map[uint32][]binding{}
	var order []uint32
	for slot, s := range sources {
		switch s.Kind {
		case slotFromCard:
			// NORMAL, and checked by nothing. See the comment above.
			continue
		case slotFromSeed:
			// RESOLVED, not merely existence-checked: the grouping reads MasterFP.
			seed, ok := reg.at(s.SeedID)
			if !ok {
				return nil, errBuildSlotAssignment{Slot: slot}
			}
			if _, seen := bound[seed.MasterFP]; !seen {
				order = append(order, seed.MasterFP)
			}
			bound[seed.MasterFP] = append(bound[seed.MasterFP],
				binding{
					slot:   slot,
					origin: derivedSlotOrigin(script, s.Account).String(),
					label:  seed.Label,
				})
		case slotFromBoth:
			seed, ok := reg.at(s.SeedID)
			if !ok {
				return nil, errBuildSlotAssignment{Slot: slot}
			}
			if s.Card < 0 || s.Card >= len(cards) {
				return nil, errBuildSlotAssignment{Slot: slot}
			}
			card := cards[s.Card]
			k, err := bothSlotKey(card)
			if err != nil {
				return nil, errBuildUnreadableCard{Slot: slot}
			}
			// THE COMPARISON, through the shipped one. findUserSlot derives at
			// each key's own OriginPath and matches on (chainCode, pubkey) -- never
			// base58, never == on mismatched array/slice types.
			if _, _, matched := findUserSlot(seed.Mnemonic, seed.Passphrase, net,
				[]md.ExpandedKey{k}); !matched {
				return nil, errBuildSeedKeyMismatch{Slot: slot, Declared: card.Path}
			}
			// 4.3 row 3. Checked AFTER the derivation on purpose: a card that is
			// wrong about the KEY is wrong about the funds, and naming the graver
			// disagreement first is what the operator needs. A card that is right
			// about the key and wrong about the fingerprint reaches here.
			if card.Fingerprint != "" {
				derived := fmt.Sprintf("%08x", seed.MasterFP)
				if !strings.EqualFold(strings.TrimSpace(card.Fingerprint), derived) {
					return nil, errBuildFingerprintContradicts{
						Slot: slot, Declared: card.Fingerprint, Derived: derived,
					}
				}
			}
			if _, seen := bound[seed.MasterFP]; !seen {
				order = append(order, seed.MasterFP)
			}
			// The binding key is the PARSED origin, not the card's spelling:
			// m/48'/0'/1'/2' and m/48h/0h/1h/2h are the same path, and a
			// distinctness test over the two strings would call them different
			// origins and emit a multi-account notice for one key twice.
			bound[seed.MasterFP] = append(bound[seed.MasterFP],
				binding{slot: slot, origin: k.OriginPath.String(), label: seed.Label})
		}
	}

	// 4.3 row 5: one seed at >=2 slots under DISTINCT origins is the legitimate
	// multi-account wallet. PROCEED, with the notice findUserSlot already
	// specifies for the same shape.
	//
	// Two slots at the SAME origin under one seed are the same key twice, and
	// that is the duplicate-key refusal's business, over the final slot set. It
	// is deliberately not re-decided here: two places deciding one thing is how
	// "the payload carries n-1" grew back last time.
	var notices []string
	for _, fp := range order {
		bs := bound[fp]
		if len(bs) < 2 {
			continue
		}
		seen := map[string]bool{}
		distinct := true
		for _, b := range bs {
			if seen[b.origin] {
				distinct = false
				break
			}
			seen[b.origin] = true
		}
		if !distinct {
			continue
		}
		slots := make([]string, len(bs))
		for i, b := range bs {
			slots[i] = fmt.Sprintf("@%d", b.slot)
		}
		// THE LABEL TRAVELS WITH THE BINDING rather than being looked up again.
		// The group is now keyed on a fingerprint, so there is no seedID to look
		// up -- and several registry entries can share one fingerprint, which is
		// precisely the shape this notice exists for. Every entry in a group is the
		// same (seed, passphrase) pair by construction, so the first one's label is
		// the group's; taking it from the binding keeps the sentence and the
		// grouping reading the same datum.
		notices = append(notices, fmt.Sprintf(
			"Slots %s all come from %s, at different key origins. That is a "+
				"multi-account wallet and is allowed.", joinAnd(slots), bs[0].label))
	}
	return notices, nil
}

// ─── The operator-facing surfaces ────────────────────────────────────────────

// buildSelfSourceFlow asks the ONE slot-source question this stage offers: is
// the operator's own key also on a payload card?
//
// IT DOES NOT SPEAK SPEC LANGUAGE, and that is a requirement rather than a
// style note. `payloadKey`, `derived(seedID, account)` and `both` name a data
// model, not a choice; on screen the operator either approves a model they have
// not understood or is alarmed by a distinction that does not concern them. So
// the screen asks whose key this slot is and where it came from, in those words.
//
// Returns ok=false on Back, which abandons the build exactly as Back at the
// gather does.
//
// IT TAKES THE WHOLE HELD SET FROM S5, AND IT STILL ASKS ONCE. That is a
// deliberate, announced assumption rather than an oversight, and §0.1's ladder
// is what licenses it:
//
//   - clause 1 (authority) has nothing to say: no standard rules whether a
//     held slot's key also sits on a card. What replaces an authority here is
//     the operator's own answer, and they give exactly one.
//   - clause 2 (auditability, the binding one) PASSES, and it is the reason
//     this is allowed at all. The resulting per-slot assignment is printed,
//     slot by slot, on buildSlotSourceLines' "Key sources" review BEFORE
//     anything is derived or assembled -- "@1 yours: derived from your seed
//     for @1" against "@1 yours: payload card 2, checked against ...". A wrong
//     answer is therefore readable, not invisible. It is readable in the kept
//     artifacts too: a `derived` slot's key and origin land in that slot's mk1
//     and on the restore doc.
//   - clause 3 (reversibility) PASSES because the announcement is the QUESTION
//     ITSELF. That is what the plural lead below is for: a screen reading "Is
//     your @0 key on a card?" that silently also answers for @1 and @2 announces
//     the assumption nowhere, and an operator cannot audit a question they were
//     not asked.
//
// WHAT IT COSTS, said plainly: a genuinely MIXED build -- @0 on a card, @1
// derived -- is not expressible through the screens. It is expressible in the
// MODEL (slotSource is per-slot and assembleBuildPolicy reads the mixture off
// the held-key set), so this is a picker limit, not a model limit. Answering YES
// when only some held slots are carded fails LOUDLY at the gate, naming the slot
// whose card does not derive from its seed; answering NO derives every held slot
// and says so on the review. Neither is silent. Making it per-slot means turning
// buildPolicyParams.SelfFromCard into a per-slot set, which is a change to the
// model this block does not own.
//
// FILED AS F-196 in design/FOLLOWUPS.md, owning phase: the spec (it is a model
// change and earns its own R0). The ID is here so the claim is a GREP rather
// than a promise: this comment previously said the limit was "filed rather than
// smuggled in" and it was not -- the implementer's report drafted the entry and
// the controller landed its sibling instead, so a reviewer inherited a filing
// that did not exist. An assertion of that shape has to name something a reader
// can look up.
func buildSelfSourceFlow(ctx *Context, th *Colors, held []int) (slotSourceKind, bool) {
	title := "Your key"
	choices := []string{"NO, JUST MY SEED", "YES, CHECK THE CARD"}
	if len(held) > 1 {
		title = "Your keys"
		choices = []string{"NO, JUST MY SEEDS", "YES, CHECK THE CARDS"}
	}
	cs := &ChoiceScreen{
		Title:   title,
		Lead:    buildSelfSourceLead(held),
		Choices: choices,
	}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return slotFromSeed, false
	}
	if sel == 1 {
		return slotFromBoth, true
	}
	return slotFromSeed, true
}

// buildSelfSourceLead is the question, and it NAMES EVERY SLOT THE ANSWER WILL
// BE APPLIED TO.
//
// The one-slot wording is byte-identical to the shipped screen on purpose:
// "key on a card?" is a pinned walk needle with exactly one production site
// (cmd/emu/needle_test.go's buildFlowNeedles), and three walk drivers anchor on
// it. The plural form deliberately does NOT contain that substring, so it adds
// no second production site for the needle -- it is a different question and it
// is anchored, if ever, as one.
//
// NO EM-DASH: it is a zero-pixel glyph in the body face and a line carrying one
// does not draw at all (F-78/F-151).
func buildSelfSourceLead(held []int) string {
	if len(held) == 1 {
		return fmt.Sprintf("Is your @%d key on a card?", held[0])
	}
	labels := make([]string, len(held))
	for i, s := range held {
		labels[i] = fmt.Sprintf("@%d", s)
	}
	return fmt.Sprintf("Are your %s keys on cards?", joinAnd(labels))
}

// buildSlotSourceLines is the review SPEC 4.3 requires before assembly: every
// slot, its source, in the operator's vocabulary.
//
// EVERY COMPARISON THIS SCREEN ASKS FOR MUST BE ONE THE OPERATOR CAN PERFORM, so
// it asks for none. "Verify X" is a claim that X is obtainable, and on this
// device the operator has no artifact to hold a slot's key against -- the
// coordinator comparison is S5's surface, once the per-slot keys are shown. What
// this screen does instead is state what the device checked and what it took on
// trust, which is a claim about the device and needs nothing from the operator.
//
// S5: the account is named on a derived slot only when it is NOT account 0.
// Before S5 the test was "is this the operator's one self slot", which cannot be
// asked of a SET of held slots; the account is what actually distinguishes two
// slots held from one master, and account 0 is the ordinary single-slot case
// whose line reads exactly as it always did.
func buildSlotSourceLines(sources []slotSource, origins []cosignerOrigin, reg *seedRegistry, notices []string) []string {
	lines := []string{"Where each key comes from:"}
	card := func(slot int) string {
		for _, o := range origins {
			if o.slot == slot {
				return fmt.Sprintf("payload card %d", o.card)
			}
		}
		return "a payload card"
	}
	checked := 0
	for slot, s := range sources {
		switch s.Kind {
		case slotFromBoth:
			seed, ok := reg.at(s.SeedID)
			label := "your seed"
			if ok {
				label = seed.Label
			}
			checked++
			lines = append(lines, fmt.Sprintf("@%d  yours: %s, checked against %s",
				slot, card(slot), label))
		case slotFromSeed:
			seed, ok := reg.at(s.SeedID)
			label := "your seed"
			if ok {
				label = seed.Label
			}
			if s.Account == 0 {
				lines = append(lines, fmt.Sprintf("@%d  yours: derived from %s", slot, label))
			} else {
				lines = append(lines, fmt.Sprintf("@%d  yours: derived from %s, account %d",
					slot, label, s.Account))
			}
		default:
			lines = append(lines, fmt.Sprintf("@%d  a cosigner: %s, taken as supplied",
				slot, card(slot)))
		}
	}
	lines = append(lines, notices...)
	if checked > 0 {
		lines = append(lines, fmt.Sprintf("%s checked against a seed on this device. "+
			"The rest are taken as supplied - this device has nothing to check them against.",
			plateWord(checked, "slot is", "slots are")))
	} else {
		lines = append(lines, "No slot claims to be both a seed and a card here, so "+
			"nothing was cross-checked. The cosigner keys are taken as supplied.")
	}
	return lines
}

// buildSlotSourceReviewFlow shows that review. Back returns false and abandons
// the build BEFORE anything is derived or assembled.
func buildSlotSourceReviewFlow(ctx *Context, th *Colors, sources []slotSource, origins []cosignerOrigin, reg *seedRegistry, notices []string) bool {
	return confirmReviewScreen(ctx, th, "Key sources",
		buildSlotSourceLines(sources, origins, reg, notices))
}

// buildSeedKeyMismatchMessage is SPEC 4.3's "FAIL LOUDLY, name the slot".
//
// A SAFETY GATE WHOSE OBVIOUS NEXT ACTION DISABLES IT IS NOT A GATE. After a
// mismatch the only route the operator can see from here is to set the slot back
// to their seed alone -- which stops the check RUNNING rather than resolving the
// disagreement, and leaves them with a wallet whose card they still believe is
// theirs. So the screen names the likely causes, says in those words that
// reassigning suppresses the check rather than fixing it, and names the host
// route.
//
// NO EM-DASH anywhere in this text. It is a zero-pixel glyph in the body face
// (F-78/F-151), and a line containing one does not draw AT ALL -- on the one
// screen that stands between a wrong key and steel.
func buildSeedKeyMismatchMessage(e errBuildSeedKeyMismatch, origins []cosignerOrigin) string {
	who := ""
	for _, o := range origins {
		if o.slot == e.Slot {
			who = fmt.Sprintf(" (payload card %d)", o.card)
			break
		}
	}
	// THE FIRST CAUSE NAMED IS THE ONE THE NEGATIVE CONTROL PRODUCES (review M2).
	// S4's walk reaches this screen purely by choosing the wrong payload card for
	// the self slot, and the earlier text named neither that cause nor its remedy
	// -- it sent the operator to rewrite a payload that was never wrong. A refusal
	// that misdirects the fix is only marginally better than one that misdirects
	// the blame, so the cause the flow can actually produce leads.
	return fmt.Sprintf("You said slot @%d%s holds YOUR key, but that card's key is not "+
		"what your seed derives at %s. Nothing was engraved.\n\n"+
		"Likely causes: the wrong card for this slot, a mistyped or skipped "+
		"passphrase, or another wallet's card.\n\n"+
		"Reassigning this slot SUPPRESSES the check rather than fixing it. Go back and "+
		"pick a different card, check the passphrase, or rewrite the payload with "+
		"`me sysw pack`.",
		e.Slot, who, e.Declared)
}

// buildFingerprintContradictsMessage is the third row's refusal. It quotes BOTH
// fingerprints, because the whole harm is that they differ and the operator is
// the only one who knows which is right.
//
// It says what a matching fingerprint would and would not have proved, because
// an operator who reads "fingerprint mismatch" and re-cuts the card with a
// corrected fingerprint has fixed the label and not the key.
func buildFingerprintContradictsMessage(e errBuildFingerprintContradicts, origins []cosignerOrigin) string {
	who := ""
	for _, o := range origins {
		if o.slot == e.Slot {
			who = fmt.Sprintf(" (payload card %d)", o.card)
			break
		}
	}
	return fmt.Sprintf("Slot @%d%s's key derives from your seed, but the card's "+
		"fingerprint is %s and your seed's is %s. Nothing was engraved.\n\n"+
		"A fingerprint is written by whoever made the card and is not bound to the "+
		"key, so they can disagree, often from a different passphrase.\n\n"+
		"Re-enter the seed with that passphrase. Reassigning only stops the check. "+
		"If the card is stale, rewrite the payload with `me sysw pack`.",
		e.Slot, who, e.Declared, e.Derived)
}
