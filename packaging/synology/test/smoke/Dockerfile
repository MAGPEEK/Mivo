# Image of the emulated install and playback smoke (task SPK-11, README.md in this directory):
# debian:stable-slim of the family's platform plus xz-utils, because package.tgz inside the SPK
# is xz-compressed and the slim image has no xz. Everything else the DSM-like layout needs
# (dash as /bin/sh, coreutils, setpriv, useradd, tar) is part of the base image.
#
#   docker build --platform linux/arm/v7 -t spk-smoke:armv7 packaging/synology/test/smoke
#
# smoke.sh builds it once per family (linux/amd64, linux/arm64, linux/arm/v7); under QEMU the
# apt step takes a minute or two and is cached afterwards. The base image is pinned by tag like
# the inspection image (test/inspect/Dockerfile); digest pinning comes with CI and Renovate.
FROM debian:stable-slim

# Package versions are not pinned: the Debian archive drops superseded versions.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install --yes --no-install-recommends xz-utils \
    && rm -rf /var/lib/apt/lists/*
