# M2-37 HLS playlist served from memory, starting at 0
Owner: mivo-media
Depends on: M2-29, M2-32 (committed)
Files/areas: `internal/adapters/http/stream/hls.go`, `respond.go`, new `playlist.go` +
`playlist_test.go`, `hls_test.go`, `handlers_test.go`, `helpers_test.go`, `export_test.go`;
`docs/tasks/BACKLOG.md` (only the rows below). **Not** `corpus_test.go` (M2-37 and M2-39 run
in parallel; M2-39 edits that file).

## Goal
A stalled or slow client can no longer hold `index.m3u8` open while it downloads, so FFmpeg's
rename over the playlist never fails on Windows because of a playlist request, and the final
playlist always gets its `#EXT-X-ENDLIST`. Native HLS players (Safari, webOS) start a remux or
transcode at the beginning instead of near the newest segment.

Closes BACKLOG rows (delete them from `docs/tasks/BACKLOG.md` when done, touch no other row):
- `http/stream` | "serve the HLS playlist from memory (bounded read, close the file before
  writing) …" (M2-24b review)
- `ffmpeg/http stream` | "native HLS players (Safari, webOS) may treat the growing EVENT
  playlist as live …" (M2-27 review)

The decisions are already recorded in ADR 0024 "Notes (M2-37 to M2-41)"; do not edit the ADR.
If the implementation has to deviate, say so in the REPORT.

## Acceptance criteria
- [ ] A playlist request reads `index.m3u8` through `SessionFiles.OpenFile` into memory with a
      bound of 4 MiB (named constant `maxPlaylistBytes`) and closes the file before the first
      byte of the response is written. Test: a response writer (or `SessionFiles` fake) that, on
      the first `Write`, asserts the opened playlist file is already closed (e.g. `Stat` on the
      recorded `*os.File` fails with `os.ErrClosed`).
- [ ] A playlist larger than `maxPlaylistBytes` answers 500 (`internal` problem) and logs one
      error record with the session ID; the read never allocates more than the bound plus one
      byte. Test with a playlist of `maxPlaylistBytes + 1` bytes (the constant may be shortened
      through `export_test.go`).
- [ ] The served playlist has `#EXT-X-START:TIME-OFFSET=0` as its second line, directly after
      the leading `#EXTM3U` line, exactly once; every other byte is unchanged. Table test of the
      pure rewrite function: typical FFmpeg playlist (LF), CRLF line ends, `#EXTM3U` only
      (no trailing newline), playlist that already has an `#EXT-X-START` tag (unchanged),
      playlist not starting with `#EXTM3U` (unchanged), empty file (unchanged).
- [ ] Range, `If-Range` and `HEAD` work on the rewritten body: `HEAD` answers the same
      `Content-Length` as `GET`; `Range: bytes=0-6` answers 206 with `#EXTM3U`; a range beyond
      the body answers 416. Content type `application/vnd.apple.mpegurl` and
      `Cache-Control: no-store` stay; no `Last-Modified`.
- [ ] A directory named `index.m3u8` still answers 404; a missing playlist 404; an open error 500
      (existing tests stay green, adapted only where the body now contains the start tag).
- [ ] `init.mp4` and segments are served from their files exactly as before (`Last-Modified`,
      `private, max-age=3600`, sendfile path through `serveContent`).
- [ ] The file on disk is never modified (assert the bytes after a request).
- [ ] Tests: `go test -race ./internal/adapters/http/stream/...` (or the Docker `-race` line
      from `CLAUDE.md`), including the unchanged `corpus_test.go` with real FFmpeg;
      `pnpm -C web e2e` passes (hls.js gets the start tag and still starts at 0).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, coverage gate
      (`go-test-coverage`) pass.

## Notes
- Keep the decision small: open → `io.ReadAll(io.LimitReader(file, maxPlaylistBytes+1))` →
  `Stat`/regular-file check → close → rewrite → `http.ServeContent(…, bytes.NewReader(body))`.
  The regular-file check must happen on the open handle before the close, as `serveContent`
  does today; consider splitting `serveContent` so the header and incomplete-body reporting are
  shared between the file path and the in-memory path instead of duplicated.
- Bound rationale (ADR 0024 note): five-digit segment numbers allow 100,000 segments of about
  32 bytes each (`#EXTINF:6.000000,` + `seg-NNNNN.m4s`), so 4 MiB never cuts a real playlist.
  The producer's readiness poll keeps its own 64 KiB bound; do not touch `adapters/ffmpeg`.
- The rewrite is a pure function on `[]byte` in `playlist.go`; match `#EXTM3U` exactly at the
  start, keep the original line ending (LF or CRLF) for the inserted line, and detect an
  existing tag by a line starting with `#EXT-X-START:`.
- RFC 8216 4.3.5.2 allows `EXT-X-START` in media playlists, at most once; `TIME-OFFSET=0` needs
  no `PRECISE` attribute.
- Do not change the signatures of existing test helpers in `helpers_test.go` that
  `corpus_test.go` uses; add new helpers instead.
- `scripts/playback-smoke` parses the playlist (`playlist.go`): it ignores unknown tags, so it
  needs no change; mention it in the REPORT if it does.
- The BACKLOG row `http/stream` "M2-29 review leftovers" (sendfile warn, `Cache-Control` on 416)
  is not part of this task.
