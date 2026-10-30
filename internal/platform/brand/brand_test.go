package brand_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestIDIsLowercase(t *testing.T) {
	t.Parallel()

	if brand.ID == "" {
		t.Fatal("ID must not be empty")
	}

	if brand.ID != strings.ToLower(brand.ID) {
		t.Errorf("ID = %q, want lowercase", brand.ID)
	}
}

func TestEnvPrefixIsUpperCaseID(t *testing.T) {
	t.Parallel()

	if want := strings.ToUpper(brand.ID); brand.EnvPrefix != want {
		t.Errorf("EnvPrefix = %q, want %q", brand.EnvPrefix, want)
	}
}

func TestDisplayNameIsSet(t *testing.T) {
	t.Parallel()

	if strings.TrimSpace(brand.DisplayName) == "" {
		t.Error("DisplayName must not be blank")
	}
}

func TestRepositoryURLIsAbsoluteHTTPS(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(brand.RepositoryURL)
	if err != nil {
		t.Fatalf("parse RepositoryURL %q: %v", brand.RepositoryURL, err)
	}

	if parsed.Scheme != "https" || parsed.Host == "" {
		t.Errorf("RepositoryURL = %q, want absolute https URL", brand.RepositoryURL)
	}
}
