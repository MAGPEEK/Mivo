package sqlite

import (
	"time"

	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The rows (task M5-13) reuse the listing's SQL building blocks (listing_sql.go): a row of
// top-level items is a listing page with an availability condition, without a cursor and
// without the items that lack the sort value.
//
// Availability is a correlated EXISTS on the primary file of the item or of one of its
// members, not the facts table of the listing: a row reads only its first items, so SQLite
// can stop at the first available member of each candidate instead of computing the primary
// file of every member of the library first.

// availableFileSQL is the condition that the file (alias "files") is probed and lies in a
// folder that can be reached.
func (scope listingScope) availableFileSQL(offlineRootIDs []string) string {
	return "files.probe_error IS NULL AND files.root_id NOT IN (" + list(scope.builder, offlineRootIDs) + ")"
}

// availableSQL is the condition that a top-level item (alias "items") is available: its own
// primary file, or for a container the primary file of one of its members the viewer may see
// (a show is available while one of its visible episodes is).
func (scope listingScope) availableSQL(offlineRootIDs []string) string {
	available := scope.availableFileSQL(offlineRootIDs)
	kind := "'" + scope.members.memberKind.String() + "'"

	if scope.kind == library.ItemKindPhotoAlbum {
		return scope.availablePhotoSQL(available)
	}

	switch scope.members.depth {
	case memberGrandchild:
		// The middle level (the season of an episode, the album of a track) needs no kind
		// test: members.kind already names the leaf, and only that kind sits two levels down.
		return "EXISTS (SELECT 1 FROM items AS middles JOIN items AS members ON members.parent_id = middles.id" +
			" JOIN media_files AS files ON files.id = " + scope.primaryFileSQL("members.id") +
			" WHERE middles.parent_id = items.id AND members.kind = " + kind + scope.visibleMemberSQL() + " AND " + available + ")"
	case memberChild:
		return "EXISTS (SELECT 1 FROM items AS members JOIN media_files AS files ON files.id = " + scope.primaryFileSQL("members.id") +
			" WHERE members.parent_id = items.id AND members.kind = " + kind + scope.visibleMemberSQL() + " AND " + available + ")"
	default:
		return "EXISTS (SELECT 1 FROM media_files AS files WHERE files.id = " + scope.primaryFileSQL("items.id") + " AND " + available + ")"
	}
}

// availablePhotoSQL is the availability of a top-level item of a photos library: a photo by
// its own primary file, a photo album by one photo anywhere below it. Albums own no files
// and nest, so the photos are found through a recursive walk down from the item (which is
// its own first descendant, so a loose photo at the top decides for itself).
func (scope listingScope) availablePhotoSQL(available string) string {
	return "EXISTS (WITH RECURSIVE descendants(id) AS (SELECT items.id" +
		" UNION ALL SELECT children.id FROM items AS children JOIN descendants ON children.parent_id = descendants.id)" +
		" SELECT 1 FROM descendants JOIN items AS members ON members.id = descendants.id" +
		" JOIN media_files AS files ON files.id = " + scope.primaryFileSQL("members.id") +
		" WHERE members.kind = '" + library.ItemKindPhoto.String() + "'" + scope.visibleMemberSQL() + " AND " + available + ")"
}

// rowItemsQuery returns the SQL of the IDs of a row of top-level items: available, with a
// value for the sort key, in the order of the query with sort title and ID as tie breakers
// in the same direction (as orderBySQL, so a row starts like its "See all" page), at most
// limit of them.
func rowItemsQuery(target listingTarget, request approws.ItemRequest) (string, []any) {
	builder := &sqlBuilder{}
	scope := newListingScope(builder, target)
	order := request.Query.Order
	value := scope.sortValueSQL(order)

	conditions := scope.filterConditions(request.Query.Filter)
	builder.writeWith(scope.commonTables(needsOf(conditions)))
	builder.write("SELECT items.id FROM items WHERE ", joinConditions(scope.base, conditions),
		" AND ", scope.availableSQL(request.Query.Filter.OfflineRootIDs))

	if value.nullable {
		builder.write(" AND ", value.sql, " IS NOT NULL")
	}

	direction := ""
	if order.IsDescending() {
		direction = " DESC"
	}

	builder.write(" ORDER BY ", value.sql, direction, ", items.sort_title", direction, ", items.id", direction,
		" LIMIT ", builder.arg(request.Limit))

	return builder.text.String(), builder.args
}

// availableEpisodeSQL is the condition that the episode (alias "episodes") has an available
// primary file.
func (scope listingScope) availableEpisodeSQL(offlineRootIDs []string) string {
	return "EXISTS (SELECT 1 FROM media_files AS files WHERE files.id = " + scope.primaryFileSQL("episodes.id") +
		" AND " + scope.availableFileSQL(offlineRootIDs) + ")"
}

// episodeScope is the scope of the episode queries: they need only the builder and the
// folder count of primaryFileSQL, not the top-level base condition.
func episodeScope(builder *sqlBuilder, target listingTarget) listingScope {
	return listingScope{builder: builder, multiFolder: target.multiFolder}
}

// libraryEpisodesSQL selects the available episodes of the target's library that the viewer
// may see, with their season and show; it binds the library first. An episode's stored
// visibility covers its season and show (ADR 0032 section 7), so no episode of a hidden show
// reaches a row, and with it neither the show's ID.
func libraryEpisodesSQL(scope listingScope, target listingTarget, offlineRootIDs []string) string {
	return "SELECT episodes.id AS id, episodes.created_at AS created_at, episodes.release_date AS release_date," +
		" seasons.id AS season_id, seasons.parent_id AS show_id" +
		" FROM items AS episodes JOIN items AS seasons ON seasons.id = episodes.parent_id" +
		" WHERE episodes.library_id = " + scope.builder.arg(target.libraryID) + " AND episodes.kind = '" + library.ItemKindEpisode.String() + "'" +
		" AND seasons.kind = '" + library.ItemKindSeason.String() + "'" + visibleInLibrarySQL(scope.builder, target, "episodes") +
		" AND " + scope.availableEpisodeSQL(offlineRootIDs)
}

// episodeGroupsQuery returns the SQL of the recently added episodes of the shows with the
// newest episodes, per season: (show_id, season_id, episode count, newest created_at). An
// episode counts when it was added within the window of its show's newest episode.
func episodeGroupsQuery(target listingTarget, request approws.EpisodeRequest) (string, []any) {
	builder := &sqlBuilder{}
	scope := episodeScope(builder, target)

	builder.write("WITH available AS (", libraryEpisodesSQL(scope, target, request.OfflineRootIDs), "),\n",
		"newest AS (SELECT show_id, MAX(created_at) AS newest FROM available GROUP BY show_id",
		" ORDER BY newest DESC, show_id LIMIT ", builder.arg(request.Limit), ")\n",
		"SELECT newest.show_id, available.season_id, COUNT(*), MAX(available.created_at)",
		" FROM available JOIN newest ON newest.show_id = available.show_id",
		" WHERE available.created_at >= newest.newest - ", builder.arg(request.Window.Milliseconds()),
		" GROUP BY newest.show_id, available.season_id")

	return builder.text.String(), builder.args
}

// recentEpisodesQuery returns the SQL of the available episodes with an air date, newest
// first (ties by date added, newest first, then ID): (id, season_id, show_id).
func recentEpisodesQuery(target listingTarget, request approws.EpisodeRequest) (string, []any) {
	builder := &sqlBuilder{}
	scope := episodeScope(builder, target)

	builder.write("SELECT id, season_id, show_id FROM (", libraryEpisodesSQL(scope, target, request.OfflineRootIDs), ")",
		" WHERE release_date <> '' ORDER BY release_date DESC, created_at DESC, id DESC LIMIT ", builder.arg(request.Limit))

	return builder.text.String(), builder.args
}

// timeOfMillis converts a stored created_at.
func timeOfMillis(millis int64) time.Time {
	return time.UnixMilli(millis).UTC()
}
