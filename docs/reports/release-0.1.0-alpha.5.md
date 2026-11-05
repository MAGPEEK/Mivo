# Release 0.1.0-alpha.5

Date: 2026-11-05. Release commit: `da5752747ee3ee2219eee062d648d00b83346bdb` (annotated tag `v0.1.0-alpha.5`).

## Approval

Standing owner approval (2026-11-04, orchestrator releases itself); alpha.5 after milestone 3 (plan.md,
owner decision 2026-11-05). Version bump commit `da57527` (docs/compose image references; the version
itself is injected at build time) on top of `e2cdaf1`; main did not move. No `latest` tag.

## Gates (clean clone of the tag)

- `go vet`, `go test ./...`, `golangci-lint run` (0 issues): green. `pnpm -C web install --frozen-lockfile`,
  `lint`, `test` (statements 99.06 %, branches 95.66 %), `build`: green.
- Corpus regenerated (`gen-testmedia -force`); `pnpm -C web e2e`: 54 passed, 0 skipped.
- `go vet -tags webui`, `go build -tags webui`: green. `go test -count=1 ./internal/adapters/corpustest/ ./cmd/...`: green.
- Image `mivo:dev` smoke (corpus at `/media/corpus`, discs at `/media/discs`): chromium-desktop 30/30,
  firefox-desktop 28/28, disc variant (`-disc-manifest`) 31/31.

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.5` and `alpha`.
- Index digest `sha256:5de7439ba6100100d6747b4cf48aaaa19220840e95945ba9a58d2d23f052c1d6`
  (amd64 `sha256:9a6fcacf...`, arm64 `sha256:81c45f6f...`, plus two buildx attestation entries).
  `linux/arm/v7` not built (owner scope: amd64, arm64).
- `docker run --rm docker.io/magpeek/mivo-media-server:alpha --version`: `Mivo 0.1.0-alpha.5 (go1.27.3)`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.5 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.5 -t <repo>:alpha --push .`

## Synology packages

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.5 -Release` from the clean clone
(revision 0, Package Center `0.1.0-1050`, FFmpeg recipe revision 4 `9.0.2-static4`). `inspect-spk.sh
--release` (spk-inspect image): 95 checks, 0 failed, 2 warnings on each SPK. No emulated or hardware
run of these exact files (DS925+ test: TESTING.md incl. B6a).

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.5 (pre-release).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.5-x86_64.spk | 22425600 | 2e3ba48da183148490cfaf4f07f81442fb247da586da2aedf79d532cf6506325 |
| mivo-0.1.0-alpha.5-armv8.spk | 19906560 | 231b0d70b471afe1d45b34114a613e2a3e53f7eb4ae0577629a51dcb26a02888 |
| mivo-0.1.0-alpha.5-armv7.spk | 17367040 | 7726e50039f4a7673b386be941bcab129b40f9a51a2e308e0afec03746abdce3 |
| ffmpeg-9.0.2-static4-source.tar | 168663040 | 79baa556c91a0c2bb2e6caae7dd84ce941370e43477423495969d6a3e4ecae89 |
| SHA256SUMS | 384 | covers the four files above |

## Deviations

- The README line "verified with the `0.1.0-alpha.4` image" (unwritable `/transcode`, group access) was
  bumped by the version replace to alpha.5 without re-verifying it on the new image.
