-- Online metadata, part two (M6-11, ADR 0031): item_match remembers the provider entry an
-- item was linked by and when enrichment last looked at the item.
--
-- The ref_* columns hold enrich.MatchResult.Ref. A MusicBrainz ID cannot live in
-- item_provider_ids (metadata.Provider has no MusicBrainz release or artist), so without
-- them every music item would be searched again on every run. Enrichment uses the stored
-- reference only when the record holds no usable TMDB or IMDb ID, so a corrected NFO ID or an
-- Identify edit always wins. checked_at drives the periodic refresh: rows from 00005 have
-- none and are due on the first run.

-- +goose Up
ALTER TABLE item_match ADD COLUMN ref_provider TEXT NOT NULL DEFAULT ''
    CHECK (ref_provider IN ('', 'tmdb', 'imdb', 'musicbrainz'));
ALTER TABLE item_match ADD COLUMN ref_kind TEXT NOT NULL DEFAULT ''
    CHECK (ref_kind IN ('', 'movie', 'tv', 'release', 'artist'));
ALTER TABLE item_match ADD COLUMN ref_id TEXT NOT NULL DEFAULT '';
ALTER TABLE item_match ADD COLUMN checked_at INTEGER;

-- Items due for the periodic refresh, oldest first; disabled matches are never refreshed.
CREATE INDEX item_match_checked ON item_match (checked_at, item_id) WHERE state <> 'disabled';

-- +goose Down
DROP INDEX item_match_checked;
ALTER TABLE item_match DROP COLUMN checked_at;
ALTER TABLE item_match DROP COLUMN ref_id;
ALTER TABLE item_match DROP COLUMN ref_kind;
ALTER TABLE item_match DROP COLUMN ref_provider;
