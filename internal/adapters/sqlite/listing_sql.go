package sqlite

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// The library listing (sort, filters, facets, letters; task M5-11) is built as SQL text at run
// time, because sqlc cannot express a WHERE clause whose parts depend on the request. Every
// value is a named parameter (sqlBuilder.arg); only the fixed fragments of this file and the
// numbers of library.ResolutionThresholds become SQL text, so no client text reaches it.
//
// Every listing query starts with the same common table expressions:
//
//   - members(top_id, item_id): the items whose files decide the technical filters of a
//     top-level item. A movie (or any item of a library without a top-level kind) decides
//     for itself; a show is decided by its episodes ("a show matches when one of its
//     episodes matches", API ItemQuickFilter).
//   - decider(top_id, file_id): the primary file of each member, the first by part number,
//     folder position and relative path, as ListPrimaryFiles picks it.
//   - facts: per decider file its container, probe state, folder and primary video stream
//     (the first default, else the first, non-picture video stream, as
//     media.Description.PrimaryVideo picks it).
//
// SQLite computes a common table expression only when the statement uses it, so a listing
// without technical filters, sorts or facets never touches the files.

// sqlBuilder collects SQL text and its named parameters.
type sqlBuilder struct {
	text strings.Builder
	args []any
}

// arg binds value to a fresh named parameter and returns its placeholder.
func (builder *sqlBuilder) arg(value any) string {
	name := "p" + strconv.Itoa(len(builder.args)+1)
	builder.args = append(builder.args, sql.Named(name, value))

	return ":" + name
}

// list binds every value and returns the placeholders separated by commas.
func list[T any](builder *sqlBuilder, values []T) string {
	placeholders := make([]string, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, builder.arg(value))
	}

	return strings.Join(placeholders, ", ")
}

func (builder *sqlBuilder) write(parts ...string) {
	for _, part := range parts {
		builder.text.WriteString(part)
	}
}

// listingTarget is the library a listing reads: its ID, the top-level kind of the filter
// (empty for libraries without one) and whether it has more than one folder.
type listingTarget struct {
	libraryID   string
	kind        library.ItemKind
	multiFolder bool
}

// listingScope is what every listing query of one request shares: the parameters of the
// library and kind and the base condition on the top-level items (alias "items").
type listingScope struct {
	builder     *sqlBuilder
	base        string
	members     memberPath
	kind        library.ItemKind
	multiFolder bool
}

// memberPath says which items decide the technical filters of a listed item: the item itself
// (depth 0), its children (1) or its grandchildren (2) of memberKind.
type memberPath struct {
	depth      int
	memberKind library.ItemKind
}

// Depths of memberPath.
const (
	memberSelf       = 0
	memberChild      = 1
	memberGrandchild = 2
)

// memberPathOf returns the deciding items of a kind: a show by its episodes, an artist by the
// tracks of its albums, an album by its tracks, everything else by itself.
func memberPathOf(kind library.ItemKind) memberPath {
	containers := map[library.ItemKind]memberPath{
		library.ItemKindShow:   {depth: memberGrandchild, memberKind: library.ItemKindEpisode},
		library.ItemKindArtist: {depth: memberGrandchild, memberKind: library.ItemKindTrack},
		library.ItemKindAlbum:  {depth: memberChild, memberKind: library.ItemKindTrack},
	}

	if path, found := containers[kind]; found {
		return path
	}

	return memberPath{depth: memberSelf}
}

// newListingScope binds the library. The base condition repeats the WHERE clause of the
// partial index items_top_level (items_albums for albums, which lie below their artist), so
// SQLite can use it as a covering index. Otherwise the kind adds no condition: the use case
// only passes a top-level kind of the library, and every top-level item of a movies, shows or
// audiobooks library has that kind (a kind test would cost a row lookup per item).
func newListingScope(builder *sqlBuilder, target listingTarget) listingScope {
	libraryParameter := builder.arg(target.libraryID)

	base := "items.library_id = " + libraryParameter + " AND items.parent_id IS NULL AND items.kind <> 'extra'"
	if target.kind == library.ItemKindAlbum {
		base = "items.library_id = " + libraryParameter + " AND items.kind = 'album'"
	}

	return listingScope{builder: builder, base: base, members: memberPathOf(target.kind), kind: target.kind, multiFolder: target.multiFolder}
}

// membersSQL selects (top_id, item_id) of the deciding items of the listed items.
func (scope listingScope) membersSQL() string {
	kind := "'" + scope.members.memberKind.String() + "'"

	switch scope.members.depth {
	case memberGrandchild:
		return "SELECT items.id AS top_id, members.id AS item_id FROM items" +
			" JOIN items AS middles ON middles.parent_id = items.id" +
			" JOIN items AS members ON members.parent_id = middles.id" +
			" WHERE " + scope.base + " AND members.kind = " + kind
	case memberChild:
		return "SELECT items.id AS top_id, members.id AS item_id FROM items" +
			" JOIN items AS members ON members.parent_id = items.id" +
			" WHERE " + scope.base + " AND members.kind = " + kind
	default:
		return "SELECT items.id AS top_id, items.id AS item_id FROM items WHERE " + scope.base
	}
}

// primaryFileSQL selects the primary file of the item whose ID column is itemColumn, in the
// order of ListPrimaryFiles. In a library with one folder every file has the same folder
// position, so the order needs no join and SQLite reads the first entry of media_files_item;
// the join costs more than half of a 50,000-item facet count (task M5-11 benchmark).
func (scope listingScope) primaryFileSQL(itemColumn string) string {
	if !scope.multiFolder {
		return "(SELECT candidates.id FROM media_files AS candidates WHERE candidates.item_id = " + itemColumn +
			" ORDER BY candidates.part_number, candidates.relative_path, candidates.id LIMIT 1)"
	}

	return "(SELECT candidates.id FROM media_files AS candidates" +
		" JOIN library_roots AS candidate_roots ON candidate_roots.id = candidates.root_id" +
		" WHERE candidates.item_id = " + itemColumn +
		" ORDER BY candidates.part_number, candidate_roots.position, candidates.relative_path, candidates.id LIMIT 1)"
}

// primaryVideoSQL selects the stream index of the primary video stream of the file whose ID
// column is fileColumn.
func primaryVideoSQL(fileColumn string) string {
	// Two MIN lookups on the primary key instead of ORDER BY is_default, which would sort
	// the streams of every file.
	videos := "SELECT MIN(videos.stream_index) FROM media_streams AS videos WHERE videos.media_file_id = " + fileColumn +
		" AND videos.kind = 'video' AND videos.is_attached_picture = 0"

	return "COALESCE((" + videos + " AND videos.is_default = 1), (" + videos + "))"
}

// tableNeeds says which common tables a statement reads. A table needs the ones it is built
// on: facts needs decider, decider needs members.
type tableNeeds struct {
	members bool
	decider bool
	facts   bool
}

// union returns the tables either needs.
func (needs tableNeeds) union(other tableNeeds) tableNeeds {
	return tableNeeds{
		members: needs.members || other.members,
		decider: needs.decider || other.decider,
		facts:   needs.facts || other.facts,
	}
}

// commonTables returns the definitions of the needed tables for a WITH clause. SQLite
// computes a MATERIALIZED table even when the statement does not read it, so only the needed
// ones are written. MATERIALIZED: the facts and every stream condition read decider, and the
// facet arms read facts several times; computing them once keeps a facet count fast.
func (scope listingScope) commonTables(needs tableNeeds) []string {
	var tables []string

	if needs.members {
		tables = append(tables, "members AS ("+scope.membersSQL()+")")
	}

	if needs.decider {
		// file_id is NULL for a member without a file; every reader joins on it, which drops those.
		tables = append(tables, "decider AS MATERIALIZED (SELECT members.top_id, "+
			scope.primaryFileSQL("members.item_id")+" AS file_id FROM members)")
	}

	if needs.facts {
		tables = append(tables, "facts AS MATERIALIZED (SELECT decider.top_id, files.container, files.probe_error, files.root_id,"+
			" video.codec AS video_codec, video.width, video.height, video.hdr"+
			" FROM decider JOIN media_files AS files ON files.id = decider.file_id"+
			" LEFT JOIN media_streams AS video ON video.media_file_id = files.id AND video.stream_index = "+
			primaryVideoSQL("files.id")+")")
	}

	return tables
}

// writeWith writes the WITH clause of the tables; nothing for none.
func (builder *sqlBuilder) writeWith(tables []string) {
	if len(tables) > 0 {
		builder.write("WITH ", strings.Join(tables, ",\n"), "\n")
	}
}

// resolutionSQL classifies the facts' primary video like library.ResolutionOf; NULL without a
// video stream.
func resolutionSQL() string {
	var text strings.Builder

	text.WriteString("CASE WHEN video_codec IS NULL THEN NULL")

	for _, threshold := range library.ResolutionThresholds() {
		fmt.Fprintf(&text, " WHEN width >= %d OR height >= %d THEN '%s'",
			threshold.MinWidth, threshold.MinHeight, threshold.Resolution)
	}

	fmt.Fprintf(&text, " ELSE '%s' END", library.ResolutionSD)

	return text.String()
}

// resolutionRankSQL orders resolution values lowest first.
func resolutionRankSQL(column string) string {
	var text strings.Builder

	text.WriteString("CASE " + column)

	for rank, resolution := range library.Resolutions() {
		fmt.Fprintf(&text, " WHEN '%s' THEN %d", resolution, rank)
	}

	text.WriteString(" END")

	return text.String()
}
