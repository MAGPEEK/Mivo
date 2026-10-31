# SPK-07 test report

Status: PASS (round 2)
Worktree: `.claude/worktrees/agent-a6cbb837bc1b5ff97`, uncommitted against base e7a965e. Nothing was edited
except this report. Docker Desktop 29.9.2, Windows 11, PowerShell 7, Git Bash.

## Gates

| Gate | Result |
|---|---|
| shellcheck v0.11.0: `build.sh`, `test/build/build-scripts.bats`, `test/build/fake-docker.bash` | exit 0, no findings |
| shellcheck `toolkit/run.sh`, `toolkit/test.sh` (unchanged files) | exit 0 |
| hadolint: `packaging/synology/Dockerfile`, `packaging/synology/test/build/Dockerfile` | exit 0, no findings |
| `go vet ./...` | rc 0 |
| `go test ./...` | all packages ok (no FAIL line) |
| `golangci-lint run` | 0 issues |
| `pwsh -NoProfile -File packaging/synology/build.ps1 -?` | rc 0, prints the syntax line |
| PSScriptAnalyzer | not installed, skipped (the task says "if installed") |
| `docker build -t spk-build-test:dev packaging/synology/test/build` + bats on `packaging/synology/test/build` | `1..32`, 32 ok (16 cases x `build.sh`, same 16 x `build.ps1` parity), 0 not ok, no skips, 21 s |
| package-script bats (`spk-script-test:dev`, `packaging/synology/test/scripts`) | `1..295`, 295 ok, 0 not ok, 0 skips |
| `toolkit/test.sh` with `SPK_TOOLKIT_VOLUME=spk-toolkit-spk07` | 67 ok, "all toolkit tests passed", rc 0, 77 s |
| Line endings (`tr -cd '\r'`) | `build.ps1` CRLF (376 CR), `build.sh`, `Dockerfile`, `Dockerfile.dockerignore`, `.gitignore`, test files: 0 CR |

Go coverage thresholds and the domain layer rule: not applicable. The diff holds no Go source (shell,
PowerShell, Dockerfiles, tests, docs); the Go gates above are green and unchanged.

## End-to-end (real Docker, `SPK_TOOLKIT_VOLUME=spk-toolkit-spk07`, test mode)

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.2`, exit 0 in every run.

Durations:

| Run | Total | Step 1 | Step 2 (x86_64 / armv8 / armv7) |
|---|---|---|---|
| Cold toolkit volume (volume deleted first) | 399 s | 6 s | 196 s / 100 s / 94 s (downloads 134 s, 47 s, 52 s; tarballs 2.1G, 2.8G, 3.5G; `EnvDeploy -v 7.2 -p <platform> -D`) |
| Immediately after (warm) | 78-79 s | 6 s | 33 s / 20 s / 16 s ("environment ds.<platform>-7.2 deployed, EnvDeploy skipped") |

Step 1 was served from the BuildKit cache in the "cold" run too (built in earlier rounds), so only the
toolkit volume was cold. Further warm runs (build.ps1 x3, build.sh x1) took 74-79 s each.

- `sha256sum -c SHA256SUMS`: three `OK` lines. File is `hash<2 blanks>name`, sorted, LF only, no CR.
- INFO of all three SPKs: `package="mivo"`, `version="0.1.0-1020"`, `beta="yes"`, `adminport="8460"`,
  `os_min_ver="7.2-64570"`, no `adminurl` line. `arch="x86_64"`, `arch="armv8"`,
  `arch="armv7 armada38x monaco"`.
- x86_64 `package.tgz` (xz) holds `bin/mivo` (0755), `bin/ffmpeg`, `bin/ffprobe`, `port_conf/mivo.sc`,
  `share/licenses/ffmpeg/`. `bin/mivo` embeds the web app (`assets/index-B2grjsbY.js`, `<div id="root">`).
- `debian:stable-slim`: `/pkg/bin/mivo --version` -> `Mivo 0.1.0-alpha.2 (go1.27.3)`, rc 0;
  `bin/ffmpeg -version` -> `ffmpeg version 8.1.3-Jellyfin`, rc 0; `bin/ffprobe -version` ok.
- `go version -m` of the packed binaries: `-tags=webui`, `-trimpath=true`, `CGO_ENABLED=0`; amd64 /
  arm64 / `GOARCH=arm GOARM=7`; no vcs stamp. `file`: armv8 aarch64 static, armv7 ELF 32-bit EABI5 static.
- Test mode: the interim banner (`WARNING: TEST BUILD WITH INTERIM FFMPEG - NOT FOR RELEASE OR DISTRIBUTION`)
  appears before step 1 and again at the end; `dist/NOT-FOR-RELEASE.txt` lists x86_64 (interim, nonfree),
  armv8 (interim, nonfree), armv7 (interim).
- `stage/build-info.txt` records version, revision (HEAD), package_revision, source_date_epoch (HEAD commit
  time 1793477617), release=false, node/go/fetch images and the FFmpeg status and licence per family.
- The exact documented step-1 command (`docker build -f packaging/synology/Dockerfile --build-arg
  VERSION=0.1.0-alpha.2 --output type=local,dest=packaging/synology/dist/stage .`) yields `x86_64/`,
  `armv8/`, `armv7/` and `build-info.txt` (file times 1970-01-01 with the default epoch 0).
- Build context (probe Dockerfile with a copy of `Dockerfile.dockerignore`, `--no-cache`): top level is
  `LICENSE cmd go.mod go.sum internal packaging web`; `packaging/synology` 113 files; no `dist`, no
  `node_modules`, no `.env*`, no `.git`. `git check-ignore -v packaging/synology/dist/SHA256SUMS` ->
  `packaging/synology/.gitignore:2:/dist/`.
- `-PackageRevision 1`: files `mivo-0.1.0-alpha.2-r1-{armv7,armv8,x86_64}.spk` (old SPKs removed first),
  `SHA256SUMS` lists the `-r1-` names and `sha256sum -c` passes; INFO `version="0.1.0-1021"`,
  staged `DSM_VERSION='0.1.0-1021'`.

### build.ps1 vs build.sh

`bash build.sh 0.1.0-alpha.2` (Git Bash, same volume, same default `SOURCE_DATE_EPOCH` from HEAD): exit 0 in
76 s, same three SPK names, same `SHA256SUMS` format, `sha256sum -c` passes, same banner and marker.

The checksums differ from the build.ps1 run, and also between two runs of the same script. Cause: the
toolkit (SPK-04, not SPK-07) writes `create_time="..."` into INFO and stamps the tar members and
`package.tgz` with the pack time. Compared run to run (x86_64 in full, armv8 and armv7 apart from INFO):
every other outer member and every file inside `package.tgz` is byte-identical; the only content difference
is the `create_time` line of INFO (e.g. `20261101-05:20:43` vs `20261101-05:22:17`). `SOURCE_DATE_EPOCH` only
fixes the staged projects, not the SPK; bit-for-bit reproducible SPKs are not an acceptance criterion.

## Release refusal

Real scripts and the real (interim) `sources.lock`:

- `build.sh --release 0.1.0-alpha.2`, `build.ps1 -Version 0.1.0-alpha.2 -Release`: rc 1,
  `[spk-build] failed: release refused: ffmpeg/sources.lock has interim FFmpeg entries for x86_64 armv8 armv7`.
  `dist/` is byte-for-byte untouched, i.e. it fails before the toolkit image build and before any run.
  With a fake `docker` on PATH, the call log stays empty (0 calls).
- `CI=true build.sh 0.1.0-alpha.2` and `CI=true build.ps1 -Version 0.1.0-alpha.2` (no `--test`): the same
  refusal, rc 1. `CI=true build.sh --test` builds with the banner; `CI=false` and `CI=0` give a test build.

Fake docker (`test/build/fake-docker.bash`) in a throw-away git repo with a lock without interim entries:

- Clean tree `--release`: success, no `NOT-FOR-RELEASE.txt`.
- Dirty tree (modified tracked file and untracked file) `--release`: rc 1, `release refused: the working
  tree has uncommitted changes:` plus the `git status --porcelain` lines; 0 docker calls; `dist/` not
  created. Untracked file alone is refused too.
- `FAKE_TOOLKIT_FAIL=armv8`: `[spk-toolkit] failed: ...` then `[spk-build] failed: toolkit run for armv8
  failed`, rc 1; `dist/` holds only the x86_64 SPK and `stage/`, no `SHA256SUMS`. A stale `SHA256SUMS` from
  an earlier run is also gone after a failed run.
- The same cases pass for `build.ps1` in the bats parity run (cases 17-32).

## Acceptance criteria

The task file was updated during this run (Step 2 and Privileges now say `--cap-add SYS_ADMIN` by default);
the privilege deviation is therefore resolved in the task text, and sys-admin was verified as the default.

| Criterion | Result | Evidence |
|---|---|---|
| Step 1: stages web, Go cross-compile per family (flags, GOARM=7), FFmpeg via `fetch.sh`, `spkstage`; one staged project per family | met | documented command run; 3 projects + `build-info.txt`; `go version -m` flags above |
| Step 2: toolkit run per family, rename to `<id>-<version>-<family>.spk`, `SHA256SUMS`; failed run stops the build | met (default `--cap-add SYS_ADMIN`, deviation accepted) | logged `docker run --rm --cap-add SYS_ADMIN -v spk-toolkit:/toolkit -v <stage>/<family>:/project:ro -v <dist>:/output spk-toolkit:dev <family>`; failure case above |
| `build.ps1 -Version 0.1.0-alpha.2` and `build.sh 0.1.0-alpha.2`: three SPKs and `SHA256SUMS`, fail on first error; second run downloads no platform environment; interim warning | met | durations table (cold 399 s, warm 78-79 s, "EnvDeploy skipped"); banner in both scripts |
| Privileges: `sys-admin` default; option (`--privilege` / `-Privilege`, `SPK_PRIVILEGE`, documented) for `sys-admin-apparmor` and `privileged` | met | fake docker log: `--cap-add SYS_ADMIN`; `... --security-opt apparmor=unconfined`; `--privileged`; `bogus` -> rc 1 with message. AppArmor and GitHub-hosted runners: **not verifiable here** |
| Build inputs `VERSION` (default `0.0.0-dev`), `REVISION`, `PACKAGE_REVISION`, `SOURCE_DATE_EPOCH`, `NODE_IMAGE`, `GO_IMAGE` same names/defaults as `packaging/docker/Dockerfile`; pkgscripts-ng pin in one place | met | `ARG NODE_IMAGE=node:24-slim`, `ARG GO_IMAGE=golang:1.27.3` in both Dockerfiles; commit `e4a8d542...` appears only in `toolkit/run.sh` (and docs), not in the build scripts or Dockerfile |
| Unpacked x86_64 SPK: `bin/mivo --version` in `debian:stable-slim`, `bin/ffmpeg -version` exit 0; INFO values; armv7 arch list | met | see end-to-end |
| `dist/` git-ignored and excluded from the build context; context limited to go.mod, go.sum, cmd, internal, web (no build output), LICENSE, packaging/synology (no dist) | met | context probe, `git check-ignore` |
| Gates: vet, golangci-lint, go test; shellcheck `build.sh`; `build.ps1 -?`; script tests and `toolkit/test.sh` | met | gates table (PSScriptAnalyzer not installed) |
| Docs: `docs/tasks/BACKLOG.md` row "spk release" removed | met | `git diff` shows the single deleted row |

## Findings (none blocking)

- SPK checksums are not reproducible run to run because of the toolkit's pack-time stamps (see above);
  document it if release verification should compare checksums across rebuilds.
- Toolkit prints `[WARNING] /tmp/_install is empty!` twice per family on every pack; it comes from the
  toolkit (SPK-04), is not an error and does not affect the SPKs.
- Not verified: AppArmor hosts / GitHub-hosted runners (`sys-admin-apparmor`), PSScriptAnalyzer.

## Cleanup

Volume `spk-toolkit-spk07` deleted, `packaging/synology/dist/` removed. The volume `spk-toolkit` and image
`spk-toolkit:dev` were not modified by volume writes (the scripts rebuild the image tag `spk-toolkit:dev`
from `packaging/synology/toolkit`, as `toolkit/test.sh` does).

## History

- Round 2: PASS (this report).
- Round 1: no report file existed in the worktree.
