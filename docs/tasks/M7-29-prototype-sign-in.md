# M7-29 Prototype: sign-in, first-run setup, change password
Owner: mivo-ux
Depends on: none
Files/areas: new `docs/design/prototypes/auth.html`, `docs/design/prototypes/README.md`, `docs/design/ux-spec.md` (new ids)

## Goal
The screens M7 adds outside `admin.html` have an approved clickable prototype: sign-in, the
first-run wizard step "Create admin account" in front of the existing library setup, change
password, and the error/lockout states.

## Acceptance criteria
- [ ] `auth.html` shows: sign-in (name, password, show-password toggle, error text for wrong
      credentials and throttling with countdown), first-run wizard step 1 "Create admin account"
      (name, password twice, hint about the local-network rule and the recovery command) leading
      to the existing "No libraries yet" state (HO-09), change password in the profile menu,
      "Signed out" and "Session expired" states, "Not available from this network" for setup
      from a remote peer; en/de toggle; phone and desktop widths.
- [ ] The legend lists new ux-spec ids (proposed `AU-01…AU-06`) and the ADR 0032 references;
      `ux-spec.md` gets section 10.4 with these ids; README row with status `draft`.
- [ ] Gates: the file opens offline without network requests (no CDN), as the other prototypes.

## Notes
Plex walk-through has no local sign-in screen; use Mivo wording (ux-spec section 14). Keep the
PIN pad and picker from `admin.html` unchanged; link to them. The orchestrator approves by
delegation and records it in the README.
