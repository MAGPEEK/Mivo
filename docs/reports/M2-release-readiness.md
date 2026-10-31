# Milestone 2 release readiness (v0.1-alpha, planned 0.1.0-alpha.2)

Task: M2-36 | Date: 2026-10-31 | Run by: mivo-backend

This report records one run of every gate on a clean checkout of the release candidate. It is
the input for the release decision (OD-9). Nothing was tagged, pushed or published.

## Release candidate

| Item | Value |
|---|---|
| Commit | `17e4a211da43f3b6bb485399a0eea2ce0aa3e965` (`fix(ffmpeg): log ffprobe failures in the adapter, refuse device paths`), the HEAD of `main` with every M2 task (M2-01 to M2-41) committed |
| Checkout | a fresh git worktree of that commit. At the start `git status` was clean and the tree held no git-ignored artefacts (no `web/node_modules`, `testdata/media`, `internal/adapters/http/webui/dist` or `bin`). Every artefact was created by the documented commands below, none was copied |
| Code under test | unchanged during the run. The M2-36 changes are Markdown and comments only (`README.md`, `docs/`, comments in `.testcoverage.yml` and `tools/go.mod`); the Docker build context (`Dockerfile.dockerignore`) contains none of them |
| Machine | Windows 11 Pro (10.0.26300), Intel Core Ultra 7 265K (20 threads), PowerShell 7 and Git Bash |
| Tools | Go 1.27.3, golangci-lint 2.15.0, Node.js 24.22.1, pnpm 10.34.5, FFmpeg 9.0.2 (gyan.dev full build), LLVM-MinGW clang 23.1.4 (cgo for `-race`), Docker Desktop 4.97.0 (Engine 29.9.2), Microsoft Edge 156.0.4314.50, Playwright Chromium headless shell 1248 |

`go test` ran with `-count=1` in addition to the documented flags, so no result came from the
test cache. Times are wall clock.

## 1. Full gates before hand-in (`docs/development.md`)

The corpus was not generated yet for this block, so the FFmpeg corpus tests skipped here. The
same gates ran again with the corpus in block 2.

| # | Command | Result | Time |
|---|---|---|---|
| 1.1 | `pnpm -C web install` | done, lockfile unchanged | 8 s |
| 1.2 | `go vet ./...` | pass | 5 s |
| 1.3 | `go test -count=1 ./...` | pass, 37 packages ok, 5 without tests | 15 s |
| 1.4 | `go test -count=1 -race "-coverprofile=coverage.out" ./...` | pass, 37 packages ok | 106 s |
| 1.5 | `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` | pass, total 96.1 % (threshold 70 %), every `internal/domain` package ≥ 90 % | 2 s |
| 1.6 | `golangci-lint run` | pass, 0 issues (incl. depguard layer rules) | 6 s |
| 1.7 | `golangci-lint fmt --diff` | pass, no diff | 33 s |
| 1.8 | `go tool "-modfile=tools/go.mod" govulncheck ./...` | pass, "No vulnerabilities found." | 3 s |
| 1.9 | `go mod tidy -diff` | pass, no diff | 0 s |
| 1.10 | `go -C tools mod tidy -diff` | pass, no diff | 1 s |
| 1.11 | `pnpm -C web lint` | pass (ESLint, dependency-cruiser, Prettier, tsc) | 36 s |
| 1.12 | `pnpm -C web test` | pass, 1220 tests, coverage 99.94 % statements / 99.89 % branches (ADR 0019 thresholds met) | 15 s |
| 1.13 | `pnpm -C web build` | pass (hls.js chunk 574 kB, size warning only) | 1 s |
| 1.14 | `go vet -tags webui ./...` | pass | 2 s |
| 1.15 | `golangci-lint run --build-tags webui` | pass, 0 issues | 7 s |
| 1.16 | `go test -count=1 -race -tags webui ./internal/adapters/http/... ./cmd/...` | pass | 7 s |
| 1.17 | `go build -tags webui -o bin/mivo.exe ./cmd/mivo` | pass | 1 s |
| 1.18 | `pnpm -C web e2e` | pass, see block 3 | 20 s |
| 1.19 | drift check: `go generate ./...`, `pnpm -C web gen`, `git diff --exit-code -- internal/adapters/http/apigen web/packages/api-client/src/generated` | pass, no diff | 4 s |
| 1.20 | `docker run --rm -v "<checkout>:/src" -w /src sqlc/sqlc:1.31.1 diff -f internal/adapters/sqlite/sqlc.yaml` | pass, no diff | 1 s |
| 1.21 | `docker build …` | pass, see block 4 | 50 s |

The drift check and the sqlc diff (step 5 of the list) ran although no spec, migration or query
changed, as a release check.

## 2. Test corpus and corpus integration test

| # | Command | Result | Time |
|---|---|---|---|
| 2.1 | `go run ./scripts/gen-testmedia` | pass: 45 generated, 0 skipped, 0 failed, 9 known gaps (ADR 0021: APE, DSD DFF/DSF, HEIC, JPEG EXIF, PGS, DVB, VobSub, RMVB RV40/Cook); FFmpeg 9.0.2 | 2 s |
| 2.2 | `go run ./scripts/gen-testmedia` (second run) | pass: 0 generated, 45 kept | 1 s |
| 2.3 | `go test -count=1 -v ./internal/adapters/corpustest/` | pass: `TestCorpusVideosPlayWithTheExpectedMode` with 58 full-capability subtests (every video, subtitle and multitrack case × `chromium-desktop` and `firefox-desktop`, incl. both seek clips started at 21 s), plus the `no_encoders` and `hls_without_codecs` groups; no skip | 4 s |
| 2.4 | `go test -count=1 ./...` (with corpus) | pass, 37 packages ok | 17 s |
| 2.5 | `go test -count=1 -short ./...` (without / with corpus) | pass / pass | 13 s / 13 s |
| 2.6 | `go test -count=1 -race "-coverprofile=coverage.out" ./...` (with corpus) | pass, 37 packages ok | 107 s |
| 2.7 | `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` (with corpus) | pass, total 96.2 % | 2 s |

## 3. E2E: `pnpm -C web e2e` (both browser projects)

Result: **31 passed, 0 failed, 0 skipped** in 17.2 s (run 20 s incl. the web build). The
global setup built the server with `-tags webui`, generated its six clips with
`gen-testmedia -only …` and started the server on a free loopback port. No session directory
was left at teardown.

| Project | Browser | Tests |
|---|---|---|
| `library` | Playwright headless Chromium shell | 2 passed (scan from the UI, axe) |
| `chromium` | Playwright headless Chromium shell | 24 passed (shell, layout, accessibility, player against mocked endpoints, WebM direct play, no-H.264 message) |
| `branded` | installed Microsoft Edge 156 (`msedge` channel) | 5 passed: MP4 by Direct Play, FLV by Direct Stream (remux), AVI by Transcode, MKV directly or by remux, remux restart at 50 % after a seek |

## 4. Docker image and playback smoke test

All commands from the repository root. The image was built from scratch (`--no-cache`) under a
tag of its own, so no existing local image was touched.

| # | Command | Result | Time |
|---|---|---|---|
| 4.1 | `docker build --no-cache -f packaging/docker/Dockerfile --build-arg VERSION=0.1.0-alpha.2-rc.m2-36 --build-arg REVISION=17e4a211da43f3b6bb485399a0eea2ce0aa3e965 -t mivo:m2-36-rc .` | pass; linux/amd64, 773 MB, user `1000:1000`, OCI labels version and revision as passed | 50 s |
| 4.2 | `docker run --rm mivo:m2-36-rc --version` | `Mivo 0.1.0-alpha.2-rc.m2-36 (go1.27.3)` | 1 s |
| 4.3 | `docker run -d --name mivo-m2-36-smoke -p 127.0.0.1:8460:8460 -v "<checkout>/testdata/media:/media:ro" mivo:m2-36-rc` | started; Docker health `healthy` after 3 s | – |
| 4.4 | `curl -sS http://127.0.0.1:8460/api/v1/health` | `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}` | 0 s |
| 4.5 | `docker exec mivo-m2-36-smoke ffmpeg -hide_banner -version` | `ffmpeg version 7.1.5-0+deb13u1`; server log: `ffmpeg ready for direct play, remux and transcode` (libx264, aac, HLS muxing) | 1 s |
| 4.6 | `go run ./scripts/playback-smoke -base http://127.0.0.1:8460` | pass: **30 passed, 0 failed, 0 not run** of 30 cases (profile `chromium-desktop`, 28 files plus 2 seek cases); remux seek starts at 20.021 s, transcode seek at 21.000 s | 8 s |
| 4.7 | `go run ./scripts/playback-smoke -base http://127.0.0.1:8460 -profile firefox-desktop` | pass: **28 passed, 0 failed, 0 not run** of 28 cases | 5 s |
| 4.8 | `docker exec mivo-m2-36-smoke ls -A /transcode/sessions` | empty: every session was removed | 0 s |
| 4.9 | `docker rm -f -v mivo-m2-36-smoke`, `docker rmi mivo:m2-36-rc` | removed | – |

The container log has no `WARN` or `ERROR` line; the startup scan catalogued 28 files with
`failed=0 skipped=0`. FFmpeg 7.1.5 of the image marks its native `opus`, `vorbis`, `truehd`
and `dca` encoders experimental, which the capability detection ignores as intended (ADR 0023
Notes M2-16).

## 5. Synology package

**Not ready – attached later (OD-SPK-1 A).** SPK wave 2 is still in test, SPK-11 (emulated smoke
per family) and SPK-12 (the owner's DSM install test) have not started, so no SPK was built from
this release candidate. The Docker release does not wait for it; SPK-13 attaches the SPKs to
the GitHub Release of `v0.1.0-alpha.2` once SPK-11/12 pass with SPKs built from the release
commit.

## 6. Documentation and backlog (M2-36)

- `docs/development.md` covers libraries and tools configuration, the corpus (seek clips,
  `-only` lists, `kept`), the corpus integration test and `-short`, the playback smoke test,
  the E2E browser projects, the run time of the FFmpeg tests and the release gates. The
  BACKLOG rows "docs coverage without cgo" and "docs nits" are done; the CLAUDE.md Commands lines were added (approval of 2026-10-30, M2-00).
- `README.md` states what v0.1-alpha can and cannot do and warns that there is no
  authentication until milestone 7.
- ADR notes handed over to M2-36 were recorded: ADR 0022 Notes (M2-15), ADR 0023 Notes
  (M2-16), ADR 0024 Notes (M2-08), (M2-17), (M2-20 review), (M2-21).
- `docs/tasks/BACKLOG.md`: no row is due "before 0.1.0-alpha.2". One row is overdue against
  its own date: http "5xx branches log `slog.Any("error", err)` uncut … shared limit
  `logtext.MaxServerTextBytes`" (M2-08 review r2), due "before M2-28/M2-29 add client-derived
  error text"; M2-28/M2-29 are done and the limit does not exist. The bodies are still bounded
  by the 64 KiB request limit; re-dated to "before M7b" by the orchestrator, not a blocker for an alpha.
- Every relative link in the changed Markdown files resolves.

## 7. Open decisions

1. **Release decision OD-9.** OD-9 A was chosen on 2026-10-30 (version `0.1.0-alpha.2`, image
   tags `0.1.0-alpha.2` and `alpha`, no `latest`, linux/amd64 and linux/arm64, no armv7, git tag
   `v0.1.0-alpha.2`, GitHub Release). Open: the go for this release candidate after test and
   review of M2-36.
2. **Docker push approval.** `mivo-release` may push to `docker.io/magpeek/mivo-media-server`
   only with the owner's explicit approval for that push (repository, tags, platforms).
3. **CLAUDE.md Commands change**: done; the owner approved this M2-36 edit on 2026-10-30
   (M2-00 overview), so the playback smoke, corpus integration test and tagged race lines
   are in CLAUDE.md.
4. **Product name.** ADR 0002 says the final name is decided "before the first public release";
   0.1.0-alpha.1 is already public under the working title. Decide whether alpha.2 also ships
   under it.
5. **Overdue BACKLOG row** (section 6): re-dated to "before M7b"; no decision left.
6. **Synology package**: none for this release (OD-SPK-1 A); the SPK decisions still pending
   are tracked in `docs/tasks/SPK-00-overview.md`.

## Cleanup

The container and the image `mivo:m2-36-rc` were removed; no image was pushed and no registry
login happened. `sqlc/sqlc:1.31.1` and `golang:1.27.3` were present before and stay. The BuildKit
cache of the build stays on the machine. The git-ignored artefacts (`web/node_modules`,
`testdata/media`, `internal/adapters/http/webui/dist`, `bin/mivo.exe`, `coverage.out`) stay in
the worktree.
