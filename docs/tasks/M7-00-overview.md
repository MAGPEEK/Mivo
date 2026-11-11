# Milestone 7 "Nutzer und Verwaltung" – overview (0.1.0 scope)

Scope (plan "Funktionsumfang für beta.1": Konten, PIN, Weiterschauen), kept lean for 0.1.0:
local accounts with argon2id sign-in and sessions, first-run admin setup, profiles with a
4-digit PIN (US-01…03), managed profiles (US-10), library access per account/profile (US-22
without single titles), content limits (US-11 without labels, US-12), users list and admin
editing (US-20), devices with revoke (US-25, moved from M7b), watch state with resume, watched
marks, badges and the Home rows Continue watching / Up next (HO-03, HO-04, LB-01, LB-42,
DT-05), a CLI password recovery. Decisions: [ADR 0032](../adr/0032-accounts-sessions-and-access.md)
on top of ADR 0006, 0024, 0027 §3 and 0028 (§4/§7 refined). Prototype:
`docs/design/prototypes/admin.html` (approved); sign-in and setup screens get a prototype first
(M7-29). Migrations: `00011` accounts, `00012` metadata settings (M6-14), `00013` watch state, `00014` age ratings, merged in
this order.

## What exists (do not re-plan)

- Rows service with a `LibrarySource` plug-in point (`app/rows.AllLibraries`) and `RowKind`
  that already reserves `continueWatching`/`upNext` (M5-12/13); cards with a badge slot, row
  component, detail actions, overflow menu, player with `startPositionSeconds` (M5, M2).
- `manageGuard` (local peer + same-origin + JSON) for library management (ADR 0028), LAN guard,
  problem mapping in `restapi`, strict-server stubs pattern (`metadata_stub.go`).
- `items.content_rating` text from NFO and TMDB (M3, M6). No user-owned data exists yet.

## Tasks

| ID | Title | Owner | Depends on |
|---|---|---|---|
| M7-01 | Contract: setup, sign-in, me, profile selection, own devices | mivo-backend | none |
| M7-02 | Contract: users, managed profiles, library access, content limits, all devices | mivo-backend | M7-01 |
| M7-03 | Contract: watch state, progress, viewer state, row kinds | mivo-backend | M7-02 |
| M7-04 | Domain: accounts and profiles | mivo-backend | none |
| M7-05 | Domain: content ratings, limits and viewer access | mivo-backend | none |
| M7-06 | Domain: watch state rules | mivo-backend | none |
| M7-07 | Adapter: argon2id hasher and session tokens | mivo-backend | none |
| M7-08 | SQLite: migration 00011 and account/profile store | mivo-backend | M7-04, M7-05 |
| M7-09 | SQLite: session and device store | mivo-backend | M7-08 |
| M7-10 | SQLite: migration 00013 watch state store and row queries | mivo-backend | M7-06, M7-08 |
| M7-11 | SQLite: migration 00014 age ratings | mivo-backend | M7-05; merges after M7-10 |
| M7-12 | SQLite: restriction in listing, facets, letters, folders | mivo-backend | M7-11 |
| M7-13 | SQLite: restriction in rows, search, children, summaries, detail | mivo-backend | M7-12 |
| M7-14 | App: viewer-scoped catalog | mivo-backend | M7-13, M7-15 |
| M7-15 | App: setup, sign-in, sessions, viewer context | mivo-backend | M7-04, M7-05 |
| M7-16 | App: profile selection, PIN lockout, devices | mivo-backend | M7-15 |
| M7-17 | App: user and profile administration | mivo-backend | M7-04, M7-05 |
| M7-18 | App: watch state use cases | mivo-backend | M7-06, M7-15 |
| M7-19 | App: Continue watching and Up next rows | mivo-backend | M7-14, M7-18 |
| M7-20 | HTTP: session middleware, setup gate, CSRF, admin guard | mivo-backend | M7-01, M7-15 |
| M7-21 | HTTP: setup, sign-in, me, profile and devices handlers | mivo-backend | M7-16, M7-20 |
| M7-22 | HTTP: user administration handlers | mivo-backend | M7-02, M7-17, M7-20 |
| M7-23 | HTTP: watch handlers, viewer state, row kinds | mivo-backend | M7-03, M7-19, M7-20 |
| M7-24 | Secret hygiene: redaction words and database file modes | mivo-backend | none |
| M7-25 | Wiring in `cmd/mivo` | mivo-backend | M7-07, M7-09, M7-10, M7-21, M7-22, M7-23, M7-27, M7-28, M7-40, M7-41 |
| M7-26 | CLI: `admin reset-password` | mivo-backend | M7-25 |
| M7-27 | Scripts: playback smoke signs in | mivo-media | M7-01 |
| M7-28 | E2E harness: first-run setup and signed-in state | mivo-frontend | M7-01 |
| M7-29 | Prototype: sign-in, first-run setup, change password | mivo-ux | none |
| M7-30 | Web: session, sign-in, setup step, route guard, sign out | mivo-frontend | M7-01, M7-29 |
| M7-31 | Web: profile picker, PIN pad, profile menu (US-01…03, SH-07) | mivo-frontend | M7-30 |
| M7-32 | Web: Admin › Users (US-20, US-22, US-11) | mivo-frontend | M7-02, M7-30 |
| M7-33 | Web: managed profiles editor (US-10) | mivo-frontend | M7-32 |
| M7-34 | Web: devices and revoke (US-25) | mivo-frontend | M7-02, M7-30 |
| M7-35 | player-core progress reporter and player integration | mivo-frontend | M7-03 |
| M7-36 | Web: resume, mark as watched, badges (DT-05, LB-41, LB-42) | mivo-frontend | M7-35 |
| M7-37 | Web: Continue watching and Up next rows (HO-03, HO-04, LB-01) | mivo-frontend | M7-36 |
| M7-38 | E2E: setup, sign-in, PIN, content limit, resume | mivo-frontend | M7-25, M7-31, M7-33, M7-34, M7-37 |
| M7-39 | Docs: first run, recovery, upgrade notes, spec updates | mivo-backend | M7-25, M7-26 |
| M7-40 | Trusted reverse proxies (`server.trusted_proxies`) | mivo-backend | M7-20, M7-21 |
| M7-41 | Unique profile names per account (migration 00015) | mivo-backend | M7-22 |

Added 2026-11-11 (orchestrator decisions, ADR 0032 amendments): M7-40 after the M7-21 review
(reverse proxy), M7-41 after the M7-22 review, and the ancestor visibility rule in M7-13 and
M7-14 after the M7-12 review (an item is visible only if it and every ancestor pass the
content limit).

Web tasks use the generated client with a stub server (MSW-style, as in M5/M6), so they start
right after their contract task, before the backend handlers exist.

## Waves

1. **Now:** M7-01, M7-04, M7-05, M7-06, M7-07, M7-24, M7-29 (all disjoint).
2. M7-02, M7-08, M7-15, M7-17, M7-27, M7-28, M7-30.
3. M7-03, M7-09, M7-10, M7-11 (merge after M7-10), M7-16, M7-18, M7-20, M7-31, M7-32, M7-34.
4. M7-12, M7-21, M7-22, M7-33, M7-35.
5. M7-13, M7-36.
6. M7-14, M7-37.
7. M7-19.
8. M7-23.
8a. **Before M7-25, in parallel with waves 5–8:** M7-40 (startable now) and M7-41 (after
    M7-22). They share no files with each other or with M7-13/M7-14/M7-19.
9. M7-25 (integration; turns sign-in on), then M7-26, M7-38, M7-39 in parallel.

Critical path: M7-05 → M7-11 → M7-12 → M7-13 → M7-14 → M7-19 → M7-23 → M7-25 → M7-38.

## Same-file rules (what may run in parallel)

- `api/openapi.yaml`: M7-01 → M7-02 → M7-03 strictly in sequence; each adds 501 stubs in its
  own `restapi/*_stub.go`.
- Domain M7-04/05/06, adapter M7-07, hygiene M7-24 and prototype M7-29 share no files.
- SQLite: M7-09, M7-10 and M7-11 own separate migrations, query files and store files and may
  run in parallel; the generated `sqlcgen/models.go` is regenerated after each merge (never
  hand-merged); migration files merge in number order. M7-12 → M7-13 are serial (listing SQL,
  then the rows/search ports in `app/rows`, `app/search`).
- App: `app/auth` M7-15 → M7-16 serial; M7-15 creates `app/access` (viewer context only),
  M7-14 adds the scoped library source there; `app/users` M7-17 and `app/watch` M7-18 are
  disjoint; `app/rows` is edited by M7-13 → M7-14 → M7-19 in that order.
- HTTP: M7-20 owns `adapters/http/server` and the problem mapping. M7-21, M7-22 and M7-23
  each own their `restapi/<area>*.go` files but all add one field to `restapi/api.go` and
  `restapi/options.go`: develop in parallel, merge one at a time.
- M7-40 edits `platform/config`, `adapters/http/server` (guards, same-origin, one `Options`
  field) and only the `secure` flag in `restapi/auth.go`. It must not run at the same time as
  another task in `adapters/http/server`, and it stays out of `restapi/api.go`,
  `restapi/options.go` (M7-22/M7-23) and `cmd/mivo`; M7-25 passes the setting to the server.
- M7-41 owns migration `00015`, the profile statements in `queries/accounts.sql` and
  `account_store*.go`; `sqlcgen` is regenerated after the merge. If M7-13 chooses the stored
  visibility columns it takes migration `00016` and merges after M7-41 (number order).
- `cmd/mivo` is touched only by M7-25 and then M7-26.
- Web: features are disjoint (`features/auth`, `features/profiles`, `features/users`,
  `features/devices`, `packages/player-core` + `features/player`, `features/item` +
  `features/library` cards, `features/home`). `app/router.ts` and the locale files get small
  additions from M7-30/31/32/34: merge one at a time. M7-35 → M7-36 → M7-37 are serial
  (resume entry, badge component, rows reuse it).
- M7-27 (Go script) and M7-28 (Playwright global setup) must work against servers **with and
  without** sign-in, so they merge before M7-25 and keep `main` green.

## Deferred to after 0.1.0 (reason)

| Item | Reason |
|---|---|
| Quick Connect / TV pairing (ADR 0007) | The webOS app is not part of 0.1.0 (plan: M10 after the first stable release); bearer sessions already serve it. |
| Invites (US-21) | Needs expiring one-time links and a public join page; admin-created accounts cover a household. |
| Guest profile (US-24) | Unauthenticated LAN access widens the attack surface; not needed for a household. |
| Permissions (US-23: download, subtitle upload, transcode, remote) | Download and upload do not exist yet; remote is M7b. |
| Single-title sharing, labels in limits (US-22/US-11 parts, ED-04) | Labels and collections are not in 0.1.0; age limits cover parental needs. |
| API keys | No third-party integrations in 0.1.0; bearer sessions from `client=cli` serve scripts. |
| 2FA | Admin is LAN-only until M7b; 2FA lands with remote access. |
| LDAP/OIDC | Post-v1 per plan and ADR 0006. |
| Plex/Jellyfin import | Plan: after v0.1; needs watch state first (this milestone). |
| Playlists, collections | Separate features with own UI; not in the beta.1 scope list. |
| Backup/restore (AD-29) | Needs a consistent SQLite snapshot design and restore UI; manual copy of the data dir is documented instead. |
| Webhooks (AD-30), scheduler/maintenance window (AD-28, AD-12) | No event bus yet; periodic scan exists in config. |
| Activity log (AD-11), live log (AD-42), sign-in log (US-26) | Need persistent events and streaming; logs stay in files. |
| Admin settings pages AD-20, AD-23…26, AD-41, Stop session (AD-10) | `config.yaml` stays authoritative; pages follow once writable settings are designed. |
| Watch-state re-attach after folder rename or library re-add | Data is kept (`attach_key`, no FK); the matching job is extra scan work. |
| Music listen state, Recently played (HO-06), LB-06/LB-07 rows, Unwatched/In-progress filters and progress sort | Need profile joins inside the faceted listing; rows and badges cover the core. |
| Sidebar layout per profile on the server | Per-device storage from M5 is enough for 0.1.0. |
