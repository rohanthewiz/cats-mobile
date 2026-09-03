# Session: Phase 6 close — wire on cats main, first CI run

- **Session ID:** `session_01VuwCDSFhQyb1Umq1ojZ2aT`
- **Date:** 2026-09-02 (late; the pushes land as 2026-09-03 UTC)
- **Branch:** main (cats-mobile); main (cats)
- **Plan:** `ai_docs/plans/2026-0902-move-to-grmob.md` §13
- **Previous session:** `2026-0902-2224-phase-6-retire-the-dart.md`

## Requests, in order

> Merge the wire package in
> push cats main
> push cats-mobile main and check the CI run
> delete the spike branch and /sess-wrap

## What happened

1. **cats:** `spike/wire-leaf` was a clean fast-forward over `main`
   (`951a8d9`, which had not moved). `git merge --ff-only`, then build,
   vet, `go test ./...` and `gofmt -l cmd internal wire`: all clean. Pushed
   `origin main` to `5add396`, four commits: the wire carve-out, the rweb
   bump, the tidy-exit countdown, the catgen-dart removal.
2. **cats-mobile:** re-pinned with the README recipe,
   `go get github.com/rohanthewiz/cats@5add396 && go mod tidy`, which the
   proxy resolved to `v0.2.3-0.20260903032341-5add3964d3d1` (a pseudo-
   version off the v0.2.2 tag now that the commit is on main). Vet, the
   race suite and the WASM build green: the wire changes that rode along
   (`pane.keep`, exit-countdown fields) added no down type without an arm.
   Committed as `4f8b7a2`, pushed.
3. **First CI run** (`33711557208`): both jobs green on the first try.
   Go and WASM 2m16s; Android AAR 2m10s, producing a 28 MB `cats.aar`.
   The NDK fallback and the pinned-gobind trick both worked on the hosted
   image unchanged from grmob's workflow.
4. **Spike branch deleted** locally and on origin. `git branch -d`
   refused because the branch was one commit ahead of its own upstream
   ref, not because it was unmerged; `merge-base --is-ancestor` confirmed
   it was in main, then `-D`.

## Gotchas

- The pin had to wait for the push: `go get cats@<sha>` goes through the
  proxy, which cannot see a local-only commit. Order is push cats, then
  bump cats-mobile.
- `git branch -d`'s "not yet merged to refs/remotes/origin/<branch>"
  warning is about the branch's *own* upstream, and fires after a local
  commit that was never pushed to it. Check `merge-base --is-ancestor`
  against main and use `-D`.
- `git merge-base` has no `--short`; pipe through `git rev-parse --short`.

## State at the end

- cats main `5add396`, pushed; no spike branch anywhere.
- cats-mobile main `4f8b7a2` plus this doc, pushed; CI green.
- Plan §13 says the loop is closed. Remaining from the plan: `wire.Marshal`
  does not stamp `"t"` (§8, §9), and the grmob-side items in §12.
- The emulator, cathost, catway on :8421 and the fake `claude` from the
  earlier sessions may still be running; kill by pid if so, never `killall`.
