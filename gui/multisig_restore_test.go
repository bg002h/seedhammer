package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/md"
)

// TestMultisigRestoreLines: a sortedmulti supplied md1 -> expandedToDescriptor
// -> address.Receive/Change show the multisig addresses (golden, I-6); a
// non-bip380 shape -> nil descriptor -> display-only "addresses unavailable"
// with NO address. No xprv ever appears.
func TestMultisigRestoreLines(t *testing.T) {
	t.Run("sortedmulti -> addresses (golden)", func(t *testing.T) {
		chunks := suppliedMultisigMd1(t)
		tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("ExpandWalletPolicyChunks: %v", err)
		}
		lines, hasAddr, err := multisigRestoreLines(tpl, keys)
		if err != nil {
			t.Fatalf("multisigRestoreLines: %v", err)
		}
		if !hasAddr {
			t.Fatal("sortedmulti should yield addresses (expandOK)")
		}
		blob := strings.Join(lines, "\n")
		const wantRecv = "bc1qg2lsdla23zewexuhn5jcx49mqzs8wqss0lxguarfpnt7ysg7k52slz4dxd"
		const wantChange = "bc1qz76qjcmpwhh6ffenfwg44hpq3cwwfuqcr54vl4485yttpjtxy9qq3yufkt"
		if !strings.Contains(blob, wantRecv) {
			t.Fatalf("receive address %s missing from:\n%s", wantRecv, blob)
		}
		if !strings.Contains(blob, wantChange) {
			t.Fatalf("change address %s missing from:\n%s", wantChange, blob)
		}
		if strings.Contains(blob, "xprv") {
			t.Fatal("xprv leaked into the restore doc")
		}
	})

	t.Run("template-only -> display-only, no address", func(t *testing.T) {
		// A multisig template with NO xpubs -> expandTemplateOnly -> nil desc.
		tpl := md.Template{Root: md.ScriptWsh, Policy: md.PolicySortedMulti, K: 2, N: 2, Renderable: true}
		keys := []md.ExpandedKey{
			{Index: 0, OriginPath: msPath(hard32+48, hard32+0, hard32+0, hard32+2), XpubPresent: false},
			{Index: 1, OriginPath: msPath(hard32+48, hard32+0, hard32+0, hard32+2), XpubPresent: false},
		}
		lines, hasAddr, err := multisigRestoreLines(tpl, keys)
		if err != nil {
			t.Fatalf("multisigRestoreLines(template-only): %v", err)
		}
		if hasAddr {
			t.Fatal("template-only must NOT yield addresses")
		}
		blob := strings.Join(lines, "\n")
		if !strings.Contains(blob, "unavailable") {
			t.Fatalf("display-only path must note addresses unavailable:\n%s", blob)
		}
		if strings.HasPrefix(blob, "bc1") || strings.Contains(blob, "\nbc1") {
			t.Fatalf("an address appeared on a display-only path:\n%s", blob)
		}
	})

	_ = hdkeychain.HardenedKeyStart // keep the import if unused above
}
