# SPK-13-pre round 3 - test report

Scope: uncommitted changes on top of e23cb6b in worktree `.claude/worktrees/agent-ac721c6e59a78d284`
(BACKLOG row `spk scripts`, "SPK-13-pre round-2 review minors"). Windows 11, Docker Desktop, Git Bash.

Result: **PASS**

## Gates

| Gate | Result |
|---|---|
| Script bats (BusyBox sh, dash, bash --posix, bash), `spk-script-test:dev test/scripts` | 367 ok, 0 not ok (round 2: 351) |
| Build-script bats, `spk-script-test:dev test/build` | 59 ok, 0 not ok |
| Build-script bats, `spk-build-test:dev test/build` (git + pwsh) | 90 ok, 0 not ok |
| Inspect bats `spk-inspect:dev bats test/inspect` | 58 ok, 0 not ok (round 2: 57) |
| shellcheck v0.11.0, 7 invocations per test/README plus inspect-spk.sh `-s sh`, inspect bats and fixture | exit 0 each |
| `go vet ./packaging/...` | clean |
| `go test -race -coverprofile ./packaging/...` | ok; spkstage 88.9 %, spkicon 96.8 %, spkmeta 99.5 %, total 92.7 % |
| `golangci-lint run ./packaging/...` | 0 issues |
| `build.sh --test 0.1.0-alpha.2` | rc 0, "done in 51s (test build)", three SPKs, SHA256SUMS, source tar |
| `inspect-spk.sh` test and `--release`, x86_64, armv8, armv7 (6 runs) | each rc 0, `91 checks, 0 failed, 2 warnings` (known `bin/ffmpeg`/`bin/ffprobe` "no GNU ABI note") |
| SPK-11 smoke (corpus: 45 kept, 0 failed) | x86_64 28 passed, 0 failed; armv8 28/0; armv7 82/0 |

Not applicable: web, E2E, Docker server image; nothing under `internal/` changed, so the domain
import rule was not re-checked. The tests ran against the uncommitted tree (no `git status`
additions besides the nine modified files; `dist/` and `testdata` are ignored).

## Backlog row items

All met.
- Lock wait restarts when the holder changes (`watch_lock_holder`, `lock_seen_holder`): bats
  `lock_wait_starts_over_for_a_new_holder` (holder empties the PID record, rewrites it; no
  "breaking" message, waits >= 3 s).
- Only this package's script counts as a live action (`is_action_process`: `$0` or suffix
  `/<id>/scripts/start-stop-status`): bats `lock_of_another_packages_action_is_broken` (broken in < 3 s).
- Hard limit `LOCK_LIMIT_SECONDS=300` with log message "pid N still holds the action lock after
  S s; it is taken for hanging" (override `PKG_TEST_LOCK_LIMIT_SECONDS`, documented in test/README):
  bats `lock_held_past_the_hard_limit_is_broken` (broken between 2 and 6 s).
- Failed write of the lock PID (`record_lock`): lock removed, action continues, message logged:
  bats `lock_without_record_is_given_up` (dangling-link fake mkdir, works as root).
- `pkg_volume_directory_kind` accepts digits only (`*[!0-9]*`): lifecycle bats rejects `volumeX`
  and `volume1x`.
- ADR 0026 section 9 reflowed/extended (hard limit, holder reset, record failure) and the
  "Architectures and kernels" line reflowed.
- inspect `read -r` blank trimming: `IFS= read -r` with `listing_mode`/`listing_name`; bats
  "member names keep trailing blanks". Leading blanks are documented as indistinguishable from
  `tar -tv` column padding (toolkit/README), a stated limit, not a defect of the row.

## Observations (not failures)

- The row asked for "a hard upper bound"; the implementation uses 5 minutes, documented in
  the script, ADR and test README.
- toolkit/README line 436 is one line over the wrapped width after the edit (cosmetic).

## Findings

None.

## History

- Round 1: `SPK-13-pre-scripts-test.md`, `SPK-13-pre-inspect-test.md`.
- Round 2: `SPK-13-pre-round2-test.md` (91 checks per SPK).
- Round 3: this report; 91 checks per SPK.
