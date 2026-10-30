package config

import (
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
)

// The settings table and the YAML file model list the keys twice; this keeps them aligned.
func TestEverySettingHasAFileValue(t *testing.T) {
	t.Parallel()

	values := fileConfig{}.values()
	if len(values) != len(settings()) {
		t.Errorf("file model has %d keys, settings table has %d", len(values), len(settings()))
	}

	for _, entry := range settings() {
		if _, ok := values[entry.key]; !ok {
			t.Errorf("setting %s has no field in the YAML file model", entry.key)
		}
	}
}

func TestEnvName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want string
	}{
		{key: "server.port", want: brand.EnvPrefix + "_SERVER_PORT"},
		{key: "paths.transcode", want: brand.EnvPrefix + "_PATHS_TRANSCODE"},
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
