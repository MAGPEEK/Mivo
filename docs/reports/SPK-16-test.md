# SPK-16 test report

Status: PASS (round 2)

## Gates (worktree, base 9ab6832, uncommitted)

| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test ./...` | all packages ok (incl. `packaging/synology/cmd/spkstage`, `internal/recipecheck`) |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` | no diff |
| `go mod tidy -diff` | no diff |
| shellcheck v0.11.0: build.sh, ffmpeg/build/scripts/*.sh, ffmpeg/build/verify/*.sh | clean |
| shellcheck `-s sh` fetch.sh | clean |
| shellcheck `-s bats` all bats files, `-s bash` fake-docker.bash, helpers.bash | clean |
| hadolint: packaging/synology/Dockerfile, ffmpeg/build/Dockerfile | clean |
| bats `spk-build-test:dev` on `test/build` | `1..56`, 56 ok (build.sh 22, build.ps1 22 = parity, fetch 8, package-gates 4) |
| bats fetch.bats + package-gates.bats under BusyBox sh (`spk-script-test:dev`, Alpine) | `1..12`, 12 ok |
| `docker build -t spk-ffmpeg-static:dev packaging/synology/ffmpeg/build` (warm) | 1.5 s, all layers CACHED |
| six binary hashes vs `sources.lock` pins | all equal (exported with `--target all`) |
| `toolkit/test.sh` on a copy of the toolkit volume (seeded read-only from `spk-toolkit`) | "all toolkit tests passed" (73 s) |

Coverage thresholds not applicable: no `internal/domain` change; the Go diff is packaging-only.

## End-to-end release build

Throw-away git repo of the worktree tree, `bash packaging/synology/build.sh --release 0.1.0-alpha.2`
(toolkit volume = the copy):

- rc 0, three SPKs + `ffmpeg-9.0.2-static1-source.tar` + `SHA256SUMS`; `sha256sum -c`: 4 OK.
- Build 1 (FFmpeg image already warm): 77 s. Build 2 (second build): step 0 (FFmpeg) 1 s, total 41 s.
  A cold recipe build was not repeated (SPK-15: 10-14 min).
- Source archive SHA-256 `fef77f6c...d7563` equals the `Source SHA-256:` line in SOURCE.txt.
- Binaries inside the SPKs (extracted from package.tgz) have the pinned hashes; SOURCE.txt is in
  `share/licenses/ffmpeg/` with Status `selected`, plus 8 licence files.

### inspect-spk.sh --release (SPK-10, main, image built from its Dockerfile)

| Family | Result |
|---|---|
| x86_64 | 67 checks, 0 failed, 0 warnings |
| armv8 | 67 checks, 0 failed, 0 warnings |
| armv7 | 67 checks, 0 failed, 0 warnings |

Warnings: none. `info` lines only (FFmpeg build: version 9.0.2-static1, status selected, licence
GPL-3.0-or-later; no GNU ABI note in the static musl binaries; armv7 e_flags 0x05000400 hard-float).

### `bin/ffmpeg -version` under `--platform` (from the unpacked SPKs)

- linux/amd64: `ffmpeg version 9.0.2-static1`, configure `--arch=x86_64 --cpu=x86-64 ...`
- linux/arm64: `ffmpeg version 9.0.2-static1`, configure `--arch=aarch64 --cpu=armv8-a ...`
- linux/arm/v7: `ffmpeg version 9.0.2-static1`, configure `--arch=arm --cpu=armv7-a ...`
- `ffprobe -version` also reports 9.0.2-static1 in all three.

### Refusals

- Dirty tree: `build.sh --release` in the throw-away repo with a modified README.md exits 1:
  "release refused: the working tree has uncommitted changes".
- Modified licence in a staged SOURCE.txt: bats case "release refuses licence mismatch" (fake
  docker, sh and ps1) passes: a licence other than the pinned one is refused, and so is a longer
  value that merely starts with the pinned one.

## Acceptance criteria

1. build.sh/build.ps1 take FFmpeg from the recipe image (step 0, BuildKit cached); no `fetch.sh`
   download in build.sh/build.ps1/Dockerfile (grep: only a history comment); the staged project
   holds ffmpeg, ffprobe, licence files, SOURCE.txt `selected` - MET (E2E build, bats "ffmpeg image
   built before stage").
2. `grep -q '^[^#][^ ]* interim ' sources.lock` fails (rc 1); the header describes the own build;
   `fetch.sh` is kept for staging by hand and README.md says so - MET.
3. `dist/` holds `ffmpeg-9.0.2-static1-source.tar` and its `SHA256SUMS` line - MET.
4. `inspect-spk.sh --release` passes on all three SPKs, 0 failed - MET.
5. `bin/ffmpeg -version` under emulation shows 9.0.2-static1 and the configure line - MET.
6. A second build reuses the cache: FFmpeg step 1 s, total 41 s versus 77 s - MET.
7. Gates: shellcheck, script tests, Go gates green; the build succeeded on Docker Desktop - MET.

BACKLOG: the row "spk/ffmpeg build" (SPK-15 review leftovers) is removed; the static-PIE row
stays (`docs/tasks/BACKLOG.md:68`).

## Observations (not failures)

- SPK files are not byte-reproducible between two builds (toolkit packing, see SPK-10 header); the
  FFmpeg binaries and the source archive are.
- The build log shows `[auth] library/spk-ffmpeg-static:pull token` lines: BuildKit probes the
  registry for the local-only tag, then resolves the local digest. Harmless.

## History

- Round 1: earlier version of this report (not retained).
- Round 2: PASS, all gates and criteria as above.
