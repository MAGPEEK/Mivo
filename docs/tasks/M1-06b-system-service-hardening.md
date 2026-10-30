# M1-06b Harden the system service
Owner: mivo-backend
Depends on: M1-06
Files/areas: `internal/domain/system/**`, `internal/app/system/**`

## Goal
Resolve the M1-06 review findings before the HTTP and SQLite adapters build on the service.

## Acceptance criteria
- [ ] `EnsureIdentity` loads without holding a lock; only the create path is serialised, with a
      context-aware wait (1-slot channel semaphore with `select` on `ctx.Done()`), and it loads
      again after acquiring. Concurrent first calls still save exactly once (test with `-race`);
      a caller whose `ctx` is cancelled while waiting returns `ctx.Err()` (test).
- [ ] If `SaveIdentity` returns `domain.ErrConflict`, the service loads once more and returns the
      stored identity (test with a fake that conflicts).
- [ ] `ServerID` validation rejects the nil UUID (`00000000-0000-0000-0000-000000000000`) and the
      max UUID (`ffffffff-…`) with `ErrInvalid` (table test).
- [ ] Health `Detail` never contains raw adapter error text: fixed messages
      ("database ping failed", "database ping timed out") – the raw error is returned to the
      caller separately or left for adapters to log; document the choice in the code comment.
      Timeout wording does not claim a specific duration when the caller's deadline fired first.
- [ ] `NewService` rejects an empty `ProductName` with `ErrInvalid`.
- [ ] No "Mivo" literal in comments or test data (use a neutral name such as `TestProduct`).
- [ ] Gates: 100 % coverage for both packages stays, golangci-lint 0 issues, `go test -race -count=3` green.

## Notes
- Source: M1-06 reviewer (minor 1–3, nits 1–4). The APIVersion ↔ `/api/v1` drift test belongs to M1-09.

## Deferred (from review, APPROVE) – pick up with the next change to internal/app/system
- identity.go:92 – on reload-after-conflict failure wrap only the load error (`%w`), name the conflict with `%v`, so HTTP maps it to 500 not 409/404.
- service_test.go – concurrent test also asserts generator `callCount() == 1`.
- health.go – after a fired deadline prefer `pingContext.Err()` over the adapter's error to choose "timed out".
- Nits: check `ctx.Err()` before the semaphore `select`; `t.Cleanup` to release paused savers; distinct error context for the reload.
- From M1-08: `createIdentity` returns `CreatedAt` with ns precision, storage keeps ms → first start vs. restart differ. Truncate to `time.Millisecond` in app/system and test it.
