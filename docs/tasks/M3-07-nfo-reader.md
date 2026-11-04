# M3-07 Metadata adapter: Kodi NFO parser
Owner: mivo-backend
Depends on: M3-04b
Files/areas: `internal/adapters/metadata/nfo/**` (new package), `testdata` fixtures inside the
package (`internal/adapters/metadata/nfo/testdata/*.nfo`), `go.mod`/`go.sum` (only the
promotion of `golang.org/x/text` to a direct requirement)

## Goal
Local Kodi NFO files fill titles, summaries, genres, people, release dates, content ratings,
collections and provider IDs of movies, shows, seasons and episodes, offline and without any
network access. UX: DT-06, DT-08 (names), LB-03/LB-04 data (release date, genres), ED-07
data (provider IDs); ADR 0027 §7.

## Acceptance criteria
- [ ] `nfo.Parse(data []byte, kind nfo.Kind) (nfo.Result, error)` with kinds `movie`,
      `show`, `season`, `episode`; `Result{Records []metadata.Record, Warnings []string}` –
      one record, or one per `<episodedetails>` root for multi-episode files. Warnings are
      short codes without file content (for example `invalid_provider_id`). M3-10 declares
      the scan's NFO port with exactly this shape.
- [ ] Reads every element of ADR 0027 §7; `plot` falls back to `outline`; `year` from
      `premiered`/`releasedate`/`aired` when missing; `uniqueid` with `default="true"` wins
      over other IDs of the same provider; provider IDs validated with
      `metadata.ValidateProviderID` (invalid ones are dropped with a warning, not an error).
- [ ] URL NFO and XML followed by a URL line yield provider IDs from
      `themoviedb.org/(movie|tv)/<id>`, `imdb.com/title/tt…`, `thetvdb.com/…?id=` and
      `thetvdb.com/series/<id>` forms.
- [ ] Encoding: UTF-8 (with or without BOM) and ISO-8859-1/Windows-1252 declarations; other
      encodings → `domain.ErrUnsupported`. Text is NFC-normalised (`golang.org/x/text/unicode/
      norm`) and trimmed; HTML entities decoded by `encoding/xml`; inner whitespace kept.
- [ ] Limits: input > 1 MiB → `domain.ErrUnsupported` (named constant); element nesting
      depth and element count bounded; malformed XML → `domain.ErrUnsupported` with a short
      reason (no file content in the error).
- [ ] Nothing is fetched: URLs in `thumb`, `fanart` and actor `thumb` are ignored (test that
      the parser has no network or file access: it takes bytes only).
- [ ] Fixture tests (≥ 20 files): Kodi movie/tvshow/season/episode examples, Jellyfin and
      Plex-exporter variants, multi-episode, URL-only, mixed XML+URL, Latin-1, BOM, NFD text,
      malformed, oversized, unknown elements, empty file. Coverage ≥ the adapter threshold.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/metadata/...`, coverage gate, `go mod tidy -diff`.

## Notes
- `encoding/xml` does not resolve external entities; still keep the size limit before
  decoding and use a `CharsetReader` only for the two Latin encodings.
- M3-07b promotes `golang.org/x/text` in `go.mod` as well; the change is identical, so the
  second task to merge rebases without conflict. No other `go.mod` change in this wave.
- Mapping `mpaa` values like `Rated PG-13` keeps the raw text in M3 (ADR 0027 §5).
