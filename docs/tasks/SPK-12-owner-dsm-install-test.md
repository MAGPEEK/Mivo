# SPK-12 Manual install test on the owner's Synology
Owner: mivo-tester (prepares, guides and records; the **project owner performs** the steps on his
NAS)
Depends on: SPK-09, SPK-10 PASS, SPK-11 PASS, SPK-16 (own FFmpeg); SPKs built with SPK-07 from the
0.1.0-alpha.2 release candidate commit (the commit M2-36 reports on)
Files/areas: `docs/reports/SPK-12-dsm-install-<version>.md` (new) only

## Goal
The x86_64 server SPK is proven on real hardware – the media server installed as a DSM service and
used from a browser on another device – on the owner's AMD Synology Plus without iGPU, through
install, use, restart, upgrade and uninstall, and the facts that ADR 0026 Notes leave unconfirmed
are checked on a real DSM 7.2.

## Acceptance criteria
- [ ] The tester hands the owner the exact SPK files, their SHA-256 (all `inspect-spk.sh --release`
      clean) and `packaging/synology/TESTING.md`, and records the owner's observations per step
      (pass/fail, notes, screenshots optional). Agents never log in to the NAS.
- [ ] Steps the owner performs (expected result in brackets):
  1. Model, platform and DSM version from Info Center; over SSH `uname -r`, `ls -l /bin/sh`
     [DSM ≥ 7.2, x86_64 platform, kernel recorded, shell recorded].
  2. Checksum on the PC: `Get-FileHash <file>.spk -Algorithm SHA256` [matches].
  3. Package Center > Manual Install [third-party alert, GPL licence page to accept, the **classic
     wizard** with the shared-folder field defaulting to `video`, German texts when DSM is German];
     first try a non-existent share name [refused with a message, no new share created], then an
     existing video share; finish.
  4. Package list [display name, icon, version `0.1.0-1020`, beta label, status "Running"].
  5. Control Panel > Shared Folder > the share > Edit > Permissions > System internal user
     [package user `<id>` has Read only]; over SSH `ls -l /var/packages/<id>/shares/`
     [link to the share] and `ls -l /var/packages/<id>/scripts/` [`common.sh` and `package.env`
     present].
  6. Control Panel > Security > Firewall > rules > select applications [service with the display name
     and `8460/tcp`].
  7. "Open" in Package Center [web app at `http://<nas>:8460/` (record the exact URL), library
     listed, scan finishes, items appear].
  8. Play one direct-play, one remux and one transcode file [plays; overlay shows the mode; Resource
     Monitor shows FFmpeg CPU load during transcode; over SSH
     `/var/packages/<id>/target/bin/ffmpeg -version | head -1` shows the own build].
  9. Logs [look for a log view in Package Center and record what exists; over SSH
     `sudo tail -n 50 /var/packages/<id>/var/<id>.log` shows startup and scan;
     `sudo tail /var/log/packages/<id>.log` shows the scripts' short output].
  10. LAN client over IPv6 if available [plays]. Optional: QuickConnect URL with `:8460` [expected to
      fail]; optional, then removed again: a temporary port forward from outside [403].
  11. Stop in Package Center [stopped within 20 s, no timeout dialog; `ps aux | grep ffmpeg` empty].
  12. Start again [running; cold start time recorded]; create a file in
      `/var/packages/<id>/tmp/transcode/` and reboot the NAS [package starts by itself, library
      available, the file was removed by `start`].
  13. Upgrade with the same version, `PACKAGE_REVISION=1` (`0.1.0-1021`) [upgrade offered,
      `config.yaml` kept, items listed without rescan, share grant and link still there, no port
      conflict reported].
  14. **Version-scheme test** with two pairs of dummy SPKs built by SPK-07 (server versions chosen to
      give these DSM versions): `0.1.0-1020` → `0.1.0-1030` and `0.1.0-9000` → `0.1.1-1010`
      [Package Center offers the second of each pair as an upgrade]. If DSM refuses either, record
      the dialog; the owner then decides on the globally monotonic build number (ADR 0026 Notes).
  15. Add a second share: grant Read in Control Panel, add it to `config.yaml` over SSH as the
      README describes, restart [second library appears].
  16. Uninstall with "delete data" unchecked, reinstall [previous config and library back]; uninstall
      with "delete data" checked [`/var/packages/<id>/var` contents gone; shares, grants and media
      untouched].
- [ ] The report lists every step with result, SPK checksums, DSM version and the owner's
      confirmation, and a table "unconfirmed facts → result" (wizard format, `data-share`
      templating, shares link, extra files in `scripts/`, `/bin/sh`, start/stop timeouts, `tmp`
      after reboot, Open-button URL, log view, QuickConnect, version comparison). Any failed step
      → status FAIL with reproduction notes and a proposed fix task.
- [ ] Gates: none besides the report (no code changes).

## Notes
- Steps 13 and 14 need extra SPKs; the orchestrator builds them with SPK-07 before the session and
  labels them clearly as test-only (never published).
- The owner's NAS may already run the Docker container on port 8460: stop it first, or record what
  Package Center does with the conflict (`checkport`) as an extra observation.
- If the classic wizard does not work (step 3), stop and report: the fallback is the Vue wizard
  with `os_min_ver` `7.2-72806` (new task).
- armv8 and armv7 are not hardware-tested here; their status comes from SPK-11 (OD-SPK-4).
