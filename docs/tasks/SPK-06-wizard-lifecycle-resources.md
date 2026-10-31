# SPK-06 Install wizard, lifecycle scripts, privilege and resources
Owner: mivo-synology
Depends on: SPK-01, SPK-05
Files/areas: `packaging/synology/package/WIZARD_UIFILES/**`,
`packaging/synology/package/scripts/{preinst,postinst,preuninst,postuninst,preupgrade,postupgrade}`,
`packaging/synology/package/conf/{privilege,resource}`, `packaging/synology/package/port/service.sc`
(name/location inside the SPK per SPK-01), new test files under `packaging/synology/test/scripts/`

## Goal
Installing the server SPK asks for the video shared folder the server should scan, grants it read-only to the package user,
writes a working `config.yaml`, registers port 8460 with the DSM firewall, and upgrades and
uninstalls keep the user's data unless the user asks to delete it.

## Acceptance criteria
- [ ] Install wizard in English and German (file names per SPK-01): short explanation (the media
      server runs on the NAS and is opened in a browser on port 8460; local network only until
      accounts exist; more folders later via Control Panel), one text field
      `wizard_media_share` (default `video`) with a validator that rejects an empty value and
      the characters DSM forbids in share names (SPK-01).
- [ ] Uninstall wizard in English and German: checkbox `wizard_delete_data`, unchecked by default,
      text per OD-SPK-8 (or, if SPK-01 shows DSM always deletes `var`, a notice saying so).
- [ ] Every JSON file under `package/` parses as JSON as it is in the repository (build tokens
      appear only inside strings) – checked by the script test suite (e.g. `jq empty`).
- [ ] `conf/privilege`: `"run-as": "package"` and nothing that grants root.
      `conf/resource`: `data-share` granting **ro** on the wizard's share to the package user
      (templating per SPK-01; if DSM cannot template wizard values, no `data-share` and the
      wizard tells the user to grant "Read" in Control Panel), and `port-config` pointing at the
      `.sc` file.
- [ ] `.sc` file (format per SPK-01): service id `@ID@`, title `@DISPLAY_NAME@`, `@PORT@/tcp`,
      `port_forward="no"`.
- [ ] `preinst`: exits non-zero with a user-visible message (mechanism per SPK-01) when the share
      name is empty or the share does not exist; 0 otherwise. Tests for all three.
- [ ] `postinst`: creates the directories (0750) and, only if `config.yaml` is absent, writes it
      with one library: `name` = share name, `path` = the location of ADR 0026 §7, `kind: movies`,
      values double-quoted and escaped. An existing `config.yaml` stays byte-identical (test).
      Tests for share names with spaces, non-ASCII letters and a backslash (if DSM allows it).
- [ ] The generated `config.yaml` is accepted by the real server: the REPORT shows
      `go run ./cmd/mivo --config-dir <dir>` (with `MIVO_PATHS_DATA`/`_CACHE`/`_TRANSCODE` set to
      temp directories) logging `http server listening` and the library (library path may not
      exist: it is reported unavailable, ADR 0022).
- [ ] `preupgrade`/`postupgrade`: keep `config.yaml`, `data/` and the log (test: content
      unchanged after an upgrade sequence); `postuninst` deletes the contents of `var` only when
      `wizard_delete_data` is `true` and never touches anything outside the package directories
      (tests for both values).
- [ ] Brand-free: `rg -i mivo packaging/synology/package` finds nothing.
- [ ] Tests: the SPK-05 harness runs the new cases under busybox `sh` and `bash`; shellcheck on
      all six scripts → no findings.
- [ ] Gates: shellcheck + script tests green; Go gates unaffected.

## Notes
- Scripts source `common.sh` from SPK-05 for paths and brand values; if a helper is missing there,
  add it in a backwards-compatible way and mention it in the REPORT (SPK-05 is done by then).
- Writing YAML from `sh`: escape `\` and `"` inside double quotes; reject control characters.
  Never generate keys from user input.
- Pitfall: on upgrade DSM may run the resource workers again with the upgrade wizard's (empty)
  values – SPK-01 Q7 tells whether an upgrade wizard must carry `wizard_media_share` over.
- The wizard text must not promise remote access: the server refuses non-local clients until
  milestone 7 (`server.lan_only`).
- Texts name the product only through `@DISPLAY_NAME@`.
