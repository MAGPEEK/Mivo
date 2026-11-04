-- Library management before accounts (ADR 0028): several folders per library, per-library
-- options and metadata language, and the ledger of configuration roots that were imported.
-- ADR 0004 conventions apply: timestamps are INTEGER Unix milliseconds in UTC (the ledger's
-- imported_at too), IDs are text UUIDs chosen by the caller, every table is STRICT.
--
-- Libraries move from libraries.root_path to library_roots. The rows of libraries are kept:
-- each existing library gets one folder (same id, position 0), one ledger entry and
-- imported_from_config = 1, because it came from the configuration.
--
-- Removing the UNIQUE root_path column needs a rebuild of libraries, and every foreign key
-- below it (items and the tables under them) would then have to be rebuilt too. The catalog
-- is rebuildable (ADR 0027 section 5), so this migration drops it and recreates it with
-- root_id on the tables that store library-relative paths; the next scan re-probes the
-- files. Users upgrading from 0.1.0-alpha.2 run 00003 and 00004 in one start and see one
-- rebuild. Dropped with it: items, media_files, media_streams, artwork, library_sidecars,
-- item_field_locks (field locks are lost), item_provider_ids, item_people, item_tags, people
-- and tags.
--
-- Invariant for later writers: media_files.root_id and library_sidecars.root_id must belong to
-- the row's library_id (library_roots.library_id); the schema does not enforce it.
--
-- path_key of carried-over folders is the path with outer slashes trimmed, which is
-- library.FolderKey without case folding. SQL lower() is not Unicode-complete, so on Windows
-- the adapter re-keys carried-over folders in Go (DB.RekeyFolders).

-- +goose Up
CREATE TABLE libraries_carry AS
SELECT id, name, kind, root_path, created_at, updated_at
FROM libraries;

DROP TABLE library_sidecars;
DROP TABLE artwork;
DROP TABLE item_field_locks;
DROP TABLE item_provider_ids;
DROP TABLE item_people;
DROP TABLE item_tags;
DROP TABLE people;
DROP TABLE tags;
DROP TABLE media_streams;
DROP TABLE media_files;
DROP TABLE items;
DROP TABLE libraries;

CREATE TABLE libraries (
    id         TEXT    NOT NULL PRIMARY KEY,
    name       TEXT    NOT NULL CHECK (name <> ''),
    kind       TEXT    NOT NULL CHECK (kind IN ('movies', 'shows', 'music', 'audiobooks', 'photos')),
    metadata_language    TEXT    NOT NULL DEFAULT '',
    scan_extras          INTEGER NOT NULL DEFAULT 1 CHECK (scan_extras IN (0, 1)),
    read_nfo             INTEGER NOT NULL DEFAULT 1 CHECK (read_nfo IN (0, 1)),
    local_artwork        INTEGER NOT NULL DEFAULT 1 CHECK (local_artwork IN (0, 1)),
    imported_from_config INTEGER NOT NULL DEFAULT 0 CHECK (imported_from_config IN (0, 1)),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
) STRICT;

-- The folders of a library (ADR 0028). position orders them, path_key is the comparison
-- key library.FolderKey builds, so one folder can belong to one library only.
CREATE TABLE library_roots (
    id         TEXT    NOT NULL PRIMARY KEY,
    library_id TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    path       TEXT    NOT NULL CHECK (path <> ''),
    path_key   TEXT    NOT NULL UNIQUE CHECK (path_key <> ''),
    position   INTEGER NOT NULL CHECK (position >= 0),
    created_at INTEGER NOT NULL,
    -- Also serves the library_id foreign key.
    UNIQUE (library_id, position)
) STRICT;

-- Configuration roots that were imported once; an entry stays when the library is deleted,
-- so the import does not bring it back (ADR 0028).
CREATE TABLE library_imports (
    path_key    TEXT    NOT NULL PRIMARY KEY CHECK (path_key <> ''),
    path        TEXT    NOT NULL CHECK (path <> ''),
    imported_at INTEGER NOT NULL
) STRICT;

-- match_key identifies an item among its siblings of one kind across rescans (ADR 0027 §3).
-- number is the season, episode or track number, number_end the last episode of a
-- multi-episode file. Only extras have an extra_type, and movies and shows are never below
-- another item.
CREATE TABLE items (
    id                TEXT    NOT NULL PRIMARY KEY,
    library_id        TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    parent_id         TEXT    REFERENCES items (id) ON DELETE CASCADE,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie', 'extra', 'show', 'season', 'episode',
                                                       'artist', 'album', 'track', 'audiobook',
                                                       'photo_album', 'photo')),
    match_key         TEXT    NOT NULL CHECK (match_key <> ''),
    title             TEXT    NOT NULL CHECK (title <> ''),
    sort_title        TEXT    NOT NULL CHECK (sort_title <> ''),
    original_title    TEXT    NOT NULL DEFAULT '',
    year              INTEGER NOT NULL DEFAULT 0 CHECK (year >= 0),
    release_date      TEXT    NOT NULL DEFAULT ''
                              CHECK (release_date = '' OR release_date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    summary           TEXT    NOT NULL DEFAULT '',
    tagline           TEXT    NOT NULL DEFAULT '',
    content_rating    TEXT    NOT NULL DEFAULT '',
    number            INTEGER CHECK (number >= 0),
    number_end        INTEGER CHECK (number_end > number),
    absolute_number   INTEGER CHECK (absolute_number >= 0),
    disc_number       INTEGER CHECK (disc_number >= 1),
    extra_type        TEXT    CHECK (extra_type IN ('trailer', 'featurette', 'behind_the_scenes',
                                                    'deleted_scene', 'interview', 'scene', 'short',
                                                    'other')),
    metadata_language TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    CHECK ((kind = 'extra') = (extra_type IS NOT NULL)),
    CHECK (number_end IS NULL OR number IS NOT NULL),
    CHECK (parent_id IS NULL OR kind NOT IN ('movie', 'show'))
) STRICT;

-- Also serves the library_id foreign key.
CREATE UNIQUE INDEX items_match_key ON items (library_id, ifnull(parent_id, ''), kind, match_key);
-- Keyset pagination of a library's top-level items (movies, shows) by (sort_title, id).
CREATE INDEX items_top_level ON items (library_id, sort_title, id)
    WHERE parent_id IS NULL AND kind <> 'extra';
-- Children in display order; also serves the parent_id foreign key.
CREATE INDEX items_children ON items (parent_id, number, sort_title, id);

-- part_number 0 is a file that is not part of a multi-part version. The primary file of an
-- item is the first by (part_number, relative_path).
CREATE TABLE media_files (
    id            TEXT    NOT NULL PRIMARY KEY,
    item_id       TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    library_id    TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    root_id       TEXT    NOT NULL REFERENCES library_roots (id) ON DELETE CASCADE,
    relative_path TEXT    NOT NULL CHECK (relative_path <> ''),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    container    TEXT    NOT NULL CHECK (container <> ''),
    format_name   TEXT    NOT NULL,
    duration_ms   INTEGER NOT NULL CHECK (duration_ms >= 0),
    bit_rate      INTEGER NOT NULL CHECK (bit_rate >= 0),
    probed_at     INTEGER NOT NULL,
    probe_error   TEXT    CHECK (probe_error <> ''),
    version_label TEXT    NOT NULL DEFAULT '',
    part_number   INTEGER NOT NULL DEFAULT 0 CHECK (part_number >= 0),
    layout        TEXT    NOT NULL DEFAULT 'file' CHECK (layout IN ('file', 'iso', 'bdmv', 'video_ts')),
    -- Also serves the root_id foreign key.
    UNIQUE (root_id, relative_path)
) STRICT;

-- Serves the library_id foreign key and the lookup of a file by its place in a library.
CREATE INDEX media_files_library ON media_files (library_id, relative_path);

-- An item's files in primary-file order; also serves the item_id foreign key.
CREATE INDEX media_files_item ON media_files (item_id, part_number, relative_path, id);

-- frame_rate keeps the exact fraction as media.FrameRate.String() writes it ("24000/1001",
-- '' when unknown), so media.ParseFrameRate reads back the same value.
CREATE TABLE media_streams (
    media_file_id       TEXT    NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    stream_index        INTEGER NOT NULL CHECK (stream_index >= 0),
    kind                TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle', 'other')),
    codec               TEXT    NOT NULL,
    codec_tag           TEXT    CHECK (codec_tag <> ''),
    profile             TEXT    NOT NULL,
    level               INTEGER NOT NULL CHECK (level >= 0),
    pixel_format        TEXT    NOT NULL,
    bit_depth           INTEGER NOT NULL CHECK (bit_depth >= 0),
    width               INTEGER NOT NULL CHECK (width >= 0),
    height              INTEGER NOT NULL CHECK (height >= 0),
    frame_rate          TEXT    NOT NULL CHECK (frame_rate = '' OR (frame_rate GLOB '[1-9]*/[1-9]*'
                                                                    AND frame_rate NOT GLOB '*[^0-9/]*'
                                                                    AND frame_rate NOT GLOB '*/*/*')),
    color_primaries     TEXT    NOT NULL,
    color_transfer      TEXT    NOT NULL,
    color_space         TEXT    NOT NULL,
    hdr                 TEXT    NOT NULL,
    channels            INTEGER NOT NULL CHECK (channels >= 0),
    channel_layout      TEXT    NOT NULL,
    sample_rate         INTEGER NOT NULL CHECK (sample_rate >= 0),
    language            TEXT    NOT NULL,
    title               TEXT    NOT NULL,
    is_default          INTEGER NOT NULL CHECK (is_default IN (0, 1)),
    is_forced           INTEGER NOT NULL CHECK (is_forced IN (0, 1)),
    is_attached_picture INTEGER NOT NULL CHECK (is_attached_picture IN (0, 1)),
    bit_rate            INTEGER NOT NULL CHECK (bit_rate >= 0),
    PRIMARY KEY (media_file_id, stream_index)
) STRICT;

-- Genres, studios, countries and collections (NFO <set>), shared between items.
CREATE TABLE tags (
    id   TEXT NOT NULL PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('genre', 'studio', 'country', 'collection')),
    name TEXT NOT NULL CHECK (name <> ''),
    UNIQUE (kind, name)
) STRICT;

-- position keeps the order of the source (first genre first).
CREATE TABLE item_tags (
    item_id  TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    tag_id   TEXT    NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    -- Also serves the item_id foreign key.
    PRIMARY KEY (item_id, tag_id)
) STRICT;

CREATE INDEX item_tags_tag ON item_tags (tag_id);

-- People are identified by name until milestone 6 adds provider IDs.
CREATE TABLE people (
    id        TEXT NOT NULL PRIMARY KEY,
    name      TEXT NOT NULL UNIQUE CHECK (name <> ''),
    sort_name TEXT NOT NULL CHECK (sort_name <> '')
) STRICT;

-- role is a growing set (actor, director, writer, producer, guest_star; author and narrator
-- with audiobooks). character is '' for every role but actor and guest_star.
CREATE TABLE item_people (
    item_id   TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    person_id TEXT    NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    role      TEXT    NOT NULL CHECK (role <> ''),
    character TEXT    NOT NULL DEFAULT '',
    position  INTEGER NOT NULL CHECK (position >= 0),
    -- Also serves the item_id foreign key.
    PRIMARY KEY (item_id, person_id, role)
) STRICT;

CREATE INDEX item_people_person ON item_people (person_id);

-- provider is a growing set (tmdb, imdb, tvdb; MusicBrainz IDs with music).
CREATE TABLE item_provider_ids (
    item_id     TEXT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    provider    TEXT NOT NULL CHECK (provider <> ''),
    external_id TEXT NOT NULL CHECK (external_id <> ''),
    PRIMARY KEY (item_id, provider)
) STRICT;

-- A locked field keeps its stored value whatever a scan or provider reports (ADR 0027 §6).
CREATE TABLE item_field_locks (
    item_id TEXT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    field   TEXT NOT NULL CHECK (field <> ''),
    PRIMARY KEY (item_id, field)
) STRICT;

-- One artwork candidate of an item. Each source fills exactly its own location group:
--   local    root_id + relative_path (a file in a library folder)
--   embedded media_file_id + stream_index (an attached picture)
--   provider provider + remote_url, data_path once it is downloaded (milestone 6)
--   upload   data_path (milestone 6)
-- data_path is relative to the data directory. size_bytes and modified_at describe the
-- source file and feed the cache tag; width and height are 0 when unknown. At most one
-- candidate per (item, kind) is selected.
CREATE TABLE artwork (
    id            TEXT    NOT NULL PRIMARY KEY,
    item_id       TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    kind          TEXT    NOT NULL CHECK (kind <> ''),
    source        TEXT    NOT NULL CHECK (source IN ('local', 'embedded', 'provider', 'upload')),
    root_id       TEXT    REFERENCES library_roots (id) ON DELETE CASCADE,
    relative_path TEXT    CHECK (relative_path <> ''),
    media_file_id TEXT    REFERENCES media_files (id) ON DELETE CASCADE,
    stream_index  INTEGER CHECK (stream_index >= 0),
    data_path     TEXT    CHECK (data_path <> ''),
    provider      TEXT    CHECK (provider <> ''),
    remote_url    TEXT    CHECK (remote_url <> ''),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    width         INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height        INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    tag           TEXT    NOT NULL CHECK (tag <> ''),
    selected      INTEGER NOT NULL DEFAULT 0 CHECK (selected IN (0, 1)),
    created_at    INTEGER NOT NULL,
    CHECK ((root_id IS NOT NULL) = (source = 'local')),
    CHECK ((relative_path IS NOT NULL) = (source = 'local')),
    CHECK ((media_file_id IS NOT NULL) = (source = 'embedded')),
    CHECK ((stream_index IS NOT NULL) = (source = 'embedded')),
    CHECK ((provider IS NOT NULL) = (source = 'provider')),
    CHECK ((remote_url IS NOT NULL) = (source = 'provider')),
    CHECK (CASE source
               WHEN 'upload' THEN data_path IS NOT NULL
               WHEN 'provider' THEN 1
               ELSE data_path IS NULL
           END)
) STRICT;

-- Also serves the item_id foreign key.
CREATE INDEX artwork_item ON artwork (item_id, kind);
CREATE UNIQUE INDEX artwork_selected ON artwork (item_id, kind) WHERE selected = 1;
CREATE INDEX artwork_root ON artwork (root_id, relative_path);
CREATE INDEX artwork_media_file ON artwork (media_file_id, stream_index);

-- NFO and image files seen by the last scan, for change detection like media_files.
CREATE TABLE library_sidecars (
    library_id    TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    root_id       TEXT    NOT NULL REFERENCES library_roots (id) ON DELETE CASCADE,
    relative_path TEXT    NOT NULL CHECK (relative_path <> ''),
    kind          TEXT    NOT NULL CHECK (kind IN ('nfo', 'image')),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    -- Also serves the root_id foreign key.
    PRIMARY KEY (root_id, relative_path)
) STRICT;

-- Serves the library_id foreign key and the sidecar lookups of one library.
CREATE INDEX library_sidecars_library ON library_sidecars (library_id, relative_path);

INSERT INTO libraries (id, name, kind, imported_from_config, created_at, updated_at)
SELECT id, name, kind, 1, created_at, updated_at
FROM libraries_carry;

INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
SELECT id, id, root_path, CASE WHEN trim(root_path, '/') = '' THEN '/' ELSE trim(root_path, '/') END, 0, created_at
FROM libraries_carry;

INSERT INTO library_imports (path_key, path, imported_at)
SELECT CASE WHEN trim(root_path, '/') = '' THEN '/' ELSE trim(root_path, '/') END, root_path, created_at
FROM libraries_carry;

DROP TABLE libraries_carry;

-- +goose Down
-- Restores the catalog of 00003. Only the first folder of a library survives as root_path,
-- and a library without a folder is dropped.
CREATE TABLE libraries_carry AS
SELECT libraries.id, libraries.name, libraries.kind, library_roots.path AS root_path,
       libraries.created_at, libraries.updated_at
FROM libraries
JOIN library_roots ON library_roots.library_id = libraries.id AND library_roots.position = 0;

DROP TABLE library_sidecars;
DROP TABLE artwork;
DROP TABLE item_field_locks;
DROP TABLE item_provider_ids;
DROP TABLE item_people;
DROP TABLE item_tags;
DROP TABLE people;
DROP TABLE tags;
DROP TABLE media_streams;
DROP TABLE media_files;
DROP TABLE items;
DROP TABLE library_imports;
DROP TABLE library_roots;
DROP TABLE libraries;

CREATE TABLE libraries (
    id         TEXT    NOT NULL PRIMARY KEY,
    name       TEXT    NOT NULL CHECK (name <> ''),
    kind       TEXT    NOT NULL CHECK (kind IN ('movies', 'shows', 'music', 'audiobooks', 'photos')),
    root_path  TEXT    NOT NULL UNIQUE CHECK (root_path <> ''),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

-- match_key identifies an item among its siblings of one kind across rescans (ADR 0027 §3).
-- number is the season, episode or track number, number_end the last episode of a
-- multi-episode file. Only extras have an extra_type, and movies and shows are never below
-- another item.
CREATE TABLE items (
    id                TEXT    NOT NULL PRIMARY KEY,
    library_id        TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    parent_id         TEXT    REFERENCES items (id) ON DELETE CASCADE,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie', 'extra', 'show', 'season', 'episode',
                                                       'artist', 'album', 'track', 'audiobook',
                                                       'photo_album', 'photo')),
    match_key         TEXT    NOT NULL CHECK (match_key <> ''),
    title             TEXT    NOT NULL CHECK (title <> ''),
    sort_title        TEXT    NOT NULL CHECK (sort_title <> ''),
    original_title    TEXT    NOT NULL DEFAULT '',
    year              INTEGER NOT NULL DEFAULT 0 CHECK (year >= 0),
    release_date      TEXT    NOT NULL DEFAULT ''
                              CHECK (release_date = '' OR release_date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    summary           TEXT    NOT NULL DEFAULT '',
    tagline           TEXT    NOT NULL DEFAULT '',
    content_rating    TEXT    NOT NULL DEFAULT '',
    number            INTEGER CHECK (number >= 0),
    number_end        INTEGER CHECK (number_end > number),
    absolute_number   INTEGER CHECK (absolute_number >= 0),
    disc_number       INTEGER CHECK (disc_number >= 1),
    extra_type        TEXT    CHECK (extra_type IN ('trailer', 'featurette', 'behind_the_scenes',
                                                    'deleted_scene', 'interview', 'scene', 'short',
                                                    'other')),
    metadata_language TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    CHECK ((kind = 'extra') = (extra_type IS NOT NULL)),
    CHECK (number_end IS NULL OR number IS NOT NULL),
    CHECK (parent_id IS NULL OR kind NOT IN ('movie', 'show'))
) STRICT;

-- Also serves the library_id foreign key.
CREATE UNIQUE INDEX items_match_key ON items (library_id, ifnull(parent_id, ''), kind, match_key);
-- Keyset pagination of a library's top-level items (movies, shows) by (sort_title, id).
CREATE INDEX items_top_level ON items (library_id, sort_title, id)
    WHERE parent_id IS NULL AND kind <> 'extra';
-- Children in display order; also serves the parent_id foreign key.
CREATE INDEX items_children ON items (parent_id, number, sort_title, id);

-- part_number 0 is a file that is not part of a multi-part version. The primary file of an
-- item is the first by (part_number, relative_path).
CREATE TABLE media_files (
    id            TEXT    NOT NULL PRIMARY KEY,
    item_id       TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    library_id    TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    relative_path TEXT    NOT NULL CHECK (relative_path <> ''),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    container     TEXT    NOT NULL CHECK (container <> ''),
    format_name   TEXT    NOT NULL,
    duration_ms   INTEGER NOT NULL CHECK (duration_ms >= 0),
    bit_rate      INTEGER NOT NULL CHECK (bit_rate >= 0),
    probed_at     INTEGER NOT NULL,
    probe_error   TEXT    CHECK (probe_error <> ''),
    version_label TEXT    NOT NULL DEFAULT '',
    part_number   INTEGER NOT NULL DEFAULT 0 CHECK (part_number >= 0),
    layout        TEXT    NOT NULL DEFAULT 'file' CHECK (layout IN ('file', 'iso', 'bdmv', 'video_ts')),
    -- Also serves the library_id foreign key.
    UNIQUE (library_id, relative_path)
) STRICT;

-- An item's files in primary-file order; also serves the item_id foreign key.
CREATE INDEX media_files_item ON media_files (item_id, part_number, relative_path, id);

-- frame_rate keeps the exact fraction as media.FrameRate.String() writes it ("24000/1001",
-- '' when unknown), so media.ParseFrameRate reads back the same value.
CREATE TABLE media_streams (
    media_file_id       TEXT    NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    stream_index        INTEGER NOT NULL CHECK (stream_index >= 0),
    kind                TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle', 'other')),
    codec               TEXT    NOT NULL,
    codec_tag           TEXT    CHECK (codec_tag <> ''),
    profile             TEXT    NOT NULL,
    level               INTEGER NOT NULL CHECK (level >= 0),
    pixel_format        TEXT    NOT NULL,
    bit_depth           INTEGER NOT NULL CHECK (bit_depth >= 0),
    width               INTEGER NOT NULL CHECK (width >= 0),
    height              INTEGER NOT NULL CHECK (height >= 0),
    frame_rate          TEXT    NOT NULL CHECK (frame_rate = '' OR (frame_rate GLOB '[1-9]*/[1-9]*'
                                                                    AND frame_rate NOT GLOB '*[^0-9/]*'
                                                                    AND frame_rate NOT GLOB '*/*/*')),
    color_primaries     TEXT    NOT NULL,
    color_transfer      TEXT    NOT NULL,
    color_space         TEXT    NOT NULL,
    hdr                 TEXT    NOT NULL,
    channels            INTEGER NOT NULL CHECK (channels >= 0),
    channel_layout      TEXT    NOT NULL,
    sample_rate         INTEGER NOT NULL CHECK (sample_rate >= 0),
    language            TEXT    NOT NULL,
    title               TEXT    NOT NULL,
    is_default          INTEGER NOT NULL CHECK (is_default IN (0, 1)),
    is_forced           INTEGER NOT NULL CHECK (is_forced IN (0, 1)),
    is_attached_picture INTEGER NOT NULL CHECK (is_attached_picture IN (0, 1)),
    bit_rate            INTEGER NOT NULL CHECK (bit_rate >= 0),
    PRIMARY KEY (media_file_id, stream_index)
) STRICT;

-- Genres, studios, countries and collections (NFO <set>), shared between items.
CREATE TABLE tags (
    id   TEXT NOT NULL PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('genre', 'studio', 'country', 'collection')),
    name TEXT NOT NULL CHECK (name <> ''),
    UNIQUE (kind, name)
) STRICT;

-- position keeps the order of the source (first genre first).
CREATE TABLE item_tags (
    item_id  TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    tag_id   TEXT    NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    -- Also serves the item_id foreign key.
    PRIMARY KEY (item_id, tag_id)
) STRICT;

CREATE INDEX item_tags_tag ON item_tags (tag_id);

-- People are identified by name until milestone 6 adds provider IDs.
CREATE TABLE people (
    id        TEXT NOT NULL PRIMARY KEY,
    name      TEXT NOT NULL UNIQUE CHECK (name <> ''),
    sort_name TEXT NOT NULL CHECK (sort_name <> '')
) STRICT;

-- role is a growing set (actor, director, writer, producer, guest_star; author and narrator
-- with audiobooks). character is '' for every role but actor and guest_star.
CREATE TABLE item_people (
    item_id   TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    person_id TEXT    NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    role      TEXT    NOT NULL CHECK (role <> ''),
    character TEXT    NOT NULL DEFAULT '',
    position  INTEGER NOT NULL CHECK (position >= 0),
    -- Also serves the item_id foreign key.
    PRIMARY KEY (item_id, person_id, role)
) STRICT;

CREATE INDEX item_people_person ON item_people (person_id);

-- provider is a growing set (tmdb, imdb, tvdb; MusicBrainz IDs with music).
CREATE TABLE item_provider_ids (
    item_id     TEXT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    provider    TEXT NOT NULL CHECK (provider <> ''),
    external_id TEXT NOT NULL CHECK (external_id <> ''),
    PRIMARY KEY (item_id, provider)
) STRICT;

-- A locked field keeps its stored value whatever a scan or provider reports (ADR 0027 §6).
CREATE TABLE item_field_locks (
    item_id TEXT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    field   TEXT NOT NULL CHECK (field <> ''),
    PRIMARY KEY (item_id, field)
) STRICT;

-- One artwork candidate of an item. Each source fills exactly its own location group:
--   local    library_id + relative_path (a file in the library root)
--   embedded media_file_id + stream_index (an attached picture)
--   provider provider + remote_url, data_path once it is downloaded (milestone 6)
--   upload   data_path (milestone 6)
-- data_path is relative to the data directory. size_bytes and modified_at describe the
-- source file and feed the cache tag; width and height are 0 when unknown. At most one
-- candidate per (item, kind) is selected.
CREATE TABLE artwork (
    id            TEXT    NOT NULL PRIMARY KEY,
    item_id       TEXT    NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    kind          TEXT    NOT NULL CHECK (kind <> ''),
    source        TEXT    NOT NULL CHECK (source IN ('local', 'embedded', 'provider', 'upload')),
    library_id    TEXT    REFERENCES libraries (id) ON DELETE CASCADE,
    relative_path TEXT    CHECK (relative_path <> ''),
    media_file_id TEXT    REFERENCES media_files (id) ON DELETE CASCADE,
    stream_index  INTEGER CHECK (stream_index >= 0),
    data_path     TEXT    CHECK (data_path <> ''),
    provider      TEXT    CHECK (provider <> ''),
    remote_url    TEXT    CHECK (remote_url <> ''),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    width         INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height        INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    tag           TEXT    NOT NULL CHECK (tag <> ''),
    selected      INTEGER NOT NULL DEFAULT 0 CHECK (selected IN (0, 1)),
    created_at    INTEGER NOT NULL,
    CHECK ((library_id IS NOT NULL) = (source = 'local')),
    CHECK ((relative_path IS NOT NULL) = (source = 'local')),
    CHECK ((media_file_id IS NOT NULL) = (source = 'embedded')),
    CHECK ((stream_index IS NOT NULL) = (source = 'embedded')),
    CHECK ((provider IS NOT NULL) = (source = 'provider')),
    CHECK ((remote_url IS NOT NULL) = (source = 'provider')),
    CHECK (CASE source
               WHEN 'upload' THEN data_path IS NOT NULL
               WHEN 'provider' THEN 1
               ELSE data_path IS NULL
           END)
) STRICT;

-- Also serves the item_id foreign key.
CREATE INDEX artwork_item ON artwork (item_id, kind);
CREATE UNIQUE INDEX artwork_selected ON artwork (item_id, kind) WHERE selected = 1;
CREATE INDEX artwork_library ON artwork (library_id, relative_path);
CREATE INDEX artwork_media_file ON artwork (media_file_id, stream_index);

-- NFO and image files seen by the last scan, for change detection like media_files.
CREATE TABLE library_sidecars (
    library_id    TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    relative_path TEXT    NOT NULL CHECK (relative_path <> ''),
    kind          TEXT    NOT NULL CHECK (kind IN ('nfo', 'image')),
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    modified_at   INTEGER NOT NULL,
    -- Also serves the library_id foreign key.
    PRIMARY KEY (library_id, relative_path)
) STRICT;

INSERT INTO libraries (id, name, kind, root_path, created_at, updated_at)
SELECT id, name, kind, root_path, created_at, updated_at
FROM libraries_carry;

DROP TABLE libraries_carry;
