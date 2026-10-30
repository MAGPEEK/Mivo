# M1-20 test report: Dockerfile (multi-stage, non-root, FFmpeg)

Result: **PASS** (round 2; round 1 results stay below, round 2 section at the end)

Docker build ran from a clean tree: `git archive HEAD` (924b018) plus the uncommitted M1-20 files
(`packaging/docker/*`, `healthprobe/*`, `cmd/mivo/{healthcheck.go,healthcheck_test.go,main.go}`).
The uncommitted M1-17 web changes were not in the build context (verified with `diff -rq`).
Build ran with `--no-cache`, `--build-arg VERSION=0.1.0-test REVISION=924b018`.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./...` | pass | exit 0, no output |
| `go test -race ./cmd/... ./internal/adapters/http/...` | pass | `ok cmd/mivo 2.949s`, `ok .../healthprobe 1.689s`, all other packages ok |
| `go test -race -coverprofile ./...` | pass | all packages ok |
| `golangci-lint run` | pass | `0 issues.` (v2.15.0) |
| Coverage total >= 70 % | pass | 90.7 % |
| Coverage `internal/domain/...` >= 90 % | pass | `domain/system` 100.0 %, `domain` has no statements |
| Layer rule (domain deps) | pass | `go list -deps ./internal/domain/...` has no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |
| `docker buildx build --check` | pass | `Check complete, no warnings found.` |
| `docker build` (clean tree, `--no-cache`) | pass | exit 0, 47 s, build context 14.5 kB; `vite build` ran in the web stage |
| web (`pnpm lint`/`test`/`e2e`) | skipped | not part of M1-20 |

New-code coverage: `healthprobe` 96.0 % (`Check`, `Address`, `isAllInterfaces` 100 %; `newClient` 50 %
only because the redirect closure is not counted as executed, the behaviour itself is tested by
"redirect is not followed"). `cmd/mivo/healthcheck.go` 100 % for all three functions; `cmd/mivo` 88.9 %.

## Acceptance criteria

- [x] Build from clean checkout on Docker Desktop, linux/amd64: exit 0. `docker inspect` shows `linux/amd64`.
- [x] Stages: `node:24-slim` with corepack and `pnpm fetch` + `pnpm install --frozen-lockfile --offline` +
  `pnpm build` (pnpm v10.34.5, vite 8.3.5); `golang:1.27.3` (matches go.mod toolchain 1.27.3 per
  `go version`) with `CGO_ENABLED=0 -tags webui -trimpath -buildvcs=false -ldflags -X ...version=${VERSION}`,
  default `0.0.0-dev`; runtime `debian:stable-slim` with `ca-certificates ffmpeg tzdata`,
  `--no-install-recommends`, `rm -rf /var/lib/apt/lists/*`. Verified in the Dockerfile and in the build log.
  `/usr/share/zoneinfo/Europe/Berlin` and `/etc/ssl/certs/ca-certificates.crt` exist in the image.
- [x] Builder stages use `FROM --platform=$BUILDPLATFORM`; `GOOS/GOARCH` from `TARGETOS/TARGETARCH`, `GOARM`
  derived from `TARGETVARIANT` for `arm` (Dockerfile:24, 47, 66-72). Only linux/amd64 was built (as required).
- [x] Runtime config (`docker inspect`):
  - `User` = `1000:1000`
  - `Volumes` = `/config`, `/media`, `/transcode`
  - `ExposedPorts` = `8460/tcp`
  - `Env` = `MIVO_CONFIG_DIR=/config`, `MIVO_PATHS_DATA=/config/data`, `MIVO_PATHS_CACHE=/config/cache`, `MIVO_PATHS_TRANSCODE=/transcode`
  - Labels: `source=https://github.com/MAGPEEK/Mivo`, `licenses=GPL-3.0-or-later`, `version=0.1.0-test`,
    `revision=924b018` (plus title, description, url)
  - Directories: `/config`, `/transcode` are `1000:1000` mode 0750; `/media` is `root` 0755; binary 13.4 MB.
- [x] `docker inspect -f '{{.Config.User}}'` -> `1000:1000`. `ffmpeg -version` -> `ffmpeg version 7.1.5-0+deb13u1`, exit 0;
  `ffprobe -version` -> `ffprobe version 7.1.5-0+deb13u1`, exit 0. `docker run --rm <img> --version` ->
  `Mivo 0.1.0-test (go1.27.3)`, exit 0.
- [x] Run with empty bind-mounted `/config` on free host port 57288:
  - healthy after 2.5 s (limit 30 s)
  - `GET /api/v1/health` -> 200 `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`
  - `GET /` -> 200 `text/html`, the built `index.html` (hashed `/assets/index-*.js` and `.css`, `<title>Mivo</title>`), not the placeholder;
    `GET /assets/index-Cmd4vm6N.js` -> 200 `text/javascript`, 394 242 bytes
  - `GET /api/v1/system/info` -> `"version":"0.1.0-test"`
  - `/config` contains `data/server.db` (+ wal/shm) and `cache/`
  - Restart with the same volume: healthy again, `serverId` unchanged (`a7c1aa67-2920-49a9-9282-b2898bedf91e`).
- [x] `HEALTHCHECK`: `CMD ["/usr/local/bin/mivo","healthcheck"]`, interval 30 s, timeout 5 s, start-period 60 s,
  start-interval 2 s, retries 3. Probe is 3 s (`DefaultTimeout`) at `127.0.0.1:<configured port>/api/v1/health`.
  Exit 0 on a healthy server; with no server: `connection refused`, exit 1. `MIVO_SERVER_PORT=9999`: probe
  follows the port (container healthy on 9999; probe against a server-less container reports 9999, exit 1).
  Unit tests (`httptest`): 200, 503, 204, redirect, timeout (`TestCheckTimesOut`), connection refused
  (`TestCheckReportsConnectionRefused`), invalid address, cancelled context, `Address` table.
- [x] `Dockerfile.dockerignore` excludes `.git`, `**/node_modules`, `testdata/media`, `bin`,
  `internal/adapters/http/webui/dist`, coverage and Playwright output (`playwright-report`, `test-results`,
  `blob-report`); additionally an allow list (`*` then `!go.mod`, `!go.sum`, `!cmd/`, `!internal/`, `!web/`).
  The context was 14.5 kB.
- [x] Final image size: **768 MB** on disk (`docker images`), 207 MB content size (compressed layer content).
- [x] Gates `golangci-lint run`, `go test`, coverage check, Docker build: all pass (see above).

## Extra checks (task notes and request)

- Wrong-owner `/config` (non-empty named volume, owned by root 0755, container user 1000):
  `mivo: prepare directories: create directory /config/data: mkdir /config/data: permission denied`, exit 1, no panic.
- `--user 2000:2000` against a `1000:1000` 0750 non-empty volume:
  `mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied`, exit 1.
  This matches the message quoted in `packaging/docker/README.md`.
- Test-method note: an empty named volume is re-chowned by Docker to the image's directory owner on first
  use, so the wrong-owner case needs a non-empty volume (a first attempt with an empty volume started normally).
  Default user with a fresh named volume works without setup (matches the Dockerfile comment and the README).
- `docker stop` (graceful): 0.39 s, exit code 0, `OOMKilled=false`; logs end with
  `msg="http server shutting down"` and `msg="Mivo stopped"`. Well inside the 10 s default stop timeout (no SIGKILL).
- Restart: 0.5 s.

## Observations (no action required)

- Server logs `address=[::]:8460` and the probe uses `127.0.0.1`: works because the listener is dual-stack. If
  `MIVO_SERVER_HOST` were set to an IPv6-only address the probe would target that host as configured (covered by `TestAddress`).
- The HEALTHCHECK `start-interval` needs Docker Engine 25+; Docker 29.9.2 was used here. Older engines ignore it (documented in the Dockerfile).
- Base images are tag-pinned only (digest pinning deferred to CI per task notes).
- Not tested: multi-arch builds (arm64, arm/v7); the task limits M1 to linux/amd64.

## Cleanup

Removed: all `mivo-m120-*` containers and volumes, scratch source tree, temp config dir, temp coverage file.
Kept: image `mivo:m1-20-test` (768 MB) for possible reuse in M1-23. Nothing staged, nothing pushed.

## History

- Round 1: PASS.
- Round 2: PASS (see section below).

# Round 2

Changes verified: `Dockerfile` (syntax line, `--prefer-offline`, `ARG DEBIAN_FRONTEND`, brand-free cache ids,
tini/`--init` comment), `Dockerfile.dockerignore` (`**/.env*`, `**/*.local`), `README.md`, `healthprobe_test.go`
(redirect with `Location`), `.gitignore` (`.env`, `.env.*`, `*.local`).

Docker ran from a clean scratch tree: `git archive HEAD` (924b018) plus only the uncommitted M1-20 files
(`packaging/docker/*`, `healthprobe/*`, `cmd/mivo/{healthcheck.go,healthcheck_test.go,main.go}`, `.gitignore`).
`diff -rq` against the working tree showed the only differences were the M1-17 web/docs files (absent from the
scratch tree) and my dummy secret files. Docker Engine 29.9.2.

## Round-2 gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./...` | pass | exit 0, no output |
| `go test -race ./cmd/... ./internal/adapters/http/...` | pass | `ok cmd/mivo`, `ok .../healthprobe 1.959s`, all other http packages ok |
| `go test -race -count=1 -cover` (new code) | pass | `healthprobe` 100.0 % (round 1: 96.0 %), `cmd/mivo` 88.9 % |
| `golangci-lint run` | pass | `0 issues.` |
| Layer rule (`go list -deps ./internal/domain/...`) | pass | no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |
| `docker buildx build --check` | pass | `Check complete, no warnings found.`; frontend `docker/dockerfile:1` resolved from the syntax line |
| `docker build --no-cache` (VERSION=0.1.0-r2test, REVISION=924b018) | pass | exit 0, 47.3 s, context 14.52 kB, `vite v8.3.5` ran, no pnpm errors |
| web gates | skipped | not part of M1-20 |

Total coverage was not re-measured in round 2 (only test files and non-Go files changed besides the
probe test; round 1: 90.7 % total, `domain/system` 100 %).

## Round-2 checks

- Redirect test: `TestCheckDoesNotFollowRedirect` (`healthprobe_test.go:89`) serves `http.Redirect(..., 302)` with a
  `Location` to a path answering 200. It asserts `ErrUnhealthy` and the message contains `status 302`; passes with `-race`.
- `DEBIAN_FRONTEND` not in the image: `docker image inspect` Env is only `PATH`, `MIVO_CONFIG_DIR`, `MIVO_PATHS_DATA`,
  `MIVO_PATHS_CACHE`, `MIVO_PATHS_TRANSCODE`. In the running container `docker inspect` Env and `docker exec env` have no
  `DEBIAN*` variable. The apt layer still ran non-interactively (148 packages installed, no prompt).
- Secret files not in the build context: scratch tree contained `web/.env`, `web/apps/web/.env.local` and
  `web/foo.local`. A throwaway `ctxprobe.Dockerfile` (own `ctxprobe.Dockerfile.dockerignore`, a copy of the real one; the
  real Dockerfile was not modified) ran `COPY . /ctx` and `find /ctx -name '.env*' -o -name '*.local'`:
  - real ignore file: no match (`env-like files:` empty; `web/` listing has no `.env`, `.local`)
  - control with the two rules removed: all three files appear (`/ctx/web/.env`, `/ctx/web/apps/web/.env.local`, `/ctx/web/foo.local`)

  This proves the rules are what keeps the files out. The probe files were deleted before the real build; the real build context
  was 14.52 kB.
- Run (empty bind-mounted `/config`, host port 57391): `healthy` after ~2 s, `GET /api/v1/health` -> 200
  `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`, `GET /` -> 200 `text/html; charset=utf-8` with the built
  `index.html` (`/assets/index-Cmd4vm6N.js`, `<title>Mivo</title>`), `--version` -> `Mivo 0.1.0-r2test (go1.27.3)`.
  Config: `User` `1000:1000`; labels `version=0.1.0-r2test`, `revision=924b018`, `licenses=GPL-3.0-or-later`, `source` as in round 1.
- `docker stop -t 20`: 0.37 s, `ExitCode=0`, `OOMKilled=false`; logs end with `http server shutting down`, `Mivo stopped`.
- Cache-mount prune + rebuild: `docker builder prune --filter type=exec.cachemount -f` reclaimed 821 MB (three mounts: pnpm
  store, Go module cache, Go build cache); afterwards only a 2 MB mount remained. After appending a line to
  `web/vitest.config.ts` in the scratch tree, the `COPY web/` layer and the install layer re-ran: `pnpm install --frozen-lockfile
  --prefer-offline` -> `Lockfile is up to date`, `Already up to date`, `Done in 550ms`, `vite build` ok, exit 0, 4.0 s; the
  Go and runtime layers were cached. With `--prefer-offline` the install no longer depends on the pruned store.
  Note: the install found the packages already in `node_modules` from the `pnpm fetch` layer, so this run did not exercise a
  download of missing tarballs. The fallback is the documented behaviour of `--prefer-offline`; the old `--offline` flag is
  what failed in that situation.
- README: reviewed against the Dockerfile and behaviour. The `permission denied` message quoted for the wrong user was
  reproduced in round 1; `stop-timeout 20` matches the measured shutdown; `--init` and tini comments are consistent with the
  Dockerfile (server is PID 1 and handles SIGTERM; verified by the 0.37 s clean stop).

## Round-2 cleanup

Removed: container `mivo-r2test`, tag `mivo:m1-20-r2test`, scratch tree, config dir, build logs. Kept: `mivo:m1-20-test` and
`mivo:m1-20-r2` (and the unrelated `mivo:dev`). Nothing staged or pushed. The cache mounts were pruned as requested (build
cache is rebuilt on the next build).

REPORT
status: PASS
task: M1-20
files: docs/reports/M1-20-test.md
tests: go vet ./... ok; go test -race ./cmd/... ./internal/adapters/http/... ok (healthprobe 100 %, cmd/mivo 88.9 %); golangci-lint run 0 issues; docker buildx build --check no warnings; docker build --no-cache from clean scratch tree ok; DEBIAN_FRONTEND absent in running container; .env/.local files excluded from context (with control); healthy in ~2 s, /api/v1/health 200, / SPA, docker stop -t 20 exit 0 in 0.37 s; cache-mount prune + invalidated install layer rebuild ok
findings: none blocking. The rebuild after prune did not need to download tarballs because node_modules from the fetch layer was reused, so the pruned-store fallback is not exercised end to end.
next: send to mivo-reviewer

