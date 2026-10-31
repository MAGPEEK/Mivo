# SPK-05 test report - round 3 (wave SPK-03/04/05)

Tester: mivo-tester, 2026-10-31. Clean worktree at HEAD 1895eae plus all changed/untracked
`packaging/synology/**` files; worktree removed afterwards.

## Gates

| Gate | Result |
|---|---|
| shellcheck v0.11.0 `-s sh` on start-stop-status + common.sh | exit 0, no findings |
| Full bats suite (bats/bats:1.14.0), 5 consecutive runs | 5/5 rc=0, 111 ok / 0 not ok each (94-107 s) |
| Timing filter ("ignores sigterm\|terminates gracefully\|budget\|slow server\|unhealthy\|early exit"), 30 iterations | 30/30 passed, 0 failures (27 cases each) |
| dash run (debian:stable-slim, SCRIPT_TEST_SHELLS=dash) | rc=0, 39 ok / 0 not ok |
| Brand check `grep -rli mivo packaging/synology/package` | no matches |
| SPK-04 `toolkit/test.sh` (throw-away volume spk-toolkit-r3tmp, removed; spk-toolkit untouched) | rc=0, "all toolkit tests passed", 0 not ok |
| SPK-03 `go vet` (packaging/synology/internal) | clean |
| SPK-03 `go test -race -cover ./packaging/synology/internal/...` | spkicon 96.8 %, spkmeta 100.0 %, both ok |
| SPK-03 `golangci-lint run ./packaging/synology/internal/...` | 0 issues |

## SPK-05 acceptance criteria

- common.sh location, env derivation, brand-free prefix: met (bats start/exports/package.env cases, 111/111).
- start (dirs 0750, stale transcode removal, log rotation, log append, PID file, health poll,
  early exit exit 1 + log tail, unhealthy after budget 0, already running): met.
- stop (SIGTERM, wait, SIGKILL, PID removal, exit 0 when idle): met (timing filter 30/30).
- status 0/3/150, stale and reused PID: met.
- prestart/prestop/log 0, unknown argument 1 with usage: met.
- Short stdout, server output only in log: met.
- Brand-free package dir and DEMO_* fixture: met (grep empty; bats "package files do not name the product").
- Harness under busybox sh, bash --posix, bash (and dash on request) covering all listed cases: met.
- One documented Docker command for bats (README.md, pinned 1.14.0), passes on Docker Desktop: met.
- shellcheck command: met.
- Gates (shellcheck + script tests green, Go gates unaffected): met.

## Notes

- The README's dash one-liner works as documented.
- Running the timing loop concurrently with other Docker work was not done; the loop ran alone.

## History

- Round 3: PASS (this report). Rounds 1-2: see docs/reports/SPK-wave2-test.md.
