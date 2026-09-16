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
| 6 | Pairing-link paste path | Android walked end to end; the button also renders on iOS |
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

**Status at save time: the first `CatsPairUITests` run was still executing.** The
harness is therefore written and wired but *not yet proven*; the pairing walk,
and items 7 and 8 behind it, are unverified.

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

- cats-mobile `main`: `f6d60bd` plus this doc and the walk harness.
- grmob: **read only, all session**, clean at `00f0c08`.
- The desk is as found: `w1:p1`, agent `claude`, idle.
- Devices: emulator-5554 up with both apps installed; iPhone 17 Pro simulator up
  with both apps installed, **a stale "Open in GrMobApp?" alert on screen**, and
  an `xcodebuild test` run in flight.
- Running (kill by pid, never `killall`): catway 47364 on `:8421` and cathost
  43451, plus six older cats processes from other sessions.

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = blocks work now; medium = blocks one named thing or a visible
defect; low = nobody has hit it, or contingent.

1. **Prove the iOS walk harness** **(age 0 · value high)**. `CatsPairUITests`
   was still running when this doc was saved. Until it passes, `ios-walk.sh`,
   the SpringBoard dismissal and the pasteboard path are unverified, and items
   7 and 8 stay blocked behind it. Result bundle:
   `.shell/grmob-v0.5.0/ios/build/CatsPairUITests.xcresult`.
2. **Camera QR pairing** **(age 10 · value medium)**. The deep-link half is
   done. The camera half is unchanged and is grmob work: `CameraView` is a stub
   on both native shells (no `Camera.kt`, no `Camera.swift`, at v0.5.0 or
   master). gozxing has nothing to decode until a shell delivers frames.
3. **iOS deep link unverified end to end** **(age 0 · value medium)**. The
   scheme is claimed and the handler is tested, but iOS raises an "Open in …?"
   confirmation that needs a tap. Now tractable: add it to the XCUITest harness.
4. **Pairing-link paste path on iOS** **(age 8 · value medium)**. The button
   renders on the simulator and `CatsPairUITests` exercises it; see item 1.
5. **Resume's probe-and-redial on iOS** **(age 9 · value medium)**.
   `CatsResumeUITests` is written, never run.
6. **iOS confirm dialog not checked by eye** **(age 3 · value low)**.
   `CatsConfirmDialogUITests` is written, never run. The desk has a pane
   (`w1:p1`, agent `claude`) for it to find.
7. **Cancel-on-unblock on iOS** **(age 2 · value low)**. `scripts/hook.py` is the
   lever; the hard half is inspecting Notification Center from XCUITest to see a
   banner *removed*.
8. **iOS mangles typed input in the pairing link field** **(age 1 · value
   medium)**. The keyboard autocapitalizes; `core.Input` exposes no
   autocapitalization control. Needs a grmob prop. Less pressing now that the
   paste and deep-link paths both avoid typing.
9. **`scripts/wasm.sh` still reads `../grmob`'s working tree** **(age 0 · value
   medium)**. It kept its own `GRMOB=${GRMOB:-../grmob}` block and copies the
   runtime JS from whatever is checked out there, so it now carries a drift
   exposure the native scripts no longer have. Point it at `.shell/` too.
10. **A stale SpringBoard alert is on the shared simulator** **(age 0 · value
    low)**. One tap on Cancel clears it; not done because another session shares
    the device. The pair test now clears it at launch.
11. **church_mobile form copy-on-write race** **(age 10 · value low)**. Unchanged.
12. **Leftover processes** **(age 9 · value low)**. Eight cats processes across
    five sessions, plus two shared devices.
13. **Followed window closes: no badge, no census note** **(age 7 · value low)**.
14. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 6 · value
    medium)**. The gap is CI, not the harness — and CI could now also run the
    tracked XCUITests.
15. **Background alerts that survive the pocket** **(age 2 · value medium)**.
    Both platforms stop a backgrounded socket within seconds. Options: push from
    catway (APNs/FCM) or an Android foreground service. *Undecided — a product
    call.*
16. **Chrome on Android web notifications** **(age 2 · value low)**. Needs a
    service worker the web runtime does not have. Contingent.
17. **Android notification small icon** **(age 2 · value low)**. Falls back to a
    platform drawable; cats-mobile ships no `ic_notification`.
18. **church_mobile has no pre-migration test baseline** **(age 1 · value low)**.
    The Calendar ARIA-grid change alters rendered structure and is worth an
    eyeball in a real shell.
19. **Row cross axis / native Box still pack** **(age 3 · value low)**.
    *Deliberate non-goal.*

Read by value instead: **high** 1 · **medium** 2, 3, 4, 5, 8, 9, 14, 15 ·
**low** 6, 7, 10, 11, 12, 13, 16, 17, 18 · **non-goal** 19.

*Deleted this session:* "Build scripts report success on failure" — measured on
both platforms and false; see above.
