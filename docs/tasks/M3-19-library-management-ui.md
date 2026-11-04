# M3-19 Library management in the app (parent: scope, decisions, slices)
Owner: mivo-architect (parent file); slices M3-19a … M3-19o name their own owners
Depends on: the M3 data model tasks (M3-02, M3-04, M3-05 … M3-05d via M3-11b), M3-06, M3-11,
M3-12b; ADR 0028 (written, Accepted 2026-11-04)
Files/areas: none directly; each slice lists its own

## Goal
Owner decision 2026-11-04: libraries are configured in the Mivo web app, not in an installer and
not by editing `config.yaml`. An admin on the local network opens **Settings › Libraries**, adds a
library (kind, name, one or more folders picked in a server-side folder browser, metadata
language, the advanced options milestone 3 supports), edits or removes it, and the server scans
a new library right away. A server with zero libraries shows a call to action that leads there.
When the server process cannot read a folder, the app explains how to grant access – on Synology
DSM through Control Panel › Shared Folder › Edit › Permissions for the package's internal user,
in Docker by mounting the folder. UX ids: AD-40, LS-01, LS-02, LS-03 (M3 subset), SH-18 (entry
only), HO-09 (empty state). This unblocks SPK-17 (install wizard without the folder question) for the
release after 0.1.0-alpha.2.

## Acceptance criteria (end to end; each slice has its own, M3-19o and M3-19l verify these)
- [ ] With zero libraries, Home and the sidebar show an empty state whose button opens
      Settings › Libraries (`/admin/manage/libraries`).
- [ ] Add library: kind tiles (only kinds the server reports as supported; M3: Movies, Shows),
      name (prefilled from the first folder), 1 … 16 folders chosen in the folder browser,
      metadata language, advanced options "Find extras", "Read NFO files", "Use local artwork".
      Saving creates the library, it appears in the sidebar, and its first scan starts at once.
- [ ] Edit library: name, folders, language and options change; the kind cannot change. A folder
      or option change starts a scan; removed folders lose their catalog entries.
- [ ] Remove library (with confirmation): the library, its catalog and its watches are gone; a
      running scan of it is cancelled first.
- [ ] The folder browser lists only directories below the configured browse roots that the
      server process can read; it never leaves a root (`..`, absolute links, junctions, links
      that escape are refused), hides hidden and system entries (leading `.`, names starting
      with `@` such as `@eaDir`, `@appdata`, `@appstore`; `#recycle`, `#snapshot`,
      `$RECYCLE.BIN`, `System Volume Information`, `lost+found`; Windows hidden/system
      attributes) and never lists the config, data, cache or transcode directory.
- [ ] Validation refuses, with a field-specific problem code: a missing, unreadable, duplicate or
      nested folder, a folder shared with or nested in another library, a folder that is, holds
      or lies inside the config, data, cache or transcode directory, a blank or overlong name, an
      unsupported kind, a malformed language.
- [ ] An unreadable folder (and a browse listing with unreadable entries) shows install-specific
      help: Synology – Control Panel › Shared Folder › select the share › Edit › Permissions ›
      "System internal user" › `<package user>` › Read only; Docker – mount the folder into the
      container (`-v /host/path:/media/name:ro`) and restart; other installs – give the service
      user (name shown) read access.
- [ ] Management requests (create, update, delete, browse) are answered only for local peers
      (the LAN classification of ADR 0018 Notes M2-31), **also when `server.lan_only=false`**, and
      only same-origin (a present `Origin` that does not match the request host, or
      `Sec-Fetch-Site: cross-site`, is refused with 403 `forbidden`).
- [ ] Libraries live in the database; `libraries` in `config.yaml` and `<PREFIX>_LIBRARIES` are a
      one-time import seed (Notes "Persistence"). An alpha.2 install (SPK or Docker) keeps its
      library after the upgrade.
- [ ] Tests: per slice; the Playwright flow of M3-19o; the Go integration test of M3-19l.
- [ ] Gates: lint + layer rules + coverage thresholds pass in every slice.

## Slices

| ID | Title | Owner | Depends on | Lane |
|---|---|---|---|---|
| [M3-19a](M3-19a-api-contract-library-management.md) | Contract: library CRUD, folder roots, folder listing + generated code | mivo-backend | M3-02 | L1 |
| [M3-19b](M3-19b-domain-library-management.md) | Domain: library v2 definition, folder rules, browse name policy | mivo-backend | M3-04 | L1 |
| [M3-19c](M3-19c-config-browse-roots-and-library-seed.md) | Config: `browse.roots`, `install.kind`, `libraries` as seed | mivo-backend | M3-11 | L1 |
| [M3-19d](M3-19d-fs-folder-browser.md) | FS: folder browser and folder check | mivo-backend | 19b | L2 |
| [M3-19e](M3-19e-app-library-management.md) | App: create/update/remove/import/browse use cases | mivo-backend | 19b, M3-06 | L2 |
| [M3-19f](M3-19f-sqlite-migration-library-folders.md) | SQLite: migration `00004` library folders, options, import ledger | mivo-backend | M3-11b | L3 |
| [M3-19i](M3-19i-scan-multi-folder-and-options.md) | Scan: several folders per library, per-library options, cancel | mivo-backend | 19b, M3-11b | L3 |
| [M3-19j](M3-19j-scantrigger-dynamic-libraries.md) | Scan triggers: dynamic library set, one watch per folder | mivo-backend | 19b, M3-11b | L3 |
| [M3-19k](M3-19k-http-library-management.md) | HTTP: management handlers + management guard | mivo-backend | 19a, 19e, M3-11b | L3 |
| [M3-19m](M3-19m-web-settings-libraries.md) | Web: Settings › Libraries page, settings entry, empty states | mivo-frontend | 19a, M3-12b | L3 |
| [M3-19g](M3-19g-sqlite-library-store-v2.md) | SQLite: library store v2 (CRUD, folders, ledger) | mivo-backend | 19e, 19f | L4 |
| [M3-19h](M3-19h-sqlite-multi-folder-catalog.md) | SQLite: catalog writer/reader per folder | mivo-backend | 19f, 19i | L4 |
| [M3-19n](M3-19n-web-library-dialog-folder-browser.md) | Web: add/edit dialog, folder browser, permission help | mivo-frontend | 19m | L4 |
| [M3-19l](M3-19l-composition-root-library-management.md) | Composition root, Docker defaults, Go integration test | mivo-backend | 19c, 19d, 19g, 19h, 19i, 19j, 19k | L5 |
| [M3-19o](M3-19o-e2e-library-management.md) | E2E: empty state → add → scan → edit → remove | mivo-frontend | 19l, 19n, M3-18 | L6 |

Lanes and their place in the M3 waves: `docs/tasks/M3-00-overview.md`.

## Notes

### Persistence: database, with the configuration as an import seed (decision for ADR 0028)
Options weighed against ADR 0022 and ADR 0027:
- *Write `config.yaml` from the app.* Rejected: the server would rewrite a user-edited file
  (comments, order, strict parser of ADR 0018), and `<PREFIX>_LIBRARIES` (set by the Docker
  image) would lock the list in the UI (UX principle 8), so Docker users could not manage
  libraries without unsetting the variable.
- *Config libraries locked, app libraries editable (two origins).* Rejected: the alpha.2 SPK
  library would stay locked behind SSH forever, and the Docker default `/media` would block
  every library below `/media` (nesting rule).
- **Chosen: the database is the source of truth.** `libraries` (YAML) and `<PREFIX>_LIBRARIES`
  stay valid and strictly validated (ADR 0018 unchanged), but are an **import seed**: at
  startup every configured entry whose cleaned root is not in the import ledger
  (`library_imports`) becomes a library with one folder, and its root is recorded in the
  ledger. Entries already in the ledger are ignored, whatever happened to that library since
  (renamed, folders changed, removed). An entry whose root is already a folder of another
  library is recorded and skipped with an info log. Removing an entry from the configuration no
  longer deletes anything. Imported libraries are ordinary libraries in the UI (editable,
  removable), with an "Imported from the configuration" hint.
- Migration `00004` seeds the ledger with the roots of every library present at migration
  time, so libraries that came from the configuration before are not imported twice.
- Upgrade from 0.1.0-alpha.2: `00003` (ADR 0027 §5) drops the catalog including `libraries`,
  `00004` then seeds an empty ledger, and the first start imports the configured libraries
  again. Both migrations ship in the same release, so the user sees one catalog rebuild
  (already announced by ADR 0027) and keeps the library definition.
- Library identity becomes the library ID (UUID), no longer the root path. Folders have their
  own IDs; changing a folder path is "remove folder + add folder" (its catalog entries go, the
  new folder is scanned). Item identity across folders follows the match keys of ADR 0027 §3,
  computed from the folder-relative path, so the same show folder in two library folders is one
  show.

### ADRs affected (flagged, not edited by this task)
- **ADR 0028 "Library management in the app" must be written by mivo-architect before lane L1
  starts**, from these Notes (persistence, import seed, management guard, browse roots, install
  kind, several folders, M3 options). It supersedes in ADR 0022: "Library definitions" (OD-1 A:
  defined in the configuration; "a removed path deletes the library"; identity is the root
  path; "Creating libraries from the UI comes with the setup wizard in milestone 7") and the
  consequence "Library management in the UI waits for milestone 7". ADR 0022 is Accepted and
  is not rewritten; it gets a one-line pointer "partly superseded by ADR 0028" as a Notes entry.
- ADR 0027 (Proposed) §5: "the startup sync recreates the libraries" becomes "the first start
  imports the configured libraries"; the catalog tables gain a folder id (`00004`). A Notes
  entry after acceptance.
- ADR 0018 Notes M2-04: `libraries` semantics change to a seed (M3-19c writes a Notes section).
- ADR 0026 §6/§7/§10 and OD-SPK-6: changed by SPK-17 (list in that task).
- UX spec principle "until milestone 7 … the admin area is read-only" gets the library exception
  (done with this task set in `docs/design/ux-spec.md`).

### Security until accounts exist (milestone 7)
ADR 0022 deferred UI library management because an unauthenticated "add library" endpoint lets
any device on the network expose server directories. The owner decision accepts management
before accounts under these limits:
- **Management guard** (M3-19k): local peers only, independent of `server.lan_only`; same-origin
  only (cross-site form posts and scripted requests from other sites in a LAN browser are
  refused); JSON bodies only.
- **Browse roots** bound what can ever become a library: only folders below `browse.roots` can
  be added through the API (ADR 0022 "the API never accepts file system paths" holds in spirit:
  the API takes a browse-root id plus a root-relative path, resolved inside an `os.Root`).
- **Known residual risks**, to be named in ADR 0028 and the docs: reverse proxies and DSM's nginx
  make every client look local (ADR 0018 Notes M2-31); DNS rebinding can make a hostile web page
  same-origin with the server, so a LAN browser visiting such a page could add a library below
  a browse root – the same exposure the read API already has for existing libraries. Both end
  with authentication in milestone 7.

### Browse roots and install kind (M3-19c)
- `browse.roots` (`<PREFIX>_BROWSE_ROOTS`, OS path list): the only directories the folder
  browser starts from. Defaults by `install.kind`: Docker image `/media` (set in the image);
  Synology `/volume1:/volume2:…` (set by the package's start script, SPK-17); Windows: the
  present drive roots; Linux and others: those of `/media`, `/mnt`, `/srv` and the user's home
  directory that exist.
- `install.kind` (`<PREFIX>_INSTALL_KIND`: `standard` | `docker` | `synology`, default
  `standard`) only selects help texts and defaults; it grants nothing.

### Several folders per library (OD-M3-6) - DECIDED 2026-11-04: A (ADR 0028)
The owner asked for "one or more folders" (LS-02). It costs one migration and three backend
slices (M3-19f, M3-19h, M3-19i) because media files, sidecars and local artwork are stored
relative to a folder. Recommendation **A: now** (Plex parity; NAS users spread movies over
volumes). Alternative B: one folder per library in M3 (contract `maxItems: 1`), several in M7;
saves M3-19h and half of M3-19f/M3-19i. Slices follow A until the owner answers.

### Advanced options in M3 (LS-03 subset)
Only options milestone 3 implements: **Find extras** (`scanExtras`), **Read NFO files**
(`readNfo`), **Use local artwork** (`localArtwork`), all default on. Visibility, original
titles, artwork language, trickplay, intro detection, collections and episode ordering stay in
their milestones (LS-03 row). The **metadata language** is stored now and takes effect with
online metadata in M6; the certification country waits for content limits (M7).

### Docker default (OD-M3-7) - DECIDED 2026-11-04: replace (ADR 0028)
The image sets `MIVO_LIBRARIES=/media`. Recommendation: **replace it** by
`MIVO_BROWSE_ROOTS=/media` and `MIVO_INSTALL_KIND=docker`, so Docker starts empty like the SPK
and the user adds `/media` or folders below it in the app. Existing Docker installs lose
nothing they cannot restore with one click: the catalog is rebuilt by `00003` in the same
release anyway, and the release notes say "add your library in Settings › Libraries". If the
owner keeps the variable, M3-19l leaves the image's `MIVO_LIBRARIES` alone (the seed imports
`/media` once and the user can still remove or edit it in the app). The image changes only in
M3-19l, when the management API works end to end, so the Docker release smoke never runs
against an image without a way to add a library.

### Not in this task set
Customize rows (HO-08), "Merge recently added" (AD-40 option), visibility, certification
country, the admin shell with its own sidebar (M5/M7), sign-in, a setup wizard (`/setup`, M7),
writing any other setting from the UI.
