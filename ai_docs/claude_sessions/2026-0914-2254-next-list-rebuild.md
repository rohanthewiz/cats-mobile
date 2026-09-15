# Session: the Next list, rebuilt from all ten session docs

- **Session ID:** `451ff707-62fd-483d-aac8-c61623e44858`
- **Date:** 2026-09-14
- **Branch:** main (cats-mobile)
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md` §11–§14
- **Previous session:** `2026-0902-2321-grmob-lifecycle-and-cosmetics.md`

## Request

> /sl
> What's in the Next list?
> make a new session_doc to include this Next list

No code changed. This session read every doc in `ai_docs/claude_sessions/`
and plan §11–§14, checked each open item against the code in cats-mobile,
grmob, cats and church, and rebuilt the list below with `/next-list`.

## Window

All ten docs, from `2026-0902-1316-how-cats-and-cats-mobile-stay-in-lockstep`
to `2026-0902-2321-grmob-lifecycle-and-cosmetics`. That is the repo's whole
history, so no age is floored. Age counts session docs, not days (all ten
share 2026-09-02). This doc replaces the previous doc's **Not done** section
as the live list.

One commit landed after the previous doc and is in none of them:
`6d5c5d8 wire: pin cats at the sleep/clean protocol` (cats
`v0.2.3-0.20260904234655-5d1e4a6716fe`). CI run `33931294523` on it is green.
Its message says there is nothing to render: a sleeping workspace has no
window, so it drops out of the Windows census on its own.

## What the rebuild found

**Lapsed.** Seven items were never done and fell off the hand-carried list:

| item | last named in |
|---|---|
| grmob native gaps (clipboard, haptics, local notifications) | `2026-0902-1449` (body, never a Next item) |
| Camera QR pairing | `2026-0902-2123` "Not exercised" |
| church_mobile form race | plan §11 decisions only |
| Background → foreground walk on a device | `2026-0902-1545` walk step 5 |
| Pairing-link paste path on a device | `2026-0902-2123` "Not exercised" |
| Both probe sockets closed at once | `2026-0902-2211` "Unexplained" |
| Followed window closes: no badge, no census note | `2026-0902-2211` / plan §12 |

**Wrong premise.** The previous doc said "church needs its own pin bump to
v0.2.3". `church/church_mobile/go.mod:25` has
`replace github.com/rohanthewiz/grmob => ../../grmob`, so church builds against
the live grmob checkout, not a pin. It already has every fix, along with any
uncommitted work in that checkout. The replace's own comment (lines 22–24) says
to drop it once a grmob release carries the system-event channel, which
v0.2.0 did.

**Changed premise.** Camera QR was blocked on grmob having no camera. grmob's
ROADMAP now checks off `CameraView` with a capture event. The missing pieces
are a QR decoder (gozxing is in no go.mod) and a grmob pin newer than v0.2.4.

**Resolved, dropped.**

- *"More's usage line assumes `Pct` is a percentage"* (plan §11): correct.
  `cats/wire/down.go:748` documents 0–100, -1 when unknown, and
  `app/more.go:187` handles both.
- *Dart format churn* (`2026-0902-1316`): the Dart was deleted in phase 6.
- *`record` / `runbookRuns` UI* (`2026-0902-1316`): `app/alerts.go`, phase 4.
- *Notification actions untested live* (plan §11): walked in
  `2026-0902-2123`.

## State checked on disk

- cats-mobile pins grmob **v0.2.4**. grmob has since tagged v0.2.5, v0.3.0 and
  v0.4.0; HEAD is `v0.4.0-17-gfb75a4b`, 310 commits past the pin.
- The grmob checkout is dirty with another session's work:
  `M android/.../GrMobStyle.kt`, `M android/.../Renderer.kt`,
  `?? examples/tutorial/zz_rowcensus_test.go`. `scripts/lib.sh` warns on
  every device build until the checkout sits at the pinned tag. Leave those
  files alone; they are not this repo's.
- `grmob/ios/verify/run.sh:56` still type-checks only `Runtime/*.swift`.
- grmob's "a Row that fills" (`fb75a4b`) is about grow strips. A Row's
  cross axis still packs by default; only an explicit `AlignItems: "stretch"`
  stretches it (`docs/platforms/native.md:463-475`).
- Still running, all started by earlier sessions: cathost pid `43451`, catway
  on :8421 pid `47364`, the fake `claude` pid `63569`, emulator-5554 pid
  `70608`. The iPhone 17 Pro simulator is no longer booted.

## Gotchas

- `ls -t` on the session docs sorts by mtime, which a checkout can change;
  `ls | sort` on the `YYYY-MMDD-HHMM-` names is the reliable order.
- The church repos live under `~/projs/go/church/` (`church_mobile`,
  `church`, `ccswm`, `cema`), not beside cats-mobile. `../../grmob` from
  `church_mobile` resolves to the same `~/projs/go/grmob` this repo builds
  its shells from.

## Next

**Age** is how many session docs ago an item was first raised; the newest
doc is 0 (this doc does not count, since it raised nothing new). **Value** is
the payoff, not the effort. **high**: something is being worked around today.
**medium**: it blocks one named thing, or it is a visible defect. **low**:
nobody has hit it yet, or it depends on something that doesn't exist.
**lapsed**: never done, but fell off the list.

1. **grmob's native gaps that cats-mobile wants: clipboard (paste into the
   composer), haptics (a buzz when an agent blocks), local notifications**
   **(age 8 · value low)**. *Lapsed:* named in `2026-0902-1449`, never
   carried. All three are unchecked in `grmob/ROADMAP.md:1445-1448`; lifecycle
   was the fourth gap and it shipped. Nothing waits on them: a candidate for
   deletion rather than another carry.
2. **Camera QR pairing, landing together with `cats://pair` deep links**
   **(age 7 · value medium)**. *Lapsed after `2026-0902-2123`.* grmob now has
   `CameraView`; what is missing is the decoder (gozxing, in no go.mod) and a
   grmob pin past v0.2.4. Deep links (`ROADMAP.md:1446`, unchecked) are the
   other way from a QR into the app.
3. **church_mobile's login and giving forms have the copy-on-write race**
   **(age 7 · value low)**. *Lapsed:* plan §11 decisions only.
   `church_mobile/app/giving.go:129-131` still changes the form in place and
   Sets the same pointer again. It is only a race if a goroutine writes to it,
   which is unconfirmed. church's item, not this repo's.
4. **Leftover processes** **(age 6 · value low)**. In every doc since
   `2026-0902-1545`. Running: cathost `43451`, catway :8421 `47364`, fake
   `claude` `63569`, emulator-5554 `70608`. Kill by pid, never `killall`.
5. **Walk background → foreground on a device** **(age 6 · value medium)**.
   *Lapsed:* step 5 of the `2026-0902-1545` walk. The hook, `Resume` and
   `Probe` shipped in `2026-0902-2321`, but only
   `TestForegroundProbesTheSocketAndRedials` has run them. No device has fired
   `ProcessLifecycleOwner` or `scenePhase` into this app.
6. **The pairing-link paste path has never run on a device**
   **(age 5 · value medium)**. *Lapsed after `2026-0902-2123`.* Every walk
   typed host/port/password, which carries no pin. The designed path spends
   the grant and pins the certificate from the link (`app/pair.go:39-49`).
7. **Both probe sockets closed at the same instant, not reproduced**
   **(age 4 · value low)**. *Lapsed after `2026-0902-2211`.* "Worth watching
   for", and nobody has. Candidate for deletion.
8. **When the followed window closes, the phone stays on that workspace with
   no Showing badge and no census note** **(age 4 · value low)**. *Lapsed:*
   plan §12 observation. `app/windows.go:110,121` still show both only for a
   live window.
9. **iOS `App/` Swift is compiled only by a local xcodebuild**
   **(age 3 · value medium)**. Raised as "No iOS job" in `2026-0902-2224`,
   again as a gotcha in `2026-0902-2321`. It shipped one broken tag (v0.2.2).
   `grmob/ios/verify/run.sh:56` type-checks `Runtime/*.swift` only.
10. **church: drop the grmob `replace` and pin a release**
    **(age 0 · value medium)**. *Premise corrected* from "bump church's pin to
    v0.2.3": church has no pin in effect (`church_mobile/go.mod:25` replaces
    to `../../grmob`) and builds against a checkout that is now v0.4.0+17 and
    dirty.
11. **The iOS confirm dialog (`presentationBackground`) has not been checked
    by eye** **(age 0 · value low)**. No tap tool on the simulator; the
    throwaway XCUITest approach from `2026-0902-2123` would reach it.
12. **Row's cross axis and native Box still pack** **(age 0 · value low)**.
    *Deliberate non-goal* (plan §14), still true after grmob `fb75a4b`.
    Nothing in the app depends on it. Candidate for deletion.

Read by value instead: **high** none · **medium** 2, 5, 6, 9, 10 ·
**low** 1, 3, 4, 7, 8, 11, 12.
