-- External subtitle writer and reader (M4-06b). Subtitles are stored per media file in the
-- order the scan listed them.

-- name: DeleteFileExternalSubtitles :exec
DELETE FROM external_subtitles
WHERE media_file_id = ?;

-- name: InsertExternalSubtitle :exec
INSERT INTO external_subtitles (media_file_id, track_id, ordinal, relative_path, format, language, title,
                                forced, sdh, is_default)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLibraryExternalSubtitles :many
SELECT external_subtitles.media_file_id, external_subtitles.relative_path, external_subtitles.format,
       external_subtitles.language, external_subtitles.title, external_subtitles.forced,
       external_subtitles.sdh, external_subtitles.is_default
FROM external_subtitles
JOIN media_files ON media_files.id = external_subtitles.media_file_id
WHERE media_files.library_id = ?
ORDER BY external_subtitles.media_file_id, external_subtitles.ordinal;

-- name: ListFileExternalSubtitles :many
SELECT external_subtitles.media_file_id, external_subtitles.relative_path, external_subtitles.format,
       external_subtitles.language, external_subtitles.title, external_subtitles.forced,
       external_subtitles.sdh, external_subtitles.is_default
FROM external_subtitles
WHERE external_subtitles.media_file_id = ?
ORDER BY external_subtitles.ordinal;
