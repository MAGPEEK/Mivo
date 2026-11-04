# M3-17 test report: test corpus library layouts

Result: PASS (round 1)

## Gates

| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race -coverprofile=coverage.out ./...` | all packages ok, no FAIL |
| `go-test-coverage --config=.testcoverage.yml` | package threshold PASS, total threshold (70 %) PASS, total 96.0 % |
| `internal/domain/...` coverage | 100 % in every package with statements (threshold 90 %) |
| `scripts/gen-testmedia` coverage | 98.2 % |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` | no diff |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | no diff |
| Layer rule (`go list -deps ./internal/domain/...` for database/sql, net/http, os/exec, adapters, app) | no hits |
| `go test -count=1 -v ./internal/adapters/corpustest/` (real FFmpeg 9.0.2) | PASS, nothing skipped |
| gen-testmedia integration tests with `-race` (`TestIntegrationGeneratesTheLibraryTree` and the others) | PASS (library tree 1.4 s) |

Not run: web gates, docker build (the task does not touch them).

## Generator runs

- Fresh (`testdata/library` removed): 20 generated, 0 failed, 1.1 s. Summary lists the library tree.
- Repeat: 20 kept, 0 generated.
- `-force`: 45 media cases and 20 library cases regenerated, 0 failed, 2.2 s.
- `-library ""`: no library output.
- `testdata/media` summary unchanged: 45 cases, 9 known gaps (pre-existing).

## Acceptance criteria

1. Library tree, flags and manifest: MET. `-library` default `testdata/library`, `""` skips. `manifest.json` has schemaVersion 1, `libraries` (movies and shows) and `cases`. The schema is documented in the `main.go` package comment (line 23ff). The `testdata/media` manifest is unchanged.
2. Movies: MET. On disk: Big Buck Bunny with `- 1080p` and `- 4K`, `Trailers/`, `-featurette`, `sample.mkv`, `poster.jpg`, `fanart.jpg`, `clearlogo.png` (PNG 800x310 8-bit RGBA). Blade Runner `{edition-Director's Cut}`. Kill Bill `cd1`/`cd2`. "Blender Open Movies" folder with loose movies. Cosmos Laundromat `movie.nfo` title "Cosmos Laundromat: First Cycle". Spring URL-only NFO. Das Boot Latin-1 NFO. Caminandes truncated (malformed) NFO. Glass Half MKV: stream 2 is mjpeg `attached_pic=1`, `filename=cover.jpg`, `mimetype=image/jpeg`. Night of the Living Dead MP4: stream 2 mjpeg `attached_pic=1`, no local poster.
   - NFD folder: `ls | od -c` shows `A m e 314 201 l i e`, which is e + U+0301.
   - Latin-1 NFO: header says ISO-8859-1, `file` reports "ISO-8859 text", bytes `366 337` (ö ß).
   - Posters are 600x900 and fanart 1280x720 (`file`).
3. Shows: MET. Pioneer One has `Season 01`, `Season 02`, `Specials`, `S01E01-E02` (manifest number 1, numberEnd 2), `tvshow.nfo` with tmdb/imdb/tvdb uniqueid, `season01-poster.jpg`, episode NFO and `-thumb.jpg`. Star Wreck has its files in the library root.
   - DT-24: `shows/Dragon Ball Super/` has exactly 131 files, `- 001.mkv` to `- 131.mkv`. The manifest has one show, one season, 131 episodes numbered 1..131 with no gaps.
   - The second folder `Dragon Ball Super Fansub/` holds `[Group] Dragon Ball Super - 130 [1080p] [ABCD1234].mkv`.
4. Copies: MET. The 131 episodes are byte-identical copies (`cmp` equal) of one built file. 20 FFmpeg runs for 157 videos. Covered by `TestLibraryCopiesShareOneFFmpegRunPerItem` and `TestGenerateWritesCopiesAndSidecarsInOtherFolders`.
5. Images and text: MET. Posters, backdrops and logo come from FFmpeg. NFO files come from the generator. Nothing is downloaded.
6. Idempotence, `-force`, summary: MET (see Generator runs above).
7. Tests: MET.
   - Unit tests for the case table: `TestLibraryCasesHaveUniqueNamesAndPaths`, `TestLibraryCatalogIsBackedByCases`, `TestLibraryCopiesShareOneFFmpegRunPerItem`.
   - Fake-FFmpeg tree run: `TestGenerateLibraryWritesTheTreeWithFakeFFmpeg`.
   - Real-FFmpeg integration test: `TestIntegrationGeneratesTheLibraryTree` (skips without FFmpeg).
8. Gates: MET (table above).

## Manifest against disk

Script walked every `file`, `parts`, `files` and `ignored` path in `manifest.json` (171 paths). All 171 exist. The only file on disk that the manifest does not reference is `manifest.json` itself. Seasons and episodes: Pioneer One has season 0 [1 ep], 1 [2 eps], 2 [1 ep]. Star Wreck has season 1 [2 eps]. Dragon Ball Super has season 1 [131 eps].

## .gitignore

`.gitignore:23: /testdata/library/` matches `testdata/library/manifest.json` (`git check-ignore -v`). `git status` shows no `testdata` entries. `coverage.out` is ignored.

## Findings

None blocking. `docs/tasks/BACKLOG.md` is untouched, which the task allows ("only if picked up").

## History

- Round 1: PASS.

REPORT
status: PASS
task: M3-17
files: docs/reports/M3-17-test.md
tests: go vet, go test -race with coverage (total 96.0 %, domain 100 %, gen-testmedia 98.2 %), go-test-coverage, golangci-lint run and fmt --diff, go mod tidy -diff, gen-testmedia fresh, repeat, -force and -library "", gen-testmedia integration tests and corpustest with real FFmpeg 9.0.2: all green
findings: none
next: send to mivo-reviewer
