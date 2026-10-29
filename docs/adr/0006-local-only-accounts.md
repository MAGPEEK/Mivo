# 0006 Local-only user accounts

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

Plex requires a cloud account. Mivo users want to own their server without depending on a
third-party service.

## Decision

- Accounts exist only in the server's SQLite database. Passwords use argon2id. Sessions use
  server-issued tokens.
- Friends join through **invite links** that the admin creates. No cloud identity provider,
  no central account service.
- Profiles with PIN, parental ratings and per-library permissions come in milestone 7.
- The first-run wizard creates the admin. There are no default credentials.

## Consequences

- No vendor lock-in and no outage dependency.
- No "forgot password via provider". The admin resets passwords. A local CLI recovery path
  is needed later.
- SSO (LDAP/OIDC) remains a post-v1 option and does not change this default.
