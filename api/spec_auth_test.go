package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/magpeek/mivo/internal/platform/brand"
)

// Contract of task M7-01 (ADR 0032): setup, sign-in, me, profile selection and own devices.

const (
	authTag = "auth"

	setupPath       = "/api/v1/setup"
	authSessionPath = "/api/v1/auth/session"
	mePath          = "/api/v1/me"
	meProfilePath   = "/api/v1/me/profile"
	meDevicesPath   = "/api/v1/me/devices"
	meDevicePath    = meDevicesPath + "/{deviceId}"

	sessionCookieScheme = "sessionCookie"
	bearerScheme        = "bearer"

	// sessionCookieSuffix follows the brand ID in the cookie name (ADR 0032 section 3).
	sessionCookieSuffix = "_session"
)

func authOperationCases() []operationCase {
	return []operationCase{
		{method: http.MethodGet, path: setupPath, operationID: "getSetupState", tag: authTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "SetupState", unauthenticated: true},
		{method: http.MethodPost, path: setupPath, operationID: "completeSetup", tag: authTag, statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "Session", unauthenticated: true},
		{method: http.MethodPost, path: setupPath, operationID: "completeSetup", tag: authTag, statuses: []int{http.StatusConflict}, mediaType: problemMediaType, schema: "Problem", unauthenticated: true},
		{method: http.MethodPost, path: authSessionPath, operationID: "createSession", tag: authTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Session", unauthenticated: true},
		{method: http.MethodPost, path: authSessionPath, operationID: "createSession", tag: authTag, statuses: []int{http.StatusUnauthorized, http.StatusTooManyRequests}, mediaType: problemMediaType, schema: "Problem", unauthenticated: true},
		{method: http.MethodDelete, path: authSessionPath, operationID: "deleteSession", tag: authTag},
		{method: http.MethodGet, path: mePath, operationID: "getMe", tag: authTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Me"},
		{method: http.MethodPut, path: meProfilePath, operationID: "selectProfile", tag: authTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Me"},
		{method: http.MethodPut, path: meProfilePath, operationID: "selectProfile", tag: authTag, statuses: []int{http.StatusForbidden, http.StatusTooManyRequests}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodDelete, path: meProfilePath, operationID: "clearProfileSelection", tag: authTag},
		{method: http.MethodPut, path: mePath + "/password", operationID: "changeOwnPassword", tag: authTag, statuses: []int{http.StatusForbidden, http.StatusTooManyRequests}, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: meDevicesPath, operationID: "listOwnDevices", tag: authTag, statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "DeviceList"},
		{method: http.MethodDelete, path: meDevicePath, operationID: "revokeOwnDevice", tag: authTag},
	}
}

func TestAuthOperations(t *testing.T) {
	spec := loadSpec(t)

	for _, test := range authOperationCases() {
		t.Run(fmt.Sprintf("%s %s %v", test.method, test.path, test.statuses), func(t *testing.T) {
			operation := getOperation(t, spec, test.method, test.path)

			if operation.OperationID != test.operationID || !slices.Contains(operation.Tags, test.tag) {
				t.Errorf("operation = %q %v, want %q tagged %s", operation.OperationID, operation.Tags, test.operationID, test.tag)
			}

			for _, status := range test.statuses {
				assertResponseSchema(t, operation.Responses.Status(status), test.mediaType, test.schema)
			}

			if test.unauthenticated {
				assertUnauthenticated(t, operation)
			} else {
				assertSessionRequired(t, specOperation{method: test.method, path: test.path, operation: operation})
			}
		})
	}
}

// assertSessionRequired: the operation inherits the global session requirement.
func assertSessionRequired(t *testing.T, entry specOperation) {
	t.Helper()

	if entry.operation.Security != nil {
		t.Errorf("%s: security = %s, want the inherited global session requirement", entry, describeSecurity(entry.operation.Security))
	}
}

// TestOnlyDiscoveryAndSignInAreOpen pins ADR 0032 section 6: everything except the health
// check, system info, the setup pair and sign-in needs a session.
func TestOnlyDiscoveryAndSignInAreOpen(t *testing.T) {
	spec := loadSpec(t)

	open := []string{
		"get /api/v1/health", "get /api/v1/system/info",
		"get " + setupPath, "post " + setupPath, "post " + authSessionPath,
	}

	var gotOpen []string

	for _, entry := range allOperations(spec) {
		if entry.operation.Security != nil {
			gotOpen = append(gotOpen, strings.ToLower(entry.method)+" "+entry.path)
		}
	}

	slices.Sort(open)
	slices.Sort(gotOpen)

	if !slices.Equal(gotOpen, open) {
		t.Errorf("operations with their own security = %v, want exactly %v", gotOpen, open)
	}
}

// TestSecuritySchemes: browsers use the brand-derived cookie, other clients a bearer token,
// and both are accepted by every operation through the global requirement.
func TestSecuritySchemes(t *testing.T) {
	spec := loadSpec(t)

	cookie := spec.Components.SecuritySchemes[sessionCookieScheme]
	if cookie == nil || cookie.Value.Type != "apiKey" || cookie.Value.In != "cookie" {
		t.Fatalf("%s is not an apiKey security scheme in a cookie", sessionCookieScheme)
	}

	if want := brand.ID + sessionCookieSuffix; cookie.Value.Name != want {
		t.Errorf("cookie name = %q, want %q (brand ID)", cookie.Value.Name, want)
	}

	if !strings.Contains(cookie.Value.Description, "brand ID") {
		t.Error("sessionCookie does not document that its name derives from the brand ID")
	}

	bearer := spec.Components.SecuritySchemes[bearerScheme]
	if bearer == nil || bearer.Value.Type != "http" || bearer.Value.Scheme != "bearer" {
		t.Fatalf("%s is not an http bearer security scheme", bearerScheme)
	}

	var global []string
	for _, requirement := range spec.Security {
		for name := range requirement {
			global = append(global, name)
		}
	}

	slices.Sort(global)

	if !slices.Equal(global, []string{bearerScheme, sessionCookieScheme}) {
		t.Errorf("global security = %v, want the alternatives %s and %s", global, bearerScheme, sessionCookieScheme)
	}
}

// TestAuthWritesNameTheSameOriginRule: setup names the local-peer rule, every write the
// same-origin rule (ADR 0028, ADR 0032 section 3).
func TestAuthWritesNameTheSameOriginRule(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if !slices.Contains(entry.operation.Tags, authTag) || entry.method == http.MethodGet {
			continue
		}

		for _, text := range []string{"ADR 0028", "same-origin", "ADR 0032"} {
			if !strings.Contains(entry.operation.Description, text) {
				t.Errorf("%s does not mention %s", entry, text)
			}
		}
	}

	if setup := getOperation(t, spec, http.MethodPost, setupPath); !strings.Contains(setup.Description, "local peers") {
		t.Error("completeSetup does not name the local-peer rule")
	}
}

// TestSecretsAreWriteOnly: passwords and PINs are never returned, and Me never carries the
// session token (ADR 0032 section 3).
func TestSecretsAreWriteOnly(t *testing.T) {
	spec := loadSpec(t)

	secrets := []string{
		"SetupRequest.password", "SignInRequest.password", "ProfileSelection.pin",
		"PasswordChange.currentPassword", "PasswordChange.newPassword",
	}

	for _, location := range secrets {
		property := propertySchema(spec, location)
		if property == nil || !property.Value.WriteOnly || property.Value.Format != "password" {
			t.Errorf("%s must be writeOnly with format password", location)
		}
	}

	if _, hasToken := spec.Components.Schemas["Me"].Value.Properties["token"]; hasToken {
		t.Error("Me must not carry the session token")
	}
}

func TestPinIsFourDigits(t *testing.T) {
	spec := loadSpec(t)

	pin := propertySchema(spec, "ProfileSelection.pin")
	if pin == nil || pin.Value.Pattern != "^[0-9]{4}$" {
		t.Fatal("ProfileSelection.pin must match exactly 4 ASCII digits")
	}
}

func TestAuthEnums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]string{
		"ClientKind":   {"web", "tv", "cli"},
		"AccountRole":  {"admin", "user"},
		"AddressClass": {"lan", "remote"},
		"AvatarColor":  {"coral", "blue", "green", "amber", "violet", "teal", "pink", "slate"},
	}

	for name, want := range tests {
		if got := enumValues(spec, name); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func enumValues(spec *openapi3.T, name string) []string {
	var values []string

	for _, value := range spec.Components.Schemas[name].Value.Enum {
		values = append(values, fmt.Sprint(value))
	}

	return values
}

// TestMeShape: the picker needs every profile field, and currentProfile is null until a
// profile is selected.
func TestMeShape(t *testing.T) {
	spec := loadSpec(t)

	current := propertySchema(spec, "Me.currentProfile")
	if current == nil || !current.Value.Nullable || !slices.Contains(spec.Components.Schemas["Me"].Value.Required, "currentProfile") {
		t.Error("Me.currentProfile must be required and nullable")
	}

	profile := spec.Components.Schemas["ProfileSummary"].Value
	for _, field := range []string{"id", "name", "avatarColor", "hasPin", "isPrimary", "isAdmin"} {
		if !slices.Contains(profile.Required, field) {
			t.Errorf("ProfileSummary.%s is not required", field)
		}
	}

	device := spec.Components.Schemas["Device"].Value
	for _, field := range []string{"id", "name", "client", "platform", "lastSeenAt", "addressClass", "current"} {
		if device.Properties[field] == nil {
			t.Errorf("Device has no property %s", field)
		}
	}
}

// TestAuthProblemCodesAreDocumented complements TestPublishedProblemCodesAreDocumented with
// the codes of ADR 0032.
func TestAuthProblemCodesAreDocumented(t *testing.T) {
	spec := loadSpec(t)

	description := spec.Components.Schemas["Problem"].Value.Properties["code"].Value.Description

	codes := map[string]int{
		"unauthenticated": http.StatusUnauthorized, "setup_required": http.StatusUnauthorized,
		"invalid_credentials": http.StatusUnauthorized, "profile_required": http.StatusForbidden,
		"pin_invalid": http.StatusForbidden, "profile_locked": http.StatusTooManyRequests,
		"too_many_attempts": http.StatusTooManyRequests,
	}

	for code, status := range codes {
		if documented := fmt.Sprintf(`"%s" (%d`, code, status); !strings.Contains(description, documented) {
			t.Errorf("Problem.code description does not document %s)", documented)
		}
	}
}

// TestTooManyAttemptsCarriesRetryAfter: throttled sign-ins and locked profiles say when to
// try again.
func TestTooManyAttemptsCarriesRetryAfter(t *testing.T) {
	spec := loadSpec(t)

	response := spec.Components.Responses["TooManyAttempts"]
	if response == nil || response.Value.Headers["Retry-After"] == nil {
		t.Fatal("components.responses.TooManyAttempts has no Retry-After header")
	}
}

// TestSpecVersionCoversAuth: the additive milestone-7 contract raised the minor version.
func TestSpecVersionCoversAuth(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 11

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
