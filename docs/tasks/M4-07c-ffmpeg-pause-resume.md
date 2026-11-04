# M4-07c FFmpeg: pause and resume a production (throttling primitive)
Owner: mivo-media
Depends on: none
Files/areas: `internal/adapters/ffmpeg/pause_linux.go`, `pause_other.go`, `producer.go` (+ tests); optional `Pausable` interface in `internal/app/playback/ports.go`

## Goal
The playback service can freeze a running FFmpeg session without killing it (ADR 0030 §2).

## Acceptance criteria
- [ ] `Production` implements `Pause() error` / `Resume() error` / `Pausable() bool`: Linux sends `SIGSTOP`/`SIGCONT` to the
      process; other OS: `Pausable()` false, calls return `ErrUnsupported` (wraps `domain.ErrUnavailable`).
- [ ] `Pause` and `Resume` are idempotent; `Stop` on a paused process resumes it first, then kills; a paused process
      still dies with the server (Pdeathsig hardening unchanged).
- [ ] Tests (Linux/Docker): a paused transcode stops writing segments (file count constant for 2 s), resumes and finishes;
      unsupported-OS test on Windows; race detector clean.
- [ ] Gates: lint + layer rules + `go test -race` in Docker + coverage gate.

## Notes
- A SIGSTOPped process cannot answer a stderr pipe; the stderr tail reader must not block shutdown.
- The producer's thread-locking rule (ADR 0024 Notes M2-24b) applies: signal from any goroutine is fine, only start/wait are pinned.
