-- Photo libraries (M3-15e, ADR 0029). Albums and photos are ordinary items (kinds
-- photo_album and photo since 00003); this table adds the EXIF values of a photo and the keys
-- an album orders its photos by. Additive: no existing table changes.
--
-- The RAW file next to a JPEG is a second media file of the photo item; primary_file_id names
-- the file the photo is shown from (the JPEG), because the primary-file order by path would
-- often pick the RAW file ("a.cr2" before "a.jpg"). It turns NULL when that file is deleted;
-- the readers then fall back to the primary-file order, while name_key, taken_at and the
-- EXIF values keep describing the deleted file until the next scan saves the details again.
--
-- taken_at is the capture time in Unix milliseconds (wall-clock read as UTC): the EXIF time,
-- or the file's modification time when the file has none (taken_at_source says which).
-- name_key is the lower-case base name of the photo's own file, the second sort key.
-- library_id and parent_id copy the item's columns (parent NULL for a photo in the library
-- folder); an item never changes either, because the placement finds items by (library,
-- parent, kind, match key) and no statement updates items.parent_id or items.library_id
-- (MoveFile re-parents files, not items), so the copies cannot go stale.
-- Zero values mean "not present" as in photo.Info; lat and lon are both NULL without GPS.

-- +goose Up
CREATE TABLE photo_info (
    item_id         TEXT    NOT NULL PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    library_id      TEXT    NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    parent_id       TEXT    REFERENCES items (id) ON DELETE CASCADE,
    primary_file_id TEXT    REFERENCES media_files (id) ON DELETE SET NULL,
    name_key        TEXT    NOT NULL CHECK (name_key <> ''),
    taken_at        INTEGER NOT NULL,
    taken_at_source TEXT    NOT NULL CHECK (taken_at_source IN ('exif', 'file')),
    make            TEXT    NOT NULL,
    model           TEXT    NOT NULL,
    lens            TEXT    NOT NULL,
    exposure        REAL    NOT NULL CHECK (exposure >= 0),
    f_number        REAL    NOT NULL CHECK (f_number >= 0),
    iso             INTEGER NOT NULL CHECK (iso >= 0),
    focal_mm        REAL    NOT NULL CHECK (focal_mm >= 0),
    orientation     INTEGER NOT NULL CHECK (orientation BETWEEN 0 AND 8),
    lat             REAL    CHECK (lat BETWEEN -90 AND 90),
    lon             REAL    CHECK (lon BETWEEN -180 AND 180),
    width           INTEGER NOT NULL CHECK (width >= 0),
    height          INTEGER NOT NULL CHECK (height >= 0),
    thumbnail_state TEXT    NOT NULL CHECK (thumbnail_state IN ('ready', 'unsupported')),
    CHECK ((lat IS NULL) = (lon IS NULL))
) STRICT;

-- The photos of an album in display order (keyset pagination); also serves the parent_id
-- foreign key.
CREATE INDEX photo_info_album ON photo_info (parent_id, taken_at, name_key, item_id);
-- Every photo of a library in the same order, for the library-wide photo listing (timeline);
-- also serves the library_id foreign key.
CREATE INDEX photo_info_library ON photo_info (library_id, taken_at, name_key, item_id);
-- Serves the primary_file_id foreign key, so deleting a file does not scan the table.
CREATE INDEX photo_info_primary_file ON photo_info (primary_file_id);

-- +goose Down
DROP TABLE photo_info;
