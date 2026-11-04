# SPK-17 Install wizard without the media folder question
Owner: mivo-synology
Depends on: M3-19 (all slices through M3-19o: libraries are added in the web app; browse roots
and install kind exist), SPK-12 (owner device test of alpha.2), SPK-13 (alpha.2 SPKs published –
the upgrade baseline)
Files/areas: `packaging/synology/package/WIZARD_UIFILES/{install_uifile,install_uifile_ger}`,
`packaging/synology/package/conf/resource`,
`packaging/synology/package/scripts/{preinst,postinst,preupgrade,postupgrade,common.sh,start-stop-status}`,
`packaging/synology/test/**` (`scripts/*.bats`, `fixtures/**`, `share-name-check.js`,
`Dockerfile`, `inspect/**`, `smoke/**`, `README.md`),
`packaging/synology/internal/spkmeta/template_test.go` (example strings only),
`packaging/synology/{README.md,TESTING.md,DSM-NOTES.md}`,
`docs/adr/0026-synology-spk-package.md` (a new Notes section only – **not before the other
agent's current edit of ADR 0026 has merged**), `docs/reports/**` (device test report)

## Goal
Owner decision 2026-11-04: from the release after 0.1.0-alpha.2, installing the Mivo package
asks no media folder question. The package installs, starts with zero libraries, and the web app
leads the user to Settings › Libraries, where shared folders are added with the folder browser;
shared folders the package user cannot read come with the DSM permission steps (M3-19n). An
alpha.2 installation keeps its library across the upgrade. The uninstall wizard (keep or delete
data) stays unchanged.

## Acceptance criteria
- [ ] **Install wizard**: no input field and no `wizard_media_share` key anywhere in the
      package. Either one info-only page (en/de: the server runs on the NAS, open it at port
      8460 or through the DSM main menu, add libraries in Settings › Libraries, grant the
      internal user `@ID@` read access to shared folders in Control Panel › Shared Folder ›
      Edit › Permissions; local network only until accounts exist) or no install wizard at all
      – decided on the owner's device (DSM 7.2.2 must accept an info-only classic wizard;
      if it does not, the files are removed). The uninstall wizards are byte-identical to
      alpha.2.
- [ ] **`conf/resource`**: the `data-share` resource is removed; `port-config` is unchanged.
      `conf/privilege` unchanged (`run-as: package`).
- [ ] **`preinst`**: no share validation any more; it stays as a minimal script that exits 0
      (unless DSM-NOTES shows the script may be omitted – then remove it and update the member
      list). **`postinst`**: creates the directories as before and writes **no** `config.yaml`;
      an existing `config.yaml` stays byte-identical.
- [ ] **`start-stop-status`**: exports `<ENV_PREFIX>_INSTALL_KIND=synology` and
      `<ENV_PREFIX>_BROWSE_ROOTS` = the `:`-joined list of existing directories matching
      `/volume[0-9]*` and `/volumeUSB[0-9]*` at each start (sorted; empty list when none), next to
      the package's path variables. Tests with fake volume roots (the
      `PKG_TEST_VOLUMES_ROOT` pattern of SPK-06).
- [ ] **First start state**: a fresh install has no `config.yaml`; `GET /api/v1/libraries`
      returns `[]`; the web app shows the empty state with "Add library" (M3-19m).
- [ ] **Upgrade from 0.1.0-alpha.2** (config.yaml written by alpha.2's `postinst` with
      `path: "/var/packages/<id>/shares/<share>"`): `preupgrade` records the resolved target of
      every `/var/packages/<id>/shares/*` link (`readlink -f`) in a file under
      `$SYNOPKG_PKGVAR`; `postupgrade` rewrites only library `path` lines that match exactly the
      format alpha.2 wrote and point into `/var/packages/<id>/shares/`, to the recorded target
      (`/volumeN/<share>`; fallback: the single `/volume*/<share>` directory if no target was
      recorded), after saving the original as `config.yaml.bak-0.1.0-alpha.2`; lines in any
      other form are left alone and logged. Because the release also carries migrations
      `00003`/`00004`, the server then imports the library once (M3-19 Notes "Persistence");
      the path change costs no extra rebuild. Tests: alpha.2 file, hand-edited file, link
      present / missing, share with spaces and non-ASCII letters, no `config.yaml`.
- [ ] **Grant after the upgrade**: DSM's `data-share` `Release()` does nothing (DSM-NOTES §7), so
      the read-only grant of the alpha.2 share is expected to stay; the device test confirms
      it. If it does not stay, the library is shown unavailable/unreadable with the permission
      help and README/TESTING describe the one-time grant.
- [ ] **Tests**: `lifecycle.bats` and `package-files.bats` updated (no share-name cases for the
      wizard; resource has only `port-config`; wizard JSON valid; brand-free); the share-name
      fixtures and `share-name-check.js` (and Node in `test/Dockerfile`, if only it needed Node)
      are deleted when nothing uses them any more; `pkg_check_share_name` and `pkg_share_link`
      stay only if the upgrade rewrite uses them. shellcheck clean on all scripts.
- [ ] **Inspect expectations** (`test/inspect/**`): `expected-members.txt` matches the new
      package (wizard files present or absent per the device decision), the inspection fails
      an SPK whose `conf/resource` contains `data-share` or whose wizard contains
      `wizard_media_share`.
- [ ] **Emulated smoke** (`test/smoke/**`): installs without wizard keys, starts, checks
      `GET /api/v1/libraries` = `[]`, creates a library through `POST /api/v1/libraries` with
      the browse root `/volume1` and the corpus share, waits for the scan and runs the playback
      smoke; a second scenario upgrades an alpha.2-shaped installation (`config.yaml` with the
      link path, data directory) and checks the library is listed with the `/volume1/<share>`
      path. Green for every family (SPK-11 rules).
- [ ] **Docs**: `packaging/synology/README.md` – install steps without the folder question;
      "Add libraries" in the web app with the DSM permission steps (Control Panel › Shared
      Folder › select › Edit › Permissions › "System internal user" › `<id>` › Read only); the
      SSH `config.yaml` section reduced to "advanced: `libraries` in `config.yaml` is imported
      once"; troubleshooting row for "a shared folder is missing in the folder browser";
      upgrade note for alpha.2. `TESTING.md` – rows A3 … A8, A11, B2, D1 … D3, E1 … E4, F3 and
      the "unconfirmed" table rewritten for the new flow (fresh install empty state, add a
      library in the app, folder browser shows only granted shares, permission help, grant then
      reopen, upgrade from alpha.2 keeps the library, path rewritten, backup file present).
      `DSM-NOTES.md` – §7 marked "no longer used since SPK-17" (facts kept as history), the
      wizard-without-input finding added.
- [ ] **Owner device test** (DS925+, DSM 7.2.2) recorded in `docs/reports/`: fresh install,
      empty state, add library, permission help for an ungranted share, upgrade from the
      installed alpha.2 SPK keeps the library; German DSM labels of the permission steps checked
      against the strings of M3-19n (corrections listed in the report for mivo-frontend).
- [ ] Gates: script tests under busybox `sh`, `dash` and `bash`; shellcheck; `go test
      ./packaging/synology/...`; the SPK build for all families; SPK-10 inspection green.

## ADR 0026 changes this task makes (as a new Notes section; the decision text is not rewritten)
1. **Owner decision 2026-11-04 supersedes OD-SPK-6 A and §7**: the install wizard asks for no
   shared folder; there is no `data-share` resource; `postinst` writes no `config.yaml`;
   libraries are added in the web app (ADR 0028, M3-19); access to shared folders is granted by
   the user in Control Panel › Shared Folder › Edit › Permissions, guided by the app.
2. **§6 table**: two rows – `<PREFIX>_BROWSE_ROOTS` (the existing `/volume*` and `/volumeUSB*`
   directories, set by `start-stop-status`) and `<PREFIX>_INSTALL_KIND=synology`; the sentence
   "libraries come from `config.yaml`" becomes "libraries are managed in the app; `libraries` in
   `config.yaml` is a one-time import seed".
3. **§10 install/upgrade**: install = no wizard input (or no wizard) → `postinst` creates
   directories only; upgrade from alpha.2 = `preupgrade` records share-link targets,
   `postupgrade` rewrites alpha.2's link paths to `/volumeN/<share>` with a backup.
4. **Consequences**: "Library management on DSM needs SSH until milestone 7; the wizard covers
   the first library" is replaced by "libraries are managed in the web app from the release
   after 0.1.0-alpha.2; granting share access stays a DSM step".
5. **Notes "Wizard"**: the classic wizard is now uninstall-only (plus the info page if kept);
   result of the device check.
6. **Notes "Resources"**: `data-share` no longer used; the alpha.2 grant persists after the
   upgrade (or not – device result).
7. The decisions table keeps its rows; the new Notes section states that it overrides OD-SPK-6
   ("where a note contradicts the decision text above, the note wins", as the earlier Notes).

## Notes
- The release that ships this task must also ship M3-19 (libraries cannot be added otherwise)
  and the migrations `00003`/`00004`; SPK-13's release checklist gets that line.
- Browse roots are package-owned like the paths (the environment wins over `config.yaml`); a
  user cannot widen them on DSM, which is intended: shares are the unit of access on DSM.
- `/volume1` lists every share name; the server shows only those the package user can read and
  counts the others (M3-19d), so share names the user did not grant are never displayed.
- Do not edit `docs/adr/0026-synology-spk-package.md` while another agent's edit is open; add
  the Notes section in this task's last step after rebasing.
- Version and file names follow ADR 0026 §5 for whatever version the release gets.
