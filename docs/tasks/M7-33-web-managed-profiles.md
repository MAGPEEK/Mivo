# M7-33 Web: managed profiles editor (US-10)
Owner: mivo-frontend
Depends on: M7-32
Files/areas: `web/apps/web/src/features/users/profiles/` (new subfolder), entry points in the users list and the profile picker, locale files

## Goal
An admin, and an account owner for their own account, create and edit managed profiles with
name, avatar colour, optional PIN, content limit and library subset.

## Acceptance criteria
- [ ] Editor per `admin.html` "Create managed profile": name, avatar colour tokens, PIN
      (4 digits, set/change/remove), content limit presets + custom + hide unrated, libraries
      (subset of the account's access; others shown disabled with a hint), the server-side
      enforcement note (`u.enforce`).
- [ ] Reached from Admin › Users (any account) and from "Add profile" in the picker / own
      settings (own account); delete with confirmation; `profile_limit` error shown.
- [ ] Tests: Vitest (`pnpm -C web test`): PIN validation, subset rule, owner vs admin entry,
      delete flow.
- [ ] Gates: `pnpm -C web lint` and coverage pass.

## Notes
"Remote access allowed" from the prototype is M7b: hide it (prototype decision 4 style: no
placeholders in the release).
