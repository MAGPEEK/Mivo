# SPK-06 Install wizard, lifecycle scripts, privilege and resources
Owner: mivo-synology
Depends on: SPK-01 (done), SPK-05
Files/areas: `packaging/synology/package/WIZARD_UIFILES/**`,
`packaging/synology/package/scripts/{preinst,postinst,preuninst,postuninst,preupgrade,postupgrade}`,
`packaging/synology/package/conf/{privilege,resource}`, `packaging/synology/package/port/service.sc`
(staged by SPK-14 as `port_conf/<id>.sc` in the payload), new test files under
`packaging/synology/test/scripts/`

## Goal
Installing the server SPK asks for the video shared folder the server should scan, grants it
read-only to the package user, writes a working `config.yaml`, registers port 8460 with the DSM
firewall, and upgrades and uninstalls keep the user's data unless the user asks to delete it.

## Acceptance criteria
- [ ] Wizards in the **classic JSON format** (orchestrator decision, ADR 0026 Notes):
      `install_uifile` and `install_uifile_ger` – short explanation (the media server runs on the
      NAS and is opened in a browser on port 8460; local network only until accounts exist; more
      folders later via Control Panel), one `textfield` with key `wizard_media_share`, default
      `video`, `validator` with `allowBlank: false` and a `regex` implementing DSM's share-name
      rules (1–32 characters; none of `` ! " # % & ' ( ) * + , / : ; < = > ? @ [ ] \ ^ ` { } | ~ ``;
      no leading `-` or space; no trailing space; `$` only at the end).
      `uninstall_uifile` and `uninstall_uifile_ger` – checkbox key `wizard_delete_data`, unchecked
      by default, text: unchecked keeps database, settings and logs (DSM keeps `var`), checked
      deletes them; media and shared folders are never touched.
- [ ] A test feeds the share-name regex a table of valid names (spaces, umlauts, `name$`) and
      invalid ones (each forbidden character, leading `-`, trailing space, 33 characters, empty).
- [ ] Every JSON file under `package/` parses as JSON as it is in the repository (build tokens appear
      only inside strings) – checked by the script test suite (e.g. `jq empty`).
- [ ] `conf/privilege`: `{"defaults": {"run-as": "package"}}`, nothing else that grants root, no
      `username`/`groupname`. `conf/resource`: `data-share` granting **ro** on
      `{{wizard_media_share}}` to the user `@ID@`, and `port-config` with
      `"protocol-file": "port_conf/@ID@.sc"`.
- [ ] `.sc` template: section `[@ID@]`, `title="@DISPLAY_NAME@"`, `desc="@DISPLAY_NAME@ media server"`
      (required), `port_forward="no"`, `dst.ports="@PORT@/tcp"`; a test checks that the rendered
      section name contains only letters, digits, `-` and `.`.
- [ ] `preinst`: only when `SYNOPKG_PKG_STATUS=INSTALL`, exits non-zero with the message in
      `$SYNOPKG_TEMP_LOGFILE` when the share name is empty or no `/volume*/<share>` directory exists
      (the `data-share` worker would otherwise **create** a new empty share); 0 otherwise and always
      0 on `UPGRADE`. Tests for empty, missing, existing and the upgrade case.
- [ ] `postinst`: creates the directories (0750); only on `INSTALL` and only if `config.yaml` is
      absent, writes it with one library: `name` = share name, `path` =
      `/var/packages/<id>/shares/<share>` (written without testing: DSM creates the link when the
      resource is acquired, after `postinst`), `kind: movies`, values double-quoted and escaped. An
      existing `config.yaml` stays byte-identical; on `UPGRADE` nothing is written. Tests for share
      names with spaces, non-ASCII letters and a `$` suffix.
- [ ] The generated `config.yaml` is accepted by the real server: the REPORT shows
      `go run ./cmd/mivo --config-dir <dir>` (with `MIVO_PATHS_DATA`/`_CACHE`/`_TRANSCODE` set to temp
      directories) logging `http server listening` and the library (reported unavailable when the
      path does not exist, ADR 0022).
- [ ] `postuninst`: deletes the contents of `$SYNOPKG_PKGVAR` and `$SYNOPKG_PKGHOME` only when
      `SYNOPKG_PKG_STATUS=UNINSTALL` **and** `wizard_delete_data=true`; never on `UPGRADE`; never
      outside the package directories. `preupgrade`/`postupgrade`/`preuninst` keep `config.yaml`,
      `data/` and the log. Tests for every status/flag combination, including the upgrade order
      (old `preuninst`/`postuninst`, new `preinst`/`postinst` with `UPGRADE`).
- [ ] Error output: none of the scripts prints `Error`, `error:` or `fatal error` except for a real
      failure (keeps the toolkit's log grep and DSM's logs clean).
- [ ] Brand-free: `rg -i mivo packaging/synology/package` finds nothing.
- [ ] Tests: the SPK-05 harness runs the new cases under busybox `sh` and `bash`; shellcheck on
      all six scripts → no findings.
- [ ] Gates: shellcheck + script tests green; Go gates unaffected.

## Notes
- Scripts source `common.sh` from SPK-05 for paths and brand values; add missing helpers there
  backwards-compatibly and mention them in the REPORT.
- Writing YAML from `sh`: escape `\` and `"` inside double quotes; reject control characters.
  Never generate keys from user input.
- Unconfirmed (SPK-12 checks): `{{wizard_media_share}}` templating in `conf/resource`; that the
  upgrade wizard need not repeat the share (no `upgrade_uifile` in the first SPK); the classic
  wizard format on DSM 7.2.2. If the classic format fails, the fallback is the Vue format with
  `os_min_ver` `7.2-72806` – a separate task.
- The wizard text must not promise remote access (`server.lan_only` until milestone 7) and names
  the product only through `@DISPLAY_NAME@`.

## Orchestrator note (from the SPK wave-2 review)
- Harden `start-stop-status` while you touch the scripts (in scope for this task): a concurrent
  `status` in the fork→exec gap must not delete a fresh PID file (delete only if the process is
  dead, or wait in `launch_server` until `/proc/<pid>/exe` is the binary before writing the file);
  a failed PID-file write kills the just-started server and exits 1; `rm`/`mv` failures in
  clear_transcode_dir/rotate_log log an error; tests: a running server's transcode dir survives a
  second `start`, an upper bound for the SIGKILL wait; add dash to the default bats matrix.
