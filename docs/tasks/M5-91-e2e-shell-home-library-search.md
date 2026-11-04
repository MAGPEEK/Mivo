# M5-91 E2E: shell, home, library, search
Owner: mivo-frontend
Depends on: M5-25, M5-34, M5-51
Files/areas: `web/apps/web/e2e/shell.spec.ts`, `home.spec.ts`, `library-browse.spec.ts`, `search.spec.ts`, helpers

## Goal
Playwright covers the core browse flows against the real server and the test corpus.

## Acceptance criteria
- [ ] Home shows rows for the corpus libraries; "See all" lands on Browse with the right filter.
- [ ] Browse: sort, one quick filter, one field filter with chip removal, view switch, A-Z jump
      (corpus is small: assert offsets, plus a seeded 2,000-item library via the API/fixture
      helper for virtualisation).
- [ ] Sidebar: pin, unpin, reorder with keyboard, reload keeps layout, reset.
- [ ] Search: Ctrl+K, dropdown results, Enter opens results page, pivots work.
- [ ] Phone viewport (390 px): bottom navigation and filter sheet usable.
- [ ] Axe scan on Home, Browse, search results.
- [ ] Gates: `pnpm -C web e2e` green; `pnpm -C web lint`.

## Notes
Fixtures only through test helpers, no sleeps; use web-first assertions.
