# Milestone 5 "Web-UI komplett" – overview

Scope (plan, milestone 5; order 3 -> 6 -> 5 -> 4 -> 7 per OD-UX-2): all screens of
`docs/design/ux-spec.md` that are not built yet. Element ids (SH-, HO-, LB-, DT-, SR-, PL-, AD-)
refer to the spec; section 15 lists the M5 set. Research: `docs/research/plex-walkthrough-2026-11-04.md`.

## What exists (do not re-plan)

- Shell `app/layout` (AppLayout, collapsible sidebar, narrow-screen overlay, Ctrl+K focus
  shortcut only), `shared/ui` (TopBar, Sidebar, Card, Poster, Badge, Skeleton, EmptyState,
  ConfirmDialog, icons), libraries in the sidebar with kind icons (SH-01, SH-02, SH-10, SH-11).
- Home: placeholder `HomePage`. Library: `LibraryPage` with `ItemCard`/`LibraryItemGrid`,
  cursor "Load more", scan control. Item: pages for movie, show, season, episode, extra
  (M3-12/12b), technical chips, track list, previous/next arrows.
- Player: `PlayerPage`, top bar, controls, scrubber with produced range, keyboard (M2-30),
  fullscreen, stream-info overlay (PL-06, PL-41 base). `packages/player-core`: profile, timeline,
  seek, stream info labels; no queue yet.
- Settings > Libraries (M3-19m/n). Music, audiobook and photo basic pages are M3-13h, M3-14h,
  M3-15i (prerequisites here, not repeated).
- API today: libraries, items, children, images, playback sessions. **Missing for M5:** filtered
  /sorted/faceted item listing, folders, home/overview rows, search, activity, re-analyze.

## Prototype gate (mivo-ux, owner approves first)

| Prototype | Covers | State | Task |
|---|---|---|---|
| `browse.html` | shell, search, home, library pivots, toolbar | draft | owner approval = gate for M5-21 ... M5-36, M5-50, M5-51 |
| `admin.html` | libraries, users, AD-10 dashboard | draft | gate for M5-81 |
| `detail.html` | detail pages, player | in progress | M5-01 finishes it; gate for M5-40 ... M5-44, M5-61 ... M5-67 |
| `media.html` (new) | music bar, artist/album, photo timeline/lightbox, audiobook | missing | M5-02; gate for M5-70 ... M5-73 |
| `settings.html` (new) | settings for this device, key help sheet, queue/mini player if absent from detail.html | missing | M5-03; gate for M5-63, M5-80 |

Frontend tasks whose prototype is not `approved` in `docs/design/prototypes/README.md` stay
BLOCKED; the orchestrator asks the owner (per the autonomy rule: decides by recommendation and
records it in the README, as for browse/admin).

## Tasks

Contract column: B = backend task of this milestone, M3/M4/M6/M7 = needs contract/data of that
milestone. "M7-deferred" items are not tasks (see end).

| ID | Title | Owner | Depends on | Contract / gate |
|---|---|---|---|---|
| M5-01 | Prototype: detail pages and player (finish) | mivo-ux | none | - |
| M5-02 | Prototype: music, photos, audiobooks | mivo-ux | none | - |
| M5-03 | Prototype: device settings, key help, queue/mini player gaps | mivo-ux | none | - |
| M5-10 | Contract: item listing sort/filter/facets/letters, folders | mivo-backend | M3-06b | B |
| M5-11 | Items query: filters, sort, facets, letter offsets, folders | mivo-backend | M5-10 | B |
| M5-12 | Contract: home and overview rows | mivo-backend | M3-06b | B |
| M5-13 | Rows service and endpoints | mivo-backend | M5-12 | B |
| M5-14 | SQLite FTS5 index and query | mivo-backend | M3-05c | B |
| M5-15 | Search contract, use case, endpoint | mivo-backend | M5-14 | B |
| M5-16 | Activity and now-playing (read-only) | mivo-backend | M3-11 | B |
| M5-17 | Re-analyze item endpoint | mivo-backend | M3-06b | B |
| M5-20 | Shared UI primitives: menu, sheet, tabs, segmented control | mivo-frontend | none | - |
| M5-21 | Sidebar: pin, reorder, all libraries, reset (SH-12...14, 16) | mivo-frontend | M5-20 | browse.html |
| M5-22 | Phone layout: bottom navigation (SH-17) | mivo-frontend | M5-20 | browse.html |
| M5-23 | Activity indicator (SH-04, SH-05) | mivo-frontend | M5-16, M5-20 | B |
| M5-24 | MediaRow component (HO-02) | mivo-frontend | M5-20 | browse.html |
| M5-25 | Home rows (HO-05, HO-07, HO-09 keep) | mivo-frontend | M5-13, M5-24 | B |
| M5-26 | Home hero (HO-01) | mivo-frontend | M5-25 | M6 for logos |
| M5-30 | Library pivots and Overview rows (LB-02...04, 09) | mivo-frontend | M5-13, M5-24 | B |
| M5-31 | Browse toolbar: URL state, type switch, sort, count (LB-21...23) | mivo-frontend | M5-11, M5-20 | B |
| M5-32 | Filter menu with facets and chips (LB-20) | mivo-frontend | M5-31 | B; M6/M7 filters hidden |
| M5-33 | View styles and size slider (LB-27, LB-28) | mivo-frontend | M5-31 | - |
| M5-34 | Virtualised grid and A-Z bar (LB-29, LB-30) | mivo-frontend | M5-31, M5-33 | B |
| M5-35 | Card overlay, overflow menu, selection mode (LB-26, 41, 43) | mivo-frontend | M5-34, M5-41 | M5-17 |
| M5-36 | Browse by folder (LB-21 Folders) | mivo-frontend | M5-31 | B |
| M5-40 | Detail layout: backdrop, poster, title block, summary (DT-01...03, 06) | mivo-frontend | M5-20 | M3 artwork |
| M5-41 | Detail actions and item actions menu (DT-05, section 7) | mivo-frontend | M5-20, M5-17, M5-42 | B |
| M5-42 | Media info dialog (DT-11, PL-36) | mivo-frontend | M5-20 | M3 item streams |
| M5-43 | Cast and crew, extras, trailer (DT-05, 08, 09) | mivo-frontend | M5-40 | M3 NFO people and extras |
| M5-44 | Season and episode grid layout (DT-20...22) | mivo-frontend | M5-33, M5-40 | - |
| M5-50 | Search dropdown (SR-01) | mivo-frontend | M5-15, M5-20 | B |
| M5-51 | Search results page (SR-02) | mivo-frontend | M5-50 | B |
| M5-60 | player-core queue model (PL-31) | mivo-frontend | none | - |
| M5-61 | Player layout and controls (PL-01...04) | mivo-frontend | M5-60, M5-20 | detail.html |
| M5-62 | Mode indicator, explanations, overlay extras (PL-40...42) | mivo-frontend | M5-61 | - |
| M5-63 | Keyboard shortcuts and help sheet (PL-09) | mivo-frontend | M5-61, M5-03 | - |
| M5-64 | Playback options shell: auto play, speed (PL-14, 15) | mivo-frontend | M5-61 | M4 fills quality/audio/subtitles |
| M5-65 | Queue panel and filling the queue (PL-32, 33) | mivo-frontend | M5-60, M5-61 | - |
| M5-66 | Next-up card (PL-08) | mivo-frontend | M5-65 | M4 for segments |
| M5-67 | Mini player (PL-38) | mivo-frontend | M5-61 | detail.html |
| M5-70 | Music player bar | mivo-frontend | M5-60, M5-67, M3-13h | media.html |
| M5-71 | Artist and album pages | mivo-frontend | M5-40, M3-13h | media.html |
| M5-72 | Photo timeline and lightbox | mivo-frontend | M5-34, M3-15i | media.html |
| M5-73 | Audiobook UI | mivo-frontend | M5-64, M3-14h | media.html; M7 for server resume |
| M5-80 | Settings for this device (AD-01, AD-03) | mivo-frontend | M5-21, M5-03 | - |
| M5-81 | Read-only dashboard "Now playing" (AD-10, PL-43) | mivo-frontend | M5-16 | admin.html |
| M5-90 | i18n completeness and localisation | mivo-frontend | M5-80 | - |
| M5-91 | E2E: shell, home, library, search | mivo-frontend | M5-25, M5-34, M5-51 | - |
| M5-92 | E2E: player, queue, mini player | mivo-frontend | M5-65, M5-67 | - |
| M5-93 | E2E: music, photos, audiobooks | mivo-frontend | M5-70, M5-72, M5-73 | - |

## Parallel lanes

- Prototypes M5-01, 02, 03 and backend M5-10, 12, 14, 16, 17, 60 and 20 start immediately.
- Backend chain: 10 -> 11, 12 -> 13, 14 -> 15; frontend lanes follow their contract tasks.
- Lanes touch different folders: shell (`app/layout`, `shared/ui`), `features/home`,
  `features/library`, `features/item`, `features/search`, `features/player` +
  `packages/player-core`, `features/music|photos|audiobooks`, `features/settings`. M5-20 is
  the only shared prerequisite.

## M7-deferred (not planned here, OD-UX-3 option B would pull them in)

Continue watching, Up next, Recently watched/played, watched toggle and badges, progress bars,
Unwatched/In progress filters, resume position (HO-03/04/06, LB-01/06/07, LB-42 parts). Every M5
component leaves a slot (row source list, card badge slot, filter registry) so M7 adds them
without refactoring. M6-deferred: person pages and portraits, ratings, edit pencil, related,
logos/TMDB backdrops. M4: quality/audio/subtitle panel sections, selectors DT-07, chapters,
trickplay.
