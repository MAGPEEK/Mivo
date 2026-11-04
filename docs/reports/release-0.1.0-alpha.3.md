# Release 0.1.0-alpha.3

Date: 2026-11-04. Commit/tag: `40801206a4b87d416c75994300f99a43bc924551` (annotated tag `v0.1.0-alpha.3`).

## Approval

Owner, 2026-11-04: "Ja und auch gleich pushen für Github und Dockerhub" – covers the GitHub tag and
pre-release and the Docker Hub push of exactly this version. Owner decision: the SPKs are published
under 0.1.0-alpha.3 (alpha.2 untouched). No `latest` tag.

## Gates (release commit)

- `go vet ./...`, `go test ./...`: green. `pnpm -C web lint`, `test` (coverage 99.9 %), `build`: green.
- Docker image built from the tree (`mivo:dev`); `playback-smoke` default 30/30, `firefox-desktop` 28/28.
- Same smoke against the pulled `docker.io/magpeek/mivo-media-server:0.1.0-alpha.3`: 30/30 and 28/28.

## Docker

- Repository: `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.3` and `alpha`.
- Index digest: `sha256:b1adf3d766f81922f9cb6d2a1d6d4d98e431213dbdcc915f4f62a9425ff48e5f`
  (amd64 `sha256:3ab3cdfd...`, arm64 `sha256:48932de3...`; two further "unknown/unknown" entries are
  buildx attestations). `linux/arm/v7` not built (owner scope: amd64, arm64).
- Local image size (amd64, uncompressed): 773 MB. `docker run --rm <image> --version` prints
  `Mivo 0.1.0-alpha.3 (go1.27.3)`.
- Labels: source, url, title, description, version `0.1.0-alpha.3`, revision
  `40801206a4b87d416c75994300f99a43bc924551`, licenses `GPL-3.0-or-later`.
- Command: `docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64
  --build-arg VERSION=0.1.0-alpha.3 -f packaging/docker/Dockerfile
  --label org.opencontainers.image.revision=<HEAD> -t <repo>:0.1.0-alpha.3 -t <repo>:alpha --push .`

## Synology packages

Built from a clean clone of tag `v0.1.0-alpha.3` (the working tree had the untracked
`.claude/worktrees/`, which `-Release` refuses): `pwsh -NoProfile -File packaging/synology/build.ps1
-Version 0.1.0-alpha.3 -Release` (package revision 0, Package Center version `0.1.0-1030`).
`inspect-spk.sh --release` (spk-inspect image): 93 checks, 0 failed, 2 accepted warnings (ffmpeg/ffprobe
ABI note) on each SPK. SPK-11 `smoke.sh`: x86_64 28 passed, armv8 28 passed, armv7 82 passed (three CPU
models), 0 failed. No SPK-12 hardware run for this exact file (x86_64 hardware test was on DS925+, DSM 7.4.1,
earlier build).

## GitHub pre-release

https://github.com/MAGPEEK/Mivo/releases/tag/v0.1.0-alpha.3 (pre-release). Downloaded x86_64 SPK matches
`SHA256SUMS`.

| Asset | Size (bytes) | SHA-256 |
|---|---|---|
| mivo-0.1.0-alpha.3-x86_64.spk | 21473280 | 87cb6eb7d67e5479f6ed635ab18bb77bf1a80312a25da75091525fe3276fe69b |
| mivo-0.1.0-alpha.3-armv8.spk | 19005440 | d48cebdc44e2d054635518d6443140d0c97c0e7fa7f8e7bfe6eaab762a16e95c |
| mivo-0.1.0-alpha.3-armv7.spk | 16506880 | 8e61decc9421a07ad8f1a8323d23fbba9872b8942e4792a99186d16c6d4aa5b8 |
| ffmpeg-9.0.2-static1-source.tar | 168652800 | 0bc6950f255d83e297c504bd6ea01a35ce90538052e1f06638aa80ea04ef1349 |
| SHA256SUMS | 384 | covers the four files above |

Notes: changes since alpha.2, Docker pull lines and a "Synology packages (native, DSM 7.2+)" section
(table per SPK, model lookup, install steps, FFmpeg source and GPL note, Container Manager advice).
