package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

// visibilityMatrixKinds are all item kinds: "hide unrated" treats the video kinds differently.
func visibilityMatrixKinds() []string {
	return []string{
		"movie", "extra", "show", "season", "episode", "artist", "album", "track", "audiobook", "photo_album", "photo",
	}
}

// seedVisibilityMatrix stores one item per kind, stored bucket (NULL and every bucket) and
// unrated flag, and returns their IDs.
func seedVisibilityMatrix(t *testing.T, db *DB) []string {
	t.Helper()

	execute := func(query string, args ...any) {
		t.Helper()

		if _, err := db.writer.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	execute(`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('library', 'Library', 'movies', 0, 0)`)

	buckets := []sql.NullInt64{{}, {Valid: true}, {Int64: 6, Valid: true}, {Int64: 12, Valid: true}, {Int64: 16, Valid: true}, {Int64: 18, Valid: true}}

	var ids []string

	for _, kind := range visibilityMatrixKinds() {
		extraType := sql.NullString{String: "trailer", Valid: kind == "extra"}

		for _, bucket := range buckets {
			for unrated := range 2 {
				id := fmt.Sprintf("%s-%d-%t-%d", kind, bucket.Int64, bucket.Valid, unrated)
				ids = append(ids, id)

				execute(`INSERT INTO items (id, library_id, kind, match_key, title, sort_title, extra_type, created_at, updated_at)
					VALUES (?1, 'library', ?2, ?1, ?1, ?1, ?3, 0, 0)`, id, kind, extraType)
				// The insert trigger computed the pair of an unrated top-level item; the matrix
				// overwrites it.
				execute(`UPDATE items SET visibility_age_rating = ?, visibility_unrated = ? WHERE id = ?`, bucket, unrated, id)
			}
		}
	}

	return ids
}

// matrixRestrictions are the content limits a profile can have, on all libraries.
func matrixRestrictions(t *testing.T) map[string]access.Restriction {
	t.Helper()

	restrictions := map[string]access.Restriction{
		"no limit":            access.Unrestricted(),
		"only unrated hidden": access.Viewer{Libraries: access.AllLibraries(), Limit: access.NoLimit().HidingUnrated(true)}.Restriction(),
	}

	for _, maxAge := range []access.AgeBucket{access.AgeAll, access.Age6, access.Age12, access.Age16, access.Age18} {
		for _, hide := range []bool{false, true} {
			restrictions[fmt.Sprintf("up to %d, hide unrated %t", maxAge, hide)] = restrictedTo(t, access.AllLibraries(), maxAge, hide)
		}
	}

	return restrictions
}

// sharedPredicateIDs returns the sorted IDs of the items restrictionConditionSQL lets pass.
func sharedPredicateIDs(t *testing.T, db *DB, restriction access.Restriction) []string {
	t.Helper()

	builder := &sqlBuilder{}
	builder.write("SELECT items.id FROM items WHERE 1 = 1", andSQL(restrictionConditionSQL(builder, restriction, "items")),
		" ORDER BY items.id")

	rows, err := db.reader.QueryContext(t.Context(), builder.text.String(), builder.args...)
	if err != nil {
		t.Fatalf("shared predicate: %v", err)
	}

	defer closeRows(rows)

	var ids []string

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("shared predicate: scan: %v", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("shared predicate: %v", err)
	}

	return ids
}

// boundPredicateIDs returns the sorted IDs of the given items the generated GetVisibleItem
// lets pass.
func boundPredicateIDs(t *testing.T, db *DB, restriction access.Restriction, ids []string) []string {
	t.Helper()

	var (
		visible []string
		view    = viewOf(restriction)
		queries = sqlcgen.New(db.reader)
	)

	for _, id := range ids {
		_, err := view.item(t.Context(), queries, id)
		if err == nil {
			visible = append(visible, id)
		} else if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("bound predicate: %v", err)
		}
	}

	slices.Sort(visible)

	return visible
}

// The generated queries repeat the content limit in bound form. This holds it equal to the
// shared predicate (ageConditionSQL) for every kind, stored visibility and limit, so the two
// forms cannot drift apart.
func TestGeneratedVisibilityMatchesTheSharedPredicate(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	ids := seedVisibilityMatrix(t, db)

	for name, restriction := range matrixRestrictions(t) {
		shared, bound := sharedPredicateIDs(t, db, restriction), boundPredicateIDs(t, db, restriction, ids)

		if !slices.Equal(shared, bound) {
			t.Errorf("%s: the shared predicate shows %d items, the generated queries %d:\n%v\n%v", name, len(shared), len(bound), shared, bound)
		}

		if name == "no limit" && len(bound) != len(ids) {
			t.Errorf("no limit shows %d of %d items", len(bound), len(ids))
		}
	}
}

// "Hide unrated" hides the video kinds only; a maximum age hides every kind.
func TestHideUnratedAppliesToVideoKindsOnly(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	seedVisibilityMatrix(t, db)

	queries := sqlcgen.New(db.reader)
	hiding := viewOf(matrixRestrictions(t)["only unrated hidden"])
	upToSix := viewOf(restrictedTo(t, access.AllLibraries(), access.Age6, true))
	video := []string{"movie", "extra", "show", "season", "episode"}

	for _, kind := range visibilityMatrixKinds() {
		_, unratedErr := hiding.item(t.Context(), queries, kind+"-0-false-1")
		if hidden := unratedErr != nil; hidden != slices.Contains(video, kind) {
			t.Errorf("unrated %s hidden = %t, want %t", kind, hidden, !hidden)
		}

		if _, err := upToSix.item(t.Context(), queries, kind+"-16-true-0"); err == nil {
			t.Errorf("an FSK 16 %s is visible up to 6", kind)
		}

		// The uncomputed state (18, 1) is closed for every kind under a maximum age.
		if _, err := upToSix.item(t.Context(), queries, kind+"-18-true-1"); err == nil {
			t.Errorf("an uncomputed %s is visible up to 6", kind)
		}
	}
}

func TestNullableBucketRejectsUnexpectedTypes(t *testing.T) {
	t.Parallel()

	if bucket, err := nullableBucket(nil); err != nil || bucket.Valid {
		t.Errorf("nullableBucket(nil) = %+v, %v, want NULL", bucket, err)
	}

	if bucket, err := nullableBucket(int64(12)); err != nil || bucket != (sql.NullInt64{Int64: 12, Valid: true}) {
		t.Errorf("nullableBucket(12) = %+v, %v, want 12", bucket, err)
	}

	// A driver that returned text must not turn a rated chain into "nothing rated".
	for _, value := range []any{"12", []byte("12"), 12.0, 12} {
		if bucket, err := nullableBucket(value); err == nil {
			t.Errorf("nullableBucket(%T) = %+v, want an error", value, bucket)
		}
	}
}

func TestContentLimitOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		restriction access.Restriction
		want        contentLimit
	}{
		{name: "unrestricted", restriction: access.Unrestricted(), want: contentLimit{maxAge: 18, allowUnrated: 1}},
		{name: "up to 6, unrated hidden", restriction: restrictedTo(t, access.AllLibraries(), access.Age6, true), want: contentLimit{maxAge: 6}},
		{name: "all ages, unrated shown", restriction: restrictedTo(t, access.AllLibraries(), access.AgeAll, false), want: contentLimit{allowUnrated: 1}},
		// The zero restriction denies through its empty library set, not through the limit.
		{name: "the zero restriction", want: contentLimit{maxAge: 18, allowUnrated: 1}},
	}
	for _, test := range tests {
		if got := contentLimitOf(test.restriction); got != test.want {
			t.Errorf("%s: contentLimitOf = %+v, want %+v", test.name, got, test.want)
		}
	}
}

// The scope of the episode queries carries the viewer's restriction, so a condition built
// from the scope cannot be unrestricted by omission.
func TestEpisodeScopeCarriesTheRestriction(t *testing.T) {
	t.Parallel()

	restriction := restrictedTo(t, access.AllLibraries(), access.Age6, true)
	scope := episodeScope(&sqlBuilder{}, listingTarget{libraryID: "library", restriction: restriction})

	if scope.visibleMemberSQL() == "" {
		t.Error("visibleMemberSQL of an episode scope is empty for a restricted viewer")
	}
}
