# M4-16b Web: playback options panel (audio, subtitles, size, quality)
Owner: mivo-frontend
Depends on: M4-16a, mivo-ux prototype approval
Files/areas: `web/apps/web/src/features/player/**`, `src/shared/ui`, i18n namespaces `player` (en, de)

## Goal
During playback the viewer picks audio track, subtitles, subtitle size and quality in a panel (PL-10 … PL-13), with burn-in and transcode reasons visible.

## Acceptance criteria
- [ ] Panel opens from the player bar, keyboard-operable, focus trap and `Esc` close, labelled for screen readers.
- [ ] Audio list: language, codec, channels, title; choosing restarts with a spinner (PL-11).
- [ ] Subtitles: Off + tracks (language, forced/SDH, format, external badge); text tracks switch instantly; image tracks show "burned in – transcodes" hint (PL-12).
- [ ] Subtitle size (small/normal/large/huge) stored per device (localStorage via a hook), applied to the rendered cue layer (PL-13); disabled with a hint for burn-in.
- [ ] Quality: Original / Automatic / ladder steps with implied resolution; steps above the source hidden; choice per device (home setting only in M4).
- [ ] `subtitle_burn_in_unavailable` and `bitrate_cap` warnings appear as toast text.
- [ ] Tests: component tests with fake engine and fake `player-core` outputs (selection, restarts, hints); axe check; German strings present (i18n completeness test).
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- No server logic in components; hooks only. Cue rendering is a plain overlay `div` (ASS styling is post-0.1.0, M4-19).
