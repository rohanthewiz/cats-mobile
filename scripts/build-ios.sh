#!/bin/sh
# Build the cats app as an iOS xcframework, via gomobile bind.
#
#   scripts/build-ios.sh
#   scripts/build-ios.sh --sim   # also xcodegen + xcodebuild for the simulator
#
# Binds grmob's `mobile` bridge plus ./app into $GRMOB/ios/Frameworks, so open
# the Xcode project under $GRMOB/ios to run it. Needs full Xcode.
#
# Runs gomobile from this module rather than delegating to $GRMOB/ios/build.sh
# for the reason spelled out in build-android.sh: gobind can only see packages
# of the module it runs in, and ./app is not one of grmob's.
#
# No server URL is baked in; see build-android.sh. On the simulator the Mac
# is localhost, and grmob's shell sets NSAllowsLocalNetworking, so a catway
# started without --tls is reachable as ws://localhost:8421.
set -e

. "$(dirname "$0")/lib.sh"

if [ ! -d "$GRMOB/ios" ]; then
  echo "no ios shell under $GRMOB" >&2
  exit 1
fi
if ! xcodebuild -version >/dev/null 2>&1; then
  echo "full Xcode is required (xcodebuild not available); install it and run: sudo xcode-select -s /Applications/Xcode.app" >&2
  exit 1
fi

# See build-android.sh for why LDFLAGS may not be "".
LDFLAGS="${LDFLAGS:-}"

OUT="$GRMOB/ios/Frameworks/GrMob.xcframework"
mkdir -p "$(dirname "$OUT")"

echo "==> binding ./app into $OUT"
gomobile bind -target=ios,iossimulator \
  -o "$OUT" \
  ${LDFLAGS:+-ldflags "$LDFLAGS"} \
  github.com/rohanthewiz/grmob/mobile ./app

if [ "$1" = "--sim" ]; then
  # The .xcodeproj is untracked in grmob and regenerated from project.yml;
  # after any Swift change it is stale, so always regenerate before building.
  echo "==> xcodegen + xcodebuild (simulator) in $GRMOB/ios"
  (cd "$GRMOB/ios" && xcodegen generate --quiet && \
    xcodebuild -quiet -project GrMobApp.xcodeproj -scheme GrMobApp \
      -destination 'generic/platform=iOS Simulator' \
      -derivedDataPath build build)
  echo "==> built; open $GRMOB/ios/GrMobApp.xcodeproj to run it, or use simctl on build/Build/Products/Debug-iphonesimulator/GrMobApp.app"
else
  cat <<'NEXT'

==> done. To run it:
      cd $GRMOB/ios && xcodegen generate, then open GrMobApp.xcodeproj and press Run,
      or re-run with --sim.
NEXT
fi
