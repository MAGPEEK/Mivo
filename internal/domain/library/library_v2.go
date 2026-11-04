package library

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// MaxFolders is the most folders one library may combine.
const MaxFolders = 16

// Folder is one media folder of a library. Path is cleaned and absolute.
type Folder struct {
	ID   string
	Path string
}

// ManagedLibrary is a library as managed through the UI: several folders, a metadata
// language and options. It is distinct from Library, the scan view with a single root,
// which the scan keeps using until it moves to folders.
type ManagedLibrary struct {
	ID                 string
	Name               string
	Kind               Kind
	Folders            []Folder
	MetadataLanguage   string
	Options            Options
	ImportedFromConfig bool
}

// Field names a library field in a FieldError.
type Field string

// Fields that ValidateLibrary reports.
const (
	FieldName     Field = "name"
	FieldKind     Field = "kind"
	FieldLanguage Field = "language"
	FieldFolders  Field = "folders"
)

// FieldError reports an invalid non-folder field. It wraps domain.ErrInvalid.
type FieldError struct {
	Field  Field
	Reason string
}

// Error implements error.
func (e *FieldError) Error() string {
	return fmt.Sprintf("library %s: %s", e.Field, e.Reason)
}

// Unwrap lets errors.Is match domain.ErrInvalid.
func (e *FieldError) Unwrap() error { return domain.ErrInvalid }

// ValidateLibrary checks a candidate: name, kind (against allowedKinds), 1..MaxFolders
// folders, language and the folder rules. The first violation is returned; every error
// wraps domain.ErrInvalid (FieldError or FolderError).
func ValidateLibrary(candidate ManagedLibrary, allowedKinds []Kind, rules FolderRules) error {
	if err := validateName(candidate.Name); err != nil {
		return fmt.Errorf("validate library: %w", &FieldError{Field: FieldName, Reason: err.Error()})
	}

	if !kindAllowed(candidate.Kind, allowedKinds) {
		return fmt.Errorf("validate library %q: %w", candidate.Name,
			&FieldError{Field: FieldKind, Reason: fmt.Sprintf("kind %q is not available", candidate.Kind)})
	}

	if len(candidate.Folders) == 0 || len(candidate.Folders) > MaxFolders {
		reason := fmt.Sprintf("need 1 to %d folders, got %d", MaxFolders, len(candidate.Folders))

		return fmt.Errorf("validate library %q: %w", candidate.Name,
			&FieldError{Field: FieldFolders, Reason: reason})
	}

	if _, err := ParseMetadataLanguage(candidate.MetadataLanguage); err != nil {
		return fmt.Errorf("validate library %q: %w", candidate.Name,
			&FieldError{Field: FieldLanguage, Reason: err.Error()})
	}

	if err := rules.Check(candidate); err != nil {
		return fmt.Errorf("validate library %q: %w", candidate.Name, err)
	}

	return nil
}

func kindAllowed(kind Kind, allowed []Kind) bool {
	for _, candidate := range allowed {
		if candidate == kind {
			return true
		}
	}

	return false
}

// FromDefinition turns a configuration Definition into a one-folder candidate flagged as
// imported. The library and folder IDs are assigned by the store.
func FromDefinition(definition Definition) ManagedLibrary {
	return ManagedLibrary{
		Name:               definition.Name,
		Kind:               definition.Kind,
		Folders:            []Folder{{Path: definition.Root}},
		Options:            DefaultOptions(),
		ImportedFromConfig: true,
	}
}

// pathElements splits a cleaned path into elements. Both separators count when folding
// case, because that is the Windows mode where "\" separates; elsewhere "\" is a name byte.
func pathElements(path string, caseInsensitive bool) []string {
	if caseInsensitive {
		path = strings.ReplaceAll(path, `\`, "/")
		path = strings.ToLower(path)
	}

	var elements []string

	for _, element := range strings.Split(path, "/") {
		if element != "" {
			elements = append(elements, element)
		}
	}

	return elements
}

// FolderKey is the comparison key for folder uniqueness: elements joined with "/", case
// folded when caseInsensitive.
func FolderKey(path string, caseInsensitive bool) string {
	return strings.Join(pathElements(path, caseInsensitive), "/")
}
