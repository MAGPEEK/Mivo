# M3-19m Web: Settings › Libraries page, settings entry, empty states
Owner: mivo-frontend
Depends on: M3-19a, M3-12b (owns `router.ts` changes in wave 3)
Files/areas: `web/apps/web/src/features/settings/**` (new feature: `libraries/` page, hooks,
api queries and mutations, locales en/de, tests), `web/apps/web/src/app/router.ts` (one route),
`web/apps/web/src/shared/ui/Sidebar.tsx` and its test (settings entry, empty-library link),
`web/apps/web/src/features/home/components/HomePage.tsx` and its test (empty state),
`web/apps/web/src/shared/i18n/resources.ts` (register the namespace),
`web/apps/web/vite.config.ts` (dev proxy `Origin` rewrite, see M3-19k Notes)

## Goal
An admin finds **Settings › Libraries**, sees every library with its folders and state, can scan,
edit (the dialog arrives in M3-19n) and remove a library, and a server without libraries leads
the user there from Home and the sidebar. UX: AD-40 (M3 part), SH-18 (entry only), HO-09.

## Acceptance criteria
- [ ] Route `/admin/manage/libraries` (UX spec section 1) with the page heading "Libraries" and a
      breadcrumb "Settings › Libraries"; a "Settings" entry at the end of the sidebar (SH-18)
      links to it.
- [ ] List: kind icon, name, folder paths (each with an "unavailable" marker), item count, last
      scan result and time, "Imported from the configuration" hint where set; actions **Scan**
      (existing scan mutation), **Edit** (opens the M3-19n dialog; until it lands a disabled
      button), **Remove** (confirmation dialog naming the library and saying the catalog is
      removed and media files are not touched) and a primary **Add library** button.
- [ ] Remove calls `deleteLibrary`, then invalidates the library queries; the sidebar and Home
      update without reload; errors shown inline with the problem detail.
- [ ] Empty states: with zero libraries Home shows "No libraries yet" with an **Add library**
      button to the settings page (opening the add dialog via a search param, e.g.
      `?add=1`, once M3-19n exists), and the sidebar's library section shows the same link.
- [ ] A 403 `forbidden` from a management call shows "Library management is only available from
      the local network until accounts exist" (localised) instead of a generic error.
- [ ] All strings in en and de (namespace `settings`); wording per UX spec section 14.
- [ ] Vite dev proxy: management calls through `pnpm -C web dev` pass the server's guard (the
      proxy rewrites `Origin` to the target); covered by a short note in `docs/development.md`
      only if the dev command changes.
- [ ] Tests (Vitest + Testing Library, fake API of `src/test/fakeApi.ts`): list rendering incl.
      unavailable folder and imported hint, remove flow with confirmation and cancel, 403
      message, Home and sidebar empty states with working links, keyboard access to every
      action; axe checks of the existing pattern.
- [ ] Gates: `pnpm -C web lint` (ESLint, dependency-cruiser, Prettier, tsc), `pnpm -C web test`
      with coverage thresholds, `pnpm -C web build`.

## Notes
- Components render, hooks decide (CLAUDE.md): mutations, invalidation and error mapping live in
  hooks; the page has no business logic.
- The admin shell with its own sidebar is M5/M7; this page uses the normal shell.
- Shared files: `router.ts` and `Sidebar.tsx` are touched once each and only for this entry.
