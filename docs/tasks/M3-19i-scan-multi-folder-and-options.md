# M3-19i Scan: several folders per library, per-library options, cancel
Owner: mivo-backend
Depends on: M3-19b, M3-11b (scan v2 is the only pipeline then)
Files/areas: `internal/app/scan/**` (scan v2 pipeline after M3-11b: library lookup port, run,
placement input, metadata/artwork steps, service `Cancel`, fakes and tests)

## Goal
A scan walks every folder of a library, keeps the catalog of a folder that is temporarily
unavailable, honours the library's options (extras, NFO, local artwork) and can be cancelled
for one library, so library management can change and remove libraries safely.

## Acceptance criteria
- [ ] The scan reads the library through a port returning the M3-19b `Library` (kind, folders,
      options) at the start of each scan, so edits apply to the next scan without a restart.
- [ ] Every folder is walked; placement and match keys use the folder-relative path (ADR 0027
      §3), so equal show or movie folders in two library folders become one item; every file,
      sidecar and local artwork record carries its folder id (`RootID`) for the writer.
- [ ] A folder that is missing or lost during its walk keeps all its catalog entries; the
      other folders are scanned normally; the scan finishes with `unavailableFolderCount` set.
      Only when **every** folder is unavailable does the scan fail with `root_unavailable`
      (ADR 0022 rule, unchanged for one-folder libraries).
- [ ] Options: `ScanExtras=false` ignores extras (files placed as extras are not stored, so
      existing extras disappear); `ReadNFO=false` skips the NFO source in the merge;
      `LocalArtwork=false` skips local artwork candidates (embedded artwork stays). When the
      options differ from those of the library's last finished scan, every item of the library
      is refreshed once (metadata and artwork), not only changed items.
- [ ] `Service.Cancel(ctx, libraryID)` cancels a running or queued scan of that library and
      returns when it has stopped or ctx is done; the scan ends with `scan_canceled` and
      deletes nothing it had not already justified (ADR 0022 Notes M2-19).
- [ ] Tests with fakes: two folders with merged show and movie items, one folder unavailable
      (entries kept, count 1), all unavailable (`root_unavailable`), each option off and the
      one-time full refresh after an option change, cancel of running and queued scans, record
      `RootID`s; existing scan tests stay green.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/scan/...`, coverage gate.

## Notes
- Remember the options of the last finished scan in the scan status (memory) or in the catalog –
  memory is enough: after a restart the startup scan refreshes changed items only, which is
  correct because the stored metadata already reflects the options in force.
- The sqlite writer ignores `RootID` until M3-19h; until then multi-folder libraries are not
  wired (M3-19l), so no wrong row can be written.
- The walker (`libraryfs.Walk`) is per root already; no adapter change is expected – report if
  one is needed.
