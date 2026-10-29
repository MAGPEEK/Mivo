# M1-20 Dockerfile: multi-stage, non-root, debian:stable-slim with FFmpeg
Owner: mivo-backend
Depends on: M1-16
Files/areas: `packaging/docker/Dockerfile`, `packaging/docker/Dockerfile.dockerignore`,
`internal/adapters/http/healthprobe/**`, `cmd/mivo/**` (`healthcheck` subcommand only)

## Goal
`docker build` produces a runnable image with the embedded web UI and FFmpeg. It runs as
non-root with the `/config`, `/media` and `/transcode` volumes and reports its health to
Docker. This is the basis for the NAS release (ADR 0014).

## Acceptance criteria
- [ ] From a clean checkout,
      `docker build -f packaging/docker/Dockerfile -t mivo:dev .` succeeds on Docker Desktop
      (linux/amd64).
- [ ] Stages:
      - web: `node:24` slim variant, pnpm through corepack and `packageManager`,
        `pnpm install --frozen-lockfile`, `pnpm build`
      - go: official `golang` image matching `go.mod`, `CGO_ENABLED=0`, `-tags webui`,
        `-trimpath`, `-buildvcs=false`, version through `-ldflags -X` from build arg
        `VERSION` (default `0.0.0-dev`)
      - runtime: `debian:stable-slim` with `ffmpeg`, `ca-certificates` and `tzdata`
        (`--no-install-recommends`, apt lists removed)
- [ ] Builder stages use `--platform=$BUILDPLATFORM` and derive `GOOS`/`GOARCH`/`GOARM` from
      `TARGETOS`/`TARGETARCH`/`TARGETVARIANT` (multi-arch ready; publishing is milestone 9).
- [ ] Runtime: `USER 1000:1000` (numeric), `VOLUME ["/config","/media","/transcode"]`,
      `EXPOSE 8460`, ENV `MIVO_CONFIG_DIR=/config`, `MIVO_PATHS_DATA=/config/data`,
      `MIVO_PATHS_CACHE=/config/cache`, `MIVO_PATHS_TRANSCODE=/transcode`. OCI labels
      `org.opencontainers.image.source`, `.licenses=GPL-3.0-or-later` and `.version`.
- [ ] `docker inspect -f '{{.Config.User}}' mivo:dev` → `1000:1000`.
      `docker run --rm --entrypoint ffmpeg mivo:dev -version` and the same with `ffprobe`
      exit 0.
- [ ] `docker run -d --name mivo-test -p 8460:8460 -v <empty temp dir>:/config mivo:dev`
      → within 30 s `docker inspect -f '{{.State.Health.Status}}' mivo-test` is `healthy`,
      `GET http://localhost:8460/api/v1/health` is 200, and `GET /` returns the built
      `index.html` (not the placeholder).
- [ ] `HEALTHCHECK` runs `mivo healthcheck`. The subcommand GETs
      `http://127.0.0.1:<configured port>/api/v1/health` with a named timeout (3 s) and exits
      0 on 200, 1 otherwise. `healthprobe` is unit-tested with `httptest` (200, 503,
      timeout, connection refused).
- [ ] `Dockerfile.dockerignore` excludes `.git`, `**/node_modules`, `testdata/media`, `bin`,
      `internal/adapters/http/webui/dist`, coverage and Playwright output.
- [ ] The REPORT states the final image size (informational).
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check pass. The Docker
      build passes.

## Notes
- FFmpeg comes from Debian's package for now (interim rule in ADR 0015). jellyfin-ffmpeg
  comes in milestone 4 or 9.
- `PUID`/`PGID` with an entrypoint privilege drop is milestone 9. Document `--user` / compose
  `user:` as the M1 way to match NAS file ownership. The mounted `/config` must be writable
  by that UID. Fail with a clear error, not a crash, when it is not (M1-10 already handles
  unwritable dirs).
- Pin base images by tag in M1. Digest pinning and Renovate come with CI.
- `cmd/mivo` remains wiring only. The probe logic lives in
  `internal/adapters/http/healthprobe`.
