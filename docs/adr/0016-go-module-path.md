# 0016 Go module path `github.com/magpeek/mivo`

- Status: Proposed
- Date: 2026-10-29

## Context

The repository is `github.com/MAGPEEK/Mivo`. GitHub resolves owner and repository names
case-insensitively. Go module paths are case-sensitive. Uppercase letters are escaped in the
module cache and proxy URLs (`pkg/mod/github.com/!m!a!g!p!e!e!k/!mivo`). If two spellings are
mixed, the go command reports "module declares its path as … but was required as …".

## Decision

- Module path: **`github.com/magpeek/mivo`** (all lowercase, Go convention).
- Every import, ldflags `-X` target and tool configuration (goimports local prefix, depguard
  rules, sqlc/oapi-codegen package paths) uses this exact spelling.
- The GitHub repository name stays `MAGPEEK/Mivo`. Links to the repository use the
  canonical GitHub spelling from `internal/platform/brand`.

## Consequences

- No `!`-escaped paths, and the path is easy to type.
- `go install github.com/magpeek/mivo/cmd/mivo@<version>` works because GitHub serves the
  repository under the lowercase URL and `go.mod` declares the lowercase path. After the
  first public push, verify this through `proxy.golang.org`. If it fails, supersede this ADR
  with the mixed-case path.
- Renaming the product changes the module path (rename checklist, ADR 0002).
