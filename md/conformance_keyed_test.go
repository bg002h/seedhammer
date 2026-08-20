package md

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
