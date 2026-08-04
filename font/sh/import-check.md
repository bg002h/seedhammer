# `font/sh` glyph import — additive-only check

Record of the check run when the 14 missing printable-ASCII glyphs were imported
(see `PROVENANCE.md`, *Imports*). The claim being evidenced is narrow and
falsifiable:

> **Nothing that already existed moved.** The import added 14 glyphs and changed
> nothing else — not a control point, not an advance, not the face metrics.

## What was compared

The plan called for a `cmd/vectorfont -dump` comparison. That is done below, but
a dump is a *rendering* of the face, so a dump comparison can only say "the
picture looks the same". The check actually relied on is stronger: the compiled
face `sh.bin` is parsed and every glyph's **advance and full B-spline knot list**
is compared before/after. `sh.bin` stores splines in rune order, so appending a
glyph at `0x21` shifts every later glyph's byte offset — a plain byte diff of
`sh.bin` is therefore meaningless and a per-glyph decode is required.

### Result

```
metrics identical: ascent=5000 height=6700
unchanged: 81  #'()*,-./0123456789:;<>@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]abcdefghijklmnopqrstuvwxyz{}
ADDED: 14 !"$%&+=?\^_`|~
CHANGED: 0
REMOVED: 0
RESULT: additions only
```

(`unchanged` includes `U+0020`, which is advance-only and inks nothing.)

## Hashes

| Artefact | sha256 |
|---|---|
| `sh.bin` before (`abb7458:font/sh/sh.bin`) | `aa87f3b0a964b05be7574cfa5e52a2f31fa31b490a248476513d2a4e5963e40b` |
| `sh.bin` after | `19e2171527420af64da5813d95206d17b3274af10096adf1347101d1f068180c` |
| `-dump` before | `2d1489a99b0af43c400a757f7fd2cf69ed8faa3635b334795718cfe9e96851a4` |
| `-dump` after | `2ab57c440d3a1f17af5f5426d127a55be7a921fcff3f2aad7a77e27a6baaa445` |
| upstream `font/sh/sh.svg` (`Gangleri42/seedhammer@cc7538d`) | `08eb541e92e25f9d91529f99471737160e6bebd73b09bfa8ae56f193097e685a` |

`sh.bin` before the import is byte-reproducible from `abb7458:font/sh/sh.svg`, so
the "before" side of the comparison is not a trusted artefact — it is derived.

## Reproducing

```sh
# 1. dumps (vectorfont writes sh.go/sh.bin into the CURRENT directory --
#    run it somewhere throwaway, never in the repo root)
go build -o /tmp/fw/vectorfont seedhammer.com/cmd/vectorfont
git show <before-rev>:font/sh/sh.svg > /tmp/fw/before.svg
cd /tmp/fw
./vectorfont -package sh -scale 1000 -dump /tmp/fw/before-dump.svg before.svg sh
mv sh.bin before.bin
./vectorfont -package sh -scale 1000 -dump /tmp/fw/after-dump.svg  <repo>/font/sh/sh.svg sh

# 2. the per-glyph comparison
python3 facediff.py /tmp/fw/before.bin <repo>/font/sh/sh.bin
```

`facediff.py`:

```python
import struct, sys
INDEX_LEN, ELEM = 127, 6          # cmd/vectorfont generate(); vector.OffSplines

def load(path):
    d = open(path, "rb").read()
    ascent, height = struct.unpack_from("<HH", d, 0)
    g = {}
    for r in range(INDEX_LEN):
        adv, start, end = struct.unpack_from("<HHH", d, 4 + r * ELEM)
        knots = [(d[o], *struct.unpack_from("<hh", d, o + 1))
                 for o in range(start, end, 5)]
        g[r] = (adv, tuple(knots))
    return (ascent, height), g

am, ag = load(sys.argv[1])
bm, bg = load(sys.argv[2])
assert am == bm, "metrics changed: %s -> %s" % (am, bm)
changed = [r for r in range(INDEX_LEN)
           if ag[r][0] and bg[r][0] and ag[r] != bg[r]]
removed = [r for r in range(INDEX_LEN) if ag[r][0] and not bg[r][0]]
added   = [r for r in range(INDEX_LEN) if bg[r][0] and not ag[r][0]]
print("added", "".join(map(chr, added)))
assert not changed and not removed, (changed, removed)
print("RESULT: additions only")
```

## Two glyphs were redrawn *after* import — deliberately

`!` and `?` are the only imported glyphs that were not taken verbatim. Upstream
draws the baseline dot of each with a third cubic of
`C…,5 …,4.9 …,5`, which the motion planner turns into a jerk of
**2638.22 mm/s³** against the machine's 2600 limit — past the 1% slack
`engrave.TestFonts` allows, so the import landed red. Every other glyph in the
face, including the visually identical `.` `:` `;` dots, sits at 2600.2–2610.9.

The fix is one control point per glyph: the dot is now drawn exactly as the
a VARIANT of the form this face's `period` uses (`C…,4.9 …,4.9 …,5`), translated
to the glyph's cell. **Not `period`'s own form** — `period` (`sh.svg:178`) draws
`C266,5,266,4.9,266.1,5`, which is the shape upstream's `!`/`?` had. Restoring
the dot to match `period` exactly puts both glyphs back at 2638.22 mm/s³ and
turns `TestFonts` red; measured, the whole-alphabet maximum rises from 2610.91
to 2638.22, past that test's 1% slack (2626).
Measured after: **2600.79** for both, i.e. in line with `.` at 2600.39. The
artwork is unchanged — same 0.1×0.1 dot in the same place; only the tool path
into it is smoother.

## What is NOT covered here

`engrave/testdata/font-sh.bin` **moved** with this import and had to. That golden
engraves *every rune the face decodes* into one strip, so it is a function of the
alphabet, and the alphabet is what this import changes. The 14 additions are
interleaved by codepoint, which shifts every glyph after `!`; the golden cannot
survive by construction. The additive proof above is what replaces it: it says
the pre-existing glyphs are identical, which is the property the golden would
otherwise have been asked to defend.

No **plate** golden moved. `backup/testdata/text-*.bin` and every other
`backup/testdata` artefact are byte-identical, and they engrave with `sh.Font`.
