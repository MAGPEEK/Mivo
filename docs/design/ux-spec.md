# Mivo UX specification

- Status: Draft for the owner (2026-11-04). Binding for task planning once `docs/plan.md` is
  approved with it; open decisions OD-UX-1 … OD-UX-3 in `docs/plan.md` may change milestones.
- Source: owner request of 2026-11-04 ("study the Plex UX and adopt it for our plan"), the
  walkthrough in `docs/research/plex-walkthrough-2026-11-04.md`, the design section of
  `docs/plan.md`, ADR 0011 (design direction), ADR 0022 (library model), ADR 0010 (outbound
  connections), ADR 0006 (local accounts).
- Scope: the web app (`web/apps/web`). The TV app (milestone 10) reuses the information
  architecture with a 10-foot layout; differences are named where they matter.

## 0 Principles

1. **Adopt structure, not identity.** Mivo takes over Plex's information architecture and
   interaction patterns because users know them. Name, logo, colours (coral `#FF6B57`, tokens of
   ADR 0011), icons, illustrations and all wording are Mivo's own. No Plex assets, texts,
   trademarks or its orange. Section 14 maps Plex terms to Mivo wording.
2. **No cloud or social features.** Everything runs on the user's server (ADR 0006, ADR 0010).
   Section 13 lists what is deliberately not adopted and why.
3. **Transparency is the differentiator.** The viewer always sees how a stream is delivered:
   mode indicator in the player bar, stream-info overlay, and the same data in the admin "Now
   playing" view (section 9.8).
4. **No paid tier.** Everything Plex gates behind a subscription is either free in Mivo or not
   adopted.
5. **Library roots stay read-only** (ADR 0022). Downloaded or uploaded subtitles and artwork
   live in Mivo's data directory, never next to the media. Media deletion is not offered.
6. **Every view state is a URL.** Pivot, filter, sort, view style, search query, open panel of a
   page: all in route params or search params (TanStack Router), so back/forward, reload and
   shared links restore the view. Size slider and sidebar layout are user preferences, not URL
   state.
7. **Progressive disclosure.** Settings show the common options; an "Expert settings" switch per
   page reveals the rest. One "Save" per page; leaving with unsaved changes asks first.
8. **Settings that come from the environment are visible but locked.** Configuration precedence
   (ADR 0018: environment > `config.yaml` > defaults) is shown in the UI: a field set by an
   environment variable is read-only with a hint naming the variable.
9. **Accessible and keyboard-first.** Every action is reachable by keyboard, focus is visible,
   menus and dialogs are Radix primitives, rows and grids are lists with labelled links. The
   same structure maps to spatial navigation on the TV.
10. **Components render, hooks decide.** Business rules (row generation, filters, track choice,
    restrictions) live in the server, in hooks or in `packages/player-core`, not in components
    (CLAUDE.md).

**Milestone legend.** Each element has an id (for task files) and a target milestone of
`docs/plan.md`: M3 … M10, M7b, **later** (after v1, milestone 11) or **–** (not adopted). An
element that spans milestones names the milestone that delivers it to the user first; later
parts are named in its row. "Data" means the server work, "UI" the screen work.

## 1 Information architecture

| Route | Screen | Section | Milestone |
|---|---|---|---|
| `/` | Home | 3 | M5 (rows), exists since M1 as placeholder |
| `/libraries/$libraryId?view=overview` | Library – Overview pivot | 4.1 | M5 |
| `/libraries/$libraryId?view=browse&…` | Library – Browse pivot (grid/list/details) | 4.2 | M5 (grid exists since M2) |
| `/libraries/$libraryId/folders/*` | Library – browse by folder | 4.2 | M5 |
| `/items/$itemId` | Detail: movie, show, season, episode, extra | 5 | M3 (hierarchy), M5 (full layout) |
| `/people/$personId` | Person page | 5.5 | M6 (M3 for names from local NFO) |
| `/collections/$collectionId` | Collection page | 5.6 | M7 |
| `/playlists`, `/playlists/$playlistId` | Playlists | 5.6 | M7 |
| `/search?q=…&type=…` | Search results with pivots | 6 | M5 |
| `/items/$itemId/play?…` | Player (full) | 9 | exists since M2, layout M5 |
| (overlay, all routes) | Mini player | 9.7 | M5 |
| `/profiles` | Profile picker | 10.1 | M7 |
| `/login`, `/setup` | Login, first-run wizard | 10, 11 | M7 |
| `/settings/app/*` | Settings for this device (client) | 11.2 | M5 |
| `/settings/account/*` | Own account and profile | 11.3 | M7 |
| `/admin/status/*` | Admin – Status group | 11.4 | M5 (read-only dashboard), M7 |
| `/admin/settings/*` | Admin – Server settings | 11.5 | M7 (pages per milestone) |
| `/admin/manage/*` | Admin – Manage group | 11.6 | M7 |
| `/admin/users/*` | Admin – Users and access | 10.3 | M7 |

## 2 Shell

### 2.1 Top bar (left to right)

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| SH-01 | Sidebar toggle | Collapses the sidebar to icons; state remembered per device. Exists (M1-15). | exists |
| SH-02 | Logo | Brand mark from `ui-tokens/brand.ts`; links to Home. | exists |
| SH-03 | Global search field | Ctrl+K / Cmd+K focuses it (exists). Search-as-you-type dropdown, see 6.1. | M5 |
| SH-04 | Activity indicator | Icon with a badge while server work runs: scans (count and progress), metadata refresh, trickplay/chapter image generation, active transcodes (admin only). Popover lists each activity with progress and a link to its admin page. Also shows "Update available" (admin, ADR 0010 update check). Data through the event WebSocket; polling is acceptable for the first slice. | M5 (scans, transcodes), M6 (metadata), M7 (update notice) |
| SH-05 | Settings button | Opens `/admin` for admins, `/settings/app` for others. | M5 (app), M7 (admin split) |
| SH-06 | Cast button | Chromecast sender; hidden until casting is enabled (ADR 0010: SDK loads only then). | M8 |
| SH-07 | Profile menu | Avatar with initial: switch profile, own settings, sign out; shows the current profile's restriction badge. | M7 |
| – | Upsell button | Not adopted (no paid tier). | – |

### 2.2 Sidebar

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| SH-10 | Home entry | First entry, always visible. | exists |
| SH-11 | Library entries | One per library, icon by kind (movies, shows, music, photos, audiobooks, mixed), "unavailable" marker (exists since M2-22). | exists |
| SH-12 | "All libraries" | Last entry flips the sidebar into the full list with a **pin toggle** per library; unpinned libraries are only in this list. | M5 |
| SH-13 | Reorder | Pinned libraries can be reordered by drag and drop and by keyboard (move up/down buttons in the full list). | M5 |
| SH-14 | Reset layout | In the full list and in Settings › This device. | M5 |
| SH-15 | Playlists entry | Appears when the profile has playlists. | M7 |
| SH-16 | Persistence | Pins and order are a per-device preference (local storage) in M5 and move to the server per profile in M7 (with a one-time import of the device layout). | M5, M7 |
| SH-17 | Phone layout | Below the small breakpoint the sidebar becomes a bottom navigation: Home, Libraries (sheet with the pinned list), Search, Profile. | M5 |
| SH-18 | Settings entry | Last sidebar entry: Settings (opens `/admin` for admins, `/settings/app` for others, like SH-05). | M5 |

The sidebar contains **only** Home, the server's own libraries (pinnable, reorderable) and
Mivo's own sections (playlists, settings). Plex's online sidebar part is not adopted
(owner instruction 2026-11-04, section 13). Mivo's own live TV/DVR (later, milestone 11) is a
separate plan item and would appear as a library-like entry of the server, not as an online
section.

## 3 Home

Home shows rows of the libraries the profile may see. Plex has no hero in the observed version;
Mivo keeps the hero of ADR 0011 as an **optional** element above the rows.

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| HO-01 | Hero (optional) | Large backdrop with logo/title, summary line, buttons "Play"/"Resume" and "More info"; rotates through up to 5 titles (continue watching first, then recently added) every 8 s, pauses on hover/focus, respects `prefers-reduced-motion`. A switch in Settings › This device hides it (default: on). Without artwork the hero is not shown. | M5 (local artwork from M3), M6 (logos, backdrops from TMDB) |
| HO-02 | Row component | Title with a "See all" link, horizontally scrolling cards, previous/next arrow buttons (visible on hover and focus), keyboard: arrows move focus within the row. Rows are virtualised horizontally. Card formats: poster 2:3, episode 16:9, album/photo 1:1. | M5 |
| HO-03 | Continue watching | 2:3 poster with progress bar; for episodes show title, episode title and a compact `S3 · E9` label. Overflow menu: "Remove from continue watching", "Mark as watched". | M7 (M5 if OD-UX-3 B) |
| HO-04 | Up next | Next unwatched episode per show in progress. | M7 (M5 if OD-UX-3 B) |
| HO-05 | Recently added | One row per library by default; episodes grouped into a show card (or a season card when only one season changed) with a "new episodes" count badge. Option "Merge recently added by media type" (admin, 11.6). | M5 (grouping), M7 (merge option, unwatched counts) |
| HO-06 | Recently played (music) | Albums and tracks played recently. | M7 (watch/listen state) |
| HO-07 | Row source | Rows are generated on the server (one endpoint returning row definitions with their first page of items; "See all" opens the library's Browse pivot with the row's filter and sort in the URL). The client renders them, it does not compute them. | M5 |
| HO-08 | Row configuration | Which libraries feed Home and which rows appear, in which order: admin per library ("Customize rows", 11.6) and per profile (hide a row). | M7 |
| – | Social pivots (trending, activity feed, friends, public profile) | Not adopted. | – |

## 4 Library page

Every library has two pivots as tabs under the library title: **Overview** (rows) and
**Browse** (everything). The last used pivot is remembered per library and device.

### 4.1 Overview pivot

Rows generated on the server per library (HO-07 mechanism). Mivo row names are its own (section 14).

| Id | Row | Data needed | Milestone |
|---|---|---|---|
| LB-01 | Continue watching (this library) | watch state | M7 (M5 if OD-UX-3 B) |
| LB-02 | Recently added | `created_at` | M5 |
| LB-03 | New releases (by release date) | release date (NFO M3, TMDB M6) | M5 |
| LB-04 | "More <genre>" (2–3 genres with the most titles, rotating daily) | genres (NFO M3, TMDB M6) | M5 |
| LB-05 | "Starring <person>" (people with the most titles) | cast (NFO M3, TMDB M6) | M6 |
| LB-06 | Not watched yet / Start a show (shows with no watched episode, badge with episode or season count) | watch state | M7 |
| LB-07 | Recently watched | watch state | M7 |
| LB-08 | Highest rated | rating (TMDB M6) | M6 |
| LB-09 | Recently released episodes (show libraries; show poster with `S3 · E7`) | air date | M5 |

The Overview pivot shows a short hint instead of an empty page when a library has too little
data for rows (for example no metadata yet).

### 4.2 Browse pivot

**Toolbar** (one line on desktop, wraps on tablets, a bottom sheet with the same controls on
phones):

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| LB-20 | Filter menu | Quick filters (single choice) and field filters (combinable, shown as removable chips under the toolbar). Quick filters: All; HDR; Dolby Vision; HDR10+; Dolby Atmos; 4K; Unavailable (probe failed); Duplicates (an item with more than one file); Unwatched, In progress (M7); Unmatched (M6). Field filters: genre, year, decade, content rating, collection (M7), director, actor, writer, country, studio, resolution, video codec, audio codec, container, audio language, subtitle language. Each value shows its count (facets from the server). | M5 (technical and local fields), M6 (metadata fields), M7 (watch state, collections) |
| LB-21 | Type switch | Library's item type (Movies / Shows / Seasons / Episodes for show libraries) or **Folders**: browse the library's folder tree (relative paths only, never absolute; breadcrumb from the library root; files in a folder open their item). | M5 |
| LB-22 | Sort menu | Title, year, release date, rating (M6), content rating, duration, date added, resolution, bitrate, file size, random; progress, plays, date watched (M7). Ascending/descending toggle. Title sort uses `sort_title` (leading articles per metadata language, M3). | M5 (M6, M7 parts) |
| LB-23 | Item count | "367 movies" for the current filter. | M5 |
| LB-24 | Play all / Shuffle | Puts the current filtered and sorted list into the queue (9.5). | M5 |
| LB-25 | Add to … | Add the selection or the filtered list to a playlist or collection. | M7 |
| LB-26 | Selection mode | Multi-select (checkbox on hover/focus, Shift-range) for bulk actions: mark watched (M7), add to playlist/collection (M7), refresh metadata (M6), re-analyze (M5). | M5 |
| LB-27 | View style | Grid (posters), List (compact rows: title, year, duration, resolution chip, watched), Details (list with poster thumbnail, summary line and technical chips). | M5 |
| LB-28 | Size slider | Changes the card width in the grid (5 steps); preference per device. | M5 |
| LB-29 | A–Z jump bar | Right edge in title sort: `#`, A–Z; letters without titles are dimmed; a click scrolls the virtualised grid to the first title of the letter (the server returns the start offset per letter for the current filter). Hidden for other sorts. | M5 |
| LB-30 | Virtualised grid | TanStack Virtual, rows of cards, for 50,000+ items; paging from the server. Replaces "Load more" of M2-22. | M5 |

**Cards** (grid, rows and search):

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| LB-40 | Card | Poster 2:3 (16:9 for episodes and extras, 1:1 for albums and photos), title and year below (episodes: `S1 · E3` and title). Artwork from the server's image endpoint in the size the slider needs (LB-44); BlurHash or a neutral placeholder while loading. | M3 (local artwork), M5 (layout) |
| LB-41 | Hover/focus overlay | Play button (centre), overflow menu, edit pencil (admin, M6), watched toggle (M7). | M5 |
| LB-42 | Badges | Watched check mark corner, unwatched episode count, progress bar (M7); "unavailable" (exists); version count when several files (M4). | M5, M7 |
| LB-43 | Overflow menu | The item actions of section 7 that apply to cards. | M5 |
| LB-44 | Image endpoint | Server-side resizing and caching of artwork: `GET /api/v1/images/{imageId}?w=&h=` with a fixed set of allowed sizes (no free sizes, so the cache stays bounded), `ETag` and long cache headers. | M3 |

## 5 Detail pages

All detail pages share one layout; the content differs by item kind.

### 5.1 Common layout

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| DT-01 | Backdrop | Blurred and darkened backdrop behind the page (backdrop, else poster); text contrast holds WCAG AA (ADR 0011). | M5 |
| DT-02 | Poster | Left column; content-rating badge; for shows the next episode ("Up next: S1 · E1") and an unwatched badge (M7). | M5 |
| DT-03 | Title block | Title (logo image when available, M6), original title when different, line with content rating, year, runtime ("1 h 39 min" localised), genres (first three, "+n" opens all; each links to the Browse pivot filtered by the genre), director line. | M5 |
| DT-04 | Ratings row | Provider ratings with source names (TMDB first; others only from providers Mivo actually uses, ADR 0010), own rating (stars, M7). | M6, M7 |
| DT-05 | Action buttons | **Play** (primary, coral; becomes **Resume** with the position, plus "Play from start" in its split menu), **Mark as watched** (M7), **Edit** (admin, M6), overflow menu (section 7). Trailer button when a local trailer extra exists (M5). | M5 |
| DT-06 | Summary | Tagline, summary clamped to 4 lines with "More". | M5 (NFO from M3) |
| DT-07 | Technical block | Three selectors: **Version** (only when several files, M4), **Audio** (all tracks with language, codec, channels; default from the profile's language preference, M4; remembered per item and profile, M7), **Subtitles** (Off, tracks, "Search online…" M6, "Upload…" M7). The choice is passed to the player. Below them the technical chips of M2-23 (`MKV · HEVC 10-bit HDR10 · TrueHD 7.1 · PGS`). | M4 (selectors), exists (chips) |
| DT-08 | Cast & crew | Row of round portraits with name and role; links to person pages. Names without portraits from local NFO in M3/M5; portraits and person pages in M6. | M5 (local), M6 |
| DT-09 | Extras | Row of trailers, featurettes, behind the scenes, deleted scenes (16:9). | M5 (scan of extras folders: M3) |
| DT-10 | Related | "More like this": same collection first, then shared genres and people (server-side). | M6 |
| DT-11 | Media info entry | "Media info" in the overflow menu opens the dialog of 9.6 (also reachable from the player). | M5 |
| – | Community reviews, "rate & review" for the public, streaming-service links ("more ways to watch") | Not adopted. | – |

### 5.2 Movie

Common layout; sections in order: technical block, cast & crew, extras, related.

### 5.3 Show → season → episode

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| DT-20 | Show page | Common layout; Play starts the up-next episode (M7; before that the first episode); **Seasons** row with season posters, "Season 1 · 9 episodes", unwatched badge (M7); specials (season 0) last; then cast, extras, related. | M3 (minimal: title and seasons list), M5 (layout) |
| DT-21 | Season page | Season poster with play overlay, show title (link) and season title, Play, actions, summary; then the **episodes** as a grid of 16:9 thumbnails (title, `Episode 8`, runtime, progress/watched; hover: play, edit, overflow, watched toggle) or as a list (view toggle and size slider as in 4.2). Default order **ascending** with a toggle (Plex showed descending). Previous/next season arrows next to the title. | M3 (minimal list), M5 (layout) |
| DT-22 | Episode page | Common layout with a 16:9 still instead of the poster, `S1 · E3`, air date, links to show and season, previous/next episode arrows. | M3 (minimal), M5 |
| DT-23 | Episode thumbnails | Local still from the file (generated frame at 10 % of the duration) until provider stills exist. | M4 (generated), M6 (TMDB stills) |
| DT-24 | Absolute numbering | Shows with absolute episode numbers (anime) list their episodes in season 1 or by the provider's mapping, never one season per episode (parser regression test from the walkthrough). | M3 (parser), M6 (provider mapping) |

### 5.4 Music, photos, audiobooks

Out of the walkthrough's scope; the plan's screens 5–7 stay (artist/album pages with the
persistent music bar, photo timeline and lightbox, audiobook chapters). They use the same shell,
cards, toolbar and detail layout. Milestone M5 (UI), data in M3.

### 5.5 Person page

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| DT-30 | Person page | Portrait, name, known-for department, birth/death date and place, biography (clamped), then the **filmography in the user's libraries** as rows by role (acting, directing, writing), sorted by year. No links to titles that are not in a library. | M6 (M5: name and filmography from local NFO people) |

### 5.6 Collections and playlists

Collection page: like a show page with the member titles as a grid (sorted by release date).
Playlists: list page and playlist page with drag-and-drop order. Milestone M7 (plan).

## 6 Search

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| SR-01 | Dropdown | Opens after 2 characters, debounced 200 ms; up to 8 top results across types: thumbnail, title, year, type, library; a play button and an overflow menu per result; arrow keys move, Enter on a result opens it, Enter in the field opens the results page. Recent searches (per device) when the field is empty. | M5 |
| SR-02 | Results page | `/search?q=&type=`: pivots **Top results**, Movies, Shows, Episodes, People (M6), Music (artists, albums, tracks), Photos, Audiobooks, Collections (M7). Each pivot is a grid with the card of its type. | M5 |
| SR-03 | Engine | SQLite FTS5 over titles, original titles, sort titles, people, genres; accent- and case-insensitive; prefix matching. Results respect the profile's library access and restrictions (M7). | M5 |
| – | Live TV and streaming-service pivots | Not adopted (live TV is "later" and gets its own pivot then). | – |

## 7 Item actions (overflow menu)

The same action set appears on detail pages, cards, search results and the player's "More"
menu; each surface shows the subset that makes sense there. Admin-only actions are hidden for
other profiles from M7 on.

| Action | Mivo | Milestone |
|---|---|---|
| Play / Resume / Play from start | yes | exists / M7 (resume) |
| Play next, Add to queue | yes (9.5) | M5 |
| Add to playlist / collection | yes | M7 |
| Mark as watched / unwatched | yes | M7 (M5 if OD-UX-3 B) |
| Edit details | yes, admin (section 8) | M6 |
| Refresh metadata | yes, admin | M6 |
| Correct match / Remove match | yes, admin (plan: "Zuordnung korrigieren") | M6 |
| Re-analyze file | yes, admin: probes the files of the item again | M5 |
| Media info | yes (9.6) | M5 |
| Download original file | yes, if the profile has the "downloads" permission | M7 |
| Share with user | yes, admin: grants a user access to this single title (10.3) | M7 |
| Play history | yes (per item: who, when, which device, mode) | M7 |
| Pre-convert ("optimize") | later (offline conversions, section 13) | later |
| Watch together | later (SyncPlay, plan milestone 11) | later |
| Delete media file | – | – |
| Report issue | – (no vendor backend) | – |
| Add to watchlist, share to social | – | – |

## 8 Edit details dialog (per item)

Admin-only dialog, opened from the edit button of a detail page or a card. Tabs on the left,
"Cancel" and "Save" at the bottom; the dialog is a route-addressable modal
(`?edit=general`) so a reload keeps it open.

| Id | Tab / element | Content | Milestone |
|---|---|---|---|
| ED-01 | General | Title, sort title, original title, edition (free text, e.g. "Director's Cut"; links to versions, M4), release date, content rating (with certification country), studio, tagline, summary. Episodes: season/episode number, air date. | M6 |
| ED-02 | Field locks | Each field has a lock toggle. Editing a field locks it automatically; a locked field is never overwritten by a metadata refresh or an NFO change. "Unlock all" resets to provider values on the next refresh. | M6 |
| ED-03 | Tags | Chip inputs with autocomplete from existing values and remove per chip: genres, directors, writers, producers, countries, studios; collections (M7). Each tag list has its own lock. | M6 (M7 collections) |
| ED-04 | Labels | Free labels used by content restrictions (10.2) and filters. | M7 |
| ED-05 | Artwork: Poster, Backdrop, Logo, Square | One picker per kind: gallery of candidates (local files of the item folder, then providers), current one marked; **Upload file** (drag and drop; type and size limited, decoded and re-encoded by the server); **From URL** (server fetches the image: http/https only, public addresses only, size and type limits, timeout – SSRF guard). Choosing locks the artwork. Files are stored in the data directory, never in the library. | M6 |
| ED-06 | Files | Library name and library-relative paths of all files with size and container (no absolute paths); per file "Re-analyze". | M5 (read-only list in media info), M6 (tab) |
| ED-07 | Advanced | Per-item metadata language (overrides the library's), "Use original title", per-item match provider ID (TMDB id; changing it rematches). | M6 |
| ED-08 | Batch edit | The same dialog for a multi-selection (LB-26) edits only the fields touched (genres, content rating, collections, labels). | M7 |

## 9 Player

### 9.1 Layout (full player)

```
┌ top bar ───────────────────────────────────────────────────────────────┐
│ [▾ minimise]                     title (centre on TV only)  [⛶ full]   │
│                                                                        │
│                        video  (+ skip-intro button bottom right, M4)   │
│                                                                        │
├ seek bar (full width, hover time + trickplay thumbnail, chapter ticks) ┤
│ Title · Year            ⏮  ↺10  ▶/❚❚  30↻  ⏭  ■        [●mode] ⋮ 🔁 🔀 ⚙ ☰ ▤ 🔊 │
│ 0:12:04 / 1:39:00                                                      │
└────────────────────────────────────────────────────────────────────────┘
```

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| PL-01 | Top bar | Minimise (to the mini player, 9.7) on the left, fullscreen on the right. Back (browser history) stays as in M2. | M5 |
| PL-02 | Left block | Title (link to detail; episodes: show title and `S1 · E3 · Title`), year, "elapsed / total". | M5 |
| PL-03 | Centre controls | Previous, back 10 s, play/pause, forward 30 s, next, stop. Previous/next are disabled without a queue; stop ends the session and returns to the detail page. | M5 |
| PL-04 | Right controls | **Mode indicator** (PL-40), More (9.6), repeat (off/all/one), shuffle, **Playback options** (9.2), Chapters (9.4), Queue (9.5), volume (mute and slider). | M5 |
| PL-05 | Seek bar | Full width, hover/focus tooltip with time; trickplay thumbnail when generated (M4); chapter ticks (M4); produced range of a running transcode shaded (exists since M2). | M5, M4 |
| PL-06 | Auto-hide | Controls hide after 3 s of inactivity while playing (exists since M2-30). | exists |
| PL-07 | Skip segment | "Skip intro" / "Skip credits" button while inside a detected segment (plan: media segments). | M4 |
| PL-08 | Next-up card | In the last seconds (credits segment or last 30 s): card with the next queue item and a countdown when Auto play is on. | M5 (countdown with segments: M4) |
| PL-09 | Keyboard | Space/K, ←/→ ±10 s, J/L −10/+30 s, ↑/↓ volume, F fullscreen, M mute, I stream info, C subtitles on/off, N next, P previous, Escape closes the top panel / leaves fullscreen. A "?" help sheet lists them. | M5 (extends M2-30) |

### 9.2 Playback options panel

Slides up over the lower part of the video; one panel, sections in this order. On the TV it is
a side panel (plan screen 4).

| Id | Section | Behaviour | Milestone |
|---|---|---|---|
| PL-10 | Quality | **Original** (shows the source bitrate and resolution; checked when the mode is Direct Play or Direct Stream), **Automatic** (adapts to measured bandwidth for remote clients), "Show all" → bitrate ladder (e.g. 20, 12, 8, 4, 2, 1 Mbit/s with the resolution each implies). A choice below the source forces a transcode; the mode indicator and overlay say so. Default per device: home quality and remote quality (Settings › This device). | M4 |
| PL-11 | Audio | All audio tracks (language, codec, channels, title). Switching restarts the stream at the current position with a short spinner. Remembered for the item per device (M4) and per profile on the server (M7). | M4 |
| PL-12 | Subtitles | Off, then tracks (language, forced/SDH flags, format), then **Search online…** (9.3, M6) and **Upload…** (9.3, M7). Text subtitles render in the client (SRT/VTT, ASS via JASSUB); image subtitles are burned in (forces a transcode; the overlay says why). | M4 |
| PL-13 | Subtitle style | Size (small/normal/large/huge), shown while a text subtitle is active; position offset and background opacity under "More". Per device. Not applicable to burned-in subtitles (hint). | M4 |
| PL-14 | Auto play | Checkbox "Play next automatically" (per device, M7 per profile). | M5 |
| PL-15 | Playback speed | 0.5–2.0× (audiobooks need it; also for video). | M5 |

### 9.3 Subtitle search and upload

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| PL-20 | Search online | Dialog: language (default: profile's subtitle language), query prefilled with the parsed title (not the raw file name), optional season/episode; results with release name, language, format, hearing-impaired flag, download count, and a match hint (hash match first). One click downloads through the server. Provider: OpenSubtitles (REST API, needs the admin's own API key/user; outbound per ADR 0010, off until configured). Server stores the file in the data directory linked to the media file; it appears as a subtitle track at once. | M6 |
| PL-21 | Upload | File picker for SRT, VTT, ASS/SSA; size limit; the server validates (parses) it, converts encoding to UTF-8 and stores it in the data directory. Needs the "upload subtitles" permission. | M7 |
| PL-22 | Manage external subtitles | In Media info: list of server-stored subtitles with delete. | M7 |
| – | Saving downloaded subtitles next to the media file | Not adopted (library roots are read-only, ADR 0022). | – |

### 9.4 Chapters

PL-30: horizontal row of chapter cards (thumbnail, title or "Chapter 3", start time); the current
chapter is marked; Enter/click seeks. Chapter thumbnails are generated (plan: Kapitelbilder);
without them the cards show times only. Milestone **M4**.

### 9.5 Queue

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| PL-31 | Queue model | `packages/player-core` holds the queue (items, current index, repeat, shuffle) framework-free; the web app persists it per tab session; the server stores it per profile in M7 so another device can continue. | M5 (client), M7 (server) |
| PL-32 | Queue panel | List with the current item marked, durations, remove, drag to reorder, "Clear upcoming". | M5 |
| PL-33 | Filling the queue | Play on a show/season queues its episodes from the start point; "Play all"/"Shuffle" (LB-24) queue the filtered list; "Play next"/"Add to queue" (section 7). | M5 |

### 9.6 More menu and media info

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| PL-35 | More menu | Add to playlist (M7), Re-analyze (M5, admin), Download (M7), Play history (M7), Media info (M5). | M5 |
| PL-36 | Media info dialog | Library and library-relative path (no absolute path), media block (duration, overall bitrate, resolution, aspect ratio, container, frame rate), file block (size, modified), one block per stream: video (codec, profile, level, bit depth, chroma, colour primaries/transfer, HDR format, frame rate, bitrate, scan type, title), audio (codec, channels and layout, sample rate, bitrate, language, title, default/forced), subtitle (codec, language, forced, external/embedded). "Copy as JSON" instead of a raw XML view. | M5 |

### 9.7 Mini player

PL-38: "Minimise" shrinks the video into a docked mini player (bottom right on desktop, above the
bottom navigation on phones) that keeps playing while the user browses; it shows title,
play/pause, progress, expand and close. Music uses the persistent player bar of plan screen 5
instead. Only one playback at a time. Milestone **M5**.

### 9.8 Stream transparency (Mivo differentiator)

Plex shows the delivery mode only in the admin dashboard. Mivo shows it to every viewer.

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| PL-40 | Mode indicator | Small badge in the player bar in the status colour (Direct Play green, Direct Stream blue, Transcode amber; ADR 0011) with the short mode name; click or `I` opens the overlay. Changes live when a track or quality switch changes the mode. | M5 |
| PL-41 | Stream-info overlay | Exists since M2-30: mode, translated reasons and warnings, per-stream source → output lines, container, encoder, source bitrate. Extended with: output bitrate and resolution of the chosen quality step (M4), hardware encoder and decoder name ("Intel Quick Sync (h264_qsv)", M4), tone mapping applied (M4), subtitle burn-in reason (M4), buffer ahead and network throughput measured by the client (M5), transcode speed (e.g. "2.3× real time") and throttling state (M4). | exists; M4, M5 |
| PL-42 | Explanations | Each reason has a one-line "why" and, where the user can act, a hint ("Choose an AAC track to avoid transcoding", "Disable subtitles to avoid burning in"). | M5 |
| PL-43 | Admin "Now playing" | The same data per session for the admin (11.4), plus user, profile, device, client address class (LAN/remote), CPU usage of the FFmpeg process. | M5 (read-only), M7 (users) |

## 10 Users, profiles and access

Accounts are local (ADR 0006). Mivo distinguishes **accounts** (can sign in with a password on
their own devices; the admin is one) and **managed profiles** (belong to an account, no password,
optional PIN, chosen in the profile picker). Friends join through invite links, not through a
vendor account.

### 10.1 Profile picker

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| US-01 | Picker screen | Shown after sign-in on a device when the account has managed profiles, and on "Switch profile". Tiles with avatar (initial on a token colour, or an uploaded image), name, lock icon when a PIN is required, an admin marker; "Guest" tile when guest access is on; "Add profile" for the account owner. | M7 |
| US-02 | PIN entry | 4-digit PIN pad (keyboard and pointer, TV-friendly); 5 wrong attempts lock the profile for 5 minutes (rate limit, plan security). | M7 |
| US-03 | Remember profile | "Remember on this device" per profile (default off for profiles with PIN). | M7 |

### 10.2 Managed profiles and content limits

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| US-10 | Create managed profile | Name, avatar, optional PIN, **content limit**, libraries, remote access allowed (M7b). | M7 |
| US-11 | Content limit presets | Presets by age (Mivo wording): "Young children", "Children", "Teens", "Adults" (no limit), mapped to the maximum certification per country (Germany FSK 0/6/12/16; US TV-Y/TV-G/G, TV-PG/PG, TV-14/PG-13; further countries in a table). "Custom" (free in Mivo): maximum certification plus **labels** to allow or block (ED-04). Unrated titles: hidden for limited profiles by default (switch). | M7 |
| US-12 | Enforcement | On the server for every list, row, search, detail and stream request – never only in the client. | M7 |

### 10.3 Admin: users and access (`/admin/users`)

| Id | Element | Behaviour | Milestone |
|---|---|---|---|
| US-20 | Users list | Accounts and their managed profiles: name, role (admin/user), content limit, last activity, remote access, status (invited/active/disabled). | M7 |
| US-21 | Invite | Creates an invite link (expiry, one-time) with preset libraries, content limit and permissions; the invitee chooses name and password. | M7 |
| US-22 | Library access | Per user: all libraries, selected libraries, or **single titles** (shows, movies, collections) – the latter also from an item's "Share with user" action. | M7 |
| US-23 | Permissions | Per user: download originals, upload subtitles, remote access (M7b), may transcode (off = Direct Play/Stream only), maximum remote bitrate (M7b), manage own profiles. | M7 |
| US-24 | Guest | Off by default. When on: a guest profile without sign-in, **LAN only**, with its own libraries and content limit; no watch state. | M7 |
| US-25 | Devices | Per account and globally: device name, client (web, TV app, DLNA, Chromecast), platform, app version, last seen, last address class (LAN/remote); filter by client type; **revoke** (ends its tokens and sessions). | M7b (plan: Geräteverwaltung) |
| US-26 | Sign-in log | Successful and failed sign-ins, new-device notice (plan security). | M7b |
| – | Sharing by vendor account / e-mail, friends lists | Not adopted (ADR 0006: invite links). | – |

## 11 Admin area

### 11.1 Structure

The admin area has its own sidebar (replacing the library sidebar while inside `/admin`) with
three groups, plus Users (10.3). Every settings page has a header with title and short
description, the **Expert settings** switch on the right, and one **Save** button at the bottom
(sticky); unsaved changes are marked and guarded.

```
Status        Dashboard · Activity log · Scheduled task runs
Settings      General · Remote access · Metadata · Library · Network · Transcoder ·
              Languages · DLNA · Scheduled tasks · Backup · Notifications (webhooks)
Manage        Libraries · Troubleshooting · Live log
Users         Users · Invites · Devices · Sign-in log
```

Writing settings needs authentication; until milestone 7, settings stay in `config.yaml`
(ADR 0022) and the admin area is read-only.

### 11.2 Settings for this device (`/settings/app`, every user)

| Id | Section | Content | Milestone |
|---|---|---|---|
| AD-01 | General | Interface language (en/de, Weblate later), time format, theme (dark; light later), hero on Home (HO-01), reset sidebar layout (SH-14). | M5 |
| AD-02 | Playback | Home quality, remote quality (Original / Automatic / fixed step), subtitle style (PL-13), auto play, skip intro automatically (M4). | M4 (quality, subtitles), M5 |
| AD-03 | Diagnostics (expert) | Force Direct Stream / force transcode for tests, show the client capability profile (what this browser reported), copy diagnostics. | M5 |

### 11.3 Account (`/settings/account`, every user)

Name, avatar, password, PIN of own profiles, preferred audio and subtitle language and subtitle
mode (Off / Forced only / When audio is foreign / Always), 2FA (M7b). Milestone **M7**.

### 11.4 Status group

| Id | Page | Content | Milestone |
|---|---|---|---|
| AD-10 | Dashboard | **Now playing** (PL-43) with a "Stop session" action (M7, admin), library scans with progress and last result (exists in the library page since M2), transcoder load (active/limit), server version and uptime, update notice (M7, ADR 0010), warnings (low disk space for the transcode directory, unavailable libraries, FFmpeg capabilities missing). | M5 (read-only), M7 |
| AD-11 | Activity log | Persistent event list: scans started/finished, items added/removed, metadata matched/failed, sign-ins (M7b), sessions started/ended, task runs; filter by type and level; live updates. | M7 |
| AD-12 | Task runs | History of scheduled task runs with duration and result. | M7 |
| – | Conversions ("optimized versions") | Later (section 13). | later |

### 11.5 Settings group

"(E)" marks options behind the Expert switch.

| Id | Page | Options | Milestone |
|---|---|---|---|
| AD-20 | General | Server name (shown in the TV app and DLNA), update check on/off (ADR 0010) and channel (stable/pre-release), log level (E), server version and licence information. No crash reports, no telemetry (ADR 0010). | M7 |
| AD-21 | Remote access | The wizard of the plan ("Fernzugriff einrichten"): status (reachable / not reachable, with the reason: CGNAT, no port mapping, …), path A (HTTPS + DuckDNS + port mapping) or path B (built-in Tailscale), reachability test, public port (E), custom certificate (E). | M7b |
| AD-22 | Metadata | Providers per library kind as an **ordered list** with enable switches: Local (NFO), Embedded tags, Local artwork, TMDB, MusicBrainz + Cover Art Archive, (later OMDb, TVDB, AniDB). Own TMDB key (optional, ADR 0009), OpenSubtitles credentials (PL-20), artwork language preference. "Powered by TMDB" attribution. Each online provider states the hosts it contacts (ADR 0010). | M6 |
| AD-23 | Library | Scan on file changes (watcher), periodic scan interval, remove missing items after N scans (E; protects against a share that is briefly away), "played" threshold (default 90 %), continue-watching window (weeks, max items, include new seasons), generation of trickplay thumbnails / chapter images / intro detection: never / during maintenance / when added. | M3 (config keys for watcher and interval), M4 (generation keys), M7 (page) |
| AD-24 | Network | Port (E; restart needed), base URL (E), trusted proxies (E), **LAN networks** (CIDR list; decides LAN vs. remote for quality defaults, the LAN-only guard, "admin only from LAN" and guest access), LAN-only guard switch (exists as `server.lan_only`), client discovery (E, M10), webhooks switch → Notifications. | M7 (M7b secure connections) |
| AD-25 | Transcoder | **Maximum simultaneous transcodes** (exists as `transcode.max_sessions`), transcode directory (read-only display when set by environment), hardware acceleration: auto / off / choose device, with a **"Test hardware acceleration"** button and its result per codec (M4), encoder preset (E), throttle ahead in seconds (E), disable video transcoding (E), HDR tone mapping on/off (M4), maximum output resolution (E; M2's 1080p cap). | M4 (backend + keys), M7 (page) |
| AD-26 | Languages | Server defaults for new profiles: preferred audio language, subtitle language, subtitle mode; metadata language default for new libraries. | M4 (track choice), M7 (page) |
| AD-27 | DLNA | Enable, server name override, renderer profiles list (detected renderers with the profile used, override), discovery interval (E), allowed networks (E). | M8 |
| AD-28 | Scheduled tasks | **Maintenance window** (start and end hour), task list with interval, last run, next run and "Run now": library scan, metadata refresh, trickplay/chapter generation, intro detection, database optimisation, cache cleanup, transcode directory purge, backup. | M7 |
| AD-29 | Backup | Backup interval and directory, keep last N, "Back up now", list of backups with download and **restore** (also on other hardware, plan). | M7 |
| AD-30 | Notifications | Webhooks (generic JSON, Discord, ntfy/Gotify) with event selection and a test button. Free (Plex: paid). | M7 |
| – | Extras (cinema trailers, pre-roll video) | Later (section 13). | later |

### 11.6 Manage group

| Id | Page | Content | Milestone |
|---|---|---|---|
| AD-40 | Libraries | List with kind icon, name, folders, item count, last scan, availability; per library **Scan**, **Edit** (12), **Customize rows** (HO-08, LB rows), **Remove** (confirm; watch state is kept by item identity where possible). "Add library" opens 12. Option "Merge recently added by media type". | M7 (the list read-only with scan buttons in M5) |
| AD-41 | Troubleshooting | Optimise database, clean caches (images, trickplay), download logs (zip, secrets redacted), download a support bundle (config with secrets removed, capability report), download database (admin, with a warning). | M7 |
| AD-42 | Live log | Streaming server log with level filter, search, pause, download; secrets redacted as in the file log (M1-05b). | M7 |
| – | Plugins | Not adopted in v1 (plan: no in-process plugins; webhooks and provider interface later). | – |
| – | Live TV & DVR | Later (plan milestone 11). | later |

## 12 Library settings dialog (add / edit library)

Admin dialog with tabs on the left; from M7 it replaces editing `libraries` in `config.yaml`
(ADR 0022: no unauthenticated library management).

| Id | Tab | Content | Milestone |
|---|---|---|---|
| LS-01 | General | **Kind** as large tiles: Movies, Shows, Music, Photos, Audiobooks, Mixed videos; name; metadata language; certification country. The kind cannot change after creation (create a new library instead). | M7 |
| LS-02 | Folders | One or more root folders; a **server-side folder browser** limited to the allowed browse roots (config key; in Docker `/media`, on DSM the granted shares, on Windows drive roots) so the browser cannot list system directories; checks of ADR 0022 (absolute, unique, not nested, not in data/cache/transcode). | M7 |
| LS-03 | Advanced | Visibility: in Home and search / only search / hidden (only via the sidebar); prefer local metadata (NFO, embedded tags) over providers; use local artwork; use original titles; artwork language; find extras (on/off); trickplay and chapter images (inherit / on / off); intro detection for shows; collections: show collections / hide items that are in a collection / show both; minimum size of automatic collections; ratings source shown first; episode ordering for shows (aired / absolute / DVD, M6). | M7 (effects implemented in M3/M4/M6) |
| – | Scanner and agent selection | Not adopted as separate concepts: Mivo has one scanner per kind and the ordered provider list (AD-22) per library. | – |

## 13 Not adopted

| Plex element | Why not |
|---|---|
| Vendor account sign-in, friends, sharing by e-mail, "find friends", public profile, activity feed, trending | Mivo has local accounts and invite links only (ADR 0006); no cloud identity, no social layer. |
| Watchlist synced with the vendor cloud, "discover" across services, streaming-service links ("more ways to watch") | Needs a vendor cloud and third-party catalogue data; contradicts ADR 0010. A local "Favourites" list (plan M7) covers the personal need. |
| Relay service | Plan "Fernzugriff": running a relay costs money; remote access uses path A or B. |
| Online part of the sidebar (the vendor's own section, discover, live TV, movies & shows, watchlist under "More") and online media sources | Owner instruction 2026-11-04: the Mivo sidebar shows only Home, the server's own libraries and Mivo's own sections (playlists, settings). These are vendor cloud content services. Mivo's own live TV/DVR (tuners, IPTV, milestone 11) is unaffected. |
| Community reviews, public ratings by Mivo users | Needs a central service; only provider ratings and the own star rating. |
| Upsell, paid features (hardware transcoding, labels, editions, custom content limits, statistics, webhooks) | No paid tier; these are free in Mivo where adopted. |
| Crash reports | ADR 0010: no telemetry. |
| Allow media deletion, "delete files", saving subtitles as sidecars | ADR 0022: Mivo never writes into library roots; Docker mounts them read-only. |
| **IP list allowed without authentication** | A list that skips sign-in turns every device in those networks into an admin-capable client and conflicts with the plan's security section (no default credentials, 2FA for admins). Mivo uses the LAN networks list (AD-24) only to classify LAN vs. remote. Quick Connect (ADR 0007) and the guest profile (US-24) cover the convenience case. Revisit only by a new decision. |
| Separate scanner/agent selection, legacy agents | One scanner per kind and an ordered provider list are simpler (12, AD-22). |
| Plugins | Plan: no in-process plugin system in v1. |
| Raw XML view of media info | Mivo offers "Copy as JSON" from its own API. |
| Cinema trailers / pre-roll, optimized versions (conversions), watch together | Not in v1 scope; **later** (milestone 11 list). |

## 14 Wording (Plex term → Mivo term)

Mivo uses its own wording; German strings follow the same choices in the locale files.

| Plex | Mivo (en) | Mivo (de) |
|---|---|---|
| Recommended / Library (pivots) | Overview / Browse | Übersicht / Durchsuchen |
| Hub | Row | Reihe |
| On Deck | Up next | Als Nächstes |
| Continue Watching | Continue watching | Weiterschauen |
| Manage Recommendations | Customize rows | Reihen anpassen |
| Show Advanced | Expert settings | Experteneinstellungen |
| Playback Settings | Playback options | Wiedergabeoptionen |
| Play Queue | Queue | Warteschlange |
| Get Info | Media info | Medieninfo |
| Fix Match / Unmatch | Correct match / Remove match | Zuordnung korrigieren / Zuordnung entfernen |
| Edit (item) | Edit details | Details bearbeiten |
| Analyze | Re-analyze | Neu analysieren |
| Grant Access | Share with user | Für Nutzer freigeben |
| Managed account | Managed profile | Verwaltetes Profil |
| Restriction profile | Content limit | Altersgrenze |
| Select User | Choose profile | Profil wählen |
| Alerts | Activity log | Aktivitätsprotokoll |
| Console | Live log | Live-Log |
| Authorized Devices | Devices | Geräte |
| Square Art | Square artwork | Quadratisches Bild |

## 15 Milestone summary

| Milestone | UX elements delivered |
|---|---|
| M3 Libraries complete | Show → season → episode hierarchy with minimal pages (DT-20…22), local artwork and image endpoint (LB-44, LB-40), NFO fields (summary, genres, people names), extras scan, absolute-numbering parser test (DT-24), config keys for watcher and periodic scan (AD-23) |
| M4 Streaming | Playback options panel: quality ladder, audio, subtitles, subtitle style (PL-10…13); detail technical selectors (DT-07); chapters (PL-30); trickplay and chapter ticks (PL-05); skip segment (PL-07); generated episode stills (DT-23); overlay extensions (PL-41); hardware test and transcoder keys (AD-25); device playback settings (AD-02) |
| M5 Web UI complete | Shell (SH-03, SH-04, SH-12…18), Home rows and optional hero (HO-01, HO-02, HO-05, HO-07), library pivots, toolbar, filters, sort, folders, views, size slider, A–Z, virtual grid, selection (LB-02…04, LB-09, LB-20…30, LB-41…43), detail layout (DT-01…03, DT-05, DT-06, DT-08, DT-09, DT-11), search (SR-01…03), player layout, queue, mini player, media info, mode indicator (PL-01…04, PL-08, PL-09, PL-14, PL-15, PL-31…36, PL-38, PL-40, PL-42), read-only dashboard (AD-10, PL-43), settings for this device (AD-01, AD-03) |
| M6 Online metadata | Edit details dialog (ED-01…03, ED-05…07), ratings (DT-04), portraits and person pages (DT-08, DT-30), related (DT-10), people rows and top rated (LB-05, LB-08), unmatched filter, online subtitle search (PL-20), metadata settings (AD-22), provider episode ordering |
| M7 Users and administration | Profiles and picker (US-01…03), managed profiles and content limits (US-10…12), users, invites, access, permissions, guest (US-20…24), watch state everywhere (HO-03, HO-04, HO-06, LB-01, LB-06, LB-07, badges), labels and batch edit (ED-04, ED-08), playlists, collections, history, downloads, subtitle upload (PL-21, PL-22), row configuration (HO-08), admin settings pages (AD-11, AD-12, AD-20, AD-23…26, AD-28…30), manage pages (AD-40…42), library dialog (LS-01…03) |
| M7b Remote access | Remote access page (AD-21), devices and revoke (US-25), sign-in log (US-26), remote permissions and bitrate limits |
| M8 DLNA + Chromecast | Cast button (SH-06), DLNA page (AD-27) |
| M10 webOS | The same IA in 10-foot form: Home rows, library Browse with filters and sort, detail, player with the playback options side panel and stream info, profile picker with PIN pad, Quick Connect |
| later | Conversions, cinema trailers/pre-roll, watch together, live TV pivot |
