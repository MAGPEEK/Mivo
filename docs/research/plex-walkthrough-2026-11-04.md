# Plex walkthrough – UX observations (2026-11-04)

- Purpose: input for Mivo's UX specification (`docs/design/ux-spec.md`) and the plan update of
  2026-11-04 (owner request: "study the UX design – overviews and arrangements, backend/admin and
  user control, per-item settings, the player – and adopt it for our plan").
- Source: the owner's own Plex Media Server installation (server 1.43.4, Plex Web 4.160.0) on the
  owner's Synology NAS, opened by the owner and walked **read-only** in a browser. Nothing was
  changed on the server.
- Content: one movie library (about 370 titles) and one TV show library.
- Privacy: account names, e-mail addresses, IP addresses, device lists and library titles are
  deliberately left out. The only device fact kept is the TV model (section 12), because it
  affects a plan decision. Screenshots stayed local and are not part of the repository.
- This file records **what was observed**. It contains no Plex assets and quotes no UI texts
  beyond the short labels needed to identify an element. Mivo's own design and wording are in
  `docs/design/ux-spec.md`.

## 1 Global shell and navigation

**Top bar** (left to right)
- Hamburger button: collapses or expands the sidebar.
- Product logo.
- Global search field. Typing opens a dropdown with "top results": poster thumbnail, title, year,
  type, owning server, a quick-play button and an overflow (⋮) menu per result. Enter opens a
  full search page with pivots per result type (top results, movies, shows, episodes, live TV,
  people, streaming services).
- Upsell button for the paid subscription.
- Activity icon: running server activities (scans, metadata refresh) and a "server update
  available" entry that links to the admin dashboard.
- Settings icon (wrench), cast / "select player" icon, account avatar menu.

**Left sidebar**
- Home, then one entry per library with an icon by library type.
- A "More" entry flips the sidebar into a full list: every library with a **pin toggle**, plus
  the vendor's online sections. Libraries can be pinned, unpinned and **reordered**, so the
  sidebar is customisable per user; a "reset customisation" action exists in the client
  settings.

**Common patterns**
- Every library page has two pivots (tabs): a **recommended** view made of rows and a **library**
  view with the full grid.
- Every list page has a **poster size slider** and a **grid/list toggle** at the top right.
- URLs are deep-linkable (hash routes for item details, a library with its pivot, and a search
  query).

## 2 Home

- Pivots at the top: Home plus vendor social features (trending, activity feed, find friends,
  own profile). These are services of the vendor's cloud.
- No hero banner in this version and setup: Home consists only of horizontal rows ("hubs").
- Each row scrolls horizontally with left/right arrow buttons and has a "see all" link.
- Rows observed:
  - Continue watching: 2:3 posters with a progress bar; for episodes the show title, the episode
    title and a compact "S3 · E9" label.
  - Recently added, one row **per library**. Episodes are grouped: a show card carries an
    unwatched-count badge (for example "47") instead of one card per episode.
- A server option merges the per-library "recently added" rows by media type. Each library can
  be included in or excluded from Home ("manage recommendations").
- API calls seen: continue-watching hub, promoted hubs per library, home "recently added" hub,
  library listing sorted by date added and paged with a start offset and a page size of 24,
  a **server-side image resize endpoint** (posters requested at 300 × 450), session status,
  server activities, server preferences, per-library preferences, updater status.

## 3 Library – recommended view

**Movies**: rows "recently released", "recently added", "top movies in <genre>", "top movies
with <actor>", "top unwatched", "recently watched", more further down. Rows are generated on the
server and configurable per library.

**Shows**: continue watching; recently released episodes (shown as show posters with an
episode subtitle such as "S3 · E7"); recently added (show cards with an unwatched badge, or a
season card "season 3"); "start watching" (unwatched shows with episode-count or season-count
badges); more in <genre>; top rated.

**Parser defect seen**: an anime series that uses absolute episode numbers was listed with
"130 seasons" – the absolute episode numbers were taken as season numbers. This is a test case
for Mivo's name parser (milestone 3).

## 4 Library – library view (grid)

**Toolbar**
- Left: filter dropdown ("all"), **type dropdown** with the library's item type or **"folders"
  (browse the file system folders of the library)**, sort dropdown ("by title"), item count.
- Right: play all, shuffle, add to playlist or collection, more.

**Filter menu**
- Quick filters: all, HDR, Dolby Vision, HDR10+, Atmos, unwatched, in progress, unmatched,
  duplicates.
- Field filters: genre, year, decade, content rating, collection, director, actor, writer,
  producer, country, studio, resolution, and more.

**Sort menu** (ascending/descending each): title, year, release date, critic rating, audience
rating, own rating, content rating, duration, progress, plays, date added, date viewed,
resolution, bitrate, random.

**Grid**
- A–Z jump bar on the right edge (#, A … Z).
- 2:3 posters with title and year below. Hover shows a play button, an overflow menu, a watched
  toggle and an edit pencil; watched items carry a check mark in a corner.
- View styles: grid, list, detailed list.

## 5 Movie detail page

- Layout: blurred backdrop; poster on the left with a content-rating badge; right column with
  title, "directed by …", a line with content rating, year, runtime ("1 hr 39 min") and genres
  (the first ones, then "and more"; genres link to the filtered library).
- Ratings row: ratings from several providers (IMDb score, Rotten Tomatoes critic and audience
  percentages, TMDB percentage) and a "rate & review" action.
- Action buttons: play (primary; becomes "resume" with a progress indication), add to the
  vendor's watchlist, mark as watched, edit, share, overflow menu.
- Overflow menu: watch together, play next, add to queue, add to playlist/collection, more ways
  to watch (streaming services), refresh metadata, analyze, fix match, unmatch, optimize
  (pre-convert), save file (download), grant access, delete, view play history, report issue,
  get info.
- Summary with a "more" expander.
- Technical block with three dropdowns: video version ("1080p (H.264)", a dropdown only when
  several versions exist), audio track ("English (DTS 5.1)"; lists all tracks; the choice is
  remembered per item) and subtitles ("none"; tracks plus "search…" and "upload…").
- Sections below: cast & crew (round portraits, name, role; paged carousel; each links to a
  person page with filmography), ratings & reviews (community and critic reviews), more ways to
  watch, related movies (row), extras and trailers when present.

## 6 Show → season → episode

- **Show page** like the movie page. The poster shows the next episode to watch ("on deck":
  "S1 · E1") and an unwatched badge; play starts that episode. Ratings row, summary, then a
  **seasons row** (season posters with unwatched badge, "season 1 – 9 episodes"), cast & crew,
  reviews, related shows.
- **Season page**: season poster with a play overlay, show and season title, play, actions,
  summary, cast, reviews, then "9 episodes" as a grid of **16:9 thumbnails** (episode title,
  "episode 8"; hover: play, edit, overflow, watched toggle). The episodes were ordered
  **descending** here. Previous/next season arrows in the top bar, list/grid toggle and size
  slider.

## 7 Item edit dialog (per movie)

Tabbed dialog with "cancel" and "save changes":
- **General**: title, sort title, original title, edition (paid feature), originally available
  (release date), content rating, rating, studio, tagline, summary. **Every field has a lock
  icon** that pins its value against metadata refreshes.
- **Tags**: directors, country, genres, writers, producers, collections – chip inputs with a
  remove (×) per chip.
- **Labels** (paid feature): free labels, used for sharing restrictions.
- **Poster**, **background**, **logo**, **square art**: the same picker each – a gallery of
  candidate images from the metadata providers, plus choose file, drag and drop, or URL.
- **Info**: file paths of all parts.
- **Advanced**: per-item metadata language, use original title.

## 8 Player (web, full player)

**Layout**
- Top: collapse button on the left (shrinks the player into a **mini player** while browsing),
  fullscreen on the right.
- Bottom bar, left: title, year, "elapsed / total".
- Bottom bar, centre: previous, skip back 10 s, play/pause, skip forward 30 s, next, stop.
- Bottom bar, right: more actions, repeat, shuffle, playback settings, chapters, play queue,
  volume (mute and slider).
- Seek bar across the full width with a hover time tooltip; preview thumbnails only when the
  server generated them (switched off on this server).

**Playback settings panel** (slides up over the video)
- Quality: play original quality (with bitrate and resolution, checked), convert automatically,
  "show all" → a bitrate ladder.
- Audio stream: list of tracks (for example "German (AC3 stereo)", "English (DTS 5.1)").
  Switching restarts the stream briefly; the choice is saved for the item.
- Subtitles: none, tracks, "search…", "upload…".
- Subtitle size (only while a subtitle is active).
- Auto play (next item) checkbox.

**Subtitle search dialog**: language dropdown and a query prefilled with the file name; results
from OpenSubtitles with release name, language and format, and download count; one click
downloads the subtitle and stores it next to the media file (sidecar).

**Chapters panel**: horizontal row of chapter cards (thumbnail, chapter number, start time).

**Play queue panel**: list of queued items with a marker for the current one and durations;
add-to-queue button.

**More actions**: add to playlist, analyze, save file, grant access, view play history, get
info.

**Media info dialog** ("get info"): file path (with "delete files"); media block (duration,
bitrate, resolution, aspect ratio, container, frame rate, audio and video profile); part block
(file, size); one block per stream – video (codec, bitrate, bit depth, chroma subsampling,
level, profile, reference frames, scan type, title) and audio (codec, channels, layout, bitrate,
language and tag, sampling rate, display title); a raw XML view.

**Stream decision** (seen in network requests): the client asked the server's decision endpoint
for HLS with direct play off, direct stream (remux) of the video on, audio not direct
(DTS → AAC), subtitles burned in, and an added client-profile rule allowing H.264 and HEVC
video, AAC audio over HLS. This is the same strategy as Mivo's milestone 2 (remux video,
transcode audio).

**No stream-mode indicator in the player**: the web player shows nowhere whether it plays
directly, remuxes or transcodes, or why. That information exists only in the admin dashboard
("now playing"). Mivo's stream-info overlay is therefore a differentiator.

**Progress**: starting playback records progress at once; the item then shows "resume" and
appears in "continue watching".

## 9 User and profile control

- **Profile picker** at start-up (and in a fresh browser tab): tiles with avatar initial and
  name, a lock icon for profiles that need a **PIN**, a marker for the admin, a "guest" tile and
  "add user". Choosing a locked profile asks for its PIN.
- **Home-users settings**: create a **managed account** (name plus a **restriction profile**
  preset: younger kid, older kid, teen – each a set of maximum US content ratings – or a custom
  profile as a paid feature); edit PIN; enable/disable guest; a remote-watch restriction per
  user.
- **Library access management**: list of accounts with access; "grant library access" by user
  name or e-mail → choose libraries **or single titles**, and restrict features (downloads, live
  TV, …).
- **Authorised devices**: filter by platform group (all, iOS, other, server, web); each device
  with name, last seen, app version, product, platform/OS; can be removed (revoke).
- Further account pages: account, online media sources, watchlist, webhooks (paid), streaming
  services, privacy.
- Item level: "grant access" shares one item.

## 10 Admin / server settings

The settings sidebar has three sections: account (section 9), web client and server.

**Web client** (settings stored for this browser)
- General: language, auto sign-in, remember the selected tab, theme music, insecure connection
  fallback, time format, settings sync, reset customisation.
- Quality: automatic adjustment, internet streaming quality, "play smaller videos at original
  quality", home streaming quality.
- Debug; Player: direct play / direct stream toggles, alternate protocol.

**Server**, in three groups:
- **Status**: dashboard ("now playing" sessions; statistics are a paid feature), alerts (a live
  event log such as "loading metadata for …", "finished processing …"), conversions (jobs that
  pre-convert media).
- **Settings**:
  - General: friendly server name, version with check/install update, update channel, crash
    reports, debug and verbose logging.
  - Remote access: reachability status ("not reachable from outside"), a diagram of private and
    public address and port, manual public port, retry.
  - Metadata agents: providers, and agents composed of an **ordered list of providers** (movie,
    series, music, personal media, NFO movie, NFO series, local media); custom provider URLs.
    A legacy agents page also exists.
  - Library: scan automatically on change, partial scan, periodic scan interval, empty trash
    after scan, allow media deletion, continue-watching window in weeks plus maximum items and
    "include season premieres", "played" threshold (90 %), generation of preview thumbnails, ad
    markers, chapter thumbnails and loudness analysis (never / as a scheduled task / when media
    is added), location visibility, database cache size.
  - Network: secure connections (required / preferred), custom certificate, preferred network
    interface, strict TLS, local discovery, relay, custom access URLs, **a list of IP addresses
    and networks that are allowed without authentication**, webhooks toggle.
  - Transcoder: quality profile, temporary directory, download temporary directory, throttle
    buffer, background encoder preset, disable video transcoding, **maximum simultaneous CPU
    transcodes**, maximum background transcodes; a hardware acceleration toggle appears with the
    paid plan and suitable hardware.
  - Languages: auto-select audio and subtitle tracks, preferred audio language, subtitle mode
    ("shown with foreign audio"), preferred subtitle language.
  - DLNA: enable, client preferences, timeline reporting, protocol info, discovery interval,
    lease time, icons.
  - Scheduled tasks: **maintenance window** (start and end hour), database backup every three
    days with a backup directory, weekly database optimisation, removal of old bundles and
    caches, refresh local metadata, update libraries, upgrade media analysis, periodic metadata
    refresh, extensive media analysis.
  - Extras: cinema trailers before a movie, pre-roll video, music video path.
- **Manage**:
  - Libraries: list with type icons; per library "manage recommendations" and "edit library";
    "add library", "scan library files"; the "merge recently added" option.
  - Optimized versions, live TV & DVR, plugins.
  - Troubleshooting: optimise database, clean bundles, download logs, download database.
  - Console: live log.
- **Every settings page** has a "show advanced / hide advanced" toggle that hides expert
  options, and one "save changes" button per page.

## 11 Library settings ("edit library" dialog)

Tabs on the left:
- **General**: library type as large tiles (movies, TV shows, music, photos, other videos), name,
  metadata language.
- **Add folders**: several root folders per library.
- **Advanced**: scanner, agent, visibility (include in home and search / exclude from home /
  exclude from both), cinema trailers, certification country, use original titles, prefer
  artwork in the library language, use local assets, prefer local metadata (embedded tags,
  NFO), find extras (all / trailers only / red-band / localized subtitles), allow explicit
  matches, minimum size of automatic collections, ratings source (Rotten Tomatoes, IMDb, TMDB),
  enable video preview thumbnails and delete existing ones, collections display mode (hide
  collections / hide items in collections / show both), ad detection.

## 12 Device fact relevant to the plan

- The owner's TV, as listed by the server: **LG OLED55C97LA, webOS 4.10** (model year 2019),
  running the vendor's webOS app 5.94.
- Mivo's plan and ADR 0013 target **webOS 6.0 and newer**, so the owner's own TV would not be
  supported. This is open decision OD-UX-1 in `docs/plan.md`.
- To verify on the device: LG markets the 2019 line as "webOS 4.5"; the "4.10" string may be the
  firmware's platform version. The browser engine of webOS 4.x is Chromium 53 according to LG's
  developer site (verify on the device with `ares-inspect`).

## 13 Take-aways for Mivo (summary)

1. Plex's information architecture – shell, Home rows, library pivots with a powerful toolbar,
   detail pages with a technical block, an edit dialog with field locks, a player with a
   settings panel, profiles with PIN, an admin area grouped into status / settings / manage with
   progressive disclosure – is a proven, familiar structure. Mivo adopts the structure and the
   interaction patterns, not the assets, texts or the vendor's cloud features.
2. Plex hides how a stream is delivered from the viewer. Mivo shows it (stream-info overlay and a
   mode indicator in the player bar).
3. Plex gates several features behind a subscription (hardware transcoding, labels, editions,
   statistics, custom restriction profiles, webhooks). Mivo has no paid tier.
4. Plex writes into media folders (downloaded subtitles as sidecars, media deletion). Mivo never
   writes into library roots (ADR 0022) and stores such files in its own data directory.
5. The absolute-episode misparse is a regression test for Mivo's name parser.
