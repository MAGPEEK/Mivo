# M5-36 Browse by folder
Owner: mivo-frontend
Depends on: M5-31
Files/areas: `web/apps/web/src/features/library/**` (`components/FolderBrowser.tsx`, `lib/folderPath.ts`), route `/libraries/$libraryId/folders/*` in `app/router.ts`, locales

## Goal
Type "Folders" shows the library's folder tree with a breadcrumb from the library root;
folders open, files open their item (LB-21).

## Acceptance criteria
- [ ] Route splat path is parsed and validated in `folderPath.ts` (no `..`, no absolute
      paths, no empty segments; table tests); invalid paths show the 404 state.
- [ ] Breadcrumb links to every ancestor; only relative names are ever displayed.
- [ ] Folders and items in one list/grid reusing the M5-33 views; unavailable items marked.
- [ ] Back/forward and reload restore the folder.
- [ ] Tests: path lib, component with fake API, axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Contract in M5-10; if the API returns an absolute path anywhere, report a bug instead of
hiding it client-side.
