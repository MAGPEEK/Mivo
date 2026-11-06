# M7-01 Contract: setup, sign-in, me, profile selection, own devices
Owner: mivo-backend
Depends on: none
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client`, new `internal/adapters/http/restapi/auth_stub.go`

## Goal
The wire contract for signing in exists, so the session middleware (M7-20), the handlers
(M7-21), the web sign-in/picker screens (M7-30, M7-31) and the e2e/smoke harnesses (M7-27,
M7-28) can be built in parallel against it.

## Acceptance criteria
- [ ] New tag `auth` with: `GET /api/v1/setup` → `{required}`; `POST /api/v1/setup` (`name`,
      `password`) → `201` with the session (cookie and, for `client` ≠ `web`, `token`), `409`
      when an account exists; `POST /api/v1/auth/session` (`name`, `password`, `client`
      `web|tv|cli`, optional `deviceName`) → `200` session or `401 invalid_credentials`,
      `429` with `Retry-After`; `DELETE /api/v1/auth/session` → `204`.
- [ ] `GET /api/v1/me` → account (id, name, role), `currentProfile` (nullable) and `profiles`
      for the picker (id, name, avatarColor, hasPin, isPrimary, isAdmin);
      `PUT /api/v1/me/profile` (`profileId`, optional `pin`, `remember`) → `200` me,
      `403 pin_invalid`, `429 profile_locked` with `retryAfterSeconds`;
      `DELETE /api/v1/me/profile` → `204`; `PUT /api/v1/me/password` (`currentPassword`,
      `newPassword`) → `204`.
- [ ] `GET /api/v1/me/devices` (id, name, client, platform, lastSeenAt, addressClass `lan|remote`,
      `current`) and `DELETE /api/v1/me/devices/{deviceId}` → `204`.
- [ ] Security schemes `sessionCookie` (apiKey in cookie, name documented as brand-derived) and
      `bearer`; applied globally; `health`, `system/info`, setup and `POST auth/session` are
      `security: []`. Problem codes `unauthenticated`, `setup_required`, `profile_required`,
      `invalid_credentials`, `pin_invalid`, `profile_locked` are documented with their status.
- [ ] Setup and every write mention the ADR 0028/0032 local-peer + same-origin rule.
- [ ] `auth_stub.go` answers `501` for every new operation; the server still builds.
- [ ] Tests: `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean;
      the OpenAPI lint step passes; `go test ./internal/adapters/http/...` green.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
ADR 0032 §3–§6. Passwords and PINs are `writeOnly`, `format: password`. The session token is
never part of `me`. Keep the schemas small; M7-02 adds the admin side, M7-03 watch state.
