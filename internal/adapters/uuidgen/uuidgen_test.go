package uuidgen_test

import (
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/domain/system"
)

// The adapter must satisfy the port it is wired into.
var _ appsystem.IDGenerator = uuidgen.Generator{}

func TestNewIDReturnsUniqueCanonicalVersion4UUIDs(t *testing.T) {
	t.Parallel()

	const (
		calls         = 16
		versionOffset = 14
		variantOffset = 19
		versionDigit  = '4'
		variantDigits = "89ab"
	)

	generator := uuidgen.New()
	seen := make(map[string]bool, calls)

	for range calls {
		id, err := generator.NewID()
		if err != nil {
			t.Fatalf("NewID error = %v", err)
		}

		if _, err := system.ParseServerID(id); err != nil {
			t.Errorf("NewID = %q, not a canonical lowercase UUID: %v", id, err)
		}

		if id[versionOffset] != versionDigit || !strings.ContainsRune(variantDigits, rune(id[variantOffset])) {
			t.Errorf("NewID = %q, want a version 4 RFC 9562 variant UUID", id)
		}

		if seen[id] {
			t.Errorf("NewID returned %q twice", id)
		}

		seen[id] = true
	}
}
