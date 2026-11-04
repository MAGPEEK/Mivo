# Milestone 6 "Online-Metadaten" – overview

Scope of this breakdown: TMDB (movies, shows, seasons, episodes), MusicBrainz + Cover Art
Archive (albums, artists), provider artwork cache, background enrichment merged through
`metadata.Merge` with field locks, and the user-visible **Identify / "Zuordnung korrigieren"**
flow ("Remove match", "Refresh metadata", filter "not matched"), provider settings with the
TMDB attribution. Decisions: [ADR 0031](../adr/0031-online-metadata-providers.md) on top of
ADR 0010, 0027 §6-§8, 0028.

Not in this breakdown (follow-up tasks M6-2x, to be cut after this lane is green): edit dialog
with field locks, tags and image picker/upload/URL (ED-01…07), rating row (DT-04), portraits and
person pages (DT-08, DT-30), "Similar" (DT-10), rows "With <person>" / "Top rated", ordered
provider list in Settings (AD-22), episode ordering absolute/DVD, OpenSubtitles (PL-20). The
model already carries what they need (locks, provider artwork sources, `user` source).

Hard rules for every task: no network in tests (recorded fixtures + `httptest`), no file paths
or user data in outgoing requests, offline must never fail a scan or a page, every outgoing
host is in ADR 0031 §1.

## Tasks

| ID | Title | Owner | Depends on |
|---|---|---|---|
| M6-01 | API contract: identify, refresh, provider settings | mivo-backend | M3-02 |
| M6-02 | Domain: matching, confidence, match state | mivo-backend | M3-04b |
| M6-03 | SQLite: migration 00008 (00004 is library management; 00005 music, 00006 audiobooks, 00007 photos are reserved; take the next free number at merge), match state, cache, queue | mivo-backend | M3-05d, M6-02 |
| M6-04 | Outbound client: SSRF guard, limiter, backoff, breaker | mivo-backend | none |
| M6-05 | Config: online switches, keys, build-time key | mivo-backend | none |
| M6-06 | App: provider ports and enrichment of one item | mivo-backend | M6-02, M3-06 |
| M6-07 | TMDB adapter: search and details (movie, show, season, episode) | mivo-media | M6-02, M6-04, M6-06 |
| M6-08 | Artwork fetcher: provider images into the data directory | mivo-media | M6-04, M6-06, M3-09 |
| M6-09 | MusicBrainz adapter | mivo-media | M6-04, M6-06, M3-13a |
| M6-10 | Cover Art Archive adapter | mivo-media | M6-04, M6-06, M6-08 |
| M6-11 | App: enrichment queue, cache, offline behaviour | mivo-backend | M6-03, M6-06 |
| M6-12 | App: identify, unmatch, refresh use cases | mivo-backend | M6-06, M6-11 |
| M6-13 | HTTP handlers for identify, refresh, provider settings | mivo-backend | M6-01, M6-12 |
| M6-14 | Wiring in `cmd/mivo`, scan trigger, periodic refresh | mivo-backend | M6-05, M6-07…M6-13 |
| M6-15 | Web: Identify dialog, Remove match, Refresh, "not matched" | mivo-frontend | M6-01 |
| M6-16 | Web: Settings › Metadata with attribution; privacy doc | mivo-frontend | M6-01 |
| M6-17 | E2E: fake provider, offline, identify flow | mivo-frontend | M6-14, M6-15, M6-16 |

M6-15 and M6-16 use the generated client and an MSW-style stub, so they start right after M6-01.

## Parallel lanes

- A (contract, first): M6-01.
- B (pure/app): M6-02 → M6-06 → M6-11 → M6-12 → M6-13.
- C (infra, parallel to B): M6-04, M6-05, M6-03 (after M6-02).
- D (adapters, after M6-04 + M6-06): M6-07, M6-08, M6-09, M6-10 (disjoint packages).
- E (web, after M6-01): M6-15, M6-16.
- F (integration, last): M6-14, M6-17.
