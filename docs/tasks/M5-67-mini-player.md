# M5-67 Mini player
Owner: mivo-frontend
Depends on: M5-61; prototype `detail.html` approved
Files/areas: `web/apps/web/src/features/player/**` (hoist the player host into `app/layout` via a `PlayerHost` provider, `components/MiniPlayer.tsx`, `hooks/usePlayerHost.ts`), `app/router.ts` (player route keeps URL semantics), locales

## Goal
"Minimise" docks the video as a mini player (bottom right on desktop, above the bottom nav on
phones) that keeps playing while the user browses; it shows title, play/pause, progress, expand
and close (PL-38). Only one playback at a time.

## Acceptance criteria
- [ ] Playback continues without restart when minimising and when navigating; expanding goes
      back to the player route at the current position.
- [ ] Close ends the session (server session stopped); starting another item replaces the
      current one.
- [ ] Browser back from the player does not re-create the session when minimised.
- [ ] Keyboard reachable, named controls; mini player never covers the bottom nav.
- [ ] Tests: host state machine in `lib/` table-tested, component tests with fake engine,
      existing PlayerPage tests green.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
The video element must live outside the route tree (host provider), or minimising would reload
it. The music bar (M5-70) reuses the host and the "single playback" rule.
