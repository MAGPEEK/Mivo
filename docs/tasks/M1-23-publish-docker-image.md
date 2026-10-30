# M1-23 Publish the Docker image to Docker Hub
Owner: mivo-release
Depends on: M1-20 (PASS + APPROVE), M1-22
Files/areas: `docs/reports/release-<version>.md` only

## Goal
The first alpha image is available on Docker Hub as `docker.io/magpeek/mivo-media-server`,
so it can be pulled by Synology Container Manager and QNAP Container Station.

## Acceptance criteria
- [ ] The project owner approved this push (tag + repository) in chat; the REPORT records it.
- [ ] `docker buildx imagetools inspect docker.io/magpeek/mivo-media-server:<version>` lists
      `linux/amd64` and `linux/arm64` (and `linux/arm/v7` if M1-20 supports it).
- [ ] Tags pushed: `<version>` (e.g. `0.1.0-alpha.1`) and `alpha`. No `latest`.
- [ ] OCI labels present: `org.opencontainers.image.source`, `.version`, `.revision`,
      `.licenses=GPL-3.0-or-later`.
- [ ] `docker run --rm docker.io/magpeek/mivo-media-server:<version> --version` prints the version.
- [ ] `docs/reports/release-<version>.md` lists digest, platforms, compressed size and commands.

## Notes
- The repository name contains the working title (ADR 0002 rename checklist): add
  `docker.io/magpeek/mivo-media-server` to that checklist.
- Docker Hub login is done by the user (`docker login -u magpeek`); agents never handle
  credentials.
- Later (CI task): GitHub Actions pushes on tags with a Docker Hub access token stored as a
  repository secret.
