# FIX: the Refugium build refuses an unsealed systemwide payload holding a secret

**Small track** (one Opus review round). Class: other (not a funds path). Grade I, three rows, one defect:
`kitarch-a-s1-sysw-plaintext-secret`, `kitarch-b-s1-plaintext-secret-admitted` (seedhammer half),
`kitarch-c-a1-plaintext-secret-not-refused` (coordinator `status/secret-criticals.md` L300, L302, L304; recon
`agent-reports/secret-recon-sh-1122.md` L13, L17, L20).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 127cf04. Anchors are at that
base unless stated.

**BUILD GATE: BLOCKED ON Q3.** Brian's Q3 ruling (coordinator `memory/kit-engraving-rulings-2026-11-22.md`: "Refugium
build WARNS on a plaintext payload holding a secret (does not refuse); the kitarch *-s1/*-a1 secret Importants stay on
the alpha list") is the current product rule, and this FIX contradicts it. The coordinator dispatched it to be
authored now and built only when Q3 is lifted (before the first non-alpha release, Brian 2026-11-19). It re-rules
nothing; it is the build-ready change for that moment. STOP-2 (a sealed systemwide payload holding a secret WARNS,
Brian 2026-11-22) is not touched.

## 1. The defect

In the Refugium build, `syswLoadFlow` loads a plaintext (unsealed) systemwide payload holding a record whose class is
secret (`sysw.Class.IsSecret`: ClassMnemonic, ClassCodex32Secret, ClassPassphrase, ClassPreimage, ClassPhrase). It shows
the digest, creates the session (`gui/sysw_load.go:202-207`), then shows "Payload Warnings" with F1's line and the
§14f Refugium line (`gui/sysw_load_refugium.go:9-17`, `gui/sysw_load.go:228-247`) and offers KEEP / UNLOAD. KEEP leaves
the secret loaded and every program may take it. The rows' rule (Brian 2026-11-22, "secrets only sealed"; ruling Q1
routes secrets through the Sealed Payload, the wiping container) is met only by a refusal. Seen on base: RED rows R1,
R2 (§6) draw the digest screen where the refusal belongs.

Only `pass:` is refused today (`gui/sysw_load.go:160-175`, plan F7 §4.3); this FIX adds the other secret classes for
unsealed payloads at the same point.

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (refuse).** Refugium build only. After `sysw.Open` succeeds and after the `pass:` refusal, when the header is
  NOT sealed (`!h.Sealed()`) and any record's class is secret by `sysw.Classify(r).IsSecret()`, the flow shows the
  modal of §3 and returns false. No digest screen, no session (`ctx.sysw` is not assigned), no Payload Warnings, no
  KEEP / UNLOAD. Identical on every route into `syswLoadFlow`: boot (`gui/gui.go:2136`, after LOAD), the carousel's
  Load Payload and LOAD AGAIN (`gui/sysw_unload.go:120`); all three are this one function.
- **R2 (position).** The modal names the FIRST secret record by 1-based position counted over `p.Public` then
  `p.Secret` (the order `syswSession.load` appends them, `gui/sysw_session.go:104-106`), never its contents.
- **R3 (class, not flag).** Secrecy is by CLASS only, as §14f decides it: a record secret only because it is an
  unconfirmed md1/mk1 (`[mdmk-decode]`) or an unconfirmed mt1 (`[mt-decode]`) is NOT refused; its F1 warning and the
  KEEP / UNLOAD offer are unchanged.
- **R4 (precedence).** A payload holding a `pass:` record keeps the existing `pass:` refusal and its text, unchanged,
  even when another secret record comes before it. The `pass:` check runs first (it keys on the prefix, so it also
  catches a `pass:` that classifies ClassUnknown); R1 runs only when it did not fire.
- **R5 (sealed unchanged).** A sealed systemwide payload holding a secret keeps today's behaviour in the Refugium
  build: §14f line last on Payload Warnings, then KEEP / UNLOAD (STOP-2 ruled (a)).
- **R6 (default build unchanged).** The default build (`!refugium`) is byte-for-byte unchanged in behaviour:
  `refugiumProfile` is the const `false` there (`gui/profile_default.go:10`), so the new branch is dead code it does
  not take; F1 + KEEP / UNLOAD stay.
- **R7 (no secret enters a session).** The refusal is decided before `ctx.sysw` is created, as the `pass:` refusal's
  comment (`gui/sysw_load.go:160-169`) argues: `sysw.Open` has already decoded the records into Go strings (not
  wipeable), but no session, picker default or program ever sees them.

## 3. Holder-visible changes (complete list)

One new modal, Refugium build only, via `showError` (the same call and title as the `pass:` refusal):

- Title: `Load Payload` — PROPOSED (architect confirms). Reused from the sibling `pass:` refusal at
  `gui/sysw_load.go:172`; the recon's "Not Loaded" was considered and not taken, because "Not Loaded" titles the
  declined-digest case, which is not a refusal.
- Body, `%d` the R2 position — PROPOSED (architect confirms):
  `Record %d of this payload is a secret, and the payload is not sealed. This build takes secrets only in the Sealed Payload, so nothing was loaded. Pack the payload again without it.`

  It names no class: `txClassName` (`gui/transaction.go:276-299`) has no name for ClassPreimage or ClassPhrase (they
  read "unrecognised record"), and a new class-name vocabulary is out of scope for a small row. "Sealed Payload" is the
  §14f line's own name for the wiping container.

No other text changes. No exit code (device UI). The §14f line, F1's line and KEEP / UNLOAD are unchanged and still
shown for sealed payloads (R5) and for unconfirmed-only plaintext payloads (R3).

## 4. Code changes (builder applies; reference implementation in `design/FIX_sec-sh-sysw-plaintext-refuse-probe.diff`)

- `gui/sysw_load.go`, inside the existing `if refugiumProfile {` block (L170-175), after the `pass:` check: when
  `!h.Sealed()`, call a new `syswFirstSecretRecord(p)` and on found `showError(ctx, th, "Load Payload",
  syswPlaintextSecretRefusal(n))` and `return false`.
- `gui/sysw_load.go`, two new functions after `syswPassphraseRecordRefusal` (L359-363):
  `syswFirstSecretRecord(p *sysw.Payload) (int, bool)` (R2 order, `sysw.Classify(r).IsSecret()`), and
  `syswPlaintextSecretRefusal(position int) string` (the §3 body via `fmt.Sprintf`).
- Comments (no behaviour): `gui/sysw_load.go:160-169` gains one sentence that the same block also refuses an unsealed
  payload holding a secret class (this FIX, rows above); `gui/sysw_load_refugium.go:5-8` reads "WARNS, and does not
  refuse, when a loaded SEALED payload holds a record whose class is secret by 3.3.1; a plaintext one is refused at
  load (syswLoadFlow)"; `gui/sysw_load.go:330-331` drops "(nothing is refused)" for "(the payload is sealed here: a
  plaintext secret was refused at load)".
- Kept as is, by decision: the plaintext arm of `syswLoadWarnings` that appends the §14f line after F1
  (`gui/sysw_load.go:306-311`). The Refugium load flow no longer reaches it with a plaintext secret, but it is the
  warnings renderer's rule over any session and `TestRefugiumSecretWarningLine` pins it at unit level; removing it is
  not needed to close the rows.
- `scripts/refugium-tagged-test.sh:22`: "one of six" becomes "one of seven" (the new skip reason, §5).

The builder may copy the probe diff's `gui/` and `scripts/` hunks verbatim; they are what produced §6's GREEN rows.

## 5. Tests

New file `gui/sysw_plaintext_secret_refugium_test.go` (`//go:build refugium`), in the probe diff:

- `TestKitarchPlaintextSecretRefusedBeforeDigest` (R1): payload `[text:48656c6c6f, <secret>]` for each of ClassMnemonic
  (BIP-39 test mnemonic abandon x11 + about), ClassCodex32Secret (BIP-93 vector `refugiumMS1Vec`), ClassPreimage
  (`composerTestPreimageRecord`), ClassPhrase (`composerTestPhraseRecord`), on boot and non-boot routes: the refusal
  with "Record 2" is drawn, no "Compare this against", `ctx.sysw == nil`, return false. The fixture's class is asserted
  first (INCONCLUSIVE otherwise).
- `TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord` (R2): `[mnemonic, ms1]` names Record 1.
- `TestKitarchPassRecordRefusalKeepsPrecedence` (R4, guard): `[mnemonic, pass:636f7272]` shows
  `syswPassphraseRecordRefusal(2)` and not the new text.
- `TestKitarchPlaintextWithoutSecretClassStillLoads` (R3, guard): a public `text:` payload and `kit-mk1-alone`'s
  unconfirmed md1 reach "Compare this against".

Tests that change (each loads a plaintext payload holding a secret through `syswLoadFlow`, so under `-tags refugium`
it now meets the refusal; found by running the whole tagged suite with the fix and nothing else, §6): each gains
`skipUnderRefugium(t, refugiumSkipPlaintextSecret)` as its first statement, and runs unchanged in the default build:

| Test | File |
|---|---|
| TestKitRefusedAfterWarnings | gui/kit_door_test.go |
| TestKitRefusedOnceAtEveryLoad | gui/kit_refused_once_test.go |
| TestKitRefusedOnceThenMenuAfterLoadAgain | gui/kit_menu_after_load_test.go |
| TestKitRefusedAfterLoadAgainAndOnceAtBoot | gui/kit_extra_test.go |
| TestNoKitPayloadFromCarouselOpensNoMenu | gui/kit_nokit_carousel_test.go |
| TestF1OffersUnloadWhereItOnceOfferedErase | gui/sysw_unload_test.go |
| TestChainCodex32FromAMePackedPayloadToAnMs1Plate | gui/chain_class_walk_test.go |
| TestChainMdMkFromTheEmulatorsOwnPayloadToFourPlates | gui/chain_class_walk_test.go |
| TestChainMnemonicFromAMePackedPayloadToASeedPlate | gui/chain_class_walk_test.go |

New skip reason in `gui/profile_skip_test.go` (test-log text, not holder-visible): `refugiumSkipPlaintextSecret =
"an unsealed payload holding a secret is refused at load (FIX sec-sh-sysw-plaintext-refuse)"`. The script accepts any
reason starting "refugium profile: " (`scripts/refugium-tagged-test.sh`, the python block's `profile_skips`).

Unchanged and still pinning R5/R3: `TestRefugiumSecretWarningLine` (`gui/kit_warning_refugium_test.go`, sealed rows S-D
and S-E through the real flow), `TestRefugiumLineSealedUnconfirmedMdmkAloneRaisesNone`
(`gui/kit_extra_refugium_test.go`), and the default build's `TestF1OffersUnloadWhereItOnceOfferedErase` (R6).

## 6. RED and GREEN rows (go1.26.4 at `~/.local/go/bin`, `GOTOOLCHAIN=local CGO_ENABLED=0`, GOCACHE and TMPDIR under the worktree)

RED, on base 127cf04 with only the new test file added
(`git apply --include='gui/sysw_plaintext_secret_refugium_test.go' design/FIX_sec-sh-sysw-plaintext-refuse-probe.diff`):

`go test -tags refugium -count=1 -run '^TestKitarch' -v ./gui/`

Expected (seen; copied from the run, digests abbreviated by the reader only):

```
--- FAIL: TestKitarchPlaintextSecretRefusedBeforeDigest
    ...: mnemonic boot=false: want the refusal before any digest; got "PayloadDigestComparethisagainst`mesyswshow<file>`onthehost:b13bd6cdb1f3b750de030a2619885f7a"
    (and boot=true, codex32 secret, preimage, phrase: 8 lines, each the digest screen)
--- FAIL: TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord
    ...: want Record 1 named; got "PayloadDigestComparethisagainst`mesyswshow<file>`onthehost:333461fd47ea959706906f8a27d6c0f0"
--- PASS: TestKitarchPassRecordRefusalKeepsPrecedence
--- PASS: TestKitarchPlaintextWithoutSecretClassStillLoads
FAIL	seedhammer.com/gui
```

GREEN, with the whole probe diff applied (`git apply design/FIX_sec-sh-sysw-plaintext-refuse-probe.diff`):

- the same command: 4 PASS, `ok seedhammer.com/gui`.
- `./scripts/refugium-tagged-test.sh 6`: `RESULT: ok -- 1314 passed, 68 stepped aside for the profile and 2 for the
  environment, 9 deferred-minor, of 1393 tests under -tags refugium`.
- Fix without the nine skips (how the "tests that change" list was found): the same script, `RESULT: FAIL`, exactly
  the nine tests of §5 failing.
- `go test -timeout 20m ./...` (untagged): see §8.

## 7. Coverage map (rule -> rows)

| Rule | Rows |
|---|---|
| R1 refuse, both routes, no session | TestKitarchPlaintextSecretRefusedBeforeDigest (RED) |
| R2 first position | TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord (RED), R1's "Record 2" |
| R3 class not flag | TestKitarchPlaintextWithoutSecretClassStillLoads (guard) |
| R4 pass: precedence | TestKitarchPassRecordRefusalKeepsPrecedence (guard) |
| R5 sealed unchanged | TestRefugiumSecretWarningLine sealed rows, TestRefugiumLineSealedUnconfirmedMdmkAloneRaisesNone (existing) |
| R6 default unchanged | the untagged suite, incl. the nine changed tests running unskipped (existing) |
| R7 no session | R1's `ctx.sysw == nil` and return-false assertions |

## 8. Gate (the repo's CI jobs, `.github/workflows/test.yml`; run once each to a file and grep it)

`go test -timeout 20m ./...`; `go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`;
`./scripts/test-32bit.sh`; `GOOS=js GOARCH=wasm go vet ./cmd/emu/` and the same with `-tags refugium`;
`./scripts/refugium-tagged-test.sh`; the TinyGo device builds (default and `-tags refugium`, nix) and
`scripts/refugium-elf-check.sh`, locally or as green CI on the head.

Author's runs with the probe applied (each once, to a file, grepped): untagged `go test -timeout 20m ./...`: every
package ok (`ok seedhammer.com/gui 217.538s`) except `cmd/emu` `TestNothingImportsTheEmulator`, which failed only
because the author's TMPDIR was inside the worktree (its tree walk found a live `_testmain.go`); re-run with TMPDIR
outside the worktree: rc=0. oraclelive compile rc=0; both wasm vets rc=0; `./scripts/test-32bit.sh` rc=0;
`./scripts/refugium-tagged-test.sh 6` RESULT ok (§6); TinyGo pico-plus2 default and `-tags refugium` builds rc=0
(refugium 1622040 flash bytes) and `./scripts/refugium-elf-check.sh` "RESULT: ok -- 11 markers present in the default
ELF and absent in the Refugium ELF". Keep TMPDIR and GOCACHE outside the worktree when re-running.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. No `mr`/`md-cli` flag or printed text changes; the manual has no device-screen text for this
  flow (`git grep` of `docs/manual` at mnemonic-toolkit origin/master for "unencrypted in flash", "Payload Warnings",
  "secrets travel only": no hits).
- Spec lockstep, mnemonic-engrave `design/SPEC_systemwide_payloads.md` (origin/master 16356ac1), landed with the build
  when Q3 is lifted (texts PROPOSED, architect confirms):
  - §3 third amendment (L531-534): replace "in the Refugium build, a loaded payload (plaintext or sealed) holding a
    genuinely secret class gets one more warning line ... It warns and does not refuse (Brian's Q3 ruling)." with "in
    the Refugium build, a plaintext payload holding a genuinely secret class is refused at load, and a sealed one gets
    one more warning line, which names the Sealed Payload as the only way secrets travel in that build (Q3 lifted,
    FIX sec-sh-sysw-plaintext-refuse)."
  - §14a Q3 line (L1708-1710) and §14f (L1930-1949): the plaintext half becomes the R1-R4 refusal with §3's text;
    the sealed half stands. Test row 31 (L1439-1444): "plaintext, directly after the F1 line" becomes "plaintext:
    refused at load (§14f)".
  - §14k (L2120-2126): `kitarch-a-s1`, `kitarch-c-a1` and the seedhammer half of `kitarch-b-s1` are closed by this
    build.
- FOLLOWUPS: none closed (the rows live in coordinator `status/secret-criticals.md`); none filed by this document.
  `kitarch-b-s1` stays open until its host half (`me sysw pack` permits plaintext secrets, spec decision 6) lands in
  its own mnemonic-engrave unit.

## 10. Out of scope

- Sealed systemwide payloads holding a secret (STOP-2 ruled warn; R5).
- `me sysw pack` refusing to write such a payload (the host half of `kitarch-b-s1`; a mnemonic-engrave unit).
- Wiping the decoded strings `sysw.Open` returns (the systemwide container is non-wiping by design; the Sealed
  Payload wipe gaps are their own rows).
- A class-name vocabulary for ClassPreimage / ClassPhrase in `txClassName`.

## 11. How this was checked

RED rows run on base 127cf04 with only the new test file (output `/scratch/cache/sec-sh-sysw-red.txt`); GREEN rows and
the gate with the probe diff applied (`/scratch/cache/sec-sh-sysw-green.txt`, `-probe-tagged.txt` without the skips,
`-probe-tagged2.txt` with them, `-probe-untagged.txt`). The probe was reverted after the runs; the branch carries only
this document and the probe diff.
