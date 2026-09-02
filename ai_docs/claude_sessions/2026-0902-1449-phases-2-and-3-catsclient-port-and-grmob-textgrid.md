# Session: Phases 2 and 3 — the catsclient port and grmob's TextGrid

- **Session ID:** `session_01YMfr7rAruzLSVjfgCL5zzM`
- **Date:** 2026-09-02
- **Branch:** main
- **Repos touched:** `cats-mobile` (this one), `grmob` (beside it)
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md`, §9 and §10 are the
  durable record of what landed; this doc is the narrative
- **Commits:**
  - cats-mobile `b112a16` — go.mod + internal/catsclient: port the protocol layer from Dart
  - cats-mobile `ce27423` — docs(plan): phase 3 result
  - grmob `65d96b2` — core.TextGrid: a monospace grid of styled runs, four renderers

## Request

> start on phase 2 of the plan

then

> start on phase 3

Phase 1 (the cats `wire` carve-out) had already landed on cats branch
`spike/wire-leaf` at `c0a250f`, and `CATS_REV` already pointed at it, so
both phases could go straight to work.

## Phase 2: `internal/catsclient`

The port is file-for-file, and every Go test file mirrors its Dart
counterpart so parity is checkable by reading the two side by side. 76 tests,
green under `-race`; the package builds and vets for `GOOS=js GOARCH=wasm`.

| Dart | Go |
|---|---|
| `endpoint.dart` | `endpoint.go`, `trust.go` |
| `connection.dart` | `conn.go`, `commands.go`, `backoff.go`, `ws.go`, `dial.go`, `dial_js.go` |
| `grid.dart` | `grid.go` |
| `session.dart` | `session.go` |
| `viewer_mode_test.dart` | `viewer_mode_test.go` |
| `wire_test.dart` | none; the codec is cats's, `wire/proto_test.go` covers it |

Things decided along the way, each written up in plan §9:

- **No `catgen-go` in cats yet.** `commands.go` is a generic `Call[R]` plus
  five hand-written helpers. One line per command as screens need them.
- **`wire.Marshal` still does not stamp `"t"`.** The connection stamps it in
  the handshake and in `Send`; two tests pin that. Still worth fixing in cats.
- **`Send` refuses `Focus` and `Raw`** on top of `Resize` and `Init`. A
  phone's focus report would unpark a TUI's caret at the desk.
- **`Session.Apply` has an explicit `ignoredDownTypes` list** with a reason
  per entry (`Welcome`, `CmdResult`, `Clipboard`, `History`, the five
  `chat_*`). `coverage_test.go` reads `DecodeDown`'s arms out of the pinned
  wire package with `go/ast` and fails on any type neither folded nor
  listed, and on a listed type the decoder no longer produces. This is the
  §4 "make it mechanical" item.
- **`viewer_mode_test.go` walks `app/` and `internal/` with `go/ast`**,
  skipping test files, and fails on a `wire.Resize` composite literal or a
  non-zero `Cols`/`Rows` key. Confirmed it fires with a throwaway offender.
- **WASM auth is the cookie route.** `coder/websocket` ignores upgrade
  headers under `GOOS=js`; `dial_js.go` dials bare and phase 4's pair screen
  must `POST /login` first. Native `dial.go` sends the bearer, pins the cert
  through `PinnedTLSConfig`, pings every 20 s.
- **Read limit raised to 16 MiB**; the library's 32 KiB default would drop
  the first full frame of a desktop-sized pane.

Concurrency model, as the plan drew it: one reader goroutine in `Conn`
decodes, settles the pending `Invoke` if the message is its reply, then hands
the message to `OnMessage`. `Session` carries no lock; the app holds it
behind a pointer with a mutex (church's `hooks.go` pattern). Every pending
call fails on disconnect, timeouts are per call with the 10-minute ceiling
reserved for `pane.wait_for_output`, and `Close` must not be called from
`OnMessage` because it waits for the reader to exit.

## Phase 3: grmob `core.TextGrid`

Spike 2 in the plan ("confirm the reconciler handles a row-keyed update
without re-sending the whole grid") was never run in phase 0, so it was run
here first, by reading `reconcile/patch.go`: children pair by index and
props compare with `reflect.DeepEqual`. That settled the shape before any
renderer was touched.

**Shape.** `TextGrid` is a container node with one `GridRow` child per row;
a row's runs (`GridRun{Text, Fg, Bg, Attr}`, json `t fg bg a`) are one prop.
Five attribute bits. No `core.Cached` per row is needed; an unchanged row is
one `DeepEqual` and no patch. Measured on an 80×24 grid of six-run rows: full
tree 7,430 bytes; one changed row is one 263-byte patch; three rows 787;
all 24 rows 6,303.

**Renderers.**
- Compose: `AnnotatedString` of `SpanStyle` runs in `FontFamily.Monospace`,
  `softWrap=false`, horizontal scroll. Rows keyed by index in the grid.
- SwiftUI: `AttributedString` in the `.monospaced` design, `lineLimit(1)`,
  horizontal `ScrollView`. An empty row gets a single space in the base font
  so it keeps its line height.
- WASM runtime and htmlout: `<pre>` of `<div>` rows of `<span>` runs, shared
  chassis (`margin:0; line-height:1.2; white-space:pre; overflow-x:auto`,
  rows `min-height:1.2em`). Dim is `opacity:0.6`.

**The bug the JS test caught.** The runtime's `styleFromGrMob` reassigns
every property on every call, deliberately, so an `update-style` patch can
clear a field that went back to zero. A grid chassis set at element creation
was therefore wiped by the first style patch. The chassis now lives inside
`styleFromGrMob`, keyed on the node type, with the author's values winning
where set. Worth remembering for any future node with fixed styling: put it
in `styleFromGrMob`, not `createElement`.

**Tests, by layer.** `core/textgrid_test.go`; `reconcile/textgrid_test.go`
(one patch per changed row, add/remove on resize); `htmlout/textgrid_test.go`
plus the existing tag-table pin; `wasm/verify/textgrid_test.mjs` driving
mount and `update-props` through the real runtime file, plus the existing
runtime/htmlout tag comparison, which is what forced both tag tables to gain
the two rows; `mobile/verify/textgrid_test.go`, which parses both native
type dispatches and requires a `TextGrid` and a `GridRow` arm.

**Verified on this machine:** grmob `go test ./...`, `wasm/verify/run.sh`,
Gradle `compileDebugKotlin` (exit 0, fresh `RendererKt` classes),
`swiftc -parse`. **Not verified:** any render on a device or simulator.

grmob's ROADMAP now lists the native gaps cats-mobile will want next: deep
links, clipboard, haptics, local notifications, lifecycle events.

## Gotchas worth carrying forward

- macOS has no `timeout`; use the tool's own timeout for long Gradle runs.
- zsh spells `$pipestatus` in lowercase; `${PIPESTATUS[0]}` is empty there.
- grmob's `mobile/verify` switch parser wants one arm per line, string
  literals first, catch-all last. The two new arms follow that.
- The grmob wasm harness DOM throws on `appendChild` onto an element with
  `textContent`; `GridRow` sets none and its spans are outside the node tree,
  which is also what keeps positional patch addressing intact.

## What's next

Phase 4, the app on WASM first. Plan §10 has the mapping notes: fold cats's
ratatui modifier bits onto the five grid bits, swap fg/bg for reverse video,
coalesce adjacent equal cells into runs, and add `replace grmob => ../grmob`
to `go.mod` once there is a consumer. grmob `65d96b2` is not pushed; neither
are this repo's commits until `/sess-wrap` does it.
