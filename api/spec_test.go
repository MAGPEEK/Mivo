package api_test

import (
	"fmt"
	"maps"
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
	// imagesTag marks the image route, also a plain net/http handler (ADR 0027 section 8).
	imagesTag = "images"
	// metadataTag groups the online metadata operations of milestone 6 (ADR 0031).
	metadataTag = "metadata"

	// metadataSettingsPath is the route of the online metadata settings.
	metadataSettingsPath = "/api/v1/settings/metadata"
)

// plainHandlerTags are the tags whose operations oapi-codegen must not generate: they
// serve bytes with range or caching semantics the strict server cannot express.
func plainHandlerTags() []string {
	return []string{streamTag, imagesTag}
}

// imagePath is the route of the image endpoint (ADR 0027 section 8).
const imagePath = "/api/v1/images/{imageId}"

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

// isPlainHandler reports whether the operation is served by a plain net/http handler
// outside the generated strict server.
func (entry specOperation) isPlainHandler() bool {
	return slices.ContainsFunc(entry.operation.Tags, func(tag string) bool {
		return slices.Contains(plainHandlerTags(), tag)
	})
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
		item     = "/api/v1/items/{itemId}"
		sessions = "/api/v1/playback/sessions"
		session  = sessions + "/{sessionId}"
	)

	return []operationCase{
		{method: http.MethodGet, path: "/api/v1/health", operationID: "getHealth", tag: "system", statuses: []int{http.StatusOK, http.StatusServiceUnavailable}, mediaType: jsonMediaType, schema: "HealthResponse", unauthenticated: true},
		{method: http.MethodGet, path: "/api/v1/system/info", operationID: "getSystemInfo", tag: "system", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "SystemInfo", unauthenticated: true},
		{method: http.MethodGet, path: "/api/v1/libraries", operationID: "listLibraries", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "LibraryList"},
		{method: http.MethodPost, path: "/api/v1/libraries", operationID: "createLibrary", tag: "libraries", statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "Library"},
		{method: http.MethodPatch, path: library, operationID: "updateLibrary", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Library"},
		{method: http.MethodDelete, path: library, operationID: "deleteLibrary", tag: "libraries"},
		{method: http.MethodGet, path: "/api/v1/folder-roots", operationID: "listFolderRoots", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "FolderRootList"},
		{method: http.MethodGet, path: "/api/v1/folder-roots/{rootId}/folders", operationID: "listFolders", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "FolderListing"},
		{method: http.MethodGet, path: library, operationID: "getLibrary", tag: "libraries", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Library"},
		{method: http.MethodPost, path: library + "/scan", operationID: "startLibraryScan", tag: "libraries", statuses: []int{http.StatusAccepted}, mediaType: jsonMediaType, schema: "ScanStatus"},
		{method: http.MethodGet, path: library + "/items", operationID: "listLibraryItems", tag: "items", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemPage"},
		{method: http.MethodGet, path: "/api/v1/items/{itemId}", operationID: "getItem", tag: "items", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemDetail"},
		{method: http.MethodGet, path: "/api/v1/items/{itemId}/children", operationID: "listItemChildren", tag: "items", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemPage"},
		{method: http.MethodGet, path: imagePath, operationID: "getImage", tag: imagesTag, statuses: []int{http.StatusOK}, mediaType: "image/jpeg", schema: "ImageBytes", unauthenticated: true},
		{method: http.MethodGet, path: imagePath, operationID: "getImage", tag: imagesTag, statuses: []int{http.StatusOK}, mediaType: "image/png", schema: "ImageBytes", unauthenticated: true},
		{method: http.MethodPost, path: sessions, operationID: "createPlaybackSession", tag: "playback", statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "PlaybackSession"},
		{method: http.MethodPost, path: sessions, operationID: "createPlaybackSession", tag: "playback", statuses: []int{http.StatusServiceUnavailable}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: session, operationID: "getPlaybackSession", tag: "playback", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "PlaybackSession"},
		{method: http.MethodDelete, path: session, operationID: "stopPlaybackSession", tag: "playback"},
		{method: http.MethodGet, path: session + "/file", operationID: "getPlaybackFile", tag: streamTag, statuses: []int{http.StatusOK, http.StatusPartialContent}, mediaType: "video/*", schema: "MediaBytes", unauthenticated: true},
		{method: http.MethodHead, path: session + "/file", operationID: "headPlaybackFile", tag: streamTag, unauthenticated: true},
		{method: http.MethodGet, path: session + "/hls/{fileName}", operationID: "getPlaybackHlsFile", tag: streamTag, statuses: []int{http.StatusOK, http.StatusPartialContent}, mediaType: "video/iso.segment", schema: "MediaBytes", unauthenticated: true},
		{method: http.MethodHead, path: session + "/hls/{fileName}", operationID: "headPlaybackHlsFile", tag: streamTag, unauthenticated: true},
		{method: http.MethodGet, path: session + "/hls/{fileName}", operationID: "getPlaybackHlsFile", tag: streamTag, statuses: []int{http.StatusServiceUnavailable}, mediaType: problemMediaType, schema: "Problem", unauthenticated: true},
		{method: http.MethodPost, path: item + "/identify/search", operationID: "searchIdentifyCandidates", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "IdentifyCandidateList"},
		{method: http.MethodPost, path: item + "/identify", operationID: "identifyItem", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemDetail"},
		{method: http.MethodDelete, path: item + "/match", operationID: "removeItemMatch", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ItemDetail"},
		{method: http.MethodPost, path: item + "/refresh", operationID: "refreshItemMetadata", tag: metadataTag},
		{method: http.MethodPost, path: library + "/refresh", operationID: "refreshLibraryMetadata", tag: metadataTag},
		{method: http.MethodGet, path: metadataSettingsPath, operationID: "getMetadataSettings", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "MetadataSettings"},
		{method: http.MethodPut, path: metadataSettingsPath, operationID: "updateMetadataSettings", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "MetadataSettings"},
		{method: http.MethodGet, path: "/api/v1/metadata/status", operationID: "getMetadataStatus", tag: metadataTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "MetadataStatus"},
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
// an operation fails with, clients can parse the body as a problem. Stream and image
// operations are exempt because their success bodies are bytes, not JSON; they declare it
// anyway.
func TestEveryOperationDeclaresProblemDefault(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if entry.isPlainHandler() {
			continue
		}

		response := entry.operation.Responses.Default()
		if response == nil || response.Ref != problemResponseRef {
			t.Errorf("%s: default response is not %s", entry, problemResponseRef)
		}
	}
}

// TestPlainHandlerOperationsAreExcludedFromTheStrictServer keeps oapi-codegen.yaml in step
// with the stream and images tags: a plain-handler route the generator still emitted would
// clash with its mount, and any other route left out would have no handler.
func TestPlainHandlerOperationsAreExcludedFromTheStrictServer(t *testing.T) {
	spec := loadSpec(t)

	var plainOperationIDs []string

	for _, entry := range allOperations(spec) {
		if entry.isPlainHandler() {
			plainOperationIDs = append(plainOperationIDs, entry.operation.OperationID)
			assertUnauthenticated(t, entry.operation)
		}
	}

	excluded := excludedOperationIDs(t)

	slices.Sort(plainOperationIDs)
	slices.Sort(excluded)

	if !slices.Equal(plainOperationIDs, excluded) {
		t.Errorf("stream and images operations %v, but %s excludes %v", plainOperationIDs, generatorConfigFile, excluded)
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
		"folder_not_found": http.StatusBadRequest, "folder_unreadable": http.StatusBadRequest,
		"folder_overlap": http.StatusConflict, "folder_reserved": http.StatusBadRequest,
		"folder_outside_roots": http.StatusBadRequest, "kind_unsupported": http.StatusBadRequest,
		"name_invalid": http.StatusBadRequest, "language_invalid": http.StatusBadRequest,
		"too_many_folders": http.StatusBadRequest,
		"online_disabled":  http.StatusConflict, "provider_unavailable": http.StatusServiceUnavailable,
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
			if isID && property.Value.Format != "uuid" && !isProviderScopedID(name, propertyName) {
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

// TestPagingParameters pins the bounds of the paged item listings (ADR 0022, ADR 0027).
func TestPagingParameters(t *testing.T) {
	spec := loadSpec(t)

	for _, path := range []string{"/api/v1/libraries/{libraryId}/items", "/api/v1/items/{itemId}/children"} {
		t.Run(path, func(t *testing.T) {
			assertPagingParameters(t, getOperation(t, spec, http.MethodGet, path))
		})
	}
}

// TestChildrenOrderParameter pins the optional sort direction of listItemChildren.
func TestChildrenOrderParameter(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, "/api/v1/items/{itemId}/children")

	order := operation.Parameters.GetByInAndName("query", "order")
	if order == nil || order.Required {
		t.Fatal("optional query parameter order is not declared")
	}

	if order.Schema.Ref != "#/components/schemas/SortOrder" {
		t.Errorf("order schema = %q, want SortOrder", order.Schema.Ref)
	}

	assertEnum(t, "SortOrder", order.Schema.Value, []any{"asc", "desc"})

	if order.Schema.Value.Default != "asc" {
		t.Errorf("order default = %v, want asc", order.Schema.Value.Default)
	}
}

func assertPagingParameters(t *testing.T, operation *openapi3.Operation) {
	t.Helper()

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
// "taken from its streamUrl" or "the streamUrl's ID". The word boundaries keep words that
// merely end in one of the prepositions ("proof the streamUrl", "begin streamUrl") out.
var streamURLAsSource = regexp.MustCompile(`(?i)\b(from|of|in|out of) (the |its |a |the session's )?streamUrl\b|\bstreamUrl's\b`)

func TestStreamURLAsSourcePattern(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"taken from its streamUrl":        true,
		"the ID out of the streamUrl":     true,
		"the value in a streamUrl":        true,
		"part of the session's streamUrl": true,
		"the streamUrl's session ID":      true,
		"FROM STREAMURL":                  true,
		"clients load streamUrl":          false,
		"proof the streamUrl":             false,
		"begin streamUrl":                 false,
		"from the streamUrlBase":          false,
	}

	for text, want := range tests {
		if got := streamURLAsSource.MatchString(text); got != want {
			t.Errorf("streamURLAsSource matches %q = %v, want %v", text, got, want)
		}
	}
}

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

// otherDescriptions returns the descriptions of the document, tags, operations, responses,
// response headers and schemas.
func otherDescriptions(spec *openapi3.T) map[string]string {
	descriptions := map[string]string{"info": spec.Info.Description}

	for _, tag := range spec.Tags {
		descriptions["tag "+tag.Name] = tag.Description
	}

	for _, entry := range allOperations(spec) {
		descriptions[entry.String()] = entry.operation.Description

		for status, response := range entry.operation.Responses.Map() {
			addResponseDescriptions(descriptions, entry.String()+" "+status, response)
		}
	}

	for name, response := range spec.Components.Responses {
		addResponseDescriptions(descriptions, "response "+name, response)
	}

	for name, header := range spec.Components.Headers {
		descriptions["header "+name] = header.Value.Description
	}

	for name, schema := range spec.Components.Schemas {
		descriptions["schema "+name] = schema.Value.Description
	}

	return descriptions
}

// addResponseDescriptions adds the description of a response and of its headers.
func addResponseDescriptions(descriptions map[string]string, location string, response *openapi3.ResponseRef) {
	if response.Value.Description != nil {
		descriptions[location] = *response.Value.Description
	}

	for name, header := range response.Value.Headers {
		descriptions[location+" header "+name] = header.Value.Description
	}
}

// TestMediaFileExposesNoPath: the API never exposes file system paths (ADR 0022).
func TestMediaFileExposesNoPath(t *testing.T) {
	spec := loadSpec(t)

	for name, schema := range spec.Components.Schemas {
		if slices.Contains(folderBrowserSchemas(), name) {
			continue
		}

		for propertyName := range schema.Value.Properties {
			if strings.Contains(strings.ToLower(propertyName), "path") {
				t.Errorf("%s.%s looks like a file system path", name, propertyName)
			}
		}
	}
}

// folderBrowserSchemas are the schemas that carry root-relative folder paths of the library
// management API (ADR 0028); they never hold an absolute media file path.
func folderBrowserSchemas() []string {
	return []string{"LibraryFolder", "FolderRef", "FolderRoot", "FolderEntry", "FolderListing"}
}

// TestLibraryManagementContract pins the schemas of task M3-19a (ADR 0028).
func TestLibraryManagementContract(t *testing.T) {
	spec := loadSpec(t)

	shapes := map[string]schemaShape{
		"Library":        {required: []string{"folders", "metadataLanguage", "options", "importedFromConfig"}},
		"LibraryFolder":  {required: []string{"id", "path", "available"}},
		"LibraryOptions": {required: []string{"scanExtras", "readNfo", "localArtwork"}, optional: []string{"onlineMetadata"}},
		"LibraryCreate":  {required: []string{"kind", "name", "folders"}, optional: []string{"metadataLanguage", "options"}},
		"LibraryUpdate":  {optional: []string{"name", "folders", "metadataLanguage", "options"}},
		"FolderRef":      {optional: []string{"folderId", "rootId", "path"}},
		"FolderRoot":     {required: []string{"id", "path", "available"}},
		"FolderRootList": {required: []string{"roots", "installKind", "serviceUser"}},
		"FolderListing":  {required: []string{"rootId", "path", "folders", "unreadableCount", "truncated"}, optional: []string{"parentPath"}},
		"FolderEntry":    {required: []string{"name", "path", "selectable"}, optional: []string{"reason"}},
		"ScanStatus":     {required: []string{"state", "counts", "unavailableFolderCount"}},
		"SystemInfo":     {required: []string{"libraryKinds"}},
		"ItemDetail":     {optional: []string{"number", "numberEnd"}},
	}

	for name, shape := range shapes {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}

	for _, name := range []string{"LibraryCreate", "LibraryUpdate"} {
		additional := spec.Components.Schemas[name].Value.AdditionalProperties
		if additional.Has == nil || *additional.Has {
			t.Errorf("%s must set additionalProperties: false", name)
		}
	}

	// Per-library online switch of ADR 0031 section 2: optional because create and update
	// reuse LibraryOptions, and on when omitted. The default is stated in the description,
	// not as a schema default: openapi-typescript would make a defaulted property required,
	// which would force every client request to send it.
	online := propertySchema(spec, "LibraryOptions.onlineMetadata")
	if online == nil || online.Value.Default != nil || !strings.Contains(online.Value.Description, "ADR 0031") ||
		!strings.Contains(online.Value.Description, "Omitted means on") {
		t.Error("LibraryOptions.onlineMetadata must cite ADR 0031 and say that omitted means on, without a schema default")
	}

	if spec.Components.Schemas["LibraryUpdate"].Value.Properties["kind"] != nil {
		t.Error("LibraryUpdate must not offer kind")
	}

	if minProps := spec.Components.Schemas["LibraryUpdate"].Value.MinProps; minProps < 1 {
		t.Errorf("LibraryUpdate.minProperties = %d, want at least 1", minProps)
	}

	for _, location := range []string{"LibraryCreate.folders", "LibraryUpdate.folders"} {
		folders := propertySchema(spec, location).Value
		if folders.MinItems != 1 || folders.MaxItems == nil || *folders.MaxItems != 16 {
			t.Errorf("%s bounds = %d..%v, want 1..16", location, folders.MinItems, folders.MaxItems)
		}
	}
}

// TestFileNameDescriptionNamesBackslash pins the BACKLOG note of M3-06b.
func TestFileNameDescriptionNamesBackslash(t *testing.T) {
	spec := loadSpec(t)

	description := propertySchema(spec, "MediaFile.fileName").Value.Description
	if !strings.Contains(description, "backslash") {
		t.Errorf("MediaFile.fileName does not say that a backslash separates directories: %q", description)
	}
}

// assertEnum compares the enum values of schema with want; integers are compared by their
// text because the YAML loader reads them as float64.
func assertEnum(t *testing.T, location string, schema *openapi3.Schema, want []any) {
	t.Helper()

	if !slices.Equal(enumTexts(schema.Enum), enumTexts(want)) {
		t.Errorf("%s enum = %v, want %v", location, schema.Enum, want)
	}
}

func enumTexts(values []any) []string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, fmt.Sprint(value))
	}

	return texts
}

// TestLibraryV2Enums pins the value sets of ADR 0027 (sections 2, 4, 5 and 8).
func TestLibraryV2Enums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]any{
		"ItemKind":    {"movie", "show", "season", "episode", "extra"},
		"ExtraType":   {"trailer", "featurette", "behind_the_scenes", "deleted_scene", "interview", "scene", "short", "other"},
		"ArtworkKind": {"poster", "backdrop", "thumb", "logo", "banner"},
		"PersonRole":  {"actor", "director", "writer", "producer", "guest_star"},
		"ImageWidth":  {160, 320, 480, 640, 960, 1280, 1920},
		"SortOrder":   {"asc", "desc"},
	}

	for name, want := range tests {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertEnum(t, name, schema.Value, want)
	}

	if width := spec.Components.Schemas["ImageWidth"]; width == nil || !width.Value.Type.Is(openapi3.TypeInteger) {
		t.Error("ImageWidth is not an integer enum")
	}
}

// schemaShape lists the required and optional properties a schema must declare.
type schemaShape struct {
	required []string
	optional []string
}

func libraryV2Shapes() map[string]schemaShape {
	return map[string]schemaShape{
		"ImageRef":     {required: []string{"id", "tag"}},
		"ProviderIds":  {optional: []string{"tmdb", "imdb", "tvdb"}},
		"ItemPerson":   {required: []string{"name", "role"}, optional: []string{"character"}},
		"ItemAncestor": {required: []string{"id", "kind", "title"}, optional: []string{"number"}},
		"Artwork":      {optional: []string{"poster", "backdrop", "thumb", "logo", "banner"}},
		"ItemSummary": {
			required: []string{"id", "libraryId", "kind", "title"},
			optional: []string{
				"year", "parentId", "number", "numberEnd", "absoluteNumber", "extraType",
				"childCount", "leafCount", "poster", "thumb", "media",
			},
		},
		"ItemDetail": {
			required: []string{"id", "libraryId", "kind", "title", "files"},
			optional: []string{
				"originalTitle", "year", "releaseDate", "summary", "tagline", "contentRating",
				"genres", "studios", "countries", "people", "providerIds", "artwork", "ancestors",
				"previousId", "nextId", "playItemId", "extras", "childCount", "leafCount",
			},
		},
		"MediaFile": {optional: []string{"versionLabel", "partNumber"}},
	}
}

// TestLibraryV2SchemaShapes pins the item model of the milestone-3 contract (task M3-02).
func TestLibraryV2SchemaShapes(t *testing.T) {
	spec := loadSpec(t)

	for name, shape := range libraryV2Shapes() {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}
}

func assertSchemaShape(t *testing.T, name string, schema *openapi3.Schema, shape schemaShape) {
	t.Helper()

	for _, property := range slices.Concat(shape.required, shape.optional) {
		if schema.Properties[property] == nil {
			t.Errorf("%s has no property %q", name, property)
		}
	}

	for _, property := range shape.required {
		if !slices.Contains(schema.Required, property) {
			t.Errorf("%s.%s is not required", name, property)
		}
	}

	for _, property := range shape.optional {
		if slices.Contains(schema.Required, property) {
			t.Errorf("%s.%s is required, want optional", name, property)
		}
	}
}

// propertySchema returns the schema of "Schema.property", or nil when either is missing.
func propertySchema(spec *openapi3.T, location string) *openapi3.SchemaRef {
	schemaName, propertyName, _ := strings.Cut(location, ".")

	schema := spec.Components.Schemas[schemaName]
	if schema == nil {
		return nil
	}

	return schema.Value.Properties[propertyName]
}

// TestLibraryV2PropertyReferences pins which component schema a property uses, so the
// generated types stay shared between summary, detail and the image helper.
func TestLibraryV2PropertyReferences(t *testing.T) {
	spec := loadSpec(t)

	references := map[string]string{
		"ItemSummary.kind": "ItemKind", "ItemSummary.extraType": "ExtraType",
		"ItemSummary.poster": "ImageRef", "ItemSummary.thumb": "ImageRef",
		"ItemDetail.kind": "ItemKind", "ItemDetail.providerIds": "ProviderIds",
		"ItemDetail.artwork": "Artwork", "ItemAncestor.kind": "ItemKind", "ItemPerson.role": "PersonRole",
		"Artwork.poster": "ImageRef", "Artwork.backdrop": "ImageRef", "Artwork.thumb": "ImageRef",
		"Artwork.logo": "ImageRef", "Artwork.banner": "ImageRef",
	}

	for location, want := range references {
		if property := propertySchema(spec, location); property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}

	arrays := map[string]string{
		"ItemDetail.people": "ItemPerson", "ItemDetail.ancestors": "ItemAncestor", "ItemDetail.extras": "ItemSummary",
	}

	for location, want := range arrays {
		property := propertySchema(spec, location)
		if property == nil || property.Value.Items == nil || property.Value.Items.Ref != "#/components/schemas/"+want {
			t.Errorf("%s is not an array of %s", location, want)
		}
	}
}

// TestArtworkIsKeyedByArtworkKind: every artwork kind has exactly one slot in Artwork.
func TestArtworkIsKeyedByArtworkKind(t *testing.T) {
	spec := loadSpec(t)

	kinds, artwork := spec.Components.Schemas["ArtworkKind"], spec.Components.Schemas["Artwork"]
	if kinds == nil || artwork == nil {
		t.Fatal("ArtworkKind or Artwork is not declared")
	}

	wanted := enumTexts(kinds.Value.Enum)
	slots := slices.Collect(maps.Keys(artwork.Value.Properties))

	slices.Sort(wanted)
	slices.Sort(slots)

	if !slices.Equal(wanted, slots) {
		t.Errorf("Artwork properties %v, want the ArtworkKind values %v", slots, wanted)
	}
}

// TestItemDetailBounds pins the formats and bounds of ItemDetail that clients rely on.
func TestItemDetailBounds(t *testing.T) {
	spec := loadSpec(t)

	const wantMaxExtras = 100
	if extras := propertySchema(spec, "ItemDetail.extras"); extras == nil || extras.Value.MaxItems == nil || *extras.Value.MaxItems != wantMaxExtras {
		t.Errorf("ItemDetail.extras has no maxItems %d", wantMaxExtras)
	}

	if releaseDate := propertySchema(spec, "ItemDetail.releaseDate"); releaseDate == nil || releaseDate.Value.Format != "date" {
		t.Error("ItemDetail.releaseDate is not format date")
	}

	for _, name := range []string{"genres", "studios", "countries"} {
		property := propertySchema(spec, "ItemDetail."+name)
		if property == nil || property.Value.Items == nil || !property.Value.Items.Value.Type.Is(openapi3.TypeString) {
			t.Errorf("ItemDetail.%s is not a string array", name)
		}
	}
}

// TestListingScopesAreDocumented: library listings and counts cover top-level items only,
// and extras are never listed on their own (ADR 0027 section 2).
func TestListingScopesAreDocumented(t *testing.T) {
	spec := loadSpec(t)

	itemCount := propertySchema(spec, "Library.itemCount")
	if itemCount == nil {
		t.Fatal("Library.itemCount is not declared")
	}

	descriptions := map[string]string{
		"listLibraryItems":  getOperation(t, spec, http.MethodGet, "/api/v1/libraries/{libraryId}/items").Description,
		"Library.itemCount": itemCount.Value.Description,
		"listItemChildren":  getOperation(t, spec, http.MethodGet, "/api/v1/items/{itemId}/children").Description,
	}

	for location, description := range descriptions {
		if !strings.Contains(description, "top-level") && !strings.Contains(description, "never children") {
			t.Errorf("%s does not say which items it covers: %q", location, description)
		}

		if !strings.Contains(strings.ToLower(description), "extras") {
			t.Errorf("%s does not mention extras: %q", location, description)
		}
	}
}

// TestImageParameters pins the parameters of getImage (ADR 0027 section 8).
func TestImageParameters(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, imagePath)

	if parameter := spec.Paths.Value(imagePath).Parameters.GetByInAndName("path", "imageId"); parameter == nil {
		t.Error("path parameter imageId is not declared on the path")
	}

	width := operation.Parameters.GetByInAndName("query", "width")
	if width == nil || !width.Required || width.Schema.Ref != "#/components/schemas/ImageWidth" {
		t.Error("required query parameter width with schema ImageWidth is not declared")
	}

	tag := operation.Parameters.GetByInAndName("query", "tag")
	if tag == nil || tag.Required {
		t.Fatal("optional query parameter tag is not declared")
	}

	const wantMaxTagLength = 64
	if maxLength := tag.Schema.Value.MaxLength; maxLength == nil || *maxLength != wantMaxTagLength {
		t.Errorf("tag maxLength = %v, want %d", maxLength, wantMaxTagLength)
	}

	pattern := regexp.MustCompile(tag.Schema.Value.Pattern)

	tests := map[string]bool{
		"0123456789abcdef": true, "a": true,
		"": false, "ABCDEF": false, "0x12": false, "12 34": false, "../x": false,
	}

	for value, want := range tests {
		if got := pattern.MatchString(value); got != want {
			t.Errorf("tag pattern matches %q = %v, want %v", value, got, want)
		}
	}
}

// TestImageCachingContract: the image responses carry the validators and the caching rule
// of ADR 0027 section 8, and clients build the URL only through the api-client helper.
func TestImageCachingContract(t *testing.T) {
	spec := loadSpec(t)

	operation := getOperation(t, spec, http.MethodGet, imagePath)

	success := operation.Responses.Status(http.StatusOK)
	if success == nil {
		t.Fatal("getImage declares no 200 response")
	}

	for _, header := range []string{"ETag", "Cache-Control"} {
		if success.Value.Headers[header] == nil {
			t.Errorf("getImage 200 has no %s header", header)
		}
	}

	if operation.Responses.Status(http.StatusNotModified) == nil {
		t.Error("getImage declares no 304 response")
	}

	mentions := []string{
		"max-age=31536000", "immutable", "no-cache", "If-None-Match", "imageUrl",
		`"invalid"`, `"not_found"`, `"unsupported"`, `"unavailable"`,
	}

	for _, text := range mentions {
		if !strings.Contains(operation.Description, text) {
			t.Errorf("getImage description does not mention %s", text)
		}
	}
}

// isProviderScopedID reports whether schema.property holds an ID in the namespace of an
// external metadata provider (ADR 0031) instead of a Mivo UUID.
func isProviderScopedID(schema, property string) bool {
	switch schema + "." + property {
	case "IdentifyCandidate.externalId", "IdentifyRequest.externalId", "IdentifySearchRequest.providerId":
		return true
	default:
		return false
	}
}

// specOperationKey names one operation by method and path.
type specOperationKey struct{ method, path string }

// TestMetadataContract pins the schemas of task M6-01 (ADR 0031).
func TestMetadataContract(t *testing.T) {
	spec := loadSpec(t)

	shapes := map[string]schemaShape{
		"IdentifySearchRequest":   {required: []string{"query"}, optional: []string{"year", "providerId"}},
		"IdentifyCandidate":       {required: []string{"provider", "externalId", "title", "confidence"}, optional: []string{"year", "overview", "posterUrl"}},
		"IdentifyCandidateList":   {required: []string{"candidates"}},
		"IdentifyRequest":         {required: []string{"provider", "externalId"}},
		"MetadataSettings":        {required: []string{"onlineEnabled", "language", "providers"}},
		"MetadataProviderSetting": {required: []string{"provider", "enabled"}, optional: []string{"keyStatus"}},
		"MetadataSettingsUpdate":  {optional: []string{"onlineEnabled", "language", "providers", "tmdbApiKey"}},
		"MetadataProviderToggle":  {required: []string{"provider", "enabled"}},
		"MetadataStatus":          {required: []string{"queueLength", "providers"}},
		"MetadataProviderStatus":  {required: []string{"provider", "state"}},
		"ItemSummary":             {optional: []string{"matchState", "metadataSources"}},
		"ItemDetail":              {optional: []string{"matchState", "metadataSources"}},
		"Problem":                 {optional: []string{"retryAfterSeconds"}},
	}

	for name, shape := range shapes {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		assertSchemaShape(t, name, schema.Value, shape)
	}
}

func TestMetadataEnums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]any{
		"MatchState":        {"matched", "unmatched", "disabled"},
		"MetadataProvider":  {"tmdb", "musicbrainz", "coverartarchive"},
		"ProviderKeyStatus": {"none", "bundled", "own"},
		"ProviderState":     {"ok", "rate_limited", "offline", "disabled"},
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

func TestMetadataPropertyReferences(t *testing.T) {
	spec := loadSpec(t)

	references := map[string]string{
		"ItemSummary.matchState": "MatchState", "ItemDetail.matchState": "MatchState",
		"IdentifyCandidate.provider": "MetadataProvider", "IdentifyRequest.provider": "MetadataProvider",
		"MetadataProviderSetting.provider": "MetadataProvider", "MetadataProviderSetting.keyStatus": "ProviderKeyStatus",
		"MetadataProviderToggle.provider": "MetadataProvider", "MetadataProviderStatus.provider": "MetadataProvider",
		"MetadataProviderStatus.state": "ProviderState",
	}

	for location, want := range references {
		if property := propertySchema(spec, location); property == nil || property.Ref != "#/components/schemas/"+want {
			t.Errorf("%s does not reference %s", location, want)
		}
	}

	for _, location := range []string{"ItemSummary.metadataSources", "ItemDetail.metadataSources"} {
		property := propertySchema(spec, location)
		if property == nil || property.Value.Items == nil || !property.Value.Items.Value.Type.Is(openapi3.TypeString) || property.Value.MaxItems == nil {
			t.Errorf("%s is not a bounded string array", location)
		}
	}
}

// TestMetadataKeyIsWriteOnly: the TMDB key is never returned (ADR 0031 section 3).
func TestMetadataKeyIsWriteOnly(t *testing.T) {
	spec := loadSpec(t)

	key := propertySchema(spec, "MetadataSettingsUpdate.tmdbApiKey")
	if key == nil || !key.Value.WriteOnly {
		t.Error("MetadataSettingsUpdate.tmdbApiKey is not writeOnly")
	}

	for _, name := range []string{"MetadataSettings", "MetadataProviderSetting", "MetadataStatus", "IdentifyCandidate"} {
		for property := range spec.Components.Schemas[name].Value.Properties {
			if strings.Contains(strings.ToLower(property), "apikey") {
				t.Errorf("%s.%s would return a key", name, property)
			}
		}
	}

	update := spec.Components.Schemas["MetadataSettingsUpdate"].Value
	if update.AdditionalProperties.Has == nil || *update.AdditionalProperties.Has || update.MinProps < 1 {
		t.Error("MetadataSettingsUpdate must set additionalProperties: false and minProperties 1")
	}
}

// TestMetadataManagementOperationsNameTheGuard: every operation that changes metadata or
// settings carries the ADR 0028 management guard note.
func TestMetadataManagementOperationsNameTheGuard(t *testing.T) {
	spec := loadSpec(t)

	managed := []specOperationKey{
		{http.MethodPost, "/api/v1/items/{itemId}/identify/search"},
		{http.MethodPost, "/api/v1/items/{itemId}/identify"},
		{http.MethodDelete, "/api/v1/items/{itemId}/match"},
		{http.MethodPost, "/api/v1/items/{itemId}/refresh"},
		{http.MethodPost, "/api/v1/libraries/{libraryId}/refresh"},
		{http.MethodGet, metadataSettingsPath},
		{http.MethodPut, metadataSettingsPath},
	}

	for _, key := range managed {
		operation := getOperation(t, spec, key.method, key.path)
		if !strings.Contains(operation.Description, "ADR 0028") {
			t.Errorf("%s %s does not carry the ADR 0028 management guard note", key.method, key.path)
		}
	}
}

// TestMetadataErrorResponses pins the documented failures (task M6-01): every online
// operation answers 409 when online metadata is off; the live provider calls also answer
// 503 with Retry-After, while the refreshes only queue and so work offline.
func TestMetadataErrorResponses(t *testing.T) {
	spec := loadSpec(t)

	queued := []specOperationKey{
		{http.MethodPost, "/api/v1/items/{itemId}/refresh"}, {http.MethodPost, "/api/v1/libraries/{libraryId}/refresh"},
	}
	live := []specOperationKey{
		{http.MethodPost, "/api/v1/items/{itemId}/identify/search"}, {http.MethodPost, "/api/v1/items/{itemId}/identify"},
	}

	for _, key := range slices.Concat(live, queued) {
		operation := getOperation(t, spec, key.method, key.path)

		assertResponseSchema(t, operation.Responses.Status(http.StatusConflict), problemMediaType, "Problem")

		if !strings.Contains(operation.Description, `"not_found"`) {
			t.Errorf("%s %s does not document 404 not_found", key.method, key.path)
		}
	}

	for _, key := range live {
		operation := getOperation(t, spec, key.method, key.path)

		assertResponseSchema(t, operation.Responses.Status(http.StatusServiceUnavailable), problemMediaType, "Problem")

		if operation.Responses.Status(http.StatusServiceUnavailable).Value.Headers["Retry-After"] == nil {
			t.Errorf("%s %s: 503 has no Retry-After header", key.method, key.path)
		}
	}

	for _, key := range queued {
		operation := getOperation(t, spec, key.method, key.path)

		if operation.Responses.Status(http.StatusAccepted) == nil {
			t.Errorf("%s %s declares no 202", key.method, key.path)
		}

		if operation.Responses.Status(http.StatusServiceUnavailable) != nil || strings.Contains(operation.Description, "provider_unavailable") {
			t.Errorf("%s %s only queues and must not fail with provider_unavailable", key.method, key.path)
		}
	}
}

// TestMetadataSettingsLanguage: a malformed language answers the existing language_invalid
// code and an empty one means the server default, as in updateLibrary.
func TestMetadataSettingsLanguage(t *testing.T) {
	spec := loadSpec(t)

	description := getOperation(t, spec, http.MethodPut, metadataSettingsPath).Description
	for _, text := range []string{`"language_invalid"`, "empty language", "server default"} {
		if !strings.Contains(description, text) {
			t.Errorf("updateMetadataSettings description does not mention %s", text)
		}
	}
}

// TestUnmatchedFilterAndPosterProxy: the library listing filters by match state, and a
// candidate poster is a Mivo URL, never a provider host (ADR 0010, ADR 0031).
func TestUnmatchedFilterAndPosterProxy(t *testing.T) {
	spec := loadSpec(t)

	filter := getOperation(t, spec, http.MethodGet, "/api/v1/libraries/{libraryId}/items").Parameters.GetByInAndName("query", "matchState")
	if filter == nil || filter.Required || filter.Schema.Ref != "#/components/schemas/MatchState" {
		t.Error("optional query parameter matchState with schema MatchState is not declared on listLibraryItems")
	}

	poster := propertySchema(spec, "IdentifyCandidate.posterUrl")
	if poster == nil || !strings.Contains(poster.Value.Description, "never") {
		t.Fatal("IdentifyCandidate.posterUrl does not say it never points at a provider host")
	}

	posterPattern := regexp.MustCompile(poster.Value.Pattern)
	posterTests := map[string]bool{
		"/api/v1/images/7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4?width=320": true,
		"https://image.tmdb.org/t/p/w500/x.jpg":                         false,
		"//image.tmdb.org/x.jpg":                                        false,
		"api/v1/images/x":                                               false,
	}

	for value, want := range posterTests {
		if got := posterPattern.MatchString(value); got != want {
			t.Errorf("posterUrl pattern matches %q = %v, want %v", value, got, want)
		}
	}

	retry := propertySchema(spec, "Problem.retryAfterSeconds")
	if retry == nil || retry.Value.Min == nil || *retry.Value.Min < 1 {
		t.Error("Problem.retryAfterSeconds is missing or unbounded below")
	}
}

// TestSpecVersionCoversMetadata: the additive milestone-6 contract raised the minor version.
func TestSpecVersionCoversMetadata(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 4

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
