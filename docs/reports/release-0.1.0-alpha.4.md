# Release 0.1.0-alpha.4

Date: 2026-11-05. Release commit: `59d787f6a7f7550b0ac464f18826993fff802aaf` (annotated tag `v0.1.0-alpha.4`).

## Approval and deviation

Standing owner approval (2026-11-04, orchestrator releases itself). The version bump commit `c9a5497`
(docs/compose image references; the version itself is injected at build time) was gated, but main moved
before tagging, so the tag landed on `59d787f`, which adds M6-01 (API 1.4.0, metadata endpoints answering
501). The orchestrator decided to release `59d787f`; all gates were rerun on a clean clone of the tag.
No `latest` tag.

## Gates (clean clone of the tag)

- `go vet`, `go test ./...`, `golangci-lint run`: green. `pnpm -C web lint`, `test` (1518 passed, 99.29 %),
  `build`, `e2e` (31 passed, 7 skipped): green. `go vet`/`go build -tags webui`: green.
- Image `mivo:dev` smoke (`MIVO_LIBRARIES=/media`): chromium-desktop 30/30, firefox-desktop 28/28.

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.4` and `alpha`.
- Index digest `sha256:974ad5d2f592035baf5b1473ac264a11bd64aa739d13cab27c663d6f6b4fa6e3`
  (amd64 `sha256:0fe1da4a...`, arm64 `sha256:23994410...`, plus two buildx attestation entries).
  `linux/arm/v7` not built (owner scope: amd64, arm64).
- `docker run --rm docker.io/magpeek/mivo-media-server:alpha --version`: `Mivo 0.1.0-alpha.4 (go1.27.3)`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.4 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.4 -t <repo>:alpha --push .`

## Synology packages

Built from the clean clone: `pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.4
-Release` (revision 0, Package Center `0.1.0-1040`). `inspect-spk.sh --release` (spk-inspect image): 95
checks, 0 failed, 2 warnings on each SPK. SPK-11 emulated smoke and a hardware run were not repeated for
these exact files.

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.4 (pre-release).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.4-x86_64.spk | 22097920 | ac5ee18e08c44945024a1fd6dc997b07dd1d75465772c88b374a38ba426307d3 |
| mivo-0.1.0-alpha.4-armv8.spk | 19568640 | a0e8f47263c5e0ed6264f54a7e0cb1c232ced48a1c94cb96fea661139b0f6803 |
| mivo-0.1.0-alpha.4-armv7.spk | 17039360 | 50d8374b55790a8d28330e9986183e452cee0ef53c747ca5e05d45cd65361969 |
| ffmpeg-9.0.2-static2-source.tar | 168652800 | 69c02f6a45048f8adef510eb1063f41316d4d16ba51dda8c2f19750938c223fc |
| SHA256SUMS | 384 | covers the four files above |
