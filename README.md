# cats-mobile

The phone client for [cats](https://github.com/rohanthewiz/cats): see what
every agent across every workspace is doing, get pushed when one blocks, and
reply, without needing a laptop.

Written in Go on [grmob](https://github.com/rohanthewiz/grmob), so one codebase
builds for Android, iOS and the browser, and the wire layer is not generated
into a second language: the app imports cats's own `wire` package, and `go.mod`
is the pin.

Its own repo, following the `cats-todo` precedent: cats's CI is Go + Zig + a
vendored libghostty build, and the Android SDK and Xcode would triple that
matrix for a repo where almost no commits touch the phone.

## Layout

```
app/                     the UI: one file per screen, grmob nodes over catsclient
  register.go            registers the root view with grmob's mobile bridge
  connection.go          pair → dial → reconnect loop; owns the catsclient.Conn
  agents.go, windows.go, pane.go, alerts.go, more.go, pair.go
  gridview.go            catsclient.Grid → core.TextGrid
  app_test.go            bridge journeys against a fake catway
  snapshot_test.go       htmlout goldens under testdata/
internal/catsclient/     the protocol layer: Endpoint, Conn, Session, Grid
  session.go             the switch over every down-message type
  coverage_test.go       fails when wire grows a down type session.go ignores
  viewer_mode_test.go    the go/ast guard described below
internal/store/          bytdb persistence: endpoints, tokens, certificate pins
wasm/                    the browser entry point (scripts/wasm.sh)
scripts/                 build-android.sh, build-ios.sh, wasm.sh, lib.sh
```

The split keeps grmob out of `internal/`: `catsclient` imports only `wire` and
`coder/websocket`, so "no UI types in the protocol layer" is a compile-time
fact, and it is where most of the tests live.

## The organizing principle

**The phone observes and replies; it never rearranges the desktop.**

catway takes the session grid from the *first* connection that declares one,
and shares it with every other. A phone honestly reporting 40×20 would reflow
every pane for everybody at the desk. So a cats client always connects as a
viewer, and four independent layers enforce it:

1. `catsclient.Conn` builds the `Init` itself, with zero cols and rows and
   `Viewer: true`. App code cannot supply one.
2. `Conn.Send` refuses a `Resize` outright. It also refuses `Focus` (the
   server folds every connection's focus into one "is anyone looking" bit that
   parks a TUI's caret at the desk) and the deprecated pre-encoded `Raw`.
3. `FollowWorkspace` and `FollowPrimaryView` check for `wire.CapWindow` before
   sending `workspace.focus`; on an older server the same command is a
   session-wide switch.
4. `viewer_mode_test.go` walks `app/` and `internal/` with `go/ast` and fails
   on any `wire.Resize` literal and any non-zero `Cols`/`Rows` in an `Init`.
   It sees through import aliases, which a grep would not.

Anything that *would* move the desktop's viewport (`agent.focus`, `tab.focus`,
`pane.zoom`) stays behind an explicit confirmed gesture, never a side effect of
navigating the app.

`workspace.focus` is the exception, on a server that advertises the `window`
capability: there a connection is a **view**, the command moves only the
connection that sent it, and picking which desktop window to watch disturbs
nobody. The pin rides `Init.Workspace` through a reconnect. Which window the
phone is *actually* looking through comes from the server, not the pin:
`Session.ViewWorkspace` reads the layout's own active flag, because a pinned
workspace can be closed at the desk and the server falls back silently.

## Keeping the phone in lockstep with cats

`go.mod` pins the cats commit the app is built against. That is the whole
mechanism: there is no generated mirror, no golden, no separate revision file.

```sh
go get github.com/rohanthewiz/cats@<sha> && go mod tidy
go test ./...
```

Two things can go wrong on a bump, and each has a test:

- **A new down-message type.** The compiler adds the type but has no opinion
  about what the session does with one. `TestEveryDownTypeHasAnArm` fails on a
  type that `session.go` neither folds nor lists as deliberately ignored, and
  on a listed type the decoder no longer produces.
- **A new command the app should refuse.** `viewer_mode_test.go` is the guard,
  and `Conn.Send`'s refusal list is the place to extend.

At runtime, a `welcome.v` mismatch shows "app update required" rather than a
stream of decode failures: catway requires exact protocol-version equality and
closes the socket, so there is no partial-compatibility mode to guess at.

## Develop

```sh
go test -race ./...            # the race build matters: the reader goroutine is the design
go vet ./...
scripts/wasm.sh                # build + serve the browser preview on :8080
```

The browser preview pairs through `POST /login` and a same-site cookie, so run
catway on the same host and trust its certificate in the browser first.

### Devices

Both scripts bind grmob's `mobile` bridge plus `./app` from *this* module into
grmob's native shells, so a grmob checkout is needed beside this one (or set
`GRMOB`). `scripts/lib.sh` warns when that checkout is not at the tag `go.mod`
builds against; the Kotlin/Swift half and the Go half are versioned together.

```sh
go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
scripts/build-android.sh --install     # AAR, then gradle + adb install
scripts/build-ios.sh --sim             # xcframework, then xcodegen + xcodebuild
```

No server URL is baked in: the phone learns its catway by pairing. On the
Android emulator the host is `10.0.2.2`; on the iOS simulator it is
`localhost`. Both shells already permit cleartext to those, so a catway
started without `--tls` works for development. A real device talks `wss` with
the certificate pinned on first pair.

## CI

`.github/workflows/ci.yml` runs gofmt, `go vet`, `go test -race`, the WASM
build, and a `gomobile bind` of the Android AAR, on every push and pull
request.
