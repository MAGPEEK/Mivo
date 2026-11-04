# M3-19k HTTP: library management handlers and the management guard
Owner: mivo-backend
Depends on: M3-19a, M3-19e, M3-11b (owns `restapi` until then)
Files/areas: `internal/adapters/http/restapi/` new `library_manage.go`, `folders.go`,
`library_manage_response.go` and tests (the M3-19a stubs are replaced; the
`not_implemented` path is deleted when no operation uses it), `restapi/problem.go` (new codes),
`restapi/system.go` (`libraryKinds`), `internal/adapters/http/server/` new
`manageguard.go` + tests (reusing the LAN classifier of `languard.go`)

## Goal
The five management operations answer as the contract says, map every domain error to its
problem code, and are reachable only from the local network and the server's own origin until
accounts exist. UX: AD-40, LS-01 … LS-03 (M3 subset).

## Acceptance criteria
- [ ] Handlers for `createLibrary`, `updateLibrary`, `deleteLibrary`, `listFolderRoots`,
      `listFolders` call the M3-19e use cases; `Library` responses include folders with
      availability, options, language, `importedFromConfig` and scan status; create answers 201
      with `Location`; the scan status in the response already shows the started scan.
- [ ] Error mapping: folder errors → 400 with code (`folder_not_found`, `folder_unreadable`,
      `folder_overlap` with `libraryId`, `folder_reserved`, `folder_outside_roots`) and `field`
      `folders[i]`; kind, name, language, folder count → their codes; unknown library → 404;
      store conflict → 409 `folder_overlap`; browse: bad path 400, unknown root or missing folder
      404, timeout 503 `unavailable`. Domain errors are mapped only here (CLAUDE.md).
- [ ] **Management guard** (middleware on exactly these five operations plus the existing scan
      trigger `POST …/scan`): refuses with 403 `forbidden` and a fixed detail when the socket
      peer is not local by the classification of ADR 0018 Notes M2-31 – **also when
      `server.lan_only=false`**; when an `Origin` header is present and its scheme+host+port do
      not equal the request's (`Host`), or `Sec-Fetch-Site` is `cross-site`; and, for bodies,
      when the content type is not `application/json`. `X-Forwarded-*` headers are ignored
      (unchanged rule).
- [ ] `SystemInfo.libraryKinds` lists the kinds `cmd` passes in (M3: `movies`, `shows`).
- [ ] Tests: handler tests with fake use cases for every status and code; guard tests with
      `RemoteAddr` local/remote/IPv4-mapped/on-link IPv6, `lan_only` on and off, matching and
      foreign `Origin`, `Sec-Fetch-Site` values, missing `Origin` (allowed: same-origin `fetch`
      and non-browser clients send none for GET), wrong content type; read endpoints
      (`GET /libraries`) unaffected by the guard.
- [ ] Gates: `go vet ./...` and `go vet -tags webui ./...`, `golangci-lint run` (+
      `--build-tags webui`), `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/http/...`, coverage gate.

## Notes
- Problem detail texts are English and contain no absolute server paths except the folder path
  the client itself chose (it is the admin's own input).
- The guard's detail text: "Library management is only available from the local network until
  accounts exist." Document the residual risks (reverse proxies, DNS rebinding) in code comments
  pointing at ADR 0028.
- The web app on another origin (Vite dev server on :5173) proxies `/api`, so the browser's
  `Origin` is the dev server: the proxy must rewrite `Origin` (Vite `changeOrigin` does not)
  or the guard must accept the configured dev origin. Prefer documenting
  `MIVO_DEV_API_TARGET` + a Vite proxy header rewrite in `web/apps/web/vite.config.ts` – that
  file belongs to M3-19m; list the requirement in the REPORT so M3-19m picks it up.

## Orchestrator notes from the M3-19a review (2026-11-04)

Acceptance additions for this task:
- Mappers fill `folders` (non-nil, ≥ 1), `libraryKinds`, `metadataLanguage`, `options`, `unavailableFolderCount`; no `null` arrays.
- `PATCH /libraries/{id}` rejects a body containing `kind` with 400 (`DisallowUnknownFields` or explicit check) – the generated decoder does not enforce `additionalProperties: false`; `LibraryUpdate.options` needs all three booleans.
- Add constants and a pin test in `restapi/problem.go` for the nine new problem codes (`folder_*`, `kind_unsupported`, `name_invalid`, `language_invalid`, `too_many_folders`) mapped from the M3-19b domain errors (`ReasonPath`, `reserved`, `duplicate`, `nested`, `overlap`); `folder_overlap` is 409 with `libraryId`.
- Absolute `LibraryFolder.path`/`FolderRoot.path` become admin-only when accounts arrive (M7); keep `FolderEntry`/`FolderListing`/`FolderRef` paths root-relative.
- Spec nits: reword stale `Library.id`/`name` descriptions ("from the configuration"); assert the 1..16 folders bound on `LibraryUpdate.folders` too; "4096 characters" instead of bytes.
