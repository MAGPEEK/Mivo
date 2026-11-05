# 0031 Online metadata providers: TMDB, MusicBrainz, Cover Art Archive (milestone 6)

- Status: Accepted (orchestrator decision 2026-11-04, owner delegated; online metadata default on with one-switch opt-out)
- Date: 2026-11-04
- Builds on: ADR 0010 (outbound connections), ADR 0027 §6-§8 (merge, locks, artwork), ADR 0028

## Context

Milestone 3 reads names, NFO files and local artwork. Milestone 6 adds online metadata. The
rules of ADR 0010 stay: no telemetry, the daily update check is the only unsolicited
connection, metadata lookups are a documented, switchable feature. Mivo must keep working
offline, must not leak the user's files, and CI must never touch the network.

## Decision

1. **Hosts (complete list).** `api.themoviedb.org`, `image.tmdb.org`, `musicbrainz.org`,
   `coverartarchive.org` (redirects to `*.archive.org` are followed for images only). Nothing
   else is contacted by this feature. Documented in `docs/privacy.md` (M6-16).
2. **Switches.** Global `metadata.online.enabled` (config + Settings) and per library
   `online_metadata` (both default **on** for new installs so the first scan yields posters;
   first-run text in Settings names the hosts). Off means no connection at all. Each provider
   can be switched off separately.
3. **Keys.** The project TMDB key is injected at build time (`-ldflags`, variable in
   `internal/platform/config`, empty in dev builds, which disables TMDB unless an own key is
   set). An own key in Settings or `metadata.tmdb.api_key` takes precedence. The bundled key is
   extractable by design; this is acceptable for an open-source client under the TMDB terms,
   and the own-key option exists for that reason. Keys are never logged and never returned by
   the API (only "set / not set / bundled").
4. **Attribution.** "Powered by TMDB" with the TMDB logo (bundled asset, ADR 0010: no third-party
   assets at runtime) and the TMDB disclaimer ("This product uses the TMDB API but is not
   endorsed or certified by TMDB") in Settings › Metadata, the About page and the detail page
   footer of any item whose data came from TMDB. MusicBrainz data is CC0; Cover Art Archive
   images carry their own licences, shown as "Cover Art Archive" credit in the same places.
5. **Privacy.** A request contains only: a normalised title, year, season/episode numbers,
   metadata language, provider IDs and the key. Never file names, paths, library names, file
   hashes, sizes, user names or IP-revealing headers beyond what TCP implies. User-Agent is
   `Mivo/<version> (+<project URL>)` (MusicBrainz requires a contact URL). No cookies.
6. **Ports (consumer side, `internal/app/...`).** Small interfaces such as `MovieLookup`,
   `ShowLookup`, `MusicLookup`, `ArtworkSource`; adapters live in `internal/adapters/metadata/
   {tmdb,musicbrainz,coverart}`; they return domain types and map HTTP errors to
   `ErrUnavailable` / `ErrRateLimited` / `ErrNotFound`. Provider JSON never leaves the adapter.
7. **Outbound client.** One shared constructor in `internal/adapters/netaccess/outbound`: HTTPS
   only (test servers excepted via an explicit option), timeouts, response size cap, redirect
   limit, SSRF guard (resolved IPs must be public; applied to user-supplied URLs and to image
   downloads), per-host token-bucket limiter (TMDB 20 req/s burst 20; MusicBrainz and Cover Art
   Archive 1 req/s), retry with exponential backoff and jitter on 429/5xx honouring
   `Retry-After`, a circuit breaker per provider (open for 5 min after repeated failures).
8. **Local first, offline safe.** Merge order, highest first: `user` (locks always win), `nfo`,
   `tmdb`/`musicbrainz`, `filename`. A scan never waits for or fails because of the network:
   enrichment is a separate background queue (persisted, resumable) triggered after a scan, by
   "Refresh metadata" and by the periodic task. Provider responses are cached in the database
   (TTL 7 days metadata, 30 days "not found"); stale cache is used when offline. A TMDB or
   IMDb ID in an NFO or a `{tmdb-123}` style tag is trusted and skips searching.
9. **Matching.** Domain function scores candidates (normalised title equality, year distance,
   popularity as tie-break only). Auto-match only when confidence ≥ 0.85 and the best is clearly
   ahead of the second; otherwise the item stays `unmatched` and appears under the filter "not
   matched". States: `matched`, `unmatched`, `disabled` ("Remove match" sets `disabled`: online
   data is dropped, provider artwork deleted, no auto-match again until the user identifies it).
10. **Identify (UX "Zuordnung korrigieren").** Admin-only action (management guard of ADR 0028
    until M7): search by title/year or by pasted provider ID/URL, pick a candidate, apply. Apply
    stores the provider IDs, locks `provider_ids`, re-runs enrichment for that item (and for a
    show its seasons and episodes), keeps all other locks.
11. **Artwork.** Provider images are downloaded once into `<data>/artwork/<tag>.<ext>` as
    `artwork.source = provider` rows (ADR 0027 §5) and then served by the existing image endpoint.
    Allowed hosts and content types are checked before saving (image/jpeg|png|webp, ≤ 15 MiB).
    Selection order stays ADR 0027 §8 (local, embedded, then provider) unless the user locked or
    picked a kind.
12. **Tests.** Adapters are tested against recorded JSON/image fixtures served by
    `httptest.Server` (`testdata/providers/<provider>/`). No test reaches the network; a
    guard test fails if `http.DefaultTransport` dials a non-loopback address in CI.
    Fixtures are recorded by a dev-only script outside CI (`scripts/record-provider-fixtures`).

## Consequences

- The bundled key can be rate-limited by TMDB if the install base grows; own keys are the
  escape hatch, and `ErrRateLimited` is surfaced in the UI as "TMDB is busy, will retry".
- Default-on online lookup is a deliberate trade-off against the "works out of the box" goal;
  the opt-out is one switch and the host list is documented.
- A new table set (migration `00005`: match state, provider cache, enrichment queue) is needed.
- Episode ordering (absolute/DVD) and OpenSubtitles are not part of this ADR (M6 follow-ups).
