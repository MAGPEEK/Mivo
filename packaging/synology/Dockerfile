# syntax=docker/dockerfile:1
# The syntax line pins the Dockerfile frontend to the current 1.x release, which the
# here-documents of the RUN instructions below need.
#
# Step 1 of the Synology package build (ADR 0026 section 2, task SPK-07): prebuilds the server
# binary and FFmpeg of every architecture family and stages one Package Toolkit project per
# family with spkstage (SPK-14). Step 2, packing with the toolkit, needs `docker run` with
# CAP_SYS_ADMIN and is done by build.ps1 / build.sh next to this file. From the repository root:
#
#   docker build -f packaging/synology/Dockerfile --build-arg VERSION=0.1.0-alpha.2 --output type=local,dest=packaging/synology/dist/stage .
#
# The output holds x86_64/, armv8/ and armv7/ (staged projects, layout in
# packaging/synology/toolkit/README.md "Staged project") and build-info.txt.
#
# Build arguments:
#   VERSION            server version (default 0.0.0-dev), mapped to the DSM version by spkstage
#   REVISION           git commit, recorded in build-info.txt (default empty)
#   PACKAGE_REVISION   packaging-only revision 0-9 of the same server version (default 0)
#   SOURCE_DATE_EPOCH  modification time of every staged file (default 0 = 1970-01-01); the
#                      local exporter also applies it to the exported files. BuildKit stamps
#                      the directories WORKDIR creates with a given value, so a new value
#                      rebuilds the web and Go stages (from their cache mounts); the FFmpeg
#                      stage has no WORKDIR and its download stays cached.
#   RELEASE            true refuses an FFmpeg build whose SOURCE.txt is not "Status: selected"
#                      or whose licence is non-free, any spkstage warning (interim builds,
#                      dynamically linked FFmpeg) and VERSION 0.0.0-dev; false (default) stages
#                      interim builds with spkstage's warning, for local test builds only. In
#                      both modes a SOURCE.txt needs exactly one Status: and one Licence: line.
#   NODE_IMAGE, GO_IMAGE  same names and defaults as packaging/docker/Dockerfile
#   FFMPEG_FETCH_IMAGE    image that runs ffmpeg/fetch.sh (default alpine:3)
# build-info.txt records all of these arguments and the status and licence of each family's
# FFmpeg build.
#
# The web and Go stages duplicate packaging/docker/Dockerfile on purpose (ADR 0026
# consequences): the container image and the packages build the same binary, but neither file
# depends on the other. Base images are pinned by tag, like there; digest pinning comes with CI.

ARG NODE_IMAGE=node:24-slim
# Must match the toolchain line of go.mod: the golang image sets GOTOOLCHAIN=local and never
# downloads a newer toolchain.
ARG GO_IMAGE=golang:1.27.3
ARG FFMPEG_FETCH_IMAGE=alpine:3

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
# ffmpeg: FFmpeg, ffprobe, licence files and SOURCE.txt per family. This is the one stage that
# decides where FFmpeg comes from: today the interim pins of sources.lock through fetch.sh
# (SPK-02); SPK-16 replaces this stage with the own static build (SPK-15) and keeps its output
# layout, /out/<family>/{ffmpeg,ffprobe,SOURCE.txt,<licence files>}.
FROM --platform=$BUILDPLATFORM ${FFMPEG_FETCH_IMAGE} AS ffmpeg

# fetch.sh needs sha256sum (busybox), tar, xz and curl: with curl it downloads with
# --proto '=https', so a redirect can never fall back to plain HTTP (busybox wget has no such
# option). Versions are not pinned: alpine:3 moves with its point releases (as
# packaging/docker/Dockerfile does for Debian); the downloads themselves are pinned by SHA-256
# in sources.lock.
# hadolint ignore=DL3018
RUN apk add --no-cache curl xz

COPY packaging/synology/ffmpeg/fetch.sh packaging/synology/ffmpeg/sources.lock /fetch/
# The layer stays cached until fetch.sh or sources.lock change, so a second build downloads
# nothing.
RUN for family in x86_64 armv8 armv7; do sh /fetch/fetch.sh "$family" "/out/$family" || exit 1; done

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
COPY --from=ffmpeg /out/ /prebuilt/ffmpeg/

ARG VERSION=0.0.0-dev
ARG REVISION=""
ARG PACKAGE_REVISION=0
ARG SOURCE_DATE_EPOCH=0
ARG RELEASE=false
# The global image arguments, recorded in build-info.txt.
ARG NODE_IMAGE
ARG GO_IMAGE
ARG FFMPEG_FETCH_IMAGE

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# A release refuses the interim FFmpeg outright (BACKLOG "spk release", ADR 0026 section 4):
# the check reads the SOURCE.txt that was staged, so it holds whatever stage supplies FFmpeg.
RUN <<'STAGE'
set -euo pipefail
die() { echo "stage: $*" >&2; exit 1; }
case "$RELEASE" in true | false) ;; *) die "RELEASE must be true or false, not '$RELEASE'" ;; esac
[ "$RELEASE" = false ] || [ "$VERSION" != 0.0.0-dev ] || die "a release needs VERSION, not the default 0.0.0-dev"
mkdir -p /stage
{
	echo "version=$VERSION"
	echo "revision=$REVISION"
	echo "package_revision=$PACKAGE_REVISION"
	echo "source_date_epoch=$SOURCE_DATE_EPOCH"
	echo "release=$RELEASE"
	echo "node_image=$NODE_IMAGE"
	echo "go_image=$GO_IMAGE"
	echo "ffmpeg_fetch_image=$FFMPEG_FETCH_IMAGE"
} >/stage/build-info.txt
for family in x86_64 armv8 armv7; do
	warnings="/tmp/spkstage-$family.log"
	if ! spkstage -family "$family" -version "$VERSION" -package-revision "$PACKAGE_REVISION" \
		-source-date-epoch "$SOURCE_DATE_EPOCH" -binary "/prebuilt/server/$family/server" \
		-ffmpeg-dir "/prebuilt/ffmpeg/$family" -repo /src -out "/stage/$family" 2>"$warnings"; then
		cat "$warnings" >&2
		die "spkstage failed for $family"
	fi
	cat "$warnings" >&2
	notice="/stage/$family/prebuilt/share/licenses/ffmpeg/SOURCE.txt"
	# Exactly one of each, so neither value can hide behind a second line.
	[ "$(grep -c '^Status:' "$notice")" = 1 ] && [ "$(grep -c '^Licence:' "$notice")" = 1 ] ||
		die "$family: SOURCE.txt needs exactly one Status: and one Licence: line"
	status="$(sed -n 's/^Status:[[:space:]]*\([^[:space:]]*\).*/\1/p' "$notice")"
	licence="$(sed -n 's/^Licence:[[:space:]]*\([^[:space:]]*\).*/\1/p' "$notice")"
	echo "ffmpeg_$family=${status:-unknown} ${licence:-unknown}" >>/stage/build-info.txt
	if [ "$RELEASE" = true ]; then
		[ "$status" = selected ] || die "release refused: the FFmpeg build of $family has status '${status:-none}', not 'selected' (interim builds are for local test builds only)"
		case "$licence" in *nonfree*) die "release refused: the FFmpeg build of $family is non-free ($licence) and not redistributable" ;; esac
		! grep -q 'warning:' "$warnings" || die "release refused: spkstage warned for $family (see above)"
	elif [ "$status" != selected ]; then
		echo "stage: WARNING: $family uses an FFmpeg build with status '${status:-none}'; this is a test build, not for release or distribution" >&2
	fi
done
STAGE

# ---------------------------------------------------------------------------------------------
# staged: the export of step 1 (last stage, so `docker build --output` needs no --target).
FROM scratch AS staged
COPY --from=stage /stage/ /
