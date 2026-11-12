# Release 0.1.0-alpha.9 (PARTIAL: Docker image only)

Date: 2026-11-12. Release commit: `18b812e1f2995c2b059006fa8ddd58f1c77eb1a3`. Standing owner approval
(CLAUDE.md release rule 6). HEAD == `origin/main`, tree clean. No `latest` tag.

**Status:** the Docker image is published. The SPKs, the git tag `v0.1.0-alpha.9` and the GitHub
pre-release are NOT published: the emulated Synology smoke (a real gate) failed, see below.

## Contents

Sign-in ON with first-run administrator setup (existing alpha catalogs are kept); audio language switch in
the player; subtitles (sidecar WebVTT, image burn-in; text burn-in intentionally degrades with
`subtitle_burn_in_unavailable`); quality ladder and bitrate cap; trickplay; hardware encoder detection
(DS925+ has no iGPU: software); throttling; continue watching and up next; activity and re-analyze (admin);
virtualised library grid with A-Z bar; artist and album pages.

Known open items: `transcode.allow_burn_in` unwired, `transcode.hwaccel` has no runtime writer, text
burn-in not wired, DS925+ checkpoint M4-18 pending. `linux/arm/v7` image not built.

## Gates (clean clone of the release commit, corpus generated)

- `go vet`, `go vet -tags webui`, `golangci-lint run` (and `--build-tags webui`): exit 0.
- `go test -count=1 ./...`: exit 0. `go test -race -timeout 30m -tags webui ./internal/adapters/http/... ./cmd/...`: exit 0.
- Web: install `--frozen-lockfile`, lint, build: exit 0. `pnpm -C web test`: first run 1 failure,
  `PlayerPage.overlay.test.tsx` "shows a direct-play session in green" (Buffer ahead "0 s" instead of
  "unknown", load flake); alone 17/17, full rerun 3521/3521, statements 98.66 %, branches 95.23 %.
- `pnpm -C web e2e` (build first): 70 passed, 0 skipped. corpustest: 0 skipped, pass.
- Docker smoke (`mivo:dev`, container `mivo-smoke`, `127.0.0.1:8460`, account `smoke` through
  `MIVO_SMOKE_PASSWORD`, first-run setup performed by the smoke): chromium-desktop 43/43, firefox-desktop 43/43.
- No other known load flakes occurred.

## Docker (published)

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.9` and `alpha`.
- Index digest `sha256:e51655f9251372f706f5209db879b9e9e8c82a370cdeea9991ed66d4ca2121dd`;
  amd64 `sha256:efd9886fdada95002fa51040e62d804e7d5e044f93ba5ca95cded2f1a9000b88`,
  arm64 `sha256:647e5581deb52b20f4aa93f7ff4c2d9ee436cc24dffb61393e67caecdee27bea` (plus two buildx attestations).
- Labels: source `https://github.com/MAGPEEK/Mivo`, version `0.1.0-alpha.9`, revision `18b812e1...`,
  licenses `GPL-3.0-or-later`.
- `docker run --rm --pull always <repo>:0.1.0-alpha.9 --version` and `<repo>:alpha --version`:
  `Mivo 0.1.0-alpha.9 (go1.27.3)`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.9 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.9 -t <repo>:alpha --push .`
- The Docker README documents the first-admin warning and `MIVO_SERVER_TRUSTED_PROXIES` (not re-verified here).

## Synology packages: NOT published

Built with `pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.9 -Release`
(FFmpeg `9.0.2-static4`; source tar sha256 `79baa556...cae89`, identical to alpha.8). `inspect-spk.sh
--release`: 95 checks, 0 failed, 2 warnings (x86_64 run in the smoke; the other two were not inspected
separately). Built SPK SHA-256 (not released):

- x86_64 `c7a12ee7665e01fc648f5a0aa5902cc348ee12307887028767a381352f7d8a32`
- armv8 `832437a4d48d74e4ff74b741e8ec6254509a134fb7c4fbbcd9edcb2835772d7a`
- armv7 `bce2ef9b180163729f250ce2add430c06748c83435758a8c271c49c65168ea90`

Emulated smoke `smoke.sh x86_64` (Docker Desktop 29.9.2): 42 passed, 1 failed (reproduced on a second run).
PASS: install, versions, start/status, first-run setup through the published port, chromium-desktop 43/43,
stop, upgrade, sign-in after upgrade (catalogue kept, 38 items, 0 added), uninstall/reinstall with sign-in
and catalogue kept, delete-data uninstall, alpha.2-shaped upgrade. FAIL: firefox-desktop case
`subtitle-mkv-srt-ass+sidecar`: `GET .../subtitles/s:2.vtt` answers 422 unsupported.

Root cause (reproduced outside the harness): the bundled FFmpeg `9.0.2-static4` has NO subtitle encoders
(`ffmpeg -encoders` lists no `webvtt`; `-c:s webvtt` fails with "Unknown encoder 'webvtt'"), only the
webvtt muxer and subtitle decoders. So sidecar subtitle conversion cannot work on any Synology SPK. Fix
needed in `packaging/synology/ffmpeg` (enable the webvtt encoder, new recipe revision static5 or later), rebuild
the three SPKs under a new version. armv8 and armv7 emulated runs were not done (same FFmpeg recipe).

## Not done

Tag `v0.1.0-alpha.9`, GitHub pre-release, assets, copy to `Mivo-SPK-Test/2026-11-12-alpha.9/`. The Docker image
`alpha.9` is therefore a Docker-only version; the SPKs follow as the next version (a published version is never
changed).
