-- Chapters per media file (M3-14d, ADR 0027 second wave). Audiobooks store the chapter list
-- of an M4B or one chapter per part file; milestone 4 reuses the table for video chapters
-- (PL-30), so it hangs off media_files, not off an item kind. Offsets are relative to the
-- start of their file; the reader adds the durations of the preceding files of an item to
-- expose work-relative offsets. Additive: no existing table changes.

-- +goose Up
-- ordinal is the 0-based position of the chapter within its file. The primary key keeps
-- (file, ordinal) unique, orders a file's chapters and serves the media_file_id foreign key.
CREATE TABLE media_chapters (
    media_file_id TEXT    NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    ordinal       INTEGER NOT NULL CHECK (ordinal >= 0),
    title         TEXT    NOT NULL,
    start_ms      INTEGER NOT NULL CHECK (start_ms >= 0),
    end_ms        INTEGER NOT NULL CHECK (end_ms > start_ms),
    PRIMARY KEY (media_file_id, ordinal)
) STRICT;

-- +goose Down
DROP TABLE media_chapters;
