package api_test

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// Contract of task M4-02b (ADR 0030 sections 5, 7 and 8): the list of running sessions, the
// hardware acceleration status with its test run, and trickplay.

const (
	playbackSessionsPath = "/api/v1/playback/sessions"
	transcoderPath       = "/api/v1/system/transcoder"
	transcoderTestPath   = transcoderPath + "/test"
	trickplayPath        = "/api/v1/media-files/{fileId}/trickplay"
	trickplaySheetPath   = trickplayPath + "/{sheet}.jpg"

	playbackTag = "playback"
	systemTag   = "system"
)

func sessionsOperationCases() []operationCase {
	return []operationCase{
		{method: http.MethodGet, path: playbackSessionsPath, operationID: "listPlaybackSessions", tag: playbackTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "PlaybackSessionList"},
		{method: http.MethodGet, path: transcoderPath, operationID: "getTranscoderStatus", tag: systemTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "TranscoderStatus"},
		{method: http.MethodPost, path: transcoderTestPath, operationID: "testTranscoder", tag: systemTag, statuses: []int{http.StatusAccepted}, mediaType: jsonMediaType, schema: "TranscoderStatus"},
		{method: http.MethodPost, path: transcoderTestPath, operationID: "testTranscoder", tag: systemTag, statuses: []int{http.StatusConflict}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: trickplayPath, operationID: "getTrickplayManifest", tag: playbackTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "TrickplayManifest"},
		{method: http.MethodGet, path: trickplayPath, operationID: "getTrickplayManifest", tag: playbackTag, statuses: []int{http.StatusNotFound}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodPost, path: trickplayPath, operationID: "requestTrickplay", tag: playbackTag, statuses: []int{http.StatusNotFound}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: trickplaySheetPath, operationID: "getTrickplaySheet", tag: streamTag, statuses: []int{http.StatusOK}, mediaType: "image/jpeg", schema: "ImageBytes"},
		{method: http.MethodGet, path: trickplaySheetPath, operationID: "getTrickplaySheet", tag: streamTag, statuses: []int{http.StatusNotFound}, mediaType: problemMediaType, schema: "Problem"},
	}
}

// TestSessionsOperations: every operation of the task is declared, tagged, answers its
// schema and needs the signed-in session.
func TestSessionsOperations(t *testing.T) {
	spec := loadSpec(t)

	for _, test := range sessionsOperationCases() {
		t.Run(fmt.Sprintf("%s %s %v", test.method, test.path, test.statuses), func(t *testing.T) {
			operation := getOperation(t, spec, test.method, test.path)

			if operation.OperationID != test.operationID || !slices.Contains(operation.Tags, test.tag) {
				t.Errorf("operation = %q %v, want %q tagged %s", operation.OperationID, operation.Tags, test.operationID, test.tag)
			}

			for _, status := range test.statuses {
				assertResponseSchema(t, operation.Responses.Status(status), test.mediaType, test.schema)
			}

			assertSessionRequired(t, specOperation{method: test.method, path: test.path, operation: operation})
		})
	}
}

// TestSessionsManagementOperationsDocumentTheGuard: the session list and the transcoder
// status and test are management operations (ADR 0028) and document their 403.
func TestSessionsManagementOperationsDocumentTheGuard(t *testing.T) {
	spec := loadSpec(t)

	managed := []specOperationKey{
		{http.MethodGet, playbackSessionsPath},
		{http.MethodGet, transcoderPath},
		{http.MethodPost, transcoderTestPath},
	}

	for _, key := range managed {
		operation := getOperation(t, spec, key.method, key.path)

		texts := []string{"Management operation (ADR 0028)", "ADR 0032 section 9", "local", `403 "forbidden"`}
		if key.method != http.MethodGet {
			texts = append(texts, "same-origin", "ADR 0032 section 3")
		}

		for _, text := range texts {
			if !strings.Contains(operation.Description, text) {
				t.Errorf("%s %s does not mention %s", key.method, key.path, text)
			}
		}

		assertResponseSchema(t, operation.Responses.Status(http.StatusForbidden), problemMediaType, "Problem")
	}
}

// TestTrickplayIsNoManagementOperation: every viewer scrubs, also remotely and on a managed
// profile, so the trickplay routes carry no management guard.
func TestTrickplayIsNoManagementOperation(t *testing.T) {
	spec := loadSpec(t)

	for _, key := range []specOperationKey{
		{http.MethodGet, trickplayPath}, {http.MethodPost, trickplayPath}, {http.MethodGet, trickplaySheetPath},
	} {
		operation := getOperation(t, spec, key.method, key.path)
		if strings.Contains(operation.Description, "Management operation") {
			t.Errorf("%s %s is documented as a management operation", key.method, key.path)
		}
	}

	request := getOperation(t, spec, http.MethodPost, trickplayPath)

	accepted := request.Responses.Status(http.StatusAccepted)
	if accepted == nil || len(accepted.Value.Content) != 0 {
		t.Error("requestTrickplay does not answer 202 without a body")
	}

	for _, text := range []string{"Idempotent", "same-origin", `404 "not_found"`, "A full queue still answers 202"} {
		if !strings.Contains(request.Description, text) {
			t.Errorf("requestTrickplay does not mention %s", text)
		}
	}
}

// TestMediaFileRoutesHideInvisibleFiles: a file the selected profile may not see (library
// access, content limit) answers like an unknown one, so the routes keyed by file ID reveal
// nothing about it.
func TestMediaFileRoutesHideInvisibleFiles(t *testing.T) {
	spec := loadSpec(t)

	const phrase = "unknown file or one the selected profile may not see"

	for _, key := range []specOperationKey{
		{http.MethodGet, trickplayPath},
		{http.MethodPost, trickplayPath},
		{http.MethodGet, trickplaySheetPath},
		{http.MethodGet, subtitleTrackPath},
	} {
		operation := getOperation(t, spec, key.method, key.path)
		if !strings.Contains(joinLines(operation.Description), phrase) {
			t.Errorf("%s %s does not say: %s", key.method, key.path, phrase)
		}
	}
}

func sessionsShapes() map[string]schemaShape {
	return map[string]schemaShape{
		"PlaybackSessionList": {required: []string{"sessions"}},
		"PlaybackSessionSummary": {
			required: []string{"id", "itemId", "title", "state", "mode", "hardware", "clientName", "startedAt"},
			optional: []string{"videoEncoder", "outputBitRate", "transcodeSpeed", "paused"},
		},
		"TranscoderStatus": {
			required: []string{"hwaccel", "devices", "throttleSupported"},
			optional: []string{"lastTestedAt"},
		},
		"TranscoderDevice": {
			required: []string{"api", "encoder", "codec", "usable", "degraded"},
			optional: []string{"message"},
		},
		"TrickplayManifest": {
			required: []string{"tileWidth", "tileHeight", "columns", "rows", "intervalSeconds", "sheetCount", "sheetUrls"},
		},
	}
}

func TestSessionsSchemaShapes(t *testing.T) {
	spec := loadSpec(t)

	for name, shape := range sessionsShapes() {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}
}

func TestSessionsEnumsAndReferences(t *testing.T) {
	spec := loadSpec(t)

	enums := map[string][]any{
		"HardwareAccelerationSetting": {"auto", "off", "vaapi", "qsv", "nvenc", "amf"},
		"HardwareAccelerationApi":     {"vaapi", "qsv", "nvenc", "amf"},
	}

	for name, want := range enums {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertEnum(t, name, schema.Value, want)
	}

	references := map[string]string{
		"PlaybackSessionSummary.state": "PlaybackSessionState",
		"PlaybackSessionSummary.mode":  "PlaybackMode",
		"TranscoderStatus.hwaccel":     "HardwareAccelerationSetting",
		"TranscoderDevice.api":         "HardwareAccelerationApi",
	}

	for location, want := range references {
		property := propertySchema(spec, location)
		if property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}

	for location, want := range map[string]string{
		"PlaybackSessionList.sessions": "PlaybackSessionSummary",
		"TranscoderStatus.devices":     "TranscoderDevice",
	} {
		list := propertySchema(spec, location)
		if list == nil || list.Value.Items == nil || list.Value.Items.Ref != "#/components/schemas/"+want {
			t.Errorf("%s is no array of %s", location, want)
		}
	}
}

// TestSessionSummaryLeaksNoPath: the list is read by an administrator about other viewers'
// sessions; it names the item by its catalog title and carries no path, file name or URL.
func TestSessionSummaryLeaksNoPath(t *testing.T) {
	spec := loadSpec(t)

	summary := spec.Components.Schemas["PlaybackSessionSummary"]
	if summary == nil {
		t.Fatal("components.schemas.PlaybackSessionSummary is not declared")
	}

	for _, forbidden := range []string{"path", "fileName", "streamUrl", "url"} {
		if summary.Value.Properties[forbidden] != nil {
			t.Errorf("PlaybackSessionSummary exposes %q", forbidden)
		}
	}

	if title := requiredProperty(t, spec, "PlaybackSessionSummary.title"); !strings.Contains(title.Description, "catalog") {
		t.Error("PlaybackSessionSummary.title does not say it comes from the catalog")
	}

	speed := requiredProperty(t, spec, "PlaybackSessionSummary.transcodeSpeed")
	if !speed.Type.Is(openapi3.TypeNumber) || !strings.Contains(speed.Description, "1.0 is real time") {
		t.Error("PlaybackSessionSummary.transcodeSpeed is no number with 1.0 as real time")
	}

	for _, location := range []string{"PlaybackSessionSummary.transcodeSpeed", "PlaybackSessionSummary.paused"} {
		if !strings.Contains(requiredProperty(t, spec, location).Description, "Omitted for direct play") {
			t.Errorf("%s does not say it is omitted for direct play", location)
		}
	}

	if message := requiredProperty(t, spec, "TranscoderDevice.message"); !strings.Contains(message.Description, "never contains a file system path") {
		t.Error("TranscoderDevice.message does not exclude paths")
	}
}

// TestTrickplaySheetRoute: the sprite sheets are a plain handler like the subtitle route,
// with opaque URLs handed out by the manifest.
func TestTrickplaySheetRoute(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, trickplaySheetPath)

	if !slices.Contains(excludedOperationIDs(t), operation.OperationID) {
		t.Errorf("%s does not exclude %s from the strict server", generatorConfigFile, operation.OperationID)
	}

	for _, text := range []string{`"not_found"`, "opaque", "TrickplayManifest.sheetUrls"} {
		if !strings.Contains(operation.Description, text) {
			t.Errorf("the trickplay sheet route does not name %s", text)
		}
	}

	parameter := spec.Paths.Value(trickplaySheetPath).Parameters.GetByInAndName(openapi3.ParameterInPath, "sheet")
	if parameter == nil || parameter.Schema.Value.Pattern == "" || parameter.Schema.Value.MaxLength == nil {
		t.Fatal("the trickplay sheet route declares no bounded sheet parameter")
	}

	pattern := regexp.MustCompile(parameter.Schema.Value.Pattern)

	for sheet, want := range map[string]bool{
		"0": true, "7": true, "42": true, "9999": true,
		"": false, "-1": false, "01": false, "10000": false, "1.jpg": false, "../0": false, "a": false,
	} {
		if got := pattern.MatchString(sheet); got != want {
			t.Errorf("sheet pattern matches %q = %v, want %v", sheet, got, want)
		}
	}

	urls := requiredProperty(t, spec, "TrickplayManifest.sheetUrls")
	if urls.MaxItems == nil || urls.Items == nil || urls.Items.Value.Format != "uri-reference" ||
		!strings.Contains(urls.Items.Value.Description, "opaque") {
		t.Error("TrickplayManifest.sheetUrls is no bounded list of opaque URI references")
	}
}

// TestSpecVersionCoversSessions: the additive contract of M4-02b raised the minor version.
func TestSpecVersionCoversSessions(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 16

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
