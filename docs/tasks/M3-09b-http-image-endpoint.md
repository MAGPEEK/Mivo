# M3-09b HTTP: image endpoint
Owner: mivo-backend
Depends on: M3-02, M3-09
Files/areas: `internal/adapters/http/images/**` (new package: `doc.go`, `routes.go`,
`handler.go` + tests)

## Goal
`GET /api/v1/images/{imageId}?width=&tag=` serves posters and other artwork in fixed sizes with
correct caching headers, so grids load small images that browsers keep. UX: LB-44, LB-40;
ADR 0027 §8, contract of M3-02.

## Acceptance criteria
- [ ] A plain `net/http` handler mounted like the stream routes (pattern constant, mounted
      through `restapi.Options.Mounts` by M3-11b); `GET` also answers `HEAD`; other methods →
      405 problem.
- [ ] `imageId` must be a UUID, `width` one of `artwork.Width`, `tag` optional lowercase hex
      ≤ 64; anything else → 400 `invalid` problem without echoing the value.
- [ ] 200 with `Content-Type` (`image/jpeg`/`image/png`), `Content-Length`, `ETag:
      "<tag>-<width>"`, `X-Content-Type-Options: nosniff`, `Cache-Control: private,
      max-age=31536000, immutable` when the tag matched, else `private, no-cache`;
      `If-None-Match` with the current ETag → 304 without body (use `http.ServeContent` or
      equivalent so `Range` and conditional requests behave).
- [ ] Errors: `ErrNotFound` → 404, `ErrUnsupported` → 422 `unsupported`, `ErrUnavailable` → 503
      `unavailable` with `Retry-After`, context cancelled → no body and no error log, anything
      else → 500 without internal text.
- [ ] Image requests are logged at debug while they succeed (add the pattern to the quiet
      patterns like `stream.QuietPatterns`).
- [ ] Tests with a fake image service: every status, headers, HEAD, 304, bad parameters, quiet
      logging pattern; `api/spec_test.go` (from M3-02) still passes.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/http/...`, coverage gate.

## Notes
- The LAN-only guard (ADR 0018 Notes M2-31) covers this route automatically; authentication
  for images is milestone 7 (cookie or signed URL).
- Keep the problem writing consistent with `restapi` (reuse its exported helper if one exists;
  otherwise write the small RFC 9457 body here and note the duplication for a later cleanup
  task in the REPORT).
