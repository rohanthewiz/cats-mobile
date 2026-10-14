# Session: plugin panes on the phone, split by type

- **Session ID:** `7ab3755d-352c-4a79-b172-0e131924d160`
- **Date:** 2026-10-14
- **Branch:** main
- **Previous session:** `2026-0916-1611-git-sync-dot-on-window-rows.md`
- **Run from:** the cats checkout (`~/projs/go/cats`), working in `../cats-mobile`

## Requests, in order

1. **cats N-020** (from the cats-todo backlog): "cats-mobile: nothing draws
   `Session.Plugins` yet. If the phone gets a plugins list, split it by
   `PluginPane.Type` the way the desktop does." This is item 17 of the previous
   doc's `## Next`.
2. Wrap the session on cats-mobile.

## What the desktop does (the reference)

`cmd/catway/web/js/17-agentlist.js` in cats, `renderAgents(items, plugins)`:

- `isAgentPlugin(p)` is `p.type === "agent"`, mirroring
  `wire.IsAgentPluginType`. An untyped plugin, or a type the client doesn't
  know, is a tool.
- Agent-typed plugins join the AGENTS section as a block under the detected
  agents. Everything else fills PLUGINS, and that section is hidden while empty.
- The server sends `Plugins` grouped by plugin id, so each section cuts its
  blocks by walking its share once and drawing a hairline wherever the id
  changes.
- Row: a muted dot, the short id (`pluginShort`, the part after the last `.`),
  the terminal title in the dim half, then `typeLabel · handle`.
  `pluginTypeLabel` drops a `_mgr`/`_client` suffix. A plugin row has no state
  and no age.

## What changed (cats-mobile `85ebdec`)

- `internal/catsclient/session.go`: added `Session.SplitPlugins() (agents,
  tools []wire.PluginPane)`. It decides with `wire.IsAgentPluginType`, not a
  local comparison, so the phone and the desktop can't drift on unknown types.
  It keeps the rollup's order and returns fresh slices, never views into
  `s.Plugins`, which the reader goroutine replaces on the next rollup. The
  `MsgPluginNotice` ignore reason no longer says the phone has no plugins list.
- `app/agents.go`:
  - The Agents screen now has the roster's state groups, then **AGENT PLUGINS**
    (only when there are any), then **PLUGINS** (only when there are any).
  - `appendPluginBlocks` draws a `comps.Separator` between plugin-id blocks.
  - `pluginRow` shows the short id as the title, and
    `title · pub · workspace · flag` as the subtitle. Where an agent row has its
    state badge, it shows a neutral type badge. Tapping pushes `paneScreen`.
  - Rows are keyed `"plugin:"+pub`, so a pane that appears in both lists can't
    collide with its agent row.
- `app/format.go`: added `pluginShort` and `pluginTypeLabel`, the phone's
  copies of the desktop helpers.

### Decisions

- **Agent plugins get their own heading.** On the desktop they are a
  headless block under the agents. On the phone every run of rows sits under a
  state heading, so a headless block after IDLE would read as idle agents. The
  heading is "AGENT PLUGINS".
- **No state badge and no age on plugin rows.** "Unknown" would make every
  plugin look like a broken agent. The type badge says what the row actually
  knows.
- **Rules go only between blocks.** The desktop also draws one under the last
  block, to close it against the next sidebar section. Here a section heading
  or the end of the scroll does that job.
- **The empty note follows the desktop's AGENTS test.** "No agents running"
  shows when there are neither detected agents nor agent plugins, even if
  tool plugins are listed below it. A backlog open in a pane is not an agent.

## Tests

- `TestSplitPluginsRoutesOnlyAgentTypeToAgents` (catsclient) covers `editor`,
  `agent`, `dev_server`, untyped, and the near miss `agents`. It also checks
  order and that the halves are copies.
- `TestPluginPanesSplitBetweenAgentsAndPlugins` (app) checks section order
  IDLE < AGENT PLUGINS < PLUGINS, short ids, stripped type labels, and that no
  "Unknown" badge appears.
- `TestToolPluginsAloneKeepTheNoAgentsNote` (app).
- New golden `app/testdata/agents_plugins.html` (`TestSnapshotAgentsPlugins`).
- `go test -race ./...` passed, and `scripts/wasm.sh --build-only` built.

## Gotcha

`scripts/wasm.sh` **serves by default** (`python3 -m http.server` on :8080
after the build). Chaining it after tests without `--build-only` hung the
command for ten minutes. Only the one python server it started was killed.

## Elsewhere

- cats `0bacf3d`: N-020 moved to Closed in `ai_docs/todo/next-list.md`, and
  its closing line now names this doc. Both repos are pushed to `main`.
- The untracked `.cats-todo/`, `.dart_tool/`, `packages/` and `pubspec.lock`
  in cats-mobile predate this session and were left alone. The Dart ones are
  Flutter leftovers.

## Next

**Age** counts session docs since an item was first raised (this doc is 0).
**Value**: high = blocks work now; medium = blocks one named thing or a visible
defect; low = nobody has hit it, or contingent.

1. **Camera QR pairing** **(age 13 · value medium)**. The deep-link half is
   done. The camera half is grmob work: `CameraView` is a stub on both native
   shells.
2. **iOS mangles typed input in the pairing link field** **(age 4 · value
   medium)**. `core.Input` exposes no autocapitalization control, so this
   needs a grmob prop.
3. **`scripts/wasm.sh` still reads `../grmob`'s working tree** **(age 3 · value
   medium)**. It still has its own `GRMOB=${GRMOB:-../grmob}` block, as seen
   this session. Point it at `.shell/` the way the native scripts are.
4. **church_mobile form copy-on-write race** **(age 13 · value low)**. Unchanged.
5. **Leftover processes** **(age 12 · value low)**. Leftover
   `catway`/`cathost` processes from earlier sessions, plus two shared devices.
   The simulator's app is paired, and a reinstall drops that.
6. **Followed window closes: no badge, no census note** **(age 10 · value low)**.
7. **iOS `App/` Swift only compiled by a local xcodebuild** **(age 9 · value
   medium)**. The gap is CI, which could also run the tracked XCUITests.
8. **Background alerts that survive the pocket** **(age 5 · value medium)**.
   APNs/FCM push from catway, or an Android foreground service. *Undecided:
   it's a product call.*
9. **Chrome on Android web notifications** **(age 5 · value low)**. Needs a
   service worker the web runtime doesn't have. Contingent.
10. **Android notification small icon** **(age 5 · value low)**. cats-mobile
    ships no `ic_notification`.
11. **Row cross axis / native Box still pack** **(age 6 · value low)**.
    *Deliberate non-goal.*

**Validate**

12. **Resume's probe-and-redial on iOS is still not measured** **(age 12 ·
    value medium)**. A 20 s suspension never killed the socket. Background for
    minutes, or drop the connection at the desk, then assert the banner
    appears **and then clears**.
13. **iOS deep link unverified end to end** **(age 3 · value medium)**. Needs a
    tap through iOS's "Open in …?" confirmation. The pair walk already
    dismisses a SpringBoard alert, so the pattern exists.
14. **`CatsPairUITests` cannot re-run against a paired app** **(age 3 · value
    low)**. Have the walk forget the device first (More → forget).
15. **Cancel-on-unblock on iOS** **(age 5 · value low)**. The hard part is
    seeing a banner *removed* from Notification Center in XCUITest.
16. **church_mobile has no pre-migration test baseline** **(age 4 · value
    low)**. The Calendar ARIA-grid change is worth an eyeball in a real shell.
17. **The git dot has never been seen on a device** **(age 1 · value low)**.
    Check it on the next device walk, alongside item 12.
18. **Plugin sections never seen on a device** **(age 0 · value low)**. New.
    AGENT PLUGINS / PLUGINS, the hairline between blocks, and the type badge
    are pinned by a golden and screen tests only. On glass, check the
    trailing badge against a long terminal title in the subtitle on a narrow
    phone, and whether the hairline reads as a block break. Same device walk
    as items 12 and 17.

Read by value instead: **medium** 1, 2, 3, 7, 8, 12, 13 ·
**low** 4, 5, 6, 9, 10, 14, 15, 16, 17, 18 · **non-goal** 11.

*Closed this session:* item 17 of the previous doc, "Nothing draws the plugin
panes" (cats N-020). Item 18 is new and was raised by it.
