# SPK-12 – Owner DSM test (DS925+)

Protocol: `packaging/synology/TESTING.md`. Results are added step by step; the device facts below
were read over SSH (key login of the dedicated user, no root) on 2026-11-04.

## Device facts

| Item | Value | Source |
|---|---|---|
| Model | DS925+ | `/proc/sys/kernel/syno_hw_version` |
| CPU | AMD Ryzen Embedded V1500B, 8 threads | `/proc/cpuinfo`, `nproc` |
| RAM | 7.7 GiB (7894 MB) | `free -m` |
| Architecture | `x86_64` → SPK family `x86_64` | `uname -m` |
| DSM platform | `v1000nk` (unique `synology_v1000nk_ds925+`) | `synogetkeyvalue /etc.defaults/synoinfo.conf unique` |
| DSM version | **7.4.1-90080 Update 1** (build date 2026-10-02) | `/etc.defaults/VERSION` |
| Kernel | 5.10.55+ (GCC 12.2.0) | `/proc/version` |
| `/bin/sh` | symlink to `bash` | `ls -l /bin/sh` |
| Container Manager | 24.0.2-1706, Docker client 24.0.2 | package INFO, `docker version` |
| Port 8460 | free (5000, 5001 DSM; 32400 Plex) | `netstat -ltn` |
| Other media packages | Plex Media Server, Synology Media Server | `/var/packages` |

## Findings so far

- **DSM is 7.4.1, not 7.2**: the SPK's `os_min_ver="7.2-64570"` allows it, but every DSM fact in
  ADR 0026 and `DSM-NOTES.md` was collected for 7.2; the device test confirms or corrects them
  for 7.4.1. The toolkit stays on the DSM 7.2 branch (no 7.4 toolkit was checked).
- **Platform `v1000nk`** is not in the DSM 7.2 toolkit platform list (`v1000` is). The x86_64 SPK
  uses `arch="x86_64"`, which DSM is expected to accept for every x86_64 platform; to confirm at
  install (TESTING.md A6).
- **`/proc/sys/kernel/syno_platform_name` does not exist** on DSM 7.4.1; TESTING.md now uses
  `synogetkeyvalue /etc.defaults/synoinfo.conf unique` instead.
- **`/bin/sh` is bash** on this device, so the package scripts run under bash here, not ash;
  they are tested under BusyBox sh, dash, `bash --posix` and bash.
- **Docker Engine 24.0.2** confirms the Docker-guide note: `HEALTHCHECK --start-interval` is
  ignored, *healthy* appears after about 30 s.
- **Docker image name**: `image: mivo-media-server` failed with "pull access denied … repository
  does not exist"; the correct name is `docker.io/magpeek/mivo-media-server:<tag>`.
- **SSH for a non-root user**: DSM only admits members of `administrators` over SSH; the test
  user `claude` (uid 1047) is in that group but no password or `sudo` is ever used by the agent,
  so the Docker socket and `/var/packages/<id>/var` stay unreadable for it.

## Run 1 (2026-11-04): test build of 804b256

SPK: `mivo-0.1.0-alpha.2-x86_64.spk`, SHA-256
`6df5885c19894c73ef62fe42a3c41999ee40622eb1bf8a9a65877d39d7ee4025` (test build with the own
FFmpeg 9.0.2-static1; release files are rebuilt in SPK-13).

| Step | Result |
|---|---|
| Install through Package Center (manual install) | **pass** (owner) – DSM 7.4.1 accepted `arch="x86_64"` on platform `v1000nk` |
| A2 dialog order | **pass** (owner) – third-party alert, then licence page, then wizard; alert wording not recorded |
| A3 wizard | **pass** (owner) – classic wizard rendered in **German**, share field prefilled with `video` |
| B playback | **pass** (owner) – a video played in the web app; the stream mode (Direct Play, Remux, Transcode) was not recorded |
| Uninstall (F4 path) | **pass** (owner) – the "delete data" checkbox was present and the owner **ticked** it |
| Not run or not recorded | A4–A5 (refusals), A7–A12 details, B1/B3–B12 details, C (stop/start/reboot), D (upgrade), F1–F3 (keep data and reinstall), F5 leftover check |

Owner's verdict: everything works but is "very rudimentary" so far – expected for 0.1.0-alpha.2
(milestone 2 scope: scan, browse, play; no accounts, metadata, subtitles or artwork yet).

After the run SSH on port 22 was refused, so the agent could not check what the uninstall left
behind (`/var/packages/<id>`, `@appdata/<id>`, the firewall service file).

## Run 2 (2026-11-04): test build of f1c690f with the DSM main-menu entry

SPK: `mivo-0.1.0-alpha.2-x86_64.spk`, SHA-256
`4aecc6a7b153f70cbd7dfff188ed7f291f9d16dfc89ee7798eb2741f7d04ed6e` (test build).

| Step | Result |
|---|---|
| Install, stop and start in Package Center (incl. the new action lock) | **pass** (owner) |
| A13 Mivo icon in the DSM main menu | **fail** (owner) – no Mivo entry in the main menu |
| Uninstall | **pass** (owner) |
| Main menu as a non-admin DSM user | not run |

The main-menu failure needs a fix before SPK-13 (see the SPK-13 prerequisites); the cause was
not visible without SSH.

## Run 3 (2026-11-04): test build 0.1.0-alpha.3 revision 1 (INFO dsmappname)

SPK: `mivo-0.1.0-alpha.3-r1-x86_64.spk` (Package Center: 0.1.0-1031), SHA-256
`82f06ae26ec6ccc5d21897e76a2fce2555b9cd4a65db9552fb3a313e8ef0b536` (test build, never published).

| Step | Result |
|---|---|
| A13 Mivo icon in the DSM main menu, opens the web app | **pass** (owner: "works as it should") – confirms that INFO `dsmappname` equal to the `ui/config` key was the missing piece |
| Main menu as a non-admin DSM user, removal on uninstall | not recorded |

Hand-over note: uninstall this 0.1.0-1031 test build before installing the 0.1.0-alpha.3 release
(0.1.0-1030), otherwise Package Center treats the release as a downgrade.
