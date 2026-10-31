# SPK wave 1 – test report

Tester: mivo-tester, 2026-10-31, round 1. Tasks SPK-01, SPK-02, SPK-04, SPK-08 (the SPK packages
the media **server** as a DSM service).

| Task | Verdict |
|---|---|
| SPK-01 DSM package facts | **PASS** |
| SPK-02 FFmpeg builds per arch (evaluation task) | **PASS** as an evaluation with a documented negative result; 3 criteria are not met by the interim pins (listed below) |
| SPK-04 Toolkit walking skeleton | **PASS** |
| SPK-08 Package icons | **PASS** |

## How this was tested (read first)

The implementer edited the tree while this test ran: `INFO.sh`, `SynoBuildConf/install`, `run.sh`,
`test.sh`, `fetch.sh`, `sources.lock`, `Dockerfile`, a new `toolkit/tarballs.sha256` and both READMEs
changed between 13:50 and 13:58. A first `test.sh` run was therefore invalid: bash reads a running
script lazily and `test.sh` itself was rewritten during the run (4 "INFO.sh" failures, caused by the
edit, not by the code). All results below are from a **frozen copy** of the final files (copy in the
scratchpad, hashes in `snapshot3.sha256`), except where marked. After the run, only
`toolkit/README.md` differed from the copy (updated afterwards: `--cap-add SYS_ADMIN` default,
"Pinned tarballs", no `adminurl`; I re-read it, it matches the scripts). Every script, the lock file
and the fixture are byte-identical to what was tested.

Environment: Docker Desktop 29.9.2, `koalaman/shellcheck:v0.11.0`, named volume `spk-toolkit`
(reused, three environments deployed), QEMU for arm64 and arm/v7.

## Repository-wide findings that are out of scope

- `go vet ./...` fails in `cmd/mivo/wire_tools_test.go:105:14: undefined: envToolsFFmpeg`. `cmd/mivo/*`
  is being changed by other work in the tree (`cmd/mivo/main.go`, `serve.go`, new `wire_http.go`). It
  is not SPK code and not `internal/adapters/ffmpeg`; the M2 tester should see it.
- `packaging/synology/internal/spkmeta/`, `packaging/synology/package/scripts/` and
  `packaging/synology/test/` appeared during the test (other tasks). I did not test them and scoped
  the Go gates to `./packaging/synology/internal/spkicon/`.

---

## SPK-01 – DSM-NOTES.md: PASS

| Criterion | Result | Evidence |
|---|---|---|
| 15 questions, each in its own numbered section | met | `^## [0-9]+` gives sections 1 to 15 in order |
| Every answer has status and source | met, 3 nits | 82 answer rows, all carry `confirmed` or `unconfirmed`. Rows without a source link: §5 "Time limits" (line 92), §8 "Reboot" (130), §10 "Reinstall after keeping data" (150). They are `unconfirmed`, name an assumption and a check, and no source exists, so I accept them. |
| `unconfirmed` answers state assumption and a later check (SPK-10/11/12) | met | 29 `unconfirmed` rows, awk check: every one contains `Assumption` and `check:` (none without) |
| Every link resolves | met | 58 link definitions, all HTTP 200. A control request for a non-existent guide page returns 404, so the 200s are real pages |
| "Impact on ADR 0026" with proposed wording | met | 22 numbered items |
| Gates: no code changed | met | the task touches one Markdown file |

Claims checked against the cited sources (all hold), 26 in total:

1. `pkg_make_spk`: `tar cf` at `pkg_util.sh:281`, `$pack ... $(ls $source_path)` at 317, `extractsize`/`toolkit_version`/`create_time` at 307-311 (pinned commit `e4a8d54`, read from the volume's checkout).
2. `pkg_make_package`: `tar_option="cJf"` at 215, `ls | tar` at 231, so `package.tgz` is xz.
3. `pkg_dump_info`: unquoted `echo $f=\"${!f}\"` at 193; field list contains `precheckstartstop`, `thirdparty`, `beta`, `os_min_ver`, no `support_move`, no `changelog`.
4. `__get_spk_name` sources INFO (`. $info_path`, line 240).
5. `plat_to_family`: `armv7` = `alpine | alpine4k` only; `armada38x`, `monaco` listed as "armv7 not ready platforms" (lines 84-85); `armv8` = rtd1296, armada37xx, rtd1619b; x86_64 list incl. r1000, epyc7002, v1000nk.
6. Compiler flags: `platform.alpine`/`alpine4k` `-mfpu=neon-vfpv4`, `monaco` `-mfpu=neon`, `armada38x` `-mfpu=vfpv3` (lines 17/17/18/15).
7. `toolkit.config` line 4 platform list (incl. `kvmcloud`) and `platforms` kernel series (3.10 / 4.4 / 5.10 per platform).
8. Guide "Appendix A": `armv7` row = `alpine, alpine4k`; `armada38x`, `monaco` only in the platform value list.
9. Guide "scripts": `prestart` runs "before starting a package at booting up after DSM 7.0"; `prestop` described.
10. Guide "Optional Fields": `precheckstartstop` default `"yes"`.
11. Guide "Package Filesystem Hierarchy Standard": `var`/`tmp`/`home`/`etc`/`target` locations (`@appdata`, `@apptemp`, `@apphome`, `@appconf`, `@appstore`), modes (home 0700, others 0755), remove timing (`tmp`, `target` on uninstall/upgrade; `etc`, `var`, `home` none), script variables.
12. Guide "Data Share": creates the share, `Release()` does nothing, symlink under `/var/packages/<id>/shares/` since 7.0-41201.
13. Guide "Port Config": `protocol-file` example `port_conf/xxdns.sc`, copy to `/usr/local/etc/service.d/`.
14. Guide "Port": `desc` is required, `service_name` characters exclude underscore.
15. Guide "Necessary Fields": version delimiters, numeric parts, range 0 to 2^31-1, "Build number should be increased on every version".
16. Guide "Breaking Changes in 7.0": `os_min_ver` at least `7.0-40000`; 64x64 icon; log locations `/var/log/packages/<id>.log` and `/var/log/synopkg.log`; keyring and trust level removed.
17. Guide "Script Environment Variables": all listed `SYNOPKG_*` names present.
18. Guide "WIZARD_UIFILES [7.2.2]": file names `install_uifile`, `_<lang>` suffix, `.sh` variants, which scripts see which keys; `custom_render_fn` appears.
19. Guide "Privilege Config": `username`/`groupname` default to the package name, `chown -hR`.
20. Guide "Pack Stage": `pkg_get_unified_platform` is in the guide and in no file of the toolkit checkout.
21. ExamplePackages `INFO.sh:11` (`arch="$(pkg_get_platform)"`, `os_min_ver="7.0-40000"`) and `start-stop-status:25-29` (`log) exit 0`).
22. spksrc jellyfin Makefile: `SPK_REV = 43`, `ADMIN_PORT = $(SERVICE_PORT)`.
23. Synology release-note API: `7.2-64561` published 2023-05-22 and recalled, `7.2-64570` follows (+ Updates 1-4).
24. Container Manager "Applied Models": DS120j, DS124, DS220j, DS223, DS223j, DS420j, DS423 listed; DS416play, DS218j, DS216play absent; no 32-bit ARM model.
25. KB CPU table lists DS119j and DS218j.
26. Toolkit `PkgCreate.py`: `-I` = "Not install projects" (line 59-60), `-i` "Only install" (61); no `--no-sign` and no signing step.

Not re-verified (marked in the file as supporting or unconfirmed): spksrc templating, RAM per model, QuickConnect and reverse-proxy statements, patch levels of the kernels.

Consequence for the other tasks (information, not a defect of SPK-01): `INFO.sh` originally wrote
`adminurl="/"`, contradicting Impact 5; the implementer fixed it during this test (see SPK-04).

---

## SPK-02 – FFmpeg builds per arch: PASS (evaluation with a documented negative result)

The task's Notes allow this outcome ("if no prebuilt meets the criteria for a family ... say so in
the README and the REPORT, status DONE with the finding"). The README does so, and all three lock
entries are `interim`. What is and is not met:

| Criterion | x86_64 (jellyfin 8.1.3-1) | armv8 (jellyfin 8.1.3-1) | armv7 (johnvansickle 7.0.2) |
|---|---|---|---|
| README comparison, chosen build and reason | met | met | met |
| `sources.lock`: exactly 3 entries, documented format | met | met | met |
| `fetch.sh` works, shellcheck clean, negative cases | met | met | met |
| `-version` of `ffmpeg` and `ffprobe` exit 0 on the family's platform | met | met | met |
| ELF is the right machine | met (x86-64) | met (aarch64) | met (ARM EABI5 hard-float) |
| **statically linked**, ABI note <= oldest DSM kernel | **not met**: dynamic (6 NEEDED, interpreter, GLIBC_2.28), ABI 4.18.20 > 3.10 | **not met**: dynamic (5 NEEDED), ABI 4.18.20 > 4.4 | met: static, ABI 3.2.0 <= 3.10 |
| `libx264`, `aac`, muxer `hls`, `format_whitelist` | met | met | met |
| **FFmpeg >= 7.1** | met (8.1.3) | met (8.1.3) | **not met** (7.0.2) |
| Corresponding source: URL and SHA-256 in lock, HTTP 200 | met; tag archive holds FFmpeg tree, patches and build scripts (dependency sources not included, stated in README) | met, same archive | **not met in substance**: the lock pins only the upstream FFmpeg 7.0.2 tarball (200), no build scripts or library sources; the README says this rules the build out for a release |
| Gates | shellcheck clean, no Go code | | |

Additional finding the README records and I confirmed: the armv7 binary is compiled for VFPv4
(`readelf -A`: `Tag_FP_arch: VFPv4`, `NEONv1 with Fused-MAC`), which Cortex-A9 platforms
(armada38x, monaco) lack (DSM-NOTES §13). The interim armv7 build is for development only.
The jellyfin builds link `--enable-libfdk-aac` (`ffmpeg -version`), without `--enable-nonfree`; the
lock now marks them `GPL-3.0-or-later+nonfree` and `SOURCE.txt` states "not redistributable".

Evidence, final `fetch.sh` (SHA-256 `c1d5a644...`), `debian:stable-slim` (GNU) and `alpine:3`
(BusyBox `wget`, `sha256sum`, `tar`):

- Shellcheck: `koalaman/shellcheck:v0.11.0 -s sh packaging/synology/ffmpeg/fetch.sh` -> no output, exit 0.
- Positive fetch, all three families, both images: exit 0; binaries `-rwxr-xr-x`, other files
  `-rw-r--r--`; the Debian and Alpine outputs are byte-identical (`cmp`).
- `-version`, run with `--platform linux/amd64`, `linux/arm64`, `linux/arm/v7` (exit 0 for each):

| Family | `ffmpeg -version` | `ffprobe -version` |
|---|---|---|
| x86_64 | `ffmpeg version 8.1.3-Jellyfin Copyright (c) 2000-2026 the FFmpeg developers` | `ffprobe version 8.1.3-Jellyfin Copyright (c) 2007-2026 the FFmpeg developers` |
| armv8 | `ffmpeg version 8.1.3-Jellyfin Copyright (c) 2000-2026 the FFmpeg developers` | `ffprobe version 8.1.3-Jellyfin Copyright (c) 2007-2026 the FFmpeg developers` |
| armv7 | `ffmpeg version 7.0.2-static https://johnvansickle.com/ffmpeg/  Copyright (c) 2000-2024 the FFmpeg developers` | `ffprobe version 7.0.2-static https://johnvansickle.com/ffmpeg/  Copyright (c) 2007-2024 the FFmpeg developers` |

- `file -L`: x86_64 "ELF 64-bit LSB pie executable, x86-64, ... dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2, for GNU/Linux 4.18.20"; armv8 "ARM aarch64 ... dynamically linked ... 4.18.20"; armv7 "ELF 32-bit LSB executable, ARM, EABI5 ... statically linked, for GNU/Linux 3.2.0".
- Capabilities (all three): `libx264`, `aac` in `-encoders`; `hls` in `-muxers`; `format_whitelist` in `-h full`.
- Archive sizes match the README table (ffmpeg/ffprobe bytes 109525528/109230488, 79796424/79534280, 31793580/31671692).
- Lock URLs: all five return 200 or 206 on a ranged GET; the new versioned armv7 URL
  (`ffmpeg-7.0.2-armhf-static.tar.xz`) hashes to the pinned `7d41f558...`. (A plain HEAD on GitHub
  returns the 302 to the CDN; I followed it with a ranged GET.)
- Negative cases (exit code, dest content afterwards):

| Case | Exit | `<dest-dir>` afterwards |
|---|---|---|
| wrong archive checksum (lock copy), dest pre-filled with stale `ffmpeg`/`ffprobe` | 3 | empty (stale files removed) |
| wrong source checksum (x86_64, licence from source) | 3 | not created |
| unknown family `mips` | 2 | not created |
| archive directory wrong -> `ffmpeg`/`ffprobe` missing in a verified archive | 4 | not created |
| licence member missing in a verified archive, dest pre-filled | 4 | empty |
| no arguments | 2 | - |
| lock file missing | 2 | not created |
| duplicate lock entry | 2 | not created |
| `http://` URL in the lock | 2 | not created |
| licence path `../etc/passwd` | 2 | not created |

- Switching families into one directory (x86_64 then armv7): the x86_64 licence files are removed
  (`.fetch-manifest`); only the armv7 files stay.

Observation (not a failure): the README states the johnvansickle amd64 `ffprobe` crashes on the
corpus' MPEG-TS files. That candidate is not selected; I did not regenerate the corpus to re-check it.

Required follow-up (owner decision, already proposed in the README): own static FFmpeg build (SPK-15).
Until then `grep -q '^[^#][^ ]* interim ' sources.lock` must block a release.

---

## SPK-04 – Toolkit walking skeleton: PASS

| Criterion | Result | Evidence |
|---|---|---|
| Container definition and run script: pinned commit in a named volume, EnvDeploy only when not deployed, project copied to `source/<project>/`, `PkgCreate.py` for 7.2 unsigned, SPK copied out; one `docker run` per family from PowerShell | met | `bash test.sh` all green; the README quick start executed literally in PowerShell for all three families: exit 0, "environment ds.<platform>-7.2 deployed, EnvDeploy skipped", `wrote demo-<family>-1.2.3-1040.spk in 1-2s`. Checkout in the volume: `e4a8d54209a31cef21b7636ad58b1b0e86b5b201` "Release for DSM7.2.2" |
| Signing disabled | met | `PkgCreate.py` at the pinned commit has no signing step and no `--no-sign` (grep of `PkgCreate.py` and `include/`: no match for `no-sign`, `CodeSign`, `gpg`); `-I`/`-i` semantics as the README states (`PkgCreate.py:59-62`) |
| Privileges documented and tried | met | default capabilities: exit 1, `mount: /toolkit/build_env/ds.rtd1619b-7.2/proc: permission denied` (reproduced); `--cap-add SYS_ADMIN`: packs x86_64 and all families; `--privileged`: packs all three. README default is now `--cap-add SYS_ADMIN` with `--privileged` and an AppArmor fallback (unverified, stated as such) |
| Toolkit never in a Windows bind mount, explained | met | README "Why the toolkit never lives in a Windows bind mount"; volume used throughout |
| Platforms: one per family, README lists the `arch` value per candidate | met | `plat_to_family` read from the checkout: `alpine` -> `armv7`, `armada38x` -> `armada38x`, rtd1619b/armada37xx -> `armv8`, v1000/geminilake -> `x86_64`; chosen v1000, rtd1619b, alpine. INFO `arch` is set by `INFO.sh` from `build.env` `ARCH` (cross-checked against `pkg_get_platform_family`) |
| `build` compiles nothing, clear failure; `install` copies with modes and packs with helpers; `depends` minimal; `INFO.sh` from `build.env`, appends unsupported keys | met | `test.sh` build-stage cases: missing ffprobe, server not executable, wrong ELF machine, unknown family, no licences; modes in `package.tgz` asserted; `support_move="no"` appended; README lists the keys |
| Placeholder project -> three SPKs, `tar -tvf` shows INFO, package.tgz, scripts/, conf/, no signature; INFO has `package`, `version`, family `arch` | met | see "Unpacked SPKs" |
| Caching: 2nd run downloads nothing; README records sizes and times | met | `test.sh` second run with `--network none` passes for all families; README numbers re-checked: base 1,183,129,564 B = 1.10 GiB, v1000 platform 0.91 GiB, deployed 7.0 / 7.1 / 6.6 GB (`du -sxh`), tarballs 3.5G, a pack takes 1-2 s. First-run durations cannot be re-measured (cached) |
| Brand-free | met | `rg -i mivo packaging/synology/package packaging/synology/toolkit` -> no match (exit 1) |
| shellcheck on run script, `SynoBuildConf/*`, `INFO.sh` | met | `-s bash` on `run.sh test.sh stage-demo.sh build install INFO.sh` and `-s sh` on the seven fixture scripts: exit 0, no findings |
| Gates | met | no Go code; Docker commands succeed |

`bash packaging/synology/toolkit/test.sh` on the frozen files: **all toolkit tests passed**, 57 `ok`,
0 `FAIL` (counted from the run output). Besides the cases above it covers: INFO.sh rejection of `$`, backquote, backslash, `*`, `?`,
`[`, `"`, double space, tab, newline; PORT `0`, `65536`, `08460`, `84a`; DSM_VERSION and OS_MIN_VER
formats; foreign family; `armada38x` outside armv7; arch list `armv7 armada38x monaco`; no `beta` key
for a release; no `adminurl`; refusal of non-literal `build.env` lines; an escaped quote reaching INFO;
redeploy after a toolkit commit change; tarball hash mismatch refused and the file deleted.

Tarball pins: `sha256sum -c` of `toolkit/tarballs.sha256` against the seven files in the volume: all OK.

### Unpacked SPKs (final scripts; `--privileged` and `--cap-add SYS_ADMIN` both produce them)

| Check | x86_64 | armv8 | armv7 |
|---|---|---|---|
| SPK format | plain tar, members `INFO LICENSE conf/ package.tgz scripts/` (toolkit `ls` order) | same | same |
| `INFO` `package`, `version` | `demo`, `1.2.3-1040` | same | same |
| `INFO` `arch` | `x86_64` | `armv8` | `armv7` |
| Other INFO | `adminprotocol="http"`, `adminport="9999"`, `os_min_ver="7.2-64570"`, `beta="yes"`, `description_ger` (UTF-8 intact), `support_move="no"`, `extractsize`, `toolkit_version`, `create_time`; no `adminurl` | same | same |
| `package.tgz` | xz (`fd 37 7a 58 5a 00`, `xz -t` ok): `bin/demo`, `bin/ffmpeg`, `bin/ffprobe` `-rwxr-xr-x`; `share/licenses/ffmpeg/{SOURCE.txt,LICENSE.md}` `-rw-r--r--`; directories 0755 | same | same |
| `scripts/` | 7 files `-rwxr-xr-x`, `#!/bin/sh` | same | same |
| `conf/privilege` | `{"defaults": {"run-as": "package"}}` | same | same |
| signature member | none (0 matches for `sig`, `.asc`, `gpg`) | none | none |
| Payload ELF e_machine | 62 | 183 | 40 |

Observations (no criterion violated):

- The fixture SPK has no `PACKAGE_ICON.PNG`/`PACKAGE_ICON_256.PNG` (the build only logs "note: ... not staged"). DSM-NOTES §1 lists both as required members, so SPK-14 or SPK-07 should make a missing icon a hard error for the real package.
- `testdata/demo/scripts/start-stop-status` (stub) returns 1 for unknown arguments, including `prestart`/`prestop`. DSM-NOTES §5 and Impact 11 say those must exit 0. It is a stub; SPK-05 must not copy the pattern.
- `build.env` is data, not a shell script; `shellcheck` on it reports SC2148/SC2034, which is expected and not part of the gate.
- `toolkit_version` differs between environments (72723 for v1000, 72726 for the others); the README documents 72723 as an example only.

---

## SPK-08 – Package icons: PASS

| Criterion | Result | Evidence |
|---|---|---|
| Function renders a square PNG of a given size: coral rounded square on transparent, centred triangle `#3A0E07`, no text | met | `spkicon.Render`/`EncodePNG`; 256 px output viewed: right-pointing triangle centred in a coral rounded square, transparent corners, no text. Colours in the code equal `web/packages/ui-tokens/src/colors.ts` (`accent` `#FF6B57`, `onAccent` `#3A0E07`) and ADR 0011 |
| Named size constants (64 / 256), sizes outside 16-1024 are an error | met | `PackageIconSize = 64`, `PackageIcon256Size = 256`, `MinSize = 16`, `MaxSize = 1024`; the guide (DSM-NOTES §4) confirms 64 and 256; sizes 15 and 1025 return `ErrSizeOutOfRange`, 16 and 1024 render |
| Tests: dimensions, transparent corner, centre = triangle colour, coral pixel off the triangle, byte-identical renders | met | `spkicon_test.go`: dimensions at 16, 64, 100, 256, 1024; four corners `{0,0,0,0}`; centre `{58,14,7,255}`; coral at three positions; identical PNG bytes; mirror symmetry; anti-aliasing; range errors |
| Coverage >= 85 % | met | 96.8 % (60/62 statements); package gate `Package coverage threshold (85%) satisfied: PASS` |
| Gates | met | see below |

Independent rendering with a copy of the package outside the repository: `PACKAGE_ICON.PNG` 64 x 64
and `PACKAGE_ICON_256.PNG` 256 x 256, both "PNG image data, 8-bit/color RGBA, non-interlaced"
(732 and 2361 bytes); centre pixel `{58 14 7 255}`, left of the triangle `{255 107 87 255}`, corner
`{0 0 0 0}`; at 256 px: 17208 transparent, 48064 opaque, 264 partly transparent pixels; two runs give
identical SHA-256 (`f7eb74d1...`, `54d5f1bd...`).

Gates (scoped to `./packaging/synology/internal/spkicon`; the repository-wide `go vet ./...` is red
for the unrelated reason above):

- `go vet ./packaging/...` (at the time only spkicon existed): exit 0.
- `go test -race -count=1 -coverprofile=... ./packaging/...`: `ok ... 35.737s coverage: 96.8% of statements` (`-race` works here: cgo available).
- `golangci-lint run --allow-parallel-runners ./packaging/synology/internal/spkicon/...`: `0 issues.`
- `golangci-lint fmt --diff`: no diff, exit 0 (also repository-wide at that time).
- `go tool -modfile=tools/go.mod go-test-coverage` with a scratch config (`total: 70`, `package: 85`): total and package thresholds PASS. The repository's `.testcoverage.yml` was not changed.
- Layer rule: `go list -deps` of the package lists no `mivo` package besides itself and no `database/sql`, `net/http`, `os/exec` (standard library only).

Non-blocking: the package is the only Go code that defines the colours a second time (the web tokens
are the source of truth); the code comments say so and the tests spell the values out, so a drift fails.

---

## Cross-cutting checks

- Shellcheck v0.11.0, every shell script of the wave: `fetch.sh` (`-s sh`), `run.sh`, `test.sh`,
  `stage-demo.sh`, `SynoBuildConf/build`, `SynoBuildConf/install`, `INFO.sh` (`-s bash`), the seven
  fixture scripts (`-s sh`): no findings. `SynoBuildConf/depends` is INI, not shell.
- Brand: `rg -i mivo packaging/synology` outside `spkmeta`: only Go import paths
  `github.com/magpeek/mivo/...` in `spkicon_test.go`. No packaging script, README or fixture names the
  product (an earlier hit in `ffmpeg/README.md` was removed during the test).
- No TODO/FIXME in the wave's files.
- Repository left clean by this test: all work files are in the scratchpad; I added only this report.
  The Docker image `spk-toolkit:dev` and the volume `spk-toolkit` remain (reused cache, 24 GB). My
  scratch volume `spkw1-out` was removed. `git status` shows only the pre-existing untracked
  `packaging/synology/`.

## History

- Round 1 (2026-10-31): all four tasks PASS. First `test.sh` run invalid because the script was
  edited while it ran; repeated on a frozen copy.
