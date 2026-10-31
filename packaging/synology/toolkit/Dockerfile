# syntax=docker/dockerfile:1
#
# Host container for Synology's Package Toolkit pkgscripts-ng (ADR 0026 section 2, task SPK-04).
# The image holds only the host tools that EnvDeploy and PkgCreate.py need; the toolkit itself,
# the downloaded tarballs and the deployed platform environments live in a named Docker volume
# mounted at /toolkit, so they survive the container and are never baked into the image.
#
#   docker build -t spk-toolkit:dev packaging/synology/toolkit
#
# Usage, privileges and the volume layout: packaging/synology/toolkit/README.md.
#
# The base image is pinned by tag, like packaging/docker/Dockerfile; digest pinning comes
# with CI and Renovate.
ARG BASE_IMAGE=debian:12-slim
FROM ${BASE_IMAGE}

# What the pinned toolkit sources call on the host side (outside the chroot):
# - python3: EnvDeploy, PkgCreate.py and include/python/*.py
# - git, ca-certificates: run.sh clones pkgscripts-ng; urllib downloads the tarballs over HTTPS
# - xz-utils, pixz: tar -xhf / tar -Ipixz in pkgdeploy.py (pixz decompresses in parallel)
# - mount (util-linux): exec_env.py mounts /proc into the chroot; chroot is in coreutils
# Everything that runs inside the chroot (bash, SynoBuild, SynoInstall, the project's scripts)
# comes from the platform environment, not from this image.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        ca-certificates \
        git \
        mount \
        pixz \
        python3 \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

COPY run.sh /usr/local/bin/spk-toolkit-run
# SHA-256 pins of the platform tarballs; run.sh verifies every tarball before EnvDeploy.
COPY tarballs.sha256 /usr/local/share/spk-toolkit/tarballs.sha256
RUN chmod 0755 /usr/local/bin/spk-toolkit-run \
    && chmod 0644 /usr/local/share/spk-toolkit/tarballs.sha256

VOLUME ["/toolkit"]
ENTRYPOINT ["/usr/local/bin/spk-toolkit-run"]
