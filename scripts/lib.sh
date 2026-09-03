#!/bin/sh
# Shared preamble for build-android.sh and build-ios.sh. Sourced, not run.
#
# Sets GRMOB (the grmob checkout that holds the native shells), puts gomobile
# on PATH, and checks that the shell we are about to link into was built from
# the same grmob the Go side compiles against. The Go half comes from go.mod
# (a tagged release in the module cache); the Kotlin/Swift half comes from
# $GRMOB on disk. grmob versions its bridge, runtime and engine together, so a
# shell one commit ahead of the module fails in ways that look like app bugs
# (a patch op the shell does not know, a bridge symbol that is not there).

# The module root, so relative paths below are stable however this is invoked.
cd "$(dirname "$0")/.."

GRMOB="${GRMOB:-../grmob}"
if [ ! -d "$GRMOB/mobile" ]; then
  echo "grmob not found at $GRMOB — set GRMOB to your checkout" >&2
  exit 1
fi

# gomobile/gobind live in GOPATH/bin, which isn't always on PATH. `go tool`
# would run the pinned versions too, but gomobile execs `gobind` by name from
# PATH, so it has to be installed.
PATH="$PATH:$(go env GOPATH)/bin"
if ! command -v gomobile >/dev/null; then
  echo "gomobile is required: go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind" >&2
  exit 1
fi

# Version agreement between the two halves. `go list -m` reports what go.mod
# resolves grmob to: a tag such as v0.2.0, or a directory when a replace
# directive is in force (then $GRMOB *is* the module and nothing can drift).
# `git describe` reports which tag the checkout sits on; "" when it is between
# tags or dirty, which is worth a warning but not a stop — that is exactly the
# state a grmob developer is in while fixing a shell bug for this app.
want="$(go list -m -f '{{if .Replace}}{{.Replace.Path}}{{else}}{{.Version}}{{end}}' github.com/rohanthewiz/grmob)"
case "$want" in
  v*)
    have="$(git -C "$GRMOB" describe --tags --exact-match 2>/dev/null || true)"
    dirty="$(git -C "$GRMOB" status --porcelain 2>/dev/null | head -1)"
    if [ "$have" != "$want" ] || [ -n "$dirty" ]; then
      echo "warning: go.mod builds against grmob $want but $GRMOB is at '${have:-no tag}'${dirty:+ (dirty)}." >&2
      echo "         The native shell there may not match the bridge being linked. Check out $want in $GRMOB, or add a replace directive." >&2
    fi
    ;;
esac
