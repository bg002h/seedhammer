package address

// Conformance against PUBLISHED BIP test vectors, vendored verbatim in
// testdata/bips (see its README for provenance and for what each BIP does and
// does not publish).
//
// Why this file exists: the rest of address_test.go asserts real derived
// addresses with no cited source, so it proves the device agrees with itself.
// These tests anchor the same code to bytes published outside this project.
//
// Every expected value here is either quoted from a vendored BIP or derived
// from one by a step this file spells out and names. Where a value is derived,
// the comment says which half is published and which half is ours — an
// unattributed expected-address is self-agreement wearing the costume of a
// test.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"seedhammer.com/bip380"
)

// bipVectorSources pins each vendored source by content hash. The commit these were
// taken from is recorded in testdata/bips/README.md; re-pinning means updating
// both. An edited or substituted oracle must be a test failure rather than a
// silent change of ground truth.
var bipVectorSources = map[string]string{
	"bip-0067.mediawiki": "4cc48c5c159c05585962a8eb264b05ccb4ad710b1a16c870232e0f0eb1428991",
	"bip-0084.mediawiki": "1900feec6cafca65b8c09906ca0658d2d742b4c9b44cb15678996985b6bfe627",
	"bip-0086.mediawiki": "d8d01dee331da07c2562615bc1f064c1868ec3fce61184973da76d1196c7f5b0",
	"bip-0143.mediawiki": "62bc71351563e68baeb12643c68355d217953ae9eb6a6e68b2b0323275b6beec",
	"bip-0383.mediawiki": "54d752399568838555d6224f271ed9f2875f16628396c3e0d4c60543bc81ad21",
}

// readBIP returns a vendored BIP source, failing if its content hash does not
// match the pin.
func readBIP(t *testing.T, name string) string {
	t.Helper()
	want, ok := bipVectorSources[name]
	if !ok {
		t.Fatalf("%s: not pinned in bipVectorSources", name)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "bips", name))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != want {
		t.Fatalf("%s: sha256 %s, want %s: the vendored oracle is not the pinned one", name, got, want)
	}
	return string(b)
}

func TestBipVectorSourcesMatchTheirPins(t *testing.T) {
	for name := range bipVectorSources {
		readBIP(t, name)
	}
	ents, err := os.ReadDir(filepath.Join("testdata", "bips"))
	if err != nil {
		t.Fatal(err)
	}
	// A vendored source nobody pinned is a source nobody checked.
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".mediawiki") {
			continue
		}
		if _, ok := bipVectorSources[e.Name()]; !ok {
			t.Errorf("%s is vendored but not pinned in bipVectorSources", e.Name())
		}
	}
}

// section returns the lines of src between the line equal to start and the
// first following line equal to end.
func section(t *testing.T, src, start, end string) []string {
	t.Helper()
	lines := strings.Split(src, "\n")
	from := slices.Index(lines, start)
	if from < 0 {
		t.Fatalf("section %q not found", start)
	}
	rest := lines[from+1:]
	to := slices.Index(rest, end)
	if to < 0 {
		t.Fatalf("section %q has no terminator %q", start, end)
	}
	return rest[:to]
}

// --- BIP-383 -----------------------------------------------------------------

// bip383Vector is one published "descriptor followed by the scripts it
// produces" entry. Descriptors over derived child keys list the 0th, 1st and
// 2nd scripts.
type bip383Vector struct {
	descriptor string
	scripts    []string
}

// bip383ValidVectors is the number of valid descriptor entries in BIP-383's
// Test Vectors section. Asserted so a parser that silently matches nothing
// cannot pass this file's tests vacuously.
const bip383ValidVectors = 9

func parseBip383(t *testing.T, src string) []bip383Vector {
	t.Helper()
	var out []bip383Vector
	for _, ln := range section(t, src, "==Test Vectors==", "Invalid descriptors") {
		switch {
		case strings.HasPrefix(ln, "** <tt>"):
			if len(out) == 0 {
				t.Fatalf("script line before any descriptor: %q", ln)
			}
			v := &out[len(out)-1]
			v.scripts = append(v.scripts, unTT(t, ln, "** "))
		case strings.HasPrefix(ln, "* <tt>"):
			out = append(out, bip383Vector{descriptor: unTT(t, ln, "* ")})
		}
	}
	if len(out) != bip383ValidVectors {
		t.Fatalf("parsed %d BIP-383 valid vectors, want %d", len(out), bip383ValidVectors)
	}
	return out
}

func unTT(t *testing.T, line, prefix string) string {
	t.Helper()
	s := strings.TrimPrefix(line, prefix)
	s = strings.TrimPrefix(s, "<tt>")
	s, ok := strings.CutSuffix(s, "</tt>")
	if !ok {
		t.Fatalf("unterminated <tt> in %q", line)
	}
	return s
}

// TestBip383SortedMultiScriptMatchesPublishedVectors runs BIP-383's
// sortedmulti() vector over two xpubs through this package, at all three
// published child indices.
//
// PUBLISHED: the multisig script bytes for each index — which is exactly the
// witnessScript of the wsh(sortedmulti(...)) this device builds, and which pins
// the two things a wrong implementation gets wrong: the key ORDER after
// derivation, and the script ENCODING.
//
// COMPOSED, and it has to be: BIP-383 publishes no wsh(sortedmulti(...)) vector
// at all (`grep -c 'wsh(sortedmulti' bip-0383.mediawiki` is 0) and no addresses
// of any kind, so the device's actual output shape cannot be quoted end to end
// from any BIP. This test supplies the missing half itself — wrapping the
// published script in wsh() and taking 0020||sha256(script) to a bech32
// address — and that half is stated here rather than smuggled into a fixture.
//
// NOT usable: BIP-383's wsh(...) vectors are all multi(), not sortedmulti(),
// and this codebase refuses unsorted multi by design (subtest below).
func TestBip383SortedMultiScriptMatchesPublishedVectors(t *testing.T) {
	vectors := parseBip383(t, readBIP(t, "bip-0383.mediawiki"))

	var v bip383Vector
	var found int
	for _, c := range vectors {
		// The only sortedmulti() vector over extended keys; the other one is
		// over raw (and partly uncompressed) keys, which no descriptor this
		// device parses can carry.
		if strings.HasPrefix(c.descriptor, "sortedmulti(") && strings.Contains(c.descriptor, "xpub") {
			v, found = c, found+1
		}
	}
	if found != 1 {
		t.Fatalf("found %d sortedmulti-over-xpub vectors in BIP-383, want 1", found)
	}
	if len(v.scripts) != 3 {
		t.Fatalf("vector %q publishes %d scripts, want 3", v.descriptor, len(v.scripts))
	}

	// The wsh() wrapper is ours; everything inside the parentheses is quoted.
	desc, err := bip380.Parse("wsh(" + v.descriptor + ")")
	if err != nil {
		t.Fatalf("parse wsh(%s): %v", v.descriptor, err)
	}
	if desc.Type != bip380.SortedMulti || desc.Script != bip380.P2WSH || desc.Threshold != 2 {
		t.Fatalf("parsed to type=%v script=%v threshold=%d", desc.Type, desc.Script, desc.Threshold)
	}

	for i, s := range v.scripts {
		script, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("index %d: published script is not hex: %v", i, err)
		}
		// DERIVED from the published script: P2WSH commits to sha256 of the
		// witnessScript, so an address match is a script match.
		h := sha256.Sum256(script)
		want, err := address.NewAddressWitnessScriptHash(h[:], &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Receive(desc, uint32(i))
		if err != nil {
			t.Fatalf("index %d: %v", i, err)
		}
		if got != want.String() {
			t.Errorf("index %d: Receive = %s, want %s (P2WSH of BIP-383's published script %s)",
				i, got, want.String(), s)
		}
	}

	// Why the wsh(multi(...)) vectors are not used above: this codebase has no
	// unsorted multi. bip380.MultisigType has two values and Parse accepts only
	// the literal "sortedmulti", so BIP-383's wsh() vectors are precisely the
	// ones the device is designed never to accept. Pinned here so a future
	// reader does not "fix" the omission by adding them.
	t.Run("unsorted_multi_is_refused", func(t *testing.T) {
		unsorted := strings.Replace(v.descriptor, "sortedmulti(", "multi(", 1)
		if unsorted == v.descriptor {
			t.Fatalf("failed to build an unsorted variant of %q", v.descriptor)
		}
		if _, err := bip380.Parse("wsh(" + unsorted + ")"); err == nil {
			t.Fatal("bip380.Parse accepted an unsorted multi()")
		}
	})
}

// --- BIP-67 ------------------------------------------------------------------

// bip67Vector is one published vector: the four fields List, Sorted, Script and
// Address.
type bip67Vector struct {
	name    string
	list    []string
	sorted  []string
	script  string
	address string
}

// bip67Vectors is the number of vectors BIP-67 publishes. Note this is 4, not
// the 5 addresses an earlier recon reported.
const bip67Vectors = 4

func parseBip67(t *testing.T, src string) []bip67Vector {
	t.Helper()
	var out []bip67Vector
	var field string
	for _, ln := range section(t, src, "==Test vectors==", "==Acknowledgements==") {
		switch {
		case strings.HasPrefix(ln, "Vector "):
			out = append(out, bip67Vector{name: strings.TrimSpace(ln)})
		case strings.HasPrefix(ln, "* "):
			// The document is inconsistent about a trailing colon.
			field = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(ln, "* ")), ":")
		case strings.HasPrefix(ln, "** "):
			if len(out) == 0 {
				t.Fatalf("value before any vector: %q", ln)
			}
			v := &out[len(out)-1]
			val := strings.TrimSpace(strings.TrimPrefix(ln, "** "))
			switch field {
			case "List":
				v.list = append(v.list, val)
			case "Sorted":
				v.sorted = append(v.sorted, val)
			case "Script":
				v.script = val
			case "Address":
				v.address = val
			default:
				t.Fatalf("unknown field %q in %q", field, ln)
			}
		}
	}
	if len(out) != bip67Vectors {
		t.Fatalf("parsed %d BIP-67 vectors, want %d", len(out), bip67Vectors)
	}
	for _, v := range out {
		if len(v.list) == 0 || len(v.list) != len(v.sorted) || v.script == "" || v.address == "" {
			t.Fatalf("%s: incomplete: list=%d sorted=%d script=%q address=%q",
				v.name, len(v.list), len(v.sorted), v.script, v.address)
		}
	}
	return out
}

// TestBip67SortedMultiKeyOrderScriptAndAddress drives this package's own
// ordering and script construction with BIP-67's published keys, and asserts
// all four published fields: the sorted key order, the multisig script, and the
// P2SH address.
//
// Everything asserted here is quoted. Nothing is derived.
//
// This is the one property with no other symptom: a wrong sort produces a
// perfectly well-formed address for a wallet nobody can restore. Vector 2 is
// already sorted (a genuine no-op case), and vector 3's keys differ only in the
// 02/03 prefix and the final byte, which is what a comparator that sorts on the
// wrong bytes gets wrong.
func TestBip67SortedMultiKeyOrderScriptAndAddress(t *testing.T) {
	vectors := parseBip67(t, readBIP(t, "bip-0067.mediawiki"))
	// "Two signatures are required in each of these test vectors."
	const threshold = 2

	for _, v := range vectors {
		t.Run(strings.ReplaceAll(v.name, " ", "_"), func(t *testing.T) {
			keys := make([]*address.AddressPubKey, 0, len(v.list))
			for _, k := range v.list {
				b, err := hex.DecodeString(k)
				if err != nil {
					t.Fatalf("%s: not hex: %v", k, err)
				}
				pk, err := address.NewAddressPubKey(b, &chaincfg.MainNetParams)
				if err != nil {
					t.Fatalf("%s: %v", k, err)
				}
				keys = append(keys, pk)
			}

			// The production sort, in place, on the published unsorted list.
			script, err := sortedMultisigScript(keys, threshold)
			if err != nil {
				t.Fatalf("sortedMultisigScript: %v", err)
			}

			var order []string
			for _, k := range keys {
				order = append(order, hex.EncodeToString(k.PubKey().SerializeCompressed()))
			}
			if !slices.Equal(order, v.sorted) {
				t.Errorf("sorted order:\n got %v\nwant %v", order, v.sorted)
			}
			if got := hex.EncodeToString(script); got != v.script {
				t.Errorf("script:\n got %s\nwant %s", got, v.script)
			}
			addr, err := address.NewAddressScriptHash(script, &chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("NewAddressScriptHash: %v", err)
			}
			if got := addr.String(); got != v.address {
				t.Errorf("P2SH address: got %s, want %s", got, v.address)
			}
		})
	}
}

// --- BIP-143 -----------------------------------------------------------------

// bip143Nested is BIP-143's P2SH-P2WSH example: a 6-of-6 multisig with all
// three script layers published.
type bip143Nested struct {
	scriptPubKey  string
	redeemScript  string
	witnessScript string
}

func parseBip143Nested(t *testing.T, src string) bip143Nested {
	t.Helper()
	var out bip143Nested
	for _, ln := range section(t, src, "=== P2SH-P2WSH ===", "=== No FindAndDelete ===") {
		name, val, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		// "scriptPubKey : a914...87, value: 9.87654321"
		val, _, _ = strings.Cut(strings.TrimSpace(val), ",")
		switch strings.TrimSpace(name) {
		case "scriptPubKey":
			out.scriptPubKey = val
		case "redeemScript":
			out.redeemScript = val
		case "witnessScript":
			out.witnessScript = val
		}
	}
	if out.scriptPubKey == "" || out.redeemScript == "" || out.witnessScript == "" {
		t.Fatalf("BIP-143 P2SH-P2WSH: incomplete: %+v", out)
	}
	return out
}

// TestBip143NestedP2wshScriptPubKeyMatchesPublishedVector pins the P2SH-P2WSH
// nesting the device uses for sh(wsh(sortedmulti(...))).
//
// It replaces the BIP-141 test this deliverable originally named: BIP-141
// publishes no vectors at all — every example in it is a structural template
// with no concrete hash, key or script (`grep -cE '[0-9a-f]{40,}'` over
// bip-0141.mediawiki is 0) — so there was nothing there to quote or even to
// derive from. BIP-143 §P2SH-P2WSH publishes a concrete 6-of-6 multisig with
// all three layers, and it is a multisig, which is what this device cuts.
//
// Part A quotes: it rebuilds the published redeemScript and scriptPubKey from
// the published witnessScript, both by explicit byte construction and through
// the same btcd calls addressAt uses, so neither this project nor btcd is
// taken at its word.
//
// Part B ties the device to that algebra, using BIP-383's published scripts as
// the witnessScript. Between them the whole chain the device walks is anchored
// in published bytes: the script (BIP-383), the nesting (BIP-143), and the
// base58 P2SH encoding of the result (BIP-67's published addresses).
func TestBip143NestedP2wshScriptPubKeyMatchesPublishedVector(t *testing.T) {
	v := parseBip143Nested(t, readBIP(t, "bip-0143.mediawiki"))
	ws, err := hex.DecodeString(v.witnessScript)
	if err != nil {
		t.Fatalf("witnessScript is not hex: %v", err)
	}

	t.Run("published_chain", func(t *testing.T) {
		// redeemScript = OP_0 PUSH32 sha256(witnessScript)
		h := sha256.Sum256(ws)
		rs := append([]byte{txscript.OP_0, 32}, h[:]...)
		if got := hex.EncodeToString(rs); got != v.redeemScript {
			t.Fatalf("redeemScript: got %s, want %s", got, v.redeemScript)
		}
		// scriptPubKey = OP_HASH160 PUSH20 hash160(redeemScript) OP_EQUAL
		spk := append([]byte{txscript.OP_HASH160, 20}, address.Hash160(rs)...)
		spk = append(spk, txscript.OP_EQUAL)
		if got := hex.EncodeToString(spk); got != v.scriptPubKey {
			t.Fatalf("scriptPubKey: got %s, want %s", got, v.scriptPubKey)
		}

		// The same two steps through the calls addressAt makes.
		wa, err := address.NewAddressWitnessScriptHash(h[:], &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		rs2, err := txscript.PayToAddrScript(wa)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(rs2); got != v.redeemScript {
			t.Errorf("btcd redeemScript: got %s, want %s", got, v.redeemScript)
		}
		sa, err := address.NewAddressScriptHash(rs2, &chaincfg.MainNetParams)
		if err != nil {
			t.Fatal(err)
		}
		spk2, err := txscript.PayToAddrScript(sa)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(spk2); got != v.scriptPubKey {
			t.Errorf("btcd scriptPubKey: got %s, want %s", got, v.scriptPubKey)
		}
	})

	// The device cannot be driven with BIP-143's raw keys — its descriptors
	// carry extended keys only — so the tie-in reuses BIP-383's published
	// sortedmulti scripts as the witnessScript and asks for the nested form.
	t.Run("device_nesting_follows_it", func(t *testing.T) {
		vectors := parseBip383(t, readBIP(t, "bip-0383.mediawiki"))
		var inner string
		for _, c := range vectors {
			if strings.HasPrefix(c.descriptor, "sortedmulti(") && strings.Contains(c.descriptor, "xpub") {
				inner = c.descriptor
			}
		}
		if inner == "" {
			t.Fatal("no sortedmulti-over-xpub vector in BIP-383")
		}
		desc, err := bip380.Parse("sh(wsh(" + inner + "))")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if desc.Script != bip380.P2SH_P2WSH {
			t.Fatalf("parsed to script %v, want P2SH-P2WSH", desc.Script)
		}
		scripts := vectors[slices.IndexFunc(vectors, func(c bip383Vector) bool {
			return c.descriptor == inner
		})].scripts
		if len(scripts) != 3 {
			t.Fatalf("got %d published scripts, want 3", len(scripts)) // no vacuous loop
		}
		for i, s := range scripts {
			script, err := hex.DecodeString(s)
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(script)
			rs := append([]byte{txscript.OP_0, 32}, h[:]...) // the form Part A verified
			want, err := address.NewAddressScriptHash(rs, &chaincfg.MainNetParams)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Receive(desc, uint32(i))
			if err != nil {
				t.Fatalf("index %d: %v", i, err)
			}
			if got != want.String() {
				t.Errorf("index %d: Receive = %s, want %s", i, got, want.String())
			}
		}
	})
}

// --- BIP-84 / BIP-86 — the singlesig shapes ----------------------------------

// bip8xAccount is the shape BIP-84 and BIP-86 share: one account-level extended
// public key plus the first two receive addresses and the first change address
// derived under it. Both are rooted at the standard
// "abandon abandon … about" mnemonic.
type bip8xAccount struct {
	xpub    string
	recv    [2]string
	change0 string
	// spk is BIP-86's published scriptPubKey per receive address; BIP-84
	// publishes none, so it stays empty there.
	spk [2]string
}

// parseBip8xAccount reads the `key = value` lines of a BIP-84/86 Test vectors
// block, keyed by the `// Account 0, …` comment that introduces each group.
func parseBip8xAccount(t *testing.T, src string) bip8xAccount {
	t.Helper()
	var out bip8xAccount
	var group string
	for _, ln := range section(t, src, "==Test vectors==", "==Reference==") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "//") {
			group = ln
			continue
		}
		key, val, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch {
		case strings.Contains(group, "Account 0, root") && key == "xpub":
			out.xpub = val
		case strings.Contains(group, "first receiving address"):
			switch key {
			case "address":
				out.recv[0] = val
			case "scriptPubKey":
				out.spk[0] = val
			}
		case strings.Contains(group, "second receiving address"):
			switch key {
			case "address":
				out.recv[1] = val
			case "scriptPubKey":
				out.spk[1] = val
			}
		case strings.Contains(group, "first change address") && key == "address":
			out.change0 = val
		}
	}
	if out.xpub == "" || out.recv[0] == "" || out.recv[1] == "" || out.change0 == "" {
		t.Fatalf("incomplete account vector: %+v", out)
	}
	return out
}

// TestBip84And86SinglesigAddressesMatchPublishedVectors runs the two singlesig
// shapes for which a BIP publishes real mainnet addresses.
//
// Everything asserted here is quoted — the account xpub, all three addresses,
// and BIP-86's scriptPubKeys. Nothing is derived, and no descriptor text is
// invented beyond the script function itself, because both BIPs publish the
// account-level key that a descriptor names directly.
//
// The unqualified xpub relies on this package's documented default of <0;1>/*,
// so Receive(i) is .../0/i and Change(i) is .../1/i — which is exactly the
// m/84'/0'/0'/{0,1}/i and m/86'/0'/0'/{0,1}/i the vectors publish. That default
// is therefore itself pinned here, against published bytes.
//
// The other two singlesig shapes have no equivalent, and this is not an
// oversight — see testdata/bips/README.md:
//   - pkh: BIP-44 publishes no test vectors of any kind.
//   - sh(wpkh): BIP-49's vectors are TESTNET (upub / 2Mww8…), and this package
//     rejects the upub version outright, so reaching them needs a SLIP-132
//     version rewrite. Left undone rather than done quietly.
func TestBip84And86SinglesigAddressesMatchPublishedVectors(t *testing.T) {
	for _, tc := range []struct {
		bip    string
		script string
	}{
		{"bip-0084.mediawiki", "wpkh"}, // account xpub is a zpub
		{"bip-0086.mediawiki", "tr"},
	} {
		t.Run(tc.script, func(t *testing.T) {
			v := parseBip8xAccount(t, readBIP(t, tc.bip))
			desc, err := bip380.Parse(tc.script + "(" + v.xpub + ")")
			if err != nil {
				t.Fatalf("parse %s(%s): %v", tc.script, v.xpub, err)
			}
			for i, want := range v.recv {
				got, err := Receive(desc, uint32(i))
				if err != nil {
					t.Fatalf("receive %d: %v", i, err)
				}
				if got != want {
					t.Errorf("receive %d: got %s, want %s", i, got, want)
				}
				if v.spk[i] == "" {
					continue
				}
				// Second published field: the output script the address encodes.
				a, err := address.DecodeAddress(got, &chaincfg.MainNetParams)
				if err != nil {
					t.Fatalf("decode %s: %v", got, err)
				}
				spk, err := txscript.PayToAddrScript(a)
				if err != nil {
					t.Fatal(err)
				}
				if h := hex.EncodeToString(spk); h != v.spk[i] {
					t.Errorf("receive %d scriptPubKey: got %s, want %s", i, h, v.spk[i])
				}
			}
			got, err := Change(desc, 0)
			if err != nil {
				t.Fatalf("change 0: %v", err)
			}
			if got != v.change0 {
				t.Errorf("change 0: got %s, want %s", got, v.change0)
			}
		})
	}
}
