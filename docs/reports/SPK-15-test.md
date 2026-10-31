# SPK-15 test report: own static FFmpeg build

Round 1, 2026-10-31. Status: **PASS**.

Tested in a clean worktree at HEAD (531f20b). `packaging/synology/ffmpeg/build/**` and `packaging/synology/ffmpeg/README.md` were copied in. The corpus `testdata/media` was copied (48 files). Docker Desktop 29.9.2, 20 CPUs. The worktree has been removed and pruned. The BuildKit cache was kept.

## Gates

| Gate | Result |
|---|---|
| `docker build` (target `all`), warm cache | exit 0, 20 s including export |
| `docker build --no-cache` (target `all`), cold layers, downloads in the cache mount | exit 0, **850 s** (about 14 min; implementer: 822 s and 620 s) |
| shellcheck v0.11.0 `-x scripts/*.sh verify/*.sh` | exit 0, no findings |
| hadolint (`Dockerfile`, `verify/Dockerfile`) | exit 0, no findings |
| `verify.sh x86_64` (Opteron_G1) | 31/31 |
| `verify.sh armv8` (cortex-a53) | 31/31 |
| `verify.sh armv7` (cortex-a9, cortex-a15, `cortex-a9,neon=off,vfp-d32=off`) | 31/31 for each of the three |
| Go gates, web gates | not applicable (no Go or web code in this task) |

The verify runs used the binaries of the `--no-cache` build.

Details from the verify logs:
- Every run reports `uname` 3.10.108 (armv8: 4.4.302).
- The syscall filter shows `getrandom=ENOSYS statx=ENOSYS`.
- The corpus probe is 48/48 and the format names match 48/48.
- The hostile `evil.mkv` and `evil.m3u8` are refused, by path and through fd 3.
- The HLS transcode gives h264 High yuv420p plus aac stereo.
- The server's `go test ./internal/adapters/ffmpeg` passes in every run.
- Hwcap of the third armv7 model is `swp half thumb fastmult vfp edsp thumbee vfpv3 vfpv3d16 tls`, with no `neon`.

## Reproducibility

The `--no-cache` build was compared byte for byte (`cmp`) with the implementer's `outB`. All six binaries (3 families x ffmpeg and ffprobe) are identical, and they equal the warm build and the README table. SHA-256, same in all three places:

| Family | ffmpeg | ffprobe |
|---|---|---|
| x86_64 | `3574d337...eca768` | `db96c3fe...7307b7` |
| armv8 | `bb4b8a6c...cacf2f` | `a7603e1d...c47e67` |
| armv7 | `0dc27fb1...a0a0` | `42ab9706...286286` |

The source archive of the cold build is identical (`cmp`) to the warm build: `dde5753ee547cb105edce8a480ece6b5803cf56a05922011b5bd341470046c12`. This equals the `Source SHA-256` in `SOURCE.txt`.

It differs from the archive in `outB` (`96e6dd4a...`). The only difference is `recipe/README.md`, which was edited after `outB` was built. This is expected, and the README says so ("Changing README.md ... changes the archive, but not the binaries").

Archive contents: 17 source files, all members owner 0/0, modes 0644/0755/0755 for directories, mtime 2026-10-31 01:00.

## Tamper tests (copy of the recipe, build stopped as claimed)

| Test | Result |
|---|---|
| One character of the zlib SHA-256 changed in `sources.list` (`--target download`) | `checksum mismatch for zlib`, **exit code 3** |
| Extra file `libs/unlisted.txt` in a replaced `sources` context (`--build-context sources=...`, `--target verified-sources`) | `files not in the list: libs/unlisted.txt`, **exit code 4** |
| Unpacked source archive unchanged | `verified-sources` passes, exit 0 |
| Extra byte appended to the zlib tarball in the replaced `sources` context | `checksum mismatch for libs/zlib-1.3.2.tar.xz`, exit code 3 (additional case) |

## Static, licence and CPU checks (independent, on the cold-build binaries)

- `readelf -l`: no `PT_INTERP` in any of the 6 binaries. `readelf -d`: no `NEEDED`. The ELF type is EXEC (non-PIE).
- ELF ABI note (`NT_GNU_ABI_TAG`): absent in all six. x86_64 and armv8 carry only `.note.gnu.property` with a stack size of 0x200000; armv7 has no notes. `GNU_STACK` size is 0x200000 as documented.
- Licence: `SOURCE.txt` says `GPL-3.0-or-later`, status `selected`. The configure line has `--enable-gpl --enable-version3`. grep finds no `nonfree` or `fdk` in the configure lines. The `-L` and `-version` output was checked by `checks.sh` in all 5 runs (PASS).
- armv7 VFPv4: `objdump -d | grep -cE 'v(fma|fms|fnma|fnms)\.'` gives **0** for ffmpeg and ffprobe, and `grep -c vfma` also gives 0. The NEON code is present only behind run-time detection: `vld1` appears 10542 times per binary, matching the README.
- The 3-CPU armv7 emulation (A9, A15, A9 without NEON and D32) passes all checks. Hwcap without `neon` is shown above.

### Deviation: armv7 `readelf -A` shows VFPv3 and NEONv1, not VFPv3-D16

`readelf -A armv7/ffmpeg` reports `CPU_arch: ARMv7`, `FP_arch: VFPv3`, `Advanced_SIMD_arch: NEONv1`, `ABI_VFP_args: AAPCS VFP`. The AC says "VFPv3-D16".

Judgement: the explanation in `build/README.md` holds, and I consider the AC met in substance.
- The attributes in a linked ELF are the maximum over all input objects. The NEON assembly objects (FFmpeg, x264 and dav1d, assembled with `.fpu neon`) imply VFPv3 with 32 registers and NEONv1, so they raise the merged tags.
- The README states that configure rejects compiler NEON, that every C object is compiled with `-mfpu=vfpv3-d16`, and that the toolchain default is `--with-fpu=vfpv3-d16`. The NEON assembly is selected only by `AT_HWCAP` or a SIGILL-guarded probe.
- The decisive behavioural evidence is that the whole check set (31 checks: corpus, hostile inputs, HLS transcode with x264 and aac, and the Go adapter tests) passes under `cortex-a9,neon=off,vfp-d32=off`. There QEMU makes NEON and D16-D31 undefined, so a stray use on a path that was exercised would have crashed. VFPv4 is statically 0.

Limits of this evidence:
- The binaries are stripped, so I could not map D16-D31 and Q8-Q15 use to NEON-only functions statically. Disassembly shows such registers almost only in NEON mnemonics (`vmlal`, `vld1`, `vext`, ...) plus `vldm`/`vstm` spills, which fits NEON assembly.
- Paths not exercised by the corpus are not proven. The device test (SPK-12) settles that.
- The tag cannot honestly be "VFPv3-D16" with the NEON objects linked in. The main README table row says "armv7-a, VFPv3-D16, hard-float; NEON only at run time", which describes the compile baseline and is correct. The deviation is documented only in `build/README.md`. Suggestion: add one sentence with the `readelf -A` result to the "Own build" section of `packaging/synology/ffmpeg/README.md`. This is not a blocker.

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | One recipe, 3 families, output with licences and `SOURCE.txt`, runs on Docker Desktop | met | the `all` build gives `x86_64/ armv8/ armv7/ source/`. Each family has ffmpeg, ffprobe, COPYING.GPLv3, LICENSE.md, x264, dav1d, zlib, musl and gcc licence files, and `SOURCE.txt` (versions, URLs, SHA-256 of every source and of both binaries, configure line, `Status: selected`). |
| 2 | Fully static, no PT_INTERP or NEEDED, ABI note within limits, values recorded in the README | met | see static checks above. The README table "Static linking and kernels" records "none" for all families. musl 1.2.6. |
| 3 | FFmpeg >= 7.1, all libraries pinned by version and SHA-256 in one list, changed checksum fails | met | FFmpeg 9.0.2. `sources.list` has 13 pinned entries. The tamper tests give exit 3 and exit 4. |
| 4 | GPLv3, libx264, native AAC, no fdk, no nonfree | met | configure line, `-L` and `-version` checks pass in all runs. |
| 5 | Baselines: x86-64 v1, armv8-a, armv7-a vfpv3-d16 hard-float, NEON only behind detection, 0 vfma recorded | met (with the `readelf -A` deviation above) | `-march=x86-64`, `-march=armv8-a` and `-march=armv7-a -mfpu=vfpv3-d16 -mfloat-abi=hard` are in the configure lines. VFPv4 count is 0. The 3rd armv7 CPU model passes everything. |
| 6 | Main README: NEON claim replaced by the ADR 0026 Notes statement | met | "All DSM 7.2 armv7 platforms have NEON" occurs 0 times. The armv7 rows say "NEON *unconfirmed*" for armada38x (`-mfpu=vfpv3`) and the line about run-time detection. Wording for ADR 0026: the implementer must provide it in their REPORT; ADR 0026 already contains the withdrawal at lines 422-426. |
| 7 | Features: server's whitelist demuxers, hls muxer, libx264 and aac, `format_whitelist` in `-h full` | met | `checks.sh` capability checks pass in all 5 runs. It reads `probe.go` and fails on a missing demuxer. The README lists what was enabled. |
| 8 | Verification per family under emulation, results table in the README | met | 31/31 for all 5 combinations, 48/48 corpus, hostile refused, HLS transcode h264 and aac fMP4. Results table in `build/README.md`. |
| 9 | One source archive with the exact sources, recipe and SHA-256 | met | `ffmpeg-9.0.2-static1-source.tar` with 17 source files plus `recipe/` and `README.txt`. `.sha256` is correct and matches `SOURCE.txt`. The archive is reproducible, because warm and cold builds are byte-identical. |
| 10 | Reproducibility and cost recorded | met | two independent clean builds (mine and the implementer's) are byte-identical. Cold time 850 s measured here, in line with the README (620-822 s). Sizes and warm time are in the README. The Debian build tools are not pinned (apt in the `host` stage); this is documented, and the output was identical. |
| 11 | Gates: shellcheck clean, hadolint clean, Docker builds succeed | met | see the gates table. |

## Findings (non-blocking)

- `packaging/synology/ffmpeg/README.md`, "Own build" table, armv7 row: it says "VFPv3-D16" for the baseline. Add a pointer that `readelf -A` shows the merged VFPv3 and NEONv1 attributes (see `build/README.md`, "Baselines"). Optional.
- The cold-build time varies with machine load: 850 s here, against 620 and 822 s in the README. The README ranges are honest; consider "10-14 min".
- The Debian build tools are not pinned. This is documented and the output was identical in both builds. Pinning the apt snapshot would make the input list complete.
- HEAD moved from 531f20b to abb3f8d during the run (a commit made elsewhere). This did not affect the files under test.

## Cleanup

- The worktree was removed and `git worktree prune` run (no `\\?\` needed).
- Image `hadolint/hadolint:latest`, which I pulled, was removed. No containers of mine remain. The BuildKit cache was kept.
- Outputs (`outNC`, `outQA`, logs) remain in the session scratchpad.

## History

- Round 1: PASS.
