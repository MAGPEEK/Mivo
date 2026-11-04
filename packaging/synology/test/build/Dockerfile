# Image of the build script tests with PowerShell (README.md in the parent directory): the
# bats-core of the script test image, PowerShell for the build.ps1 parity cases and git for the
# dirty-tree case of build-scripts.bats.
#
#   docker build -t spk-build-test:dev packaging/synology/test/build
#
# bats is pinned by digest like ../Dockerfile; the PowerShell image by tag (digest pinning
# comes with CI and Renovate).
FROM bats/bats:1.14.0@sha256:5322b877351fda0cc435de8c6116de7d0a2ec79d7c680132a0ef329a633bc66f AS bats

FROM mcr.microsoft.com/powershell:7.4-ubuntu-22.04

# Package versions are not pinned: the Ubuntu archive drops superseded versions.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install --yes --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bats /opt/bats /opt/bats
RUN ln -s /opt/bats/bin/bats /usr/local/bin/bats

ENTRYPOINT ["bats"]
