# Session: Phase 4 — the app on WASM

- **Session ID:** `session_01YMfr7rAruzLSVjfgCL5zzM`
- **Date:** 2026-09-02
- **Branch:** main
- **Repos touched:** `cats-mobile` (this one). grmob and cats were read,
  not changed.
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md`, §11 is the durable
  record; this doc is the narrative
- **Commit:** `1d8ef28` — app: phase 4 — the cats app on grmob, WASM first

## Request

> start on phase 4

Phases 1–3 had landed (cats `wire` on a branch, `internal/catsclient`,
grmob `core.TextGrid`), so this went straight to the app.

## What was built

`app/` (package `catsapp`), `internal/store`, `internal/catsclient/login*.go`,
`wasm/`, `scripts/wasm.sh`. church_mobile's `app/` was the model throughout:
its `ui.go`, `root.go`, `hooks.go`, the singleton `Services`, the test
harness and the WASM host were all read first and adapted rather than
reinvented.

| file | what |
|---|---|
| `app/register.go` | init → `mobile.Register`, `AppName` (gobind needs it) |
| `app/services.go` | `Services{Store, Conn}` singleton; `Bind` dials the active endpoint |
| `app/connection.go` | reconnect loop, `Status`, generation numbers, `Read`/`Info`/`Live` |
| `app/gridview.go` | `PaneGrid` → `[]core.GridRow`: run coalescing, attr bits, reverse swap |
| `app/root.go` | `App`, `shell` (pair screen until paired, else 4 tabs + banner) |
| `app/pair.go` | paste link or host/port/password; `Login` redemption; pin; connect |
| `app/agents.go` | roster grouped by state (order is `Session.Roster`'s) |
| `app/windows.go` | census, follow / follow primary behind `CapWindow` |
| `app/pane.go` | chrome + `TextGrid` + composer + confirmed "reveal at the desk" |
| `app/alerts.go` | notifications newest-first, nullable record indicator, runbook runs |
| `app/more.go` | connection, caps, endpoints, forget device, cats sha via `ReadBuildInfo` |
| `app/theme.go`, `ui.go`, `format.go` | dark theme from the server's `theme` message; shared vocabulary |
| `internal/store` | bytdb `TrustStore` + endpoint list; memory fallback with no data dir |
| `internal/catsclient/login*.go` | POST /login: JSON+bearer on native, cookie on WASM |

Two small additions to `catsclient.Conn`: `Done()` (reader exited) and
`Err()` (why), which the reconnect loop selects on.

## Decisions

- **Composer sends `pane.send_input`, not Paste + Key.** Addressed by
  construction; Paste/Key need `CapKeyPane` or they ride the desk's shared
  focus. A bridge test asserts nothing else leaves the phone.
- **WASM auth is the cookie.** `login_js.go` posts without Accept JSON and
  sets `js.fetch:credentials: include`; the store holds the literal token
  `"cookie"` so `ReadToken` reports paired. Same-site strict means the
  preview page must share the catway's origin or a localhost loop.
- **Generation-numbered connection loop.** `Connect` bumps a counter; writes
  from an older loop are dropped. Cert mismatch, refused credential and
  protocol mismatch are hard stops; everything else walks the backoff, and
  `Retry` wakes a waiting loop.
- **Copy-on-write form state.** `-race` caught the pane composer mutating a
  `core.State` pointer struct from a goroutine while a render read it.
  `update` now copies the latest, mutates the copy, stores that.
  church_mobile's login/giving forms have the same race.
- **Theme ink must be pushed into typography.** `components.ListRow` draws
  titles in `Typography.Body` (DefaultTheme black); every list title was
  invisible on the dark ground until `themeFor` set Body/Caption/Title/
  Subtitle, `Components.Text`, Input and TextArea from the palette.
- **ratatui attribute bits retyped in `gridview.go`** (cats keeps them
  unexported); all six pinned by test. Trailing blank runs trimmed per row.

## Tests

`app/app_test.go`: a `fakeDesk` Dialer hands out in-memory sockets; the
harness drives `render.Manager` like a native shell. Journeys: unpaired →
pair screen; boot sends a viewer `init` with the stored bearer and pin;
roster groups; open a pane, frame, diff, back; reply sends one addressed
`pane.send_input`; socket drop → banner → second dial, old roster gone;
cert mismatch is one dial; Windows sends `workspace.focus`; Alerts
newest-first, no record until told; typed input survives keystrokes.
`snapshot_test.go` pins six htmlout goldens in `app/testdata/` (`go test
./app -update`). `gridview_test.go` covers the grid conversion.

**Verified:** `go test -race ./...` ×3, gofmt, vet, `GOOS=js` builds of
`./app` and `./wasm`, `scripts/wasm.sh --build-only`, and the bundle mounted
in Chrome with no console errors; the pair form takes typed input.
**Not verified:** pairing against a live catway; any native build.

## Gotchas worth carrying forward

- Chrome automation screenshots are scaled: click coordinates from a
  screenshot landed on the wrong input, which looked exactly like "the app
  loses keystrokes". Confirm with `document.activeElement` before blaming
  the app.
- `Endpoint.HTTPURL()` ends in `/`; trim before appending a path.
- `wire.UsageWindow.Pct` is a float64; `UsagePctUnknown` is -1.
- `ctx.Scope("pair")` for the pair screen keeps its hook slots apart from
  the tab shell's; the shell calls `NewState` first, unconditionally.

## What's next

Phase 5, device bring-up (Android first): copy church's `scripts/`, bind
from this module with the `tool` block pinning x/mobile, re-read church's
gotchas in plan §3. Open gaps listed in plan §11: camera QR (gozxing),
notification actions untested live, no foreground/background lifecycle hook
(grmob ROADMAP). grmob `65d96b2` is still unpushed.
