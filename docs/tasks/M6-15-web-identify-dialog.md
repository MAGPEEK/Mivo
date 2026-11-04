# M6-15 Web: Identify dialog, Remove match, Refresh, "not matched"
Owner: mivo-frontend
Depends on: M6-01
Files/areas: `web/apps/web/src/features/identify/**`, small hooks into `features/library` and `features/detail`, i18n strings

## Goal
On a movie, show or album detail page an admin can open "Fix match" (Zuordnung korrigieren),
search, pick the right candidate, apply, remove the match or refresh metadata; unmatched
items are findable.

## Acceptance criteria
- [ ] Detail page menu: "Fix match…", "Remove match", "Refresh metadata"; visible only when the
      management guard allows (as for library management); hidden for episodes/seasons (they
      act on the show).
- [ ] Dialog: search field prefilled with the title and year, results list with poster,
      title, year, overview excerpt and provider badge; accepts a pasted ID/URL; empty and
      error states ("No results", "Provider offline, try again later", "Online metadata is
      switched off – Settings › Metadata").
- [ ] Apply shows progress and then refreshes the item; Remove match asks for confirmation
      and explains that local data stays.
- [ ] Library filter "Not matched" (`matchState=unmatched`) and a subtle badge on unmatched
      items in the list view.
- [ ] Logic in hooks (`useIdentify`), components free of business logic; keyboard and focus
      handling per `docs/design/ux-spec.md`; strings via i18n.
- [ ] Tests: Vitest + Testing Library with the generated client mocked (search, apply,
      error, offline); coverage thresholds hold; lint incl. dependency-cruiser.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
Needs an approved prototype from `mivo-ux` in `docs/design/prototypes/` first; if none exists
the orchestrator requests it before this task starts. Never show provider hosts to the browser;
use the poster URLs from the API only.
