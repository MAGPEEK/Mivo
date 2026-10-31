# SPK-15 Own static FFmpeg build for x86_64, armv8 and armv7
Owner: mivo-media
Depends on: none (uses the findings of SPK-01/02; can start now)
Files/areas: `packaging/synology/ffmpeg/build/**` (new: Dockerfile recipe, pinned source list with
checksums, helper scripts, `README.md`), `packaging/synology/ffmpeg/README.md` (new section "Own
build"; the candidate tables stay; correct the unsourced NEON claim in "DSM 7.2 kernels and CPUs
per family")

## Goal
The server SPKs can bundle an FFmpeg and ffprobe that meet every criterion of ADR 0026 §4 (as
revised by the owner on 2026-10-31, ADR 0026 Notes): fully static, runnable on DSM 7.2's oldest
kernels, GPLv3 without non-free parts, with the exact corresponding source of everything linked.

## Acceptance criteria
- [ ] One Docker recipe builds `ffmpeg` and `ffprobe` for the three families, e.g.
      `docker build -f packaging/synology/ffmpeg/build/Dockerfile --target <family> --output type=local,dest=<dir> .`
      (or one target for all), runnable on Docker Desktop (Windows). Output per family:
      `ffmpeg`, `ffprobe`, the licence files and a `SOURCE.txt` (versions, URLs, SHA-256 of every
      source and of both binaries, configure line, status `selected`).
- [ ] Fully static: `readelf -l` shows no `PT_INTERP`, `readelf -d` no `NEEDED`; built against musl,
      or against glibc with `--enable-kernel` ≤ 3.10. The ELF ABI note (if present) is ≤ 3.10.108
      for x86_64 and armv7 and ≤ 4.4.302 for armv8; values recorded in the README.
- [ ] FFmpeg ≥ 7.1 and every library (x264, the C library if built, others) pinned by version
      and SHA-256 in one source list; a changed checksum fails the build.
- [ ] Licence: `--enable-gpl --enable-version3 --enable-libx264`, native AAC encoder, **no
      `libfdk-aac`, no `--enable-nonfree`**; `ffmpeg -hide_banner -L` / `-version` show GPLv3 and
      the configure line contains neither.
- [ ] Baselines: x86-64 v1 (no `-march` above it), armv8-a, armv7-a hard-float with
      `-march=armv7-a -mfpu=vfpv3-d16 -mfloat-abi=hard`; NEON code only behind FFmpeg's and x264's
      run-time CPU detection (NEON on armada38x/monaco is **unconfirmed**, ADR 0026 Notes – assume
      it is absent); no VFPv4 instructions in the armv7 binaries (e.g. `objdump -d | grep -c vfma`
      → 0, recorded).
- [ ] `packaging/synology/ffmpeg/README.md`: the sentence "All DSM 7.2 armv7 platforms have NEON"
      and the NEON entries of the armv7 row are replaced by the ADR 0026 Notes statement (armada38x
      built by Synology with `-mfpu=vfpv3`, NEON unconfirmed), or a cited vendor datasheet for
      Armada 385/388 and STiH412 is added. Wording proposed for ADR 0026 goes into the REPORT.
- [ ] Features: every demuxer of the server's `-format_whitelist`
      (`internal/adapters/ffmpeg/probe.go`) and all formats the producer uses are present;
      `-muxers` lists `hls` (fMP4 segments work); `-encoders` lists `libx264` and `aac`;
      `-h full` of both tools lists `format_whitelist`. A reduced configure (only what the server
      needs) is welcome; the README lists what was enabled.
- [ ] Verification per family under emulation as in SPK-02: glibc-2.36 userland
      (`debian:bookworm-slim`), the old kernel's syscall surface (`kernelsim` with the 3.10 or 4.4
      list, `uname` reporting the DSM kernel) and baseline CPU models (x86_64 `Opteron_G1`, armv8
      `cortex-a53`, armv7 `cortex-a9` **and** `cortex-a15`): `-version`, capability checks, all
      files of `testdata/media` probed with the server's arguments (48/48 or the current corpus
      count), the hostile ffconcat list refused, a 3 s HLS transcode of `hevc_main10_ac3.mkv` with
      the producer's options producing h264 + aac fMP4. Results table in the README.
- [ ] Corresponding source: the recipe also produces **one source archive** with the exact sources
      of FFmpeg and every linked library plus the recipe and patches (if any), with its SHA-256,
      ready to attach to a GitHub Release (GPL-3.0 §6).
- [ ] Reproducibility and cost: two builds from a clean cache give binaries with identical SHA-256
      (or the README explains which input differs); README records the cold build time per family,
      the warm (cached) time and the binary sizes.
- [ ] Gates: shellcheck on helper scripts → no findings; hadolint (if available) on the Dockerfile;
      no Go code; the Docker builds above succeed.

## Notes
- Owner of this task is `mivo-media` (FFmpeg configure and the server's format whitelist); the
  integration into the SPK build is SPK-16 (`mivo-synology`).
- Cross-compile on the build machine (`$BUILDPLATFORM`) with musl cross toolchains or
  equivalents instead of building under QEMU; pin toolchain images or archives by checksum.
- Keep the recipe independent of the Synology packaging, so the Docker image and the Windows
  installer can reuse it later (ADR 0015 Notes).
- Network: source tarballs, toolchains, base images (approved downloads).
- Do not edit `sources.lock` or `fetch.sh` (SPK-16), ADR 0015 or ADR 0026; report findings.
