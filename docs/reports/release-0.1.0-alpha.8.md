# Release 0.1.0-alpha.8

Date: 2026-11-12. Release commit: `27e5f155e28fa92b755fd44afcfcedaa254b931d` (annotated tag `v0.1.0-alpha.8`).

## Approval

Standing owner approval (CLAUDE.md release rule 6, orchestrator releases itself). HEAD == `origin/main`,
working tree clean before and after. No `latest` tag.

## Contents

Account/profile building blocks, resume/watched UI (inactive until M7-23), new playback contract,
server-side subtitle decision, bitrate ladder and hardware encoder logic. NOT the audio language
switch (alpha.9).

## Gates (clean clone of the release commit)

- `go vet`, `go test ./...`, `golangci-lint run` (0 issues): green, no load flakes this run.
  `pnpm -C web install --frozen-lockfile`, `lint`, `test` (statements 98.69 %, branches 95.51 %), `build`: green.
- Corpus regenerated (`gen-testmedia -force`); `pnpm -C web e2e`: 61 passed.
- `go vet -tags webui`, `go build -tags webui`: green. `go test -count=1 ./internal/adapters/corpustest/ ./cmd/...`: green.
- Image `mivo:dev` (clone, `VERSION=0.1.0-alpha.8`) smoke, container `mivo-smoke` on `127.0.0.1:8460`
  (free beforehand), `testdata/media` read-only at `/media`: chromium-desktop 43/43, firefox-desktop 41/41.
  Container and volumes removed afterwards.

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.8` and `alpha`.
- Index digest `sha256:13ff526c20490431fb0496599bc108ecdf235b73734aca04ad00cddfc20d499e`
  (amd64 `sha256:95b698abbc8c5f963f35b96cb56638f0cb9ad3eaa9f7818a2f3861e922582cd7`,
  arm64 `sha256:fba1a7741925629a6cf10ca1ec3940671644d9130a291c5dfa745d460b473bc3`, plus two buildx
  attestation entries). `linux/arm/v7` not built (owner scope: amd64, arm64).
- Compressed size (manifest config + layers): amd64 208953061 bytes, arm64 203180265 bytes.
- Labels: source `https://github.com/MAGPEEK/Mivo`, version `0.1.0-alpha.8`, revision `27e5f155...`,
  licenses `GPL-3.0-or-later`.
- `docker run --rm --pull always <repo>:0.1.0-alpha.8 --version` and `<repo>:alpha --version`:
  `Mivo 0.1.0-alpha.8 (go1.27.3)`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.8 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.8 -t <repo>:alpha --push .`

## Synology packages

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.8 -Release` from the clean clone
(FFmpeg `9.0.2-static4`, source archive identical to alpha.7). `inspect-spk.sh --release` (spk-inspect
image): 95 checks, 0 failed, 2 warnings on each SPK. No emulated or hardware run of these exact files.

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.8 (pre-release, not a draft, 5 assets).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.8-x86_64.spk | 22753280 | 7d40343fa7d58ae50429629d08dd426d7cc9e34a56b2060b00c3fc8dcf666822 |
| mivo-0.1.0-alpha.8-armv8.spk | 20224000 | 4f2165ba74346e33224d304f97522b666900a90922c353a963efb5b08ef1d054 |
| mivo-0.1.0-alpha.8-armv7.spk | 17684480 | ff24968c25e6d16f49ddef0b437e554e22fe8f843d82f00e0325f654fe675a4f |
| ffmpeg-9.0.2-static4-source.tar | 168663040 | 79baa556c91a0c2bb2e6caae7dd84ce941370e43477423495969d6a3e4ecae89 |
| SHA256SUMS | 384 | covers the four files above |

The x86_64 SPK was copied to `Mivo-SPK-Test/2026-11-12-alpha.8/`; its SHA-256 matches the downloaded
release asset and `SHA256SUMS`.

## Deviations

- The Docker daemon was down at the start; Docker Desktop was started (no `docker login`; existing
  credential-store entry used).
- A first smoke attempt from Git Bash failed 43/41 cases because the `$PWD` mount path was empty in the
  container ("0 file names catalogued"); rerun from PowerShell with the Windows path passed. Not a product defect.
- The disc variant (`-disc-manifest`) smoke was not run, as in alpha.7.
