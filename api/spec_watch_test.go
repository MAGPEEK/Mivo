package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// Contract of task M7-03 (ADR 0032 section 8): progress, watched marks, Continue watching and
// the per-profile viewerState on items.

const (
	watchTag = "watch"

	itemProgressPath         = "/api/v1/items/{itemId}/progress"
	itemWatchedPath          = "/api/v1/items/{itemId}/watched"
	itemContinueWatchingPath = "/api/v1/items/{itemId}/continue-watching"

	viewerStateRef = "#/components/schemas/ViewerState"
)

func watchOperationCases() []operationCase {
	return []operationCase{
		{method: http.MethodPut, path: itemProgressPath, operationID: "reportItemProgress"},
		{method: http.MethodPut, path: itemWatchedPath, operationID: "markItemWatched"},
		{method: http.MethodDelete, path: itemWatchedPath, operationID: "markItemUnwatched"},
		{method: http.MethodDelete, path: itemContinueWatchingPath, operationID: "hideFromContinueWatching"},
	}
}

// TestWatchOperations: every watch operation is tagged, needs a session, answers 204 without
// a body and names its rules: the same-origin rule, 400 "invalid", 404 and profile_required.
func TestWatchOperations(t *testing.T) {
	spec := loadSpec(t)

	for _, test := range watchOperationCases() {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			operation := getOperation(t, spec, test.method, test.path)

			if operation.OperationID != test.operationID || !slices.Contains(operation.Tags, watchTag) {
				t.Errorf("operation = %q %v, want %q tagged %s", operation.OperationID, operation.Tags, test.operationID, watchTag)
			}

			noContent := operation.Responses.Status(http.StatusNoContent)
			if noContent == nil || len(noContent.Value.Content) != 0 {
				t.Error("operation does not answer 204 without a body")
			}

			assertWatchRulesNamed(t, operation.Description)
			assertSessionRequired(t, specOperation{method: test.method, path: test.path, operation: operation})
		})
	}
}

func assertWatchRulesNamed(t *testing.T, description string) {
	t.Helper()

	for _, text := range []string{"same-origin", "ADR 0032 section 3", `"invalid" (400`, `"not_found" (404`, `"profile_required" (403)`} {
		if !strings.Contains(description, text) {
			t.Errorf("description does not mention %s", text)
		}
	}

	// Clients retry with keepalive, so every watch write must be safe to repeat.
	if !strings.Contains(strings.ToLower(description), "idempotent") {
		t.Error("description does not say the operation is idempotent")
	}
}

// TestProgressReportShape: the position is a required non-negative double in seconds, the
// playback session an optional UUID, and non-finite positions are refused (BACKLOG m7-06).
func TestProgressReportShape(t *testing.T) {
	spec := loadSpec(t)

	body := getOperation(t, spec, http.MethodPut, itemProgressPath).RequestBody
	if body == nil || !body.Value.Required || body.Value.Content.Get(jsonMediaType).Schema.Ref != "#/components/schemas/ProgressReport" {
		t.Fatal("reportItemProgress does not require a ProgressReport body")
	}

	report := spec.Components.Schemas["ProgressReport"].Value
	if !slices.Equal(report.Required, []string{"positionSeconds"}) {
		t.Errorf("ProgressReport.required = %v, want [positionSeconds]", report.Required)
	}

	position := report.Properties["positionSeconds"].Value
	if position.Format != "double" || position.Min == nil || *position.Min != 0 {
		t.Error("positionSeconds must be a double with minimum 0")
	}

	for _, text := range []string{"finite", `"bad_request"`, `"invalid"`, "startPositionSeconds"} {
		if !strings.Contains(position.Description, text) {
			t.Errorf("positionSeconds does not mention %s", text)
		}
	}

	if session := report.Properties["playbackSessionId"]; session == nil || session.Value.Format != "uuid" {
		t.Error("playbackSessionId must be an optional uuid")
	}
}

// TestViewerStateShape: items, details and therefore row cards carry the optional viewerState
// with the fields the badges and the Resume button need.
func TestViewerStateShape(t *testing.T) {
	spec := loadSpec(t)

	for _, location := range []string{"ItemSummary.viewerState", "ItemDetail.viewerState"} {
		if property := propertySchema(spec, location); property == nil || property.Ref != viewerStateRef {
			t.Errorf("%s does not reference ViewerState", location)
		}
	}

	for _, name := range []string{"ItemSummary", "ItemDetail"} {
		if slices.Contains(spec.Components.Schemas[name].Value.Required, "viewerState") {
			t.Errorf("%s.viewerState must be optional", name)
		}
	}

	state := spec.Components.Schemas["ViewerState"].Value
	if want := []string{"watched", "positionSeconds", "playCount"}; !slices.Equal(state.Required, want) {
		t.Errorf("ViewerState.required = %v, want %v", state.Required, want)
	}

	for _, field := range []string{"lastPlayedAt", "unwatchedCount"} {
		if state.Properties[field] == nil {
			t.Errorf("ViewerState has no %s", field)
		}
	}

	if state.Properties["lastPlayedAt"].Value.Format != "date-time" {
		t.Error("ViewerState.lastPlayedAt is not a date-time")
	}
}

// TestWatchRowsDescribeTheirRules: the new row kinds name the ADR 0032 bounds and give episode
// cards what the "S3 · E9" label needs.
func TestWatchRowsDescribeTheirRules(t *testing.T) {
	spec := loadSpec(t)

	row := spec.Components.Schemas["Row"].Value.Description
	for _, text := range []string{"continueWatching", "upNext", "12 weeks", "at most 40", "at most 20", "season 0", "seasonNumber", "showTitle", "viewerState"} {
		if !strings.Contains(row, text) {
			t.Errorf("Row description does not mention %s", text)
		}
	}

	for _, field := range []string{"seasonNumber", "number", "showTitle"} {
		if propertySchema(spec, "ItemSummary."+field) == nil {
			t.Errorf("ItemSummary has no %s", field)
		}
	}

	home := getOperation(t, spec, http.MethodGet, homeRowsPath).Description
	overview := getOperation(t, spec, http.MethodGet, libraryRowsPath).Description

	if !strings.Contains(home, `"continueWatching" and "upNext"`) || !strings.Contains(overview, `"continueWatching"`) {
		t.Error("the rows operations do not place the watch rows")
	}
}

// TestSpecVersionCoversWatch: the additive M7-03 contract raised the minor version.
func TestSpecVersionCoversWatch(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 14

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
