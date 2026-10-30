# Backlog of deferred review findings

Non-blocking findings from APPROVE reviews that are not yet scheduled in a task. Each entry
names its source; pick entries up the next time the area is touched, or schedule them as a task.
Entries already carried into a later task file are listed there, not here.

| Area | Item | Source | Due before |
|---|---|---|---|
| logging | `pin` gaps, extra words (`passcode`, `bearer`, `jwt`, `otp`, `pwd`, …), secret config key naming rule | M1-05c (Deferred) | M6 (TMDB key) / M7 (PINs) |
| app/system | reload-error wrapping, generator-count assertion, deadline classification, `CreatedAt` ms truncation | M1-06b (Deferred) | next change to `internal/app/system` |
| config/paths | drive-relative `C:cfg` config dir, ADR 0018 node-walk note, `ErrInvalid` wrapping nits | M1-04b (Deferred) | next change to `internal/platform/config` |
| archtest | `go/build`, `go/importer` and file-reading std functions in the domain | M1-03b (Deferred) | next change to `internal/archtest` |
| sqlite | reader pool `SetMaxIdleConns(maxOpen)`; `GOMAXPROCS(0)` instead of `NumCPU` | M1-08 review | next change to `internal/adapters/sqlite` |
| sqlite | concurrent `Migrate` from two processes unguarded → document "one process per data dir" and add a single-instance lock (M1-10 or later) | M1-08 review | before distribution (M9) |
| sqlite | DB/WAL/SHM files created 0644 inside a 0750 data dir → pre-create 0600 or document umask | M1-08 review | M7 (auth data) |
| sqlite | nits: Ping that touches the file, reject zero `CreatedAt`, UNC path test, `t.Run` in dsn role loop | M1-08 review | next change |
| testmedia | `encoders` for kept files: never claim this run's alternative and never drop them on interrupted or `-only` runs (`keptOrSkipped` in runner.go) – carry over from the old manifest; label kept files "kept" instead of "generated"; tests | M1-19 review + test | next change to `scripts/gen-testmedia` |
| testmedia | nits: `expected()` sidecar slice aliasing; second Ctrl+C via `context.AfterFunc(ctx, stop)`; default dispositions for video/audio in multi-track MKVs | M1-19 review | next change |
| http | restapi and webui log the path (and bind / file error text) uncut – move `truncateForLog` to a shared `internal/adapters/http/logtext` and use it in both | M1-09 review r2 | first route with path/query parameters |
| http | nits: abort-before-headers logged as status=200; re-panic plain `http.ErrAbortHandler` (not the wrapped value); statuswriter treats 101 as final; "shutdown" → "Close" in two comments | M1-09 review r2 | next change to `internal/adapters/http` |
| http/webui | Content-Security-Policy (`default-src 'self'`, `frame-ancestors 'none'`) for UI responses, tuned to the Vite build and the Cast SDK exception (ADR 0010) | M1-16 review | before M7b (remote access) |
| http/webui | nits: SHA-256 ETags at startup for 304s; replace `testing/fstest.MapFS` in the placeholder with a tiny `fs.FS`; extend the content-type table when images/fonts land in `public/` | M1-16 review | next change |
| http/webui | nits: check the API path before the method in webui; Fallback doc wording; nil-fallback guard test row; cross-reference `isAPIPath` / `isAPIName` | M1-16 review r2 | next change |
| ui-tokens | add a dedicated `danger`/error colour token (the collapsed-sidebar error indicator currently uses the coral accent) | M1-17 | M5 (web UI complete) |
| api-client | nits: `client.ts` comment overstates that the scheme is secret-free (describe the echo as a bounded trade-off); `@throws` should say the scheme is lower-cased and matched case-insensitively; negative tests for non-http schemes without a literal `scheme://` prefix (`"  ftp://…"`, `"ftp:/…"`) | M1-17b review r2 | next change to `web/packages/api-client` |
| web/e2e | one unidentified e2e failure ("1 failed, 9 passed") in 11 implementer runs, not reproduced in 886 later runs; keep `playwright-report` when it recurs | M1-17b r2 | when it recurs |
| docs | ADR 0002 rename checklist: list the docs that carry the name (`README.md`, `CLAUDE.md`, `docs/development.md`, `packaging/docker/README.md`) | M1-22 review | before the final name is chosen |
| docs | coverage without cgo: add `"-coverprofile=coverage.out"` to the Docker race line (or document a non-race coverage run) so the coverage gate works without a C toolchain | M1-22 review | next change to the Commands section |
| docs | nits: tagged race line in CLAUDE.md; Go-job skip condition = "no staged `*.go`"; `sha256:` digest prefix; Linux `--user` for sqlc in Docker; `[net]` on Docker lines; publishing only via mivo-release with approval; unset `MIVO_*` after the isolated run; pnpm row in the winget column; quote examples in `.testcoverage.yml` / `tools/go.mod`; verify the Docker Desktop version | M1-22 review + test | next change to `docs/development.md` |
| docker | `debian:stable-slim` floats across Debian majors (FFmpeg/libc change silently) – pin by digest (or codename) with Renovate; stamp the commit into the binary (`-X`) since `-buildvcs=false` | M1-20 review | M9 (distribution) |
| docker | set `HOME`/`XDG_CACHE_HOME` (e.g. `/config`) for fontconfig/libass caches before subtitle burn-in | M1-20 review | M3/M4 |
