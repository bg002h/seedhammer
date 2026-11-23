# FIX: bip39.MnemonicSeed derives from the NFKD form of the passphrase

**Small track** (one Opus review round). Grade I, class other (seed-derivation correctness; no device caller can pass
a non-ASCII passphrase today, so not the funds path by device policy). Secret row
`SECRET id=bh1119-sh-i13-passphrase-no-nfkd` (coordinator `status/secret-criticals.md`); its grade-M duplicate
`bh-sh-i13` (same defect, superseded) closes with it. Source: `design/agent-reports/bug-hunt-2026-11-19.md` I13
(L114-118). Owner FOLLOWUP: mnemonic-engrave `design/FOLLOWUPS.md` `bip39-passphrase-nfkd-normalization` (L120).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 127cf04. Anchors are at that base.

## 1. The defect

`bip39/bip39.go:261-270` `MnemonicSeed` builds the PBKDF2 salt as `[]byte("mnemonic"+password)` (L269) with no
normalization. BIP-39 ("From mnemonic to seed") requires the passphrase to be NFKD-normalized first. Seen on base (§6):
on the mnemonic `abandon` x11 + `about`, `"café"` and `"café"` derive two different seeds (SHA-256
`19fcc124...` and `ab8cc794...`, the hunt's two hashes); only the second is the BIP-39 seed. Library callers that pass a
passphrase through: `bip32/master.go:27`, `gui/derive.go:20`, `gui/bip85.go:76`. The device's own entry refuses non-ASCII
(`passphrase/passphrase.go` `ErrNonASCII`), so the device does not reach the defect today; a library caller does.

The mnemonic sentence needs no normalization: every word comes from `LabelFor` over the English list, which is ASCII
(`LC_ALL=C grep -c '[^ -~]' bip39/wordlist.txt` prints `0`).

**Rust-primary check** (required by the owner FOLLOWUP): the Rust primary uses the `bip39` crate 2.2.x, whose
`Mnemonic::to_seed` NFKD-normalizes the passphrase (`bip39-2.2.2/src/lib.rs:562-569`; feature `alloc` implies
`unicode-normalization`). Rust already conforms, so this is a Go porting error and is fixed in Go directly.

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (NFKD).** `MnemonicSeed(m, password)` uses the salt `"mnemonic"` followed by the NFKD form of `password`
  (`golang.org/x/text/unicode/norm`, via the helper `appendNFKD`, §4.2). Equivalent spellings (canonical: U+00E9 vs `e`+U+0301; compatibility:
  U+FB01 vs `fi`) derive one seed, equal to an independent NFKD computation. This includes runs of more than 30
  non-starters: x/text's Stream-Safe U+034F insertion is not part of NFKD and is not applied (`appendNFKD`). Unicode
  version: x/text v0.37.0 `norm.Version` is 15.0.0 (the Rust primary's `unicode-normalization` 0.1.25 is 17.0); a
  character assigned after 15.0 passes through unnormalized, so a passphrase using one can derive a different seed from
  an implementation on a newer Unicode. BIP-39 pins no version and this cannot be closed here.
- **R2 (ASCII unchanged).** A passphrase whose bytes are all below 0x80 is its own NFKD form: the salt is the exact
  bytes `"mnemonic"+password`, as at base, without calling `norm` (the ASCII fast path). Every published ASCII vector
  (empty, `TREZOR`) keeps its seed.
- **R3 (invalid UTF-8).** Bytes that are not valid UTF-8 pass through unchanged; valid text around them is still
  normalized (the `norm` package's documented behaviour, measured in §6). No error, no panic: `MnemonicSeed` keeps its
  signature. BIP-39 does not define a seed for non-UTF-8 input, and no device caller can produce one.
- **R4 (unchanged elsewhere).** Signature, return value length (64), iteration count and the sentence are unchanged.
  No holder-visible text, flag, exit code or device behaviour changes: the device still refuses non-ASCII with the
  existing `ErrNonASCII` text (the finding says the device keyboard does not change).

## 3. Holder-visible changes (complete list)

None. No printed text, refusal, exit code or device screen changes. Library behaviour changes only for non-ASCII
passphrases (R1, R3). No toolkit-manual lockstep (`biptool` and the device take no non-ASCII passphrase; the
toolkit manual `40-cli-reference/` documents no seedhammer passphrase behaviour).

## 4. Code changes (builder applies)

The scratch version is `design/FIX_sec_sh_passphrase_nfkd-probe.diff` (evidence, applied and reverted by the author).

1. `bip39/bip39.go` import block (L6-20): add `"golang.org/x/text/unicode/norm"` after `"golang.org/x/crypto/pbkdf2"` (L19); add `"unicode/utf8"` after `"strings"` (`"sort"` is already imported).
2. `bip39/bip39.go:269`: the salt argument becomes a call to a new unexported helper (probe name `seedSalt(password
   string) []byte`, placed after `MnemonicSeed`). The helper allocates one buffer with capacity
   `len("mnemonic")+len(password)` holding `"mnemonic"`; if any byte of `password` is >= 0x80 it returns
   `appendNFKD(buf, password)`, else `append(buf, password...)`. Doc comment (exact): `seedSalt returns
   "mnemonic" followed by the NFKD form of password, as BIP-39 requires. ASCII is its own NFKD form and is copied
   unchanged. Invalid UTF-8 bytes pass through unchanged. No stream-safe U+034F is inserted (see appendNFKD).`
   `appendNFKD` is a new unexported helper in `bip39/bip39.go` after `seedSalt` (imports `sort`, `unicode/utf8`,
   `golang.org/x/text/unicode/norm`); the body is in the probe diff:
   ```go
   // appendNFKD appends the NFKD form of s to dst without the Stream-Safe Text
   // Format (UAX #15 s.13) U+034F insertion that norm applies after 30
   // non-starters: each rune is decomposed alone (no single decomposition reaches
   // 30 non-starters), then each run of non-starters is stably sorted by CCC.
   // Invalid UTF-8 bytes pass through unchanged as starters.
   func appendNFKD(dst []byte, s string) []byte {
   	start := len(dst) // start of the current run of non-starters
   	for i := 0; i < len(s); {
   		r, n := utf8.DecodeRuneInString(s[i:])
   		if r == utf8.RuneError && n == 1 {
   			dst = append(dst, s[i])
   			i++
   			start = len(dst)
   			continue
   		}
   		for _, d := range norm.NFKD.String(s[i : i+n]) {
   			if norm.NFD.PropertiesString(string(d)).CCC() == 0 {
   				dst = utf8.AppendRune(dst, d)
   				start = len(dst)
   				continue
   			}
   			dst = utf8.AppendRune(dst, d)
   			run := []rune(string(dst[start:]))
   			sort.SliceStable(run, func(a, b int) bool {
   				return norm.NFD.PropertiesString(string(run[a])).CCC() < norm.NFD.PropertiesString(string(run[b])).CCC()
   			})
   			dst = append(dst[:start], string(run)...)
   		}
   		i += n
   	}
   	return dst
   }
   ```
3. `go.mod`: move `golang.org/x/text v0.37.0` from the indirect block (L27, drop `// indirect`) to the direct block
   after `golang.org/x/image v0.41.0` (L16). No `go.sum` change (`go mod tidy -diff` exit 0 on the probe).
4. `passphrase/passphrase.go` L3-7, the package doc's second paragraph, becomes (exact):

   `// The device accepts only printable ASCII. That is NOT a BIP-39 rule -- the`
   `// standard permits any string -- it is this device's engraving limit`
   `// (ErrNonASCII). bip39.MnemonicSeed applies BIP-39's NFKD normalization, so a`
   `// non-ASCII passphrase from a library caller derives the conformant seed.`
   `// SPEC_seedhammer_engrave_bip39_password.md D3 records the earlier reason (no`
   `// NFKD), removed by FIX sec-sh-passphrase-nfkd.`

Secret residue: the salt and `sentence` buffers are not wiped, as at base; that class is owned by the open M row
`sweep1122-sh-seed-residue-unwiped-class` and is out of scope. The ASCII path makes exactly one salt buffer (no
growth); the NFKD path may grow it inside `appendNFKD` (only reachable by a non-ASCII library caller).

## 5. Tests

New file `bip39/fix_sec_sh_passphrase_nfkd_test.go` (in the probe diff; test function names are the TRACE row names).
All rows use the mnemonic `abandon` x11 + `about` and compare `hex(MnemonicSeed(m, p))` to a 64-byte seed computed
independently with Python 3 `hashlib.pbkdf2_hmac("sha512", sentence, b"mnemonic"+unicodedata.normalize("NFKD", p).encode(), 2048, 64)`
(raw bytes for the invalid-UTF-8 rows). Test inputs only, no real secret.

- `TestFixSecShPassphraseNfkdComposedEqualsDecomposed` (R1): `"café"` and `"café"` both ->
  `af8bbd2566df7b69...ff7ae9818`.
- `TestFixSecShPassphraseNfkdCompatibility` (R1, K not D): `"ﬁ"` and `"fi"` both -> `6f8c92074395505f...a56f17d8`.
- `TestFixSecShPassphraseNfkdAsciiUnchanged` (R2, control, passes at base): `""` -> `5eb00bbddcf06908...ce9e38e4`;
  `"TREZOR"` -> `c55257c360c07c72...e7463b04` (the BIP-39 published vector).
- `TestFixSecShPassphraseNfkdInvalidUTF8` (R3): `"caf\xe9"` -> seed of the raw salt `mnemoniccaf\xe9`
  (`439aad27d41563be...918ff82c`); `"\xffcafé"` -> seed of `mnemonic\xffcafé` (`f3d30b72b6e7a2c0...5fa43115`).

Full 128-hex values are in the probe test file.

- `TestFixSecShPassphraseNfkdLongCombiningRun` (R1, no stream-safe; test file imports `strings`):
  `"a"+strings.Repeat("\u0301", 31)` ->
  `5b60cf53d7fd9730cee8e18f0dcf8fbc01e53d9cabfb3ae4aaf9bdd0ac9e58ba4d861773b6065c7ba30ea70157f3f8cee9e73229b7cc4a307e1ce19385a3cb6b`;
  `"a"+strings.Repeat("\u0301", 30)+"\u0316"` ->
  `1dfb7d57cac09826e0111bb912a7e9ae1e0529aa12d631ea797c57778729c5f75cbe87712e560ee2681119c44987288ebea09b874b3d90c5b615a94d7114d8b0`.
  Expected values come from Python NFKD. **Tests that change:** none. Existing seed tests (`bip32/master_test.go`,
`gui/derive_test.go`, `gui/master_key_residue_test.go`, `gui/gui_test.go`, `backup/`) use ASCII passphrases and pass
unchanged (§8).

## 6. RED rows (run on base 127cf04 with the §5 test file added)

Command: `CGO_ENABLED=0 go test -count=1 -v -run 'TestFixSecShPassphraseNfkd' ./bip39/`

| row | at base (seen) | after the fix (seen on the probe) |
|---|---|---|
| TestFixSecShPassphraseNfkdComposedEqualsDecomposed | FAIL: precomposed `MnemonicSeed("café") = 3f71749629d237c8...` (decomposed matches) | PASS |
| TestFixSecShPassphraseNfkdCompatibility | FAIL: ligature `MnemonicSeed("ﬁ") = e5b3ee632ee9f793...` (`fi` matches) | PASS |
| TestFixSecShPassphraseNfkdAsciiUnchanged | PASS (control) | PASS |
| TestFixSecShPassphraseNfkdInvalidUTF8 | FAIL: mixed `MnemonicSeed("\xffcafé") = ff9373f48020e609...` (latin1 matches) | PASS |
| TestFixSecShPassphraseNfkdLongCombiningRun | FAIL at base, and FAIL with plain `norm.NFKD.AppendString` (U+034F inserted; seed differs from Python) | PASS with `appendNFKD` |

Mutations seen on the probe: `norm.NFD` instead of `norm.NFKD` fails Compatibility; `norm.NFKC` fails
ComposedEqualsDecomposed and InvalidUTF8.

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 NFKD, canonical and compatibility, no stream-safe | TestFixSecShPassphraseNfkdComposedEqualsDecomposed; TestFixSecShPassphraseNfkdCompatibility; TestFixSecShPassphraseNfkdLongCombiningRun |
| R2 ASCII unchanged | TestFixSecShPassphraseNfkdAsciiUnchanged; existing TREZOR seed tests in bip32 and gui |
| R3 invalid UTF-8 passes through | TestFixSecShPassphraseNfkdInvalidUTF8 |
| R4 unchanged elsewhere | existing `./...` suite; device builds (§8) |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`);
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`; `./scripts/test-32bit.sh`;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and with `-tags refugium`; `./scripts/refugium-tagged-test.sh`; both TinyGo
device builds (`.github/workflows/test.yml` L150, L153) and `./scripts/refugium-elf-check.sh default.elf refugium.elf`.

Measured on the probe by the author (go1.26.7, /scratch/code/shibboleth/.toolchain/go): `go test` of `./bip39/
./bip32/ ./passphrase/ ./backup/ ./bip85/ ./cmd/biptool/` ok; `./gui/` with `-run 'Derive|MasterKey|Fingerprint|Bip85|BIP85|Passphrase|Xpub'`
ok; `GOARCH=386 go test ./bip39/` ok; `go vet ./bip39/ ./passphrase/` ok; gofmt clean; `go mod tidy -diff` exit 0.
Not run by the author: the rest of `./...` and the other jobs. The builder runs the whole gate.

**Device size (for the reviewer and architect).** TinyGo pico-plus2 default build with the CI flags, `-size short`:
base 127cf04 flash 1693432 / static RAM 63376; probe flash 1749568 / RAM 82824: **+56136 B flash, +19448 B static
RAM** (data 32228 -> 51660; the `norm` tables are package variables, which TinyGo places in `.data`). The ASCII fast
path does not reduce this (the package is linked either way). `-print-stacks` reports every goroutine as recursive
at base and probe alike (no stack figure to compare). CI has no size gate; the recorded budget (mnemonic-engrave
`design/STAGED_PLAN_tr_wsh_concrete_descriptors.md` L23: 62 KB = 11.8% static RAM) puts the cost near +3.7% of
static RAM. The figures were measured on the plain-`norm` probe, before `appendNFKD` (r1 I1); the helper adds `sort` and
`unicode/utf8` use and is not re-measured. **Architect to confirm this budget before the build** (r1 M2). The FIX takes the cost so the library is BIP-39-conformant on every target; the alternative (keep `norm`
out of the TinyGo build behind a build tag, with a different non-ASCII behaviour on device) is a new edge-case
decision and is not proposed.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none (§3). README/docs: `git grep -n -i nfkd -- ':!design/agent-reports'` at base names only
  `passphrase/passphrase.go:6` (changed in §4.4).
- mnemonic-engrave `design/SPEC_seedhammer_engrave_bip39_password.md` L126-137 and L1020 describe the pre-fix state;
  a historical spec, not edited here (the new passphrase doc points at it as the earlier reason).
- Closes on merge: secret rows `bh1119-sh-i13-passphrase-no-nfkd` and `bh-sh-i13`; the mnemonic-engrave FOLLOWUP
  `bip39-passphrase-nfkd-normalization` (closing line in the author report, appended after merge by direct push).

## 10. Evidence

RED and GREEN rows run by the author as in §6. Probe diff `design/FIX_sec_sh_passphrase_nfkd-probe.diff` sha256
0b68d3bc3e28e527b613fecea56ab296c93fb6354434caab4e03c08ad497e037; it was applied for the GREEN and size runs and
reverted. Seed values in §5/§6 are captured from the runs and the Python computation, not written by hand.
