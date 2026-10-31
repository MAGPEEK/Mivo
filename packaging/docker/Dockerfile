# syntax=docker/dockerfile:1
# The syntax line pins the Dockerfile frontend to the current 1.x release instead of the one
# built into the engine, so flags such as HEALTHCHECK --start-interval parse on every
# BuildKit version.
#
# Container image of the media server (ADR 0014). Build from the repository root:
#
#   docker build -f packaging/docker/Dockerfile -t mivo:dev .
#
# Build arguments: VERSION (release version, default 0.0.0-dev) and REVISION (git commit for
# the OCI label). The working title appears here on purpose: image labels, the binary path
# and the MIVO_* variables are structural identifiers on the rename checklist of ADR 0002.
#
# The builder stages run on the build machine ($BUILDPLATFORM) and cross-compile for the
# target, so a multi-arch build needs no emulation for Node or Go:
#
#   docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -f packaging/docker/Dockerfile .
#
# Base images are pinned by tag in milestone 1; digest pinning comes with CI and Renovate.

ARG NODE_IMAGE=node:24-slim
# Must match the toolchain line of go.mod: the golang image sets GOTOOLCHAIN=local and never
# downloads a newer toolchain.
ARG GO_IMAGE=golang:1.27.3
ARG RUNTIME_IMAGE=debian:stable-slim

# ---------------------------------------------------------------------------------------------
# web: builds the browser app into internal/adapters/http/webui/dist (ADR 0020).
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web

# corepack installs the pnpm version pinned by "packageManager" in web/package.json; the
# prompt would block a non-interactive build.
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    PNPM_STORE_DIR=/pnpm/store
RUN corepack enable

WORKDIR /src/web

# Fetching from the lockfile alone keeps the dependency layer cached until the lockfile
# changes, without listing every workspace package.json here.
COPY web/pnpm-lock.yaml web/pnpm-workspace.yaml web/package.json ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm fetch --store-dir "$PNPM_STORE_DIR"

# --prefer-offline, not --offline: pnpm fetch also fills node_modules/.pnpm in its layer, but
# the store lives in a cache mount that BuildKit may prune while that layer stays cached.
# Whatever the install then needs from the store is downloaded instead of failing with
# ERR_PNPM_NO_OFFLINE_TARBALL.
COPY web/ ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --prefer-offline --store-dir "$PNPM_STORE_DIR" \
 && pnpm build

# ---------------------------------------------------------------------------------------------
# go: cross-compiles the static server binary with the embedded web app.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/adapters/http/webui/dist/ internal/adapters/http/webui/dist/

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=0.0.0-dev

# CGO_ENABLED=0: the SQLite driver is pure Go, so the binary is static and cross-compiles.
# GOARM is only meaningful for 32-bit ARM, where TARGETVARIANT is "v6" or "v7".
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    if [ "$TARGETARCH" = "arm" ]; then export GOARM="${TARGETVARIANT#v}"; fi \
 && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -tags webui -trimpath -buildvcs=false \
      -ldflags "-s -w -X github.com/magpeek/mivo/internal/platform/buildinfo.version=${VERSION}" \
      -o /out/mivo ./cmd/mivo

# ---------------------------------------------------------------------------------------------
# runtime: Debian with FFmpeg. Debian's ffmpeg package is the interim build until
# jellyfin-ffmpeg arrives in milestone 4 or 9 (ADR 0015).
FROM ${RUNTIME_IMAGE} AS runtime

# ARG, not ENV: it silences debconf prompts during the build only and does not leak into the
# environment of the running container.
ARG DEBIAN_FRONTEND=noninteractive

# Package versions are not pinned: stable-slim moves with Debian point releases and pinned
# versions would vanish from the archive. Digest pinning of the base image comes with CI.
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates ffmpeg tzdata \
 && rm -rf /var/lib/apt/lists/*

# Docker copies ownership and mode of these directories into new named or anonymous volumes,
# so the default user can write /config and /transcode without a bind mount. /media is read
# by the server and stays owned by root. Bind mounts keep the ownership of the host directory
# and must be writable by the user the container runs as (see packaging/docker/README.md).
RUN install -d -m 0750 -o 1000 -g 1000 /config /transcode \
 && install -d -m 0755 /media

COPY --from=go /out/mivo /usr/local/bin/mivo

ARG VERSION=0.0.0-dev
ARG REVISION=""
LABEL org.opencontainers.image.title="Mivo" \
      org.opencontainers.image.description="Open-source media server that plays practically any format" \
      org.opencontainers.image.source="https://github.com/MAGPEEK/Mivo" \
      org.opencontainers.image.url="https://github.com/MAGPEEK/Mivo" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# Paths of ADR 0018 for Docker: the server does not compute them, the image sets them.
# MIVO_LIBRARIES makes the /media mount the default library (OD-1), so "docker run -v
# <videos>:/media:ro" works without a config file. The variable replaces the libraries of
# /config/config.yaml as a whole (ADR 0018); packaging/docker/README.md shows how to set
# several libraries or use the file instead.
ENV MIVO_CONFIG_DIR=/config \
    MIVO_PATHS_DATA=/config/data \
    MIVO_PATHS_CACHE=/config/cache \
    MIVO_PATHS_TRANSCODE=/transcode \
    MIVO_LIBRARIES=/media

# Numeric, so Kubernetes' runAsNonRoot can verify it without a passwd entry. Override with
# --user or compose "user:" to match the owner of the mounted directories on a NAS.
USER 1000:1000

VOLUME ["/config", "/media", "/transcode"]
EXPOSE 8460

# start-interval probes every 2 s while the server starts, so the container turns healthy
# within seconds. Parsing the flag needs Dockerfile frontend 1.6+, which the syntax line at
# the top provides; without it, older built-in frontends fail the build on the unknown flag.
# At run time Docker Engine 25+ honours it; older engines ignore it and use the interval.
# The timeout is above the probe's own 3 s limit, so a slow probe reports its error.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --start-interval=2s --retries=3 \
    CMD ["/usr/local/bin/mivo", "healthcheck"]

# No init process (tini) is needed: the server is the only process, handles SIGTERM and
# SIGINT itself as PID 1 and waits for every child it starts. After the first signal it
# restores the default handling, which the kernel ignores for PID 1, so a second Ctrl+C in
# an interactive "docker run -it" does nothing; add "--init" there to make it terminate.
ENTRYPOINT ["/usr/local/bin/mivo"]
