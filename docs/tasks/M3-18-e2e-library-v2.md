# M3-18 E2E: posters and show → season → episode
Owner: mivo-frontend
Depends on: M3-11b, M3-12b, M3-17
Files/areas: `web/apps/web/e2e/library-v2.spec.ts` (new), `web/apps/web/e2e/support/**`
(`server.ts`: optional config file with typed libraries; new `libraryCorpus.ts`),
`web/apps/web/package.json` only if a script is needed

## Goal
The core milestone-3 flow runs in a real browser against the real server: a movies and a
shows library are scanned, posters load from the image endpoint, a show opens with its
seasons, a season lists its episodes in order, an episode links to its neighbours and plays,
and the absolute-numbered show shows one season. UX: LB-40, LB-44, DT-20 … DT-22, DT-24.

## Acceptance criteria
- [ ] The E2E server support can start the binary with a generated `config.yaml` that defines
      typed libraries (`movies`, `shows`) pointing at copies of `testdata/library/{movies,
      shows}`; the `<PREFIX>_LIBRARIES` variable is unset for such runs (it would replace
      the list, ADR 0018). Existing specs keep their current setup.
- [ ] The suite skips with a clear message when `testdata/library/manifest.json` is missing.
- [ ] Movies library: after the scan, the grid shows one card per expected movie (versions
      appear once), the NFO title, and at least one poster `<img>` whose request hits
      `/api/v1/images/…?width=…&tag=…`, returns 200 `image/jpeg` and decodes
      (`naturalWidth > 0`); a movie without artwork shows the placeholder.
- [ ] Shows library: the show card opens the show page with seasons in order and
      `Specials` last; the season page lists episodes ascending, `?order=desc` reverses
      them after a reload; the episode page's previous/next arrows navigate; Play on an
      episode starts playback (reuse the M2 player assertions for one mode).
- [ ] **DT-24:** the "Dragon Ball Super" show page shows exactly one season with 131 episodes
      (count from the season label and the paged list).
- [ ] A second image request for the same URL is served from the browser cache or answered
      304 (checked through the request log of the page).
- [ ] Axe checks (existing helper) on the show, season and episode pages without blocking
      violations.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web e2e` (bundled Chromium; the
      branded channel is not needed for these specs).

## Notes
- Scanning 140 small files takes a while with two probes at a time; give the scan wait a
  generous, named timeout and poll the library status like the M2 support code.
- Keep fixtures out of the repository: copy from `testdata/library` into the run's temp root
  as the M2 suite does with the media corpus.
- Record the run time of the new spec in the REPORT; if it exceeds two minutes, propose a
  smaller DT-24 fixture for E2E while the Go integration test (M3-11b) keeps the full 131.
