# Session: How cats and cats-mobile stay in lockstep

- **Session ID:** `cc08ac4d-9540-49ad-a0fa-84f2ec69d875`
- **Date:** 2026-09-02
- **Branch:** main
- **Repo:** `cats-mobile` (worked from a `cats` checkout beside it)
- **Commit:** `5d2d400` — regenerate the wire layer against cats `951a8d9`
- **First session doc in this repo.** `ai_docs/claude_sessions/` is created here;
  the sessions that built `catsproto` were all recorded in `cats`.

## Request

> ensure cats-mobile is up-to-date with cats. I think the catgen-dart tool is
> used here

Correct on both counts, and the second half is the part worth writing down: the
answer to "is the phone current?" is never read off the Dart. It is read off a
generator in the *other* repo.

## The relationship

Two repos, one protocol, and the protocol lives in exactly one of them.

```
cats/                                    cats-mobile/
  internal/app/*.go          ─┐            CATS_REV          ← the pin
  internal/browserproto/*.go  ├─ TRUTH     packages/catsproto/
  internal/orchestration/*.go─┘              lib/src/generated/  ← emitted, never edited
  cmd/catgen-dart/            ← generator    lib/src/            ← hand-written
    testdata/golden/*.g.dart  ← gate #1        connection, endpoint, grid, session
```

`cmd/catgen-dart` reads the Go wire structs and command table and emits five
Dart files. It is deliberately hybrid — `reflect` for the field set, JSON tags,
`omitempty`, embedding and the ~50 type aliases in `browserproto/cmd.go`;
`go/ast` for the doc comments and const blocks. Neither alone is enough, and the
doc comments are the best documentation in cats, so they are worth the second
pass.

The repos are split because cats's CI is Go + Zig + a vendored libghostty
build. Adding Flutter, Xcode and the Android SDK to that matrix would triple CI
time for a repo where almost no commits touch Dart. Same reasoning as
`cats-todo`.

The split has one cost, and the whole mechanism below exists to pay it: a wire
change in cats is invisible to cats-mobile until somebody regenerates.

### The one directional rule

**cats never depends on cats-mobile.** The generator's output is committed in
*both* repos, but cats's copy under `cmd/catgen-dart/testdata/golden/` is the
original and this repo's copy is a mirror. That is not redundancy — it is where
the gate bites, explained below.

### The organizing principle on the wire

Worth restating because it constrains what an update is allowed to add: **the
phone observes and replies; it never rearranges the desktop.** catway takes the
session grid from the first connection that declares one, so a phone honestly
reporting 40×20 would reflow every pane at the desk. Four independent layers
enforce viewer-only — the handshake, `CatsConnection.send`, a `@Deprecated`
`Resize` promoted to an analyzer *error*, and a test that greps `lib/` for
`Resize(`. Any regeneration that introduced a new desktop-moving command would
need that reviewed; this one didn't.

## The update mechanism

### The three files that matter

| file | role |
|---|---|
| `CATS_REV` | the cats commit the generated Dart was emitted from |
| `packages/catsproto/lib/src/generated/*.g.dart` | the emitted output, five files |
| `cats/cmd/catgen-dart/testdata/golden/*.g.dart` | the same bytes, committed in cats |

### Running it

There is a script for this — `tool/regen.sh` — and it should be used rather
than the two commands typed by hand. It defaults to `../cats`, refuses a
directory with no `go.mod`, re-pins `CATS_REV` in the same breath, and prints
the FLUTTER_ROOT note. Its own header says why it exists: *"a regeneration into
the wrong directory silently leaves the old files in place and the new ones
somewhere nobody looks."*

```sh
tool/regen.sh              # ../cats
tool/regen.sh ~/src/cats   # elsewhere
```

Equivalent by hand, which is what this session actually ran before finding the
script:

```sh
cd ../cats
go run ./cmd/catgen-dart -out ../cats-mobile/packages/catsproto/lib/src/generated
git rev-parse HEAD > ../cats-mobile/CATS_REV
```

### keys.g.dart is the exception

Four of the five files come from Go. `keys.g.dart` comes from a *Flutter SDK*
data file (`physical_key_data.g.json`), so it needs `-flutter-root` /
`FLUTTER_ROOT`. Without it the generator **omits the file** rather than emitting
an empty one — so a regeneration with no SDK on hand leaves the committed table
untouched, which is the right default. Its input changes only on an SDK upgrade.

### Never reformat the generated files

They carry `// dart format off` so they stay byte-identical to cats's golden. A
reformatted copy would make the drift gate compare a *transformation* of the
output against the output, which passes when it should fail and fails when it
should pass.

### The three drift gates

1. **In cats — the one that actually works.** `TestGoldenIsUpToDate` diffs the
   generator's output against `testdata/golden` and runs in `make check`. The
   point is that the gate bites *in the repo where the drift happens*: adding a
   field to `PaneFrame` is a cats commit, and that commit fails its own build
   rather than quietly leaving the phone a version behind until somebody notices
   a missing value on a screen. Same rigor `TestCommandSpecsRouted` applies to
   the command table.
2. **Here — not built yet.** The README describes CI that regenerates at
   `CATS_REV` and fails on `git diff`. `.github/workflows/` does not exist in
   this repo. Gate 1 is doing all the work today, and gate 1 cannot detect *this*
   repo falling behind — only that cats's own copy is current. Which is exactly
   the drift this session found.
3. **At runtime.** A `welcome.v` mismatch shows "app update required" rather
   than a stream of decode failures. catway requires exact protocol-version
   equality and closes the socket, so there is no partial-compatibility mode to
   guess at.

### What "up to date" is verified by

Not "the tests pass". Byte equality with cats's golden:

```sh
for f in attrs codec commands keys wire; do
  cmp -s ../cats/cmd/catgen-dart/testdata/golden/$f.g.dart \
         packages/catsproto/lib/src/generated/$f.g.dart \
    && echo "$f ok" || echo "$f DRIFT"
done
```

## What this session found

`CATS_REV` was pinned at `942e7f8`, **40 commits behind** cats `951a8d9`. cats's
own golden test was green — gate 1 had nothing to say, because cats was
perfectly in sync with itself. Only gate 2, the one that doesn't exist, watches
this direction.

Two of five files moved. Purely additive:

- **commands** — `flag.pane`, `flag.workspace`, `flag.list` with their params
  and results; `FlagInfo` on `PaneInfo` and `WorkspaceEntry`.
- **wire** — three new down-messages: `pane_respawned`, `record`,
  `runbook_runs` (with `RunbookRun`).

No new capability bit, no new deprecation, nothing touching the viewer-mode
invariants.

## The part the generator does not do

This is the reusable lesson, and it is why "regenerate and commit" is not the
whole job.

`CatsSession.apply` is a hand-written `switch` over decoded down-messages. The
generator adds the *types* and teaches `decodeDown` to build them; it has no
opinion about what the session should *do* with one. A new down-message
therefore lands in a client that decodes it perfectly and then drops it on the
floor — silently, with no analyzer warning, because the switch has no default
arm to miss.

So: **after every regeneration, diff the new down-messages against
`session.dart`'s switch.** Three landed here, and one of them was a live bug
rather than a missing feature:

- **`PaneRespawned` → `exitCodes.remove(m.pane)`.** A pane's death is
  *remembered* by the client, never re-derived from the layout — the chrome a
  late joiner gets simply omits `pane_exited` for a live pane, which is exactly
  why the server has to tell an already-connected client. Without this arm the
  phone keeps drawing "exited (130)" over a live shell until reconnect. Removing
  the key rather than storing a sentinel keeps "is this pane dead" the plain
  `containsKey` it already was.
- **`RecordMsg` → nullable `record`.** Nullable for the same reason `clients`
  is: a server too old to send it sends nothing, so null means *unknown*, not
  *idle*. The phone should draw no recorder indicator rather than an unlit one
  it cannot vouch for.
- **`RunbookRuns` → `runbookRuns`, replaced wholesale.** The message carries the
  entire in-flight set, so a finished run is *absent* from the next push rather
  than marked done in it. Merging would leave it on screen forever.

Two tests added in `session_test.dart`. `dart test` green (85), `dart analyze`
clean.

## A snag worth knowing about

Under this machine's Dart 3.10.4, `dart format` also wants to reformat
`views_test.dart` and `wire_test.dart` — the new-style formatter disagrees with
whatever SDK last formatted them. That churn predates this work and was
deliberately kept out of the commit; folding unrelated reformatting into a
regeneration commit makes the interesting diff unreadable.

It is still sitting there. Whoever adds the CI format gate (see gate 2) will
trip it on the first run and should reformat those two files in a commit of
their own first.

## Known limits / next

- **Gate 2 does not exist.** `.github/workflows/` is empty in this repo. Until
  it is written, "is cats-mobile current?" is a question only a human thinks to
  ask — and the answer this session was "40 commits, no."
- The two-file format churn above, unresolved.
- Nothing surfaces `record` or `runbookRuns` in a UI, because
  `packages/cats_mobile/` — the Flutter app — has not landed. The protocol layer
  is deliberately ahead of it; that is the split working as intended.
- `flag.*` is decodable but unused: `FlagInfo` rides on `PaneInfo` and
  `WorkspaceEntry` and needs no session plumbing, so there is no equivalent
  "dropped on the floor" gap there.

## Commands, for the next person

```sh
tool/regen.sh                                   # regenerate + re-pin
cd packages/catsproto && dart test && dart analyze
cat CATS_REV && (cd ../cats && git rev-parse HEAD)   # should match
```
