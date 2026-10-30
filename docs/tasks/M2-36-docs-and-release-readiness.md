# M2-36 Documentation and release readiness for v0.1-alpha
Owner: mivo-backend
Depends on: all other M2 tasks
Files/areas: `docs/development.md`, `README.md`, `CLAUDE.md` ("Commands" section only, needs the
user's approval like M1-22), `docs/tasks/BACKLOG.md`, `docs/reports/M2-release-readiness.md`

## Goal
A new contributor and a NAS user can set up libraries, FFmpeg and playback from the docs, and the
milestone ends with one recorded run of every gate on a clean checkout, ready for the release
decision (OD-9).

## Acceptance criteria
- [ ] `docs/development.md`: libraries and tools configuration (keys, env, examples for PowerShell
      and bash), test corpus incl. long clips and `-only` lists, the corpus integration test and
      `-short`, the playback smoke script, E2E browser projects (bundled Chromium and branded
      channel), the run-time impact of FFmpeg tests.
- [ ] BACKLOG rows "docs coverage without cgo" and "docs nits" are done.
- [ ] `README.md`: what v0.1-alpha can do (scan, browse, play every container of the corpus,
      Direct Play / Remux / Transcode with overlay), what it cannot yet (login, subtitles, HW
      acceleration, metadata), the LAN-only warning (no authentication until milestone 7).
- [ ] `CLAUDE.md` commands: smoke script and corpus generation lines (after approval).
- [ ] `docs/tasks/BACKLOG.md` lists every non-blocking finding from M2 reviews not yet done.
- [ ] `docs/reports/M2-release-readiness.md`: on a clean clone, the full gate list of
      `docs/development.md` ("Full gates before hand-in"), `go run ./scripts/gen-testmedia`,
      the corpus integration test, `pnpm -C web e2e` (both projects) and the Docker smoke run,
      each with command and result; list of open decisions still unanswered.
- [ ] Every relative link in the changed Markdown files resolves (tester checks with a grep of
      `](` targets against the tree).
- [ ] Gates: docs-only change, so the Go and web gates must still pass unchanged
      (`golangci-lint run`, `go test ./...`, `pnpm -C web lint`).

## Notes
- The release itself (version tag, image push) is done by `mivo-release` only after PASS +
  APPROVE and the user's explicit approval (OD-9).
- Keep the product name out of prose where the brand rule applies (ADR 0002 rename checklist).

## Orchestrator note (from M2-06)
- `docs/development.md` "Test media corpus": `-only` now takes a comma-separated glob list
  (e.g. `'video-mp4-*,video-webm-vp9-opus'`), and kept files are labelled `kept` in progress and
  summary. Document both.
- (from M2-12) User docs: absolute symlinks and Windows junctions inside a library are skipped
  (only relative links that stay inside the root are followed); use relative links or add the
  target as its own library.
- (from M2-13) User docs: keep the data directory (`/config` in Docker) on local storage; file
  locks on SMB/NFS shares may not be honoured.
- (from M2-08) ADR 0024 "Notes (M2)": requests with a body get a 30 s read deadline
  (`restapi.RequestBodyReadTimeout`); bodiless requests (stream GETs) never do.
