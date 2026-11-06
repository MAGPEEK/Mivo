package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// Routes of the rows contract (task M5-12).
const (
	homeRowsPath    = "/api/v1/home/rows"
	libraryRowsPath = "/api/v1/libraries/{libraryId}/rows"
)

// maxHeroItems is the hero size of HO-01.
const maxHeroItems = 5

// pagingParameters are the listLibraryItems parameters that page instead of selecting, so a
// Browse query does not carry them.
func pagingParameters() []string {
	return []string{"limit", "cursor"}
}

// isOpaqueRowID reports whether schema.property is the row identifier, a stable opaque
// string instead of a UUID: rows are generated, not stored.
func isOpaqueRowID(schema, property string) bool {
	return schema == "Row" && property == "id"
}

func TestRowEnums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]any{
		"RowKind":    {"recentlyAdded", "newReleases", "moreGenre", "recentEpisodes", "continueWatching", "upNext"},
		"CardFormat": {"poster", "episode", "square"},
		"RowsHint":   {"noLibraries", "noItems", "scanning", "notEnoughData"},
	}

	for name, want := range tests {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertEnum(t, name, schema.Value, want)
	}
}

func TestRowSchemaShapes(t *testing.T) {
	spec := loadSpec(t)

	shapes := map[string]schemaShape{
		"Row": {
			required: []string{"id", "kind", "titleKey", "cardFormat", "items"},
			optional: []string{"titleParams", "libraryId", "genre", "seeAllQuery"},
		},
		"HeroItem": {
			required: []string{"itemId", "libraryId", "kind", "title", "backdrop"},
			optional: []string{"year", "summaryLine", "logo"},
		},
		"HomeRows":    {required: []string{"rows", "hero"}, optional: []string{"hint"}},
		"LibraryRows": {required: []string{"libraryId", "rows"}, optional: []string{"hint"}},
		"ItemSummary": {optional: []string{"seasonNumber", "showTitle", "newEpisodeCount"}},
	}

	for name, shape := range shapes {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}

	references := map[string]string{
		"Row.kind": "RowKind", "Row.cardFormat": "CardFormat", "Row.seeAllQuery": "BrowseQuery",
		"HeroItem.backdrop": "ImageRef", "HeroItem.logo": "ImageRef", "HeroItem.kind": "ItemKind",
		"HomeRows.hint": "RowsHint", "LibraryRows.hint": "RowsHint",
	}
	for location, want := range references {
		if property := propertySchema(spec, location); property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}
}

// TestRowArraysAreBounded: rows, row items and the hero list have upper bounds, the hero at
// most five titles (HO-01).
func TestRowArraysAreBounded(t *testing.T) {
	spec := loadSpec(t)

	for _, location := range []string{"Row.items", "HomeRows.rows", "HomeRows.hero", "LibraryRows.rows"} {
		property := propertySchema(spec, location)
		if property == nil || property.Value.MaxItems == nil {
			t.Errorf("%s is not a bounded array", location)
		}
	}

	if hero := propertySchema(spec, "HomeRows.hero"); hero != nil && hero.Value.MaxItems != nil && *hero.Value.MaxItems != maxHeroItems {
		t.Errorf("HomeRows.hero maxItems = %d, want %d", *hero.Value.MaxItems, maxHeroItems)
	}

	params := propertySchema(spec, "Row.titleParams")
	if params == nil || params.Value.MaxProps == nil || params.Value.AdditionalProperties.Schema == nil {
		t.Error("Row.titleParams is not a bounded string map")
	}
}

// TestBrowseQueryMirrorsListing: a row's seeAllQuery is exactly a listLibraryItems query, so
// the spec validates it with the same names, enums and bounds (task M5-12).
func TestBrowseQueryMirrorsListing(t *testing.T) {
	spec := loadSpec(t)

	query := spec.Components.Schemas["BrowseQuery"].Value
	if len(query.Required) != 0 {
		t.Errorf("BrowseQuery requires %v; every property must be optional", query.Required)
	}

	listing := getOperation(t, spec, http.MethodGet, libraryItemsPath)

	for _, parameter := range listing.Parameters {
		name := parameter.Value.Name
		if parameter.Value.In != "query" || slices.Contains(pagingParameters(), name) {
			continue
		}

		property := query.Properties[name]
		if property == nil {
			t.Errorf("BrowseQuery has no property for listLibraryItems parameter %s", name)

			continue
		}

		if err := sameSchema(parameter.Value.Schema, property); err != nil {
			t.Errorf("BrowseQuery.%s differs from parameter %s: %v", name, name, err)
		}
	}

	for name := range query.Properties {
		if listing.Parameters.GetByInAndName("query", name) == nil {
			t.Errorf("BrowseQuery.%s is no listLibraryItems parameter", name)
		}
	}
}

// sameSchema compares the reference or the validation keywords of two schemas, recursing
// into array items.
func sameSchema(want, got *openapi3.SchemaRef) error {
	if want.Ref != "" || got.Ref != "" {
		if want.Ref != got.Ref {
			return fmt.Errorf("reference %q, want %q", got.Ref, want.Ref)
		}

		return nil
	}

	if err := sameBounds(want.Value, got.Value); err != nil {
		return err
	}

	if want.Value.Items == nil || got.Value.Items == nil {
		if want.Value.Items != got.Value.Items {
			return errors.New("items mismatch")
		}

		return nil
	}

	return sameSchema(want.Value.Items, got.Value.Items)
}

func sameBounds(want, got *openapi3.Schema) error {
	keywords := map[string][2]any{
		"type":      {want.Type, got.Type},
		"format":    {want.Format, got.Format},
		"minimum":   {want.Min, got.Min},
		"maximum":   {want.Max, got.Max},
		"minLength": {want.MinLength, got.MinLength},
		"maxLength": {want.MaxLength, got.MaxLength},
		"maxItems":  {want.MaxItems, got.MaxItems},
		"multiple":  {want.MultipleOf, got.MultipleOf},
	}

	for keyword, values := range keywords {
		if !reflect.DeepEqual(values[0], values[1]) {
			return fmt.Errorf("%s differs", keyword)
		}
	}

	return nil
}

// TestRowOperationsDescribeTheContract pins the parts of the contract text the server and
// clients rely on.
func TestRowOperationsDescribeTheContract(t *testing.T) {
	spec := loadSpec(t)

	for _, path := range []string{homeRowsPath, libraryRowsPath} {
		operation := getOperation(t, spec, http.MethodGet, path)

		if !hasParameterRef(operation, "RowItemLimit") {
			t.Errorf("%s does not reference parameter RowItemLimit", path)
		}

		for _, text := range []string{`"invalid"`, `"not_implemented"`, "hint", "HO-07"} {
			if !strings.Contains(operation.Description, text) {
				t.Errorf("%s description does not mention %s", path, text)
			}
		}
	}

	limit := spec.Components.Parameters["RowItemLimit"].Value
	if limit.Name != "limit" || limit.Required || limit.Schema.Value.Max == nil {
		t.Error("RowItemLimit is not the optional bounded query parameter limit")
	}

	row := spec.Components.Schemas["Row"].Value.Description
	for _, text := range []string{"YYYYMMDD", "newEpisodeCount", "season", "translation key"} {
		if !strings.Contains(row, text) {
			t.Errorf("Row description does not mention %s", text)
		}
	}
}

// TestSpecVersionCoversRows: the additive rows contract raised the minor version.
func TestSpecVersionCoversRows(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 6

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
