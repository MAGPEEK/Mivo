# M4-07a FFmpeg: weak-NAS software baseline (preset, threads, priority)
Owner: mivo-media
Depends on: M4-05a
Files/areas: `internal/adapters/ffmpeg/hlsargs.go`, `process.go`, `lifetime_*.go`, new `priority_*.go` (+ tests)

## Goal
A software transcode on the DS925+ leaves CPU for the web app and the scan: veryfast preset, capped
threads and low OS priority (ADR 0030 §1).

## Acceptance criteria
- [ ] The producer options carry `Preset`, `Threads`, `LowPriority`; HLS args use `-preset <value>` and
      `-threads <n>` (omitted when 0 = auto resolved by the composition root to `max(1, NumCPU/2)`).
- [ ] Linux: the tool starts with niceness 10 (`syscall.Setpriority` after start, or `SysProcAttr`); Windows:
      `BELOW_NORMAL_PRIORITY_CLASS`; other OS: no-op. ffprobe keeps normal priority.
- [ ] Existing hardening tests (parent-death kill, job object) still pass.
- [ ] Tests: arg-builder table tests; Linux test (Docker) reads the child's niceness from `/proc/<pid>/stat`; Windows test reads the
      priority class; corpus test: a veryfast transcode of the 720p clip succeeds.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Must not change the output for existing hlsargs golden tests except the preset/threads flags.
- Setting priority after start leaves a tiny window; acceptable (document).
