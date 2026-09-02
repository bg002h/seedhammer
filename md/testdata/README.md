# md/testdata/vectors — vendored md-codec golden vectors

These are byte-exact cross-language golden vectors for the md1 encoder,
copied verbatim from the constellation source of truth:

- Source crate: `descriptor-mnemonic/crates/md-codec`
- Commit: `5a0a4f41` (md-codec v0.42.0, tag `md-cli-v0.13.0`)
- Path: `tests/vectors/<name>.{bytes.hex,phrase.txt,descriptor.json}`

Each vector has three files:

- `<name>.bytes.hex` — the exact `encode_payload` byte output (hex), the
  PRIMARY byte-parity gate for `encodePayload`.
- `<name>.phrase.txt` — the full md1 string. For single-string vectors this
  is the `encode_md1_string` output (the `encodeMD1String == .phrase.txt`
  gate). For the force-chunked vector (`wsh_multi_chunked`) it is a
  multi-chunk-format string prefixed with a `chunk-set-id:` header line, so
  it is EXCLUDED from the single-string parity table (R0-M3).
- `<name>.descriptor.json` — the AST used to build the Go `descriptor` input.

The original 10-entry MANIFEST (md-codec `src/test_vectors.rs`), including the
force-chunked `wsh_multi_chunked`, is vendored here. `chunked_md1_vector` (the
≥4-chunk 6-key wsh-sortedmulti from `me-cli/src/bundle.rs:547-585`) is NOT a
copyable file — it is hand-built in `chunk_test.go`.

Re-pinned 2026-08-14 (S0 D8, 0.36.0 → 0.42.0): all 30 files across these 10
vectors are byte-identical between the two commits — `cmp` confirms zero
drift in `.bytes.hex`, `.descriptor.json`, and `.phrase.txt` for every
vector. As of the new commit, `MANIFEST` in the primary is a canonical
**15-entry** corpus: the original 10 plus five Part-3 additions.

**All five additions are now vendored, and four of them are exercised.** D8 is
a *coverage* catch-up — the plan's own words are that the vendored set was "an
older, **smaller** sample, and a gate accepting them would prove agreement with
a subset of ourselves" — so re-pinning the provenance strings while leaving the
new vectors out would have delivered none of it. Note that
`md/testdata_test.go` enumerates vectors from a **hand-maintained list**, not a
directory scan, so copying files in without adding their names is a silent
no-op. Compare that list against the primary's `MANIFEST` on every re-pin.

| vector | state |
| --- | --- |
| `tr_with_leaf`, `nums_taproot`, `single_string_boundary` | exercised, single-string + byte parity |
| `wsh_sortedmulti_2chunk` | exercised, byte parity only — its `.phrase.txt` is a 3-line chunk-format string with a `chunk-set-id:` header, like `wsh_multi_chunked` |
| `sh_wpkh` | **vendored but NOT exercised.** This package refuses it: `md: missing explicit origin` (`md/md.go:893`), because its descriptor carries a pathless shared origin, `"path_decl":{"tag":"Shared","data":"m"}`. The primary gained pathless decode in the very release this re-pin points at. **The Go port is behind the Rust primary** — convergence work, tracked as F-166. Reproduce in one line: add `"sh_wpkh"` to `singleStringVectorNames` and run `go test ./md/`. |

## Keyed conformance vectors (R3, vendored 2026-08-20)

- Source: `descriptor-mnemonic` `b3b10f09`, `crates/md-codec/tests/vectors/`
- Names: `keyed_*` — 12 vectors, 5 files each including `.conformance.json`
  (`keyed_tr_sortedmulti_a` added 2026-08-20 with R5, from `75032c2f`;
  `keyed_tr_depth2_rightspine` added the same day from `b8663056` — the
  mirror of the left-heavy depth-2 tree, without which a tree-rebuilder
  mutation that ignores leaf depth passes)

These carry REAL xpubs (BIP-39's published "abandon … about" mnemonic at
`bip48-p2wsh` accounts 0..3, master fingerprint `73c5da0a` — never put funds
behind them), which is what the other 15 vectors could not do: every entry in
the primary's MANIFEST was keyless, so this port could agree with Rust about
every byte on the wire and still compute a different KEY-DEPENDENT identity.

`<name>.conformance.json` carries the template, path, keys, fingerprints,
`md1_encoding_id`, BOTH wallet ids, and per chain the canonical descriptor
string plus three addresses. `md/conformance_keyed_test.go` is the gate.

It found F-212 on its first run: Go and Rust compute different
`WalletPolicyId`s when the origin is ELIDED, and agree when it is explicit.
That gap is pinned by shape in the test, not skipped.

### REGENERATED 2026-08-20 with per-key origins (F-217)

Every multi-key keyed vector previously declared **one** key origin for
**several different** keys — `[73c5da0a/48'/0'/0'/2']` bound to two, three or
four distinct xpubs. BIP-32 is deterministic, so that pair names exactly one
key: the vectors pinned a wallet that cannot exist. Measured before the fix:
**9 contradictory, 0 consistent**; after: **0 and 11**.

Cause was `md encode --path`, which flattens per-key ("Divergent") origins to a
single shared one. The keys are genuinely accounts 0..3 of BIP-39's test seed,
so each now declares its TRUE origin `48'/0'/N'/2'` — written in the TEMPLATE
(`@0/48'/0'/0'/2'/<0;1>/*`), which is where md has always taken per-key origins,
and `path` is dropped on exactly those vectors because `--path` would flatten
back what the template just said.

Nothing here caught it, and nothing could have: addresses derive from the xpubs
a card CARRIES, never from the origin it declares, so every address in every
vector matched either way. `descriptor-mnemonic` `fe4b1ec9` adds the encoder
refusal and a corpus gate; the Go port needs no change to AGREE, since the wire
bytes and every identity moved together.

### `keyless_tr_with_leaf` (Stage 4, added 2026-08-20)

The same template as `keyed_tr_with_leaf` with **no keys**, so it lands on the
device's `expandUnsupported` branch carrying nothing an address could be derived
from. `gui/policy_address_test.go` uses it as the NEGATIVE case: the addresses
affordance must not be drawn, and the "display only" refusal must survive.

Without it that suite could pass while offering the addresses button to every
policy — including ones no address exists for.

Regenerate (`descriptor-mnemonic` at or after `bf028ad0`):

```sh
md encode "$(cat md/testdata/vectors/keyed_tr_with_leaf.template)" \
   --path "48'/0'/0'/2'" --force-chunked | grep '^md1' \
   > md/testdata/vectors/keyless_tr_with_leaf.phrase.txt
```

`--path` is required: the `tr()` wrapper has no canonical default origin, so
without it the card only PARTIAL-decodes. It carries no `.conformance.json` —
there are no addresses to conform to, which is the entire point of it.

### `keyed_tr_multi_a` (Stage 4, added 2026-08-20)

From `descriptor-mnemonic` `97d39e4b`, where it was added FIRST per the
Rust-primary rule. It is the corpus's only order-SENSITIVE tap leaf: every other
multi-key leaf is `sortedmulti_a`, which sorts on the derived keys and therefore
reads the same in any order, so "preserve the WRITTEN key order" was asserted by
nothing. A mutation reversing a leaf's key indices passed the whole suite.

It discriminates, and that was checked rather than assumed — `multi_a` and
`sortedmulti_a` over the SAME two keys give different addresses
(`bc1pf4auj…` vs `bc1p588jm…`). Had they matched, the written order would already
have been sorted order.

### `gap_tr_leaf_and_v` (Stage 4, added 2026-08-20)

`tr(@0/<0;1>/*,and_v(v:pk(@1/<0;1>/*),older(144)))` — a timelocked taproot leaf.
**A capability gap, vendored deliberately.** The card encodes, and the PRIMARY
derives its addresses; this port's tap-leaf emitter describes pk / multi_a /
sortedmulti_a only, so it refuses. `gui/policy_address_test.go` asserts the
refusal, which is what covers `complexAddressSource`'s derive probe — every
conformance vector derives, so deleting the probe broke nothing without this.

Rust's addresses are stored in its `.conformance.json` so that a future emitter
fix has ground truth. It is named `gap_*` rather than `keyed_*` on purpose: it is
NOT a conformance vector, and must not be swept into
`md/conformance_keyed_test.go`'s glob, which expects identities it has no reason
to carry. Pinned by shape — when the emitter grows `and_v`/`older` leaves the
test FAILS saying the gap is closed, rather than going quiet.

**Status 2026-09-02: CLOSED.** The paragraph above describes the gap as it was
filed. F-214's emitter grew `and_v`/`older` leaves, the pinned test failed with
"THE GAP IS CLOSED", the derived address matched the vendored one byte for byte,
and the test is now the positive `TestTheTimelockedTapLeafGapIsCLOSED`
(`gui/policy_address_test.go`). The vector stays, as the only timelocked tap leaf
in this repo.

## The compose corpus (wallet-policy composer, Stage 2)

The 26 `compose_*` / `keyed_compose_*` vectors are the Rust primary's
`MANIFEST` entries for the composer (descriptor-mnemonic
`crates/md-codec/tests/compose_support.rs::family()`), vendored by
`scripts/vendor-compose-vectors.sh` and pinned in
`compose_vectors.provenance.json` (checked by `md/compose_vectors_pin_test.go`).
They are all FORCE-CHUNKED, so they are deliberately absent from
`singleStringVectorNames`/`byteParityVectorNames`; their byte and chunk parity
is asserted by `md/compose_test.go` against the BUILDER (`md.Compose`), not
against a hand-loaded descriptor. Two further `family()` entries
(`compose_wsh_keyless_hash_path`, `compose_wsh_keyless_hash_only`) are
`no-corpus`: the primary's exporter refuses a signature-free path, so they are
mirrored as chunk-set literals in `md/compose_test.go`, produced by
`md compose ... --experimental | md encode --experimental --force-chunked`.

### `gap_wsh_andor` (composer Stage 2 fold, added 2026-09-02)

`wsh(andor(pk(@0),older(144),pk(@1)))`, keyed with the journey's cosigners @0
and @1 (fingerprint 73c5da0a), encoded by the Rust primary at descriptor-mnemonic
`66bdf2f4`:

    md encode --force-chunked "<the .template>" --key @0=<xpub> --key @1=<xpub> \
      --fingerprint @0=73c5da0a --fingerprint @1=73c5da0a

It exists because `gap_tr_leaf_pkh` stopped being a gap: Stage 2's `pk_h` arm
derives it (`gui/policy_address_test.go`, `TestThePkhTapLeafGapIsCLOSED`), and
`TestWalletPolicyConsentNeverHidesTheAbsenceOfAddresses` needed a KEYED shape the
emitter still refuses to show the "can't derive" consent wording on. `andor` is
that shape (`md/script_emit.go` has no `tagAndOr` arm). No `.conformance.json`:
nothing asserts its addresses, and the `keyed_*` globs do not enrol `gap_*`. When
an `andor` arm lands, this test fails; unlike its two predecessors this fixture
carries no vendored Rust addresses, so whoever closes the gap must first generate
`gap_wsh_andor.conformance.json` from the primary (`md address` at the pinned
commit) to have ground truth to be right against.

