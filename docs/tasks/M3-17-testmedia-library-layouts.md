# M3-17 Test corpus: library layouts, NFO files and artwork
Owner: mivo-media
Depends on: M3-01
Files/areas: `scripts/gen-testmedia/**` (new `cases_library.go`, `library.go`,
`library_manifest.go` + tests; flag handling in `main.go`), `.gitignore` (one line:
`testdata/library/`), `docs/development.md` (the corpus section), `docs/tasks/BACKLOG.md`
(only the `testmedia` row if picked up)

## Goal
A generated, rights-free library tree lets integration and E2E tests prove the milestone-3
behaviour on real files: folder and loose movies, versions, parts, extras, shows with seasons,
specials and multi-episode files, the absolute-numbered "Dragon Ball Super" show, NFO files of
every kind and local and embedded artwork. ADR 0021 rules apply (lavfi sources, generated
text, skip what the local FFmpeg cannot build).

## Acceptance criteria
- [ ] `go run ./scripts/gen-testmedia` additionally writes `testdata/library/` (flag
      `-library <dir>`, default `testdata/library`; `-library ""` skips it) with
      `movies/` and `shows/` trees and `testdata/library/manifest.json` describing the
      **expected catalog**: per library the top-level items with kind, title, year, version
      labels, extras, NFO-provided fields, artwork kinds present, and for shows the seasons
      with episode numbers. `testdata/media` and its manifest stay unchanged.
- [ ] Movies: a folder movie with two versions (`- 1080p`, `- 4K` label via file name), an
      edition (`{edition-Director's Cut}`), a two-part movie (`cd1`/`cd2`), loose movies in
      one folder, a movie whose `movie.nfo` title differs from the folder name, a URL-only
      NFO, a Latin-1 NFO, a malformed NFO, `Trailers/` and a `-featurette` suffix extra, a
      `sample` file, an NFD-named folder (`Amélie (2001)`), `poster.jpg`,
      `fanart.jpg`, `clearlogo.png` (alpha), and one MKV with a cover attachment and one MP4
      with `covr` but no local poster.
- [ ] Shows: a show with `Season 01`, `Season 02`, `Specials`, a multi-episode file
      (`S01E01-E02`), `tvshow.nfo` with `uniqueid` values, `season01-poster.jpg`, an episode
      NFO and `-thumb.jpg`; a show with files directly in the root; **`Dragon Ball Super/`
      with 131 files `Dragon Ball Super - 001.mkv` … `- 131.mkv`** plus one anime-style name
      (`[Group] … - 130 [1080p] [ABCD1234].mkv` in a second show folder to keep keys apart).
- [ ] Bulk names reuse one encoded clip: a case may list `Copies` (extra output paths) that
      the generator fills by copying the built file, so the 131 episodes cost one FFmpeg run.
- [ ] Images are generated with FFmpeg (`testsrc2`/`color` single frame, 600×900 posters,
      1280×720 backdrops, RGBA PNG logo); NFO and other text files are written by the
      generator; nothing is downloaded.
- [ ] Idempotent like the corpus (`-force` rebuilds); the summary lists the library tree.
- [ ] Tests: unit tests for the case table (paths unique, every manifest entry backed by a
      case, copies counted), a fake-FFmpeg run producing the tree layout, and the existing
      integration test extended to generate the library into a temp dir with real FFmpeg
      (skipped without FFmpeg).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./scripts/gen-testmedia/...`, coverage gate.

## Notes
- Keep the library tree out of `testdata/media`: the playback corpus test, the smoke script
  and the M2 E2E suite use that folder as one movies library, and 140 more files would change
  their counts and slow them down.
- Clips: the shortest valid H.264/AAC MKV of the existing cases (2 s, 320×240) is enough;
  the library tests check placement and metadata, not playback.
- The manifest is the contract for M3-08 (artwork corpus tests), M3-11b (integration test)
  and M3-18 (E2E); document its schema in the package doc comment.
- Real show and movie names are used only as file and folder names; the content is generated.
