# syntax=docker/dockerfile:1
#
# Static FFmpeg and ffprobe for the DSM architecture families x86_64, armv8 and armv7, plus the
# corresponding source archive (task SPK-15, ADR 0026 Notes). Build from the repository root:
#
#   docker build --target armv7 --output type=local,dest=out/armv7 packaging/synology/ffmpeg/build
#   docker build --target source --output type=local,dest=out/source packaging/synology/ffmpeg/build
#   docker build --output type=local,dest=out packaging/synology/ffmpeg/build    # all (default)
#
# Targets: x86_64, armv8, armv7 (ffmpeg, ffprobe, licence texts, SOURCE.txt), source (the
# corresponding source archive and its .sha256), all (every family in a subdirectory, plus
# source/). Build argument JOBS limits parallel compile jobs per stage (default: all CPUs).
#
# Everything runs on the build machine and cross-compiles with musl toolchains built from
# pinned sources; nothing runs under emulation. The context is this directory only, so the
# recipe does not depend on the Synology packaging around it. Details: README.md.

# Pinned by digest (debian:bookworm-slim, 2026-10-27): the same image on every build machine.
ARG BUILDER_IMAGE=debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3

# ---------------------------------------------------------------------------------------------
# host: build tools from Debian. They build the toolchains and run configure scripts; none of
# their code ends up in the binaries. Versions are recorded in SOURCE.txt.
FROM --platform=$BUILDPLATFORM ${BUILDER_IMAGE} AS host
ARG DEBIAN_FRONTEND=noninteractive
# Debian package versions move with point releases; pinning them would break the build once a
# version leaves the archive. The toolchains and every linked source are pinned instead.
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        bzip2 ca-certificates curl file gawk gcc g++ libc6-dev m4 make meson ninja-build \
        patch pkgconf python3 xz-utils \
 && rm -rf /var/lib/apt/lists/*
SHELL ["/bin/sh", "-eu", "-c"]
WORKDIR /recipe

# ---------------------------------------------------------------------------------------------
# download: every pinned source, checked against sources.list. The cache mount keeps downloads
# (named by SHA-256) across builds, so a changed pin fetches only the changed file.
FROM host AS download
COPY sources.list scripts/fetch-sources.sh ./
RUN --mount=type=cache,id=ffmpeg-static-downloads,target=/cache \
    sh fetch-sources.sh download sources.list /sources /cache

# sources: the inputs below <stage>/. Replace this stage to build from an unpacked source
# archive without downloading: docker build --build-context sources=<archive>/sources ...
FROM scratch AS sources
COPY --from=download /sources/ /

# verified-sources: whatever "sources" provides is verified again and must match the list
# exactly; versions.env per stage tells the build scripts the pinned versions.
FROM host AS verified-sources
COPY sources.list scripts/fetch-sources.sh ./
COPY --from=sources / /sources/
RUN sh fetch-sources.sh verify sources.list /sources

# ---------------------------------------------------------------------------------------------
# tools: NASM from source for the x86 assembly of FFmpeg, x264 and dav1d, so the assembler
# version is pinned like the compiler.
FROM host AS tools
ARG JOBS
COPY --from=verified-sources /sources/host/ /sources/host/
WORKDIR /build/nasm
RUN . /sources/host/versions.env \
 && tar -xJf "/sources/host/$NASM_FILE" --strip-components=1 \
 && ./configure --prefix=/usr/local >/dev/null \
 && make -j"${JOBS:-$(nproc)}" >/dev/null \
 && make install >/dev/null \
 && find . -mindepth 1 -delete \
 && nasm -v
WORKDIR /recipe

# toolchain-common: what every family's toolchain stage needs. Only the toolchain sources and
# the target definitions enter, so pin changes of FFmpeg or the libraries keep these cached.
FROM tools AS toolchain-common
COPY --from=verified-sources /sources/toolchain/ /sources/toolchain/
COPY scripts/families.sh scripts/layout.sh scripts/build-toolchain.sh /recipe/scripts/

FROM toolchain-common AS toolchain-x86_64
ARG JOBS
RUN JOBS="${JOBS:-}" sh scripts/build-toolchain.sh x86_64 /sources/toolchain /opt/cross

FROM toolchain-common AS toolchain-armv8
ARG JOBS
RUN JOBS="${JOBS:-}" sh scripts/build-toolchain.sh armv8 /sources/toolchain /opt/cross

FROM toolchain-common AS toolchain-armv7
ARG JOBS
RUN JOBS="${JOBS:-}" sh scripts/build-toolchain.sh armv7 /sources/toolchain /opt/cross

# ---------------------------------------------------------------------------------------------
# libs-<family>: zlib, x264 and dav1d as static libraries in /build/prefix.
FROM toolchain-x86_64 AS libs-x86_64
ENV PATH=/opt/cross/bin:$PATH
ARG JOBS
COPY --from=verified-sources /sources/libs/ /sources/libs/
COPY scripts/compile-flags.sh scripts/build-libs.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-libs.sh x86_64 /sources/libs /build/prefix

FROM toolchain-armv8 AS libs-armv8
ENV PATH=/opt/cross/bin:$PATH
ARG JOBS
COPY --from=verified-sources /sources/libs/ /sources/libs/
COPY scripts/compile-flags.sh scripts/build-libs.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-libs.sh armv8 /sources/libs /build/prefix

FROM toolchain-armv7 AS libs-armv7
ENV PATH=/opt/cross/bin:$PATH
ARG JOBS
COPY --from=verified-sources /sources/libs/ /sources/libs/
COPY scripts/compile-flags.sh scripts/build-libs.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-libs.sh armv7 /sources/libs /build/prefix

# ---------------------------------------------------------------------------------------------
# ffmpeg-<family>: the configured, stripped ffmpeg and ffprobe in /build/out.
FROM libs-x86_64 AS ffmpeg-x86_64
ARG JOBS
COPY --from=verified-sources /sources/ffmpeg/ /sources/ffmpeg/
COPY scripts/recipe.sh scripts/build-ffmpeg.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-ffmpeg.sh x86_64 /sources/ffmpeg /build/prefix /build/out

FROM libs-armv8 AS ffmpeg-armv8
ARG JOBS
COPY --from=verified-sources /sources/ffmpeg/ /sources/ffmpeg/
COPY scripts/recipe.sh scripts/build-ffmpeg.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-ffmpeg.sh armv8 /sources/ffmpeg /build/prefix /build/out

FROM libs-armv7 AS ffmpeg-armv7
ARG JOBS
COPY --from=verified-sources /sources/ffmpeg/ /sources/ffmpeg/
COPY scripts/recipe.sh scripts/build-ffmpeg.sh /recipe/scripts/
RUN JOBS="${JOBS:-}" sh scripts/build-ffmpeg.sh armv7 /sources/ffmpeg /build/prefix /build/out

# ---------------------------------------------------------------------------------------------
# source-archive: every verified source plus this recipe, the corresponding source (GPL-3.0
# section 6) to attach to each GitHub Release.
FROM host AS source-archive
COPY --from=verified-sources /sources/ /sources/
COPY . /recipe-files/
RUN . /sources/ffmpeg/versions.env \
 && sh /recipe-files/scripts/make-source-archive.sh /sources /recipe-files /out/source "$FFMPEG_VERSION"

# ---------------------------------------------------------------------------------------------
# package-<family>: checks (static, ABI note, no VFPv4 on armv7) and SOURCE.txt.
FROM ffmpeg-x86_64 AS package-x86_64
ARG BUILDER_IMAGE
COPY sources.list scripts/package.sh /recipe/scripts/
COPY --from=source-archive /out/source/*.sha256 /build/archive/
RUN BUILDER_IMAGE="$BUILDER_IMAGE" sh scripts/package.sh x86_64 /build/out scripts/sources.list /build/archive /out/x86_64

FROM ffmpeg-armv8 AS package-armv8
ARG BUILDER_IMAGE
COPY sources.list scripts/package.sh /recipe/scripts/
COPY --from=source-archive /out/source/*.sha256 /build/archive/
RUN BUILDER_IMAGE="$BUILDER_IMAGE" sh scripts/package.sh armv8 /build/out scripts/sources.list /build/archive /out/armv8

FROM ffmpeg-armv7 AS package-armv7
ARG BUILDER_IMAGE
COPY sources.list scripts/package.sh /recipe/scripts/
COPY --from=source-archive /out/source/*.sha256 /build/archive/
RUN BUILDER_IMAGE="$BUILDER_IMAGE" sh scripts/package.sh armv7 /build/out scripts/sources.list /build/archive /out/armv7

# ---------------------------------------------------------------------------------------------
# Output targets for --output type=local.
FROM scratch AS x86_64
COPY --from=package-x86_64 /out/x86_64/ /

FROM scratch AS armv8
COPY --from=package-armv8 /out/armv8/ /

FROM scratch AS armv7
COPY --from=package-armv7 /out/armv7/ /

FROM scratch AS source
COPY --from=source-archive /out/source/ /

FROM scratch AS all
COPY --from=package-x86_64 /out/ /
COPY --from=package-armv8 /out/ /
COPY --from=package-armv7 /out/ /
COPY --from=source-archive /out/ /
