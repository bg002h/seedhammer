# md/testdata/vectors/singlesig_* — key-bearing wallet-policy md1 goldens (T6a-1)

These are differential goldens for `md.EncodeSingleSig` + `md.WalletPolicyId`,
generated from the constellation source of truth. Unlike the template-only
`wpkh_basic`/etc. MANIFEST vectors (which carry `pubkeys:null` and CANNOT gate a
key-bearing wallet-policy encoder — R0-C1), each `singlesig_*` set is a FULL
WALLET POLICY (pubkeys + fingerprints TLV + explicit `path_decl.Shared` origin),
and each md1 is CHUNKED (the ~81-byte payload exceeds the 320-bit single-string
cap — R0-I1), so `.md1.txt` has one chunk per line (3 chunks each).

## Provenance (R0-M3)

- Source crate: `descriptor-mnemonic/crates/md-codec` v0.36.0, git `c85cd49`
  (`descriptor-mnemonic-md-cli-v0.7.0-5-gc85cd49`). The crates.io v0.36.0 used by
  the toolkit has the identical published tree (lockfile checksum
  `75b1bfb71335d439e10bcf5c1e6dacdd25da5eddd3c0051b4c6c6abf628804d6`).
- Generator: `mnemonic-toolkit` (binary v0.58.1), git `4e21d94`, linking
  md-codec 0.36.0 + ms-codec 0.4.4 (verified in its Cargo.lock — the pinned SHAs).
- Test seed: the standard BIP-39 "abandon" 12-word seed
  (`abandon abandon abandon abandon abandon abandon abandon abandon abandon
  abandon abandon about`). Its BIP-39 entropy is 16 zero bytes; its ms1 is
  `ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f` (the pinned zero-16 vector).
- Master fingerprint pin: `73c5da0a` (the abandon seed — R0-M3).

## How each set was produced

For each template T in {bip44, bip49, bip84, bip86}:

    mnemonic bundle --network mainnet --template T \
        --slot @0.phrase=<abandon seed> --json --no-engraving-card

→ `md1` (the 3 chunk strings, vendored verbatim in `.md1.txt`), `origin_path`,
`master_fingerprint`. The account xpub is recovered by `mnemonic inspect --json
--mk1 <both mk1 chunks>` (vendored in `.xpub.txt`). The xpub's 32-byte chain code
and 33-byte compressed pubkey (the components `EncodeSingleSig` takes) are the
base58 body's bytes [13:45] and [45:78]. The reassembled `encode_payload` bytes +
bit length, the `compute_wallet_policy_id` (full 16 bytes + the `[0:4]` stub), and
the `compute_md1_encoding_id` (the DISTINCT chunk-set-id source) were captured by
a throwaway Rust harness depending on md-codec 0.36.0.

## Script ↔ template map

| set                  | template | script   | origin        |
|----------------------|----------|----------|---------------|
| singlesig_pkh        | bip44    | pkh      | m/44'/0'/0'   |
| singlesig_sh_wpkh    | bip49    | sh(wpkh) | m/49'/0'/0'   |
| singlesig_wpkh       | bip84    | wpkh     | m/84'/0'/0'   |
| singlesig_tr         | bip86    | tr       | m/86'/0'/0'   |

## Files per set

- `<set>.md1.txt` — the chunked md1 (one codex32 chunk string per line, 3 lines).
- `<set>.xpub.txt` — the base58 account xpub.
- `<set>.meta.json` — template/script/fp/origin, the chaincode + compressed_pubkey
  hex, the `payload_bits` + `payload_hex` (the form-independent byte-parity gate),
  and `wallet_policy_id` / `wallet_policy_id_stub` / `md1_encoding_id`.
