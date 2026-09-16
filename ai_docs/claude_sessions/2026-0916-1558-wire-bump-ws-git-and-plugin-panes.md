# Session: a rollup that must not outlive its socket

- **Session ID:** `5c65dcdd-e961-43b3-a40b-e20b4994611a`
- **Date:** 2026-09-16
- **Branch:** main
- **Previous session:** `2026-0916-1451-ios-walk-harness-and-app-identity.md`

## Requests, in order

> pick up the latest wire protocol (if necessary) from the (../cats) repo
> /sw

## What happened

The pin was 6 wire-touching commits behind. It is now current, the one silent
drop the bump introduced is folded, and the whole suite is green.

| | |
|---|---|
| Pin before | `v0.2.3-0.20260904234655-5d1e4a6716fe` (2026-09-04) |
| Pin after | `v0.2.3-0.20260916184950-e4919840f1a8` (cats `e491984`, on origin/main) |
| Gate that caught it | `TestEveryDownTypeHasAnArm` |
| Verified | `go build`, `go vet`, `go test ./...` all clean |

Nothing on the previous `## Next` list was touched — this was an unrelated
request — so all sixteen items carry forward, one session older.

## What actually changed on the wire

Six commits in `cats` touched `wire/`, but only three things reach the phone:

1. **`ws_git` / `wire.WorkspaceGit`** — a new down-message. Per workspace,
   whether its trunk is level with the remote, ahead of it, or behind it, plus
   the branch and remote that were compared and an ahead-count. Its own message
   rather than fields on `WorkspaceInfo` because the two move on different
   clocks: the layout goes out on every split and focus change, this on a
   two-minute poll.
2. **`Agents.Plugins`** — the agents rollup gained a second list, `[]PluginPane`:
   panes running a cats plugin action, plus editor panes, which the server files
   here too. **No compile error** — an added field is invisible to the compiler,
   so the phone was quietly discarding it.
3. **`NewAgents(items, plugins)`** and the new `peer.*` commands — nothing to do.
   The phone has no `NewAgents` call site, and `commands.go` is hand-written and
   deliberately limited to what the phone actually calls.

The go.mod comment's promise held exactly as written: *the compiler adds the
types, it has no opinion about what the session does with one.* The build passed
clean after the bump; the coverage test is what said

```
wire.WorkspaceGit ("ws_git") reaches Session.Apply and is silently dropped
```

## The one decision worth recording

Where to clear `WorkspaceGit`. `ResetForNewSocket` already drops the grids, the
layout and the census, and deliberately keeps the agents and hosts rollups. The
question is which group the git rollup joins — and the answer is in catway's
`registerConn`, not in the wire types:

```go
o.send(c, o.agentsMsg())          // unconditional
if m := o.workspaceGitMsg(); len(m.Workspaces) > 0 {
        o.send(c, m)              // GATED on non-empty
}
o.send(c, o.hostsMsg())           // unconditional
```

Agents and hosts are pushed on every connect, so carrying them across a socket
is harmless — they are overwritten before anything draws. The git rollup is not:
a session with no local checkout never sends it at all, and a fresh server is
silent for up to two minutes before its first sweep (`gitSyncStartDelay` 10 s,
then a 2-minute ticker). Kept, it would let the previous server's answers sit
under the new socket's workspace ids and colour dots nobody has vouched for.

So it is cleared, and the comment in `ResetForNewSocket` says why it is in the
clear-list while its neighbours on the same message are not.

## The fold

`internal/catsclient/session.go`:

- `Plugins []wire.PluginPane`, set on the agents arm, kept **out** of `Roster()`.
  A plugin pane has no agent state and no age — it is a program, not something
  taking turns — so merging the lists would feed stateless rows into the state
  grouping *and* into the attention watch, which reads the roster off the folded
  session (`app/connection.go:450`). The server keeps the two lists apart on the
  wire for the same reason; keeping them apart here makes it structural rather
  than a filter every reader has to remember.
- `WorkspaceGit map[string]wire.WorkspaceGitInfo` plus a `GitSync(ws)` accessor.
  Absence is a real answer, not missing data: the server omits every workspace it
  has nothing to say about, so "we do not know" and "we have not asked yet"
  arrive identically, and both mean the plain dot.
- The arm **rebuilds** the map rather than patching it, so a workspace that drops
  out of a sweep leaves with the rollup instead of lingering forever.

`internal/catsclient/wsgit_test.go` (new) covers the by-workspace fold, wholesale
replacement when a workspace drops out, the reconnect clear, and plugins staying
out of the roster. Fixtures go through the real `DecodeDown`, matching the
house style in `session_test.go`, so each one is the bytes the server sends.

## A harness note

After the bump the editor reported `undefined: wire.WorkspaceGitInfo`,
`wire.GitAhead` and friends — twice, on two different files. Both times it was
**stale gopls** holding the previous module version, not a real error: `go build`
and `go test` were clean against the same source, and the resolved package
directory (`~/go/pkg/mod/github.com/rohanthewiz/cats@v0.2.3-0.20260916184950-e4919840f1a8/wire`)
does contain the types. Worth knowing after any `go get` bump: check the
compiler before believing the diagnostics.

## Not done, deliberately

The UI is untouched. Both surfaces are now on the session and nothing draws
them. That is a product call, not a protocol one, and it is item 17 below.

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = blocks work now; medium = blocks one named thing or a visible
defect; low = nobody has hit it, or contingent.

1. **Resume's probe-and-redial on iOS is still not measured** **(age 10 · value
   medium)**. `CatsResumeUITests` passes, and that is the problem: the roster
   came back **1.0 s** after foregrounding, against Android's ~1 minute on the
   backoff ladder. A 20 s suspension almost certainly never killed the socket —
   the process is frozen but its TCP connection stays established — so no redial
   was exercised. The roster row cannot discriminate either: the app shows "the
   last known state" *while* reconnecting, so it is present either way. To make
   this conclusive: background for minutes, or drop the connection at the desk,
   and assert the banner appears **and then clears**.
2. **Camera QR pairing** **(age 11 · value medium)**. The deep-link half is
   done. The camera half is unchanged and is grmob work: `CameraView` is a stub
   on both native shells (no `Camera.kt`, no `Camera.swift`, at v0.5.0 or
   master). gozxing has nothing to decode until a shell delivers frames.
3. **iOS deep link unverified end to end** **(age 1 · value medium)**. The
   scheme is claimed and the handler is tested, but iOS raises an "Open in …?"
   confirmation that needs a tap. Tractable, and the pattern exists: the pair
   walk already dismisses a SpringBoard alert.
4. **`CatsPairUITests` cannot re-run against a paired app** **(age 1 · value
   low)**. It looks for the Paste button, which is only on the pair screen, so a
   second run fails with "no Paste button". A walk that first forgets the device
   (More → forget) would make it repeatable; today it is a one-shot after a
   fresh install.
5. **iOS mangles typed input in the pairing link field** **(age 2 · value
   medium)**. The keyboard autocapitalizes; `core.Input` exposes no
   autocapitalization control. Needs a grmob prop. Less pressing now that the
   paste and deep-link paths both avoid typing.
6. **Cancel-on-unblock on iOS** **(age 3 · value low)**. `scripts/hook.py` is the
   lever; the hard half is inspecting Notification Center from XCUITest to see a
   banner *removed*.
7. **`scripts/wasm.sh` still reads `../grmob`'s working tree** **(age 1 · value
   medium)**. It kept its own `GRMOB=${GRMOB:-../grmob}` block and copies the
   runtime JS from whatever is checked out there, so it carries a drift exposure
   the native scripts no longer have. Point it at `.shell/` too.
8. **church_mobile form copy-on-write race** **(age 11 · value low)**. Unchanged.
9. **Leftover processes** **(age 10 · value low)**. Nine `catway`/`cathost`
   processes across sessions, plus two shared devices. Note the simulator's app
   is currently paired and a reinstall drops that.
10. **Followed window closes: no badge, no census note** **(age 8 · value low)**.
11. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 7 · value
    medium)**. The gap is CI, not the harness — and CI could now also run the
    tracked XCUITests.
12. **Background alerts that survive the pocket** **(age 3 · value medium)**.
    Both platforms stop a backgrounded socket within seconds. Options: push from
    catway (APNs/FCM) or an Android foreground service. *Undecided — a product
    call.*
13. **Chrome on Android web notifications** **(age 3 · value low)**. Needs a
    service worker the web runtime does not have. Contingent.
14. **Android notification small icon** **(age 3 · value low)**. Falls back to a
    platform drawable; cats-mobile ships no `ic_notification`.
15. **church_mobile has no pre-migration test baseline** **(age 2 · value low)**.
    The Calendar ARIA-grid change alters rendered structure and is worth an
    eyeball in a real shell.
16. **Row cross axis / native Box still pack** **(age 4 · value low)**.
    *Deliberate non-goal.*
17. **Nothing draws `ws_git` or the plugin panes** **(age 0 · value medium)**.
    The session now holds both and no screen reads either. Two separate pieces
    of work: a sync dot or badge on the workspace rows in `app/windows.go:140`
    (drawn from `DesktopWindow.Label()`), and a plugins section on the agents
    screen — the rollup arrives grouped by plugin id then pane, so a UI cuts the
    groups by walking the list once. `Session.GitSync(ws)` and
    `Session.Plugins` are the accessors.

Read by value instead: **medium** 1, 2, 3, 5, 7, 11, 12, 17 ·
**low** 4, 6, 8, 9, 10, 13, 14, 15 · **non-goal** 16.

*Closed this session:* nothing from the list — the wire bump was an unrelated
request. Item 17 is new and was raised by it.
