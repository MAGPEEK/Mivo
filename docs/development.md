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
| pnpm | 10 (`packageManager: pnpm@10.34.5`) | web workspace | none, see below |
| golangci-lint | v2.15.0 | Go lint and format | `GolangCI.golangci-lint` |
| Git | tested with 2.56, with Git Bash on Windows | hooks run in Git's `sh` | `Git.Git` |
| FFmpeg (`ffmpeg`, `ffprobe`) | tested with 9.0.2 (gyan.dev full build) | test media corpus, scans, remux and transcode, FFmpeg tests, E2E, playback smoke test | `Gyan.FFmpeg` |
| Docker Desktop | tested with 4.97.0 (Engine 29.9.2) | image build, sqlc, race tests without a C toolchain, playback smoke test | `Docker.DockerDesktop` |
| C toolchain | LLVM-MinGW (UCRT), tested 23.1.4-20261020 | `go test -race` on Windows only | `MartinStorsjo.LLVM-MinGW.UCRT` |
| Microsoft Edge or Google Chrome | any current stable | the `branded` E2E project (optional, skipped when missing) | preinstalled (Edge) / `Google.Chrome` |

Install with `winget install --exact --id <id>` and open a new terminal afterwards so that
`PATH` is updated. pnpm is not installed through winget: once Node.js is installed, run
`npm install -g pnpm@10`. The pinned Go tools (go-test-coverage, govulncheck, oapi-codegen,
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
- **Coverage without a C toolchain**: the race line needs cgo (see "Race detector on
  Windows"). Without it, write the profile with a run without `-race` and run the race tests
  in Docker:

  ```
  go test "-coverprofile=coverage.out" ./...
  go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"
  ```

  Or let the Docker race run write the profile into the mounted tree (it then measures the
  Linux build, so Windows-only files are missing and Linux-only files count):
  `docker run --rm -v "${PWD}:/src" -v gomodcache:/go/pkg/mod -w /src golang:1.27.3 go test -race "-coverprofile=coverage.out" ./...`,
  followed by the coverage gate on the host.
- `go test ./...` includes the tests that run the real FFmpeg; see "Tests that run FFmpeg"
  for their run time and how `-short` skips them.
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

  On Linux the container runs as root, so the files it writes belong to root. Add
  `--user "$(id -u):$(id -g)"` after `--rm` to keep your own user as the owner. Docker
  Desktop on Windows needs nothing extra.

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
  `sha256sum`). GitHub prints the digest with a `sha256:` prefix
  (`sha256:352711fa…`); compare the part after it.

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
  a free port and an isolated configuration. It needs Go, `ffmpeg` and `ffprobe` on `PATH`
  and the Chromium shell from the first setup; without FFmpeg it stops at once with a message.
  It never uses port 8460 or 5173, so it can run next to a dev server.
  Traces and screenshots of failed tests go to `web/apps/web/test-results` and the report to
  `web/apps/web/playwright-report` (both git-ignored). The server log of the run is
  `web/apps/web/test-results/server.log`.

### E2E browser projects

After the server's startup scan, the global setup generates six clips into the server's
library with `go run ./scripts/gen-testmedia -only <cases>` (WebM VP9/Opus, MP4 H.264/AAC,
MKV H.264/AAC, FLV H.264/AAC, AVI Xvid/MP3 and the 30-second MKV seek clip). It does not use
`testdata/media`. Three Playwright projects run against that server:

| Project | Browser | What it checks |
|---|---|---|
| `library` | bundled headless Chromium shell | scans the library from the UI; every other project depends on it |
| `chromium` | bundled headless Chromium shell | shell, layout, accessibility (axe), the player UI against mocked playback endpoints, real direct play of the WebM, and the message for a browser without H.264 (codec emulation: the spec makes the browser's media probes deny formats) |
| `branded` | installed Microsoft Edge (Windows) or Google Chrome (elsewhere) | real playback with the proprietary codecs of a user's browser: direct play, remux and transcode with the reason in the stream info, the MKV directly or by remux, and a seek that restarts the remux in the long clip |

The `branded` project uses Playwright's channel of an installed browser, which
`playwright install` does not download. Choose another channel with
`MIVO_E2E_BRANDED_CHANNEL` (for example `chrome` or `msedge-beta`):

```
$env:MIVO_E2E_BRANDED_CHANNEL = 'chrome'
pnpm -C web e2e
```

In bash: `MIVO_E2E_BRANDED_CHANNEL=chrome pnpm -C web e2e`. When the channel is not installed,
the run prints `Skipping the branded project: …` once and reports its tests as skipped; the
other projects still run. A release run needs both browser projects green (see "Release
gates").
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

The variables stay set for the rest of the PowerShell session. Remove them afterwards, so a
later `go run` does not silently use the throwaway directories:

```
Remove-Item Env:MIVO_CONFIG_DIR, Env:MIVO_PATHS_DATA, Env:MIVO_PATHS_CACHE, Env:MIVO_PATHS_TRANSCODE
```

In bash, put the variables in front of the command
(`MIVO_CONFIG_DIR=/tmp/mivo-dev MIVO_PATHS_DATA=data … go run ./cmd/mivo`); they then apply
to that one command only.

With the embedded web app (after `pnpm -C web build`):

```
go run -tags webui ./cmd/mivo
go build -tags webui -o bin/mivo.exe ./cmd/mivo
```

### Libraries and tools

`libraries` is an import seed (ADR 0028): the server imports the entries once at startup and
you then manage libraries in Settings > Libraries; removing an entry from the configuration
deletes nothing. `browse.roots` / `MIVO_BROWSE_ROOTS` (OS path list) set where the folder
browser may start (default: present drive roots on Windows; `/media`, `/mnt`, `/srv` and the
home directory that exist elsewhere), and `install.kind` / `MIVO_INSTALL_KIND` is `standard`
(default), `docker` or `synology`.

| YAML key | Environment | Default | Meaning |
|---|---|---|---|
| `libraries` (list of `name`, `path`, `kind`) | `MIVO_LIBRARIES` (OS path list) | none | library folders imported once at the start (ADR 0028); `kind` is `movies` (default) or `shows`; add and change libraries in the web app (Settings > Libraries) |
| `tools.ffmpeg` | `MIVO_TOOLS_FFMPEG` | `ffmpeg` | bare name looked up on `PATH`, or a path |
| `tools.ffprobe` | `MIVO_TOOLS_FFPROBE` | `ffprobe` | as `tools.ffmpeg` |
| `transcode.max_sessions` | `MIVO_TRANSCODE_MAX_SESSIONS` | `2` | concurrent re-encoding sessions, `1`–`8` |

`config.yaml` in the config directory, for example `%LOCALAPPDATA%\Mivo\config.yaml`:

```yaml
libraries:
  - name: Movies
    path: D:\Video\Movies
  - name: Series
    path: D:\Video\Series
    kind: shows
tools:
  ffmpeg: C:\Tools\ffmpeg\bin\ffmpeg.exe
  ffprobe: C:\Tools\ffmpeg\bin\ffprobe.exe
transcode:
  max_sessions: 1
```

`MIVO_LIBRARIES` replaces the whole list of the file. Libraries are imported once per folder: one you delete in the app does not return at the next start. Its elements are separated by the OS list
separator (`;` on Windows, `:` on Linux); each one becomes a `movies` library named after its
last folder. An empty value means no library at all. To point a dev server at the test corpus
(see "Test media corpus"):

```
$env:MIVO_LIBRARIES = "$PWD\testdata\media"
$env:MIVO_TOOLS_FFMPEG = 'C:\Tools\ffmpeg\bin\ffmpeg.exe'   # only when ffmpeg is not on PATH
go run ./cmd/mivo
```

In bash: `MIVO_LIBRARIES="$PWD/testdata/media" go run ./cmd/mivo`, several roots as
`MIVO_LIBRARIES=/srv/movies:/srv/series`.

Rules worth knowing:

- Use absolute roots: a relative path resolves against the config directory, which is rarely
  what you want. Roots must not be nested in each other, and must neither be, contain nor lie
  inside the config, data, cache or transcode directory. A root need not exist at start (a NAS share may mount later); it is shown as
  unavailable until it does.
- **Links**: only relative symlinks that stay inside the root are followed. Absolute symlinks
  and Windows junctions inside a library are skipped and logged, even when their target lies
  inside the root. Use relative links, or add the target folder as its own library.
- **Tools**: `ffmpeg` and `ffprobe` are resolved once at startup. The server logs what FFmpeg
  can do (`ffmpeg ready for direct play, remux and transcode`). Without ffmpeg it starts with
  direct play only and answers remux and transcode requests with `transcode_unavailable`;
  without ffprobe its scans fail with `prober_unavailable`. On Windows a `.bat` or `.cmd`
  wrapper as a tool fails the start, because `cmd.exe` would re-parse its command line.
- **Data directory on local storage**: keep the data directory (`/config` in Docker) on a
  local disk. The server locks it against a second instance, and file locks on SMB or NFS
  shares may not be honoured. Libraries on network shares are fine.
- **No accounts yet**: until milestone 7 the server refuses clients outside the local network
  (`server.lan_only`, default `true`, ADR 0018). Do not expose the port to the internet.

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
(or `-ffmpeg <path>`) and skips cases the local FFmpeg cannot build; the manifest lists them
with the reason, and gaps FFmpeg cannot produce at all. A full run (45 files) took 2 s on the
dev machine.

- **Clips**: most video clips last 2 s. The two seek clips `video-mkv-h264-aac-long`
  (H.264/AAC in Matroska, the remux path) and `video-ts-mpeg2-mp2-long` (MPEG-2/MP2 in
  MPEG-TS, the transcode path) last 30 s with a keyframe exactly every 2 s.
- **Existing files are kept**: a second run only builds what is missing. Kept files are
  labelled `kept` in the progress lines and counted as `kept` in the summary
  (`N generated, N kept, N skipped, N not selected, N gaps, N failed` per category).
  `-force` regenerates every selected case.
- **`-only`** takes a comma-separated list of globs over the case names, for example
  `-only 'video-mkv-*'` or `-only 'video-mp4-*,video-webm-vp9-opus'`. Keep the quotes: an
  unquoted comma makes PowerShell pass an array, and bash would expand `*` against files.
  Cases outside the list are left alone: an existing file from an earlier run is listed as
  `kept`, a missing one as `not selected`, so a filtered run never shrinks the manifest.
- `-jobs N` limits the parallel FFmpeg processes (default: number of CPUs), `-out <dir>`
  changes the output directory.

Tests and the smoke test that need the corpus skip with a message when it is missing.

### Library tree

The same run also writes a library tree for the milestone-3 scan (task M3-17, ADR 0027) into
`testdata/library/` (git-ignored): `movies/` and `shows/` as two library roots plus
`manifest.json`, which describes the **expected catalog** (top-level items with kind, title,
year, version labels, extras, NFO fields, artwork, and seasons with episode numbers) and the
build result of every library case. The schema is documented in the package comment of
`scripts/gen-testmedia/main.go`. It stays apart from `testdata/media`, which tests and the
smoke script use as one movies library.

- **Movies**: folder movie with `- 1080p`/`- 4K` versions, `Trailers/` and a `-featurette`
  extra, a `sample.mkv`, `poster.jpg`, `fanart.jpg` and an RGBA `clearlogo.png`; an
  `{edition-Director's Cut}`; a `cd1`/`cd2` movie; loose movies in one folder; NFO files
  whose title differs from the folder, URL-only, ISO-8859-1 and malformed; an NFD folder
  name (`Amélie (2001)`); a Matroska cover attachment and an MP4 `covr` without local poster.
- **Shows**: `Season 01`, `Season 02`, `Specials`, a multi-episode file, `tvshow.nfo` with
  provider IDs, `season01-poster.jpg`, an episode NFO and `-thumb.jpg`; a show with files in
  the library root; `Dragon Ball Super/` with 131 absolutely numbered files (UX DT-24: one
  show, one season, 131 episodes) and an anime-style release name in a second folder.
- All videos are the same 2 s H.264/AAC clip: one FFmpeg run per item, further files are
  byte copies, so the 165 media files take 20 runs. Images are single `testsrc2` frames (600×900
  posters, 1280×720 backdrops and thumbnails); NFO files are written by the generator.
- **Discs** (`discs/`, task M3-16i, ADR 0029; a library root of its own, so movie counts stay
  unchanged): `Movie (2019)/VIDEO_TS` (6 s PAL feature split into two VOBs, a bonus title
  set, IFO/BUP files), `Feature (2021)/BDMV` (4 s + 3 s feature playlist, a longer menu loop
  and a playlist with a missing clip), an ISO 9660 image of the DVD, and two protected
  stand-ins that only carry the flags of the encryption check (CSS `PES_scrambling_control`;
  AACS folder, `copy_permission_indicator` and lost sync bytes). Nothing is encrypted, there
  are no keys. FFmpeg writes the streams to `.disc-sources/` (outside every library root);
  the generator writes IFO, MPLS, index files and the image itself. The features carry real
  bitmap subtitles: FFmpeg cannot render text into bitmaps, so the generator writes a raw PGS
  stream that the Blu-ray copies (`hdmv_pgs_subtitle`) and the DVD re-encodes with `dvdsub`
  (`dvd_subtitle`). The corpus test checks detection, title selection, error codes, probe
  results and one DVD title produced as HLS (`go test -count=1 -run "Disc|DVD" ./internal/adapters/corpustest/`).
- `-library <dir>` changes the directory, `-library ""` skips the tree, and
  `-only 'library-*'` builds only the tree (`-only 'library-discs-*'` only the discs). `-force` and kept files work as for the corpus;
  deleted copies and NFO files are restored on the next run.

### Corpus integration test

`internal/adapters/corpustest` proves the milestone promise against the generated corpus
(ADR 0025): it builds the server's graph like `cmd/mivo` (temporary SQLite catalog, the corpus
as the only library, the real walker, ffprobe and FFmpeg producer), scans the corpus and plays
every video case with the reference profiles `chromium-desktop` and `firefox-desktop`. It
checks mode, stream actions, reasons and warnings against `expectations.go`, probes the joined
HLS output, starts both seek clips at 21 s, and repeats the decisions for a server without
encoders. Run it alone with:

```
go test -count=1 -v ./internal/adapters/corpustest/
```

It runs as part of `go test ./...` as well. It skips, with the reason, when `ffmpeg` or
`ffprobe` is not on `PATH`, when `testdata/media/manifest.json` is missing, and under
`-short`. `-count=1` bypasses the test cache, which does not see the files FFmpeg reads, nor
a changed FFmpeg.

### Tests that run FFmpeg

Besides the corpus integration test, several packages run the real FFmpeg: `adapters/ffmpeg`
(capabilities, probe, keyframe lookup, HLS arguments, producer), `adapters/http/stream`,
`cmd/mivo` (playback through the wired server), `scripts/gen-testmedia` and
`scripts/playback-smoke`. Without FFmpeg on `PATH` or without the corpus they skip.
Wall-clock times on the dev machine (Intel Core Ultra 7 265K, 20 threads, FFmpeg 9.0.2,
2026-10-31, warm build cache):

| Command | Without corpus | With corpus |
|---|---|---|
| `go test -count=1 ./...` | 15 s | 17 s |
| `go test -count=1 -short ./...` | 13 s | 13 s |
| `go test -count=1 -race "-coverprofile=coverage.out" ./...` | 106 s | 107 s |
| `go run ./scripts/gen-testmedia` (45 files) | – | 2 s |
| `go run ./scripts/gen-testmedia -force` (45 files and the library tree, 2026-11-04) | – | 2.5 s |

- The corpus itself costs little: the clips are small, and `corpustest` plays all of them in
  about 5 s (9 s with `-race`).
- `internal/adapters/ffmpeg` dominates: about 12 s, and about 98 s with `-race`, also with
  `-short`. Under `-race` its process stress test `TestManyShortLivedStartsSucceed` alone takes
  about 65 s; it starts short-lived processes and needs no FFmpeg, so `-short` does not skip
  it. A slower machine multiplies all numbers.
- `-short` skips only the real-FFmpeg tests of `corpustest`, `adapters/ffmpeg`,
  `adapters/http/stream` and `cmd/mivo`. The integration tests of `scripts/gen-testmedia` and
  `scripts/playback-smoke` still run whenever FFmpeg (and for the latter the corpus) is
  there. The pre-commit hook runs no FFmpeg tests. The full gates run without `-short`.

### Playback smoke test

`go test` checks playback with the FFmpeg of the machine it runs on. The smoke script checks a
running server, normally the Docker image with its own FFmpeg, through the public API only
(ADR 0025). It scans every library, waits for the scan, then plays every generated video
case and the seek cases of the profile: it compares mode, stream actions, reasons and
warnings with the same `expectations.go`, downloads the stream (one range request for direct
play; the whole playlist, init segment and media segments for HLS), probes it with the local
`ffprobe` and deletes the session. It prints one line per case and a summary and exits with
`1` on any mismatch.

```
go run ./scripts/gen-testmedia
docker build -f packaging/docker/Dockerfile -t mivo:dev .
docker run -d --name mivo-smoke -p 127.0.0.1:8460:8460 -v "${PWD}/testdata/media:/media:ro" mivo:dev
go run ./scripts/playback-smoke -base http://127.0.0.1:8460
go run ./scripts/playback-smoke -base http://127.0.0.1:8460 -profile firefox-desktop
docker exec mivo-smoke ls /transcode/sessions
docker rm -f -v mivo-smoke
```

- The server must serve the same corpus as the local `-manifest` (default
  `testdata/media/manifest.json`); a local `ffprobe` is needed (`-ffprobe <path>` otherwise).
- `-profile` is `chromium-desktop` (default) or `firefox-desktop`; `-case-timeout` (default
  `2m`) and `-scan-timeout` (default `5m`) bound one case and the scan.
- `docker exec … ls /transcode/sessions` must print nothing: every session was removed.
- A server without a library gets one over its first available browse root (`/media` preferred) through `POST /api/v1/libraries`. It works against any server, for example `go run ./cmd/mivo` with `MIVO_LIBRARIES` set to
  the corpus. Linux file modes and `--user` for the bind mount: `packaging/docker/README.md`,
  "Verify an image".
- Run time on the dev machine: about 5 s per profile against the image (30 cases for
  `chromium-desktop`, 28 for `firefox-desktop`), plus the first compile of the script.

## Docker image

```
docker build -f packaging/docker/Dockerfile -t mivo:dev .
docker run -d --rm --name mivo -p 127.0.0.1:8460:8460 --stop-timeout 20 mivo:dev
```

The build needs Docker Desktop and network access for the base images. It builds the web
app and the binary inside the image, so no local build is required. Volumes, user IDs,
libraries, transcoding, the LAN-only guard and NAS notes are in
`packaging/docker/README.md`; the user guides for the published image are
`docs/guides/docker.md` and `docs/guides/docker-compose.md` (with
`packaging/docker/docker-compose.yml`). The README's multi-line examples use POSIX `\` continuations; in
PowerShell write them on one line or continue with a backtick. To verify an image, run the
playback smoke test against it (see "Playback smoke test").

Never push an image from a dev machine. Releases (version tag, image push to Docker Hub) are
published only by the `mivo-release` agent, after the gates pass and the review approves, and
only with the owner's explicit approval for each push.

## Bundled TMDB key (release builds)

Online metadata (ADR 0031) uses the project's TMDB key when the owner sets no own key
(`metadata.tmdb.api_key`). The key is injected at link time into
`config.BundledTMDBKey` and is empty in every other build, so `go run`, the Dockerfile and
the SPK build work without it (TMDB then stays off until an own key is set):

```
go build -ldflags "-X github.com/magpeek/mivo/internal/platform/config.BundledTMDBKey=$TMDB_KEY" ./cmd/mivo
```

CI passes the key from a repository secret (note for `mivo-release`); never commit a real key
and never print it in build logs.

## Synology package

The native DSM package (ADR 0026) is built with Docker only: a normal `docker build` prebuilds
the web app, the static server binaries and FFmpeg for x86_64, armv8 and armv7 and stages one
toolkit project per family, then Synology's Package Toolkit packs each project in a container
with `--cap-add SYS_ADMIN`. One command builds all three SPKs into `packaging/synology/dist/`
(git-ignored) together with `SHA256SUMS`:

```
pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.2
pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.2 -Release
```

In bash (Linux, Git Bash): `bash packaging/synology/build.sh 0.1.0-alpha.2`, release builds
with `--release`. Without `-Release`/`--release` the result is a test build for the owner's
device only (it may carry an interim FFmpeg and writes `dist/NOT-FOR-RELEASE.txt`); a release
build refuses interim FFmpeg, the default version and a dirty working tree. The first run
downloads Synology's platform environments into the named volume `spk-toolkit` (about 24 GB
for all families, about 10 minutes; CDN speed varies); later runs take about 80 s. Options,
the volume, privileges
(AppArmor hosts), the FFmpeg build and the toolkit pitfalls:
[packaging/synology/README.md](../packaging/synology/README.md), "Build". The user guide for
DSM is the same file; the hardware test protocol is
[packaging/synology/TESTING.md](../packaging/synology/TESTING.md).

Tests of the packaging (all in Docker; Git Bash needs `MSYS_NO_PATHCONV=1` and `"$(pwd -W)"`
instead of `"${PWD}"`):

```
docker build -t spk-script-test:dev packaging/synology/test
docker run --rm -v "${PWD}:/code" -w /code spk-script-test:dev packaging/synology/test/scripts
docker build -t spk-build-test:dev packaging/synology/test/build
docker run --rm -v "${PWD}:/code" -w /code spk-build-test:dev packaging/synology/test/build
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/package/scripts/start-stop-status packaging/synology/package/scripts/common.sh packaging/synology/package/scripts/preinst packaging/synology/package/scripts/postinst packaging/synology/package/scripts/preuninst packaging/synology/package/scripts/postuninst packaging/synology/package/scripts/preupgrade packaging/synology/package/scripts/postupgrade
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/build.sh packaging/synology/ffmpeg/fetch.sh packaging/synology/toolkit/run.sh packaging/synology/toolkit/test.sh packaging/synology/test/build/build-scripts.bats packaging/synology/test/build/fake-docker.bash
bash packaging/synology/toolkit/test.sh
go test ./packaging/...
```

The first pair runs the package scripts under four shells against a fake server (a few
minutes), the second pair the build scripts against a fake Docker (`build.sh` and `build.ps1`
parity, seconds); `toolkit/test.sh` packs placeholder projects for all families on the
`spk-toolkit` volume; the Go line covers `spkmeta`, `spkicon` and `spkstage` and is part of
`go test ./...`. Details: `packaging/synology/test/README.md` and
`packaging/synology/toolkit/README.md`.

To look into a built SPK without installing it (outer tar, `INFO`, then the xz-compressed
`package.tgz`):

```
docker run --rm -e DEBIAN_FRONTEND=noninteractive -v "${PWD}/packaging/synology/dist:/dist:ro" debian:stable-slim sh -c 'apt-get -qq update >/dev/null && apt-get -qq install -y xz-utils >/dev/null && cd /dist && f=mivo-0.1.0-alpha.2-x86_64.spk && tar -tvf "$f" && tar -xOf "$f" INFO && tar -xOf "$f" package.tgz | tar -tJvf -'
```

Task SPK-10 adds `packaging/synology/test/inspect/inspect-spk.sh [--release] <file.spk> <family>`,
the check releases and the hardware test run on the exact files.

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
- **Limitation**: the Go jobs run only when the commit stages at least one `*.go` file outside
  `web/`. A commit without one, for example one that changes only `go.mod`, `go.sum` or
  `.golangci.yml`, skips them; run the full Go gates for such a commit.

## Full gates before hand-in

The hooks are fast on purpose. Before a task is done, run in this order:

1. Go gates (incl. the coverage gate and govulncheck)
2. `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`
3. Gates with the embedded web app (`golangci-lint run --build-tags webui`, tagged build)
4. `pnpm -C web e2e`
5. Generated-code drift check, when the spec, migrations or queries changed
6. `docker build …` when the Dockerfile, the build or the dependencies changed

### Release gates

Before a release candidate, additionally run on a clean checkout (the git-ignored corpus and
build output created by the commands, not copied):

1. `go run ./scripts/gen-testmedia`, then the full gates above, so `go test ./...` includes the
   FFmpeg and corpus tests
2. the corpus integration test: `go test -count=1 -v ./internal/adapters/corpustest/`
3. `pnpm -C web e2e` with both browser projects passing (`branded` not skipped)
4. the drift check and the sqlc diff (step 5), whether or not the inputs changed
5. a fresh `docker build` and the playback smoke test against that image with both profiles

The results go into a release-readiness report in `docs/reports/`.

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
