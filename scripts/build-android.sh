#!/bin/sh
# Build the cats app as an Android AAR, via gomobile bind.
#
#   scripts/build-android.sh
#   scripts/build-android.sh --apk      # also run gradle: assembleDebug
#   scripts/build-android.sh --install  # ...and install on the connected device/emulator
#
# Binds grmob's `mobile` bridge (the Kotlin runtime's call surface) plus ./app,
# whose init calls mobile.Register. The output lands in grmob's own
# android/app/libs/grmob.aar: the Kotlin shell there is the host, this AAR is
# the app. Open $GRMOB/android in Android Studio, or pass --apk / --install.
#
# Why this runs gomobile here rather than delegating to $GRMOB/android/build.sh:
# gobind loads packages in the module it is invoked from. Run from grmob, a
# path to ./app resolves to *no* package — cats-mobile is not one of grmob's
# dependencies — and gobind reports "no exported names", which is a misleading
# way of saying "package not found". Run from this module, grmob's packages
# resolve through go.mod, so both halves of the bind are visible. That also
# needs golang.org/x/mobile in this go.mod, which the `tool` block there pins
# to the version grmob uses.
#
# There is no server URL to bake in: the phone learns its catway by pairing
# (paste the link, or type host/port/password). On the emulator, the host
# machine is 10.0.2.2, and grmob's shell already permits cleartext to it, so a
# catway started without --tls is reachable as ws://10.0.2.2:8421.
set -e

. "$(dirname "$0")/lib.sh"

if [ ! -d "$GRMOB/android" ]; then
  echo "no android shell under $GRMOB" >&2
  exit 1
fi

# gomobile locates the SDK/NDK through these; default to the standard macOS
# install location when the caller hasn't exported them.
[ -n "$ANDROID_HOME" ] || export ANDROID_HOME="$HOME/Library/Android/sdk"
if [ -z "$ANDROID_NDK_HOME" ] && [ -d "$ANDROID_HOME/ndk" ]; then
  export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$(ls "$ANDROID_HOME/ndk" | sort -V | tail -1)"
fi

# LDFLAGS is handed to the Go linker unchanged; empty must mean no -ldflags
# argument at all, because gomobile rejects "".
LDFLAGS="${LDFLAGS:-}"

OUT="$GRMOB/android/app/libs/grmob.aar"
mkdir -p "$(dirname "$OUT")"

echo "==> binding ./app into $OUT"
gomobile bind -target=android -androidapi 24 \
  -o "$OUT" \
  ${LDFLAGS:+-ldflags "$LDFLAGS"} \
  github.com/rohanthewiz/grmob/mobile ./app

case "$1" in
  --apk|--install)
    echo "==> gradle assembleDebug in $GRMOB/android"
    (cd "$GRMOB/android" && ./gradlew --quiet assembleDebug)
    APK="$GRMOB/android/app/build/outputs/apk/debug/app-debug.apk"
    echo "==> built $APK"
    if [ "$1" = "--install" ]; then
      echo "==> adb install"
      "$ANDROID_HOME/platform-tools/adb" install -r "$APK"
      echo "==> installed; launch with: adb shell am start -n com.grmob.app/.MainActivity"
    fi
    ;;
  *)
    cat <<'NEXT'

==> done. To run it:
      open the android/ project in $GRMOB in Android Studio and press Run,
      or re-run with --apk / --install.
NEXT
    ;;
esac
