# Plan: move cats-mobile from Dart/Flutter to Go on grmob

- **Date:** 2026-09-02
- **Repo:** `cats-mobile`, worked from checkouts of `cats`, `grmob` and
  `church/church_mobile` beside it
- **Precedent:** `church_mobile` commit `bad9571` ("Port the app from Flutter to
  grmob") and its `ai_docs/claude_sessions/2026-0902-0205-grmob-go-port.md`
- **Related:** `ai_docs/claude_sessions/2026-0902-1316-how-cats-and-cats-mobile-stay-in-lockstep.md`

## 0. Where things stand

cats-mobile today is a protocol package and nothing else. There is no Flutter
app (`packages/cats_mobile` never landed). What exists:

| piece | lines | fate |
|---|---|---|
| `packages/catsproto/lib/src/generated/*.g.dart` | ~7,400 | emitted by `cats/cmd/catgen-dart`; goes away |
| `connection.dart`, `endpoint.dart`, `grid.dart`, `session.dart`, `sha256.dart` | ~1,370 | ported to Go, then deleted |
| `packages/catsproto/test/*.dart` | ~1,550 | **the spec** for the Go tests; deleted last |
| `CATS_REV`, `tool/regen.sh` | | retired; `go.mod` becomes the pin |

So this is a cleaner move than church's. church kept its Flutter tree as a
working reference because it had a UI to compare against. Here the Dart is
protocol-only, and its worth is the test suite. Keep the Dart until the Go
tests reach parity, then delete it in one commit.

### The relationship with cats, and what changes

The session doc's organizing principle stands: **cats never depends on
cats-mobile**, and **the phone observes and replies; it never rearranges the
desktop**. What changes is *how* the wire contract crosses the repo boundary.

Today: Go structs → `catgen-dart` (reflect + go/ast) → Dart → mirror-committed
here → `CATS_REV` pin → three drift gates, of which gate 2 (this repo's CI)
was never built and gate 1 cannot see this repo falling behind.

Tomorrow: the phone is Go, so it can import the Go structs. The generator, the
golden mirror, `CATS_REV`, `regen.sh` and the "never reformat" rule all
dissolve into one line in `go.mod`:

```
require github.com/rohanthewiz/cats v0.0.0-20260902...-951a8d9310e3
```

Gate 1 becomes `go build`. Gate 2 becomes `go build`. Gate 3 (the `welcome.v`
exact-version check at runtime) is unchanged and still needed.

**But the wire types are not importable yet.** Verified this session:

- everything lives under `cats/internal/` (no `pkg/`, `api/`, `client/`);
- `internal/browserproto` imports `internal/app`, `internal/orchestration`,
  `internal/terminal`, `internal/layout`, `internal/workspace`;
- `internal/orchestration` imports `terminal`, `detect`, `hostmeter`,
  `filexfer`, `worktree`, `shellenv`... the whole server;
- `internal/terminal` wraps libghostty behind `-tags ghostty`, and `creack/pty`
  is unconditional.

None of that compiles for `GOOS=js` or under `gomobile bind`, and none of it
belongs on a phone. Phase 1 below carves out a leaf package. That is a cats
change and it must land first; everything after depends on it.

### Comparison with the precedents

| | cats-todo | church_mobile | cats-mobile (this plan) |
|---|---|---|---|
| language | Go (Bubble Tea) | Go (grmob) | Go (grmob) |
| contract sync | copies `ctlproto` + `command_vocab.go` by hand | hand-written DTOs + httptest contract tests | **imports** `cats/wire`, pinned in `go.mod` |
| transport | control unix socket | REST + SSE | WebSocket `/ws`, JSON text frames |
| native shells | n/a | grmob's own `android/`, `ios/`, bound from the app module | same |

cats-todo's copy-by-hand approach is exactly the drift the lockstep doc spent
a session on. Importing is strictly better once the leaf package exists.

## 1. Decisions taken up front

church's session doc records three decisions made before any code. Same here.

1. **Capability gaps → extend grmob upstream, minimally, via `replace`.**
   church needed `core.OpenURL` and system events. cats needs a monospace
   text-grid node (§4 below). Nothing else blocks a first release.
2. **Dart → port, verify parity against the Dart tests, delete.** Not kept as
   a bilingual repo; there is no UI to compare and the protocol tests are
   fully expressible in Go.
3. **Targets → WASM preview first, then Android, then iOS**, plus `htmlout`
   snapshot tests. WASM is the fastest loop and catway serves cleartext on
   localhost for it. Same order church used for bring-up.
4. **Persistence → bytdb** in `mobile.DataDir()`, the church/todoapp pattern:
   open lazily on the first render pass, nil-receiver-safe so an empty
   `DataDir` degrades to memory. Endpoints, tokens, cert pins and preferences
   go there. A keystore is on grmob's roadmap; token-in-bytdb is the accepted
   interim (grmob `ROADMAP.md:139` says so about church).
5. **WebSocket client → a pure-Go library, not a hand-rolled framer.**
   `cats/cmd/catctl/probe.go` has a stdlib-only client (1,731 lines) and is
   the reference for the *fold*, but its framer skips cert verification and
   was written for a one-shot CLI. Use `github.com/coder/websocket` (pure Go,
   no cgo, works under gomobile and `GOOS=js`). Verify in the phase-0 spike.

## 2. Target layout

Mirror `church_mobile` so the two apps stay recognisable to each other.

```
cats-mobile/
  go.mod                 module github.com/rohanthewiz/cats-mobile
                         require cats (pinned sha), grmob, bytdb, coder/websocket
                         replace grmob => ../grmob   (until a release carries TextGrid)
                         replace cats  => ../cats    (dev only; see §7)
                         tool (gobind, gomobile)     pinned to grmob's x/mobile
  app/                   package catsapp — the UI
    register.go            init(): mobile.Register; func AppName() string   ← mandatory
    root.go                Navigator + bottom nav: Agents · Windows · Alerts · More
    pair.go                pairing screen (paste URI / scan QR / cert TOFU prompt)
    agents.go              the roster, grouped by state, longest-blocked first
    windows.go             desktop windows census; follow / follow primary
    pane.go                pane viewer: TextGrid + title/cwd/agent/exit chrome
    reply.go               composer: Paste / Key / confirmed focus gestures
    alerts.go              notifications, record indicator, runbook runs
    settings.go            endpoints, forget device, theme
    ui.go                  shared vocabulary (church's ui.go is the model)
    hooks.go               useSession, useConnection (pointer-in-state + mutex)
    theme.go               dark theme from the server's `theme` message
    app_test.go            bridge tests: RenderInitial / TriggerCallback journeys
    snapshot_test.go       htmlout goldens in app/testdata/*.html
  internal/catsclient/   the port of packages/catsproto/lib/src
    conn.go                CatsConnection: handshake, Send guard, Invoke, caps
    backoff.go             500 ms → 30 s, ±20 % jitter
    endpoint.go            Endpoint, PairGrant, ParsePairURI (cats://pair?u=&t=&f=)
    trust.go               CertVerdict, DecideCert, TLS VerifyPeerCertificate hook
    grid.go                PaneGrid: column store, dirty rows, applyFrame/applyDiff
    session.go             CatsSession: the apply switch, views, roster
    *_test.go              one-for-one with the Dart tests, plus viewer_mode_test
  internal/store/        bytdb: endpoints, tokens, pins, prefs (TrustStore impl)
  wasm/                  main.go (host, near-copy of church's), index.html
  scripts/               build-android.sh, build-ios.sh, wasm.sh (copy church's)
  ai_docs/               plans/, claude_sessions/
```

Deleted at the end: `packages/`, `pubspec.yaml`, `pubspec.lock`, `CATS_REV`,
`tool/regen.sh`, the Dart lines in `.gitignore`.

## 3. Phase plan

### Phase 0 — spikes (before committing to anything)

Three unknowns, each a half-day, each answered with a throwaway branch.

1. **Leaf-package feasibility in cats.** Copy `proto.go`, `up.go`, `down.go`,
   `cmd.go` into a scratch `wire/` package and see what breaks. Known
   entanglements: `down.go` references `orchestration` attribute bits and
   `layout`/`workspace` types; `cmd.go` aliases `internal/app` param and
   result structs. Goal: `GOOS=js GOARCH=wasm go build ./wire` succeeds with
   no cats-internal imports.
2. **Monospace grid on grmob.** Prototype a `core.TextGrid` node rendering an
   80×24 grid of styled runs on the WASM target and one native. Measure the
   patch size per `PaneDiff` and confirm the reconciler handles a row-keyed
   update without re-sending the whole grid.
3. **WebSocket under gomobile and WASM.** `coder/websocket` dialing a local
   catway with a bearer header, TLS pin callback, ping every 20 s. Confirm
   the `Authorization` header survives on both targets (browsers cannot set
   upgrade headers, so the WASM path may need the cookie route instead:
   `POST /login` then the session cookie. catway already supports both).

### Phase 1 — cats: the `wire` package (cats repo, cats never learns about the phone)

1. Create `github.com/rohanthewiz/cats/wire` (non-internal). Move in:
   `ProtocolVersion`, `Type` consts, `Marshal`, `DecodeUp`, `DecodeDown`,
   `ErrUnknownType`, all up/down structs, the capability consts, and the
   command name/param/result types currently aliased in `cmd.go`.
2. Break the reverse dependencies: attribute bits become exported consts in
   `wire` (they are what `attrs.g.dart` was scraped from); `layout`/`workspace`
   types that ride on the wire get wire-side mirrors or move; server-only
   builders (`frame.go`, `layout.go`) stay in `internal/browserproto`, which
   now imports `wire`.
3. Command vocabulary. `internal/app.CommandSpecs()` needs `flags` and
   `layout`; keep it internal. Add a `wire/commands_gen.go` with typed
   `Invoke*` helpers emitted by a new `cmd/catgen-go` reusing catgen-dart's
   reflection pass, golden-tested the same way (`TestGoldenIsUpToDate`,
   `TestEveryCommandReaches...`). This keeps gate 1's property: adding a
   command without regenerating fails cats's own build.
4. catway, `catctl probe`, and browserproto tests compile against `wire`.
   `make check` green.
5. Tag or note the sha. The phone pins it.
6. `cmd/catgen-dart` and its golden stay until Phase 6.

Directional rule preserved: cats gains a public package and no knowledge of
who imports it.

### Phase 2 — cats-mobile: `internal/catsclient` (the port, with the Dart tests as spec)

Port file by file; each Go test file mirrors its Dart counterpart so parity is
checkable by reading the two side by side.

| Dart | Go | notes |
|---|---|---|
| `endpoint.dart` | `endpoint.go`, `trust.go` | `ParsePairURI` returns `(PairGrant, bool)` and never panics: a camera sees arbitrary QRs. `wss://host:port/ws`. TOFU only when no pin exists at all. |
| `sha256.dart` | *(deleted)* | `crypto/sha256` on the cert DER. The hand-roll existed only to keep the Dart package dependency-free. |
| `connection.dart` | `conn.go`, `backoff.go` | `Init` built inside `Connect` with zeros and `Viewer: true`; app code cannot supply one. `Send` refuses `*wire.Resize` and a second `Init`. `Invoke` ids `c0, c1…`, per-call timer, `pane.wait_for_output` gets the 10 min ceiling. `followWorkspace`/`followPrimaryView` gate on `CapWindow` and update the pin only after `ok`. Every pending call fails on disconnect. |
| `grid.dart` | `grid.go` | column store (`[]uint32` fg/bg/link, `[]uint16` attr, `[]string` glyph, `[]bool` dirty). Colour resolution against `def_fg`/`def_bg` at apply time. Diff before frame and out-of-range index are dropped **and counted**. `nil` scroll clears. |
| `session.dart` | `session.go` | the apply switch over ~19 down types; `PaneRespawned` removes the exit code; `Clients` nil ≠ empty; `RunbookRuns` replaced wholesale; `ResetForNewSocket` drops every grid. Views: `ViewWorkspace` off the layout's active flag, `DesktopWindows`, `Roster`. |
| `viewer_mode_test.dart` | `viewer_mode_test.go` | walk `app/` and `internal/` with `go/ast`; fail on any `wire.Resize` composite literal and any non-zero `Cols`/`Rows` key. Better than the Dart grep: the AST sees through aliasing. |

Concurrency model (church's `hooks.go` pattern, grmob's strength here):

```
WS reader goroutine ──► session.mu.Lock(); session.Apply(msg); Unlock()
                     ──► ctx.RequestRender()
                                └─► buffer-1 coalescing channel: a burst of
                                    PaneDiffs becomes one diff + one patch
```

`core.State` holds a **pointer** to the session struct with its own mutex,
because `State.Set` is atomic per call but a fold is read-modify-write.
Cancellation rides `ctx.OnClose` so a popped pane screen stops nothing
global, but the connection itself lives at the root scope, not in a nav
frame.

Add a `wire`-level version check: on `Welcome.V != wire.ProtocolVersion`
surface "app update required" (gate 3).

### Phase 3 — grmob: the TextGrid node (grmob repo, via `replace`)

grmob has no `FontFamily` in `core.Style`, no canvas, no preformatted text.
Building a grid from nested `Row`/`Text` nodes is a non-starter for 80×24 at
diff rate. Add one node type, four renderer arms, one conformance test:

- `core.TextGrid(rows []core.GridRow, opts...)` where a row is a slice of
  runs `{Text, Fg, Bg, Attr}`; rows are keyed by index so a dirty-row update
  is a per-row patch. `core.Cached` per unchanged row.
- Renderers: Compose `AnnotatedString` in a monospace `Text`; SwiftUI
  `AttributedString` with `.monospaced()`; WASM `<pre>` with `<span>` runs;
  `htmlout` the same markup (so the snapshot tests can pin a grid).
- `mobile/verify/*_test.go` gains the switch-arm coverage check for the new
  node, matching how `ContentModes()` is verified.
- A `FontFamily` style field is the smaller change if TextGrid proves too
  large; decide after the phase-0 measurement.

Later, non-blocking grmob gaps to note in its ROADMAP: URL-scheme deep links
(`cats://pair` from the QR), clipboard (paste into the composer), haptics on
"agent blocked", local notifications, app lifecycle events (reconnect on
foreground). None gate a first build.

### Phase 4 — the app (`app/`), WASM first

Screens, in the order they unblock each other:

1. **Pair.** Paste a `cats://pair` URI (the QR from `catctl pair`); later,
   `core.CameraView` frames through a pure-Go decoder (`makiuchi-d/gozxing`).
   Redeem with `POST /login password=<token>`, store the credential and the
   cert pin in bytdb. Cert TOFU prompt on first connect; mismatch is a hard
   stop with a "forget device" path.
2. **Agents** (the roster). Grouped by state, longest `since_ms` first; tap
   opens the pane. This is the product: "see what every agent is doing".
3. **Windows.** Census join of desktop windows, viewers excluded. Follow one /
   follow primary, both behind `CapWindow`. Show which window the phone is
   *actually* looking through, from the server's layout flag, not the pin.
4. **Pane viewer.** TextGrid + chrome (title, cwd, agent, mode, exit code).
   Read-only by default.
5. **Reply.** Composer sends `Paste`, Enter sends `Key`. Anything that moves
   the desktop viewport (`agent.focus`, `tab.focus`, `pane.zoom`) sits behind
   an explicit confirmed gesture, never a navigation side effect.
6. **Alerts.** `notifications`, the nullable `record` indicator (null means
   *unknown*, draw nothing), `runbookRuns`.
7. **Settings / More.** Endpoints, forget device, theme, version + pinned
   cats sha (from `debug.ReadBuildInfo`).

Tests at church's four layers: `catsclient` unit tests (Phase 2), a fake
catway over `httptest` + WebSocket for contract tests, `app_test.go` bridge
journeys, `htmlout` goldens. `go test -race ./...` must stay green because
the reader goroutine is the whole design.

### Phase 5 — device bring-up (Android, then iOS)

Copy church's `scripts/` verbatim, rename the package. Re-read church's
gotchas before starting; every one of them will recur:

- bind **from this module**: `gomobile bind github.com/rohanthewiz/grmob/mobile ./app`,
  with the `tool` block pinning `x/mobile` to grmob's version;
- `func AppName() string` in `app/register.go` or gobind drops the package
  and the bridge is nil;
- `${LDFLAGS:+-ldflags "$LDFLAGS"}`, never an empty `-ldflags ""`;
- `xcodegen generate` after any Swift change (the `.xcodeproj` is untracked);
- local catway needs the cleartext exceptions already in grmob's shells
  (`10.0.2.2` on Android, `NSAllowsLocalNetworking` on iOS); a real device
  talks `wss` with the pinned cert;
- `core.AlignItemsProp`, not `core.AlignItems(...)`.

Manual walk on the simulator: pair → roster → open pane → watch diffs → reply
→ background/foreground → reconnect with `ResetForNewSocket`.

### Phase 6 — retire the Dart, close the loop

1. Delete `packages/`, `pubspec.*`, `CATS_REV`, `tool/regen.sh`; strip the
   Dart entries from `.gitignore`. One commit, after `go test ./...` proves
   parity against the Dart suite it replaces.
2. In cats: delete `cmd/catgen-dart` and `testdata/golden` (~9,700 lines).
   The doc comments the generator carried into Dart are now plain godoc on
   `wire`.
3. Rewrite `README.md`: layout, the organizing principle (unchanged, with the
   four viewer-mode layers restated in Go terms), build/run, and the new
   lockstep story: "`go.mod` is the pin; bump it with `go get
   github.com/rohanthewiz/cats@<sha>`; a new down-message still needs a new
   arm in `session.go`'s switch."
4. **Gate 2, finally:** `.github/workflows/ci.yml` modelled on grmob's:
   `gofmt`, `go vet`, `go test -race ./...`, `GOOS=js` build, and a
   `gomobile bind` AAR job. The two-file `dart format` churn noted in the
   lockstep doc disappears with the Dart.
5. Session doc via `/sess-save`.

## 4. The invariants, restated for Go

The four layers that keep the phone a viewer, mapped:

| Dart | Go |
|---|---|
| `CatsConnection` builds `init` with zeros and `viewer: true` | `catsclient.Connect` builds `wire.Init` privately; there is no exported way to pass one |
| `send` refuses `Resize` | `Conn.Send` type-switches on an allowlist (`Key`, `Mouse`, `Paste`, `Image`, `Cmd`); everything else is `ErrViewerModeViolation` |
| `@Deprecated Resize` promoted to an analyzer error | no Dart analyzer; replaced by the AST test below plus `go vet` |
| `viewer_mode_test.dart` greps `lib/` | `viewer_mode_test.go` walks `app/` and `internal/` with `go/ast` |

And the lesson the lockstep doc closed on still holds: **the compiler adds the
types; it has no opinion about what the session does with one.** After every
`go get cats@<sha>`, diff `wire`'s down-message list against `session.go`'s
switch. Make that mechanical: a test that enumerates `wire`'s `Type` consts
and asserts each has an arm (or is on an explicit "deliberately ignored" list:
`Welcome`, `CmdResult`).

## 5. Risks

- **The `wire` carve-out is the long pole.** `down.go` is 951 lines with
  tendrils into `orchestration`, `layout` and `workspace`. If phase-0 spike 1
  shows it cannot be made a leaf without reshaping server types, the
  fallback is cats-todo's copy-by-hand plus a cats-side test that diffs the
  copy against the original. That keeps gate 1 but re-creates the mirror
  problem. Prefer the carve-out.
- **TextGrid performance on natives.** Unknown until measured. The mitigation
  is row-keyed patches and per-row caching; the fallback is a coarser
  "peek" view (last N rows as plain text) for v1.
- **Header auth on WASM.** Browsers cannot set upgrade headers. The WASM
  build may need the cookie path. Both exist server-side; the client just
  needs a build-tagged dial.
- **grmob is v0.1.0 and moving daily.** Pin via `replace` like church, and
  expect the occasional re-record of snapshots after a grmob fix.
- **Push while backgrounded** (APNs/FCM) is out of scope. "Get pushed when
  one blocks" is in-app first; a `notifications` arm plus haptics covers the
  foreground case. Background push is a separate design.

## 6. Order of commits (proposed)

```
cats:         wire: carve the browser protocol out of internal/browserproto
cats:         catgen-go: typed command helpers, golden-tested
cats-mobile:  go.mod + internal/catsclient: port the protocol layer from Dart
grmob:        core.TextGrid: a monospace styled-run grid, four renderers
cats-mobile:  app: pair, agents, windows, pane, reply, alerts, settings (WASM)
cats-mobile:  scripts + Android bring-up
cats-mobile:  iOS bring-up
cats-mobile:  retire the Dart; README; CI (gate 2)
cats:         remove catgen-dart and its golden
```

## 7. Commands, for the next person

```sh
# pin a new cats revision (replaces tool/regen.sh)
go get github.com/rohanthewiz/cats@<sha> && go mod tidy
go test -race ./...            # includes the switch-coverage and viewer-mode tests

# browser loop
scripts/wasm.sh                # copies grmob-runtime.js + wasm_exec.js each build

# Android / iOS (from this module; see church_mobile/scripts for the originals)
scripts/build-android.sh && (cd ../grmob/android && ./gradlew assembleDebug)
scripts/build-ios.sh && (cd ../grmob/ios && xcodegen generate && xcodebuild ...)
```

During development, before the cats `wire` package is pushed, `go.mod`
carries `replace github.com/rohanthewiz/cats => ../cats`. Drop the replace
and pin a real sha before the README claims gate 2 is real.

## 8. Spike 1 result (2026-09-02): `wire` can be a leaf

Done on cats branch `spike/wire-leaf` (uncommitted scratch `wire/` directory,
throwaway). The answer is yes, and it is smaller than the plan feared.

What the carve-out actually is:

- `internal/browserproto/{proto,up,down}.go` + `internal/app/command_vocab.go`
  → one package, 3,526 lines. `cmd.go` disappears: it was only aliases of
  what `command_vocab.go` declares.
- **Two name collisions**, `WorkspaceInfo` and `TabInfo`: the layout-chrome
  shape in `down.go` and the `workspace.list`/`tab.list` row in the vocab.
  catgen-dart already resolves them with a rename table
  (`cmd/catgen-dart/types.go:86`): the vocab ones become `WorkspaceEntry`
  and `TabEntry`. Use the same names in Go so the Dart names carry over.
- `internal/flags` and `internal/layout` are pure leaves (no imports at all,
  1,160 lines), but `wire` does not need them: the only non-comment uses are
  **four server-side conversion helpers**, `SplitDirection`, `NavDirection`,
  `NewFlagInfo`, `optPaneID`. Those stay in `internal/app` (callers:
  `cmd/catway/catway.go`, `cmd/catctl/subcommands.go`,
  `internal/browserproto/layout.go`). With them out, the import closure of
  `wire` is the standard library only.
- The server-only builders `frame.go` and `layout.go` stay in
  `internal/browserproto`, which will import `wire`.

Verified:

```
GOOS=js GOARCH=wasm go build ./wire        OK
go vet ./wire                               OK
go list -deps ./wire | grep '\.'            github.com/rohanthewiz/cats/wire   (nothing else)
go build ./... && go test ./internal/{browserproto,app}   OK (nothing else touched)
```

And from a throwaway second module with `replace github.com/rohanthewiz/cats
=> ../cats`, importing only `wire`: builds and runs on the host, builds for
`GOOS=js GOARCH=wasm`, and pulls **none** of cats's dependencies (no pty, no
libghostty, no rweb), because Go only loads the packages imported.

One thing to fix in phase 1 rather than the spike: `wire.Marshal(&wire.Init{...})`
emits `"t":""` unless the caller sets `T` itself. Either `Marshal` should
stamp the type from the Go type, or `wire` should export constructors
(`NewInit` alongside the existing `NewWelcome`). The phone's `Connect` must
never be able to forget it.

Phase 1 is therefore mechanical: move the four files, apply the two renames,
move four helpers into `internal/app`, point `browserproto`, `app`, catway,
catctl and catgen-dart at `wire`, run `make check`.
