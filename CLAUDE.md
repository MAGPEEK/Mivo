# Mivo – project rules for Claude

Mivo (working title) is an open-source (GPL-3.0) media server: Go backend, React/TypeScript
web app, later an LG webOS TV app. The approved plan lives in `docs/plan.md`; architecture
decisions in `docs/adr/`. Read both before changing behaviour.

Language: code, comments, docs, commits and UI base strings in **English**. The user
communicates in German – reports to the user are German, files stay English.

## Product name is configurable

"Mivo" is a working title. Never hard-code the name in more than one place:
Go uses `internal/platform/brand`, the web app uses `web/packages/ui-tokens/src/brand.ts`.

## Repository layout

```
cmd/mivo/            composition root only – wiring, no logic
internal/domain/     pure business logic, NO I/O imports (no database/sql, net/http, os/exec)
internal/app/        use cases; defines the ports (interfaces) it needs
internal/adapters/   port implementations: http, sqlite, ffmpeg, fs, metadata, dlna, netaccess
internal/platform/   config, logging, OS paths, Windows service, brand
api/openapi.yaml     API contract (spec-first, generate server stubs and TS client)
web/                 pnpm monorepo: apps/web, apps/webos, packages/{api-client,ui-tokens,player-core}
scripts/             gen-testmedia and dev helpers
docs/                plan.md, adr/, tasks/, reports/
```

## Architecture rules (enforced by depguard / dependency-cruiser)

- Dependency direction: `domain` ← `app` ← `adapters` ← `cmd`. Inner layers never import outer ones.
- Interfaces are declared by the consumer (in `app`), are small (1–3 methods). Accept interfaces, return structs.
- Dependency injection via constructors, wired only in `cmd/mivo/main.go`. No globals, no `init()` side effects, no singletons.
- Frontend: feature folders `src/features/<feature>`, shared UI in `src/shared/ui`. Components contain no business logic; it lives in hooks or `packages/player-core` (framework-free TS). No import cycles.
- The TypeScript API client is generated from `api/openapi.yaml`. Never edit generated code by hand.

## Code rules

- One responsibility per package, type and function. Functions ≤ 50 lines, cognitive complexity ≤ 15, ≤ 4 parameters (else an options struct).
- Descriptive English names, no unexplained abbreviations, no magic numbers (named constants).
- Errors: always handled, wrapped with context (`fmt.Errorf("probe %s: %w", path, err)`). Domain errors (`ErrNotFound`, `ErrForbidden`) are mapped to HTTP status only in `adapters/http`. No `panic` outside `main`.
- Pass `context.Context` through every call chain that does I/O or may be cancelled.
- Comments explain *why*, not *what*. Exported identifiers have doc comments.
- TypeScript `strict`, no `any`.
- File access only inside configured library roots (path traversal protection).
- Boy Scout rule: leave touched code cleaner.

## Tests

- Domain: table-driven unit tests, no mocks, coverage ≥ 90 %.
- Use cases: fakes (in-memory port implementations), not mock frameworks.
- Adapters: integration tests against real SQLite (temp dir) and real FFmpeg with the test corpus.
- E2E with Playwright for core flows. Total coverage ≥ 70 %.
- Bugs: write the failing test first, then fix.

## Commands (Windows dev machine, PowerShell)

Commands are added here as the tooling lands in milestone 1. Expected:

```
go test ./...                       # all Go tests
go test -race -cover ./...          # with race detector and coverage
golangci-lint run                   # Go lint incl. depguard layer rules
pnpm -C web install
pnpm -C web lint                    # ESLint + dependency-cruiser
pnpm -C web test                    # Vitest
pnpm -C web e2e                     # Playwright
go run ./cmd/mivo                   # start server
```

## Definition of Done

1. Acceptance criteria from `docs/tasks/<id>.md` met
2. Tests written and green, coverage thresholds met
3. Lint clean (Go and web), layer rules pass
4. Docs / ADR updated when behaviour or a decision changed
5. No TODO without a task reference
6. Commit uses Conventional Commits (`feat(playback): …`) and ends with the attribution line

## Agent workflow

The main session orchestrates; subagents cannot start other subagents.

1. `mivo-architect` – writes `docs/tasks/<id>.md` with acceptance criteria
2. implementer (`mivo-backend`, `mivo-media`, `mivo-frontend`, `mivo-webos`) – code + tests
3. `mivo-tester` – runs all gates; FAIL → back to step 2 with the report
4. `mivo-reviewer` – read-only review; REVISE → back to step 2 with findings
5. PASS + APPROVE → commit. After 3 failed rounds stop and ask the user.
6. `mivo-release` – publishes artifacts (Docker image `docker.io/magpeek/mivo-media-server`) only
   after PASS + APPROVE and with the user's explicit approval for each push.

Every agent ends its answer with a `REPORT` block:

```
REPORT
status: DONE | PASS | FAIL | APPROVE | REVISE | BLOCKED
task: <id>
files: <changed files or none>
tests: <command + result summary>
findings: <bullets or none>
next: <what the orchestrator should do>
```
