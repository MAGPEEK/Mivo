# M3-02 test report

Status: PASS (round 1)

## Gates

| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race -coverprofile=coverage.out ./...` | all packages ok |
| `go-test-coverage --config=.testcoverage.yml` | total 95.8 % (>= 70 %), PASS |
| `golangci-lint run` / `--build-tags webui` | 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff` (root and tools) | clean |
| `go generate ./...` + `pnpm -C web gen` | no content change; a second run left the hashes of `apigen.gen.go` and `schema.ts` identical |
| `pnpm -C web lint` (eslint, depcruise, prettier, tsc) | clean |
| `pnpm -C web test` | 85 files, 1230 tests passed; statements 99.94 % |
| `pnpm -C web build` + `go vet -tags webui`, `go test -race -tags webui ./internal/adapters/http/... ./cmd/...` | ok |
| `pnpm -C web e2e` | 31 passed |

Layer rule: `internal/domain` is unchanged by this task; archtest (`internal/archtest`) passes.
`internal/domain/...` packages are at 100 %.

Note: the work is uncommitted, so `git diff --exit-code` cannot show drift against HEAD. Drift was
checked by regenerating and comparing file hashes and `git status` before and after.

## Acceptance criteria

- info.version 1.2.0, enums documented as extensible: met (openapi.yaml:11, "Clients must tolerate" on every new enum).
- ItemKind, ExtraType, ImageRef, ImageWidth, ArtworkKind, PersonRole, ProviderIds: met (openapi.yaml:937-1073, pinned by `TestLibraryV2Enums` and `TestLibraryV2SchemaShapes`).
- ItemSummary fields: met (:1116-1195; `kind` required, `media` optional).
- ItemDetail and MediaFile fields (`maxItems: 100` on extras at :1337, `versionLabel`/`partNumber`): met.
- `listItemChildren` (limit 1-500 default 100, cursor, order asc|desc default asc, ItemPage, sort rules, 404, leaf gives empty page): met (:220-270; `TestPagingParameters`, `TestChildrenOrderParameter`).
- `listLibraryItems` and `Library.itemCount` document top-level only: met (:162, :776).
- `getImage` (tag `images`, `security: []`, width required, tag maxLength 64 and pattern, 200 jpeg/png with ETag and Cache-Control, 304, default problem, ADR 0027 section 8 caching rule, helper-only URL): met (:271-335).
- `oapi-codegen.yaml` excludes `getImage`; spec_test compares the excluded list with the stream and images tags; BACKLOG `api` row deleted; `\b` regex and header/response description scan added with `TestStreamURLAsSourcePattern`: met.
- restapi answers `listItemChildren` with 501 / `not_implemented`, table test pins it: met (`TestListItemChildrenAnswersNotImplemented`). `kind` is filled with one line per mapper in `items_response.go`.
- api-client exports the new types, `imageUrl`, `imageSrcSet` (and `imageWidths`); Vitest covers encoding and the width union (`images.test.ts`): met.
- Generation leaves no drift: met (see the note above).
- Tests and gates listed in the task: met.

## Failures

None.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M3-02
files: docs/reports/M3-02-test.md
tests: go vet; go test -race with coverage (total 95.8 %); golangci-lint run, run with the webui tag and fmt --diff (0 issues); go mod tidy -diff; go generate and pnpm gen (no drift); pnpm -C web lint, test (1230 passed) and build; webui-tagged Go gates; pnpm -C web e2e (31 passed)
findings: none
next: hand over to mivo-reviewer
