# Image of the package script tests (README.md in this directory): bats-core with bats-support
# and bats-assert, plus dash for the default shell matrix and jq for the JSON checks.
#
#   docker build -t spk-script-test:dev packaging/synology/test
#
# Pinned so every machine tests with the same shells and tools: the base image by digest
# (bats/bats:1.14.0, 2026-11-04), the Alpine packages by version. Alpine keeps the packages of
# a release branch in its archive; when an update of the base image moves to another Alpine
# release, these versions are updated with it (`apk add` names the new ones).
FROM bats/bats:1.14.0@sha256:5322b877351fda0cc435de8c6116de7d0a2ec79d7c680132a0ef329a633bc66f
RUN apk add --no-cache dash=0.5.13.1-r2 jq=1.8.2-r0
