# Privacy: outbound connections

Mivo sends no telemetry, analytics, crash reports or usage statistics (ADR 0010). The web app
loads no third-party assets: fonts, icons and the TMDB logo are bundled and served by your
server. This page lists every host the server contacts, when, what it sends and how to turn
it off. Hosts not listed here are never contacted.

All connections use HTTPS, send no cookies and identify themselves with the User-Agent
`Mivo/<version> (+<project URL>)` (MusicBrainz requires a contact URL).

| Feature                           | Hosts                                                                                       | Trigger                                                                                       | Data sent                                                                                                          | How to disable                                                                                                                                          |
| --------------------------------- | ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Update check (planned, not built) | `api.github.com`                                                                            | Once a day, once the feature ships                                                            | The request for the latest release of the project repository; nothing about your server or media                   | Will be a switch in Settings › General and in the configuration file. Until it ships, no update check runs.                                            |
| TMDB (movies and shows)           | `api.themoviedb.org`, `image.tmdb.org`                                                      | A scan finds a new movie or show, a metadata refresh, or "Identify" on an item                | Normalised title, year, season and episode numbers, metadata language, provider IDs and the API key                | Settings › Metadata: switch off "TMDB" or "Look up metadata online"; per library: "Use online metadata" in the library dialog; or `metadata.online.enabled: false` |
| MusicBrainz (music)               | `musicbrainz.org`                                                                           | A scan finds a new album or artist, a metadata refresh, or "Identify" on an item              | Album, artist and track titles, year, metadata language and MusicBrainz IDs                                         | Settings › Metadata: switch off "MusicBrainz and Cover Art Archive" or "Look up metadata online"; per library: "Use online metadata"                   |
| Cover Art Archive (album covers)  | `coverartarchive.org`, plus `archive.org` and `*.archive.org` (cover JSON and images) | After MusicBrainz identified an album, to fetch its cover                                     | The MusicBrainz release ID in the URL                                                                              | Same switch as MusicBrainz; per library: "Use online metadata"                                                                                          |

Never sent to any provider: file names, folder paths, library names, file hashes or sizes,
user names, or anything else from your catalogue.

## Switches

- **Global:** Settings › Metadata › "Look up metadata online", or `metadata.online.enabled` in
  the configuration file. Off means no connection to any metadata provider.
- **Per provider:** Settings › Metadata › Providers.
- **Per library:** "Use online metadata" in the Advanced tab of the library dialog.
- **Own TMDB key:** Settings › Metadata. The key is stored on the server and never returned by
  the API or shown again; removing it falls back to the project key.

## Attribution

Data from TMDB is credited with "Powered by" and the TMDB logo plus the notice "This product
uses the TMDB API but is not endorsed or certified by TMDB" in Settings › Metadata, Settings ›
About and the footer of every item page whose data came from TMDB. MusicBrainz data (CC0) and
Cover Art Archive images (each under its own licence) are credited by name in the same places.

Decisions: ADR 0010 (outbound connections), ADR 0031 (online metadata providers).
