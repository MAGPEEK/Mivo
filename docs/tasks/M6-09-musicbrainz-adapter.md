# M6-09 MusicBrainz adapter
Owner: mivo-media
Depends on: M6-04, M6-06, M3-13a
Files/areas: `internal/adapters/metadata/musicbrainz/**`, `testdata/providers/musicbrainz/**`

## Goal
Implement `MusicLookup`: match albums (release groups) and artists, return title, artist
credits, year, genres/tags, MusicBrainz IDs.

## Acceptance criteria
- [ ] Search by album + artist (+ year) and lookup by MBID (`release-group`, `release`,
      `artist`); local embedded MBIDs from tags skip the search.
- [ ] Per ADR 0031 §7: 1 request/s, User-Agent with contact URL, JSON (`fmt=json`), 503 with
      `Retry-After` → `ErrRateLimited`; no key.
- [ ] Mapping respects `metadata` record fields and the music kinds of M3-13a; unknown
      fields ignored.
- [ ] Requests carry only tag-derived names, year and MBIDs (no paths, no file names).
- [ ] Tests: `httptest` + recorded fixtures (album found, ambiguous, none, rate-limited);
      limiter verified with the injected clock; no network.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
MusicBrainz data is CC0; credit it in M6-16. Audio fingerprinting (AcoustID) is out of scope.
