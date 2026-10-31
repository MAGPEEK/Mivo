# SPK-12 Manual install test on the owner's Synology
Owner: mivo-tester (prepares, guides and records; the **project owner performs** the steps on his
NAS)
Depends on: SPK-09, SPK-10 PASS, SPK-11 PASS; SPKs built with SPK-07 from the 0.1.0-alpha.2
release candidate commit (the commit M2-36 reports on)
Files/areas: `docs/reports/SPK-12-dsm-install-<version>.md` (new) only

## Goal
The x86_64 server SPK is proven on real hardware – the media server installed as a DSM service
and used from a browser on another device – – the owner's AMD Synology Plus without iGPU – through
install, use, restart, upgrade and uninstall, before it is published.

## Acceptance criteria
- [ ] The tester hands the owner the exact SPK file, its SHA-256 and `packaging/synology/TESTING.md`,
      and records the owner's observations per step in the report (pass/fail, notes,
      screenshots optional). Agents never log in to the NAS.
- [ ] Steps the owner performs (expected result in brackets):
  1. Report model, platform (Info Center or `uname -a` over SSH), DSM version [DSM ≥ 7.2, family
     x86_64].
  2. Check the checksum on the PC: `Get-FileHash <file>.spk -Algorithm SHA256` [matches].
  3. Package Center > Manual Install > choose the SPK [third-party/unsigned notice as documented,
     GPL licence page, wizard with the shared-folder field defaulting to `video`]; enter an
     existing video share and finish.
  4. Package list [display name, icon, version `0.1.0-1020`, beta label if SPK-01 confirmed it,
     status "Running"].
  5. Control Panel > Shared Folder > the share > Edit > Permissions > System internal user
     [package user has Read only].
  6. Control Panel > Security > Firewall > rules > select applications [service with the display
     name and `8460/tcp` listed].
  7. Click "Open" [web app at `http://<nas>:8460/`, library listed, scan finishes, items appear].
  8. Play one direct-play, one remux and one transcode file (corpus files copied to the share or
     own files) [plays; overlay shows the mode; Resource Monitor shows FFmpeg CPU load during
     transcode].
  9. Logs [Package Center log view per SPK-01, or over SSH:
     `sudo tail -n 50 /var/packages/<id>/var/<id>.log` shows startup and the scan].
  10. From a LAN client over IPv6, if the network has it [plays]; optional: from outside the LAN
      with a temporary port forward [403], then remove the forward.
  11. Stop in Package Center [stopped within 20 s; over SSH `ps aux | grep ffmpeg` shows nothing].
  12. Start again [running]; reboot the NAS [package starts by itself, library available].
  13. Upgrade: Manual Install of the same version with package revision + 1 [upgrade dialog,
      `config.yaml` kept, items still listed without a rescan].
  14. Add a second share: grant Read in Control Panel, add it to `config.yaml` over SSH as the
      README describes, restart [second library appears].
  15. Uninstall with "delete data" unchecked, reinstall [previous config and library back, or the
      DSM behaviour SPK-01 documented]; uninstall with "delete data" checked
      [`/var/packages/<id>` gone; shares and media untouched].
- [ ] The report lists every step with result, the SPK checksum, DSM version and the owner's
      confirmation; any failed step → status FAIL with reproduction notes and a proposed fix task.
- [ ] Gates: none besides the report (no code changes).

## Notes
- Step 13 needs a second SPK of the same version with `PACKAGE_REVISION=1`; the orchestrator
  builds it with SPK-07's command before the session.
- The owner's NAS may already run the Docker container on port 8460: stop it first, or note what
  Package Center does with the port conflict (`checkport`) as an extra observation.
- armv8 and armv7 are not hardware-tested here; their status comes from SPK-11 (OD-SPK-4).
