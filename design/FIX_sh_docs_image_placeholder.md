# FIX: the install pages print `<image>` placeholders that a shell reads as redirections

**Small track** (one Opus review round). Grade I. Not a funds-path (Class B) row; no secret handling.
Key: bughunt-1119 `bh-sh-I12`. Table row 58, coordinator `agent-reports/architect-bughunt-1119-questions.md` L170
(`FIX-sh-docs-image-placeholder (sh) | I12 | I | small | none`). No T-ruling names this row. The architect ruled the
placeholder spelling (grade-sh.md Q6) a TEXT item, not a Brian question (questions report L29): the texts below are
PROPOSED (architect confirms). Grader fix line (grade-sh.md L73): "FIX-sh-docs-image-placeholder (bh-sh-I12).
Small-fix with holder-visible text". Consultant fix direction (`design/agent-reports/bug-hunt-2026-11-19.md` L112):
"Set a quoted shell variable to a concrete example filename, and use the quoted signed filename in the inspect and
load examples. Test the printed commands against argument recorders in a directory containing an `image` sentinel and
an image path with a space."
Author: Claude Opus 5.5 (claude-opus-5-5), 2026-11-22. Base: seedhammer `origin/main` 25abe8e. All anchors are at
that base.

## 1. The defect

`README.md` L166-168 (step 5 of the install route) and `docs/custom-firmware.md` L370, L389, L399 (§7) print, inside
` ```sh ` blocks:

    ../mnemonic-engrave/scripts/sign-firmware.sh <image>.uf2 ~/.sh2/sh2-boot-key.pem
    picotool load --verify <image>.signed.uf2

(custom-firmware.md: `/path/to/mnemonic-engrave/...` at L370, `picotool info -a <image>.signed.uf2 \` at L389,
the same load line at L399). A POSIX shell parses `<image>.uf2` as `<image` (stdin from a file named `image`) and
`>.uf2` (stdout to a file named `.uf2`). In a directory with no `image` file the line fails with
`image: No such file or directory`; with one present the command runs **without its file argument** and its stdout
truncates or creates the hidden file `.uf2` / `.signed.uf2`. Seen at base (§6): the signer recorder got one argument
(the key path only), picotool got `load --verify` with no file, and the directory gained `.signed.uf2` and `.uf2`.
The prose at custom-firmware.md L373 names the output `` `<image>.signed.uf2` `` (inline code, not run; changed for
consistency, R3).

## 2. Spec delta (rules; §7 maps each to its rows)

- **R1 (example name in a quoted variable). PROPOSED (architect confirms).** Each page sets the image's base name once,
  in the block that signs, right after `nix run .#build-firmware`, as the two lines

      # the name it printed after "Built ", without .uf2
      IMG="seedhammerii-v0.0.0-bg1a2b3c4"

  In custom-firmware.md the comment line is
  `# the name it printed after "Built ", without .uf2; later commands reuse $IMG`, because its signing, fingerprint
  and flashing commands are three blocks (one shell session). Grounds: `build-firmware` (flake.nix L125, L135)
  prints `Built seedhammerii-<VERSION>.uf2` with `VERSION` = `v0.0.0-bg<short sha>[-dirty]`; `sign-firmware.sh`
  (mnemonic-engrave `scripts/sign-firmware.sh` L50) writes `${IMG_IN%.uf2}.signed.uf2`. `1a2b3c4` is a made-up
  short sha: copied unedited, the signer refuses a missing file (fails closed); it never names a real build.
- **R2 (every use quoted).** Every printed command that names the image uses `"$IMG.uf2"` or `"$IMG.signed.uf2"`
  (double quotes around the whole word), so a name holding a space or shell metacharacters stays one argument:
  - README.md L167 -> `../mnemonic-engrave/scripts/sign-firmware.sh "$IMG.uf2" ~/.sh2/sh2-boot-key.pem &&`
  - README.md L168 -> `  picotool load --verify "$IMG.signed.uf2"` (chained with `&&`: the block is pasteable as one unit now,
    and `sign-firmware.sh` leaves `$IMG.signed.uf2` behind when a check after its L51 copy fails, so the flash
    must run only when the signer exits 0; review r1 I1, PROPOSED)
  - custom-firmware.md L370 -> `/path/to/mnemonic-engrave/scripts/sign-firmware.sh "$IMG.uf2" ~/.sh2/sh2-boot-key.pem`
  - custom-firmware.md L389 -> `picotool info -a "$IMG.signed.uf2" \` (the L390 pipeline is unchanged)
  - custom-firmware.md L399 -> `picotool load --verify "$IMG.signed.uf2"`
- **R3 (prose).** custom-firmware.md L373: `` `sign-firmware.sh` writes `<image>.signed.uf2` and `` ->
  `` `sign-firmware.sh` writes `$IMG.signed.uf2` and `` (rest of the sentence unchanged). PROPOSED.
- **R4 (no other placeholder in a run line).** No line inside a ` ```sh ` block of README.md or
  docs/custom-firmware.md carries a `<word>` placeholder outside a `#` comment. The comment at custom-firmware.md
  L366-367 (`# -> seedhammerii-<version>.uf2, named v0.0.0-bg<sha> ...`) stays: a comment is not parsed for
  redirections. `/path/to/` at L370 stays (a path, not a metacharacter).
- **R5 (unchanged).** Every other line of both pages is unchanged; no code, flag or exit code changes. README L196-200
  (the upstream `$ nix run .#build-firmware` block) is not a signing block and is unchanged.

Exact new blocks (PROPOSED):

README.md L165-169 becomes

    ```sh
    nix run .#build-firmware
    # the name it printed after "Built ", without .uf2
    IMG="seedhammerii-v0.0.0-bg1a2b3c4"
    ../mnemonic-engrave/scripts/sign-firmware.sh "$IMG.uf2" ~/.sh2/sh2-boot-key.pem &&
      picotool load --verify "$IMG.signed.uf2"
    ```

custom-firmware.md L363-371 becomes (new lines 368-370 inserted after the existing L367 comment, before `# sign it`)

    ```sh
    # from this repo (needs Nix with flakes)
    nix run .#build-firmware
    # -> seedhammerii-<version>.uf2, named v0.0.0-bg<sha> by flake.nix -- NOT by
    #    `git describe`, which would inherit upstream's tags and read as official

    # the name it printed after "Built ", without .uf2; later commands reuse $IMG
    IMG="seedhammerii-v0.0.0-bg1a2b3c4"

    # sign it
    /path/to/mnemonic-engrave/scripts/sign-firmware.sh "$IMG.uf2" ~/.sh2/sh2-boot-key.pem
    ```

## 3. Holder-visible changes (complete list)

| where | before | after |
|---|---|---|
| README.md L166-168 block | `<image>.uf2`, `<image>.signed.uf2` | + comment and `IMG="seedhammerii-v0.0.0-bg1a2b3c4"`; `"$IMG.uf2"`, `"$IMG.signed.uf2"` |
| custom-firmware.md §7 first block | `<image>.uf2` | + blank, comment, `IMG=...`, blank; `"$IMG.uf2"` |
| custom-firmware.md L373 prose | `` `<image>.signed.uf2` `` | `` `$IMG.signed.uf2` `` |
| custom-firmware.md L389, L399 | `<image>.signed.uf2` | `"$IMG.signed.uf2"` |

No CLI output, exit code or flag changes. Toolkit manual: none (no `mr`/`md-cli` text; the manual does not print
these commands).

## 4. Changes (builder applies)

`README.md` and `docs/custom-firmware.md` only, exactly as §2. The scratch version is
`design/FIX_sh_docs_image_placeholder-probe.diff` (doc edits plus the §5 test; evidence, applies with `git apply` at
base 25abe8e).

## 5. Tests

New file `docs/fix_sh_docs_image_placeholder_test.go`, package `docs_test` (a test-only package in `docs/`, picked up
by CI's `go test ./...`; no `.github/` change). Test function names are the TRACE row names. The test reads
`../README.md` and `custom-firmware.md`, takes every ` ```sh ` block that names `sign-firmware.sh` or a
`.signed.uf2` file (README: one block; custom-firmware.md: the three §7 blocks, concatenated in page order as one
shell session), rewrites the two known signer prefixes (`../mnemonic-engrave/scripts/sign-firmware.sh `,
`/path/to/mnemonic-engrave/scripts/sign-firmware.sh `) to a recorder function, and runs the lines under
`bash --noprofile --norc -c` in a fresh temp directory that holds a file `image` containing `SENTINEL\n`, with a temp
`HOME`. Recorders (`nix`, the signer, `picotool`) append `<name> argc=<n> [arg]...` to a log outside the directory;
`awk`, `head`, `xxd`, `sha256sum` are stubs that drain stdin. A missing `bash` is a test failure, not a skip.

- `TestFixShDocsImagePlaceholderVerbatim` (R1, R2): each page run as printed; bash exits 0; the log is exactly
  `nix argc=2 [run] [.#build-firmware]`, `sign-firmware.sh argc=2 [seedhammerii-v0.0.0-bg1a2b3c4.uf2]
  [HOME/.sh2/sh2-boot-key.pem]`, (custom-firmware.md only) `picotool argc=3 [info] [-a]
  [seedhammerii-v0.0.0-bg1a2b3c4.signed.uf2]`, `picotool argc=3 [load] [--verify]
  [seedhammerii-v0.0.0-bg1a2b3c4.signed.uf2]`; afterwards the directory holds only `image`, still `SENTINEL\n`.
- `TestFixShDocsImagePlaceholderSpacedName` (R1, R2): exactly one line per page matches `^IMG="[^"$`\\]*"` and its
  value is `seedhammerii-v0.0.0-bg1a2b3c4`; with that value replaced by `my image`, the same log with `my image.uf2` /
  `my image.signed.uf2` as single arguments, and the same directory check.
- `TestFixShDocsImagePlaceholderNoAngleWords` (R4): no ` ```sh ` line of either page matches `<[A-Za-z][A-Za-z0-9_-]*>`
  after removing a `#` comment (a `#` at line start or after a space).

- `TestFixShDocsImagePlaceholderSignFailNoFlash` (R2): README's signing block run with the signer recorder
  redefined to exit 1 (`sign_firmware() { rec sign-firmware.sh "$@"; return 1; }` prepended); fails if any
  recorded call starts with `picotool argc=3 [load]`.

**Tests that change:** none (no existing test reads these pages).

## 6. RED rows (run on base 25abe8e with the §5 test added; signet not involved, no keys)

Command (Go 1.26.7, `GOTOOLCHAIN=local`, own `GOCACHE`):
`CGO_ENABLED=0 go test -count=1 -v -run 'TestFixShDocsImagePlaceholder' ./docs/`

| row | at base (seen) | after the fix (seen on the scratch fix) |
|---|---|---|
| TestFixShDocsImagePlaceholderVerbatim | FAIL x4: README log `sign-firmware.sh argc=1 [HOME/.sh2/sh2-boot-key.pem]`, `picotool argc=2 [load] [--verify]`; custom-firmware.md also `picotool argc=2 [info] [-a]`; both pages: `working directory after the printed commands = [".signed.uf2" ".uf2" "image"]` | PASS |
| TestFixShDocsImagePlaceholderSpacedName | FAIL x2: `../README.md: 0 lines assign IMG="..." in the signing blocks, want 1`, same for custom-firmware.md | PASS |
| TestFixShDocsImagePlaceholderNoAngleWords | FAIL x5: `../README.md:167`, `:168`, `custom-firmware.md:370`, `:389`, `:399` `placeholder outside a comment in a sh block` | PASS |
| TestFixShDocsImagePlaceholderSignFailNoFlash | PASS at base (the `<image` redirection stops the load line); FAIL on the probe as first written (`signer failed but the block still ran picotool argc=3 [load] [--verify] [seedhammerii-v0.0.0-bg1a2b3c4.signed.uf2]`) | PASS |

Mutation seen on the scratch fix: unquoting README's load line (`picotool load --verify $IMG.signed.uf2`) fails
TestFixShDocsImagePlaceholderSpacedName (picotool gets `my` and `image.signed.uf2`). An unquoted assignment
(`IMG=seedhammerii-...`) fails it too (0 matching assignment lines).

## 7. Coverage map (rule -> rows)

| rule | rows |
|---|---|
| R1 example name in a quoted variable, set once per page | TestFixShDocsImagePlaceholderVerbatim; TestFixShDocsImagePlaceholderSpacedName (value pinned, one assignment) |
| R2 every use quoted, one argument; README flash only after a successful signer | TestFixShDocsImagePlaceholderVerbatim (sentinel dir, no hidden file); TestFixShDocsImagePlaceholderSpacedName; TestFixShDocsImagePlaceholderSignFailNoFlash |
| R3 prose | review only (inline code in prose is not run) |
| R4 no `<word>` in a run line | TestFixShDocsImagePlaceholderNoAngleWords |
| R5 unchanged | `git diff --stat` touches only the two pages and the new test; review |

## 8. Gate (the repo's CI jobs; run once each to a file and grep it)

`CGO_ENABLED=0 go test -timeout 20m ./...` (gui may be sharded with mnemonic-engrave `scripts/gui-shard-test.sh ./gui/ 12`);
`CGO_ENABLED=0 go test -tags oraclelive -run '^$' ./oracle/ ./gui/ ./sysw/ ./cmd/emu/`; `./scripts/test-32bit.sh`;
`GOOS=js GOARCH=wasm go vet ./cmd/emu/` and `-tags refugium`; `./scripts/refugium-tagged-test.sh`; both TinyGo
device builds and `scripts/refugium-elf-check.sh` (CI). Measured on the scratch fix by the author (go1.26.7,
/scratch/code/shibboleth/.toolchain/go): `go test -v ./docs/` ok (3 tests PASS); `GOARCH=386 go test ./docs/` ok;
`go vet ./docs/` ok; `go build ./...` ok; gofmt clean. Not run by the author: the rest of `./...` and the other jobs
(no code changes; the new package imports nothing from the repo). The builder runs the whole gate.

## 9. Lockstep and FOLLOWUPS

- Toolkit manual: none.
- Closes: bh-sh-I12 with the GREEN merge. seedhammer has no `design/FOLLOWUPS.md`; nothing to close there.
- Files (proposed in the author's report, for mnemonic-engrave `design/FOLLOWUPS.md` after merge): the same `<word>`
  shell-placeholder pattern in seedhammer developer notes, out of this row: `font/sh/import-check.md` L52, L56, L59
  and `slip39/testdata/GEN.md` L30.

## 10. Source files this FIX changes (build ordering)

`README.md`, `docs/custom-firmware.md`, new `docs/fix_sh_docs_image_placeholder_test.go`. No Go source outside the new
test; no overlap with the open sh FIX branches (each changes only its own `design/FIX_*` files so far).
