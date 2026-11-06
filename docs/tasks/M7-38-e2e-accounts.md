# M7-38 E2E: setup, sign-in, PIN, content limit, resume
Owner: mivo-frontend
Depends on: M7-25, M7-31, M7-33, M7-34, M7-37
Files/areas: new specs under `web/apps/web/e2e/` (accounts, parental, resume), corpus metadata needs noted in the spec if a rated fixture is missing

## Goal
The core M7 flows are covered by Playwright against the real server binary, including the plan's
"Profil mit PIN wählen und Altersgrenze prüfen" case.

## Acceptance criteria
- [ ] Fresh server: setup wizard creates the admin and reaches "No libraries yet"; sign out;
      sign in again; wrong password shows the error.
- [ ] Admin creates a managed profile "Kid" with PIN and the "Children" limit; switching to it
      needs the PIN; 5 wrong PINs show the lockout.
- [ ] As "Kid", a title rated above the limit appears neither in the library list, nor in
      search suggestions/results, nor by direct link (`/items/<id>` shows not found), nor in
      Home rows.
- [ ] Resume: play a movie for > 30 s, leave, the detail page shows "Resume" and Home shows it
      in Continue watching; as another profile it does not.
- [ ] Devices: a second browser context signs in; revoking it from the first forces sign-in in
      the second.
- [ ] Tests: `pnpm -C web e2e` green (corpus generated first); runtime added ≤ 2 minutes.
- [ ] Gates: `pnpm -C web lint` passes.

## Notes
Use the M7-28 API helper for account setup where the UI is not under test. The corpus needs at
least one item with an FSK 16+ NFO rating and one with FSK 0; if `scripts/gen-testmedia` lacks
them, add NFO sidecars in the spec's temp library instead of changing the corpus.
