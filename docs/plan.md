# Mivo (Arbeitstitel, vormals „Any Media Server“) – Architektur, Tech Stack & Design

## Festgelegte Entscheidungen
| Thema | Entscheidung |
|---|---|
| Zielgruppe / Lizenz | Open Source für alle, **GPL-3.0** |
| Name | **Mivo** als Arbeitstitel (es gibt eine Kollision mit der indonesischen App „Mivo TV“). Der Name ist zentral konfigurierbar (Binary, Docker-Image, App-ID) und wird vor dem ersten öffentlichen Release endgültig festgelegt. |
| Erster Release | **v0.1-Alpha = Video-Kern**: Filme und Serien scannen, jedes Format im Browser, Docker für NAS. Danach Releases alle zwei bis vier Wochen. |
| Mindest-Hardware | auch schwache ARM-NAS ab 1 GB RAM (arm64/armv7). Ohne Transcoding-Leistung laufen nur Direct Play und Remux, mit klarem Hinweis in der UI. |
| Konten | nur lokal, Freunde per Einladungslink, kein Cloud-Dienst |
| TV-Kopplung | rein lokal: Netzwerksuche (JS-Service), sonst manuelle Adresseingabe. Login per QR-Code bzw. Quick-Connect-Code, bestätigt im Browser am Handy. Eine offizielle Online-Instanz kommt später. |
| Mobile | PWA, vorerst keine nativen Apps |
| Fotos | Galerie ohne KI: Zeitleiste, Ordner, Alben, EXIF, Karte, Diashow, RAW/HEIC. Kein Handy-Upload. |
| Metadaten-Keys | mitgelieferter Projekt-Key für TMDB, eigener Key optional, Quellenhinweis „Powered by TMDB“ |
| Ausgehende Verbindungen | nur ein täglicher Update-Check gegen GitHub-Releases, abschaltbar, keine Telemetrie |
| Testhardware | Synology Plus mit AMD (ohne iGPU, nur Software-Transcoding), LG-TV mit webOS 23+, Chromecast und DLNA-TV, Windows-PC mit NVIDIA (NVENC). QNAP und Intel Quick Sync sind „Community-getestet“. Empfehlung: später ein Intel-N100-Gerät für QSV-Tests. |
| webOS-Mindestversion | 6.0 (2021) – **erneut offen** (OD-UX-1): der eigene TV des Owners ist ein LG OLED55C97LA mit webOS 4.10 (2019) |
| LG-Seller | später entscheiden. Bis dahin wird die App per Developer Mode verteilt. |
| Design | näher an Plex: dichtere Poster, Akzent **Koralle #FF6B57**. Seit 2026-11-04 (Owner-Auftrag nach dem Plex-Rundgang): Informationsarchitektur und Bedienmuster von Plex werden übernommen – Shell, Start-Reihen, Bibliothek mit „Übersicht“/„Durchsuchen“, Detailseiten, Bearbeiten-Dialog mit Feldsperren, Player mit Wiedergabeoptionen, Profile mit PIN, Admin-Bereich „Status/Einstellungen/Verwalten“. Eigener Name, Logo, Farben und Wortlaut; keine Plex-Assets, keine Cloud-/Social-Funktionen. Der Hero-Bereich bleibt, ist aber abschaltbar. Spezifikation: `docs/design/ux-spec.md`, Rundgang: `docs/research/plex-walkthrough-2026-11-04.md` |
| Sprachen | Englisch als Basis für Code, Doku und UI, Deutsch vollständig, weitere über Weblate |
| Repository | GitHub `MAGPEEK/Mivo` (Remote `origin`). Push nur nach Freigabe. |
| Dev-Umgebung | nativ unter Windows (Go, Node, FFmpeg), Linux-Tests in Docker Desktop und auf der Synology |
| Migration | Import aus Plex und Jellyfin (Gesehen-Status, Fortschritt, Bewertungen, Playlists) **nach v0.1** |
| Synology-Paket (SPK) | **vorgezogen** (Entscheidung 2026-10-31): natives SPK für DSM 7.2+, parallel zu Meilenstein 2, erstes SPK mit 0.1.0-alpha.2; je ein SPK für x86_64, armv8 und armv7 mit gebündeltem statischem FFmpeg. Docker bleibt die primäre NAS-Verteilung, QPKG bleibt „später“. Details: ADR 0026, Aufgaben `docs/tasks/SPK-*`. Ergänzung 2026-11-04: **Verknüpfung im DSM-Hauptmenü** (Mivo-Symbol im DSM-Startmenü wie bei anderen Paketen, öffnet die Web-App), wird in der laufenden SPK-Arbeit umgesetzt (ADR 0026, Notes UX-Plan) |

## Offene Entscheidungen (Stand 2026-11-04)

Vom Owner zu entscheiden; der Architekt entscheidet sie nicht. OD-UX-2 ist entschieden (B); OD-UX-1 und OD-UX-3 sind offen.

**OD-UX-1 – webOS-Mindestversion.** Der eigene TV des Owners (LG OLED55C97LA, Modelljahr 2019) meldet **webOS 4.10** (LG vermarktet die 2019er-Reihe als „webOS 4.5“; am Gerät prüfen). Der Plan und ADR 0013 verlangen webOS 6.0+, der TV wäre also nicht unterstützt. Auch die Zeile „Testhardware: LG-TV mit webOS 23+“ passt nicht dazu – bitte bestätigen, welche LG-TVs zum Testen wirklich verfügbar sind.
- A: **6.0 bleibt.** Moderne Engine (Chromium 79), weniger Transpiling, einfachere Profile. Der eigene TV bleibt außen vor; Tests auf Emulator bzw. einem anderen Gerät.
- B: **Auf webOS 4.x senken** (ADR 0013 würde durch ein neues ADR ersetzt). Engine laut LG Chromium 53 (5.0: Chromium 68; am Gerät prüfen): kein natives `async/await`, kein CSS Grid, keine ES-Module → Build für die TV-App mit Legacy-Transpiling und Polyfills; gemeinsame Pakete (`player-core`, `api-client`, `ui-tokens`) dürfen nur transpilierbare Sprach- und API-Features nutzen; Client-Profile pro Modelljahr (2018/2019 haben andere Codec-Listen, z. B. DTS, Dolby Vision, AV1 fehlt); MSE/hls.js und die Spatial-Navigation-Bibliothek auf Chromium 53 prüfen. Vorteil: der eigene TV ist das Testgerät.
- C: **4.x als „Best Effort“**: der Build zielt auf 4.x, freigegeben und voll getestet wird nur 6.0+, der eigene TV ist das einzige 4.x-Testgerät.
- Dringlichkeit: die Antwort bestimmt schon jetzt, welche Sprachfeatures die gemeinsamen Pakete in Meilenstein 4/5 verwenden dürfen, nicht erst in Meilenstein 10.

**OD-UX-2 – Reihenfolge der Meilensteine – entschieden 2026-11-04: B (3 → 6 → 5 → 4 → 7).** Der Owner empfindet die Alpha als „sehr rudimentär“.
- A: **Plan-Reihenfolge 3 → 4 → 5 → 6 → 7.** Technisch geradlinig; sichtbar viel UI erst nach dem Streaming-Ausbau.
- B: **Vorschlag des Orchestrators 3 → 6 → 5 → 4 → 7** (Namensparser und lokale Metadaten, dann TMDB mit Postern, dann die komplette UI): größter sichtbarer Sprung. Folge: der Player in Meilenstein 5 hat noch keine Audio-/Untertitelwahl und keine Qualitätsstufen (kommt mit 4); der Bearbeiten-Dialog (Meilenstein 6) erscheint vor dem vollständigen UI-Gerüst und muss in Meilenstein 5 nachgezogen werden.
- C: **wie B, aber mit einem kleinen Teil von 4 in 5**: Audiospur-Wahl und Text-Untertitel (SRT/VTT/ASS) kommen mit dem Player-Umbau in 5; HW-Beschleunigung, Bild-Untertitel, Trickplay, Tonemapping, Bandbreitenstufen bleiben in 4. Ein Player ohne Spur- und Untertitelwahl wirkt sonst weiter rudimentär.

**OD-UX-3 – Fortschritt und Gesehen-Status vor den Nutzerkonten.** „Weiterschauen“, „Als Nächstes“, „Fortsetzen“ und Gesehen-Markierungen brauchen `watch_state`, der laut Plan erst mit den Konten in Meilenstein 7 kommt.
- A: **erst in Meilenstein 7.** Bis dahin fehlen auf der Startseite die wichtigsten Reihen.
- B: **ab Meilenstein 5 für ein implizites Standardprofil** (LAN-only-Guard aktiv); Meilenstein 7 ordnet die Daten dem Admin-Konto zu. Folge: ADR 0022 („nichts Nutzereigenes, Bibliothek löschen kaskadiert“) bekommt eine Notiz, weil ab dann nutzereigene Daten existieren, die ein Neuscan oder ein geänderter Bibliothekspfad nicht löschen darf.

---

## Kontext

Ziel ist ein eigener Medienserver ("Any Media Server", kurz **Mivo**), der vor allem auf Linux läuft, insbesondere auf Synology DSM und QNAP, und zusätzlich auf Windows. Er soll praktisch **jedes Container-Format** abspielen können. Die Bedienung orientiert sich an Plex Media Server: dunkles UI, Poster-Raster, „Weiterschauen“-Reihen. Name, Logo und Farben sind aber eigenständig, es wird kein Plex-Branding verwendet.

Entscheidungen des Users:
- Alle Medienarten von Anfang an: Filme, Serien, Musik, Fotos, Hörbücher, Disc-Images
- Backend in **Go**
- Clients: **Web-App + DLNA/UPnP + Chromecast + LG-webOS-TV-App** (Veröffentlichung im LG Content Store)
- NAS-Verteilung: **zuerst Docker** (Multi-Arch), native SPK/QPKG-Pakete später

Der Projektordner `C:\Users\stefa\Documents\Any Media Server` ist leer. Es wird alles neu aufgebaut.

---

## Grundprinzip „jedes Format abspielen“

Kein Client kann alle Formate. Deshalb **passt der Server jeden Stream an den jeweiligen Client an**, und zwar mit drei Stufen:

| Stufe | Wann | Was passiert | CPU-Last |
|---|---|---|---|
| **Direct Play** | Client kann Container + Codecs | Datei wird per HTTP-Range unverändert ausgeliefert | ~0 |
| **Direct Stream (Remux)** | Codecs passen, Container nicht (z. B. MKV/AVI/TS mit H.264+AAC im Browser) | FFmpeg packt ohne Neukodierung in fMP4/HLS um | sehr gering |
| **Transcode** | Codec nicht unterstützt (z. B. HEVC 10-bit, DTS, TrueHD, VC-1, MPEG-2) | FFmpeg kodiert neu, nach Möglichkeit mit Hardware (VAAPI/QSV/NVENC/AMF) | hoch |

- **Demuxing/Decoding:** FFmpeg (libavformat/libavcodec) unterstützt über 300 Container und Codecs. Gebündelt wird ein angepasster FFmpeg-Build (z. B. *jellyfin-ffmpeg*, GPL, als **separater Prozess** aufgerufen, nicht gelinkt) mit libbluray, libdvdnav, libass und HW-Treibern.
- **Entscheidungs-Engine:** Jeder Client meldet ein Capability-Profil, im Browser über `MediaCapabilities`/`canPlayType`, bei DLNA über Renderer-Profile. Die Engine vergleicht es mit dem ffprobe-Ergebnis für Container, Video, Audio und Untertitel und wählt die günstigste Stufe. Der Grund wird im Player angezeigt, z. B. „MKV → Remux zu fMP4“.
- **Abgedeckte Formate:**
  - Video: MKV, MP4/M4V, AVI, MOV, TS/M2TS/MTS, WebM, FLV, WMV/ASF, OGV, 3GP, MPEG-PS/VOB, RMVB, DivX/Xvid, MXF
  - Disc: ISO, BDMV- und VIDEO_TS-Ordner, nur **unverschlüsselte** Discs, da CSS/AACS aus rechtlichen Gründen nicht umgangen werden
  - Audio: FLAC, ALAC, MP3, AAC, Opus, Vorbis, WAV, AIFF, APE, WavPack, DSD (DSF/DFF → PCM), M4B
  - Fotos: JPEG, PNG, WebP, GIF, HEIC/HEIF, AVIF, TIFF, RAW (CR2/CR3/NEF/ARW/DNG/ORF/RW2), Live-/Kurzvideos
- **Untertitel:** SRT/VTT werden direkt geladen, ASS/SSA im Browser mit libass-WASM (JASSUB) gerendert. Bild-Untertitel (PGS/VobSub/DVB) werden beim Transcoden eingebrannt. Externe Untertiteldateien neben dem Video werden automatisch erkannt.

---

## Tech Stack

### Backend (Go, eine einzelne Binary `mivo`)
| Bereich | Wahl | Begründung |
|---|---|---|
| Sprache | Go (aktuelle Stable) | statische Binary, Cross-Compile für amd64/arm64/armv7, geringer RAM-Bedarf auf NAS |
| HTTP | `net/http` (Go-Routing-Patterns) + kleine Middleware | keine schwere Framework-Abhängigkeit |
| API | REST/JSON unter `/api/v1`, **OpenAPI-3 spec-first** (`oapi-codegen`), WebSocket für Events | daraus wird auch der TypeScript-Client generiert |
| Datenbank | **SQLite** (WAL) über `modernc.org/sqlite` (ohne cgo), `sqlc` für typsichere Queries, `goose` für Migrationen, **FTS5** für die Suche | keine externe Datenbank auf dem NAS nötig, cgo-freies Cross-Compiling |
| Medienanalyse | `ffprobe -print_format json` | ein Werkzeug für alle Formate |
| Streaming | FFmpeg-Prozesse, HLS mit fMP4-Segmenten, Seek per Neustart am Keyframe, Drosselung, Transcode-Cache | Standard für Browser, Chromecast und Apple |
| HW-Beschleunigung | Auto-Erkennung mit Testlauf: VAAPI/QSV (Intel-NAS, `/dev/dri`), NVENC, AMF/D3D11 (Windows), später V4L2/RKMPP (ARM) | viele Synology-/QNAP-Geräte haben Intel-iGPUs |
| Bilder | `vipsthumbnail` (libvips mit libheif) für Thumbnails, LibRaw bzw. eingebettete Vorschau für RAW, `exiftool` für EXIF, BlurHash-Platzhalter | schnell und speicherschonend |
| Trickplay | FFmpeg erzeugt Sprite-Sheets für die Vorschaubilder an der Zeitleiste | |
| Dateisystem | `fsnotify` + periodischer Scan, da SMB/NFS keine Events liefert | zuverlässig auf NAS-Freigaben |
| Metadaten | lokal zuerst (NFO im Kodi-Format, eingebettete Tags, `poster.jpg`), online: **TMDB** (Filme/Serien), **MusicBrainz + Cover Art Archive**, optional TheTVDB/fanart.tv | funktioniert auch offline |
| Auth | argon2id, Session-Tokens, Profile mit PIN, Altersfreigaben, Rechte pro Bibliothek | |
| DLNA | SSDP + UPnP ContentDirectory/ConnectionManager, Renderer-Profile (Samsung/LG/Sony), MPEG-TS-Transcode für TVs; `anacrolix/dms` (BSD) als Referenz | |
| Logging/Config | `log/slog`, YAML-Config + ENV-Overrides, OS-spezifische Pfade | |
| Windows | `golang.org/x/sys/windows/svc` als Windows-Dienst, optional ein Tray-Icon | |

### Frontend (Web-App, in die Go-Binary eingebettet über `embed`)
- **React + TypeScript + Vite**
- TanStack Router, TanStack Query und TanStack Virtual (virtualisierte Raster für über 50.000 Einträge)
- **Tailwind CSS** + Radix-UI-Primitives für Barrierefreiheit
- Player: **hls.js** (Safari spielt HLS nativ), **JASSUB** für ASS-Untertitel, Trickplay-Vorschau
- **Chromecast:** Google Cast Web Sender SDK mit dem Default Media Receiver, ein eigener Receiver kommt später
- i18n mit i18next (Deutsch und Englisch), PWA-fähig

### Build & Verteilung
- **GoReleaser:** Binaries für linux/amd64, linux/arm64, linux/armv7 und windows/amd64, außerdem `.deb`/`.rpm` (nfpm) und Docker-Images
- **Docker (primär für NAS):** Multi-Arch-Image auf Basis von `debian:stable-slim` mit gebündeltem FFmpeg. Volumes `/config`, `/media`, `/transcode`; `PUID/PGID`; optional `--device /dev/dri`. Mitgeliefert werden fertige `docker-compose.yml` für **Synology Container Manager** und **QNAP Container Station**.
- **Windows:** Inno-Setup-Installer mit Dienst, FFmpeg und Firewall-Regel
- **Linux:** systemd-Unit
- **CI:** GitHub Actions mit Lint, Tests, Build-Matrix und Docker-Push
- **Später:** natives Paket QPKG (QDK). Das Synology-SPK ist vorgezogen (Entscheidung 2026-10-31, ADR 0026) und wird mit Synologys offiziellem Package Toolkit `pkgscripts-ng` in einem Linux-Container gebaut, nicht mit spksrc.

---

## Architektur

```
                ┌──────────── Web-App (React) ─────────────┐   DLNA-TV   Chromecast
                └───────────── REST / WS / HLS ────────────┘      │          │
┌───────────────────────────────── mivo (Go) ─────────────────────┴──────────┴─┐
│ api ─ auth ─ events(ws)        dlna(ssdp, contentdirectory)                   │
│ library(scanner, watcher, nameparser) → probe(ffprobe) → metadata(providers)  │
│ stream(decision, sessions, hls, transcoder, hwaccel)  subtitles  images       │
│ db (SQLite/sqlc)                platform (Pfade, Windows-Dienst)              │
└──────────────── FFmpeg / ffprobe / vips / exiftool (externe Prozesse) ────────┘
```

### Projektstruktur
```
mivo/
  cmd/mivo/main.go          # nur Verdrahtung (Composition Root), keine Logik
  internal/
    domain/                 # reine Fachlogik, keine I/O-Imports
      media/                #   Library, Item, MediaFile, Stream (Entities, Value Objects)
      playback/             #   ClientProfile, DecisionEngine, PlaybackPlan
      user/                 #   User, Profile, Permission, ParentalRating
    app/                    # Use Cases; definiert die Ports (Interfaces), die es braucht
      library/              #   ScanLibrary, RefreshMetadata, …
      playback/             #   StartPlayback, ReportProgress, StopSession
      auth/                 #   Login, QuickConnect, Invite, …
    adapters/               # Implementierungen der Ports (I/O)
      http/                 #   REST/WS-Handler (aus OpenAPI generiert) → rufen Use Cases auf
      sqlite/               #   Repositories, Migrationen, sqlc-Queries
      ffmpeg/               #   Prober, Transcoder, HW-Erkennung
      fs/                   #   Scanner, Watcher, Namensparser
      metadata/             #   tmdb/, musicbrainz/, nfo/
      dlna/  netaccess/     #   SSDP/UPnP · ACME, UPnP-IGD, tsnet
    platform/               # Config, Logging, OS-Pfade, Windows-Dienst
  api/openapi.yaml
  web/                      # pnpm-Monorepo
    apps/web/               # Browser-App (Vite, in mivo eingebettet)
    apps/webos/             # LG-TV-App (.ipk)
    packages/api-client/    # aus openapi.yaml generiert, von beiden Apps genutzt
    packages/ui-tokens/     # Farben, Typo, Abstände (Web- und 10-Fuß-Skala)
    packages/player-core/   # Capability-Erkennung, Track-Wahl, Fortschritt melden
  packaging/{docker,synology,qnap,windows,linux}/
  scripts/gen-testmedia.*   # erzeugt Testdateien in allen Containern
  docs/
  .goreleaser.yaml
```

### Clean Code & Architekturprinzipien

**Architektur: Clean/Hexagonal Architecture, idiomatisch für Go umgesetzt**
- **Abhängigkeitsregel:** `domain` ← `app` ← `adapters` ← `cmd`. Innere Schichten kennen keine äußeren: Die Domäne importiert weder SQL noch HTTP noch FFmpeg. Das wird automatisch geprüft, mit `depguard` in golangci-lint bzw. `go-arch-lint` in der CI.
- **Ports & Adapter:** Interfaces werden dort definiert, wo sie gebraucht werden, also in `app` („accept interfaces, return structs“). Sie bleiben klein, meist mit ein bis drei Methoden. Beispiel: `app/playback` braucht einen `Transcoder`; `adapters/ffmpeg` liefert ihn.
- **Dependency Injection** über Konstruktoren, verdrahtet ausschließlich in `cmd/mivo/main.go`. Kein globaler Zustand, keine `init()`-Magie, keine Singletons.
- **Fachlogik ist testbar ohne I/O:** Entscheidungs-Engine, Namensparser und Rechteprüfung sind reine Funktionen bzw. Strukturen in `domain`.
- **Frontend:** Ordner nach Features (`src/features/{home,library,detail,player,music,photos,settings}`), dazu `src/shared/ui` (Design-System-Komponenten) und `src/shared/lib`. Komponenten enthalten keine Fachlogik; die liegt in Hooks bzw. in `packages/player-core` (reines TypeScript, ohne React). Schichtregeln werden mit `dependency-cruiser` geprüft, keine Import-Zyklen.

**Code-Regeln (SOLID, Clean Code)**
- Eine Verantwortung pro Paket, Typ und Funktion. Funktionen ≤ 50 Zeilen, kognitive Komplexität ≤ 15, Parameter ≤ 4 (sonst ein Options-Struct).
- Sprechende Namen in Englisch nach Go- bzw. TS-Konventionen, keine Abkürzungen außer etablierten (`ctx`, `id`, `URL`). Keine Magic Numbers, sondern benannte Konstanten.
- **Fehler:** immer behandeln und mit Kontext versehen (`fmt.Errorf("probe %s: %w", path, err)`). Fehlertypen der Domäne (`ErrNotFound`, `ErrForbidden`) werden erst im HTTP-Adapter auf Statuscodes abgebildet. Kein `panic` außerhalb von `main`.
- `context.Context` wird überall durchgereicht, sodass Abbruch, Timeouts und Stream-Stopps sauber funktionieren.
- Kommentare erklären das **Warum**, nicht das Was. Öffentliche Typen und Funktionen bekommen Doc-Kommentare.
- TypeScript im `strict`-Modus, kein `any`. Der API-Client wird generiert und nie von Hand geschrieben.
- Boy-Scout-Regel: Code, den man anfasst, wird sauberer hinterlassen.

**Tests (Testpyramide)**
- Domäne: tabellengetriebene Unit-Tests ohne Mocks, **Abdeckung ≥ 90 %**
- Use Cases: mit Fakes (In-Memory-Implementierungen der Ports) statt Mock-Frameworks
- Adapter: Integrationstests gegen echtes SQLite (temporär) und echtes FFmpeg mit dem Test-Korpus
- E2E mit Playwright für die Kernabläufe. Gesamt-Abdeckung ≥ 70 %, Pflicht in der CI.
- Neue Fehler werden zuerst mit einem fehlschlagenden Test nachgestellt, dann behoben.

**Werkzeuge & Qualitäts-Gates**
- Go: `gofumpt`, `golangci-lint` (strenge Konfiguration: revive, errcheck, gosec, gocognit, funlen, depguard, …), `go vet`, `govulncheck`
- Web: ESLint (typescript-eslint strict, react-hooks, jsx-a11y, import/no-cycle), Prettier, Vitest, Playwright, `dependency-cruiser`
- `lefthook` als Pre-Commit-Hook mit Format, Lint und schnellen Tests. Die vollständigen Prüfungen laufen in der CI; ein Commit, der sie nicht besteht, gilt als nicht fertig.
- **Conventional Commits**, Changelog über `git-cliff`
- **Architecture Decision Records** in `docs/adr/`. Die oben festgelegten Entscheidungen werden als erste ADRs festgehalten.
- **Definition of Done:** Tests grün, Lint sauber, Abdeckung erreicht, Doku bzw. ADR aktualisiert, keine TODOs ohne Issue-Verweis

### Datenmodell (Kern)
`libraries` (Typ: movie/show/music/photo/audiobook/mixed) → `items` (hierarchisch: Serie → Staffel → Episode, Künstler → Album → Titel, Ordner → Foto) → `media_files` (Pfad, Größe, mtime, Container) → `streams` (Video/Audio/Untertitel mit Codec, Profil, Sprache, HDR) · `metadata`, `artwork`, `people` · `users`, `profiles`, `watch_state` (Position, gesehen) · `sessions` (aktive Streams).

---

## Design-Vorschlag (an Plex angelehnt, eigene Identität)

**Farben (Dark-first, Light-Theme optional)**
- Hintergrund `#0E1014`, Fläche `#181B21`, erhöhte Fläche `#22262E`, Trennlinie `#2C313A`
- Text `#E8EAED`, gedämpfter Text `#9AA0A6`
- **Akzent „Koralle“ `#FF6B57`** (Hover `#FF8572`, Text darauf `#3A0E07`). Warm wie Plex, aber klar unterscheidbar von Plex-Orange (#E5A00D).
- Statusfarben: Direct Play grün, Direct Stream blau, Transcode amber
- Schrift **Inter**, Rundungen 6–10 px, dezente Schatten, Hover hebt die Karte an und zeigt einen Play-Button

**UX-Spezifikation (seit 2026-11-04).** Aufbau und Bedienmuster folgen dem Plex-Rundgang vom 2026-11-04 (`docs/research/plex-walkthrough-2026-11-04.md`). Verbindlich im Detail ist `docs/design/ux-spec.md`: dort hat jedes Element eine ID (z. B. `LB-20`, `PL-10`) und einen Ziel-Meilenstein; Aufgaben verweisen auf diese IDs. Grundsätze: Struktur übernehmen, nicht Identität (eigener Wortlaut, Abschnitt 14 der Spezifikation); keine Cloud-/Social-Funktionen; jeder Ansichtszustand ist eine URL; „Experteneinstellungen“-Schalter pro Einstellungsseite; per Umgebungsvariable gesetzte Werte erscheinen gesperrt mit Hinweis; Bibliotheksordner bleiben schreibgeschützt (auch heruntergeladene Untertitel und Bilder liegen im Datenverzeichnis).

**Layout**
- **Linke Sidebar** (einklappbar): **nur** Start, die Bibliotheken des eigenen Servers mit Icon pro Typ und Mivos eigene Bereiche (Playlists, Einstellungen). Bibliotheken lassen sich **anheften und umsortieren**; über „Alle Bibliotheken“ erscheint die volle Liste mit Anheften-Schalter; „Layout zurücksetzen“. Gespeichert zuerst pro Gerät (M5), ab M7 pro Profil auf dem Server. Der Online-Teil der Plex-Sidebar wird nicht übernommen (Owner-Anweisung 2026-11-04). Auf dem Handy wird daraus eine Bottom-Navigation.
- **Topbar:** Sidebar-Schalter, Logo, globale Suche (Strg+K) mit **Vorschlagsliste beim Tippen** (Top-Treffer mit Poster, Jahr, Typ, Play-Button, ⋮-Menü) und Enter → **Ergebnisseite mit Reitern** (Top-Treffer, Filme, Serien, Episoden, Personen, Musik, Fotos, Hörbücher), **Aktivitätsanzeige** (Scans, Metadaten, Bildgenerierung, Transcodes, „Update verfügbar“), Einstellungen, Cast-Button, Profilmenü.

**Screens**
1. **Start:** oben ein **Hero-Bereich** mit Backdrop und Logo, Buttons „Abspielen“ und „Mehr Infos“, automatisch rotierend – **abschaltbar** pro Gerät (Plex hat keinen; Mivo behält ihn als eigenes Element). Darunter horizontal scrollende **Reihen** mit Pfeiltasten und „Alle anzeigen“: „Weiterschauen“ (Fortschrittsbalken, bei Episoden `S3 · E9`), „Als Nächstes“, „Kürzlich hinzugefügt“ je Bibliothek (Episoden zu Serienkarten mit Zähler gruppiert; optional nach Medientyp zusammengefasst), „Zuletzt gehört“. Die Reihen erzeugt der Server; welche Bibliotheken und Reihen auf Start erscheinen, stellt der Admin pro Bibliothek ein („Reihen anpassen“). Kartenformate: Poster 2:3, Episode 16:9, Album/Foto 1:1.
2. **Bibliothek:** zwei Reiter **„Übersicht“** (vom Server erzeugte Reihen: kürzlich hinzugefügt, Neuerscheinungen, „Mehr <Genre>“, „Mit <Person>“, noch nicht gesehen, zuletzt gesehen, am besten bewertet) und **„Durchsuchen“** (alles). Werkzeugleiste: **Filter** (Schnellfilter HDR, Dolby Vision, HDR10+, Atmos, 4K, nicht verfügbar, Duplikate, ungesehen, angefangen, nicht zugeordnet; Feldfilter Genre, Jahr, Jahrzehnt, Altersfreigabe, Sammlung, Regie, Darsteller, Land, Studio, Auflösung, Codecs, Sprachen – mit Anzahl pro Wert, als entfernbare Chips), **Typ** (Filme bzw. Serien/Staffeln/Episoden oder **Ordner** = Bibliothek nach Ordnerstruktur durchsuchen), **Sortierung** (Titel, Jahr, Veröffentlichung, Bewertung, Altersfreigabe, Dauer, hinzugefügt, Auflösung, Bitrate, Größe, Zufall; Fortschritt, Wiedergaben, zuletzt gesehen), **Anzahl**, „Alle abspielen“, „Zufällig“, „Hinzufügen zu …“, Mehrfachauswahl. Ansichten **Raster, Liste, Details**; **Größenregler** für Poster; **A–Z-Sprungleiste** rechts; virtualisiertes Raster. Karten mit Hover-Aktionen (Play, ⋮, Bearbeiten, gesehen) und Badges (gesehen, ungesehene Folgen, Fortschritt, Versionen).
3. **Detailseite:** unscharfer Backdrop, Poster links (Altersfreigabe-Badge), Titel/Originaltitel, Altersfreigabe/Jahr/Laufzeit/Genres (verlinkt auf gefilterte Bibliothek), Regie, **Bewertungszeile** (Anbieter-Bewertungen, eigene Sterne), Buttons **Abspielen/Fortsetzen**, „Als gesehen markieren“, „Bearbeiten“, ⋮-Menü; Inhaltsangabe mit „Mehr“; **Technikblock** mit Auswahl von Version, Audio und Untertiteln (inkl. „Online suchen…“ und „Hochladen…“) plus **Technik-Chips** (`MKV · HEVC 10-bit HDR10 · TrueHD 7.1 · PGS`); Besetzung als Porträt-Reihe (→ Personenseite), Extras, Ähnliches. **Serie → Staffel → Episode:** Serienseite mit „Als Nächstes“ und Staffel-Reihe; Staffelseite mit Episoden als 16:9-Raster oder Liste (aufsteigend, umschaltbar), Pfeile zur vorigen/nächsten Staffel; Episodenseite mit Pfeilen zur vorigen/nächsten Episode. **Personenseite** mit Porträt, Biografie und Filmografie aus den eigenen Bibliotheken.
4. **Video-Player:** Leiste unten – links Titel, Jahr, „verstrichen / gesamt“; Mitte vorherige, −10 s, Play/Pause, +30 s, nächste, Stopp; rechts **Modus-Anzeige** (farbiges Badge, Mivo-Alleinstellung), ⋮ (Playlist, neu analysieren, Download, Verlauf, **Medieninfo**), Wiederholen, Zufall, **Wiedergabeoptionen**, **Kapitel**, **Warteschlange**, Lautstärke. Oben Minimieren (→ **Mini-Player** beim Weiterblättern) und Vollbild. Zeitleiste über die volle Breite mit Zeit-Tooltip, Trickplay-Vorschau und Kapitelmarken; „Intro überspringen“. **Wiedergabeoptionen-Panel**: Qualität (Original / Automatisch / Bitratenstufen), Audiospur (Neustart an derselben Position, gemerkt pro Titel), Untertitel (Spuren, „Online suchen…“, „Hochladen…“), Untertitelgröße, automatisch weiter, Geschwindigkeit. **Stream-Info-Overlay** mit Modus, Grund, Bitrate, HW-Encoder, Tonemapping, Puffer und Transcode-Geschwindigkeit – Plex zeigt das nur im Admin-Dashboard.
5. **Musik:** Künstler- und Albumseiten; **persistente Player-Leiste unten** mit Warteschlange, die beim Navigieren bestehen bleibt; Gapless-Wiedergabe.
6. **Fotos:** Zeitleisten-Raster nach Datum, Lightbox mit EXIF-Panel, Diashow.
7. **Hörbücher:** Kapitelliste, Fortsetzen pro Nutzer, Wiedergabegeschwindigkeit.
8. **Admin-Bereich** (Struktur siehe unten): Dashboard mit aktiven Streams (Nutzer, Gerät, Modus, Bandbreite, CPU), Scan-Status, Logs, Transcoder-Einstellungen mit „HW-Beschleunigung testen“.
9. **Einrichtungsassistent** beim ersten Start: Admin-Konto anlegen, Bibliotheken hinzufügen, Metadaten-Sprache wählen.
10. **Profilauswahl:** Kacheln mit Avatar, Schloss bei PIN, Admin-Markierung, „Gast“ (wenn aktiviert), „Profil hinzufügen“; PIN-Feld, das auch mit der TV-Fernbedienung bedienbar ist.

**Bearbeiten-Dialog pro Titel („Details bearbeiten“, nur Admin)**
- Reiter **Allgemein** (Titel, Sortiertitel, Originaltitel, Edition, Veröffentlichung, Altersfreigabe mit Land, Studio, Tagline, Inhalt; bei Episoden Staffel/Episode, Ausstrahlung), **Tags** (Genres, Regie, Drehbuch, Produktion, Länder, Studios, Sammlungen als Chip-Eingaben), **Labels** (für Altersgrenzen und Filter), **Bilder** (Poster, Hintergrund, Logo, quadratisch – Kandidaten aus lokalen Dateien und Anbietern, Datei-Upload, URL mit SSRF-Schutz), **Dateien** (Bibliothek und relative Pfade, „neu analysieren“), **Erweitert** (Metadaten-Sprache pro Titel, Originaltitel verwenden, Anbieter-ID).
- **Jedes Feld hat ein Schloss**: bearbeitete Felder werden automatisch gesperrt und bei Metadaten-Aktualisierung oder NFO-Änderung nie überschrieben.
- Mehrfachbearbeitung über die Auswahl in der Bibliothek. Gespeicherte Bilder liegen im Datenverzeichnis, nie in der Bibliothek.

**Admin-Bereich (Struktur)** – eigene Sidebar unter `/admin`; jede Einstellungsseite hat den Schalter „Experteneinstellungen“ und genau einen „Speichern“-Button. Schreibende Einstellungen gibt es erst mit Login (M7); davor bleibt `config.yaml` maßgeblich (ADR 0022) und der Bereich ist nur lesend.
- **Status:** Dashboard (Jetzt läuft mit Modus und Grund pro Sitzung, Scans, Transcoder-Last, Version, Update-Hinweis, Warnungen wie wenig Platz im Transcode-Verzeichnis), Aktivitätsprotokoll (live), Aufgabenläufe.
- **Einstellungen:** Allgemein (Servername, Update-Prüfung und Kanal, Log-Level), Fernzugriff (Assistent, M7b), Metadaten (Anbieter pro Bibliothekstyp als **geordnete Liste**: NFO, eingebettete Tags, lokale Bilder, TMDB, MusicBrainz …; eigener TMDB-Key, OpenSubtitles-Zugang), Bibliothek (Watcher, periodischer Scan, fehlende Einträge erst nach N Scans entfernen, „gesehen“-Schwelle 90 %, Weiterschauen-Fenster, Trickplay/Kapitelbilder/Intro-Erkennung: nie / im Wartungsfenster / beim Hinzufügen), Netzwerk (Port, Base-URL, Trusted Proxies, **LAN-Netze** als CIDR-Liste, LAN-only-Guard), Transcoder (**max. gleichzeitige Transcodes**, Transcode-Verzeichnis, HW-Beschleunigung mit Test, Preset, Drosselung, Tonemapping, max. Ausgabeauflösung), Sprachen (Standard-Audio-/Untertitelsprache, Untertitelmodus), DLNA (M8), geplante Aufgaben (**Wartungsfenster**, Aufgabenliste mit „Jetzt ausführen“), Backup (Intervall, Ziel, Wiederherstellen), Benachrichtigungen (Webhooks).
- **Verwalten:** Bibliotheken (Liste, Scannen, **Hinzufügen/Bearbeiten** mit Typ-Kacheln, mehreren Ordnern über einen auf erlaubte Wurzeln beschränkten Ordner-Browser und **erweiterten Optionen**: Sichtbarkeit auf Start/Suche, lokale Metadaten bevorzugen, lokale Bilder, Originaltitel, Bildsprache, Extras, Trickplay/Kapitelbilder, Intro-Erkennung, Sammlungen-Anzeige, Mindestgröße automatischer Sammlungen, Episodenreihenfolge; „Reihen anpassen“), Fehlerbehebung (DB optimieren, Caches leeren, Logs und Support-Paket ohne Geheimnisse herunterladen), Live-Log.
- **Nutzer:** Konten und verwaltete Profile, Einladungslinks, Bibliothekszugriff pro Nutzer (alle, ausgewählte Bibliotheken oder **einzelne Titel**), Rechte (Download, Untertitel-Upload, Fernzugriff, Transcoding), **Altersgrenzen** als Voreinstellungen (FSK/US-Freigaben, „eigene“ mit Labels), Gast (aus, nur LAN), Geräte mit Widerruf (M7b), Anmeldeprotokoll (M7b).
- **Einstellungen für dieses Gerät** (jeder Nutzer): Sprache, Zeitformat, Hero an/aus, Sidebar zurücksetzen, Qualität im Heimnetz/unterwegs, Untertitelstil, Diagnose.

**Nicht übernommen** (Begründung in `docs/design/ux-spec.md`, Abschnitt 13): Online-Teil der Sidebar und Online-Medienquellen, Anmeldung über ein Anbieterkonto, Freunde, Teilen per E-Mail, Watchlist-Sync, „Mehr Möglichkeiten zum Ansehen“ (Streamingdienste), Relay, Community-Bewertungen, Bezahlfunktionen/Upsell (Mivo hat keine), Crash-Reports, Löschen von Mediendateien und Untertitel neben die Medien schreiben, **IP-Liste ohne Anmeldung** (Sicherheitsabschnitt; die LAN-Liste dient nur der Einordnung LAN/Remote), getrennte Scanner/Agenten, Plugins. **Später:** Vorkonvertierung („optimierte Versionen“), Kino-Trailer/Pre-Roll, gemeinsames Schauen.

---

## Fernzugriff aus dem Internet

Ziel ist ein Assistent **„Fernzugriff einrichten“** mit wenigen Klicks. Fernzugriff ist anfangs **aus**. Der Assistent prüft zuerst automatisch: Gibt es eine öffentliche IPv4 oder CGNAT bzw. DS-Lite (bei deutschen Kabel- und Glasfaseranschlüssen häufig)? Gibt es IPv6? Spricht der Router UPnP-IGD, NAT-PMP oder PCP? Je nach Ergebnis schlägt er einen von zwei Wegen vor.

### Weg A – Direkt über HTTPS (Standard, nötig für LG-TV und Browser ohne Zusatz-App)
- `mivo` hat einen **eigenen HTTPS-Listener** mit automatischem Let's-Encrypt-Zertifikat über `certmagic`. Die Erneuerung läuft automatisch.
- **Domain ohne eigene Kosten:** kostenloses DynDNS über DuckDNS. Das Zertifikat kommt per DNS-01-Challenge über die DuckDNS-API, deshalb müssen Port 80/443 nicht offen sein. Alternativ lässt sich eine vorhandene NAS-Domain nutzen (`*.synology.me`, myQNAPcloud) oder eine eigene Domain.
- **Portfreigabe automatisch** per UPnP/NAT-PMP/PCP für genau einen Port. Wenn das nicht klappt, zeigt der Assistent eine Schritt-für-Schritt-Anleitung, z. B. für die FRITZ!Box.
- Danach folgt ein **Erreichbarkeitstest von außen**, damit sichtbar ist, ob es wirklich funktioniert.
- Für NAS-Nutzer gibt es als dokumentierte Alternative den eingebauten Reverse Proxy von DSM bzw. QNAP mit dessen Zertifikat.
- **Grenze:** Bei CGNAT/DS-Lite ist IPv4 von außen nicht erreichbar. Dann funktioniert nur IPv6, wenn das Netz, in dem der Client gerade ist, IPv6 hat, oder Weg B.

### Weg B – Privat über Tailscale (bei CGNAT oder für maximale Sicherheit)
- **Tailscale ist direkt in `mivo` eingebaut** (Go-Bibliothek `tailscale.com/tsnet`). Auf dem NAS muss nichts extra installiert werden. Man aktiviert es in den Einstellungen und klickt einmal den Login-Link an. Der Server bekommt eine HTTPS-Adresse `mivo.<tailnet>.ts.net` (`ListenTLS`).
- **Kein offener Port**, funktioniert hinter CGNAT, Ende-zu-Ende verschlüsselt (WireGuard).
- **Grenze:** Jeder Client braucht die Tailscale-App. Das geht auf PC, Handy und Tablet, aber **nicht auf dem LG-TV**. Ein TV außerhalb des eigenen Heimnetzes braucht deshalb Weg A.

### Bewusst nicht empfohlen
- **Cloudflare Tunnel:** Die Nutzungsbedingungen schränken Video-Auslieferung über das kostenlose CDN ein, Accounts können gesperrt werden. Das ist laut Drittquellen eine Grauzone und damit für einen Medienserver ungeeignet.
- **Einfache Portfreigabe mit HTTP ohne TLS:** Passwörter und Tokens würden im Klartext übertragen.
- **Eigener Relay-Dienst wie bei Plex:** Der Server-Betrieb und der Traffic würden laufende Kosten verursachen. Das ist frühestens eine spätere Option.

### Sicherheit (gilt für beide Wege)
- Von außen nur HTTPS, mit HSTS, sicheren Headern und CSP. HTTP-Anfragen von außen werden abgelehnt.
- Der Einrichtungsassistent erzwingt ein starkes Admin-Passwort; es gibt keine Standard-Zugangsdaten.
- **2FA (TOTP)** für Admins. Ohne aktiviertes 2FA ist der Fernzugriff für Admins gesperrt.
- **Rate-Limiting und Sperre** nach Fehlversuchen, Login-Protokoll, Benachrichtigung bei neuem Gerät.
- Recht „Fernzugriff erlaubt“ **pro Nutzer**. Admin-Einstellungen sind optional nur aus dem LAN erreichbar.
- Kurzlebige Zugriffstokens mit Refresh, **Geräteverwaltung** zum Widerrufen von Geräten, **signierte, zeitlich begrenzte Stream-URLs**. Das ist auch für Chromecast, DLNA und den TV nötig.
- Bandbreitenlimit und automatische Qualitätsanpassung für Remote-Streams.
- Dateizugriff nur innerhalb freigegebener Bibliotheksordner (Schutz vor Path Traversal). Der Docker-Container läuft als Nicht-Root.
- Hinweis auf verfügbare Updates im Dashboard.

Quellen: [tsnet (Tailscale)](https://tailscale.com/docs/features/tsnet), [tsnet Go-Paket](https://pkg.go.dev/tailscale.com/tsnet), [Cloudflare-Bedingungen und Plex (selfhost.it.com)](https://selfhost.it.com/blog/expose-plex-with-cloudflare), [Cloudflare Tunnel und Jellyfin (Lumadock)](https://lumadock.com/tutorials/cloudflare-tunnel-jellyfin)

---

## LG-webOS-TV-App

### Technik
- **React + TypeScript + Vite**, im selben Monorepo wie die Web-App. Gemeinsam genutzt werden `api-client`, `ui-tokens` und `player-core`. Die UI ist eine eigene 10-Fuß-Oberfläche (1920×1080, große Schrift, Fokus statt Hover).
- **Fernbedienungs-Navigation** mit `@noriginmedia/norigin-spatial-navigation`: 4-Wege-Tasten, OK und Zurück. Zusätzlich muss der Zeiger der Magic Remote funktionieren, also auch Maus-Events. Alternative ist LGs eigenes React-Framework **Enact/Sandstone**; es bringt Spotlight-Navigation mit, gibt aber LGs Look vor. Empfehlung: norigin, damit das Mivo-Design einheitlich bleibt.
- **Ziel-Versionen: webOS 6.0 (2021) und neuer.** Ältere Geräte haben sehr alte Chromium-Engines. Deshalb wird der Build per `@vitejs/plugin-legacy`/Browserslist auf die Engine der Mindestversion transpiliert.
- **Wiedergabe über das native `<video>`-Element.** Der TV-Hardware-Decoder kann laut LG-Spezifikation (webOS 6.0) MKV, MP4/MOV, HEVC Main10 bis 4K60, VP9 und AV1 sowie Dolby Digital/DD+ und AAC. Dadurch gibt es auf dem TV deutlich häufiger **Direct Play** als im Browser. Pro webOS-Version gibt es ein eigenes Client-Profil in der Entscheidungs-Engine. Wichtig: DTS fehlt auf einigen Modelljahren, das Profil darf es also nicht pauschal annehmen. HLS (inkl. AES-128) wird unterstützt und dient als Fallback beim Transcoden.
- **Untertitel:** Text-Untertitel rendert die App selbst; Bild-Untertitel werden wie im Web eingebrannt.
- **Server-Verbindung (rein lokal):** Ein gebündelter JS-Service sucht den Server per SSDP bzw. UDP-Discovery im Heimnetz. Wird nichts gefunden, gibt man die Adresse manuell ein. Danach zeigt der TV einen QR-Code und einen Quick-Connect-Code. Das Handy öffnet den Link zum eigenen Server, man bestätigt dort, und der TV erhält sein Token. Ein Online-Kopplungsdienst kommt später als Option. Der Server braucht CORS-Header für die App-Herkunft.
- **Werkzeuge:** webOS TV CLI (`ares-setup-device`, `ares-package`, `ares-install`, `ares-inspect`), Simulator bzw. Emulator, Deployment auf ein echtes Gerät im Developer Mode

### Abgleich deiner Recherche
| Punkt | Bewertung |
|---|---|
| HTML5/CSS/JS, Frameworks erlaubt, `.ipk` über das SDK | ✅ stimmt. Zusätzlich auf die alte Chromium-Version pro webOS-Release achten (Transpiling). |
| Developer-Account, Developer-Mode-App, Key Server, CLI/VS Code | ✅ stimmt. Ergänzung: Die Dev-Mode-Sitzung läuft ab. Früher waren es 50 h, laut LG-Forum heute bis zu 1000 h. Sie lässt sich in der App oder per `ares-launch com.palmdts.devmode -p extend=true` verlängern. |
| Bedienung komplett über Pfeiltasten, OK und Zurück | ✅ Pflicht laut Self-Checklist. Zusätzlich verlangt: Bedienung per Zeiger der Magic Remote und **sichtbarer Fokus auf jedem Objekt**. |
| Zurück auf der Startseite führt zum webOS-Home | ✅ laut Checklist so gefordert, ausdrücklich für webOS 23–25. Auf älteren Versionen gilt ein anderes Exit-Verhalten. Das wird pro Version umgesetzt. |
| 1920×1080, Schrift ≥ 20 px | 1080p ✅. Die 20-px-Regel konnte ich in den offiziellen Quellen nicht bestätigen. Wir planen sie trotzdem als Mindestgröße ein. |
| Seller Lounge | ✅ Es gibt **Einzel-** und **Firmen-Seller**. Ob die Registrierung etwas kostet, konnte ich nicht bestätigen; das bitte selbst in der Seller Lounge prüfen. |
| Assets | ⚠️ Teilweise ergänzt: In `appinfo.json` ist ein `icon` mit **80×80 PNG** Pflicht, `largeIcon` 130×130 optional. In der Seller Lounge kommt ein **400×400-Icon** hinzu. `splashBackground` hat 1920×1080 PNG. Die Screenshot-Maße habe ich nicht verifiziert. |
| Self-Checklist + UX-Szenario | ✅ Pflicht. Achtung: Es muss die **aktuelle Vorlage** aus der Seller Lounge verwendet werden, sonst wird die App abgelehnt. Ein falsches „N/A“ führt ebenfalls zur Ablehnung. |
| QA durch LG | ✅ Geprüft wird in drei Stufen: Pretest, Funktionstest und Content-Test. |
| **Fehlt in der Recherche** | Die Tester brauchen vermutlich einen **erreichbaren Testserver mit Testkonto und Demo-Medien**, weil die App ohne Server nichts zeigt. Das ist meine Annahme und muss vor der Einreichung geprüft werden. Außerdem braucht es eine **Datenschutzerklärung**, denn die Checklist prüft Privacy und den Umgang mit Zugangsdaten. Und nur rechtefreie Demo-Inhalte verwenden, da es einen Content-Test gibt. |

Quellen: [App Approval Process](https://webostv.developer.lge.com/distribute/app-approval-process), [App Self Checklist](https://webostv.developer.lge.com/distribute/app-self-checklist), [appinfo.json](https://webostv.developer.lge.com/develop/references/appinfo-json), [Audio/Video webOS 6.0](https://webostv.developer.lge.com/develop/specifications/video-audio-60), [Streaming & DRM](https://webostv.developer.lge.com/develop/specifications/streaming-protocol-drm), [App Ecosystem](https://webostv.developer.lge.com/distribute/app-ecosystem), [Forum: Dev-Mode verlängern](https://forum.webostv.developer.lge.com/t/whats-the-way-to-extend-dev-time-with-webos-cli/28033)

---

## Funktionsumfang – Abgleich mit Jellyfin

Referenz ist Jellyfin 12.0 (Sept. 2026, .NET 10, GPL-2.0). **Wir übernehmen keinen Jellyfin-Code**, sondern entwickeln in Go eigenständig neu. Genutzt wird nur das separat ausgelieferte `jellyfin-ffmpeg` als externer Prozess; dessen Lizenzhinweise und Quellverweis müssen mitgeliefert werden. Legende: **v1** = im Plan, **+** = neu in den Plan aufgenommen, **später** = nach v1, **–** = bewusst nicht.

| Bereich | Jellyfin | Mivo |
|---|---|---|
| Bibliothekstypen | Filme, Serien, Musik, Bücher, Fotos; „Gemischt“ wird nicht empfohlen | v1: Filme, Serien, Musik, Fotos, Hörbücher, Disc-Images · **+ E-Books/Comics** (EPUB/PDF/CBZ/CBR, OPF/ComicInfo) **später** |
| Scan | Mehrere Pfade, Watcher, geplante Scans; ersetzte Dateien bereinigen | v1 · **+** Bereinigung von Trickplay, Untertiteln und Kapiteln bei ersetzten Dateien · **+** Warnung bei wenig Speicherplatz |
| Mehrere Versionen und Extras | Alternative Versionen (auch Episoden), Trailer und Extras | **+ v1**: Versionsauswahl, Extras-Ordner (Trailer, Featurettes) |
| Metadaten | TMDb, OMDb, NFO; per Plugin TVDB, fanart.tv, AniDB | v1: TMDB, MusicBrainz, NFO · **+** OMDb, TVDB-IDs, Anime (AniDB/AniList) **später** |
| Metadaten-Editor | Bearbeiten, Bilder ändern, „Identifizieren“ | **+ v1** (Dialog „Zuordnung korrigieren“, Bildauswahl, Felder sperren) |
| Sammlungen, Playlists, Favoriten | ja; TMDb-Box-Sets per Plugin | **+ v1**: automatische Sammlungen aus TMDB, Playlists, Favoriten, Bewertungen |
| Musik-Extras | Songtexte, ReplayGain, Empfehlungen über ListenBrainz | **+ v1**: Songtexte (LRC/eingebettet), ReplayGain · ListenBrainz **später** |
| Direct Play / Stream / Transcode | ja, mit `jellyfin-ffmpeg` | v1 (Kernfunktion) |
| HW-Beschleunigung | QSV, NVENC, AMF, VAAPI, VideoToolbox, RKMPP | v1: QSV, VAAPI, NVENC, AMF · RKMPP (ARM-NAS) **später** · VideoToolbox – (kein macOS-Ziel) |
| HDR → SDR-Tonemapping | HDR10/HLG, Dolby Vision P5/P8, 3D-LUT | **+ v1**: HDR10/HLG per GPU (VPP/OpenCL), Dolby Vision P5/P8 **später** |
| Trickplay | ja | v1 |
| Intro/Abspann überspringen | Media Segments (per Provider/Plugin) | **+ v1**: Segment-Modell + Intro-Erkennung per Audio-Fingerprint bei Serien · „Überspringen“-Button im Player |
| Kapitelbilder | ja | **+ v1** |
| Untertitel | Extraktion, VobSub, OpenSubtitles per Plugin, Einstellungen pro Bibliothek | v1 · **+** OpenSubtitles-Suche, Untertitel-Einstellungen pro Bibliothek |
| Nutzer und Rechte | Nutzer, Kindersicherung, Fernzugriff pro Nutzer, LDAP per Plugin | v1 · **+** LDAP/OIDC-SSO **später** |
| Quick Connect | Login per Code am TV | **+ v1**, **wichtig für die LG-App**: Code am TV, Bestätigung am Handy |
| API-Keys | ja | **+ v1** (für Skripte und Integrationen wie Sonarr/Radarr) |
| Live-TV und DVR | HDHomeRun, M3U/IPTV, XMLTV/Schedules Direct, Aufnahmen | **+ später** (eigener Meilenstein) |
| SyncPlay (gemeinsam schauen) | ja | **+ später** |
| Plugins | Katalog, Repositories, .NET-Plugins | **später**: Erweiterungen über Webhooks und Metadaten-Provider-Schnittstelle; kein In-Process-Plugin-System in v1 |
| Benachrichtigungen | Webhook-Plugin, Gotify, Slack | **+ v1**: Webhooks (generisch, Discord, ntfy/Gotify) |
| Geplante Aufgaben | Aufgaben-Seite (Scan, DB optimieren, Trickplay …) | **+ v1**: Aufgabenplaner mit UI |
| Backup und Restore | Live-Backup der DB seit 10.11 | **+ v1**: Backup von DB und Config über die UI, zeitgesteuert, Restore auch auf anderer Hardware |
| Admin | Dashboard, Log-Viewer (live), Aktivitäten, Branding | v1 · **+** Live-Logs, Aktivitätsprotokoll, Branding/Custom CSS **später** |
| DLNA | aus dem Core in ein Plugin ausgelagert | v1 **im Core** (Vorteil von Mivo) |
| Netzwerk | HTTP 8096, HTTPS 8920, Discovery UDP 7359, Base-URL, Known Proxies; eingebautes TLS soll entfallen | v1 + **eingebautes HTTPS mit Let's Encrypt und Fernzugriffs-Assistent** (Vorteil) · **+** Base-URL, Trusted Proxies, LAN-CIDR-Liste, Client-Discovery |
| Plattformen | ARM32 seit 10.11 entfernt, nur noch ARM64 | Mivo unterstützt **auch armv7** (ältere NAS), solange FFmpeg-Builds verfügbar sind (Vorteil) |
| API | REST + OpenAPI/Swagger | v1 (spec-first, Swagger-UI unter `/api/docs`) |
| Remote-Transcoding (rffmpeg) | ja | – (zu speziell) |

**Was Mivo besser machen soll als Jellyfin:** schlankes Go-Binary für schwache NAS inklusive ARM32, DLNA und HTTPS im Core, ein Fernzugriffs-Assistent, besserer Foto-Bereich, Disc-Images und Stream-Info-Transparenz direkt im Player.

Quellen: [Jellyfin GitHub](https://github.com/jellyfin/jellyfin), [Release 12.0](https://jellyfin.org/posts/jellyfin-release-12.0), [Release 10.11.0](https://jellyfin.org/posts/jellyfin-release-10.11.0), [HW-Beschleunigung](https://jellyfin.org/docs/general/post-install/transcoding/hardware-acceleration/), [Plugins](https://jellyfin.org/docs/general/server/plugins/), [Live TV](https://jellyfin.org/docs/general/server/live-tv/), [Netzwerk](https://jellyfin.org/docs/general/post-install/networking/), [Bibliotheken](https://jellyfin.org/docs/general/server/libraries), [Metadaten](https://jellyfin.org/docs/general/server/metadata/)

---

## Projektordner & Agenten-Team

### Ordner umbenennen
- `C:\Users\stefa\Documents\Any Media Server` → **`C:\Users\stefa\Documents\Mivo`**
- Der alte Ordner ist leer, und diese Sitzung nutzt ihn als Arbeitsverzeichnis. Unter Windows lässt sich ein geöffneter Ordner nicht umbenennen. Deshalb lege ich `Mivo` neu an, wechsle das Arbeitsverzeichnis der Sitzung dorthin und entferne danach den leeren alten Ordner, nur wenn er wirklich leer ist.
- Danach `git init` mit `.gitignore`, `.gitattributes` (LF), `LICENSE` (GPL-3.0) und einer `README.md` als Grundgerüst.

### Agenten (`.claude/agents/*.md`, im Repo versioniert)
Jeder Agent hat eine klar abgegrenzte Aufgabe, nur die Werkzeuge, die er dafür braucht, und beendet seine Arbeit mit einem einheitlichen **REPORT-Block** (Status, geänderte Dateien, Testergebnis, offene Punkte). Grundlage für alle ist die Projekt-`CLAUDE.md` mit Architekturregeln, Befehlen und Definition of Done.

| Agent | Aufgabe | Werkzeuge | Ergebnis |
|---|---|---|---|
| `mivo-architect` | zerlegt einen Meilenstein in kleine Aufgaben mit Akzeptanzkriterien; schreibt und aktualisiert ADRs und `docs/tasks/*.md` | Read, Grep, Glob, Write, Edit (nur `docs/`) | Aufgabenliste + ADRs |
| `mivo-backend` | setzt eine Go-Aufgabe in `domain`/`app`/`adapters` um, Test zuerst | Read, Grep, Glob, Edit, Write, Bash/PowerShell | Code + Unit- und Integrationstests, lokal grün |
| `mivo-media` | Spezialist für FFmpeg: Probing, Entscheidungs-Engine, HLS/Transcoding, HW-Erkennung, Test-Korpus | wie backend | Code + Tests gegen den Test-Korpus |
| `mivo-frontend` | Web-App und Design-System (React/TS), Feature-Ordner, Barrierefreiheit | wie backend | Komponenten + Vitest-Tests, Playwright-Tests für Abläufe |
| `mivo-webos` | TV-App (10-Fuß-UI, Spatial Navigation, `.ipk`) – ab Meilenstein 10 | wie backend | App + Tests, Hinweise zur Store-Checkliste |
| `mivo-tester` | **ändert keinen Produktivcode.** Führt alle Gates aus: Lint, Unit-, Integrations- und E2E-Tests, Abdeckung, Test-Korpus-Matrix, Docker-Build; prüft die Akzeptanzkriterien | Read, Grep, Glob, Bash/PowerShell, Write (nur `docs/reports/`) | Testbericht PASS/FAIL mit Fehlerdetails |
| `mivo-reviewer` | **nur lesend.** Prüft Code gegen Clean-Code-Regeln, Schichtenregel, ADRs und Sicherheit (Auth, Pfade, Fernzugriff) | Read, Grep, Glob | Tabelle der Befunde + **APPROVE** oder **REVISE** |

### Ablauf pro Aufgabe (Feedback-Schleife)
Die Hauptsitzung steuert den Ablauf, weil Subagenten selbst keine weiteren Agenten starten können:

1. `mivo-architect` → `docs/tasks/<id>.md` mit Akzeptanzkriterien
2. passender Umsetzungs-Agent (`backend`, `media`, `frontend` oder `webos`) → Code und Tests
3. `mivo-tester` → bei **FAIL** geht der Bericht zurück an den Umsetzungs-Agenten (zurück zu 2)
4. `mivo-reviewer` → bei **REVISE** gehen die Befunde zurück an den Umsetzungs-Agenten (zurück zu 2)
5. bei PASS + APPROVE → Commit (Conventional Commit), Aufgabe abgehakt
6. nach höchstens 3 Durchläufen ohne Erfolg → Stopp und Rückfrage an dich

Unabhängige Aufgaben, z. B. Backend und Frontend, laufen parallel, solange sie keine Dateien teilen. Nach jedem Meilenstein bekommst du eine Zusammenfassung und entscheidest, ob es weitergeht.

---

## Umsetzung in Meilensteinen

Alle Medienarten sind Teil von v1. Gebaut wird trotzdem als **„Walking Skeleton“**: zuerst ein dünner Durchstich von Anfang bis Ende, danach in die Breite.

1. **Fundament:** Schichtenstruktur (domain/app/adapters/platform), Go-Modul, Config, slog, SQLite + Migrationen, OpenAPI-Grundgerüst, React-App mit Feature-Struktur und Design-Tokens, Einbettung ins Binary, Dockerfile, **Qualitäts-Gates** (golangci-lint, depguard, ESLint, dependency-cruiser, lefthook, Abdeckungs-Grenzen), ADRs 001–015 für die festgelegten Entscheidungen, `scripts/gen-testmedia`
2. **Durchstich Video:** Ordner scannen → ffprobe → Bibliothek in der Web-UI → Abspielen per Direct Play, Remux oder Transcode inklusive Stream-Info-Overlay. **Danach kann bereits jeder Video-Container im Browser abgespielt werden.**
Umsetzungsreihenfolge (OD-UX-2, entschieden 2026-11-04): **3 → 6 → 5 → 4 → 7**; die Nummern bleiben als Kennung. Element-IDs in Klammern verweisen auf `docs/design/ux-spec.md`.

3. **Bibliotheken komplett:** Namensparser (Filme/Serien; Regressionstest: Serien mit absoluter Episodennummerierung dürfen nicht als „130 Staffeln“ erscheinen, DT-24), Hierarchie Serie → Staffel → Episode mit einfachen Serien-, Staffel- und Episodenseiten (DT-20…22), Musik-Tags, Foto-EXIF, Hörbuch-Kapitel, Disc-Images (ISO/BDMV/VIDEO_TS), lokale NFO-Dateien (Inhalt, Genres, Personen, Altersfreigabe, Veröffentlichung) und lokales Artwork, **Bild-Endpunkt mit serverseitiger Skalierung in festen Größen und Cache** (LB-44) und Poster in Raster und Detailseite (LB-40), Extras-Ordner, Watcher und periodischer Scan (Config-Schlüssel, AD-23)
4. **Streaming-Ausbau:** HW-Beschleunigung mit Auto-Erkennung und „HW-Beschleunigung testen“ (AD-25), Untertitel (ASS/PGS), Trickplay, DSD/Hi-Res-Audio, Bandbreitenstufen, Session-Verwaltung und Drosselung, HDR→SDR-Tonemapping, Media Segments mit Intro-Erkennung und „Überspringen“-Button (PL-07), Kapitelbilder und Kapitel-Panel (PL-30), mehrere Versionen. UI-Anteil: **Wiedergabeoptionen-Panel** mit Qualität (Original/Automatisch/Bitratenstufen), Audiospur, Untertiteln und Untertitelgröße (PL-10…13), Auswahl von Version/Audio/Untertitel auf der Detailseite (DT-07), Trickplay und Kapitelmarken an der Zeitleiste (PL-05), erzeugte Episodenbilder (DT-23), Erweiterungen des Stream-Info-Overlays (Ausgabebitrate, HW-Encoder/-Decoder, Tonemapping, Einbrenn-Grund, Transcode-Geschwindigkeit, PL-41), Qualitäts- und Untertitel-Einstellungen pro Gerät (AD-02)
5. **Web-UI komplett:** alle Screens nach `docs/design/ux-spec.md`: Shell mit anheft- und sortierbarer Sidebar, Suche mit Vorschlagsliste und Ergebnisseite mit Reitern (FTS5), Aktivitätsanzeige (SH-*, SR-*); Start mit vom Server erzeugten Reihen und abschaltbarem Hero (HO-01, HO-02, HO-05, HO-07); Bibliothek mit „Übersicht“/„Durchsuchen“, Filter mit Facetten, Typ inkl. **Ordner**, Sortierung, Anzahl, Mehrfachauswahl, Raster/Liste/Details, Größenregler, A–Z-Leiste, virtualisiertes Raster (LB-*); Detailseiten-Layout, Personen aus NFO, Extras (DT-*); Player-Leiste im neuen Layout mit **Modus-Anzeige**, Warteschlange (in `player-core`), Wiederholen/Zufall, automatisch weiter, Geschwindigkeit, Medieninfo-Dialog, **Mini-Player**, Tastenkürzel-Hilfe (PL-01…04, PL-08, PL-09, PL-14, PL-15, PL-31…36, PL-38, PL-40, PL-42); nur lesendes Admin-Dashboard „Jetzt läuft“ (AD-10, PL-43); „Einstellungen für dieses Gerät“ (AD-01, AD-03); Musik-Player-Leiste, Foto-Lightbox, Responsive-Layout mit Bottom-Navigation, i18n
6. **Online-Metadaten:** TMDB, MusicBrainz, Artwork-Cache, Dialog „Zuordnung korrigieren“ / „Zuordnung entfernen“, „Metadaten aktualisieren“, **Bearbeiten-Dialog mit Feldsperren, Tags, Bildauswahl (Kandidaten, Upload, URL mit SSRF-Schutz) und Metadaten-Sprache pro Titel** (ED-01…03, ED-05…07; vorgezogen aus 7, weil die Sperren erst mit Online-Aktualisierung nötig werden), Bewertungszeile (DT-04), Porträts und Personenseiten (DT-08, DT-30), „Ähnliches“ (DT-10), Reihen „Mit <Person>“ und „Am besten bewertet“ (LB-05, LB-08), Filter „nicht zugeordnet“, Anbieter-Einstellungen als geordnete Liste (AD-22), Episodenreihenfolge nach Anbieter (absolut/DVD), **Online-Untertitelsuche** über OpenSubtitles mit eigenem Zugang des Admins, Ablage im Datenverzeichnis (PL-20; ausgehende Verbindung nach ADR 0010 nur wenn eingerichtet)
7. **Nutzer und Verwaltung:** Login, **Profilauswahl mit PIN** (US-01…03), **verwaltete Profile mit Altersgrenzen** (FSK/US-Voreinstellungen, eigene mit Labels; serverseitig erzwungen, US-10…12), Freigaben pro Bibliothek **und für einzelne Titel**, Rechte pro Nutzer (Download, Untertitel-Upload, Transcoding), Gast (aus, nur LAN) (US-20…24), Gesehen-Status und Fortschritt mit „Weiterschauen“, „Als Nächstes“, „Fortsetzen“ und Badges (falls nicht schon in 5, OD-UX-3), Audio-/Untertitelwahl pro Titel und Profil, Quick Connect, API-Keys, Favoriten, Playlists, Sammlungen, Labels und Mehrfachbearbeitung (ED-04, ED-08), Wiedergabeverlauf, Download, Untertitel-Upload (PL-21, PL-22), **Admin-Bereich schreibend**: Einstellungsseiten mit „Experteneinstellungen“ (AD-20, AD-23…26, AD-28…30), **Bibliotheken hinzufügen/bearbeiten** mit Typ, Ordnern und erweiterten Optionen (LS-01…03, AD-40), „Reihen anpassen“ (HO-08), Aufgabenplaner mit Wartungsfenster, Backup und Restore, Webhooks, Aktivitätsprotokoll, Live-Logs, Fehlerbehebung (AD-11, AD-12, AD-41, AD-42), Update-Hinweis, Sidebar-Layout pro Profil, Import aus Plex und Jellyfin, Einladungslinks
7b. **Fernzugriff:** Netzwerk-Erkennung (CGNAT/IPv6/UPnP), HTTPS mit certmagic + DuckDNS, automatische Portfreigabe, Erreichbarkeitstest, eingebautes tsnet, 2FA, Rate-Limiting, **Geräteverwaltung** mit Filter nach Client-Typ, „zuletzt gesehen“ und Widerruf (US-25), Anmeldeprotokoll (US-26), Fernzugriff und max. Remote-Bitrate pro Nutzer, Einstellungsseite „Fernzugriff“ (AD-21), signierte Stream-URLs
8. **DLNA + Chromecast:** SSDP/ContentDirectory, Renderer-Profile, Cast-Sender in der Web-App (Cast-Button SH-06), Einstellungsseite DLNA mit erkannten Renderern (AD-27)
9. **Verteilung:** Docker Multi-Arch und Compose-Dateien für DSM/QNAP, Windows-Installer und -Dienst, deb/rpm, Dokumentation. Synology-SPK (vorgezogen, ADR 0026) inklusive **Verknüpfung im DSM-Hauptmenü**: Mivo-Symbol im DSM-Startmenü wie bei anderen Paketen, öffnet die Web-App auf Port 8460 in einem neuen Tab (wird in der laufenden SPK-Arbeit umgesetzt; kein eingebettetes DSM-Fenster, kein DSM-SSO)
10. **LG-webOS-App:** 10-Fuß-UI mit derselben Informationsarchitektur wie die Web-App (Start-Reihen, Bibliothek mit Filter und Sortierung, Detail, Player mit Wiedergabeoptionen als Seitenpanel und Stream-Info, Profilauswahl mit PIN-Feld, Einstellungen), Spatial Navigation, webOS-Client-Profile (Mindestversion: OD-UX-1), `.ipk`-Build in der CI. Danach Test auf einem echten TV im Developer Mode, Store-Assets (Icons, Splash, Screenshots), Self-Checklist, UX-Szenario als PowerPoint, Testserver für die LG-QA, Einreichung in der Seller Lounge.
11. **Nach v1:** Live-TV und DVR (HDHomeRun, M3U/IPTV, XMLTV, Aufnahmen), SyncPlay, Vorkonvertierung („optimierte Versionen“), Kino-Trailer/Pre-Roll, E-Books/Comics, LDAP/OIDC, Dolby-Vision-Tonemapping, RKMPP, Anime-Metadaten, ListenBrainz, Erweiterungs-Schnittstelle

**Release v0.1-Alpha** = Meilensteine 1 + 2 + Docker-Image (Teil von Meilenstein 9), getestet auf deiner Synology.

**Erste Schritte nach Freigabe:**
1. Ordner `C:\Users\stefa\Documents\Mivo` anlegen, Sitzung dorthin wechseln, `git init`
2. `CLAUDE.md` (Projektregeln) und die sieben Agenten unter `.claude/agents/` anlegen
3. Werkzeuge prüfen (Go, Node, pnpm, FFmpeg, Docker) und fehlende melden
4. Meilenstein 1 über die Agenten-Schleife, danach Meilenstein 2 Voraussetzungen auf deinem Rechner sind Go, Node.js (LTS) und FFmpeg. Ich prüfe vor dem Start, was installiert ist.

---

## Verifikation

- **Unit-Tests (tabellengetrieben):** Namensparser, Entscheidungs-Engine (Format-Matrix × Client-Profile), HLS-Playlist-Generierung, Auth
- **Test-Korpus:** `scripts/gen-testmedia` erzeugt mit FFmpeg (`lavfi testsrc/sine`) kurze Clips in allen genannten Containern und Codecs, außerdem Audio-, Foto- und Untertitel-Beispiele
- **Integrationstest:** Korpus scannen; für jede Datei einen Stream mit Browser-Profil anfordern; die erzeugte HLS-Ausgabe erneut mit ffprobe prüfen. Erwartet wird, dass jede Datei abspielbar ist und den erwarteten Modus hat.
- **E2E (Playwright):** Einrichtungsassistent, Bibliothek anlegen, Film abspielen, Untertitel wechseln, Musik-Warteschlange; ab Meilenstein 5 zusätzlich Filter/Sortierung/Ansicht per URL wiederherstellen, Suche mit Vorschlagsliste, Mini-Player beim Navigieren; ab Meilenstein 7 Profil mit PIN wählen und Altersgrenze prüfen (gesperrter Titel erscheint weder in Liste, Suche noch per Direktlink)
- **Fernzugriff:** Integrationstests für Token-Ablauf, Rate-Limit und signierte URLs. Manuell testen: Zugriff per Mobilfunk mit WLAN aus, sowohl über Weg A als auch über Weg B; SSL-Labs-Test der Domain; ein Port-Scan von außen darf nur den einen freigegebenen Port zeigen.
- **webOS:** Unit-Tests für das Fokus- und Zurück-Verhalten. Im Simulator die Navigation prüfen; auf einem echten LG-TV die Wiedergabe-Matrix aus dem Test-Korpus durchspielen, denn der Simulator kann kein HEVC. Vor der Einreichung die Self-Checklist Punkt für Punkt selbst durchtesten.
- **Manuell:** Docker auf Synology (Container Manager) und QNAP (Container Station) mit `/dev/dri`, Windows-Dienst, DLNA mit VLC (UPnP-Browser) und einem Smart-TV, Chromecast mit einem echten Gerät
