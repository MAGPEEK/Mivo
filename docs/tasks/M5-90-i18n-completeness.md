# M5-90 i18n completeness and localisation
Owner: mivo-frontend
Depends on: M5-80 (run last in each lane; may be repeated as M5-90b)
Files/areas: `web/apps/web/src/**/locales/**`, `app/i18n.ts`, `app/i18n.test.ts`, `lib` formatters for dates/numbers/plurals, ESLint config for i18n rules

## Goal
Every user-visible string of M5 exists in en and de, uses correct plural forms and localised
dates, numbers, durations and file sizes; nothing is hard-coded.

## Acceptance criteria
- [ ] Test enumerates all locale files: key sets of `en` and `de` are identical, no empty
      values, no unused keys (script in `i18n.test.ts`), interpolation variables match.
- [ ] Lint rule or test fails on literal JSX text and hard-coded `aria-label` strings in
      `src/features` and `src/app` (allow-list file documented).
- [ ] Dates, relative times, numbers, durations, plural counts go through `Intl` helpers
      with the active language; tests in en and de (e.g. "1 Staffel / 2 Staffeln").
- [ ] Brand name only through `ui-tokens` brand constant (grep test).
- [ ] Document-language attribute updates with the language switch.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Missing translations are fixed here or reported to the owning task; new languages are not in
scope for M5.
