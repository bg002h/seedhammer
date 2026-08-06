#!/usr/bin/env bash
# variants.sh renders NUMBERED design options for one glyph of font/constant,
# as a cell sheet and as a word, and leaves the font exactly as it found it.
#
# Numbering is the point (operator, 2026-08-05): options are chosen by being
# talked about, and "the second one" is only unambiguous if the picture says 2.
# Option 1 is always the glyph AS IT IS, so every set has its own before.
#
# THE NUMBER IS THE OPTION, THE LETTER IS THE ROUND. A single round is plain
# 1, 2, 3. Iterate on those and the next round is 1b, 2b, 3b, so "3c" is the
# third option in the third round and means one thing only. Set ROUND=b to
# label a second round.
#
# Usage:
#   variants.sh GLYPH WORD 'points for option 2' ['points for option 3' ...]
#   ROUND=b variants.sh GLYPH WORD ...
#
# e.g.
#   variants.sh a 'a canal at malta' \
#     '307,3.5 310,4 311,5 311,8 307,8 307,6 311,6 ' \
#     '307,3.5 310,4 311,5 311,8.5 307,8 307,6 311,5.5 '
#
# THE FONT IS RESTORED ON EVERY EXIT PATH, including a failure part way through
# -- a half-applied variant left in constant.svg would be regenerated into
# constant.bin by the next build and engraved without anyone deciding to.
set -euo pipefail

cd "$(dirname "$0")/../.."
SVG=font/constant/constant.svg
OUT=${OUT:-/tmp/glyphvariants}
ROUND=${ROUND:-}
mkdir -p "$OUT"

restore() { git checkout -- font/constant/ 2>/dev/null || true
            nix develop --command go generate ./font/constant/ >/dev/null 2>&1 || true; }
trap restore EXIT

glyph=$1; word=$2; shift 2
id=${ID:-$glyph}

# The glyph's current points, read from the file rather than passed in, so an
# option can never be compared against a stale idea of the original.
orig=$(python3 -c "
import re,sys
s=open('$SVG').read()
m=re.search(r'<polyline id=\"%s\" class=\"st0\" points=\"([^\"]*)\"' % re.escape('$id'), s)
if not m: sys.exit('no polyline id=\"$id\" in $SVG (pass ID= if the SVG id is not the character)')
print(m.group(1))
")
echo "option 1 (as it is): $orig"

n=0
render() { # $1=label
  n=$((n+1))
  local id="$n$ROUND"
  nix develop --command go run ./cmd/glyphtrace -glyphs "$glyph" -cols 1 -px 640 \
    -label "OPTION $id - $1" -o "$OUT/cell-$n.png" 2>/dev/null
  nix develop --command go run ./cmd/glyphtrace -word "$word" -px 1500 \
    -label "OPTION $id - $1" -o "$OUT/word-$n.png" 2>/dev/null
  printf 'option %-3s ' "$id"
  # awk on an exact first field, NOT grep: the glyph may be a regex
  # metacharacter -- '*', '(', '{' and '$' are all on the work list -- and
  # `grep -E "^\Q..\E"` is Perl syntax that grep does not honour, so it fails
  # and takes the whole run down under `set -e`. Captions lead with the
  # character, so $1 is the glyph even for the named ones like "@ at".
  nix develop --command go run ./cmd/glyphtrace -counters 2>/dev/null |
    awk -v g="$glyph" '$1==g' | head -1 || true
}

apply() { python3 -c "
s=open('$SVG').read()
old='<polyline id=\"$id\" class=\"st0\" points=\"$orig\"/>'
assert old in s, 'anchor missing -- did the glyph change under us?'
open('$SVG','w').write(s.replace(old, '<polyline id=\"$id\" class=\"st0\" points=\"$1\"/>'))
"
  nix develop --command go generate ./font/constant/ >/dev/null 2>&1
}

if [ $(($# % 2)) -ne 0 ]; then
  echo "options must come in DESCRIPTION POINTS pairs; got $# argument(s)" >&2
  exit 2
fi

render "as it is"
while [ $# -gt 0 ]; do
  desc=$1; pts=$2; shift 2
  restore >/dev/null 2>&1
  apply "$pts"
  render "$desc"
done

magick "$OUT"/cell-*.png +append -bordercolor '#bbb' -border 2 "$OUT/$glyph-cells$ROUND.png"
magick "$OUT"/word-*.png -append -bordercolor '#bbb' -border 2 "$OUT/$glyph-words$ROUND.png"
rm -f "$OUT"/cell-*.png "$OUT"/word-*.png
echo "-> $OUT/$glyph-cells$ROUND.png"
echo "-> $OUT/$glyph-words$ROUND.png"
