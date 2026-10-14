# cats-mobile: the Next list

This is the one living list of follow-ups for cats-mobile. Sessions edit it in
place rather than copying a `## Next` section forward from the previous session
doc. A session doc's `## Next` becomes a short summary instead, for example
`Closed: N-004. Raised: N-019. Moved: N-016 → Open.`

It was seeded on 2026-10-14 by `/next-list seed` from all 17 session docs,
`2026-0902-1316` to `2026-1014-1554`, in session `2026-1014-1610-next-list-seed-and-n005-windowless-view`.

cats and grmob each keep their own list with their own `N-###` IDs. A bare ID
here means this file. Write `cats N-020` or `grmob N-058` for the others.

## Conventions

- **IDs are permanent.** Once an ID is assigned it is never reused or
  renumbered, even after the item closes.
- **`raised`** is the session-doc stem (`ai_docs/claude_sessions/<stem>.md`)
  where the item first appeared, wherever that falls, not where it was last
  carried.
- **Age is computed, never stored.** It is the number of session docs since
  `raised`, so an item raised in the newest doc has age 0. `/next-list` works
  it out. A stored age would be stale after the next session.
- **Value** is the payoff of doing the item, not the effort:
  - `high`: something is being worked around today, or a second independent
    consumer has arrived.
  - `medium`: it blocks one named thing, or it is a visible defect nobody has
    to route around yet.
  - `low`: a gap nobody has bumped into, or contingent on something that
    doesn't exist.
- **Sections:**
  - **Open** is the next build work: code to write, a defect to fix, a
    decision to make.
  - **Validate** is the next testing. The remaining work is only a check: run,
    look, hear or measure, or write or repair the test that covers it. No
    product change is planned unless the check finds a defect.
  - **Roadmap** is work we want but won't start soon. It is parked, not
    declined.
  - **Non-goals** is work we are likely never to do.
- **Nothing leaves Open, Validate or Roadmap without a line in another
  section.** A finished item goes to Closed. A declined one goes to Non-goals.
  Two items that turn out to be one thing keep the older ID, and the other
  goes to Closed as `merged into N-xxx`.
- **Open, Validate and Roadmap stay in ID order.** Sort for reading with
  `/next-list [by value | by age]`, not by reordering the file.

**Next ID:** N-020

## Open

- **N-001** · raised `2026-0902-1522-phase-4-the-app-on-wasm` · value medium
  **Camera QR pairing.** The deep-link half is done. The camera half is grmob
  work: `CameraView` is a stub on both native shells, with no `Camera.kt` and
  no `Camera.swift`. That is still true at grmob v0.6.0-8, where the only
  camera code is `core/camera.go` and `wasm/camera.js`. gozxing has nothing to
  decode until a shell delivers frames.

- **N-008** · raised `2026-0915-0147-native-bridges-and-device-walk` · value medium
  **Background alerts that survive the pocket.** Both platforms stop a
  backgrounded socket within seconds, so a block minutes later raises nothing.
  The options are push from catway (APNs/FCM, which needs a server side) or an
  Android foreground service (a persistent notification). *Undecided: it's a
  product call.*

- **N-009** · raised `2026-0915-0147-native-bridges-and-device-walk` · value low
  **Chrome on Android web notifications.** The `Notification` constructor
  needs a service worker there. The grmob web runtime has none, so the web
  build is silent. Contingent on anyone using the web build on a phone.

- **N-010** · raised `2026-0915-0147-native-bridges-and-device-walk` · value low
  **Android notification small icon.** The shell falls back to a platform
  drawable until an app ships `res/drawable/ic_notification`. cats-mobile
  still ships none.

- **N-012** · raised `2026-0916-1411-native-shell-contention-and-church-migration` · value medium
  **iOS mangles typed input in the pairing link field.** The keyboard
  autocapitalizes, and `core.Input` exposes no autocapitalization or
  autocorrect control, so this needs a grmob prop. At grmob v0.6.0-8 only
  `CodeEditor` turns them off. It is less pressing now that the paste and
  deep-link paths both avoid typing.

- **N-014** · raised `2026-0916-1451-ios-walk-harness-and-app-identity` · value medium
  **`scripts/wasm.sh` still reads `../grmob`'s working tree.** It still has
  its own `GRMOB=${GRMOB:-../grmob}` block and copies `grmob-runtime.js` and
  `wasm_exec.js` from whatever is checked out there. Point it at `.shell/` the
  way the native scripts are. The drift is no longer hypothetical: as of
  2026-10-14 `../grmob` is at v0.6.0-8, while `go.mod` pins v0.5.0. A wasm
  build therefore pairs a newer runtime JS with the v0.5.0 Go engine.

## Validate

These items need testing only: run, look, hear or measure, or write or repair
a test. No product change is planned unless a check finds a defect. They are
batched into testing sessions, so a device walk can work through several at
once.

- **N-004** · raised `2026-0902-1545-phase-5-android-bring-up-wip` · value medium
  **Resume's probe-and-redial on iOS is still not measured.**
  `CatsResumeUITests` passes, but the roster came back 1.0 s after
  foregrounding, against Android's ~1 minute on the backoff ladder. A 20 s
  suspension almost certainly never killed the socket, so no redial was
  exercised. The roster row can't tell the cases apart either, because the
  app shows the last known state while reconnecting. To make the test
  conclusive, background the app for minutes or drop the connection at the
  desk, then assert that the banner appears **and then clears**. This was first
  written down as step 5 of the `2026-0902-1545` walk. Android was confirmed
  in `2026-0916-1411`.

- **N-006** · raised `2026-0902-2224-phase-6-retire-the-dart` · value medium
  **iOS `App/` Swift is compiled only by a local xcodebuild.** It shipped one
  broken tag (v0.2.2). The gap is CI: `.github/workflows/ci.yml` has only
  ubuntu jobs (Go and WASM, Android AAR). A macOS job could also run the
  tracked XCUITests in `ios/uitests/`.

- **N-011** · raised `2026-0915-0147-native-bridges-and-device-walk` · value low
  **Cancel-on-unblock on iOS.** It has been exercised on Android only.
  `scripts/hook.py` is the lever. The hard part is inspecting Notification
  Center from XCUITest to see a banner *removed*.

- **N-015** · raised `2026-0916-1451-ios-walk-harness-and-app-identity` · value medium
  **iOS deep link unverified end to end.** The scheme is claimed and the
  handler is tested, but iOS raises an "Open in …?" confirmation that needs a
  tap. The pattern exists: `CatsPairUITests` already dismisses a SpringBoard
  alert.

- **N-016** · raised `2026-0916-1451-ios-walk-harness-and-app-identity` · value low
  **`CatsPairUITests` cannot re-run against a paired app.** It looks for the
  Paste button, which only the pair screen has, so a second run fails with
  "no Paste button". If the walk forgets the device first (More → forget), the
  test becomes repeatable. Today it works once per fresh install.

- **N-017** · raised `2026-0916-1611-git-sync-dot-on-window-rows` · value low
  **The git dot has never been seen on a device.** It is pinned by a golden
  and asserted by a screen test, but a 10px glyph in a badge row may read
  differently on glass: too small, or crowded against "Showing" on a narrow
  phone. Check it on the same device walk as N-004.

- **N-018** · raised `2026-1014-1554-n020-plugin-panes-split-by-type` · value low
  **Plugin sections never seen on a device.** AGENT PLUGINS / PLUGINS, the
  hairline between blocks and the type badge are pinned only by a golden and
  screen tests. On glass, check the trailing badge against a long terminal
  title in the subtitle on a narrow phone, and check whether the hairline
  reads as a block break. Same device walk as N-004 and N-017.

- **N-019** · raised `2026-1014-1610-next-list-seed-and-n005-windowless-view` · value low
  **The windowless-view notice has never been seen on a device.** This is
  N-005's fix, pinned only by `TestWindowsTabSaysSoWhenTheFollowedWindowCloses`
  and the catsclient tests. Re-run step 6 of the `2026-0902-2211` walk: follow
  a side window, close it at the desk, then check that the census line reads
  "showing <name> (no window)" and that the notice strip wraps cleanly on a
  narrow phone. Same device walk as N-004, N-017 and N-018.

## Roadmap

Wanted, but deferred: not the next thing to pick up, and not declined. Empty
at seeding. No session doc parked an item explicitly, and deciding next versus
later is the user's call.

## Non-goals

- **N-007** · declined `2026-0902-2321-grmob-lifecycle-and-cosmetics`. Row
  cross axis and native Box still pack. This is a deliberate non-goal (plan
  §14), and nothing in the app depends on it. grmob has since added
  `core.AlignSelf` on Compose and SwiftUI (`99dc23e`, after v0.5.0), which
  would be the tool if a screen ever needs it.

## Closed

Newest first. Closures from before this file existed are recorded in the
session docs' `## Next` sections and closing notes.

- **N-005** · raised `2026-0902-2211-phase-5-windows-follow-walk` · closed
  2026-10-14, `2026-1014-1610-next-list-seed-and-n005-windowless-view`. **Followed window closes: no badge, no census note.** Fixed by
  the new `Session.ViewWithoutWindow()`, which reports the view's workspace
  when the census has no window showing it. When that happens, the Windows
  tab's census line reads "showing <name> (no window)" and a notice strip
  says the phone stays on it until a window is picked or the primary view is
  followed. Deliberately no Showing badge, since no row is honest to put it
  on. Tests: `TestWindowsAViewWhoseWindowClosedIsReportedByName`,
  `TestWindowsViewWithoutWindowWaitsForBothCensusAndLayout` and
  `TestWindowsTabSaysSoWhenTheFollowedWindowCloses`. Device check is N-019.

- **N-013** · raised `2026-0916-1411-native-shell-contention-and-church-migration`
  · closed 2026-10-14, `2026-1014-1610-next-list-seed-and-n005-windowless-view`. **church_mobile has no pre-migration test baseline.**
  Dropped as a repo mixup, the same as N-002: church has nothing to do with
  cats. It came in with the church grmob migration done during that session.

- **N-002** · raised `2026-0902-1522-phase-4-the-app-on-wasm` · closed
  2026-10-14, `2026-1014-1610-next-list-seed-and-n005-windowless-view`. **church_mobile form copy-on-write race.** Dropped as a repo
  mixup: church has nothing to do with cats, so the item never belonged on
  this list. It came in through phase 4, which used church_mobile's `app/` as
  a model.

- **N-003** · raised `2026-0902-1545-phase-5-android-bring-up-wip` · closed
  2026-10-14, `2026-1014-1610-next-list-seed-and-n005-windowless-view`. **Leftover processes.** These were `catway`/`cathost`
  processes started by earlier sessions, plus a shared emulator and
  simulator. On 2026-10-14, `ps` showed none of them. The only
  `catway`/`cathost` running were `/Applications/Cats.app`'s own pair,
  started 2026-10-09, which are the installed app rather than session
  leftovers. No emulator or simulator was running.
