# M6-10 Cover Art Archive adapter
Owner: mivo-media
Depends on: M6-04, M6-06, M6-08
Files/areas: `internal/adapters/metadata/coverart/**`, `testdata/providers/coverart/**`

## Goal
Implement `ArtworkSource` for albums: list front/back cover candidates by release (group)
MBID and hand them to the artwork fetcher; local covers keep priority (ADR 0027 §8).

## Acceptance criteria
- [ ] `GET /release-group/{mbid}` (fallback `/release/{mbid}`) parsed to candidates with kind
      `poster` (front, `square` once M3-13a defines it) and thumbnail sizes 500/1200.
- [ ] 404 → no candidates without error; 503/429 → `ErrRateLimited`; 1 request/s shared
      limiter with MusicBrainz is not required, own host limiter applies.
- [ ] Only `coverartarchive.org` and its `*.archive.org` redirects are used.
- [ ] Tests: `httptest` + fixtures (front only, front+back, 404, redirect JSON); no network.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Images inherit licences of the uploader; the UI credit lives in M6-16.
