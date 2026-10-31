# Synology package (SPK)

The Synology package installs the **media server on the NAS as a DSM service**: the server
binary with the embedded web app, the library scanner and a bundled FFmpeg/ffprobe for probing,
remux and transcode, listening on port **8460**. You use it from a **web browser on another
device** (PC, tablet, phone; later the TV app). It is not a player app for DSM's desktop:
Package Center only starts and stops it and offers an "Open" button.

This file is the user guide (install, configure, upgrade, remove) and, from "Build" on, the
contributor guide. The step-by-step protocol for the hardware test is `TESTING.md`; the DSM
facts behind the package are in `DSM-NOTES.md`, the toolkit findings in `toolkit/README.md`
and the FFmpeg builds in `ffmpeg/README.md`.

Names in this file: `<id>` is the package name, currently `mivo` (`brand.ID`, ADR 0002); the
display name shown in Package Center is `Mivo`. `<version>` is the server version, for example
`0.1.0-alpha.2`. Replace `<id>` in paths and commands with the package name.

## Requirements

- **DSM 7.2 or newer** (`os_min_ver` is `7.2-64570`; DSM 7.2.1 and 7.2.2 are fine). DSM 6 and
  DSM 7.0/7.1 are not supported.
- A NAS of one of the three **architecture families** below; one SPK per family.
- An existing **shared folder with videos** (the installer asks for its name, default `video`).
- About **1 GB RAM** recommended. The package does not refuse smaller models, but software
  transcoding needs memory and CPU (see "Limits").
- A browser on another device in the **same local network** (see "Local network only").
- For settings beyond the first library: **SSH** access (Control Panel > Terminal & SNMP) with
  an administrator account, until a settings page exists (milestone 7).

### Which SPK fits which model

The SPK is chosen by the CPU family of the NAS. Find the CPU in Synology's knowledge-base
article "What kind of CPU does my NAS have?"
(<https://kb.synology.com/en-global/DSM/tutorial/What_kind_of_CPU_does_my_NAS_have>), or look
at Control Panel > Info Center > General (model name) and over SSH run `uname -m`:

| SPK | `uname -m` | DSM platforms (CPU) | Models (examples) | Status |
|---|---|---|---|---|
| `<id>-<version>-x86_64.spk` | `x86_64` | every Intel and AMD platform of DSM 7.2 (apollolake, geminilake, v1000, r1000, denverton, epyc7002, broadwell*, braswell, avoton, kvmx64, …) | all "+", "xs", "xs+", FS, SA, RS models with Intel/AMD CPUs, DS416play, DS418play, VMs with DSM | tested on the owner's AMD model (`TESTING.md`) |
| `<id>-<version>-armv8.spk` | `aarch64` | Realtek RTD1296, RTD1619B; Marvell Armada 37xx | DS118, DS218, DS218play, DS418, DS418j, DS420j, RS819, DS124, DS223, DS223j, DS423, DS119j, DS120j, DS220j | **tested under emulation only** |
| `<id>-<version>-armv7.spk` | `armv7l` | Annapurna Labs Alpine (`armv7` family); in addition Marvell Armada 385/388 (`armada38x`) and STiH412 (`monaco`) | DS1517, DS1817, DS416 (Alpine); DS116, DS216, DS216j, DS218j, DS416j, DS416slim, DS419slim, RS217, RS816 (Armada 38x); DS216play (STiH412) | **tested under emulation only**; installation on Armada 38x and STiH412 models is **untested** (they are not in DSM's `armv7` family; the SPK lists them as extra `arch` values) |

The Status column describes the **published release**: "tested on the owner's AMD model" is
confirmed by the hardware test (SPK-12, `TESTING.md`), "tested under emulation only" by the
emulated smoke test (SPK-11); a family whose test did not pass is not published.

Package Center refuses an SPK whose `arch` does not match the NAS. If none fits, the model is
not a DSM 7.2 model of these families. Synology's Container Manager is an alternative on most
x86_64 and some armv8 models, never on 32-bit ARM; see `../docker/README.md`.

## Install

### 1 Download and check the file

Every release that carries SPKs attaches to its GitHub Release (`https://github.com/MAGPEEK/Mivo/releases`):

- one SPK per family, `<id>-<version>-<family>.spk` (a packaging-only fix of the same server
  version carries a revision, `<id>-<version>-r<N>-<family>.spk`, and replaces the earlier file);
- `SHA256SUMS` with the checksums of the SPKs;
- the FFmpeg corresponding source archive and its `SHA256SUMS` line (see "FFmpeg"; target
  state, added to the build by SPK-16).

Check the download before installing it. On Windows (PowerShell):

```powershell
(Get-FileHash .\mivo-0.1.0-alpha.2-x86_64.spk -Algorithm SHA256).Hash.ToLower()
Select-String -Path .\SHA256SUMS -Pattern 'x86_64'
```

The two values must be identical. On Linux or macOS: `sha256sum -c --ignore-missing SHA256SUMS`
(`shasum -a 256 -c --ignore-missing SHA256SUMS` on macOS) prints `OK` for the files present.

### 2 Install through Package Center

The package is not signed by Synology (DSM 7 dropped package signing), so it is installed by
hand:

1. Open **Package Center** and click **Manual Install** (top right).
2. **Browse** to the `.spk` file and click **Next**.
3. DSM warns that the package comes from a third party ("not from Synology" or similar). Confirm
   to continue. This alert appears for every package outside Synology's catalogue; there is no
   trust-level setting to change any more.
4. The **licence page** shows the GPL-3.0 text of the package, which you accept to continue.
   This is the open-source licence of the software, not a restriction on your use.
5. The **install wizard** asks for the **shared folder** that holds your videos (default
   `video`). Enter the name of an existing shared folder, without a path. The Next button stays
   disabled for a name DSM would not accept as a shared folder name; a name that passes but
   **does not exist is refused by the installer** with a message ("The shared folder … does not
   exist") – the installer never creates shared folders. Go back and correct the name.
6. The summary offers **Run after installation**; keep it ticked and click **Done**.

The exact order of the alert, the licence page and the wizard pages is not documented by
Synology and may differ slightly from the list above.

What the installation does:

- creates the DSM system user `<id>` that runs the server (the package never runs as root);
- grants that user **read-only** access to the shared folder you named and links it as
  `/var/packages/<id>/shares/<share>`;
- writes `/var/packages/<id>/var/config.yaml` with that folder as one movie library – **only
  if no `config.yaml` exists**. A configuration left by an earlier installation (uninstall with
  the data kept, see "Uninstall") is used unchanged, so the folder you just chose is **not
  added** to it; edit the file as described under "Configure" if you want it there;
- registers the firewall service "<display name>" for `8460/tcp`;
- starts the server. The first start scans the library; items appear in the web app while the
  scan runs.

### 3 Open the web app

In Package Center the package shows the display name, its icon, the version (see "Version
numbers"), a beta label for pre-releases and the status "Running". The **Open** button opens
the web app in a new tab; the address is `http://<NAS address>:8460/`, which you can also type
into a browser on any device in your local network. Which address DSM puts into the button
(host name or IP) is checked in the hardware test.

If the **DSM firewall** is enabled with a default "deny" policy, allow the service: Control
Panel > Security > Firewall > Edit Rules > Create > select from the list of built-in
applications > the display name (port `8460/tcp`). The service appears there because the
package registers its port with DSM. Port forwarding on the router is not offered for this
service on purpose (there are no user accounts yet).

## Configure

### Settings the package fixes

The start script sets these through the server's environment, which takes precedence over
`config.yaml` (ADR 0018), so they cannot be changed in the file:

| Setting | Value | Why |
|---|---|---|
| config directory, `config.yaml` | `/var/packages/<id>/var` (`<PREFIX>_CONFIG_DIR`) | DSM's `var` directory survives upgrades and uninstalls |
| database, instance lock | `/var/packages/<id>/var/data` | same |
| cache | `/var/packages/<id>/var/cache` | same |
| transcode output | `/var/packages/<id>/tmp/transcode` | DSM's `tmp` directory, on the data volume, wiped on upgrade and uninstall; the start script clears leftovers |
| `ffmpeg`, `ffprobe` | `/var/packages/<id>/target/bin/` | the bundled static build (see "FFmpeg") |
| `HOME` | `/var/packages/<id>/home` | DSM's home directory of the package user |

`<PREFIX>` is the environment prefix of the server, currently `MIVO`.

**Port 8460 is fixed in the package.** `server.port` in `config.yaml` is not supported here:
the firewall service and the "Open" button keep pointing at 8460. Everything else in
`config.yaml` – libraries, `transcode.max_sessions`, `server.lan_only`, log settings – works as
described in `../../docs/development.md` ("Libraries and tools").

### Add more shared folders (libraries)

Until the settings page of milestone 7 exists, more libraries need two steps:

1. **Grant read access.** Control Panel > Shared Folder > select the folder > Edit >
   Permissions. In the drop-down at the top choose **System internal user**, find the user
   `<id>` and tick **Read only**. Save. Without this step the library is listed but reported as
   unavailable (the scan ends with `root_unavailable`).
2. **Edit `config.yaml` over SSH.** The file belongs to the package user with mode `0640`, so
   an administrator edits it with `sudo`:

   ```sh
   sudo vi /var/packages/<id>/var/config.yaml
   ```

   Add one entry per folder. The installer wrote the file below (for the wizard share `video`;
   the three comment lines and the first library are exactly what it generates); the `path` of
   the first entry is DSM's link to the share chosen in the wizard. Further folders use their
   **volume path** `/volumeN/<share>` (Control Panel > Shared Folder shows the volume of each
   folder; `ls -d /volume*/<share>` over SSH prints the path):

   ```yaml
   # Settings of <display name>, written once at installation; later installs and upgrades keep this file.
   # More libraries: grant the package user read access to the shared folder in Control Panel,
   # add an entry below and restart the package.
   libraries:
     - name: "video"
       path: "/var/packages/<id>/shares/video"
       kind: "movies"
   ```

   Optional additions, written by hand – a second library and a lower transcode limit:

   ```yaml
     - name: "Series"
       path: "/volume1/series"
       kind: "shows"
   transcode:
     max_sessions: 1
   ```

   `kind` is `movies` (default) or `shows` (listed flat like movies until milestone 3). Do not
   list the same folder twice (once through the link and once through `/volumeN/…`), and do not
   nest one library inside another. Check afterwards that the owner did not change
   (`ls -l /var/packages/<id>/var/config.yaml` shows `<id> <id>`); if your editor replaced the
   file, restore it with `sudo chown <id>:<id> /var/packages/<id>/var/config.yaml`.
3. **Restart the package** in Package Center (stop it and start it again). Libraries are read
   at start; the new one is scanned right away.

The package has read access only; it never writes into shared folders (transcode output goes
to its own `tmp` directory).

### Local network only

There are **no user accounts until milestone 7**. The server refuses every request whose
connection does not come from the local network with `403` (`server.lan_only`, default `true`;
what counts as local is defined in ADR 0018,
[Notes M2-31](../../docs/adr/0018-configuration-and-os-paths.md#notes-m2-31), and explained
with examples in [`../docker/README.md`](../docker/README.md#local-network-only)). Because the
package runs directly on the NAS, it sees the real client addresses and the NAS's own IPv6
prefixes, so LAN clients over IPv4 and IPv6 work without further settings.

**DSM's reverse proxy and QuickConnect defeat this guard.** A rule in Control Panel > Login
Portal > Advanced > Reverse Proxy, an application portal alias, or anything else that forwards
through DSM's own web server presents every client to the server as loopback or as the NAS's own
address – every client then looks local, and the guard admits everyone, including the internet
if the rule is reachable from outside. QuickConnect is documented by Synology only for DSM and
Synology's own packages and is expected not to reach port 8460 at all. Until accounts exist:

- do not publish the package through DSM's reverse proxy, the application portal or
  QuickConnect;
- do not forward port 8460 on your router;
- do not set `server.lan_only: false` unless another layer controls who reaches the port.

### Logs

| What | Where | How to read it |
|---|---|---|
| server log (startup, scans, playback, errors) | `/var/packages/<id>/var/<id>.log`; rotated at start when larger than 10 MiB, one old file `<id>.log.1` | over SSH: `sudo tail -n 50 /var/packages/<id>/var/<id>.log` |
| output of the package scripts (install, start, stop; a few lines each) | `/var/log/packages/<id>.log` | `sudo tail /var/log/packages/<id>.log` |
| DSM's own record of package operations | `/var/log/synopkg.log` | `sudo tail /var/log/synopkg.log` |

Package Center shows no log of the package, as far as known (the hardware test looks for one).
When the server fails to start, Package Center's error dialog **should show** the reason and
the last 20 lines of the server log: the start script writes them to the file DSM reads for its
dialogs, which is confirmed for the install and uninstall scripts but not yet for `start`
(the hardware test checks it). The same text is always in `/var/log/packages/<id>.log`.

## Upgrade

Install the newer SPK through **Manual Install** exactly like the first time; Package Center
recognises it as an upgrade of the installed package (the wizard does not ask for the shared
folder again). DSM stops the package, replaces the program files and starts it again:

- **kept**: `/var/packages/<id>/var` – `config.yaml`, the database, the cache and the log – and
  the shared-folder grant and link;
- **wiped**: `/var/packages/<id>/tmp` (transcode output only).

The server migrates its database at the first start of the new version. **Downgrades are not
supported**: an older version does not understand a newer database. To go back, uninstall with
"delete data" (see below) and install the older version, which starts with an empty library.

### Version numbers

DSM requires a version of the form `<numbers>-<build>`, so Package Center shows a mapped
version; the web app and `<id> --version` show the real one:

| Server version | Package Center shows |
|---|---|
| `0.1.0-alpha.2` | `0.1.0-1020` |
| `0.1.0-alpha.2`, package revision 1 (file `…-r1-…spk`) | `0.1.0-1021` |
| `0.1.0-beta.1` | `0.1.0-2010` |
| `0.1.0-rc.3` | `0.1.0-3030` |
| `0.1.0` (final) | `0.1.0-9000` |
| local test build (`0.0.0-dev`) | `0.0.0-0000` |

The build number is stage (alpha 1, beta 2, rc 3, final 9) × 1000 + pre-release number × 10 +
package revision (ADR 0026 section 5).

## Uninstall

Package Center > the package > Uninstall. The uninstall wizard has one checkbox, **"Also delete
the database, settings and logs"**, unchecked by default:

- **unchecked**: DSM removes the program files and `tmp`; `/var/packages/<id>/var`
  (`config.yaml`, database, cache, log) stays on the volume. A later installation on the same
  volume continues with that configuration – and therefore **ignores the folder entered in the
  new install wizard** (see "Install", step 2).
- **checked**: the package also deletes the contents of `var` and `home`.

Your media files and shared folders are never touched. DSM removes the firewall service entry;
the read-only grant for the user `<id>` on the shared folder stays (DSM keeps it on purpose) –
remove it yourself in Control Panel > Shared Folder > Edit > Permissions if you want.

## Limits of the first package

- **Software transcoding only.** No hardware acceleration (`/dev/dri` is not used), matching
  the first release; transcodes of 1080p content need a capable CPU. On **armv7** models expect
  Direct Play and Remux only; a transcode will not keep up in real time.
- **RAM**: 1 GB is the recommended minimum of the plan. Many armv7 models have 512 MB and the
  smallest armv8 model (DS119j) has 256 MB; the package installs there but transcoding is likely
  to fail or swap.
- **armv8 and armv7 are tested under emulation only** (QEMU with the old-kernel syscall surface
  and baseline CPU models, SPK-11); no 32-bit or 64-bit ARM NAS was available. x86_64 is the
  only family tested on real hardware (SPK-12). Reports from ARM models are welcome.
- **No accounts, no HTTPS**: local network only (above); HTTPS and certificates come with
  milestone 7.
- **Libraries are configured in `config.yaml`** over SSH; no settings page yet.
- **Fixed port 8460**; the package cannot be moved to another volume from Package Center
  (`support_move` is off).
- Everything else the release cannot do yet is listed in the repository `README.md`.

## FFmpeg

The package bundles `ffmpeg` and `ffprobe` in `/var/packages/<id>/target/bin/` and runs them as
separate processes (ADR 0015). They are an **own static build** for each family (recipe in
[`ffmpeg/build/`](ffmpeg/build/README.md), task SPK-15; measurements and the comparison with
prebuilt builds in [`ffmpeg/README.md`](ffmpeg/README.md)):

- FFmpeg 9.0.2 (recipe revision 1), built against musl, fully static, no dynamic loader, no
  ELF ABI note – it runs on the oldest DSM 7.2 kernels (3.10 on x86_64 and armv7, 4.4 on armv8);
- **GPL version 3 or later** (`--enable-gpl --enable-version3`) with libx264 (H.264 encoder),
  FFmpeg's native AAC encoder, dav1d (AV1 decoder) and zlib; **no** libfdk-aac and nothing
  non-free, so the binaries are redistributable;
- reduced to what the server uses: the demuxers of the server's format whitelist, all decoders,
  the HLS and MP4 muxers, the libx264 and AAC encoders, file access only (no network protocols);
- baselines x86-64 v1, ARMv8-A and ARMv7-A with VFPv3-D16 hard-float; NEON and newer x86
  instruction sets only through run-time CPU detection, so the same binary runs on Armada 38x
  (whose NEON support is unconfirmed) and on Alpine.

**Licence and source.** The licence texts and a source notice are installed next to the
binaries in `/var/packages/<id>/target/share/licenses/ffmpeg/`: `COPYING.GPLv3`, `LICENSE.md`
(FFmpeg), the licences of x264, dav1d, zlib, musl and libgcc, and `SOURCE.txt` with the FFmpeg
version, the configure line, the SHA-256 of every pinned source and of the binaries. The
**complete corresponding source** – FFmpeg and every linked library at the pinned versions plus
the build recipe – is the archive `ffmpeg-<version>-static<revision>-source.tar` (about
170 MB) attached to the same GitHub Release as the SPKs, with its checksum in `SHA256SUMS`
(GPL-3.0 section 6). `ffmpeg -version` prints the configure line of the build.

> **Status (SPK-16).** The text above is the target state decided by the owner (ADR 0026 Notes).
> The switch of the build pipeline from the interim prebuilt FFmpeg of SPK-02 to this own build
> is task SPK-16, which runs in parallel to this documentation. Until it lands, the SPKs
> `build.ps1`/`build.sh` produce are **test builds** with an interim FFmpeg (`dist/NOT-FOR-RELEASE.txt`
> is written, `SOURCE.txt` says `Status: interim`, and a release build refuses them): the
> x86_64 and armv8 test builds carry jellyfin-ffmpeg, which **links libfdk-aac** (non-free, not
> redistributable) and needs a newer kernel than DSM 7.2 has; the armv7 test build is
> johnvansickle's **FFmpeg 7.0.2**, below the 7.1 minimum of ADR 0026, without corresponding
> source. Never hand such a test build to anyone but the owner's device. SPK-16 has
> to confirm: the exact file name of the source archive in the release and its `SHA256SUMS`
> line, the licence file names staged into `share/licenses/ffmpeg/`, and the FFmpeg version line
> shown by `ffmpeg -version` on each family.

## Troubleshooting

| Symptom | Cause and what to do |
|---|---|
| Package Center reports a **port conflict** during installation or upgrade | DSM checks whether port 8460 is free (`checkport`). Something else listens on 8460 – typically the Docker container of the server. Stop it and try again. (Whether DSM counts the package's own firewall entry as a conflict on upgrade is unconfirmed; the hardware test checks it.) |
| The package starts but stops at once; a dialog **should say** "**stopped during start**" with log lines (if Package Center shows no text, read `/var/log/packages/<id>.log`) | Read the lines: `address already in use` means port 8460 is taken (see above); other causes are in `/var/packages/<id>/var/<id>.log`. |
| Package Center shows **Running**, but the web app does not answer yet | A slow first start or a database migration: the start script counts a server that is alive but not yet healthy after 30 s as started and writes "not healthy after 30 s but keeps running" to `/var/log/packages/<id>.log` (Package Center shows no text). Wait a moment and refresh; `/var/packages/<id>/var/<id>.log` shows `http server listening` when the start is complete. |
| Status "**broken**" / "**damaged**" in Package Center | The server binary or a package file is missing or not executable (status exit 150), or a post-install script failed; `/var/log/packages/<id>.log` names the file. Uninstall with "delete data" unchecked and install the same SPK again: `var` (settings, database) stays. |
| The installer refuses the shared folder: "**does not exist**" | A typo, or the folder is not a shared folder (sub-folders of a share are not accepted). Enter the name exactly as Control Panel > Shared Folder shows it, without a path. The installer never creates shared folders, because a mistyped name would otherwise become a new, empty share. |
| The library is listed but **unavailable**; the log says `root_unavailable` or **permission denied** | The package user cannot read the folder. For the wizard folder: `ls -l /var/packages/<id>/shares/` must show the link and the folder must grant "Read only" to the system internal user `<id>`; for folders added by hand: grant the permission (see "Add more shared folders") and check the `/volumeN/<share>` path. Restart the package afterwards. |
| The "**Open**" button is missing or opens the wrong address | Type `http://<NAS address>:8460/` into the browser. Please report the model and DSM version. |
| `403` **from another device** | The device is not in the local network as the server sees it (VPN, guest network, a proxy in between). See "Local network only". |
| The wizard or texts appear in **English on a German DSM** | Please report it; the package ships German wizard texts and messages. |

## Build

Contributors build the three SPKs on the Windows dev machine or any Linux host with nothing but
Docker. The procedure is ADR 0026 section 2: a normal Docker build produces the binaries and
stages one toolkit project per family, then Synology's official Package Toolkit `pkgscripts-ng`
packs each project in a container.

### Prerequisites

- **Docker Desktop** (tested with 4.97.0, Engine 29.9.2) or Docker with BuildKit on Linux.
  The toolkit container needs `--cap-add SYS_ADMIN` (see "Privileges"); Docker Desktop allows
  that by default.
- **PowerShell 7.2 or newer** for `build.ps1`, or **bash 4 or newer** for `build.sh` (Git Bash
  on Windows works; the script sets `MSYS_NO_PATHCONV` itself).
- **Disk space**: about 24 GB for the named volume `spk-toolkit` (three platform
  environments, 3.4 GiB of tarballs) plus the BuildKit cache of the web, Go and FFmpeg stages.
- **Network** for the first run: base images, Go modules and pnpm packages, the `pkgscripts-ng`
  clone and Synology's platform tarballs (about 2 GiB for x86_64, 0.7 GiB per ARM family).
  The owner approved these downloads; later runs are served from the caches.
- Nothing else: no Go, Node or FFmpeg on the host.

### Usage

From the repository root:

```powershell
# PowerShell (Windows)
pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.2
```

```sh
# bash (Linux, macOS, Git Bash)
bash packaging/synology/build.sh 0.1.0-alpha.2
```

| `build.ps1` | `build.sh` | Meaning |
|---|---|---|
| `-Version <v>` (also positional) | `<v>` (positional) | server version, `<major>.<minor>.<patch>[-alpha.N\|-beta.N\|-rc.N]` or `0.0.0-dev` (default); checked against spkmeta's scheme before anything runs |
| `-Release` | `--release` | release build (also `SPK_BUILD_MODE=release`; the default on CI hosts, i.e. when `CI` is set to anything but empty, `false` or `0`): refuses interim or non-free FFmpeg builds, spkstage warnings, the default version and, with git, a dirty working tree |
| `-TestBuild` | `--test` | local test build (also `SPK_BUILD_MODE=test`; the default elsewhere): packs an interim FFmpeg with a loud warning and writes `dist/NOT-FOR-RELEASE.txt` |
| `-Revision <sha>` | `--revision <sha>` | git commit recorded in `dist/stage/build-info.txt` (default `git rev-parse HEAD`, empty without git) |
| `-PackageRevision <0-9>` | `--package-revision <0-9>` | packaging-only revision of the same server version (default 0); N ≥ 1 names the files `…-r<N>-<family>.spk` |
| `-SourceDateEpoch <s>` | `--source-date-epoch <s>` | file time of the staged projects in seconds since 1970 (default `SOURCE_DATE_EPOCH`, else the commit time of HEAD, else 0) |
| `-Privilege <mode>` | `--privilege <mode>` | privileges of the toolkit container (also `SPK_PRIVILEGE`): `sys-admin` (default), `sys-admin-apparmor`, `privileged` – see "Privileges" |
| `-?` | `-h`, `--help` | help |

Environment: `SPK_TOOLKIT_VOLUME` (default `spk-toolkit`) and `SPK_TOOLKIT_IMAGE` (default
`spk-toolkit:dev`) select the toolkit volume and image; `NODE_IMAGE`, `GO_IMAGE` and
`FFMPEG_FETCH_IMAGE` are passed to the Dockerfile when set (same names and defaults as
`../docker/Dockerfile`); `BUILDKIT_PROGRESS=plain` shows every log line of step 1. An option
given with an empty value is refused; an empty environment variable counts as unset.

A warm build takes about 80 s on the dev machine (step 1 from the BuildKit cache, 15–35 s of
packing per family); the first build adds the environment downloads and deployments (about
10 minutes for all three families – 7 to 10 minutes were measured, the speed of Synology's CDN
varies; `toolkit/README.md`, "Caching") and the cold web, Go and FFmpeg stages.

### The two steps

1. **Prebuild and stage** – `docker build -f packaging/synology/Dockerfile … --output type=local,dest=packaging/synology/dist/stage .`,
   unprivileged. Stages: the web app (pnpm, as in `../docker/Dockerfile`), the Go server
   cross-compiled statically for the three families (`CGO_ENABLED=0 -tags webui -trimpath`,
   version via `-ldflags -X`; armv7 with `GOARM=7`), FFmpeg per family, and `spkstage`
   (`cmd/spkstage`, SPK-14), which renders the templates of `package/` with the brand values,
   validates the binaries (ELF class and machine of the family, static linking) and writes one
   toolkit project per family. The output is `dist/stage/<family>/` plus `build-info.txt`
   (version, revision, package revision, epoch, mode, images, FFmpeg status per family).
2. **Pack with the toolkit** – for each family
   `docker run --rm --cap-add SYS_ADMIN -v spk-toolkit:/toolkit -v <stage>/<family>:/project:ro -v <dist>:/output spk-toolkit:dev <family>`
   (image from `toolkit/Dockerfile`, entrypoint `toolkit/run.sh`). The first run per family
   clones `pkgscripts-ng`, downloads the platform tarballs and deploys the environment
   (`EnvDeploy -v 7.2 -p <platform> -D`); every run then calls
   `PkgCreate.py -v 7.2 -p <platform> -c <id>`, which runs `SynoBuildConf/build` (checks the
   prebuilt files), `SynoBuildConf/install` (copies them into the package layout, writes INFO
   with `INFO.sh` and packs `package.tgz` and the SPK with the toolkit helpers) and collects the
   SPK. The families run one after another: the volume serialises runs with a lock. The build
   script renames the toolkit's `<id>-<family>-<dsm version>.spk` to `<id>-<version>-<family>.spk`
   and writes `SHA256SUMS`. The first error stops the build.

Platforms per family: x86_64 → `v1000`, armv8 → `rtd1619b`, armv7 → `alpine`; details and the
measured download sizes and times in [`toolkit/README.md`](toolkit/README.md).

### Inputs and outputs

| Input | Where |
|---|---|
| server sources | `go.mod`, `go.sum`, `cmd/`, `internal/`, `web/` (without build output), `LICENSE` – the build context is limited by `packaging/synology/Dockerfile.dockerignore` |
| package templates, scripts, wizard, resources, icons | `packaging/synology/package/`, `packaging/synology/internal/spkicon` (icons are generated, no image files) |
| brand values | `internal/platform/brand` through `packaging/synology/internal/spkmeta` (rendered into `build.env` and `scripts/package.env`) |
| FFmpeg | `packaging/synology/ffmpeg/sources.lock` + `packaging/synology/ffmpeg/fetch.sh` today; the recipe `packaging/synology/ffmpeg/build/` after SPK-16 |
| toolkit pin | `PKGSCRIPTS_COMMIT` in `packaging/synology/toolkit/run.sh`; platform tarball checksums in `packaging/synology/toolkit/tarballs.sha256` |

| Output (all under `packaging/synology/dist/`, git-ignored) | Content |
|---|---|
| `<id>-<version>-<family>.spk` (three files) | the packages |
| `SHA256SUMS` | `sha256sum` format, sorted by name, LF |
| `stage/<family>/`, `stage/build-info.txt` | the staged toolkit projects of step 1 |
| `NOT-FOR-RELEASE.txt` | only for test builds: which families carry a non-releasable FFmpeg |
| the FFmpeg corresponding source archive and its `SHA256SUMS` line | **target state** – not produced today; SPK-16 adds it |

SPKs are **not byte-reproducible** across builds: the toolkit writes `create_time` into INFO and
stamps the tar members with the packing time. Content is reproducible – the same members, modes
and file contents inside `package.tgz` – and `SOURCE_DATE_EPOCH` fixes the times of the staged
projects. Release assets are therefore the exact files that were tested, not a rebuild.

### The toolkit volume

The named volume **`spk-toolkit`** holds the `pkgscripts-ng` clone, the downloaded platform
tarballs (`toolkit_tarballs/`, 3.4 GiB) and the three deployed environments (about 7 GB each);
about **24 GB** in total. It lives in Docker Desktop's Linux VM on purpose: the environments
contain device nodes, symlinks, hard links, Unix modes and case-colliding paths that NTFS behind
a bind mount cannot hold. A second run reuses it ("environment deployed, EnvDeploy skipped").

- Reset everything: `docker volume rm spk-toolkit` (the next build downloads and deploys again,
  about 10 minutes for all families; CDN speed varies).
- Keep the environments but drop the tarballs (saves 3.4 GiB; a later redeploy downloads them
  again): `docker run --rm -v spk-toolkit:/toolkit alpine:3 rm -rf /toolkit/toolkit_tarballs`.
- Use another volume, for example for a throw-away run: `$env:SPK_TOOLKIT_VOLUME = 'spk-try'`
  (bash: `SPK_TOOLKIT_VOLUME=spk-try bash packaging/synology/build.sh …`).

`run.sh` verifies the SHA-256 of the platform tarballs against `toolkit/tarballs.sha256` before
every deploy (trust on first use of the 2026-10-31 downloads; Synology publishes no checksums)
and deletes a mismatching file so that the next run downloads it again.

### Privileges

The toolkit `chroot`s into the platform environment and mounts `/proc` inside it, which needs
`CAP_SYS_ADMIN`; Docker's default capability set fails with `mount: … permission denied`. The
build scripts run the toolkit container with:

| `--privilege` / `-Privilege` / `SPK_PRIVILEGE` | `docker run` options | When |
|---|---|---|
| `sys-admin` (**default**) | `--cap-add SYS_ADMIN` | Docker Desktop on Windows (WSL2): verified for all three families; Docker Desktop on macOS is expected to behave the same (its VM runs no AppArmor) but was not measured. Keeps Docker's seccomp profile, device cgroup and the other dropped capabilities. |
| `sys-admin-apparmor` | `--cap-add SYS_ADMIN --security-opt apparmor=unconfined` | Linux hosts with AppArmor (Ubuntu, GitHub-hosted runners), where Docker's default AppArmor profile denies `mount` even with the capability. **Not verified yet** (no such host was available); the CI task confirms it. |
| `privileged` | `--privileged` | fallback only, for hosts where the narrow set fails. |

Measurements and the reasoning are in `toolkit/README.md`, "Privileges".

### FFmpeg in the build

Today the `ffmpeg` stage of `Dockerfile` runs `ffmpeg/fetch.sh` for each family with the
**interim** pins of `ffmpeg/sources.lock`: jellyfin-ffmpeg 8.1.3 for x86_64 and armv8, which
**links libfdk-aac** (non-free, not redistributable; `sources.lock` marks its licence
`+nonfree`), is glibc-dynamic and needs Linux ≥ 4.18; and johnvansickle's **FFmpeg 7.0.2** for
armv7, below the 7.1 minimum and without corresponding source. The layer stays cached until
`fetch.sh` or `sources.lock` change, so a second build downloads nothing. These builds miss
criteria of ADR 0026 section 4 and are for pipeline work and emulated tests only; test builds
say so with a banner and `NOT-FOR-RELEASE.txt`, and `--release` refuses them.

After **SPK-16** the stage builds FFmpeg with the recipe in [`ffmpeg/build/`](ffmpeg/build/README.md)
(SPK-15): one Dockerfile that cross-compiles musl toolchains, x264, dav1d, zlib and FFmpeg for
the three families, writes `SOURCE.txt` with `Status: selected`, and produces the corresponding
source archive. Its cache: downloads live in the BuildKit cache mount `ffmpeg-static-downloads`
(keyed by checksum, so a changed pin downloads one file), and the toolchains – 80–90 % of a
cold build of 10–14 minutes for all three families – stay in the layer cache across FFmpeg,
library and configure changes. A warm build of the recipe takes seconds. SPK-16 documents the
exact wiring (how the stage is invoked, how the source archive reaches `dist/`).

### The pinned `pkgscripts-ng` commit

`toolkit/run.sh` clones branch `DSM7.2` of <https://github.com/SynologyOpenSource/pkgscripts-ng>
and checks out the commit in `PKGSCRIPTS_COMMIT` (`e4a8d54209a31cef21b7636ad58b1b0e86b5b201`,
"Release for DSM7.2.2", 2025-05-28), after verifying that it is an ancestor of
`origin/DSM7.2`; local changes in the clone are discarded. The deploy marker of each environment
records the commit, so an environment deployed by another commit is deployed again. The
platform environments are versioned separately: the 7.2 tarballs of 2024-06-06 report DSM 7.2.2
build 72723 (INFO `toolkit_version`).

To update the pin: set `PKGSCRIPTS_COMMIT` in `toolkit/run.sh` to a commit of `DSM7.2` (the
repository's default branch is already `DSM7.4`; DSM 7.3/7.4 are out of scope), run
`bash packaging/synology/toolkit/test.sh` (it redeploys from the cached tarballs and packs all
families), check the helper names and line numbers quoted in `toolkit/README.md` and
`DSM-NOTES.md`, and update them. If Synology republishes a platform tarball, `run.sh` refuses it
until its line in `toolkit/tarballs.sha256` is updated on purpose.

### Toolkit pitfalls

- **`SynoBuildConf/depends` must be pure ASCII.** The chroot's Python 3.4 reads it with an
  ASCII locale; one non-ASCII byte silently empties the dependency list, and the build ends with
  "0 projects" and a `ConfigNotFoundError` in the collect stage. `run.sh` refuses non-ASCII
  bytes in `SynoBuildConf/` and CRLF line endings in the project files before it calls the
  toolkit.
- **Success is decided by grepping the logs.** `SynoBuildConf/build` and `install` are sourced
  by bash and their exit status is ignored; the toolkit fails a stage when its log contains
  `Error`, `error:`, `fatal error`, `No such file or directory` and a few more. Our scripts print
  `Error: <reason>` on failure and never print those words otherwise; the same rule applies to
  the package scripts, whose output readers of DSM's package log take for failures.
- **No signing option.** The guide still lists `--no-sign`, but `PkgCreate.py` at the pinned
  commit rejects it; signing was removed with DSM 7.0. Nothing to pass; the SPK carries no
  signature member.
- `[WARNING] /tmp/_install is empty!` is printed twice per family by the toolkit; it is harmless.
- `package.tgz` is **xz-compressed** despite its name (`tar cJf`); the outer SPK is an
  uncompressed tar.
- Docker Desktop shows every file of a Windows bind mount as mode `0777`; `SynoBuildConf/install`
  sets every mode explicitly, so modes in the SPK do not depend on the host.
- `test.sh` and the build scripts share the volume; runs are serialised by a lock, so a build
  waits for a test run on the same volume.

### Tests

| What | Command (repository root; Git Bash: `MSYS_NO_PATHCONV=1` and `"$(pwd -W)"` for `${PWD}`) |
|---|---|
| package scripts (bats, four shells) and templates | `docker build -t spk-script-test:dev packaging/synology/test` then `docker run --rm -v "${PWD}:/code" -w /code spk-script-test:dev packaging/synology/test/scripts` |
| build scripts (`build.sh`, `build.ps1` parity, fake Docker) | `docker build -t spk-build-test:dev packaging/synology/test/build` then `docker run --rm -v "${PWD}:/code" -w /code spk-build-test:dev packaging/synology/test/build` |
| shellcheck of the package scripts | `docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/package/scripts/start-stop-status packaging/synology/package/scripts/common.sh packaging/synology/package/scripts/preinst packaging/synology/package/scripts/postinst packaging/synology/package/scripts/preuninst packaging/synology/package/scripts/postuninst packaging/synology/package/scripts/preupgrade packaging/synology/package/scripts/postupgrade` |
| shellcheck of the build and test scripts | `docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/build.sh packaging/synology/ffmpeg/fetch.sh packaging/synology/toolkit/run.sh packaging/synology/toolkit/test.sh packaging/synology/test/build/build-scripts.bats packaging/synology/test/build/fake-docker.bash` |
| toolkit (lint, build-stage checks, pack and verify all families) | `bash packaging/synology/toolkit/test.sh` (needs the volume; `lint` or a family name narrows it) |
| Go packages (`spkmeta`, `spkicon`, `spkstage`) | `go test ./packaging/...` (part of `go test ./...`) |

Details: `test/README.md` (script tests, the fake server, shells), `toolkit/README.md`
(toolkit tests), `ffmpeg/build/README.md` ("Gates" and "Verification under emulation").

### Inspecting an SPK

An SPK is an uncompressed tar with `INFO`, `package.tgz` (xz), `scripts/`, `conf/`,
`WIZARD_UIFILES/`, `LICENSE` and the two icons. To list it and read INFO and the payload
without installing it (GNU tar with xz; `tar.exe` of Windows may read it too):

```powershell
docker run --rm -e DEBIAN_FRONTEND=noninteractive -v "${PWD}/packaging/synology/dist:/dist:ro" debian:stable-slim sh -c 'apt-get -qq update >/dev/null && apt-get -qq install -y xz-utils >/dev/null && cd /dist && f=mivo-0.1.0-alpha.2-x86_64.spk && tar -tvf "$f" && tar -xOf "$f" INFO && tar -xOf "$f" package.tgz | tar -tJvf -'
```

Task SPK-10 adds `test/inspect/inspect-spk.sh [--release] <file.spk> <family>`, which checks
members, modes, INFO, the ELF properties of the binaries and the FFmpeg status against ADR 0026
and fails on an interim FFmpeg with `--release`; releases and the hardware test run it on the
exact files.
