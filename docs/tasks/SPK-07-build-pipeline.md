# SPK-07 CI-ready build: prebuild in Docker, pack with the toolkit
Owner: mivo-synology
Depends on: SPK-02, SPK-04, SPK-14
Files/areas: `packaging/synology/Dockerfile` (new, prebuild + staging),
`packaging/synology/Dockerfile.dockerignore`, `packaging/synology/build.ps1`,
`packaging/synology/build.sh`, `packaging/synology/.gitignore` (ignores `dist/`); small changes to
`packaging/synology/toolkit/run.sh` only if needed (interface kept)

## Goal
One command on the Windows dev machine (Docker Desktop only) – and later unchanged in GitHub
Actions – builds the three server SPKs of a version from a clean checkout: the server binary and
FFmpeg are prebuilt in a normal Docker build, Synology's toolkit packs them.

## Acceptance criteria
- [ ] Step 1, `docker build -f packaging/synology/Dockerfile --build-arg VERSION=<v> --output type=local,dest=packaging/synology/dist/stage .`:
      stages for the web app (as `packaging/docker`), the Go cross-compile per family
      (`CGO_ENABLED=0`, `-tags webui`, `-trimpath`, `-buildvcs=false`, version via `-ldflags -X`;
      armv7 `GOARM=7`), FFmpeg via `ffmpeg/fetch.sh` (interim pins until SPK-16), and `spkstage`
      (SPK-14) per family; the output holds one staged project per family.
- [ ] Step 2: for each family `docker run --rm --privileged -v spk-toolkit:/toolkit -v <stage>/<family>:/project:ro -v <dist>:/output spk-toolkit:dev <family>`
      (SPK-04's image and `run.sh`); the scripts rename the toolkit's
      `<id>-<family>-<DSM_VERSION>.spk` to `<id>-<version>-<family>.spk` and write `SHA256SUMS`.
      A failed toolkit run (exit 1, `[spk-toolkit] failed:`) stops the build.
- [ ] `packaging/synology/build.ps1 -Version 0.1.0-alpha.2` (PowerShell) and
      `packaging/synology/build.sh 0.1.0-alpha.2` (bash) run both steps for all three families and
      fail on the first error; on Docker Desktop (Windows) the PowerShell script produces the three
      SPKs and `SHA256SUMS`. A second run downloads no platform environment (durations of both runs
      in the REPORT). Both scripts print a warning when `sources.lock` still has `interim` entries.
- [ ] Privileges: the scripts use `--privileged` by default; an option (parameter or environment
      variable, documented) switches to `--cap-add SYS_ADMIN`, plus
      `--security-opt apparmor=unconfined` for AppArmor hosts such as GitHub-hosted Ubuntu runners
      (ADR 0026 Notes; not verifiable here – say so in the REPORT).
- [ ] Build inputs: `VERSION` (default `0.0.0-dev`), `REVISION`, `PACKAGE_REVISION` (default 0),
      `SOURCE_DATE_EPOCH` (default documented), and the image args `NODE_IMAGE`, `GO_IMAGE` with
      the **same names and defaults** as `packaging/docker/Dockerfile`; the `pkgscripts-ng` commit
      stays pinned in one place (`run.sh`).
- [ ] Unpacking the x86_64 SPK (`package.tgz` is xz despite its name) and running `bin/<id> --version`
      in `debian:stable-slim` prints the version; `bin/ffmpeg -version` exits 0 (other families:
      SPK-11). INFO has `arch="x86_64"`, `version="0.1.0-1020"`, `beta="yes"`, `adminport="8460"`
      and no `adminurl`; the armv7 SPK has `arch="armv7 armada38x monaco"`.
- [ ] `packaging/synology/dist/` is ignored by git and excluded from the Docker build context; the
      dockerignore keeps the context to `go.mod`, `go.sum`, `cmd/`, `internal/`, `web/` (without
      build output), `LICENSE` and `packaging/synology/` (without `dist/`).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `go test ./...` unchanged green; shellcheck on
      `build.sh` → no findings; a clean `pwsh -NoProfile -File packaging/synology/build.ps1 -?`
      (PSScriptAnalyzer if installed); script tests and `toolkit/test.sh` still green.

## Notes
- The web and Go stages duplicate `packaging/docker/Dockerfile` on purpose (ADR 0026
  consequences); do not edit the Docker image's Dockerfile.
- Step 1 cannot be privileged (`docker build`), step 2 must be (`docker run`), hence two steps.
  The toolkit volume serialises runs with a lock; run the families one after another.
- CI-ready means: nothing on the host but Docker (and PowerShell or bash), no secrets, no network
  after the downloads. The GitHub Actions workflow and an actions cache for the ~24 GB volume (or a
  cache of `toolkit_tarballs/` only, ~3.4 GiB) are a later task – put the exact commands in the
  REPORT.
- The FFmpeg source switches to the own build in SPK-16; keep the FFmpeg step a single, replaceable
  stage.
- `build.ps1` keeps CRLF (`.gitattributes`), `build.sh` LF.
