#!/bin/sh
# Shared preamble for build-android.sh and build-ios.sh. Sourced, not run.
#
# Produces $GRMOB: the native shell (Kotlin/Swift host app) that the Go half
# gets linked into. grmob versions its bridge, runtime and engine together, so
# a shell one commit away from the module fails in ways that look like app bugs
# (a patch op the shell does not know, a bridge symbol that is not there).
#
# Two things used to go wrong here, and the private-shell step below fixes
# both at once:
#
#  1. DRIFT. The Go half comes from go.mod (a tagged release in the module
#     cache); the shell used to come from whatever state ../grmob happened to
#     be in. A grmob developer's half-finished master would link against a
#     released bridge and fail deep inside Gradle/Swift — e.g. a shell calling
#     MobileSetTimeZone, which v0.5.0 does not export. This was only a warning,
#     so the build went ahead and failed confusingly.
#
#  2. IDENTITY. cats-mobile has no shell of its own; it borrows grmob's. That
#     meant our app built as com.grmob.app / com.grmob.demo — the very ids
#     grmob's own demo installs under. Two projects then overwrite each other
#     on the same emulator or simulator, and the build also wrote its AAR into
#     grmob's working tree.
#
# So: extract the *pinned tag* from grmob's git object store into a copy this
# repo owns, stamp our own application id, label and URL scheme onto that copy,
# and build against it. The extraction is `git archive`, which reads committed
# objects only — it never touches ../grmob's working tree, index or HEAD, so a
# grmob checkout being dirty, mid-rebase or driven by someone else is fine.
#
#   ../grmob (.git objects)          .shell/grmob-v0.5.0/  (ours, disposable)
#      |                                  |
#      +-- git archive <tag> --> tar -x --+-- app id  -> com.rohanthewiz.catsmobile
#                                         +-- label   -> Cats
#                                         +-- scheme  -> cats://
#
# An explicit GRMOB= in the environment turns all of this off and uses that
# directory verbatim, which is the grmob developer's workflow: edit the shell
# in place, build cats-mobile against it, see the change. That path keeps the
# old drift warning, because there the drift is the point.

# The module root, so relative paths below are stable however this is invoked.
cd "$(dirname "$0")/.."

# How the app identifies itself to the OS. These exist so cats-mobile can share
# a device with grmob's demo; they are deliberately NOT grmob's ids.
#
# Only applicationId changes, not the Gradle `namespace`. The namespace is the
# Kotlin package the shell's own sources live in (com.grmob.app.MainActivity),
# and rewriting that would mean rewriting every Kotlin file's package line.
# applicationId is the identity Android installs and launches under, which is
# the only half that collides. Hence the launch component is the cross product:
# com.rohanthewiz.catsmobile/com.grmob.app.MainActivity.
CATS_APP_ID="${CATS_APP_ID:-com.rohanthewiz.catsmobile}"
CATS_APP_LABEL="${CATS_APP_LABEL:-Cats}"
# Replaces grmob:// rather than joining it. A custom scheme is claimable by any
# app, so two installed apps both claiming grmob:// makes which one opens a
# link a coin toss — we would be stealing grmob's deep links on the same device.
CATS_URL_SCHEME="${CATS_URL_SCHEME:-cats}"

# gomobile/gobind live in GOPATH/bin, which isn't always on PATH. `go tool`
# would run the pinned versions too, but gomobile execs `gobind` by name from
# PATH, so it has to be installed.
PATH="$PATH:$(go env GOPATH)/bin"
if ! command -v gomobile >/dev/null; then
  echo "gomobile is required: go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind" >&2
  exit 1
fi

# What go.mod resolves grmob to: a tag such as v0.5.0, or a directory when a
# replace directive is in force (then the checkout *is* the module and nothing
# can drift, so there is nothing to pin).
want="$(go list -m -f '{{if .Replace}}{{.Replace.Path}}{{else}}{{.Version}}{{end}}' github.com/rohanthewiz/grmob)"

# Rewrite one exact string in a file, and fail if it was not there.
#
# This assertion is the whole point of the helper. A sed that matches nothing
# exits 0 and leaves the file untouched, so a shell whose layout moved between
# releases would silently build under grmob's identity again — reintroducing
# the collision this file exists to prevent, with no error to notice. Better to
# stop at the patch than to find out by watching two apps overwrite each other.
patch_file() {
  _pf_file="$1"; _pf_old="$2"; _pf_new="$3"
  if ! grep -qF "$_pf_old" "$_pf_file"; then
    echo "shell patch failed: expected to find in $_pf_file:" >&2
    echo "    $_pf_old" >&2
    echo "  The grmob shell's layout changed at $want. Update the patches in scripts/lib.sh." >&2
    exit 1
  fi

  # The caller passes LITERAL strings, but sed reads its pattern as a basic
  # regular expression, so every metacharacter has to be escaped first. This is
  # not hypothetical fussiness: 'CFBundleURLSchemes: [grmob]' contains a
  # bracket expression that matches a single character from {g,r,m,o,b}, so
  # unescaped it matches nothing here, sed exits 0, and the file silently keeps
  # grmob's URL scheme. `]` leads the class below because that is the one
  # position where a bracket expression treats it as a literal.
  _pf_old_esc=$(printf '%s' "$_pf_old" | sed 's|[][\\/.*^$]|\\&|g')
  # The replacement side has a smaller special set: & means "the whole match".
  _pf_new_esc=$(printf '%s' "$_pf_new" | sed 's|[\\/&]|\\&|g')

  # A temp file + mv rather than `sed -i`, whose in-place flag takes a
  # mandatory argument on BSD/macOS sed and none on GNU sed.
  sed "s/$_pf_old_esc/$_pf_new_esc/g" "$_pf_file" > "$_pf_file.tmp" && mv "$_pf_file.tmp" "$_pf_file"

  # Assert the substitution actually landed. Checking that the string EXISTS
  # beforehand is not the same assertion, and only this one catches a pattern
  # that was found by grep but not by sed. (Sound because every replacement
  # here removes its old string outright; a patch whose new text still
  # contained the old would need a different check.)
  if grep -qF "$_pf_old" "$_pf_file"; then
    echo "shell patch did not take effect in $_pf_file: still contains" >&2
    echo "    $_pf_old" >&2
    echo "  This is a bug in patch_file's escaping, not in the shell." >&2
    exit 1
  fi
}

# Insert a line above the first line containing an anchor, and fail if the
# anchor was not there.
#
# A sibling of patch_file for the case where the shell has no line to rewrite:
# the key simply is not in the file and has to be added. sed can do an insert,
# but expressing a two-line replacement portably means a literal newline in the
# replacement text (BSD sed reads \n there as a plain 'n'), so this goes through
# awk instead, where the added line is data rather than part of a pattern.
insert_line_before() {
  _il_file="$1"; _il_anchor="$2"; _il_add="$3"
  if ! grep -qF "$_il_anchor" "$_il_file"; then
    echo "shell patch failed: expected to find in $_il_file:" >&2
    echo "    $_il_anchor" >&2
    echo "  The grmob shell's layout changed at $want. Update the patches in scripts/lib.sh." >&2
    exit 1
  fi
  awk -v anchor="$_il_anchor" -v add="$_il_add" '
    !done && index($0, anchor) { print add; done = 1 }
    { print }
  ' "$_il_file" > "$_il_file.tmp" && mv "$_il_file.tmp" "$_il_file"
  if ! grep -qF "$_il_add" "$_il_file"; then
    echo "shell patch did not take effect in $_il_file: missing" >&2
    echo "    $_il_add" >&2
    exit 1
  fi
}

if [ -n "$GRMOB" ]; then
  # Explicit override: use this shell as-is, and warn about drift the way this
  # script always has. Nothing is patched, so a build from here still carries
  # grmob's identity — which is what a grmob developer testing their own shell
  # actually wants.
  if [ ! -d "$GRMOB/mobile" ]; then
    echo "grmob not found at $GRMOB — set GRMOB to your checkout" >&2
    exit 1
  fi
  case "$want" in
    v*)
      have="$(git -C "$GRMOB" describe --tags --exact-match 2>/dev/null || true)"
      dirty="$(git -C "$GRMOB" status --porcelain 2>/dev/null | head -1)"
      if [ "$have" != "$want" ] || [ -n "$dirty" ]; then
        echo "warning: go.mod builds against grmob $want but $GRMOB is at '${have:-no tag}'${dirty:+ (dirty)}." >&2
        echo "         The native shell there may not match the bridge being linked." >&2
        echo "         Unset GRMOB to build against a pristine $want shell instead." >&2
      fi
      ;;
  esac
  echo "==> shell: $GRMOB (explicit GRMOB, unpatched — grmob's own app id)"
else
  GRMOB_SRC="${GRMOB_SRC:-../grmob}"
  if [ ! -d "$GRMOB_SRC/.git" ] && [ ! -f "$GRMOB_SRC/.git" ]; then
    echo "no grmob git checkout at $GRMOB_SRC — set GRMOB_SRC to one, or GRMOB to a shell to use as-is" >&2
    exit 1
  fi
  case "$want" in
    v*) ;;
    *)
      echo "go.mod has a replace directive for grmob ($want); set GRMOB explicitly to build against it" >&2
      exit 1
      ;;
  esac
  if ! git -C "$GRMOB_SRC" rev-parse -q --verify "refs/tags/$want" >/dev/null; then
    echo "grmob tag $want not found in $GRMOB_SRC — fetch it: git -C $GRMOB_SRC fetch --tags" >&2
    exit 1
  fi

  GRMOB=".shell/grmob-$want"
  # The stamp records what the copy was built from AND the identity patched
  # into it, so changing an id above forces a clean re-extract rather than
  # leaving a stale shell that still answers to the old application id.
  #
  # patch_rev is the recipe's own version: bump it whenever the patch list or
  # patch_file's mechanics change. Without it, a fix to a patch that was
  # silently doing nothing would not re-extract — the stamp still matches — so
  # the repaired script would keep building the broken shell it already made.
  # That is not hypothetical; rev 2 is exactly such a fix (the URL scheme
  # patch's bracket expression).
  patch_rev=3
  stamp_want="$want $(git -C "$GRMOB_SRC" rev-parse "$want") $CATS_APP_ID $CATS_APP_LABEL $CATS_URL_SCHEME r$patch_rev"
  stamp_file="$GRMOB/.cats-shell-stamp"
  if [ "$(cat "$stamp_file" 2>/dev/null || true)" != "$stamp_want" ]; then
    echo "==> extracting a pristine grmob $want shell into $GRMOB"
    # Re-extracting into a populated directory would leave files from the old
    # tag behind, so the copy is rebuilt from nothing. Only ever this path,
    # which this repo owns; ../grmob is never written to.
    rm -rf "$GRMOB"
    mkdir -p "$GRMOB"
    git -C "$GRMOB_SRC" archive "$want" | tar -x -C "$GRMOB"

    echo "==> stamping cats identity: $CATS_APP_ID ($CATS_APP_LABEL), $CATS_URL_SCHEME://"
    patch_file "$GRMOB/android/app/build.gradle" \
      'applicationId = "com.grmob.app"' "applicationId = \"$CATS_APP_ID\""
    patch_file "$GRMOB/android/app/src/main/AndroidManifest.xml" \
      'android:label="GrMob"' "android:label=\"$CATS_APP_LABEL\""
    patch_file "$GRMOB/android/app/src/main/AndroidManifest.xml" \
      '<data android:scheme="grmob" />' "<data android:scheme=\"$CATS_URL_SCHEME\" />"
    # iOS has no checked-in Info.plist at this tag — xcodegen generates it from
    # project.yml's info: block, so project.yml is the only file to patch.
    patch_file "$GRMOB/ios/project.yml" \
      'PRODUCT_BUNDLE_IDENTIFIER: com.grmob.demo' "PRODUCT_BUNDLE_IDENTIFIER: $CATS_APP_ID"
    patch_file "$GRMOB/ios/project.yml" \
      'CFBundleURLName: com.grmob.deeplink' "CFBundleURLName: $CATS_APP_ID.deeplink"
    patch_file "$GRMOB/ios/project.yml" \
      'CFBundleURLSchemes: [grmob]' "CFBundleURLSchemes: [$CATS_URL_SCHEME]"
    # The home-screen name, which is Android's android:label. Added rather than
    # rewritten: the shell sets no CFBundleDisplayName, so iOS falls back to
    # CFBundleName ($(PRODUCT_NAME), "GrMobApp") and this app and grmob's demo
    # appear as two icons with the same name and no way to tell them apart.
    #
    # CFBundleDisplayName and not PRODUCT_NAME, which would also rename the
    # built bundle from GrMobApp.app and break the paths build-ios.sh prints
    # and the scheme xcodebuild is asked for. Only the label needs to move.
    insert_line_before "$GRMOB/ios/project.yml" \
      'UILaunchScreen: {}' "        CFBundleDisplayName: $CATS_APP_LABEL"

    echo "$stamp_want" > "$stamp_file"
  fi

  # This repo's XCUITests, dropped into the shell's UI-test target.
  #
  # The simulator has no tap or text-entry CLI, so a UI test is the only way a
  # script can touch an iOS screen — and previous walks wrote those tests into a
  # scratchpad, which meant every walk rewrote them and no finding could be
  # re-checked later. Tracking them here and copying them in is what makes them
  # ordinary tests.
  #
  # Outside the stamp guard on purpose: the copy is two small files, and an
  # edited test has to reach a shell that is already extracted. The target also
  # holds grmob's own demo tests, which are written against the tutorial app, so
  # a run has to name its test with -only-testing rather than testing the target.
  if [ -d ios/uitests ]; then
    cp ios/uitests/*.swift "$GRMOB/ios/GrMobUITests/" 2>/dev/null || true
  fi
  echo "==> shell: $GRMOB (pristine $want, app id $CATS_APP_ID)"
fi
