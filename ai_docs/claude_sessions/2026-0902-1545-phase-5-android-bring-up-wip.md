# Session: Phase 5 — Android bring-up (WIP, stopped mid-walk)

- **Session ID:** `session_01YMfr7rAruzLSVjfgCL5zzM`
- **Date:** 2026-09-02
- **Branch:** main (cats-mobile); grmob `master`; cats `spike/wire-leaf`
- **Repos touched:** `cats-mobile`, `grmob`, `cats` — **all three have
  uncommitted changes** (see "State on disk")
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md` §3 (Phase 5), §11
- **Previous session:** `2026-0902-1522-phase-4-the-app-on-wasm.md`

## Request

> push and tag grmod changes, then start on phase 5

Stopped by the user mid-walk (usage limit). Nothing is committed from this
session; this doc is the hand-off.

## Done

### grmob pushed and tagged v0.2.0

`master` was already on origin; the annotated tag `v0.2.0` (TextGrid,
OpenURL, native audio, system events, CI) was created and pushed. Twelve
commits above v0.1.0.

### cats-mobile go.mod now pins the tag

`github.com/rohanthewiz/grmob v0.2.0`, the `replace` to `../grmob` is gone,
and a `tool` block pins `golang.org/x/mobile` to grmob's version
(`v0.0.0-20251021151156-188f512ec823`) so `gomobile bind` runs from this
module. `go test -race ./...`, gofmt, vet all green against the tag.

### scripts/

| file | what |
|---|---|
| `scripts/lib.sh` | shared preamble: GRMOB (default `../grmob`), gomobile on PATH, and a **version-agreement check**: `go list -m` grmob vs `git describe --tags --exact-match` in `$GRMOB`; warns (not stops) when the shell checkout is not at the tag or is dirty |
| `scripts/build-android.sh` | bind `grmob/mobile` + `./app` into `$GRMOB/android/app/libs/grmob.aar`; `--apk` runs gradle, `--install` adb-installs |
| `scripts/build-ios.sh` | bind into `$GRMOB/ios/Frameworks/GrMob.xcframework`; `--sim` runs xcodegen + xcodebuild (project is `GrMobApp`) |
| `scripts/wasm.sh` | comment updated: no replace directive any more |

No `API_BASE` (church has one): the phone learns its catway by pairing.

### Android: bind, build, run — works after one grmob fix

`scripts/build-android.sh --apk` binds and builds in ~21 s. The APK runs on
the `Medium_Phone_API_36.1` emulator. The first launch was a **blank white
screen**; root cause and fix below. After the fix: pair screen renders,
typed input works, tapping Pair logs in (POST /login JSON, bearer stored),
the shell switches to the four tabs, More shows the connection state, the
desk with its pin, Pair another desk, Forget this device, and the cats sha.

## Bugs found (two upstream, both diagnosed)

### 1. grmob Compose: a FlexGrow child of a Scroll has zero height

`components.Screen{Fill: true, Scroll: true}` renders
`SafeArea → Scroll → Column(FlexGrow 1)`. On Android, Scroll was a Compose
`Column(...verticalScroll())` and ColumnChildren mapped FlexGrow to
`Modifier.weight`, which under the unbounded height of a scroll resolves
to **zero** — the whole screen vanished. The DOM and SwiftUI don't collapse
it, which is why WASM never showed this. church_mobile never combines Fill
and Scroll, which is why church never saw it.

**Fix (uncommitted, `grmob/android/.../Renderer.kt`):** Scroll is now
`GrMobScroll`: a `BoxWithConstraints` carrying the node's modifiers, with
the scrolling Column inside; `ColumnChildren` gained `growMinHeight: Dp?`
and gives a FlexGrow child `heightIn(min = viewport)` instead of weight
when set (null when the viewport is unbounded). Verified on the emulator;
`go test ./mobile/...` (the verify harness) still passes. Needs: commit in
grmob, tag **v0.2.1**, then bump cats-mobile's go.mod to it (until then
`scripts/lib.sh` warns that the shell is dirty — that warning is correct).

### 2. rweb (via catway) refuses WebSocket upgrades from Go clients

catway logged `connection not upgraded to websocket` for every dial from
the phone while auth passed. rweb v0.1.26's `request.Header(key)` matched
only the exact key or the all-lowercase key; Go's `net/http` canonicalizes
to `Sec-Websocket-Key` (lowercase s), which matched neither, so
`IsWebSocketUpgrade` was false. Browsers and Dart send `Sec-WebSocket-Key`
and were never affected — which is why the Flutter app worked.

**Already fixed upstream** in rweb `dfc4d7a` (EqualFold), shipped in
**v0.1.28**. `cats/go.mod` bumped `rweb v0.1.26 → v0.1.28` (uncommitted, on
`spike/wire-leaf`); catway's tests pass; catway rebuilt into the scratchpad
and the phone's dial then reached auth (got a 401 for the *old* session,
which is right — see next).

### Restarting catway invalidates the phone's session

Expected, but worth knowing for the walk: the session HMAC changes per
catway process (or the state dir), so after a catway restart the phone
gets 401 → hard stop → "Your session has expired. Pair again from More."
The app handled it exactly as designed. Re-pair via More → Forget this
device → pair screen.

## Where the walk stopped

Tapped **Forget this device** on More (screenshot not yet inspected). Next
steps of the manual walk, in order:

1. Re-pair: Host `10.0.2.2`, Port `8421`, Password `changeme`, Pair.
2. Confirm the roster arrives (needs a pane on the desk: open
   `https://localhost:8421` in a browser and sign in, or `catctl` with
   `--socket /tmp/cm-control.sock`; `catctl help` lists verbs).
3. Open a pane → TextGrid frame → watch diffs → reply (`pane.send_input`).
4. Windows → follow; Alerts.
5. Background/foreground → reconnect (`ResetForNewSocket`). Note: no
   lifecycle hook yet (grmob ROADMAP), so only a socket death reconnects.
6. Then iOS: `scripts/build-ios.sh --sim` (never run yet).

## Cosmetic issues noticed on Android (not fixed)

- Inputs size to their content width instead of stretching (Compose Column
  default is not stretch; grmob-wide, church has the same).
- The status-bar strip stays light: Screen puts the background on the
  Column, below the SafeArea inset. Give SafeArea/Theme the background, or
  set the window background in the shell.
- More lists the desk as `Endpoint(10.0.2.2:8421, direct, tls)` —
  `Endpoint.String()` leaking into UI; show `host:port` instead
  (`app/more.go`, and the pair-result toast/banner if it uses the same).

## State on disk (all uncommitted)

```
cats-mobile   M go.mod go.sum scripts/wasm.sh
              ?? scripts/build-android.sh scripts/build-ios.sh scripts/lib.sh
grmob         M android/app/src/main/java/com/grmob/runtime/Renderer.kt
cats          M go.mod go.sum        (rweb v0.1.28, on spike/wire-leaf)
```

Suggested commit order: grmob (fix → tag v0.2.1 → push), cats (rweb bump),
cats-mobile (go.mod → grmob v0.2.1 + scripts). Then append a "Phase 5
result" §12 to the plan.

## Processes left running (kill by pid, never `killall`)

Started from the scratchpad
(`/private/tmp/claude-501/-Users-ro-projs-go-cats-mobile/080d2270-…/scratchpad`):

- Android emulator `Medium_Phone_API_36.1` (`adb devices` → emulator-5554)
- `cathost -socket /tmp/cm-cathost.sock -persistent`
- `catway --addr :8421 --tls --tls-san 10.0.2.2 --socket /tmp/cm-cathost.sock
  --control-socket /tmp/cm-control.sock --hook-socket /tmp/cm-hooks.sock
  --state-dir <scratchpad>/catstate` with `CATS_PASSWORD=changeme`
  (binaries built from `../cats` with `-tags ghostty` into
  `<scratchpad>/catsbin`; the ones in `cats/bin` are from Aug 29 and predate
  `wire`)

The `cm-` socket names and the scratchpad state dir were chosen so the
user's real cats state (`~/.local/state/cats`, `/tmp/cats-*.sock`) was not
touched. `pgrep -f catsbin/catway`, `pgrep -f catsbin/cathost`.

## Gotchas worth carrying forward

- **Unix socket paths have a ~104-byte limit**: `cathost -socket` under the
  scratchpad failed with `bind: invalid argument`. Use `/tmp/<short>.sock`.
- catway's cathost flag is `--socket`, not `--cathost-socket`.
- Manual pairing in the app is **always TLS** (`pair.go` sets `TLS: true`),
  so a local catway must run with `--tls` (auto self-signed;
  `--tls-san 10.0.2.2` for the emulator). The Android cleartext exception
  is irrelevant to this path.
- Go stderr on Android appears in logcat under tag `GoLog`; nothing there
  is normal for a healthy render.
- `adb shell input tap` coordinates are in device pixels (1080×2400 here);
  screenshots viewed at 900×2000 need ×1.2.
- The zsh in this environment expands `--include=*.go` as a glob and fails;
  use plain `grep -rn … | grep "\.go:"`.
- To see the initial tree the app hands a shell, a throwaway
  `internal/tmpdump/main.go` that imports `_ ".../app"` and prints
  `mobile.RenderInitial()` works from this module (delete afterwards).
