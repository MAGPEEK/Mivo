# M7-22 HTTP: user administration handlers
Owner: mivo-backend
Depends on: M7-02, M7-17, M7-20
Files/areas: `internal/adapters/http/restapi/users*.go` (replaces `users_stub.go`), one field each in `restapi/api.go` and `restapi/options.go`

## Goal
The M7-02 operations work end to end on top of `app/users` and the device part of `app/auth`.

## Acceptance criteria
- [ ] Users, profiles and all-devices operations map request bodies to the use cases and errors
      to `403 forbidden`, `404`, `409 last_admin|name_taken|profile_limit`, `400 invalid` with
      field names.
- [ ] `contentLimit.preset` and `maxAge` are consistent in responses (preset derived from
      `maxAge` when not custom); `pin` is never returned, only `hasPin`.
- [ ] Tests: handler tests with fake services (`go test ./internal/adapters/http/restapi/ -run 'User|Profile|Device'`)
      for every status code above and for an owner editing their own managed profile.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Admin checks live in the use cases and in the guard (M7-20); handlers do not repeat them.
