-- External subtitle files next to a media file (M4-06b, ADR 0030 section 3). The scan stores
-- what the folder holds; the playback input builder reads it with the file's own tracks.
-- Additive: no existing table changes.

-- +goose Up
-- ordinal is the 0-based position in the order the scan listed the files (path-sorted), so
-- the reader hands the list back unchanged and an unchanged folder is never rewritten.
-- track_id is the stable "x:" id of the subtitle (media.ExternalSubtitle.TrackID); the primary
-- key keeps it unique per file and serves the media_file_id foreign key.
CREATE TABLE external_subtitles (
    media_file_id TEXT    NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    track_id      TEXT    NOT NULL,
    ordinal       INTEGER NOT NULL CHECK (ordinal >= 0),
    relative_path TEXT    NOT NULL,
    format        TEXT    NOT NULL,
    language      TEXT    NOT NULL,
    title         TEXT    NOT NULL,
    forced        INTEGER NOT NULL CHECK (forced IN (0, 1)),
    sdh           INTEGER NOT NULL CHECK (sdh IN (0, 1)),
    is_default    INTEGER NOT NULL CHECK (is_default IN (0, 1)),
    PRIMARY KEY (media_file_id, track_id),
    UNIQUE (media_file_id, ordinal)
) STRICT;

-- +goose Down
DROP TABLE external_subtitles;
