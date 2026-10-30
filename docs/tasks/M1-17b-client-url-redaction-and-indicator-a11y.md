# M1-17b Close the remaining URL-echo gap and polish the error indicator
Owner: mivo-frontend
Depends on: M1-17
Files/areas: `web/packages/api-client/src/**`, `web/apps/web/src/features/system/**`, `web/apps/web/src/shared/ui/Sidebar.tsx`, `web/apps/web/src/app/layout/**`, `web/apps/web/e2e/support/server.ts`, `web/apps/web/e2e/shell.spec.ts`

## Goal
Base-URL error messages never contain query, fragment or credential text, and the collapsed
error indicator meets WCAG 1.4.13 and announces what it does.

## Acceptance criteria
- [ ] Unparsable and non-http(s) base URLs are never echoed raw: the message names the problem
      ("expected an http(s) URL") and shows at most the scheme, or nothing. Test with
      `ftp://server.test/?email=me@x.y&token=SECRET`, `media.local:8460/?u=a@b&token=SECRET`,
      full-width `＠`, and a 10 000-character input (message length is capped by a named constant).
- [ ] The thrown `TypeError` does not carry the raw input in `cause` (or the cause is a sanitised
      error) – Node's `TypeError: Invalid URL` exposes `cause.input`; test it.
- [ ] The doc comment / `@throws` matches the behaviour exactly.
- [ ] Collapsed indicator button: `aria-expanded={false}` and `aria-controls` pointing at the nav
      (passed through `Sidebar`), so screen readers announce that it expands the sidebar.
- [ ] Tooltip meets WCAG 1.4.13: dismissible with Escape (without moving focus), hoverable
      (no `pointer-events-none` gap), persistent while hovered/focused. Unit tests for Escape.
- [ ] Nits: replace the always-true `queryByTitle` assertion in `AppLayout.test.tsx`; in
      `e2e/support/server.ts` distinguish maxBuffer from timeout, close the log handle if `spawn`
      throws synchronously, and note in a comment that SIGTERM is a hard kill on Windows.
- [ ] Gates: pnpm -C web lint, test (100 % coverage stays), build, e2e (2 runs).

## Notes
- Source: M1-17 round-2 review (minor 1–4, nits) and tester observation about `cause.input`.
- Stale-dir cleanup by mtime (6 h) stays as is; documented trade-off.
