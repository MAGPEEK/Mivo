# M7-32 Web: Admin › Users (US-20, US-22, US-11)
Owner: mivo-frontend
Depends on: M7-02, M7-30
Files/areas: new `web/apps/web/src/features/users/`, `app/router.ts` (admin route), admin sidebar entry, locale files

## Goal
An admin sees all accounts with their managed profiles and creates, edits, disables, deletes
accounts, resets passwords and sets library access and content limits, as drawn in
`admin.html` (Users).

## Acceptance criteria
- [ ] Users list: name, role, content limit, last activity, status, profile count; managed
      profiles nested under their account.
- [ ] Create/edit dialog: name, initial password (create) or "Reset password" action, role,
      disabled switch, library access (All / selected libraries with checkboxes), content
      limit presets with "Custom" age choice and "Hide unrated titles"; one Save button.
- [ ] Errors shown inline: `last_admin`, `name_taken`, field validation; delete asks for
      confirmation and states that watch state of the account is removed.
- [ ] Route and sidebar entry visible only for admin viewers (`me.currentProfile.isAdmin`);
      non-admins hitting the URL see the not-found page.
- [ ] Tests: Vitest with the stub server (`pnpm -C web test`): create, edit access, last-admin
      error, admin-only visibility; en and de strings.
- [ ] Gates: `pnpm -C web lint` and coverage pass.

## Notes
No invites, guest, or permission toggles (deferred). Mutations through hooks in the feature;
components stay presentational.
