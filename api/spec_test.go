package api_test

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"go.yaml.in/yaml/v3"

	"github.com/magpeek/mivo/internal/domain/system"
	"github.com/magpeek/mivo/internal/platform/brand"
)

// specFile is relative to this package directory, the working directory of go test.
const specFile = "openapi.yaml"

// generatorConfigFile is the oapi-codegen configuration, relative to this package directory.
const generatorConfigFile = "../internal/adapters/http/apigen/oapi-codegen.yaml"

const (
	problemMediaType = "application/problem+json"
	jsonMediaType    = "application/json"

	// problemResponseRef is the shared error response every operation must declare as default.
	problemResponseRef = "#/components/responses/Problem"

	// streamTag marks the delivery routes that are plain net/http handlers instead of
	// generated strict handlers (ADR 0024).
	streamTag = "stream"
)

// versionedPathPrefix is the path prefix every operation must carry: /api/v<major>/ with the
// major version the server reports as SystemInfo.apiVersion (system.APIVersion).
const versionedPathPrefix = "/api/v" + system.APIVersion + "/"

// loadSpec parses the contract without following external references: the spec must be
// self-contained so both generators (oapi-codegen, openapi-typescript) see the same file.
func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	spec, err := loader.LoadFromFile(specFile)
	if err != nil {
		t.Fatalf("load %s: %v", specFile, err)
	}

	return spec
}

// specOperation is one operation with its method and path.
type specOperation struct {
	method    string
	path      string
	operation *openapi3.Operation
}

func (entry specOperation) String() string {
	return entry.method + " " + entry.path
}

func (entry specOperation) isStream() bool {
	return slices.Contains(entry.operation.Tags, streamTag)
}

// allOperations lists every operation of the spec in a stable order.
func allOperations(spec *openapi3.T) []specOperation {
	var operations []specOperation

	for _, path := range spec.Paths.InMatchingOrder() {
		for method, operation := range spec.Paths.Value(path).Operations() {
			operations = append(operations, specOperation{method: method, path: path, operation: operation})
		}
	}

	slices.SortFunc(operations, func(left, right specOperation) int {
		return strings.Compare(left.String(), right.String())
	})

	return operations
}

func TestSpecIsValidOpenAPI(t *testing.T) {
	spec := loadSpec(t)

	// Validation includes the examples, so every example matches its schema.
	if err := spec.Validate(t.Context()); err != nil {
		t.Fatalf("validate %s: %v", specFile, err)
	}
}

// TestSpecHeader pins the decisions of ADR 0017: OpenAPI 3.0.3 (oapi-codegen supports 3.1
// only partially), full paths with servers "/", a document version whose major matches the
// path prefix, and the product title derived from the brand (ADR 0002 rename list).
func TestSpecHeader(t *testing.T) {
	spec := loadSpec(t)

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("openapi = %q, want 3.0.3", spec.OpenAPI)
	}

	wantTitle := brand.DisplayName + " API"
	if spec.Info.Title != wantTitle {
		t.Errorf("info.title = %q, want %q", spec.Info.Title, wantTitle)
	}

	if !strings.HasPrefix(spec.Info.Version, system.APIVersion+".") {
		t.Errorf("info.version = %q, want major version %s (system.APIVersion)", spec.Info.Version, system.APIVersion)
	}

	if len(spec.Servers) != 1 || spec.Servers[0].URL != "/" {
		t.Errorf("servers = %v, want exactly one server with url /", spec.Servers)
	}
}

// TestEveryOperationTagIsDeclared keeps the tag list, which documentation tools group by,
// complete.
func TestEveryOperationTagIsDeclared(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if len(entry.operation.Tags) == 0 {
			t.Errorf("%s has no tag", entry)
		}

		for _, tag := range entry.operation.Tags {
			if spec.Tags.Get(tag) == nil {
				t.Errorf("%s: tag %q is not declared", entry, tag)
			}
		}
	}
}

// TestEveryPathCarriesAPIVersion ties the path prefix to system.APIVersion, the value the
// server reports as SystemInfo.apiVersion: bumping one without the other fails here.
func TestEveryPathCarriesAPIVersion(t *testing.T) {
	spec := loadSpec(t)

	paths := spec.Paths.InMatchingOrder()
	if len(paths) == 0 {
		t.Fatal("the spec declares no paths")
	}

	for _, path := range paths {
		if !strings.HasPrefix(path, versionedPathPrefix) {
			t.Errorf("path %s does not start with %s (system.APIVersion = %q)", path, versionedPathPrefix, system.APIVersion)
		}
	}
}

// TestEveryOperationIsNamedAndDescribed: generated Go and TypeScript names come from the
// operationId, so it must exist for every operation (kin-openapi checks uniqueness).
func TestEveryOperationIsNamedAndDescribed(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if entry.operation.OperationID == "" {
			t.Errorf("%s has no operationId", entry)
		}

		if entry.operation.Summary == "" || entry.operation.Description == "" {
			t.Errorf("%s needs a summary and a description", entry)
		}
	}
}

// operationCase describes the expected shape of one operation.
type operationCase struct {
	method      string
	path        string
	operationID string
	tag         string
	// statuses are the success (and documented failure) statuses with a body of schema.
	statuses        []int
	mediaType       string
	schema          string
	unauthenticated bool
}

func operationCases() []operationCase {
	const (
		library  = "/api/v1/libraries/{libraryId}"
		sessions = "/api/v1/playback/sessions"
		session  = sessions + "/{sessionId}"
	)

	return []operationCase{
		{method: http.MethodGet, path: "/api/v1/health", operationID: "getHealth", tag: "system", statuses: []int{http.StatusOK, http.StatusServiceUnavailable}, mediaType: jsonMediaType, schema: "HealthResponse", unauthenticated: true},
		{method: http.MethodGet, path: "/api/v1/system/info", operationID: "getSystemInfo", tag: "system", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "SystemInfo", unauthenticated: true},
		{method: http.MethodGet, path: "/api/v1/libraries", operationID: "listLibraries", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "LibraryList"},
		{method: http.MethodGet, path: library, operationID: "getLibrary", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Library"},
		{method: http.MethodPost, path: library + "/scan", operationID: "startLibraryScan", tag: "libraries", statuses: []int{http.StatusAccepted}, mediaType: jsonMediaType, schema: "ScanStatus"},
		{method: http.MethodGet, path: library + "/items", operationID: "listLibraryItems", tag: "items", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemPage"},
		{method: http.MethodGet, path: "/api/v1/items/{itemId}", operationID: "getItem", tag: "items", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemDetail"},
		{method: http.MethodPost, path: sessions, operationID: "createPlaybackSession", tag: "playback", statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "PlaybackSession"},
		{method: http.MethodPost, path: sessions, operationID: "createPlaybackSession", tag: "playback", statuses: []int{http.StatusServiceUnavailable}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: session, operationID: "getPlaybackSession", tag: "playback", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "PlaybackSession"},
		{method: http.MethodDelete, path: session, operationID: "stopPlaybackSession", tag: "playback"},
		{method: http.MethodGet, path: session + "/file", operationID: "getPlaybackFile", tag: streamTag, statuses: []int{http.StatusOK, http.StatusPartialContent}, mediaType: "video/*", schema: "MediaBytes", unauthenticated: true},
		{method: http.MethodHead, path: session + "/file", operationID: "headPlaybackFile", tag: streamTag, unauthenticated: true},
		{method: http.MethodGet, path: session + "/hls/{fileName}", operationID: "getPlaybackHlsFile", tag: streamTag, statuses: []int{http.StatusOK, http.StatusPartialContent}, mediaType: "video/iso.segment", schema: "MediaBytes", unauthenticated: true},
		{method: http.MethodHead, path: session + "/hls/{fileName}", operationID: "headPlaybackHlsFile", tag: streamTag, unauthenticated: true},
		{method: http.MethodGet, path: session + "/hls/{fileName}", operationID: "getPlaybackHlsFile", tag: streamTag, statuses: []int{http.StatusServiceUnavailable}, mediaType: problemMediaType, schema: "Problem", unauthenticated: true},
	}
}

func TestSpecOperations(t *testing.T) {
	spec := loadSpec(t)

	for _, test := range operationCases() {
		t.Run(fmt.Sprintf("%s %s %v", test.method, test.path, test.statuses), func(t *testing.T) {
			operation := getOperation(t, spec, test.method, test.path)

			if operation.OperationID != test.operationID {
				t.Errorf("operationId = %q, want %q", operation.OperationID, test.operationID)
			}

			if !slices.Contains(operation.Tags, test.tag) {
				t.Errorf("tags = %v, want %s", operation.Tags, test.tag)
			}

			for _, status := range test.statuses {
				assertResponseSchema(t, operation.Responses.Status(status), test.mediaType, test.schema)
			}

			if test.unauthenticated {
				assertUnauthenticated(t, operation)
			}
		})
	}
}

func getOperation(t *testing.T, spec *openapi3.T, method, path string) *openapi3.Operation {
	t.Helper()

	item := spec.Paths.Value(path)
	if item == nil || item.GetOperation(method) == nil {
		t.Fatalf("%s %s is not declared", method, path)
	}

	return item.GetOperation(method)
}

// assertUnauthenticated requires an explicit empty security list, so a global security
// requirement added later cannot silently lock out readiness probes and client discovery.
func assertUnauthenticated(t *testing.T, operation *openapi3.Operation) {
	t.Helper()

	if operation.Security == nil || len(*operation.Security) != 0 {
		t.Errorf("security = %s, want an explicit empty list", describeSecurity(operation.Security))
	}

	if !strings.Contains(operation.Description, "unauthenticated") {
		t.Errorf("description %q does not state that the endpoint is unauthenticated", operation.Description)
	}
}

// describeSecurity prints the requirements themselves instead of the pointer address that
// %v would show for *openapi3.SecurityRequirements.
func describeSecurity(security *openapi3.SecurityRequirements) string {
	if security == nil {
		return "absent (inherits the global security)"
	}

	return fmt.Sprintf("%v", *security)
}

func assertResponseSchema(t *testing.T, response *openapi3.ResponseRef, mediaType, schema string) {
	t.Helper()

	if response == nil || response.Value == nil {
		t.Fatalf("response with %s %s is not declared", mediaType, schema)
	}

	content := response.Value.Content.Get(mediaType)
	if content == nil || content.Schema == nil {
		t.Fatalf("response has no %s content", mediaType)
	}

	want := "#/components/schemas/" + schema
	if content.Schema.Ref != want {
		t.Errorf("%s schema = %q, want %q", mediaType, content.Schema.Ref, want)
	}
}

// TestEveryOperationDeclaresProblemDefault keeps the error contract uniform: whatever status
// an operation fails with, clients can parse the body as a problem. Stream operations are
// exempt because their success bodies are media, not JSON; they declare it anyway.
func TestEveryOperationDeclaresProblemDefault(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if entry.isStream() {
			continue
		}

		response := entry.operation.Responses.Default()
		if response == nil || response.Ref != problemResponseRef {
			t.Errorf("%s: default response is not %s", entry, problemResponseRef)
		}
	}
}

// TestStreamOperationsAreExcludedFromTheStrictServer keeps oapi-codegen.yaml in step with
// the stream tag: a stream route the generator still emitted would clash with its mount,
// and a non-stream route left out would have no handler.
func TestStreamOperationsAreExcludedFromTheStrictServer(t *testing.T) {
	spec := loadSpec(t)

	var streamOperationIDs []string

	for _, entry := range allOperations(spec) {
		if entry.isStream() {
			streamOperationIDs = append(streamOperationIDs, entry.operation.OperationID)
			assertUnauthenticated(t, entry.operation)
		}
	}

	excluded := excludedOperationIDs(t)

	slices.Sort(streamOperationIDs)
	slices.Sort(excluded)

	if !slices.Equal(streamOperationIDs, excluded) {
		t.Errorf("stream operations %v, but %s excludes %v", streamOperationIDs, generatorConfigFile, excluded)
	}
}

func excludedOperationIDs(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile(generatorConfigFile)
	if err != nil {
		t.Fatalf("read %s: %v", generatorConfigFile, err)
	}

	var config struct {
		OutputOptions struct {
			ExcludeOperationIDs []string `yaml:"exclude-operation-ids"`
		} `yaml:"output-options"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse %s: %v", generatorConfigFile, err)
	}

	return config.OutputOptions.ExcludeOperationIDs
}

// TestProblemFollowsRFC9457 checks the error contract every client relies on: the RFC 9457
// members plus a required machine-readable code, reusable as a components response.
func TestProblemFollowsRFC9457(t *testing.T) {
	spec := loadSpec(t)

	problem := spec.Components.Schemas["Problem"]
	if problem == nil || problem.Value == nil {
		t.Fatal("components.schemas.Problem is not declared")
	}

	for _, property := range []string{"type", "title", "status", "detail", "instance", "code"} {
		if problem.Value.Properties[property] == nil {
			t.Errorf("Problem has no property %q", property)
		}
	}

	if !slices.Contains(problem.Value.Required, "code") {
		t.Errorf("Problem.required = %v, want it to contain code", problem.Value.Required)
	}

	assertResponseSchema(t, spec.Components.Responses["Problem"], problemMediaType, "Problem")
}

// TestPublishedProblemCodesAreDocumented lists the codes of the restapi constants
// (internal/adapters/http/restapi/problem.go pins the same list), so clients find every
// code they may receive in Problem.code.
func TestPublishedProblemCodesAreDocumented(t *testing.T) {
	spec := loadSpec(t)

	description := spec.Components.Schemas["Problem"].Value.Properties["code"].Value.Description

	codes := map[string]int{
		"bad_request": http.StatusBadRequest, "invalid": http.StatusBadRequest,
		"forbidden": http.StatusForbidden, "not_found": http.StatusNotFound,
		"method_not_allowed": http.StatusMethodNotAllowed, "conflict": http.StatusConflict,
		"payload_too_large": http.StatusRequestEntityTooLarge, "unsupported": http.StatusUnprocessableEntity,
		"no_compatible_format": http.StatusUnprocessableEntity, "transcode_unavailable": http.StatusUnprocessableEntity,
		"internal_error": http.StatusInternalServerError, "unavailable": http.StatusServiceUnavailable,
		"session_limit_reached": http.StatusServiceUnavailable, "stream_not_ready": http.StatusServiceUnavailable,
	}

	for code, status := range codes {
		documented := fmt.Sprintf(`"%s" (%d`, code, status)
		if !strings.Contains(description, documented) {
			t.Errorf("Problem.code description does not document %s)", documented)
		}
	}
}

// TestEverySchemaIsDescribed enforces the task rule that every schema and property has a
// description: the generated Go and TypeScript code and third-party users rely on it.
func TestEverySchemaIsDescribed(t *testing.T) {
	spec := loadSpec(t)

	for name, schema := range spec.Components.Schemas {
		if schema.Value.Description == "" {
			t.Errorf("schema %s has no description", name)
		}

		assertNestedSchemasDescribed(t, name, schema.Value)
	}
}

// assertNestedSchemasDescribed walks the inline schemas below parent (properties, array
// items, additionalProperties). References are skipped: their target is a component schema
// checked on its own, which also keeps the walk finite for recursive types.
func assertNestedSchemasDescribed(t *testing.T, location string, parent *openapi3.Schema) {
	t.Helper()

	for childLocation, child := range nestedSchemas(location, parent) {
		if child == nil || child.Ref != "" || child.Value == nil {
			continue
		}

		if child.Value.Description == "" {
			t.Errorf("schema %s has no description", childLocation)
		}

		assertNestedSchemasDescribed(t, childLocation, child.Value)
	}
}

// nestedSchemas returns the direct child schemas of parent keyed by their location.
func nestedSchemas(location string, parent *openapi3.Schema) map[string]*openapi3.SchemaRef {
	nested := make(map[string]*openapi3.SchemaRef, len(parent.Properties)+2)
	for propertyName, property := range parent.Properties {
		nested[location+"."+propertyName] = property
	}

	nested[location+"[items]"] = parent.Items
	nested[location+"{additionalProperties}"] = parent.AdditionalProperties.Schema

	return nested
}

// TestEnumsAreExtensible: enums grow within API v1, so every enum must tell clients to
// tolerate new values (ADR 0017).
func TestEnumsAreExtensible(t *testing.T) {
	spec := loadSpec(t)

	for name, schema := range spec.Components.Schemas {
		if len(schema.Value.Enum) > 0 && !strings.Contains(schema.Value.Description, "Clients must tolerate") {
			t.Errorf("enum %s does not tell clients to tolerate new values", name)
		}

		for propertyName, property := range schema.Value.Properties {
			if property.Ref == "" && len(property.Value.Enum) > 0 {
				t.Errorf("%s.%s has an inline enum; declare it as a component schema", name, propertyName)
			}
		}
	}
}

// TestIDsAreUUIDs: every ID in paths and bodies is a UUID, so a malformed ID is rejected by
// the binder and stream URLs cannot be guessed (ADR 0024).
func TestIDsAreUUIDs(t *testing.T) {
	spec := loadSpec(t)

	for name, parameter := range spec.Components.Parameters {
		if strings.HasSuffix(parameter.Value.Name, "Id") && parameter.Value.Schema.Value.Format != "uuid" {
			t.Errorf("parameter %s is not format uuid", name)
		}
	}

	for name, schema := range spec.Components.Schemas {
		for propertyName, property := range schema.Value.Properties {
			isID := propertyName == "id" || strings.HasSuffix(propertyName, "Id")
			if isID && property.Value.Format != "uuid" {
				t.Errorf("%s.%s is not format uuid", name, propertyName)
			}
		}
	}
}

// TestClientProfileIsBounded: the profile is the largest client input; every array and
// free string in it has an upper bound (ADR 0023, ADR 0024).
func TestClientProfileIsBounded(t *testing.T) {
	spec := loadSpec(t)

	assertBounded(t, "ClientProfile", spec.Components.Schemas["ClientProfile"])
}

// assertBounded follows references too; the profile has no recursive types.
func assertBounded(t *testing.T, location string, schemaRef *openapi3.SchemaRef) {
	t.Helper()

	schema := schemaRef.Value

	switch {
	case schema.Type.Is(openapi3.TypeArray) && schema.MaxItems == nil:
		t.Errorf("array %s has no maxItems", location)
	case schema.Type.Is(openapi3.TypeString) && len(schema.Enum) == 0 && schema.MaxLength == nil:
		t.Errorf("string %s has no maxLength", location)
	}

	for childLocation, child := range nestedSchemas(location, schema) {
		if child != nil {
			assertBounded(t, childLocation, child)
		}
	}
}

func TestMediaIdentifierPattern(t *testing.T) {
	spec := loadSpec(t)

	pattern := regexp.MustCompile(spec.Components.Schemas["MediaIdentifier"].Value.Pattern)

	tests := map[string]bool{
		"h264": true, "pcm_s16le": true, "mpeg2video": true, "3gp": true, "mpegts": true, "e-ac-3": true,
		"H264": false, "": false, "-h264": false, "h 264": false, "video/mp4": false,
		strings.Repeat("a", 33): false,
	}

	for identifier, want := range tests {
		if got := pattern.MatchString(identifier); got != want {
			t.Errorf("MediaIdentifier matches %q = %v, want %v", identifier, got, want)
		}
	}
}

// TestHLSFileNamePattern pins the file names a session directory may serve (ADR 0024).
func TestHLSFileNamePattern(t *testing.T) {
	spec := loadSpec(t)

	const hlsPath = "/api/v1/playback/sessions/{sessionId}/hls/{fileName}"

	getOperation(t, spec, http.MethodGet, hlsPath)

	parameter := spec.Paths.Value(hlsPath).Parameters.GetByInAndName("path", "fileName")
	if parameter == nil {
		t.Fatal("path parameter fileName is not declared on the path")
	}

	pattern := regexp.MustCompile(parameter.Schema.Value.Pattern)

	tests := map[string]bool{
		"index.m3u8": true, "init.mp4": true, "seg-00000.m4s": true, "seg-12345.m4s": true,
		"INDEX.M3U8": false, "seg-1.m4s": false, "seg-000000.m4s": false, "../x": false,
		"%2e%2e/x": false, "index.m3u8.tmp": false, "xindex.m3u8": false, "seg-0000a.m4s": false,
	}

	for name, want := range tests {
		if got := pattern.MatchString(name); got != want {
			t.Errorf("fileName pattern matches %q = %v, want %v", name, got, want)
		}
	}
}

// TestPagingParameters pins the bounds of listLibraryItems (ADR 0022).
func TestPagingParameters(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, "/api/v1/libraries/{libraryId}/items")

	limit := operation.Parameters.GetByInAndName("query", "limit")
	if limit == nil {
		t.Fatal("query parameter limit is not declared")
	}

	schema := limit.Schema.Value

	const wantMinimum, wantMaximum, wantDefault = 1, 500, 100
	if schema.Min == nil || *schema.Min != wantMinimum || schema.Max == nil || *schema.Max != wantMaximum || schema.Default != float64(wantDefault) {
		t.Errorf("limit = min %v max %v default %v, want %d–%d default %d", schema.Min, schema.Max, schema.Default, wantMinimum, wantMaximum, wantDefault)
	}

	if cursor := operation.Parameters.GetByInAndName("query", "cursor"); cursor == nil || cursor.Required {
		t.Error("optional query parameter cursor is not declared")
	}
}

// TestStreamURLIsOpaque: clients must never build or parse stream URLs, so milestone 7 can
// sign them without breaking v1 clients (ADR 0024).
func TestStreamURLIsOpaque(t *testing.T) {
	spec := loadSpec(t)

	streamURL := spec.Components.Schemas["PlaybackSession"].Value.Properties["streamUrl"]
	if streamURL == nil || !strings.Contains(streamURL.Value.Description, "opaque") {
		t.Error("PlaybackSession.streamUrl does not tell clients to treat it as opaque")
	}
}

// streamURLAsSource matches wording that sends clients to streamUrl for a value, such as
// "taken from its streamUrl" or "the streamUrl's ID".
var streamURLAsSource = regexp.MustCompile(`(?i)(from|of|in|out of) (the |its |a |the session's )?streamUrl|streamUrl's`)

// TestNoValueIsTakenFromStreamURL keeps streamUrl opaque (ADR 0024): no parameter or
// property may mention it, and no other text may present it as a source of values, so
// milestone 7 can add a token segment without breaking clients.
func TestNoValueIsTakenFromStreamURL(t *testing.T) {
	spec := loadSpec(t)

	for location, description := range valueDescriptions(spec) {
		if strings.Contains(description, "streamUrl") {
			t.Errorf("%s mentions streamUrl: %q", location, description)
		}
	}

	for location, description := range otherDescriptions(spec) {
		if streamURLAsSource.MatchString(description) {
			t.Errorf("%s presents streamUrl as a source of values: %q", location, description)
		}
	}
}

// valueDescriptions returns the descriptions of every parameter and every schema property
// except streamUrl itself, keyed by location.
func valueDescriptions(spec *openapi3.T) map[string]string {
	descriptions := parameterDescriptions(spec)

	for name, schema := range spec.Components.Schemas {
		for propertyName, property := range schema.Value.Properties {
			// A referenced schema is checked on its own in otherDescriptions.
			if property.Ref != "" || (name == "PlaybackSession" && propertyName == "streamUrl") {
				continue
			}

			descriptions[name+"."+propertyName] = property.Value.Description
		}
	}

	return descriptions
}

// parameterDescriptions returns the descriptions of the component, path and operation
// parameters, keyed by location.
func parameterDescriptions(spec *openapi3.T) map[string]string {
	descriptions := map[string]string{}

	for name, parameter := range spec.Components.Parameters {
		descriptions["parameter "+name] = parameter.Value.Description
	}

	for _, path := range spec.Paths.InMatchingOrder() {
		item := spec.Paths.Value(path)
		for _, parameter := range item.Parameters {
			descriptions["parameter "+parameter.Value.Name+" of "+path] = parameter.Value.Description
		}
	}

	for _, entry := range allOperations(spec) {
		for _, parameter := range entry.operation.Parameters {
			descriptions["parameter "+parameter.Value.Name+" of "+entry.String()] = parameter.Value.Description
		}
	}

	return descriptions
}

// otherDescriptions returns the descriptions of the document, tags, operations and schemas.
func otherDescriptions(spec *openapi3.T) map[string]string {
	descriptions := map[string]string{"info": spec.Info.Description}

	for _, tag := range spec.Tags {
		descriptions["tag "+tag.Name] = tag.Description
	}

	for _, entry := range allOperations(spec) {
		descriptions[entry.String()] = entry.operation.Description
	}

	for name, schema := range spec.Components.Schemas {
		descriptions["schema "+name] = schema.Value.Description
	}

	return descriptions
}

// TestMediaFileExposesNoPath: the API never exposes file system paths (ADR 0022).
func TestMediaFileExposesNoPath(t *testing.T) {
	spec := loadSpec(t)

	for name, schema := range spec.Components.Schemas {
		for propertyName := range schema.Value.Properties {
			if strings.Contains(strings.ToLower(propertyName), "path") {
				t.Errorf("%s.%s looks like a file system path", name, propertyName)
			}
		}
	}
}
