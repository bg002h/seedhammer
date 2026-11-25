# Bug hunt 2026-11-25 (Fable): seedhammer

Report-only review of `seedhammer` at `e4fa676`, part of a constellation-wide hunt across all 11 bg002h repositories. Nothing in this repository was changed by the hunt; this file is the only addition.

**How it was done.** Eight Fable reviewers each audited an area (one of them the boundaries between repositories), reproducing what they could with locally built binaries or scratch tests that were not committed. Three further Fable verifiers then tried to disprove every High and Medium finding (and several Low ones) and re-graded them. Grades below are the verifiers' final grades where they ran.

**Grades.** Critical: can lose funds or leak a seed in ordinary use. High: wrong wallet, addresses or keys, or secret exposure, plausible in normal use, or recovery broken. Medium: needs an unusual but realistic input, or fails in a way a careful user might not notice. Low: fails closed, hardening, or a small exposure. Info: context, not a bug.

**Categories.** security (secret or key exposure, injection, integrity of what is trusted), funds (wrong address, amount, descriptor or recovery that could lose or strand coins), other.

**Status.** "Confirmed (reproduced)" means a command or test showed it. "Confirmed by reading" means the code path was traced but not executed. "Reviewer-confirmed, not re-verified" means the first reviewer reproduced or read it and no verifier re-checked it.

**Totals:** 1 High, 1 Medium, 4 Low. No Critical findings.

## Summary

| ID | Grade | Category | Status | Finding |
|---|---|---|---|---|
| SH#1 | High | funds | Confirmed (reproduced) | A legacy `c:pk_k`/`c:pk_h` md1 card shows a different (unspendable) address on the device than Rust derives |
| SH#2 | Medium | security | Confirmed (reproduced) | The policy summary (consent screen) lists one spend path for a policy with an `or` inside a branch |
| SH#3 | Low | security | Confirmed (reproduced) | Go `md.Decode` accepts four malformed-card shapes Rust refuses |
| SH#4 | Low | other | Confirmed (reproduced) | Go `mk.Encode` mints key cards mk-codec 0.6.0 refuses |
| SH#5 | Low | other | Reviewer-confirmed, not re-verified | Provenance pins behind the Rust crates; the lag list is incomplete |
| SH#6 | Low | other | Confirmed by reading | A key with no multipath gets a "change" list equal to its receive list |

IDs: `md#`, `mk#`, `ms#`, `toolkit#`, `G`, `refugium#`, `wallet#`, `E`, `T`, `SH#`, `M` are per-repository findings; `X` findings are at a boundary between repositories and appear in every report they touch.

## Findings

## SH#1: A legacy `c:pk_k`/`c:pk_h` md1 card shows a different (unspendable) address on the device than Rust derives

**Grade:** High · **Category:** funds · **Status:** Confirmed (reproduced)

**Reviewer write-up** (source: `seedhammer`)

### 1. A legacy `c:pk_k` / `c:pk_h` card derives a different address on the device than in Rust

**Severity: High. CONFIRMED.** Rust repo: descriptor-mnemonic (md-codec `to_miniscript.rs`).

**Evidence**

- `md/script_emit.go:158-181` (`tagPkK`) emits `<key> OP_CHECKSIG`, and `:183-204` (`tagPkH`) emits the full `DUP HASH160 <h> EQUALVERIFY CHECKSIG`: the wire's bare key already carries the implicit `c:`.
- `md/script_emit.go:206-216` (`tagCheck`) emits its child and then appends another `OP_CHECKSIG`, with no special case for a bare-key child.
- Rust `crates/md-codec/src/to_miniscript.rs:767-786` treats `Tag::Check` over a bare `PkK`/`PkH` as the same fragment as the bare tag ("Check-idempotence"), and says why: the toolkit walker in non-tap context and pre-v0.30 md-cli both emitted `Check(PkK/PkH)`, "and such cards are already engraved — the renderer must accept it". mnemonic-toolkit stopped emitting that shape in v0.55.0, 2026-06-12 (`crates/mnemonic-toolkit/src/parse_descriptor.rs:840-847`).
- The Go port is also inconsistent with itself: `md/tapleaves.go:114-119` (`describeTapLeaf`) unwraps `Check` over a key, while the emitter used for the address (`md/tapleaves.go:230`, same `emitFragment`) does not.
- Reached from the screens through `gui/policy_address.go:187-201` (`EmitWitnessScriptChunks`) and `:158-180` (tap leaves).

**Impact.** For any md1 card whose wire has `Check(PkK)` or `Check(PkH)` (a `pk()`/`pkh()` inside wsh/sh miniscript made by mnemonic-toolkit before 0.55.0 or by md-cli before 0.30), the device builds a script with a doubled `OP_CHECKSIG` and shows well-formed addresses that belong to no wallet the Rust tools, or any coordinator, derive. An operator checking a backup sees a mismatch on a correct card; an operator who takes a receive address from the device sends to a script that the wallet does not watch and that `<key> CHECKSIG CHECKSIG` cannot satisfy in segwit v0.

**Reproduction.** Policy `wsh(or_d(pk(@0),and_v(v:pk(@1),older(144))))`, two test xpubs at `m/48'/0'/0'/2'`, encoded twice: bare keys (card A) and `Check`-wrapped keys (card B).

```
$ cp work-sh/md_zz_bughunt2_test.go.txt md/zz_bughunt2_test.go
$ go test ./md/ -run TestBH_CheckPkK -count=1 -v
== bare PkK (current encoders)
   Go witness script: 2102752f…821dac 7364 21028722…091c ad 029000b268
   Go receive 0/0: bc1qhzn56dvpfvw4l5eq7lac2ap6v0jfsx4p222srpupjp0vymsfn3wsen6k8f
== Check(PkK) (legacy toolkit / pre-v0.30 md-cli wire)
   md1fxu4uzqpq2tvyyy4qqxpxps5fnfs4wqqqqqjq9uywy6gez58teec6zhqjdqpsyheqa3fyhu9dw
   md1fxu4uzqfgg5qrl27y8tyjd0ekdldrtwxelp5y3al7gj2lc8u8h33r6kspn8fqggp4mvx0lu3ss
   md1fxu4uzqs37zgcfswjwg6wst89herwvuykvflpqg5s2ucc9qk9mecpn9hc7g3ssz5epnrjua4hh
   md1fxu4uzq7gav2qdauztw4pzh92sdd4gv2344f3jnj6ey57j4uadwhdvq9lfqhsstnknmd5g4926
   md1fxu4uzp83q8c8wcfde2kqjl8urv5l3pac325mh2a4dsnacnx6a9mnxqqc3w5ugjfuflv3
   Go witness script: 2102752f…821d ac ac 7364 21028722…091c ac ad 029000b268
   Go receive 0/0: bc1qedfsrcvwzansndz8zmg0wtv3tx6atxt49hgrk8t8xxul4zq7zcpq4989qf
```

Rust, same five card-B strings:

```
$ md decode <B…>      -> wsh(or_d(pk(@0/<0;1>/*),and_v(v:pk(@1/<0;1>/*),older(144))))
$ md address <B…> --count 1
bc1qhzn56dvpfvw4l5eq7lac2ap6v0jfsx4p222srpupjp0vymsfn3wsen6k8f      (exit 0)
```

Rust gives `bc1qhzn56d…` for both cards; Go gives it for card A only and `bc1qedfsrc…` for card B.

**Fix direction.** In `emitFragment`'s `tagCheck` arm, when the child is a bare `tagPkK`/`tagPkH` key leaf, emit the child alone (as Rust does). Add a vector for the legacy wire in both wsh and tap-leaf position, pinned to Rust's address.

**Verifier re-check** (source: `verify-1`)

### seedhammer#1: legacy `c:pk_k` card gives a different address on the Go device

**CONFIRMED. High. funds.**

Re-ran the reviewer's test through an overlay: for the `Check(PkK)` wire form of `wsh(or_d(pk(@0),and_v(v:pk(@1),older(144))))`, Go's witness script contains `…821d ac ac 7364 …091c ac ad…` (doubled `OP_CHECKSIG`) and the address is `bc1qedfsrcvwzansndz8zmg0wtv3tx6atxt49hgrk8t8xxul4zq7zcpq4989qf`. Rust `md address` on the same five strings gives `bc1qhzn56dvpfvw4l5eq7lac2ap6v0jfsx4p222srpupjp0vymsfn3wsen6k8f`, the same as the bare-`PkK` card. Cause: `md/script_emit.go:206-216` appends `OP_CHECKSIG` after a child that already emitted one.

Do such cards exist? The Rust sources say so in two places: `md-codec/src/to_miniscript.rs:767-786` ("such cards are already engraved — the renderer must accept it") and the toolkit changelog for 0.55.0 (2026-06-12), which lists eight affected shapes (`wsh(pk)`, `wsh(pkh)`, `wsh(and_v(…pk…))`, `wsh(or_d(…pk…))`, `wsh(thresh(…pk…))`, `sh(pk)`, `sh(pkh)`, `sh(wsh(pk))`) and promises "Cards engraved by any prior toolkit version remain readable". How many are actually on steel I cannot measure.

Does the device show the address? Yes for a keyed card: `gui/md1_gather.go:226` calls `complexAddressSource`, which reaches `md.EmitWitnessScriptChunks` at `gui/policy_address.go:187-201` and feeds the address list screen. I found no gate that refuses a `Check`-over-key tree first. I did not find a path that engraves the address; it is displayed.

Grade held at High because the shown address is for a script that cannot be satisfied (the second `CHECKSIG` has nothing to consume), so coins sent to it would be unspendable, and the card is one the Rust primary accepts. The exposure is limited to legacy-wire cards with `pk()`/`pkh()` in wsh/sh miniscript; a user who only compares addresses sees a mismatch and no loss.

## SH#2: The policy summary (consent screen) lists one spend path for a policy with an `or` inside a branch

**Grade:** Medium · **Category:** security · **Status:** Confirmed (reproduced)

**Reviewer write-up** (source: `seedhammer`)

### 2. The policy summary lists one spend path for a policy that has an `or` inside a branch

**Severity: Medium. CONFIRMED (Go side run; Rust side by its own test).** Rust repo: descriptor-mnemonic (md-codec `policy_shape.rs`, fixed in 0.48.5).

**Evidence**

- `md/policy_shape.go:446-458`: `collect` walks `tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI` like a conjunction, merging both alternatives' keys and locks into one `Branch` and leaving `Complete` true. `splitBranches` (`:218-258`) only splits an `or` at the top of a branch; the file's own comment at `:292-299` notes that an `or` under `and_v` "is constructible and the wire carries it".
- Rust `crates/md-codec/src/policy_shape.rs:645-659`: a disjunction inside a branch returns `false` (summary incomplete, no branches), except `or_i`/`andor` with a `0` arm. Changelog, md-codec 0.48.5: "`policy_shape` no longer lists a wrong shape for a disjunction inside a branch". Rust test: `tests/bughunt.rs:514 bughunt_md_05b_every_nested_disjunction_tag_marks_incomplete`.
- Shown to the operator at `gui/template_engrave.go:151-196` (`policySummaryLines`, used by the template consent at `gui/multisig_build.go:808`) and the composer consent (`gui/composer_consent.go:167`).

**Impact.** For `wsh(and_v(v:pk(@0), or_d(pk(@1), older(52560))))` the consent screen prints `Spend path: 1` / `1: 2 key(s), custom +timelock`. That reads as "both keys and a timelock". The real policy lets `@0` spend alone once the timelock passes. This is exactly the partial summary the file's "honesty contract" forbids, shown before an engrave; Rust shows no summary for it.

**Reproduction.**

```
$ cp work-sh/md_zz_bughunt_test.go.txt md/zz_bughunt_test.go
$ go test ./md/ -run TestBH_PolicyShape -count=1 -v
SHAPE or_d-under-and_v: Complete=true branches=1 {K=0 N=0 Keys=2 Timelock=true}
SHAPE tag=0x15 Complete=true branches=1 (Rust: complete=false, branches=0)
SHAPE tag=0x16 Complete=true branches=1 (Rust: complete=false, branches=0)
SHAPE tag=0x17 Complete=true branches=1 (Rust: complete=false, branches=0)
SHAPE tag=0x18 Complete=true branches=1 (Rust: complete=false, branches=0)
SHAPE tag=0x19 Complete=true branches=1 (Rust: complete=false, branches=0)
```

The five tag rows are the same trees the Rust test builds. I did not run the Rust test (it needs a second build); the Rust behaviour is from its source and that test's assertion.

**Fix direction.** Port the 0.48.5 rule into `collect`: a disjunction tag returns `false` unless it is `or_i` with a `0` child or `andor` with a `0` third child.

**Verifier re-check** (source: `verify-3`)

### seedhammer#2 — the Go policy summary shows one spend path for a nested `or`

- Verdict: **CONFIRMED**. Grade: **Medium**. Category: **security** (misleading consent screen).
- Re-run in a scratch copy (`work-v3/sh`, `go test ./md/ -run TestBH_PolicyShape -count=1 -v`):
  `SHAPE or_d-under-and_v: Complete=true branches=1 {K=0 N=0 Keys=2 Timelock=true}`, and `Complete=true branches=1`
  for each of tags 0x15-0x19 under `and_v`.
- Go: `md/policy_shape.go:446-458` walks `tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI` as a conjunction. Rust:
  `descriptor-mnemonic/crates/md-codec/src/policy_shape.rs:645-659` returns `false` for a disjunction inside a branch
  (except the `0`-arm forms); fixed in 0.48.5. Rust side read, not run.
- Shown to the operator: `gui/template_engrave.go:151-196` prints `Spend path: 1` / `1: 2 key(s), custom +timelock`
  for `wsh(and_v(v:pk(@0),or_d(pk(@1),older(52560))))`, where `@0` spends alone after the lock. A policy supplied by
  someone else can hide a sole-spender path behind that line. Convergence port under the Rust-primary rule.

## SH#3: Go `md.Decode` accepts four malformed-card shapes Rust refuses

**Grade:** Low · **Category:** security · **Status:** Confirmed (reproduced)

**Reviewer write-up** (source: `seedhammer`)

### 3. `md.Decode` accepts four malformed-card shapes that Rust refuses

**Severity: Medium. CONFIRMED.** Rust repo: descriptor-mnemonic (md-codec `decode.rs`, `validate.rs`, `tlv.rs`).

**Evidence**

- `md/md.go:1206-1233` (`decodePayloadValidated`) runs placeholder usage, multipath consistency, tap-tree leaves, explicit origin, xpub bytes. Rust `decode_payload_with_opts` (`crates/md-codec/src/decode.rs`) additionally runs `validate_use_site_overrides_canonical` (`validate.rs:160-173`, since md-codec 0.37.0) and `validate_no_empty_origin_overrides` (`validate.rs:532-540`, since 0.42.0).
- `md/md.go:607-630` (`readTLV`) breaks out on a short tail (`:625`) without checking it; Rust `tlv.rs:232-235, 318-321, 337-350` calls `reject_non_zero_pad` (since 0.41.0).
- `md/bits.go:4-12` says the port tracks md-codec 0.47.0 and lists its known lags (F-166, F-654); none of these is listed. The use-site pair is mentioned as an optional gap in old design notes (`mnemonic-engrave/design/SPEC_seedhammer_template_engrave.md:38`); I found no record for the empty-origin or padding checks.

**Impact.** A card the device decodes, inspects and derives addresses from, but which every Rust tool refuses to decode. For the `@0` override the device even applies it: it derives `@0` from `<2;3>/*` in the test below. Current encoders never write these shapes, so this needs a hand-made or corrupted-then-miscorrected card; the cost is a steel card that looks verified on the device and cannot be restored with the tools.

**Reproduction.**

```
$ go test ./md/ -run TestBH_Decode_Divergences -count=1 -v
CASE1 baseline-override  md1yq802gggqpsqpvupgtqup0mzd8ztny0l
  Go Decode err=<nil> useSite[@0]="<2;3>/*"
CASE2 empty-origin-override  md1yq802gggqpsqvuqx24ty777vsm0p
  Go Decode err=<nil> origin[@0]="m"
CASE3 payload bits=58 symbols=12 pad bits=2
  clean md1yq802gggqpsqwgtua24e7ssf3 Go err=<nil>
  dirty md1yq802gggqpsphn65anvqptgep Go err=<nil>
CASE4 redundant-override  md1ypfdsssj5qqcy8ppgzcuqqcraaev3s8l3skq
  Go Decode err=<nil>
```

Rust `md decode` on the same strings:

```
md1yq802gggqpsqpvupgtqup0mzd8ztny0l      -> use-site override keyed on baseline @0; @0 cannot be overridden (exit 1)
md1yq802gggqpsqvuqx24ty777vsm0p          -> origin-path override for @0 is present but empty (zero components) (exit 1)
md1yq802gggqpsqwgtua24e7ssf3  (clean)    -> wpkh(@0/<0;1>/*) (exit 0)
md1yq802gggqpsphn65anvqptgep  (dirty)    -> malformed payload padding: 2 trailing pad bit(s) were not all zero (exit 1)
md1ypfdsssj5qqcy8ppgzcuqqcraaev3s8l3skq  -> redundant use-site override for @1; equals the baseline use-site path (exit 1)
```

**Fix direction.** Add the two validators to `decodePayloadValidated` in Rust's order, add the zero-pad check at both `readTLV` break points, and vendor Rust's negative vectors.

**Verifier re-check** (source: `verify-1`)

### seedhammer#3: Go `md.Decode` accepts malformed shapes

**CONFIRMED-but-downgraded. Low. security (integrity of what the device trusts).**

Overlay run: Go decodes all four cases with `err=<nil>` and applies the `@0` override (`useSite[@0]="<2;3>/*"`). Rust `md decode` refuses the same strings: "use-site override keyed on baseline @0", "origin-path override for @0 is present but empty", "malformed payload padding: 2 trailing pad bit(s) were not all zero", "redundant use-site override for @1". Downgraded because no shipping encoder writes these shapes and each string carries a valid BCH checksum, so reaching them needs a hand-built card; the stricter side (Rust) then refuses, which fails closed. It is a conformance and hardening gap under the Rust-primary rule.

## SH#4: Go `mk.Encode` mints key cards mk-codec 0.6.0 refuses

**Grade:** Low · **Category:** other · **Status:** Confirmed (reproduced)

**Reviewer write-up** (source: `seedhammer`)

### 4. `mk.Encode` mints key cards that mk-codec 0.6.0 refuses to mint

**Severity: Medium (low end). CONFIRMED on the Go side; Rust side from source and changelog.** Rust repo: mnemonic-key (mk-codec `bytecode/encode.rs`).

**Evidence**

- `mk/encode.go:81-131` (`encodeBytecode`) and `:154-180` (`compactFromXpub`) check stub count, depth and child number against the path, network, and fingerprint length only.
- Rust `crates/mk-codec/src/bytecode/encode.rs:123-155` (`check_key_rules`, mk-codec 0.6.0, 2026-11-22) also refuses: a depth-0 key with a non-zero parent fingerprint (`MasterParentFingerprintNonzero`), a depth-0 card whose origin fingerprint is not the key's own (`MasterFingerprintMismatch`), and a depth-1 card whose origin fingerprint is not the xpub's parent fingerprint (`OriginParentFingerprintMismatch`). `mk/mk.go:5` still names "mk-codec 0.2" as what the port tracks.
- Reachable where the fingerprint and xpub come from outside the device: the composer re-mint (`gui/composer_cards.go:41-53`) and `reStubMk1` (`gui/template_engrave.go:41-48`). The device's own derive paths pair the fingerprint with the key correctly.

**Impact.** The device can engrave an mk1 card whose stated origin fingerprint contradicts its own xpub. Rust decoders still read such a card (0.6.0 keeps decode unchanged), so nothing is lost, but the contradiction the Rust encoder now stops reaches steel through the Go port. The third 0.6.0 rule (path index below 2^31) is already met by `bip32.ParsePathElement`.

**Reproduction.**

```
$ cp work-sh/mk_zz_bughunt_test.go.txt mk/zz_bughunt_test.go
$ go test ./mk/ -run TestBH_ -count=1 -v
R3 depth-0, origin fp deadbeef != key fp: Go Encode err=<nil> strings=2
R4 depth-1, origin fp deadbeef != parent fp 5bf9f809: Go Encode err=<nil> strings=2
   Go Decode: fp=deadbeef path=m/0h err=<nil>
```

I did not build `mk`; the Rust refusals are read from `encode.rs` and `crates/mk-codec/CHANGELOG.md` ("Unreleased" block describing 0.6.0).

**Fix direction.** Port `check_key_rules` into `encodeBytecode` (including the all-zero-chain-code exception for the depth-0 rule) and update the provenance pin.

**Verifier re-check** (source: `verify-3`)

### seedhammer#4 — Go `mk.Encode` lacks mk-codec 0.6.0's origin refusals

- Verdict: **CONFIRMED-but-downgraded**. Grade: **Low**. Category: **other**.
- Re-run (`work-v3/sh`, `go test ./mk/ -run TestBH_ -count=1 -v`):
  `R3 depth-0, origin fp deadbeef != key fp: Go Encode err=<nil> strings=2`,
  `R4 depth-1, origin fp deadbeef != parent fp 5bf9f809: Go Encode err=<nil> strings=2`.
  Rust refuses both: `mnemonic-key/crates/mk-codec/src/bytecode/encode.rs:123-155` (`check_key_rules`, 0.6.0).
- Why Low: the device's own derive paths pair fingerprint and key correctly; only a re-mint of an imported card that
  already carries the contradiction reaches it (`gui/composer_cards.go:41-53`, `gui/template_engrave.go:41-48`).
  Decoders still read such a card; no key or address changes. Stale provenance pin (`mk/mk.go:5`, "mk-codec 0.2").

## SH#5: Provenance pins behind the Rust crates; the lag list is incomplete

**Grade:** Low · **Category:** other · **Status:** Reviewer-confirmed, not re-verified

**Reviewer write-up** (source: `seedhammer`)

### 5. Provenance pins are behind the Rust crates, and the lag list is incomplete

**Severity: Low. CONFIRMED (by reading).** Rust repos: descriptor-mnemonic, mnemonic-key, mnemonic-transaction.

- `md/bits.go:4-12`: tracks md-codec 0.47.0; Rust is 0.48.7. Findings 2 and 3 are lags it does not list (three of the four checks in finding 3 predate 0.47.0).
- `mk/mk.go:5`: "mk-codec 0.2"; Rust is 0.6.0 (finding 4).
- `mt/mt.go:8`: mt-codec 0.1.0; Rust is 0.2.0. I compared `mt.Decode` with the 0.2.0 changelog (count mismatch refused, non-ASCII refused) and found the Go port already at least as strict; only the pin text is stale.

**Impact.** The project rule is that each Go port carries a pin so drift can be audited. With the pins stale, findings 1 to 4 were not visible from the pins.

**Fix direction.** Re-pin after converging findings 1 to 4, and list any lag left open.

## SH#6: A key with no multipath gets a "change" list equal to its receive list

**Grade:** Low · **Category:** other · **Status:** Confirmed by reading

**Reviewer write-up** (source: `seedhammer`)

### 6. A key with no multipath gets a "change" list equal to its receive list

**Severity: Low. SUSPECTED (reading only, not run).** Rust repo: descriptor-mnemonic (md-codec 0.48.7).

- `gui/md1_expand.go:227-231` maps a bare `*` use-site to a single wildcard step, and `address/address.go:302-319` (`derivePubKey`) only consults `change` on a range step. So for a key written `@N/*`, `change=true` derives the same child as `change=false`.
- Rust 0.48.7 (`CHANGELOG.md`, "refuse a chain no key has") now returns `ChainIndexOutOfRange` for a chain at or beyond the widest multipath alt-count; before that fix Rust had the same behaviour.

**Impact.** On a card whose keys have no `<0;1>` multipath, the device's change addresses would repeat the receive addresses instead of being refused. I did not drive the address-list screen to confirm that it offers a change list for such a card, which is why this is suspected only.

**Fix direction.** Refuse `change=true` when no key in the policy has a multipath step, matching Rust.

**Verifier re-check** (source: `verify-1`)

### seedhammer#6: bare `*` keys get a change list equal to the receive list

**CONFIRMED by reading (not executed). Low. other.**

`gui/address_polish.go:49-125` always offers the receive/change toggle and titles the second list "Change addresses". `gui/md1_expand.go:227-231` maps a bare `*` use-site to one wildcard step, and `address/address.go:302-319` consults `change` only on a range step, so both lists derive the same children. Rust refuses the same request: `md address --template "wpkh(@0/*)" --key @0=$X0 --chain 1` prints `chain index 1 out of range; use-site multipath alt-count is 1`. The addresses shown do belong to the wallet, so this is a mislabel, not a wrong address.

---

_Generated by [Claude Code](https://claude.ai/code) on 2026-11-25. Findings were not fixed; each fix needs its own review under this repository's gates._
