# Provenance

What in this repository is ours, what came from elsewhere, and which of it
carries obligations someone else imposes on us.

**The default here is public domain.** Upstream SeedHammer released its firmware
under the Unlicense, this fork inherits that, and everything written for the fork
is dedicated the same way. Two font directories are the exception, and they are
the reason this file exists: a blanket "it's all public domain" claim about this
repo would be **false**.

## Summary

| Component | Origin | License | Obligations on us |
|---|---|---|---|
| Firmware base | [`seedhammer/seedhammer`](https://github.com/seedhammer/seedhammer) | Unlicense | none |
| Everything written for this fork | this repo | Unlicense | none |
| `font/constant/` | upstream, extended here | Unlicense | none |
| `font/sh/` | upstream, plus 14 glyphs from a fork — see *Imports* | Unlicense | none |
| **`font/comfortaa/`** | [Comfortaa](https://github.com/alexeiva/comfortaa) | **SIL OFL 1.1** | **yes — see below** |
| **`font/poppins/`** | [Poppins](https://github.com/itfoundry/Poppins) | **SIL OFL 1.1** | **yes — see below** |

## The two that bind us

`font/comfortaa/` and `font/poppins/` are **not** public domain. Each carries its
own `LICENSE` file, and under this repo's own terms — *"the files in this
repository are in the public domain as described in the LICENSE file, except
files in directories with their own LICENSE files"* — those directories are
governed by the OFL, not by the Unlicense.

Both are GUI screen fonts (TTF outlines rendered to bitmaps for the display).
Neither is an engraving font; nothing they contain reaches a plate.

**SIL Open Font License 1.1 requires that we:**

- keep the copyright notice and the OFL text with the font files — do not delete
  `font/comfortaa/LICENSE` or `font/poppins/LICENSE`, and do not relicense those
  directories;
- do not sell the font software on its own (bundled in this firmware is fine);
- keep any *modified* version under the OFL as well;
- respect the **Reserved Font Name**. Comfortaa reserves "Comfortaa". A modified
  version must not be distributed under the reserved name.

Compiled artefacts derived from them (`bold17.bin`, `bold10.bin`, `bold16.bin`,
`bold20.bin`, and their `.go` wrappers) are derivatives of the OFL fonts and
carry the same terms as the sources they are generated from.

## Rule for anything imported later

Before importing third-party code or font data, record it here with:

1. **Source** — repository URL and the exact commit or release taken from.
2. **License** — the actual terms, read from the source, not assumed from the
   project's headline license. A repo-level licence does **not** override a
   per-directory `LICENSE`, and this repo is itself the proof.
3. **What was taken** — the specific files or glyphs, so the boundary between
   ours and theirs stays greppable.
4. **Obligations**, if any, stated plainly.

Font data deserves particular care: a public-domain dedication is only as good as
the dedicator's right to make it, so check whether the data is original or traced
from another face before relying on the dedication.

## Imports

### `font/sh/` — 14 printable-ASCII glyphs

1. **Source** — [`Gangleri42/seedhammer`](https://github.com/Gangleri42/seedhammer),
   commit `cc7538d76cd64c545da96aa70f86bf755d929ea3` (`main`, "docs: the engrave
   screen is the plate confirm now"), file `font/sh/sh.svg`
   (sha256 `08eb541e92e25f9d91529f99471737160e6bebd73b09bfa8ae56f193097e685a`).
2. **License** — Unlicense. Read from that repo's root `LICENSE`, and checked
   the way this file demands: `font/sh/` there carries **no** `LICENSE` of its
   own, so the root terms do govern it. (`font/comfortaa/` and `font/poppins/`
   are the only per-directory licences in that tree, exactly as here.)
3. **What was taken** — 14 glyph elements, appended after `semicolon`, for the
   codepoints `font/sh` was missing:

   | rune | our element id | upstream element id |
   |---|---|---|
   | `!` | `exclam` | `exclamation` |
   | `"` | `quotedbl` | `quote` |
   | `$` | `dollar` | `dollar` |
   | `%` | `percent` | `percent` |
   | `&` | `ampersand` | `ampersand` |
   | `+` | `plus` | `plus` |
   | `=` | `equal` | `equals` |
   | `?` | `question` | `question` |
   | `\` | `backslash` | `backslash` |
   | `^` | `asciicircum` | `caret` |
   | `_` | `_` | `underscore` |
   | `` ` `` | `grave` | `grave` |
   | `\|` | `bar` | `bar` |
   | `~` | `asciitilde` | `tilde` |

   Six ids were renamed to the convention `cmd/vectorfont`'s `mapChar` already
   uses and `font/constant/constant.svg` already follows; the geometry is
   upstream's, untouched, except for `!` and `?` — see below.

   **The base glyphs were verified byte-identical to what this repo already
   had**: all 81 pre-existing glyphs decode to identical advances and identical
   B-spline knot lists, face metrics unchanged, 0 changed and 0 removed. The
   procedure, hashes and result are in `font/sh/import-check.md`.

   **Two glyphs were redrawn after import.** Upstream's `!` and `?` draw their
   baseline dot with a control point the motion planner turns into a 2638 mm/s³
   jerk against the machine's 2600 limit — the import landed red on
   `engrave.TestFonts`. Their dot is now drawn with a VARIANT of the form this
   face's `period` uses (`C…,4.9 …,4.9 …,5`) — **not `period`'s own**
   (`C…,5 …,4.9 …,5`), which is the shape upstream had. That puts both at
   2600.79, in line with `.` at 2600.39. **Copying `period` verbatim instead
   restores the 2638.22 red build**, so do not "simplify" this to match it. Same
   artwork, smoother path: ink bounds are byte-identical and only 3 of 34 control
   points move, by ≈2.7 µm at the 6.0 mm rung. Details in `import-check.md`.

   **Deliberately NOT taken** from the same upstream file: its rework of the
   existing `e o q s w O S W nine zero star at` glyphs, and (from
   `backup/backup.go`) its removal of the `qrc.Size > 33` guard on the seed
   path.
4. **Obligations** — none. Unlicense is a public-domain dedication; nothing to
   reproduce, attribute or preserve. This entry exists for traceability, not
   because the licence demands it.
