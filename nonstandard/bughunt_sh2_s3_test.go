package nonstandard

import (
	"fmt"
	"reflect"
	"testing"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).
// Keys are the BlueWallet fixture keys of TestDescriptors.

const (
	bhBWKey1 = "5A0804E3: xpub6F148LnjUhGrHfEN6Pa8VkwF8L6FJqYALxAkuHfacfVhMLVY4MRuUVMxr9pguAv67DHx1YFxqoKN8s4QfZtD9sR2xRCffTqi9E8FiFLAYk8"
	bhBWKey2 = "DD4FADEE: xpub6DnediUuY8Pcc6Fej8Yt2ZntPCyFdpbHBkNV7EawesRMbc6i9MKKMhKEv4JMMzwDJckaV4czBvNdc6ikwLiZqdUqMd5ZKQGYaQT4cXMeVjf"
	bhBWKey3 = "9BACD5C0: xpub6EefrCrMAduhNwnsHb3dAs8DYZSw4f63WyR6DaEByUHjwvPDdhczj15FyBBG4tbEJtf4vRKTv1ng5SPPnWv1Pve1f15EJfiBY5oYDN6VLEC"
)

// Bug 6b: the BlueWallet text parser accepts "Policy: 0 of 2", "3 of 2" and
// "-1 of 2".
func TestBughuntSh206bBlueWalletThreshold(t *testing.T) {
	file := func(policy string) []byte {
		return []byte(fmt.Sprintf("Name: t\nPolicy: %s\nDerivation: m/48'/0'/0'/2'\nFormat: P2WSH\n\n%s\n\n%s\n", policy, bhBWKey1, bhBWKey2))
	}
	// Positive control: a sane policy parses.
	if _, err := OutputDescriptor(file("2 of 2")); err != nil {
		t.Fatalf("control 2 of 2 refused: %v", err)
	}
	for _, p := range []string{"0 of 2", "3 of 2", "-1 of 2"} {
		d, err := OutputDescriptor(file(p))
		if err == nil {
			t.Errorf("Policy %q accepted: Threshold=%d, %d keys", p, d.Threshold, len(d.Keys))
		}
	}
}

// Bug 50a: a file with only "Name: x" is accepted as a descriptor with 0 keys.
func TestBughuntSh250aBlueWalletNameOnly(t *testing.T) {
	d, err := OutputDescriptor([]byte("Name: x\n"))
	if err == nil {
		t.Errorf("Name-only file accepted: Title=%q Threshold=%d Script=%v keys=%d", d.Title, d.Threshold, d.Script, len(d.Keys))
	}
}

// Bug 50b: a file without a "Format:" line is accepted with an unknown script.
func TestBughuntSh250bBlueWalletNoFormat(t *testing.T) {
	full := fmt.Sprintf("Name: t\nPolicy: 2 of 2\nDerivation: m/48'/0'/0'/2'\nFormat: P2WSH\n\n%s\n\n%s\n", bhBWKey1, bhBWKey2)
	// Positive control: with the Format line the file parses with a known script.
	c, err := OutputDescriptor([]byte(full))
	if err != nil {
		t.Fatalf("control refused: %v", err)
	}
	if c.Script.Singlesig() || c.Script == 0 {
		t.Fatalf("control has script %v", c.Script)
	}
	noFormat := fmt.Sprintf("Name: t\nPolicy: 2 of 2\nDerivation: m/48'/0'/0'/2'\n\n%s\n\n%s\n", bhBWKey1, bhBWKey2)
	d, err := OutputDescriptor([]byte(noFormat))
	if err == nil {
		t.Errorf("file without Format accepted: Script=%v (unknown=%v), %d keys", d.Script, d.Script == 0, len(d.Keys))
	}
}

// Bug 50c: a key line that comes before the "Derivation:" header gets an empty
// origin path. The correct result is a refusal, or the same path as when the
// Derivation line comes first.
func TestBughuntSh250cBlueWalletKeyBeforeDerivation(t *testing.T) {
	derivFirst := fmt.Sprintf("Name: t\nPolicy: 2 of 2\nDerivation: m/48'/0'/0'/2'\nFormat: P2WSH\n\n%s\n\n%s\n", bhBWKey1, bhBWKey2)
	derivLast := fmt.Sprintf("Name: t\nPolicy: 2 of 2\nFormat: P2WSH\n\n%s\n\n%s\n\nDerivation: m/48'/0'/0'/2'\n", bhBWKey1, bhBWKey2)
	want, err := OutputDescriptor([]byte(derivFirst))
	if err != nil {
		t.Fatalf("control (Derivation first) refused: %v", err)
	}
	if len(want.Keys) != 2 || len(want.Keys[0].DerivationPath) == 0 {
		t.Fatalf("control has no origin path: %+v", want.Keys)
	}
	got, err := OutputDescriptor([]byte(derivLast))
	if err != nil {
		return // refusing is a correct outcome
	}
	if len(got.Keys) != len(want.Keys) {
		t.Fatalf("got %d keys, want %d", len(got.Keys), len(want.Keys))
	}
	for i := range want.Keys {
		if !reflect.DeepEqual(got.Keys[i].DerivationPath, want.Keys[i].DerivationPath) {
			t.Errorf("key %d: origin path %v with Derivation after the key lines, want %v (or a refusal)", i, got.Keys[i].DerivationPath, want.Keys[i].DerivationPath)
		}
	}
}
