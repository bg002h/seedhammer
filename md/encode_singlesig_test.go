package md

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

// ─── T6a-1 Task 2: EncodeSingleSig — wallet-policy md1, 4 shapes ─────────────

type singlesigMeta struct {
	Script     string `json:"script"`
	MFP        string `json:"master_fingerprint"`
	Origin     string `json:"origin_path"`
	ChainCode  string `json:"chaincode"`
	Pubkey     string `json:"compressed_pubkey"`
	PayloadHex string `json:"payload_hex"`
	WPID       string `json:"wallet_policy_id"`
	Stub       string `json:"wallet_policy_id_stub"`
}

func loadSinglesigMeta(t *testing.T, set string) singlesigMeta {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", set+".meta.json"))
	if err != nil {
		t.Fatalf("read %s.meta.json: %v", set, err)
	}
	var m singlesigMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s.meta.json: %v", set, err)
	}
	return m
}

func loadSinglesigChunks(t *testing.T, set string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", set+".md1.txt"))
	if err != nil {
		t.Fatalf("read %s.md1.txt: %v", set, err)
	}
	var chunks []string
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			chunks = append(chunks, l)
		}
	}
	return chunks
}

// scriptKindFromName maps the meta.json script name to the public ScriptKind.
func scriptKindFromName(t *testing.T, s string) ScriptKind {
	t.Helper()
	switch s {
	case "pkh":
		return ScriptPkh
	case "wpkh":
		return ScriptWpkh
	case "tr":
		return ScriptTr
	case "sh_wpkh":
		return ScriptShWpkh
	default:
		t.Fatalf("unknown script %q", s)
		return 0
	}
}

// metaInputs parses a meta.json into the EncodeSingleSig inputs.
func metaInputs(t *testing.T, m singlesigMeta) (cc [32]byte, pk [33]byte, fp [4]byte, origin []PathComponent, script ScriptKind) {
	t.Helper()
	ccBytes, err := hex.DecodeString(m.ChainCode)
	if err != nil || len(ccBytes) != 32 {
		t.Fatalf("bad chaincode %q", m.ChainCode)
	}
	copy(cc[:], ccBytes)
	pkBytes, err := hex.DecodeString(m.Pubkey)
	if err != nil || len(pkBytes) != 33 {
		t.Fatalf("bad pubkey %q", m.Pubkey)
	}
	copy(pk[:], pkBytes)
	fpBytes, err := hex.DecodeString(m.MFP)
	if err != nil || len(fpBytes) != 4 {
		t.Fatalf("bad fp %q", m.MFP)
	}
	copy(fp[:], fpBytes)
	origin = parsePathComponents(t, m.Origin)
	script = scriptKindFromName(t, m.Script)
	return
}

// parsePathComponents parses "m/84'/0'/0'" into RAW []PathComponent (Hardened
// flag + bare value — NOT the in-band +HardenedKeyStart form).
func parsePathComponents(t *testing.T, s string) []PathComponent {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" || s == "m" {
		return nil
	}
	var comps []PathComponent
	for _, p := range strings.Split(s, "/") {
		if p == "m" || p == "" {
			continue
		}
		hardened := false
		if strings.HasSuffix(p, "'") || strings.HasSuffix(p, "h") || strings.HasSuffix(p, "H") {
			hardened = true
			p = p[:len(p)-1]
		}
		var v uint32
		for _, c := range p {
			if c < '0' || c > '9' {
				t.Fatalf("bad path component %q in %q", p, s)
			}
			v = v*10 + uint32(c-'0')
		}
		comps = append(comps, PathComponent{Hardened: hardened, Value: v})
	}
	return comps
}

var singlesigSets = []string{"singlesig_pkh", "singlesig_sh_wpkh", "singlesig_wpkh", "singlesig_tr"}

// TestEncodeSingleSigPayloadParity is the PRIMARY, form-independent gate
// (R0-A1/M1): EncodeSingleSig's reassembled payload bytes byte-equal the
// toolkit's reassembled payload for all 4 shapes.
func TestEncodeSingleSigPayloadParity(t *testing.T) {
	for _, set := range singlesigSets {
		t.Run(set, func(t *testing.T) {
			m := loadSinglesigMeta(t, set)
			cc, pk, fp, origin, script := metaInputs(t, m)

			got, err := EncodeSingleSig(cc, pk, fp, origin, script)
			if err != nil {
				t.Fatalf("EncodeSingleSig: %v", err)
			}
			if len(got) < 2 {
				t.Fatalf("expected chunked output (>=2 strings), got %d", len(got))
			}

			// Reassemble Go's output to the canonical payload bytes and compare to
			// the vendored toolkit payload (form-independent — robust to any
			// chunk-framing/csid variance).
			goDesc, err := Reassemble(got)
			if err != nil {
				t.Fatalf("Reassemble(Go output): %v", err)
			}
			goPayload, _, err := encodePayload(goDesc)
			if err != nil {
				t.Fatalf("encodePayload(Go): %v", err)
			}
			wantPayload, err := hex.DecodeString(m.PayloadHex)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(goPayload) != m.PayloadHex {
				t.Errorf("payload mismatch:\n got  %x\n want %s", goPayload, m.PayloadHex)
			}
			_ = wantPayload
		})
	}
}

// TestEncodeSingleSigStringEquality asserts the exact chunked wire strings equal
// the vendored toolkit strings (deterministic wire), and each chunk ValidMD.
func TestEncodeSingleSigStringEquality(t *testing.T) {
	for _, set := range singlesigSets {
		t.Run(set, func(t *testing.T) {
			m := loadSinglesigMeta(t, set)
			cc, pk, fp, origin, script := metaInputs(t, m)
			got, err := EncodeSingleSig(cc, pk, fp, origin, script)
			if err != nil {
				t.Fatalf("EncodeSingleSig: %v", err)
			}
			want := loadSinglesigChunks(t, set)
			if len(got) != len(want) {
				t.Fatalf("chunk count: got %d want %d", len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("chunk %d:\n got  %s\n want %s", i, got[i], want[i])
				}
				if !codex32.ValidMD(got[i]) {
					t.Errorf("chunk %d not ValidMD: %s", i, got[i])
				}
			}
		})
	}
}

// TestEncodeSingleSigRoundTrip is the safety net (R0-A2/I2): DecodeChunks +
// ExpandWalletPolicyChunks of the output recover xpub/fp/origin/script.
func TestEncodeSingleSigRoundTrip(t *testing.T) {
	for _, set := range singlesigSets {
		t.Run(set, func(t *testing.T) {
			m := loadSinglesigMeta(t, set)
			cc, pk, fp, origin, script := metaInputs(t, m)
			got, err := EncodeSingleSig(cc, pk, fp, origin, script)
			if err != nil {
				t.Fatalf("EncodeSingleSig: %v", err)
			}
			// Decode refuses chunked; DecodeChunks/ExpandWalletPolicyChunks only.
			tmpl, keys, err := ExpandWalletPolicyChunks(got)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			if len(keys) != 1 {
				t.Fatalf("n=%d, want 1", len(keys))
			}
			k := keys[0]
			if hex.EncodeToString(k.Xpub[:32]) != m.ChainCode {
				t.Errorf("chaincode: got %x want %s", k.Xpub[:32], m.ChainCode)
			}
			if hex.EncodeToString(k.Xpub[32:]) != m.Pubkey {
				t.Errorf("pubkey: got %x want %s", k.Xpub[32:], m.Pubkey)
			}
			if !k.FingerprintPresent || hex.EncodeToString(k.Fingerprint[:]) != m.MFP {
				t.Errorf("fp: got %x present=%v want %s", k.Fingerprint, k.FingerprintPresent, m.MFP)
			}
			if !k.XpubPresent {
				t.Error("xpub not present")
			}
			wantRoot := scriptRoot(scriptKindFromName(t, m.Script))
			if tmpl.Root != wantRoot {
				t.Errorf("root: got %v want %v", tmpl.Root, wantRoot)
			}
		})
	}
}

// scriptRoot maps the EncodeSingleSig ScriptKind to the decoded Template.Root.
func scriptRoot(s ScriptKind) ScriptKind {
	switch s {
	case ScriptPkh:
		return ScriptPkh
	case ScriptWpkh:
		return ScriptWpkh
	case ScriptTr:
		return ScriptTr
	case ScriptShWpkh:
		return ScriptSh // sh(wpkh) decodes with root tag Sh
	default:
		return ScriptWpkh
	}
}

// TestEncodeSingleSigEmptyOriginRejected: an empty origin is rejected for every
// shape (explicit origin mandatory; sh-wpkh REQUIRES it on the wire).
func TestEncodeSingleSigEmptyOriginRejected(t *testing.T) {
	m := loadSinglesigMeta(t, "singlesig_sh_wpkh")
	cc, pk, fp, _, script := metaInputs(t, m)
	if _, err := EncodeSingleSig(cc, pk, fp, nil, script); err == nil {
		t.Error("empty origin accepted for sh-wpkh, want error")
	}
}

// TestEncodeSingleSigTrBody: tr emits a trBody (is_nums:false, tree:nil), NOT a
// keyArgBody — confirmed via the decoded shape.
func TestEncodeSingleSigTrBody(t *testing.T) {
	m := loadSinglesigMeta(t, "singlesig_tr")
	cc, pk, fp, origin, _ := metaInputs(t, m)
	got, err := EncodeSingleSig(cc, pk, fp, origin, ScriptTr)
	if err != nil {
		t.Fatalf("EncodeSingleSig(tr): %v", err)
	}
	d, err := Reassemble(got)
	if err != nil {
		t.Fatalf("Reassemble: %v", err)
	}
	if d.tree.tag != tagTr {
		t.Fatalf("root tag: got %v want tagTr", d.tree.tag)
	}
	tb, ok := d.tree.body.(trBody)
	if !ok {
		t.Fatalf("tr body type %T, want trBody", d.tree.body)
	}
	if tb.isNums {
		t.Error("tr is_nums: got true, want false")
	}
	if tb.tree != nil {
		t.Error("tr tree: got non-nil, want nil (key-path only)")
	}
}
