# Manual test protocol on a Synology NAS

The step-by-step protocol of task SPK-12: the x86_64 package is installed, used, restarted,
upgraded and removed on the project owner's NAS (an AMD Synology Plus without iGPU, DSM 7.2),
and the DSM facts that ADR 0026 Notes leave unconfirmed are checked on a real DSM. The tester
prepares the files and this protocol, the **owner performs every step on the NAS** (agents never
log in to it), and the tester records the results in `docs/reports/SPK-12-dsm-install-<version>.md`.

`<id>` is the package name (currently `mivo`); `<nas>` is the NAS's address. SSH commands are
run as an administrator; `sudo` asks for the password. The user guide (`README.md` in this
directory) explains what each dialog means; this file only says what to do and what to expect.

## Hand-over (tester → owner)

| Item | Content |
|---|---|
| SPK for the NAS | `<id>-<version>-x86_64.spk`, built with `build.ps1 -Release` from the release-candidate commit (the one `docs/reports/` reports on); `inspect-spk.sh --release` clean |
| checksums | the `SHA256SUMS` line of that file (and of every extra SPK below) |
| extra SPKs for the upgrade and version steps (D2–D6), **test-only, never published** | the same version with package revision 1 (`-PackageRevision 1`, Package Center `0.1.0-1021`); the pairs `0.1.0-1020` → `0.1.0-1030` and `0.1.0-9000` → `0.1.1-1010` (server versions `0.1.0-alpha.3`, `0.1.0` and `0.1.1-alpha.1`). The build scripts name them like real releases, so the tester **renames them by hand** (for example `<id>-0.1.1-alpha.1-TEST-x86_64.spk`) before handing them over; they may be test builds with an interim FFmpeg, since D4–D6 only look at Package Center's version handling |
| this file | printed or open on the PC; the Result column is filled in per step |

What the owner needs on the NAS before starting: DSM 7.2 or newer; SSH enabled (Control Panel >
Terminal & SNMP); an existing shared folder with a few videos – ideally one MP4 (H.264/AAC, direct
play), one MKV with H.264 (remux) and one file that needs a transcode (HEVC, MPEG-2 or VC-1); a
second shared folder with at least one video for step E1; a second device in the LAN with a
browser; and **the Docker container of the server stopped** if one runs on port 8460 (or note what
Package Center does with the conflict as an extra observation in step A2).

## Data the owner reports back

| Data | How to get it | Value |
|---|---|---|
| Model | Control Panel > Info Center > General | |
| DSM version and build | Info Center > General ("DSM 7.2.x-xxxxx Update n") | |
| CPU / platform | Info Center > General (CPU) and over SSH `cat /proc/sys/kernel/syno_platform_name` (if present) or `uname -m` | |
| Kernel | `uname -r` | |
| `/bin/sh` | `ls -l /bin/sh` and `/bin/sh --version` (prints a version for bash; an error for BusyBox) | |
| RAM | Info Center > General | |
| DSM language | Control Panel > Regional Options (German or English) | |
| SPK file name and SHA-256 | file name and `(Get-FileHash <file>.spk -Algorithm SHA256).Hash` on the PC | |
| Date of the test | | |

## Protocol

Result: **pass** / **fail** plus a short note (and screenshots of dialogs with wording we could
not know in advance: the third-party alert, the licence page, the wizard, any error). A failed
step is not fatal for the run: continue where possible and record what happened.

### A Install

| # | Action | Expected | Result |
|---|---|---|---|
| A1 | On the PC: `(Get-FileHash <file>.spk -Algorithm SHA256).Hash.ToLower()` and compare with the hand-over line | identical | |
| A2 | Package Center > Manual Install > Browse > the SPK > Next | a third-party alert appears (record its wording), then a licence page with the GPL-3.0 text to accept, then the install wizard (record the order of alert, licence and wizard). If the Docker container still runs on 8460: record whether DSM reports a port conflict here | |
| A3 | In the wizard, read the explanatory text; field "Shared folder" | the **classic wizard** renders: one page with a text, a text field prefilled with `video`; the texts are **German when DSM is German** (else English) | |
| A4 | Enter a name that breaks DSM's naming rules (for example `video/x` or a leading `-`) | the Next button stays disabled (or an error text under the field appears) | |
| A5 | Enter a well-formed name of a shared folder that **does not exist** (for example `nosuchshare`), Next, finish | the installation is **refused** with a message naming the folder ("does not exist …"); afterwards Control Panel > Shared Folder shows **no new shared folder** of that name | |
| A6 | Manual Install again, enter the real video share, Next, keep "Run after installation" ticked, Done. SSH: `cat /proc/sys/kernel/syno_platform_name` | installation completes without error (DSM accepted `arch="x86_64"` on this platform; record the platform name); Package Center lists the package | |
| A7 | Package Center > Installed: the package entry and its detail page | display name, icon, version **`0.1.0-1020`**, a beta label, status **Running**; the description text is shown in full (German description on a German DSM, English otherwise) | |
| A8 | Control Panel > Shared Folder > the video share > Edit > Permissions > drop-down "System internal user" | the user `<id>` is listed with **Read only** ticked, nothing else | |
| A9 | SSH: `ls -l /var/packages/<id>/shares/` | a symbolic link named after the share, pointing at `/volumeN/<share>` | |
| A10 | SSH: `ls -l /var/packages/<id>/scripts/` | the seven scripts **plus `common.sh` and `package.env`** are present | |
| A11 | SSH: `sudo cat /var/packages/<id>/var/config.yaml` and `ls -l /var/packages/<id>/var/` | `config.yaml` (mode `-rw-r-----`, owner `<id>`) with one library whose `path` is `/var/packages/<id>/shares/<share>`; directories `data`, `cache` (mode `drwxr-x---`); `<id>.log` and `<id>.pid` | |
| A12 | Control Panel > Security > Firewall > Edit Rules > Create > select from the list of built-in applications | a service with the display name and port `8460/tcp` is listed (cancel afterwards unless a rule is needed) | |

### B Use

| # | Action | Expected | Result |
|---|---|---|---|
| B1 | Package Center > the package > **Open** | a new browser tab with the web app; **record the exact URL** (host name or IP, port 8460, path). If no Open button exists, record that and type `http://<nas>:8460/` | |
| B2 | In the web app: the library of the wizard share is listed; wait for the scan (or start it from the UI) | the scan finishes; items of the share appear | |
| B3 | Play one direct-play file (MP4 H.264/AAC) | plays; the stream-info overlay shows **Direct Play** | |
| B4 | Play one remux file (MKV H.264) | plays; overlay shows **Direct Stream** (remux) | |
| B5 | Play one transcode file (HEVC, MPEG-2, …); meanwhile DSM > Resource Monitor > Process | plays; overlay shows **Transcode**; an `ffmpeg` process of user `<id>` uses CPU | |
| B6 | SSH: `/var/packages/<id>/target/bin/ffmpeg -version \| head -1` | the version line of the **own build** (FFmpeg 9.0.2 …; "Jellyfin" or 7.0.2 would be an interim build = wrong SPK) | |
| B7 | Package Center: look for any log view of the package (buttons, menu, the package's page) | record whether one exists and what it shows (none is expected) | |
| B8 | SSH: `sudo tail -n 50 /var/packages/<id>/var/<id>.log` | start-up lines (`http server listening … lan_only=true`), the library sync and the scan; no errors | |
| B9 | SSH: `sudo tail /var/log/packages/<id>.log` | the scripts' short output (`wrote … config.yaml`, `starting …`, `… is running`) | |
| B10 | From a second device in the LAN, over **IPv6** if the network has it (`http://[<ipv6 of nas>]:8460/`) | the web app loads and plays | |
| B11 | Optional: the NAS's QuickConnect URL with `:8460` (and the QuickConnect relay URL with the port) | **expected to fail** (no connection); record what happens | |
| B12 | Optional, then removed again: a temporary port forward 8460 on the router, opened from a mobile network | `403` ("Requests from outside the local network are refused"); remove the forward | |

### C Stop, start, reboot

| # | Action | Expected | Result |
|---|---|---|---|
| C1 | Package Center > Stop; note the time until the status changes | **Stopped** within 20 s, no timeout dialog; SSH: `ps aux \| grep -c '[f]fmpeg'` prints `0`; `ls /var/packages/<id>/var/<id>.pid` – no PID file | |
| C2 | Package Center > Run (start); note the time until **Running** | Running; record the start time (expected a few seconds; a slow start may show "not healthy after 30 s" in the package log) | |
| C2a | **Provoked start failure**: Package Center > Stop; SSH: `python3 -m http.server 8460 --bind 0.0.0.0` (or any other program listening on 8460, for example the Docker container); Package Center > Run | the start **fails**; record whether Package Center shows a dialog with "stopped during start (exit status …)" and the last log lines, or only an error without text; `sudo tail /var/log/packages/<id>.log` shows that text in any case, and `/var/packages/<id>/var/<id>.log` ends with `address already in use`. Stop the listener (Ctrl-C), then Run again: Running | |
| C3 | SSH: `sudo touch /var/packages/<id>/tmp/transcode/stale.txt` (the package user owns the directory; `sudo` writes as root, which is fine for this test), then reboot the NAS | after the reboot the package is **Running** without doing anything; the library is available in the web app; `ls /var/packages/<id>/tmp/transcode/` shows **no `stale.txt`** (and record whether `tmp` itself survived the reboot) | |

### D Upgrade and version scheme

| # | Action | Expected | Result |
|---|---|---|---|
| D1 | SSH: `sudo sha256sum /var/packages/<id>/var/config.yaml`; note the item count in the web app | baseline for D3 | |
| D2 | Manual Install with the **revision-1 SPK** (`…-r1-x86_64.spk`) | Package Center offers it as an **upgrade** (not a new install), no shared-folder page, **no port conflict** reported; afterwards version `0.1.0-1021`, Running | |
| D3 | After D2: SSH `sudo sha256sum /var/packages/<id>/var/config.yaml` and `ls -l /var/packages/<id>/shares/`; Control Panel permissions of the share; the web app | the checksum equals D1 (`config.yaml` unchanged); link and grant still there; items listed **without a rescan** (same count as D1) | |
| D4 | Version pair 1: with `0.1.0-1020` installed (reinstall the first SPK if needed: uninstall with data kept, install), Manual Install the `0.1.0-1030` test SPK | offered as an **upgrade** | |
| D5 | Version pair 2: install the `0.1.0-9000` test SPK (uninstall first), then Manual Install the `0.1.1-1010` test SPK | offered as an **upgrade**. If DSM refuses either pair, **record the dialog text** – the owner then decides on a globally monotonic build number (ADR 0026 Notes) | |
| D6 | Restore the real package: uninstall the test SPK (data kept) and install `<id>-<version>-x86_64.spk` again | Running, `0.1.0-1020`, the configuration from before (D1) is used | |

### E Second shared folder

| # | Action | Expected | Result |
|---|---|---|---|
| E1 | Control Panel > Shared Folder > second folder > Edit > Permissions > System internal user > `<id>` > **Read only** > Save | saved | |
| E2 | SSH: `sudo vi /var/packages/<id>/var/config.yaml`, add a second library with `path: "/volumeN/<share>"` (see `README.md`, "Add more shared folders"); `ls -l` afterwards | the file still belongs to `<id>` with mode `-rw-r-----` | |
| E3 | Package Center: Stop, then Run | the second library appears in the web app and is scanned; its items play | |
| E4 | Negative check: remove the Read permission of E1 again, restart the package | the second library is reported as **unavailable** (log: `root_unavailable`), the first one still works; restore the permission and restart | |

### F Uninstall and reinstall

| # | Action | Expected | Result |
|---|---|---|---|
| F1 | Package Center > Uninstall; the wizard page with the checkbox "Also delete the database, settings and logs" | checkbox present and **unchecked by default**; German text on a German DSM. Leave it unchecked, finish | |
| F2 | SSH: `sudo ls -l /var/packages/<id>/var/ 2>/dev/null \|\| sudo ls -l /volume*/@appdata/<id>/` | `config.yaml`, `data/`, `cache/`, `<id>.log` are **still there** (DSM keeps `var`; the `/var/packages/<id>` link may be gone, then look in `@appdata`) | |
| F3 | Manual Install the SPK again; in the wizard enter the **second** share (not the first) | installation completes; the web app shows the **previous libraries** (first share from the kept `config.yaml`; the second only if it was added in E2) and the previous items **without a full rescan** – the wizard folder is **not added** to a kept configuration (as the README says) | |
| F4 | Uninstall again, this time with the checkbox **checked** | uninstall completes | |
| F5 | SSH: `sudo ls -l /var/packages/<id>/var/ /volume*/@appdata/<id>/ 2>&1`; Control Panel > Shared Folder (both folders, content and permissions) | the `var` contents are **gone**; both shared folders and their media are **untouched**; record whether the `<id>` permission entries are still listed | |

## Unconfirmed facts → result

Facts Synology does not document, taken as assumptions by the package (`DSM-NOTES.md`,
ADR 0026 Notes). The tester fills this table from the steps named.

| Fact (assumption) | Step | Result |
|---|---|---|
| Manual Install order: alert → licence page → wizard; alert wording | A2 | |
| classic JSON wizard renders on DSM 7.2.x; `invalid_next_disabled_v2` disables Next | A3, A4 | |
| German wizard files and script messages are used on a German DSM | A3, A5, F1 | |
| a failing `preinst` shows the text of `$SYNOPKG_TEMP_LOGFILE` in the error dialog | A5 | |
| a failing `start` shows the text of `$SYNOPKG_TEMP_LOGFILE` (reason and last log lines) in a dialog | C2a | |
| `arch="x86_64"` installs on the owner's platform (the toolkit maps it to the family; the guide's table may not list it) | A6 | |
| INFO texts are displayed as written (`description`, `description_ger`, `displayname`; no truncation, no broken characters) | A7 | |
| `data-share` templating `{{wizard_media_share}}` grants the share read-only to `<id>` | A8 | |
| `/var/packages/<id>/shares/<share>` link exists before the first start | A9, B2 | |
| extra files in `scripts/` (`common.sh`, `package.env`) are kept by DSM | A10 | |
| path variables (`SYNOPKG_PKGVAR`, …) are set in every script | A11, B9 | |
| `/bin/sh` of DSM (bash or BusyBox) | report table | |
| "Open" button exists without `adminurl`; which host it uses | B1 | |
| no package log view in Package Center | B7 | |
| start and stop finish within Package Center's limits (30 s / 20 s budgets) | C1, C2 | |
| `tmp` survives a reboot; stale transcode output is removed at start | C3 | |
| package starts at boot (`prestart`/`start` with empty status) | C3 | |
| `checkport`: the package's own firewall entry causes no conflict on upgrade | D2 | |
| upgrade keeps the `data-share` grant and link without an upgrade wizard | D3 | |
| DSM compares versions feature-first, then build (`1020` → `1030`, `9000` → `0.1.1-1010`) | D4, D5 | |
| reinstall on the same volume reuses `var` (`postinst` keeps `config.yaml`) | F2, F3 | |
| the share grant stays after uninstall | F5 | |
| QuickConnect cannot reach port 8460 | B11 | |
| LAN-only guard: real client addresses, IPv6 LAN clients admitted, outside refused | B10, B12 | |
| no DSM user named `<id>` existed before (collision risk) | A8 (if the user looks odd, note it) | |

## Not covered here

armv8 and armv7 are not hardware-tested; their status comes from the emulated smoke test
(SPK-11, OD-SPK-4). Installation on Armada 38x and STiH412 models (`armada38x`, `monaco`) stays
unconfirmed until a community test with such a device.
