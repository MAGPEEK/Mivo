# SPK-06 test report

Status: PASS (round 1)
Worktree: `.claude/worktrees/agent-a9444488de54da6c0`, uncommitted against base 4fc65ca. Nothing was edited
except this report.

## Gates

| Gate | Result |
|---|---|
| shellcheck (v0.11.0) package scripts, `-s sh`, 9 files | exit 0, no findings |
| shellcheck test files (`*.bats`, helpers, fake-server, fixtures), 7 files | exit 0, no findings |
| Build test image (`packaging/synology/test/Dockerfile`, tagged `spk06-qa-test:tmp` so the existing `spk-script-test:dev` stayed untouched) | built |
| bats, 3 consecutive runs, default matrix (busybox sh, bash --posix, bash, dash) | `1..295`; 295 ok, 0 not ok, 0 skipped in each run (69 cases x 4 shells + 19 shell-independent) |
| Debian dash one-off (`SCRIPT_TEST_SHELLS=dash`, `debian:stable-slim`, start-stop-status + lifecycle) | `1..73`, 73 ok |
| `jq empty` on 4 wizard files, `conf/privilege`, `conf/resource` (jq in the test image) | all 6 OK |
| Brand check `rg -i mivo packaging/synology/package` | no match (rc 1) |
| `go test -count=1 ./packaging/...` | ok spkicon, ok spkmeta |
| `packaging/synology/toolkit/test.sh` with throw-away volume `spk06-qa-toolkit` (seeded read-only from `spk-toolkit`, `SPK_TOOLKIT_VOLUME`) | rc 0, "all toolkit tests passed", 0 failures, x86_64/armv8/armv7 packed |

Cleanup: volume `spk06-qa-toolkit` removed, image `spk06-qa-test:tmp` removed. `spk-toolkit` (volume and
image) and `spk-script-test:dev` were not modified.

Go coverage thresholds and layer rule: not applicable, no Go source changed (the diff contains only
`packaging/synology` shell, templates, tests and `docs/tasks/BACKLOG.md`).

## Real-server check

The Go 1.27.3 binary was built from the worktree (`CGO_ENABLED=0 GOOS=linux go build ./cmd/mivo`) and
run in the `golang:1.27.3` container, rather than through `go run`, which compiles the same code. `package.env` was
rendered with `mivo`/`Mivo`/`MIVO`/`8460`; share name `Vidéos Ärger$` (non-ASCII, space, `$` suffix).

- `preinst` then `postinst` with `SYNOPKG_PKG_STATUS=INSTALL`, under both dash and bash: rc 0.
- Generated `var/config.yaml` (mode 0640; `cache` and `data` directories 0750):
  ```
  libraries:
    - name: "Vidéos Ärger$"
      path: "/var/packages/mivo/shares/Vidéos Ärger$"
      kind: "movies"
  ```
- `mivo --config-dir /nas/var` with `MIVO_PATHS_DATA/_CACHE/_TRANSCODE` set to temp dirs:
  - config accepted: `libraries synced from the configuration libraries=1`,
    `http server listening address=[::]:8460 lan_only=true`.
  - link absent (as right after `postinst`): `library scan finished ... name="Vidéos Ärger$" state=failed
    code=root_unavailable cause="library folder is missing or unreadable"` (unavailable, ADR 0022).
  - link present: `state=idle`, no error.
  - clean shutdown on SIGTERM.

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Classic-JSON wizards (install en/ger: key `wizard_media_share`, default `video`, `allowBlank:false`, regex; uninstall en/ger: `wizard_delete_data` checkbox unchecked) | met | Files read: textfield with the key, `defaultValue: "video"`, `allowBlank: false`, regex with `errorText`; uninstall files use `multiselect` with `defaultValue: false`, text says unchecked keeps database/settings/logs, checked deletes them, media and shares never touched. No remote-access promise ("local network only"). Product only via `@DISPLAY_NAME@`. |
| 2 | Regex test with valid/invalid table | met | `package-files.bats` + `fixtures/share-names.bash` + `share-name-check.js` (Node), green in all runs; `case_share_name_check_follows_dsm_rules` ties `pkg_check_share_name` to the same table. |
| 3 | Every JSON under `package/` parses | met | `jq empty` OK on all 6 files; also in `package-files.bats`. |
| 4 | `conf/privilege`, `conf/resource` | met | privilege is exactly `{"defaults":{"run-as":"package"}}`, keys `["defaults"]`, no `username`/`groupname`/root (`grep` rc 1). Resource: `data-share` ro on `{{wizard_media_share}}` for `@ID@`; `port-config` `protocol-file` `port_conf/@ID@.sc`. |
| 5 | `.sc` template | met | `[@ID@]`, `title="@DISPLAY_NAME@"`, `desc="@DISPLAY_NAME@ media server"`, `port_forward="no"`, `dst.ports="@PORT@/tcp"`; section-name character test in `package-files.bats`, green. |
| 6 | `preinst` | met | Only acts on INSTALL, message to `$SYNOPKG_TEMP_LOGFILE`, exit 1 for empty, invalid or missing share, exit 0 for UPGRADE; cases `preinst_accepts_existing_share`, `_rejects_empty_share`, `_rejects_missing_share`, `_skips_the_check_on_upgrade` green. Real-run: rc 0 for an existing share. |
| 7 | `postinst` | met | Directories 0750; config only on INSTALL and when absent; double-quoted and escaped values; existing config kept byte-identical; nothing written on UPGRADE; cases `postinst_*` incl. `quotes_special_share_names` green; real-run output above. |
| 8 | Config accepted by the real server | met | See "Real-server check". |
| 9 | `postuninst` and upgrade order | met | Deletes `PKGVAR` and `PKGHOME` contents only for UNINSTALL and `wizard_delete_data=true`, only if both are package directories (`pkg_is_package_directory`); `postuninst_deletes_data_only_when_asked_on_uninstall`, `_keeps_data_for_every_other_combination`, `_never_deletes_outside_the_package_directories`, `upgrade_order_keeps_user_data` green. `preupgrade`, `postupgrade`, `preuninst` are `exit 0` (`other_scripts_keep_everything`). |
| 10 | Error output | met | No `Error`, `error:` or `fatal error` string in the new scripts; success cases call `refute_error_words` (7 uses in `lifecycle.bats`). |
| 11 | Brand-free | met | `rg -i mivo packaging/synology/package` finds nothing. |
| 12 | Tests under busybox sh and bash, shellcheck on all six scripts | met | 4-shell matrix green x3, Debian dash green, shellcheck clean. |
| 13 | Gates | met | See table; Go unaffected. |
| - | Uninstall keeps the data by default | met | `wizard_delete_data` unchecked by default; `postuninst` exits 0 without touching anything unless the flag is exactly `true`; `uninstall_keeping_data_then_reinstall` green. |
| - | PID-file hardening (orchestrator note) | met | `start-stop-status`: `wait_for_exec` until `/proc/<pid>/exe` is the binary before the PID file is written; `write_pid_file` writes via `.new` and `mv`; failure leads to `abort_start` (kill, reap, message) and `exit 1`; `running_pid` re-reads the file before removing it; `report_failed_start` removes the file only if it holds its own PID. `rm`/`mv` failures in `clear_transcode_dir`/`rotate_log` log via `pkg_error`. Tests: `start_keeps_transcode_output_of_running_server`, `start_names_transcode_output_it_cannot_remove`, `start_names_log_it_cannot_rotate`, `assert_elapsed_below ... 7` for the SIGKILL wait, and dash in the default matrix (`helpers.bash:20`); all green. |
| - | 18 s comment | met | The `STOP_TIMEOUT_SECONDS` comment now names the 18 s `shutdownBudget` (ADR 0024 Notes M2-32); this matches `cmd/mivo/serve.go:41` (`18 * time.Second`) and ADR 0024:316. |
| - | BACKLOG row "spk scripts" removed | met | `git diff docs/tasks/BACKLOG.md` removes exactly that row; both nits are addressed: the poll cap is rounded up (`(timeout + interval - 1) / interval + 1`) and the README "Used by" reads "start budget cases". |

## Findings

None.

## Notes

- Not testable here, as the task says (SPK-12): `{{wizard_media_share}}` templating in `conf/resource`, the classic wizard
  format on DSM 7.2.2, and link creation by DSM after `postinst`.
- Scratchpad logs: `bats1..3.log`, `dash-oneoff.log`, and `spk06qa/{toolkit.log,real.sh}` in the session scratchpad.

## History

- Round 1: PASS.

REPORT
status: PASS
task: SPK-06
files: docs/reports/SPK-06-test.md
tests: shellcheck (package + test files) clean; bats 3x295/295 ok (busybox sh, bash --posix, bash, dash); Debian dash one-off 73/73 ok; jq empty on 6 JSON files ok; brand check clean; go test -count=1 ./packaging/... ok; toolkit/test.sh (throw-away volume, removed) all passed; real server accepted generated config.yaml for "Vidéos Ärger$" (http server listening, library root_unavailable without link, idle with link)
findings: none
next: orchestrator may proceed to mivo-reviewer
