# M1-05c test report: fail-closed log redaction

Status: PASS (round 1)
Scope: `./internal/platform/logging/...` (files: redact.go, redact_test.go, logging.go, logging_test.go)

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/platform/logging/...` | exit 0, no output |
| test | `go test -race -count=1 -cover ./internal/platform/logging/...` | `ok ... coverage: 100.0% of statements` |
| lint | `golangci-lint run ./internal/platform/logging/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/platform/logging/...` | exit 0, empty diff |
| layer | `go list -deps` of the package | only stdlib and itself (no domain/app/adapters involvement) |

Not run (out of scope for this task): web gates, media, docker, repo-wide coverage thresholds.

## Acceptance criteria

1. Normalisation drops every non-ASCII-letter/digit, then lower-cases: MET. `normaliseKey` (redact.go:149) works on bytes; probe: `Pass-Word`, `sec<U+200B>ret`, `s_e_c_r_e_t`, `se cret` are redacted; `größe` stays visible (test).
2. "Contains a sensitive word unless followed by exactly one safe qualifier"; allowlist count/hint/type/length/expiry/expiresat/ttl, `id` not included: MET. `isSensitiveKey`/`holdsSecret` (redact.go:85, 98), `safeQualifiers` (redact.go:52); `tokenId`, `credentialId` redacted in tests.
3. Sensitive words include all 14 required, `pin` documented and tested: MET. `sensitiveWords` (redact.go:29) has all of them. The `pin` boundary rule is documented at redact.go:117-122 (`matchesAt`) and in the `normaliseKey` doc. `spinner`, `pinned`, `mapping`, `shippingAddress`, `isSpinning`, `PINNED` are visible in `TestNewKeepsNonSensitiveAttributes`.
4. Table tests: MET. Every redacted key from the task is in `TestNewRedactsSensitiveKeys` (redact_test.go:33-56). Every visible key is in `TestNewKeepsNonSensitiveAttributes` (redact_test.go:154-177). `secretary` is decided as redacted, tested (line 51) and documented in logging.go:18-20.
5. Package doc states the rule once, points at the functions holding the word lists, and mentions intentional over-redaction (`nextPageToken`): MET (logging.go:7-20). It has no word list in prose, only examples.
6. Missing-writer test no longer passes trivially: MET. It is a separate `TestNewRejectsMissingWriter` (logging_test.go:160) that checks `errors.Is(err, ErrNoWriter)` and a nil logger. The remaining table in `TestNewRejectsInvalidOptions` has a non-empty `wantQuote` in every row.
7. Gates (100 % coverage, 0 lint issues, race green): MET (see above).

## Probe results

A throwaway internal test run through `go run -overlay`-style `go test -overlay`. It lived in the scratch area only, and `git status` shows nothing new in the repo. It called `isSensitiveKey` directly.

Task table, redacted (all true): secretKey, aws_secret_access_key, signingKey, authorizationHeader, cookieHeader, passwordHash, cookies, tokens, credentials, `api key`, `auth:token`, `auth/token`, pin.

Task table, visible (all false except secretary, as decided): tokenCount, token_count, passwordHint, publicKey, spinner, pinned. `secretary` is redacted by decision.

Adversarial keys, redacted:
`Pass-Word`, `pass_word`, `pass word`, `x_auth_token_value`, `sessionId`, `SESSIONID`, `session-id`, `sessionID`, `Session_Id`, `authorization_header`, `Authorization`, `Proxy-Authorization`, `apiKey`, `API_KEY`, `X-Api-Key`, `apikeys`, `secret_access_key`, `secretAccessKey`, `private-key`, `privateKeyPem`, `signing key`, `encryption_key`, `encryptionKeyId`, `clientSecretId`, `csrfToken`, `set-cookie`, `DB_PASSWORD`, `mysql.password`, `SeCrEt`, `xsecret`, `secret\x00`, `secret\n`, `  token  `, `tokenCount2`, `tokenCountSecret`, `tokenCount_token`, `countToken`, `hintToken`, `tokensHint`, `passwordsHint`, `credentialsCount`, `cookiesCount`, `tokenCOUNTS`, `tokenlengthx`, `tokenExpiresIn`, `tokenTypeValue`, and Cyrillic-prefixed `ТокенToken`.
`pin` variants redacted: pinCode, PIN_CODE, pin_hash, profile_pin, profilePIN, ProfilePin, pin1, 1pin, `pin-`, mypin, userPin, UserPIN, PINs, pinS, pins_count.

Adversarial keys, visible (false):

- Intended by the safe-qualifier rule: `clientSecretType`, `secretCount`, `secretcount`, `secretTtl`, `passwordExpiresAt`, `authorizationCount`, `cookieCount`, `tokenType`, `tokenHint`, `tokenLength`, `tokenTTL`, `tokenExpiry`, `tokenExpiresAt`, `TOKEN_COUNT`, `pinCount`, `pinHint`, `pinnedItems`, `opinion`, `Pinterest`, `mapping`, `shipping`.
- Not covered by the required word list (not a defect against the criteria): `passw0rd`, `pwd`, `pw`, `bearer`, `jwt`, `auth`, `access_key`, `accessKey`, `accessKeyId`, `privKey`, `hmacKey`, `otp`, `totp`, `totpSeed`, `mfaCode`, `recoveryCode`, `licenseKey`, `csrf`, `xsrf`, `dsn`, `connectionString`, `databaseUrl`.
- Non-ASCII look-alikes: `tökeN` and `tokеn` (Cyrillic e). The non-ASCII byte is removed, so the word breaks. This is the documented behaviour of the normalisation and is not realistic for Go-source keys.

No key from the task's tables, and no required-word key, leaks.

## Findings (non-blocking)

1. `clientSecretType` (and any `<sensitive word>Type`/`Count`/`Hint`/`Ttl` key) is visible. This follows the specified rule and mirrors the required `cookieType`/`tokenCount`. It trusts that such keys hold metadata. Keep this in mind when naming config keys: never use a qualifier name for a field that holds the value.
2. Secret-looking words outside the required list (`bearer`, `jwt`, `accessKey`/`access_key`, `hmacKey`, `pwd`, `otp`, `auth`) are visible. Add them to `sensitiveWords` if such keys are ever logged. The planned first secret keys (TMDB key, stream signing key, profile PIN) are covered via `apikey`, `signingkey` and `pin`. A bare `tmdbKey` would NOT be redacted, so name it `tmdbApiKey`/`tmdb_api_key` in config.
3. `spin` and `mypin`-style keys are over-redacted by design (documented).

## History

- Round 1: PASS, all gates green, all criteria met.

REPORT
status: PASS
task: M1-05c
files: docs/reports/M1-05c-test.md
tests: go vet (clean); go test -race -count=1 -cover ./internal/platform/logging/... ok, 100.0%; golangci-lint run 0 issues; golangci-lint fmt --diff empty; probe of about 170 keys, no secret-looking key from the task tables leaks
findings:
- No blocking findings
- clientSecretType and other sensitive-word + safe-qualifier keys are visible by design
- bearer/jwt/accessKey/hmacKey/pwd/otp and a bare tmdbKey are not in the word list; name future secret config keys with apikey/secret/token/signingkey/pin
next: send to mivo-reviewer
