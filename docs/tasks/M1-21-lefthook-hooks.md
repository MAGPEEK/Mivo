# M1-21 lefthook: pre-commit and commit-msg hooks
Owner: mivo-backend
Depends on: M1-03, M1-11 (if lefthook is installed through npm, also schedule after M1-17;
see Notes)
Files/areas: `lefthook.yml`, `scripts/commitmsg/**` (Go checker + tests), `go.mod`/`go.sum`
**or** `web/package.json` + `web/pnpm-workspace.yaml` (lefthook installation, one of the two)

## Goal
Commits are checked locally before they land: formatting, fast lint, fast tests and the
Conventional Commits message format (ADR 0019).

## Acceptance criteria
- [ ] `lefthook.yml` defines:
      - `pre-commit` (parallel, staged files only where the tool supports it): `golangci-lint
        fmt --diff` and a fast `golangci-lint run` for Go changes; `prettier --check` and
        `eslint` on staged web files; `go test ./internal/domain/... ./internal/app/...`;
        `vitest related --run` on staged web files **with coverage disabled** (partial runs
        must not trip the global thresholds)
      - `commit-msg`: `go run ./scripts/commitmsg {1}`
- [ ] `scripts/commitmsg` accepts `type(scope)!: subject` with types `feat fix docs style
      refactor perf test build ci chore revert` and optional scope `[a-z0-9-]+`. It also
      accepts git's own `Merge …` and `Revert "…"` messages and ignores comment lines and the
      trailer block. Its error message shows the expected format. Table-driven tests.
- [ ] Installation works on Windows without a global install. The chosen method (npm
      devDependency in `web/` or `go tool` directive) is documented in the REPORT for M1-22.
      After `pnpm -C web install` or the documented command, `.git/hooks/pre-commit` and
      `.git/hooks/commit-msg` exist.
- [ ] In a **temporary clone**: `git commit --allow-empty -m "update stuff"` is rejected and
      `git commit --allow-empty -m "chore: hook test"` succeeds. `lefthook run pre-commit` on
      a clean tree passes in under 60 s.
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check pass.
      `pnpm -C web lint` passes if web files changed.

## Notes
- If you choose the npm route, pnpm 10 blocks lifecycle scripts. Add `lefthook` to
  `onlyBuiltDependencies` or run `lefthook install` explicitly. This edits the shared
  `web/package.json`, so do not run it in parallel with M1-12 to M1-17.
- If you choose `go tool lefthook`, check that the generated hook scripts can find the
  binary on Windows (PowerShell, Git Bash). Otherwise use the npm route.
- The full gates run in CI later. Hooks must stay fast, or people will skip them.
