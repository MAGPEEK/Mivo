# M3-19a Contract: library management and folder browser
Owner: mivo-backend
Depends on: M3-02, ADR 0028 written (M3-19 Notes)
Files/areas: `api/openapi.yaml`, `api/spec_test.go`, `internal/adapters/http/apigen/**`
(regenerated), `internal/adapters/http/restapi/**` (only `not_implemented` stubs for the new
operations, like M3-02), `web/packages/api-client/**` (regenerated)

## Goal
The API contract describes adding, editing and removing libraries and browsing server folders,
so the backend handlers (M3-19k) and the web app (M3-19m, M3-19n) are built against one fixed
shape from the start. UX: AD-40, LS-01, LS-02, LS-03 (M3 subset).

## Acceptance criteria
- [ ] `Library` gains `folders` (array of `LibraryFolder {id, path, available}`, ≥ 1, ordered),
      `metadataLanguage` (string, empty = server default), `options`
      (`LibraryOptions {scanExtras, readNfo, localArtwork}`, booleans) and `importedFromConfig`
      (boolean). The existing single root field stays, documented as deprecated and equal to the
      first folder's path.
- [ ] `ScanStatus` gains `unavailableFolderCount` (integer ≥ 0).
- [ ] `POST /api/v1/libraries` (`createLibrary`): body `LibraryCreate {kind, name,
      folders: FolderRef[1..16], metadataLanguage?, options?}`, `additionalProperties: false`;
      201 with `Library` and `Location`; 400, 403, 409, 500 problems.
- [ ] `PATCH /api/v1/libraries/{libraryId}` (`updateLibrary`): body `LibraryUpdate {name?,
      folders?, metadataLanguage?, options?}`, `additionalProperties: false` (so `kind` is
      refused), at least one property; 200 `Library`; 400, 403, 404, 409, 500.
- [ ] `DELETE /api/v1/libraries/{libraryId}` (`deleteLibrary`): 204; 403, 404, 500.
- [ ] `FolderRef`: either `{folderId}` (keep an existing folder of this library) or
      `{rootId, path}` (`path` root-relative, `/`-separated, empty = the root itself, ≤ 4096
      bytes). Modelled as one object with optional properties (the server rejects any other
      combination with `invalid`), not `oneOf`, to keep the generated Go and TS types simple.
- [ ] `GET /api/v1/folder-roots` (`listFolderRoots`): `{roots: [{id, path, available}],
      installKind: standard|docker|synology, serviceUser: string}`; 403, 500.
- [ ] `GET /api/v1/folder-roots/{rootId}/folders?path=` (`listFolders`): `{rootId, path,
      parentPath (nullable), folders: [{name, path, selectable, reason?}], unreadableCount,
      truncated}`; `reason` ∈ `reserved | in_use`; 400 (bad path), 403, 404 (unknown root or
      missing folder), 503 `unavailable` (root does not answer in time).
- [ ] `SystemInfo` gains `libraryKinds` (array of the library kinds the server accepts now).
- [ ] Problem codes documented in the spec and listed in `spec_test.go`: `folder_not_found`,
      `folder_unreadable`, `folder_overlap`, `folder_reserved`, `folder_outside_roots`,
      `kind_unsupported`, `name_invalid`, `language_invalid`, `too_many_folders`; problems for
      a folder carry `field` (e.g. `folders[1]`) and, for `folder_overlap`, `libraryId` of the
      other library when there is one.
- [ ] `go generate ./...` and `pnpm -C web gen` output committed; `git diff --exit-code` clean
      after regenerating (ADR 0017). The restapi package compiles with 501 `not_implemented`
      stubs for the five new operations.
- [ ] Tests: `api/spec_test.go` checks the new operation ids, the problem codes, that
      `LibraryCreate`/`LibraryUpdate` forbid additional properties and that `kind` is absent
      from `LibraryUpdate`.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `go test ./api/... ./internal/adapters/http/...`,
      `pnpm -C web lint`, `pnpm -C web test`, coverage thresholds.

## Notes
- Naming follows the existing spec (camelCase properties, `libraryId` path parameter).
- Limits are named in the spec description and become constants in the domain (M3-19b):
  16 folders, name 1–128 characters (the existing `library.MaxNameLength`), listing capped at
  1000 entries (`truncated`).
- The folder listing returns directories only; files are never listed.
- Do not change existing operations beyond the additive fields above; the web app keeps
  compiling against the regenerated client (`pnpm -C web lint` proves it).
