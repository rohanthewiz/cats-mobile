# Session: grmob's clipboard, haptics and notifications, and the device walk

- **Session ID:** `451ff707-62fd-483d-aac8-c61623e44858`
- **Date:** 2026-09-14 → 2026-09-15
- **Branch:** main (cats-mobile); master (grmob)
- **Previous session:** `2026-0914-2254-next-list-rebuild.md` (item 1 of its Next list)

## Requests, in order

> 1
> (all three features; work in grmob's main checkout)
> (push + tag v0.5.0; migration + features)
> walk the emulator and simulator to check the three features
> yes (add the Android limits to grmob's docs and correct attention.go)
> /sw

## grmob — v0.5.0, pushed and tagged

| sha | what |
|---|---|
| `3a08b83` | Clipboard: `core.WriteClipboard` / `core.ReadClipboard` |
| `915a558` | Haptics: `core.Haptic`, seven kinds in `core.HapticKinds()` |
| `623a9b1` | Local notifications + `permission.Notifications`; **tag v0.5.0** |
| `7a231e2` | Docs: background limits, measured (**not pushed**) |

All three ride the existing channels (`core.SendSystemEvent` out,
`core.ReceiveHostEvent` in); the gomobile surface did not change.

- **Clipboard.** `"clipboard"` system event `{command: write|read, id}`; a
  read is answered by the `"clipboard"` host event `{id, text, ok}`. Core's
  first reply to one request: the id correlates it (a string, since it
  crosses JSON twice). Headless reads answer `("", false)` at once; a missing
  `ok` is a refusal. Kotlin `Clipboard.kt` (ClipboardManager, coerceToText),
  Swift `App/Clipboard.swift` (UIPasteboard), JS async Clipboard API.
- **Haptics.** `"haptic"` `{kind}`; kinds not durations because iOS offers
  none. UIKit feedback generators; Android Vibrator service with predefined
  effects (waveforms below API 29) and **VIBRATE in the manifest** (vibrate()
  throws without it); `navigator.vibrate` patterns.
- **Notifications.** `core.PostNotification(LocalNotification{ID, Title,
  Body})`, `CancelNotification(id)`, `OnNotificationTap(fn)` (typed wrapper
  like `OnDeepLink`). Android: one `"grmob"` channel, Go id as the tag,
  PendingIntent request code per id, taps reported from `MainActivity`
  (`onCreate` only when `savedInstanceState == null`, and `onNewIntent`),
  small icon `res/drawable/ic_notification` or a platform fallback.
  iOS: `Notifications.swift` is the `UNUserNotificationCenter` delegate, set
  from `SystemEvents.attach` in `GrMobApp.init` (foreground presentation and
  cold-launch taps both need it). Web: one `Notification` per id.
  `permission.Notifications`: POST_NOTIFICATIONS on 13+, the app's switch
  below (never Prompt), `requestAuthorization` + read-back on iOS,
  `Notification.permission` / `requestPermission` in the browser.
- `mobile/verify/{clipboard,haptics,notifications}_test.go` pin spellings,
  dispatch arms, VIBRATE, the tap report and the iOS delegate.
- Checks per feature: go vet/test, `wasm/verify/run.sh`, `ios/verify/run.sh`
  (app layer), `android/build.sh` + `gradlew assembleDebug`.
- The v0.5.0 tag also released 21 commits another session had made since
  v0.4.0.

## cats-mobile

| sha | what |
|---|---|
| `cdfdeff` | grmob v0.2.4 → v0.5.0: `components` → `comps` (8 files), `Components.Text` line dropped (v0.4.0 removed the field; it was inert), six goldens re-recorded (export spells `display:flex`, `border:none`) |
| `e6b736e` | Paste button; `app/attention.go` blocked-agent watch; notification taps; More → THIS PHONE → Notifications |
| `1b5c1cd` | attention.go comment corrected after the walk |

- **Paste** appends `core.ReadClipboard`'s text to the draft; a refusal or an
  empty clipboard toasts.
- **Attention.** `Connection.onMessage` runs `blockedWatch.observe` on every
  `agents` rollup (reading `session.Agents`). Newly blocked → one
  `core.Haptic(HapticWarning)` per rollup and, when not active, a notification
  with the pane handle as id; unblocked → `CancelNotification`. The first
  rollup after `Connect` seeds silently; a socket drop keeps the watch.
- **Taps.** `Services.Bind` subscribes `core.OnNotificationTap`; the shell
  records its context (`SetNavigator`) and the tap does `PopToRoot` + `Push`.
- `app/attention_test.go`: watch unit test, buzz vs banner, tap routing,
  Paste. Race suite, WASM build and the Android APK green.

## The device walk

Android emulator (API 36) via adb + uiautomator; iOS simulator via a
throwaway XCUITest (deleted afterwards, project regenerated) with a shell
script flipping the desk's agent state on marker files.

| | Android | iOS |
|---|---|---|
| Permission from More | dialog → granted, row "On" | alert → allowed, row "On" |
| Paste | restored the cut text | pasted the simulator pasteboard |
| Buzz, foreground | recorded as `finished` per block | not observable on a simulator |
| No banner in foreground | none posted | none shown |
| Banner in background | "walkbot needs you · w1:p1 · w1" | same, on the home screen |
| Cancel on unblock | `notification_canceled` logged (fast run) | not exercised |
| Tap opens the pane | yes, notification cleared | yes |

**Platform limits found** (now in grmob `native.md`, `core/haptics.go`,
`core/notifications.go` and cats-mobile `attention.go`):

- Android 12+ ignores a background app's vibration (`ignored_background`).
- Android's background firewall cuts a backgrounded app's network after
  ~5 s (`dumpsys netpolicy`: `effective=APP_BACKGROUND`,
  `Firewall rule changed: <uid>-background-default`). Only the first block
  in that window posts; later blocks and cancels wait until the app is in
  front. The freezer was *not* involved (`isFrozen=false` throughout).

## Gotchas

- **catway hook suppression.** After `pane.release_agent`, a (source, agent)
  pair with no official session ref is suppressed for good and every report
  still answers `ok`. Use a fresh source per run (`HOOK_SOURCE` in the
  scratchpad `hook.py`). Reports: one JSON line on `/tmp/cm-hooks.sock`,
  `pane.report_agent {pane_id, source, agent, state, seq}`; reserved
  `cats:claude` states are ignored.
- **Prose file counts in grmob.** `wasm/verify` pins "N tracked Go files" in
  five sentences (`repowalks_test.go`, `timings_test.go`), counting
  untracked files too: every new Go file moves them (509 → 518 here).
- zsh does not word-split `$files`: pipe to `xargs`. macOS sed has no `\b`:
  use perl.
- Android: `cmd clipboard` has no shell implementation; seed the clipboard by
  typing and Ctrl+A / Ctrl+X (`input keycombination`). The system clipboard
  overlay then covers the composer's Paste for a few seconds and eats the
  tap. `KEYCODE_BACK` on the root screen exits the app. `dumpsys vibrator_manager`,
  `logcat -b events` (`notification_enqueue/canceled/clicked`) and
  `dumpsys netpolicy` are the readouts.
- iOS XCUITest: Keychain's "Save Password?" sheet covers the app after
  pairing (dismiss "Not Now"); `app.textFields["…"]` matches identifiers,
  and grmob sets the accessibility *label* — match by label predicate.
- A `-modfile` scratch replace is a safe way to dry-run a pin bump.

## State at the end

- grmob master `7a231e2`, one commit ahead of origin (docs only); tag v0.5.0
  at `623a9b1` pushed.
- cats-mobile main at `1b5c1cd` plus this doc, pushed by the wrap.
- Running (kill by pid, never `killall`): emulator (qemu pid 8049), cathost
  43451, catway :8421 47364, fake `claude` 63569; the iPhone 17 Pro
  simulator is booted. The desk's `w1:p1` is back on detection (claude, idle).

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = worked around today; medium = blocks one named thing or a
visible defect; low = nobody has hit it, or contingent.

1. **Camera QR pairing with `cats://pair` deep links** **(age 8 · value
   medium)**. grmob v0.5.0 is now pinned, so only the decoder (gozxing) and
   the deep-link scheme remain.
2. **church_mobile form copy-on-write race** **(age 8 · value low)**. Unchanged.
3. **Leftover processes** **(age 7 · value low)**. Listed under State; now
   also the simulator.
4. **Background → foreground reconnect walk** **(age 7 · value medium)**.
   Partly covered: the walk backgrounded and foregrounded both apps and they
   carried on, but `Resume`'s probe-and-redial was not observed directly.
5. **Pairing-link paste path on a device** **(age 6 · value medium)**. Both
   walks typed host/port/password again.
6. **Both probe sockets closed at once** **(age 5 · value low)**. Deletion
   candidate.
7. **Followed window closes: no badge, no census note** **(age 5 · value low)**.
8. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 4 · value
   medium)**. `ios/verify` does type-check `App/` when the iPhoneOS SDK is
   present (it did all session), so the gap is CI, not the harness.
9. **church: drop the grmob `replace` and pin a release** **(age 1 · value
   medium)**. Now v0.5.0; church also needs the `components` → `comps` rename.
10. **iOS confirm dialog not checked by eye** **(age 1 · value low)**.
11. **Row cross axis / native Box still pack** **(age 1 · value low)**.
    *Deliberate non-goal.*
12. **Push grmob `7a231e2`** **(age 0 · value low)**. Docs only; no tag needed.
13. **Background alerts that survive the pocket** **(age 0 · value medium)**.
    Both platforms stop a backgrounded socket within seconds, so a block
    minutes later raises nothing. Options: push from catway (APNs/FCM, needs
    a server side) or an Android foreground service (a persistent
    notification). *Undecided — a product call.*
14. **Cancel-on-unblock on iOS** **(age 0 · value low)**. Exercised on Android
    only.
15. **Chrome on Android web notifications** **(age 0 · value low)**. The
    `Notification` constructor needs a service worker there; the grmob web
    runtime has none, so it is silent. Contingent on anyone using the web
    build on a phone.
16. **Android notification small icon** **(age 0 · value low)**. The shell
    falls back to a platform drawable until an app ships
    `res/drawable/ic_notification`; cats-mobile builds into grmob's shell,
    which has none.

Read by value instead: **medium** 1, 4, 5, 8, 9, 13 · **low** 2, 3, 6, 7, 10,
11, 12, 14, 15, 16.
