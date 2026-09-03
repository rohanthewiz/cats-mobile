#!/bin/sh
# Build the cats app for the browser and serve it.
#
#   scripts/wasm.sh                 # build + serve on :8080
#   scripts/wasm.sh --build-only    # just build
#   PORT=9000 scripts/wasm.sh       # serve elsewhere
#
# The JS runtime and the wasm_exec shim are COPIED from grmob rather than
# vendored by hand, every build: grmob's runtime and its Go engine are
# versioned together (a patch format change lands in both), so a stale copy
# here would fail in ways that look like app bugs. Nothing under wasm/dist is
# edited; see .gitignore.
#
# Pairing from the preview: the socket rides a cookie that POST /login sets,
# and that cookie is same-site strict. Serve this page and run catway on the
# same host (localhost), and trust catway's self-signed certificate in the
# browser first, or the pair step fails at the browser rather than the app.
set -e

cd "$(dirname "$0")/.."

# Where grmob lives. go.mod pins a tagged grmob for the Go side; the JS
# runtime is copied from a checkout beside this one, and should be at the
# same tag (scripts/lib.sh checks that for the native builds). Let an
# explicit GRMOB win for anyone whose layout differs.
GRMOB="${GRMOB:-../grmob}"
if [ ! -d "$GRMOB/wasm" ]; then
  echo "grmob not found at $GRMOB — set GRMOB to your checkout" >&2
  exit 1
fi

mkdir -p wasm/dist

echo "==> building main.wasm"
GOOS=js GOARCH=wasm go build -o wasm/dist/main.wasm ./wasm

echo "==> copying the grmob runtime from $GRMOB"
cp "$GRMOB/wasm/grmob-runtime.js" wasm/dist/
cp "$GRMOB/wasm/wasm_exec.js" wasm/dist/
cp wasm/index.html wasm/dist/

if [ "$1" = "--build-only" ]; then
  echo "==> built wasm/dist"
  exit 0
fi

PORT="${PORT:-8080}"
echo "==> serving http://localhost:$PORT"
cd wasm/dist && python3 -m http.server "$PORT"
