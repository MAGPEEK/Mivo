-- Online metadata, part three (M6-14, ADR 0031): the settings changed in Settings › Metadata
-- and the records of an item's local sources.
--
-- Number 00012 follows 00011 accounts (M7-08); watch state and age ratings of milestone 7 take
-- 00013 and 00014 (docs/tasks/M7-00-overview.md). goose refuses missing earlier versions, so
-- 00011 must be merged before a build with this migration is released.
--
-- metadata_settings has at most one row. Without it the configuration applies; with it the
-- stored state wins, so a change made in the UI survives a restart. tmdb_api_key is the
-- user's own key ('' for none); it is never returned by the API or logged.
--
-- item_local_records keeps what the NFO and the file name (with the embedded tags) said in
-- the last scan, as provider-neutral JSON. Enrichment merges them again under and above the
-- provider record (ADR 0031 §8: user, nfo, provider, filename), and "Remove match" falls
-- back to them. An item without a row has never been scanned since this migration; its
-- stored record is then local data only and stands in for the file name source.

-- +goose Up
CREATE TABLE metadata_settings (
    id           INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    online       INTEGER NOT NULL CHECK (online IN (0, 1)),
    tmdb         INTEGER NOT NULL CHECK (tmdb IN (0, 1)),
    musicbrainz  INTEGER NOT NULL CHECK (musicbrainz IN (0, 1)),
    language     TEXT    NOT NULL,
    tmdb_api_key TEXT    NOT NULL,
    updated_at   INTEGER NOT NULL
) STRICT;

CREATE TABLE item_local_records (
    item_id    TEXT    NOT NULL PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    nfo        TEXT    NOT NULL CHECK (json_valid(nfo)),
    filename   TEXT    NOT NULL CHECK (json_valid(filename)),
    updated_at INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE item_local_records;
DROP TABLE metadata_settings;
