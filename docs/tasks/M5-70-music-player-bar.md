# M5-70 Music player bar
Owner: mivo-frontend
Depends on: M5-60, M5-67, M3-13h; prototype `media.html` approved
Files/areas: `web/apps/web/src/features/music/components/MusicPlayerBar.tsx`, `hooks/useAudioPlayback.ts`, mounting in `app/layout`, locales

## Goal
A persistent bottom bar plays music across navigation: cover, title/artist, previous/play/next,
seek, volume, queue, repeat, shuffle; gapless-as-possible track changes (plan screen 5).

## Acceptance criteria
- [ ] Playing an album/track fills the shared queue (M5-60/65) and starts the bar; navigation
      keeps playing.
- [ ] Starting video closes/pauses the bar and vice versa (single playback rule with M5-67).
- [ ] Media Session API metadata and hardware keys (play/pause/next/previous) work (faked
      `navigator.mediaSession` in tests).
- [ ] Bar sits above the phone bottom nav; all controls named; seek is keyboard operable.
- [ ] Audio delivered through the existing playback session API (direct play of mp3/flac/aac
      per browser profile; transcode otherwise) - no new delivery code.
- [ ] Tests: hook with fake audio element, queue integration, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Hi-res/DSD handling is M4; the bar just shows the stream-info mode badge small if available.
