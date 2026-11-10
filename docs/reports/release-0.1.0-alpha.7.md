# Release 0.1.0-alpha.7

Date: 2026-11-10. Release commit: `5038d2ddf1b4bec664e430bd1a5a9519ca5ae08b` (annotated tag `v0.1.0-alpha.7`).

## Approval

Standing owner approval (2026-11-04, orchestrator releases itself). Version bump commit `5038d2d`
(docs/compose image references; the version itself is injected at build time); HEAD == `origin/main`,
working tree clean before and after. No `latest` tag.

## Gates (clean clone of the release commit)

- `go vet`, `go test ./...`, `golangci-lint run` (0 issues): green. `pnpm -C web install --frozen-lockfile`,
  `lint`, `test` (statements 98.7 %, branches 95.13 %), `build`: green.
- Corpus regenerated (`gen-testmedia -force`); `pnpm -C web e2e`: 59 passed, 0 skipped.
- `go vet -tags webui`, `go build -tags webui`: green. `go test -count=1 ./internal/adapters/corpustest/ ./cmd/...`: green.
- Image `mivo:dev` (built from the clone with `VERSION=0.1.0-alpha.7`) smoke: container `mivo-smoke` on
  `127.0.0.1:8460` (port free beforehand), `testdata/media` mounted read-only at `/media`:
  chromium-desktop 43/43, firefox-desktop 41/41. Container and its volumes removed afterwards.

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.7` and `alpha`.
- Index digest `sha256:6eda89a9834a4c7977b3d22782a7ce72319b8ef5e8c20fde68c9edd8a3c9e14e`
  (amd64 `sha256:e17f56b5ae3b0240397429d7f8e19caaecf8cd7edacd534197a102b9f97ee203`,
  arm64 `sha256:793183c97645184bc16689a22bdad35d46bc31d1be0bd36d000f8d8626edada2`, plus two buildx
  attestation entries). `linux/arm/v7` not built (owner scope: amd64, arm64, as in alpha.6).
- Compressed size (manifest config + layers): amd64 208852279 bytes, arm64 203084481 bytes.
- Labels: source `https://github.com/MAGPEEK/Mivo`, version `0.1.0-alpha.7`, revision `5038d2d...`,
  licenses `GPL-3.0-or-later`.
- `docker run --rm --pull always docker.io/magpeek/mivo-media-server:0.1.0-alpha.7 --version`:
  `Mivo 0.1.0-alpha.7 (go1.27.3)` (same for `alpha`).
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.7 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.7 -t <repo>:alpha --push .`

## Synology packages

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.7 -Release` from the clean clone
(FFmpeg recipe revision 4 `9.0.2-static4`, source archive identical to alpha.6). `inspect-spk.sh --release`
(spk-inspect image): 95 checks, 0 failed, 2 warnings on each SPK. No emulated or hardware run of these
exact files.

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.7 (pre-release, not a draft).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.7-x86_64.spk | 22691840 | fd28785d226722e676d18b2cb002770155491a177b111d52aa51ccf95993b8ee |
| mivo-0.1.0-alpha.7-armv8.spk | 20152320 | 112570610867fbd34da1cb9adc60db23fd7ace8b0f7c6ebe3d9aed1d2d371df2 |
| mivo-0.1.0-alpha.7-armv7.spk | 17602560 | ad3569c218bbe6e6de66989cc99d10b33d1ee046faead5b22d289d76e65a2927 |
| ffmpeg-9.0.2-static4-source.tar | 168663040 | 79baa556c91a0c2bb2e6caae7dd84ce941370e43477423495969d6a3e4ecae89 |
| SHA256SUMS | 384 | covers the four files above |

The x86_64 SPK was copied to `Mivo-SPK-Test/2026-11-10-alpha.7/`; its SHA-256 matches the downloaded
release asset and `SHA256SUMS`.

## Deviations

- The Docker daemon was down at the start; Docker Desktop was started (no `docker login`; the existing
  credential-store entry for `magpeek` was used).
- Smoke ran as the orchestrator specified (port 8460, `testdata/media` at `/media`, two profiles); the
  disc variant (`-disc-manifest`) of alpha.6 was not run.
- The README line "verified with the `0.1.0-alpha.N` image" was not re-verified for alpha.7.
