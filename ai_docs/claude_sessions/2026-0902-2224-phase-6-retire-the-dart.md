# Session: Phase 6 — retire the Dart, close the loop

- **Session ID:** `session_01VuwCDSFhQyb1Umq1ojZ2aT`
- **Date:** 2026-09-02
- **Branch:** main (cats-mobile); cats `spike/wire-leaf`
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md` §3 Phase 6 → result in §13
- **Previous session:** `2026-0902-2211-phase-5-windows-follow-walk.md`

## Request

> what's left in plan §12? … let's start phase 6

§12 was a results section with nothing unwalked; the open items were all
grmob-side (lifecycle hook, four cosmetics) or observations. Phase 6 from §3
was the next step.

## Commits

cats-mobile, main:

| sha | subject |
|---|---|
| `2d61982` | retire the Dart: the Go port is the client now |
| `70c6f24` | docs+ci: the README in Go terms, and gate 2 as a workflow |
| `8b2522b`, `c143476` | docs(plan): §13, phase 6 result |

cats, `spike/wire-leaf`:

| sha | subject |
|---|---|
| `5add396` | remove catgen-dart and its golden: the phone imports wire directly |

## What was done

1. **Parity gate, both ways, before deleting anything.** `dart test` in
   `packages/catsproto`: 85 passed. Then `go test -race ./...` with a
   scratch modfile that omitted the `../cats` replace, so the Go suite ran
   against the *pushed* `c0a250f` from the proxy: green, including
   `TestEveryDownTypeHasAnArm`. That justified dropping the replace for real.
2. **Deleted** `packages/`, `pubspec.yaml`, `pubspec.lock`, `CATS_REV`,
   `tool/regen.sh`, `.dart_tool`; `.gitignore` reduced to the Go-era lines.
   `go.mod` lost the replace and its comment no longer mentions CATS_REV.
   Comments in `internal/catsclient` (`doc.go`, the four "Mirrors …" test
   headers, `commands.go` untouched) and `app/register.go` now describe the
   Dart as lineage, not a neighbour.
3. **README** rewritten: Go layout, the four viewer-mode layers as
   `Conn` builds Init / `Send` refuses Resize+Focus+Raw / `FollowWorkspace`
   gates on `CapWindow` / `viewer_mode_test.go` AST walk, the bump recipe
   (`go get cats@sha && go mod tidy && go test`), the two tests that catch a
   bad bump, device build notes.
4. **CI** `.github/workflows/ci.yml`, modelled on grmob's: a `go` job
   (gofmt, vet, `go test -race`, `GOOS=js` build of `./wasm`) and an
   `android` job (`go tool gomobile bind` of the AAR from this module, with
   the pinned gobind built into `RUNNER_TEMP/bin` first). No iOS job; no
   gradle step, since the Kotlin shell lives in grmob. Not yet run: the
   repo has no remote push in this session.
5. **cats:** `git rm -rf cmd/catgen-dart docs/protocols/dart-client.md`,
   `mkdocs.yml` nav entry dropped, `docs/index.md` layout block and the
   command-table paragraph in `docs/protocols/index.md` reworded, comment
   pointers in `internal/flags/flags.go`, `internal/app/command_vocab_test.go`
   and `wire/vocab_test.go` updated. `go build ./...` and the three packages'
   tests green.

## Gotchas

- **Another session was live in `../cats`.** Its work-in-progress was
  unstaged when I started, then staged (index mtime changed under me), then
  committed as `be61142` two minutes later. Selective `git add` of only my
  paths plus waiting for that commit kept the two apart. Lesson: check
  `git status` and `.git/index` mtime in a shared checkout before staging.
- `git rm -r` refuses a directory holding modified files (the golden had
  uncommitted `pane.keep` regenerations); `-f` is right there because the
  whole directory is going.
- `set -e` inside a `cd ../cats && …` chain does not stop the chain after a
  failing `git rm`; the later steps ran anyway. Check the first step's
  output before trusting the rest.
- `go tool gomobile version` prints "binary is out of date, re-install it";
  that is gomobile's own stamp check and harmless (grmob's CI uses the
  same invocation).
- Running `dart test` from the workspace root fails ("no test/ directory");
  it must run inside `packages/catsproto`.
- `-modfile=<scratch>/go.mod` needs a `go.sum` beside it; copying the real
  one works, and `GOFLAGS=-mod=mod` lets it fetch the pinned commit.

## Still open

- `wire` is on `spike/wire-leaf`, not cats main. CI resolves the pinned
  commit through the proxy only while that branch stays pushed; merging the
  carve-out into main is the real close of the loop.
- `wire.Marshal` still does not stamp `"t"` (§8, §9).
- The first CI run has not happened; watch the Android job's NDK fallback
  and the `go tool gomobile` path on a hosted runner.
- The emulator, cathost, catway (:8421) and the fake `claude` from the
  earlier session may still be running; kill by pid if so, never `killall`.
