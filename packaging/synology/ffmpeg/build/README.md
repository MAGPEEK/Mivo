# Static FFmpeg build (x86_64, armv8, armv7)

One Docker recipe that builds fully static `ffmpeg` and `ffprobe` for the three DSM architecture
families, plus the corresponding source archive (task SPK-15; owner decision of 2026-10-31 in
ADR 0026 Notes). The server runs these binaries as child processes (ADR 0015). The recipe does
not depend on the Synology packaging around it: its build context is this directory only, so the
Docker image or the Windows installer can reuse it later (ADR 0015 Notes).

| File | Purpose |
|---|---|
| `Dockerfile` | the recipe: download and verify, toolchains, libraries, FFmpeg, checks, source archive |
| `Dockerfile.dockerignore` | allow list of the build context (everything in it enters the source archive) |
| `sources.list` | every input, pinned by version and SHA-256 (format in its header) |
| `scripts/` | the build steps (POSIX `sh`), one per stage |
| `verify/` | emulation checks: `verify.sh` (host driver), `checks.sh` (in the container), `kernelsim.c`, `sysprobe.c`, syscall lists, `Dockerfile` for these tools |

## Building

From the repository root, on Docker Desktop (Windows) or any Docker with BuildKit:

```sh
docker build --target armv7  --output type=local,dest=out/armv7  packaging/synology/ffmpeg/build
docker build --target source --output type=local,dest=out/source packaging/synology/ffmpeg/build
docker build --output type=local,dest=out packaging/synology/ffmpeg/build   # target "all" (default)
```

| Target | Output |
|---|---|
| `x86_64`, `armv8`, `armv7` | `ffmpeg`, `ffprobe` (0755), licence texts, `SOURCE.txt` |
| `source` | `ffmpeg-<version>-source.tar` and its `.sha256` |
| `all` | `x86_64/`, `armv8/`, `armv7/`, `source/` |

Build argument `JOBS` limits the parallel compile jobs per stage (default: all CPUs). Keep build
output outside the repository or in an ignored directory; never commit the binaries.

Per family the output holds `ffmpeg`, `ffprobe`, FFmpeg's `COPYING.GPLv3` and `LICENSE.md`,
`x264-COPYING`, `dav1d-COPYING`, `zlib-LICENSE`, `musl-COPYRIGHT`, `gcc-COPYING.RUNTIME` and
`gcc-COPYING3` (libgcc), and `SOURCE.txt`. `SOURCE.txt` has the same head as the one
`../fetch.sh` writes (`Version:`, `Status: selected`, `Licence:`, `ffmpeg SHA-256:`, `ffprobe
SHA-256:`, `Corresponding source:`, `Source SHA-256:`), followed by the configure line, every
pinned source (version, linked or tool, SHA-256, URL), the build checks and the build host.

### How it builds

Everything runs on the build machine (`$BUILDPLATFORM`) and cross-compiles; nothing runs under
emulation.

1. `download`: `scripts/fetch-sources.sh download` fetches every entry of `sources.list` over
   HTTPS and refuses a file whose SHA-256 differs (exit 3). A BuildKit cache mount keeps the files
   by checksum, so a changed pin downloads only that file.
2. `verified-sources`: whatever the `sources` stage provides is verified again and must match the
   list exactly (no missing, no unlisted file); `versions.env` per stage hands the versions to the
   scripts. Replacing `sources` with an unpacked source archive builds without downloads (below).
3. `tools`: NASM from source (the x86 assembler of FFmpeg, x264 and dav1d), so it is pinned too.
4. `toolchain-<family>`: musl-cross-make with the pinned gcc, binutils, musl, gmp, mpc and mpfr;
   musl-cross-make's patches (including the musl CVE patches of 2026) come with its pinned
   tarball. The family baseline is the compiler default (`--with-arch` etc.), so musl and libgcc
   are built for it as well.
5. `libs-<family>`: static zlib, x264 and dav1d.
6. `ffmpeg-<family>`: FFmpeg configured as below; the script refuses a configuration with
   `nonfree` or `fdk` or a licence other than "GPL version 3 or later".
7. `package-<family>`: refuses binaries with a `PT_INTERP`, a `NEEDED` entry, an ELF ABI note
   above the family's oldest DSM 7.2 kernel or (armv7) any VFPv4 instruction; writes the output.
8. `source-archive`: the corresponding source (below).

### Cache strategy

Each stage copies only what it needs, and BuildKit keys `COPY` by content:

- the toolchain stages see only the toolchain tarballs and `families.sh`, `layout.sh`,
  `build-toolchain.sh`, so a new FFmpeg, x264, dav1d or zlib pin, or a configure change, reuses
  the toolchains (the expensive part);
- the library stages see only the library tarballs and their scripts; `recipe.sh` (revision) is
  read only by the FFmpeg, package and source stages;
- downloads live in the cache mount `ffmpeg-static-downloads`, independent of the layer cache.

Build times and sizes: see "Measurements".

## Configuration

`--enable-gpl --enable-version3 --enable-libx264` with FFmpeg's native AAC encoder; no
`--enable-nonfree`, no `libfdk-aac` (ADR 0015 Notes). `ffmpeg -L` prints the GPL version 3
notice, `-version` prints the configure line. Reduced to what the server needs
(`scripts/build-ffmpeg.sh`):

| Component | Enabled | Why |
|---|---|---|
| Demuxers | exactly the server's `-format_whitelist` (`allowedDemuxers` in `internal/adapters/ffmpeg/probe.go`) plus `image2`: matroska, mov, mpegts, mpegps ("mpeg"), mpegvideo, avi, flv, live_flv, ogg, asf, mxf, rm, flac, mp3, wav, aiff, wv, ape, dsf, iff, aac, ac3, eac3, dts, gif, image_{png,jpeg,tiff,webp}_pipe, srt, webvtt, ass, sup, image2 | ADR 0022 Notes M2-24b. `concat`, `hls`, `dash`, `imf` and `vobsub` are not built at all, so a hostile list cannot be followed even without the whitelist. **When `allowedDemuxers` changes, change this list** (`verify.sh` reads probe.go and fails on a missing demuxer). |
| Decoders, parsers, bitstream filters | all (no external libraries besides those below) | the server plays any format; transcode needs the decoder of whatever is in a library |
| Muxers | hls, mp4, null (hls selects mov, mpegts, webvtt) | fMP4 HLS of ADR 0024; `null` for diagnostics |
| Encoders | libx264, aac | software transcode targets of ADR 0023 |
| Filters | buffer, buffersink, abuffer, abuffersink, format, aformat, null, anull, scale, aresample, trim, atrim, crop, hflip, vflip, transpose, rotate, setpts, asetpts, fps, pad, setsar, apad, volume | graph endpoints, what `ffmpeg` inserts itself (format conversion, `-t`/`-ss`, display-matrix rotation, crop metadata, `-r`, `-apad`, `-vol`) and the producer's `scale` |
| Protocols | file, pipe; `--disable-network` | the server passes `-protocol_whitelist file` |
| Libraries | zlib 1.3.2 (PNG decoder, compressed Matroska/MP4 headers), x264 (8-bit 4:2:0 only, the ADR 0023 target), dav1d 1.5.4 (FFmpeg's own AV1 decoder needs a hardware decoder), iconv from musl | `--disable-autodetect`: nothing else is picked up |
| Off | ffplay, avdevice, docs, debug symbols, every hardware API | no hardware acceleration on DSM (ADR 0026 §4) |

Milestone-4 features (subtitle burn-in, tone mapping, trickplay) will need more filters or
encoders; they change this list and `RECIPE_REVISION` in `scripts/recipe.sh`.

Hardening and run time: `-fstack-protector-strong` for all C code (it parses untrusted files);
static, non-PIE executables. musl's default thread stack is 128 KiB, so the executables carry a
2 MiB `PT_GNU_STACK` size, which musl uses as the default thread stack (FFmpeg, x264 and dav1d
run deep call chains on worker threads). Static PIE (ASLR) was left out because QEMU user mode
loads the binary itself, so the emulation cannot show whether a 3.10 kernel loads a static PIE
correctly; revisit after the device test (SPK-12).

## Static linking and kernels

Built against **musl 1.2.6**, not glibc: musl has no "kernel too old" check and writes no
`NT_GNU_ABI_TAG` note, and it falls back to older syscalls where newer ones are missing (for
example `statx` to `fstatat`, the 64-bit time syscalls to the 32-bit ones on armv7).

| Family | Toolchain target | `PT_INTERP` | `NEEDED` | ELF ABI note | Limit (oldest DSM 7.2 kernel) |
|---|---|---|---|---|---|
| x86_64 | `x86_64-linux-musl` | none | none (no dynamic section) | none | 3.10.108 |
| armv8 | `aarch64-linux-musl` | none | none | none | 4.4.302 |
| armv7 | `arm-linux-musleabihf` | none | none | none | 3.10.108 |

`package.sh` checks this for every build; the values of a build are in its `SOURCE.txt`.

## Baselines

| Family | Compile baseline | Optional instruction sets |
|---|---|---|
| x86_64 | `-march=x86-64 -mtune=generic` (x86-64 v1: SSE2) | SSE3 … AVX-512 only through FFmpeg's, x264's and dav1d's run-time CPU detection |
| armv8 | `-march=armv8-a` (NEON is part of ARMv8-A) | dot product, i8mm, SVE only through run-time detection (`getauxval`) |
| armv7 | `-march=armv7-a -mfpu=vfpv3-d16 -mfloat-abi=hard -marm` | NEON only through run-time detection |

armv7 in detail (ADR 0026 Notes: assume no NEON on armada38x):

- The compiler, musl and libgcc never emit NEON or VFPv4: the toolchain is configured with
  `--with-fpu=vfpv3-d16`, and every library and FFmpeg get `-mfpu=vfpv3-d16`.
- `readelf -A` of the binaries reports `Tag_FP_arch: VFPv3` and `Tag_Advanced_SIMD_arch:
  NEONv1`. These build attributes are the maximum the linker merges from all objects; the NEON
  assembly files (assembled with `.fpu neon`, selected only at run time) raise them. They are
  informational: the kernel ignores them for static executables, and they say nothing about which
  code runs. What runs is checked by the emulation on a CPU without NEON and D32 (below).
- **FFmpeg**: configure finds that the compiler does not accept NEON (`neon_inline` and
  `neon_external` off), so NEON is no core CPU flag; its NEON assembly is still built (with a
  `.fpu neon` directive) and selected only when `AT_HWCAP` has `neon` (`libavutil/arm/cpu.c`).
- **x264**: without compiler NEON it builds its NEON assembly and tests for NEON at start with a
  guarded instruction (SIGILL handler, `common/cpu.c`).
- **dav1d**: NEON assembly selected by `AT_HWCAP` (`src/arm/cpu.c`).
- No VFPv4 at all: `objdump -d | grep -cE 'v(fma|fms|fnma|fnms)\.'` is **0** for both binaries
  (`package.sh` fails the build otherwise). The NEON code behind detection is visible as `vld1`
  instructions (counts in `SOURCE.txt`).
- The emulation runs on a Cortex-A9 model **without NEON and with 16 double registers**
  (`QEMU_CPU=cortex-a9,neon=off,vfp-d32=off`, `AT_HWCAP: … vfpv3 vfpv3d16 tls`, no `neon`), the
  Armada 385/388 worst case, in addition to `cortex-a9` and `cortex-a15`. QEMU makes NEON and
  D16–D31 instructions undefined there, so any NEON or D32 use outside detection would crash.

## Corresponding source

The `source` target writes `ffmpeg-<version>-static<revision>-source.tar` and its `.sha256`, to
attach to every GitHub Release that carries the binaries (GPL-3.0 section 6). It contains:

- `sources/<stage>/`: every file of `sources.list`, byte-identical to the pinned downloads:
  FFmpeg, x264, dav1d, zlib, musl and gcc (libgcc is linked), the musl-cross-make tarball with
  its patches, binutils, gmp, mpc, mpfr and NASM;
- `recipe/`: this directory (Dockerfile, list, scripts, verification tools, this README);
- `README.txt`: how to rebuild.

The archive is reproducible (sorted members, owner 0, modes 0644/0755, mtime 2026-10-31 from
`scripts/recipe.sh`) and not compressed (the tarballs inside already are). Rebuild from it
without downloading the sources (the base image and Debian's build tools still come from the
network):

```sh
tar -xf ffmpeg-9.0.2-static1-source.tar && cd ffmpeg-9.0.2-static1-source
docker build --build-context sources=sources --target all --output type=local,dest=out recipe
```

Changing `README.md`, a script or the list changes the archive and therefore the `Source SHA-256`
in `SOURCE.txt`, but not the binaries.

## Verification under emulation

`verify/verify.sh <family> <binaries-dir> [cpu-model ...]` runs, per CPU model, a
`debian:bookworm-slim` container of the family's platform (glibc 2.36, as the DSM 7.2
toolchains) under these conditions, as in SPK-02:

- **old kernel's syscall surface**: `kernelsim` (seccomp, built by `verify/Dockerfile`) makes
  every syscall added after the family's oldest kernel fail with `ENOSYS`
  (`verify/syscalls-after-3.10.txt`, `syscalls-after-4.4.txt`; whitespace-separated names only,
  no comments, because every word is looked up as a syscall). `sysprobe` shows the filter is in
  force (`getrandom=ENOSYS statx=ENOSYS` under the 3.10 list).
- **uname** reports the DSM kernel (`QEMU_UNAME`, `qemu-x86_64 -r`).
- **baseline CPU**: x86_64 binaries through a static `qemu-x86_64 -cpu Opteron_G1` (wrappers named
  `ffmpeg`/`ffprobe` on `PATH`), arm binaries through Docker Desktop's binfmt QEMU with
  `QEMU_CPU`. `kernelsim` runs natively and filters the host syscalls of the emulator too.

`verify/checks.sh` then checks:

- `-version` (configure line has `--enable-gpl --enable-version3`, no `nonfree`, no `fdk`),
  `-L` (GPL version 3 or later);
- `-encoders` lists `libx264` and `aac`, `-muxers` lists `hls` and `mp4`, `-demuxers` lists
  every demuxer of probe.go's whitelist plus `image2` and none of concat, hls, dash, imf, vobsub;
  `-h full` of ffmpeg and ffprobe lists `format_whitelist`;
- every file of `testdata/media` probed with the server's arguments (`-format_whitelist <list>
  -protocol_whitelist file file:<path>`), and its `format_name` compared with the manifest;
- hostile inputs: an ffconcat list named `evil.mkv` and an HLS playlist `evil.m3u8` that name a
  real clip are refused, by path and through `/proc/self/fd/3` like the Linux producer; a
  demuxer outside the whitelist is refused with "not on whitelist";
- an HLS transcode of `video/hevc_main10_ac3.mkv` (HEVC Main 10 + AC-3) with the producer's
  options (`-preset veryfast -crf 23 -profile:v high -pix_fmt yuv420p -force_key_frames …
  -sc_threshold 0`, scale, `-c:a aac -ac 2 -b:a 192k`, fMP4 segments, `-hls_flags
  independent_segments+temp_file`, input through descriptor 3) and `-t 3`; the clip is 2 s
  long, so the whole clip. The playlist must map `init.mp4` and end with `#EXT-X-ENDLIST`;
  `init.mp4` plus segments must probe as h264 High yuv420p and aac stereo;
- finally the **server's own FFmpeg adapter tests** (`go test ./internal/adapters/ffmpeg`,
  cross-compiled with the pinned Go image and run in the same container): corpus probe against
  the manifest, hostile inputs, keyframe lookup, HLS argument plans with FFmpeg, producer.

Results: see "Measurements".

What the emulation does not prove: the real DSM kernels (apart from the blocked syscalls the
host's 6.x kernel answers), the kernel's ELF loader (QEMU user mode loads the binary), real
performance, and on armv7 the fallback from the 64-bit time syscalls (`clock_gettime64`,
`futex_time64`, … added in Linux 5.1): QEMU maps them to the x86_64 host's syscalls, which the
filter cannot tell apart. musl falls back to the 32-bit syscalls on `ENOSYS` by design; the
device test (SPK-12, or a community armv7 test) settles it.

## Measurements

Measured on 2026-10-31 with recipe revision 1 (FFmpeg 9.0.2-static1) on Docker Desktop 29.9.2,
Windows 11, 20 CPUs, 16 GB for the Docker VM. MB = 10^6 bytes.

### Binaries

| Family | `ffmpeg` | `ffprobe` | SHA-256 `ffmpeg` | SHA-256 `ffprobe` |
|---|---|---|---|---|
| x86_64 | 24.26 MB | 24.04 MB | `3574d33703c321a3e01e8569773246abfebfd7fc18da329597bfacb828eca768` | `db96c3fe12234e8cc5953fed02876100a3b01a1aa54440a3008cd72b487307b7` |
| armv8 | 18.68 MB | 18.42 MB | `bb4b8a6c0917e6d9cb7975f40cd0423a5434742f736260129b2fcba460cacf2f` | `a7603e1dbc282370f9687095df2f7d7c3650913c33f1726e083391c7c9c47e67` |
| armv7 | 15.91 MB | 15.71 MB | `0dc27fb17ad0532566c32cd9fda81167b12e10af414653a59a96587f2881a0a0` | `42ab97069e03a8b949fc83ff4de0507939c1a71bd5f11d4290da2cc49f286286` |

For comparison (SPK-02): jellyfin-ffmpeg 109.5 MB, johnvansickle 79.8 / 51.1 / 31.8 MB per
`ffmpeg`. armv7 build checks: 0 VFPv4 instructions; 10542 `vld1` (NEON behind detection) per
binary. Source archive `ffmpeg-9.0.2-static1-source.tar`: 168.6 MB, 17 source files (its
SHA-256 changes with every recipe change and is recorded in each `SOURCE.txt`).

### Build times

| Build | Time |
|---|---|
| `all`, cold (`--no-cache`, downloads in the cache mount) | 822 s and 620 s (two runs); the three families build in parallel |
| per family inside it: toolchain / libraries / FFmpeg | x86_64 624 / 17 / 46 s; armv8 519 / 16 / 94 s; armv7 508 / 20 / 68 s |
| `x86_64` alone, cold | 491 s (toolchain 364 s) |
| downloading all sources (empty cache mount) | about 105 s |
| `all`, warm (nothing changed) | 3 s |
| FFmpeg configuration change (toolchains and libraries cached) | the FFmpeg stage, 46–94 s per family |

The toolchains are 80–90 % of a cold build; the cache strategy above keeps them cached across
pin and configuration changes.

### Reproducibility

Two `--no-cache` builds of `all` gave byte-identical `ffmpeg` and `ffprobe` for all three
families (the SHA-256 above). A rebuild from the unpacked source archive with
`--build-context sources=sources` gave the same x86_64 `ffmpeg`. The source archive built twice
from the same recipe has the same SHA-256. Inputs that are not pinned: the Debian build tools
(`apt-get` in the `host` stage; versions recorded in `SOURCE.txt`). They build the toolchains
and run configure; none of their code is linked, and they did not change the output above.

A changed checksum in `sources.list` stops the build (`fetch-sources.sh`, exit 3); an unlisted
file in a replaced `sources` stage stops it too (exit 4). Both were tried.

### Emulation results

`verify.sh` for each family on the binaries above; corpus of 48 files (generated by FFmpeg
9.0.2, `testdata/media/manifest.json`). Each row passed 31 of 31 checks.

| Family | Conditions | Capabilities | Corpus probe (server's arguments) | Hostile inputs | HLS transcode (hevc_main10_ac3.mkv) | Server's adapter tests |
|---|---|---|---|---|---|---|
| x86_64 | bookworm amd64, 3.10 surface, `uname` 3.10.108, `Opteron_G1` | all ✓ | 48/48, format names 48/48 | refused (path and fd 3); whitelist enforced | ✓ h264 High yuv420p + aac stereo, fMP4, 2.10 s | 82 passed, 2 skipped (Windows only), 0 failed |
| armv8 | bookworm arm64, 4.4 surface, `uname` 4.4.302, `cortex-a53` | all ✓ | 48/48, 48/48 | refused | ✓ | 82 / 2 / 0 |
| armv7 | bookworm arm/v7, 3.10 surface, `uname` 3.10.108, `cortex-a9` (NEON, D32) | all ✓ | 48/48, 48/48 | refused | ✓ | 82 / 2 / 0 |
| armv7 | same, `cortex-a15` (VFPv4, NEON) | all ✓ | 48/48, 48/48 | refused | ✓ | 82 / 2 / 0 |
| armv7 | same, `cortex-a9,neon=off,vfp-d32=off` (`AT_HWCAP` without `neon`, with `vfpv3d16`) | all ✓ | 48/48, 48/48 | refused | ✓ | 82 / 2 / 0 |

"Capabilities": `-version` and `-L` show GPLv3 and no nonfree/fdk; `-encoders` lists libx264
and aac; `-muxers` lists hls and mp4; all 33 whitelisted demuxers plus image2 are built, concat,
hls, dash, imf and vobsub are not; `-h full` of both tools lists `format_whitelist`. An
unrestricted ffprobe cannot follow the ffconcat list either, because the concat demuxer is not
built; the whitelist itself is shown to work by refusing a WAV file under `-format_whitelist
matroska` ("not on whitelist").

## Pins

How the pins of `sources.list` were established (2026-10-31): each file was downloaded once and
its upstream signature checked before its SHA-256 was recorded:

| Source | Evidence |
|---|---|
| FFmpeg 9.0.2 | OpenPGP signature, FFmpeg release key `FCF9 86EA 15E6 E293 A564 4F10 B432 2F04 D676 58D8` |
| gcc, binutils, gmp, mpc, mpfr | OpenPGP signatures, checked against `gnu-keyring.gpg`; also musl-cross-make's SHA-1 hashes |
| musl 1.2.6 | OpenPGP signature, Rich Felker `8364 8929 0BB6 B70F 99FF DA05 56BC DB59 3020 450F`; musl-cross-make's SHA-1 |
| zlib 1.3.2 | OpenPGP signature, Mark Adler `5ED4 6A67 21D3 6558 7791 E2AA 783F CD8E 58BC AFBA` |
| dav1d 1.5.4 | OpenPGP signature (VideoLAN key `65F7 C6B4 206B D057 A7EB 7378 7180 713B E58D 1ADC`) and VideoLAN's `.sha256` |
| x264 `b35605ace3dd` (head of the `stable` branch, 2025-06-08, `X264_BUILD` 165) | GitLab archive; its contents equal a `git fetch` of that commit (`diff -r`) |
| NASM 2.16.03 | no signature published; SHA-256 equals Debian's `nasm_2.16.03.orig.tar.xz` |
| musl-cross-make `5e8042e63de2` (2026-10-21) | GitHub commit archive |

GitLab and GitHub generate commit archives on demand; their checksums have been stable, but
neither guarantees it. If one changes, the download fails with exit 3 and the build stops; the
release source archives keep the exact files.

Updating a pin: download the new file, check its signature, replace the line in `sources.list`
(version, file name, SHA-256, URL), bump `RECIPE_REVISION` in `scripts/recipe.sh`, build, run
`verify.sh` for all families and update "Measurements". For a new gcc, binutils or musl version
musl-cross-make must support it (`hashes/` and `patches/` of the pinned commit).

## Gates

```sh
# from the repository root (Git Bash: export MSYS_NO_PATHCONV=1)
cd packaging/synology/ffmpeg/build
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -x scripts/*.sh verify/*.sh
docker run --rm -i hadolint/hadolint:v2.14.0 hadolint - < Dockerfile
docker run --rm -i hadolint/hadolint:v2.14.0 hadolint - < verify/Dockerfile
bash verify/verify.sh armv7 out/armv7     # after building the family
```
