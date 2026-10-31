# SPK-09 test report (Synology documentation and manual test protocol)

Round 1. Result: **PASS** (one criterion needs the owner's approval, see "CLAUDE.md").

Worktree `.claude/worktrees/agent-adf7ca50739a69d8d`, base 9ab6832, uncommitted. Change set:
`README.md`, `docs/development.md`, `docs/tasks/BACKLOG.md` (modified);
`packaging/synology/README.md`, `packaging/synology/TESTING.md` (new). Nothing was edited by the
tester except this report.

## Checks requested

### 1 Relative links and anchors - met

A node script (not committed) extracted every `](target)` and `<...>` target outside code fences
from the four changed Markdown files that contain links and checked the file and, for `#anchor`
targets, the GitHub-style heading slug.

- 15 relative links, 0 broken: `README.md` -> `packaging/synology/README.md` (and the existing
  docker links), `docs/development.md` -> `../packaging/synology/README.md` and `TESTING.md`,
  `packaging/synology/README.md` -> `../../docs/adr/0018-configuration-and-os-paths.md#notes-m2-31`,
  `../docker/README.md#local-network-only`, `ffmpeg/build/README.md`, `ffmpeg/README.md`,
  `toolkit/README.md`. `TESTING.md` has no Markdown links.
- Non-link references in backticks also exist: `DSM-NOTES.md`, `test/README.md`,
  `Dockerfile.dockerignore`, `toolkit/tarballs.sha256`, `toolkit/{run.sh,test.sh,Dockerfile}`,
  `ffmpeg/{fetch.sh,sources.lock}`, `cmd/spkstage`, `internal/{spkicon,spkmeta}` (under
  `packaging/synology/`), ADR 0002/0018/0026, the heading "Libraries and tools" in
  `docs/development.md:299`, and the sections "Privileges", "Platforms", "Caching" in
  `toolkit/README.md` and "Gates", "Verification under emulation" in `ffmpeg/build/README.md`.
- Intentionally absent: `packaging/synology/test/inspect/inspect-spk.sh` (SPK-10). Both documents
  say "Task SPK-10 adds ...", so this is a forward reference, not a broken one.

### 2 Commands against the real scripts - met

`pwsh -NoProfile -File packaging/synology/build.ps1 -?`:
`build.ps1 [[-Version] <string>] [-Release] [-TestBuild] [-Revision <string>] [-PackageRevision <int>] [-SourceDateEpoch <string>] [-Privilege <string>]`
`bash packaging/synology/build.sh --help` (Git Bash): `--release`, `--test`, `--revision REV`,
`--package-revision N`, `--source-date-epoch N`, `--privilege MODE`, `-h, --help`, positional
`[version]`.

| Documented | Source of truth | Result |
|---|---|---|
| option table in `README.md` "Usage" (7 options plus help, `-?` / `-h --help`) | both help texts, `[ValidateRange(0, 9)]`, `[ValidateNotNullOrEmpty()]` | identical names, meanings, defaults |
| env `SPK_TOOLKIT_VOLUME`, `SPK_TOOLKIT_IMAGE`, `NODE_IMAGE`, `GO_IMAGE`, `FFMPEG_FETCH_IMAGE`, `BUILDKIT_PROGRESS`, `SPK_BUILD_MODE`, `SPK_PRIVILEGE`, `SOURCE_DATE_EPOCH`, CI rule | `build.sh:34-36,64-65,142,76`, `build.ps1` header | match |
| PowerShell 7.2 or newer | `build.ps1:1` `#Requires -Version 7.2` | match |
| bash 4 or newer, `MSYS_NO_PATHCONV` set by the script | `build.sh:3,68` | match |
| step 1 `docker build -f packaging/synology/Dockerfile ... --output type=local,dest=.../dist/stage .` | `build.sh:227-229` | match |
| step 2 `docker run --rm --cap-add SYS_ADMIN -v spk-toolkit:/toolkit -v <stage>/<family>:/project:ro -v <dist>:/output spk-toolkit:dev <family>` | `build.sh:303-305` | match |
| privilege modes `sys-admin`, `sys-admin-apparmor`, `privileged` and their docker options | `build.sh:158-160`, help | match |
| platforms x86_64 -> v1000, armv8 -> rtd1619b, armv7 -> alpine; `EnvDeploy -v 7.2 -p <platform> -D`; `PkgCreate.py -v 7.2 -p <platform> -c <id>` | `toolkit/run.sh:44-52,150,257` | match |
| pin `e4a8d54209a31cef21b7636ad58b1b0e86b5b201`, branch `DSM7.2`, `PKGSCRIPTS_COMMIT` | `toolkit/run.sh:19-21` | match |
| `NOT-FOR-RELEASE.txt`, `SHA256SUMS`, file names `<id>-<version>[-r<N>]-<family>.spk` | `build.sh:47-48,266-279,319-330` | match |
| interim pins: jellyfin-ffmpeg 8.1.3-1 for x86_64/armv8, johnvansickle 7.0.2 for armv7 | `ffmpeg/sources.lock` | match |
| Go build flags `CGO_ENABLED=0 -tags webui -trimpath`, `GOARM=7` | `Dockerfile:90-106` | match |
| version table 0.1.0-alpha.2 -> 0.1.0-1020, r1 -> 1021, beta.1 -> 2010, rc.3 -> 3030, 0.1.0 -> 9000, 0.0.0-dev -> 0.0.0-0000 | `internal/spkmeta/version_test.go:22-31` | match |
| test commands (`spk-script-test:dev`, `spk-build-test:dev`, shellcheck v0.11.0 file lists, `toolkit/test.sh [lint\|family]`) | `test/README.md:18-21,143-147`, `toolkit/test.sh` header | match; the shellcheck list for build/test scripts additionally names `fetch.sh`, `run.sh`, `test.sh` (superset of `test/README.md`) |

Executed from the docs to confirm they work: the two shellcheck commands (exit 0, exit 0) and the
build-script test (`docker build -t spk-build-test:dev ...` then `docker run ...`; 32 cases, last
lines `ok 25 ... ok 32`, exit 0, includes the `build.ps1` parity cases). Not run (not part of this
gate, heavy or needs the 24 GB volume): the package-script bats image, `toolkit/test.sh`, a real
`build.ps1`/`build.sh` build.

### 3 SPK inspection command - met

Built an SPK-shaped tar in `debian:stable-slim` (INFO, xz-compressed `package.tgz`, `scripts/`,
name `mivo-0.1.0-alpha.2-x86_64.spk`) in the scratchpad and ran the documented one-liner from
`docs/development.md` verbatim (only the `dist` mount source differs). Exit 0, output:

```
-rw-r--r-- root/root  50 INFO
-rw-r--r-- root/root 208 package.tgz
drwxr-xr-x root/root   0 scripts/
-rw-r--r-- root/root   2 scripts/postinst
package="mivo"
version="0.1.0-1020"
arch="x86_64"
drwxr-xr-x root/root 0 ./
drwxr-xr-x root/root 0 ./bin/
-rw-r--r-- root/root 2 ./bin/mivo
```

The listing, the INFO read and the xz payload listing all work.

### 4 LF only - met

`tr -cd '\r' < file | wc -c` is 0 for all five changed files; each ends with a single `\n`. The
three tracked files are `i/lf w/lf attr/text=auto eol=lf`; the two new files are `w/lf` under the
same attribute. (A first `grep -c $'\r'` showed line counts only because of Git Bash quoting; the
`tr` count is the authoritative one.)

### 5 Brand rule (ADR 0002) - met

`packaging/synology/README.md` and `TESTING.md` are on the rename checklist (ADR 0002 Notes
"Docs that name the product"). Every occurrence of `mivo` / `Mivo` / `MIVO` in the change set
(case-insensitive search of the five files plus the added lines of `README.md` and
`docs/development.md`):

| Where | Occurrence | Reason |
|---|---|---|
| `packaging/synology/README.md:14` | `` `mivo` `` | states the current value of `<id>` (`brand.ID`), source of the placeholder convention |
| `packaging/synology/README.md:15` | `` `Mivo` `` | states the display name shown in Package Center |
| `packaging/synology/README.md:51` | `github.com/MAGPEEK/Mivo/releases` | repository URL, where the files are published |
| `packaging/synology/README.md:61` | `mivo-0.1.0-alpha.2-x86_64.spk` | literal file name in a copy-paste checksum command (derived value) |
| `packaging/synology/README.md:133` | `MIVO` | states the current environment prefix next to `<PREFIX>` |
| `packaging/synology/README.md:550` | `mivo-0.1.0-alpha.2-x86_64.spk` | file name in the copy-paste inspection command |
| `packaging/synology/TESTING.md:9` | `` `mivo` `` | states the current value of `<id>` |
| `docs/development.md` (added line in the "Synology package" inspection command) | `mivo-0.1.0-alpha.2-x86_64.spk` | file name in the copy-paste command; `docs/development.md` is itself on the checklist |
| `README.md` (root, added lines) | none | |
| `docs/tasks/BACKLOG.md` | none added | |

All other places use `<id>`, `<display name>`, `<PREFIX>`. No structural identifier was added; only
prose and derived values. Nit (not blocking): the three `mivo-0.1.0-alpha.2-...spk` file names
could use `<id>` and would then drop off the rename checklist, but they are copy-paste commands.

### 6 BACKLOG "spk docs" row - met, with one observation

`git diff` of `docs/tasks/BACKLOG.md` removes exactly the line starting `| spk docs |` (no other
change). Both points are documented:

- Point 1 (extra libraries use `path: "/volumeN/<share>"`, edited over SSH with `sudo`, file 0640,
  package user): `packaging/synology/README.md` "Add more shared folders" (lines 140-185: `sudo vi`,
  `/volumeN/<share>`, mode `0640`, owner `<id>`, `chown` recovery, `ls -d /volume*/<share>`) and
  the matching step E2 in `TESTING.md`.
- Point 2 (a configuration kept from an earlier installation is used unchanged, the chosen folder
  is not added): README "Install" lines 95-99 and "Uninstall" lines 256-259, `TESTING.md` F3.

Observation (not a failure of SPK-09, which is docs-only): the row also asked that the generated
`config.yaml` comment (`package/scripts/postinst:35-37`) and the install wizard text
(`WIZARD_UIFILES/install_uifile:11`) say this themselves. They still do not mention
`/volumeN/<share>`, `sudo` or the kept-configuration rule; the wizard mentions the config path but
not the sudo/volume path or that a kept file wins. With the row deleted, nothing tracks that
template change. The task text and the orchestrator brief define the row as covered by the docs;
the orchestrator may want to keep a one-line follow-up (for SPK-12 or the next change to the
package texts).

### 7 Acceptance criteria of SPK-09

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | README.md of `packaging/synology`: up-front statement and full topic list | met | Up front: lines 3-7 (media server on the NAS as a DSM service, browser on port 8460, not a player app). Topics: DSM 7.2+ and families (18-45); family lookup via Synology KB and Info Center plus `uname -m` (32-41); download and checksum (49-66); Manual Install and third-party warning (68-88); wizard (80-84); more shared folders with Control Panel path, package user, "Read only" (140-185); `config.yaml` over SSH with path, example, restart (148-181); fixed settings and why (119-138); Open button and firewall service (103-115); LAN guard, ADR 0018 Notes M2-31 link, reverse proxy and QuickConnect warning (186-206); logs (208-218, Package Center has no log view as far as known); upgrade, downgrade, uninstall data behaviour (220-264); version mapping 0.1.0-1020 (234-249); SPK per model incl. armv7 alpine/armada38x/monaco untested (30-45); limits (266-282: software transcoding, 1 GB RAM vs 256 MB armv8, armv7 Direct Play/Remux, emulation only); FFmpeg licence and source in the release asset (284-318); troubleshooting incl. port in use, permission denied, "broken", start failure message, share-name typo (320-332). |
| 2 | "Build" section | met | Line 334 onward: prerequisites (343-353, Docker Desktop; the privileges are `--cap-add SYS_ADMIN`, which Docker Desktop allows by default - the brief's "privileged containers allowed" is covered by that sentence), `build.ps1`/`build.sh` usage (355-388), the two steps (390-413), volume `spk-toolkit` about 24 GB with reset and dropping `toolkit_tarballs/` (438-455), `--privileged` vs `--cap-add SYS_ADMIN` and AppArmor (457-469), inputs and outputs (415-436), own FFmpeg build SPK-15 and cache (471-487), pinned commit and update procedure (489-504), pitfalls: ASCII-only `depends`, log-grep success check, no signing (506-527). |
| 3 | `docs/development.md` "Synology package" | met | New section at `docs/development.md:504`: build command in PowerShell and bash, script tests, shellcheck, `go test ./packaging/...`, `toolkit/test.sh`, SPK inspection command (executed, see 3). |
| 4 | `TESTING.md` protocol | met | Steps A1-F5 (42 steps in six tables, each with Action, Expected, Result column; ids are letter+number rather than plain numbers, in order), hand-over table, "Data the owner reports back" with model, DSM version/build, platform/CPU, kernel, `/bin/sh`, RAM, language, SPK file name and SHA-256, date; plus the table "Unconfirmed facts -> result" tying every assumption to a step. |
| 5 | `README.md` links to `packaging/synology/README.md` | met | `README.md:55-57`, link resolves. |
| 6 | Links resolve, English, no `CLAUDE.md` in the change set | met | Check 1; all text English; `git status --short` shows no `CLAUDE.md`, `git diff 9ab6832 --name-only` lists only the three tracked files. |
| 7 | Gates unchanged | met | Check 8. |

### CLAUDE.md - pending owner approval

SPK-09 states "**`CLAUDE.md` is not changed** (owner decision 2026-10-31)", and the task did not
change it: `git status --short CLAUDE.md` is empty and `CLAUDE.md` is absent from
`git diff --name-only`. This is the one line that stays **pending owner approval**: whether the
"Commands" block of `CLAUDE.md` should one day gain the Synology build/test commands is the
owner's decision and was explicitly not taken by the implementer or the tester. The tester does
not mark it as satisfied beyond "untouched".

### 8 Unchanged gates - met

| Gate | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test ./...` | exit 0, every package `ok`, including `packaging/synology/cmd/spkstage`, `.../internal/spkicon`, `.../internal/spkmeta` and `internal/archtest`; no FAIL lines |
| `golangci-lint run` | `0 issues.` (first attempt refused with "parallel golangci-lint is running" because of another agent; rerun after 20 s was clean) |
| `pnpm -C web install --frozen-lockfile` | done in 22.4 s (node_modules was missing in the worktree) |
| `pnpm -C web lint` | exit 0: dependency-cruiser "no dependency violations found (304 modules, 763 dependencies cruised)", Prettier "All matched files use Prettier code style!", `tsc -b` clean, ESLint passed |
| SPK script tests | build-script bats in Docker: 32 cases ok (includes `build.ps1` parity); shellcheck of the package scripts and of the build/test scripts: exit 0 |

Not run (docs-only change, no code touched): `go test -race` with coverage, web tests and E2E,
Docker image build, media corpus. The domain layer rule is not affected (no Go file in the
change set).

`git status --short` after all runs shows only the five expected paths; the pnpm install and the
Docker runs left no tracked change.

## Findings

No failing finding. Observations for the orchestrator or reviewer:

1. `docs/tasks/BACKLOG.md`: the removed `spk docs` row also asked for wording changes in the
   generated `config.yaml` comment and the install wizard; only the Markdown documentation was
   added (see check 6). Consider a one-line follow-up for SPK-12 / the next package-text change.
2. `packaging/synology/README.md:161-173`: the `config.yaml` example adds a `transcode:` block
   and a second library that the generated file does not contain. It is presented as the result
   after editing, so it is not wrong, but a reader comparing it with the real file sees extra
   lines.
3. `packaging/synology/README.md:419-423`: the "Inputs" table mixes repository-root paths
   (`go.mod`, `cmd/`, `internal/`, `web/`) and paths relative to `packaging/synology/`
   (`package/`, `internal/spkicon`, `internal/spkmeta`). A short "(repository root)" /
   "(packaging/synology)" marker per row would remove the ambiguity.
4. `packaging/synology/README.md` Build prerequisites does not say "privileged containers
   allowed"; it states that `--cap-add SYS_ADMIN` is needed and allowed by default in Docker
   Desktop, which is the accurate requirement.

## History

- Round 1: PASS; the CLAUDE.md line explicitly pending owner approval.

REPORT
status: PASS
task: SPK-09
files: docs/reports/SPK-09-test.md
tests: relative links and anchors (15 links, 0 broken); build.ps1 -? and build.sh --help compared with the docs (match); documented SPK inspection command run on an SPK-shaped tar in Docker (exit 0); LF check (0 CR in 5 files); go vet ./... exit 0; go test ./... all ok; golangci-lint run 0 issues; pnpm -C web lint exit 0; shellcheck (package and build scripts) exit 0; build-script bats 32 ok
findings:
- no failing finding; CLAUDE.md untouched, its line stays pending owner approval
- BACKLOG row "spk docs" removed; docs cover both points, but the config.yaml comment (postinst:35-37) and the wizard text (install_uifile:11) still lack the /volumeN, sudo and kept-config hints, and nothing tracks that now
- README "Inputs" table mixes repo-root and packaging/synology-relative paths; config.yaml example shows more than the generated file (nits)
- brand occurrences listed in the report: 8 places, all prose or derived values (current id/display name/prefix, repo URL, three copy-paste SPK file names)
next: orchestrator runs mivo-reviewer; decide whether to keep a one-line BACKLOG follow-up for the config.yaml comment and wizard wording; CLAUDE.md change (if any) needs the owner's explicit approval
