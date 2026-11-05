-- Music writer and reader (M3-13d). Artists, albums and tracks are ordinary items; the
-- placement writer creates them, these queries add what a placement level cannot carry and
-- the music listings.

-- name: SetTrackDisc :exec
UPDATE items
SET disc_number = sqlc.arg(disc_number), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND kind = 'track' AND disc_number IS NOT sqlc.arg(disc_number);

-- UpsertMusicProviderID stores one MusicBrainz ID of the tags; a newer tag value replaces the
-- stored one.
-- name: UpsertMusicProviderID :exec
INSERT INTO item_provider_ids (item_id, provider, external_id)
VALUES (?, ?, ?)
ON CONFLICT (item_id, provider) DO UPDATE
SET external_id = excluded.external_id;

-- name: DeleteMusicProviderID :exec
DELETE FROM item_provider_ids
WHERE item_id = ? AND provider = ?;

-- ListArtistAlbums returns the albums of an artist after the key (after_sort_title, after_id)
-- in keyset order, each with its track count. The empty key lies before every album.
-- name: ListArtistAlbums :many
SELECT sqlc.embed(items),
       (SELECT COUNT(*) FROM items AS tracks
        WHERE tracks.parent_id = items.id AND tracks.kind = 'track') AS child_count
FROM items
WHERE items.parent_id = sqlc.arg(parent_id)
  AND items.kind = 'album'
  AND items.sort_title >= sqlc.arg(after_sort_title)
  AND (items.sort_title > sqlc.arg(after_sort_title) OR items.id > sqlc.arg(after_id))
ORDER BY items.sort_title, items.id
LIMIT sqlc.arg(row_limit);

-- ListLibraryAlbums is ListArtistAlbums over every album of a library. The conditions repeat
-- the partial index items_albums, so SQLite scans it as a range.
-- name: ListLibraryAlbums :many
SELECT sqlc.embed(items),
       (SELECT COUNT(*) FROM items AS tracks
        WHERE tracks.parent_id = items.id AND tracks.kind = 'track') AS child_count
FROM items
WHERE items.library_id = sqlc.arg(library_id)
  AND items.kind = 'album'
  AND items.sort_title >= sqlc.arg(after_sort_title)
  AND (items.sort_title > sqlc.arg(after_sort_title) OR items.id > sqlc.arg(after_id))
ORDER BY items.sort_title, items.id
LIMIT sqlc.arg(row_limit);

-- ListAlbumTracks returns every track of an album with the relative path of its primary file
-- ('' without file). The adapter orders them by library.CompareTracks, which needs the file
-- name; an album holds at most a few hundred tracks.
-- name: ListAlbumTracks :many
SELECT sqlc.embed(items),
       CAST(ifnull((SELECT media_files.relative_path FROM media_files
                    WHERE media_files.item_id = items.id
                    ORDER BY media_files.part_number, media_files.relative_path
                    LIMIT 1), '') AS TEXT) AS primary_path
FROM items
WHERE items.parent_id = sqlc.arg(parent_id)
  AND items.kind = 'track';
