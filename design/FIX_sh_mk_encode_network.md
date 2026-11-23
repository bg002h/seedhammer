# FIX: mk.Encode refuses a Card.Network that disagrees with the xpub version

**Small track** (one Opus review round). Grade I. Not class B; not secret handling (public key material only).
Key: bughunt-1119 `bh-sh-I10` (consultant report `design/agent-reports/bug-hunt-2026-11-19.md` L96-100; grader
coordinator `agent-reports/bughunt-1119/grade-sh.md` L39, L71, L80; triage `triage-sh-t3.md` L11).
Table row 32, coordinator `agent-reports/architect-bughunt-1119-questions.md` L144.
Ruling (decided, not re-ruled here): **T36** (same report L80): "the network comes from the xpub version; a non-empty
Card.Network that disagrees is refused. One source of truth." Library-only reach, graded I by P7 (L104).
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are at that
base (the bug-hunt lines, made at 9aec7a8, are unchanged in `mk/`).

## 1. The defect

`mk.Encode` (mk/encode.go:39) and its builder `encodeBytecode` (:75) never read `card.Network`. The wire carries only
the xpub's 4 version bytes (`compactFromXpub`, :142, admits `0488b21e` and `043587cf`), and `Decode` names the network
from them (`reconstructXpub`, mk/mk.go:383-398). So `Card{Network: "mainnet", Xpub: <tpub>}` encodes without error and
decodes as `Network: "testnet"`: the public encoder's round-trip guarantee (`Decode(Encode(card)) == card`, the doc
comment at :31-35) fails open, and an inspectable card can carry a network other than the one asked for. The same
holds the other way (`"testnet"` + xpub) and for any other string (`"signet"`, `"Mainnet"`, ...). `DerivedChunkSetID`
(:64) shares the builder and has the same gap.

Device reach: none today. Every in-tree caller passes a Network that matches its xpub (§4). The fix closes the
library gap for integrators (P7).

## 2. Spec delta (rules; §6 maps each to its rows)

- **R1 (refusal).** In `encodeBytecode`, when `card.Network` is non-empty and differs from the network named by the
  xpub's version bytes (`0488b21e` -> `"mainnet"`, `043587cf` -> `"testnet"`, exactly the names `Decode` returns),
  return a new error `errEncodeNetwork` and no bytes. `Encode` then returns `nil, errEncodeNetwork`;
  `DerivedChunkSetID` returns `0, errEncodeNetwork`.
- **R2 (exact match).** "Differs" is byte-for-byte string inequality: no case folding, no trimming, no aliases. So
  `"Mainnet"`, `" mainnet"`, `"bitcoin"`, `"TESTNET"`, and `"signet"`/`"regtest"` with a tpub are all refused. Reason:
  Card.Network's documented domain is `"mainnet" | "testnet"` (mk/mk.go:134) and `Decode` can return only those two,
  so any other non-empty value can never round-trip (T36 "one source of truth"). **Ruled (architect
  1122-late Q1): refuse; §8 Q1.**
- **R3 (empty and matching are unchanged).** An empty Network is accepted (T36 decides this; the grader's Q4) and the
  card decodes with the xpub's network. A matching Network encodes exactly as before: no output byte of any accepted
  card changes. A signet-derived key (tpub version) labelled `"testnet"` is a matching card.
- **R4 (DerivedChunkSetID).** Gets R1-R3 through the shared builder. Its two callers (gui/bundle.go:325,
  gui/mk1_inspect.go:275) pass a card fresh from `mk.Decode`, whose Network always matches, and already treat an
  error as "no warning", so they are unchanged.
- **R5 (order).** The network check runs immediately after `compactFromXpub` succeeds (after mk/encode.go:90-93) and
  before the fingerprint check (:99). So: an unparsable, private, depth/child-mismatched or unsupported-version xpub
  keeps `errEncodeXpub` / `errEncodeInvariant` (no network can be read from it); stub-count and path errors keep
  priority as today; a mismatch with a bad fingerprint returns `errEncodeNetwork`.
- **R6 (error text).** `errEncodeNetwork = errors.New("mk: network does not match the xpub version")`, unexported
  like its siblings at :15-22. **Ruled (architect 1122-late Q2).** The text reaches no screen: every gui caller maps an
  Encode error to its own fixed screen text, and `cmd/buildpayloadcards` (a dev tool, always mainnet) prints
  `mk.Encode <label>: <err>` only on a failure it cannot hit.
- **R7 (one source).** The version -> name mapping lives in one helper used by both `reconstructXpub` (Decode) and
  R1 (Encode), returning `""` for an unsupported version; `reconstructXpub` keeps its error text
  `mk: invalid xpub version: %x` byte-identical.

## 3. Holder-visible changes (complete list)

| surface | condition | before | after |
|---|---|---|---|
| `mk.Encode`, `mk.DerivedChunkSetID` (Go library API) | non-empty Card.Network != xpub-version network | Ok; card decodes with the xpub's network | error `mk: network does not match the xpub version`, no strings / id 0 |

No device screen, flag, verb, exit code or printed text changes (§4). No manual or README text describes
Card.Network or this encoder (`git grep -n -i 'card.network'` outside `*.go` and `design/agent-reports/`: none);
toolkit manual: no lockstep (seedhammer is not `mr`/`md-cli`).

## 4. Callers (checked, none changes behaviour)

| caller | Network passed | xpub network | why they agree |
|---|---|---|---|
| gui/derive_xpub.go:446-454 | `netName` | `net` from the same `pathPickerFlow` | one picker result |
| gui/singlesig_derive.go:84 | `networkName(net)` (:154-159) | `deriveAccountXpub(..., net, ...)` (:56) | same `net`; callers pass MainNetParams (gui/singlesig.go:182, gui/singlesig_verify.go:131) |
| gui/multisig_derive.go:48-49 | `"mainnet"` | `net` param | only caller chain passes `&chaincfg.MainNetParams` (gui/multisig_build.go:484-485 -> multisig_build_tail.go:107) |
| gui/composer_cards.go:41-53 | `"mainnet"`, or a decoded card's own Network | key records: sysw/composer_records.go:418 `IsForNet(MainNet)`; seeds: gui/composer_sources.go:353 MainNetParams; cards: composerCardIsMainnet (:166) admits `""`/`"mainnet"` only | gated mainnet |
| gui/template_engrave.go:48 | decoded card | same card | Decode output |
| cmd/buildpayloadcards/main.go:111-112 | `"mainnet"` | `chaincfg.MainNetParams` (:81) | constant |

## 5. Code changes (builder applies)

The scratch version is `design/FIX_sh_mk_encode_network-probe.diff` (evidence, not a patch to copy blindly):

1. mk/encode.go:15-22: add `errEncodeNetwork` with R6's text.
2. mk/mk.go: add `networkOfVersion(version []byte) string` (R7) next to `reconstructXpub`; `reconstructXpub`
   (:391-398) uses it and keeps its error text.
3. mk/encode.go, between :93 and :95: `if card.Network != "" && card.Network != networkOfVersion(compact[0:4])
   { return nil, errEncodeNetwork }`, with a comment naming this FIX, bh-sh-I10 and T36.
4. Comments that become stale: mk/encode.go:31-38 (Encode doc) gains, after the line `// codex32.ValidMK are the load-bearing correctness gates.`, a `//` line and then these four lines verbatim:
   `// Card.Network is checked, never serialized: the wire carries only the xpub's`
   `// version bytes. A non-empty Network must be exactly the name Decode gives those`
   `// bytes ("mainnet" for an xpub, "testnet" for a tpub, signet and regtest keys`
   `// included), else Encode returns an error; an empty Network takes the xpub's.` mk/mk.go:134 field comment: `// "mainnet" | "testnet"; Decode
   names it from the xpub version; Encode accepts "" or the matching name`; gui/multisig_derive.go:25 "it is not
   serialized nor verified" becomes "it is not serialized; mk.Encode refuses it if it disagrees with the xpub
   (T36)". gui/composer_cards.go:42 and gui/multisig_derive.go:49 "LABEL only" comments stay true (not serialized).

## 6. Tests

New file mk/fix_sh_mk_encode_network_test.go (function names are the TRACE row names), in the probe diff:

| row | rule | test | base (RED) | fix (GREEN) |
|---|---|---|---|---|
| 1 | R1 | TestMkEncodeNetworkMainnetLabelTpubRefused | FAIL `Encode(mainnet, tpub): err=<nil>, want errEncodeNetwork; Decode -> Network="testnet" err=<nil>` | PASS |
| 2 | R1 | TestMkEncodeNetworkTestnetLabelXpubRefused | FAIL `Encode(testnet, xpub): err=<nil>, want errEncodeNetwork; Decode -> Network="mainnet" err=<nil>` | PASS |
| 3 | R2 | TestMkEncodeNetworkOffVocabularyRefused (`Mainnet`, ` mainnet`, `bitcoin` with xpub; `signet`, `regtest`, `TESTNET` with a SigNet-derived tpub) | FAIL, six lines `Encode(Network="<v>"): err=<nil>, want errEncodeNetwork` | PASS |
| 4 | R3 | TestMkEncodeNetworkEmptyTakesXpubVersion (control) | PASS | PASS |
| 5 | R3 | TestMkEncodeNetworkMatchingRoundTrips (control: mainnet+xpub, testnet+tpub, testnet+SigNet tpub) | PASS | PASS |
| 6 | R4 | TestMkDerivedChunkSetIDNetworkMismatchRefused | FAIL `DerivedChunkSetID(mainnet, tpub) = 0x2b977, <nil>; want 0, errEncodeNetwork` | PASS |
| 7 | R5 | TestMkEncodeNetworkCheckOrder | FAIL `mismatch + bad fingerprint: err=mk: invalid fingerprint, want errEncodeNetwork` | PASS |
| 8 | R6 | TestMkEncodeNetworkErrorText | PASS once the sentinel exists (pins the text) | PASS |

RED was run on base plus the sentinel declaration alone (the test file does not compile without it):
`go test -count=1 -v -run 'TestMkEncodeNetwork|TestMkDerivedChunkSetIDNetwork' ./mk/` -> `FAIL seedhammer.com/mk`,
rows 1, 2, 3, 6, 7 failing with the texts above; rows 4, 5, 8 pass.

Existing tests that change: none. With the probe applied, `go test ./mk/` is `ok` (23 top-level tests) and the whole
module passes (61 non-gui packages `ok`; gui 1434/1434 over 6 verified-exhaustive shards; run before row 5's signet
case was added, a test-only change re-run in `./mk/`),
so no test relied on a mismatch (mk/compose_stubs_test.go:15,47 and mk/encode_test.go:76-200 all pass matching or
mainnet-on-xpub cards).

## 7. Gate

`CGO_ENABLED=0 go test -timeout 20m ./...` (CI test.yml:75; locally `gui` via
`mnemonic-engrave/scripts/gui-shard-test.sh ./gui/ 6 20m` and the rest with `go test -count=1 ./...` minus `gui`),
`gofmt -l mk` empty (base already lists three `gui/transaction*.go` files; not this FIX), `go vet ./mk/`. The CI jobs at test.yml:103-127 (oraclelive compile, GOOS=js vet) and the
TinyGo device build compile `mk`; the change adds no import, so they are expected unchanged; the builder runs them
or takes green CI on the head.

## 8. Questions for the architect (ruled 1122-late: Q1 (a) refuse, Q2 fixed text)

- **Q1 (R2, signet label).** T36 says "disagrees"; Card.Network's domain is two names. Options: (a) exact match, so
  `"signet"`/`"regtest"` with a tpub are refused (written above; round-trip holds; matches mk-codec's two
  `NetworkKind`s, `mk-codec/src/bytecode/xpub_compact.rs:56-67`); (b) also accept `"signet"`/`"regtest"` for a tpub
  (the card then decodes as `"testnet"`, so the round-trip guarantee breaks for those inputs). **Recommend (a).**
- **Q2 (R6 text).** PROPOSED `mk: network does not match the xpub version`. Alternative with values:
  `mk: network "mainnet" does not match the xpub version (testnet)`. **Recommend the fixed text** (sentinel, siblings
  carry no values, no screen shows it).

## 9. FOLLOWUPS

Closes none (bh-sh-I10 was rated 2 in October and never filed). Files none.
