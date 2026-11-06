# 0032 Accounts, sessions, profiles and access control (milestone 7)

- Status: Accepted (orchestrator by owner delegation, 2026-11-06; proposed by the architect the same day – the owner may still object)
- Date: 2026-11-06
- Refines: 0006 (local-only accounts), 0024 (streaming sessions)
- Supersedes: 0028 §4 "Security before accounts" and §7 "Accepted residual risks until M7" in
  part (see Decision 9)

## Context

Milestone 7 brings sign-in, profiles with PIN, per-user library access, content limits and
watch state, scoped to the lean 0.1.0 set in `docs/plan.md` ("Funktionsumfang für beta.1":
Konten, PIN, Weiterschauen). Until now every LAN client sees everything, management is guarded
only by the local-peer + same-origin rule of ADR 0028, and no user-owned data exists (OD-UX-3 B
was decided, but M5 left watch state to M7, so there is nothing to migrate). The target
hardware includes 1 GB ARM NAS devices, so password hashing must stay cheap in memory. ADR 0027
§3 asks that watch state survive rescans; backlog rows ask that a library sync never deletes
user data. UX ids: `docs/design/ux-spec.md` section 10 (US-*), prototype
`docs/design/prototypes/admin.html` (approved).

## Decision

### 1 Accounts and profiles
- **Account** (`users`): id (UUID), name (1–40 runes, NFC, unique case-insensitively), role
  `admin|user`, `disabled`, argon2id password hash, created/updated times. No e-mail.
- Every account has exactly one **primary profile** (created with it, same name, cannot be
  deleted on its own) and up to 10 **managed profiles** (no password, chosen in the picker).
  Watch state, the PIN and the content limit belong to a profile.
- **Admin powers** apply only when the selected profile is the primary profile of an admin
  account. A managed profile of the admin account is never admin.
- The **last enabled admin** cannot be deleted, disabled or demoted (`409 conflict`).
- Passwords: 8–128 bytes UTF-8, no composition rules; an admin reset or a self change
  revokes the account's other sessions.
- Deferred (after 0.1.0): invites (US-21), guest (US-24), per-user permissions (US-23),
  single-title sharing (US-22 part), labels in content limits (US-11 part), 2FA, LDAP/OIDC.
  In 0.1.0 the admin creates accounts with an initial password.

### 2 Password and PIN hashing
- argon2id via `golang.org/x/crypto/argon2`, PHC string format
  (`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`), 16-byte salt, 32-byte key. Parameters are
  the OWASP minimum (19 MiB, 2 passes, 1 lane) because of 1 GB NAS devices; the encoded
  parameters allow raising them later: a successful verify with weaker parameters rehashes.
- At most 2 hashes run at once (semaphore), so a burst of sign-ins cannot exhaust memory.
- Unknown account names still run one verify against a fixed dummy hash (no timing oracle);
  the answer never says whether the name exists.
- PIN: exactly 4 ASCII digits (US-02, prototype decision 5), hashed the same way. Brute force
  is stopped by the lockout, not by the hash.

### 3 Sessions (one per signed-in device)
- On sign-in the server creates a session: 32 random bytes (crypto/rand), base64url; the
  database stores only the SHA-256 of the token. Idle timeout 30 days (sliding), absolute
  lifetime 365 days; `last_seen` is written at most every 5 minutes.
- The session row is also the **device** (US-25): device name (from the request, else a short
  label derived from User-Agent), client `web|tv|cli`, platform, last seen, last address class
  `lan|remote`. "Revoke device" deletes the session. Devices and revoke move from M7b to M7
  (owner brief 2026-11-06); the sign-in log (US-26) stays M7b.
- **Transport.** Browsers get an `HttpOnly`, `SameSite=Lax`, `Path=/` cookie named
  `<brand id>_session` (from `internal/platform/brand`), `Secure` on TLS connections. Non-browser
  clients (`client` = `tv|cli`) get the token in the response body and send
  `Authorization: Bearer <token>`; for them no cookie is set. Media URLs (direct file, HLS) are
  same-origin, so the cookie covers them; URL tokens for Cast/DLNA are M8.
- Session lookups are cached in memory for at most 30 s (HLS fetches a segment every few
  seconds); revoke, sign-out, password change and account disable evict the cache entries.
- **CSRF.** Every unsafe request (`POST|PUT|PATCH|DELETE`) authenticated by the cookie must pass
  the ADR 0028 same-origin rule (`Origin` matches or is absent, `Sec-Fetch-Site` is not
  `cross-site`) and send `application/json` when it has a body. Bearer requests are exempt
  (no ambient credential).

### 4 Profile selection, PIN lockout, remember
- A session carries the selected profile or none. After sign-in the server selects the primary
  profile automatically when the account has no managed profiles and the primary has no PIN;
  otherwise the client shows the picker (US-01). Catalog, media and watch requests without a
  selected profile answer `403` with code `profile_required`.
- PIN check: 5 wrong attempts lock the profile for 5 minutes (US-02), counted per profile in
  memory (a restart clears the counter; accepted, the window is short). Locked answers `429`
  with `retryAfterSeconds`.
- "Remember on this device" (US-03): a selection with `remember=false` (default for PIN
  profiles) ends after 4 hours without requests; the client then shows the picker again.
- Switching from a managed profile to the primary profile needs the primary's PIN when it has
  one. The UI asks account owners to set a PIN on their own profile once managed profiles exist.

### 5 Sign-in throttling
- Per account name: after 5 failures, sign-in is refused for 1 minute, doubling per further
  failure up to 15 minutes. Per peer address: 20 failures per 15 minutes. Refusals answer `429`
  with `Retry-After`; in memory, like the PIN counter.

### 6 First-run setup
- While no account exists the server is in **setup state**: `GET /api/v1/setup` says
  `required: true`; `POST /api/v1/setup` (name, password) creates the admin with its primary
  profile in one transaction that only succeeds while the `users` table is empty, and signs in.
- Setup is served to **local peers only, same-origin, JSON only** (the ADR 0028 rule), also with
  `server.lan_only=false`, so a fresh server reachable from the internet cannot be taken over.
- Open without a session: `GET /api/v1/health`, `GET /api/v1/system/info`, the setup pair,
  sign-in, and the static web UI. Everything else answers `401` with code `unauthenticated`
  (`setup_required` while in setup state). There are no default credentials (ADR 0006).
- Recovery: `mivo admin reset-password --name <account>` on the host (reads the new password
  from stdin, works on a stopped or running server's database, revokes the account's sessions).
  Docker: `docker exec`; Synology: SSH. This is the "local CLI recovery path" ADR 0006 asks for.
- Upgrade from an alpha: the catalog stays, the first LAN visitor sees the setup wizard.

### 7 Library access and content limits (enforced on the server, US-12)
- Library access: per account (`all` or a list of library ids, admin always `all`) and per
  managed profile (`all` = the account's set, or a subset). Effective set = account ∩ profile.
  `all` includes libraries added later; a list does not.
- Content limit per profile: `maxAge` ∈ {0, 6, 12, 16, 18} or none, `hideUnrated` (default
  true when a limit is set). Presets (US-11): "Young children" 0, "Children" 6, "Teens" 12,
  "Adults" none, "Custom" = any `maxAge` (labels deferred). A managed profile's effective limit
  is the stricter of its own and its account's primary profile.
- **Ratings** are normalised by a pure domain function from the stored `content_rating` text
  (NFO, TMDB certification) to an age bucket 0/6/12/16/18 or unknown: German FSK and bare ages
  (`FSK 12`, `de/12`, `DE-12`, `12`), US MPAA (G 0, PG 6, PG-13 12, R 16, NC-17 18), US TV
  (TV-Y/TV-G 0, TV-Y7/TV-PG 6, TV-14 12, TV-MA 16), UK BBFC (U 0, PG 6, 12/12A 12, 15 16,
  18/R18 18); anything else is unknown. Ages between buckets round **up** (13 → 16).
  Amendment: X/XXX → 18 (orchestrator 2026-11-06); a text with non-ASCII characters other
  than spaces and punctuation (`1８`, `1⁸`, `Ⅻ`, combining marks) is unknown, so it fails
  closed instead of losing characters.
- The bucket is stored per item (`age_rating`, migration `00013`) together with the effective
  bucket that children inherit from their show (an episode without its own rating uses the
  show's), so one indexed predicate filters every query.
- The restriction (library set, max age, hide unrated) is applied in SQL for every listing,
  facet, letter, folder, row, search, children, summary and detail read, and checked before a
  playback session starts. A hidden item answers `404 not_found`, never `403`, so its existence
  does not leak. Images stay readable for any signed-in profile by opaque id (accepted: ids are
  not guessable and carry no title).
- The **viewer** (account, profile, admin flag, effective access, limit) is resolved by the HTTP
  adapter and passed to the use cases in the `context.Context` through `internal/app/access`
  (`WithViewer`, `ViewerFrom`). Use cases that list or read catalog data **fail closed**: no
  viewer → `domain.ErrUnauthenticated`. Background jobs (scan, enrichment) never use the
  viewer-scoped reads.

### 8 Watch state
- Table `watch_state` (migration `00012`): `profile_id` (FK, `ON DELETE CASCADE`), `item_id`
  (**no foreign key**, so a rescan, a folder change or a library removal never deletes it),
  `position_seconds`, `watched`, `play_count`, `last_played_at`, `hidden_from_continue`,
  `attach_key` (the item's provider reference at write time, e.g. `tmdb:movie:603`, for a later
  re-attach step), `updated_at`. Orphaned rows are ignored by joins. The re-attach job after a
  folder rename or library re-add is deferred (after 0.1.0); the data needed for it is kept.
- Rules (pure domain): a position counts as "in progress" when it is at least
  max(30 s, 1 % of the duration) and below 90 % (AD-23 default); at or above 90 % the item
  becomes watched, the position resets to 0 and `play_count` increments. The duration comes
  from the catalog, never from the client. Unknown duration: the position is stored, nothing
  becomes watched automatically.
- Kinds with watch state in 0.1.0: movie, episode, audiobook. Marking a show or season
  (un)watched applies to its episodes. Music listen state (HO-06) is deferred.
- **Continue watching** (HO-03, LB-01): in-progress items, not hidden, played within 12 weeks,
  newest first, at most 40; per show only its most recently played episode.
- **Up next** (HO-04): per show with a watched episode in the last 12 weeks and no in-progress
  episode, the first unwatched episode after the last watched one in season/episode order,
  specials (season 0) skipped; newest show activity first, at most 20.
- Clients report progress with `PUT /api/v1/items/{id}/progress` every 10 s while playing and
  on pause, stop and page hide (`fetch` with `keepalive`).

### 9 Administration after accounts (refines ADR 0028)
- Every management and administration request (libraries write, scan, folder browser,
  identify/refresh/match, metadata settings, users, profiles of other accounts, all devices)
  requires an **admin viewer** (`403 forbidden` otherwise) **and** keeps the ADR 0028 local-peer
  rule. Admin from remote networks waits for M7b (secure connections) and a new decision.
- ADR 0028 §7's residual risks shrink to: a reverse proxy in the LAN makes every client look
  local (admin still needs to sign in); DNS rebinding cannot reach admin endpoints without the
  `SameSite=Lax` session cookie.

### 10 Migrations
- `00011` accounts, profiles, library access, sessions/devices; `00012` watch state; `00013`
  age ratings. They merge in numeric order (goose refuses gaps on existing databases).

## Consequences

- Every API client needs a session. The Playwright harness, `scripts/playback-smoke` and the
  release gate sign in (setup on a fresh data directory); `docs/development.md` and the Docker
  and Synology guides explain first-run setup and the recovery command.
- A new production dependency `golang.org/x/crypto` (argon2).
- `domain.ErrUnauthenticated` joins the domain errors; `adapters/http` maps it to `401`.
- `app/rows.AllLibraries` is replaced by a viewer-scoped library source; the plug-in point
  already exists.
- Lockout and throttle counters are lost on restart (accepted). Sessions survive restarts.
- Upgrades keep the catalog; nothing user-owned existed before, so no data migration.
- Deferred to after 0.1.0, each with a reason in `docs/tasks/M7-00-overview.md`: Quick Connect
  for TV, invites, API keys, 2FA, LDAP/OIDC, Plex/Jellyfin import, playlists, collections,
  backup/restore, webhooks, scheduler, activity log, live logs, guest, permissions, labels,
  watch-state re-attach, music listen state.
