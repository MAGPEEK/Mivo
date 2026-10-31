# SPK-07 CI-ready build: prebuild in Docker, pack with the toolkit
Owner: mivo-synology
Depends on: SPK-02, SPK-04, SPK-14
Files/areas: `packaging/synology/Dockerfile` (new, prebuild + staging),
`packaging/synology/Dockerfile.dockerignore`, `packaging/synology/build.ps1`,
`packaging/synology/build.sh`, `packaging/synology/.gitignore` (ignores `dist/`), small changes
to `packaging/synology/toolkit/**` run script if needed (interface kept)

## Goal
One command on the Windows dev machine (Docker Desktop only) – and later unchanged in GitHub
Actions – builds the three server SPKs of a version from a clean checkout: the server binary and
FFmpeg are prebuilt in a normal Docker build, Synology's toolkit packs them.

## Acceptance criteria
- [ ] Step 1, `docker build -f packaging/synology/Dockerfile --build-arg VERSION=<v> --output type=local,dest=packaging/synology/dist/stage .`:
      stages for the web app (as `packaging/docker`), the Go cross-compile per family
      (`CGO_ENABLED=0`, `-tags webui`, `-trimpath`, `-buildvcs=false`, version via `-ldflags -X`),
      FFmpeg via `ffmpeg/fetch.sh` (SPK-02) and `spkstage` (SPK-14) per family; the output holds
      one staged project per family.
- [ ] Step 2: the privileged toolkit container of SPK-04 packs each staged project with
      `PkgCreate.py` (signing off) using the platform environments cached in the named volume, and
      writes `packaging/synology/dist/<id>-<version>-<family>.spk` (renamed from the toolkit's
      output) plus `SHA256SUMS`.
- [ ] `packaging/synology/build.ps1 -Version 0.1.0-alpha.2` (PowerShell) and
      `packaging/synology/build.sh 0.1.0-alpha.2` (bash) run both steps for all three families and
      fail on the first error; on Docker Desktop (Windows) the PowerShell script produces the three
      SPKs and `SHA256SUMS`. A second run reuses the cached environments (no environment download;
      durations of both runs in the REPORT).
- [ ] Build inputs: `VERSION` (default `0.0.0-dev`), `REVISION`, `PACKAGE_REVISION` (default 0),
      `SOURCE_DATE_EPOCH` (default documented), and the image args `NODE_IMAGE`, `GO_IMAGE` with
      the **same names and defaults** as `packaging/docker/Dockerfile`; the `pkgscripts-ng` commit
      is pinned in one place.
- [ ] Unpacking the x86_64 SPK (`package.tgz` inside) and running `bin/<id> --version` in
      `debian:stable-slim` prints the version; `bin/ffmpeg -version` exits 0 (other families:
      SPK-11). INFO has `arch=x86_64`, `version=0.1.0-1020`, the beta flag and `adminport=8460`.
- [ ] `packaging/synology/dist/` is ignored by git and excluded from the Docker build context; the
      dockerignore keeps the context to `go.mod`, `go.sum`, `cmd/`, `internal/`, `web/` (without
      build output), `LICENSE` and `packaging/synology/` (without `dist/`).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `go test ./...` unchanged green; shellcheck on
      `build.sh` → no findings; PSScriptAnalyzer (if installed) or a clean `pwsh -NoProfile -File`
      run for `build.ps1`; script tests of SPK-05/06 still green.

## Notes
- The web and Go stages duplicate `packaging/docker/Dockerfile` on purpose (ADR 0026
  consequences); do not edit the Docker image's Dockerfile (M2-35 owns it).
- Step 1 cannot be privileged (`docker build`), step 2 must be (`docker run`), hence two steps.
- CI-ready means: nothing on the host but Docker (and PowerShell or bash), no secrets, no network
  after the downloads. GitHub-hosted Linux runners allow privileged containers; the workflow and
  an actions cache for the platform environments are a later task – put the exact commands for it
  in the REPORT.
- `build.ps1` keeps CRLF (`.gitattributes`), `build.sh` LF.
- Network: FFmpeg archives, images, npm and Go modules, platform environments (approved).
