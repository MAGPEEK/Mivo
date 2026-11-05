-- Read side of the library model v2 (ADR 0027): listings, children, details and artwork.
--
-- Every listing returns child_count (seasons of a show, episodes of a season) and leaf_count
-- (episodes of a show; 0 for everything else) from correlated subqueries. They cost one
-- lookup per page row into items_children plus a row fetch for the kind test; a page holds at
-- most library.MaxPageLimit rows, so the cost does not grow with the library.

-- ListItemsWithCounts returns the listed items with their counts. The library listing finds
-- the IDs of a page with a query built at run time (listing_query.go: sort and filters) and
-- reads the rows here, so the column mapping stays generated.
-- name: ListItemsWithCounts :many
SELECT sqlc.embed(items),
       (SELECT COUNT(*) FROM items AS children
        WHERE children.parent_id = items.id AND children.kind <> 'extra') AS child_count,
       (SELECT COUNT(*) FROM items AS seasons
        JOIN items AS episodes ON episodes.parent_id = seasons.id
        WHERE seasons.parent_id = items.id AND seasons.kind = 'season'
          AND episodes.kind = 'episode') AS leaf_count
FROM items
WHERE items.id IN (sqlc.slice(item_ids));

-- ListChildrenAscending and ListChildrenDescending return the children of an item in child
-- order (ChildReader): order_group 0 holds the numbered children and 1 the specials (season
-- 0) and the unnumbered ones, which come last; then by number, sort title and ID. The
-- descending query is the exact reverse. has_after = 0 starts at the beginning; otherwise the
-- page follows the key (after_group, after_number, after_sort_title, after_id). The parent_id
-- search uses items_children; the order group is an expression, so SQLite sorts one parent's
-- children in a temporary B-tree per page. That is intended: a parent has at most a few
-- hundred children, and the sort never touches other parents' rows.
-- name: ListChildrenAscending :many
SELECT sqlc.embed(items),
       CAST(CASE WHEN items.number IS NULL OR (items.kind = 'season' AND items.number = 0)
                 THEN 1 ELSE 0 END AS INTEGER) AS order_group,
       CAST(ifnull(items.number, 0) AS INTEGER) AS order_number,
       (SELECT COUNT(*) FROM items AS children
        WHERE children.parent_id = items.id AND children.kind <> 'extra') AS child_count,
       (SELECT COUNT(*) FROM items AS seasons
        JOIN items AS episodes ON episodes.parent_id = seasons.id
        WHERE seasons.parent_id = items.id AND seasons.kind = 'season'
          AND episodes.kind = 'episode') AS leaf_count
FROM items
WHERE items.parent_id = sqlc.arg(parent_id)
  AND items.kind <> 'extra'
  AND (CAST(sqlc.arg(has_after) AS INTEGER) = 0 OR (
        CASE WHEN items.number IS NULL OR (items.kind = 'season' AND items.number = 0)
             THEN 1 ELSE 0 END,
        ifnull(items.number, 0), items.sort_title, items.id)
       > (CAST(sqlc.arg(after_group) AS INTEGER), CAST(sqlc.arg(after_number) AS INTEGER),
          CAST(sqlc.arg(after_sort_title) AS TEXT), CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY order_group, order_number, items.sort_title, items.id
LIMIT sqlc.arg(row_limit);

-- name: ListChildrenDescending :many
SELECT sqlc.embed(items),
       CAST(CASE WHEN items.number IS NULL OR (items.kind = 'season' AND items.number = 0)
                 THEN 1 ELSE 0 END AS INTEGER) AS order_group,
       CAST(ifnull(items.number, 0) AS INTEGER) AS order_number,
       (SELECT COUNT(*) FROM items AS children
        WHERE children.parent_id = items.id AND children.kind <> 'extra') AS child_count,
       (SELECT COUNT(*) FROM items AS seasons
        JOIN items AS episodes ON episodes.parent_id = seasons.id
        WHERE seasons.parent_id = items.id AND seasons.kind = 'season'
          AND episodes.kind = 'episode') AS leaf_count
FROM items
WHERE items.parent_id = sqlc.arg(parent_id)
  AND items.kind <> 'extra'
  AND (CAST(sqlc.arg(has_after) AS INTEGER) = 0 OR (
        CASE WHEN items.number IS NULL OR (items.kind = 'season' AND items.number = 0)
             THEN 1 ELSE 0 END,
        ifnull(items.number, 0), items.sort_title, items.id)
       < (CAST(sqlc.arg(after_group) AS INTEGER), CAST(sqlc.arg(after_number) AS INTEGER),
          CAST(sqlc.arg(after_sort_title) AS TEXT), CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY order_group DESC, order_number DESC, items.sort_title DESC, items.id DESC
LIMIT sqlc.arg(row_limit);

-- GetItemCounts returns the counts of ListItemsWithCounts for one item.
-- name: GetItemCounts :one
SELECT (SELECT COUNT(*) FROM items AS children
        WHERE children.parent_id = items.id AND children.kind <> 'extra') AS child_count,
       (SELECT COUNT(*) FROM items AS seasons
        JOIN items AS episodes ON episodes.parent_id = seasons.id
        WHERE seasons.parent_id = items.id AND seasons.kind = 'season'
          AND episodes.kind = 'episode') AS leaf_count
FROM items
WHERE items.id = ?;

-- ListExtras returns the extras of an item by type (the order of library.ExtraType), then
-- title.
-- name: ListExtras :many
SELECT *
FROM items
WHERE parent_id = sqlc.arg(parent_id) AND kind = 'extra'
ORDER BY CASE extra_type
             WHEN 'trailer' THEN 0
             WHEN 'featurette' THEN 1
             WHEN 'behind_the_scenes' THEN 2
             WHEN 'deleted_scene' THEN 3
             WHEN 'interview' THEN 4
             WHEN 'scene' THEN 5
             WHEN 'short' THEN 6
             ELSE 7
         END,
         sort_title, id
LIMIT sqlc.arg(row_limit);

-- FirstEpisodeOfShow returns the first episode of the first regular season (number above 0)
-- that has one; the specials are not a play target.
-- name: FirstEpisodeOfShow :one
SELECT episodes.id
FROM items AS seasons
JOIN items AS episodes ON episodes.parent_id = seasons.id AND episodes.kind = 'episode'
WHERE seasons.parent_id = ? AND seasons.kind = 'season' AND seasons.number > 0
ORDER BY seasons.number, episodes.number IS NULL, episodes.number, episodes.sort_title, episodes.id
LIMIT 1;

-- ListPrimaryFiles returns the primary file (first by part number, folder position, relative
-- path) of each
-- listed item that has a file; the file row names its item.
-- name: ListPrimaryFiles :many
SELECT sqlc.embed(media_files)
FROM items
JOIN media_files ON media_files.id = (
    SELECT candidates.id
    FROM media_files AS candidates
    JOIN library_roots AS candidate_roots ON candidate_roots.id = candidates.root_id
    WHERE candidates.item_id = items.id
    ORDER BY candidates.part_number, candidate_roots.position, candidates.relative_path, candidates.id
    LIMIT 1)
WHERE items.id IN (sqlc.slice(item_ids));

-- ListSelectedArtwork returns the selected artwork of the listed items.
-- name: ListSelectedArtwork :many
SELECT item_id, id, kind, tag
FROM artwork
WHERE selected = 1 AND item_id IN (sqlc.slice(item_ids))
ORDER BY item_id, kind;

-- ListItemTags returns an item's genres, studios, countries and collections in source order.
-- name: ListItemTags :many
SELECT tags.kind, tags.name
FROM item_tags
JOIN tags ON tags.id = item_tags.tag_id
WHERE item_tags.item_id = ?
ORDER BY tags.kind, item_tags.position, tags.name;

-- ListItemCredits returns an item's people in source order.
-- name: ListItemCredits :many
SELECT people.name, item_people.role, item_people.character, item_people.position
FROM item_people
JOIN people ON people.id = item_people.person_id
WHERE item_people.item_id = ?
ORDER BY item_people.position, people.name, item_people.role;

-- name: ListItemProviderIDs :many
SELECT provider, external_id
FROM item_provider_ids
WHERE item_id = ?
ORDER BY provider;

-- GetArtworkWithRoot returns one artwork with the root of the library its bytes live in: the
-- artwork's own library for a local picture, the media file's library for an embedded one,
-- empty for a picture in the data directory.
-- name: GetArtworkWithRoot :one
SELECT artwork.id, artwork.kind, artwork.source, artwork.relative_path, artwork.stream_index,
       artwork.data_path, artwork.size_bytes, artwork.modified_at, artwork.tag,
       media_files.relative_path AS media_relative_path,
       CAST(ifnull(library_roots.path, '') AS TEXT) AS library_root
FROM artwork
LEFT JOIN media_files ON media_files.id = artwork.media_file_id
LEFT JOIN library_roots ON library_roots.id = ifnull(artwork.root_id, media_files.root_id)
WHERE artwork.id = ?;

-- name: ListArtworkTags :many
SELECT id, tag
FROM artwork;
