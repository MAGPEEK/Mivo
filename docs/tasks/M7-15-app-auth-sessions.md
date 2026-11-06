# M7-15 App: setup, sign-in, sessions, viewer context
Owner: mivo-backend
Depends on: M7-04, M7-05
Files/areas: new `internal/app/auth/`, new `internal/app/access/` (context helpers only)

## Goal
The server can be set up once, accounts sign in and out, and every request's token resolves to
a viewer, with throttling and caching per ADR 0032 §2–§6.

## Acceptance criteria
- [ ] `app/access`: `WithViewer(ctx, access.Viewer)` and `ViewerFrom(ctx) (Viewer, bool)`.
- [ ] Ports (1–3 methods each): account reader/creator, session store, `Hasher`
      (`Hash`, `Verify`, `DummyVerify` as in M7-07), `TokenSource`, clock.
- [ ] `SetupRequired`, `Setup(name, password, device)` (only while no account exists, signs in);
      `SignIn(name, password, device)` → session + token (+ auto-selected profile when allowed);
      `SignOut(token)`; `ChangePassword` (revokes other sessions); rehash on `needsRehash`.
- [ ] `Resolve(ctx, token, peer)` → session, account, profile and `access.Viewer` (effective
      access and limit built from account and profile inputs), `domain.ErrUnauthenticated` for
      unknown, expired, revoked or disabled; results cached ≤ 30 s, evicted on sign-out,
      revoke, password change, disable; `last_seen` touch delegated to the store.
- [ ] Throttling per name and per peer as ADR 0032 §5 returns `domain.ErrRateLimited` with a
      retry-after; unknown names run `DummyVerify`; error never reveals whether a name exists.
- [ ] `PurgeExpired(ctx)` for a periodic call.
- [ ] Tests: fakes only (`go test -race ./internal/app/auth/ ./internal/app/access/`):
      setup twice, concurrent setup, wrong password, disabled account, throttle and back-off
      with a fake clock, cache eviction on revoke, rehash path.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Counters and cache are in-memory structs owned by the service (no globals). Profile selection
and devices are M7-16 in the same package – leave a seam (a `profiles` file) but do not
implement it here.
