# 0014 Docker-first distribution for NAS

- Status: Accepted
- Date: 2026-10-29
- Source: decision table and build section in `docs/plan.md`

## Context

Synology DSM and QNAP both support containers. Native SPK and QPKG packages are costly to
build and maintain.

## Decision

- The primary NAS distribution is a **multi-arch Docker image** (linux/amd64, linux/arm64,
  linux/armv7) based on `debian:stable-slim` with FFmpeg bundled (ADR 0015).
- Volumes: `/config`, `/media`, `/transcode`. Optional `--device /dev/dri`.
- The container never runs as root. Milestone 1 uses a fixed non-root UID/GID that users can
  override with `--user`. `PUID`/`PGID` support (entrypoint privilege drop) comes in
  milestone 9 together with the compose files for Synology Container Manager and QNAP
  Container Station.
- The Dockerfile lives in `packaging/docker/`. The build context is the repository root.
  Builder stages are cross-compile ready (`$BUILDPLATFORM`, `TARGETARCH`, `TARGETVARIANT`).
- Native SPK/QPKG packages come later. Windows installer, deb/rpm and GoReleaser come in
  milestone 9.

## Consequences

- One artifact covers most NAS models.
- Hardware transcoding depends on the host exposing `/dev/dri`. This is documented per NAS
  model.

## Notes (2026-10-31)

- The project owner decided to start the native **Synology** package now, in parallel to
  milestone 2; the first SPK ships with 0.1.0-alpha.2. ADR 0026 (Accepted) records its design.
  Docker stays the primary NAS distribution; QPKG stays "later".
