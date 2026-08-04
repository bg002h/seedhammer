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
| `font/sh/` | upstream | Unlicense | none |
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

*(none yet)*
