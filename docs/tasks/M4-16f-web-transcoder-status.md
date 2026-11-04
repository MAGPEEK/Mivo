# M4-16f Web: transcoder status and hardware test button (minimal settings)
Owner: mivo-frontend
Depends on: M4-02b, M4-14b
Files/areas: `web/apps/web/src/features/settings/transcoder/**`, route under Settings, i18n

## Goal
The admin sees active sessions and whether hardware acceleration works, and can run the test (AD-25 minimal; the full admin page follows in M7).

## Acceptance criteria
- [ ] Settings › Transcoder shows: acceleration setting, per-API result (usable / not available / degraded) with message, "none found – software
      transcoding is used" state, throttle supported yes/no, "Test hardware acceleration" button (disabled while running, shows result).
- [ ] Active sessions table with stop button (title, mode, encoder, bit rate, speed); refresh every 5 s while visible.
- [ ] A 403 (outside LAN/guard) shows the management-guard hint of M3-19m.
- [ ] Tests: component tests with fake API; axe; German strings.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- Read-only apart from test and stop; changing `transcode.hwaccel` stays in config until M7.
