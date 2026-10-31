# SPK-02 Select and pin static FFmpeg builds per architecture family
Owner: mivo-synology
Depends on: none (uses SPK-01's kernel list if available, otherwise records its own sources)
Files/areas: `packaging/synology/ffmpeg/**` (new): `README.md`, `sources.lock`, `fetch.sh`

## Goal
Each SPK can bundle an FFmpeg and ffprobe that run on DSM 7.2 for its family, are pinned by
checksum, and come with their licence and corresponding source, so the package plays the same
corpus as the Docker image.

## Acceptance criteria
- [ ] `README.md` compares the candidates for each family (x86_64, armv8, armv7): jellyfin-ffmpeg
      portable, BtbN FFmpeg-Builds (release branch), johnvansickle static release builds, and
      "own static build" as fallback. Columns: availability, FFmpeg version, fully static,
      licence variant, ELF ABI note (minimum kernel), instruction-set baseline, size of ffmpeg
      and ffprobe, URL immutability, corresponding source available. It names the chosen build
      per family and why, following the criteria and preference order of ADR 0026 §4.
- [ ] `sources.lock` has exactly three entries, one per family, in a documented line format
      readable by POSIX `sh` without `jq` (e.g. `family url sha256 ffmpeg_version licence
      source_url source_sha256`), with a header comment explaining every field.
- [ ] `fetch.sh <family> <dest-dir>` (POSIX `sh`): downloads the archive of the family, checks
      its SHA-256 and, only on a match, extracts `ffmpeg`, `ffprobe` and the licence file(s) into
      `<dest-dir>` (`ffmpeg`/`ffprobe` mode 0755). A wrong checksum, unknown family or missing
      member exits non-zero and leaves `<dest-dir>` without binaries. Checked with
      `docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:<pinned tag> -s sh packaging/synology/ffmpeg/fetch.sh`
      → no findings.
- [ ] For each family, after `fetch.sh` in a `debian:stable-slim` container:
      `docker run --rm --platform <linux/amd64|linux/arm64|linux/arm/v7> -v <dest>:/ff debian:stable-slim /ff/ffmpeg -version`
      and the same with `ffprobe` exit 0; the REPORT quotes both version lines per family.
- [ ] For each family `file` (or `readelf -h -n`) shows a statically linked executable of the right
      machine; the ABI note's minimum kernel is ≤ the oldest DSM 7.2 kernel of the family
      (SPK-01 Q13, or a cited source); values recorded in the README.
- [ ] For each family `ffmpeg -hide_banner -encoders` lists `libx264` and `aac`, `-muxers` lists
      `hls`, and `ffmpeg -hide_banner -h full` lists `format_whitelist`; FFmpeg version ≥ 7.1.
- [ ] The corresponding source of each chosen build (FFmpeg source archive plus patches/build
      scripts of that build) has a URL and SHA-256 in `sources.lock`; a HEAD request returns 200.
- [ ] Tests: `fetch.sh` negative cases (wrong checksum via a temporary lock copy, unknown family)
      shown in the REPORT with their exit codes.
- [ ] Gates: shellcheck clean; no Go code changed.

## Notes
- If no prebuilt meets the criteria for a family (likely candidate: armv7), say so in the README
  and the REPORT (status DONE with the finding) and propose the follow-up "own static FFmpeg
  build for <family>" – the owner decides (OD-SPK-3). Do not start an own build in this task.
- The `fetch.sh` script is reused by the prebuild stage of SPK-07 (outside the toolkit chroot);
  keep its interface stable and let it take the lock file path from an optional third argument
  for tests.
- FFmpeg here is the server's transcoder and prober, bundled next to the server binary.
- Check x86_64 builds for an `-march` above x86-64 v1 (Celeron J-series NAS have no AVX) and
  armv7 builds for NEON if SPK-01 finds DSM 7.2 armv7 platforms without it.
- Hardware-acceleration features are irrelevant (software only, ADR 0026); smaller builds are
  welcome if they keep the encoders and demuxers above.
- Licence: the GPL variant is expected (libx264). The licence files go to
  `target/share/licenses/ffmpeg/` in SPK-07.
- Do not edit ADR 0026; findings go into the README and the REPORT.
