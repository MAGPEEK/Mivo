# Milestone 1 – planning summary

Produced by `mivo-architect`: 21 ADRs (`docs/adr/0001`–`0021`) and 22 tasks (`docs/tasks/M1-01`–`M1-22`).
No source code yet.

## Open decisions for the user (before M1-02 starts)

- ADRs 0016–0021 are **Proposed**: module path `github.com/magpeek/mivo` (lowercase), default
  port 8460, OpenAPI 3.0.3 with committed generated code, configuration/OS paths, quality gates,
  web UI embedding behind the `webui` build tag, generated test media corpus.
- License: GPL-3.0-**or-later** (ADR 0001) vs. GPL-3.0-only.
- TMDB key: injected only into release builds by CI, not committed (ADR 0009).
- Outbound traffic: "update check is the only *unsolicited* connection"; Cast SDK documented exception (ADR 0010).

## Approvals the orchestrator needs

- M1-01: download the official GPL-3.0 text from gnu.org
- M1-03: network access for `govulncheck`
- M1-17: install Playwright browsers
- M1-22: edit of the `CLAUDE.md` commands section

## Known blockers / risks

- `go test -race` needs cgo + a C toolchain on Windows → install one (e.g. MSYS2 gcc / LLVM-MinGW) or run race tests in Docker. Affects `CLAUDE.md` and `mivo-tester`.
- `sqlc` cannot be `go install`ed on Windows → pinned Docker image (M1-08).
- `go.mod`/`go.sum` and `web/pnpm-lock.yaml` are shared by parallel tasks → serialize dependency installs or use worktrees.
- New PATH entries (Go 1.27, FFmpeg 9.0.2, golangci-lint 2.15) need an app restart to be visible in agent shells.
- CI (GitHub Actions) is not in M1 scope yet → separate task needed.

## Execution order (waves, parallel within a wave)

1. M1-02 ‖ M1-11 ‖ M1-01 (after approval)
2. M1-03 ‖ M1-12
3. M1-04 ‖ M1-05 ‖ M1-06 ‖ M1-07 ‖ M1-18 ‖ M1-14
4. M1-08 ‖ M1-09 ‖ M1-13 ‖ M1-19 ‖ (M1-21)
5. M1-10 ‖ M1-15
6. M1-16
7. M1-17 ‖ M1-20
8. M1-22

Walking-skeleton checkpoint after M1-17: web → embed → HTTP → app → SQLite.
