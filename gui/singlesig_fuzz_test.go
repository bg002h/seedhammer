package gui

import (
	"testing"

	"seedhammer.com/md"
)

// FuzzSingleSigRestoreDescriptor feeds arbitrary (scriptKind, masterFP, parentFP,
// purpose) into the restore-doc descriptor build + line render and asserts it
// NEVER panics (Task 8). When the descriptor builds, address.Receive/Change must
// be total (no panic) and the encoding must carry no private material.
func FuzzSingleSigRestoreDescriptor(f *testing.F) {
	f.Add(uint8(0), uint32(0x73c5da0a), uint32(0x1234abcd), uint8(84))
	f.Add(uint8(5), uint32(0), uint32(0), uint8(49))
	f.Add(uint8(255), uint32(0xffffffff), uint32(0), uint8(0))

	scripts := []md.ScriptKind{md.ScriptWpkh, md.ScriptPkh, md.ScriptSh, md.ScriptWsh, md.ScriptTr, md.ScriptShWpkh}
	purposes := []int{44, 49, 84, 86, 0, 48}

	f.Fuzz(func(t *testing.T, sIdx uint8, masterFP, parentFP uint32, pIdx uint8) {
		script := scripts[int(sIdx)%len(scripts)]
		purpose := purposes[int(pIdx)%len(purposes)]
		desc, err := singleSigRestoreDescriptor(knownAccountXpub84, masterFP, parentFP, script, singleSigPath(purpose))
		if err != nil {
			// An unsupported script (Sh/Wsh as a single-sig) is a valid rejection —
			// just must not panic.
			return
		}
		// No private material: every serialized key is a PUBLIC extended key
		// (xpub/tpub), NEVER a private one (xprv/tprv). Asserting on the Key string
		// directly avoids a false positive from "xprv" appearing incidentally as a
		// bech32 substring inside the descriptor checksum.
		for _, k := range desc.Keys {
			ks := k.String()
			if hasKeyPrefix(ks, "xprv") || hasKeyPrefix(ks, "tprv") {
				t.Fatalf("restore key leaks private material: %q", ks)
			}
		}
		// The descriptor built → the line render must be total (no panic). It may
		// error for an address-underivable shape; that is fine.
		if _, lerr := singleSigRestoreLines(masterFP, desc); lerr != nil {
			return
		}
	})
}

// hasKeyPrefix reports whether the serialized extended key begins with prefix
// (the version-prefix position) — a real private-key leak, not an incidental
// substring elsewhere.
func hasKeyPrefix(key, prefix string) bool {
	return len(key) >= len(prefix) && key[:len(prefix)] == prefix
}
