# syntax=docker/dockerfile:1
# The syntax line pins the Dockerfile frontend to the current 1.x release, which the
# here-documents of the RUN instructions below need.
#
# Step 1 of the Synology package build (ADR 0026 section 2, task SPK-07): prebuilds the server
# binary of every architecture family, takes FFmpeg from the own static build (SPK-15/SPK-16)
# and stages one Package Toolkit project per family with spkstage (SPK-14). Step 2, packing
# with the toolkit, needs `docker run` with CAP_SYS_ADMIN and is done by build.ps1 / build.sh
# next to this file, which also build the FFmpeg image first (step 0). From the repository root:
#
#   docker build -t spk-ffmpeg-static:dev packaging/synology/ffmpeg/build
#   docker build -f packaging/synology/Dockerfile --build-arg VERSION=0.1.0-alpha.2 --output type=local,dest=packaging/synology/dist/stage .
#
# The output holds x86_64/, armv8/ and armv7/ (staged projects, layout in
# packaging/synology/toolkit/README.md "Staged project"), ffmpeg-source/ (the FFmpeg
# corresponding source archive and its .sha256) and build-info.txt.
#
# Build arguments:
#   VERSION            server version (default 0.0.0-dev), mapped to the DSM version by spkstage
#   REVISION           git commit, recorded in build-info.txt (default empty; a release needs it)
#   PACKAGE_REVISION   packaging-only revision 0-9 of the same server version (default 0)
#   SOURCE_DATE_EPOCH  modification time of every staged file (default 0 = 1970-01-01); the
#                      local exporter also applies it to the exported files. BuildKit stamps
#                      the directories WORKDIR creates with a given value, so a new value
#                      rebuilds the web and Go stages (from their cache mounts); the FFmpeg
#                      image is built outside this file and is not affected.
#   RELEASE            true refuses an FFmpeg build whose SOURCE.txt is not "Status: selected"
#                      or whose licence is non-free, any spkstage warning (interim builds,
#                      dynamically linked FFmpeg), VERSION 0.0.0-dev and an empty REVISION; false
#                      (default) stages interim builds with spkstage's warning, for local test
#                      builds only. In both modes a SOURCE.txt needs exactly one Status: and one
#                      Licence: line (stage.sh).
#   NODE_IMAGE, GO_IMAGE  same names and defaults as packaging/docker/Dockerfile
#   FFMPEG_IMAGE       the image of the FFmpeg recipe's "all" target (default
#                      spk-ffmpeg-static:dev): /<family>/ with ffmpeg, ffprobe, the licence
#                      texts and SOURCE.txt, and /source/ with the corresponding source archive.
#                      It must exist locally (step 0 of build.sh / build.ps1 builds it); a
#                      missing tag makes BuildKit look it up in a registry, where no such image
#                      exists, and the build fails
# build-info.txt records all of these arguments and the status and licence of each family's
# FFmpeg build. build.sh / build.ps1 compare the staged binaries with the pins of
# packaging/synology/ffmpeg/sources.lock afterwards.
#
# The web and Go stages duplicate packaging/docker/Dockerfile on purpose (ADR 0026
# consequences): the container image and the packages build the same binary, but neither file
# depends on the other. Base images are pinned by tag, like there; digest pinning comes with CI.

ARG NODE_IMAGE=node:24-slim
# Must match the toolchain line of go.mod: the golang image sets GOTOOLCHAIN=local and never
# downloads a newer toolchain.
ARG GO_IMAGE=golang:1.27.3
ARG FFMPEG_IMAGE=spk-ffmpeg-static:dev

# ---------------------------------------------------------------------------------------------
# web: builds the browser app into internal/adapters/http/webui/dist (ADR 0020), exactly like
# the web stage of packaging/docker/Dockerfile.
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web

# corepack installs the pnpm version pinned by "packageManager" in web/package.json; the
# prompt would block a non-interactive build.
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    PNPM_STORE_DIR=/pnpm/store
RUN corepack enable

WORKDIR /src/web

COPY web/pnpm-lock.yaml web/pnpm-workspace.yaml web/package.json ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm fetch --store-dir "$PNPM_STORE_DIR"

# --prefer-offline, not --offline: the store lives in a cache mount that BuildKit may prune
# while the fetch layer stays cached (see packaging/docker/Dockerfile).
COPY web/ ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --prefer-offline --store-dir "$PNPM_STORE_DIR" \
 && pnpm build

# ---------------------------------------------------------------------------------------------
# go-sources: the Go module with its dependencies, shared by the server and spkstage builds.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go-sources

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# ---------------------------------------------------------------------------------------------
# server: cross-compiles the static server binary with the embedded web app for every family.
# The binary is named "server" here; spkstage stages it under the package name.
FROM go-sources AS server

COPY --from=web /src/internal/adapters/http/webui/dist/ internal/adapters/http/webui/dist/

ARG VERSION=0.0.0-dev

# CGO_ENABLED=0: the SQLite driver is pure Go, so the binary is static and cross-compiles.
# armv7 targets ARMv7 hard-float (GOARM=7; Go emits VFPv3-D16 code without NEON, which matches
# the armv7 target of ADR 0026 Notes). The module path is read from go.mod instead of being
# written here; ./cmd/mivo is a structural identifier on the rename checklist of ADR 0002.
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build <<'BUILD'
set -eu
module="$(go list -m)"
for target in x86_64:amd64: armv8:arm64: armv7:arm:7; do
	family="${target%%:*}"
	rest="${target#*:}"
	goarch="${rest%%:*}"
	goarm="${rest#*:}"
	echo "building the server for $family (GOARCH=$goarch${goarm:+, GOARM=$goarm})"
	CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" \
		go build -tags webui -trimpath -buildvcs=false \
		-ldflags "-s -w -X $module/internal/platform/buildinfo.version=$VERSION" \
		-o "/out/$family/server" ./cmd/mivo
done
BUILD

# ---------------------------------------------------------------------------------------------
# ffmpeg: FFmpeg, ffprobe, licence files and SOURCE.txt per family, plus the corresponding
# source archive. This is the one stage that decides where FFmpeg comes from: the own static
# build of packaging/synology/ffmpeg/build (SPK-15), built by build.sh / build.ps1 into the
# local image FFMPEG_IMAGE first (its "all" target: /<family>/{ffmpeg,ffprobe,SOURCE.txt,
# <licence files>} and /source/). BuildKit caches that build, so a second run reuses it. Until
# SPK-16 this stage ran ffmpeg/fetch.sh for the interim prebuilts.
FROM ${FFMPEG_IMAGE} AS ffmpeg

# ---------------------------------------------------------------------------------------------
# spkstage: the staging command of SPK-14, built for the build machine.
FROM go-sources AS spkstage

COPY packaging/synology/cmd/ packaging/synology/cmd/
COPY packaging/synology/internal/ packaging/synology/internal/
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /usr/local/bin/spkstage ./packaging/synology/cmd/spkstage

# ---------------------------------------------------------------------------------------------
# stage: one toolkit project per family from the templates, the binaries and FFmpeg.
FROM spkstage AS stage

COPY LICENSE ./
COPY packaging/synology/package/ packaging/synology/package/
COPY --from=server /out/ /prebuilt/server/
COPY --from=ffmpeg / /prebuilt/ffmpeg/

ARG VERSION=0.0.0-dev
ARG REVISION=""
ARG PACKAGE_REVISION=0
ARG SOURCE_DATE_EPOCH=0
ARG RELEASE=false
# The global image arguments, recorded in build-info.txt.
ARG NODE_IMAGE
ARG GO_IMAGE
ARG FFMPEG_IMAGE

# stage.sh (tested by test/build/stage.bats) reads the build arguments above from its
# environment. A release refuses the interim or non-free FFmpeg outright (BACKLOG "spk release",
# ADR 0026 section 4) and judges the SOURCE.txt that was staged, so the check holds whatever
# stage supplies FFmpeg.
COPY packaging/synology/stage.sh packaging/synology/stage.sh
RUN bash packaging/synology/stage.sh /prebuilt /src /stage

# ---------------------------------------------------------------------------------------------
# staged: the export of step 1 (last stage, so `docker build --output` needs no --target).
FROM scratch AS staged
COPY --from=stage /stage/ /
