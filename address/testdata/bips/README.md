# address/testdata/bips — vendored published BIP test vectors

Ground truth from outside this project, for `bip_vectors_test.go`.

The rest of `address_test.go` asserts real derived addresses whose fixtures cite
no source, so it proves this device agrees with itself. These files are the
oracle that does not.

## Provenance

Copied verbatim, byte for byte, from the BIPs repository:

- Source repo: `bitcoin/bips`
- Commit: `60f5b33b0a7be3cf09b933d97b78071d684db7d1`
- Path: `bip-XXXX.mediawiki` (repository root)
- Fetched as `https://raw.githubusercontent.com/bitcoin/bips/<commit>/bip-XXXX.mediawiki`

Each file's SHA-256 is pinned in `bipVectorSources` in `bip_vectors_test.go`
and checked on every read, so an edited or substituted oracle is a test failure
rather than a silent change of ground truth. **Re-pinning to a newer commit
means updating the commit above and all three hashes.**

    4cc48c5c159c05585962a8eb264b05ccb4ad710b1a16c870232e0f0eb1428991  bip-0067.mediawiki
    1900feec6cafca65b8c09906ca0658d2d742b4c9b44cb15678996985b6bfe627  bip-0084.mediawiki
    d8d01dee331da07c2562615bc1f064c1868ec3fce61184973da76d1196c7f5b0  bip-0086.mediawiki
    62bc71351563e68baeb12643c68355d217953ae9eb6a6e68b2b0323275b6beec  bip-0143.mediawiki
    54d752399568838555d6224f271ed9f2875f16628396c3e0d4c60543bc81ad21  bip-0383.mediawiki

The whole document is vendored rather than an extracted excerpt, so the hash
above is checkable against upstream by anyone, and so the surrounding prose that
says what a vector *means* travels with the vector. The tests parse the vector
sections out of the mediawiki directly — there is no generated intermediate to
drift out of date — and each parser asserts the exact number of vectors it
found, so a parser that silently matches nothing fails instead of passing
vacuously.

## Per file — what it supplies, and what it does NOT

### `bip-0383.mediawiki` — `multi()` / `sortedmulti()`

Publishes **descriptor → output script hex** pairs. Descriptors over derived
child keys list the 0th, 1st and 2nd scripts. 9 valid descriptor entries.

Used: the one `sortedmulti()` vector over two xpubs, whose three scripts pin key
ordering *after* derivation — the device's actual path.

- **Publishes no addresses at all.** Not one, of any type.
- **Publishes no `wsh(sortedmulti(...))` vector** — `grep -c 'wsh(sortedmulti'`
  is 0 — and that is precisely the shape this device builds. Its `wsh(...)`
  vectors are all `multi()`, which `bip380` refuses by design (two enum values;
  `Parse` accepts only the literal `"sortedmulti"`; `address.go` sorts
  unconditionally). So the device's own output shape cannot be quoted end to end
  from any BIP; `TestBip383SortedMultiScriptMatchesPublishedVectors` composes it
  from the published script plus a local `0020||sha256` bech32 wrap, and says so
  in the test.
- Its other `sortedmulti()` vector is over raw keys, one of them uncompressed,
  which no descriptor this device parses can carry.

### `bip-0067.mediawiki` — deterministic key ordering

Publishes **four** fields per vector: List (unsorted) · Sorted · Script ·
Address. **4 vectors, 4 P2SH addresses** — not the 5 an earlier recon reported.

Used: all four, end to end. This is the only file here from which the device's
sort, script construction and final address can all be quoted with nothing
derived. Vector 2 is already sorted (a genuine no-op); vector 3's keys differ
only in the `02`/`03` prefix and the final byte, which is what a comparator
sorting on the wrong bytes gets wrong.

- Keys are raw public keys, not xpubs, so these vectors exercise the comparator
  and script construction, not derivation. Reaching them needs the
  `sortedMultisigScript` seam in `address.go`.

### `bip-0143.mediawiki` — the P2SH-P2WSH nesting

§P2SH-P2WSH publishes a concrete 6-of-6 multisig with all three layers:
`scriptPubKey`, `redeemScript`, `witnessScript`.

Used: the layer chain, to pin `sh(wsh(...))` nesting.

- Its witnessScript is an **unsorted** `multi`, so it pins the nesting algebra
  only — never ordering.
- **This replaces a BIP-141 citation that does not hold.** BIP-141 publishes no
  test vectors: every example in it is a structural template, and
  `grep -cE '[0-9a-f]{40,}'` over the whole document returns 0. There was
  nothing in it to quote, and nothing to derive from either.

### `bip-0084.mediawiki` — `wpkh`, and `bip-0086.mediawiki` — `tr`

Both publish, from the standard `abandon abandon … about` mnemonic, an
**account-level extended public key** plus the first two receive addresses and
the first change address under it. BIP-86 publishes each receive
**scriptPubKey** too. All mainnet.

Used: both, end to end, nothing derived. They are the only BIPs that publish an
address for a shape this device's descriptors can name directly, and because
they give the account key rather than a root key, the descriptor is
`wpkh(<published zpub>)` / `tr(<published xpub>)` with no invented text at all.

A side effect worth naming: this package defaults an unqualified key to
`<0;1>/*`, so `Receive(i)` is `.../0/i` and `Change(i)` is `.../1/i`. That is
exactly what these vectors publish, so the **default itself is now pinned
against published bytes** rather than only against itself.

## Not vendored, and why

- **BIP-44** — `pkh`, one of the two singlesig shapes left unanchored. It
  publishes **no test vectors of any kind**: no keys, no addresses, nothing to
  quote.
- **BIP-49** — `sh(wpkh)`, the other one, unanchored for a different reason. Its
  vectors exist and are complete, and they are **testnet**: a `upub` account key
  and a `2Mww8…` address. `bip380.ParseExtendedKey` rejects the `upub` version
  outright — it is not in the accepted set — so reaching them means rewriting
  the SLIP-132 version bytes to a `tpub` first. That is a legitimate and
  checkable transformation, and it is deliberately **not** done here: a
  conversion invented by a test is exactly the sort of step that quietly becomes
  the thing being tested. Filed rather than smuggled in.
- **BIP-48** publishes no vectors at all — its Examples table is path semantics
  with no keys. **No published vector pins `m/48'` derivation**, so nothing here
  should be read as doing so.
- **BIP-382** is `wsh()` alone and contains no `multi(`; the `wsh(multi(...))`
  vectors belong to 383.
- **BIP-39** mnemonic→seed vectors are already exercised elsewhere in the tree.
