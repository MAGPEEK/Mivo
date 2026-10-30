# M2-23 Web: item detail page with technical chips and Play
Owner: mivo-frontend
Depends on: M2-01, M2-07, M2-14 (labels)
Files/areas: `web/apps/web/src/features/item/**` (incl. its own test fakes)

## Goal
The user opens a video and sees what it is technically – container, video with HDR and bit
depth, audio tracks with channels, subtitle tracks – and starts playback with one button (plan
screen 3 "Technik-Chips").

## Acceptance criteria
- [ ] `/items/$itemId` loads `GET /api/v1/items/{itemId}` and shows the title, duration, file name
      and size, and chips in the plan's form: `MKV · HEVC 10-bit HDR10 · AC-3 5.1 · SRT`, built
      from the `@mivo/player-core` label functions (M2-14) by a pure function with a table test
      (corpus codecs, HDR10/HLG, missing fields).
- [ ] Track lists: audio and subtitle streams with language (via `Intl.DisplayNames`, fallback to
      the code), title, default/forced flags.
- [ ] A large "Play" button links to `/items/$itemId/play?fileId=<primary file>`; disabled with an
      explanation when the file has a probe error.
- [ ] Unknown item → not-found message with a link back; network error → retry.
- [ ] Strings only in `features/item/locales/{en,de}.json`.
- [ ] Tests (Vitest + Testing Library + a fake API local to the feature): render, chips, disabled
      Play, not found. Coverage thresholds hold.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
- Do not edit `src/test/**` in this wave (M2-22 owns it); keep fakes inside the feature.
- No artwork yet (milestone 3); use a neutral placeholder area so the layout does not change later.
- Subtitle tracks are listed, but the player ignores them until milestone 4; say so in a muted
  hint.

## Orchestrator note (from M2-07)
- Feature translation namespaces are registered in `web/apps/web/src/app/i18n.ts` (one line per
  feature), not in `shared/i18n/resources.ts`; the i18next type augmentation lives in
  `src/app/i18next.d.ts`. Features only edit their own `locales/{en,de}.json`.
- Pages receive route params as props (`LibraryPage {libraryId}`, `ItemPage {itemId}`,
  `PlayerPage {itemId, fileId, startAt}`); `features/player/lib/playerSearch.ts`
  (`validatePlayerSearch`) already exists.
- (from M2-14) `apps/web` depends on `@mivo/player-core` (workspace) since M2-14; use its
  `containerLabel`, `codecLabel`, `channelLabel`, `hdrLabel`, `bitDepthLabel`,
  `resolutionLabel(height, width?)` for chips. Do not change pnpm-lock.yaml in wave 3 (M2-27 owns
  it in wave 4).
