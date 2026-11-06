package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// Contract of task M7-02 (ADR 0032): accounts, managed profiles, library access, content
// limits and the devices of all accounts.

const (
	usersTag = "users"

	usersPath        = "/api/v1/users"
	userPath         = usersPath + "/{userId}"
	userPasswordPath = userPath + "/password"
	userProfilesPath = userPath + "/profiles"
	profilePath      = "/api/v1/profiles/{profileId}"
	devicesPath      = "/api/v1/devices"
	devicePath       = devicesPath + "/{deviceId}"

	// adminNote is the ADR 0032 section 9 rule every administration operation names.
	adminNote = "ADR 0032 section 9"
)

func userOperationCases() []operationCase {
	conflict := []int{http.StatusConflict}

	return []operationCase{
		{method: http.MethodGet, path: usersPath, operationID: "listUsers", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "UserList"},
		{method: http.MethodPost, path: usersPath, operationID: "createUser", statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "User"},
		{method: http.MethodPost, path: usersPath, operationID: "createUser", statuses: conflict, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: userPath, operationID: "getUser", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "User"},
		{method: http.MethodPatch, path: userPath, operationID: "updateUser", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "User"},
		{method: http.MethodPatch, path: userPath, operationID: "updateUser", statuses: conflict, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodDelete, path: userPath, operationID: "deleteUser", statuses: conflict, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodPut, path: userPasswordPath, operationID: "resetUserPassword"},
		{method: http.MethodGet, path: userProfilesPath, operationID: "listUserProfiles", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "ProfileList"},
		{method: http.MethodPost, path: userProfilesPath, operationID: "createProfile", statuses: []int{http.StatusCreated}, mediaType: jsonMediaType, schema: "Profile"},
		{method: http.MethodPost, path: userProfilesPath, operationID: "createProfile", statuses: conflict, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodPatch, path: profilePath, operationID: "updateProfile", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "Profile"},
		{method: http.MethodDelete, path: profilePath, operationID: "deleteProfile", statuses: conflict, mediaType: problemMediaType, schema: "Problem"},
		{method: http.MethodGet, path: devicesPath, operationID: "listDevices", statuses: []int{http.StatusOK}, mediaType: jsonMediaType, schema: "AccountDeviceList"},
		{method: http.MethodDelete, path: devicePath, operationID: "revokeDevice"},
	}
}

func TestUserOperations(t *testing.T) {
	spec := loadSpec(t)

	for _, test := range userOperationCases() {
		t.Run(fmt.Sprintf("%s %s %v", test.method, test.path, test.statuses), func(t *testing.T) {
			operation := getOperation(t, spec, test.method, test.path)

			if operation.OperationID != test.operationID || !slices.Contains(operation.Tags, usersTag) {
				t.Errorf("operation = %q %v, want %q tagged %s", operation.OperationID, operation.Tags, test.operationID, usersTag)
			}

			for _, status := range test.statuses {
				assertResponseSchema(t, operation.Responses.Status(status), test.mediaType, test.schema)
			}

			assertSessionRequired(t, specOperation{method: test.method, path: test.path, operation: operation})
		})
	}
}

// TestUserOperationsNameTheAdminRule: every user operation names the admin viewer and the
// local-peer rule (ADR 0032 section 9, ADR 0028), every write the same-origin rule.
func TestUserOperationsNameTheAdminRule(t *testing.T) {
	spec := loadSpec(t)

	for _, entry := range allOperations(spec) {
		if !slices.Contains(entry.operation.Tags, usersTag) {
			continue
		}

		texts := []string{adminNote, "local peers", "ADR 0028", `403 "forbidden"`}
		if entry.method != http.MethodGet {
			texts = append(texts, "same-origin", "ADR 0032 section 3")
		}

		for _, text := range texts {
			if !strings.Contains(entry.operation.Description, text) {
				t.Errorf("%s does not mention %s", entry, text)
			}
		}
	}
}

// TestUserConflictCodes: each conflict the admin screens show has its own documented code.
func TestUserConflictCodes(t *testing.T) {
	spec := loadSpec(t)

	tests := []struct {
		method string
		path   string
		codes  []string
	}{
		{http.MethodPost, usersPath, []string{`"name_taken" (409`}},
		{http.MethodPatch, userPath, []string{`"name_taken" (409`, `"last_admin" (409`}},
		{http.MethodDelete, userPath, []string{`"last_admin" (409`}},
		{http.MethodPost, userProfilesPath, []string{`"name_taken" (409`, `"profile_limit" (409`}},
		{http.MethodDelete, profilePath, []string{`409 "conflict"`, "primary profile"}},
	}

	for _, test := range tests {
		operation := getOperation(t, spec, test.method, test.path)
		for _, code := range test.codes {
			if !strings.Contains(operation.Description, code) {
				t.Errorf("%s %s does not document %s", test.method, test.path, code)
			}
		}
	}

	description := spec.Components.Schemas["Problem"].Value.Properties["code"].Value.Description
	for _, code := range []string{"name_taken", "last_admin", "profile_limit"} {
		if documented := fmt.Sprintf(`"%s" (%d`, code, http.StatusConflict); !strings.Contains(description, documented) {
			t.Errorf("Problem.code description does not document %s)", documented)
		}
	}
}

func TestUserEnums(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]string{
		"UserStatus":         {"active", "disabled"},
		"ContentLimitPreset": {"youngChildren", "children", "teens", "adults", "custom"},
		"AgeRating":          {"0", "6", "12", "16", "18"},
	}

	for name, want := range tests {
		if got := enumValues(spec, name); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

// TestContentLimitStoresAgeAndUnratedDirectly: a preset alone cannot express "no maxAge but
// hide unrated", so maxAge and hideUnrated are required on their own (BACKLOG m7-05).
func TestContentLimitStoresAgeAndUnratedDirectly(t *testing.T) {
	spec := loadSpec(t)

	limit := spec.Components.Schemas["ContentLimit"].Value
	for _, field := range []string{"preset", "maxAge", "hideUnrated"} {
		if !slices.Contains(limit.Required, field) {
			t.Errorf("ContentLimit.%s is not required", field)
		}
	}

	maxAge := limit.Properties["maxAge"]
	if !maxAge.Value.Nullable || len(maxAge.Value.AllOf) != 1 || maxAge.Value.AllOf[0].Ref != "#/components/schemas/AgeRating" {
		t.Error("ContentLimit.maxAge must be a nullable AgeRating")
	}

	access := spec.Components.Schemas["LibraryAccess"].Value
	ids := access.Properties["libraryIds"]

	if !slices.Contains(access.Required, "all") || ids == nil || ids.Value.Items.Value.Format != "uuid" || ids.Value.MaxItems == nil {
		t.Error("LibraryAccess needs a required all and a bounded uuid list libraryIds")
	}
}

// TestUserSchemasCarryTheAdminFields pins the fields of the US-20 list and the profile editor.
func TestUserSchemasCarryTheAdminFields(t *testing.T) {
	spec := loadSpec(t)

	tests := map[string][]string{
		"User":          {"id", "name", "role", "status", "createdAt", "libraryAccess", "contentLimit", "profileCount"},
		"Profile":       {"id", "userId", "name", "avatarColor", "hasPin", "isPrimary", "contentLimit", "libraryAccess"},
		"AccountDevice": {"id", "userId", "userName", "name", "client", "lastSeenAt", "addressClass", "current"},
	}

	for name, required := range tests {
		schema := spec.Components.Schemas[name].Value
		for _, field := range required {
			if !slices.Contains(schema.Required, field) {
				t.Errorf("%s.%s is not required", name, field)
			}
		}
	}

	if propertySchema(spec, "User.lastActiveAt") == nil {
		t.Error("User has no lastActiveAt")
	}

	if propertySchema(spec, "Profile.pin") != nil {
		t.Error("Profile must not return the PIN")
	}

	if avatar := propertySchema(spec, "ProfileUpdate.avatarColor"); avatar == nil || avatar.Ref != "#/components/schemas/AvatarColor" {
		t.Error("ProfileUpdate.avatarColor must reuse AvatarColor")
	}
}

// TestUserSecretsAreWriteOnly: passwords and PINs of the administration are never returned,
// and a PIN is exactly 4 ASCII digits; null in an update removes it.
func TestUserSecretsAreWriteOnly(t *testing.T) {
	spec := loadSpec(t)

	for _, location := range []string{"UserCreate.password", "PasswordReset.password", "ProfileCreate.pin", "ProfileUpdate.pin"} {
		property := propertySchema(spec, location)
		if property == nil || !property.Value.WriteOnly || property.Value.Format != "password" {
			t.Errorf("%s must be writeOnly with format password", location)
		}
	}

	for _, location := range []string{"ProfileCreate.pin", "ProfileUpdate.pin"} {
		if pin := propertySchema(spec, location); pin == nil || pin.Value.Pattern != "^[0-9]{4}$" {
			t.Errorf("%s must match exactly 4 ASCII digits", location)
		}
	}

	if pin := propertySchema(spec, "ProfileUpdate.pin"); pin == nil || !pin.Value.Nullable {
		t.Error("ProfileUpdate.pin must be nullable, so null removes the PIN")
	}
}

// TestSpecVersionCoversUsers: the additive M7-02 contract raised the minor version.
func TestSpecVersionCoversUsers(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 13

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}
