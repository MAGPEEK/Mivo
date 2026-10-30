# Backlog of deferred review findings

Non-blocking findings from APPROVE reviews that are not yet scheduled in a task. Each entry
names its source; pick entries up the next time the area is touched, or schedule them as a task.
Entries already carried into a later task file are listed there, not here.

| Area | Item | Source | Due before |
|---|---|---|---|
| logging | `pin` gaps, extra words (`passcode`, `bearer`, `jwt`, `otp`, `pwd`, …), secret config key naming rule | M1-05c (Deferred) | M6 (TMDB key) / M7 (PINs) |
| app/system | reload-error wrapping, generator-count assertion, deadline classification, `CreatedAt` ms truncation | M1-06b (Deferred) | next change to `internal/app/system` |
| config/paths | drive-relative `C:cfg` config dir, ADR 0018 node-walk note, `ErrInvalid` wrapping nits | M1-04b (Deferred) | next change to `internal/platform/config` |
| archtest | `go/build`, `go/importer` and file-reading std functions in the domain | M1-03b (Deferred) | next change to `internal/archtest` |
| sqlite | reader pool `SetMaxIdleConns(maxOpen)`; `GOMAXPROCS(0)` instead of `NumCPU` | M1-08 review | next change to `internal/adapters/sqlite` |
| sqlite | concurrent `Migrate` from two processes unguarded → document "one process per data dir" and add a single-instance lock (M1-10 or later) | M1-08 review | before distribution (M9) |
| sqlite | DB/WAL/SHM files created 0644 inside a 0750 data dir → pre-create 0600 or document umask | M1-08 review | M7 (auth data) |
| sqlite | `sqlc.yaml`: verify the binary against the release `checksums.txt`; verify the Docker path produces identical output (blocked by Docker Hub login) | M1-08 review | M1-22 |
| sqlite | nits: Ping that touches the file, reject zero `CreatedAt`, UNC path test, `t.Run` in dsn role loop | M1-08 review | next change |
| testmedia | `encoders` for kept files must not claim this run's alternative (record only when encoded or carry over from the old manifest; test it) | M1-19 review | next change to `scripts/gen-testmedia` |
| testmedia | nits: `expected()` sidecar slice aliasing; second Ctrl+C via `context.AfterFunc(ctx, stop)`; default dispositions for video/audio in multi-track MKVs | M1-19 review | next change |
