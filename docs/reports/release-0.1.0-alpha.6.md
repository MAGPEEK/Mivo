# Release 0.1.0-alpha.6

Date: 2026-11-06. Release commit: `ee7031cc330e8b23b3c9b4132c0545dc044656d1` (annotated tag `v0.1.0-alpha.6`).

## Approval

Standing owner approval (2026-11-04, orchestrator releases itself). Version bump commit `ee7031c`
(docs/compose image references; the version itself is injected at build time) on top of `2a5bfbe`;
main did not move. No `latest` tag.

## Gates (clean clone of the tag)

- `go vet`, `go test ./...`, `golangci-lint run` (0 issues): green. `pnpm -C web install --frozen-lockfile`,
  `lint`, `test` (statements 98.94 %, branches 95.8 %), `build`: green.
- Corpus regenerated (`gen-testmedia -force`); `pnpm -C web e2e`: 59 passed, 0 skipped.
- `go vet -tags webui`, `go build -tags webui`: green. `go test -count=1 ./internal/adapters/corpustest/ ./cmd/...`: green.
- Image `mivo:dev` smoke (corpus at `/media/corpus`, discs at `/media/discs`, host port 18461, throwaway
  container): chromium-desktop 43/43, firefox-desktop 41/41, disc variant (`-disc-manifest`) 44/44;
  no session directories left.

## Docker

- `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.6` and `alpha`.
- Index digest `sha256:3c525af2d256d7d522e013bdee064b03e8fe1b9d1e1f7b506b70f9565abe765a`
  (amd64 `sha256:29991292...`, arm64 `sha256:8addda5d...`, plus two buildx attestation entries).
  `linux/arm/v7` not built (owner scope: amd64, arm64).
- Labels: source, version `0.1.0-alpha.6`, revision `ee7031c...`, licenses `GPL-3.0-or-later`.
- `docker run --rm --pull always docker.io/magpeek/mivo-media-server:alpha --version`: `Mivo 0.1.0-alpha.6 (go1.27.3)`
  (same for the exact tag).
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.6 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.6 -t <repo>:alpha --push .`

## Synology packages

`pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.6 -Release` from the clean clone
(FFmpeg recipe revision 4 `9.0.2-static4`). `inspect-spk.sh --release` (spk-inspect image): 95 checks,
0 failed, 2 warnings on each SPK. No emulated or hardware run of these exact files.

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.6 (pre-release).

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.6-x86_64.spk | 22435840 | a6500fda59ac365205365ce7d5f6b162b7d22cb40198e41ec1dcf6235171c06a |
| mivo-0.1.0-alpha.6-armv8.spk | 19927040 | f0829ea095e45da3b12e0cc72e1b2e249f8ae10230808fa12322e675c9d56dc9 |
| mivo-0.1.0-alpha.6-armv7.spk | 17377280 | 2bbb9670e41985cd6ecc721dd76b57b51b411f6784cdf43e4f675438f4fc7abf |
| ffmpeg-9.0.2-static4-source.tar | 168663040 | 79baa556c91a0c2bb2e6caae7dd84ce941370e43477423495969d6a3e4ecae89 |
| SHA256SUMS | 384 | covers the four files above |

The x86_64 SPK was copied to `Mivo-SPK-Test/2026-11-06-alpha.6/`; its SHA-256 matches `SHA256SUMS`.

## Deviations

- The Docker daemon was down at the start of the resumed session; Docker Desktop was started.
- The README line "verified with the `0.1.0-alpha.4` image" was set to alpha.5 in the previous release
  and is left unchanged here (not re-verified on alpha.6); all image references were bumped.
- First smoke attempt failed 43/43 because Git Bash rewrote the `/media/...` mount paths (empty library);
  rerun with `MSYS_NO_PATHCONV=1` passed. No release artifact was affected.
