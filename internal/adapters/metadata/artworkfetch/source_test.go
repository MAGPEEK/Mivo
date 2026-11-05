package artworkfetch

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestCheckTMDBPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, path string
		valid      bool
	}{
		{"plain", "/abc_DEF-123.jpg", true},
		{"webp", "/x.webp", true},
		{"upper extension", "/x.PNG", true},
		{"no leading slash", "abc.jpg", false},
		{"dot dot", "/../../etc/passwd.jpg", false},
		{"nested", "/a/b.jpg", false},
		{"backslash", "/..\\b.jpg", false},
		{"encoded", "/%2e%2e.jpg", false},
		{"query", "/a.jpg?x=1", false},
		{"no extension", "/abc", false},
		{"empty stem", "/.jpg", false},
		{"wrong extension", "/a.svg", false},
		{"double extension", "/a.jpg.svg", false},
		{"too long", "/" + string(make([]byte, maxTMDBFileName)) + ".jpg", false},
		{"url", "https://image.tmdb.org/t/p/original/a.jpg", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkTMDBPath(test.path)
			if test.valid && err != nil {
				t.Fatalf("checkTMDBPath(%q): %v", test.path, err)
			}

			if !test.valid && !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("checkTMDBPath(%q) error = %v, want ErrInvalid", test.path, err)
			}
		})
	}
}

func TestCheckedURL(t *testing.T) {
	t.Parallel()

	hosts := CoverArtHosts()
	tests := []struct {
		name, raw string
		insecure  bool
		want      error
	}{
		{"valid", "https://coverartarchive.org/release/abc/front-500", false, nil},
		{"upper host", "https://CoverArtArchive.org/release/abc/front", false, nil},
		{"http refused", "http://coverartarchive.org/release/abc/front", false, domain.ErrInvalid},
		{"http in tests", "http://coverartarchive.org/release/abc/front", true, nil},
		{"ftp", "ftp://coverartarchive.org/x", true, domain.ErrInvalid},
		{"foreign host", "https://evil.example/release/abc/front", false, domain.ErrForbidden},
		{"subdomain not listed", "https://ia.archive.org/x.jpg", false, domain.ErrForbidden},
		{"lookalike", "https://coverartarchive.org.evil.example/x", false, domain.ErrForbidden},
		{"credentials", "https://user:pw@coverartarchive.org/x", false, domain.ErrInvalid},
		{"query", "https://coverartarchive.org/x?y=1", false, domain.ErrInvalid},
		{"fragment", "https://coverartarchive.org/x#y", false, domain.ErrInvalid},
		{"dot dot", "https://coverartarchive.org/release/../../etc/passwd", false, domain.ErrInvalid},
		{"single dot", "https://coverartarchive.org/./x", false, domain.ErrInvalid},
		{"encoded dot dot", "https://coverartarchive.org/release/%2e%2e/x", false, domain.ErrInvalid},
		{"encoded slash", "https://coverartarchive.org/release%2Fx", false, domain.ErrInvalid},
		{"backslash", "https://coverartarchive.org/release\\..\\x", false, domain.ErrInvalid},
		{"opaque", "https:coverartarchive.org", false, domain.ErrForbidden},
		{"unparsable", "https://coverartarchive.org/%zz", false, domain.ErrInvalid},
		{"relative", "/release/abc/front", false, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := checkedURL(test.raw, hosts, test.insecure)
			if test.want == nil && err != nil {
				t.Fatalf("checkedURL(%q): %v", test.raw, err)
			}

			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("checkedURL(%q) error = %v, want %v", test.raw, err, test.want)
			}
		})
	}
}

func TestRefTag(t *testing.T) {
	t.Parallel()

	poster := enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/a.jpg"}
	backdrop := poster
	backdrop.Kind = artwork.KindBackdrop
	other := poster
	other.Provider = enrich.ArtworkProviderCoverArt

	if !isTag(refTag(poster)) {
		t.Fatalf("refTag = %q, want %d hex digits", refTag(poster), artwork.TagLength)
	}

	if again := poster; refTag(poster) != refTag(again) {
		t.Fatal("refTag must be stable")
	}

	if refTag(poster) == refTag(backdrop) {
		t.Fatal("kinds must not share a file: the URL builder may pick another size per kind")
	}

	if refTag(poster) == refTag(other) {
		t.Fatal("different providers must not share a file")
	}
}

func TestIsStoredDataPath(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"artwork/0123456789abcdef.jpg":     true,
		"artwork/0123456789abcdef.webp":    true,
		"artwork/0123456789ABCDEF.jpg":     false,
		"artwork/0123456789abcde.jpg":      false,
		"artwork/0123456789abcdeg.jpg":     false,
		"artwork/0123456789abcdef.gif":     false,
		"artwork/0123456789abcdef":         false,
		"artwork/../config.yaml":           false,
		"../artwork/0123456789abcdef.png":  false,
		"artwork/sub/0123456789abcdef.png": false,
		"/artwork/0123456789abcdef.png":    false,
		"mivo.db":                          false,
	}

	for dataPath, want := range tests {
		if got := isStoredDataPath(dataPath); got != want {
			t.Errorf("isStoredDataPath(%q) = %v, want %v", dataPath, got, want)
		}
	}
}
