package library

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func folders(paths ...string) []Folder {
	result := make([]Folder, 0, len(paths))
	for _, path := range paths {
		result = append(result, Folder{Path: path})
	}

	return result
}

func candidateWith(modify func(*ManagedLibrary), paths ...string) ManagedLibrary {
	candidate := ManagedLibrary{ID: "me", Name: "Films", Kind: KindMovies, Folders: folders(paths...)}
	if modify != nil {
		modify(&candidate)
	}

	return candidate
}

func TestValidateLibrary(t *testing.T) {
	t.Parallel()

	kinds := []Kind{KindMovies}
	other := ManagedLibrary{ID: "other", Folders: folders("/media/tv")}
	rules := FolderRules{Reserved: []string{"/data"}, OtherLibraries: []ManagedLibrary{other}}
	winRules := FolderRules{CaseInsensitive: true}
	many := make([]string, MaxFolders+1)

	for i := range many {
		many[i] = "/m/" + string(rune('a'+i))
	}

	tests := []struct {
		name      string
		candidate ManagedLibrary
		rules     FolderRules
		field     Field
		reason    FolderReason
		index     int
		otherID   string

		emptyKinds bool
	}{
		{name: "valid", candidate: candidateWith(nil, "/media/movies", "/media/more"), rules: rules},
		{name: "valid language", candidate: candidateWith(func(c *ManagedLibrary) { c.MetadataLanguage = "de-de" }, "/a"), rules: rules},
		{name: "blank name", candidate: candidateWith(func(c *ManagedLibrary) { c.Name = " " }, "/a"), rules: rules, field: FieldName},
		{name: "kind not allowed", candidate: candidateWith(func(c *ManagedLibrary) { c.Kind = KindShows }, "/a"), rules: rules, field: FieldKind},
		{name: "no folders", candidate: candidateWith(nil), rules: rules, field: FieldFolders},
		{name: "too many", candidate: candidateWith(nil, many...), rules: rules, field: FieldFolders},
		{name: "bad language", candidate: candidateWith(func(c *ManagedLibrary) { c.MetadataLanguage = "english" }, "/a"), rules: rules, field: FieldLanguage},
		{name: "blank folder", candidate: candidateWith(nil, "/a", " "), rules: rules, reason: ReasonPath, index: 1},
		{name: "NUL folder", candidate: candidateWith(nil, "/a\x00b"), rules: rules, reason: ReasonPath},
		{name: "relative", candidate: candidateWith(nil, "media/x"), rules: rules, reason: ReasonPath},
		{name: "dotdot bypass", candidate: candidateWith(nil, "/media/../data"), rules: rules, reason: ReasonPath},
		{name: "dot element", candidate: candidateWith(nil, "/media/./x"), rules: rules, reason: ReasonPath},
		{name: "drive without separator", candidate: candidateWith(nil, "C:x"), rules: winRules, reason: ReasonPath},
		{name: "drive ok", candidate: candidateWith(nil, `C:\media`), rules: winRules},
		{name: "unc ok", candidate: candidateWith(nil, `\\nas\share\x`), rules: winRules},
		{name: "empty allowed kinds", candidate: candidateWith(nil, "/a"), rules: FolderRules{}, field: FieldKind, emptyKinds: true},
		{name: "win reserved backslash", candidate: candidateWith(nil, `D:\Data\x\`), rules: FolderRules{CaseInsensitive: true, Reserved: []string{`d:\data`}}, reason: ReasonReserved},
		{name: "win overlap", candidate: candidateWith(nil, `C:\`), rules: FolderRules{CaseInsensitive: true, OtherLibraries: []ManagedLibrary{{ID: "w", Folders: folders(`c:\Media`)}}}, reason: ReasonOverlap, otherID: "w"},
		{name: "unc overlap", candidate: candidateWith(nil, `\\NAS\Share\a`), rules: FolderRules{CaseInsensitive: true, OtherLibraries: []ManagedLibrary{{ID: "u", Folders: folders(`\\nas\share`)}}}, reason: ReasonOverlap, otherID: "u"},
		{name: "duplicate", candidate: candidateWith(nil, "/a", "/a/"), rules: rules, reason: ReasonDuplicate, index: 1},
		{name: "nested inside", candidate: candidateWith(nil, "/a", "/a/b"), rules: rules, reason: ReasonNested, index: 1},
		{name: "nested containing", candidate: candidateWith(nil, "/a/b", "/a"), rules: rules, reason: ReasonNested, index: 1},
		{name: "sibling prefix ok", candidate: candidateWith(nil, "/media/movie", "/media/movies2"), rules: rules},
		{name: "overlap equal", candidate: candidateWith(nil, "/media/tv"), rules: rules, reason: ReasonOverlap, otherID: "other"},
		{name: "overlap inside", candidate: candidateWith(nil, "/media/tv/x"), rules: rules, reason: ReasonOverlap, otherID: "other"},
		{name: "overlap containing", candidate: candidateWith(nil, "/a", "/media"), rules: rules, reason: ReasonOverlap, index: 1, otherID: "other"},
		{name: "reserved inside", candidate: candidateWith(nil, "/data/x"), rules: rules, reason: ReasonReserved},
		{name: "reserved containing", candidate: candidateWith(nil, "/"), rules: rules, reason: ReasonReserved},
		{name: "case sensitive differs", candidate: candidateWith(nil, "/Media/TV"), rules: rules},
		{name: "own library skipped", candidate: candidateWith(func(c *ManagedLibrary) { c.ID = "other" }, "/media/tv"), rules: rules},
		{name: "windows case", candidate: candidateWith(nil, `C:\Media`, `c:/media`), rules: winRules, reason: ReasonDuplicate, index: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			allowed := kinds
			if tt.emptyKinds {
				allowed = nil
			}

			err := ValidateLibrary(tt.candidate, allowed, tt.rules)
			switch {
			case tt.field == "" && tt.reason == "":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			case tt.field != "":
				assertFieldError(t, err, tt.field)
			default:
				assertFolderError(t, err, FolderError{Index: tt.index, Reason: tt.reason, OtherLibraryID: tt.otherID})
			}
		})
	}
}

func assertFieldError(t *testing.T, err error, field Field) {
	t.Helper()

	var fieldErr *FieldError
	if !errors.Is(err, domain.ErrInvalid) || !errors.As(err, &fieldErr) || fieldErr.Field != field {
		t.Fatalf("want invalid field %s, got %v", field, err)
	}
}

func assertFolderError(t *testing.T, err error, want FolderError) {
	t.Helper()

	var folderErr *FolderError
	if !errors.Is(err, domain.ErrInvalid) || !errors.As(err, &folderErr) || *folderErr != want {
		t.Fatalf("want %+v, got %v", want, err)
	}
}

func TestValidateLibraryKeepsLanguageText(t *testing.T) {
	t.Parallel()

	// Normalisation is ParseMetadataLanguage's job; ValidateLibrary only accepts any case.
	for _, language := range []string{"DE-de", "pt", ""} {
		candidate := candidateWith(func(c *ManagedLibrary) { c.MetadataLanguage = language }, "/a")
		if err := ValidateLibrary(candidate, []Kind{KindMovies}, FolderRules{}); err != nil {
			t.Errorf("language %q: %v", language, err)
		}
	}
}

func TestErrorTexts(t *testing.T) {
	t.Parallel()

	if (&FolderError{Index: 1, Reason: ReasonNested}).Error() == "" ||
		(&FolderError{Reason: ReasonOverlap, OtherLibraryID: "x"}).Error() == "" ||
		(&FieldError{Field: FieldName}).Error() == "" {
		t.Fatal("empty error text")
	}
}

func TestFolderKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		fold bool
		want string
	}{
		{"/media/Movies/", false, "media/Movies"},
		{`C:\Media\Film`, true, "c:/media/film"},
		{`/a\b`, false, `a\b`},
	}
	for _, tt := range tests {
		if got := FolderKey(tt.path, tt.fold); got != tt.want {
			t.Errorf("FolderKey(%q,%v) = %q, want %q", tt.path, tt.fold, got, tt.want)
		}
	}
}

func TestFromDefinition(t *testing.T) {
	t.Parallel()

	got := FromDefinition(Definition{Name: "F", Root: "/r", Kind: KindShows})
	if got.Name != "F" || got.Kind != KindShows || len(got.Folders) != 1 ||
		got.Folders[0].Path != "/r" || !got.ImportedFromConfig || got.Options != DefaultOptions() {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestParseMetadataLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
		bad      bool
	}{
		{"", "", false},
		{"de", "de", false},
		{"DE", "de", false},
		{"de-de", "de-DE", false},
		{"fil", "fil", false},
		{"PT-br", "pt-BR", false},
		{"d", "", true},
		{"deut", "", true},
		{"de-", "", true},
		{"de-deu", "", true},
		{"d1", "", true},
		{"de_DE", "", true},
		{"-DE", "", true},
	}
	for _, tt := range tests {
		got, err := ParseMetadataLanguage(tt.in)
		if tt.bad != (err != nil) || (tt.bad && !errors.Is(err, domain.ErrInvalid)) || got != tt.want {
			t.Errorf("ParseMetadataLanguage(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestIsHiddenFolderName(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		".git": true, "@eaDir": true, "@appdata": true, "#recycle": true, "#Recycle": true,
		"#snapshot": true, "$RECYCLE.BIN": true, "System Volume Information": true,
		"system volume information": true, "lost+found": true,
		"recycle": false, "a@b": false, "Movies": false, "snapshot": false, "": false,
	}
	for name, want := range tests {
		if got := IsHiddenFolderName(name); got != want {
			t.Errorf("IsHiddenFolderName(%q) = %v, want %v", name, got, want)
		}
	}
}
