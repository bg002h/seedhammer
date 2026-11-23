# FIX: the Refugium build refuses a systemwide payload holding a secret outside a sealed section

**Small track** (one Opus review round). Class: other (not a funds path). Grade I, three rows, one defect:
`kitarch-a-s1-sysw-plaintext-secret`, `kitarch-b-s1-plaintext-secret-admitted` (seedhammer half),
`kitarch-c-a1-plaintext-secret-not-refused` (coordinator `status/secret-criticals.md` L300, L302, L304; recon
`agent-reports/secret-recon-sh-1122.md` L13, L17, L20).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 127cf04. Anchors are at that
base unless stated.

**Q3 re-ruled by Brian 2026-11-22, "Refuse, with override"** (coordinator memory/kit-engraving-rulings-2026-11-22.md; texts from agent-reports/architect-sh19-q3-override.md). The Refugium build refuses by default and offers an explicit override. STOP-2 (a sealed payload's sealed-section secret WARNS) is not touched.

## 1. The defect

In the Refugium build, `syswLoadFlow` loads a plaintext (unsealed) systemwide payload, or a sealed one whose PUBLIC
(cleartext) section holds a record, whose class is secret (`sysw.Class.IsSecret`: ClassMnemonic, ClassCodex32Secret, ClassPassphrase, ClassPreimage, ClassPhrase). It shows
the digest, creates the session (`gui/sysw_load.go:202-207`), then shows "Payload Warnings" with F1's line and the
§14f Refugium line (`gui/sysw_load_refugium.go:9-17`, `gui/sysw_load.go:228-247`) and offers KEEP / UNLOAD. KEEP leaves
the secret loaded and every program may take it. The rows' rule (Brian 2026-11-22, "secrets only sealed"; ruling Q1
routes secrets through the Sealed Payload, the wiping container) is met only by a refusal. Seen on base 127cf04: the first RED rows R1 and R2 drew the digest screen where the refusal belongs (/scratch/cache/sec-sh-sysw-red.txt); §6 now holds the override's RED on 19c8dd18.

Only `pass:` is refused today (`gui/sysw_load.go:160-175`, plan F7 §4.3); this FIX adds the other secret classes, for any record outside a sealed section, at the same point. Only a
sealed payload's SEALED section is encrypted; its public section is cleartext in flash and in the host file, merely
authenticated (`sysw/open.go:44-64`), so the refusal keys on the SECTION, not on the header (r1 I-2).

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (refuse).** Refugium build only. After `sysw.Open` succeeds and after the `pass:` refusal, when any record in the payload's PUBLIC (cleartext) section, which is every record of a
  plaintext payload, has a secret class by `sysw.Classify(r).IsSecret()`, the flow shows the
  modal of §3, then the override screen of §3; DO NOT LOAD (the default) or Back returns false. Unless LOAD ANYWAY is chosen: no digest screen, no session (`ctx.sysw` is nil from the refusal on: a session an earlier LOAD ANYWAY left, which LOAD AGAIN re-reading the same region would otherwise keep, is dropped before the modal, as declining the digest drops one), no Payload Warnings, no
  KEEP / UNLOAD. Identical on every route into `syswLoadFlow`: boot (`gui/gui.go:2136`, after LOAD), the carousel's
  Load Payload and LOAD AGAIN (`gui/sysw_unload.go:120`); all three are this one function.
- **R2 (position).** The modal names the FIRST secret record by 1-based position counted over `p.Public` (in a plaintext payload `p.Secret` is always empty,
  `sysw/open.go:50`; public is counted first, as `syswSession.load` appends, `gui/sysw_session.go:104-106`, so the
  position is the same as the whole-payload count), never its contents.
- **R3 (class, not flag).** Secrecy is by CLASS only, as §14f decides it: a record secret only because it is an
  unconfirmed md1/mk1 (`[mdmk-decode]`) or an unconfirmed mt1 (`[mt-decode]`) is NOT refused; its F1 warning and the
  KEEP / UNLOAD offer are unchanged.
- **R4 (precedence).** A payload holding a `pass:` record keeps the existing `pass:` refusal and its text, unchanged,
  even when another secret record comes before it. The `pass:` check runs first (it keys on the prefix, so it also
  catches a `pass:` that classifies ClassUnknown); R1 runs only when it did not fire. The pass: refusal has no override.
- **R5 (sealed section unchanged).** A sealed systemwide payload holding a secret only in its sealed section keeps
  today's behaviour in the Refugium build: §14f line last on Payload Warnings, then KEEP / UNLOAD (STOP-2 ruled (a)).
- **R6 (default build unchanged).** The default build (`!refugium`) is byte-for-byte unchanged in behaviour:
  `refugiumProfile` is the const `false` there (`gui/profile_default.go:10`), so the new branch is dead code it does
  not take; F1 + KEEP / UNLOAD stay.
- **R7 (no secret enters a session).** Unless the operator chooses LOAD ANYWAY, the refusal is decided before `ctx.sysw` is created, as the `pass:` refusal's
  comment (`gui/sysw_load.go:160-169`) argues: `sysw.Open` has already decoded the records into Go strings (not
  wipeable), but no session, picker default or program ever sees them.
- **R8 (override).** LOAD ANYWAY continues the unchanged flow: digest, session, Payload Warnings with the §14f line (after F1 in a plaintext payload; last and alone of the secret lines in a sealed one), then KEEP / UNLOAD. Nothing about wiping changes (the session is non-wiping, decision 2). The choice is per load and is never remembered.
- **R9 (default and Back).** On the override screen DO NOT LOAD is choice 0, highlighted and the default (`Initial` left at zero); row 0 is the row labelled DO NOT LOAD. DO NOT LOAD, or Back (`!ok`), returns false with no further screen and leaves nothing loaded, on LOAD AGAIN too (R1).

## 3. Holder-visible changes (complete list)

Two screens, Refugium build only, in this order. First, one modal via `showError` (the same call and title as the `pass:` refusal):

- Title: `Load Payload`. Reused from the sibling `pass:` refusal at
  `gui/sysw_load.go:172`; the recon's "Not Loaded" was considered and not taken, because "Not Loaded" titles the
  declined-digest case, which is not a refusal.
- Body, `%d` the R2 position:
  `Record %d of this payload is a secret, and it is not sealed. This build takes secrets only in the Sealed Payload, so nothing was loaded. Pack the payload again without it, or load it anyway on the next screen.`

  It names no class: `txClassName` (`gui/transaction.go:276-299`) has no name for ClassPreimage or ClassPhrase (they
  read "unrecognised record"), and a new class-name vocabulary is out of scope for a small row. "Sealed Payload" is the
  §14f line's own name for the wiping container.

Second, the override screen, a `ChoiceScreen`:

- Title: `Unsealed Secret`.
- Lead: `Load it anyway? Every program can then use its secret. It stays unencrypted in flash until you wipe it from the host.`
- Choices: `{"DO NOT LOAD", "LOAD ANYWAY"}`, `Initial` left at its zero value, so DO NOT LOAD (index 0) is highlighted and is the
  default (the pattern of `gui/sysw_unload.go:156-167`, "BACK is choice 0 and therefore the DEFAULT").
- DO NOT LOAD, or Back (`!ok`): `return false`, with no further screen (the modal already said "nothing was loaded"). On LOAD AGAIN after an earlier LOAD ANYWAY and KEEP, the loaded session (the same region, so the same secret) is dropped before the modal is drawn, so the modal is true of the device and DO NOT LOAD or Back leaves nothing loaded; LOAD ANYWAY loads it again. No text changes for this.
- LOAD ANYWAY (index 1): falls through to the unchanged flow (digest, session, Payload Warnings, KEEP / UNLOAD).
- Not remembered: the answer lives only in this call, with no field and no state. Boot, Load Payload and LOAD AGAIN each
  ask again.

Both texts are confirmed (agent-reports/architect-sh19-q3-override.md Q1); "without it" stays. The lead's last sentence was "It stays unencrypted in flash until a new payload is written." until the review fold (coordinator agent-reports/architect-sh19-review-qs.md Q2): `me sysw pack` without `--region` writes only the container, and a shorter one leaves the old records in flash (mnemonic-engrave `crates/me-cli/src/sysw/overwrite.rs:8-10`); the new sentence uses the UNLOAD result's "Wipe it from the host." (`gui/sysw_unload.go:178`). Measured with `widget.Labelw` at 480 px: both leads are two lines, 450x44, inside the 44 px lead band.

No other text changes. No exit code (device UI). The §14f line, F1's line and KEEP / UNLOAD are unchanged and still
shown for sealed payloads (R5) and for unconfirmed-only plaintext payloads (R3).

## 4. Code changes (builder applies; reference implementation: `design/FIX_sec-sh-sysw-plaintext-refuse-probe.diff` (127cf04 to the refusal, already built as sh#19, head 19c8dd18) plus `design/FIX_sec-sh-sysw-plaintext-refuse-override.diff` (19c8dd18 to the override))

- `gui/sysw_load.go`, inside the existing `if refugiumProfile {` block (L170-175), after the `pass:` check, with no
  header condition: call a new `syswFirstSecretRecord(p)` and on found `showError(ctx, th, "Load Payload",
  syswPlaintextSecretRefusal(n))`, then the override screen of §3 in place of the former `return false`. The call-site code is
  the prototype's `gui/sysw_load.go:178-195` (`design/FIX_sec-sh-sysw-plaintext-refuse-override.diff`, hunk at `@@ -177,7 +178,19 @@`): a `ChoiceScreen`
  `cs` with Title `"Unsealed Secret"`, Lead `syswPlaintextSecretOverrideLead` and Choices `[]string{"DO NOT LOAD",
  "LOAD ANYWAY"}`, then `if choice, ok := cs.Choose(ctx, th); !ok || choice != 1 { return false }`.
- `gui/sysw_load.go`, review fold (architect-sh19-review-qs Q1): directly before that `showError(ctx, th, "Load Payload", syswPlaintextSecretRefusal(n))`, `ctx.sysw = nil` with the comment "A session already loaded here came from this same region (LOAD AGAIN re-reads it, and it cannot change while the GUI runs), so it holds this secret: drop it before the modal says "nothing was loaded", as declining the digest drops one. LOAD ANYWAY loads it again below."
- `gui/sysw_load.go`, a new const `syswPlaintextSecretOverrideLead` (prototype L410-413), after
  `syswPlaintextSecretRefusal`: `"Load it anyway? Every program can then use its secret. " + "It stays unencrypted in flash until you wipe it from the host."`
  (the §3 lead). `syswPlaintextSecretRefusal`'s text gains the §3 tail, "Pack the payload again without it, or load it
  anyway on the next screen."
- `gui/sysw_load.go`, two new functions after `syswPassphraseRecordRefusal` (L359-363):
  `syswFirstSecretRecord(p *sysw.Payload) (int, bool)` (a loop over `p.Public` only, `sysw.Classify(r).IsSecret()`), and
  `syswPlaintextSecretRefusal(position int) string` (the §3 body via `fmt.Sprintf`).
- Comments (no behaviour), replacing the three comment texts the probe diff carried; the texts are the prototype's
  (`design/FIX_sec-sh-sysw-plaintext-refuse-override.diff`), verbatim:
  - `gui/sysw_load.go:160-169`, the comment's last sentences read: "The same block also refuses, by default, a payload
    holding a secret class outside a sealed section (FIX sec-sh-sysw-plaintext-refuse): every record of a plaintext
    payload, or a record in a sealed payload's public section. That refusal alone has an override, LOAD ANYWAY, after
    which the payload loads as before and warns (Q3 re-ruled 2026-11-22)."
  - At the call site, the comment: "Refused BY DEFAULT, with an explicit override (Brian 2026-11-22, Q3 re-ruled "Refuse,
    with override"). DO NOT LOAD is choice 0 and therefore the default, as BACK is on the UNLOAD confirmation: the
    resting position is the one that loads nothing. Back is DO NOT LOAD. The answer is not remembered: every load asks
    again."
  - `gui/sysw_load_refugium.go:5-9`: "syswRefugiumLine is SPEC_systemwide_payloads.md 14f: the Refugium build WARNS, and
    does not refuse, when a loaded SEALED payload holds a record whose class is secret by 3.3.1 in its sealed section, or
    outside one when the operator chose LOAD ANYWAY over the refusal at load (syswLoadFlow). It is decided by CLASS: an
    md1/mk1 record that is secret only because it is unconfirmed does not raise it."
  - `gui/sysw_load.go:349-351`, on `syswRefugiumLineShown`: "(the secret is in a sealed section, or outside one and loaded
    by LOAD ANYWAY)".
  - The const's comment: "syswPlaintextSecretOverrideLead is the override screen's question and its cost: the systemwide
    session is the non-wiping class (sysw_session.go), and UNLOAD drops it without zeroing it."
- Kept as is, by decision: the plaintext arm of `syswLoadWarnings` that appends the §14f line after F1 (`gui/sysw_load.go:306-311`). The Refugium load flow reaches it with a plaintext secret only after LOAD ANYWAY (R8). It is the only arm that adds the line to a plaintext payload (the other is `if s.sealed`). `TestKitarchPlaintextSecretLoadAnywayLoadsAndWarns` pins it at flow level and `TestRefugiumSecretWarningLine` at unit level, so it must stay.
- `scripts/refugium-tagged-test.sh:22`: "one of six" becomes "one of seven" (the new skip reason, §5).

The rebuild starts from sh#19's head 19c8dd18 (branch `build/sec-sh-sysw-plaintext-refuse`), whose commits already carry the probe diff; copy nothing more from the probe diff (its refusal text and its R1 test predate the override). In the build worktree: first `git checkout fix/sec-sh-sysw-plaintext-refuse -- design/FIX_sec-sh-sysw-plaintext-refuse.md design/FIX_sec-sh-sysw-plaintext-refuse-override.diff` (this document replaces the branch's stale copy); then the RED commit, `git apply --include='gui/*_test.go' design/FIX_sec-sh-sysw-plaintext-refuse-override.diff` (§6 RED: 5 FAIL, 3 PASS); then the GREEN commit, `git apply --exclude='gui/*_test.go' design/FIX_sec-sh-sysw-plaintext-refuse-override.diff` (§6 GREEN: 8 PASS).

## 5. Tests

New file `gui/sysw_plaintext_secret_refugium_test.go` (`//go:build refugium`), created by the probe diff and extended by the override diff:

- `TestKitarchPlaintextSecretRefusedBeforeDigest` (R1): payload `[text:48656c6c6f, <secret>]` for each of ClassMnemonic
  (BIP-39 test mnemonic abandon x11 + about), ClassCodex32Secret (BIP-93 vector `refugiumMS1Vec`), ClassPreimage
  (`composerTestPreimageRecord`), ClassPhrase (`composerTestPhraseRecord`), on boot and non-boot routes: the refusal
  with "Record 2" is drawn; then Button3 dismisses it, and the test must see the override lead with both labels (DO NOT
  LOAD, LOAD ANYWAY), then press Button3 (the default). The flow must then return within 64 frames, never drawing "Compare
  this against" or "Keep this payload loaded?" (so a flow that does not return fails, r1 I-1);
  `ctx.sysw == nil`, return false. `plaintextSecretLoad` returns its `frame` func for this. The fixture's class is
  asserted first (INCONCLUSIVE otherwise).
- `TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord` (R2): `[mnemonic, ms1]` names Record 1.
- `TestKitarchPassRecordRefusalKeepsPrecedence` (R4, guard): `[mnemonic, pass:636f7272]` shows
  `syswPassphraseRecordRefusal(2)` and not the new text.
- `TestKitarchPlaintextWithoutSecretClassStillLoads` (R3, guard): a public `text:` payload and `kit-mk1-alone`'s
  unconfirmed md1 reach "Compare this against"; and the unconfirmed md1, driven on, shows F1's "Payload Warnings"
  and then "Keep this payload loaded?" (r1 M-3: pins F1 then KEEP / UNLOAD at flow level, which the skipped
  `TestF1OffersUnloadWhereItOnceOfferedErase` no longer does under this tag).
- `TestKitarchSecretInTheCleartextSectionOfASealedPayload` (R1, unit, r1 I-2): `syswFirstSecretRecord` on
  {Public: [text:41, mnemonic], Secret: [ms1]} gives (2, true); on {Public: [text:41], Secret: [ms1]} gives (0, false).
- `TestKitarchSealedPayloadWithASecretInItsPublicSectionIsRefused` (R1, flow, r1 I-2): a hand-sealed region (passphrase
  "abandon about", Public [text:48656c6c6f, mnemonic], Secret [text:41]), driven as `loadSealedForWarnings` does; the
  refusal for Record 2 is drawn; the test then drives on through LOAD ANYWAY, the digest and Payload Warnings (§14f line
  present, F1 absent), to KEEP / UNLOAD.
- `TestKitarchPlaintextSecretLoadAnywayLoadsAndWarns` (R8): LOAD ANYWAY, digest, F1 and the §14f line, KEEP, `ret` true and a
  session loaded. A second `syswLoadFlow` on the same ctx draws the refusal again.
- `TestKitarchPlaintextSecretOverrideBackLoadsNothing` (R9): Back on the override screen loads nothing.
- `TestKitarchLoadAgainDeclineLeavesNothingLoaded` (R1c, review fold Q1): LOAD ANYWAY, digest, Payload Warnings, KEEP; then a second `syswLoadFlow` on the same ctx (LOAD AGAIN), and DO NOT LOAD (subtest 1) or Back (subtest 2) on the override: the flow returns false and `ctx.sysw == nil`.
- `TestKitarchPlaintextSecretOverrideLabelOrder` (R9b, review s2 I-1): on the override screen "DO NOT LOAD" is drawn before "LOAD ANYWAY", so row 0, the default, carries the DO NOT LOAD label.

The two review-fold rows are verbatim from coordinator agent-reports/architect-sh19-review-qs.md, not from the diffs; the diffs also keep the former lead text, as the record of 19c8dd18.

All test texts, and the changed texts of the existing tests, are taken verbatim from `design/FIX_sec-sh-sysw-plaintext-refuse-override.diff`. The nine
skipped tests and the skip reason stay as they are: "refused at load" is still the default, and driving them through the
override would only re-test R8.

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

## 6. RED and GREEN rows (go1.26.4 at `~/.local/go/bin`, `GOTOOLCHAIN=local CGO_ENABLED=0`, GOCACHE and TMPDIR outside the worktree)

The same command throughout: `go test -tags refugium -count=1 -run '^TestKitarch' -v ./gui/`

- **RED on 19c8dd18 plus the new test file** (`/scratch/cache/arch-sh19-q3/red.txt`), rc=1:
  - FAIL R1: 8 lines `want the refusal before any digest; got "Record2ofthispayloadisasecret,anditisnotsealed...withoutit.LoadPayload"` and 8 lines `no override screen after the refusal`.
  - FAIL R2 `want Record 1 named`.
  - FAIL sealed-public `no refusal drawn`.
  - FAIL R8 and R9 `no refusal drawn`.
  - PASS G1, G2 and the unit row.
- **RED P1, the behavioural RED.** 19c8dd18 with only the new body text planted (no screen), `/scratch/cache/arch-sh19-q3/p1.txt`, rc=1:
  - R1 FAILs 8 lines `<class> boot=<b>: no override screen after the refusal; got ""`.
  - Sealed-public, R8 and R9 FAIL `no override screen`.
- **GREEN**, the prototype (`green.txt`): 8 PASS, `ok seedhammer.com/gui`.
- **Plants on GREEN:**
  - P2: `Initial: 1`, so LOAD ANYWAY is the default. R1 FAILs 8 times `the flow went on past the default DO NOT LOAD: "PayloadDigest..."` plus `did not return` (`p2.txt`).
  - P3: Back loads. R9 FAILs `Back on the override screen went on to the digest` (`p3.txt`).
  - P4: the override is remembered in a package var. R8 FAILs at its second load `no refusal drawn; got "PayloadDigest..."` (`p4.txt`).
- **Gate on the prototype:**
  - `./scripts/refugium-tagged-test.sh 12`: `RESULT: ok -- 1318 passed, 68 stepped aside for the profile and 2 for the environment, 9 deferred-minor, of 1397 tests under -tags refugium` (`tagged.txt`).
  - Untagged `go test -run '^$' ./...` rc=0.
  - Untagged `-run 'Sysw|F1Offers|Kit' ./gui/` ok.
  - `GOOS=js GOARCH=wasm go vet -tags refugium ./cmd/emu/` rc=0.
  - gofmt clean on the changed files. The 3 gofmt hits in gui/transaction*.go exist on 19c8dd18 too.
  - Not run: the full untagged suite, TinyGo and elf-check (the rebuild owes them).
- **Review fold (architect-sh19-review-qs, on a9a47470), same command:**
  - RED, a9a47470 with only the test-file changes (`/scratch/cache/arch-sh19-rv/red-all.txt`), rc=1: 6 FAIL (R1, sealed-public, R8, R9 on the changed lead; R1c `DO NOT LOAD on LOAD AGAIN left a session (ended=true, ret=false, sysw=true)` and the same for Back; R9b on the lead), 4 PASS.
  - RED R9b, plant `Choices: []string{"LOAD ANYWAY", "DO NOT LOAD"}` on a9a47470 (`/scratch/cache/arch-sh19-rv/plant-r9c.txt`): `want DO NOT LOAD before LOAD ANYWAY (row 0 first)`, FAIL.
  - GREEN (`/scratch/cache/arch-sh19-rv/green.txt`): 10 PASS, `ok seedhammer.com/gui`.
  - `./scripts/refugium-tagged-test.sh 6`: `RESULT: ok -- 1320 passed, 68 stepped aside for the profile and 2 for the environment, 9 deferred-minor, of 1399 tests under -tags refugium` (`/scratch/cache/arch-sh19-rv/tagged.txt`).

## 7. Coverage map (rule -> rows)

| Rule | Rows |
|---|---|
| R1 refuse, both routes, no session, dismiss-and-drain | TestKitarchPlaintextSecretRefusedBeforeDigest (RED) |
| R1 a session already loaded is dropped (LOAD AGAIN, then DO NOT LOAD or Back) | TestKitarchLoadAgainDeclineLeavesNothingLoaded (RED) |
| R1 section-keyed (public section of a sealed payload) | TestKitarchSecretInTheCleartextSectionOfASealedPayload (unit), TestKitarchSealedPayloadWithASecretInItsPublicSectionIsRefused (RED) |
| R2 first position | TestKitarchPlaintextSecretRefusalNamesFirstSecretRecord (RED), R1's "Record 2" |
| R3 class not flag; F1 then KEEP / UNLOAD unchanged | TestKitarchPlaintextWithoutSecretClassStillLoads (guard, incl. the flow to "Keep this payload loaded?") |
| R4 pass: precedence | TestKitarchPassRecordRefusalKeepsPrecedence (guard) |
| R5 sealed section unchanged | TestRefugiumSecretWarningLine sealed rows, TestRefugiumLineSealedUnconfirmedMdmkAloneRaisesNone (existing) |
| R6 default unchanged | the untagged suite, incl. the nine changed tests running unskipped (existing) |
| R7 no session | R1's dismiss-and-drain plus `ctx.sysw == nil` |
| R8 override loads and warns | TestKitarchPlaintextSecretLoadAnywayLoadsAndWarns, plus the sealed-public flow test |
| R9 / default is DO NOT LOAD | TestKitarchPlaintextSecretOverrideBackLoadsNothing, TestKitarchPlaintextSecretOverrideLabelOrder (label of row 0; plant R9c), plus R1 (P2, P3) |
| per load, never remembered | R8's second load (P4) |

## 8. Gate (the repo's CI jobs, `.github/workflows/test.yml`; run once each to a file and grep it)

`go test -timeout 20m ./...`; `go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`;
`./scripts/test-32bit.sh`; `GOOS=js GOARCH=wasm go vet ./cmd/emu/` and the same with `-tags refugium`;
`./scripts/refugium-tagged-test.sh`; the TinyGo device builds (default and `-tags refugium`, nix) and
`scripts/refugium-elf-check.sh`, locally or as green CI on the head.

Historical, sh#19 as first built (the probe, before the override); the override's runs are §6's, and the rebuild re-runs this whole list: (each once, to a file, grepped): untagged `go test -timeout 20m ./...`: every
package ok (`ok seedhammer.com/gui 217.538s`) except `cmd/emu` `TestNothingImportsTheEmulator`, which failed only
because the author's TMPDIR was inside the worktree (its tree walk found a live `_testmain.go`); re-run with TMPDIR
outside the worktree: rc=0. oraclelive compile rc=0; both wasm vets rc=0; `./scripts/test-32bit.sh` rc=0;
`./scripts/refugium-tagged-test.sh 6` RESULT ok; TinyGo pico-plus2 default and `-tags refugium` builds rc=0
(refugium 1622040 flash bytes) and `./scripts/refugium-elf-check.sh` "RESULT: ok -- 11 markers present in the default
ELF and absent in the Refugium ELF". Keep TMPDIR and GOCACHE outside the worktree when re-running.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none. No `mr`/`md-cli` flag or printed text changes; the manual has no device-screen text for this
  flow (`git grep` of `docs/manual` at mnemonic-toolkit origin/master for "unencrypted in flash", "Payload Warnings",
  "secrets travel only": no hits).
- Spec lockstep, mnemonic-engrave `design/SPEC_systemwide_payloads.md` (origin/master c35e8613), landed with the build.
  Texts are confirmed (agent-reports/architect-sh19-q3-override.md Q4). Each is anchored by its quoted text at
  c35e8613; the former 16356ac1 line numbers were stale:
  - §3 third amendment (anchor: "in the Refugium build, a loaded payload (plaintext or sealed) holding a genuinely secret
    class gets one more warning line"), replace the paragraph with: `A third amendment, 2026-11-22 (§14f): **in the Refugium build, a payload holding a genuinely secret class outside a sealed section (any record of a plaintext payload, or one in a sealed payload's public section) is refused at load by default, and loads only if the operator chooses LOAD ANYWAY on an explicit override screen**; a loaded payload holding one (in a sealed section, or loaded anyway) gets one more warning line, which names the Sealed Payload as the only way secrets travel in that build (Brian's Q3 ruling, re-ruled 2026-11-22: refuse, with override). `ClassMr` is not secret and raises no flag but F3.`
  - §14a, the Q3 bullet (anchor: "- Q3: the Refugium build WARNS on a loaded payload"), replace it with: `- Q3 (re-ruled by Brian 2026-11-22, "Refuse, with override"): the Refugium build REFUSES at load, by default, a payload holding a record of a genuinely secret class outside a sealed section (every such record of a plaintext payload, and any in the public section of a sealed one), and loads it only when the operator chooses LOAD ANYWAY (§14f); it WARNS on a loaded payload holding one, in a sealed section (STOP-2, RULED (a): Brian 2026-11-22) or loaded anyway. Decision 6 is unchanged for the default build; §3.3.3 and §14f add the refusal, the override and the warning line.`
  - §14f first paragraph (anchor: up to "The line:"), replace it with: `In the Refugium build, a payload holding a record whose class is secret by §3.3.1 (`ClassMnemonic`, `ClassCodex32Secret`, `ClassPassphrase`, `ClassPreimage`, `ClassPhrase`) in its public section, which is every record of a plaintext payload, is refused at load by default: after the `pass:` refusal, before the digest, and before any session exists, with the title "Load Payload" and the text "Record <n> of this payload is a secret, and it is not sealed. This build takes secrets only in the Sealed Payload, so nothing was loaded. Pack the payload again without it, or load it anyway on the next screen." Here <n> is the first such record's position as §14g counts it. The override screen follows, titled "Unsealed Secret", asking "Load it anyway? Every program can then use its secret. It stays unencrypted in flash until you wipe it from the host." with the choices DO NOT LOAD (the default) and LOAD ANYWAY. DO NOT LOAD, or Back, ends the load with nothing loaded, also on a second load after an earlier LOAD ANYWAY (that payload is dropped). LOAD ANYWAY goes on as a load with no refusal. Every load asks again. When a loaded payload holds such a record, in its sealed section (STOP-2, RULED (a): Brian 2026-11-22) or loaded anyway, the "Payload Warnings" screen shows the line below: in a plaintext payload directly after the line "A SECRET is stored unencrypted in flash."; in a sealed payload as the screen's last line, after F2's line when F2 is raised, and the screen shows for this line alone when no flag is raised. The line:`
  - §14f (anchor: "The KEEP / UNLOAD choice follows the screen whenever this line is on it, and nothing is refused (Q3)." through "is unchanged."), replace that text with: `The KEEP / UNLOAD choice follows the screen whenever this line is on it. An md1/mk1 or mt1 record that is secret only because the device could not confirm it (`[mdmk-decode]`) is neither refused nor raises this line; its own F1 line is unchanged. The `pass:` refusal comes first and has no override. LOAD ANYWAY changes nothing else: the session is the same non-wiping session (decision 2).`
  - Test row 31 (anchor: "31. **(§14f)**"), replace the row with: `31. **(§14f)** In the Refugium build a payload holding a genuinely secret class outside a sealed section (plaintext, or a sealed payload's public section) is refused before the digest and then offered the override with DO NOT LOAD the default; DO NOT LOAD and Back leave nothing loaded, also on a second load after LOAD ANYWAY; LOAD ANYWAY reaches the digest, then the warning line (plaintext, directly after the F1 line; sealed, as the last line), then KEEP / UNLOAD, and a second load asks again. A sealed payload holding one only in its sealed section gets the line as the last line (alone on a strong-passphrase payload), then KEEP / UNLOAD; an unconfirmed md1/mk1 alone is neither refused nor raises it; the default build never refuses or shows it.`
  - §14k (anchor: "and keeps every kit row open" through "(§14f warns; nothing refuses);"), replace that text with: `itself. The build of FIX sec-sh-sysw-plaintext-refuse (Q3 re-ruled: refuse, with override, §14f) closes `kitarch-a-s1`, `kitarch-c-a1` and the seedhammer half of `kitarch-b-s1`; `kitarch-b-s1` stays open for its host half (`me sysw pack` still writes such a payload, decision 6);`
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

The override amendment's rows (§6) were run by the architect on the prototype (/scratch/cache/arch-sh19-q3/*.txt), and the fold re-ran RED and GREEN (/scratch/cache/fold-sh19-q3/{red,green}.txt). Earlier, for the probe: The r1 fold (Claude Sonnet 5.5, claude-sonnet-5-5) re-ran the probe's RED and GREEN rows on the folded probe (`/scratch/cache/fold-sysw-*.txt`). Original runs: RED rows run on base 127cf04 with only the new test file (output `/scratch/cache/sec-sh-sysw-red.txt`); GREEN rows and
the gate with the probe diff applied (`/scratch/cache/sec-sh-sysw-green.txt`, `-probe-tagged.txt` without the skips,
`-probe-tagged2.txt` with them, `-probe-untagged.txt`). The probe was reverted after the runs; the branch carries only
this document and the probe diff.
