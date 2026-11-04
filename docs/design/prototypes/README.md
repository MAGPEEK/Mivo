# Mivo UX prototypes

Clickable, self-contained HTML prototypes of the screens in `docs/design/ux-spec.md`, for the
owner to review before `mivo-frontend` implements them. Open a file directly in a browser – no
build step, no network requests. Each file starts with a legend (element ids, milestone, open
questions). German UI texts by default, en/de toggle in the top bar.

| Prototype | Screens | ux-spec ids | Status |
|---|---|---|---|
| [browse.html](browse.html) | App shell (top bar, search dropdown and results page, activity indicator, sidebar with pinnable/reorderable libraries, collapse, phone bottom navigation), Home (optional hero, rows), library page with Overview and Browse pivots for a movies and a shows library | SH-01…05, SH-07, SH-10…18, HO-01…03, HO-05, HO-07, HO-09, LB-01…04, LB-06, LB-09, LB-20…30, LB-40…43, SR-01, SR-02 | draft |
| [admin.html](admin.html) | Admin area (own sidebar: Status, Settings, Manage, Users; "Expert settings" disclosure), Settings > Libraries list, empty state on first start, Add/Edit library dialog (type tiles, name, language, folders, advanced) with server-side folder browser (breadcrumbs, hidden system folders, unreadable-folder help for DSM/Docker/other), profile picker with PIN pad, users and managed profiles, library access, permissions, devices | AD-10…12, AD-20…30, AD-40…42, LS-01…03, HO-09, US-01…03, US-10…12, US-20…25 | draft |
| detail.html | Detail pages and player | DT-*, PL-* | in progress |

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

browse.html / detail.html questions (Q1–Q6 each): defaults as drawn – hero on Home on but switchable per device,
watched toggle and ⋮ menu separate, phone tab 4 "Settings", coral unwatched badge, A–Z bar always visible
in title sort, new library opens "Browse"; "Play from start" in the split menu, mode badge short name with
reason on hover/focus, episodes as grid, Save disabled until a change plus lock icon on locked artwork,
playback options as bottom sheet on phones, stream info as corner overlay.
