# M7-31 Web: profile picker, PIN pad, profile menu (US-01…03, SH-07)
Owner: mivo-frontend
Depends on: M7-30
Files/areas: new `web/apps/web/src/features/profiles/`, the TopBar profile menu slot in `web/apps/web/src/shared/ui` (or `app/layout`), `app/router.ts` (one route), locale files

## Goal
After sign-in the user picks a profile; PIN profiles ask for the 4-digit PIN on a pad usable by
keyboard, pointer and TV remote; the profile menu switches profile and signs out, as drawn in
`admin.html`.

## Acceptance criteria
- [ ] `/profiles` shows tiles (avatar initial on the token colour, name, lock icon for PIN,
      admin marker); "Add profile" for the account owner links to M7-33's editor.
- [ ] PIN pad: 4 digits, digit keys and arrow/Enter navigation, Backspace, auto-submit on the
      4th digit, wrong-PIN shake and message, lockout message with countdown from
      `retryAfterSeconds`; "Remember on this device" (default off for PIN profiles).
- [ ] Profile menu (SH-07) in the top bar: avatar, current profile, content-limit badge,
      "Switch profile", "Change password" (M7-30 dialog), "Sign out"; a hint to set a PIN on
      the own profile when managed profiles exist (ADR 0032 §4).
- [ ] After selection the app returns to `next` or Home and refetches catalog queries.
- [ ] Tests: Vitest (`pnpm -C web test`): keyboard-only PIN entry, lockout countdown, remember
      default, menu actions; axe check of the picker has no violations.
- [ ] Gates: `pnpm -C web lint` and coverage pass.

## Notes
PIN pad logic (digit buffer, submit, lockout timer) lives in a hook or a small framework-free
module so the webOS app can reuse it later (`packages/player-core` is not the place; keep it in
the feature unless a shared package is agreed).
