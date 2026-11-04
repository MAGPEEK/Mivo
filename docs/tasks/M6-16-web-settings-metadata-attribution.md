# M6-16 Web: Settings › Metadata, attribution, privacy doc
Owner: mivo-frontend
Depends on: M6-01
Files/areas: `web/apps/web/src/features/settings/metadata/**`, `web/packages/ui-tokens` (assets only), `docs/privacy.md`

## Goal
Admins see and control what goes online, can enter their own TMDB key, and the required
attribution is displayed.

## Acceptance criteria
- [ ] Settings › Metadata: master switch, per-provider switches (TMDB, MusicBrainz + Cover
      Art Archive), TMDB key field (write-only, status "Project key" / "Your key" / "None"),
      metadata language, status panel (queue length, provider state ok / busy / offline /
      disabled) polling `/metadata/status`.
- [ ] Per-library "Use online metadata" toggle in the library edit dialog.
- [ ] A short text lists the contacted hosts and states that only titles, years and IDs are
      sent, never file names or paths.
- [ ] "Powered by TMDB" with the bundled TMDB logo and the disclaimer text in Settings ›
      Metadata, About, and the detail page footer when `metadataSources` contains `tmdb`;
      Cover Art Archive / MusicBrainz credit likewise for music.
- [ ] The logo is a bundled asset; no request leaves the app origin (Playwright/Vitest check
      for absence of external URLs in the rendered DOM).
- [ ] `docs/privacy.md` lists every outbound host, trigger, data sent and how to disable it
      (update check, TMDB, MusicBrainz, Cover Art Archive).
- [ ] Tests: Vitest with mocked client incl. key never rendered back; lint + coverage.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
Check the current TMDB branding rules before choosing logo size and wording and record them
in the task report.
