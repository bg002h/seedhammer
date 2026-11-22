# FIX: a hardened use-site step below an xpub is refused, at the deriver and at the NFC scan door

**Class B (funds path). Full track** (up to three review rounds, no separate plan). Critical.
Keys: bughunt-1119 `bh-sh-C2` and sweep-1122 row 20 (`device-hardened-silently-unhardened`), one defect.
Rulings (decided, not re-ruled here): T4 (coordinator `agent-reports/architect-bughunt-1119-questions.md` L38) and
EN-Q7 (`agent-reports/architect-sweep-1122-questions.md` L152-154).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 9aec7a87ac920697a59990d4784c0da15fa8fbf0.
All line anchors below are at that base.

## 1. The defect

BIP-380 (bitcoin/bips master, checked against the upstream text) L82: a key may be "followed by a single `/*` or `/*h`
final step to denote all direct unhardened or hardened children"; L228-229 list `xpub.../3h/4h/5h/*` and
`xpub.../3h/4h/5h/*h` as valid expressions. A hardened child of an extended PUBLIC key cannot be derived (BIP-32 CKDpub has
no hardened case); the descriptor is valid and its addresses exist, but only the xprv holder can compute them.

`address.derivePubKey` (address/address.go:289-317) never reads `Derivation.Hardened`. For `wpkh(K/0h/*)` it derives
`K/0/i` and returns that address with a nil error. `address.Supported` (address.go:28-31) is true, so the descriptor
screen opens "Show addresses" and "Verify an address" (gui/gui.go:3295, 3316-3327); `address.Find` answers
"found" for the `/0/*` address, which the verify flow reports as controlled by this descriptor. Funds sent there
are spendable by the xprv holder, but no wallet that holds the real descriptor ever scans that path.

Reach: the NFC scan door. `scanner.Scan` (gui/scan.go:87) returns any descriptor `nonstandard.OutputDescriptor`
parses, and `engraveObjectFlow` (gui/gui.go:2644-2645) opens `descriptorFlow` with it. The payload door already
refuses (sysw `admitDescriptor`, conjunct 7 `useSitesOK`, sysw/descriptor.go:327-335, 439-461), and the md1 door
already refuses (gui/md1_expand.go:223-247). The Rust primary refuses (mnemonic-engrave
`crates/me-cli/src/descriptor/admit.rs` L255-262). mnemonic-engrave `design/SPEC_descriptor_input.md` L772-776
records the device behaviour as measured-broken; address.go:216-219 described it as intended (T4 settles it: refuse).

## 2. Spec delta (each rule numbered; §7 maps every rule to its row)

- **S1.** `derivePubKey` returns an error for ANY element of the key's effective children whose `Hardened` is true,
  whatever its `Type` (child, range, wildcard), before deriving anything. The error wraps `errUnsupported`, and its
  text is exactly `hardened use-site path element: unsupported descriptor`. Through `addressAt` the caller sees
  exactly `address: hardened use-site path element: unsupported descriptor` (Receive and Change alike).
  **Confirmed** (architect 2026-11-22, coordinator/agent-reports/architect-sh8-proposed-texts.md): this string. It reaches no screen on any door after S5 (§3), so it is a log/test
  text, not a holder text.
- **S2.** Hence `Supported(desc)` is false for such a descriptor (singlesig pkh, wpkh, sh(wpkh), tr key path, and
  sortedmulti when ANY seat carries a hardened step, unless an EARLIER seat already fails with a plain error (a non-consecutive range, or any other non-errUnsupported error, §10). `addressAt` returns the first seat's error, so e.g. `wsh(sortedmulti(1,K/<0;2>/*,K/0h/*))` keeps `Supported` true with no address, the §10 class, unchanged here); `Receive`/`Change` return the S1 error on both chains;
  `Find` returns that error with `found == false` (it propagates the first per-index error, address.go:72-76);
  `DeriveChild` (address/taproot_script_path.go:293-295) returns `nil` and the S1 error. No other function changes:
  the taproot script-path derivers call `derivePubKey` and inherit S1.
- **S3.** `resolveChild` (address.go:220) reports `ok == false` for any element with `Hardened` true, mirroring S1.
  So `DerivesSameKey` is false whenever either side has a hardened element (neither derives, as the existing
  "unsupported range" row). Consequence, intended: `descriptorRepeatsAKey` no longer reports
  `wsh(sortedmulti(2,A/0h/*,A/0/*,B))` or `.../*h` vs `/*` as a repeated key; such a descriptor shows no address at all
  (S2), which is the stronger refusal, and after S5 it does not reach the descriptor screen from any door.
- **S4.** sysw's use-site conjunct is exported unchanged: rename `useSitesOK` to `UseSitesOK` (sysw/descriptor.go:439
  and its one call at :333). Its predicate (`useSiteOK`, :448-460) does not change. One predicate, asked at both doors.
- **S5.** NFC scan door (EN-Q7: the use-site conjunct only). In `engraveObjectFlow`'s `*bip380.Descriptor` case
  (gui/gui.go:2644), before `descriptorFlow`: if `!sysw.UseSitesOK(scan)`, return `false`. The scanner
  (gui/scan.go) does NOT change: it still recognises the tag as a descriptor.
  `engraveObjectFlow` returning false already sets the main screen's status to `scanUnknownFormat`
  (gui/gui.go:2210-2212), which draws the existing text **`Unknown format`** (gui/gui.go:2400-2401). No new text,
  screen or status. **Confirmed** (architect 2026-11-22, coordinator/agent-reports/architect-sh8-proposed-texts.md): reusing `Unknown format` for a refused scanned descriptor. It
  mirrors the payload door, where the same refusal makes the record ClassUnknown and inert
  (sysw/descriptor.go:47-49). Alternative not taken: refusing inside `scanner.Scan` would also change other screens'
  texts for that tag (derive-xpub's seed scan "That tag is not a seed." would become "Unrecognized tag.";
  the bundle gather's "Not an md1/mk1 card." and the mt1 gather's "Not an mt1 string." would become silence;
  the verify-address scan would gain "Not a recognized address.").
- **S6.** What S5 refuses at the scan door is exactly conjunct 7's complement, for any key of the descriptor:
  admitted use-sites are absent, `/*`, `/i/*`, `/<i;i+1>`, `/<i;i+1>/*` with no hardened element; refused are every
  hardened element (`/0h/*`, `/0'/*`, `/*h`, `/*'`, `/0/*h`, `/5h`) and every other parsable shape (`/5`, `/0/1`,
  `/0/1/*`, `/<0;2>/*`, `/<0;1>/5`). The second group derived correct-looking addresses before (all parse at base,
  probed); refusing them is EN-Q7's ruling (the scan door asks the same conjunct as sysw), not a new rule here.
  `/<0h;1h>/*` and `<a;b;c>` are parse refusals already (unchanged).
- **S7.** Edge cases, decided: a multisig with ONE hardened seat is refused (S2 at the deriver, S5 at the door); a
  descriptor whose keys carry hardened ORIGIN steps (`[fp/48h/0h/0h/2h]xpub.../<0;1>/*`) is unaffected (origin is
  `DerivationPath`, not `Children`); an absent use-site (empty `Children`) still defaults to `<0;1>/*` and derives;
  the scan door applies S5 to every descriptor form `nonstandard.OutputDescriptor` returns (plain text, JSON
  wallet exports and the other non-standard forms alike); none of the other six `admitDescriptor` conjuncts is added
  at the scan door (EN-Q7; audit filed as a FOLLOWUPS line, §9).
- **S8.** No exit code, CLI flag, plate layout or engraved content changes. The firmware has no exit codes; the only
  holder-visible change is S5's status line.

## 3. Holder-visible changes (complete list)

| where | before | after |
|---|---|---|
| main screen, NFC tag holding a descriptor refused by S6 | descriptor screen opens; "Show addresses" lists `/0/i` addresses; "Verify an address" says controlled | no screen opens; status line `Unknown format` (existing text) |
| descriptor screen reached by any door with a hardened step | Addresses button active (wrong addresses) | unreachable after S5 (md1 and payload doors already refuse); were it reached, the Addresses button is inert, as for every unsupported descriptor (gui/gui.go:3316, existing gate) |
| every other NFC screen (derive-xpub, bundle, md1/mk1/mt1 gathers, verify-address) | unchanged | unchanged (the scanner still returns the descriptor object) |

## 4. Code changes (builder applies; comments are part of the change)

1. address/address.go:290, right after `children := effectiveChildren(k)` and before `xpub := k.ExtendedKey()`, a separate loop: `for _, c := range children { if c.Hardened { return nil, fmt.Errorf("hardened use-site path element: %w", errUnsupported) } }`. The derive loop does not change. (A pre-scan, so S1's error wins over a later element's plain error, as S1 says.)
2. address/address.go:220, first statement of `resolveChild`: return `0, false, false` when `c.Hardened`.
3. address/address.go:216-219, replace the "NOTE WHAT IS ABSENT" paragraph with exactly:
   `// A HARDENED element is !ok, because derivePubKey refuses it: a hardened step below an extended public key`
   `// cannot be derived, and deriving the unhardened child instead showed a wrong address (bh-sh-C2,`
   `// FIX-sh-address-hardened-usesite).`
4. address/address.go:244 "four spellings" becomes "two spellings"; delete the two table rows at :250-251
   (`A/0h/*` and `A/*h`); after the table add exactly:
   `// A hardened spelling is not in this table: it derives nothing (derivePubKey refuses it), so it is never the`
   `// same key as anything.`
5. sysw/descriptor.go: S4's rename (:333, :439). In the conjunct-7 comment, replace the sentence at :433-436
   ("A HARDENED use-site component silently derives ... impossible).") with exactly:
   `// A HARDENED use-site component cannot be derived from an xpub; package address refuses it`
   `// (errUnsupported), and before FIX-sh-address-hardened-usesite it derived the UNhardened child and showed`
   `// addresses that are not the descriptor's (only the xprv holder can derive those).`
   and add after the comment's last line:
   `// Exported because the NFC scan door asks this same conjunct (gui engraveObjectFlow, sweep-1122 EN-Q7).`
6. gui/gui.go:2644, S5's check, with the comment:
   `// The scan door asks the payload door's use-site conjunct (sweep-1122 EN-Q7); a refused descriptor reads as`
   `// "Unknown format", as sysw makes it ClassUnknown.`
7. gui/descriptor_duplicate_keys.go:32, the comment "Three more spellings did the same." counted the two hardened spellings, which no longer derive: it becomes "One more spelling did the same."

The scratch version of 1, 2, 4's code half, 5's rename and 6 is in `design/FIX_sh_address_hardened_usesite-probe.diff`
(with the tests of §5); it is evidence, not the patch to copy blindly: its comments are not the §4 texts.

## 5. Tests

New (test function names are the TRACE row names):
- address/fix_sh_hardened_usesite_test.go: `TestFixShHardenedUseSiteRefused` (11 shapes: wpkh `/0h/*`, `/0'/*`,
  `/*h`, `/*'`, `/0/*h`, `/5h`; pkh `/0h/*`; sh(wpkh) `/*h`; tr `/0h/*`; wsh(sortedmulti(1, K/1/*, K/0h/*)); wpkh `/<0;2>/*h` (non-consecutive range then wildcard h; the pre-scan row); each: both
  chains error with S1's exact text and `errors.Is(err, errUnsupported)`, `Supported` false, `Find` of
  `tb1qnskw4kf6qevrcyjjd7cnn39uemz9j2cnaxa4u3` not found with an errUnsupported error);
  `TestFixShDeriveChildRefusesHardened` (`/0h/*`, `/*h`, `/5h`, both chains: nil key, errUnsupported);
  `TestFixShUnhardenedUseSiteUnchanged` (control: wpkh `/0/*` index 0 is `tb1qnskw4kf6qevrcyjjd7cnn39uemz9j2cnaxa4u3`,
  wpkh `/*` index 0 is `tb1qhdgyak30llluz36r6lk5ywup5uq0jz0ejkztjl`, both Supported).
  Key K is the signet/testnet tpub `tpubD6NzVbkrYhZ4Wgf68pWW...5Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ` from triage-sh-t1
  (full string in the probe diff).
- gui/fix_sh_hardened_usesite_test.go: `TestFixShScanDoorRefusesUseSite` (S6's 11 refused use-sites, each as wpkh
  and as wsh(sortedmulti(1, K+use-site, K/9/*)); the text goes through the real scanner (`scanOnce`, which must
  still return a `*bip380.Descriptor`), then `engraveObjectFlow` with Back queued must return false);
  `TestFixShScanDoorAdmitsClosedSet` (control: S6's 5 admitted use-sites, both forms, return true);
  `TestFixShHardenedSpellingDerivesNothing` (the two rows moved out of TestEverySpellingOfTheSameKeyIsCaught:
  `descriptorRepeatsAKey` false and `address.Supported` false).

Tests that change:
- address/derives_same_key_test.go `TestDerivesSameKeySpellings`: rows "hardened child vs not" (:61-63) and
  "hardened wildcard vs not" (:64-65) flip to `false`, why texts "derivePubKey refuses a hardened use-site step, so
  the hardened side derives nothing and cannot collide" and "same reason"; add row "hardened both sides"
  (`childD(0,true),wildD(false)` on both sides, `false`, "neither side derives, as the unsupported range row").
- gui/descriptor_duplicate_keys_test.go `TestEverySpellingOfTheSameKeyIsCaught`: delete rows "hardened vs not" and
  "hardened wildcard vs not" (:349-352); header comment :329 "the four spellings" becomes "the two spellings", and
  :338 "Three of them differ from the fixture by a single token." becomes "The two hardened spellings moved to
  TestFixShHardenedSpellingDerivesNothing: they no longer derive."

No other existing test changes: the whole suite passes on the scratch fix (§8).

## 6. RED rows (run on base 9aec7a8 with the §5 tests added, signet/testnet keys only)

Command: `CGO_ENABLED=0 go test -count=1 -run 'TestFixSh|TestDerivesSameKeySpellings|TestEverySpellingOfTheSameKeyIsCaught' ./address/ ./gui/`

| row | expected at base (seen) | expected after the fix (seen on scratch fix) |
|---|---|---|
| TestFixShHardenedUseSiteRefused | FAIL; e.g. `change=false: derived "tb1qnskw4kf6qevrcyjjd7cnn39uemz9j2cnaxa4u3", want refusal`, `Supported = true, want false`, `Find = chain 0 index 0 found true err <nil>, want not found and errUnsupported` (wpkh `/0h/*`); 11/11 shapes fail | PASS |
| TestFixShHardenedUseSiteRefused, shape 11 `wpkh(K/<0;2>/*h)` | FAIL `change=false: err "address: unsupported range path element" (Is errUnsupported=false), want "address: hardened use-site path element: unsupported descriptor"` and `Supported = true, want false` (an in-loop check fails the same way; only the pre-scan passes) | PASS (pre-scan) |
| TestFixShDeriveChildRefusesHardened | FAIL `/0h/* change=false: DeriveChild = true <nil>, want nil and errUnsupported` (6 lines) | PASS |
| TestDerivesSameKeySpellings | FAIL `DerivesSameKey = true, want false -- derivePubKey refuses a hardened use-site step, ...` (3 rows, both orders) | PASS |
| TestFixShScanDoorRefusesUseSite | FAIL `use-site "/0h/*" (wpkh): engraveObjectFlow = true (descriptor screen opened), want false (Unknown format)`; 33/33 cases | PASS |
| TestFixShHardenedSpellingDerivesNothing | FAIL `descriptorRepeatsAKey = true, want false ...` and `address.Supported = true, want false ...` (2 rows) | PASS |
| TestFixShUnhardenedUseSiteUnchanged, TestFixShScanDoorAdmitsClosedSet, TestEverySpellingOfTheSameKeyIsCaught | PASS (controls) | PASS |

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| S1 error, exact text, errUnsupported, all element types | TestFixShHardenedUseSiteRefused (child `0h`/`0'`, wildcard `*h`/`*'`, fixed `5h`); TestFixShDeriveChildRefusesHardened. Range-hardened is a parse refusal (`/<0h;1h>/*`), so S1 covers it by construction only. |
| S2 Supported/Receive/Change/Find/DeriveChild, every script, any multisig seat | TestFixShHardenedUseSiteRefused (pkh, wpkh, sh(wpkh), tr, wsh(sortedmulti) one seat); TestFixShDeriveChildRefusesHardened |
| S3 resolveChild / DerivesSameKey / descriptorRepeatsAKey | TestDerivesSameKeySpellings (3 rows); TestFixShHardenedSpellingDerivesNothing |
| S4 one predicate, unchanged | TestFixShScanDoorRefusesUseSite + TestFixShScanDoorAdmitsClosedSet (via `sysw.UseSitesOK`); sysw's existing seam-vector suite stays green |
| S5 scan door returns false, scanner unchanged | TestFixShScanDoorRefusesUseSite (asserts scanner object AND false) |
| S6 refused and admitted sets | TestFixShScanDoorRefusesUseSite (11 use-sites x 2 forms); TestFixShScanDoorAdmitsClosedSet (5 x 2) |
| S7 one hardened seat; origin hardening unaffected; absent default | TestFixShHardenedUseSiteRefused (wsh row); existing sysw/address suites with `[fp/48h/..]` origins stay green; TestFixShScanDoorAdmitsClosedSet `""` row |
| S8 no other holder change | whole suite green (§8); §3 table |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`,
which asserts its partition is exhaustive) ; `CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/` ;
`./scripts/test-32bit.sh` ; `GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium` ; `./scripts/refugium-tagged-test.sh` ;
both TinyGo device builds (`.github/workflows/test.yml` L150, L153) and `scripts/refugium-elf-check.sh` (CI; no tinygo here).
Measured on the scratch fix (go1.26.7): all-but-gui ok; gui "all 1434 tests ran across 12 shards", ok; oraclelive,
32-bit, both wasm vets rc 0; refugium-tagged "1291 passed ... of 1361" on 4 of 5 runs. One run printed `RESULT: FAIL`
and its detail was not kept; the next three runs passed and the new rows passed 20/20 under `-tags refugium`. The
builder captures that job to a file and greps it. TinyGo builds owed to CI.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. No `mr` or `md-cli` flag or printed text changes (device firmware only).
- mnemonic-engrave `design/SPEC_descriptor_input.md` L772-776 says "`md` refuses it, the device does not -
  measured". After this fix the device refuses at every door; that sentence becomes history. Filed below, not edited
  here (another repo).
- Closes: nothing tracked (row 20 was untracked, swept at R0-descriptor-input-spec-r2.md:138). bh-sh-C2 closes with
  the GREEN merge.
- Files (proposed, verbatim, for direct-push after the merge; the first is the sweep architect's line):

```
- [sh-scan-door-other-conjuncts-audit] status=open grade=M kind=funds owner=none filed=2026-11-22 src=coordinator/agent-reports/architect-sweep-1122-questions.md :: NFC scan door runs only useSitesOK after the row-20 fix; audit admitDescriptor's other six conjuncts (sysw/descriptor.go:326-334) for a wrong result at that door.
- [me-spec-descriptor-input-hardened-device-note] status=open grade=N kind=docs owner=none filed=2026-11-22 src=seedhammer:design/FIX_sh_address_hardened_usesite.md :: SPEC_descriptor_input.md conjunct 7 (L772-776) says the device does not refuse a hardened use-site; after FIX-sh-address-hardened-usesite it refuses at every door (address errUnsupported, scan door UseSitesOK).
```

## 10. Out of scope

- A non-consecutive range (`<0;2>/*`) still makes `derivePubKey` return a plain error while `Supported` is true
  (SPEC_descriptor_input.md L777-779). After S5 no door delivers one to a screen; not changed here.
- The other six `admitDescriptor` conjuncts at the scan door (FOLLOWUPS line above).

## 11. How this was checked

Probe dir `/scratch/cache/wt-sh-hardened-usesite-probe/` (not part of the build): the §5 tests were added to a clean
base worktree and run (RED, `red.txt`, `red-gui2.txt`, `red-derivechild.txt`); the scratch fix was applied and the §8
jobs were run (`green2.txt`, `refugium-r1..3.txt`); then the worktree was reset. The combined diff is committed beside
this document as `design/FIX_sh_address_hardened_usesite-probe.diff`. Scan-door shape probe at base: every §2 S6
use-site parses to a `*bip380.Descriptor` (`probe.txt`). Go: `/scratch/code/shibboleth/.toolchain/go` go1.26.7
(go.mod says go 1.25.10), `CGO_ENABLED=0`, GOCACHE under the probe dir.
