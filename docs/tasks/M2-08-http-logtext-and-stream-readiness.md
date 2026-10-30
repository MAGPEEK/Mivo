# M2-08 HTTP adapter: bounded log text, streaming-friendly writers, quiet stream logs
Owner: mivo-backend
Depends on: M2-01
Files/areas: `internal/adapters/http/logtext/**` (new), `internal/adapters/http/server/**`,
`internal/adapters/http/statuswriter/**`, `internal/adapters/http/restapi/api.go` (only
`requestAttributes`), `internal/adapters/http/webui/**`

## Goal
Before any route with path parameters gets a real implementation, every log record that contains
client-controlled text is bounded, and the HTTP plumbing is ready for long media responses:
`sendfile` stays possible through the wrappers and polling of stream segments does not flood the
info log (BACKLOG, ADR 0024).

## Acceptance criteria
- [ ] `truncateForLog` moves to `internal/adapters/http/logtext` (exported, same behaviour and
      tests); `server`, `restapi.requestAttributes` and `webui` use it for paths, the User-Agent
      and bind/file error text. A test per package logs a 10 KiB path and asserts the record
      holds at most the bound plus marker.
- [ ] `statuswriter.Recorder` implements `io.ReaderFrom` by delegating to the wrapped writer when
      it implements it (bytes still counted), so `http.ServeContent` can use `sendfile`; test with
      a writer that records whether `ReadFrom` was used.
- [ ] `server.Options` gains `QuietPathPrefixes []string`: requests below such a prefix are
      access-logged at debug level when the status is below 400, at info otherwise (table test).
- [ ] BACKLOG row "http nits": an abort before headers is logged with status 0/"aborted" instead
      of 200; re-panic plain `http.ErrAbortHandler` (not the wrapped value); statuswriter does not
      treat 101 as final; "shutdown" → "Close" in the two comments. Tests where behaviour changes.
- [ ] BACKLOG row "http/webui nits (r2)": API path checked before the method in webui; Fallback doc
      wording; nil-fallback guard test row; cross-reference `isAPIPath` / `isAPIName`.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/http/...` (also with
      `-tags webui` after `pnpm -C web build`) + coverage gate pass.

## Notes
- The architect already moved the scheduled rows out of `docs/tasks/BACKLOG.md`. The other
  webui row (SHA-256 ETags, `fstest.MapFS` replacement, content-type table) stays there.
- Do not touch `restapi/library.go` or `restapi/playback.go` (other tasks).
- The prefix value for stream routes is passed by `cmd` in M2-32
  (`/api/v1/playback/sessions/`).
