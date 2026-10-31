# M2-35 test report (round 1) - PASS

Isolated worktree at HEAD 531f20b plus only `scripts/playback-smoke/**`, `packaging/docker/Dockerfile`,
`packaging/docker/README.md`; corpus copied from `testdata/media`.

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race -coverprofile` ./... | all ok; playback-smoke 92.0 % (main.go `main` 0 %, all other funcs 66.7-100 %, i.e. >= 80 % excluding main) |
| go-test-coverage | PASS, total 96.1 % (>= 70 %) |
| `golangci-lint run` / `fmt --diff` | 0 issues / no diff |
| `go mod tidy -diff` (root, tools) | clean |
| Docker `go test -race ./...` (golang:1.27.3) | exit 0, every package ok |
| `govulncheck ./...` | No vulnerabilities found |
| `docker build` (`mivo:m235test`) | success |

## Release gate (image mivo:m235test, user 1000:1000, 127.0.0.1:18460, /media read-only)
- Container became `healthy`.
- `ffmpeg version 7.1.5-0+deb13u1 Copyright (c) 2000-2026 the FFmpeg developers`
- chromium-desktop: `summary: 30 passed, 0 failed, 0 not run of 30 cases (profile chromium-desktop) in 11s` (incl. 2 seek cases; mkv long @21s starts at 20.021 s, tolerated)
- firefox-desktop: `summary: 28 passed, 0 failed, 0 not run of 28 cases (profile firefox-desktop) in 8s` (ogv theora direct_play, as expected)
- Both exit code 0.
- `ls /transcode/sessions` empty after the runs; `touch /media/x` -> "Read-only file system"; no error/warn lines in the container log.
- README workaround `--entrypoint /usr/bin/env mivo:dev -u MIVO_LIBRARIES /usr/local/bin/mivo` with a
  `/config/config.yaml` (library "Clips", `/media/video`) verified: healthy, `/api/v1/libraries` shows "Clips", 24 items.

## Acceptance criteria
1. CLI behaviour (scan, per-case line, summary, exit code): met (runs above).
2. Unit tests with httptest, coverage >= 80 % excl. main: met (92.0 % incl. main).
3. Docker build/run/healthy/smoke pass/ffmpeg line quoted: met.
4. Sessions dir empty, /media untouched: met.
5. README (libraries, YAML alternative, transcode limit, /transcode space note, Debian FFmpeg interim ADR 0015, Synology/QNAP examples): met (README lines 46-117, 200-240).
6. Gates (lint, layer rules via archtest, race scripts, docker build): met.

## Flake (pre-existing, not M2-35): TestBodyReadDeadlineSparesBodilessStreams
`-race -count=20` alone: 20/20 pass; again with `-count=20` while a Docker full race run and a host
`go test -race ./internal/...` ran in parallel: 20/20 pass (plus a 20-run -v check, 20 PASS). 0 failures in 60 runs; flake not reproduced.

## History
- Round 1: PASS.

REPORT
status: PASS
task: M2-35
files: docs/reports/M2-35-test.md
tests: vet, race+coverage (96.1 % total, playback-smoke 92.0 %), lint, fmt, tidy, docker race, govulncheck all clean; smoke 30/30 chromium, 28/28 firefox on the Docker image
findings: none (TestBodyReadDeadlineSparesBodilessStreams not reproduced in 60 runs)
next: proceed to mivo-reviewer
