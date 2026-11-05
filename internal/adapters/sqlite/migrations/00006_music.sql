-- Music libraries (M3-13, ADR 0027 §3). Since 00003 the schema already has what music needs
-- in its tables: items.disc_number (NULL outside music, >= 1 for a track), the item kinds
-- artist, album and track, and an unconstrained artwork kind, so the square cover kind needs
-- no CHECK change. This migration therefore adds no column and rebuilds no table; it only
-- adds the index of the library-wide album listing.
--
-- MusicBrainz IDs from the audio tags live in item_provider_ids under the providers
-- musicbrainz_artist, musicbrainz_album and musicbrainz_track. They are owned by the music
-- writer: the metadata writer replaces only the other providers of an item.

-- +goose Up
-- Keyset pagination of a library's albums by (sort_title, id). Albums are below an artist, so
-- items_top_level does not cover them.
CREATE INDEX items_albums ON items (library_id, sort_title, id)
    WHERE kind = 'album';

-- +goose Down
DROP INDEX items_albums;
