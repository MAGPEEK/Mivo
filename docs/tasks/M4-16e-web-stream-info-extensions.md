# M4-16e Web: stream-info overlay extensions (PL-41)
Owner: mivo-frontend
Depends on: M4-02a, M4-02b
Files/areas: `web/apps/web/src/features/player/streamInfo/**`, `player-core` mapping tables, i18n

## Goal
The overlay names output bit rate, encoder (hardware or software), burn-in reason, bit-rate cap and transcode speed.

## Acceptance criteria
- [ ] Rows added: output bit rate, video encoder with a "hardware" tag, subtitle mode + track, transcode speed (turns warning colour below 1.0x with the
      text "too slow – stuttering likely"), paused (throttled) hidden from viewers.
- [ ] Unknown codes keep the `unknown` fallback of M2-21.
- [ ] Tests: mapping tests for new codes; component test.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- Speed is polled from `GET …/sessions/{id}` every 5 s while the overlay is open only.
