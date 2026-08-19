# cats-mobile

The phone client for [cats](https://github.com/rohanthewiz/cats) — see what
every agent across every workspace is doing, get pushed when one blocks, and
reply, without needing a laptop.

Its own repo, following the `cats-todo` precedent: cats's CI is Go + Zig + a
vendored libghostty build, and adding Flutter, Xcode and the Android SDK to that
matrix would triple CI time for a repo where almost no commits touch Dart.

## Layout

```
CATS_REV                   the cats commit the Dart wire code was generated from
packages/catsproto/        pure Dart, zero runtime deps, `dart test` in ~1s
  lib/src/generated/       emitted by cmd/catgen-dart, in the cats repo
  lib/src/                 connection, endpoint, grid, session, sha256
packages/cats_mobile/      the Flutter app (not yet)
```

The split makes "no Flutter types in the protocol layer" a compile-time fact
rather than a code-review convention, and the wire package is where the tests
actually live.

## The organizing principle

**The phone observes and replies; it never rearranges the desktop.**

catway takes the session grid from the *first* connection that declares one, and
shares it with every other. A phone honestly reporting 40×20 would reflow every
pane for everybody at the desk. So a cats client always connects as a viewer,
and four independent layers enforce it:

1. `CatsConnection` builds the `init` itself, with hardcoded zeros and
   `viewer: true`. App code cannot supply one.
2. `CatsConnection.send` refuses a `Resize` outright.
3. The generator marks `Resize` `@Deprecated`, and `analysis_options.yaml`
   promotes `deprecated_member_use` to an **error**.
4. `test/viewer_mode_test.dart` reads the source and fails on any `Resize(` or
   non-zero `cols:`/`rows:` under `lib/`.

Anything that *would* move the desktop's viewport — `agent.focus`, `tab.focus`,
`pane.zoom` — stays behind an explicit confirmed gesture, never a side effect of
navigating the app.

`workspace.focus` used to belong on that list and no longer does. On a server
advertising the `window` capability a connection is a **view**: the command
moves only the connection that sent it, so picking which desktop window to watch
disturbs nobody. `CatsConnection.followWorkspace` pins one,
`followPrimaryView` lets go again (an empty id), and `Init.workspace` carries
the pin through a reconnect. All three check the capability first — on an older
server the same command is still a session-wide switch, and sending it there is
the one bug in the phone that would rearrange somebody's desk. Which window the
phone is *actually* looking through comes from the server, not from the pin:
`CatsSession.viewWorkspace` reads the layout's own active flag, because a pinned
workspace can be closed at the desk and the server falls back silently when it
is.

## Keeping Dart in lockstep with Go

Everything under `packages/catsproto/lib/src/generated/` comes from
`cmd/catgen-dart` in the cats repo. Do not edit it.

```sh
cd ../cats
go run ./cmd/catgen-dart -out ../cats-mobile/packages/catsproto/lib/src/generated
git rev-parse HEAD > ../cats-mobile/CATS_REV
```

The key table needs a Flutter SDK, because its input is one of the SDK's own
data files. It changes only on an SDK upgrade:

```sh
go run ./cmd/catgen-dart \
    -out ../cats-mobile/packages/catsproto/lib/src/generated \
    -flutter-root "$FLUTTER_ROOT"
```

The generated files carry `// dart format off`, so they are byte-identical to
the golden committed in cats. Do not reformat them; a reformatted copy would
make the drift gate compare a transformation of the output instead of the
output.

### Three drift gates

1. **In cats.** The generated files are committed under
   `cmd/catgen-dart/testdata/golden` and diffed by `make check`. Adding a field
   to `PaneFrame` without regenerating fails the *cats* build — the same rigor
   `TestCommandSpecsRouted` applies to the command table.
2. **Here.** CI regenerates at `CATS_REV` and fails on `git diff`, so this
   repo's copy cannot fall behind the pin.
3. **At runtime.** A `welcome.v` mismatch shows "app update required" rather
   than a stream of decode failures. catway requires exact protocol-version
   equality and closes the socket, so there is no partial-compatibility mode to
   guess at.

## Develop

```sh
dart pub get
dart test
dart analyze
dart format --set-exit-if-changed .
```
