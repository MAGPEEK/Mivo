-- Full-text search index (task M5-14, SR-03): accent- and case-insensitive prefix search over
-- titles, original titles, sort titles, people and genres. Additive: no existing table changes.
--
-- item_search is a contentful FTS5 table. Its rowid cannot be items.rowid: items has a TEXT
-- primary key, and VACUUM may renumber the rowids of such a table. search_docs therefore
-- assigns each indexed item a stable document number (doc_id, an INTEGER PRIMARY KEY that
-- VACUUM keeps) and is the only link between an item and its FTS row. It also copies the
-- item's library_id and kind, so a search filters and returns hits without joining items for
-- every match: on 50,000 matches that join costs as much as the ranking. The copies cannot go
-- stale, because no statement changes items.library_id or items.kind (see 00009).
--
-- Triggers keep the index in sync with every writer, so no Go code has to remember it:
--   - a new item of an indexed kind gets a search_docs row, whose insert trigger writes the
--     FTS row from search_doc_text;
--   - deleting an item cascades to search_docs, whose delete trigger removes the FTS row
--     (foreign key actions fire triggers, so a deleted library is covered too);
--   - changing a title, the people or the genres of an item, or renaming a person or a
--     genre, rewrites the item's FTS row from search_doc_text.
-- The delete triggers of item_people and item_tags skip rows whose item is gone: deleting an
-- item or a library cascades to those rows after the item row, and rewriting the FTS row of
-- an item that is being deleted for each of its credits and genres would multiply the cost
-- of a library deletion. The title trigger skips updates that keep every indexed title.
--
-- Indexed kinds are those the search API returns (M5-15): movies, shows, episodes, artists,
-- albums, tracks, audiobooks, photo albums and photos. Seasons ("Season 1") and extras are no
-- search results. An item never changes its kind, so the kind is checked on insert only.
--
-- people and genres are the names joined by commas; the tokenizer treats a comma as a
-- separator. Genres are the tags of kind 'genre'; studios, countries and collections are not
-- searched. remove_diacritics 2 folds every diacritic ("cafe" finds "Café"); the prefix
-- indexes make the short prefixes of search-as-you-type cheap.

-- +goose Up
CREATE TABLE search_docs (
    doc_id     INTEGER PRIMARY KEY,
    item_id    TEXT    NOT NULL UNIQUE REFERENCES items (id) ON DELETE CASCADE,
    library_id TEXT    NOT NULL,
    kind       TEXT    NOT NULL
) STRICT;

CREATE VIRTUAL TABLE item_search USING fts5(
    title, original_title, sort_title, people, genres,
    tokenize = 'unicode61 remove_diacritics 2',
    prefix = '2 3'
);

-- The text of an indexed item's FTS row; the single definition every trigger writes from.
CREATE VIEW search_doc_text AS
SELECT search_docs.doc_id,
       items.title,
       items.original_title,
       items.sort_title,
       ifnull((SELECT group_concat(DISTINCT people.name)
               FROM item_people
               JOIN people ON people.id = item_people.person_id
               WHERE item_people.item_id = search_docs.item_id), '') AS people,
       ifnull((SELECT group_concat(DISTINCT tags.name)
               FROM item_tags
               JOIN tags ON tags.id = item_tags.tag_id
               WHERE item_tags.item_id = search_docs.item_id AND tags.kind = 'genre'), '') AS genres
FROM search_docs
JOIN items ON items.id = search_docs.item_id;

-- +goose StatementBegin
CREATE TRIGGER search_docs_insert AFTER INSERT ON search_docs
BEGIN
    INSERT INTO item_search (rowid, title, original_title, sort_title, people, genres)
    SELECT doc_id, title, original_title, sort_title, people, genres
    FROM search_doc_text
    WHERE doc_id = new.doc_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER search_docs_delete AFTER DELETE ON search_docs
BEGIN
    DELETE FROM item_search WHERE rowid = old.doc_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER items_search_insert AFTER INSERT ON items
WHEN new.kind IN ('movie', 'show', 'episode', 'artist', 'album', 'track', 'audiobook',
                  'photo_album', 'photo')
BEGIN
    INSERT INTO search_docs (item_id, library_id, kind) VALUES (new.id, new.library_id, new.kind);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER items_search_update AFTER UPDATE OF title, original_title, sort_title ON items
WHEN new.title IS NOT old.title OR new.original_title IS NOT old.original_title
     OR new.sort_title IS NOT old.sort_title
BEGIN
    UPDATE item_search
    SET (title, original_title, sort_title, people, genres) =
        (SELECT title, original_title, sort_title, people, genres
         FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid = (SELECT doc_id FROM search_docs WHERE item_id = new.id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_people_search_insert AFTER INSERT ON item_people
BEGIN
    UPDATE item_search
    SET people = (SELECT people FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid = (SELECT doc_id FROM search_docs WHERE item_id = new.item_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_people_search_delete AFTER DELETE ON item_people
WHEN EXISTS (SELECT 1 FROM items WHERE id = old.item_id)
BEGIN
    UPDATE item_search
    SET people = (SELECT people FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid = (SELECT doc_id FROM search_docs WHERE item_id = old.item_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_people_search_update AFTER UPDATE OF item_id, person_id ON item_people
BEGIN
    UPDATE item_search
    SET people = (SELECT people FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid IN (SELECT doc_id FROM search_docs WHERE item_id IN (old.item_id, new.item_id));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER people_search_rename AFTER UPDATE OF name ON people
BEGIN
    UPDATE item_search
    SET people = (SELECT people FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid IN (SELECT search_docs.doc_id
                    FROM item_people
                    JOIN search_docs ON search_docs.item_id = item_people.item_id
                    WHERE item_people.person_id = new.id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_tags_search_insert AFTER INSERT ON item_tags
BEGIN
    UPDATE item_search
    SET genres = (SELECT genres FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid = (SELECT doc_id FROM search_docs WHERE item_id = new.item_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_tags_search_delete AFTER DELETE ON item_tags
WHEN EXISTS (SELECT 1 FROM items WHERE id = old.item_id)
BEGIN
    UPDATE item_search
    SET genres = (SELECT genres FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid = (SELECT doc_id FROM search_docs WHERE item_id = old.item_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER item_tags_search_update AFTER UPDATE OF item_id, tag_id ON item_tags
BEGIN
    UPDATE item_search
    SET genres = (SELECT genres FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid IN (SELECT doc_id FROM search_docs WHERE item_id IN (old.item_id, new.item_id));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER tags_search_rename AFTER UPDATE OF kind, name ON tags
BEGIN
    UPDATE item_search
    SET genres = (SELECT genres FROM search_doc_text WHERE doc_id = item_search.rowid)
    WHERE rowid IN (SELECT search_docs.doc_id
                    FROM item_tags
                    JOIN search_docs ON search_docs.item_id = item_tags.item_id
                    WHERE item_tags.tag_id = new.id);
END;
-- +goose StatementEnd

-- Back-fill: the insert trigger of search_docs indexes every existing item.
INSERT INTO search_docs (item_id, library_id, kind)
SELECT id, library_id, kind
FROM items
WHERE kind IN ('movie', 'show', 'episode', 'artist', 'album', 'track', 'audiobook',
               'photo_album', 'photo');

-- +goose Down
DROP TRIGGER tags_search_rename;
DROP TRIGGER item_tags_search_update;
DROP TRIGGER item_tags_search_delete;
DROP TRIGGER item_tags_search_insert;
DROP TRIGGER people_search_rename;
DROP TRIGGER item_people_search_update;
DROP TRIGGER item_people_search_delete;
DROP TRIGGER item_people_search_insert;
DROP TRIGGER items_search_update;
DROP TRIGGER items_search_insert;
DROP TRIGGER search_docs_delete;
DROP TRIGGER search_docs_insert;
DROP VIEW search_doc_text;
DROP TABLE item_search;
DROP TABLE search_docs;
