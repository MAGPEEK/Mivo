# SPK-10 test report

Round 1, status: PASS. Base 9ab6832, implementation uncommitted in the worktree.

## Gates

| Gate | Result |
|---|---|
| `shellcheck -s sh inspect-spk.sh` (koalaman/shellcheck:v0.11.0) | no findings |
| shellcheck on `inspect-spk.bats`, `fixture.bash` | no findings |
| hadolint on `test/inspect/Dockerfile` | no findings |
| `docker build -t spk-inspect:dev packaging/synology/test/inspect` | ok |
| `bats packaging/synology/test/inspect` in the image | `1..34`, all 34 ok |
| `go vet ./...` | ok |
| `go test ./packaging/synology/...` | ok (spkstage, spkicon, spkmeta) |
| `golangci-lint run` | 0 issues |
| `git status` | only `toolkit/README.md` modified and `test/inspect/` new; no product code changed |

## Real SPKs

Built with `SPK_TOOLKIT_VOLUME=spk-toolkit-qa10 bash packaging/synology/build.sh --test 0.1.0-alpha.2`. The volume was a
copy of `spk-toolkit` (the source mounted `:ro`). The build ran twice (about 55 s each) and the volume was removed
afterwards; `dist/` was removed as well.

| SPK | test mode | `--release` |
|---|---|---|
| x86_64 | rc 0, `69 checks, 0 failed, 6 warnings` | rc 1, `6 failed, 0 warnings` |
| armv8 | rc 0, `69 checks, 0 failed, 6 warnings` | rc 1, `6 failed, 0 warnings` |
| armv7 | rc 0, `69 checks, 0 failed, 1 warnings` | rc 1, `1 failed, 0 warnings` |

- The x86_64 and armv8 warnings (and release failures) are: ffmpeg dynamically linked, ffprobe dynamically linked, ABI
  note 4.18.20 above the kernel for both (3.10.108 / 4.4.302), status interim, licence `GPL-3.0-or-later+nonfree`.
- armv7: only `SOURCE.txt status is interim`.
- All are interim-FFmpeg findings. No other failure occurred. This matches the numbers in the README.

## Tar flavour

Both archives are `format gnu` (`SPK is an uncompressed tar (format gnu)`, `package.tgz holds a tar (format gnu)`),
with `tar extensions beyond plain ustar: none` for both, in all three SPKs. Owners are `root/root`. The member order is
recorded in the output (top level `INFO LICENSE PACKAGE_ICON.PNG PACKAGE_ICON_256.PNG WIZARD_UIFILES/ conf/ package.tgz
scripts/`, unsorted inside directories) and agrees with the README.

## Negative tests

I ran these on mutated copies of the x86_64 SPK (repacked in the container). A control repack without changes gave rc 0.

| Mutation | rc | FAIL line |
|---|---|---|
| removed `bin/ffmpeg` | 1 | `package: members differ ... bin/ffmpeg(expected once, found 0)`; `bin/ffmpeg is not a little-endian ELF file` |
| wrong arch (family armv8 on the x86_64 SPK) | 1 | `INFO arch="x86_64" does not match ^armv8$`; `bin/mivo: ELF class 2, machine 62, want 2 183` |
| `arch="armv8"` in INFO | 1 | `INFO arch="armv8" does not match ^x86_64$` |
| `adminurl="/"` | 1 | `INFO has adminurl (adminurl="/")` |
| leftover `[@ID@]` in `port_conf/mivo.sc` | 1 | `unreplaced @NAME@ tokens in: package.tgz:port_conf/mivo.sc` |
| outer SPK gzip | 1 | `SPK is gzip-compressed, not an uncompressed tar` |
| `package.tgz` gzip instead of xz | 1 | `package.tgz is not xz-compressed (magic 1f8b08000000)` |
| `build.env` in package.tgz | 1 | `build.env(build leftover or unsafe name)` |
| `scripts/postinst` mode 0644 | 1 | `scripts/postinst(mode -rw-r--r--, want 0755)` |
| version `0.1.0-4294967296` | 1 | `INFO version ... has a part above 2147483647` |
| dynamic ffmpeg with `Status: selected` | 1 | `bin/ffmpeg: dynamically linked`; ABI note 4.18.20 above 3.10.108 |

The first five cover the five mutations the task names. The dynamic-ffmpeg case only fails when the status is not
interim; with `interim` it is a warning, which is the interim rule of the task.

## Reproducibility (`--members`, two builds)

For all three families the diff of the two `--members` outputs showed only the inspected path (summary and info line),
`INFO create_time` and the SHA-256 of the INFO line. The SPK files differ byte-wise (`cmp` differs at byte 141, the tar
mtime). All other member digests and modes are identical. Quoted `pkg_util.sh` lines match the README: 215 `tar_option="cJf"`,
228 `du -sk`, 307 `extractsize`, 310 `toolkit_version`, 311 `create_time=$(date +%Y%m%d-%T)`, 317 `tar cf ... $(ls ...)`.

## POSIX sh and tools

`sh` in `debian:stable-slim` is dash. Both `dash inspect-spk.sh` and `busybox sh inspect-spk.sh` (BusyBox 1.37) gave the
same result on the armv7 SPK (`69 checks, 0 failed, 1 warnings`). `checkbashisms` flags one line (640), a false positive:
the `[.. | strings ...]` is a jq expression inside single quotes.

## Acceptance criteria

| Criterion | Result |
|---|---|
| Script checks the listed items, exit 1 on failure | met (34 bats cases, 11 mutations above) |
| Passes on the three SPK-07 SPKs (warnings only), `--release` fails as expected | met (table above) |
| Tar flavour recorded in the README, from the headers | met (GNU, no extensions; the README quotes GNU tar 1.32, taken from the implementer's earlier toolchain run, not re-run here) |
| Reproducibility recorded, with `pkg_util.sh` line numbers | met (checked above) |
| Negative tests fail as required | met |
| shellcheck on the script, no findings | met |
| No product code changed, Go gates unaffected | met |

## Findings (non-blocking)

- The header of `inspect-spk.sh` and the README list the required tools as tar, xz, od, dd, awk, sed, grep, sort, mktemp,
  sha256sum and jq. The script also calls find, du, cut, wc, tr, head, stat, rm and mkdir (all in coreutils/findutils,
  present in `debian:stable-slim`, not checked by `require_tools`). The list could be completed.
- In test mode a dynamically linked ffmpeg is only a warning while the status is interim. That follows the task, but
  SPK-11 to SPK-13 should keep using `--release` for anything published.

## History

- Round 1 (2026-10-31 and 2026-11-04, after the usage limit reset): PASS.
