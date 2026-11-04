package restapi

import (
	"errors"
	"fmt"
	"net/http"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Request properties a library management problem can point at (Problem.field).
const (
	fieldName     = "name"
	fieldKind     = "kind"
	fieldLanguage = "metadataLanguage"
	fieldFolders  = "folders"
)

// libraryProblem maps the typed errors of library management to their problem with code,
// field and detail. It reports false for every other error. The details are fixed English
// text without paths: the client knows which folder it sent and the field names it.
func libraryProblem(err error) (apigen.Problem, bool) {
	var checkErr *applibrary.FolderCheckError
	if errors.As(err, &checkErr) {
		return folderCheckProblem(checkErr), true
	}

	// Checked before the field error: a store conflict wraps a FolderError and ErrConflict.
	var folderErr *library.FolderError
	if errors.As(err, &folderErr) {
		return folderProblem(folderErr), true
	}

	var fieldErr *library.FieldError
	if errors.As(err, &fieldErr) {
		return fieldProblem(fieldErr), true
	}

	return apigen.Problem{}, false
}

func folderCheckProblem(err *applibrary.FolderCheckError) apigen.Problem {
	switch err.Problem {
	case applibrary.ProblemNotFound:
		return folderRequestProblem(CodeFolderNotFound, err.Index, "The folder does not exist.")
	case applibrary.ProblemUnreadable:
		return folderRequestProblem(CodeFolderUnreadable, err.Index, "The server cannot read the folder.")
	case applibrary.ProblemOutsideRoots:
		return folderRequestProblem(CodeFolderOutsideRoots, err.Index, "The folder is not inside a folder root.")
	case applibrary.ProblemUnknownFolder:
		return folderRequestProblem(CodeInvalid, err.Index, "The library holds no folder with this ID.")
	case applibrary.ProblemReferenceShape:
		return folderRequestProblem(CodeInvalid, err.Index, "A folder names either folderId or rootId with path.")
	}

	return folderRequestProblem(CodeInvalid, err.Index, "The folder could not be resolved.")
}

func folderProblem(err *library.FolderError) apigen.Problem {
	switch err.Reason {
	case library.ReasonReserved:
		return folderRequestProblem(CodeFolderReserved, err.Index, "The folder holds a directory the server uses.")
	case library.ReasonPath:
		return folderRequestProblem(CodeFolderOutsideRoots, err.Index, "The folder is not a clean absolute path.")
	case library.ReasonOverlap, library.ReasonDuplicate, library.ReasonNested:
		return overlapProblem(err)
	}

	return folderRequestProblem(CodeInvalid, err.Index, "The folder is not valid.")
}

// overlapProblem covers an overlap with another library and a duplicate or nested folder of
// the same one; only the first names the other library.
func overlapProblem(err *library.FolderError) apigen.Problem {
	problem := folderRequestProblem(CodeFolderOverlap, err.Index, "The folder overlaps another library folder.")
	problem.Status = http.StatusConflict
	problem.Title = http.StatusText(http.StatusConflict)

	if other, parseErr := uuid.Parse(err.OtherLibraryID); parseErr == nil {
		problem.LibraryID = &other
	}

	return problem
}

func fieldProblem(err *library.FieldError) apigen.Problem {
	switch err.Field {
	case library.FieldName:
		return fieldRequestProblem(CodeNameInvalid, fieldName, "The name is empty, too long or has unsupported characters.")
	case library.FieldKind:
		return fieldRequestProblem(CodeKindUnsupported, fieldKind, "This server cannot create libraries of this kind.")
	case library.FieldLanguage:
		return fieldRequestProblem(CodeLanguageInvalid, fieldLanguage, "The metadata language is not a valid language tag.")
	case library.FieldFolders:
		return fieldRequestProblem(CodeTooManyFolders, fieldFolders, "A library needs 1 to 16 folders.")
	default:
		return fieldRequestProblem(CodeInvalid, string(err.Field), "The value is not valid.")
	}
}

func folderRequestProblem(code string, index int, detail string) apigen.Problem {
	field := ""
	if index >= 0 {
		field = fmt.Sprintf("%s[%d]", fieldFolders, index)
	}

	return fieldRequestProblem(code, field, detail)
}

func fieldRequestProblem(code, field, detail string) apigen.Problem {
	problem := NewProblem(http.StatusBadRequest, code)
	problem.Detail = &detail

	if field != "" {
		problem.Field = &field
	}

	return problem
}
