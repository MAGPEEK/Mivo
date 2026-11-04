# Image of the SPK inspection (task SPK-10; ../../toolkit/README.md "Output verification"):
# debian:stable-slim plus xz (package.tgz is xz-compressed), jq (the JSON members) and bats
# for inspect-spk.bats.
#
#   docker build -t spk-inspect:dev packaging/synology/test/inspect
#
# The base image is pinned by tag; digest pinning comes with CI and Renovate. Package versions
# are not pinned: the Debian archive drops superseded versions.
FROM debian:stable-slim

# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install --yes --no-install-recommends xz-utils jq bats \
    && rm -rf /var/lib/apt/lists/*
