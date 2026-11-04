# M5-81 Read-only dashboard "Now playing"
Owner: mivo-frontend
Depends on: M5-16; prototype `admin.html` approved
Files/areas: `web/apps/web/src/features/admin/**` (new: `components/NowPlayingPage.tsx`, `hooks/useNowPlaying.ts`), routes `/admin/status/*`, locales

## Goal
An admin status page lists active sessions with item, delivery mode and reasons (the same
translated texts as the player overlay), client address class (LAN/remote) and FFmpeg CPU
usage, plus running scans (AD-10, PL-43). Read-only; users and profiles come with M7.

## Acceptance criteria
- [ ] Updates by polling (shared interval helper with M5-23); empty state "Nothing is playing".
- [ ] Reasons use `reasonHints` of M5-62 (public export, no duplicate mapping).
- [ ] Access follows ADR 0028 guard; a forbidden response renders an explanation, not a blank page.
- [ ] No user/device columns yet (M7); table is keyboard accessible with proper headers.
- [ ] Tests: hook with fake API, page states (idle, two sessions, error, forbidden), axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Kill-session is M4 (session management), not here.
