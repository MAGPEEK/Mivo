# M1-16 Embed the built web UI into the Go binary
Owner: mivo-backend
Depends on: M1-10, M1-14
Files/areas: `internal/adapters/http/webui/**`, `cmd/mivo/**` (pass the UI handler as
fallback), `.gitignore`

## Goal
One binary serves both the API and the web app. Plain `go build` and `go test` keep working
without Node. This implements ADR 0020.

## Acceptance criteria
- [ ] `.gitignore` ignores `/internal/adapters/http/webui/dist/`. After a web build,
      `git status --porcelain` shows nothing under that path.
- [ ] **Without** the `webui` tag: `go build ./cmd/mivo` works on a clean checkout (no
      `dist`). `GET /` returns 200 HTML with the product name (from brand) and the build
      instruction.
- [ ] **With** the tag: after `pnpm -C web build`,
      `go build -tags webui -o bin/mivo.exe ./cmd/mivo` succeeds. With the binary running:
      - `GET /` → 200, the built `index.html` (contains `id="root"`),
        `Cache-Control: no-cache`
      - `GET /assets/<hashed file>` → 200, correct `Content-Type` (`text/javascript` for
        `.js`, `text/css` for `.css`),
        `Cache-Control: public, max-age=31536000, immutable`
      - `GET /some/client/route` → 200 `index.html` (SPA fallback)
      - `GET /missing.js` → 404
      - `GET /api/v1/does-not-exist` → 404 `application/problem+json` (no fallback)
      - `GET /api/v1/health` → 200 unchanged
- [ ] The handler takes an `fs.FS`. Only the two tag-selected files differ (`embed` vs
      placeholder).
- [ ] Tests: handler tests with `fstest.MapFS` cover every rule above (content types, cache
      headers, fallback, 404s, path traversal like `/../../go.mod` → 404 or 400, never file
      content). Routing test: API paths never reach the UI handler.
- [ ] Gates: `golangci-lint run`, `go test ./...` (untagged) and the coverage check pass.
      Also run `go vet -tags webui ./...` after a web build.

## Notes
- `//go:embed all:dist` plus `fs.Sub`. The `all:` prefix keeps files that start with `_` or
  `.`.
- Set content types from an explicit extension table (`.html .js .mjs .css .svg .json
  .webmanifest .woff2 .png .ico .txt .map`). Go's `mime` package on Windows can pick up
  registry values (e.g. `.js` as `text/plain`), which breaks module scripts under `nosniff`.
- Use `http.ServeFileFS` or equivalent with the safe `fs.FS` path rules. Do not join raw URL
  paths onto OS paths.
- The fallback hook in `server.Options` comes from M1-09. Only `cmd/mivo` wiring changes.

## Orchestrator note (from M1-14)
- `pnpm -C web build` already writes to `internal/adapters/http/webui/dist` (emptyOutDir). Add the
  matching `.gitignore` entry (keep a placeholder if `//go:embed` needs the directory to exist).

## Review carry-over from M1-10 (APPROVE) – fix while wiring the web UI fallback in cmd/mivo
- `serve_test.go`: assert the bound port is not `config.DefaultPort` (proves env overrides the file).
- `serve.go`/`main.go`: when startup is cancelled and `db.Close()` also fails, report/log the close error (decide "stopped" on the startup error only).
- Nits: consistent "start database %s" prefix; rename `application.run` (e.g. `start`); count the exact "http server listening" message; after the second run assert the data dir can be removed (DB closed).
