package config_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// transcodeCases cover the transcoder keys of milestone 4 (ADR 0030): default, a file
// value and an environment override per key.
func transcodeCases() []precedenceCase {
	return []precedenceCase{
		{
			key: "transcode.preset", yaml: "transcode:\n  preset: ultrafast\n",
			envName: env("TRANSCODE_PRESET"), envValue: "Medium",
			wantFile: "ultrafast", wantEnv: "medium",
			defaultOf: func(paths.DefaultDirectories) string { return "veryfast" },
			get:       func(cfg config.Config) string { return cfg.Transcode.Preset },
		},
		{
			key: "transcode.threads", yaml: "transcode:\n  threads: 4\n",
			envName: env("TRANSCODE_THREADS"), envValue: "64",
			wantFile: "4", wantEnv: "64",
			defaultOf: func(paths.DefaultDirectories) string { return "0" },
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Transcode.Threads) },
		},
		{
			key: "transcode.priority", yaml: "transcode:\n  priority: normal\n",
			envName: env("TRANSCODE_PRIORITY"), envValue: "LOW",
			wantFile: "normal", wantEnv: "low",
			defaultOf: func(paths.DefaultDirectories) string { return "low" },
			get:       func(cfg config.Config) string { return cfg.Transcode.Priority },
		},
		{
			key: "transcode.throttle_ahead_seconds", yaml: "transcode:\n  throttle_ahead_seconds: 600\n",
			envName: env("TRANSCODE_THROTTLE_AHEAD_SECONDS"), envValue: "0",
			wantFile: "600", wantEnv: "0",
			defaultOf: func(paths.DefaultDirectories) string { return "60" },
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Transcode.ThrottleAheadSeconds) },
		},
		{
			key: "transcode.hwaccel", yaml: "transcode:\n  hwaccel: vaapi\n",
			envName: env("TRANSCODE_HWACCEL"), envValue: "off",
			wantFile: "vaapi", wantEnv: "off",
			defaultOf: func(paths.DefaultDirectories) string { return "auto" },
			get:       func(cfg config.Config) string { return cfg.Transcode.HWAccel },
		},
		{
			key: "transcode.hwaccel_device", yaml: "transcode:\n  hwaccel_device: /dev/dri/renderD128\n",
			envName: env("TRANSCODE_HWACCEL_DEVICE"), envValue: "",
			wantFile: "/dev/dri/renderD128", wantEnv: "",
			defaultOf: func(paths.DefaultDirectories) string { return "" },
			get:       func(cfg config.Config) string { return cfg.Transcode.HWAccelDevice },
		},
		{
			key: "transcode.allow_burn_in", yaml: "transcode:\n  allow_burn_in: false\n",
			envName: env("TRANSCODE_ALLOW_BURN_IN"), envValue: "True",
			wantFile: "false", wantEnv: "true",
			defaultOf: func(paths.DefaultDirectories) string { return "true" },
			get:       func(cfg config.Config) string { return strconv.FormatBool(cfg.Transcode.AllowBurnIn) },
		},
	}
}

func TestLoadTranscodeAcceptsEveryAllowedValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		envSuffix string
		values    []string
		get       func(cfg config.Config) string
	}{
		{
			envSuffix: "TRANSCODE_PRESET",
			values:    []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium"},
			get:       func(cfg config.Config) string { return cfg.Transcode.Preset },
		},
		{
			envSuffix: "TRANSCODE_HWACCEL",
			values:    []string{"auto", "off", "vaapi", "qsv", "nvenc", "amf"},
			get:       func(cfg config.Config) string { return cfg.Transcode.HWAccel },
		},
		{
			envSuffix: "TRANSCODE_PRIORITY",
			values:    []string{"low", "normal"},
			get:       func(cfg config.Config) string { return cfg.Transcode.Priority },
		},
		{
			envSuffix: "TRANSCODE_THREADS",
			values:    []string{"0", "1", "64"},
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Transcode.Threads) },
		},
		{
			envSuffix: "TRANSCODE_THROTTLE_AHEAD_SECONDS",
			values:    []string{"0", "20", "600"},
			get:       func(cfg config.Config) string { return strconv.Itoa(cfg.Transcode.ThrottleAheadSeconds) },
		},
	}

	for _, tt := range tests {
		for _, value := range tt.values {
			t.Run(tt.envSuffix+"="+value, func(t *testing.T) {
				t.Parallel()

				f := newFixture(t, "", map[string]string{env(tt.envSuffix): value})

				got, err := f.load()
				if err != nil {
					t.Fatalf("Load: %v", err)
				}

				if gotValue := tt.get(got); gotValue != value {
					t.Errorf("%s = %q, want %q", tt.envSuffix, gotValue, value)
				}
			})
		}
	}
}

func TestLoadTranscodeInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		vars     map[string]string
		wantPart string
	}{
		{
			name: "file preset slow", yaml: "transcode:\n  preset: slow\n",
			wantPart: `line 2: key transcode.preset: invalid configuration value: "slow" is not one of ` +
				"ultrafast, superfast, veryfast, faster, fast, medium",
		},
		{name: "env preset empty", vars: map[string]string{env("TRANSCODE_PRESET"): ""}, wantPart: env("TRANSCODE_PRESET")},
		{
			name: "file threads above the maximum", yaml: "transcode:\n  threads: 65\n",
			wantPart: "line 2: key transcode.threads: invalid configuration value: thread count 65 is outside 0-64; use 0 for auto",
		},
		{name: "env threads negative", vars: map[string]string{env("TRANSCODE_THREADS"): "-1"}, wantPart: "is outside 0-64"},
		{name: "env threads text", vars: map[string]string{env("TRANSCODE_THREADS"): "auto"}, wantPart: env("TRANSCODE_THREADS")},
		{
			name: "file priority high", yaml: "transcode:\n  priority: high\n",
			wantPart: `line 2: key transcode.priority: invalid configuration value: "high" is not one of low, normal`,
		},
		{name: "env priority empty", vars: map[string]string{env("TRANSCODE_PRIORITY"): ""}, wantPart: env("TRANSCODE_PRIORITY")},
		{
			name: "file throttle below the minimum", yaml: "transcode:\n  throttle_ahead_seconds: 19\n",
			wantPart: "line 2: key transcode.throttle_ahead_seconds: invalid configuration value: " +
				"throttle lead 19 seconds is outside 20-600; use 0 to turn throttling off",
		},
		{name: "env throttle above the maximum", vars: map[string]string{env("TRANSCODE_THROTTLE_AHEAD_SECONDS"): "601"}, wantPart: "is outside 20-600"},
		{name: "env throttle negative", vars: map[string]string{env("TRANSCODE_THROTTLE_AHEAD_SECONDS"): "-60"}, wantPart: "is outside 20-600"},
		{name: "env throttle with unit", vars: map[string]string{env("TRANSCODE_THROTTLE_AHEAD_SECONDS"): "60s"}, wantPart: "is not a number of seconds"},
		{
			name: "file hwaccel cuda", yaml: "transcode:\n  hwaccel: cuda\n",
			wantPart: `line 2: key transcode.hwaccel: invalid configuration value: "cuda" is not one of ` +
				"auto, off, vaapi, qsv, nvenc, amf",
		},
		{name: "env hwaccel empty", vars: map[string]string{env("TRANSCODE_HWACCEL"): ""}, wantPart: env("TRANSCODE_HWACCEL")},
		{
			name: "file hwaccel device relative", yaml: "transcode:\n  hwaccel_device: dri/renderD128\n",
			wantPart: `line 2: key transcode.hwaccel_device: invalid configuration value: "dri/renderD128" is not an absolute path; ` +
				"leave it empty for the default device",
		},
		{name: "env hwaccel device relative", vars: map[string]string{env("TRANSCODE_HWACCEL_DEVICE"): "renderD128"}, wantPart: env("TRANSCODE_HWACCEL_DEVICE")},
		{
			name: "file allow burn in yes", yaml: "transcode:\n  allow_burn_in: yes\n",
			wantPart: `line 2: key transcode.allow_burn_in: invalid configuration value: "yes" is not one of true, false`,
		},
		{name: "env allow burn in empty", vars: map[string]string{env("TRANSCODE_ALLOW_BURN_IN"): ""}, wantPart: env("TRANSCODE_ALLOW_BURN_IN")},
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
		})
	}
}

func TestLoadMaxSessionsDefaultFollowsCPUCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cpuCount int
		yaml     string
		vars     map[string]string
		want     int
	}{
		{name: "one CPU", cpuCount: 1, want: 1},
		{name: "four CPUs", cpuCount: 4, want: 1},
		{name: "five CPUs", cpuCount: 5, want: config.DefaultTranscodeMaxSessions},
		{name: "sixteen CPUs", cpuCount: 16, want: config.DefaultTranscodeMaxSessions},
		{name: "file value wins on a small machine", cpuCount: 2, yaml: "transcode:\n  max_sessions: 3\n", want: 3},
		{name: "env value wins on a small machine", cpuCount: 4, vars: map[string]string{env("TRANSCODE_MAX_SESSIONS"): "2"}, want: 2},
		{name: "file value wins on a large machine", cpuCount: 32, yaml: "transcode:\n  max_sessions: 1\n", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.yaml, tt.vars)

			got, err := config.Load(config.Options{
				ConfigDirFlag: f.configDir, Environment: f.env, NumCPU: func() int { return tt.cpuCount },
			})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if got.Transcode.MaxSessions != tt.want {
				t.Errorf("MaxSessions = %d, want %d", got.Transcode.MaxSessions, tt.want)
			}
		})
	}
}

func TestLoadWithoutNumCPUUsesTheHost(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)

	got, err := config.Load(config.Options{ConfigDirFlag: f.configDir, Environment: f.env})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Transcode.MaxSessions < 1 || got.Transcode.MaxSessions > config.DefaultTranscodeMaxSessions {
		t.Errorf("MaxSessions = %d, want 1-%d", got.Transcode.MaxSessions, config.DefaultTranscodeMaxSessions)
	}
}
