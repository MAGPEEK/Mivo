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

// Contract of task M4-02a (ADR 0030 sections 3, 4 and 6; ADR 0023 Notes M4-04a): audio and
// subtitle tracks, the subtitle decision, the quality cap and file versions.

// subtitleTrackPath is the WebVTT rendition of one subtitle track (ADR 0030 section 3).
const subtitleTrackPath = "/api/v1/media-files/{fileId}/subtitles/{trackId}.vtt"

// isTrackID reports whether schema.property holds a subtitle track id ("s:3", "x:<hash>")
// instead of a UUID (task M4-02a).
func isTrackID(schema, property string) bool {
	switch schema + "." + property {
	case "SubtitleTrack.id", "SubtitleDecision.trackId", "PlaybackRequest.subtitleTrackId", "MediaFile.defaultSubtitleTrackId":
		return true
	default:
		return false
	}
}

func playbackV2Shapes() map[string]schemaShape {
	return map[string]schemaShape{
		"PlaybackRequest": {
			required: []string{"fileId", "profile"},
			optional: []string{
				"audioStreamIndex", "subtitleTrackId", "allowDegradeSubtitles", "maxBitRate", "startPositionSeconds", "replacesSessionId",
			},
		},
		"PlaybackSession": {
			required: []string{"id", "fileId", "state", "mode", "delivery", "streamUrl", "streams", "reasons", "warnings"},
			optional: []string{"subtitle", "outputBitRate", "sourceBitRate", "transcodeSpeed"},
		},
		"StreamDecision": {
			required: []string{"kind", "sourceIndex", "action", "sourceCodec", "targetCodec"},
			optional: []string{"encoder", "hardware"},
		},
		"SubtitleDecision": {required: []string{"mode"}, optional: []string{"trackId"}},
		"SubtitleTrack": {
			required: []string{"id", "format", "forced", "sdh", "default", "external", "textBased"},
			optional: []string{"language", "title", "url"},
		},
		"MediaFile": {
			required: []string{"id", "fileName", "streams"},
			optional: []string{
				"subtitleTracks", "defaultAudioStreamIndex", "defaultSubtitleTrackId",
				"versionLabel", "width", "height", "resolution", "bitRate",
			},
		},
		"MediaStream": {
			required: []string{"index", "kind", "default"},
			optional: []string{"language", "title", "channels", "sampleRate", "bitDepth"},
		},
		"ClientProfile": {
			required: []string{"name", "directPlay", "hls"},
			optional: []string{
				"preferredAudioLanguages", "preferredSubtitleLanguages", "subtitleMode", "preferSdh",
				"subtitleFormats", "maxAudioChannels", "maxAudioSampleRate", "maxAudioBitDepth",
			},
		},
	}
}

// TestPlaybackV2SchemaShapes: every addition is optional, so clients and servers of the
// milestone-2 contract keep working (additive only).
func TestPlaybackV2SchemaShapes(t *testing.T) {
	spec := loadSpec(t)

	for name, shape := range playbackV2Shapes() {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}
}

func TestPlaybackV2Enums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]any{
		"SubtitleDeliveryMode":   {"off", "sidecar", "burn_in"},
		"SubtitlePreferenceMode": {"off", "forced", "always"},
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

func TestPlaybackV2PropertyReferences(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string]string{
		"SubtitleDecision.mode":      "SubtitleDeliveryMode",
		"PlaybackSession.subtitle":   "SubtitleDecision",
		"ClientProfile.subtitleMode": "SubtitlePreferenceMode",
		"MediaFile.resolution":       "VideoResolution",
	}

	for location, want := range tests {
		property := propertySchema(spec, location)
		if property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}

	tracks := propertySchema(spec, "MediaFile.subtitleTracks")
	if tracks == nil || tracks.Value.Items == nil || tracks.Value.Items.Ref != "#/components/schemas/SubtitleTrack" {
		t.Error("MediaFile.subtitleTracks is no array of SubtitleTrack")
	}
}

// TestPlaybackReasonCodesAreListed: the code stays an open string (unknown codes get a
// generic text, as in milestone 2), but every code the server emits is documented.
func TestPlaybackReasonCodesAreListed(t *testing.T) {
	spec := loadSpec(t)

	code := propertySchema(spec, "PlaybackReason.code")
	if code == nil || len(code.Value.Enum) != 0 {
		t.Fatal("PlaybackReason.code is missing or a closed enum")
	}

	for _, want := range []string{
		"subtitle_burn_in", "bitrate_cap", "dsd_to_pcm", "hires_downsampled", "hwaccel_unavailable",
		"subtitle_burn_in_unavailable", "audio_track_not_first",
	} {
		if !strings.Contains(code.Value.Description, `"`+want+`"`) {
			t.Errorf("PlaybackReason.code does not list %q", want)
		}
	}

	if !strings.Contains(code.Value.Description, "New codes may be added") {
		t.Error("PlaybackReason.code does not announce new codes")
	}
}

// requiredProperty fails the test when "Schema.property" is not declared.
func requiredProperty(t *testing.T, spec *openapi3.T, location string) *openapi3.Schema {
	t.Helper()

	property := propertySchema(spec, location)
	if property == nil || property.Value == nil {
		t.Fatalf("%s is not declared", location)
	}

	return property.Value
}

// TestTrackSelectionFields pins the wire form of the request overrides and the server
// defaults (ADR 0023 Notes M4-04a).
func TestTrackSelectionFields(t *testing.T) {
	spec := loadSpec(t)

	maxBitRate := requiredProperty(t, spec, "PlaybackRequest.maxBitRate")
	if maxBitRate.Format != "int64" || maxBitRate.Min == nil || *maxBitRate.Min != 0 {
		t.Error("PlaybackRequest.maxBitRate is no int64 bounded below by 0")
	}

	// The field was renamed before any release carried it (ADR 0030 Notes M4-02b).
	if propertySchema(spec, "PlaybackRequest.maxBitrate") != nil {
		t.Error("PlaybackRequest still declares maxBitrate next to maxBitRate")
	}

	if outputBitRate := requiredProperty(t, spec, "PlaybackSession.outputBitRate"); outputBitRate.Format != "int64" {
		t.Error("PlaybackSession.outputBitRate is no int64")
	}

	requested := requiredProperty(t, spec, "PlaybackRequest.subtitleTrackId")
	if !strings.Contains(requested.Description, "empty string") {
		t.Error("PlaybackRequest.subtitleTrackId does not say how subtitles are switched off")
	}

	for _, location := range []string{"MediaFile.defaultAudioStreamIndex", "MediaFile.defaultSubtitleTrackId"} {
		if !requiredProperty(t, spec, location).Nullable {
			t.Errorf("%s is not nullable", location)
		}
	}
}

// TestDefaultSubtitlePolicyNeverBurnsIn: burn-in forces a transcode, so only an explicit
// subtitleTrackId gets it. A client of the milestone-2 contract, which sends neither
// subtitleMode nor subtitleFormats, therefore keeps direct play.
func TestDefaultSubtitlePolicyNeverBurnsIn(t *testing.T) {
	spec := loadSpec(t)

	requested := requiredProperty(t, spec, "PlaybackRequest.subtitleTrackId")
	for _, text := range []string{"never picks a track that needs burn-in", "only when this field names it", "keeps direct play"} {
		if !strings.Contains(joinLines(requested.Description), text) {
			t.Errorf("PlaybackRequest.subtitleTrackId does not say: %s", text)
		}
	}

	formats := requiredProperty(t, spec, "ClientProfile.subtitleFormats")
	if !strings.Contains(joinLines(formats.Description), "only an explicit PlaybackRequest.subtitleTrackId") {
		t.Error("ClientProfile.subtitleFormats does not tie burn-in to an explicit subtitleTrackId")
	}

	create := getOperation(t, spec, http.MethodPost, "/api/v1/playback/sessions")
	if !strings.Contains(joinLines(create.Description), "never picks a subtitle that needs burn-in") {
		t.Error("createPlaybackSession does not state the default subtitle policy")
	}

	// The fields a milestone-2 client does not send stay optional.
	for _, schema := range []string{"PlaybackRequest", "ClientProfile"} {
		required := spec.Components.Schemas[schema].Value.Required
		for _, property := range []string{"subtitleTrackId", "allowDegradeSubtitles", "subtitleMode", "subtitleFormats"} {
			if slices.Contains(required, property) {
				t.Errorf("%s.%s is required", schema, property)
			}
		}
	}
}

// joinLines undoes the line folding of a YAML description, so a phrase is found wherever
// the line breaks fall.
func joinLines(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// TestAllowDegradeSubtitles: the request decides whether a burn-in the server cannot do now
// drops the subtitle or fails the start (ADR 0030 Notes M4-02b).
func TestAllowDegradeSubtitles(t *testing.T) {
	spec := loadSpec(t)

	allow := requiredProperty(t, spec, "PlaybackRequest.allowDegradeSubtitles")
	if !allow.Type.Is(openapi3.TypeBoolean) {
		t.Error("PlaybackRequest.allowDegradeSubtitles is no boolean")
	}

	for _, text := range []string{`"subtitle_burn_in_unavailable"`, `"session_limit_reached"`, "omitted or false"} {
		if !strings.Contains(joinLines(allow.Description), text) {
			t.Errorf("PlaybackRequest.allowDegradeSubtitles does not mention %s", text)
		}
	}
}

// TestAudioLimitsAreProfileFields: sample rate and bit depth of audio are limits of the
// client as a whole. CodecLimit stays video only; its bit depth ends at 16, which 24-bit
// audio would not fit.
func TestAudioLimitsAreProfileFields(t *testing.T) {
	spec := loadSpec(t)

	const maxVideoBitDepth, minAudioBitDepthLimit = 16, 24

	for _, location := range []string{"ClientProfile.maxAudioSampleRate", "ClientProfile.maxAudioBitDepth"} {
		limit := requiredProperty(t, spec, location)
		if !limit.Type.Is(openapi3.TypeInteger) || limit.Min == nil || limit.Max == nil {
			t.Errorf("%s is no bounded integer", location)
		}

		if !strings.Contains(limit.Description, `"hires_downsampled"`) {
			t.Errorf("%s does not name the reason hires_downsampled", location)
		}
	}

	if depth := requiredProperty(t, spec, "ClientProfile.maxAudioBitDepth"); depth.Max != nil && *depth.Max < minAudioBitDepthLimit {
		t.Errorf("ClientProfile.maxAudioBitDepth ends at %g, below %d bit", *depth.Max, minAudioBitDepthLimit)
	}

	if video := requiredProperty(t, spec, "CodecLimit.maxBitDepth"); video.Max == nil || *video.Max != maxVideoBitDepth ||
		!strings.Contains(video.Description, "video") {
		t.Error("CodecLimit.maxBitDepth is not the video bit depth capped at 16")
	}

	code := requiredProperty(t, spec, "PlaybackReason.code")
	for _, field := range []string{"ClientProfile.maxAudioSampleRate", "ClientProfile.maxAudioBitDepth"} {
		if !strings.Contains(code.Description, field) {
			t.Errorf("reason hires_downsampled does not refer to %s", field)
		}
	}

	if depth := requiredProperty(t, spec, "MediaStream.bitDepth"); !strings.Contains(depth.Description, "audio") {
		t.Error("MediaStream.bitDepth does not cover audio streams")
	}
}

// TestSubtitleContractWording pins two rules clients rely on: the WebVTT URL is opaque, and
// the preference mode is a request value the server validates.
func TestSubtitleContractWording(t *testing.T) {
	spec := loadSpec(t)

	if url := requiredProperty(t, spec, "SubtitleTrack.url"); url.Format != "uri-reference" || !strings.Contains(url.Description, "opaque") {
		t.Error("SubtitleTrack.url is not documented as an opaque URI reference")
	}

	mode := spec.Components.Schemas["SubtitlePreferenceMode"]
	if mode == nil || !strings.Contains(joinLines(mode.Value.Description), `400 "invalid" for a value it does not know`) {
		t.Error("SubtitlePreferenceMode does not say that an unknown value answers 400 invalid")
	}
}

// TestSubtitleTrackIDPattern: track ids are stable strings, never list positions.
func TestSubtitleTrackIDPattern(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string]bool{
		"s:0": true, "s:3": true, "s:12345": true, "x:0a1b2c3d4e5f": true,
		"": false, "3": false, "s:": false, "s:-1": false, "x:": false, "x:../a": false, "X:ab": false,
		"s:3.vtt": false, "x:" + strings.Repeat("a", 65): false,
	}

	for _, location := range []string{"SubtitleTrack.id", "SubtitleDecision.trackId", "MediaFile.defaultSubtitleTrackId"} {
		assertTrackIDPattern(t, location, requiredProperty(t, spec, location), tests)
	}

	// The request also accepts the empty string, which switches subtitles off.
	requestTests := map[string]bool{"": true, "s:3": true, "x:0a1b": true, "off": false}
	assertTrackIDPattern(t, "PlaybackRequest.subtitleTrackId", requiredProperty(t, spec, "PlaybackRequest.subtitleTrackId"), requestTests)

	getOperation(t, spec, http.MethodGet, subtitleTrackPath)

	parameter := spec.Paths.Value(subtitleTrackPath).Parameters.GetByInAndName(openapi3.ParameterInPath, "trackId")
	if parameter == nil {
		t.Fatal("the subtitle route declares no trackId parameter")
	}

	assertTrackIDPattern(t, "path parameter trackId", parameter.Schema.Value, tests)
}

func assertTrackIDPattern(t *testing.T, location string, schema *openapi3.Schema, tests map[string]bool) {
	t.Helper()

	if schema.Pattern == "" || schema.MaxLength == nil {
		t.Fatalf("%s has no pattern or no maxLength", location)
	}

	pattern := regexp.MustCompile(schema.Pattern)

	for id, want := range tests {
		if got := pattern.MatchString(id); got != want {
			t.Errorf("%s matches %q = %v, want %v", location, id, got, want)
		}
	}
}

// TestPreferredLanguagePattern mirrors playback.TrackPreference.Validate: at most 8 tags, a
// primary subtag of two or three letters and optional subtags.
func TestPreferredLanguagePattern(t *testing.T) {
	spec := loadSpec(t)

	const maxPreferredLanguages = 8

	tests := map[string]bool{
		"de": true, "deu": true, "ger": true, "pt-BR": true, "pt_br": true, "zh-Hans-CN": true,
		"": false, "d": false, "german": false, "de-": false, "1de": false, "de DE": false,
	}

	for _, location := range []string{"ClientProfile.preferredAudioLanguages", "ClientProfile.preferredSubtitleLanguages"} {
		list := requiredProperty(t, spec, location)
		if list.MaxItems == nil || *list.MaxItems != maxPreferredLanguages {
			t.Errorf("%s is not bounded to %d languages", location, maxPreferredLanguages)
		}

		pattern := regexp.MustCompile(list.Items.Value.Pattern)

		for tag, want := range tests {
			if got := pattern.MatchString(tag); got != want {
				t.Errorf("%s matches %q = %v, want %v", location, tag, got, want)
			}
		}
	}
}

// TestSubtitleRoute: the WebVTT rendition is keyed by file and track, not by session, is a
// plain handler like the other stream routes and names its failures.
func TestSubtitleRoute(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, subtitleTrackPath)

	if operation.OperationID != "getSubtitleTrack" || !slices.Contains(operation.Tags, streamTag) {
		t.Errorf("operation = %q %v, want getSubtitleTrack tagged %s", operation.OperationID, operation.Tags, streamTag)
	}

	if !slices.Contains(excludedOperationIDs(t), operation.OperationID) {
		t.Errorf("%s does not exclude %s from the strict server", generatorConfigFile, operation.OperationID)
	}

	ok := operation.Responses.Status(http.StatusOK)
	if ok == nil || ok.Value.Content.Get("text/vtt") == nil {
		t.Error("the subtitle route does not answer 200 with text/vtt")
	}

	for _, status := range []int{http.StatusNotFound, http.StatusUnprocessableEntity} {
		assertResponseSchema(t, operation.Responses.Status(status), problemMediaType, "Problem")
	}

	for _, text := range []string{`"not_found"`, `"unsupported"`, "SubtitleTrack.url"} {
		if !strings.Contains(operation.Description, text) {
			t.Errorf("the subtitle route does not name %s", text)
		}
	}

	if file := spec.Components.Parameters["FileId"]; file == nil || file.Value.Schema.Value.Format != "uuid" {
		t.Error("parameter FileId is missing or no uuid")
	}
}

// TestSpecVersionCoversPlaybackV2: the additive milestone-4 contract raised the minor version.
func TestSpecVersionCoversPlaybackV2(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 15

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
