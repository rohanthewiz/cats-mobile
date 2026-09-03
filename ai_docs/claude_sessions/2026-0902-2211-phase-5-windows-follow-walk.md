# Session: Phase 5 — the Windows → follow walk

- **Session ID:** `session_01VuwCDSFhQyb1Umq1ojZ2aT`
- **Date:** 2026-09-02
- **Branch:** main (cats-mobile); grmob `master` at v0.2.1; cats `spike/wire-leaf`
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md` §12 (bullet updated)
- **Previous session:** `2026-0902-2123-phase-5-android-and-ios-bring-up.md`

## Request

> let's do the Windows → follow flow next

The one item §12 left "for a manual pass", because it needs a desktop window.

## How the desk was staged

No browser sign-in was needed: `catctl probe --workspace <id>` opens a
connection that declares a grid, so the server counts it as a sizer and the
phone lists it as a desktop window. Two of them, against the catway still
running on :8421 from the earlier session:

```
catctl new-ws follow-target                       # w2; primary moved to it, so
catctl ws w1                                      # put the primary back on w1
catctl probe --url wss://localhost:8421/ws --token <password> \
  --workspace w1 --cols 120 --rows 32 --script 'viewws:w1; wait:600000'
catctl probe ... --workspace w2 --cols 100 --rows 30 \
  --script 'viewws:w2; wait:1500; typeat:2:echo FOLLOW-ME-W2\n; wait:600000'
```

`typeat` wants the numeric pane id (`2`), not the handle (`w2:p1`).
"Primary" is the most recently focused sizer, so the second probe to connect
became primary; `catctl session` agreed (`active_workspace: w2`). Both
probes report `focused: true`, so both rows wear the Focused badge.

## The walk (Android emulator, adb taps, screencap readouts)

1. Windows tab: `3 connected, 1 viewing · showing follow-target`; two rows
   with the right badges (Showing / Primary / Focused).
2. Tap the w1 row → Showing moves to cats-mobile, "Follow the primary view"
   appears. Neither probe logged a layout change: the follow moved only the
   phone's view, as `CapWindow` promises.
3. Tap "Follow the primary view" → back on follow-target.
4. Follow w1, wifi+data off → banner, list kept; on → **pin lost** (bug 1)
   and during the gap a wrong notice (bug 2).
5. After the fix and a rebuild (`scripts/build-android.sh --install`): same
   toggle, phone stays on w1 through the reconnect, no notice in the gap.
6. Kill the w1 probe while following w1 → phone stays on w1 (the pin is to
   a workspace, not a window), no "Showing" badge anywhere, census line has
   no "showing" clause, release button still offered. Designed, but noted
   in §12 as a UX observation.

## Bugs fixed

1. **Follow did not survive a socket drop.** `Connection.run` built each
   redial's `catsclient.Conn` without `Options.Workspace`, so the Init
   asked for the primary view. The loop now keeps a per-generation
   `workspace`, set from `conn.PinnedWorkspace()` when a socket dies and
   passed to the next `New`. Generation-scoped on purpose: `Connect` (new
   pairing / endpoint) starts clean. `TestFollowSurvivesASocketDrop` fails
   without the fix (`redial init lost the follow pin`) and passes with it.
2. **Reconnect gap blamed the server.** `windows.go` showed "this desk's
   server does not support per-window following" whenever `Live()` was
   nil. The notice now needs a live connection that lacks `CapWindow`.

## Unexplained, not reproduced

In the first pass both probe sockets went to CLOSED at the same instant,
shortly after the phone app was force-stopped and relaunched (the probes
slept on in `wait:` and never noticed). A second force-stop/relaunch with
fresh probes did not reproduce it; catway's log had nothing. Possibly
related to the APK reinstall or the `go test` run that happened in the
same minute. Worth watching for.

## Processes left running (kill by pid, never `killall`)

As before, under the earlier session's scratchpad (`080d2270-…`):
emulator-5554, `cathost -socket /tmp/cm-cathost.sock`, `catway --addr :8421
--tls --tls-san 10.0.2.2 … --control-socket /tmp/cm-control.sock`, and the
fake `claude` script in w1:p1. Both probes were killed and w2 closed; the
session is back to one workspace with the primary on w1. The phone was
left following the primary view.

## Gotchas added

- `catctl` takes `-socket` (or `CATS_CONTROL_SOCKET`), not
  `--control-socket`, for the control socket.
- The tab bar on the emulator: Agents / Windows / Alerts / More at
  y≈2250 px; the first list row is at y≈330 px (1080×2400).
- `adb shell svc wifi disable; svc data disable` drops the socket in about
  five seconds; the reconnect lands within ten of re-enabling.
