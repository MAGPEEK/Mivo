-- Per-profile watch state (ADR 0032 section 8; task M7-10). The rows of watch_state have no
-- foreign key to items, so every query that lists items joins items and thereby ignores the
-- rows of items that no longer exist.

-- ListWatchStates returns the stored states of the given items of one profile; items
-- without a row are absent.
-- name: ListWatchStates :many
SELECT item_id, position_seconds, watched, play_count, last_played_at, hidden_from_continue, attach_key
FROM watch_state
WHERE profile_id = sqlc.arg(profile_id)
  AND item_id IN (sqlc.slice(item_ids));

-- UpsertWatchState stores one state; an existing row of the item is replaced.
-- name: UpsertWatchState :exec
INSERT INTO watch_state (profile_id, item_id, position_seconds, watched, play_count, last_played_at,
                         hidden_from_continue, attach_key, updated_at)
VALUES (sqlc.arg(profile_id), sqlc.arg(item_id), sqlc.arg(position_seconds), sqlc.arg(watched),
        sqlc.arg(play_count), sqlc.arg(last_played_at), sqlc.arg(hidden_from_continue),
        sqlc.arg(attach_key), sqlc.arg(updated_at))
ON CONFLICT (profile_id, item_id) DO UPDATE SET
    position_seconds     = excluded.position_seconds,
    watched              = excluded.watched,
    play_count           = excluded.play_count,
    last_played_at       = excluded.last_played_at,
    hidden_from_continue = excluded.hidden_from_continue,
    attach_key           = excluded.attach_key,
    updated_at           = excluded.updated_at;

-- CountSeasonEpisodes returns per season its episodes and how many of them the profile has
-- not watched. A season without episodes is absent.
-- name: CountSeasonEpisodes :many
SELECT seasons.id AS container_id,
       COUNT(*) AS total,
       CAST(SUM(CASE WHEN watch_state.watched = 1 THEN 0 ELSE 1 END) AS INTEGER) AS unwatched
FROM items AS seasons
JOIN items AS episodes ON episodes.parent_id = seasons.id AND episodes.kind = 'episode'
LEFT JOIN watch_state ON watch_state.profile_id = sqlc.arg(profile_id)
                     AND watch_state.item_id = episodes.id
WHERE seasons.id IN (sqlc.slice(container_ids))
  AND seasons.kind = 'season'
GROUP BY seasons.id;

-- CountShowEpisodes is CountSeasonEpisodes for shows: the episodes of all their seasons,
-- specials included, as ItemSummary.LeafCount counts them.
-- name: CountShowEpisodes :many
SELECT shows.id AS container_id,
       COUNT(*) AS total,
       CAST(SUM(CASE WHEN watch_state.watched = 1 THEN 0 ELSE 1 END) AS INTEGER) AS unwatched
FROM items AS shows
JOIN items AS seasons ON seasons.parent_id = shows.id AND seasons.kind = 'season'
JOIN items AS episodes ON episodes.parent_id = seasons.id AND episodes.kind = 'episode'
LEFT JOIN watch_state ON watch_state.profile_id = sqlc.arg(profile_id)
                     AND watch_state.item_id = episodes.id
WHERE shows.id IN (sqlc.slice(container_ids))
  AND shows.kind = 'show'
GROUP BY shows.id;

-- ListContinueWatching returns the item IDs of Continue watching (HO-03): items in progress,
-- not hidden, played at or after played_since, newest first; of a show only its most recently
-- played episode. An empty library_id means all libraries. Ties fall back to the item ID.
-- name: ListContinueWatching :many
WITH candidates AS (
    SELECT watch_state.item_id,
           watch_state.last_played_at,
           CASE WHEN items.kind = 'episode' THEN ifnull(seasons.parent_id, items.id)
                ELSE items.id END AS group_id
    FROM watch_state
    JOIN items ON items.id = watch_state.item_id
    LEFT JOIN items AS seasons ON seasons.id = items.parent_id
    WHERE watch_state.profile_id = sqlc.arg(profile_id)
      AND watch_state.last_played_at >= CAST(sqlc.arg(played_since) AS INTEGER)
      AND watch_state.position_seconds > 0
      AND watch_state.hidden_from_continue = 0
      AND items.kind IN ('movie', 'episode', 'audiobook')
      AND (CAST(sqlc.arg(library_id) AS TEXT) = '' OR items.library_id = sqlc.arg(library_id))
),
ranked AS (
    SELECT item_id, last_played_at,
           ROW_NUMBER() OVER (PARTITION BY group_id ORDER BY last_played_at DESC, item_id) AS rank_in_group
    FROM candidates
)
SELECT item_id
FROM ranked
WHERE rank_in_group = 1
ORDER BY last_played_at DESC, item_id
LIMIT sqlc.arg(row_limit);

-- ListUpNextShows returns the shows of Up next (HO-04) newest activity first, ties by show
-- ID: shows with an episode watched at or after played_since, without an episode in Continue
-- watching (in progress, not hidden, played at or after played_since), so a show is in at
-- most one of the two rows. An empty library_id means all libraries. Which episode is next
-- is decided per show by domain/watch.NextUp on ListShowEpisodeStates; a show may have none.
-- Walking every episode of every active show in SQL instead costs a multiple of this.
-- Both CTEs join the show item, so a parentless season adds no NULL show_id (which would
-- make NOT IN unknown and empty the row).
-- name: ListUpNextShows :many
WITH active_shows AS (
    SELECT shows.id AS show_id, MAX(watch_state.last_played_at) AS last_activity
    FROM watch_state
    JOIN items AS episodes ON episodes.id = watch_state.item_id AND episodes.kind = 'episode'
    JOIN items AS seasons ON seasons.id = episodes.parent_id AND seasons.kind = 'season'
    JOIN items AS shows ON shows.id = seasons.parent_id AND shows.kind = 'show'
    WHERE watch_state.profile_id = sqlc.arg(profile_id)
      AND watch_state.watched = 1
      AND watch_state.last_played_at >= CAST(sqlc.arg(played_since) AS INTEGER)
      AND (CAST(sqlc.arg(library_id) AS TEXT) = '' OR episodes.library_id = sqlc.arg(library_id))
    GROUP BY shows.id
),
continuing_shows AS (
    SELECT shows.id AS show_id
    FROM watch_state
    JOIN items AS episodes ON episodes.id = watch_state.item_id AND episodes.kind = 'episode'
    JOIN items AS seasons ON seasons.id = episodes.parent_id AND seasons.kind = 'season'
    JOIN items AS shows ON shows.id = seasons.parent_id AND shows.kind = 'show'
    WHERE watch_state.profile_id = sqlc.arg(profile_id)
      AND watch_state.position_seconds > 0
      AND watch_state.hidden_from_continue = 0
      AND watch_state.last_played_at >= CAST(sqlc.arg(played_since) AS INTEGER)
)
SELECT show_id
FROM active_shows
WHERE show_id NOT IN (SELECT show_id FROM continuing_shows)
ORDER BY last_activity DESC, show_id;

-- ListShowEpisodeStates returns every episode of the shows (specials included) with its
-- season and episode numbers (0 when missing) and whether the profile has watched it.
-- name: ListShowEpisodeStates :many
SELECT CAST(seasons.parent_id AS TEXT) AS show_id,
       episodes.id AS item_id,
       CAST(ifnull(seasons.number, 0) AS INTEGER) AS season_number,
       CAST(ifnull(episodes.number, 0) AS INTEGER) AS episode_number,
       CAST(ifnull(episodes.number_end, 0) AS INTEGER) AS episode_end,
       CAST(ifnull(watch_state.watched, 0) AS INTEGER) AS watched
FROM items AS seasons
JOIN items AS episodes ON episodes.parent_id = seasons.id AND episodes.kind = 'episode'
LEFT JOIN watch_state ON watch_state.profile_id = sqlc.arg(profile_id)
                     AND watch_state.item_id = episodes.id
WHERE seasons.parent_id IN (sqlc.slice(show_ids))
  AND seasons.kind = 'season';
