# syntax=docker/dockerfile:1
#
# Tools for verify.sh (task SPK-15): kernelsim and sysprobe (static, for the build machine) and
# a static qemu-x86_64, which runs the x86_64 binaries on a baseline CPU model. They run on an
# x86_64 Docker host (Docker Desktop on Windows); arm binaries go through the host's binfmt QEMU.
#
#   docker build --output type=local,dest=<dir> packaging/synology/ffmpeg/build/verify
#
# The platform is fixed on purpose: the tools run natively on the x86_64 Docker host, whatever
# platform the checked container has.
# hadolint ignore=DL3029
FROM --platform=linux/amd64 debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3 AS tools
ARG DEBIAN_FRONTEND=noninteractive
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc libc6-dev libseccomp-dev qemu-user-static \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY kernelsim.c sysprobe.c ./
RUN mkdir -p /out \
 && gcc -O2 -static -o /out/kernelsim kernelsim.c -lseccomp \
 && gcc -O2 -static -o /out/sysprobe sysprobe.c \
 && cp /usr/bin/qemu-x86_64-static /out/qemu-x86_64 \
 && /out/qemu-x86_64 -version

FROM scratch
COPY --from=tools /out/ /
