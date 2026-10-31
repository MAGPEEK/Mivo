# Development guide

This guide covers setting up, building, testing, linting, running and packaging the server
and the web app. Every command was run on Windows 11 in PowerShell 7 from the repository
root. Git Bash and Linux differences are noted where they matter. The short command list
for agents lives in the "Commands" section of `CLAUDE.md`. Architecture decisions are in
`docs/adr/`.

## Prerequisites

| Tool | Version | Needed for | Windows install (winget id) |
|---|---|---|---|
| Go | 1.27 or newer. `go.mod` pins `toolchain go1.27.3`, and the default `GOTOOLCHAIN=auto` downloads it | everything in Go | `GoLang.Go` |
| Node.js | 24 LTS (`engines.node: >=24`) | web app, ESLint (see note) | `OpenJS.NodeJS.LTS` |
| pnpm | 10 (`packageManager: pnpm@10.34.5`) | web workspace | `npm install -g pnpm@10` |
| golangci-lint | v2.15.0 | Go lint and format | `GolangCI.golangci-lint` |
| Git | tested with 2.56, with Git Bash on Windows | hooks run in Git's `sh` | `Git.Git` |
| FFmpeg | tested with 9.0.2 (gyan.dev full build) | test media corpus, later media features | `Gyan.FFmpeg` |
| Docker Desktop | tested with 4.97 (Engine 29.9) | image build, sqlc, race tests without a C toolchain | `Docker.DockerDesktop` |
| C toolchain | LLVM-MinGW (UCRT), tested 23.1.4-20261020 | `go test -race` on Windows only | `MartinStorsjo.LLVM-MinGW.UCRT` |

Install with `winget install --exact --id <id>` and open a new terminal afterwards so that
`PATH` is updated. The pinned Go tools (go-test-coverage, govulncheck, oapi-codegen,
lefthook) need no install: `go tool` builds them from `tools/go.mod` (ADR 0019). sqlc runs
from Docker or a release binary (see "Code generation").

ESLint needs Node 24 because `web/eslint.config.js` imports the TypeScript brand module
directly. Node 24 strips the types natively, so the brand-name guard follows a rename.

## First setup

Clone the repository, then run from its root:

```
pnpm -C web install
pnpm -C web/apps/web exec playwright install --only-shell chromium
go tool "-modfile=tools/go.mod" lefthook install
go test ./...
```

- The Playwright step downloads the headless Chromium shell (network access) into
  `%LOCALAPPDATA%\ms-playwright` (`~/.cache/ms-playwright` on Linux). Playwright is a
  dependency of `web/apps/web`, so `pnpm -C web exec playwright` does not find it.
- The lefthook step writes `.git/hooks/pre-commit` and `.git/hooks/commit-msg`
  (see "Git hooks").

## Shells: PowerShell, Git Bash and Linux

- **Quote arguments with a dot.** PowerShell splits an unquoted argument such as
  `-modfile=tools/go.mod` or `-coverprofile=coverage.out` at the dot, so
  `go tool -modfile=tools/go.mod …` fails with `file does not have .mod extension`, and
  `go test -coverprofile=coverage.out ./...` fails with `no required module provides package
  .out` and leaves a stray `coverage` file. The quoted forms in this guide also work in bash.
- **Environment variables**: PowerShell uses `$env:NAME = 'value'` (it stays set for the
  session, remove it with `Remove-Item Env:NAME`). bash uses `NAME=value command`.
- **Docker bind mounts from Git Bash**: MSYS rewrites `/src` and the mounted path. Prefix the
  command with `MSYS_NO_PATHCONV=1`, for example
  `MSYS_NO_PATHCONV=1 docker run --rm -v "${PWD}:/src" -w /src …`. PowerShell and Linux need
  nothing extra.
- **Binary name**: `bin/mivo.exe` on Windows, `bin/mivo` on Linux.
- **Line endings**: `.gitattributes` sets `* text=auto eol=lf`, so the working tree uses LF
  on Windows too (only `*.ps1` files use CRLF). Keep your editor on LF:
  `pnpm -C web lint` (Prettier) fails on CRLF files and `golangci-lint fmt --diff` shows
  them as a diff.

## Go gates

```
go vet ./...
go test ./...
go test -race "-coverprofile=coverage.out" ./...
go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"
golangci-lint run
golangci-lint fmt --diff
go tool "-modfile=tools/go.mod" govulncheck ./...
go mod tidy -diff
go -C tools mod tidy -diff
```

- The coverage gate reads `coverage.out` from the test run before it: total ≥ 70 %, every
  package below `internal/domain` ≥ 90 %. Generated packages (`apigen`, `sqlcgen`) and
  `internal/archtest` are excluded. Thresholds are never lowered.
- `golangci-lint run` includes the depguard layer rules. `internal/archtest` checks the same
  layer matrix, so `go test ./...` catches layer violations as well.
- `golangci-lint fmt` formats in place (gofumpt and goimports with the module as local
  prefix). `--diff` only reports.
- govulncheck needs network access to the Go vulnerability database.
- Maintain `tools/go.mod` only with `go -C tools …` (for example
  `go -C tools get -tool <path>@<version>`). Never run `go mod tidy -modfile=tools/go.mod`
  from the root: tidy would pull the production dependencies into the tool module.

### Gates with the embedded web app

Plain `go build`, `go test` and `golangci-lint run` never see `files_embedded.go`, which is
compiled only with the `webui` build tag (ADR 0020). After the web build, also run:

```
pnpm -C web build
go vet -tags webui ./...
golangci-lint run --build-tags webui
go test -race -tags webui ./internal/adapters/http/... ./cmd/...
go build -tags webui -o bin/mivo.exe ./cmd/mivo
```

Without a prior `pnpm -C web build` the tagged build fails on purpose with
`pattern all:dist: no matching files found`.

### Race detector on Windows

`go test -race` needs cgo and a C compiler. Without one, Go reports
`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1` or
`cgo: C compiler "gcc" not found`. Two options:

1. Install LLVM-MinGW (`winget install --exact --id MartinStorsjo.LLVM-MinGW.UCRT`) and open
   a new terminal. It puts `gcc` (clang) on `PATH`, and Go then enables cgo by default
   (`go env CGO_ENABLED` prints `1`).
2. Run the race tests in Docker (Linux, about 40 s on the dev machine; the named volume
   caches the module downloads between runs):

   ```
   docker run --rm -v "${PWD}:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.27.3 go test -race ./...
   ```

   Remove the cache with `docker volume rm gomodcache`. The image tag matches the toolchain
   in `go.mod` and `GO_IMAGE` in `packaging/docker/Dockerfile`.

The release binary itself is built with `CGO_ENABLED=0`; cgo is only needed for the race
detector.

## Code generation

Generated code is committed with a `DO NOT EDIT` header and never edited by hand.

| Output | Source | Command |
|---|---|---|
| `internal/adapters/http/apigen` | `api/openapi.yaml` | `go generate ./...` (oapi-codegen from `tools/go.mod`) |
| `web/packages/api-client/src/generated` | `api/openapi.yaml` | `pnpm -C web gen` (openapi-typescript) |
| `internal/adapters/sqlite/sqlcgen` | `internal/adapters/sqlite/{migrations,queries}` | sqlc v1.31.1, see below |

**Drift check (ADR 0017).** After a change to the spec, the migrations or the queries,
regenerate and make sure the committed code is current:

```
go generate ./...
pnpm -C web gen
git diff --exit-code -- internal/adapters/http/apigen web/packages/api-client/src/generated
docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.31.1 diff -f internal/adapters/sqlite/sqlc.yaml
```

**sqlc** needs cgo to build, so it is not installed with `go install` (ADR 0004). Use one of:

- Docker (needs Docker Desktop; the first run pulls the image):

  ```
  docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.31.1 generate -f internal/adapters/sqlite/sqlc.yaml
  ```

- The v1.31.1 release binary from <https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1>,
  run as `sqlc generate -f internal/adapters/sqlite/sqlc.yaml`. This release publishes no
  `checksums.txt`; compare the SHA-256 of the archive with the digest GitHub shows for the
  asset before you unpack it:

  ```
  (Get-FileHash .\sqlc_1.31.1_windows_amd64.zip -Algorithm SHA256).Hash.ToLower()
  (Invoke-RestMethod https://api.github.com/repos/sqlc-dev/sqlc/releases/tags/v1.31.1).assets |
    Where-Object name -eq 'sqlc_1.31.1_windows_amd64.zip' | Select-Object -ExpandProperty digest
  ```

  Expected: `sqlc_1.31.1_windows_amd64.zip`
  `352711fa7dcb05dcdfefca0ad71b2c9a74fd090f8d7fc609419de4cbc725429f`,
  `sqlc_1.31.1_linux_amd64.tar.gz`
  `497ae4fcdfa64c5b0c311ffe4c2bd991e43991e82e5367792ed78bc2dca27354` (Linux:
  `sha256sum`).

Both ways produce output byte-identical to the committed `sqlcgen` files (verified
2026-10-30 with task M1-22).

## Web gates

```
pnpm -C web install
pnpm -C web lint
pnpm -C web test
pnpm -C web build
pnpm -C web e2e
```

- `lint` runs ESLint (`--max-warnings=0`), dependency-cruiser, Prettier (`--check`) and
  `tsc -b`. `pnpm -C web format` fixes formatting.
- `test` runs Vitest with v8 coverage and the thresholds of ADR 0019.
- `build` writes the production build to `internal/adapters/http/webui/dist` (git-ignored),
  where `go build -tags webui` embeds it.
- `e2e` builds the web app, then a Playwright global setup builds the server with
  `go build -tags webui` into a fresh temporary directory and starts it on `127.0.0.1` with
  a free port and an isolated configuration. It needs Go on `PATH` and the Chromium shell
  from the first setup. It never uses port 8460 or 5173, so it can run next to a dev server.
  Traces and screenshots of failed tests go to `web/apps/web/test-results` and the report to
  `web/apps/web/playwright-report` (both git-ignored).
- The Vite scripts call `vite --configLoader runner`: the default config loader bundles the
  config and cannot resolve the workspace package `@mivo/ui-tokens`. Pass the flag when you
  call Vite directly.

## Running the server

```
go run ./cmd/mivo
```

The server listens on port **8460** on all interfaces and serves the REST API under
`/api/v1` plus a placeholder page, because the untagged build does not embed the web app.
`go run ./cmd/mivo --version` prints the version, `--help` the flags. Windows may ask for
firewall permission the first time a new binary listens on all interfaces; set
`$env:MIVO_SERVER_HOST = '127.0.0.1'` to stay on loopback.

Configuration (ADR 0018): defaults < `config.yaml` in the config directory < `MIVO_*`
environment variables. Default directories are `%LOCALAPPDATA%\Mivo` on Windows and the XDG
directories on Linux. `MIVO_CONFIG_DIR` (or `--config-dir`) moves only the config directory;
data, cache and transcode stay at their OS defaults unless set as well. For a throwaway run
that leaves your real data alone, set all four (relative `MIVO_PATHS_*` values resolve
against the config directory):

```
$env:MIVO_CONFIG_DIR = "$env:TEMP\mivo-dev"
$env:MIVO_PATHS_DATA = 'data'
$env:MIVO_PATHS_CACHE = 'cache'
$env:MIVO_PATHS_TRANSCODE = 'transcode'
go run ./cmd/mivo
```

To play something, add a library: `$env:MIVO_LIBRARIES = "$PWD\testdata\media"` points it at
the test corpus (see below). The server syncs the configured libraries and scans them at every
start. It looks up `ffmpeg` and `ffprobe` on `PATH` (`MIVO_TOOLS_FFMPEG`, `MIVO_TOOLS_FFPROBE`
override them) and logs what FFmpeg can do; without ffmpeg it starts with direct play only,
without ffprobe its scans fail with `prober_unavailable`.

With the embedded web app (after `pnpm -C web build`):

```
go run -tags webui ./cmd/mivo
go build -tags webui -o bin/mivo.exe ./cmd/mivo
```

### Web development server

```
pnpm -C web dev
```

Vite serves the app on <http://localhost:5173> (fixed, `strictPort`) and proxies `/api` to
the Go server, by default `http://localhost:8460`. Start the server first. To proxy to a
server on another address, set `MIVO_DEV_API_TARGET`:

```
$env:MIVO_DEV_API_TARGET = 'http://localhost:18460'
pnpm -C web dev
```

In bash: `MIVO_DEV_API_TARGET=http://localhost:18460 pnpm -C web dev`.

## Test media corpus

```
go run ./scripts/gen-testmedia
```

Generates rights-free clips, audio, images and subtitles from FFmpeg `lavfi` sources into
`testdata/media/` (git-ignored) plus `manifest.json` (ADR 0021). It needs `ffmpeg` on `PATH`
(or `-ffmpeg <path>`), skips cases the local FFmpeg cannot build and keeps existing files.
Flags: `-force` regenerates, `-only 'video-mkv-*'` selects cases by glob, `-jobs N` limits
parallel FFmpeg processes, `-out <dir>` changes the output directory. Integration tests skip
cleanly when the corpus is missing.

## Docker image

```
docker build -f packaging/docker/Dockerfile -t mivo:dev .
docker run -d --rm --name mivo -p 8460:8460 --stop-timeout 20 mivo:dev
```

The build needs Docker Desktop and network access for the base images. It builds the web
app and the binary inside the image, so no local build is required. Volumes, user IDs,
health check and NAS notes are in `packaging/docker/README.md`. Its multi-line examples use
POSIX `\` continuations; in PowerShell write them on one line or continue with a backtick.

## Git hooks

lefthook v2.2.2 is a pinned Go tool (ADR 0019); there is no global or npm install.

```
go tool "-modfile=tools/go.mod" lefthook install
```

Run it once per clone. It writes `.git/hooks/pre-commit` and `.git/hooks/commit-msg`, which
call `go tool -modfile=tools/go.mod lefthook` again, so the hooks work from PowerShell, Git
Bash and Linux. Git runs them with Git's `sh`, so Go (and pnpm for web files) must be on
the `PATH` that your Git client sees.

- **pre-commit** (parallel, staged files only): `golangci-lint fmt --diff` and
  `golangci-lint run` on the packages that contain staged Go files,
  `go test ./internal/domain/... ./internal/app/...`, and for staged web files Prettier,
  ESLint and `vitest related` (coverage off, partial runs would always miss the thresholds).
- **commit-msg**: `go run ./scripts/commitmsg` checks the Conventional Commits header
  `type(scope)!: subject` with the types `feat fix docs style refactor perf test build ci
  chore revert`. It accepts git's own `Merge …`, `Revert "…"`, `Reapply "…"`, `fixup! …`,
  `squash! …` and `amend! …` subjects. It strips only `#` comment lines: with another
  `core.commentChar`, a comment directly below the header is rejected as a missing blank
  line.
- Run a hook by hand: `go tool "-modfile=tools/go.mod" lefthook run pre-commit`.
- **Bypass** in an emergency: `git commit --no-verify`, or `$env:LEFTHOOK = '0'` for the
  session (bash: `LEFTHOOK=0 git commit …`). The full gates still apply before hand-in.
- **Limitation**: the Go jobs match `*.go` files only. A commit that changes only `go.mod`,
  `go.sum` or `.golangci.yml` skips them; run the full Go gates for such a commit.

## Full gates before hand-in

The hooks are fast on purpose. Before a task is done, run in this order:

1. Go gates (incl. the coverage gate and govulncheck)
2. `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`
3. Gates with the embedded web app (`golangci-lint run --build-tags webui`, tagged build)
4. `pnpm -C web e2e`
5. Generated-code drift check, when the spec, migrations or queries changed
6. `docker build …` when the Dockerfile, the build or the dependencies changed

**CI** (GitHub Actions) is planned as a separate task after milestone 1 and will run the same
gates. Until then they run locally.

## Ports

| Port | Used by |
|---|---|
| 8460 | server default (`server.port`, `MIVO_SERVER_PORT`) |
| 5173 | Vite dev server (fixed; fails if the port is taken) |
| free port | E2E server, chosen per run on `127.0.0.1` |

## Renaming the product

"Mivo" is a working title (ADR 0002). Display names come from `internal/platform/brand` and
`web/packages/ui-tokens/src/brand.ts`. Structural identifiers (module path, `cmd/mivo`,
`@mivo/*` scope, `MIVO_*` variables, Docker image name, …) are listed in the rename
checklist of ADR 0002.
