# Release 0.1.0-alpha.10

Date: 2026-11-12. Release commit: `c122b8a77cfcce5989e0378d565d4d0c954337e0` (annotated tag `v0.1.0-alpha.10`).
Standing owner approval (CLAUDE.md release rule 6). HEAD == `origin/main`, tree clean before and after. No `latest` tag.

## Contents

Everything of alpha.9 (Docker-only): sign-in ON with first-run administrator setup (existing alpha catalogs are
kept), audio language switch, subtitles (sidecar WebVTT, image burn-in; text burn-in degrades with
`subtitle_burn_in_unavailable`), quality ladder and bitrate cap, trickplay, hardware encoder detection,
throttling, continue watching and up next, activity and re-analyze, virtualised library grid, artist and album pages.

Plus, new in this version: the **Synology SPKs** (x86_64, armv8, armv7) with the bundled FFmpeg rebuilt as recipe
`9.0.2-static5` (webvtt encoder, overlay and tile filters, image2 and webvtt muxers). Sidecar WebVTT and trickplay now
work on Synology; without a webvtt encoder the server answers 503 (degrade) instead of 422. This fixes the alpha.9
emulated-smoke failure (`subtitle-mkv-srt-ass+sidecar`).

## Known open items

Text burn-in not wired; `transcode.allow_burn_in` unwired; `transcode.hwaccel` has no runtime writer; DS925+ hardware
checkpoint M4-18 pending (SPKs are emulation-tested only, never run on the NAS). The `linux/arm/v7` Docker image is not
built (amd64, arm64 only). The armv7 H.264 SIGBUS under qemu cortex-a9 neon=off (BACKLOG spk-17) did NOT occur this run.

## Gates (clean clone of c122b8a, corpus generated with `gen-testmedia -force`)

- `go vet`, `go vet -tags webui`, `golangci-lint run` (and `--build-tags webui`): exit 0.
- `go test -count=1 ./...`: exit 0 (first attempt failed only because the corpus was not yet generated; rerun green).
  `go test -race -timeout 30m -tags webui ./internal/adapters/http/... ./cmd/...`: exit 0.
  corpustest `-v`: exit 0, 0 skipped.
- Web: `install --frozen-lockfile`, `lint`, `build`: exit 0; `test` 3521/3521, statements 98.66 %, branches 95.23 %.
- `pnpm -C web e2e` (after `pnpm -C web build`): 70 passed, 0 skipped.
- No load flakes occurred (none of the known ones).
- Docker smoke (`mivo:dev` from the clone, container `mivo-smoke`, `127.0.0.1:8460` free before, `testdata/media` read-only,
  account `smoke` via `MIVO_SMOKE_PASSWORD`, run from PowerShell): chromium-desktop 43/43, firefox-desktop 43/43.
  Container removed afterwards.

## Synology packages

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.10 -Release` from the clean clone. The staged
ffmpeg/ffprobe of all three families equal the pins in `sources.lock` (checked by hash; nothing edited). FFmpeg
`9.0.2-static5`. `inspect-spk.sh --release`: 95 checks, 0 failed, 2 warnings for each SPK.

Emulated Synology smoke (`smoke.sh`, Docker Desktop 29.9.2, with sign-in):

| Family | CPU model | Result |
|---|---|---|
| x86_64 (native) | - | 43 passed, 0 failed; chromium-desktop 43/43, firefox-desktop 43/43 incl. `subtitle-mkv-srt-ass+sidecar` and `video-3gp-h263-aac@2Mbit/s`; upgrade, reinstall, delete-data, alpha.2 upgrade all PASS |
| armv8 | cortex-a53 | 43 passed, 0 failed; chromium 43/43, firefox 43/43 |
| armv7 | cortex-a15 | 43 passed, 0 failed; chromium 43/43, firefox 43/43 |
| armv7 | cortex-a9 and cortex-a9,neon=off,vfp-d32=off | one run, 43 passed, 0 failed per model (no SIGBUS) |

QEMU proves instruction set and behaviour, not DSM's kernel or performance. armv8/armv7 are "tested under emulation only".

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.10` and `alpha`.
- Index digest `sha256:b89c0de64eb3bc4e4bc9378b735276413bd271d4628f2d32f0f1a946f73d63c8`;
  amd64 `sha256:a50d91c42da69080b8056d567ab8f59364de4430742f6a202f17d9b9db37be54`,
  arm64 `sha256:b442fa4d7bcb8be1af515336a66a0e3c4bb1665f10671f4a5f899987666678fd` (plus two buildx attestation entries).
- Compressed size (manifest config + layers): amd64 209463935 bytes, arm64 203636895 bytes.
- Labels: source `https://github.com/MAGPEEK/Mivo`, version `0.1.0-alpha.10`, revision
  `c122b8a77cfcce5989e0378d565d4d0c954337e0`, licenses `GPL-3.0-or-later`.
- `docker run --rm --pull always <repo>:0.1.0-alpha.10 --version` and `<repo>:alpha --version`:
  `Mivo 0.1.0-alpha.10 (go1.27.3)`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.10 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.10 -t <repo>:alpha --push .`
  (existing credential store, no `docker login`).

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.10 (pre-release, not a draft, 6 assets).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.10-x86_64.spk | 23183360 | 5d21a63efc9c02a020c58acefcc9a23f34a31315856bf2e6aaa6310f31126332 |
| mivo-0.1.0-alpha.10-armv8.spk | 20613120 | 91522bbde0dcaa2bc470ed71f62d65ef32a4fc48c18cf0419734d4f5d1d523a6 |
| mivo-0.1.0-alpha.10-armv7.spk | 18042880 | 347cbdfebd5672f125b756d04749bf0efde41754e4736018da8cb021802e78d2 |
| ffmpeg-9.0.2-static5-source.tar | 168673280 | 9b7c74c3698f89f6308c3775d689ab50b7747a8d5a750aa907e9e899077e8611 |
| ffmpeg-9.0.2-static5-source.tar.sha256 | 98 | one-line checksum of the tar |
| SHA256SUMS | 387 | covers the tar and the three SPKs |

The x86_64 SPK was copied to `C:\Users\stefa\Documents\Mivo-SPK-Test\2026-11-12-alpha.10\`; its SHA-256 matches the
downloaded release asset and `SHA256SUMS`.

## Deviations

- First `go test ./...` in the fresh clone failed one libraryfs test because the corpus did not exist yet; generated and rerun green.
- The pre-existing container `festive_murdock` (not mine) was left untouched. Smoke temp dirs live under the session scratchpad.
