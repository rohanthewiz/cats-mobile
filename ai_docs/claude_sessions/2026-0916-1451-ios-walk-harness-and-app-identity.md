# Session: a shell of our own, and a walk harness that stops being a throwaway

- **Session ID:** `bcd56b6a-d339-4bc7-8bbc-458ef778a6da`
- **Date:** 2026-09-16
- **Branch:** main
- **Previous session:** `2026-0916-1411-native-shell-contention-and-church-migration.md`

## Requests, in order

> Do what we can from the Next list, but fix the device contention issue first
> commit then keep going
> /sw

## What closed

| # | item | outcome |
|---|---|---|
| 1 | Device contention | Fixed — cats-mobile builds its own shell with its own ids; verified on both devices |
| 4 | `$GRMOB` shell/pin drift | Structurally gone — the shell is the pinned tag |
| 3 | "Build scripts report success on failure" | **The premise was false.** Deleted, with measurements |
| 6 | Pairing-link paste path | Walked end to end on **both** platforms; the iOS walk paired from a pasted link |
| 8 | iOS confirm dialog by eye | The Reveal dialog renders correctly on iOS — screenshot in the result bundle |
| 2 | Camera QR pairing | Half advanced: `cats://` is claimed and handled. Camera half still blocked |

Commit `f6d60bd`. `gofmt`, `go vet`, `go test -race ./...`, the WASM target
build and `sh -n` on every script are clean.

## Item 1: the app had no shell of its own

cats-mobile binds its Go into **grmob's** native shell, so it built as
`com.grmob.app` / `com.grmob.demo` — the ids grmob's own demo installs under.
Two sessions sharing an emulator installed over each other all of last session,
each wondering why the other's app had appeared. It also wrote its AAR into
grmob's working tree.

`scripts/lib.sh` now builds a shell this repo owns:

```
../grmob (.git objects)          .shell/grmob-v0.5.0/  (ours, disposable)
   |                                  |
   +-- git archive <tag> --> tar -x --+-- app id  -> com.rohanthewiz.catsmobile
                                      +-- label   -> Cats
                                      +-- scheme  -> cats://
```

| | shell's own | ours |
|---|---|---|
| application / bundle id | `com.grmob.app`, `com.grmob.demo` | `com.rohanthewiz.catsmobile` |
| launcher label / `CFBundleDisplayName` | GrMob, GrMobApp | Cats |
| URL scheme | `grmob://` | `cats://` |

`git archive` reads committed objects only, so grmob's checkout being dirty,
mid-rebase or driven by someone else does not matter and it is **never written
to** — verified clean at `00f0c08` after every rebuild. Only `applicationId`
moves, not the Gradle `namespace`, so the activity class is still
`com.grmob.app.MainActivity` and the launch component is the cross product.
`CFBundleDisplayName` and not `PRODUCT_NAME`, which would rename `GrMobApp.app`
and break the paths the scripts print.

Drift goes with it: the Kotlin/Swift half now comes from the same release as the
Go half by construction. An explicit `GRMOB=` still builds a directory verbatim,
unpatched, which is the workflow for editing the shell itself.

**Verified.** Android: `com.grmob.app` and `com.rohanthewiz.catsmobile` both
installed, each launching its own app (grmob's tutorial vs our pair screen);
`aapt2` reports `application-label:'Cats'`. iOS: both containers present, and
the home screen shows two distinct icons, "GrMobApp" and "Cats".

## Two bugs I introduced and caught

**The assertion that asserted the wrong half.** `patch_file` guarded with
`grep -qF` *before* substituting and never checked after. In
`CFBundleURLSchemes: [grmob]` sed reads the brackets as a character class, so the
pattern matched grep and nothing in sed: exit 0, file unchanged. The shipped iOS
app carried our bundle id while still claiming **grmob's** URL scheme. Fixed by
escaping BRE metacharacters and asserting the substitution landed. The stamp
gained a `patch_rev` for the same reason — a repaired recipe must force a
re-extract, or it keeps building the broken shell it already made.

**Two apps called GrMobApp.** The bundle *identifier* moved but the iOS display
name did not, so ours and grmob's demo were indistinguishable on the home
screen. Android had `android:label`; iOS needed `CFBundleDisplayName`. Caught
only by looking at a screenshot. Adding a key rather than rewriting one needed
a second helper, `insert_line_before` (awk, because a portable two-line sed
replacement needs a literal newline that BSD sed reads as `n`).

## Item 3 was not real

Measured rather than assumed, and all three propagate:

| path | exit |
|---|---|
| Kotlin compile error through `(cd … && ./gradlew)` | 1 |
| Swift error through `xcodebuild -quiet` | 65 |
| bare `set -e` through a failing subshell | 7 |

No piped invocation of either script exists in-repo. Last session's exit 0 came
from how the build was invoked, not from the scripts. Deleted from the list.

## The deep link (`app/deeplink.go`)

The shells forward a URL verbatim — deliberately, since what a URL means is the
app's business — so cats-mobile decides what it answers to. It claims a link
only if `ParsePairURI` accepts it, and **fills the Pairing link field rather
than pairing**.

That is the whole design decision: a custom scheme is claimable by any app and
verified by nobody, and redeeming a grant spends a single-use token and pins a
certificate. A link that paired on arrival would let another app choose this
phone's desk without the person holding it ever seeing an address. Filling the
field is what the Paste button does, and it leaves the address and the Pair
button in front of a human. A paired phone ignores one; re-pairing stays the
deliberate path through More.

Subscribed in `Services.Bind` beside `OnNotificationTap`/`OnLifecycle`, not in a
screen's hook slot: the event arrives on a host-event goroutine that does not own
the slot, so the link is parked and drained by the next render. Draining is what
keeps the field editable — a link that stayed put would be re-applied over every
edit. Four tests, including that drain and the URLs it must *not* claim.

Cold start is safe by the shell's design, not by luck: `MainActivity.onCreate`
calls `reportDeepLink` **after** `start()` and `setContent`, commented as "the
subscriber lives in a hook slot and the hook has not run yet."

Walked on Android from a cold start: force-stop → `am start -a …VIEW -d
"cats://pair?…"` → the field holds the exact link, app still unpaired.

## The iOS walk harness (new, and the point of it)

Last session's XCUITests lived in a scratchpad, so every walk rewrote them and no
finding could be re-checked. They are now tracked and copied into the shell's
UI-test target by `lib.sh` on every build:

| file | purpose |
|---|---|
| `ios/uitests/CatsPairUITests.swift` | pair via the Paste button; clears a stale SpringBoard alert first |
| `ios/uitests/CatsConfirmDialogUITests.swift` | item 8 — opens the Reveal dialog, never confirms it |
| `ios/uitests/CatsResumeUITests.swift` | item 7 — home, 20 s, activate, wait out the backoff ladder |
| `scripts/ios-walk.sh` | mint a code, rewrite the host, pbcopy, `xcodegen`, `xcodebuild test` |
| `scripts/hook.py` | item 9's lever: flip a pane's agent state on the walk desk |

`-only-testing` is load-bearing: the target also holds grmob's nine demo tests,
written against the tutorial app, which fail against ours.

The link is never in the test. `catctl --json pair` returns url/token/fingerprint
(not a URI), so the script assembles `cats://pair?u=&t=&f=` and rewrites the host
— catctl reports the LAN address the desk knows itself by, while the simulator
shares the Mac's stack and should dial `localhost`, which the certificate's SANs
cover (`DNS:localhost, …, IP Address:10.0.2.2`). The test reads the device
pasteboard, so the clipboard path is exercised rather than stood in for.

**It works now, after four failed runs, every one of them a bug in the harness
rather than in the app.** In order: the walk reported `EXIT=0` for a failed test
(`xcodebuild | tail` yields tail's status — the very thing this session deleted
item 3 for, reintroduced by me); `app.tap()` hung for 8m44s; the second run died
at exit 64 because xcodebuild will not overwrite a result bundle; a duplicate
local name failed to compile and so silently took *both* other walks with it,
since the target compiles every file together.

What the walks then showed:

| walk | result |
|---|---|
| `CatsPairUITests` | paired the simulator from a pasted link — **item 6's iOS half** |
| `CatsConfirmDialogUITests` | the Reveal dialog renders correctly — **item 8**, seen by eye |
| `CatsResumeUITests` | green, but see the caveat below |

The pairing walk is worth spelling out: it failed on its assertion while the app
paired anyway. `core.ReadClipboard` takes a callback, so the field fills a render
after the tap returns, and reading `value` immediately caught it empty —
`XCTAssert` does not halt, so the test carried on, pressed Pair, and paired. The
assertion polls now. The app was right and the test was wrong, which was the
pattern all afternoon.

## Gotchas

- **The stale-LSP trap, again.** `core.ReadClipboard`, `core.OnNotificationTap`
  and `core.OnDeepLink` were all reported "undefined" for symbols that exist and
  compile, alongside fields added in the same turn. `go build ./...` is the
  truth — exactly the note from last session.
- **A SpringBoard alert outlives almost everything.** An "Open in …?" prompt
  from `simctl openurl` survived `simctl terminate com.apple.springboard` and
  relaunching the app, and draws over every app. `addUIInterruptionMonitor` does
  not help: it fires for an alert raised *during* a run, not one already up.
  Hence the explicit SpringBoard tap at the top of the pair test. Only a
  simulator restart clears it otherwise — not done, because the device is shared.
- **A grmob app never reports its run loop idle to XCUITest.** Every interaction
  logs "App event loop idle notification not received". An ordinary element tap
  warns and proceeds; `app.tap()`, which must first resolve the Target
  Application element, retries and dies ("process main thread busy for 30.0s",
  after 8m44s). Nothing may tap the application itself, and system alerts are
  reached by querying SpringBoard directly rather than through
  `addUIInterruptionMonitor` — a monitor only fires on the *next* interaction
  with the app, and `app.tap()` was the nudge that used to provide.
- **A tappable Box is a `Button`, not a `StaticText`.** A `core.Text` inside a
  Box carrying `core.OnClick` surfaces as `Button, label: 'claude-opus-5 · high'`;
  plain Text stays StaticText (`cats-mobile`, `IDLE`). Querying the wrong
  collection returns nothing — and *nothing reads as disconnected*, so two walks
  blamed the desk for a roster that was on screen the whole time. The
  "App UI hierarchy" attachment in the `.xcresult` settles this in seconds and
  should be the first thing read when a query finds nothing.
- **An absence is not a signal.** The resume walk's first version called the app
  connected when no banner text matched, and a backgrounded app matches nothing
  — so it reported a clean redial while its own screenshot showed the home
  screen. Wait on something positive, and confirm `app.wait(for: .runningForeground)`.
- **`uiautomator` escapes the dump**, still: a field holding the link reads with
  `&amp;`. Compare after unescaping.
- **`shows` unescapes too**, which is why a test can assert a raw `&`.
- **hook.py has two silent failure modes**, both now in its docstring: a reused
  `source` is suppressed for good and *still answers ok*, and the agent must not
  be `claude` (the reserved source is ignored, and the desk's pane runs a real
  one).
- **grmob moved again** during the session (`46d6eb2` → `af52275` → `00f0c08`),
  which is precisely why the shell now comes from a tag.
- `py_compile` leaves `__pycache__/`; gitignored.

## State at the end

- cats-mobile `main`: `8b4bcc7`, pushed, working tree clean. Five commits this
  session (`f6d60bd`, `87d5a74`, `4de292d`, `1c98716`, `8b4bcc7`).
- grmob: **read only, all session.** Its checkout is at `00f0c08` and is *dirty*
  — `core/canvas.go`, `htmlout/canvas.go`, `htmlout/canvas_test.go`, touched at
  15:29–15:30, none of it mine: it is the other session's chart work, matching
  that commit's subject. Which is the whole argument for the private shell —
  a build here no longer cares what state that tree is in.
- The desk is as found: `w1:p1`, agent `claude`, idle.
- Devices, both still shared with another session:
  - emulator-5554 up, `com.grmob.app` and `com.rohanthewiz.catsmobile` installed
    side by side.
  - iPhone 17 Pro booted, `com.grmob.demo` and `com.rohanthewiz.catsmobile`
    installed and now distinguishable on the home screen ("GrMobApp" / "Cats").
    The stale alert is gone, no `xcodebuild` is running, and **the app is paired
    to the catway on `:8421`** — which is the precondition for items 1 and 6 in
    the list below, and will be lost by a reinstall.
- Running (kill by pid, never `killall`): nine `catway`/`cathost` processes
  across sessions, including this walk's catway 47364 on `:8421`.

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = blocks work now; medium = blocks one named thing or a visible
defect; low = nobody has hit it, or contingent.

1. **Resume's probe-and-redial on iOS is still not measured** **(age 9 · value
   medium)**. `CatsResumeUITests` passes, and that is the problem: the roster
   came back **1.0 s** after foregrounding, against Android's ~1 minute on the
   backoff ladder. A 20 s suspension almost certainly never killed the socket —
   the process is frozen but its TCP connection stays established — so no redial
   was exercised. The roster row cannot discriminate either: the app shows "the
   last known state" *while* reconnecting, so it is present either way. To make
   this conclusive: background for minutes, or drop the connection at the desk,
   and assert the banner appears **and then clears**.
2. **Camera QR pairing** **(age 10 · value medium)**. The deep-link half is
   done. The camera half is unchanged and is grmob work: `CameraView` is a stub
   on both native shells (no `Camera.kt`, no `Camera.swift`, at v0.5.0 or
   master). gozxing has nothing to decode until a shell delivers frames.
3. **iOS deep link unverified end to end** **(age 0 · value medium)**. The
   scheme is claimed and the handler is tested, but iOS raises an "Open in …?"
   confirmation that needs a tap. Now tractable, and the pattern exists: the
   pair walk already dismisses a SpringBoard alert.
4. **`CatsPairUITests` cannot re-run against a paired app** **(age 0 · value
   low)**. It looks for the Paste button, which is only on the pair screen, so a
   second run fails with "no Paste button". A walk that first forgets the device
   (More → forget) would make it repeatable; today it is a one-shot after a
   fresh install.
5. **iOS mangles typed input in the pairing link field** **(age 1 · value
   medium)**. The keyboard autocapitalizes; `core.Input` exposes no
   autocapitalization control. Needs a grmob prop. Less pressing now that the
   paste and deep-link paths both avoid typing.
6. **Cancel-on-unblock on iOS** **(age 2 · value low)**. `scripts/hook.py` is the
   lever; the hard half is inspecting Notification Center from XCUITest to see a
   banner *removed*.
7. **`scripts/wasm.sh` still reads `../grmob`'s working tree** **(age 0 · value
   medium)**. It kept its own `GRMOB=${GRMOB:-../grmob}` block and copies the
   runtime JS from whatever is checked out there, so it now carries a drift
   exposure the native scripts no longer have. Point it at `.shell/` too.
8. **church_mobile form copy-on-write race** **(age 10 · value low)**. Unchanged.
9. **Leftover processes** **(age 9 · value low)**. Nine `catway`/`cathost`
   processes across sessions, plus two shared devices. Note the simulator's app
   is currently paired and a reinstall drops that.
10. **Followed window closes: no badge, no census note** **(age 7 · value low)**.
11. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 6 · value
    medium)**. The gap is CI, not the harness — and CI could now also run the
    tracked XCUITests, which is a stronger argument than it was this morning.
12. **Background alerts that survive the pocket** **(age 2 · value medium)**.
    Both platforms stop a backgrounded socket within seconds. Options: push from
    catway (APNs/FCM) or an Android foreground service. *Undecided — a product
    call.*
13. **Chrome on Android web notifications** **(age 2 · value low)**. Needs a
    service worker the web runtime does not have. Contingent.
14. **Android notification small icon** **(age 2 · value low)**. Falls back to a
    platform drawable; cats-mobile ships no `ic_notification`.
15. **church_mobile has no pre-migration test baseline** **(age 1 · value low)**.
    The Calendar ARIA-grid change alters rendered structure and is worth an
    eyeball in a real shell.
16. **Row cross axis / native Box still pack** **(age 3 · value low)**.
    *Deliberate non-goal.*

Read by value instead: **medium** 1, 2, 3, 5, 7, 11, 12 ·
**low** 4, 6, 8, 9, 10, 13, 14, 15 · **non-goal** 16.

*Closed after this doc's first save:* proving the walk harness; the iOS confirm
dialog (item 8, seen by eye); the pairing-link paste path on iOS (the walk
paired the simulator from a pasted link); and the stale SpringBoard alert, which
the pair test now clears at launch.

*Deleted this session:* "Build scripts report success on failure" — measured on
both platforms and false; see above.
