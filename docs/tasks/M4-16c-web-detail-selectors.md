# M4-16c Web: version, audio and subtitle selectors on the detail page
Owner: mivo-frontend
Depends on: M4-16a, M4-02a
Files/areas: `web/apps/web/src/features/item/**`, i18n `item`

## Goal
Before pressing play the user chooses version, audio and subtitles (DT-07), and the choice is passed to the player.

## Acceptance criteria
- [ ] Version selector appears only with more than one file (label, resolution, bit rate); default = highest quality the profile direct-plays.
- [ ] Audio and subtitle selectors list the tracks of the selected version; default from the policy of M4-04a (server-provided `defaultAudioStreamIndex` / `defaultSubtitleTrackId` in the item resource, otherwise first).
- [ ] Choice is forwarded to the player route (state, not URL secrets) and remembered per item on the device.
- [ ] Technical chips (existing) update with the selected version.
- [ ] Tests: component tests (single vs multiple versions, defaults, persistence); axe; German strings.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- If M4-02a lacks `default*` fields, report to the architect rather than recomputing the policy in the UI.
