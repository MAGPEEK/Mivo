package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// Routes of the Browse pivot contract (task M5-10).
const (
	libraryItemsPath   = "/api/v1/libraries/{libraryId}/items"
	libraryFacetsPath  = "/api/v1/libraries/{libraryId}/facets"
	libraryLettersPath = "/api/v1/libraries/{libraryId}/letters"
	libraryFoldersPath = "/api/v1/libraries/{libraryId}/folders"
)

// componentParameterPrefix is the reference prefix of shared parameters.
const componentParameterPrefix = "#/components/parameters/"

// fieldFilterParameters maps every field filter's query name to its component parameter.
// Filters of later milestones (people, collection, watch state) are deliberately absent.
func fieldFilterParameters() map[string]string {
	return map[string]string{
		"genre": "GenreFilter", "year": "YearFilter", "decade": "DecadeFilter",
		"contentRating": "ContentRatingFilter", "resolution": "ResolutionFilter",
		"videoCodec": "VideoCodecFilter", "audioCodec": "AudioCodecFilter",
		"container": "ContainerFilter", "audioLanguage": "AudioLanguageFilter",
		"subtitleLanguage": "SubtitleLanguageFilter",
	}
}

// listingFilterParameters adds the type and quick filters to the field filters.
func listingFilterParameters() map[string]string {
	parameters := fieldFilterParameters()
	parameters["type"] = "ItemTypeFilter"
	parameters["quick"] = "QuickFilter"

	return parameters
}

// TestListingFiltersAreShared: listing, facets and letters accept the same filters through
// the same component parameters, so "the current filter" means the same on all three.
func TestListingFiltersAreShared(t *testing.T) {
	spec := loadSpec(t)

	for _, path := range []string{libraryItemsPath, libraryFacetsPath, libraryLettersPath} {
		operation := getOperation(t, spec, http.MethodGet, path)

		for name, component := range listingFilterParameters() {
			if !hasParameterRef(operation, component) {
				t.Errorf("%s does not reference parameter %s", path, component)
			}

			parameter := spec.Components.Parameters[component]
			if parameter == nil || parameter.Value.Name != name || parameter.Value.In != "query" || parameter.Value.Required {
				t.Errorf("components.parameters.%s is not the optional query parameter %q", component, name)
			}
		}
	}
}

func hasParameterRef(operation *openapi3.Operation, component string) bool {
	for _, parameter := range operation.Parameters {
		if parameter.Ref == componentParameterPrefix+component {
			return true
		}
	}

	return false
}

// TestFieldFiltersAreBoundedArrays: every field filter is a repeatable, bounded parameter.
func TestFieldFiltersAreBoundedArrays(t *testing.T) {
	spec := loadSpec(t)

	for _, component := range fieldFilterParameters() {
		parameter := spec.Components.Parameters[component]
		if parameter == nil {
			t.Errorf("components.parameters.%s is not declared", component)

			continue
		}

		schema := parameter.Value.Schema.Value
		if !schema.Type.Is(openapi3.TypeArray) || schema.MaxItems == nil || schema.Items == nil {
			t.Errorf("%s is not a bounded array", component)
		}
	}

	decade := spec.Components.Parameters["DecadeFilter"].Value.Schema.Value.Items.Value
	if decade.MultipleOf == nil || *decade.MultipleOf != 10 {
		t.Error("DecadeFilter items are not a multiple of 10")
	}
}

func TestListLibraryItemsSortParameters(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, libraryItemsPath)

	references := map[string]string{"sort": "ItemSort", "order": "SortOrder", "matchState": "MatchState"}
	for name, want := range references {
		parameter := operation.Parameters.GetByInAndName("query", name)
		if parameter == nil || parameter.Required || parameter.Schema.Ref != "#/components/schemas/"+want {
			t.Errorf("optional query parameter %s with schema %s is not declared", name, want)
		}
	}

	seed := operation.Parameters.GetByInAndName("query", "seed")
	if seed == nil || seed.Required || !seed.Schema.Value.Type.Is(openapi3.TypeInteger) {
		t.Error("optional integer query parameter seed is not declared")
	}

	for _, text := range []string{"400", `"invalid"`, "opaque", "totalCount", `"not_implemented"`} {
		if !strings.Contains(operation.Description, text) {
			t.Errorf("listLibraryItems description does not mention %s", text)
		}
	}
}

func TestBrowseEnums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]any{
		"ItemSort": {
			"title", "year", "releaseDate", "contentRating", "duration", "dateAdded",
			"resolution", "bitrate", "fileSize", "takenAt", "random",
		},
		"ItemQuickFilter": {"all", "hdr", "dolbyVision", "hdr10plus", "atmos", "uhd", "unavailable", "duplicates"},
		"VideoResolution": {"sd", "720p", "1080p", "4k", "8k"},
		"FacetField": {
			"genre", "year", "decade", "contentRating", "resolution", "videoCodec",
			"audioCodec", "container", "audioLanguage", "subtitleLanguage",
		},
		"LibraryFolderEntryKind": {"folder", "item"},
	}

	for name, want := range tests {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertEnum(t, name, schema.Value, want)
	}

	// Every facet field is a filter parameter of the same name.
	for _, field := range spec.Components.Schemas["FacetField"].Value.Enum {
		if _, ok := fieldFilterParameters()[fmt.Sprint(field)]; !ok {
			t.Errorf("FacetField %v has no filter parameter", field)
		}
	}
}

func TestBrowseSchemaShapes(t *testing.T) {
	spec := loadSpec(t)

	shapes := map[string]schemaShape{
		"ItemPage":             {required: []string{"items"}, optional: []string{"nextCursor", "totalCount"}},
		"FacetList":            {required: []string{"facets"}},
		"Facet":                {required: []string{"field", "values", "truncated"}},
		"FacetValue":           {required: []string{"value", "count"}},
		"LetterIndex":          {required: []string{"letters", "totalCount"}},
		"LetterOffset":         {required: []string{"letter", "offset"}},
		"LibraryFolderListing": {required: []string{"libraryId", "path", "entries"}, optional: []string{"parentPath", "nextCursor", "totalCount"}},
		"LibraryFolderEntry":   {required: []string{"kind", "name", "path"}, optional: []string{"item"}},
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
		"Facet.field": "FacetField", "LibraryFolderEntry.kind": "LibraryFolderEntryKind",
		"LibraryFolderEntry.item": "ItemSummary",
	}
	for location, want := range references {
		if property := propertySchema(spec, location); property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}
}

// TestLibraryFoldersAreRelative: folder browsing never exposes absolute paths and rejects
// traversal (task M5-10).
func TestLibraryFoldersAreRelative(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, libraryFoldersPath)

	path := operation.Parameters.GetByInAndName("query", "path")
	if path == nil || path.Required || path.Schema.Value.MaxLength == nil {
		t.Fatal("optional bounded query parameter path is not declared")
	}

	for _, text := range []string{`".."`, "400", "never absolute"} {
		if !strings.Contains(operation.Description, text) {
			t.Errorf("listLibraryFolders description does not mention %s", text)
		}
	}

	entryPath := propertySchema(spec, "LibraryFolderEntry.path")
	if entryPath == nil || !strings.Contains(entryPath.Value.Description, "Never an absolute") {
		t.Error("LibraryFolderEntry.path does not say it is never absolute")
	}
}

// TestSpecVersionCoversBrowse: the additive Browse contract raised the minor version.
func TestSpecVersionCoversBrowse(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 5

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
