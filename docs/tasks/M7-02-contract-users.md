# M7-02 Contract: users, managed profiles, library access, content limits, all devices
Owner: mivo-backend
Depends on: M7-01
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client`, new `internal/adapters/http/restapi/users_stub.go`

## Goal
The admin side of accounts has a contract, so the user administration handlers (M7-22) and the
Admin › Users, profiles and devices screens (M7-32, M7-33, M7-34) can be built in parallel.

## Acceptance criteria
- [ ] Tag `users`: `GET /api/v1/users` (US-20 list: id, name, role, status `active|disabled`,
      lastActiveAt, contentLimit of the primary profile, profile count);
      `POST /api/v1/users` (name, initial password, role, libraryAccess, contentLimit) → `201`;
      `GET|PATCH|DELETE /api/v1/users/{userId}`; `PUT /api/v1/users/{userId}/password` → `204`.
      `409 last_admin` documented for delete/disable/demote of the last enabled admin,
      `409 name_taken` for duplicate names.
- [ ] Schemas `LibraryAccess` (`all: bool`, `libraryIds: uuid[]`) and `ContentLimit`
      (`preset` `youngChildren|children|teens|adults|custom`, `maxAge` 0/6/12/16/18 or null,
      `hideUnrated`), per ADR 0032 §7.
- [ ] Profiles: `GET|POST /api/v1/users/{userId}/profiles`, `PATCH|DELETE /api/v1/profiles/{profileId}`
      (name, avatarColor from a fixed token list, `pin` 4 digits write-only or `null` to clear,
      contentLimit, libraryAccess); allowed for an admin and for the owning account;
      `409 profile_limit` above 10 managed profiles; the primary profile cannot be deleted.
- [ ] `GET /api/v1/devices` (all sessions with account name; admin) and
      `DELETE /api/v1/devices/{deviceId}` → `204`.
- [ ] `users_stub.go` answers `501` for every new operation.
- [ ] Tests: `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean;
      OpenAPI lint passes; `go test ./internal/adapters/http/...` green.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Admin-only operations carry the ADR 0032 §9 note (admin viewer + local peer). No invite,
guest or permission fields (deferred, see overview); leave room by using objects, not flags at
the top level.
