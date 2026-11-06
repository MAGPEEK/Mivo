# M7-27 Scripts: playback smoke signs in
Owner: mivo-media
Depends on: M7-01
Files/areas: `scripts/playback-smoke/`, `CLAUDE.md` release-gate lines only if flags change (orchestrator applies), `docs/development.md`

## Goal
The release gate keeps working once sign-in is on: the smoke script performs first-run setup on
a fresh container or signs in with given credentials.

## Acceptance criteria
- [ ] `playback-smoke` calls `GET /api/v1/setup`; when required it runs setup with a generated
      admin (`client=cli`) and uses the bearer token; with `-user`/`-password-env NAME` it signs
      in instead; on a server without the endpoint (404 before M7-01, 501 stub until M7-21) it proceeds
      unauthenticated.
- [ ] Every API and media request of the script sends the bearer token.
- [ ] Tests: `go test ./scripts/playback-smoke/...` with an `httptest` server for the three
      cases (setup, sign-in, pre-M7).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The password must not appear in process arguments; read it from the named environment variable.
Update the release-gate snippet in `docs/development.md`; report a needed `CLAUDE.md` change in
the REPORT instead of editing it.
