# M5-63 Keyboard shortcuts and help sheet
Owner: mivo-frontend
Depends on: M5-61, M5-03 (prototype approved)
Files/areas: `web/apps/web/src/features/player/hooks/usePlayerKeyboard.ts`, `lib/keymap.ts`, `components/ShortcutHelp.tsx`, locales

## Goal
Complete PL-09: Space/K, arrows +-10 s, J/L -10/+30 s, Up/Down volume, F, M, I, C subtitles
on/off, N next, P previous, Escape closes the top panel before leaving fullscreen; "?" opens a
help sheet.

## Acceptance criteria
- [ ] One `keymap.ts` data table drives both the handler and the help sheet (test: every entry
      is listed in the sheet and handled).
- [ ] Keys are ignored while typing in inputs and with Ctrl/Meta/Alt modifiers (except the
      documented ones); `C` is a no-op with a hint until subtitles exist (M4).
- [ ] Escape order: top panel, then fullscreen, then nothing.
- [ ] Existing M2-30 keyboard tests green; new tests per key and the escape order.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Layout-independent: use `event.key`, document AZERTY/QWERTZ expectations in the file comment.
