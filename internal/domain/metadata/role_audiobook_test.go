package metadata_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestParseAudiobookRoles(t *testing.T) {
	t.Parallel()

	for _, want := range []metadata.Role{metadata.RoleAuthor, metadata.RoleNarrator} {
		if got, err := metadata.ParseRole(want.String()); err != nil || got != want {
			t.Errorf("ParseRole(%q) = %q, %v", want, got, err)
		}
	}
}
