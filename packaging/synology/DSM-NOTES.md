# DSM 7.2 package facts (SPK-01)

Answers to the DSM behaviour questions of `docs/tasks/SPK-01-dsm-package-facts.md`, which
ADR 0026 marks *(verify, SPK-01)*. The package these facts serve installs the **media server**
as a DSM service (server binary, embedded web app, bundled FFmpeg, port 8460); it is not a DSM
client app. How the toolkit itself runs (privileges, platform environments, helper usage) is
recorded by SPK-04 in `toolkit/README.md`.

The package name is written as `<id>` (`brand.ID`); no packaging file names the product.

## Status and sources

- **confirmed**: stated by Synology's DSM 7.2.2 Package Developer Guide, by the
  `pkgscripts-ng` sources, or by another Synology publication named next to it.
- **unconfirmed**: not stated by Synology. The answer gives the assumption the tasks use and the
  check that confirms it: SPK-10 (toolkit output), SPK-11 (emulated install and smoke) or SPK-12
  (manual install on the owner's x86_64 Synology). Community sources (SynoCommunity `spksrc`) may
  support an unconfirmed answer; they never make it confirmed.

Sources, retrieved 2026-10-31:

| Short name | Source |
|---|---|
| Guide | DSM 7.2.2 Package Developer Guide, <https://help.synology.com/developer-guide/> (unversioned web book; every reference gives the page URL and section title) |
| Toolkit | `pkgscripts-ng`, branch `DSM7.2`, commit `e4a8d54209a31cef21b7636ad58b1b0e86b5b201` ("Release for DSM7.2.2", 2025-05-28) |
| ExamplePackages | Synology's template package linked from the guide pages "Your First Package" and "WIZARD_UIFILES", <https://github.com/SynologyOpenSource/ExamplePackages>, commit `d2849c6fcf14ce72007d14e99a362c3d4f23be0a` |
| Synology KB / product pages | Knowledge Center, release notes and package pages on `synology.com` (Synology, but not the developer guide) |
| Community | SynoCommunity `spksrc`, commit `7b99e9d4bba86085d46e5ffe10532e97558fa7cd` – supporting evidence only |

Link definitions are collected at the end of the file.

## 1 SPK container

| Question | Answer | Status | Source |
|---|---|---|---|
| Archive format | An **uncompressed tar** archive: `pkg_make_spk` runs `tar cf <spk> -C <dir> $(ls <dir>)`. The flavour is whatever GNU `tar` in the platform chroot writes by default (GNU format); the guide itself does not name the container format. | confirmed (toolkit) | [Toolkit `pkg_make_spk`][t-make-spk] (line 281, 317) |
| Compression of the SPK | None (plain `tar cf`). | confirmed (toolkit) | [Toolkit `pkg_make_spk`][t-make-spk] |
| Required members | `INFO`, `package.tgz`, `scripts/` (all seven scripts `preinst`, `postinst`, `preuninst`, `postuninst`, `preupgrade`, `postupgrade`, `start-stop-status` are marked required), `conf/` (with `privilege`, required since DSM 7), `PACKAGE_ICON.PNG`, `PACKAGE_ICON_256.PNG`. | confirmed | [Guide "Package Introduction" – "Package Structure"][g-intro]; [Guide "scripts"][g-scripts]; [Guide "conf"][g-conf] |
| Optional members | `WIZARD_UIFILES/`, `LICENSE` (< 1 MB, shown in an "agree" dialog during installation), `conf/resource`, `conf/PKG_DEPS`, `conf/PKG_CONX`, `scripts/prereplace`, `scripts/postreplace`. No signature member: signing was removed in DSM 7.0. | confirmed | [Guide "Package Structure"][g-intro]; [Guide "License"][g-license]; [Guide "Breaking Changes in 7.0" #3][g-breaking] |
| Member order | The toolkit adds the top-level entries in `ls` order of the staging directory (alphabetical in the chroot's locale; directories recursed by `tar`). Whether DSM requires an order is not documented. **Assumption**: any order works; the toolkit's order is the reference. | unconfirmed | [Toolkit `pkg_make_spk`][t-make-spk]; check: SPK-10 records the order, SPK-12 installs it |
| Compression of `package.tgz` | The guide allows **gzip or xz** ("tgz / xz"). The toolkit's `pkg_make_package` writes **xz** (`tar cJf`) although the file is named `package.tgz`; top-level entries are added from `ls`, without a `./` prefix. | confirmed | [Guide "package.tgz"][g-tgz]; [Toolkit `pkg_make_package`][t-make-package] (line 215, 231) |
| INFO lines added at packing | `pkg_make_package` measures the payload (`du -sk`) and `pkg_make_spk` appends `extractsize`, `toolkit_version` (DSM build of the environment) and `create_time` (`date +%Y%m%d-%T`) to INFO; `create_time` makes two packings differ byte-wise. | confirmed (toolkit) | [Toolkit `pkg_make_spk`][t-make-spk] (lines 305–311) |

## 2 INFO syntax and keys

| Question | Answer | Status | Source |
|---|---|---|---|
| Line format | One `key=value` pair per line; Synology's examples and the toolkit write `key="value"`. | confirmed | [Guide "INFO" – "INFO Field Format"][g-info]; [Toolkit `pkg_dump_info`][t-dump-info] |
| Quoting / escaping | Not documented for DSM's parser. The toolkit **sources INFO as shell** (`. $info_path` in `__get_spk_name`, line 240), and `pkg_dump_info` writes the main fields with an **unquoted** `echo $f=\"${!f}\"` (line 193): word splitting collapses runs of spaces and `*`, `?`, `[` are glob-expanded against the working directory. Language variants (`description_ger`, …) are echoed quoted (lines 196–203). **Assumption**: INFO values contain no `"`, `\`, `$`, backtick or newline; values written through the main field loop (`description`, `displayname`, `maintainer`, …) also contain no `*`, `?`, `[` and no double spaces. spksrc escapes `"` as `\"` (community), which we avoid altogether. | unconfirmed (DSM parser); confirmed (toolkit constraints) | [Toolkit `pkg_dump_info`][t-dump-info]; [Toolkit `__get_spk_name`][t-get-spk-name]; community: [spksrc INFO generation][c-spk-info]; check: SPK-10 (INFO round trip), SPK-12 (Package Center shows the texts) |
| Version format | `[feature number]-[build number]`; delimiters only `.`, `-` or `_`; every delimited part numeric; major, minor, micro and build each 0 … 2^31−1. "**Build number should be increased on every version**." | confirmed | [Guide "Necessary Fields" – "version"][g-info-nec] |
| How DSM compares versions | Not documented. Indirect evidence: `PKG_DEPS` accepts `pkg_min_ver=2` meaning "version 2 or newer", which only makes sense if the feature part is compared numerically from the left. **Assumption**: DSM compares the dot-separated feature numbers numerically from the left and then the build number. The guide's rule that the build number grows with every version is **not** met by OD-SPK-5 when the feature version changes (`0.1.0-9000` → `0.1.1-1010`); see "Impact" item 2. | unconfirmed | [Guide "Necessary Fields" – "version"][g-info-nec]; [Guide "PKG_DEPS"][g-pkgdeps]; community: spksrc keeps one revision counter that only grows across versions ([jellyfin `SPK_REV`][c-jellyfin-mk]); check: SPK-12 (two dummy SPKs, see Impact 2) |
| Maximum lengths | None documented, except: `package` must not contain `:`, `/`, `>`, `<`, `\|`, `=`; version numbers ≤ 2^31−1; `LICENSE` < 1 MB. **Assumption**: short single-line texts (description ≤ ~250 characters) display fine. | unconfirmed (lengths); confirmed (listed limits) | [Guide "Necessary Fields" – "package", "version"][g-info-nec]; [Guide "Package Structure"][g-intro]; check: SPK-12 |
| Beta flag | `beta="yes"` ("the beta information will be shown in Package Center"); `report_url` also marks a package as beta. `pkg_dump_info` supports `beta`. | confirmed | [Guide "Optional Fields" – "beta", "report_url"][g-info-opt]; [Toolkit `pkg_dump_info`][t-dump-info] |
| German description | `description_ger` (also `displayname_ger`); DSM language codes `enu`, `ger`, …; `pkg_dump_info` supports all 20 language suffixes. | confirmed | [Guide "Optional Fields" – "description_[DSM language]"][g-info-opt]; [Toolkit `pkg_dump_info`][t-dump-info] (line 181) |
| `changelog` | Not documented in the guide and not in `pkg_dump_info`'s field list. spksrc writes `changelog="…"` (community). **Assumption**: omit it in the first SPK; if wanted later, `INFO.sh` appends it and SPK-12 checks the display. | unconfirmed | [Guide "Optional Fields"][g-info-opt] (absent); [Toolkit `pkg_dump_info`][t-dump-info] (absent); community: [spksrc INFO generation][c-spk-info]; check: SPK-12, only if the key is added |
| `thirdparty` | Only used by DSM 4.0–4.3 ("not used in DSM 5.0 or newer"): **do not set**. | confirmed | [Guide "Optional Fields" – "thirdparty"][g-info-opt] |
| `checkport` | Default `yes`: Package Center checks that `adminport` does not conflict with reserved or listening ports, except web-service ports (80, 443) and DSM ports (5000, 5001). When exactly (install, start) and whether the package's own `.sc` entry counts as a conflict on upgrade is not documented. **Assumption**: keep the default; install and upgrade on a NAS where 8460 is free do not report a conflict. | confirmed (semantics); unconfirmed (timing, own `.sc`) | [Guide "Optional Fields" – "checkport"][g-info-opt]; check: SPK-12 (install, then upgrade 1020 → 1021) |
| `startable` | Deprecated after 6.1-14907, replaced by `ctl_stop`. Do not set. | confirmed | [Guide "Optional Fields" – "startable"][g-info-opt] |
| `ctl_stop` | Default `yes`; `no` hides start/stop in Package Center (the script still runs at boot and shutdown). Do not set. | confirmed | [Guide "Optional Fields" – "ctl_stop"][g-info-opt] |
| `support_move` | Default `no`; `yes` lets the user move the package to another volume (DSM ≥ 6.2-22306). Not in `pkg_dump_info`'s field list. Do not set. | confirmed | [Guide "Optional Fields" – "support_move"][g-info-opt]; [Toolkit `pkg_dump_info`][t-dump-info] |
| "Open" button | `adminprotocol://<ip>:adminport/adminurl` is opened; `adminport` default 80, `adminprotocol` default `http`, `adminurl` default empty, example `adminurl="web"` (no leading slash). An `adminurl="/"` would therefore produce `http://<ip>:8460//`. Which host DSM puts in for `<ip>` and whether the button appears without `adminurl` is not documented; spksrc's media-server packages set only the port (community). **Assumption**: `adminport="8460"`, `adminprotocol="http"`, no `adminurl`. | confirmed (composition); unconfirmed (host, button without `adminurl`) | [Guide "Optional Fields" – "adminport", "adminurl", "adminprotocol"][g-info-opt]; community: [spksrc jellyfin `ADMIN_PORT`][c-jellyfin-mk]; check: SPK-12 |
| `os_min_ver` format | `X.Y-Z` (major, minor, build), at least `7.0-40000` for DSM 7. | confirmed | [Guide "Necessary Fields" – "os_min_ver"][g-info-nec]; [Guide "Breaking Changes in 7.0" #2][g-breaking] |

## 3 `arch` values and DSM 7.2 minimum build

| Question | Answer | Status | Source |
|---|---|---|---|
| Are `x86_64`, `armv8`, `armv7` accepted family values? | Yes; `arch` takes a space-separated list of family and platform values (`arch="x86_64 alpine"`). | confirmed | [Guide "Necessary Fields" – "arch"][g-info-nec]; [Guide "Appendix A: Platform and Arch Value Mapping Table"][g-arch] |
| `x86_64` covers | Guide: apollolake, avoton, braswell, broadwell, broadwellnk, broadwellntb, broadwellntbap, bromolow, cedarview, coffeelake, denverton, geminilake, grantley, kvmx64, purley, skylaked, v1000. The toolkit's `plat_to_family` additionally maps the newer DSM 7.2 platforms r1000, epyc7002, broadwellnkv2, geminilakenk, r1000nk and v1000nk to `x86_64`. Of these, DSM 7.2 platforms per the toolkit: avoton, braswell, bromolow, grantley, kvmx64, broadwellnk, denverton, apollolake, purley, v1000, broadwell, geminilake, broadwellntbap, r1000, broadwellnkv2, epyc7002, geminilakenk, r1000nk, v1000nk (and kvmcloud, which `plat_to_family` does not map). **Assumption**: DSM installs an `arch="x86_64"` package on every x86_64 platform the toolkit maps. | confirmed (guide + toolkit); unconfirmed that DSM itself installs `arch="x86_64"` on platforms missing from the guide table | [Guide Appendix A][g-arch]; [Toolkit `plat_to_family`][t-family]; [Toolkit `toolkit.config`][t-config]; check: SPK-12 on the owner's AMD model |
| `armv8` covers | rtd1296, armada37xx, rtd1619, rtd1619b (DSM 7.2 toolkit: rtd1296, armada37xx, rtd1619b). | confirmed | [Guide Appendix A][g-arch]; [Toolkit `plat_to_family`][t-family] (line 81) |
| `armv7` covers | **Only alpine and alpine4k.** The other DSM 7.2 32-bit ARM platforms, **armada38x** and **monaco**, are not in the `armv7` family: the guide lists them only as separate platform values, and the toolkit calls them "armv7 not ready platforms" and returns the platform name as family. An SPK with `arch="armv7"` therefore does not cover DS116, DS216, DS216j, DS218j, DS416j, DS416slim, DS419slim, RS217, RS816 (armada38x) and DS216play (monaco). **Assumption** for the tasks: the armv7 SPK lists all three values, `arch="armv7 armada38x monaco"`. | confirmed (family membership); unconfirmed (DSM installs the list on armada38x/monaco) | [Guide Appendix A][g-arch]; [Toolkit `plat_to_family`][t-family] (lines 78–87); models: [Synology KB CPU table][kb-cpu]; check: none available on owner hardware – stays unconfirmed until a community test (SPK-11 only proves the instruction set) |
| Helper used by `INFO.sh` | The guide's Pack Stage example calls `pkg_get_unified_platform`, which does not exist in the toolkit at the pinned commit; ExamplePackage uses `pkg_get_platform` (platform name). The family comes from `pkg_get_platform_family`; for armv7 the value is set explicitly (see above). Details: SPK-04. | confirmed (toolkit) | [Guide "Pack Stage" – "INFO.sh"][g-pack]; [Toolkit `pkg_util.sh`][t-family]; [ExamplePackage `INFO.sh`][e-info] (line 11) |
| `os_min_ver` of DSM 7.2 GA | DSM 7.2 was first released as **7.2-64561** (2023-05-22), followed by **7.2-64570** (2023-06-20, Updates 1–4). Some models (DS124, DS223j) only ever received 7.2-64570 or later. 7.2.1 is 69057, 7.2.2 is 72806 (72803 for some models). **Assumption** kept: `os_min_ver="7.2-64570"` (the agent's floor; every DSM 7.2 model got this build). | confirmed | [Synology release notes][kb-relnotes] (data from [release-note API for DS920+][kb-relnotes-api]) |

## 4 Icons

| Question | Answer | Status | Source |
|---|---|---|---|
| Files, sizes, format | `PACKAGE_ICON.PNG` **64 × 64** for DSM 7.0+ (72 × 72 was DSM 6), `PACKAGE_ICON_256.PNG` **256 × 256**; PNG; both required; file names case-sensitive, at the top level of the SPK. | confirmed | [Guide "Package Structure"][g-intro]; [Guide "Breaking Changes in 7.0" #6][g-breaking] |

## 5 Scripts

| Question | Answer | Status | Source |
|---|---|---|---|
| User running the scripts | With `"defaults": {"run-as": "package"}` every script runs as the package user (resuid set to the package user). Per-action overrides (`ctrl-script`) exist; we use none. Privileged work happens only through resource workers. | confirmed | [Guide "Privilege" – "Privilege Config", `defaults`, `ctrl-script`][g-priv-conf]; [Guide "Breaking Changes in 7.0" #1][g-breaking] |
| `/bin/sh` on DSM | Not documented (scripts start with `#!/bin/sh`). **Assumption**: a POSIX shell, bash or BusyBox `ash`; scripts stay POSIX and are tested under both (SPK-05). | unconfirmed | [Guide "scripts"][g-scripts]; check: SPK-12 (`ls -l /bin/sh; /bin/sh --version`) |
| `SYNOPKG_PKGNAME`, `SYNOPKG_PKGDEST`, `SYNOPKG_PKG_STATUS`, `SYNOPKG_OLD_PKGVER`, `SYNOPKG_TEMP_LOGFILE` | Exported by Package Center. Also `SYNOPKG_PKGVER`, `SYNOPKG_PKGDEST_VOL`, `SYNOPKG_PKGPORT` (= `adminport`), `SYNOPKG_PKGINST_TEMP_DIR`, `SYNOPKG_TEMP_UPGRADE_FOLDER`, `SYNOPKG_DSM_LANGUAGE`, `SYNOPKG_DSM_VERSION_MAJOR/MINOR/BUILD`, `SYNOPKG_DSM_ARCH`, `SYNOPKG_TEMP_SPKFILE`, `SYNOPKG_USERNAME`, `SYNOPKG_PKG_PROGRESS_PATH`. | confirmed | [Guide "Script Environment Variables"][g-env] |
| `SYNOPKG_PKGVAR`, `SYNOPKG_PKGTMP`, `SYNOPKG_PKGHOME` | Script variables of the FHS directories `var` (since 7.0-40314), `tmp` (since 7.0-40356), `home` (since 7.0-40759). | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs] |
| Availability per script | The guide does not scope the variables per script. `SYNOPKG_PKG_STATUS`: `INSTALL` in `preinst`/`postinst` (and `start-stop-status` when started right after installation); `UPGRADE` in `preupgrade`, `preuninst`, `postuninst`, `preinst`, `postinst`, `postupgrade` and in `start-stop-status` around an upgrade; `UNINSTALL` in `preuninst`/`postuninst` and the stop before uninstall; `START`/`STOP` from Package Center; **empty at boot and shutdown**. **Assumption**: the path variables are set in every script. | confirmed (status values); unconfirmed (path variables in every script) | [Guide "Script Environment Variables"][g-env]; check: SPK-12 (`postinst` and `start` echo the resolved paths to stdout, which lands in `/var/log/packages/<id>.log`) |
| Wizard keys | Keys of the wizard components become environment variables with `true`, `false` or the text: install wizard keys in `preinst`/`postinst`; upgrade wizard keys in `preupgrade`, `preuninst`, `postuninst`, `preinst`, `postinst`, `postupgrade`; uninstall wizard keys in `preuninst`/`postuninst`. Not in `start-stop-status`. | confirmed | [Guide "WIZARD_UIFILES [7.2.2]"][g-wiz] |
| Error message in Package Center | Write the text to `$SYNOPKG_TEMP_LOGFILE` and exit non-zero; `$SYNOPKG_DSM_LANGUAGE` selects the language. Non-zero from `preinst`/`preuninst`/`preupgrade` aborts the operation; non-zero from `postinst`/`postupgrade` marks the package corrupted. **Assumption**: a failing `preinst` shows the text of `$SYNOPKG_TEMP_LOGFILE` in its error dialog. | confirmed (mechanism); unconfirmed (exact dialog for a failing `preinst`) | [Guide "Show Messages to Users"][g-msg]; [Guide "scripts"][g-scripts]; check: SPK-12 |
| Time limits for `start`/`stop` | Not documented. **Assumption**: `start` waiting up to 30 s for health and `stop` waiting up to 20 s before `SIGKILL` are accepted. | unconfirmed | check: SPK-12 (measure a cold start and a stop, watch for a Package Center timeout) |
| Extra files in `scripts/` | Not documented as allowed or forbidden; the toolkit has a helper that generates scripts from `*.sh` files in a directory (`pkg_create_scripts`), and DSM keeps package metadata under `/var/packages/<id>/` (`conf/resource` is updated there). **Assumption**: `scripts/common.sh` and `scripts/package.env` are copied with the scripts; every script finds them through its own directory (`${0%/*}`), never through a fixed path. Fallback if SPK-12 disproves it: SPK-14 inlines `package.env` and `common.sh` into each script. `preinst` runs before `package.tgz` is extracted, so shared code must not live only in `target`. | unconfirmed | [Toolkit `pkg_create_scripts`][t-create-scripts]; [Guide "Resource Update"][g-res-update]; check: SPK-12 (`ls -l /var/packages/<id>/scripts/`) |
| Boot | At boot DSM runs `start-stop-status prestart` (since DSM 7.0) and `start` with an empty `SYNOPKG_PKG_STATUS`; at shutdown `stop` (no `prestop`). `prestart`/`prestop` are called when `precheckstartstop` is `yes` – **the documented default**. The data-share worker's acquire is not repeated during boot. | confirmed | [Guide "scripts" – "start-stop-status"][g-scripts]; [Guide "Optional Fields" – "precheckstartstop"][g-info-opt]; [Guide "Resource Timing"][g-res-timing]; [Guide "Breaking Changes in 7.0" #9][g-breaking] |
| `status` exit codes | 0 running; 1 dead with `/var/run` PID file; 2 dead with `/var/lock` lock file; 3 not running; 4 unknown; 150 broken, reinstall. | confirmed | [Guide "scripts" – "start-stop-status"][g-scripts] |
| Execution order | Install: `preinst`, `postinst`, then `prestart`/`start` if chosen. Upgrade: old `prestop`/`stop`, new `preupgrade`, old `preuninst`, old `postuninst`, new `preinst`, new `postinst`, new `postupgrade`, new `prestart`/`start` if it ran before. Uninstall: `prestop`/`stop`, `preuninst`, `postuninst`. | confirmed | [Guide "scripts" – "Execution Order"][g-scripts] |

## 6 `conf/privilege`

| Question | Answer | Status | Source |
|---|---|---|---|
| Schema | `{"defaults": {"run-as": "package"}}` is required; optional `username`, `groupname` (since 6.0-5940), `ctrl-script`, `executable`, `tool` (`relpath`, `user`/`group` = `package`, `permission`, `capabilities` since 7.0-40656). With `run-as: package` DSM runs `chown -hR <package>:<package>` on the installed files. | confirmed | [Guide "Privilege"][g-priv]; [Guide "Privilege Config"][g-priv-conf] |
| Package user name | Without `username`/`groupname` the **package name** is the user and group name (`<id>`). The user is listed as "System internal user" in Control Panel > Shared Folder > Edit > Permissions. spksrc sets `username` to `sc-<name>` "to avoid collision with busybox account" (community); what DSM does when a normal user with the package's name exists is not documented. **Assumption**: no DSM installation has a user named `<id>`; the default name is kept. | confirmed (name); unconfirmed (collision) | [Guide "Privilege Config" – "username / groupname", "Package User / Group Visibility On UI"][g-priv-conf]; community: [spksrc privilege generation][c-spk-priv]; check: SPK-12 |
| Options we must not use | `"run-as": "root"` in `defaults` or `ctrl-script`; `run-as system` (removed in DSM 7); `executable` entries with `run-as: root`; `tool` entries with `capabilities` or set-uid/set-gid `permission` (e.g. `4750`); `username`/`groupname` overrides (the `data-share` grant names the default user). A package that needs root cannot be installed unsigned ("Failed to install. The package should run with a lower privilege level…"). | confirmed | [Guide "Privilege Config"][g-priv-conf]; [Guide "Breaking Changes in 7.0" #1, #4][g-breaking]; [Guide "System Requirements" – "Install Development Token"][g-sysreq] |

## 7 `conf/resource`

| Question | Answer | Status | Source |
|---|---|---|---|
| `data-share` schema | `{"data-share": {"shares": [{"name": "<share>", "permission": {"ro": ["<user>"], "rw": ["<user>"]}, "once": false}]}}`; `ro` and `rw` are user arrays; `permission` and `once` optional. | confirmed | [Guide "Data Share" – "Syntax"][g-data-share] |
| Behaviour | **Creates the share if it does not exist**, otherwise only sets the permission. Timing `FROM_ENABLE_TO_POSTUNINST`: acquired before the first start (`WHEN_ENABLE`, not during boot), i.e. **after `postinst`**; with `once: false` repeated on every start. `Release()` does nothing: the share and (apparently) the grant stay after uninstall. Not updatable. | confirmed | [Guide "Data Share"][g-data-share]; [Guide "Resource Timing"][g-res-timing] |
| Templating of wizard values (`{{wizard_key}}`) | **Not documented by Synology.** spksrc generates `"name": "{{wizard_shared_folder_name}}"` for DSM 7 packages (community). **Assumption**: templating works; fallback per ADR 0026 §7 (wizard explains the Control Panel grant). | unconfirmed | community: [spksrc data-share templating][c-spk-share]; check: SPK-12 (share granted read-only to `<id>` after install) |
| Non-existent share | The worker creates it (on a volume DSM chooses). `preinst` must therefore reject a missing share, or a typo creates a new empty share. How `preinst` (running as the package user) tests existence is not documented. **Assumption**: `[ -d /volume<N>/<share> ]` over all volumes. | confirmed (creation); unconfirmed (existence test) | [Guide "Data Share"][g-data-share]; check: SPK-12 |
| Upgrade with an empty upgrade wizard | Not documented. spksrc's transmission package templates `data-share` from the install wizard and ships an `upgrade_uifile` without that key (community), which suggests DSM keeps the values rendered at install. **Assumption**: the upgrade wizard does not need to repeat `wizard_media_share`. | unconfirmed | community: [spksrc transmission `upgrade_uifile`][c-transmission-upgrade]; check: SPK-12 (upgrade 1020 → 1021, grant and share link still there) |
| Uninstall | Shared folder kept ("might delete the user's personal data"), `Release()` does nothing. | confirmed | [Guide "Data Share"][g-data-share] |
| `/var/packages/<id>/shares/<share>` | Since 7.0-41201 Package Center creates a symlink named after the share pointing to the share path, for the shares of the `data-share` resource. It appears when the resource is acquired (first start), so `postinst` cannot test it. **Assumption**: the link exists before `start` runs. | confirmed (existence, since 7.0-41201); unconfirmed (exact moment) | [Guide "Data Share"][g-data-share]; check: SPK-12 (`ls -l /var/packages/<id>/shares/`) |
| `port-config` schema | `{"port-config": {"protocol-file": "<path relative to /var/packages/<id>/target/>"}}`, e.g. `"port_conf/<id>.sc"` – the `.sc` file ships **inside `package.tgz`**. | confirmed | [Guide "Port Config"][g-port-config] |
| Where the `.sc` goes | `Acquire()` copies it to `/usr/local/etc/service.d/` (skipped if the file exists); `Release()` removes it and reloads firewall and port forwarding; `Update()` via `synopkghelper update <id> port-config`. Timing `FROM_POSTINST_TO_POSTUNINST`. | confirmed | [Guide "Port Config"][g-port-config]; [Guide "Resource Update"][g-res-update] |
| `.sc` format | INI: `[service_name]` (characters `a-z A-Z 0-9 - \ .` only, no underscore), `title` (required), `desc` (**required**), `port_forward` (optional, default `no`), `src.ports` (optional), `dst.ports` (required, e.g. `8460/tcp`). File name convention `<package>.sc`. | confirmed | [Guide "Port" – "Service Configure File Name", "Section/Key Descriptions"][g-ports] |
| `port_forward` | `yes` lists the ports when the user creates port-forwarding rules from built-in applications (router configuration); `no` (default) does not. The firewall selection list shows the service either way. | confirmed | [Guide "Port"][g-ports] |
| Interaction with `checkport` | Not documented. `servicetool --conf-port-conflict-check --tcp <port>` checks against registered services (needs root). **Assumption**: our own `.sc` does not make `checkport` fail on upgrade. | unconfirmed | [Guide "Port" – "Check port conflict"][g-ports]; check: SPK-12 |

## 8 Package directories

| Question | Answer | Status | Source |
|---|---|---|---|
| Real locations (package on a volume) | `etc` → `/volume<N>/@appconf/<id>` (since 7.0-41330), `var` → `/volume<N>/@appdata/<id>`, `tmp` → `/volume<N>/@apptemp/<id>`, `home` → `/volume<N>/@apphome/<id>`, `target` → `/volume<N>/@appstore/<id>`, all linked from `/var/packages/<id>/`. Only `install_type="system"` puts them under `/usr/local/packages/` on the system partition; we do not set it. So `var` and `tmp` are on the **data volume**. | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs]; [Guide "Optional Fields" – "install_type"][g-info-opt] |
| Owner and modes | Owner `<id>:<id>` with `run-as: package`. Modes: `etc` 0755, `var` 0755, `tmp` 0755, `home` **0700**, `target` 0755. | confirmed | [Guide "Package Filesystem Hierarchy Standard" – table, "Directory Owner Rules"][g-fhs] |
| Lifetime | Created on install and upgrade. Removed: `tmp` and `target` on **uninstall and upgrade**; `etc`, `var`, `home` **never** ("Remove Timing: none"). | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs] |
| Reboot | Not mentioned; the directories are on the data volume, not a tmpfs. **Assumption**: `tmp` survives a reboot, so stale transcode output can be left after a crash or power loss; the server or `start` clears it. | unconfirmed | check: SPK-12 (file in `tmp` before and after a reboot) |
| Is `tmp` cleaned while running? | No: the guide removes it only on uninstall and upgrade. `$SYNOPKG_PKGTMP/transcode` (OD-SPK-7 A) is suitable. | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs] |
| `home` | Exists since 7.0-40759 and is the package user's home (moved from `target` in DSM 7), mode 0700. `HOME=$SYNOPKG_PKGHOME` is valid. | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs]; [Guide "Breaking Changes in 7.0" #5][g-breaking] |

## 9 Wizard

| Question | Answer | Status | Source |
|---|---|---|---|
| File names | `WIZARD_UIFILES/install_uifile`, `upgrade_uifile`, `uninstall_uifile`; language suffix `_<lang>`, German **`install_uifile_ger`**, `upgrade_uifile_ger`, `uninstall_uifile_ger`. | confirmed | [Guide "WIZARD_UIFILES [7.2.2]"][g-wiz] |
| Dynamic wizards | `install_uifile.sh`, `upgrade_uifile.sh`, `uninstall_uifile.sh` (and `_<lang>.sh`) run before the operation and write the JSON to `$SYNOPKG_TEMP_LOGFILE`; script variables are available. | confirmed | [Guide "WIZARD_UIFILES [7.2.2]"][g-wiz] |
| JSON schema (text fields, checkboxes, validators) | The DSM 7.2.2 guide documents only the **new format**: a JSON array of `{"custom_render_fn": "<webpack-built JS>", "custom_render_name": "<name>"}` whose render function is a Vue 2.7.14 component (`pkg-center-step-content`, DSM's `v-form`/`v-input`/`v-checkbox`); validation is own code (disable the Next button in `checkState`); `getValues()` returns `{key: value}` pairs that become script variables. The guide marks the folder with **DSM requirement 7.2.2**. The classic declarative format (`step_title`, `items`, `type: textfield/singleselect/multiselect/password`, `subitems` with `key`, `desc`, `defaultValue`, `validator` with `allowBlank`/`regex`) is **no longer documented**; spksrc packages with a DSM 7.2 minimum still use it (community). **Assumption** for SPK-06: classic JSON (works on 7.2.0–7.2.2 per community use); see Impact 6. | confirmed (7.2.2 format); unconfirmed (classic format on DSM 7.2.x) | [Guide "Wizard UI Files"][g-wiz-intro]; [Guide "WIZARD_UIFILES [7.2.2]"][g-wiz]; [ExamplePackage `PlexMediaInstall.vue`][e-plex] (`getValues`, lines 38–44); community: [spksrc transmission `upgrade_uifile`][c-transmission-upgrade], [spksrc jellyfin `install_uifile`][c-jellyfin-wizard]; check: SPK-12 (install and uninstall wizard in English and German) |
| How values reach the scripts | As environment variables named after the keys, with `true`, `false` or the entered text, in the scripts listed in §5 "Wizard keys". | confirmed | [Guide "WIZARD_UIFILES [7.2.2]"][g-wiz] |
| Shared-folder names (for the validator) | 1–32 characters, case-insensitive; not allowed: `` ! " # % & ' ( ) * + , / : ; < = > ? @ [ ] \ ^ ` { } \| ~ ``; first character not `-` or space, last not space; `$` only as the last character (hidden from SMB); not `.`, `..`, `global`, `lost+found` or other reserved names. Spaces and non-ASCII letters are allowed; a **backslash is not**. | confirmed (Synology KB) | [Synology KB "Create shared folder" – "Shared folder naming rules"][kb-share-names] |

## 10 Uninstall and `var`

| Question | Answer | Status | Source |
|---|---|---|---|
| Does DSM delete `var`? | **No.** `var` (like `etc` and `home`) has remove timing "none"; only `tmp` and `target` are removed on uninstall. | confirmed | [Guide "Package Filesystem Hierarchy Standard"][g-fhs] |
| Can a package keep it on request? | It is kept by default; deleting it is the package's job. OD-SPK-8 A works as decided: uninstall wizard checkbox `wizard_delete_data`, `postuninst` deletes the contents of `var` (and `home`) only when it is `true` **and** `SYNOPKG_PKG_STATUS` is `UNINSTALL` (`postuninst` also runs during upgrades). spksrc does the same (community). | confirmed (DSM side) | [Guide "Package Filesystem Hierarchy Standard"][g-fhs]; [Guide "Script Environment Variables" – `SYNOPKG_PKG_STATUS`][g-env]; community: [spksrc `postuninst`][c-spk-postuninst] |
| Reinstall after keeping data | Not documented whether a reinstall on another volume finds the old `@appdata`. **Assumption**: reinstalling on the same volume reuses `var`, and `postinst` keeps the existing `config.yaml`. | unconfirmed | check: SPK-12 (uninstall unchecked, reinstall, library and database still there) |

## 11 Package log

| Question | Answer | Status | Source |
|---|---|---|---|
| Which file Package Center shows | The guide names no log that Package Center displays for a package in DSM 7. Script output (stdout/stderr of the control scripts) goes to **`/var/log/packages/<id>.log`**; package operations are logged in `/var/log/synopkg.log`; developers should also watch `/var/log/messages`. The `log` action of `start-stop-status` (still present in ExamplePackage) is legacy for DSM < 6 according to spksrc (community). **Assumption**: Package Center shows no package log; the server log in `$SYNOPKG_PKGVAR/<id>.log` is reached over SSH (`sudo tail /var/packages/<id>/var/<id>.log`); messages for the user go through `$SYNOPKG_TEMP_LOGFILE`. | confirmed (file locations); unconfirmed (no log view in Package Center) | [Guide "Breaking Changes in 7.0" #8][g-breaking]; [ExamplePackage `postinst`][e-postinst] (line 9); [ExamplePackage `start-stop-status`][e-sss] (lines 25–29); community: [spksrc `start-stop-status` `log`][c-spk-sss-log]; check: SPK-12 (look for a log view in Package Center) |
| Alternative for later | The `syslog-config` resource installs a logrotate config (and syslog-ng patterns) from `target` during start/stop. | confirmed | [Guide "Syslog Config"][g-syslog] |

## 12 Manual install of an unsigned SPK on DSM 7.2.2

| Question | Answer | Status | Source |
|---|---|---|---|
| Trust-level setting | **None any more**: DSM 7.0 removed keyrings and the trust level; package signing was removed. | confirmed | [Guide "Breaking Changes in 7.0" – "Package Center Changes" #1, "Package Framework Changes" #3][g-breaking]; [Guide "Sign Package (only for DSM6.X)"][g-sign] |
| Warnings | "Any non-synology package will get alert on installation." Packages requiring root are refused unless signed by Synology or a development token is installed; a `run-as: package` SPK is accepted. | confirmed | [Guide "Breaking Changes in 7.0"][g-breaking]; [Guide "System Requirements"][g-sysreq] |
| Steps | Package Center > **Manual Install** > Browse, select the `.spk` > Next > third-party alert (confirm) > `LICENSE` dialog (tick "agree") > install wizard pages > summary with "Run after installation" > Done. The exact wording of the alert and the order of license and wizard pages are not documented. **Assumption**: the order above; the docs (SPK-09) describe the steps without quoting dialog texts. | confirmed (Manual Install, alert, license dialog); unconfirmed (wording, page order) | [Guide "Breaking Changes in 7.0"][g-breaking]; [Guide "License"][g-license]; check: SPK-12 records the dialogs |

## 13 Kernels, CPU features and RAM per family

| Question | Answer | Status | Source |
|---|---|---|---|
| Kernel series per DSM 7.2 platform | x86_64: 3.10.x (avoton, braswell, bromolow, grantley), 4.4.x (apollolake, broadwell, broadwellnk, broadwellnkv2, broadwellntbap, denverton, geminilake, purley, r1000, v1000, kvmx64), 5.10.x (epyc7002, geminilakenk, r1000nk, v1000nk). armv8: 4.4.x (rtd1296, armada37xx), 5.10.x (rtd1619b). armv7: 3.10.x (alpine, alpine4k, monaco, armada38x). **Oldest per family: x86_64 3.10, armv8 4.4, armv7 3.10.** Exact patch levels are not published in the toolkit. **Assumption**: static binaries whose ELF ABI note requires Linux ≤ 3.10 run on every family (Go needs ≥ 3.2). | confirmed (series, toolkit); unconfirmed (patch level) | [Toolkit `platforms` (`AllPlatforms`)][t-platforms]; [Toolkit `toolkit.config` (7.2 platform list)][t-config]; check: SPK-12 (`uname -r` on the owner's x86_64 only) |
| DSM 7.2 models on the oldest platforms | Still on DSM 7.2 (examples): DS416play (braswell), DS3617xs (broadwell), RS18016xs+ (bromolow); DS1517, DS1817 (alpine), DS416 (alpine4k), DS218j and the other armada38x models, DS216play (monaco). Ended at DSM 7.1.1: DS715, DS1515, DS2015xs (alpine), DS215+ (alpine4k), DS415+, DS1815+ (avoton), DS3615xs (bromolow), DS713+ (cedarview). | confirmed (Synology KB / release notes) | [Synology KB CPU table][kb-cpu]; [Synology release-note API][kb-relnotes-api] (queried per model) |
| armv7 floating point and NEON | All DSM 7.2 armv7 platforms are **hard-float**. alpine/alpine4k: Cortex-A15, toolkit flags `-mfpu=neon-vfpv4`; monaco (STiH412): Cortex-A9, `-mfpu=neon`; **armada38x (Armada 385/388): Cortex-A9, toolkit flags only `-mfpu=vfpv3` (no NEON assumed by Synology)**. So binaries for the armv7 SPK may rely on ARMv7 + VFPv3 hard-float (Go `GOARM=7`) but **not on NEON** unless they detect it at run time. **Assumption**: NEON is absent on armada38x. | confirmed (toolkit flags); unconfirmed (NEON present in the Armada 38x silicon) | [Toolkit `platform.alpine`][t-plat-alpine], [`platform.alpine4k`][t-plat-alpine4k], [`platform.monaco`][t-plat-monaco], [`platform.armada38x`][t-plat-armada38x]; check: SPK-11 (QEMU with a Cortex-A9 VFPv3-only model if available) |
| Typical RAM | armv7: 512 MB (DS216, DS216j, DS218j, DS416j, DS416slim, DS419slim, RS217), 1 GB (DS116, RS816, DS216play, DS416), 2–4 GB (DS1517, DS1817). armv8: **256 MB** (DS119j), 512 MB (DS120j, DS220j), 1–2 GB (DS118, DS218, DS218play, DS418, DS418j, DS420j, RS819, DS124, DS223, DS223j, DS423). | confirmed (Synology KB) | [Synology KB CPU table][kb-cpu] |

## 14 Container Manager availability

| Question | Answer | Status | Source |
|---|---|---|---|
| Per family | Container Manager's "Applied Models" list contains **no 32-bit ARM model**. It contains **some armv8 models** – DS120j (armada37xx), DS220j and DS420j (rtd1296), DS124, DS223, DS223j and DS423 (rtd1619b) – and most x86_64 "+", "xs" and FS/SA/HD models, but **not every x86_64 model** (e.g. DS416play is absent). | confirmed (Synology package page) | [Synology "Container Manager" – "Applied Models"][kb-container-manager] |

## 15 QuickConnect, reverse proxy and client addresses

| Question | Answer | Status | Source |
|---|---|---|---|
| QuickConnect and a package port | Synology documents QuickConnect for DSM and **Synology-developed packages** (Audio Station, Video Station, Synology Photos, …); nothing for third-party packages or arbitrary ports. **Assumption**: QuickConnect cannot reach port 8460 directly; it can only reach a package through DSM's own web server (reverse proxy or Login Portal alias). | unconfirmed | [Synology KB "QuickConnect"][kb-quickconnect]; check: SPK-12 (owner tries QuickConnect with `:8460`, expecting failure) |
| DSM reverse proxy | DSM's Login Portal > Advanced > Reverse Proxy forwards requests to a backend host and port, supports custom headers (WebSocket preset) and access-control profiles by source IP or CIDR. The source address the backend sees is not documented. **Assumption**: the server sees **loopback** when the rule targets `localhost`, or **the NAS's own LAN address** when it targets that address – in both cases the LAN-only guard admits every client. DSM's access-control profile (source IP) is the only DSM-side restriction. | confirmed (feature); unconfirmed (source address) | [Synology KB "Login Portal – Advanced" – "Reverse Proxy", "Access Control Profile"][kb-reverse-proxy]; check: SPK-12 (optional; the docs advise against it either way) |

## Impact on ADR 0026

Answers above that contradict or sharpen ADR 0026 or a task file, with proposed wording. The
architect appends the accepted ones as "Notes (SPK-01)" to ADR 0026; this file does not change
the ADR or the task files.

1. **armv7 family is only alpine/alpine4k** (§2 "arch", §3 table, SPK-03 family table, SPK-04).
   ADR §2 says the family value lets "one SPK install on every model of the family"; for armv7
   that misses every armada38x and monaco model. Proposed (ADR §3, after the table): "DSM's
   `armv7` family contains only `alpine` and `alpine4k`; the DSM 7.2 platforms `armada38x` and
   `monaco` are separate `arch` values (guide Appendix A; `plat_to_family` calls them 'armv7 not
   ready'). The armv7 SPK therefore sets `arch="armv7 armada38x monaco"`; that DSM installs it
   on armada38x/monaco is unconfirmed until a community test. Binaries of the armv7 SPK need
   ARMv7 + VFPv3 hard-float only; NEON is not guaranteed (armada38x is built with
   `-mfpu=vfpv3`)." SPK-03: the armv7 row's INFO `arch` value is that list. SPK-04: the armv7
   platform candidate `armada38x` yields family `armada38x`, not `armv7`. (Also relevant to
   SPK-02: the armv7 FFmpeg must not require NEON without run-time detection.)
2. **Build number and version comparison** (§5, OD-SPK-5, SPK-03). The guide requires the build
   number to grow with every version; OD-SPK-5 resets it per feature version
   (`0.1.0-9000` → `0.1.1-1010`). The comparison rule is undocumented; feature-first numeric
   comparison (indirectly supported by `PKG_DEPS`) keeps OD-SPK-5 monotonic. Proposed: keep
   OD-SPK-5 and add to SPK-12: "install a dummy SPK `0.1.0-9000`, then `0.1.1-1010`; Package
   Center must offer it as an upgrade". If the owner prefers to follow the guide literally
   instead (owner decision, changes OD-SPK-5): build =
   (((major·100 + minor)·100 + patch)·10 + stage)·1000 + pre-release·10 + revision, e.g.
   `0.1.0-alpha.2` → `0.1.0-1001020`, `0.1.0` → `0.1.0-1009000`, `0.1.1-alpha.1` →
   `0.1.1-1011010` (fits 2^31−1 up to major 21).
3. **`os_min_ver`** (§5). Proposed: "`os_min_ver` = `7.2-64570` – the first DSM 7.2 build
   released for every 7.2 model (GA was `7.2-64561` on 2023-05-22; `7.2-64570` followed on
   2023-06-20)." Changes to `7.2-72806` only if Impact 6 chooses the Vue wizard.
4. **INFO keys** (§5, SPK-03, SPK-04). Confirmed: `beta="yes"`, `description_ger` (and
   `displayname_ger`). Proposed additions: "`thirdparty` is obsolete (DSM 4.x) and is not set;
   `changelog` is undocumented and not supported by `pkg_dump_info`, so the first SPK omits it;
   `support_move` is not in `pkg_dump_info`'s list and stays unset (default `no`)."
5. **"Open" button** (§8). DSM builds `adminprotocol://<ip>:adminport/adminurl`, so
   `adminurl=/` would give `…:8460//`. Proposed: "INFO `adminport=8460`, `adminprotocol=http`,
   no `adminurl`; SPK-12 confirms the button (fallback `adminurl` empty → `/`, which the server
   tolerates)."
6. **Wizard format** (§7, §10, SPK-06). The DSM 7.2.2 guide documents only the Vue
   render-function wizard (`custom_render_fn`, built with webpack/Vue 2.7.14, DSM requirement
   7.2.2); the classic JSON format is undocumented but used by community packages on DSM 7.2.
   Decision needed. Proposed for SPK-06: "Wizards use the classic JSON format
   (`install_uifile`, `install_uifile_ger`, `uninstall_uifile`, `uninstall_uifile_ger`;
   `textfield` with `validator.allowBlank=false` and a `regex` for share names, `singleselect`
   or `multiselect` checkbox for `wizard_delete_data`). This format is no longer in Synology's
   guide; SPK-12 confirms it on DSM 7.2.2. The Vue format would raise `os_min_ver` to
   `7.2-72806` and add a webpack build; it is the fallback if SPK-12 fails."
7. **`data-share`** (§7, SPK-06). Proposed (ADR §7): "`conf/resource` `data-share` grants
   `{{wizard_media_share}}` read-only to the user `@ID@` (templating is community practice, not
   documented by Synology; SPK-12 confirms). The worker runs before the first start, after
   `postinst`, and **creates a missing share** – so `preinst` must reject a non-existent share.
   DSM creates `/var/packages/<id>/shares/<share>` (since 7.0-41201) when the resource is
   acquired; `postinst` writes that path without testing it. The upgrade wizard does not repeat
   the share (assumption, SPK-12). The grant stays after uninstall."
8. **Scripts during upgrade** (§10, SPK-06). On upgrade DSM runs the old `preuninst`/`postuninst`
   and the new `preinst`/`postinst` with `SYNOPKG_PKG_STATUS=UPGRADE`, without the install or
   uninstall wizard keys. Proposed (SPK-06): "`preinst` validates `wizard_media_share` only
   when `SYNOPKG_PKG_STATUS=INSTALL`; `postuninst` deletes data only when
   `SYNOPKG_PKG_STATUS=UNINSTALL` and `wizard_delete_data=true`; `postinst` writes
   `config.yaml` only on `INSTALL` and only if it is absent. Tests cover the upgrade status for
   all three."
9. **`var` on uninstall** (§10, OD-SPK-8). Confirmed: DSM never removes `var`, `etc` or `home`.
   Proposed: replace "*(verify, SPK-01; if DSM always removes `var`, the wizard says so
   instead)*" with "DSM keeps `var` on uninstall (guide FHS); the checkbox makes `postuninst`
   delete the contents of `var` and `home`."
10. **Package directories** (§6, OD-SPK-7). Confirmed: `var`, `tmp`, `home` live on the install
    volume (`@appdata`, `@apptemp`, `@apphome`), owner `<id>:<id>`, `var`/`tmp` 0755, `home`
    0700; `tmp` is removed only on uninstall and upgrade. Proposed: "`$SYNOPKG_PKGTMP/transcode`
    is confirmed (OD-SPK-7 A); because `tmp` survives a reboot (assumption), stale transcode
    output after a crash is removed at start. `HOME=$SYNOPKG_PKGHOME` (exists since
    7.0-40759)." Drop the "if DSM provides it" condition.
11. **`prestart`/`prestop`** (§9, SPK-05). DSM calls `start-stop-status prestart` (also at boot)
    and `prestop` when `precheckstartstop` is `yes`, which is the documented default. SPK-05's
    "other arguments → 1" could block every start. Proposed (SPK-05): "`prestart` and `prestop`
    exit 0; `log` exits 0 (legacy); other arguments → 1 with a usage line." Optionally INFO sets
    `precheckstartstop="no"` (SPK-03/SPK-04).
12. **Package log** (§9, SPK-05, SPK-09). Proposed (ADR §9): "DSM 7's Package Center shows no
    package log; control-script output goes to `/var/log/packages/<id>.log` (system partition,
    root-readable) and operations to `/var/log/synopkg.log`. The server log stays in
    `$SYNOPKG_PKGVAR/<id>.log`, reachable over SSH; `start-stop-status` keeps its own stdout to a
    few lines. Failure messages for the user go to `$SYNOPKG_TEMP_LOGFILE`. A logrotate config
    through the `syslog-config` resource is a later option."
13. **`.sc` file** (§8, SPK-06, SPK-14). Proposed (ADR §8): "`port-config` points at
    `port_conf/<id>.sc` inside `package.tgz` (path relative to `target`); DSM copies it to
    `/usr/local/etc/service.d/` before `postinst`. The file has section `[@ID@]` (letters,
    digits, `-`, `.` only), `title="@DISPLAY_NAME@"`, **`desc="@DISPLAY_NAME@ media server"`
    (required)**, `port_forward="no"`, `dst.ports="@PORT@/tcp`." SPK-06's
    `package/port/service.sc` is staged by SPK-14 as `port_conf/<id>.sc` in the payload.
14. **Extra files in `scripts/`** (SPK-05, SPK-06, SPK-14). Unconfirmed. Proposed (SPK-05
    Notes): "`common.sh` and `package.env` live in `scripts/` and are located through the
    script's own directory; `preinst` runs before `package.tgz` is extracted, so they must not
    exist only in `target`. If SPK-12 shows that DSM does not keep extra files in `scripts/`,
    SPK-14 inlines both into every script."
15. **INFO-safe values** (SPK-03). Proposed (SPK-03): "Values written to INFO are rejected if
    they contain `"`, `\`, `$`, a backtick or a newline (the toolkit sources INFO as `sh`);
    values of the main field loop (`description`, `displayname`, `maintainer`, …) are also
    rejected if they contain `*`, `?`, `[` or two consecutive spaces (`pkg_dump_info` echoes
    them unquoted)."
16. **Icons** (SPK-08). Confirmed 64 × 64 (`PACKAGE_ICON.PNG`) and 256 × 256
    (`PACKAGE_ICON_256.PNG`), PNG – SPK-08's defaults stand; no change.
17. **Share-name validator** (SPK-06). Proposed: "The validator follows DSM's naming rules:
    1–32 characters; not `` ! " # % & ' ( ) * + , / : ; < = > ? @ [ ] \ ^ ` { } | ~ ``; no
    leading `-` or space, no trailing space; `$` only at the end. A backslash is not a valid
    share name, so the backslash test case is replaced by a `$`-suffixed name; spaces and
    non-ASCII letters stay."
18. **Container Manager** (Context). Proposed: "Container Manager is offered on no 32-bit ARM
    model, on some armv8 models (DS120j, DS220j, DS420j, DS124, DS223, DS223j, DS423) and on
    most, not all, x86_64 models (Synology's Applied Models list)."
19. **RAM** (§3). Proposed: "Many armv7 models have 512 MB; the smallest armv8 model on DSM 7.2
    (DS119j) has 256 MB." The 1 GB minimum in the docs stays.
20. **Reverse proxy and QuickConnect** (§8). Proposed: "Through DSM's reverse proxy the server
    sees loopback (rule targets `localhost`) or the NAS's own LAN address (rule targets that
    address); either way the LAN-only guard admits everyone (assumption). QuickConnect is
    documented only for DSM and Synology packages and is assumed not to reach port 8460."
21. **Privilege and package user** (§7). Confirmed: the package user and group are named `<id>`;
    every script runs as that user. Proposed addition: "No `username`/`groupname` override (the
    `data-share` grant names `@ID@`); a pre-existing DSM user named `<id>` is an unconfirmed
    collision risk (SPK-12 notes it if seen)."
22. **`LICENSE` dialog** (§11, SPK-09). Confirmed: DSM shows `LICENSE` in an "agree" dialog
    during installation. Proposed for SPK-09's install steps: "the GPL-3.0 text appears as a
    licence page that must be accepted; this is not an end-user restriction."

<!-- Guide (DSM 7.2.2 Package Developer Guide) -->
[g-intro]: https://help.synology.com/developer-guide/synology_package/introduction.html
[g-info]: https://help.synology.com/developer-guide/synology_package/INFO.html
[g-info-nec]: https://help.synology.com/developer-guide/synology_package/INFO_necessary_fields.html
[g-info-opt]: https://help.synology.com/developer-guide/synology_package/INFO_optional_fields.html
[g-tgz]: https://help.synology.com/developer-guide/synology_package/package_tgz/package_tgz.html
[g-scripts]: https://help.synology.com/developer-guide/synology_package/scripts.html
[g-env]: https://help.synology.com/developer-guide/synology_package/script_env_var.html
[g-msg]: https://help.synology.com/developer-guide/synology_package/show_massage.html
[g-conf]: https://help.synology.com/developer-guide/synology_package/conf.html
[g-license]: https://help.synology.com/developer-guide/synology_package/license.html
[g-pkgdeps]: https://help.synology.com/developer-guide/synology_package/pkgdeps.html
[g-wiz-intro]: https://help.synology.com/developer-guide/synology_package/wizard/intro.html
[g-wiz]: https://help.synology.com/developer-guide/synology_package/wizard/WIZARD_UIFILES_v2.html
[g-priv]: https://help.synology.com/developer-guide/privilege/preface.html
[g-priv-conf]: https://help.synology.com/developer-guide/privilege/privilege_config.html
[g-res-timing]: https://help.synology.com/developer-guide/resource_acquisition/timing.html
[g-res-update]: https://help.synology.com/developer-guide/resource_acquisition/config_update.html
[g-data-share]: https://help.synology.com/developer-guide/resource_acquisition/data_share.html
[g-port-config]: https://help.synology.com/developer-guide/resource_acquisition/port_config.html
[g-syslog]: https://help.synology.com/developer-guide/resource_acquisition/syslog_config.html
[g-ports]: https://help.synology.com/developer-guide/integrate_dsm/ports.html
[g-fhs]: https://help.synology.com/developer-guide/integrate_dsm/fhs.html
[g-breaking]: https://help.synology.com/developer-guide/breaking_changes.html
[g-pack]: https://help.synology.com/developer-guide/toolkit/pack_stage.html
[g-sign]: https://help.synology.com/developer-guide/toolkit/sign_package.html
[g-sysreq]: https://help.synology.com/developer-guide/getting_started/system_requirement.html
[g-arch]: https://help.synology.com/developer-guide/appendix/platarchs.html

<!-- Toolkit: pkgscripts-ng, branch DSM7.2, commit e4a8d54209a31cef21b7636ad58b1b0e86b5b201 -->
[t-family]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L67-L101
[t-create-scripts]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L108-L120
[t-dump-info]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L180-L207
[t-make-package]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L209-L232
[t-get-spk-name]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L234-L270
[t-make-spk]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/pkg_util.sh#L280-L318
[t-platforms]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/platforms#L13-L40
[t-config]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/toolkit.config#L4
[t-plat-alpine]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/platform.alpine#L17
[t-plat-alpine4k]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/platform.alpine4k#L17
[t-plat-monaco]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/platform.monaco#L18
[t-plat-armada38x]: https://github.com/SynologyOpenSource/pkgscripts-ng/blob/e4a8d54209a31cef21b7636ad58b1b0e86b5b201/include/platform.armada38x#L15

<!-- ExamplePackages (Synology), commit d2849c6fcf14ce72007d14e99a362c3d4f23be0a -->
[e-info]: https://github.com/SynologyOpenSource/ExamplePackages/blob/d2849c6fcf14ce72007d14e99a362c3d4f23be0a/ExamplePackage/INFO.sh#L11
[e-postinst]: https://github.com/SynologyOpenSource/ExamplePackages/blob/d2849c6fcf14ce72007d14e99a362c3d4f23be0a/ExamplePackage/scripts/postinst#L9
[e-sss]: https://github.com/SynologyOpenSource/ExamplePackages/blob/d2849c6fcf14ce72007d14e99a362c3d4f23be0a/ExamplePackage/scripts/start-stop-status#L25-L29
[e-plex]: https://github.com/SynologyOpenSource/ExamplePackages/blob/d2849c6fcf14ce72007d14e99a362c3d4f23be0a/ExamplePackage/WIZARD_UIFILES/src/example/PlexMediaInstall.vue#L38-L44

<!-- Synology Knowledge Center, release notes, package pages -->
[kb-cpu]: https://kb.synology.com/en-global/DSM/tutorial/What_kind_of_CPU_does_my_NAS_have
[kb-relnotes]: https://www.synology.com/en-global/releaseNote/DSM
[kb-relnotes-api]: https://www.synology.com/api/releaseNote/findChangeLog?identify=DSM&lang=en-global&model=DS920%2B
[kb-container-manager]: https://www.synology.com/en-global/dsm/packages/ContainerManager
[kb-quickconnect]: https://kb.synology.com/en-global/DSM/help/DSM/AdminCenter/connection_quickconnect?version=7
[kb-reverse-proxy]: https://kb.synology.com/en-global/DSM/help/DSM/AdminCenter/system_login_portal_advanced?version=7
[kb-share-names]: https://kb.synology.com/en-global/DSM/help/DSM/AdminCenter/file_share_create?version=7

<!-- Community (supporting evidence only): SynoCommunity spksrc, commit 7b99e9d4bba86085d46e5ffe10532e97558fa7cd -->
[c-spk-info]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/mk/spksrc.spk.mk#L200-L292
[c-spk-share]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/mk/spksrc.service.mk#L259-L274
[c-spk-priv]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/mk/spksrc.service.mk#L343-L366
[c-spk-postuninst]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/mk/spksrc.service/installer.dsm7#L181-L198
[c-spk-sss-log]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/mk/spksrc.service/start-stop-status#L191-L205
[c-jellyfin-mk]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/spk/jellyfin/Makefile#L1-L45
[c-jellyfin-wizard]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/spk/jellyfin/src/wizard/install_uifile
[c-transmission-upgrade]: https://github.com/SynoCommunity/spksrc/blob/7b99e9d4bba86085d46e5ffe10532e97558fa7cd/spk/transmission/src/wizard/upgrade_uifile
