# SPK-16 Switch the SPK build to the own FFmpeg build
Owner: mivo-synology
Depends on: SPK-07, SPK-15
Files/areas: `packaging/synology/ffmpeg/sources.lock`, `packaging/synology/ffmpeg/fetch.sh`,
`packaging/synology/ffmpeg/README.md` ("Result" section), `packaging/synology/Dockerfile`,
`packaging/synology/build.ps1`, `packaging/synology/build.sh`

## Goal
The server SPKs bundle the own static FFmpeg of SPK-15 instead of the interim prebuilts, and the
build produces the FFmpeg corresponding source archive next to the SPKs, so a release can ship.

## Acceptance criteria
- [ ] `build.ps1` / `build.sh` (SPK-07) obtain FFmpeg for each family from SPK-15's recipe (built
      once and cached by BuildKit), not from `fetch.sh` downloads; the staged project gets
      `ffmpeg`, `ffprobe`, licence files and `SOURCE.txt` with status `selected`.
- [ ] `sources.lock` no longer contains an `interim` entry (`grep -q '^[^#][^ ]* interim ' packaging/synology/ffmpeg/sources.lock`
      fails); its format and header describe the own build (or the file is replaced by SPK-15's
      source list, with `fetch.sh` removed or kept only for the interim history – README says which).
- [ ] `packaging/synology/dist/` additionally holds the FFmpeg corresponding source archive from
      SPK-15 and its line in `SHA256SUMS`.
- [ ] `inspect-spk.sh --release` (SPK-10) passes on all three SPKs (no interim, FFmpeg version and
      checksums as in SPK-15's `SOURCE.txt`).
- [ ] Unpacking each SPK and running `bin/ffmpeg -version` under emulation (`--platform` of the
      family) shows the own build's version and configure line.
- [ ] A second build reuses the cached FFmpeg (time in the REPORT).
- [ ] Gates: shellcheck on changed scripts; script tests and Go gates unchanged green; the build
      succeeds on Docker Desktop.

## Notes
- After this task SPK-11, SPK-12 and SPK-13 run with the own FFmpeg; earlier SPK-11 runs with
  interim builds count only as pipeline checks.
- Keep `SOURCE.txt` in `target/share/licenses/ffmpeg/` (SPK-14 layout) so SPK-10 can read the
  status from the package.
