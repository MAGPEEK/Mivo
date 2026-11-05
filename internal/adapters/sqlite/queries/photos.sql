-- Photo writer and reader (M3-15e, ADR 0029). Albums and photos are ordinary items saved by
-- the placement writer; photo_info adds the EXIF values and the album sort keys.

-- name: UpsertPhotoInfo :exec
INSERT INTO photo_info (item_id, library_id, parent_id, primary_file_id, name_key, taken_at, taken_at_source, make,
                        model, lens, exposure, f_number, iso, focal_mm, orientation, lat, lon, width, height,
                        thumbnail_state)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (item_id) DO UPDATE
SET library_id = excluded.library_id,
    parent_id = excluded.parent_id,
    primary_file_id = excluded.primary_file_id,
    name_key = excluded.name_key,
    taken_at = excluded.taken_at,
    taken_at_source = excluded.taken_at_source,
    make = excluded.make,
    model = excluded.model,
    lens = excluded.lens,
    exposure = excluded.exposure,
    f_number = excluded.f_number,
    iso = excluded.iso,
    focal_mm = excluded.focal_mm,
    orientation = excluded.orientation,
    lat = excluded.lat,
    lon = excluded.lon,
    width = excluded.width,
    height = excluded.height,
    thumbnail_state = excluded.thumbnail_state;

-- name: GetPhotoInfo :one
SELECT *
FROM photo_info
WHERE item_id = ?;

-- ListPhotoInfos returns the photo details of the listed items that have them.
-- name: ListPhotoInfos :many
SELECT *
FROM photo_info
WHERE item_id IN (sqlc.slice(item_ids));

-- ListPhotoSubAlbums returns the sub-albums of an album after the key (after_sort_title,
-- after_id) in keyset order, each with the number of its direct children as ListPhotoChildren
-- lists them: sub-albums plus photos with photo details. The empty key lies
-- before every album.
-- name: ListPhotoSubAlbums :many
SELECT sqlc.embed(items),
       (SELECT COUNT(*) FROM items AS children
        WHERE children.parent_id = items.id AND children.kind = 'photo_album')
       + (SELECT COUNT(*) FROM photo_info WHERE photo_info.parent_id = items.id) AS child_count
FROM items
WHERE items.parent_id = sqlc.arg(parent_id)
  AND items.kind = 'photo_album'
  AND items.sort_title >= sqlc.arg(after_sort_title)
  AND (items.sort_title > sqlc.arg(after_sort_title) OR items.id > sqlc.arg(after_id))
ORDER BY items.sort_title, items.id
LIMIT sqlc.arg(row_limit);

-- ListAlbumPhotos returns the photos of an album after the key (after_taken_at,
-- after_name_key, after_id) in display order. The leading range condition lets SQLite scan
-- photo_info_album from the key on; the caller starts with the smallest taken_at.
-- name: ListAlbumPhotos :many
SELECT sqlc.embed(items), sqlc.embed(photo_info)
FROM photo_info
JOIN items ON items.id = photo_info.item_id
WHERE photo_info.parent_id = sqlc.arg(parent_id)
  AND photo_info.taken_at >= sqlc.arg(after_taken_at)
  AND (photo_info.taken_at > sqlc.arg(after_taken_at)
       OR photo_info.name_key > sqlc.arg(after_name_key)
       OR (photo_info.name_key = sqlc.arg(after_name_key) AND photo_info.item_id > sqlc.arg(after_id)))
ORDER BY photo_info.taken_at, photo_info.name_key, photo_info.item_id
LIMIT sqlc.arg(row_limit);

-- GetPhotoMainFile returns the file a photo is rendered from: the primary file of its photo
-- details, else the first in primary-file order.
-- name: GetPhotoMainFile :one
SELECT media_files.relative_path, media_files.size_bytes, media_files.modified_at,
       library_roots.path AS root_path
FROM items
JOIN media_files ON media_files.item_id = items.id
JOIN library_roots ON library_roots.id = media_files.root_id
LEFT JOIN photo_info ON photo_info.item_id = items.id
WHERE items.id = ? AND items.kind = 'photo'
ORDER BY media_files.id IS NOT photo_info.primary_file_id, library_roots.position, media_files.relative_path,
         media_files.id
LIMIT 1;

-- ListPhotoMainFiles returns the files of every photo, per item in the order of
-- GetPhotoMainFile; the first row of an item is its main file.
-- name: ListPhotoMainFiles :many
SELECT media_files.item_id, media_files.relative_path, media_files.size_bytes, media_files.modified_at
FROM items
JOIN media_files ON media_files.item_id = items.id
JOIN library_roots ON library_roots.id = media_files.root_id
LEFT JOIN photo_info ON photo_info.item_id = items.id
WHERE items.kind = 'photo'
ORDER BY media_files.item_id, media_files.id IS NOT photo_info.primary_file_id, library_roots.position,
         media_files.relative_path, media_files.id;

-- GetPhotoAlbumTotals returns the number of photos in an album and all its sub-albums and the
-- newest of them whose thumbnail can be rendered (latest capture time, ties by the last name
-- key and ID; empty without such a photo), the album's tile. A video counts as a photo but
-- is no tile: the image endpoint renders still images only, which the scan saves with the
-- container 'unknown'. Albums nest as deep as the folder tree, usually a few levels.
-- name: GetPhotoAlbumTotals :one
WITH RECURSIVE albums (id) AS (
    SELECT items.id FROM items WHERE items.id = sqlc.arg(album_id)
    UNION ALL
    SELECT items.id
    FROM items
    JOIN albums ON items.parent_id = albums.id
    WHERE items.kind = 'photo_album'
)
SELECT (SELECT COUNT(*) FROM photo_info WHERE photo_info.parent_id IN (SELECT id FROM albums)) AS leaf_count,
       CAST(ifnull((SELECT photo_info.item_id FROM photo_info
                    WHERE photo_info.parent_id IN (SELECT id FROM albums)
                      AND photo_info.thumbnail_state = 'ready'
                      AND EXISTS (SELECT 1 FROM media_files
                                  WHERE media_files.id = photo_info.primary_file_id
                                    AND media_files.container = 'unknown')
                    ORDER BY photo_info.taken_at DESC, photo_info.name_key DESC, photo_info.item_id DESC
                    LIMIT 1), '') AS TEXT) AS cover_photo_id;

-- ListLibraryPhotos is ListAlbumPhotos over every photo of a library, whatever album it is in
-- (the photo timeline); it scans photo_info_library from the key on.
-- name: ListLibraryPhotos :many
SELECT sqlc.embed(items), sqlc.embed(photo_info)
FROM photo_info
JOIN items ON items.id = photo_info.item_id
WHERE photo_info.library_id = sqlc.arg(library_id)
  AND photo_info.taken_at >= sqlc.arg(after_taken_at)
  AND (photo_info.taken_at > sqlc.arg(after_taken_at)
       OR photo_info.name_key > sqlc.arg(after_name_key)
       OR (photo_info.name_key = sqlc.arg(after_name_key) AND photo_info.item_id > sqlc.arg(after_id)))
ORDER BY photo_info.taken_at, photo_info.name_key, photo_info.item_id
LIMIT sqlc.arg(row_limit);
