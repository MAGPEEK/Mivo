# Mivo UX prototypes

Clickable, self-contained HTML prototypes of the screens in `docs/design/ux-spec.md`, for the
owner to review before `mivo-frontend` implements them. Open a file directly in a browser – no
build step, no network requests. Each file starts with a legend (element ids, milestone, open
questions). German UI texts by default, en/de toggle in the top bar.

| Prototype | Screens | ux-spec ids | Status |
|---|---|---|---|
| [browse.html](browse.html) | App shell (top bar, search dropdown and results page, activity indicator, sidebar with pinnable/reorderable libraries, collapse, phone bottom navigation), Home (optional hero, rows), library page with Overview and Browse pivots for a movies and a shows library | SH-01…05, SH-07, SH-10…18, HO-01…03, HO-05, HO-07, HO-09, LB-01…04, LB-06, LB-09, LB-20…30, LB-40…43, SR-01, SR-02 | draft |
| [admin.html](admin.html) | Admin area (own sidebar: Status, Settings, Manage, Users; "Expert settings" disclosure), Settings > Libraries list, empty state on first start, Add/Edit library dialog (type tiles, name, language, folders, advanced) with server-side folder browser (breadcrumbs, hidden system folders, unreadable-folder help for DSM/Docker/other), profile picker with PIN pad, users and managed profiles, library access, permissions, devices | AD-10…12, AD-20…30, AD-40…42, LS-01…03, HO-09, US-01…03, US-10…12, US-20…25 | draft |
| [detail.html](detail.html) | Movie, movie without artwork, show, season (grid and list), episode, extra page; item overflow menu (M5 actions only); player (full layout, mode badge green/blue/amber with stream-info overlay and reason hints, options panel with Auto play and speed, queue panel, next-up card, keyboard help, mini player, media info dialog); responsive preview 1440 / 1024 / 390 with phone bottom sheets; "Show later milestones" toggle reveals M4/M6/M7 placeholders | DT-01…03, DT-05, DT-06, DT-08, DT-09, DT-11, DT-20…22, PL-01…05, PL-08, PL-09, PL-14, PL-15, PL-32, PL-35, PL-36, PL-38, PL-40…42 (M4/M6/M7 placeholders: DT-04, DT-07, DT-10, ED-01…07, PL-07, PL-10…13, PL-20, PL-30) | draft – ready for approval (M5-01); owner approval needed before M5-40 |
| [settings.html](settings.html) | Settings for this device (`/settings/app`): Appearance, Library layout, Playback (M4 placeholders), Search, Diagnostics (expert), About; every control labelled with storage and milestone; keyboard help sheet opened with "?" | AD-01…03, SH-14, HO-01, PL-09, SR-02, LB-20 (M5-03) | draft |
| [media.html](media.html) | Music (artists/albums grids, artist page, album page with disc/track numbers, persistent player bar with queue, repeat/shuffle, phone variant above bottom nav), photos (month/day timeline, albums/folders, lightbox with zoom, swipe/arrows, EXIF panel, slideshow, Esc), audiobooks (grid, book page with resume point, chapters, speed 0.5-2.0x, sleep timer placeholder, bar in book mode), one-playback-at-a-time demo with video mini player | SH-11, SH-17, HO-02, LB-40, SR-02, PL-15, PL-38; proposed new ids MU-01…12, PH-01…09, AB-01…07 (M5-02) | draft; owner approval needed before M5-70…73 |

Statuses: **in progress** (being built), **draft** (ready for owner review), **approved**.

## Decisions (orchestrator, 2026-11-04, owner delegated: "arbeite alles selber ab")

admin.html open questions:
1. Own admin sidebar (as drawn) – the normal sidebar only gets the settings entry (SH-18).
2. Folder browser as a second dialog on desktop and tablet; on phones it takes the whole screen.
3. Unreadable folders: shown greyed with the permission-help link (DSM/Docker tabs), never hidden.
4. Extras and Conversions pages stay hidden until implemented (no placeholder pages in the release).
5. Profile PIN: 4 digits fixed (US-02).
6. Library kind tiles: only kinds the server supports are shown (M3-19 `libraryKinds`); no greyed teasers.
7. Folder choice: "Choose this folder" for the current path is the primary action; a per-row "Choose" stays as a shortcut.

browse.html questions: defaults as drawn – hero on Home on but switchable per device, watched toggle and ⋮
menu separate, phone tab 4 "Settings", coral unwatched badge, A–Z bar always visible in title sort, new
library opens "Browse".

detail.html questions (Q1–Q8), defaults as drawn (orchestrator, 2026-11-04):
1. "Play from start" only in the split menu.
2. Mode badge shows the short name; the reason is in the stream-info overlay.
3. Episodes as grid by default; view and size remembered per device.
4. Playback options and other panels are bottom sheets on phones.
5. Stream info as corner overlay.
6. Extras open their own extra page with a Play button.
7. Mini player bottom right (above the bottom navigation on phones); clicking the video expands it.
8. No artwork: typographic placeholder and neutral backdrop, no blurred gradient.
Also: the overflow menu shows only M5 actions by default (Play next, Add to queue, Re-analyze, Media info);
M4/M6/M7 items are hidden and appear only as dashed placeholders with the "Show later milestones" toggle.
The edit dialog (ED-*) stays in the file as an M6 preview behind that toggle; it is not an M5 deliverable.

## media.html (M5-02) – open questions, defaults as drawn

1. Player bar stays visible after the first play on desktop (no hide); no full-screen "Now playing" page is drawn.
2. "Popular tracks": album order without play counts (M3), by plays from M7.
3. Photos: timeline is the default pivot; EXIF date before file date; undated photos in "No date".
4. Lightbox info panel closed by default (key I).
5. Audiobook speed: global per device (alternative: per book).
6. Skip buttons in audiobook mode fixed at -30 s / +30 s; sleep timer drawn disabled (later).
7. One playback at a time: video start pauses music/audiobook, music/audiobook start closes the video mini player.
8. The ux-spec has no ids for 5.4; MU-/PH-/AB- ids are proposed in the legend and should be added after approval.

## Approval (orchestrator, 2026-11-04)

The owner delegated all remaining decisions ("arbeite alles selber ab und hinterfrage nicht",
recorded in docs/plan.md). `browse.html`, `admin.html`, `detail.html`, `settings.html` and
`media.html` are therefore **approved with the defaults as drawn** (answers to Q1–Q8 of
detail.html, the settings and media questions as listed above). The owner may still object at
any time; a change request goes to `mivo-ux` and updates the prototype before the affected
`mivo-frontend` task builds on it. Ids MU-01..12, PH-01..09, AB-01..07 are added to ux-spec 5.4.
