package sysw

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip380"
	"seedhammer.com/nonstandard"
)

// SPEC_descriptor_input.md §5.2's CLASSIFICATION PREDICATE, ported.
//
// The spec states it once and both sides implement it:
//
//	A record is ClassDescriptor iff it parses under §4's cascade AND matches
//	§4.7's grammar -- the seven forms; conjunct 1's md1-path widening does not
//	apply here.
//
// The Rust primary is descriptor::host_admits
// (mnemonic-engrave crates/me-cli/src/descriptor/admit.rs), which is
// `cascade(normalise(input))` followed by the eight conjuncts. The
// Rust-primary rule makes parity MANDATORY rather than desirable, and the gate
// is nonstandard/testdata/descriptor_seam_vectors.json: its host_admits column
// is asserted against BOTH implementations, per row, so a divergence anywhere
// reds one of the two suites instead of shipping.
//
// WHY THIS IS NOT A CALL TO nonstandard.OutputDescriptor. That function is the
// SCAN DOOR, and it is measurably wider than the host: over the vector file it
// answers TRUE on 18 SINGLE-LINE rows the host refuses -- anyone-can-spend
// `sortedmulti(0,...)`, k > n, 21 keys, a 16-key `sh(sortedmulti(...))` whose
// redeemScript can never be spent, mixed networks, hardened use-sites, a
// bare `tpub`, a full-origin `ypub`, and two key-identity collisions. A record
// cannot contain a newline, so those 18 are exactly what a scan-door-keyed
// classifier would place -- and every one of them would reach a program and a
// screen through the admission cells in gui/sysw_admit.go, which are live.
//
// So the PARSE comes from OutputDescriptor and the ADMISSION comes from the
// narrowings below: the cascade's two single-line-reachable ones (§4.3's
// version set and §4.5's promotion ruling), then §4.7's conjuncts over the
// parsed descriptor.
//
// A record failing any of it is ClassUnknown and goes INERT -- the existing
// contract for an unclassifiable record (it stays in the session, is offered to
// nobody, and reaches no screen).
func isDescriptorRecord(raw, record string) (admitted bool) {
	// sysw.Classify runs over EVERY record of every loaded payload
	// (gui/sysw_session.go), so a panic below is a payload that will not LOAD
	// rather than a record that will not classify. Both panics this spec
	// measured are closed -- the short-fingerprint one in nonstandard/parse.go,
	// and Descriptor.Encode (§4.2 defects 1-2) is never called from here -- but
	// the parsers underneath are handed operator bytes from a container this
	// device did not write, and failing closed to ClassUnknown is the same
	// answer a refusal already gives.
	defer func() {
		if recover() != nil {
			admitted = false
		}
	}()

	// §4.6 FIRST, because it decides WHICH STRING the rest of this is about.
	//
	// `classifyConstellation` reached here through strings.TrimSpace, which
	// trims by unicode.IsSpace; the host's §4.6 normalisation is
	// `replace("\r\n","\n")` then a trim by `char::is_ascii_whitespace`
	// (crates/me-cli/src/descriptor/cascade.rs:36). The two sets differ by
	// U+000B and the whole Unicode Zs category, so a record padded with a
	// no-break space is a DIFFERENT string to the two sides: the device trims
	// it off and sees a wallet, and `me` does not and refuses at rc 3
	// (measured, 20 cases, both positions, descriptor and bare-key forms).
	// That is the device-WIDER direction this whole arm exists to close, so
	// the arm answers for its own §4.6 rather than inheriting the shared trim
	// -- which predates S2 and which the md1/mk1 arms rely on being Unicode.
	if asciiNormalise(raw) != record {
		return false
	}
	// THE PARSE next, and its position is a cost decision rather than a
	// semantic one -- this is a conjunction, so every arm has to hold whichever
	// runs first. Every record of every payload reaches here, and the
	// overwhelming majority are not descriptors at all; running the cascade
	// once and only then re-reading the record's bytes keeps their cost to that
	// one cascade.
	desc, err := nonstandard.OutputDescriptor([]byte(record))
	if err != nil {
		return false
	}
	// Then the cascade's two single-line-reachable NARROWINGS, which need the
	// version bytes the parser has just normalised away -- over the KEY TEXT
	// the cascade consumed, which is not always the whole record.
	if !keyVersionsAdmitted(cascadeKeyText(record), promotesABareKey(record)) {
		return false
	}
	// Then §4.7's conjuncts over what the cascade produced.
	return admitDescriptor(desc)
}

// asciiWhitespace is exactly `char::is_ascii_whitespace`: SPACE, TAB, LF, FF,
// CR. **U+000B VERTICAL TAB IS DELIBERATELY ABSENT** -- Rust excludes it and
// Go's unicode.IsSpace includes it, and that one character is four of the
// twenty measured divergences.
const asciiWhitespace = " \t\n\f\r"

// asciiNormalise is §4.6, ported exactly: CRLF becomes LF everywhere, then
// ASCII whitespace comes off both ends. Nothing else.
//
// It does not violate §7's invariant, and the reason is mechanical: what `me`
// PACKS is the canonical re-encoded descriptor, never the operator's file, and
// a sysw record cannot contain a newline by construction -- so the device never
// sees the whitespace the host absorbed. What it must not do is absorb MORE
// than the host, which is what this exists to prevent.
func asciiNormalise(s string) string {
	return strings.Trim(strings.ReplaceAll(s, "\r\n", "\n"), asciiWhitespace)
}

// cascadeKeyText is the substring of the record whose version bytes the HOST
// actually checks -- which is NOT the record.
//
// The host checks a version inside `parse_extended_key`, which the cascade
// reaches only through a KEY EXPRESSION. Three of the four branches hand it the
// whole input or something derived from it, but **branch 3 hands it the
// `descriptor` field alone**: `{"label": …, "descriptor": …}` copies `label`
// into `desc.Title` and never parses it (nonstandard/parse.go:44-55). A label
// is arbitrary operator text, `"`/`{`/`}`/`:`/`,` are all outside the base58
// alphabet, and an extended key is a legal thing to call a wallet -- so a label
// is its own maximal base58 run and scanning the whole record refuses a record
// whose every KEY is an `xpub`.
//
// MEASURED, both the reviewer's case and the sharper one it does not cover: a
// JSON record whose label is an unrelated `ypub`, AND one whose label is the
// `ypub` spelling of a key that IS in the descriptor, are both `host_admits`
// (rc 0 under `me sysw pack --as descriptor`) while the whole-record scan
// refused them. The second is why matching a run's key MATERIAL against the
// parsed keys is not a fix: the host does not look at the label at all, so
// neither may this.
//
// Branch 1 (BlueWallet) has the same shape -- `Name:`'s value is a title -- so
// it is scoped the same way, to the values of the headers the parser does NOT
// recognise, which are the only ones it parses as keys. Order matters and it is
// JSON FIRST: a single-line `{"label": "x", "descriptor": "y"}` is
// header-shaped under a `": "` split, and branch 1 fails on it, so testing the
// header shape first would scope a JSON record to a "value" holding its own
// label.
//
// Where a record is header-shaped and branch 1 nevertheless FAILED, the scope
// cannot matter: branch 2 needs a known script name before the first `(` and a
// parseable descriptor contains no `": "` at all, branch 3 is already excluded,
// and branch 4's ParseKey rejects a `": "` outright -- so OutputDescriptor
// errors and the arm has already returned.
func cascadeKeyText(record string) string {
	// Branch 3 -- {label, descriptor} JSON.
	var doc struct {
		Label      string `json:"label"`
		Descriptor string `json:"descriptor"`
	}
	if err := json.Unmarshal([]byte(record), &doc); err == nil {
		if _, err := bip380.Parse(doc.Descriptor); err == nil {
			return doc.Descriptor
		}
	}
	// Branch 1 -- BlueWallet. Its key material is the value of every header
	// whose name is not one of the four the parser recognises.
	if vals, ok := blueWalletKeyValues(record); ok {
		return strings.Join(vals, "\n")
	}
	// Branches 2 and 4 -- a plain BIP-380 descriptor and a bare key expression.
	// Both grammars are closed and carry no free-text field, so every run in
	// them is key material.
	return record
}

// blueWalletHeaders is the four names `parseBlueWalletDescriptor` recognises
// (nonstandard/parse.go:103-127). Everything else is a `fingerprint: xpub`
// pair, and only those values reach ParseExtendedKey.
var blueWalletHeaders = [...]string{"Name", "Policy", "Derivation", "Format"}

// blueWalletKeyValues returns branch 1's key material, and reports false when
// the record is not header-shaped at all -- which is branch 1's own first
// refusal ("bluewallet: invalid header").
func blueWalletKeyValues(record string) ([]string, bool) {
	var vals []string
	var lines int
	for _, l := range strings.Split(record, "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, ": ")
		if !ok {
			return nil, false
		}
		lines++
		if !slices.Contains(blueWalletHeaders[:], k) {
			vals = append(vals, v)
		}
	}
	return vals, lines > 0
}

// admittedVersions is §4.3's admitted set, and it is exactly five: xpub, tpub,
// zpub, Ypub, Zpub. NORMATIVE -- `me` admits the same five and no more.
var admittedVersions = []string{
	"0488b21e", // xpub
	"043587cf", // tpub
	"04b24746", // zpub
	"0295b43f", // Ypub
	"02aa7ed3", // Zpub
}

const tpubVersion = "043587cf"

// minExtendedKeyLen is the shortest an extended key can be spelled. The
// serialisation is 78 bytes plus a 4-byte checksum, and every admitted version
// byte is 0x02 or 0x04, so there is no leading-zero run and base58check always
// produces 111 or 112 characters. 100 is that floor with room to spare, and it
// is what keeps this scan off `sortedmulti`, a fingerprint and a path element.
const minExtendedKeyLen = 100

// keyVersionsAdmitted answers §4.3's version conjunct and §4.5's promotion
// ruling in ONE pass over `keyText` -- the substring the cascade consumed as
// key material, which [cascadeKeyText] narrows and which is NOT always the
// whole record.
//
// IT HAS TO BE A STRING-LEVEL CHECK, and that is structural rather than a
// preference: bip380.Key carries no version field, and ParseExtendedKey
// NORMALISES the version away before the Key exists (bip380/bip380.go:456-462
// -- a zpub becomes an xpub, a ypub becomes an xpub). By the time there is a
// descriptor to run a conjunct over, the byte that decides this is gone. F-426
// widened the SCAN DOOR to accept `ypub` (bip380/bip380.go:447), which is the
// seam-safe direction -- a device that accepts more can never be handed an
// unreadable payload -- so without this check the classifier would place a
// record `me` refuses at rc 3. It runs on BOTH paths, a key inside a descriptor
// and a bare key the cascade promotes, because the version reaches neither.
//
// §4.5's ruling is NORMATIVE and a ruling rather than a transcription: `me`
// refuses `tpub` promotion ENTIRELY. A testnet key whose only claim to being a
// wallet is a version byte mapping to the MAINNET path 44'/0'/0' is an
// inference the host declines to make. The device stays wider and §7's
// invariant permits the host to be narrower -- and this is the one row a
// conjuncts-only port gets wrong (promotion/15-bare-tpub-host-refused): every
// §4.7 conjunct passes on it, because `tpub` IS admitted for a key inside a
// descriptor. The promotion branch is where the narrowing lives.
func keyVersionsAdmitted(keyText string, promoted bool) bool {
	ok := true
	eachExtendedKey(keyText, func(version string) {
		if !slices.Contains(admittedVersions, version) {
			ok = false
		}
		if promoted && version == tpubVersion {
			ok = false
		}
	})
	return ok
}

// promotesABareKey reports whether this record reaches the cascade's branch 4,
// the promoted bare key (nonstandard/parse.go:58-73).
//
// The test is exact rather than a guess at the shape. ParseKey succeeding on
// the WHOLE record is precisely branch 4's condition, because a key expression
// reaches branch 4 only by failing the three before it -- and it fails all
// three by construction: it carries no ": " for branch 1's header split, no "("
// for branch 2's script, and is not a JSON document for branch 3.
func promotesABareKey(record string) bool {
	_, err := bip380.ParseKey(nil, []byte(record))
	return err == nil
}

// eachExtendedKey calls f with the hex version bytes of every extended key
// spelled in `keyText`, in order. Runs shorter than an extended key are skipped
// without decoding, and a run that is long enough but does not decode is not a
// key -- the parse refuses it, or never looks at it.
//
// The CALLER decides what `keyText` is, and that is the whole of C1's fix: this
// function cannot tell a key from a wallet's name, so it is never handed a
// string that can contain one.
func eachExtendedKey(keyText string, f func(version string)) {
	record := keyText
	for i := 0; i < len(record); {
		if !isBase58(record[i]) {
			i++
			continue
		}
		j := i
		for j < len(record) && isBase58(record[j]) {
			j++
		}
		if j-i >= minExtendedKeyLen {
			if xpub, err := hdkeychain.NewKeyFromString(record[i:j]); err == nil {
				f(hex.EncodeToString(xpub.Version()))
			}
		}
		i = j
	}
}

// isBase58 is Bitcoin's alphabet: the digits and letters MINUS `0`, `O`, `I`
// and `l`, the four that read alike. Every character a descriptor uses to
// SEPARATE things -- `(`, `)`, `,`, `[`, `]`, `/`, `<`, `>`, `;`, `*`, `#`,
// `'`, `:`, a space -- is outside it, which is what makes a maximal run of
// these characters exactly one token.
func isBase58(c byte) bool {
	switch {
	case c >= '1' && c <= '9':
		return true
	case c >= 'A' && c <= 'Z':
		return c != 'I' && c != 'O'
	case c >= 'a' && c <= 'z':
		return c != 'l'
	}
	return false
}

// admitDescriptor is §4.7's admission predicate, conjunct by conjunct, over a
// descriptor the cascade produced. It mirrors admit.rs's `admit` under
// Path::Descriptor -- SEMANTICS, not lines.
//
// Conjunct 1's md1-path widening and its permanent `--as descriptor` refusal
// have no arm here and cannot: this parser has no unsorted `multi` at all
// (bip380/bip380.go:335 cases `sortedmulti` alone), so a `multi` policy is a
// parse REFUSAL before admission is asked.
//
// Conjunct 4 (version bytes) is not here either -- see keyVersionsAdmitted,
// which is the only place the answer still exists.
func admitDescriptor(d *bip380.Descriptor) bool {
	return descriptorShapeOK(d) &&
		thresholdOK(d) &&
		keyCountOK(d) &&
		networkOK(d) &&
		originsOK(d) &&
		useSitesOK(d) &&
		keyIdentityOK(d)
}

// Conjunct 1, the shape half: the seven forms.
//
//	pkh(KEY)   wpkh(KEY)   sh(wpkh(KEY))   tr(KEY)
//	wsh(sortedmulti(k,KEY...))   sh(wsh(sortedmulti(k,KEY...)))
//	sh(sortedmulti(k,KEY...))
func descriptorShapeOK(d *bip380.Descriptor) bool {
	multisigSlot := d.Script == bip380.P2WSH || d.Script == bip380.P2SH_P2WSH || d.Script == bip380.P2SH
	switch d.Type {
	case bip380.Singlesig:
		// `wsh(KEY)` and `sh(KEY)` arrive here too, with a multisig slot: Parse
		// builds them as Singlesig and they are not descriptors -- measured
		// address.Supported false, no derivable address. So the answer is the
		// SCRIPT's, not the type's.
		return d.Script.Singlesig()
	case bip380.SortedMulti:
		// A sortedmulti in a single-key script slot is refused: taproot
		// multisig is multi_a/sortedmulti_a (BIP-387), so `tr(sortedmulti(...))`
		// is not a descriptor, and wpkh/pkh/sh(wpkh) each take exactly one key
		// -- the device cannot even derive an address from the last one.
		return multisigSlot
	}
	return false
}

// Conjunct 2. 1 <= k <= n. This parser makes NO threshold check at all: it
// accepts `sortedmulti(0, ...)` -- spendable by ANYONE -- and
// `sortedmulti(-1, ...)`, and derives real addresses for both.
//
// It is also what refuses the titled zero-key BlueWallet shape once the shape
// conjunct admits one, and that matters beyond tidiness: such a descriptor
// PANICS Descriptor.Encode, and the screen this class routes to encodes.
func thresholdOK(d *bip380.Descriptor) bool {
	if d.Type != bip380.SortedMulti {
		return true
	}
	return d.Threshold >= 1 && d.Threshold <= len(d.Keys)
}

// Conjunct 3 (BIP-383). The bound is the redeemScript's, not the ordering's.
//
// Under a DIRECT sh(...) the multi's own output script IS the redeemScript --
// one script element capped at 520 bytes, and 16 compressed keys need 547. In
// sh(wsh(...)) the redeemScript is the 34-byte `OP_0 <sha256>`, so only
// OP_CHECKMULTISIG's 20-key consensus limit binds, which is why a 16-key
// sh(wsh(sortedmulti(...))) is a SPENDABLE wallet and an accepted row.
//
// Measured on the refused side: this parser ACCEPTS `sh(sortedmulti(2, 16
// keys))` and `wsh(sortedmulti(2, 21 keys))` and derives payable-looking
// addresses for both -- scripts that can never be spent.
func keyCountOK(d *bip380.Descriptor) bool {
	if d.Type != bip380.SortedMulti {
		return true
	}
	max := 20
	if d.Script == bip380.P2SH {
		max = 15
	}
	return len(d.Keys) <= max
}

// Conjunct 5. All keys share one network. A mixed xpub/tpub sortedmulti parses
// clean here and address.Receive then refuses it ("multisig descriptor mixes
// networks", address/address.go:105-107), so the record would reach programs
// whose whole job is deriving addresses they cannot derive.
func networkOK(d *bip380.Descriptor) bool {
	if len(d.Keys) == 0 {
		return true
	}
	for _, k := range d.Keys {
		if k.Network != d.Keys[0].Network {
			return false
		}
	}
	return true
}

// Conjunct 6. A key that declares a fingerprint carries a non-empty origin
// path. Descriptor.encode emits the `[...]` block iff the fingerprint is
// non-zero (bip380/bip380.go:228) and ParseKey then requires a `/` at offset 8
// -- so this state re-encodes to a string this parser cannot read back.
//
// An ALL-ZERO fingerprint is the one case where a key legitimately carries no
// origin block: "master unknown" is not a claim about identity, and refusing it
// would reject files several coordinators emit.
func originsOK(d *bip380.Descriptor) bool {
	for _, k := range d.Keys {
		if k.MasterFingerprint != 0 && len(k.DerivationPath) == 0 {
			return false
		}
	}
	return true
}

// Conjunct 7. The closed set {absent, /*, /i/*, <i;i+1>, <i;i+1>/*}.
//
// Everything else in parsePath's grammar is refused as UNMEASURED, per the
// closed-set rule -- including the two classes measured BROKEN. A HARDENED
// use-site component silently derives the UNhardened child, so the device
// displays addresses for a wallet that cannot exist (hardened derivation from
// an xpub is impossible). A NON-CONSECUTIVE `<a;b>` parses, and then
// address.Receive errors `unsupported range path element` while
// address.Supported still returns true -- conjunct 5's class again.
func useSitesOK(d *bip380.Descriptor) bool {
	for _, k := range d.Keys {
		if !useSiteOK(k.Children) {
			return false
		}
	}
	return true
}

func useSiteOK(c []bip380.Derivation) bool {
	switch len(c) {
	case 0:
		return true
	case 1:
		return plainWildcard(c[0]) || consecutiveRange(c[0])
	case 2:
		head := consecutiveRange(c[0]) ||
			(c[0].Type == bip380.ChildDerivation && !c[0].Hardened)
		return head && plainWildcard(c[1])
	}
	return false
}

func plainWildcard(d bip380.Derivation) bool {
	return d.Type == bip380.WildcardDerivation && !d.Hardened
}

// parsePath already refuses a hardened or reversed range and cuts on the FIRST
// `;`, so consecutiveness is the only thing left to ask.
func consecutiveRange(d bip380.Derivation) bool {
	return d.Type == bip380.RangeDerivation && d.End == d.Index+1
}

// Conjunct 8 -- the two impossible-wallet checks (F-217/F-218).
//
// (a) One origin identifies exactly one key, so two keys declaring the same
// (fingerprint, origin path) with DIFFERENT key material describe no wallet.
// Two keys "declare the same origin" only when both actually declare one: an
// absent fingerprint means "master unknown", which is not a claim about
// identity and cannot contradict another key's.
//
// (b) No two slots carry the same (xpub, use-site path). Keyed on the USE SITE
// and not the origin, deliberately: the same xpub at <0;1>/* and <2;3>/* is a
// legal two-chain wallet, measured, and this device derives a distinct address
// for each.
func keyIdentityOK(d *bip380.Descriptor) bool {
	for i := range d.Keys {
		for j := i + 1; j < len(d.Keys); j++ {
			a, b := &d.Keys[i], &d.Keys[j]
			same := sameKeyMaterial(a, b)
			if !same && a.MasterFingerprint != 0 &&
				a.MasterFingerprint == b.MasterFingerprint &&
				slices.Equal(a.DerivationPath, b.DerivationPath) {
				return false
			}
			if same && slices.Equal(a.Children, b.Children) {
				return false
			}
		}
	}
	return true
}

func sameKeyMaterial(a, b *bip380.Key) bool {
	return bytes.Equal(a.KeyData, b.KeyData) && bytes.Equal(a.ChainCode, b.ChainCode)
}
