# M7-30 Web: session, sign-in, setup step, route guard, sign out
Owner: mivo-frontend
Depends on: M7-01, M7-29 (prototype approved)
Files/areas: new `web/apps/web/src/features/auth/`, `web/apps/web/src/app/router.ts` (guard + routes), the shared API fetch wrapper (401 handling), locale files

## Goal
A visitor of a fresh server is led through "Create admin account" into the existing library
setup; later visitors sign in; an expired session returns to sign-in without losing the target
URL (AU-* ids of M7-29).

## Acceptance criteria
- [ ] `useSession` hook (TanStack Query on `GET /me` and `GET /setup`) exposes
      `setupRequired | signedOut | needsProfile | ready`; components hold no logic.
- [ ] Router guard: `setupRequired` → `/setup`, `signedOut` → `/sign-in?next=<url>`,
      `needsProfile` → `/profiles` (route owned by M7-31; until then a placeholder), else the
      route; `next` accepts only same-app relative paths (no open redirect).
- [ ] Sign-in and setup forms per prototype, incl. throttling countdown and the remote-peer
      message; change password dialog; "Sign out" action exported for the profile menu.
- [ ] Any API `401` clears cached queries and routes to sign-in; `403 profile_required` routes to
      the picker.
- [ ] Tests: Vitest + Testing Library with the stub server (`pnpm -C web test`): guard
      transitions, `next` sanitising (`//evil`, `https://…`, `/\evil`), form validation, 401
      handling; en and de strings present.
- [ ] Gates: `pnpm -C web lint` (ESLint, dependency-cruiser, Prettier, tsc) and coverage pass.

## Notes
No token storage in `localStorage`: the cookie is `HttpOnly`. The web client always sends
`client: "web"`.
