package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/magpeek/mivo/internal/domain/system"
	"github.com/magpeek/mivo/internal/platform/brand"
)

// specFile is relative to this package directory, the working directory of go test.
const specFile = "openapi.yaml"

const (
	problemMediaType = "application/problem+json"
	jsonMediaType    = "application/json"

	// problemResponseRef is the shared error response every operation must declare as default.
	problemResponseRef = "#/components/responses/Problem"
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

func TestSpecIsValidOpenAPI(t *testing.T) {
	spec := loadSpec(t)

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

	if spec.Tags.Get("system") == nil {
		t.Error(`tag "system" is not declared`)
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

func TestSpecOperations(t *testing.T) {
	spec := loadSpec(t)

	tests := []struct {
		path         string
		operationID  string
		jsonStatuses []int
		schema       string
	}{
		{
			path:         "/api/v1/health",
			operationID:  "getHealth",
			jsonStatuses: []int{http.StatusOK, http.StatusServiceUnavailable},
			schema:       "HealthResponse",
		},
		{
			path:         "/api/v1/system/info",
			operationID:  "getSystemInfo",
			jsonStatuses: []int{http.StatusOK},
			schema:       "SystemInfo",
		},
	}

	for _, test := range tests {
		t.Run(test.operationID, func(t *testing.T) {
			operation := getOperation(t, spec, test.path)

			if operation.OperationID != test.operationID {
				t.Errorf("operationId = %q, want %q", operation.OperationID, test.operationID)
			}

			if !slices.Contains(operation.Tags, "system") {
				t.Errorf("tags = %v, want system", operation.Tags)
			}

			for _, status := range test.jsonStatuses {
				assertResponseSchema(t, operation.Responses.Status(status), jsonMediaType, test.schema)
			}

			assertUnauthenticated(t, operation)
		})
	}
}

func getOperation(t *testing.T, spec *openapi3.T, path string) *openapi3.Operation {
	t.Helper()

	item := spec.Paths.Value(path)
	if item == nil || item.Get == nil {
		t.Fatalf("GET %s is not declared", path)
	}

	return item.Get
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
// an operation fails with, clients can parse the body as a problem.
func TestEveryOperationDeclaresProblemDefault(t *testing.T) {
	spec := loadSpec(t)

	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			response := operation.Responses.Default()
			if response == nil || response.Ref != problemResponseRef {
				t.Errorf("%s %s: default response is not %s", method, path, problemResponseRef)
			}
		}
	}
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

	nested := make(map[string]*openapi3.SchemaRef, len(parent.Properties))
	for propertyName, property := range parent.Properties {
		nested[location+"."+propertyName] = property
	}

	nested[location+"[items]"] = parent.Items
	nested[location+"{additionalProperties}"] = parent.AdditionalProperties.Schema

	for childLocation, child := range nested {
		if child == nil || child.Ref != "" || child.Value == nil {
			continue
		}

		if child.Value.Description == "" {
			t.Errorf("schema %s has no description", childLocation)
		}

		assertNestedSchemasDescribed(t, childLocation, child.Value)
	}
}
