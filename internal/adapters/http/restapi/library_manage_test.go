package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const createBody = `{"kind":"movies","name":"Movies","folders":[{"rootId":"` + testRootID + `","path":"Movies"}],` +
	`"metadataLanguage":"de","options":{"scanExtras":true,"readNfo":false,"localArtwork":true}}`

func storedMovies() library.ManagedLibrary {
	return library.ManagedLibrary{
		ID: testLibraryID, Name: "Movies", Kind: library.KindMovies, MetadataLanguage: "de",
		Folders: []library.Folder{{ID: testFolderID, Path: "/media/Movies"}},
		Options: library.Options{ScanExtras: true, LocalArtwork: true},
	}
}

func managementOptions(manager *fakeManager, scans *fakeScanService) restapi.Options {
	return restapi.Options{Manager: manager, Scan: scans}
}

func TestCreateLibraryAnswersCreatedWithLocationAndRunningScan(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{stored: storedMovies()}
	scans := &fakeScanService{statuses: map[string]appscan.Status{testLibraryID: runningStatus(t)}}
	handler, _ := newManagementHandler(t, managementOptions(manager, scans))

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, librariesPath, createBody))
	assertJSONResponse(t, got, http.StatusCreated)

	if location := got.header.Get("Location"); location != libraryPath {
		t.Errorf("Location = %q, want %q", location, libraryPath)
	}

	created := decodeStrictly[apigen.Library](t, got.body)
	if created.Scan.State != apigen.ScanStateRunning || len(created.Folders) != 1 || created.Folders[0].Path != "/media/Movies" {
		t.Errorf("library = %+v, want the running scan and one folder", created)
	}

	want := applibrary.CreateRequest{
		Name: "Movies", Kind: library.KindMovies, MetadataLanguage: "de",
		Folders: []applibrary.FolderRef{{RootID: testRootID, Path: "Movies"}},
		Options: &library.Options{ScanExtras: true, LocalArtwork: true},
	}
	if len(manager.created) != 1 || !reflect.DeepEqual(manager.created[0], want) {
		t.Errorf("manager got %+v, want %+v", manager.created, want)
	}
}

func TestCreateLibraryWithoutOptionalValuesPassesDefaults(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{stored: storedMovies()}
	handler, _ := newManagementHandler(t, managementOptions(manager, &fakeScanService{}))

	body := `{"kind":"shows","name":"S","folders":[{"rootId":"` + testRootID + `"}]}`
	assertJSONResponse(t, serveRequest(t, handler, jsonRequest(t, http.MethodPost, librariesPath, body)), http.StatusCreated)

	got := manager.created[0]
	if got.Options != nil || got.MetadataLanguage != "" || got.Folders[0].Path != "" {
		t.Errorf("request = %+v, want nil options, empty language and path", got)
	}
}

func TestManagementErrorsMapToProblems(t *testing.T) {
	t.Parallel()

	otherLibrary := mustUUID(t, testSecondLibraryID)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
		wantLibID  bool
	}{
		{"folder not found", &applibrary.FolderCheckError{Index: 1, Problem: applibrary.ProblemNotFound}, 400, restapi.CodeFolderNotFound, "folders[1]", false},
		{"folder unreadable", &applibrary.FolderCheckError{Problem: applibrary.ProblemUnreadable}, 400, restapi.CodeFolderUnreadable, "folders[0]", false},
		{"outside roots", &applibrary.FolderCheckError{Problem: applibrary.ProblemOutsideRoots}, 400, restapi.CodeFolderOutsideRoots, "folders[0]", false},
		{"unknown folder id", &applibrary.FolderCheckError{Index: 2, Problem: applibrary.ProblemUnknownFolder}, 400, restapi.CodeInvalid, "folders[2]", false},
		{"reference shape", &applibrary.FolderCheckError{Problem: applibrary.ProblemReferenceShape}, 400, restapi.CodeInvalid, "folders[0]", false},
		{"reserved", &library.FolderError{Index: 3, Reason: library.ReasonReserved}, 400, restapi.CodeFolderReserved, "folders[3]", false},
		{"path", &library.FolderError{Reason: library.ReasonPath}, 400, restapi.CodeFolderOutsideRoots, "folders[0]", false},
		{"overlap", &library.FolderError{Index: 1, Reason: library.ReasonOverlap, OtherLibraryID: otherLibrary.String()}, 409, restapi.CodeFolderOverlap, "folders[1]", true},
		{"duplicate", &library.FolderError{Index: 1, Reason: library.ReasonDuplicate}, 409, restapi.CodeFolderOverlap, "folders[1]", false},
		{"nested", &library.FolderError{Index: 1, Reason: library.ReasonNested}, 409, restapi.CodeFolderOverlap, "folders[1]", false},
		{"store conflict", fmt.Errorf("%w: %w", &library.FolderError{Reason: library.ReasonOverlap}, domain.ErrConflict), 409, restapi.CodeFolderOverlap, "folders[0]", false},
		{"name", &library.FieldError{Field: library.FieldName, Reason: "empty"}, 400, restapi.CodeNameInvalid, "name", false},
		{"kind", &library.FieldError{Field: library.FieldKind, Reason: "x"}, 400, restapi.CodeKindUnsupported, "kind", false},
		{"language", &library.FieldError{Field: library.FieldLanguage, Reason: "x"}, 400, restapi.CodeLanguageInvalid, "metadataLanguage", false},
		{"folder count", &library.FieldError{Field: library.FieldFolders, Reason: "17"}, 400, restapi.CodeTooManyFolders, "folders", false},
		{"name taken", fmt.Errorf("name used: %w", domain.ErrConflict), 409, restapi.CodeConflict, "", false},
		{"unknown library", fmt.Errorf("get: %w", domain.ErrNotFound), 404, restapi.CodeNotFound, "", false},
		{"scan does not stop", fmt.Errorf("wait: %w", domain.ErrUnavailable), 503, restapi.CodeUnavailable, "", false},
		{"unexpected", errors.New("disk failure at /media/secret"), 500, restapi.CodeInternal, "", false},
		{"unexpected invalid", fmt.Errorf("stored: %w", domain.ErrForbidden), 500, restapi.CodeInternal, "", false},
	}

	for _, test := range tests {
		for _, request := range []struct{ method, target, body string }{
			{http.MethodPost, librariesPath, createBody},
			{http.MethodPatch, libraryPath, `{"name":"New"}`},
		} {
			t.Run(test.name+" "+request.method, func(t *testing.T) {
				t.Parallel()

				handler, _ := newManagementHandler(t, managementOptions(&fakeManager{err: test.err}, &fakeScanService{}))

				got := serveRequest(t, handler, jsonRequest(t, request.method, request.target, request.body))
				problem := assertProblem(t, got, test.wantStatus, test.wantCode)
				assertProblemParts(t, problem, test.wantField, test.wantLibID)
				assertBodyLacks(t, got.body, "/media", "secret")
			})
		}
	}
}

func assertProblemParts(t *testing.T, problem apigen.Problem, wantField string, wantLibraryID bool) {
	t.Helper()

	switch {
	case wantField == "" && problem.Field != nil:
		t.Errorf("field = %q, want none", *problem.Field)
	case wantField != "" && (problem.Field == nil || *problem.Field != wantField):
		t.Errorf("field = %v, want %q", problem.Field, wantField)
	}

	if wantLibraryID != (problem.LibraryID != nil) {
		t.Errorf("libraryId = %v, want present %v", problem.LibraryID, wantLibraryID)
	}

	if wantLibraryID && problem.LibraryID.String() != testSecondLibraryID {
		t.Errorf("libraryId = %s, want %s", problem.LibraryID, testSecondLibraryID)
	}
}

func TestDeleteLibrary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "removed", wantStatus: http.StatusNoContent},
		{name: "unknown", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "scan stuck", err: domain.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeUnavailable},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{err: test.err}
			handler, _ := newManagementHandler(t, managementOptions(manager, &fakeScanService{}))

			got := serve(t, handler, http.MethodDelete, libraryPath)
			if test.wantCode != "" {
				assertProblem(t, got, test.wantStatus, test.wantCode)

				return
			}

			if got.status != test.wantStatus || len(got.body) != 0 {
				t.Errorf("answer = %d %q, want %d without a body", got.status, got.body, test.wantStatus)
			}

			if !reflect.DeepEqual(manager.removed, []string{testLibraryID}) {
				t.Errorf("removed = %v, want [%s]", manager.removed, testLibraryID)
			}
		})
	}
}

func TestUpdateLibraryPassesOnlyGivenProperties(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{stored: storedMovies()}
	handler, _ := newManagementHandler(t, managementOptions(manager, &fakeScanService{}))

	body := `{"folders":[{"folderId":"` + testFolderID + `"},{"rootId":"` + testRootID + `","path":"More"}],"metadataLanguage":""}`
	assertJSONResponse(t, serveRequest(t, handler, jsonRequest(t, http.MethodPatch, libraryPath, body)), http.StatusOK)

	patch := manager.patched[0]
	wantFolders := []applibrary.FolderRef{{FolderID: testFolderID}, {RootID: testRootID, Path: "More"}}

	if patch.Name != nil || patch.Options != nil || patch.MetadataLanguage == nil || *patch.MetadataLanguage != "" ||
		patch.Folders == nil || !reflect.DeepEqual(*patch.Folders, wantFolders) {
		t.Errorf("patch = %+v, want folders %v and an empty language only", patch, wantFolders)
	}

	if manager.patchedID[0] != testLibraryID {
		t.Errorf("id = %q, want %q", manager.patchedID[0], testLibraryID)
	}
}

// TestLibraryOptionsAcceptOnlineMetadata: the optional per-library online switch of ADR 0031
// section 2 passes the strict body check; it is persisted from M6-03 on, until then the
// other switches are applied and responses report the default true.
func TestLibraryOptionsAcceptOnlineMetadata(t *testing.T) {
	t.Parallel()

	const options = `"options":{"scanExtras":true,"readNfo":false,"localArtwork":true,"onlineMetadata":false}`

	manager := &fakeManager{stored: storedMovies()}
	handler, _ := newManagementHandler(t, managementOptions(manager, &fakeScanService{}))

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPatch, libraryPath, `{`+options+`}`))
	assertJSONResponse(t, got, http.StatusOK)
	assertBodyContains(t, got.body, `"onlineMetadata":true`)

	want := library.Options{ScanExtras: true, LocalArtwork: true}
	if patch := manager.patched[0]; patch.Options == nil || *patch.Options != want {
		t.Errorf("patch options = %+v, want %+v", patch.Options, want)
	}

	create := `{"kind":"movies","name":"Movies","folders":[{"rootId":"` + testRootID + `","path":"Movies"}],` + options + `}`
	assertJSONResponse(t, serveRequest(t, handler, jsonRequest(t, http.MethodPost, librariesPath, create)), http.StatusCreated)
}

func TestLibraryBodiesAreChecked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, method, target, body string
	}{
		{"update with kind", http.MethodPatch, libraryPath, `{"kind":"movies","name":"x"}`},
		{"update without property", http.MethodPatch, libraryPath, `{}`},
		{"update unknown property", http.MethodPatch, libraryPath, `{"name":"x","color":"red"}`},
		{"update options without all switches", http.MethodPatch, libraryPath, `{"options":{"scanExtras":true}}`},
		{"update null switch", http.MethodPatch, libraryPath, `{"options":{"scanExtras":true,"readNfo":null,"localArtwork":true}}`},
		{"update unknown folder property", http.MethodPatch, libraryPath, `{"folders":[{"rootId":"` + testRootID + `","extra":1}]}`},
		{"update broken JSON", http.MethodPatch, libraryPath, `{"name":`},
		{"create unknown property", http.MethodPost, librariesPath, `{"kind":"movies","name":"x","folders":[],"id":"` + testLibraryID + `"}`},
		{"create options without switches", http.MethodPost, librariesPath, `{"kind":"movies","name":"x","folders":[],"options":{}}`},
		{"create broken JSON", http.MethodPost, librariesPath, `[`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{stored: storedMovies()}
			handler, _ := newManagementHandler(t, managementOptions(manager, &fakeScanService{}))

			got := serveRequest(t, handler, jsonRequest(t, test.method, test.target, test.body))
			assertProblem(t, got, http.StatusBadRequest, restapi.CodeBadRequest)

			if len(manager.created)+len(manager.patched) != 0 {
				t.Error("manager was called for a rejected body")
			}
		})
	}
}

func TestLibraryBodyCheckLeavesOtherRequestsAlone(t *testing.T) {
	t.Parallel()

	handler, _ := newManagementHandler(t, restapi.Options{Libraries: &fakeLibraryService{overviews: testOverviews()}})

	for _, target := range []string{librariesPath, libraryPath} {
		assertJSONResponse(t, serveRequest(t, handler, jsonRequest(t, http.MethodGet, target, "not json")), http.StatusOK)
	}

	// A body above the limit is still 413 when it is a library body.
	huge := `{"name":"` + strings.Repeat("a", restapi.MaxRequestBodyBytes) + `"}`
	assertProblem(t, serveRequest(t, handler, jsonRequest(t, http.MethodPatch, libraryPath, huge)), http.StatusRequestEntityTooLarge, restapi.CodePayloadTooLarge)
}

func TestCreateLibraryDescribeFailureIsInternal(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{describeErr: errors.New("count failed")}
	options := managementOptions(&fakeManager{stored: storedMovies()}, &fakeScanService{})
	options.Libraries = libraries

	handler, logs := newManagementHandler(t, options)

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, librariesPath, createBody))
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
	assertLogged(t, logs.String(), "count failed")
}

func TestSystemInfoListsTheLibraryKinds(t *testing.T) {
	t.Parallel()

	for name, kinds := range map[string][]library.Kind{
		"movies and shows": {library.KindMovies, library.KindShows},
		"none":             nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info := fakeSystemService{health: healthyHealth(), info: testInfo(t, "", "")}
			handler, _ := newManagementHandler(t, restapi.Options{System: info, LibraryKinds: kinds})

			got := serve(t, handler, http.MethodGet, infoPath)
			assertJSONResponse(t, got, http.StatusOK)

			body := decodeStrictly[apigen.SystemInfo](t, got.body)
			if len(body.LibraryKinds) != len(kinds) {
				t.Errorf("libraryKinds = %v, want %v", body.LibraryKinds, kinds)
			}

			if len(kinds) == 0 {
				assertBodyContains(t, got.body, `"libraryKinds":[]`)
			}
		})
	}
}

func TestProblemCodesOfLibraryManagementArePinned(t *testing.T) {
	t.Parallel()

	got := map[string]string{
		"CodeFolderNotFound":     restapi.CodeFolderNotFound,
		"CodeFolderUnreadable":   restapi.CodeFolderUnreadable,
		"CodeFolderOverlap":      restapi.CodeFolderOverlap,
		"CodeFolderReserved":     restapi.CodeFolderReserved,
		"CodeFolderOutsideRoots": restapi.CodeFolderOutsideRoots,
		"CodeKindUnsupported":    restapi.CodeKindUnsupported,
		"CodeNameInvalid":        restapi.CodeNameInvalid,
		"CodeLanguageInvalid":    restapi.CodeLanguageInvalid,
		"CodeTooManyFolders":     restapi.CodeTooManyFolders,
	}
	want := map[string]string{
		"CodeFolderNotFound":     "folder_not_found",
		"CodeFolderUnreadable":   "folder_unreadable",
		"CodeFolderOverlap":      "folder_overlap",
		"CodeFolderReserved":     "folder_reserved",
		"CodeFolderOutsideRoots": "folder_outside_roots",
		"CodeKindUnsupported":    "kind_unsupported",
		"CodeNameInvalid":        "name_invalid",
		"CodeLanguageInvalid":    "language_invalid",
		"CodeTooManyFolders":     "too_many_folders",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("problem codes = %v, want %v", got, want)
	}
}
