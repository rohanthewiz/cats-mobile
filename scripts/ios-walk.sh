#!/bin/sh
# Drive the iOS simulator through a walk: pair the app against a running catway,
# then run one of this repo's XCUITests against it.
#
#   scripts/ios-walk.sh                        # pair the app (CatsPairUITests)
#   scripts/ios-walk.sh CatsConfirmDialogUITests   # a walk on an already-paired app
#
# # Why this exists
#
# The simulator has no tap or text-entry CLI: simctl has neither, and osascript
# is refused assistive access here. So touching an iOS screen from a script means
# an XCUITest, and everything around it — minting a pairing code, getting that
# code onto the device, regenerating the project so a new test is in it — is the
# part that was being redone by hand on every walk. See ios/uitests.
#
# # The link
#
# `catctl pair` prints a URL, a single-use token and the certificate
# fingerprint; the app wants them as one cats://pair URI (internal/catsclient's
# ParsePairURI). The host is rewritten because catctl reports the address the
# desk knows itself by — a LAN IP — while the simulator shares the Mac's network
# stack and should dial localhost, which is also what the certificate's SANs
# cover. Set HOST to override (an Android emulator would want 10.0.2.2).
set -e

. "$(dirname "$0")/lib.sh"

TEST="${1:-CatsPairUITests}"
SIM="${SIM:-iPhone 17 Pro}"
CTL="${CTL:-/tmp/cm-control.sock}"
CATCTL="${CATCTL:-catctl}"
HOST="${HOST:-localhost}"

if ! command -v "$CATCTL" >/dev/null; then
  echo "catctl not found — set CATCTL, or build it: go build -o ~/bin/catctl ../cats/cmd/catctl" >&2
  exit 1
fi
if [ ! -S "$CTL" ]; then
  echo "no catway control socket at $CTL — start one, or set CTL" >&2
  exit 1
fi

# Only the pairing walk needs a code; a walk on an already-paired app would
# waste one (they are single-use and expire in minutes) and, worse, the app
# would still be on whatever screen it was on.
if [ "$TEST" = "CatsPairUITests" ]; then
  echo "==> minting a pairing code from $CTL"
  pair_json="$("$CATCTL" --socket "$CTL" --json pair)"
  # python3 rather than jq: it is always present on macOS, and the URI needs
  # percent-encoding anyway, which is a string operation jq would make awkward.
  uri="$(printf '%s' "$pair_json" | HOST="$HOST" python3 -c '
import json, os, sys, urllib.parse
d = json.load(sys.stdin)["data"]
u = urllib.parse.urlsplit(d["url"])
# Keep the scheme and port the desk reported; only the host moves.
netloc = os.environ["HOST"] + (":%d" % u.port if u.port else "")
url = urllib.parse.urlunsplit((u.scheme, netloc, u.path, "", ""))
q = {"u": url, "t": d["token"]}
if d.get("fingerprint"):
    q["f"] = d["fingerprint"]
print("cats://pair?" + urllib.parse.urlencode(q))
')"
  echo "==> pairing link: $(printf '%s' "$uri" | cut -c1-48)…"
  # The device pasteboard is where the app's Paste button reads from, which is
  # the path under test; the test never carries the link itself.
  printf '%s' "$uri" | xcrun simctl pbcopy booted
fi

# Regenerate before building: the .xcodeproj is untracked and derived from
# project.yml, whose UI-test target globs the GrMobUITests directory. lib.sh has
# just copied this repo's tests in, and a project generated before that does not
# contain them.
echo "==> xcodegen + xcodebuild test ($TEST) on $SIM"
cd "$GRMOB/ios"
xcodegen generate --quiet

# -only-testing, not the whole target: it also holds grmob's own demo tests,
# which are written against the tutorial app and fail against this one.
#
# The status is captured rather than piped. `xcodebuild ... | tail` reports
# tail's exit, not xcodebuild's, so a failing test run reads as a successful
# walk -- which is exactly the way this script lied about its first run, and the
# same mistake the Next list once (wrongly) attributed to build-android.sh.
# set -e does not help: a pipeline's status is its last command's.
log="build/$TEST.log"
bundle="build/$TEST.xcresult"

# xcodebuild refuses to write over an existing result bundle ("Existing file at
# -resultBundlePath", exit 64), so without this the second run of any walk fails
# before it starts -- which is most of them, since a walk is something you run
# again after changing what it walks. Safe to delete: this lives under .shell/,
# which is gitignored and rebuilt from a tag, and the previous run's bundle has
# already served its purpose by the time another one starts.
rm -rf "$bundle"

status=0
xcodebuild test \
  -project GrMobApp.xcodeproj \
  -scheme GrMobApp \
  -destination "platform=iOS Simulator,name=$SIM" \
  -only-testing:"GrMobUITests/$TEST" \
  -derivedDataPath build \
  -resultBundlePath "$bundle" > "$log" 2>&1 || status=$?

tail -40 "$log"
echo "==> full log: $GRMOB/ios/$log"
echo "==> screenshots and failures: $GRMOB/ios/build/$TEST.xcresult"
if [ "$status" -ne 0 ]; then
  echo "==> $TEST FAILED (xcodebuild exit $status)" >&2
  exit "$status"
fi
