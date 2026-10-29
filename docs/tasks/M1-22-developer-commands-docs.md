# M1-22 Developer documentation and the CLAUDE.md "Commands" section
Owner: mivo-backend
Depends on: M1-01 … M1-21 (all other milestone 1 tasks)
Files/areas: `CLAUDE.md` (**"Commands" section only, needs user approval**), `README.md`
("Development" section), `docs/development.md` (new)

## Goal
A new contributor on Windows (PowerShell) or Linux can set up, build, test, lint, run and
package Mivo by following the docs, and every documented command works as written.

## Acceptance criteria
- [ ] The `CLAUDE.md` "Commands" section is replaced by the final, verified command list.
      Nothing else in `CLAUDE.md` changes (`git diff CLAUDE.md` touches only that section).
      The orchestrator obtains **explicit user approval** for this diff before it is
      committed (`CLAUDE.md` is project configuration).
- [ ] The command list covers at least:
      `go test ./...`; coverage run + `go tool go-test-coverage --config=.testcoverage.yml`;
      `golangci-lint run` / `golangci-lint fmt`; `go tool govulncheck ./...`;
      `go generate ./...`; the sqlc Docker command (pinned version from M1-08);
      `pnpm -C web install|lint|test|build|gen|dev|e2e`; Playwright browser install;
      `go run ./cmd/mivo`; `go build -tags webui -o bin/mivo.exe ./cmd/mivo` (after the web
      build); `go run ./scripts/gen-testmedia`;
      `docker build -f packaging/docker/Dockerfile -t mivo:dev .`; lefthook install
      (method from M1-21).
- [ ] The race detector line is accurate for Windows: `go test -race` needs cgo and a C
      toolchain. Document how to install one, or the Docker alternative
      (`docker run --rm -v "${PWD}:/src" -w /src golang:<ver> go test -race ./...`).
- [ ] `docs/development.md` lists prerequisites with pinned/minimum versions (Go, Node 24,
      pnpm 10, golangci-lint version from M1-03, FFmpeg, Docker Desktop) and Windows notes
      (CRLF/LF, sqlc via Docker, Playwright browsers, ports 8460/5173/18460). It also lists
      the generated-code drift checks from ADR 0017 and the rename checklist pointer
      (ADR 0002).
- [ ] `README.md` gets a short "Development" section linking to `docs/development.md`.
- [ ] Tester check: runs every command from the new `CLAUDE.md` section in PowerShell on the
      dev machine. Each succeeds, or it is explicitly marked in the docs as needing
      Docker/network/approval.
- [ ] Gates: documentation only; existing gates stay green.

## Notes
- Do not edit `CLAUDE.md` outside the "Commands" section, and do not edit it without approval.
  Propose the diff in the REPORT first if approval is still pending.
- Collect the exact tool versions from the REPORTs of M1-03 (golangci-lint), M1-08 (sqlc),
  M1-11 (pnpm) and M1-21 (lefthook).
- CI (GitHub Actions) is not part of milestone 1. Mention it as upcoming in
  `docs/development.md`.
