package main

import (
	"testing"

	"seedhammer.com/codex32"
)

// deriveNoPanic runs `biptool derive <cmdline>` and reports a panic as a test
// failure.
func deriveNoPanic(t *testing.T, stdin, cmdline string) (err error, panicked any) {
	t.Helper()
	defer func() { panicked = recover() }()
	_, err = execErr([]byte(stdin), cmdline)
	return err, nil
}

// TestBughuntSh228Bip39ShortSeed: bug-hunt sh2 #28. `derive bip39` slices
// seed[:entLen] without checking len(seed): an xprv with no BIP-85 path has no
// seed at all, and a 16-byte codex32 seed cannot fill 24 words. Both must be
// an error, not a panic.
func TestBughuntSh228Bip39ShortSeed(t *testing.T) {
	const bip32Vector1 = "xprv9s21ZrQH143K3QTDL4LXw2F7HEK3wJUD2nW2nRk4stbPy6cq3jPPqjiChkVvvNKmPGJxWUtg6LnF5kejMRNNU3TGtRBeJgk33yuGBxrMPHi"
	const ms16 = "ms13cashsllhdmn9m42vcsamx24zrxgs3qqjzqud4m0d6nln" // 16-byte master seed (BIP-93 vector 3)
	if s, err := codex32.New(ms16); err != nil || len(s.Seed()) != 16 {
		t.Fatalf("precondition: ms16 seed length: err=%v", err)
	}
	for _, tc := range []struct{ name, stdin, cmdline string }{
		{"xprv, no path, 12 words", bip32Vector1, "-path m bip39 -words 12"},
		{"16-byte codex32 seed, 24 words", ms16, "-path m bip39 -words 24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err, p := deriveNoPanic(t, tc.stdin, "derive "+tc.cmdline)
			if p != nil {
				t.Errorf("derive %s panicked: %v", tc.cmdline, p)
				return
			}
			if err == nil {
				t.Errorf("derive %s succeeded; want an error (seed too short)", tc.cmdline)
			}
		})
	}
}
