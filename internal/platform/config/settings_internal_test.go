package config

import (
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/magpeek/mivo/internal/platform/brand"
)

// nestedYAML turns "a.b" and "x" into "a:\n  b: x\n".
func nestedYAML(key, value string) string {
	var builder strings.Builder

	parts := strings.Split(key, keySeparator)
	for depth, part := range parts {
		builder.WriteString(strings.Repeat("  ", depth) + part + ":")

		if depth == len(parts)-1 {
			builder.WriteString(" " + value)
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// The walker derives the accepted keys from the settings table; every setting must be
// reachable through config.yaml.
func TestEverySettingIsAcceptedInTheFile(t *testing.T) {
	t.Parallel()

	for _, entry := range settings() {
		content, err := parseFile([]byte(nestedYAML(entry.key, "x")))
		if err != nil {
			t.Fatalf("parseFile(%s): %v", entry.key, err)
		}

		if got := content.values[entry.key]; got.raw != "x" || got.line != len(parentSections(entry.key))+1 {
			t.Errorf("parseFile(%s) value = %+v, want raw x on the last line", entry.key, got)
		}
	}
}

func TestParentSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want []string
	}{
		{key: "server", want: []string{}},
		{key: "server.port", want: []string{"server"}},
		{key: "a.b.c", want: []string{"a", "a.b"}},
	}

	for _, tt := range tests {
		if got := parentSections(tt.key); !slices.Equal(got, tt.want) {
			t.Errorf("parentSections(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestRootOfDocumentWithoutContentIsNull(t *testing.T) {
	t.Parallel()

	if root := rootOf(&yaml.Node{Kind: yaml.DocumentNode}); !isNull(root) {
		t.Errorf("rootOf(empty document) = %+v, want a null node", root)
	}
}

func TestEnvName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want string
	}{
		{key: "server.port", want: brand.EnvPrefix + "_SERVER_PORT"},
		{key: "server.lan_only", want: brand.EnvPrefix + "_SERVER_LAN_ONLY"},
		{key: "paths.transcode", want: brand.EnvPrefix + "_PATHS_TRANSCODE"},
		{key: "transcode.max_sessions", want: brand.EnvPrefix + "_TRANSCODE_MAX_SESSIONS"},
		{key: "tools.ffprobe", want: brand.EnvPrefix + "_TOOLS_FFPROBE"},
		{key: keyLibraries, want: brand.EnvPrefix + "_LIBRARIES"},
	}

	for _, tt := range tests {
		if got := envName(tt.key); got != tt.want {
			t.Errorf("envName(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}

	for _, entry := range settings() {
		if name := envName(entry.key); strings.ToUpper(name) != name {
			t.Errorf("envName(%q) = %q, want upper case", entry.key, name)
		}
	}
}
