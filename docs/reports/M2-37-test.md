# M2-37 test report

Result: PASS (round 1). Worktree uncommitted against base 1895eae.

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` / `go vet -tags webui ./...` | clean / clean |
| `go test -coverprofile=coverage.out ./...` | all packages ok; `http/stream` 97.5 %, `internal/domain/...` 100 % |
| go-test-coverage (`.testcoverage.yml`) | PASS, total 96.1 % (5907/6146), threshold 70 % |
| `golangci-lint run` / `golangci-lint fmt --diff` | 0 issues / no diff |
| `go mod tidy -diff` (root and tools) | no diff |
| Docker `go test -race ./...` (golang:1.27.3, `MSYS_NO_PATHCONV=1`) | all ok |
| Corpus test with real FFmpeg (`TestCorpusRemuxIsServedOverHTTP`, unchanged file, also locally with `-race`) | PASS, not skipped |
| `pnpm -C web e2e` | 31 passed (chromium + branded) |
| Layer rule | archtest ok; domain imports unchanged (no domain files touched) |

## Live check (server built with `-tags webui`, corpus `testdata/media/video`, remux of h264_aac_long.mkv)
- GET `index.m3u8`: 200, `Content-Length: 356`, line 1 `#EXTM3U`, line 2 `#EXT-X-START:TIME-OFFSET=0`, ends with `#EXT-X-ENDLIST`.
- File on disk: 329 bytes, 0 occurrences of `EXT-X-START` (329 + 27 inserted bytes = 356).
- HEAD: 200, `Content-Length: 356`, same as GET.
- `Range: bytes=0-6`: 206, `Content-Range: bytes 0-6/356`, body `#EXTM3U`; `bytes=9999-`: 416.
- Headers: `application/vnd.apple.mpegurl`, `Cache-Control: no-store`, no `Last-Modified`.
- `seg-00000.m4s`: `Cache-Control: private, max-age=3600`, `Last-Modified` present.

## Acceptance criteria
1. Bounded in-memory read via OpenFile, `maxPlaylistBytes` = 4 MiB, closed before first write: met (`playlist.go:17`; `TestPlaylistIsClosedBeforeTheResponse`, `TestMaxPlaylistBytesIsFourMebibytes`).
2. Over-bound playlist -> 500 internal, one error log with session ID: met (`TestPlaylistOverTheBoundAnswersInternalError` asserts one `level=ERROR` and `session=<id>`; `TestPlaylistAtTheBoundIsServed`; `TestReadBounded`).
3. Start tag second line exactly once, pure rewrite table test (LF, CRLF, `#EXTM3U` only, existing tag, no `#EXTM3U`, empty): met (`TestWithStartAtZero`; live check).
4. Range / If-Range / HEAD / 416, headers: met (`TestPlaylistRangesWorkOnTheServedBody`; live check).
5. Directory 404, missing 404, open error 500: met (`TestPlaylistMissingOrDirectoryAnswersNotFound`, existing tests green).
6. `init.mp4`/segments unchanged: met (existing tests green; live headers).
7. File on disk never modified: met (`TestPlaylistOnDiskStaysUnchanged`; live: no start tag on disk).
8. Tests incl. unchanged `corpus_test.go` and e2e: met (see gates; `git diff` shows no change to `corpus_test.go`, `adapters/ffmpeg`, `docs/adr`).
9. vet / lint / fmt / coverage gate: met.

## BACKLOG
`git diff docs/tasks/BACKLOG.md`: exactly two rows removed (`http/stream` in-memory playlist, `ffmpeg/http stream` native HLS start at 0). No other row touched.

## Notes (non-blocking)
- Coverage of new code: `loadPlaylist` 90.0 %, `serveContent` 75.0 %, `writeProblem` 66.7 % (the latter two pre-existing error branches); package 97.5 %.
- `coverage.out` is untracked/ignored; no stray files in the worktree besides the report.

## History
- Round 1: PASS.
