# FFmpeg builds for the Synology packages

Each SPK bundles `ffmpeg` and `ffprobe` for its architecture family (ADR 0026 section 4). This
directory pins those builds:

| File | Purpose |
|---|---|
| `build/` | the own static build: one Docker recipe for all three families plus the corresponding source archive (task SPK-15, [`build/README.md`](build/README.md)) |
| `sources.lock` | one entry per family: status, version, licence and the SHA-256 of the `ffmpeg` and `ffprobe` the recipe produces (format in its header); `build.sh` / `build.ps1` refuse a release whose staged binaries differ |
| `fetch.sh` | `fetch.sh <family> <dest-dir> [lock-file]`: builds one family with the recipe (Docker, cached), verifies it against `sources.lock` and installs it into a directory, for staging by hand (`../toolkit/README.md`) |

**State (SPK-16, 2026-10-31): the packages bundle the own static build**, see "Own build" below;
`sources.lock` pins it and has no `interim` entry. The sections "Result" to "Verification under
emulation" are the measurement of task SPK-02 that led to it (owner decision OD-SPK-3): until
SPK-16 the lock pinned interim prebuilts, which the SPK-07, SPK-10 and SPK-14 pipeline runs used.

## Result (SPK-02 measurement, history)

**No prebuilt candidate meets every criterion of ADR 0026 section 4 for any family.** In
preference order:

- **jellyfin-ffmpeg portable** and **BtbN FFmpeg-Builds** have current FFmpeg versions for
  x86_64 and armv8, but they are **not fully static**: they link glibc dynamically (glibc ≥ 2.28).
  Their providers support **Linux ≥ 4.18** only (ELF ABI note 4.18.20, BtbN's README:
  "glibc>=2.28, linux>=4.18"). DSM 7.2 runs kernel 3.10.108 and 4.4.302 on most x86_64 and armv8
  platforms. Neither provider publishes a 32-bit ARM build.
- **jellyfin's GPL builds link `libfdk-aac` and are not redistributable under FFmpeg's own
  rules.** FFmpeg's `configure` lists libfdk_aac as non-free and refuses it together with
  `--enable-gpl` unless `--enable-nonfree` is given; a build made with `--enable-nonfree` "is
  unredistributable". jellyfin removes libfdk_aac from that list
  (`debian/patches/0026-remove-fdk-aac-from-nonfree.patch`) and configures
  `--enable-gpl --enable-version3 --enable-libfdk-aac` (`ffmpeg -version`). The patch changes the
  check, not the licences: the FDK-AAC licence is still not compatible with the GPL. The project
  therefore treats these binaries as non-free: `sources.lock` marks their licence
  `GPL-3.0-or-later+nonfree`, and `SOURCE.txt` of a fetched build says that it is not
  redistributable. The same applies to ADR 0015's plan to bundle jellyfin-ffmpeg in other
  artifacts.
- **johnvansickle** builds are fully static (ABI 3.2.0 / 3.7.0), but they stop at **FFmpeg
  7.0.2** (August 2024, below 7.1). The provider publishes no corresponding source (its
  `release-source/` directory holds FFmpeg 4.1 sources from 2018). The armhf build is compiled for
  **VFPv4**, which the Cortex-A9 platforms armada38x and monaco lack (toolkit flags `-mfpu=vfpv3`
  and `-mfpu=neon`; DSM-NOTES §13). Its amd64 `ffprobe` crashes
  with SIGSEGV on the MPEG-TS files of the corpus, also when run natively.

So `sources.lock` pinned **interim** builds (status `interim`) until SPK-16. They were good
enough to develop and test the pipeline (SPK-07, SPK-10, SPK-14), but **they must not be
released**:

| Family | Interim build (until SPK-16) | Criteria missed |
|---|---|---|
| x86_64 | jellyfin-ffmpeg 8.1.3-1 portable `linux64-gpl` | not fully static (glibc 2.28 + `libmvec.so.1`); ABI note 4.18.20 > 3.10.108; links libfdk-aac (non-free, not redistributable) |
| armv8 | jellyfin-ffmpeg 8.1.3-1 portable `linuxarm64-gpl` | not fully static (glibc 2.28); ABI note 4.18.20 > 4.4.302; links libfdk-aac (non-free, not redistributable) |
| armv7 | johnvansickle 7.0.2 `armhf` static | FFmpeg 7.0.2 < 7.1; no corresponding source; needs VFPv4, so it fits the armv7 family (alpine, alpine4k) but not armada38x or monaco |

The interim choice follows ADR 0026's preference order, because every candidate misses
something. jellyfin comes first and has an immutable release URL, the complete patches and build
scripts of the build, and the smallest current binaries. For armv7, johnvansickle is the only
prebuilt there is.

**Follow-up, decided by the owner (OD-SPK-3, task SPK-15):** "Own static FFmpeg build for x86_64,
armv8 and armv7". What this measurement suggests for it, one Docker recipe for all three families:

- fully static, against musl, or against glibc with `--enable-kernel` ≤ 3.10;
- `--enable-gpl --enable-version3 --enable-libx264` with the native AAC encoder, without
  libfdk-aac;
- baselines x86-64 v1, armv8-a, and armv7-a with VFPv3-D16 and the hard-float ABI; NEON only
  behind FFmpeg's run-time detection, because NEON on armada38x is unconfirmed;
- reduced to the demuxers, decoders and muxers the server needs.

It meets every criterion, gives one source for all families, and produces the exact corresponding
source as a by-product. It is also smaller. The alternatives that were offered (a glibc-dynamic
BtbN build after the checks under "Open checks", or jellyfin despite libfdk-aac) were not chosen.
SPK-15 built it, SPK-16 switched the pins to it ("Own build").

The status field stays: do not release with an `interim` entry. `build.sh` / `build.ps1`
refuse a release build while `grep -q '^[^#][^ ]* interim ' sources.lock` succeeds, and every
build writes its status into `SOURCE.txt`, so SPK-10 can see an interim build in the package.

## DSM 7.2 kernels and CPUs per family

Kernel versions come from Synology's DSM 7.2 toolchain archive
(<https://archive.synology.com/download/ToolChain/toolchain/7.2-72806>, also `7.2-72746` and
`7.2-63134`; the folder names state the kernel). All 7.2 toolchains are `gcc1220_glibc236`, so the
DSM 7.2 userland has glibc 2.36 *(inference, verify SPK-01)*. The CPUs are those of the platforms'
known models *(verify, SPK-01)*.

| Family | Platforms (kernel) | Oldest kernel | Oldest CPUs |
|---|---|---|---|
| x86_64 | avoton, braswell, bromolow, grantley (3.10.108); apollolake, broadwell, broadwellnk, broadwellnkv2, broadwellntbap, denverton, geminilake, kvmx64, kvmcloud, purley, r1000, v1000 (4.4.x); epyc7002, geminilakenk, r1000nk, v1000nk (5.10.55) | **3.10.108** | Atom C2000 (Silvermont), Celeron N3xx0 (Airmont): SSE4.2, no AVX |
| armv8 | rtd1296, armada37xx (4.4.302); rtd1619b (5.10.55) | **4.4.302** | Cortex-A53 |
| armv7 family | alpine, alpine4k (3.10.108) | **3.10.108** | Cortex-A15 (Annapurna Labs Alpine), toolkit flags `-mfpu=neon-vfpv4` |
| separate `arch` values (not in the armv7 family) | armada38x, monaco (3.10.108) | **3.10.108** | Cortex-A9: armada38x (Armada 385/388) toolkit flags `-mfpu=vfpv3`, NEON *unconfirmed*; monaco (STiH412) `-mfpu=neon` |

DSM's `armv7` family contains only alpine and alpine4k; armada38x and monaco are separate
`arch` values that the armv7 SPK lists in addition (`arch="armv7 armada38x monaco"`,
`packaging/synology/DSM-NOTES.md` §3; that DSM installs it there is unconfirmed). Synology builds
armada38x with `-mfpu=vfpv3` only, so whether its silicon has NEON is **unconfirmed**
(DSM-NOTES §13). An armv7 FFmpeg for this SPK may rely on ARMv7 + VFPv3 hard-float only, and on
NEON only through run-time detection.

## Candidates per family

Columns: availability; FFmpeg version; fully static (no `PT_INTERP`, no `NEEDED`); licence
variant (from `-version`); ELF ABI note, which is the minimum kernel; instruction-set baseline;
uncompressed size of `ffmpeg` / `ffprobe` (MB = 10^6 bytes); URL immutability; corresponding
source. ✓ means the ADR 0026 criterion is met, ✗ means it is missed.

### x86_64

| Candidate | Available | Version | Fully static | Licence | ABI note | Baseline | ffmpeg / ffprobe | URL immutable | Corresponding source |
|---|---|---|---|---|---|---|---|---|---|
| jellyfin-ffmpeg portable `linux64-gpl` v8.1.3-1 | yes | 8.1.3 ✓ | ✗ dynamic: libc, libm, libdl, libpthread, libmvec (GLIBC_2.28) | ✗ GPLv3 + libx264 + **libfdk-aac** (non-free, not redistributable) | 4.18.20 ✗ | x86-64 v1 (no `-march`) ✓ | 109.5 / 109.2 | ✓ GitHub release asset | ✓ tag archive: FFmpeg tree + `debian/patches` + `builder/` |
| BtbN `linux64-gpl-8.1` n8.1.3-27-g8dca1586f4 | yes | 8.1.3+27 ✓ | ✗ dynamic: as above + librt, libgcc_s | GPLv3 + libx264 ✓ | 4.18.20 ✗ | x86-64 v1 ✓ | 173.1 / 172.9 | ✗ daily build kept 14 days (month-end builds 2 years) | ✓ FFmpeg commit + FFmpeg-Builds repo commit |
| johnvansickle `amd64` release | yes | 7.0.2 ✗ | ✓ | GPLv3 + libx264 ✓ | 3.2.0 ✓ | ran under QEMU `Opteron_G1` | 79.8 / 79.7 | ✗ `ffmpeg-release-…` is replaced by the next release | ✗ none for 7.0.2 |
| own static build (SPK-15) | built (9.0.2-static5), see "Own build" | 9.0.2 ✓ | ✓ | GPLv3 + libx264 | ≤ 3.10 | x86-64 v1 | smaller (reduced configure) | ✓ GitHub Release | ✓ produced by the recipe |

### armv8

| Candidate | Available | Version | Fully static | Licence | ABI note | Baseline | ffmpeg / ffprobe | URL immutable | Corresponding source |
|---|---|---|---|---|---|---|---|---|---|
| jellyfin-ffmpeg portable `linuxarm64-gpl` v8.1.3-1 | yes | 8.1.3 ✓ | ✗ dynamic: libc, libm, libdl, libpthread (GLIBC_2.28) | ✗ GPLv3 + libx264 + **libfdk-aac** (non-free, not redistributable) | 4.18.20 ✗ | `-march=armv8-a` ✓ | 79.8 / 79.5 | ✓ | ✓ as x86_64 |
| BtbN `linuxarm64-gpl-8.1` n8.1.3-27-g8dca1586f4 | yes | 8.1.3+27 ✓ | ✗ dynamic + librt, libgcc_s | GPLv3 + libx264 ✓ | 4.18.20 ✗ | armv8-a ✓ | 131.1 / 130.9 | ✗ as x86_64 | ✓ as x86_64 |
| johnvansickle `arm64` release | yes | 7.0.2 ✗ | ✓ | GPLv3 + libx264 ✓ | 3.7.0 ✓ | ran under QEMU `cortex-a53` | 51.1 / 51.0 | ✗ | ✗ |
| own static build (SPK-15) | built (9.0.2-static5), see "Own build" | 9.0.2 ✓ | ✓ | GPLv3 + libx264 | ≤ 4.4 | armv8-a | smaller | ✓ | ✓ |

### armv7 (family alpine, alpine4k, plus the separate values armada38x and monaco)

| Candidate | Available | Version | Fully static | Licence | ABI note | Baseline | ffmpeg / ffprobe | URL immutable | Corresponding source |
|---|---|---|---|---|---|---|---|---|---|
| jellyfin-ffmpeg portable | **no** 32-bit ARM build (assets of v8.1.3-1: linux64, linuxarm64, mac, win only) | – | – | – | – | – | – | – | – |
| BtbN FFmpeg-Builds | **no** (linux64 and linuxarm64 only) | – | – | – | – | – | – | – | – |
| johnvansickle `armhf` release | yes | 7.0.2 ✗ | ✓ | GPLv3 + libx264 ✓ | 3.2.0 ✓ | armv7-a hard-float, **VFPv4** + NEON (2249 VFMA instructions): fits alpine/alpine4k, ✗ on armada38x and monaco (Cortex-A9) | 31.8 / 31.7 | ~ versioned file name `ffmpeg-7.0.2-armhf-static.tar.xz` (same SHA-256), but the directory may move to `old-releases/` | ✗ |
| own static build (SPK-15) | built (9.0.2-static5), see "Own build" | 9.0.2 ✓ | ✓ | GPLv3 + libx264 | ≤ 3.10 | armv7-a, VFPv3-D16, hard-float; NEON only with run-time detection | smaller | ✓ | ✓ |

### Archives measured

| Candidate | Archive URL | SHA-256 | Size (bytes) |
|---|---|---|---|
| jellyfin x86_64 | `https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v8.1.3-1/jellyfin-ffmpeg_8.1.3-1_portable_linux64-gpl.tar.xz` | `b86dc023c64a5a9a410e6e3a938a970460214fae78f31c1553c9f88f1c289b6a` | 60368340 |
| jellyfin armv8 | `…/v8.1.3-1/jellyfin-ffmpeg_8.1.3-1_portable_linuxarm64-gpl.tar.xz` | `7bc8e8d0986f7f4693f63e9728570f671f07b6e21c05d5465dd7ce461d1631dc` | 54137808 |
| jellyfin source | `https://github.com/jellyfin/jellyfin-ffmpeg/archive/refs/tags/v8.1.3-1.tar.gz` | `87bedcefc860cd234cdeddbebdcd3ce45aeedc81d0fbb258ed839ff778be3140` | 17772245 |
| BtbN x86_64 | `https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-10-30-13-01/ffmpeg-n8.1.3-27-g8dca1586f4-linux64-gpl-8.1.tar.xz` | `0912d9de5837ae3fbeb98c54e5a5886ac7fcb68aee7d7d1cfcd2d20c505eccf2` | 152248816 |
| BtbN armv8 | `…/autobuild-2026-10-30-13-01/ffmpeg-n8.1.3-27-g8dca1586f4-linuxarm64-gpl-8.1.tar.xz` | `84543f8877666cd24b08ccdba64f0e57acfc0b316b810dd06dfe17718594af21` | 127651832 |
| johnvansickle amd64 | `https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz` | `abda8d77ce8309141f83ab8edf0596834087c52467f6badf376a6a2a4c87cf67` | 41888096 |
| johnvansickle arm64 | `…/releases/ffmpeg-release-arm64-static.tar.xz` | `f4149bb2b0784e30e99bdda85471c9b5930d3402014e934a5098b41d0f7201b1` | 19337412 |
| johnvansickle armhf (pinned) | `https://johnvansickle.com/ffmpeg/releases/ffmpeg-7.0.2-armhf-static.tar.xz` (same file as `…/ffmpeg-release-armhf-static.tar.xz`) | `7d41f558cb1f3395b313f8ceabed78b3731c79a0962abf405ebb5cd393e93991` | 16150344 |
| FFmpeg 7.0.2 source | `https://ffmpeg.org/releases/ffmpeg-7.0.2.tar.xz` | `8646515b638a3ad303e23af6a3587734447cb8fc0a0c064ecdb8e95c4fd8b389` | – |

The GitHub checksums match the `digest` that the GitHub API reports for each release asset.
jellyfin's portable archives hold only `ffmpeg` and `ffprobe`, with no licence file, so the
interim `fetch.sh` took `COPYING.GPLv3` and `LICENSE.md` from the verified source archive of the
same tag.

## Verification under emulation

Every candidate ran in Docker Desktop (QEMU binfmt for arm64 and arm/v7) under conditions close
to DSM 7.2:

- **glibc 2.36 userland**: `debian:bookworm-slim`, which has the same glibc as the DSM 7.2
  toolchains.
- **Old kernel's syscall surface**: a small seccomp wrapper (`kernelsim`,
  [`build/verify/kernelsim.c`](build/verify/kernelsim.c)) makes every syscall added after the
  oldest kernel of the family fail with `ENOSYS`. The lists are
  [`build/verify/syscalls-after-3.10.txt`](build/verify/syscalls-after-3.10.txt) (65 names:
  `getrandom`, `memfd_create`, `statx`, `rseq`, `clone3`, `faccessat2`, `io_uring_*`, …) and
  [`syscalls-after-4.4.txt`](build/verify/syscalls-after-4.4.txt) (53 names); a name unknown to
  libseccomp is reported and skipped. In addition, `uname -r` reports the DSM kernel
  (`qemu-x86_64 -r 3.10.108`, or `QEMU_UNAME`), which glibc's "kernel too old" check reads.
- **Baseline CPU**: x86_64 under `qemu-x86_64 -cpu Opteron_G1` (SSE2 only, x86-64 v1), armv8 under
  `QEMU_CPU=cortex-a53`, armv7 under `QEMU_CPU=cortex-a9` and `cortex-a15`. QEMU's Cortex-A9
  model does trap VFMA: a test program using `vfma.f32` gets SIGILL there and runs on Cortex-A15.
  QEMU's Cortex-A9 model has NEON, so it stands for monaco but cannot show how a build behaves
  on an armada38x without NEON (unconfirmed, see above).

Checks per candidate: `-version`; `-encoders` lists `libx264` and `aac`; `-muxers` lists `hls`;
`-h full` of ffmpeg and ffprobe lists `format_whitelist`; every demuxer of the server's
`-format_whitelist` exists (`internal/adapters/ffmpeg/probe.go`); every file of
`testdata/media` (48) is probed with the server's arguments (`-format_whitelist <list>
-protocol_whitelist file file:<path>`); a hostile ffconcat list named `evil.mkv` is refused; and a
3 s HLS transcode of `hevc_main10_ac3.mkv` produces fMP4 output with libx264 and aac. It uses the
producer's options: `-preset veryfast`, `-profile:v high`, `-pix_fmt yuv420p`, scale, `-hls_flags
independent_segments+temp_file`.

| Candidate | Conditions | Capabilities | Corpus probe | Hostile list | HLS transcode |
|---|---|---|---|---|---|
| jellyfin x86_64 | bookworm, 3.10 surface, Opteron_G1 | all ✓ | 48/48 | refused | ✓ h264 + aac |
| BtbN x86_64 | same | all ✓ | 48/48 | refused | ✓ |
| johnvansickle amd64 | same | all ✓ | **44/48**: SIGSEGV on 4 MPEG-TS files (`.m2ts`, `.mts`, `.ts`), also native without QEMU | refused | ✓ |
| jellyfin armv8 | bookworm arm64, 4.4 surface, cortex-a53 | all ✓ | 48/48 | refused | ✓ |
| BtbN armv8 | same | all ✓ | 48/48 | refused | ✓ |
| johnvansickle arm64 | same | all ✓ | 48/48 | refused | ✓ |
| johnvansickle armhf | bookworm arm/v7, 3.10 surface, cortex-a9 and cortex-a15 | all ✓ | 48/48 | refused | ✓ (the tested paths contain no VFMA; others do) |

What emulation does not prove: the real DSM kernel (its syscalls behave like the host's 6.x
kernel apart from the blocked ones), the libraries DSM actually ships, and real performance.

### Open checks on a real DSM 7.2 (SPK-12, owner's device)

Only needed while a glibc-dynamic interim build is used on real hardware. Over SSH:

```sh
uname -r; ldd --version | head -1
ls -l /lib64/ld-linux-x86-64.so.2 /lib/libc.so.6 /lib/libm.so.6 /lib/libmvec.so.1 /lib/libgcc_s.so.1
/var/packages/<id>/target/bin/ffmpeg -hide_banner -version | head -1
```

The same `kernelsim` and lists serve the own build's verification (`build/verify/verify.sh`).

## Own build

Owner decision of 2026-10-31 (ADR 0026 Notes): the packages bundle an **own static build**, made
by the recipe in [`build/`](build/README.md) (task SPK-15). Since SPK-16 `sources.lock` pins it
and the SPK build takes FFmpeg from it (`../build.sh` / `../build.ps1`, step 0: the recipe's
`all` target as a local image, cached by BuildKit – a cold build takes 10–14 minutes, a warm one
seconds). It meets every criterion of ADR 0026 section 4 for all three families:

| Family | Build | Fully static | ELF ABI note | Baseline | ffmpeg / ffprobe (MB) |
|---|---|---|---|---|---|
| x86_64 | FFmpeg 9.0.2-static5, musl 1.2.6 | ✓ | none (limit 3.10.108) | x86-64 v1 | 24.60 / 24.38 |
| armv8 | same | ✓ | none (limit 4.4.302) | armv8-a | 18.95 / 18.68 |
| armv7 | same | ✓ | none (limit 3.10.108) | armv7-a, VFPv3-D16, hard-float, 0 VFPv4 instructions; NEON only behind run-time detection (FFmpeg: `HAVE_NEON=1` with `HAVE_NEON_INLINE/EXTERNAL=0`, x264, dav1d) – `readelf -A` reports the merged build attributes `Tag_FP_arch: VFPv3` and `Tag_Advanced_SIMD_arch: NEONv1` (build/README.md "Baselines") | 16.16 / 15.96 |

- GPL version 3 (`--enable-gpl --enable-version3`), libx264 and the native AAC encoder; no
  `--enable-nonfree`, no libfdk-aac. Also linked: dav1d (AV1), zlib, musl, libgcc.
- Reduced configuration: exactly the server's `-format_whitelist` demuxers plus image2, and concat
  for folder discs only (no hls, dash, imf, vobsub; recipe revision 3, M3-16j), protocols file, pipe,
  concat and subfile (ISO images, revision 4, M3-16j-b), muxers hls/mp4/null, encoders
  libx264/aac/mjpeg/png/webvtt (webvtt and the overlay filter since revision 5, SPK-17), all
  decoders; no libass, so no `subtitles` filter. The
  Go test `packaging/synology/internal/recipecheck` keeps the recipe's list and `probe.go` equal.
- Every input (FFmpeg, x264, dav1d, zlib, musl, gcc, binutils, gmp, mpc, mpfr, musl-cross-make,
  NASM) is pinned by SHA-256 in `build/sources.list`; a changed checksum fails the build.
- The recipe also produces the corresponding source archive
  (`ffmpeg-9.0.2-static5-source.tar`, 168.6 MB, all sources plus the recipe); the SPK build puts
  it into `dist/` next to the SPKs with a line in `SHA256SUMS`, to attach to every GitHub
  Release. Clean builds give byte-identical binaries (the hashes in `sources.lock`).
- Verified under the emulation conditions above for x86_64 (Opteron_G1), armv8 (cortex-a53) and
  armv7 (cortex-a9, cortex-a15, and cortex-a9 without NEON and with 16 double registers, the
  Armada 385/388 case): capabilities, 48/48 corpus files, hostile inputs refused, HLS transcode
  to h264 + aac fMP4, and the server's FFmpeg adapter tests all pass.

Build commands, configuration, checksums, build times and the full results:
[`build/README.md`](build/README.md).

## Corresponding source (GPL-3.0 section 6)

- **own build** (what the packages ship): the recipe pins every source tarball by SHA-256 and
  bundles them with itself into `ffmpeg-<version>-source.tar`; the SPK build writes the archive
  to `dist/` and each `SOURCE.txt` names its SHA-256 (`build.sh` / `build.ps1` check that the
  two match), so the release asset is the source of exactly the shipped binaries.
- **jellyfin-ffmpeg** (interim x86_64, armv8 until SPK-16): the tag archive holds the FFmpeg tree, the patches
  applied at build time (`debian/patches/`) and the build scripts (`builder/`). The build scripts
  pin the dependencies by repository and commit (43 scripts in `builder/scripts.d` have a
  `SCRIPT_COMMIT`, for example x264 `0480cb05fa188d37ae87e8f4fd8f1aea3711f7ee`). The tag
  archive does not contain those dependencies' sources. A complete source offer for the static binary must add them at the
  pinned commits. GitHub generates tag archives on demand: their checksum has been stable since
  2023, but GitHub does not guarantee it. That is one more reason to attach the source to the
  GitHub Release (ADR 0026 section 4). Independently of the source, these builds link
  libfdk-aac and are not redistributable (see "Result").
- **johnvansickle** (interim armv7 until SPK-16): only the upstream FFmpeg 7.0.2 tarball is
  pinned. The provider publishes neither the build scripts nor the library sources of this
  build. This alone rules the build out for a release.

## `sources.lock` and `fetch.sh`

The field list is in the header of `sources.lock`: per family the status, the version label
(`9.0.2-static5`: FFmpeg release plus `RECIPE_REVISION` of `build/scripts/recipe.sh`), the
licence and the SHA-256 of `ffmpeg` and `ffprobe`. The binaries are reproducible, so the lock
pins the exact files that were verified under emulation; the source archive's SHA-256 is not in
the lock, because it changes with every edit of the recipe directory (each build records it in
its `SOURCE.txt`). Who reads the lock:

- `../build.sh` / `../build.ps1` (SPK-07, SPK-16): before anything runs, a release refuses an
  `interim` entry; after staging, the SHA-256 of the staged `ffmpeg` and `ffprobe` and the
  `Version:` and `Licence:` of the staged `SOURCE.txt` must equal the entry – a release refuses
  a difference, a test build packs it with a warning and names it in `NOT-FOR-RELEASE.txt`.
- `fetch.sh` (POSIX `sh`, runs on the host with Docker; for staging by hand): runs
  `docker build --target <family>` of the recipe into a work directory, checks the two
  checksums and the `Version:`, `Status:` and `Licence:` lines of `SOURCE.txt` against the
  entry, and only then installs `ffmpeg`, `ffprobe` (0755), the licence texts and `SOURCE.txt`
  (0644) into `<dest-dir>`. Files of an earlier fetch into the same directory (listed in its
  `.fetch-manifest`) are removed first; files it did not write are left alone. Whenever the
  script ends without success, its EXIT trap removes `ffmpeg`, `ffprobe` and partial copies from
  `<dest-dir>`. Exit codes: 0 success; 1 docker, copy or missing tool; 2 usage error, unknown
  family, missing recipe, or missing, duplicate or malformed lock entry; 3 checksum or notice
  mismatch; 4 file missing in the build output. `FFMPEG_RECIPE_DIR` points it at another recipe
  directory (the tests do). `interim` and `+nonfree` entries produce a warning on stderr.

spkstage (SPK-14) stages `ffmpeg` and `ffprobe` as `target/bin/`, and everything else except
`.fetch-manifest` (`COPYING.GPLv3`, `LICENSE.md`, the licences of x264, dav1d, zlib, musl and
libgcc, plus `SOURCE.txt`) as `target/share/licenses/ffmpeg/`. A fresh `<dest-dir>` per family
is simplest.

```sh
# Lint (from the repository root)
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/ffmpeg/fetch.sh

# Tests without Docker (fake docker; test/README.md "Build script tests")
docker run --rm -v "${PWD}:/code" -w /code spk-script-test:dev packaging/synology/test/build/fetch.bats

# Fetch one family into ./out/<family> (Git Bash or Linux; Docker builds the recipe) and run it
# on the family's platform
sh packaging/synology/ffmpeg/fetch.sh armv8 out/armv8
docker run --rm --platform linux/arm64 -v "${PWD}/out/armv8:/ff" debian:stable-slim /ff/ffmpeg -version
```

## Updating a pin

A pin changes only when the binaries change: a new FFmpeg or library version, a configure or
compiler-flag change (`build/README.md` "Pins").

1. Change the recipe and bump `RECIPE_REVISION` in `build/scripts/recipe.sh`; build all three
   families (`docker build --output type=local,dest=out packaging/synology/ffmpeg/build`).
2. Verify each family under its emulation conditions (`build/verify/verify.sh <family>
   out/<family>`) and update the tables in `build/README.md`.
3. Replace the three lines of `sources.lock` with the new version label and the SHA-256 of the
   built `ffmpeg` and `ffprobe` (`out/<family>/SOURCE.txt` prints them); set status `selected`
   only if every criterion of ADR 0026 section 4 is met.
4. `build.sh --release <version>` must pass its pin check; `fetch.sh <family> <dir>` verifies a
   single family by hand.
