# M1-05b Harden secret redaction in logging
Owner: mivo-backend
Depends on: M1-05
Files/areas: `internal/platform/logging/**`

## Goal
Close the redaction gaps found in the M1-05 review before the first secret-bearing config key
or HTTP request logging exists.

## Acceptance criteria
- [ ] Key matching normalises keys before comparison (case-insensitive, `_` / `-` / `.` removed) and
      covers at least: password, secret, token, apikey, authorization, cookie, setcookie,
      proxyauthorization, accesstoken, refreshtoken, clientsecret, jwtsecret, privatekey.
      `tokenCount` and `passwordHint` stay visible (document the matching rule precisely and test it).
- [ ] Table tests for: `api_key`, `x-api-key`, `access_token`, `Set-Cookie`, a `slog.LogValuer`
      returning a group under a sensitive key, `logger.WithGroup("authorization")`.
- [ ] Error-message tests as a table: level `"loud"`, format `"xml"`, format `""` (each quotes the bad value).
- [ ] Parallel subtests do not share a `bytes.Buffer`.
- [ ] The package doc states the limit honestly: values logged as a whole via `slog.Any`
      (structs, maps, `http.Header`) are not inspected; types holding secrets must implement
      `slog.LogValuer`, and HTTP headers must be logged through an allowlist (to be done in
      `adapters/http`).
- [ ] Gates: coverage 100 % for the package stays, golangci-lint 0 issues, go test -race green.

## Notes
- Source: M1-05 reviewer findings (minor 1–4, nits 1 and 3).
- Keep the key list in one place (e.g. a function returning the normalised keys).
