# FFmpeg builds for the Synology packages

Each SPK bundles `ffmpeg` and `ffprobe` for its architecture family (ADR 0026 section 4). This
directory pins those builds:

| File | Purpose |
|---|---|
| `sources.lock` | one entry per family: binary archive and corresponding source, both pinned by SHA-256 (format in its header) |
| `fetch.sh` | `fetch.sh <family> <dest-dir> [lock-file]`: downloads, verifies and extracts one family's build (used by the prebuild stage of SPK-07) |

Measured on 2026-10-31 (task SPK-02). **Owner decision OD-SPK-3 (2026-10-31): an own static
FFmpeg build for all three families (task SPK-15).** The interim pins below stay until SPK-15
replaces them.

## Result

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

So `sources.lock` pins **interim** builds (status `interim`). They are good enough to develop and
test the pipeline (SPK-07, SPK-10, SPK-11), but **they must not be released**:

| Family | Interim build | Criteria missed |
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

Do not release with an `interim` entry. The release gate (SPK-13) should refuse the build if
`grep -q '^[^#][^ ]* interim ' sources.lock` succeeds. Every fetched build also writes its status
into `SOURCE.txt`, so SPK-10 can see an interim build in the package.

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
| own static build (SPK-15) | to build | chosen (≥ 8.1) | ✓ | GPLv3 + libx264 | ≤ 3.10 | x86-64 v1 | smaller (reduced configure) | ✓ GitHub Release | ✓ produced by the recipe |

### armv8

| Candidate | Available | Version | Fully static | Licence | ABI note | Baseline | ffmpeg / ffprobe | URL immutable | Corresponding source |
|---|---|---|---|---|---|---|---|---|---|
| jellyfin-ffmpeg portable `linuxarm64-gpl` v8.1.3-1 | yes | 8.1.3 ✓ | ✗ dynamic: libc, libm, libdl, libpthread (GLIBC_2.28) | ✗ GPLv3 + libx264 + **libfdk-aac** (non-free, not redistributable) | 4.18.20 ✗ | `-march=armv8-a` ✓ | 79.8 / 79.5 | ✓ | ✓ as x86_64 |
| BtbN `linuxarm64-gpl-8.1` n8.1.3-27-g8dca1586f4 | yes | 8.1.3+27 ✓ | ✗ dynamic + librt, libgcc_s | GPLv3 + libx264 ✓ | 4.18.20 ✗ | armv8-a ✓ | 131.1 / 130.9 | ✗ as x86_64 | ✓ as x86_64 |
| johnvansickle `arm64` release | yes | 7.0.2 ✗ | ✓ | GPLv3 + libx264 ✓ | 3.7.0 ✓ | ran under QEMU `cortex-a53` | 51.1 / 51.0 | ✗ | ✗ |
| own static build (SPK-15) | to build | chosen | ✓ | GPLv3 + libx264 | ≤ 4.4 | armv8-a | smaller | ✓ | ✓ |

### armv7 (family alpine, alpine4k, plus the separate values armada38x and monaco)

| Candidate | Available | Version | Fully static | Licence | ABI note | Baseline | ffmpeg / ffprobe | URL immutable | Corresponding source |
|---|---|---|---|---|---|---|---|---|---|
| jellyfin-ffmpeg portable | **no** 32-bit ARM build (assets of v8.1.3-1: linux64, linuxarm64, mac, win only) | – | – | – | – | – | – | – | – |
| BtbN FFmpeg-Builds | **no** (linux64 and linuxarm64 only) | – | – | – | – | – | – | – | – |
| johnvansickle `armhf` release | yes | 7.0.2 ✗ | ✓ | GPLv3 + libx264 ✓ | 3.2.0 ✓ | armv7-a hard-float, **VFPv4** + NEON (2249 VFMA instructions): fits alpine/alpine4k, ✗ on armada38x and monaco (Cortex-A9) | 31.8 / 31.7 | ~ versioned file name `ffmpeg-7.0.2-armhf-static.tar.xz` (same SHA-256), but the directory may move to `old-releases/` | ✗ |
| own static build (SPK-15) | to build | chosen | ✓ | GPLv3 + libx264 | ≤ 3.10 | armv7-a, VFPv3-D16, hard-float; NEON only with run-time detection | smaller | ✓ | ✓ |

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
jellyfin's portable archives hold only `ffmpeg` and `ffprobe`, with no licence file, so `fetch.sh`
takes `COPYING.GPLv3` and `LICENSE.md` from the verified source archive of the same tag.

## Verification under emulation

Every candidate ran in Docker Desktop (QEMU binfmt for arm64 and arm/v7) under conditions close
to DSM 7.2:

- **glibc 2.36 userland**: `debian:bookworm-slim`, which has the same glibc as the DSM 7.2
  toolchains.
- **Old kernel's syscall surface**: a small seccomp wrapper (`kernelsim`, source below) makes
  every syscall added after the oldest kernel of the family fail with `ENOSYS`. That is 54
  syscalls after 3.10 on x86_64 (`getrandom`, `memfd_create`, `statx`, `rseq`, `clone3`,
  `faccessat2`, `io_uring_*`, …) and 42 after 4.4. In addition, `uname -r` reports the DSM kernel
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

### kernelsim

```c
/* kernelsim <names-file> <cmd> [args...]: every syscall named in names-file fails with ENOSYS
 * for cmd and its children. Build: gcc -O2 -static -o kernelsim kernelsim.c -lseccomp
 * (debian:bookworm-slim with gcc libc6-dev libseccomp-dev). Runs outside the container's own
 * seccomp profile, because runc itself needs statx and fsopen to start a container. */
#include <errno.h>
#include <seccomp.h>
#include <stdio.h>
#include <unistd.h>
int main(int argc, char **argv) {
  FILE *f = argc >= 3 ? fopen(argv[1], "r") : NULL;
  if (!f) { fprintf(stderr, "usage: kernelsim names-file cmd [args]\n"); return 2; }
  scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
  char name[128];
  while (fscanf(f, "%127s", name) == 1) {
    int nr = seccomp_syscall_resolve_name(name);
    if (nr != __NR_SCMP_ERROR) seccomp_rule_add(ctx, SCMP_ACT_ERRNO(ENOSYS), nr, 0);
  }
  if (seccomp_load(ctx) != 0) return 2;
  execvp(argv[2], argv + 2);
  return 127;
}
```

Syscalls after 4.4: `copy_file_range preadv2 pwritev2 pkey_mprotect pkey_alloc pkey_free statx
io_pgetevents rseq io_uring_setup io_uring_enter io_uring_register pidfd_send_signal open_tree
move_mount fsopen fsconfig fsmount fspick pidfd_open clone3 openat2 pidfd_getfd faccessat2
close_range process_madvise epoll_pwait2 mount_setattr quotactl_fd landlock_create_ruleset
landlock_add_rule landlock_restrict_self memfd_secret process_mrelease futex_waitv
set_mempolicy_home_node cachestat fchmodat2 map_shadow_stack futex_wake futex_wait futex_requeue
statmount listmount lsm_get_self_attr lsm_set_self_attr lsm_list_modules mseal setxattrat
getxattrat listxattrat removexattrat open_tree_attr`. After 3.10, add: `sched_setattr
sched_getattr renameat2 seccomp getrandom memfd_create kexec_file_load bpf execveat userfaultfd
membarrier mlock2`. Names unknown to libseccomp 2.5.4 are skipped.

## Corresponding source (GPL-3.0 section 6)

- **jellyfin-ffmpeg** (interim x86_64, armv8): the tag archive holds the FFmpeg tree, the patches
  applied at build time (`debian/patches/`) and the build scripts (`builder/`). The build scripts
  pin the dependencies by repository and commit (43 scripts in `builder/scripts.d` have a
  `SCRIPT_COMMIT`, for example x264 `0480cb05fa188d37ae87e8f4fd8f1aea3711f7ee`). The tag
  archive does not contain those dependencies' sources. A complete source offer for the static binary must add them at the
  pinned commits. GitHub generates tag archives on demand: their checksum has been stable since
  2023, but GitHub does not guarantee it. That is one more reason to attach the source to the
  GitHub Release (ADR 0026 section 4). Independently of the source, these builds link
  libfdk-aac and are not redistributable (see "Result").
- **johnvansickle** (interim armv7): only the upstream FFmpeg 7.0.2 tarball is pinned. The
  provider publishes neither the build scripts nor the library sources of this build. This alone
  rules the build out for a release.
- **own build** (SPK-15): the recipe pins every source tarball by SHA-256 and can bundle them
  into the release asset.

## `sources.lock` and `fetch.sh`

The field list is in the header of `sources.lock`. `fetch.sh`:

- reads the family's single entry (POSIX `sh`, no `jq`);
- downloads the binary archive with `curl` or `wget` and checks its SHA-256;
- only then extracts `ffmpeg`, `ffprobe` and the `archive:` licence files;
- downloads and checks the source archive only if a licence file comes from it (`source:`);
- writes `SOURCE.txt` with version, status, licence, URLs and checksums, including the checksums of
  `ffmpeg` and `ffprobe`; for a `+nonfree` licence it adds a note that the build is not
  redistributable;
- removes the files of an earlier fetch into the same `<dest-dir>` (listed in its
  `.fetch-manifest`), so the licence files of another family never stay behind, and writes a new
  manifest; files it did not write are left alone.

Binaries get mode 0755, the other files 0644. Lock fields are split without file name expansion
(`set -f`); paths that are absolute, contain `..` or start with `-` are refused. Whenever the
script ends without success (a reported error, a command failing under `set -e`, a signal), its
EXIT trap removes `ffmpeg`, `ffprobe` and partial copies from `<dest-dir>`. Exit codes: 0
success; 1 download, extraction, copy or missing tool; 2 usage error, unknown family, or missing,
duplicate or malformed lock entry; 3 checksum mismatch; 4 member missing in a verified archive.
`interim` and `+nonfree` entries produce a warning on stderr.

SPK-07 copies `ffmpeg` and `ffprobe` to `target/bin/`, and everything else except
`.fetch-manifest` (`COPYING.GPLv3`, `LICENSE.md` or `GPLv3.txt` and `readme.txt`, plus
`SOURCE.txt`) to `target/share/licenses/ffmpeg/`. A fresh `<dest-dir>` per family is simplest.

Tested with GNU tools (`debian:stable-slim` with curl, xz-utils, ca-certificates) and BusyBox
(`alpine:3` with xz; BusyBox `wget`, `sha256sum` and `tar`).

```sh
# Lint (from the repository root)
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/ffmpeg/fetch.sh

# Fetch one family into ./out/<family> and run it on the family's platform
docker run --rm -v "${PWD}/packaging/synology/ffmpeg:/ffm:ro" -v "${PWD}/out:/out" debian:stable-slim \
  sh -c 'apt-get -qq update && apt-get -qq install -y curl xz-utils ca-certificates >/dev/null && sh /ffm/fetch.sh armv8 /out/armv8'
docker run --rm --platform linux/arm64 -v "${PWD}/out/armv8:/ff" debian:stable-slim /ff/ffmpeg -version
```

## Updating a pin

1. Measure the new build as above: static linkage and ABI note (`readelf -h -l -d -n`, and
   `readelf -A` for armv7), capabilities, corpus and transcode under the family's emulation
   conditions.
2. Replace the family's line in `sources.lock`, with archive and source checksums computed from
   the downloaded files, and update the tables here.
3. Set status `selected` only if every criterion of ADR 0026 section 4 is met.
