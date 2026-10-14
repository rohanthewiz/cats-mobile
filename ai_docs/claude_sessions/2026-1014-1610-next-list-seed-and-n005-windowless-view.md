# Session: the living Next list, and N-005 (a view whose window closed)

- **Session ID:** `81560d9a-3a18-4895-8f12-7b9524eb9ff4`
- **Date:** 2026-10-14
- **Branch:** main
- **Previous session:** `2026-1014-1554-n020-plugin-panes-split-by-type.md`

## Requests, in order

1. `/next-list seed`: build `ai_docs/todo/next-list.md` from the
   session-doc history.
2. "drop N-002, looks like a repo mixup. Church has nothing to do with cats."
3. "Yes, also drop N-013" (the other church item).
4. "Do N-005."
5. `/sess-wrap`.

## The living list

`ai_docs/todo/next-list.md` is new. From now on, sessions edit it in place,
and a session doc's `## Next` is only a `Closed: … Raised: …` summary.

- **Window:** all 17 docs (`2026-0902-1316` → `2026-1014-1554`), not the
  default 15, so no `raised` is cut off by the window.
- **IDs N-001 to N-018** were assigned oldest-raised first, as one sequence
  across every section.
- **Lapsed items: none.** Every item that dropped out of a hand-carried list
  between docs was done, and the later doc's closure table says so. Three
  commits have no session doc (`0b38a3c`, `a228479`, `8c050f3`), and none of
  them raised or dropped an item.
- **Ages in the old lists ran one low** for the items raised before
  `2026-0914`. That rebuild didn't count itself, and the error carried
  forward. Camera QR is age 14, not 13.
- **Premise checks against the code:**
  - **N-003 (leftover processes) closed.** `ps` showed no catway/cathost
    started by a session, and no emulator or simulator. The only ones
    running were `/Applications/Cats.app`'s own pair, started 2026-10-09.
  - **N-014 (`scripts/wasm.sh` reads `../grmob`) is now a live mismatch.**
    `../grmob` is at `v0.6.0-8-gcdfa8ee` while `go.mod` pins v0.5.0, so a
    wasm build pairs newer runtime JS with the v0.5.0 Go engine.
  - **grmob v0.6.0 doesn't fix N-001 or N-012.** It still has no
    `Camera.kt`/`Camera.swift`, and only `CodeEditor` turns off
    autocapitalization. It does add `core.AlignSelf` (`99dc23e`), which is
    noted on the N-007 non-goal.
- **N-006 (no iOS CI) went to Validate.** What remains is a CI job: testing
  work, with no product change.
- **N-002 and N-013 (church_mobile) closed as a repo mixup**, on the user's
  call. They came in because phase 4 used church_mobile's `app/` as a model,
  and because a later session migrated church to a grmob release. A memory
  now keeps church items off this list.

## N-005: the followed window closes

**The defect.** A follow pins a *workspace*, not a window, because the census
gives connections no ids. When the followed window closes, the server keeps
the phone on that workspace, which is correct. But `FollowedWindow()` joins
the layout's active workspace to the census, and the census has just lost
that entry. So the Windows tab showed no Showing badge on any row and no
"showing" clause in the census line. The phone was watching something the
screen didn't account for. This was first seen at step 6 of the
`2026-0902-2211` walk.

**What changed:**

- `internal/catsclient/session.go`: added `Session.ViewWithoutWindow() (label
  string, ok bool)`. It is true when the census and the layout have both
  arrived, the layout's active workspace is known, and no desktop window
  shows it. The label is the workspace name, or the id until the layout names
  it. It reads the same two server facts as `FollowedWindow`, never the local
  pin, so it also covers the moment an unpinned viewer's census and layout
  cross on the wire.
- `app/windows.go`:
  - `censusLine` takes the windowless label and reads
    `… · showing grmob (no window)`.
  - A `noticeStrip` under the census line says "No desktop window is showing
    grmob now. The phone stays on it until you pick a window or follow the
    primary view."

### Decisions

- **No Showing badge.** The rows are desktop windows. A made-up row for a
  workspace no window shows would put a window on screen that doesn't exist.
  So the line and the notice say it instead.
- **The notice has no button.** "Follow the primary view" already sits at the
  bottom of the list whenever the phone is pinned, and two buttons for one
  command would be clutter.
- **Keep the "showing" clause and mark it.** Dropping it would leave the line
  silent about what the phone shows, at the one moment that is no longer
  obvious.

### Tests

- `TestWindowsAViewWhoseWindowClosedIsReportedByName` (catsclient).
- `TestWindowsViewWithoutWindowWaitsForBothCensusAndLayout` (catsclient).
- `TestWindowsTabSaysSoWhenTheFollowedWindowCloses` (app). It closes the
  window, checks the census line and the notice, and checks that no "Showing"
  appears. Then it reopens a window on the workspace and checks that the
  notice goes and the badge comes back.
- `go vet ./...` and `go test -race ./...` pass, and
  `scripts/wasm.sh --build-only` builds. The `windows` golden is unchanged:
  its fixture has a window on the view.

## Gotcha

`rg` stands in for `grep` in this shell. `grep -l` across files therefore
prints its matches in no fixed order, which nearly mis-dated items during the
seed. Use `command grep` in a loop over `ls | sort` when the order matters.

## Elsewhere

- Memory added: `church-not-cats-scope`.
- The untracked `.cats-todo/`, `.dart_tool/`, `packages/` and `pubspec.lock`
  predate this session and were left alone.

## Next

Seeded: N-001 to N-018. Closed: N-002, N-003, N-005, N-013. Declined: None.
Raised: N-019. Deferred: None. Promoted: None. Moved: N-006 → Validate (at
seeding). Updated: N-014. Full list: `ai_docs/todo/next-list.md`.
