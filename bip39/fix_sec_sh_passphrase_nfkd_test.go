package bip39

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// FIX sec-sh-passphrase-nfkd (secret row bh1119-sh-i13-passphrase-no-nfkd):
// BIP-39 derives the seed from the NFKD form of the passphrase. Every expected
// seed below was computed independently with Python 3 (hashlib.pbkdf2_hmac,
// unicodedata.normalize("NFKD", p)) on the mnemonic "abandon" x11 + "about".
// Test inputs only; no real secret.

func nfkdAbandonAbout(t *testing.T) Mnemonic {
	t.Helper()
	m, err := ParseMnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func checkNfkdSeed(t *testing.T, m Mnemonic, name, passphrase, want string) {
	t.Helper()
	if got := hex.EncodeToString(MnemonicSeed(m, passphrase)); got != want {
		t.Errorf("%s: MnemonicSeed(%+q) = %s, want %s", name, passphrase, got, want)
	}
}

// "café" spelt precomposed (U+00E9) and decomposed (e + U+0301) is one
// passphrase under NFKD and derives one seed.
func TestFixSecShPassphraseNfkdComposedEqualsDecomposed(t *testing.T) {
	m := nfkdAbandonAbout(t)
	const want = "af8bbd2566df7b69d926f2b09dfdbd75db6c994a3399b2cc65f928d63e3fd4e61218ee0d15f8c810be4d45e66d47b43c15a5cc753976b1666912377ff7ae9818"
	checkNfkdSeed(t, m, "precomposed", "café", want)
	checkNfkdSeed(t, m, "decomposed", "café", want)
}

// NFKD, not NFD: the compatibility ligature U+FB01 derives the seed of "fi".
func TestFixSecShPassphraseNfkdCompatibility(t *testing.T) {
	m := nfkdAbandonAbout(t)
	const want = "6f8c92074395505f20fb58dee916a71167cca6b574bcfb870fbbd086931f51a7e1d812948405326fbe13783e989776629a0eb5c4e28468991c79a2e4a56f17d8"
	checkNfkdSeed(t, m, "ligature", "ﬁ", want)
	checkNfkdSeed(t, m, "ascii", "fi", want)
}

// ASCII is its own NFKD form: the published vectors keep their seeds.
func TestFixSecShPassphraseNfkdAsciiUnchanged(t *testing.T) {
	m := nfkdAbandonAbout(t)
	checkNfkdSeed(t, m, "empty", "", "5eb00bbddcf069084889a8ab9155568165f5c453ccb85e70811aaed6f6da5fc19a5ac40b389cd370d086206dec8aa6c43daea6690f20ad3d8d48b2d2ce9e38e4")
	checkNfkdSeed(t, m, "TREZOR", "TREZOR", "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04")
}

// Invalid UTF-8 bytes pass through unchanged; the valid text around them is
// still normalized.
func TestFixSecShPassphraseNfkdInvalidUTF8(t *testing.T) {
	m := nfkdAbandonAbout(t)
	checkNfkdSeed(t, m, "latin1", "caf\xe9", "439aad27d41563beae6cd52b6a2b64ef734193b39b796c4a2620e81f7ad668859ca09ba2f1ddfb521d696dec584d718ea4c046153e557a7f3c6c5e44918ff82c")
	checkNfkdSeed(t, m, "mixed", "\xffcafé", "f3d30b72b6e7a2c0b44e64bd343ce9afa5c8c88210f92be78cd3da65be6f312b1213aa33ab34859e3d8d8afb092ecd6a6d6a016bc01305d0552321ae5fa43115")
}

// x/text's norm applies the Stream-Safe Text Format (U+034F after 30
// non-starters), which is not part of NFKD; the Rust primary and Python do not.
func TestFixSecShPassphraseNfkdLongCombiningRun(t *testing.T) {
	m := nfkdAbandonAbout(t)
	checkNfkdSeed(t, m, "31 acutes", "a"+strings.Repeat("\u0301", 31), "5b60cf53d7fd9730cee8e18f0dcf8fbc01e53d9cabfb3ae4aaf9bdd0ac9e58ba4d861773b6065c7ba30ea70157f3f8cee9e73229b7cc4a307e1ce19385a3cb6b")
	checkNfkdSeed(t, m, "30 acutes + grave-below", "a"+strings.Repeat("\u0301", 30)+"\u0316", "1dfb7d57cac09826e0111bb912a7e9ae1e0529aa12d631ea797c57778729c5f75cbe87712e560ee2681119c44987288ebea09b874b3d90c5b615a94d7114d8b0")
}

// A long run of non-starters (hostile or Zalgo-style input to a library
// caller) is normalized in linear-ish time. 25000 pairs of U+0316, U+0301 (50000
// marks, CCC 220 then 230, so the run needs sorting) took minutes when the helper
// re-sorted the whole run after every mark (r2 I1); the correct helper takes
// milliseconds. The 20 s bound is far above that and far below the old cost.
func TestFixSecShPassphraseNfkdLongRunFast(t *testing.T) {
	m := nfkdAbandonAbout(t)
	const want = "05f77e030fba075675aede1f4363aca589ba7f514390964c72b3e97e313375d08e4fe779b5c2149a6a4342d789a0a2ed8eed92ee6ab8e4531f7777d2e3f3ad01"
	p := "a" + strings.Repeat("̖́", 25000)
	done := make(chan string, 1)
	start := time.Now()
	go func() { done <- hex.EncodeToString(MnemonicSeed(m, p)) }()
	select {
	case got := <-done:
		t.Logf("50000 marks took %v", time.Since(start))
		if got != want {
			t.Errorf("long run: MnemonicSeed = %s, want %s", got, want)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("long run of 50000 marks did not finish within 20s (quadratic appendNFKD)")
	}
}
