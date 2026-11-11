package sqlite

import (
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// noValue is the sort value of a container without the property (a show has no duration,
// resolution or bit rate, API ItemSort).
const noValue = "NULL"

// sortValue is the SQL of a sort key over the top-level items (alias "items") and whether it
// can be NULL; NULL values come last in both directions.
type sortValue struct {
	sql      string
	nullable bool
}

// sortValueSQL returns the value a listing sorts by. Title sorts by sort_title itself (see
// listingQuery), the others by the value with sort title and ID as tie breakers.
func (scope listingScope) sortValueSQL(order library.ListingOrder) sortValue {
	switch order.Key() {
	case library.SortByTitle:
		return sortValue{sql: "items.sort_title"}
	case library.SortByYear:
		return sortValue{sql: "NULLIF(items.year, 0)", nullable: true}
	case library.SortByReleaseDate:
		return sortValue{sql: "NULLIF(items.release_date, '')", nullable: true}
	case library.SortByContentRating:
		return sortValue{sql: "NULLIF(items.content_rating, '')", nullable: true}
	case library.SortByDateAdded:
		return sortValue{sql: "items.created_at"}
	case library.SortByRandom:
		return sortValue{sql: shuffleSQL(scope.builder.arg(order.Seed))}
	case library.SortByDuration, library.SortByResolution, library.SortByBitrate, library.SortByFileSize:
		return scope.fileSortValueSQL(order.Key())
	default:
		// Unreachable: the HTTP adapter accepts only the ItemSort values of the contract and
		// the zero value is SortByTitle (ListingOrder.Key). A key added later sorts by title
		// until it gets its own case, rather than failing the listing.
		return sortValue{sql: "items.sort_title"}
	}
}

// fileSortValueSQL returns the values of the primary file: duration, bit rate and resolution
// (none for a container: show, artist, album) and the file size (for a container the sum of
// its members' files: the episodes of a show, the tracks of an artist or album).
func (scope listingScope) fileSortValueSQL(key library.ItemSort) sortValue {
	primary := scope.primaryFileSQL("items.id")
	container := scope.members.depth != memberSelf

	switch {
	case key == library.SortByFileSize && container:
		return sortValue{sql: scope.memberSizeSQL(), nullable: true}
	case key == library.SortByFileSize:
		return sortValue{sql: "(SELECT files.size_bytes FROM media_files AS files WHERE files.id = " + primary + ")", nullable: true}
	case container:
		return sortValue{sql: noValue, nullable: true}
	case key == library.SortByDuration && scope.kind == library.ItemKindAudiobook:
		// An audiobook's duration is the sum of its parts (API 1.8.0 MediaSummary).
		return sortValue{sql: "(SELECT NULLIF(SUM(files.duration_ms), 0) FROM media_files AS files WHERE files.item_id = items.id)", nullable: true}
	case key == library.SortByDuration:
		return sortValue{sql: "(SELECT NULLIF(files.duration_ms, 0) FROM media_files AS files WHERE files.id = " + primary + ")", nullable: true}
	case key == library.SortByBitrate:
		return sortValue{sql: "(SELECT NULLIF(files.bit_rate, 0) FROM media_files AS files WHERE files.id = " + primary + ")", nullable: true}
	case key == library.SortByResolution:
		return sortValue{sql: "(SELECT NULLIF(video.width * video.height, 0) FROM media_streams AS video" +
			" WHERE video.media_file_id = " + primary + " AND video.stream_index = " + primaryVideoSQL(primary) + ")", nullable: true}
	default:
		return sortValue{sql: "items.sort_title"}
	}
}

// memberSizeSQL sums the files of the members of a container item the viewer may see,
// correlated with the listed item, so each lookup searches items_children. A hidden episode
// must not move its show in the size order (ADR 0032 section 7).
func (scope listingScope) memberSizeSQL() string {
	kind := "'" + scope.members.memberKind.String() + "'"

	if scope.members.depth == memberGrandchild {
		return "(SELECT SUM(files.size_bytes) FROM items AS middles" +
			" JOIN items AS members ON members.parent_id = middles.id" +
			" JOIN media_files AS files ON files.item_id = members.id" +
			" WHERE middles.parent_id = items.id AND members.kind = " + kind + scope.visibleMemberSQL() + ")"
	}

	return "(SELECT SUM(files.size_bytes) FROM items AS members" +
		" JOIN media_files AS files ON files.item_id = members.id" +
		" WHERE members.parent_id = items.id AND members.kind = " + kind + scope.visibleMemberSQL() + ")"
}

// Constants of the shuffle hash: a multiplicative hash of the row ID plus the seed, then the
// xor-shift-multiply finaliser of a well-known 32-bit integer hash. Every intermediate stays
// below 2^63, because SQLite turns an overflowing integer product into a float.
const (
	shuffleMask       = "4294967295" // 2^32 - 1
	shuffleRowMask    = "2147483647" // 2^31 - 1
	shuffleMultiplier = "2654435761" // Knuth's multiplicative hash constant
	shuffleFinaliser  = "73244475"   // 0x45d9f3b
	shuffleShift      = "16"
)

// shuffleSQL returns a stable pseudo-random order key of an item for the seed placeholder:
// the same seed gives the same order, another seed another one. It hashes the row ID, which
// stays the same while the item exists (a VACUUM may renumber it, which only reshuffles).
func shuffleSQL(seed string) string {
	start := "(((items.rowid & " + shuffleRowMask + ") * " + shuffleMultiplier + " + " + seed + ") & " + shuffleMask + ")"
	mixed := "((" + xorShiftSQL(start) + " * " + shuffleFinaliser + ") & " + shuffleMask + ")"

	return xorShiftSQL(mixed)
}

// xorShiftSQL returns value XOR (value >> 16); SQLite has no XOR operator, and a XOR b is
// (a | b) - (a & b).
func xorShiftSQL(value string) string {
	shifted := "(" + value + " >> " + shuffleShift + ")"

	return "((" + value + " | " + shifted + ") - (" + value + " & " + shifted + "))"
}

// orderBySQL returns the ORDER BY terms over the columns of a listed row (sort_value,
// sort_title, id).
func orderBySQL(value sortValue, titleOnly, descending bool) string {
	direction := ""
	if descending {
		direction = " DESC"
	}

	terms := []string{"sort_title" + direction, "id" + direction}
	if !titleOnly {
		terms = append([]string{"sort_value" + direction}, terms...)
	}

	if value.nullable {
		terms = append([]string{"sort_value IS NULL"}, terms...)
	}

	return strings.Join(terms, ", ")
}
