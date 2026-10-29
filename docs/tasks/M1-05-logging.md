# M1-05 Structured logging with slog
Owner: mivo-backend
Depends on: M1-02, M1-03
Files/areas: `internal/platform/logging/**`

## Goal
The server writes structured logs in text or JSON at a configurable level. Secrets never
appear in log output.

## Acceptance criteria
- [ ] `logging.New(Options{Level, Format, Writer})` returns a `*slog.Logger` or an error for
      an unknown level or format. Levels: debug, info, warn, error. Formats: text, json.
- [ ] Records below the configured level are dropped (tested for each level).
- [ ] JSON output is valid JSON per line with `time`, `level` and `msg` (tested by decoding).
- [ ] Attributes whose key is `password`, `token`, `secret`, `apiKey` or `authorization`
      (case-insensitive, also nested in groups) are written as `[REDACTED]` (tested).
- [ ] The package never calls `slog.SetDefault` and keeps no package-level logger.
- [ ] Tests: table-driven tests for level, format and redaction. Package coverage ≥ 85 %.
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check pass.

## Notes
- Use `HandlerOptions.ReplaceAttr` for redaction. Keep the redaction key list as a named,
  documented constant set.
- Accept plain strings in `Options`, not the config struct, so this package does not depend
  on `platform/config` and can run in parallel with M1-04.
- Log files and live log streaming are not part of milestone 1 (stderr only; Docker reads
  stdout/stderr).
