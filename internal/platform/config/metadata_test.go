package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const secretKey = "0123456789abcdef-secret"

// metadataCases cover the online metadata keys of milestone 6 (ADR 0031).
func metadataCases() []precedenceCase {
	boolCase := func(key, yamlSection, envSuffix string, get func(config.Config) bool) precedenceCase {
		return precedenceCase{
			key: key, yaml: yamlSection, envName: env(envSuffix), envValue: "False",
			wantFile: "false", wantEnv: "false",
			defaultOf: func(paths.DefaultDirectories) string { return "true" },
			get:       func(cfg config.Config) string { return strconv.FormatBool(get(cfg)) },
		}
	}

	return []precedenceCase{
		boolCase("metadata.online.enabled", "metadata:\n  online:\n    enabled: false\n",
			"METADATA_ONLINE_ENABLED", func(c config.Config) bool { return c.Metadata.Online }),
		boolCase("metadata.tmdb.enabled", "metadata:\n  tmdb:\n    enabled: false\n",
			"METADATA_TMDB_ENABLED", func(c config.Config) bool { return c.Metadata.TMDB.Enabled }),
		boolCase("metadata.musicbrainz.enabled", "metadata:\n  musicbrainz:\n    enabled: false\n",
			"METADATA_MUSICBRAINZ_ENABLED", func(c config.Config) bool { return c.Metadata.MusicBrainz.Enabled }),
		{
			key: "metadata.language", yaml: "metadata:\n  language: de\n",
			envName: env("METADATA_LANGUAGE"), envValue: "pt-br",
			wantFile: "de", wantEnv: "pt-BR",
			defaultOf: func(paths.DefaultDirectories) string { return "en" },
			get:       func(cfg config.Config) string { return cfg.Metadata.Language },
		},
		{
			key: "metadata.tmdb.api_key", yaml: "metadata:\n  tmdb:\n    api_key: file-key\n",
			envName: env("METADATA_TMDB_API_KEY"), envValue: " env-key ",
			wantFile: "file-key", wantEnv: "env-key",
			defaultOf: func(paths.DefaultDirectories) string { return "" },
			get:       func(cfg config.Config) string { return string(cfg.Metadata.TMDB.APIKey) },
		},
	}
}

func TestMetadataInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, key, value string }{
		{"online not a bool", "METADATA_ONLINE_ENABLED", "yes"},
		{"tmdb not a bool", "METADATA_TMDB_ENABLED", "1"},
		{"language empty", "METADATA_LANGUAGE", ""},
		{"language too long", "METADATA_LANGUAGE", "english"},
		{"language digits", "METADATA_LANGUAGE", "e1"},
		{"language bad region", "METADATA_LANGUAGE", "en-USA"},
		{"key with space", "METADATA_TMDB_API_KEY", "ab cd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := newFixture(t, "", map[string]string{env(tt.key): tt.value}).load()
			if err == nil || !strings.Contains(err.Error(), env(tt.key)) {
				t.Fatalf("load error = %v, want one naming %s", err, env(tt.key))
			}
		})
	}
}

func TestMetadataInvalidValueNeverEchoesKey(t *testing.T) {
	t.Parallel()

	_, err := newFixture(t, "", map[string]string{env("METADATA_TMDB_API_KEY"): secretKey + " x"}).load()
	if err == nil {
		t.Fatal("want an error for a key with a space")
	}

	if strings.Contains(err.Error(), secretKey) {
		t.Errorf("error %q leaks the key", err)
	}
}

// The tests below swap the package variable that -ldflags sets, so they must not run in parallel.
func TestTMDBKeyOrigin(t *testing.T) {
	tests := []struct {
		name, bundled, own string
		wantKey            string
		wantOrigin         config.KeyOrigin
	}{
		{"none", "", "", "", config.KeyOriginNone},
		{"bundled only", "bundled", "", "bundled", config.KeyOriginBundled},
		{"own only", "", "own", "own", config.KeyOriginOwn},
		{"own wins", "bundled", "own", "own", config.KeyOriginOwn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBundledKey(t, tt.bundled)

			cfg, err := newFixture(t, "", map[string]string{env("METADATA_TMDB_API_KEY"): tt.own}).load()
			if err != nil {
				t.Fatalf("load: %v", err)
			}

			key, origin := cfg.TMDBKey()
			if key != tt.wantKey || origin != tt.wantOrigin {
				t.Errorf("TMDBKey() = (%q, %q), want (%q, %q)", key, origin, tt.wantKey, tt.wantOrigin)
			}
		})
	}
}

func TestOwnKeyFromFileWinsOverBundled(t *testing.T) {
	setBundledKey(t, "bundled")

	cfg, err := newFixture(t, "metadata:\n  tmdb:\n    api_key: "+secretKey+"\n", nil).load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if key, origin := cfg.TMDBKey(); key != secretKey || origin != config.KeyOriginOwn {
		t.Errorf("TMDBKey() = (%q, %q), want own key", key, origin)
	}
}

func setBundledKey(t *testing.T, value string) {
	t.Helper()

	previous := config.BundledTMDBKey
	config.BundledTMDBKey = value

	t.Cleanup(func() { config.BundledTMDBKey = previous })
}

func TestKeyNeverAppearsInDumps(t *testing.T) {
	t.Parallel()

	cfg, err := newFixture(t, "", map[string]string{env("METADATA_TMDB_API_KEY"): secretKey}).load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var logs bytes.Buffer

	slog.New(slog.NewJSONHandler(&logs, nil)).Info("config", "config", cfg, "metadata", cfg.Metadata, "key", cfg.Metadata.TMDB.APIKey)
	slog.New(slog.NewTextHandler(&logs, nil)).Info("config", "config", cfg, "key", cfg.Metadata.TMDB.APIKey)

	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dumps := []string{
		logs.String(), string(encoded),
		fmt.Sprintf("%v|%+v|%#v|%s", cfg, cfg, cfg, cfg.Metadata.TMDB.APIKey),
	}

	for _, dump := range dumps {
		if strings.Contains(dump, secretKey) {
			t.Errorf("dump leaks the key: %.200s", dump)
		}
	}
}

func TestSecretRedactionMarksPresence(t *testing.T) {
	t.Parallel()

	if got := config.Secret("").String(); got != "" {
		t.Errorf("empty secret = %q, want empty", got)
	}

	if got := config.Secret("x").String(); got == "" || strings.Contains(got, "x") {
		t.Errorf("set secret = %q, want a redaction marker", got)
	}
}
