package nonstandard_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
	"seedhammer.com/nonstandard"
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
const seamVectorsSHA256 = "542cd492e35149b62c53f940fb755576e0ffd4d086b0e3fcda615fbc43f51974"

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
	SyswClass            string   `json:"sysw_class"`
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
	wantRows         = 71
	wantDeviceTrue   = 37
	wantDeviceFalse  = 33
	wantDeviceAbsent = 1 // the panic:parse row
	wantCanonical    = 19
	wantAddress0     = 20
	wantAddress1     = 5
	wantWalletID     = 4
	wantSyswClass    = 4
	wantPanicParse   = 1
	wantPanicEncode  = 2
	wantHostWider    = 3 // §4.6's whitespace rows, and only those
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
	var deviceTrue, deviceFalse, absent, panicParse, panicEncode int
	for _, v := range rows {
		// A mistyped vector must fail loudly, not quietly stop testing.
		sum := sha256.Sum256([]byte(v.Input))
		if got := hex.EncodeToString(sum[:]); got != v.SHA256 {
			t.Errorf("%s: input hashes to %s, declared %s", v.Name, got, v.SHA256)
			continue
		}
		switch v.DeviceProbe {
		case "panic:parse":
			panicParse++
			if v.DeviceAdmits != nil {
				t.Errorf("%s: a panic:parse row must OMIT device_admits -- the predicate "+
					"cannot be evaluated, so either boolean is a false claim", v.Name)
			}
			// Deliberately NOT parsed. nonstandard/parse.go:136-149 checks only
			// len(fp) > 4 before binary.BigEndian.Uint32(fp[:]).
			continue
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
	if panicParse != wantPanicParse || panicEncode != wantPanicEncode {
		t.Errorf("device_probe population: %d panic:parse / %d panic:encode, want %d / %d",
			panicParse, panicEncode, wantPanicParse, wantPanicEncode)
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

// TestDescriptorSeamSyswClass is S2's arm. `sysw.Classify` has no descriptor
// case today (sysw/classify.go:34 -- measured, ClassUnknown for all 39 probed
// descriptor inputs), so the column states what it will answer once §5.2's arm
// lands, and this test SKIPS with that reason rather than asserting a value
// nothing can produce. It still counts the column, so the rows cannot vanish
// while the assertion is parked.
func TestDescriptorSeamSyswClass(t *testing.T) {
	rows := loadSeamVectors(t)
	var n int
	for _, v := range rows {
		if v.SyswClass != "" {
			n++
		}
	}
	if n != wantSyswClass {
		t.Errorf("sysw_class population: %d, want %d", n, wantSyswClass)
	}
	t.Skipf("S2 (F-418): sysw.Classify has no descriptor arm yet, so the %d sysw_class "+
		"rows cannot be asserted. Un-skip when §5.2's arm lands -- importing sysw here "+
		"is why this file is package nonstandard_test.", n)
}
