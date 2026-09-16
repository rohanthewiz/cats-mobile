# Session: a dot that is not allowed to be the message

- **Session ID:** `19142b52-0b46-48c8-a46e-aebd506ae7d0`
- **Date:** 2026-09-16
- **Branch:** main
- **Previous session:** `2026-0916-1558-wire-bump-ws-git-and-plugin-panes.md`

## Requests, in order

> let's do 17 — sync dot on the workspace rows
> /sw

## What happened

The first half of item 17. `ws_git` landed on the session last session and
nothing read it; the Windows tab now does. The plugin half is untouched and
carries forward.

| | |
|---|---|
| Screen | `app/windows.go` — the Windows tab's window rows |
| Reads | `Session.GitSync(ws)`, added last session |
| Verified | `go build`, `go vet`, `go test ./...` all clean |
| Goldens moved | `app/testdata/windows.html`, and only that one |

The row now reads:

```
🖥  cats                    ● [Showing] [Primary] [Focused]
    w1 · 200×60 · main 2 ahead of origin
```

## The decision worth recording: colour carries three meanings in two tints

Deliberately. `comps.Badge` in grmob states the rule for its own variants and
it applies here unchanged: colour is *reinforcement*, never the message (WCAG
1.4.1). A phone has readers who cannot separate amber from green and readers
who hear the row instead of seeing it, and neither is served by a hue.

So the split is:

- **The dot** says *level* (green) or *not level* (amber). Which way it leans
  is not in the colour at all.
- **The subtitle** says it in words, and is the only channel that separates
  ahead from behind: `main 2 ahead of origin`.
- **The dot is `AccessibilityHidden`.** The subtitle has already said what it
  means; a reader announcing both would say it twice.

That is why the dot is a bare `core.Text("●")` with a colour rather than a
`comps.Badge`. A badge without text is exactly the thing Badge's own doc
refuses ("Overdue", not "!"), and a badge *with* text would be a fourth pill on
a row that already carries three.

### Two fallbacks to the muted ink, not one

`gitSyncDot` lands on `TextSecondary` for **no answer** and for **a `Sync`
value this build has no case for**. The second matters: the wire types are
strings, a later cats could name a fourth state, and the honest response to a
word we do not understand is the uncoloured dot — not a guess, and not a
crash. `gitSyncPhrase` has the matching `default: return ""` for the same
reason.

### Why "no answer" still draws a dot

The server omits every workspace it has nothing to say about — not a checkout,
no trunk, no remote, unreachable, or simply a sweep that has not run yet (ten
seconds behind the connect, then every two minutes). "We do not know" and "we
have not asked" arrive identically and neither is a claim about the tree.
Drawing the plain dot for them is what the desk draws, and it keeps the dot
column from jumping sideways as rows gain and lose an answer between sweeps.

### Branch and remote are named, never assumed

`main 2 ahead of origin`, not `2 ahead`. The server compares whichever trunk
and remote the checkout actually has, so a tree still on master, or one whose
main pushes to a fork, would otherwise report a state its owner cannot account
for. Both fields are `omitempty`, so each phrase degrades to the bare state
rather than trailing a dangling preposition.

*Behind* carries no count, because there is none to carry: cats'
`internal/gitsync` deliberately does not fetch, so the commits on the other
side were never counted. "Behind by some" would be a number nobody published.

## The copy under the lock

`windowsScreen` pulls each drawn window's row out of the rollup inside the
existing `conn.Read`, rather than capturing the map:

```go
for _, w := range windows {
        if g, ok := s.GitSync(w.WorkspaceID); ok {
                gitByWS[w.WorkspaceID] = g
        }
}
```

The reference would in fact be safe today — the `ws_git` arm *replaces* the
map wholesale rather than mutating it — but that invariant is written in
`session.go`, not visible at the call site, and a later patch-in-place there
would turn this into a silent data race. A handful of copies costs nothing.

## Tests

`TestWindowRowsCarryTheGitSyncRollup` (new, `app/app_test.go`) pins the actual
risk in drawing this: **the rollup is keyed by workspace and the rows are drawn
from the census**, so a join that slipped would colour the wrong tree with
real-looking data. With only `w1` in the sweep it asserts

- `main 2 ahead of origin` appears, and appears **exactly once**;
- both rows still carry a dot (the no-answer one included);
- the dot is `aria-hidden`.

The shared snapshot fixture gained one `ws_git` message. Only `windows.html`
moved, and it moved exactly as intended — subtitle extended, amber dot ahead of
the badges.

## The gopls trap, again

The editor reported `undefined: wire.WorkspaceGitInfo` / `wire.GitAhead` /
`wire.GitBehind` **ten times** against source the compiler accepts without
complaint. Same stale-module diagnostics the previous session documented after
the cats bump, and the previous session's note is what made it a non-event
rather than half an hour. The rule holds: **after a `go get` bump, check the
compiler before believing the editor.**

## Not done, deliberately

The plugins half of item 17. `Session.Plugins` still has no reader, and the
agents screen is untouched — it is a different screen and a different shape of
work (a section, not a mark on an existing row).

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = blocks work now; medium = blocks one named thing or a visible
defect; low = nobody has hit it, or contingent.

1. **Resume's probe-and-redial on iOS is still not measured** **(age 11 · value
   medium)**. `CatsResumeUITests` passes, and that is the problem: the roster
   came back **1.0 s** after foregrounding, against Android's ~1 minute on the
   backoff ladder. A 20 s suspension almost certainly never killed the socket —
   the process is frozen but its TCP connection stays established — so no redial
   was exercised. The roster row cannot discriminate either: the app shows "the
   last known state" *while* reconnecting, so it is present either way. To make
   this conclusive: background for minutes, or drop the connection at the desk,
   and assert the banner appears **and then clears**.
2. **Camera QR pairing** **(age 12 · value medium)**. The deep-link half is
   done. The camera half is unchanged and is grmob work: `CameraView` is a stub
   on both native shells (no `Camera.kt`, no `Camera.swift`, at v0.5.0 or
   master). gozxing has nothing to decode until a shell delivers frames.
3. **iOS deep link unverified end to end** **(age 2 · value medium)**. The
   scheme is claimed and the handler is tested, but iOS raises an "Open in …?"
   confirmation that needs a tap. Tractable, and the pattern exists: the pair
   walk already dismisses a SpringBoard alert.
4. **`CatsPairUITests` cannot re-run against a paired app** **(age 2 · value
   low)**. It looks for the Paste button, which is only on the pair screen, so a
   second run fails with "no Paste button". A walk that first forgets the device
   (More → forget) would make it repeatable; today it is a one-shot after a
   fresh install.
5. **iOS mangles typed input in the pairing link field** **(age 3 · value
   medium)**. The keyboard autocapitalizes; `core.Input` exposes no
   autocapitalization control. Needs a grmob prop. Less pressing now that the
   paste and deep-link paths both avoid typing.
6. **Cancel-on-unblock on iOS** **(age 4 · value low)**. `scripts/hook.py` is the
   lever; the hard half is inspecting Notification Center from XCUITest to see a
   banner *removed*.
7. **`scripts/wasm.sh` still reads `../grmob`'s working tree** **(age 2 · value
   medium)**. It kept its own `GRMOB=${GRMOB:-../grmob}` block and copies the
   runtime JS from whatever is checked out there, so it carries a drift exposure
   the native scripts no longer have. Point it at `.shell/` too.
8. **church_mobile form copy-on-write race** **(age 12 · value low)**. Unchanged.
9. **Leftover processes** **(age 11 · value low)**. Nine `catway`/`cathost`
   processes across sessions, plus two shared devices. Note the simulator's app
   is currently paired and a reinstall drops that.
10. **Followed window closes: no badge, no census note** **(age 9 · value low)**.
11. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 8 · value
    medium)**. The gap is CI, not the harness — and CI could now also run the
    tracked XCUITests.
12. **Background alerts that survive the pocket** **(age 4 · value medium)**.
    Both platforms stop a backgrounded socket within seconds. Options: push from
    catway (APNs/FCM) or an Android foreground service. *Undecided — a product
    call.*
13. **Chrome on Android web notifications** **(age 4 · value low)**. Needs a
    service worker the web runtime does not have. Contingent.
14. **Android notification small icon** **(age 4 · value low)**. Falls back to a
    platform drawable; cats-mobile ships no `ic_notification`.
15. **church_mobile has no pre-migration test baseline** **(age 3 · value low)**.
    The Calendar ARIA-grid change alters rendered structure and is worth an
    eyeball in a real shell.
16. **Row cross axis / native Box still pack** **(age 5 · value low)**.
    *Deliberate non-goal.*
17. **Nothing draws the plugin panes** **(age 1 · value medium)**. The git half
    of this item closed today. `Session.Plugins` still has no reader: the agents
    screen needs a plugins section, and the rollup arrives grouped by plugin id
    then pane, so a UI cuts the groups by walking the list once. Not a mark on
    an existing row like the dot was — a section, and a product call about where
    it sits relative to the state groups.
18. **The git dot has never been seen on a device** **(age 0 · value low)**.
    It is pinned by a golden and asserted by a screen test, but a 10px glyph in
    a badge row is exactly the kind of thing that reads differently on glass —
    too small, or crowded against "Showing" on a narrow phone. Worth an eyeball
    on the next device walk, alongside item 1.

Read by value instead: **medium** 1, 2, 3, 5, 7, 11, 12, 17 ·
**low** 4, 6, 8, 9, 10, 13, 14, 15, 18 · **non-goal** 16.

*Closed this session:* the `ws_git` half of item 17 — the Windows tab draws the
sync dot and says the state in words. Item 18 is new and was raised by it.
