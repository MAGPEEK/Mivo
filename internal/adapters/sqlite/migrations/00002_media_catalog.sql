-- The media catalog of ADR 0022: configured libraries, one item per video file (milestone 2),
-- the files with their probe result and the streams of each file. ADR 0004 conventions apply:
-- timestamps are INTEGER Unix milliseconds in UTC, IDs are text UUIDs chosen by the caller.
--
-- Deleting a library removes its items, files and streams by cascade; the adapter connects
-- with foreign_keys=ON. Every foreign key column leads an index, so a cascade never scans a
-- whole table per deleted parent row.
--
-- Columns without a value use the zero value of the domain field ('' or 0, "not reported by
-- ffprobe"), so the mapping to internal/domain/media stays one to one. Only the columns
-- whose absence the domain models separately are nullable: items.parent_id (reserved for
-- the hierarchy of milestone 3), media_files.probe_error (NULL = probed successfully) and
-- media_streams.codec_tag (NULL = the container stores no tag, as in Matroska).
--
-- CHECK constraints pin the closed sets (library, item and stream kinds) and keep sizes and
-- counts non-negative. Open sets that grow with later milestones (containers, HDR formats,
-- probe error codes) are only checked for non-empty text: SQLite cannot alter a CHECK in
-- place, and the adapter validates them through the domain parsers when it reads them.

-- +goose Up
CREATE TABLE libraries (
    id         TEXT    NOT NULL PRIMARY KEY,
    name       TEXT    NOT NULL CHECK (name <> ''),
    kind       TEXT    NOT NULL CHECK (kind IN ('movies', 'shows')),
    root_path  TEXT    NOT NULL UNIQUE CHECK (root_path <> ''),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE items (
    id         TEXT    NOT NULL PRIMARY KEY,
    library_id TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    parent_id  TEXT    REFERENCES items (id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('movie', 'episode')),
    title      TEXT    NOT NULL CHECK (title <> ''),
    sort_title TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

-- Keyset pagination of a library's items by (sort_title, id); also serves the library_id
-- foreign key.
CREATE INDEX items_library_sort_title ON items (library_id, sort_title, id);
CREATE INDEX items_parent ON items (parent_id);

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
    -- Also serves the library_id foreign key.
    UNIQUE (library_id, relative_path)
) STRICT;

CREATE INDEX media_files_item ON media_files (item_id);

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
    frame_rate          TEXT    NOT NULL CHECK (frame_rate = '' OR frame_rate GLOB '[1-9]*/[1-9]*'),
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

-- +goose Down
DROP TABLE media_streams;
DROP TABLE media_files;
DROP TABLE items;
DROP TABLE libraries;
