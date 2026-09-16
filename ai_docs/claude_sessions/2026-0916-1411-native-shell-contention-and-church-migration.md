# Session: the shell that wasn't ours, and church off the replace

- **Session ID:** `95808d23-8d76-4d9c-a1ab-97a2fd7fc425`
- **Date:** 2026-09-16
- **Branch:** main (cats-mobile); master (church_mobile)
- **Previous session:** `2026-0915-0147-native-bridges-and-device-walk.md`

## Requests, in order

> Do what we can from the Next list. Use the emulator / simulator to test for
> now. Hardware will come later.
> /sw

## What closed

| # | item | outcome |
|---|---|---|
| 12 | Push grmob `7a231e2` | Already done — an ancestor of grmob HEAD, nothing unpushed |
| 9 | church: drop the `replace`, pin a release | Migrated v0.1.0 → v0.5.0, verified green |
| 6 | Both probe sockets closed at once | Deleted — see below |
| 5 | Pairing-link paste path | Walked on Android; Paste button added |
| 4 | Background → foreground reconnect | Confirmed on Android |
| 1 | Camera QR pairing | Blocked, and further from done than the list said |

## cats-mobile

Four files, +80 lines, `go test ./...` / `go vet` / `gofmt` all clean.

- **`app/pair.go`** — a "Paste link" ghost button under the Pairing link field,
  mirroring the composer's. It *replaces* the field rather than appending
  (a reply is a sentence a paste adds to; a pairing link is a whole value, and
  appending onto a stale one can only build a string that parses as nothing),
  and trims, because a link copied out of a terminal carries a newline.
- **`internal/catsclient/endpoint.go`** — `ParsePairURI` now folds case on the
  scheme *and* the host. RFC 3986 makes both case-insensitive; `url.Parse`
  lowercases only the scheme, so `cats://Pair?…` was rejected as "That is not
  a cats pairing link", which sends the user to mint a fresh code for a link
  that was fine. Test added covering `Cats://`, `CATS://PAIR` and `cats://Pair`.
- **`app/testdata/pair.html`** — re-recorded for the new button.

## The device walks

Android emulator (API 36) via adb; iOS simulator via throwaway XCUITests in a
scratchpad copy of the shell. Both apps were built against a **pristine v0.5.0
shell** (below) and paired against the previous session's catway on `:8421`.

| | Android | iOS |
|---|---|---|
| Pair by link (not host/port/password) | yes, pinned from the link | not completed |
| Reconnect after background | yes, ~1 min on the backoff ladder | not observed |
| Paste button renders | not seen (see contention) | not seen |
| Reveal confirm dialog by eye | (already seen previously) | not completed |

**Item 4, in detail.** Backgrounded → the background firewall severed the
socket within seconds (2 → 0) → foregrounded → the app showed
`Connecting to 10.0.2.2:8421…` and redialled. It reconnected in about a
minute, not instantly: the first check at t+8s read as a failure and was
wrong. Sampling every 5s for 30s afterwards showed a steady single socket.

**Item 5, in detail.** Paired by link, which is the path that carries the
certificate fingerprint; the by-hand path carries no pin. The link was entered
with `adb shell input text` in 16-char chunks — a single long `input text`
silently drops its tail, which the first attempt proved: the field held only
`…&f=d3982aa608`, the app pinned a truncated fingerprint, dialled, and
correctly refused with a certificate mismatch. That refusal is the security
path working on a malformed link, not a defect.

## Item 1 is further away than the list said

The previous note read "only the decoder (gozxing) and the deep-link scheme
remain". Both halves are worse:

- **`CameraView` is a stub on both native shells.** Android renders it as
  `Box(style) { RenderChildren(node) }`, iOS as a `ZStack` of children. There
  is no `Camera.kt` and no `Camera.swift` — at v0.5.0 *or* on master. Only
  `wasm/camera.js` exists. No preview, no frames, `onCapture` never fires.
- **The registered URL scheme is `grmob`, not `cats`.** So `cats://pair?…`
  cannot open the app on either platform.

gozxing is fetchable (v0.1.1) but has nothing to decode. Both halves are grmob
work, not cats-mobile work.

## The build was compiling someone else's shell

Both native builds "succeeded" with exit 0 while failing:

```
e: …/grmob/android/…/GomobileBridge.kt:34:16 Unresolved reference 'setTimeZone'
…/grmob/ios/GrMob/App/GomobileBridge.swift:42:9: error: cannot find 'MobileSetTimeZone' in scope
```

cats-mobile pins grmob **v0.5.0** for the Go half, but the native *shell*
comes from `$GRMOB` on disk (`../grmob` by default), which was another
session's dirty master calling a bridge symbol v0.5.0 does not export.
`scripts/lib.sh` prints a warning about exactly this and then proceeds.

**The fix, without touching grmob:** extract the tagged shell read-only and
point `GRMOB` at it.

```sh
git -C ../grmob archive v0.5.0 | tar -x -C "$SP/grmob-v0.5.0"
GRMOB="$SP/grmob-v0.5.0" scripts/build-android.sh --install
```

`git archive` never touches the other session's working tree, index or `.git`.

## The devices are not ours alone

Both walks then broke in a way that took a while to pin down: both apps
started showing **grmob's tutorial**. The artifacts were never wrong —

| artifact | `Pair with a desk` | `GrMob Interactive Tutorial` |
|---|---|---|
| my AAR / xcframework | ✓ | ✗ |
| the APK gradle produced | ✓ | ✗ |
| the APK **installed** on the emulator | ✗ | ✓ |
| the iOS bundle **installed** | ✗ | ✗ (not my build either) |

Neither install timestamp (Android 13:47:10, iOS bundle 13:49) was ours.
grmob's checkout moved twice during the session (`46d6eb2` → `af52275`) with
an actively changing tree, a Gradle daemon had been up since 11:33, and the
modified files included `TutorialChartsUITests` and `TutorialAlarmNotifyUITests`.

**Another session is driving the same emulator and simulator, and cats-mobile
builds into grmob's shell — so both projects install over each other under
the same ids, `com.grmob.app` and `com.grmob.demo`.** That also explains an
iOS test dying with `Test crashed with signal kill`. Device work stopped
there rather than keep overwriting the other session's app mid-walk.

## Gotchas

- **A green XCUITest that proved nothing.** The first iOS pair test reported
  `** TEST SUCCEEDED **` without having paired: its final check was
  `_ = element.waitForExistence(…)`, which asserts nothing. Caught only by
  relaunching the app and finding it back on an empty pair screen. Every check
  in the replacement is a real assertion, and the failure path prints the
  labels actually on screen.
- **iOS mangles a typed pairing link.** A hand-entered link came back
  full-length but rejected; the keyboard had autocapitalized it. `core.Input`
  exposes no autocorrect/autocapitalization control and grmob's iOS renderer
  builds a bare SwiftUI `TextField`, so cats-mobile cannot fix that side. The
  parser fold above is the half this repo *can* make robust.
- **`uiautomator` escapes the dump.** A field holding the 129-char URI reads
  as 137 chars because each `&` is `&amp;`; compare after `html.unescape`, not
  before. The first comparison reported a false mismatch.
- **zsh will not run a command held in a variable that contains arguments.**
  `ADB="adb -s emulator-5554"; $ADB shell …` fails with "no such file or
  directory". Use a function.
- **An LSP diagnostic can be stale.** `undefined: core.ReadClipboard` was
  reported for a symbol that exists at `grmob@v0.5.0/core/clipboard.go:97` and
  compiles; `go build ./...` is the truth.
- **The snapshot suite is the fastest device-free reproduction.** A suspected
  render panic was disproved in seconds by `go test ./app`, which renders the
  pair screen on the host — it failed only on the stale golden.
- `osascript` has no assistive access here, so the simulator cannot be driven
  by AppleScript clicks; XCUITest is the only way to touch that screen.
- `catctl` is not in the previous session's `catsbin`; build it from
  `~/projs/go/cats/cmd/catctl`. The walk desk answers on
  `--socket /tmp/cm-control.sock`, and its certificate's SANs already cover
  `10.0.2.2`, `localhost` and `127.0.0.1`, so both guests can pin.
- The hook harness was gone; rebuilt as `hook.py` (one JSON line on
  `/tmp/cm-hooks.sock`, `pane.report_agent`, agent `walkbot`, a fresh
  `cats:walk-<epoch>` source per run because a reused source is suppressed
  silently and still answers ok).

## church_mobile

Migrated v0.1.0 → v0.5.0 by a subagent, then verified independently here:
`go build` and `go vet` exit 0, every test ok, `grmob v0.5.0` pinned with no
`replace`, zero `grmob/components` references.

It was a **repair**, not a tidy-up: the `replace => ../../grmob` pointed at
grmob master, where `components` no longer exists, so church_mobile did not
build at all before this.

- `components` → `comps` across 21 files; the `replace` and its now-obsolete
  comment removed.
- `ComponentDefaults.Text` was never set here, so step 3 was a no-op.
- Four goldens re-recorded, each traced to a named grmob commit: `comps.Calendar`
  became an ARIA grid (`role="gridcell"` / `aria-selected`, `aria-current="date"`),
  and modal overlays gained `justify-content:safe center` + `overflow-y:auto`.
- Two judgment calls left for review: doc-comment references to
  `components.X` were rewritten (cats-mobile left its equivalents stale at
  `cdfdeff`), and there is **no pre-migration test baseline** — the goldens
  were accepted by reading their diffs, not by before/after comparison.

## State at the end

- cats-mobile `main`: this doc plus the four files above.
- church_mobile `master`: the migration.
- grmob: **read only, all session.** The throwaway XCUITests exist solely in
  the scratchpad copy; the real checkout has none.
- The desk is as found: `w1:p1`, agent `claude`, idle.
- Running (kill by pid, never `killall`): the previous session's cathost 43451
  and catway 47364 on `:8421`, plus five older cats processes from other
  sessions; emulator-5554 and the iPhone 17 Pro simulator are up — **and are
  being shared with another session.**

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = worked around today; medium = blocks one named thing or a
visible defect; low = nobody has hit it, or contingent.

1. **Device contention: two sessions, one emulator and one simulator**
   **(age 0 · value high)**. cats-mobile builds into grmob's shell, so both
   projects install over each other as `com.grmob.app` / `com.grmob.demo`.
   This blocked three items today. Options: take the devices exclusively, run
   a second AVD/simulator, or give cats-mobile its own application id.
2. **Camera QR pairing with `cats://pair` deep links** **(age 9 · value
   medium)**. Blocked on grmob twice over: `CameraView` is a stub on both
   native shells (no `Camera.kt`, no `Camera.swift`, at v0.5.0 or master), and
   the registered scheme is `grmob`, not `cats`. gozxing has nothing to decode
   until a shell delivers frames.
3. **Build scripts report success on failure** **(age 0 · value high)**. Both
   `scripts/build-android.sh` and `scripts/build-ios.sh` exited 0 with Gradle
   and Swift errors in the log, which cost real time today. They should
   propagate the failure (`set -o pipefail`, check the child's status).
4. **`$GRMOB` shell/pin drift is a warning, not a stop** **(age 0 · value
   medium)**. `lib.sh` detects that the on-disk shell does not match the pinned
   release and builds anyway, producing a link error deep in Gradle/Swift.
   Consider failing, or adopting the `git archive <tag>` shell used here.
5. **iOS mangles typed input in the pairing link field** **(age 0 · value
   medium)**. The keyboard autocapitalizes; `core.Input` has no autocorrect or
   autocapitalization control and grmob's iOS renderer uses a bare
   `TextField`. Needs a grmob prop. The parser now folds case, which covers
   the first letter but not autocorrect.
6. **Pairing-link paste path on iOS** **(age 7 · value medium)**. The Paste
   button exists and the Android link-pairing walk passed, but neither
   platform has had the *button itself* exercised on a device.
7. **Resume's probe-and-redial on iOS** **(age 8 · value medium)**. Confirmed
   on Android this session (~1 min on the backoff ladder); the iOS side was
   never reached.
8. **iOS confirm dialog not checked by eye** **(age 2 · value low)**. Test
   written (`ConfirmDialogUITests`), never run — blocked by item 1.
9. **Cancel-on-unblock on iOS** **(age 1 · value low)**. Still Android-only.
10. **church_mobile form copy-on-write race** **(age 9 · value low)**. Unchanged.
11. **Leftover processes** **(age 8 · value low)**. Now eight cats processes
    across five sessions, plus two shared devices. Worth acting on.
12. **Followed window closes: no badge, no census note** **(age 6 · value low)**.
13. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 5 · value
    medium)**. The gap is CI, not the harness.
14. **Background alerts that survive the pocket** **(age 1 · value medium)**.
    Both platforms stop a backgrounded socket within seconds. Options: push
    from catway (APNs/FCM) or an Android foreground service.
    *Undecided — a product call.*
15. **Chrome on Android web notifications** **(age 1 · value low)**. Needs a
    service worker the web runtime does not have. Contingent.
16. **Android notification small icon** **(age 1 · value low)**. Falls back to
    a platform drawable; cats-mobile ships no `ic_notification`.
17. **church_mobile has no pre-migration test baseline** **(age 0 · value
    low)**. Four goldens were accepted by reading diffs against named grmob
    commits. The Calendar ARIA-grid change alters rendered structure and is
    worth an eyeball in a real shell.
18. **Row cross axis / native Box still pack** **(age 2 · value low)**.
    *Deliberate non-goal.*

Read by value instead: **high** 1, 3 · **medium** 2, 4, 5, 6, 7, 13, 14 ·
**low** 8, 9, 10, 11, 12, 15, 16, 17, 18.

*Deleted this session:* "Both probe sockets closed at once" — a one-off from
`2026-0902-2211` that never reproduced, had nothing in catway's log, and sat
"worth watching for" through five sessions with nobody hitting it.
