# M7-07 Adapter: argon2id hasher and session tokens
Owner: mivo-backend
Depends on: none
Files/areas: new `internal/adapters/credentials/`, `go.mod`, `go.sum`

## Goal
Passwords and PINs are hashed with argon2id within the memory budget of a 1 GB NAS, and session
tokens are generated and hashed in one place.

## Acceptance criteria
- [ ] `Hasher` with `Hash(ctx, secret string) (string, error)` and
      `Verify(ctx, secret, encoded string) (ok, needsRehash bool, err error)`; PHC string
      `$argon2id$v=19$m=19456,t=2,p=1$…`, 16-byte salt, 32-byte key, constant-time compare;
      parameters are named constants; a malformed encoding wraps `domain.ErrInvalid`.
- [ ] At most 2 hash operations run at once (semaphore honouring `ctx` cancellation);
      `needsRehash` is true when the stored parameters are weaker than the current ones.
- [ ] `DummyVerify(ctx, secret)` spends the same work as a real verify (for unknown names).
- [ ] `Tokens` with `New() (token, hash string, err error)` (32 bytes crypto/rand, base64url
      without padding; hash = hex SHA-256) and `HashToken(token) string`.
- [ ] Dependency `golang.org/x/crypto` added; `go mod tidy -diff` clean; `govulncheck` clean.
- [ ] Tests: `go test -race ./internal/adapters/credentials/` covering round trip, wrong
      secret, tampered salt/hash, parameter upgrade (`needsRehash`), cancelled context while
      waiting for the semaphore, token uniqueness over 1000 draws and fixed hash vectors.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The consumer ports live in `app/auth` and `app/users` (M7-15, M7-17); match the signatures
above so wiring needs no adapter glue. Tests may lower the cost through an unexported option to
stay fast; one test uses the production parameters.
