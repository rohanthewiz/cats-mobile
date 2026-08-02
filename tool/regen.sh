#!/usr/bin/env bash
# Regenerate the Dart wire layer from a cats checkout, and re-pin CATS_REV.
#
# Kept as a script rather than a Makefile target because the whole job is two
# commands and one of them needs a path the caller supplies. It exists so the
# path is spelled the same way every time — a regeneration into the wrong
# directory silently leaves the old files in place and the new ones somewhere
# nobody looks.
#
#   tool/regen.sh [path-to-cats]        # default ../cats
#
# FLUTTER_ROOT, when set, also regenerates keys.g.dart from the SDK's own
# physical_key_data.g.json. That input changes only on an SDK upgrade, so the
# common case leaves the committed table alone.
set -euo pipefail

cats_dir="${1:-$(cd "$(dirname "$0")/../.." && pwd)/cats}"
out_dir="$(cd "$(dirname "$0")/.." && pwd)/packages/catsproto/lib/src/generated"

if [[ ! -f "$cats_dir/go.mod" ]]; then
  echo "no cats checkout at $cats_dir" >&2
  echo "usage: tool/regen.sh [path-to-cats]" >&2
  exit 1
fi

args=(-out "$out_dir")
if [[ -n "${FLUTTER_ROOT:-}" ]]; then
  args+=(-flutter-root "$FLUTTER_ROOT")
else
  echo "note: FLUTTER_ROOT unset — keys.g.dart left as committed" >&2
fi

(cd "$cats_dir" && go run ./cmd/catgen-dart "${args[@]}")
(cd "$cats_dir" && git rev-parse HEAD) > "$(dirname "$0")/../CATS_REV"

echo "CATS_REV -> $(cat "$(dirname "$0")/../CATS_REV")"
echo
echo "Do NOT run dart format over the generated files: they carry"
echo "'// dart format off' so they stay byte-identical to cats's golden."
