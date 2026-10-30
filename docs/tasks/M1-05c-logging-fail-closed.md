# M1-05c Make secret redaction fail closed
Owner: mivo-backend
Depends on: M1-05b
Files/areas: `internal/platform/logging/**`

## Goal
Secret-holding keys with qualifiers or plurals must not slip through before the first
secret-bearing config key (TMDB key, stream signing key, profile PIN) lands.

## Acceptance criteria
- [ ] Normalisation removes every character that is not an ASCII letter or digit, then lower-cases.
- [ ] Rule changes from "ends with" to "contains a sensitive word", with a small allowlist of safe
      trailing qualifiers that keep a key visible: `count`, `hint`, `type`, `length`, `expiry`,
      `expiresat`, `ttl`, `id` is NOT in the allowlist. Unknown qualifiers fail closed.
- [ ] Sensitive words include at least: password, passwd, passphrase, secret, token, apikey,
      authorization, cookie, credential, privatekey, signingkey, encryptionkey, sessionid, pin
      (as a whole normalised key or with a sensitive-safe boundary – document how `pin` avoids
      matching e.g. `spinner`/`pinned`, and test it).
- [ ] Table tests: redacted – `secretKey`, `aws_secret_access_key`, `signingKey`,
      `authorizationHeader`, `cookieHeader`, `passwordHash`, `cookies`, `tokens`, `credentials`,
      `api key`, `auth:token`, `auth/token`, `pin`; visible – `tokenCount`, `token_count`,
      `passwordHint`, `secretary`? (decide and document), `publicKey`, `spinner`, `pinned`.
- [ ] Package doc states the rule once and refers to the function holding the word lists (no
      duplicated word list in prose); it mentions intentional over-redaction (e.g. `nextPageToken`).
- [ ] The missing-writer test case no longer passes trivially (no empty `wantQuote` check).
- [ ] Gates: 100 % coverage, golangci-lint 0 issues, go test -race green.

## Notes
- Source: M1-05b reviewer findings (minor 1–2, nits 1–3).
