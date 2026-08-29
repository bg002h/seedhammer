package nonstandard_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
	"seedhammer.com/nonstandard"
	"seedhammer.com/sysw"
)

// THE HOST/DEVICE DESCRIPTOR SEAM, as a gate rather than a comment.
//
// Two independent descriptor parsers will drift. `me`'s and this one are the
// same question asked in two languages, and the direction that matters is
// asymmetric: a host that admits what this parser refuses packs a payload this
// device cannot read -- an engraved plate for a wallet that will not load.
//
// This half asserts the DEVICE columns of testdata/descriptor_seam_vectors.json.
// mnemonic-engrave's crates/me-cli/tests/descriptor_seam.rs reads a
// BYTE-IDENTICAL copy of the same file and asserts the HOST columns. Neither
// implementation is ever compared to the other -- both are compared to the
// file, which is why it has to be the same file. seamVectorsSHA256 below is
// pinned identically in the Rust test, so the two copies cannot drift without
// one of the two suites going red.
//
// The file is authored in the Rust primary (SPEC_descriptor_input.md §3 makes
// `me` the primary from the moment these vectors land) and vendored here; its
// own header carries the per-row provenance and the regenerate + re-pin recipe.
//
// The package is `nonstandard_test` (EXTERNAL) deliberately: once §5.2's
// classifier arm lands, `sysw` imports `nonstandard`, and an internal test
// importing `sysw` for the sysw_class column would be an import cycle.
const seamVectorsSHA256 = "e7a4160ce064a6cb7ca31dc530e079c861cf2c8a075d75f793ef0d935f583758"

const seamVectorsPath = "testdata/descriptor_seam_vectors.json"

type seamRow struct {
	Name                 string   `json:"name"`
	Input                string   `json:"input"`
	SHA256               string   `json:"sha256"`
	HostAdmits           bool     `json:"host_admits"`
	DeviceAdmits         *bool    `json:"device_admits"`
	Md1Admits            bool     `json:"md1_admits"`
	Format               string   `json:"format"`
	Canonical            string   `json:"canonical"`
	DeviceProbe          string   `json:"device_probe"`
	Address0             string   `json:"address_0"`
	Address1             string   `json:"address_1"`
	WalletID             string   `json:"wallet_id"`
	MdDescriptorContains string   `json:"md_descriptor_contains"`
	Covers               []string `json:"covers"`
}

// The per-column POPULATION counts this half owns, pinned as literals exactly
// as the Rust half pins its own. A field renamed by a typo drops its count and
// reds the suite instead of silently disabling every assertion that reads it.
const (
	wantRows        = 72
	wantDeviceTrue  = 38
	wantDeviceFalse = 34
	wantCanonical   = 19
	wantAddress0    = 20
	wantAddress1    = 5
	wantWalletID    = 4
	wantPanicEncode = 2
	wantHostWider   = 3 // §4.6's whitespace rows, and only those
	// Rows whose `input` is ONE LINE -- the only rows that can BE a record,
	// because the public section is split on LF (sysw/open.go:67) and a record
	// therefore cannot contain one. Measured from the file, not read off it,
	// and pinned identically in the Rust half (SINGLE_LINE_ROWS /
	// SINGLE_LINE_ADMITTED, crates/me-cli/tests/descriptor_seam.rs).
	wantSingleLine = 59
	// ... of which this many are host_admits, so the derived rule below is
	// satisfiable in BOTH directions rather than vacuously one-sided.
	wantSingleLineAdmitted = 15
	// address_0 is carried on 20 rows; 4 of them the device cannot derive from
	// the INPUT -- the three §4.6 whitespace rows (raw bytes REFUSED; their
	// value is the md1 route's, and the Rust half owns it) and
	// neither/wsh-multi (device refuses `multi` outright). 20 - 4 = 16.
	wantDeviceAddr0 = 16
	// address_1 is carried on 5 rows; only neither/wsh-multi is not
	// device-derivable. 5 - 1 = 4.
	wantDeviceAddr1 = 4
)

func loadSeamVectors(t *testing.T) []seamRow {
	t.Helper()
	raw, err := os.ReadFile(seamVectorsPath)
	if err != nil {
		t.Fatalf("%s: %v", seamVectorsPath, err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != seamVectorsSHA256 {
		t.Fatalf("%s hashes to %s, not the pinned %s -- the vendored copy and the primary "+
			"have drifted, or a row changed without re-pinning BOTH literals",
			seamVectorsPath, hex.EncodeToString(sum[:]), seamVectorsSHA256)
	}
	var doc struct {
		Vectors []seamRow `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", seamVectorsPath, err)
	}
	if len(doc.Vectors) != wantRows {
		t.Fatalf("%s: %d rows, want %d", seamVectorsPath, len(doc.Vectors), wantRows)
	}
	return doc.Vectors
}

// TestDescriptorSeamDeviceColumn asserts device_admits against
// OutputDescriptor -- the SCAN DOOR, on the INPUT. It never feeds a
// `device_probe: "panic:parse"` row to the parser: a panic would CRASH the
// suite rather than fail it, which is a false-signal shape.
func TestDescriptorSeamDeviceColumn(t *testing.T) {
	rows := loadSeamVectors(t)
	var deviceTrue, deviceFalse, absent, panicEncode int
	for _, v := range rows {
		// A mistyped vector must fail loudly, not quietly stop testing.
		sum := sha256.Sum256([]byte(v.Input))
		if got := hex.EncodeToString(sum[:]); got != v.SHA256 {
			t.Errorf("%s: input hashes to %s, declared %s", v.Name, got, v.SHA256)
			continue
		}
		switch v.DeviceProbe {
		// "panic:parse" is NOT an arm any more, and its absence is the
		// assertion. S2's convergence fix made nonstandard/parse.go's
		// fingerprint guard `!= 4`, so the parse panic is gone and the file
		// carries no row that bars this test from OutputDescriptor. A
		// reintroduced "panic:parse" therefore falls to `default` and reds --
		// which is right: the harness rule it needed was retired with it, and
		// silently continuing past such a row would skip the parse it names.
		case "panic:encode":
			panicEncode++
		case "":
		default:
			t.Errorf("%s: unknown device_probe %q", v.Name, v.DeviceProbe)
		}
		if v.DeviceAdmits == nil {
			absent++
			t.Errorf("%s: device_admits is required on a row that is not panic:parse", v.Name)
			continue
		}
		_, err := nonstandard.OutputDescriptor([]byte(v.Input))
		if got := err == nil; got != *v.DeviceAdmits {
			t.Errorf("%s: device admits = %v, want %v (OutputDescriptor err = %v)",
				v.Name, got, *v.DeviceAdmits, err)
		}
		if *v.DeviceAdmits {
			deviceTrue++
		} else {
			deviceFalse++
		}
	}
	if deviceTrue != wantDeviceTrue || deviceFalse != wantDeviceFalse {
		t.Errorf("device column population: %d true / %d false, want %d / %d",
			deviceTrue, deviceFalse, wantDeviceTrue, wantDeviceFalse)
	}
	if absent != 0 {
		t.Errorf("%d rows missing device_admits", absent)
	}
	if panicEncode != wantPanicEncode {
		t.Errorf("device_probe population: %d panic:encode, want %d", panicEncode, wantPanicEncode)
	}
}

// TestDescriptorSeamInvariant is the assertion the file exists for.
//
// host_admits(input) => device_admits(CANONICAL(input)). Stated over the
// canonical string, not the input, and that is deliberate: §4.6's whitespace
// rows are host-wider on the raw bytes by design, and the no-`Derivation:`
// BlueWallet file is device-ACCEPTED as input while its own canonical does not
// re-parse -- an input-level assertion is blind to the one row class the
// invariant exists for.
//
// So for every host_admits=true row: parse `canonical`, require ACCEPT, and
// require the re-encoding of that parse to equal `canonical` -- a fixed point.
func TestDescriptorSeamInvariant(t *testing.T) {
	rows := loadSeamVectors(t)
	var canonical, hostWider int
	for _, v := range rows {
		if !v.HostAdmits {
			if v.Canonical != "" {
				t.Errorf("%s: canonical on a row the host does not admit", v.Name)
			}
			continue
		}
		canonical++
		if v.Canonical == "" {
			t.Errorf("%s: host_admits with no canonical -- the host may be WIDER than "+
				"the device only because `canonical` is what gets packed", v.Name)
			continue
		}
		if v.DeviceAdmits != nil && !*v.DeviceAdmits {
			hostWider++
		}
		d, err := nonstandard.OutputDescriptor([]byte(v.Canonical))
		if err != nil {
			t.Errorf("%s: THE HOST ADMITS WHAT THE DEVICE REFUSES -- `me` would pack "+
				"%q and this parser rejects it: %v. That is an engraved plate for a "+
				"wallet that will not load.", v.Name, v.Canonical, err)
			continue
		}
		if got := d.Encode(); got != v.Canonical {
			t.Errorf("%s: canonical is not a fixed point: re-encodes to %q, want %q",
				v.Name, got, v.Canonical)
		}
	}
	if canonical != wantCanonical {
		t.Errorf("canonical population: %d, want %d", canonical, wantCanonical)
	}
	if hostWider != wantHostWider {
		t.Errorf("host-wider-on-the-input rows: %d, want %d (§4.6's whitespace rows, "+
			"and only those)", hostWider, wantHostWider)
	}
}

// TestDescriptorSeamAddresses derives every carried address_N through
// address.Receive on the parsed INPUT -- the scan door's own string, because
// the no-`Derivation:` row has no canonical -- wherever device_admits is true.
//
// A row whose addresses only the md1 route derives (device_admits=false, e.g.
// `wsh(multi(...))`) is the Rust half's to assert; this half skips it, counted.
func TestDescriptorSeamAddresses(t *testing.T) {
	rows := loadSeamVectors(t)
	var a0, a1 int
	for _, v := range rows {
		if v.DeviceAdmits == nil || !*v.DeviceAdmits {
			continue
		}
		if v.Address0 == "" && v.Address1 == "" {
			continue
		}
		d, err := nonstandard.OutputDescriptor([]byte(v.Input))
		if err != nil {
			t.Errorf("%s: device_admits but OutputDescriptor failed: %v", v.Name, err)
			continue
		}
		for i, want := range []string{v.Address0, v.Address1} {
			if want == "" {
				continue
			}
			got, err := address.Receive(d, uint32(i))
			if err != nil {
				t.Errorf("%s: address.Receive(%d): %v", v.Name, i, err)
				continue
			}
			if got != want {
				t.Errorf("%s: address_%d = %s, want %s", v.Name, i, got, want)
			}
			if i == 0 {
				a0++
			} else {
				a1++
			}
		}
	}
	if a0 != wantDeviceAddr0 || a1 != wantDeviceAddr1 {
		t.Errorf("device-route address population: %d address_0 / %d address_1, want %d / %d",
			a0, a1, wantDeviceAddr0, wantDeviceAddr1)
	}
	// The FILE-level populations, separately: wantDeviceAddr0/1 above count
	// only the rows this route derives, so on their own they cannot see an
	// address column shrinking on the rows it skips.
	var f0, f1 int
	for _, v := range rows {
		if v.Address0 != "" {
			f0++
		}
		if v.Address1 != "" {
			f1++
		}
	}
	if f0 != wantAddress0 || f1 != wantAddress1 {
		t.Errorf("address column population: %d address_0 / %d address_1, want %d / %d",
			f0, f1, wantAddress0, wantAddress1)
	}
}

// TestDescriptorSeamWalletID is the F-212 cross-language gate.
//
// A WalletPolicyId divergence between the two languages is invisible to every
// per-repo test on either side -- 887 of 887 fork tests passed while the two
// implementations disagreed. So both suites compute the id from their OWN
// implementation and compare to the file, never to each other.
//
// Scope, stated rather than ad hoc: MULTISIG rows at the device-default
// use-site. That is this route's measured domain -- md.EncodeMultisig
// hard-codes <0;1>/* (encode_multisig.go:167) and has no single-sig arm -- and
// it is exactly §5.3(a')'s materialised base, which is what makes the id
// identical under both `--as` values.
func TestDescriptorSeamWalletID(t *testing.T) {
	rows := loadSeamVectors(t)
	var n int
	for _, v := range rows {
		if v.WalletID == "" {
			continue
		}
		n++
		if v.DeviceAdmits == nil || !*v.DeviceAdmits {
			t.Errorf("%s: wallet_id on a row this parser does not admit", v.Name)
			continue
		}
		d, err := nonstandard.OutputDescriptor([]byte(v.Input))
		if err != nil {
			t.Errorf("%s: %v", v.Name, err)
			continue
		}
		got, err := walletIDOf(d)
		if err != nil {
			t.Errorf("%s: %v", v.Name, err)
			continue
		}
		if got != v.WalletID {
			t.Errorf("%s: WalletPolicyId = %s, want %s -- the two languages disagree on "+
				"wallet IDENTITY, the F-212 class", v.Name, got, v.WalletID)
		}
	}
	if n != wantWalletID {
		t.Errorf("wallet_id population: %d, want %d", n, wantWalletID)
	}
}

// walletIDOf computes this side's WalletPolicyId over the (a')-materialised
// policy: EncodeMultisig writes the device default <0;1>/* use-site, which is
// exactly the column's stated scope.
func walletIDOf(d *bip380.Descriptor) (string, error) {
	var script md.MultisigScript
	switch d.Script {
	case bip380.P2WSH:
		script = md.MultisigWsh
	case bip380.P2SH_P2WSH:
		script = md.MultisigShWsh
	case bip380.P2SH:
		script = md.MultisigSh
	default:
		return "", fmt.Errorf("script %v has no EncodeMultisig arm", d.Script)
	}
	cos := make([]md.MultisigCosigner, len(d.Keys))
	for i, k := range d.Keys {
		if len(k.ChainCode) != 32 || len(k.KeyData) != 33 {
			return "", fmt.Errorf("key %d: %d-byte chain code, %d-byte key data",
				i, len(k.ChainCode), len(k.KeyData))
		}
		var cc [32]byte
		var pk [33]byte
		var fp [4]byte
		copy(cc[:], k.ChainCode)
		copy(pk[:], k.KeyData)
		fp[0], fp[1] = byte(k.MasterFingerprint>>24), byte(k.MasterFingerprint>>16)
		fp[2], fp[3] = byte(k.MasterFingerprint>>8), byte(k.MasterFingerprint)
		origin := make([]md.PathComponent, 0, len(k.DerivationPath))
		for _, p := range k.DerivationPath {
			if p >= hdkeychain.HardenedKeyStart {
				origin = append(origin, md.PathComponent{Hardened: true, Value: p - hdkeychain.HardenedKeyStart})
			} else {
				origin = append(origin, md.PathComponent{Value: p})
			}
		}
		cos[i] = md.MultisigCosigner{
			ChainCode:        cc,
			CompressedPubkey: pk,
			Fingerprint:      fp,
			FpPresent:        k.MasterFingerprint != 0,
			Origin:           origin,
		}
	}
	strs, _, _, err := md.EncodeMultisig(md.EncodeMultisigRequest{
		Cosigners:  cos,
		K:          uint8(d.Threshold),
		Script:     script,
		OriginMode: md.OriginDivergent,
	})
	if err != nil {
		return "", err
	}
	id, err := md.WalletPolicyIdChunks(strs)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

// ═══ §5.2's PREDICATE AT THE RECORD LAYER ═══════════════════════════════════
//
// `host_admits` IS the predicate -- the Rust half asserts the column against
// `descriptor::host_admits` row by row -- and these two tests assert that
// `sysw.Classify` answers WITH it, in both directions, over every row that can
// be a record. The Rust half runs the SAME derived rule over the SAME file
// (every_single_line_input_classifies_by_the_admission_column and
// every_admitted_rows_canonical_classifies_as_a_descriptor_record,
// crates/me-cli/tests/descriptor_seam.rs), so a Go/Rust divergence anywhere in
// the file reds one of the two suites instead of hiding in a hand-stated value.
//
// THIS REPLACES A FOUR-ROW SAMPLE. Until S2 the file carried a `sysw_class`
// column on 4 of 71 rows and this test SKIPPED rather than assert it, because
// no arm existed to produce the value. The column was a sample the plan misread
// as a population, and its input-vs-canonical basis was ambiguous. The
// derived rule has neither problem: it is exhaustive, it is stated over both
// bases separately, and it needs no column of its own.
//
// The rule is EXACT EQUALITY, not Descriptor-or-anything. That is also the
// empirical, per-row answer to "can a descriptor-shaped string collide with
// another class", which no sampled column could give.

// TestDescriptorSeamSyswClass is the input basis: for every SINGLE-LINE row,
// sysw.Classify(input) == ClassDescriptor iff host_admits, else ClassUnknown.
//
// Single-line because the public section is split on LF (sysw/open.go:67), so a
// record cannot contain one -- the multi-line rows are inputs to `me`, never
// records, and asserting a classification for them would assert something the
// wire cannot carry.
//
// This is the gate that caught the C3 divergence: keying the arm on
// nonstandard.OutputDescriptor -- the scan door -- classifies 18 of these rows
// as Descriptor that `me` refuses, and every one of them would reach a program
// and a screen through gui/sysw_admit.go's live cells. It is never relaxed to
// fit the arm.
func TestDescriptorSeamSyswClass(t *testing.T) {
	rows := loadSeamVectors(t)
	var descriptor, unknown int
	for _, v := range rows {
		if strings.Contains(v.Input, "\n") {
			continue
		}
		want := sysw.ClassUnknown
		if v.HostAdmits {
			want = sysw.ClassDescriptor
		}
		if got := sysw.Classify(v.Input); got != want {
			t.Errorf("%s: sysw.Classify = %v, want %v -- host_admits is %v, and the "+
				"classifier must answer §5.2's predicate EXACTLY. A device that "+
				"classifies what `me` refuses hands a program a wallet the host "+
				"would not pack; one that refuses what `me` packs makes the record "+
				"inert on arrival.", v.Name, got, want, v.HostAdmits)
			continue
		}
		if v.HostAdmits {
			descriptor++
		} else {
			unknown++
		}
	}
	if descriptor+unknown != wantSingleLine {
		t.Errorf("single-line rows: %d, want %d", descriptor+unknown, wantSingleLine)
	}
	if descriptor != wantSingleLineAdmitted {
		t.Errorf("admitted single-line rows: %d, want %d", descriptor, wantSingleLineAdmitted)
	}
	if unknown == 0 {
		t.Error("the REFUSING direction is untested -- every single-line row is admitted")
	}
}

// TestDescriptorSeamSyswClassCanonical is the other basis, asserted separately
// so the input-vs-canonical ambiguity the retired column had cannot come back.
//
// The canonical is always one line and it is EXACTLY what `--as descriptor`
// packs, so a canonical that does not classify is a record `me` writes and this
// device drops on the floor.
func TestDescriptorSeamSyswClassCanonical(t *testing.T) {
	rows := loadSeamVectors(t)
	var n int
	for _, v := range rows {
		if !v.HostAdmits {
			continue
		}
		if v.Canonical == "" {
			t.Errorf("%s: host_admits with no canonical", v.Name)
			continue
		}
		if strings.Contains(v.Canonical, "\n") {
			t.Errorf("%s: canonical is not one line", v.Name)
			continue
		}
		if got := sysw.Classify(v.Canonical); got != sysw.ClassDescriptor {
			t.Errorf("%s: sysw.Classify(canonical) = %v, want ClassDescriptor -- this "+
				"is the record `me sysw pack --as descriptor` writes, and the device "+
				"would leave it inert", v.Name, got)
			continue
		}
		n++
	}
	if n != wantCanonical {
		t.Errorf("canonical assertions run: %d, want %d", n, wantCanonical)
	}
}
