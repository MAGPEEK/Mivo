# M7-39 Docs: first run, recovery, upgrade notes, spec updates
Owner: mivo-backend
Depends on: M7-25, M7-26
Files/areas: `docs/development.md`, `packaging/docker/README.md`, Synology guide/`TESTING.md` under `packaging/synology/`, `docs/design/ux-spec.md` (milestone column), `docs/plan.md` (status lines), release-notes draft under `docs/`

## Goal
Owners know that the first visit creates the admin, how to recover a lost password on Docker,
Synology and Windows, and what changes on upgrade from an alpha.

## Acceptance criteria
- [ ] Docker and Synology guides: first-run setup from the local network, `docker exec … admin
      reset-password`, SSH variant for DSM, data directory to back up (manual backup until
      AD-29).
- [ ] Upgrade note: catalog kept, setup wizard appears once, existing bookmarks need sign-in.
- [ ] `ux-spec.md`: US-25 milestone M7 (moved from M7b), deferred US/AD ids marked "after 0.1.0"
      per the overview; plan status line for milestone 7.
- [ ] Release-notes draft lists the M7 features and the deferred items.
- [ ] Tests: none (docs); links resolve (`pnpm -C web lint` unaffected).
- [ ] Gates: docs only; reviewer checks consistency with ADR 0032.

## Notes
Keep the brand name out of command examples where the binary name may change; use the
placeholders the guides already use.
