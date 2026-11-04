# M3-12b test report

Result: PASS (round 1 of max 3)

| Gate | Result |
|---|---|
| pnpm -C web lint (eslint, depcruise, prettier, tsc) | pass |
| pnpm -C web test | 91 files, 1298 tests pass; coverage 99.64 % statements, thresholds met |
| pnpm -C web build | pass |
| go vet -tags webui ./... | pass |
| golangci-lint run --build-tags webui | 0 issues |
| go test -race -tags webui ./internal/adapters/http/... ./cmd/... | ok |
| pnpm -C web e2e | 31 passed (regression, incl. axe specs) |

Acceptance criteria: all met. Evidence: HierarchyPages.test.tsx covers each page kind, specials-only show, 131-episode
load-more, axe check; router.test.tsx validates `?order=desc|sideways|none`; en/de locales carry
seasons/episodes plural forms.

## History
- Round 1: PASS.
