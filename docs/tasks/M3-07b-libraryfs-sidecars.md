# M3-07b FS: sidecar files, NFC paths and bounded sidecar reads
Owner: mivo-backend
Depends on: M3-04
Files/areas: `internal/adapters/fs/libraryfs/**` (`walk.go`, `access.go`, new
`sidecar.go`, `normalize.go`, tests), `internal/app/scan/run.go` (one filter line + test in
`run_test.go`), `go.mod`/`go.sum` (only the promotion of `golang.org/x/text`)

## Goal
The confined walker reports NFO and image files next to videos with their class and an
NFC-normalised path, and a bounded, confined read returns a sidecar's bytes. The scan v2 uses
both; the milestone-2 scan keeps its behaviour until then. ADR 0027 §3 (normalisation at the
boundary), §9.

## Acceptance criteria
- [ ] `Walk` visits every file that `library.ClassifyFile` accepts, setting `FileState.Class`
      and `FileState.NormalizedPath` (NFC of the relative path; empty when equal). Skipped
      folders, link rules and the depth limit are unchanged (ADR 0022 Notes M2-12, M2-40).
- [ ] `ReadSidecar(ctx, root, relativePath string, maxBytes int64) ([]byte, error)` opens
      through `os.Root`, refuses non-regular files, returns `domain.ErrUnsupported` when the
      file is larger than `maxBytes` (without reading it whole), `domain.ErrNotFound` for a
      vanished file, `domain.ErrForbidden` for a path leaving the root, and honours `ctx`
      between reads.
- [ ] The milestone-2 scan ignores non-video classes with one filter in `run.go`'s walk
      callback (test: a library with `poster.jpg` and `movie.nfo` scans exactly as before).
- [ ] Tests: NFD file names on disk (`Amélie (2001)`) are reported with the raw path
      and the NFC path; sidecar classes; size limit; symlink to a sidecar outside the root
      refused; Windows and unix variants as today (`go test -race
      ./internal/adapters/fs/libraryfs/...` locally and via the Docker line for unix).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/fs/... ./internal/app/scan/...`, coverage gate,
      `go mod tidy -diff`.

## Notes
- macOS clients write NFD names over SMB; the raw path must stay the stored and opened one,
  only parsing uses the NFC form.
- `golang.org/x/text` is already in the module graph (indirect); M3-07 promotes it too
  (identical change).
- Do not change the `scan.Walker` port signature here; M3-10 reworks the scan.
