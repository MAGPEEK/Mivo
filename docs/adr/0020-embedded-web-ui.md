# 0020 Embedding the web UI into the Go binary

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-29

## Context

The browser app ships inside the single `mivo` binary. Go's `embed` cannot reference parent
directories, and `go build` / `go test ./...` must keep working on machines without Node or
a prior web build.

## Decision

- Vite (`web/apps/web`) writes its production build directly to
  **`internal/adapters/http/webui/dist`** (`emptyOutDir: true`). This path is git-ignored.
- Package `internal/adapters/http/webui` has two files selected by the build tag
  **`webui`**:
  - with the tag: `//go:embed all:dist` serves the real app
  - without the tag: a small placeholder page (product name from brand) explains how to build
    the UI
- Release, Docker and E2E builds use `go build -tags webui` after `pnpm -C web build`. Plain
  `go build ./...` and `go test ./...` never need the UI.
- Serving rules:
  - `/api/**` never falls back to the UI. Unknown API routes return a 404 problem response.
  - Existing files are served with explicit content types (do not rely on the Windows
    registry MIME table).
  - `/assets/*` (hashed) get `Cache-Control: public, max-age=31536000, immutable`.
  - Any other path without a file extension returns `index.html` with
    `Cache-Control: no-cache` (SPA fallback). Unknown paths with an extension return 404.
- Development: `pnpm -C web dev` (Vite on port 5173) proxies `/api` to the Go server on
  port 8460.

## Consequences

- One artifact. No CORS in same-origin use.
- A `-tags webui` build without a prior web build fails at compile time. This is intended
  and documented.
