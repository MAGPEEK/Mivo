package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
)

func TestLoadUnknownKeyIsOneLineWithKeyPathAndLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantTail string
	}{
		{name: "nested typo", yaml: "server:\n  prot: 1\n", wantTail: `line 2: unknown key "server.prot"`},
		{name: "flow mapping", yaml: "server: {prot: 1}\n", wantTail: `line 1: unknown key "server.prot"`},
		{name: "unknown section", yaml: "log:\n  level: info\nmetrics:\n  enabled: true\n", wantTail: `line 3: unknown key "metrics"`},
		{name: "key in the wrong section", yaml: "paths:\n  ffmpeg: /usr/bin/ffmpeg\n", wantTail: `line 2: unknown key "paths.ffmpeg"`},
		{name: "tools typo", yaml: "tools:\n  ffmpg: ffmpeg\n", wantTail: `line 2: unknown key "tools.ffmpg"`},
		{name: "transcode typo", yaml: "transcode:\n  max-sessions: 2\n", wantTail: `line 2: unknown key "transcode.max-sessions"`},
		{name: "library entry typo", yaml: "libraries:\n  - pth: /media\n", wantTail: `line 2: unknown key "libraries[0].pth"`},
		{
			name:     "typo in a later library entry",
			yaml:     "libraries:\n  - path: a\n  - path: b\n    title: B\n",
			wantTail: `line 4: unknown key "libraries[1].title"`,
		},
		{name: "library in flow style", yaml: "libraries: [{path: a, kinds: shows}]\n", wantTail: `line 1: unknown key "libraries[0].kinds"`},
		{name: "merge key in a library entry", yaml: "libraries:\n  - <<: {path: a}\n", wantTail: `line 2: unknown key "libraries[0].<<"`},
		{name: "library field at the top level", yaml: "path: /media\n", wantTail: `line 1: unknown key "path"`},
		{name: "leaf at the top level", yaml: "port: 1\n", wantTail: `line 1: unknown key "port"`},
		{name: "merge key", yaml: "server:\n  <<: {port: 1}\n", wantTail: `line 2: unknown key "server.<<"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, nil)

			_, err := f.load()
			if !errors.Is(err, config.ErrUnknownKey) {
				t.Fatalf("Load error = %v, want ErrUnknownKey", err)
			}

			if want := f.configFile() + ": " + tt.wantTail; err.Error() != want {
				t.Errorf("Load error = %q, want %q", err, want)
			}
		})
	}
}

func TestLoadRejectsExplicitNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantPart string
	}{
		{name: "empty path", yaml: "paths:\n  data:\n", wantPart: "line 2: key paths.data"},
		{name: "tilde level", yaml: "log:\n  level: ~\n", wantPart: "line 2: key log.level"},
		{name: "null port", yaml: "server:\n  host: ''\n  port: null\n", wantPart: "line 3: key server.port"},
		{name: "null in flow mapping", yaml: "paths: {cache: }\n", wantPart: "line 1: key paths.cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, nil)

			_, err := f.load()
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("Load error = %v, want ErrInvalid", err)
			}

			for _, part := range []string{f.configFile(), tt.wantPart, "null"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Load error = %q, want it to contain %q", err, part)
				}
			}
		})
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantLine string
	}{
		{name: "two documents", yaml: "server:\n  port: 1\n---\nlog:\n  level: debug\n", wantLine: "line 3"},
		{name: "empty second document", yaml: "server:\n  port: 1\n---\n", wantLine: "line 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, nil)

			_, err := f.load()
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("Load error = %v, want ErrInvalid", err)
			}

			for _, part := range []string{f.configFile(), tt.wantLine, "multiple YAML documents"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Load error = %q, want it to contain %q", err, part)
				}
			}
		})
	}
}

func TestLoadAcceptsLeadingDocumentMarker(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "---\nserver:\n  port: 1234\n", nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Server.Port != 1234 {
		t.Errorf("Server.Port = %d, want 1234", got.Server.Port)
	}
}

func TestLoadRejectsMalformedFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantErr  error
		wantPart string
	}{
		{name: "broken syntax", yaml: "server: [\n", wantErr: config.ErrInvalid, wantPart: "invalid YAML"},
		{name: "broken second document", yaml: "server:\n  port: 1\n---\n]\n", wantErr: config.ErrInvalid, wantPart: "invalid YAML"},
		{
			name: "mapping where a value belongs", yaml: "server:\n  port:\n    value: 1\n",
			wantErr: config.ErrInvalid, wantPart: "line 2: key server.port",
		},
		{name: "list where a value belongs", yaml: "log:\n  level: [debug]\n", wantErr: config.ErrInvalid, wantPart: "line 2: key log.level"},
		{name: "value where a section belongs", yaml: "server: 8460\n", wantErr: config.ErrInvalid, wantPart: "line 1: section server"},
		{name: "sequence at the top level", yaml: "- server\n", wantErr: config.ErrInvalid, wantPart: "line 1: top level"},
		{
			name: "duplicate key", yaml: "server:\n  port: 1\n  port: 2\n",
			wantErr: config.ErrInvalid, wantPart: "line 3: key server.port: invalid configuration value: duplicate key",
		},
		{
			name: "duplicate section", yaml: "log:\n  level: info\nlog:\n  format: json\n",
			wantErr: config.ErrInvalid, wantPart: "line 3: key log: invalid configuration value: duplicate key",
		},
		{
			name: "complex key", yaml: "server:\n  ? [port]\n  : 1\n",
			wantErr: config.ErrUnknownKey, wantPart: "line 2: unknown key in section server",
		},
		{
			name: "complex key at the top level", yaml: "? [server]\n: 1\n",
			wantErr: config.ErrUnknownKey, wantPart: "line 1: unknown key at the top level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, nil)

			_, err := f.load()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Load error = %v, want %v", err, tt.wantErr)
			}

			if want := f.configFile() + ": "; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Load error = %q, want it to start with %q", err, want)
			}

			if !strings.Contains(err.Error(), tt.wantPart) {
				t.Errorf("Load error = %q, want it to contain %q", err, tt.wantPart)
			}
		})
	}
}

func TestLoadFollowsAliases(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "paths:\n  data: &shared shared\n  cache: *shared\n", nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := filepath.Join(f.configDir, "shared")
	if got.Paths.Data != want || got.Paths.Cache != want {
		t.Errorf("Paths = %+v, want data and cache %s", got.Paths, want)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		vars     map[string]string
		wantPart string
	}{
		{name: "file port too high", yaml: "server:\n  port: 65536\n", wantPart: "line 2: key server.port"},
		{name: "file port negative", yaml: "server:\n  port: -1\n", wantPart: "line 2: key server.port"},
		{name: "file port not a number", yaml: "server:\n  host: x\n  port: http\n", wantPart: "line 3: key server.port"},
		{name: "env port not a number", vars: map[string]string{env("SERVER_PORT"): "abc"}, wantPart: env("SERVER_PORT")},
		{name: "env port too high", vars: map[string]string{env("SERVER_PORT"): "70000"}, wantPart: env("SERVER_PORT")},
		// YAML 1.2 reads yes/no/on/off as plain strings; only true and false switch the guard.
		{
			name: "file lan only yes", yaml: "server:\n  lan_only: yes\n",
			wantPart: `line 2: key server.lan_only: invalid configuration value: "yes" is not one of true, false`,
		},
		{name: "file lan only number", yaml: "server:\n  lan_only: 0\n", wantPart: "line 2: key server.lan_only"},
		{name: "env lan only one", vars: map[string]string{env("SERVER_LAN_ONLY"): "1"}, wantPart: env("SERVER_LAN_ONLY")},
		{name: "env lan only off", vars: map[string]string{env("SERVER_LAN_ONLY"): "off"}, wantPart: env("SERVER_LAN_ONLY")},
		{name: "env lan only empty", vars: map[string]string{env("SERVER_LAN_ONLY"): ""}, wantPart: env("SERVER_LAN_ONLY")},
		{name: "file unknown log level", yaml: "log:\n  level: verbose\n", wantPart: "key log.level"},
		{name: "env unknown log level", vars: map[string]string{env("LOG_LEVEL"): "trace"}, wantPart: env("LOG_LEVEL")},
		{name: "file unknown log format", yaml: "log:\n  format: xml\n", wantPart: "key log.format"},
		{name: "env unknown log format", vars: map[string]string{env("LOG_FORMAT"): "logfmt"}, wantPart: env("LOG_FORMAT")},
		{name: "file empty data path", yaml: "paths:\n  data: \"\"\n", wantPart: "key paths.data"},
		{name: "file empty cache path", yaml: "paths:\n  cache: ''\n", wantPart: "key paths.cache"},
		{name: "env empty transcode path", vars: map[string]string{env("PATHS_TRANSCODE"): ""}, wantPart: env("PATHS_TRANSCODE")},
		{name: "file empty ffmpeg", yaml: "tools:\n  ffmpeg: ''\n", wantPart: "line 2: key tools.ffmpeg"},
		{name: "file blank ffprobe", yaml: "tools:\n  ffprobe: '  '\n", wantPart: "line 2: key tools.ffprobe"},
		{name: "env empty ffprobe", vars: map[string]string{env("TOOLS_FFPROBE"): ""}, wantPart: env("TOOLS_FFPROBE")},
		{name: "file ffmpeg below home shorthand", yaml: "tools:\n  ffmpeg: ~/bin/ffmpeg\n", wantPart: "line 2: key tools.ffmpeg"},
		{name: "env ffmpeg home shorthand", vars: map[string]string{env("TOOLS_FFMPEG"): "~ffmpeg"}, wantPart: env("TOOLS_FFMPEG")},
		{name: "file max sessions zero", yaml: "transcode:\n  max_sessions: 0\n", wantPart: "line 2: key transcode.max_sessions"},
		{
			name: "file max sessions above the FFmpeg cap", yaml: "transcode:\n  max_sessions: 9\n",
			wantPart: "line 2: key transcode.max_sessions: invalid configuration value: re-encoding session limit 9 is outside 1-8; " +
				"the server runs at most 8 FFmpeg sessions at once",
		},
		{name: "env max sessions above the FFmpeg cap", vars: map[string]string{env("TRANSCODE_MAX_SESSIONS"): "9"}, wantPart: "is outside 1-8"},
		{name: "env max sessions zero", vars: map[string]string{env("TRANSCODE_MAX_SESSIONS"): "0"}, wantPart: "is outside 1-8"},
		{name: "file max sessions negative", yaml: "transcode:\n  max_sessions: -2\n", wantPart: "key transcode.max_sessions"},
		{name: "env max sessions not a number", vars: map[string]string{env("TRANSCODE_MAX_SESSIONS"): "two"}, wantPart: env("TRANSCODE_MAX_SESSIONS")},
		{name: "env max sessions empty", vars: map[string]string{env("TRANSCODE_MAX_SESSIONS"): ""}, wantPart: env("TRANSCODE_MAX_SESSIONS")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			_, err := f.load()
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("Load error = %v, want ErrInvalid", err)
			}

			if !strings.Contains(err.Error(), tt.wantPart) {
				t.Errorf("Load error = %q, want it to contain %q", err, tt.wantPart)
			}

			if tt.yaml != "" && !strings.Contains(err.Error(), f.configFile()) {
				t.Errorf("Load error = %q, want it to name %s", err, f.configFile())
			}
		})
	}
}
