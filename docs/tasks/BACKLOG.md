# Backlog of deferred review findings

Non-blocking findings from APPROVE reviews that are not yet scheduled in a task. Each entry
names its source; pick entries up the next time the area is touched, or schedule them as a task.
Entries already carried into a later task file are listed there, not here.

Scheduled in milestone 2 (removed from this table): config/paths → M2-04; testmedia encoders and
nits → M2-06; http logtext, http nits, webui r2 nits → M2-08; sqlite reader pool and nits →
M2-10; sqlite single-instance lock → M2-13; api-client nits → M2-01; docs coverage and docs nits
→ M2-36.

| Area | Item | Source | Due before |
|---|---|---|---|
| logging | `pin` gaps, extra words (`passcode`, `bearer`, `jwt`, `otp`, `pwd`, …), secret config key naming rule | M1-05c (Deferred) | M6 (TMDB key) / M7 (PINs) |
| app/system | reload-error wrapping, generator-count assertion, deadline classification, `CreatedAt` ms truncation | M1-06b (Deferred) | next change to `internal/app/system` |
| archtest | `go/build`, `go/importer` and file-reading std functions in the domain | M1-03b (Deferred) | next change to `internal/archtest` |
| sqlite | DB/WAL/SHM files created 0644 inside a 0750 data dir → pre-create 0600 or document umask | M1-08 review | M7 (auth data) |
| http/webui | Content-Security-Policy (`default-src 'self'`, `frame-ancestors 'none'`) for UI responses, tuned to the Vite build and the Cast SDK exception (ADR 0010); hls.js (M2-27) needs `worker-src blob:` and `media-src 'self' blob:` | M1-16 review, M2 planning | before M7b (remote access) |
| http/webui | nits: SHA-256 ETags at startup for 304s; replace `testing/fstest.MapFS` in the placeholder with a tiny `fs.FS`; extend the content-type table when images/fonts land in `public/` | M1-16 review | next change |
| ui-tokens | add a dedicated `danger`/error colour token (the collapsed-sidebar error indicator currently uses the coral accent) | M1-17 | M5 (web UI complete) |
| web/e2e | one unidentified e2e failure ("1 failed, 9 passed") in 11 implementer runs, not reproduced in 886 later runs; keep `playwright-report` when it recurs | M1-17b r2 | when it recurs |
| docker | `debian:stable-slim` floats across Debian majors (FFmpeg/libc change silently) – pin by digest (or codename) with Renovate; stamp the commit into the binary (`-X`) since `-buildvcs=false` | M1-20 review | M9 (distribution) |
| docker | set `HOME`/`XDG_CACHE_HOME` (e.g. `/config`) for fontconfig/libass caches before subtitle burn-in | M1-20 review | M3/M4 |
| domain/library | nits: explicit regression tests for `//server`, `\\?\`, `\\.\` (reject) and `%2e%2e`, `...`, full-width dots (accept); unreachable "Untitled" fallback (i18n placeholder instead?); `ItemKind.IsValid`; `MaxNameLength` in tests; whitespace-only root; shared `nulCharacter` placement | M2-03 review | next change to `internal/domain/library` |
| domain/library | NFC normalisation of titles/sort titles (macOS NFD file names over SMB sort differently) | M2-03 review | M3 (name parser) |
