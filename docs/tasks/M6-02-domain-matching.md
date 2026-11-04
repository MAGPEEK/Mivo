# M6-02 Domain: matching, confidence, match state
Owner: mivo-backend
Depends on: M3-04b
Files/areas: `internal/domain/metadata/**` (extend), new `internal/domain/metadata/match`

## Goal
Pure rules that decide whether an online candidate is the right title, so auto-match is
predictable and testable without a network (ADR 0031 §9).

## Acceptance criteria
- [ ] `metadata.Source` gains `tmdb`, `musicbrainz`, `user` (documented order user, nfo,
      provider, filename); existing `Merge` tests extended with a `tmdb` source between `nfo`
      and `filename` and with a locked field.
- [ ] `match.MatchState` `matched|unmatched|disabled` with `ParseMatchState`.
- [ ] `match.NormalizeTitle` (case-fold, NFC, strip punctuation, leading article, `&`/`and`,
      accents kept) table-tested incl. German and CJK titles.
- [ ] `match.Score(query Query, candidate Candidate) float64` in [0,1]: exact normalised title
      + same year ≥ 0.95; year off by 1 reduces, off by >1 strongly; popularity only breaks ties.
- [ ] `match.Decide(scored []Scored) (best *Scored, auto bool)`: auto only when best ≥ 0.85
      and lead over second ≥ 0.10; an exact provider ID query always yields confidence 1.
- [ ] `match.ParseProviderRef(text)` extracts provider + ID from a pasted ID or URL (TMDB movie/
      tv URL, IMDb `tt…` URL, MusicBrainz release/artist UUID URL); rejects others with
      `domain.ErrInvalid`.
- [ ] Tests: table-driven, no mocks, `go test ./internal/domain/...`, coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No I/O, no clock. Thresholds are named constants. Do not import provider vocabularies; the
`Candidate` type is provider-neutral.
