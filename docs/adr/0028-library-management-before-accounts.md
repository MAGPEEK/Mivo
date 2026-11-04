# 0028 Library management in the app, before accounts

Status: Accepted (2026-11-04, owner delegated the decision: "arbeite alles selber ab")

## Context
ADR 0022 defined libraries in the configuration only and deferred UI management to the setup
wizard in milestone 7, because an unauthenticated "add library" endpoint lets any LAN device
expose server directories. The owner now wants libraries managed in the web app
(Settings > Libraries) and no folder question in installers. Details: `docs/tasks/M3-19-library-management-ui.md`.

## Decision
1. **Persistence.** The database is the source of truth. `libraries` (YAML) and
   `<PREFIX>_LIBRARIES` stay strictly validated (ADR 0018) but are a **one-time import seed**:
   at startup each configured entry whose cleaned root is not in the import ledger
   (`library_imports`) becomes a library with one folder and is recorded. Ledger entries are
   ignored afterwards, whatever happened to the library. Removing an entry deletes nothing.
   Migration `00004` seeds the ledger from existing libraries.
2. **Identity.** Library ID (UUID), no longer the root path. Folders have their own IDs;
   changing a path is remove + add. Item match keys (ADR 0027 §3) use folder-relative paths.
3. **Several folders per library** (OD-M3-6 = yes, 1 to 16). Catalog tables gain a folder id
   in migration `00004`.
4. **Security before accounts.**
   - Management requests (create, update, delete, browse) are served to local peers only
     (ADR 0018 Notes M2-31 classification), **even with `server.lan_only=false`**, and only
     same-origin (mismatching `Origin` or `Sec-Fetch-Site: cross-site` gives 403); JSON only.
   - Only folders below `browse.roots` can become libraries; the API takes a root id plus a
     root-relative path, resolved inside an `os.Root`.
   - No symlink/junction escape, `..` refused; hidden and system folders (`.*`, `@*`,
     `#recycle`, `#snapshot`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`) are
     hidden; config, data, cache and transcode directories are never listed or accepted.
5. **Install kind.** `install.kind` (`standard|docker|synology`) selects help texts and browse
   root defaults only; it grants nothing.
6. **Docker** (OD-M3-7 = yes): the image starts without a library; `MIVO_BROWSE_ROOTS=/media`
   and `MIVO_INSTALL_KIND=docker` replace `MIVO_LIBRARIES=/media`.
7. **Accepted residual risks until M7 (authentication):** reverse proxies and DSM nginx make
   every client look local; DNS rebinding can make a hostile page same-origin, so a LAN browser
   could add a library below a browse root (the read API has the same exposure today).

8. **Scan coordination.** Before the library manager updates or deletes a library it calls, in
   this order: `Suspend(libraryID)` (a small port declared by the manager, satisfied by
   `*scantrigger.Trigger`, so no trigger starts a scan on the old definition), `Cancel`,
   `WaitIdle`, the store change, then the change event (`Added`/`Changed`/`Removed`) to the
   Trigger, which lifts the suspension, and on removal `scan.Service.Forget(id)` to drop the
   in-memory options and refresh state. `HoldTimeout` (2 min) only guards against a change that
   never arrives.

## Consequences
- Supersedes ADR 0022 "Library definitions" (config-only, path identity, "a removed path
  deletes the library", UI creation in M7). Rest of ADR 0022 stands.
- Needs migration `00004`, a dynamic scan-trigger set, and a management guard in `adapters/http`.
- Upgrade from alpha.2: `00003` drops the catalog, `00004` keeps the libraries (same IDs, one
  folder each) and seeds the import ledger from them; only the catalog is rebuilt by the next
  scan, so users see one rebuild.
- Docker users add their library in the app after the upgrade.
- Import skips a configured library whose name another library already uses (warn log) and still records its root, so it is not retried; the user adds it in the app.
