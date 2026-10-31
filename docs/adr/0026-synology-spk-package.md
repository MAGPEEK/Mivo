# 0026 Native Synology DSM package (SPK)

- Status: Accepted (2026-10-31, project owner; OD-SPK-1 … OD-SPK-9 decided, see the end)
- Date: 2026-10-31
- Refines: ADR 0014 (pulls the native Synology package forward; Docker stays the primary NAS
  distribution, QPKG stays "later"), ADR 0015 (FFmpeg bundling on a target without Debian
  packages), ADR 0018 (path mapping on DSM)
- Source: decisions of the project owner on 2026-10-31; tasks `docs/tasks/SPK-*.md`

## Context

**What the package is**: the SPK installs the **media server** on the NAS as a DSM service –
the `mivo` server binary with the embedded web app, the library scanner and the bundled
FFmpeg/ffprobe for probing, remux and transcode, listening on port 8460. Users reach it with a
browser (and later the TV app) from other devices. It is **not** a client or player app for DSM;
there is no DSM desktop UI beyond Package Center's start/stop and "Open" button.

`docs/plan.md` and ADR 0014 put native NAS packages ("SPK (spksrc) and QPKG (QDK)") after the
Docker image. On 2026-10-31 the project owner decided:

- start the Synology package **now, in parallel to milestone 2**; the first SPK ships with
  **0.1.0-alpha.2** (the first release that scans and plays);
- bundle a **static FFmpeg build per architecture**, as the Docker image bundles FFmpeg;
- architectures **x86_64, armv8 (aarch64) and armv7**;
- build with **Synology's official Package Toolkit** `pkgscripts-ng` (OD-SPK-2 B);
- a dedicated implementer agent `mivo-synology` builds it (`.claude/agents/mivo-synology.md`).

Why a native package besides Docker: Synology's Container Manager is only offered on part of the
model range (no 32-bit ARM model has it; SPK-01 verifies the list), and a native package
integrates with Package Center (start/stop, status, firewall service, "Open" button,
shared-folder permissions) without any Docker knowledge.

Facts taken from Synology's DSM 7.2.2 Package Developer Guide
(<https://help.synology.com/developer-guide/>) and the Package Toolkit `pkgscripts-ng`
(<https://github.com/SynologyOpenSource/pkgscripts-ng>, branch `DSM7.2`):

- The toolkit deploys a per-platform chroot (`EnvDeploy`) and builds and packs with
  `PkgCreate.py`; it needs a Linux host. A project lives in `source/<project>/` with
  `SynoBuildConf/` (`depends`, `build`, `install`), `INFO.sh` and the package files; helpers in
  `include/pkg_util.sh` create `package.tgz`, INFO and the SPK.
- An SPK holds `INFO` (required: `package`, `version` as `[feature]-[build]`, `os_min_ver`,
  `description`, `arch`, `maintainer`; optional among others `displayname`,
  `adminport`/`adminurl`/`adminprotocol`, `dsmuidir`, `checkport`, `startable`, `ctl_stop`,
  `install_dep_packages`, `os_max_ver`, `support_move`, `silent_install`), `package.tgz`
  (extracted to `/volumeN/@appstore/<pkg>`, reachable as `/var/packages/<pkg>/target`),
  `scripts/` (`preinst`, `postinst`, `preuninst`, `postuninst`, `preupgrade`, `postupgrade`,
  `start-stop-status`), `conf/privilege` (required since DSM 7, `"run-as": "package"`),
  `conf/resource` (for example `data-share` for shared folders with ro/rw grants and
  `port-config` with a `.sc` protocol file registered in the firewall), the icons
  `PACKAGE_ICON.PNG` and `PACKAGE_ICON_256.PNG`, `WIZARD_UIFILES/` and `LICENSE`.
- `start-stop-status` exit codes: 0 running, 1/2 dead, 3 not running, 4 unknown, 150 broken. It
  also runs at boot.
- Package directories: `var` (`$SYNOPKG_PKGVAR`, permanent), `etc`, `home`, `tmp`
  (`$SYNOPKG_PKGTMP`, wiped on upgrade and uninstall) and `target`.
- Package signing is deprecated since DSM 7; unsigned packages are installed with Package
  Center > Manual Install.
- `arch` accepts the families `x86_64`, `armv8`, `armv7` and platform names (`apollolake`,
  `geminilake`, `v1000`, `rtd1619b`, `armada37xx`, `alpine`, …). The guide asks not to pack
  binaries of several platforms into one SPK.

Not confirmed from the guide and therefore verified before the tasks rely on them (SPK-01 for
DSM behaviour, SPK-04 for the toolkit): the SPK container format, icon pixel sizes, trust-level
behaviour for unsigned installs, the `.sc` file format, the script environment variables, how
DSM compares versions, what DSM does with `var` on uninstall, which file Package Center shows as
the package log, the privileges the toolkit needs in a container, and which `arch` value
`PkgCreate.py` writes per platform. Statements below that depend on them are marked
*(verify, SPK-01)* or *(verify, SPK-04)*.

## Decision

### 1 Scope and timing

- The SPK packages and runs the **server** (section "What the package is" above) as a DSM
  service under Package Center; no task of the track builds a client or player.
- A separate track **SPK** (tasks under `docs/tasks/SPK-*.md`, overview
  `docs/tasks/SPK-00-overview.md`) runs in parallel to milestone 2. It changes **no server
  code**: the package wraps the same binary as the Docker image (`-tags webui`, version stamped
  through `-ldflags -X`, ADR 0020). Work lives under `packaging/synology/**`.
- The SPKs are attached to the GitHub Release `v0.1.0-alpha.2`. **The Docker release goes out on
  schedule; the SPKs are attached to the same GitHub Release when they are ready** (OD-SPK-1 A).
- Out of scope for the first SPK: hardware transcoding (software only, matching the owner's AMD
  Synology without iGPU), HTTPS and certificates (milestone 7b), a DSM desktop app or single
  sign-on (`dsmuidir`), reverse-proxy integration, `PUID`-style user mapping, migration from a
  Docker install, package signing, DSM 6 and DSM 7.0/7.1, QPKG.

### 2 Build approach: the official Package Toolkit (OD-SPK-2 B)

- **Decision**: SPKs are built and packed with **`pkgscripts-ng`** (branch `DSM7.2`, pinned to a
  commit): `EnvDeploy -v 7.2 -p <platform>` deploys the platform environment,
  `PkgCreate.py -v 7.2 -p <platform> … <project>` builds the project inside the chroot and packs
  the SPK. Signing is off (the toolkit's no-sign option; signing is deprecated since DSM 7; exact
  flag: verify, SPK-04).
- **The toolkit runs in a Linux container on the Windows dev machine** (Docker Desktop). The
  chroot needs root and mounts inside the container, so the container runs `--privileged` unless
  SPK-04 finds a narrower set of capabilities that works. The toolkit directory with the deployed
  platform environments lives in a **named Docker volume** (Linux file system), never in a
  Windows bind mount: the chroot needs device nodes, symlinks and Unix permissions that NTFS
  cannot hold. The environments are downloaded once and reused.
- **One platform environment per family**, a representative DSM 7.2 platform (candidates:
  x86_64 → `v1000` or `geminilake`, armv8 → `rtd1619b` or `armada37xx`, armv7 → `alpine` or
  `armada38x`). The SPK's `arch` must be the **family value** (`x86_64`, `armv8`, `armv7`), so
  that one SPK installs on every model of the family. SPK-04 verifies which value
  `PkgCreate.py`/`pkg_util.sh` writes for each platform; if the helper writes the platform name,
  `INFO.sh` sets the family value explicitly *(verify, SPK-04)*.
- **Binaries are prebuilt outside the chroot and only packed by the toolkit**: the static Go
  binary (`CGO_ENABLED=0`, cross-compiled in a normal Docker build stage) and the bundled FFmpeg
  (downloaded and verified per family) are placed into the staged project. The chroot's
  cross toolchains are not used.
  - `SynoBuildConf/depends`: minimal, DSM 7.2 environment only.
  - `SynoBuildConf/build`: compiles nothing; it only checks that the prebuilt files of the family
    are present and executable.
  - `SynoBuildConf/install`: copies the prebuilt files into the package layout and calls the
    toolkit's helpers (`pkg_make_package`, `pkg_make_spk` in `include/pkg_util.sh`; names verify,
    SPK-04).
  - `INFO.sh`: sources a generated `build.env` (brand values, DSM version, beta flag) and writes
    INFO with the toolkit's helper (`pkg_dump_info`); keys the helper does not know are appended
    by `INFO.sh` itself *(verify, SPK-04)*.
- **Staging** (`packaging/synology/cmd/spkstage`, Go, standard library only): renders the
  templates under `packaging/synology/package/` with brand values from `internal/platform/brand`,
  writes `build.env` and `package.env`, the generated icons, the repository `LICENSE`, and the
  prebuilt binaries plus FFmpeg notices per family into `source/<project>/`. Go writes the file
  modes explicitly, so the staged tree does not depend on NTFS.
- **CI-ready**: two steps that need nothing but Docker, wrapped by `packaging/synology/build.ps1`
  (Windows) and `build.sh` (bash, CI): a normal `docker build --output` for web app, Go binaries,
  FFmpeg and staging; then the privileged toolkit container that runs `PkgCreate.py` per family.
  GitHub-hosted Linux runners allow privileged containers; the workflow and an environment cache
  come later.
- Package scripts are POSIX `sh`, checked with `shellcheck` and tested in a Linux container with
  a fake server binary.
- **Reproducibility**: the toolkit's packing uses `tar` with file times and its own ordering, so
  byte-identical SPKs are not guaranteed. The goal is content reproducibility (same members,
  modes and member checksums); staging sets one modification time (`SOURCE_DATE_EPOCH`). SPK-10
  measures what the toolkit actually reproduces and records the limits.

**Rejected alternatives**

| Alternative | Why rejected |
|---|---|
| A: own assembler (Go writes INFO, `package.tgz` and the SPK tar; no chroot) – the architect's original recommendation | The owner prefers the official toolchain: its packing and INFO helpers are what Synology documents and maintains, so format changes in DSM reach us through the toolkit instead of our own archive code; less custom packaging code to maintain. A's advantages (no privileged container, no GB downloads, byte-reproducible) are given up knowingly. |
| C: SynoCommunity `spksrc` (the plan's original idea) | A community framework with its own conventions (service setup, users, Makefiles) aimed at the SynoCommunity package source; its cross toolchains are useless for a prebuilt static Go binary. A later listing in SynoCommunity would need it and is a separate decision. |

### 3 Architectures

| Family (`arch`) | Go target | Toolkit platform (candidate, SPK-04 decides) | Docker platform for emulated tests |
|---|---|---|---|
| `x86_64` | `GOARCH=amd64`, default `GOAMD64=v1` | `v1000` or `geminilake` | `linux/amd64` |
| `armv8` | `GOARCH=arm64` | `rtd1619b` or `armada37xx` | `linux/arm64` |
| `armv7` | `GOARCH=arm`, `GOARM=7` | `alpine` or `armada38x` | `linux/arm/v7` |

- One SPK per family, as the guide asks. SPK-01 verifies which DSM 7.2 platforms each family
  value covers, their oldest kernel and, for armv7, the floating-point and NEON features.
- No test suite runs on 32-bit ARM. The emulated smoke test (SPK-11) is mandatory for every
  family before its SPK is published. **armv7 is published only if its emulated smoke run
  passes, labelled "tested under emulation only"** (OD-SPK-4 A; consistent with OD-9 of
  milestone 2).
- The plan's minimum of 1 GB RAM stays. Many armv7 models have 512 MB; the package does not
  refuse them, the docs state the minimum, and software transcoding on armv7 is not expected to
  run in real time (Direct Play and Remux only, as the plan says).

### 4 FFmpeg (OD-SPK-3 A)

- `ffmpeg` and `ffprobe` are bundled per family as **static GPL builds** in `target/bin/` and run
  as external processes (ADR 0015 unchanged). The server finds them through
  `<PREFIX>_TOOLS_FFMPEG` and `<PREFIX>_TOOLS_FFPROBE`.
- Selection criteria (SPK-02): fully static; runs on the oldest DSM 7.2 kernel of the family
  (ELF ABI note); GPL variant with `libx264` and the native AAC encoder; FFmpeg **7.1 or newer**
  (the version the Docker image and the expectations of ADR 0025 cover); baseline instruction
  set (x86-64 v1; armv7 hard-float, NEON only if every DSM 7.2 armv7 platform has it); an
  immutable, versioned download URL pinned by SHA-256; licence text and corresponding source
  available.
- **One prebuilt static GPL source for all three families if possible**, in this order:
  jellyfin-ffmpeg portable builds (the build ADR 0015 names), BtbN FFmpeg-Builds release
  branches, johnvansickle static release builds. A family without a fitting prebuilt gets an
  **own static build from source in Docker as a follow-up task**. Known before SPK-02:
  jellyfin-ffmpeg portable and BtbN publish `linux64` and `linuxarm64`; a 32-bit ARM build is not
  confirmed for either; johnvansickle publishes `armhf`. ADR 0015's fallback "Debian's package"
  does not exist on DSM, so this order replaces it for the SPK.
- The choice is pinned in `packaging/synology/ffmpeg/sources.lock` and documented in
  `packaging/synology/ffmpeg/README.md`. FFmpeg is fetched outside the chroot (§2).
- **GPL compliance**: the SPK carries FFmpeg's licence and a source notice (FFmpeg version,
  upstream URL, SHA-256, build configuration) in `target/share/licenses/ffmpeg/`. **The exact
  corresponding source** (FFmpeg source archive plus the build's patches and scripts) **is
  attached to each GitHub Release that carries SPKs**, so the offer does not depend on a third
  party keeping old builds online (GPL-3.0 section 6).
- No `/dev/dri` access and no hardware-acceleration groups. Hardware transcoding on DSM is a
  later decision (milestone 4 or later).
- The Docker image still uses Debian's FFmpeg (ADR 0015 interim), so SPK and image may run
  different FFmpeg versions; capability detection (M2-16) handles that.

### 5 Package identity, versioning and DSM minimum

- **`package` = `brand.ID` (`mivo`) for now** (OD-SPK-9 A), `displayname` = `brand.DisplayName`,
  `maintainer` = `MAGPEEK`, `maintainer_url` = `brand.RepositoryURL`, `description` in English
  plus a German description key *(key name: verify, SPK-01)*.
- `os_min_ver` = `7.2-64570` (DSM 7.2; exact build number: verify, SPK-01), no `os_max_ver`.
- **Pre-releases set the beta flag** of INFO *(key: verify, SPK-01)*.
- **Version mapping** (OD-SPK-5 A): DSM's `version` is `<major>.<minor>.<patch>-<build>`, where
  the build number encodes the pre-release stage, its number and a package revision:

  | Server version | Package revision | DSM `version` |
  |---|---|---|
  | `0.1.0-alpha.2` | 0 | `0.1.0-1020` |
  | `0.1.0-alpha.2` | 1 (packaging-only fix) | `0.1.0-1021` |
  | `0.1.0-beta.1` | 0 | `0.1.0-2010` |
  | `0.1.0-rc.3` | 0 | `0.1.0-3030` |
  | `0.1.0` | 0 | `0.1.0-9000` |
  | `0.0.0-dev` (local builds) | 0 | `0.0.0-0000` |

  Build = stage × 1000 + pre-release number × 10 + revision; stages alpha 1, beta 2, rc 3,
  final 9; pre-release numbers 0–99, revisions 0–9, anything else is a build error. This is
  monotonic if DSM compares the feature part numerically per dot segment and then the build
  number *(verify, SPK-01)*. The server's `--version` and the web app keep showing the real
  version; the docs explain the Package Center number.
- SPK file name in the release: `<id>-<server version>-<family>.spk`, e.g.
  `mivo-0.1.0-alpha.2-armv7.spk` (the build script renames the toolkit's output).
- The package name contains the working title. DSM identifies a package by it, so a rename
  installs a different package without an upgrade path. This is on the rename checklist of
  ADR 0002 (Notes ADR 0026).

### 6 Layout and configuration mapping (ADR 0018)

| Purpose | DSM location | How the server gets it |
|---|---|---|
| server binary | `$SYNOPKG_PKGDEST/bin/<id>` | started by `start-stop-status` |
| FFmpeg | `$SYNOPKG_PKGDEST/bin/ffmpeg`, `…/ffprobe` | `<PREFIX>_TOOLS_FFMPEG`, `<PREFIX>_TOOLS_FFPROBE` |
| config directory, `config.yaml` | `$SYNOPKG_PKGVAR` | `<PREFIX>_CONFIG_DIR` |
| data (database, instance lock) | `$SYNOPKG_PKGVAR/data` | `<PREFIX>_PATHS_DATA` |
| cache | `$SYNOPKG_PKGVAR/cache` | `<PREFIX>_PATHS_CACHE` |
| transcode | `$SYNOPKG_PKGTMP/transcode` (OD-SPK-7 A; fallback `$SYNOPKG_PKGVAR/transcode` if SPK-01 finds `tmp` unsuitable) | `<PREFIX>_PATHS_TRANSCODE` |
| log | `$SYNOPKG_PKGVAR/<id>.log` (+ `.1`) | stdout and stderr redirected by the script |
| PID file | `$SYNOPKG_PKGVAR/<id>.pid` | written by the script |
| home | `$SYNOPKG_PKGHOME` if DSM provides it | `HOME` (fontconfig/libass caches later) |

- Directories are created with mode `0750`. `var` and `tmp` lie on a data volume, not the
  system partition *(verify, SPK-01)*; a transcode directory on the small system partition
  would fill it.
- The environment set by the script wins over `config.yaml` (ADR 0018 precedence): paths and
  tools belong to the package. Port, `server.lan_only`, log settings and libraries come from
  `config.yaml` or the defaults.
- Brand values reach shell code through generated files: `package.env` for the package scripts
  (`ID`, `DISPLAY_NAME`, `ENV_PREFIX`, `PORT`) and `build.env` for `INFO.sh` and
  `SynoBuildConf/*` (plus version, DSM version, beta flag, family). Other files under
  `packaging/synology/package/` (wizard, resource, `.sc`) are templates with build-time tokens
  `@NAME@` (for example `@ID@`, `@DISPLAY_NAME@`, `@PORT@`), filled by the staging step from
  `internal/platform/brand`. `{{…}}` is DSM's own wizard/resource templating and `${…}` belongs
  to `sh`, so neither is used for build tokens. No file under `packaging/synology/package/`
  contains the product name.

### 7 Media access and libraries (OD-SPK-6 A)

- The package runs as its own DSM 7 user (`conf/privilege`: `"run-as": "package"`, never root)
  and can read only shared folders that were granted to that user.
- The install wizard (English and German) asks for **one existing shared folder** with videos
  (default `video`). `conf/resource` `data-share` grants that folder **read-only** to the
  package user, with the wizard value templated into the resource *(verify, SPK-01; if DSM
  cannot template it, the wizard explains the Control Panel grant instead)*.
- `postinst` writes `config.yaml` with that folder as a `movies` library **only if no
  `config.yaml` exists**, so upgrades and reinstalls keep the user's file. The library path is
  DSM's per-package share link `/var/packages/<id>/shares/<share>` if SPK-01 confirms it exists:
  it survives moving the share to another volume, and a library's identity is its root path
  (ADR 0022). Otherwise the resolved `/volumeN/<share>` is written.
- More folders until milestone 7: grant "Read" to the package's system user in Control Panel >
  Shared Folder > Edit > Permissions, add the library to `config.yaml` (over SSH) and restart
  the package. The setup wizard and settings of milestone 7 replace this.
- The package never gets write access to media folders and never writes transcode output into
  them. Path confinement is unchanged (ADR 0022).

### 8 Network

- Port **8460** (`config.DefaultPort`) is fixed in the package: `conf/resource` `port-config`
  registers `<id>.sc` (title = display name, `8460/tcp`) so the DSM firewall lists the
  service. `port_forward="no"`, so DSM's router configuration does not offer to forward the
  port while there is no authentication *(format and semantics: verify, SPK-01)*.
- INFO `adminport=8460`, `adminprotocol=http`, `adminurl=/` give Package Center its "Open"
  button. Changing `server.port` in `config.yaml` is unsupported in the SPK until a settings UI
  exists; the firewall entry and the button would keep pointing at 8460. The docs say so.
- **LAN-only guard** (ADR 0018, Notes M2-31): the server runs natively on the host network, so
  it sees real client addresses and the host's on-link IPv6 prefixes; `server.lan_only=true`
  stays the default. Caveat for the docs: DSM's reverse proxy, the login portal's application
  aliases and anything else that forwards through DSM's nginx present every client as
  loopback, and the guard then admits everyone. Until milestone 7 the docs advise against
  publishing the package through them. Whether QuickConnect can reach a package port is
  verified in SPK-01.

### 9 Process control and logging

- `start`: starts the server in the background with the environment of section 6, appends
  stdout and stderr to the log, writes the PID file and waits up to 30 s for
  `<id> healthcheck` to succeed. If the process exits first, `start` returns 1 and puts the
  last log lines where Package Center shows them (`$SYNOPKG_TEMP_LOGFILE`, verify SPK-01). A
  process that is alive but not yet healthy after 30 s counts as started (slow first start,
  database migration). Starting a running package does not start a second process.
- `stop`: `SIGTERM`, wait up to 20 s (the server's graceful shutdown plus database close; the
  same budget as Docker's `--stop-timeout 20`), then `SIGKILL`. FFmpeg children die with the
  server (ADR 0024, Notes M2-24b).
- `status`: 0 when the PID file names a live process whose `/proc/<pid>/exe` is the package
  binary, 3 when not running (a stale PID file is removed), 150 when the binary is missing or
  not executable.
- Logs: the server logs text to stderr; the script redirects it to `$SYNOPKG_PKGVAR/<id>.log`
  and rotates it at start when it exceeds 10 MiB (one old file kept). Whether Package Center
  shows this file is verified in SPK-01. A size limit while running (server-side file logging)
  is a later change.

### 10 Install, upgrade and uninstall (OD-SPK-8 A)

- Install: wizard → `preinst` rejects an empty or non-existent shared folder with a message →
  `postinst` creates the directories and writes `config.yaml` if missing.
- Upgrade: DSM stops the package and replaces `target`; `var` (configuration, database, cache,
  log) is kept, `tmp` (transcode output only) is wiped. The server migrates the database at
  start. Downgrades are unsupported (a newer schema stays); the docs say so.
- Uninstall: the uninstall wizard offers **"also delete the database, settings and logs",
  unchecked by default; unchecked keeps `$SYNOPKG_PKGVAR`**, if DSM allows that *(verify,
  SPK-01; if DSM always removes `var`, the wizard says so instead)*. Shared folders and media
  are never touched.
- `support_move` (moving the package to another volume) stays off in the first SPK.

### 11 Distribution

- Every release with SPKs attaches to its GitHub Release: one SPK per published family,
  `SHA256SUMS`, the FFmpeg corresponding source, and release notes with the manual-install
  steps. Publishing is done by `mivo-release` after PASS + APPROVE, under the same approval
  rules as every release.
- Installation is Package Center > Manual Install; the packages are unsigned (signing is
  deprecated since DSM 7).
- A package source (an own feed, or SynoCommunity) and a GitHub Actions workflow come later.

## Consequences

- DSM users get a native package for all three families, including models without Container
  Manager. Docker remains the primary NAS distribution (ADR 0014).
- The build follows Synology's documented toolchain, at a cost: a privileged container, several
  GB of platform environments to download and cache, Linux-only tooling (through Docker Desktop
  on the dev machine) and no byte-reproducible SPKs.
- One more artifact per release: the Docker stages for web and Go are duplicated in
  `packaging/synology/Dockerfile` until GoReleaser and CI consolidate the builds (milestone 9).
- The packages are large: static FFmpeg and ffprobe add roughly 100–200 MB uncompressed per
  family.
- armv7 binaries of the server and FFmpeg are exercised only under emulation unless a community
  device tests them.
- Library management on DSM needs SSH until milestone 7; the wizard covers the first library.
- Statically linked Go dependencies (BSD/MIT licensed) also need their notices in binary
  distributions. This applies to the Docker image as well and is left to a general
  third-party-notices task (milestone 9), not solved per package.
- Because brand values are build-time inputs, a rename changes the package identity but no
  packaging file (ADR 0002 notes).

## To verify before relying on it

- SPK-01 (DSM behaviour): SPK container format and member order; INFO quoting, version
  comparison, beta and German description keys, `os_min_ver` build of DSM 7.2, `checkport`
  semantics; icon sizes; script users, environment variables, UI error messages, timeouts, extra
  files in `scripts/`; `privilege` and `resource` schemas (`data-share` wizard templating,
  `/var/packages/<pkg>/shares/` links, `port-config` and the `.sc` format); real locations and
  lifetimes of `var`, `tmp`, `home`; `var` on uninstall; the package log in Package Center;
  manual install of unsigned packages on DSM 7.2.2; DSM 7.2 kernels and CPU features per family;
  Container Manager availability; QuickConnect and package ports.
- SPK-02: availability and properties of static FFmpeg builds per family.
- SPK-04 (toolkit): privileges in Docker Desktop, download size and time of the platform
  environments, the platform per family and the `arch` value `PkgCreate.py` writes, the no-sign
  option, helper names (`pkg_make_package`, `pkg_make_spk`, `pkg_dump_info`) and the INFO keys
  `pkg_dump_info` supports.

Findings are recorded in `packaging/synology/DSM-NOTES.md`, `packaging/synology/ffmpeg/README.md`
and `packaging/synology/toolkit/README.md`; findings that change a statement above are added as
"Notes" to this ADR by the architect.

## Decisions of the project owner (2026-10-31)

| Decision | Answer |
|---|---|
| ADR 0026 | Accepted; unconfirmed facts stay marked *verify* |
| OD-SPK-1 timing | A – Docker release on schedule, SPKs attached to the same GitHub Release later |
| OD-SPK-2 build approach | **B – official Package Toolkit `pkgscripts-ng`** (not the recommended own assembler) |
| OD-SPK-3 FFmpeg | A – one prebuilt static GPL source for all families if possible, own build for a missing family as follow-up; exact corresponding source attached to each GitHub Release – **revised the same day: own static build for all families, see Notes (SPK-01, SPK-02, SPK-04)** |
| OD-SPK-4 armv7 | A – published only if its emulated smoke run passes, labelled "tested under emulation only" |
| OD-SPK-5 version | A – `0.1.0-alpha.2` → `0.1.0-1020`, final → `0.1.0-9000`, beta flag for pre-releases |
| OD-SPK-6 libraries | A – wizard asks for one shared folder, read-only `data-share`, `postinst` writes `config.yaml`; more folders via Control Panel + `config.yaml` over SSH |
| OD-SPK-7 transcode | A – `$SYNOPKG_PKGTMP/transcode`, if SPK-01 confirms |
| OD-SPK-8 uninstall | A – wizard checkbox, unchecked keeps the data |
| OD-SPK-9 identity | A – package name `brand.ID` (`mivo`) for now |

## Notes (SPK-01, SPK-02, SPK-04)

Results of wave 1 (sources and evidence: `packaging/synology/DSM-NOTES.md`,
`packaging/synology/ffmpeg/README.md`, `packaging/synology/toolkit/README.md`) and the decisions
taken on them on 2026-10-31. Where a note contradicts the decision text above, the note wins.
"Unconfirmed" items stay to be checked by SPK-10, SPK-11 or SPK-12.

**Owner decision: own static FFmpeg build (revises OD-SPK-3 and §4)**
- SPK-02 found that **no prebuilt FFmpeg meets §4** for any family: jellyfin-ffmpeg portable and
  BtbN link glibc dynamically and require Linux ≥ 4.18 (DSM 7.2 runs 3.10/4.4) and have no
  32-bit ARM build; jellyfin's GPL build links `libfdk-aac` through a patched `configure`
  (non-free upstream, see ADR 0015 Notes); johnvansickle stops at FFmpeg 7.0.2, publishes no
  corresponding source and its armhf build needs VFPv4.
- The owner decided: **one own static FFmpeg build for x86_64, armv8 and armv7** (task SPK-15,
  integration SPK-16). This replaces the preference order of §4; the other criteria of §4 stay.
  Requirements: one Docker recipe; fully static (musl, or glibc with `--enable-kernel` ≤ 3.10)
  running on kernels 3.10.108 and 4.4.302; FFmpeg ≥ 7.1 pinned by checksum; GPLv3
  (`--enable-gpl --enable-version3`) with `libx264` and the native AAC encoder, **no
  `libfdk-aac`, no `--enable-nonfree`**; baselines x86-64 v1, armv8-a, armv7-a hard-float with
  VFPv3-D16 (NEON only through run-time CPU detection, never VFPv4); every demuxer of the server's
  `-format_whitelist` plus the `hls` muxer with fMP4; the exact corresponding source of FFmpeg
  **and every linked library** as a release artefact.
- Until SPK-16 lands, `sources.lock` pins **interim** builds (jellyfin 8.1.3-1 for x86_64/armv8,
  johnvansickle 7.0.2 for armv7) for pipeline development and emulated tests only. **No release
  may contain an interim build**: SPK-10's release mode and SPK-13 refuse while `sources.lock` (or
  a packed `SOURCE.txt`) says `interim`.
- Emulation checks of FFmpeg use the old kernel's syscall surface (seccomp wrapper `kernelsim`,
  `uname` reporting the DSM kernel), baseline CPU models and a glibc 2.36 userland, as in SPK-02.

**Architectures and kernels**
- Oldest DSM 7.2 kernel per family: **x86_64 3.10.108, armv8 4.4.302, armv7 3.10.108**; static
  binaries need an ELF ABI note at or below these. The DSM 7.2 userland is glibc 2.36
  (inference from the toolchains).
- DSM's `armv7` family contains only `alpine`/`alpine4k`; `armada38x` and `monaco` are separate
  `arch` values ("armv7 not ready" in the toolkit). **Orchestrator decision**: the armv7 SPK sets
  **`arch="armv7 armada38x monaco"`**, and its binaries target ARMv7 hard-float with VFPv3(-D16),
  NEON only where detected at run time, no VFPv4 assumption. Installation on armada38x/monaco
  stays unconfirmed until a community test; this sharpens §2/§3 ("one SPK installs on every model
  of the family").
- **NEON on armada38x and monaco: unconfirmed.** Synology's toolkit builds armada38x with
  `-mfpu=vfpv3` and no NEON, monaco with `-mfpu=neon` (DSM-NOTES §13); no datasheet was cited
  for either SoC. The statement in `packaging/synology/ffmpeg/README.md` that "all DSM 7.2 armv7
  platforms have NEON" has no source and is withdrawn (SPK-15 corrects it). Binding rule until a
  vendor datasheet settles it: **assume no NEON** on armada38x – compile-time baseline
  `-march=armv7-a -mfpu=vfpv3-d16 -mfloat-abi=hard`, NEON code only behind run-time CPU detection
  (FFmpeg, x264), never VFPv4. Go's `GOARM=7` (VFPv3, no NEON) already meets this.
- Toolkit platforms chosen: **x86_64 → `v1000`, armv8 → `rtd1619b`, armv7 → `alpine`**. The
  toolkit writes no `arch` by itself: `INFO.sh` sets `ARCH` from `build.env` and checks its first
  word against `pkg_get_platform_family`.
- Container Manager: no 32-bit ARM model, some armv8 models (DS120j, DS220j, DS420j, DS124,
  DS223, DS223j, DS423), most but not all x86_64 models. The smallest DSM 7.2 armv8 model
  (DS119j) has 256 MB RAM; the documented minimum stays 1 GB.

**Toolkit (pinned `pkgscripts-ng` DSM7.2 commit `e4a8d54209a31cef21b7636ad58b1b0e86b5b201`;
platform environments DSM 7.2.2 build 72723)**
- **No signing exists and there is no `--no-sign`** at the pinned commit (the option is rejected
  as unknown; the guide's option list is outdated). "Signing off" needs no flag; SPKs carry no
  signature member. **`-I` means "not install projects"** (`-i` "only install"); the build runs
  `PkgCreate.py -v 7.2 -p <platform> -c <ID>`.
- `SynoBuildConf/build` and `install` are sourced by bash and their exit status is ignored:
  **success is decided by grepping the logs** for `Error`, `error:`, `fatal error`,
  `No such file or directory` and similar. Our scripts print `Error: <reason>` on failure and
  never print those words otherwise.
- **`SynoBuildConf/depends` must be pure ASCII**: the chroot's Python 3.4 reads it with an ASCII
  locale, and one non-ASCII byte silently empties the dependency list. `run.sh` refuses non-ASCII
  bytes in `SynoBuildConf/` and CRLF in project scripts. INFO values may be UTF-8.
- Privileges: Docker's default capabilities fail (mounting `/proc` in the chroot);
  **`--cap-add SYS_ADMIN` or `--privileged`** works on Docker Desktop. On Linux hosts with
  AppArmor (Ubuntu, GitHub-hosted runners) the narrow set will likely also need
  `--security-opt apparmor=unconfined` (unverified); the documented and tested default is `--cap-add SYS_ADMIN` (SPK-04 r2); `--privileged` is only a fallback for hosts where the narrow set fails.
- The named volume `spk-toolkit` holds about 24 GB for three families (3.4 GiB tarballs); the
  first x86_64 deployment downloads about 2 GiB, cached runs take about 2 s.
- `pkg_make_package` writes `package.tgz` **xz-compressed** (`tar cJf`) despite the name;
  `pkg_make_spk` packs an uncompressed tar (`tar cf`) in `ls` order with owner `root/root` and
  appends `extractsize`, `toolkit_version` and `create_time` to INFO, so SPKs differ byte-wise
  between builds (as §2 expected). **The tar flavour is unconfirmed**: only the `tar cf`
  invocation is known; GNU, ustar or pax depends on the chroot's `tar` defaults. SPK-10 records
  the flavour of the SPK and of `package.tgz` from the archive headers.
- The install stage runs twice (a debug pass first, skipped by our `install`). The guide's
  `pkg_get_unified_platform` does not exist; `pkg_get_platform_family` does.

**INFO**
- `os_min_ver="7.2-64570"` confirmed (DSM 7.2 GA was 64561; 64570 is the first build every 7.2
  model received). `beta="yes"`, `description_ger`/`displayname_ger` confirmed. Not set:
  `thirdparty` (DSM 4.x only), `startable` (deprecated), `changelog` (undocumented, not in
  `pkg_dump_info`); `support_move="no"` is appended by `INFO.sh`; `checkport` keeps its default.
- **Decision: `adminurl` is omitted** – DSM opens `adminprotocol://<ip>:adminport/adminurl`, so
  `adminurl="/"` would give `…:8460//`. INFO has only `adminport="8460"` and
  `adminprotocol="http"`. This **replaces** "`adminurl=/`" in §8; `INFO.sh` (which still writes
  `adminurl="/"` after SPK-04) is aligned in an SPK-04 rework round, and SPK-10 fails an SPK that
  carries `adminurl`. Whether the "Open" button appears without `adminurl` and which host it uses
  is checked in SPK-12; fallback `adminurl=""` (empty).
- INFO values must not contain `"`, `\`, `$`, a backtick, a tab or a newline (the toolkit sources
  INFO as shell); main fields (`description`, `displayname`, `maintainer`, …) also no `*`, `?`,
  `[` or double spaces (`pkg_dump_info` echoes them unquoted).
- Version format: numeric parts only, each ≤ 2^31−1; the guide says the build number "should be
  increased on every version", and how DSM compares versions is undocumented. **Orchestrator
  decision**: OD-SPK-5 stays; SPK-12 installs dummy SPKs `0.1.0-9000` → `0.1.1-1010` and
  `0.1.0-1020` → `0.1.0-1030` and checks that Package Center offers each as an upgrade. If DSM
  refuses, the owner is offered a globally monotonic build number (DSM-NOTES "Impact" 2).

**Scripts**
- All scripts run as the package user. A message for the user goes to `$SYNOPKG_TEMP_LOGFILE` with
  a non-zero exit; non-zero from `preinst`/`preuninst`/`preupgrade` aborts, from
  `postinst`/`postupgrade` marks the package corrupted.
- **`start-stop-status prestart` and `prestop` must exit 0**: DSM calls them by default
  (`precheckstartstop`), `prestart` also at boot; `log` exits 0 (legacy); only unknown arguments
  give 1.
- `SYNOPKG_PKG_STATUS` is `INSTALL`, `UPGRADE`, `UNINSTALL`, `START`/`STOP`, or empty at boot and
  shutdown. On upgrade DSM runs the old `preuninst`/`postuninst` and the new `preinst`/`postinst`
  with `UPGRADE` and without install or uninstall wizard keys. Hence: `preinst` validates the share
  only on `INSTALL`; `postinst` writes `config.yaml` only on `INSTALL` and only if absent;
  `postuninst` deletes data only on `UNINSTALL` with `wizard_delete_data=true`. Wizard keys never
  reach `start-stop-status`.
- Extra files in `scripts/` (`common.sh`, `package.env`) are not documented: scripts find them
  through their own directory; if SPK-12 shows DSM drops them, SPK-14 inlines both (unconfirmed).
- **Logs**: Package Center shows no package log (unconfirmed). Script stdout/stderr goes to
  **`/var/log/packages/<id>.log`**, package operations to `/var/log/synopkg.log`; the server log
  stays in `$SYNOPKG_PKGVAR/<id>.log` (SSH). This settles the open point of §9.

**Resources and directories**
- Package user and group are `<id>`; no `username`/`groupname` override; no `run-as: root`
  anywhere (an unsigned package needing root is refused).
- **`data-share` creates a missing share**, so `preinst` must reject a share that does not exist
  (test `/volume*/<share>`). It is acquired after `postinst`, before the first start, and repeated
  on every start; the share and the grant stay after uninstall. Templating `{{wizard_media_share}}`
  is community practice, not documented (unconfirmed). `/var/packages/<id>/shares/<share>` exists
  since DSM 7.0-41201 once the resource is acquired; `postinst` writes that path without testing
  it.
- `port-config` points at **`port_conf/<id>.sc` inside `package.tgz`** (relative to `target`). The
  `.sc` is INI: section `[<id>]` (letters, digits, `-`, `.`; no underscore), `title`, **`desc`
  (required)**, `port_forward="no"` (default), `dst.ports="8460/tcp"`.
- Share names: 1–32 characters, none of `` ! " # % & ' ( ) * + , / : ; < = > ? @ [ ] \ ^ ` { } | ~ ``,
  no leading `-` or space, no trailing space, `$` only at the end; spaces and non-ASCII letters are
  allowed.
- `var`, `tmp`, `home`, `etc` live on the install volume (`@appdata`, `@apptemp`, `@apphome`,
  `@appconf`), owner `<id>`, `var`/`tmp` 0755, `home` 0700. `tmp` and `target` are removed only on
  upgrade and uninstall, **`var`, `etc` and `home` never** – this confirms **OD-SPK-7 A**
  (`$SYNOPKG_PKGTMP/transcode`) and **OD-SPK-8 A** (`postuninst` deletes the contents of `var` and
  `home` only on request). `tmp` probably survives reboots, so `start` clears stale transcode
  output. `HOME=$SYNOPKG_PKGHOME` unconditionally.

**Wizard (orchestrator decision)**
- The DSM 7.2.2 guide documents only a Vue render-function wizard (DSM 7.2.2 required). The first
  SPK uses the **classic JSON `WIZARD_UIFILES`** (`install_uifile`, `install_uifile_ger`,
  `uninstall_uifile`, `uninstall_uifile_ger`), as community packages do on DSM 7.2; the Vue format
  is the fallback only (it would raise `os_min_ver` to `7.2-72806` and add a webpack build). SPK-12
  verifies the classic wizard on the owner's device.

**Install and network**
- Manual install: there is no trust level any more; DSM shows a third-party alert and the
  `LICENSE` text as a page to accept.
- **DSM's reverse proxy and QuickConnect defeat the LAN-only guard**: through the reverse proxy the
  server sees loopback or the NAS's own LAN address, so every client passes; QuickConnect is
  documented only for DSM and Synology packages and is assumed not to reach port 8460. The docs
  warn against both until milestone 7.
