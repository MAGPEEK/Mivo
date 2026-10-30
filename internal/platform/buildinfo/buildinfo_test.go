package buildinfo

import (
	"runtime/debug"
	"testing"
)

const testGoVersion = "go1.27.0"

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		build    *debug.BuildInfo
		override string
		want     string
	}{
		{
			name:     "linker override wins over module version",
			build:    buildWithModuleVersion("v1.2.3"),
			override: "9.9.9",
			want:     "9.9.9",
		},
		{
			name:     "linker override without build info",
			build:    nil,
			override: "9.9.9",
			want:     "9.9.9",
		},
		{
			name:  "module version when no override",
			build: buildWithModuleVersion("v0.0.0-20261030120000-abcdef123456"),
			want:  "v0.0.0-20261030120000-abcdef123456",
		},
		{
			name:  "devel module version falls back to dev version",
			build: buildWithModuleVersion("(devel)"),
			want:  devVersion,
		},
		{
			name:  "empty module version falls back to dev version",
			build: buildWithModuleVersion(""),
			want:  devVersion,
		},
		{
			name:  "missing build info falls back to dev version",
			build: nil,
			want:  devVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolve(tt.build, tt.override).Version; got != tt.want {
				t.Errorf("Version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveVCSSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     Info
	}{
		{
			name: "clean checkout",
			settings: []debug.BuildSetting{
				{Key: "vcs", Value: "git"},
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.time", Value: "2026-10-30T12:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: Info{
				Version:    devVersion,
				Commit:     "abcdef1234567890",
				CommitTime: "2026-10-30T12:00:00Z",
				GoVersion:  testGoVersion,
			},
		},
		{
			name: "modified working tree",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: Info{
				Version:   devVersion,
				Commit:    "abcdef1234567890",
				Modified:  true,
				GoVersion: testGoVersion,
			},
		},
		{
			name:     "no vcs settings",
			settings: []debug.BuildSetting{{Key: "-trimpath", Value: "true"}},
			want:     Info{Version: devVersion, GoVersion: testGoVersion},
		},
		{
			name:     "unparsable modified flag is treated as unmodified",
			settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "maybe"}},
			want:     Info{Version: devVersion, GoVersion: testGoVersion},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			build := buildWithModuleVersion("(devel)")
			build.Settings = tt.settings

			if got := resolve(build, ""); got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInfoSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "version only",
			info: Info{Version: "1.0.0"},
			want: "1.0.0",
		},
		{
			name: "with go version",
			info: Info{Version: "1.0.0", GoVersion: testGoVersion},
			want: "1.0.0 (" + testGoVersion + ")",
		},
		{
			name: "long commit is shortened",
			info: Info{Version: "1.0.0", Commit: "abcdef1234567890", GoVersion: testGoVersion},
			want: "1.0.0 (commit abcdef123456, " + testGoVersion + ")",
		},
		{
			name: "short commit kept and modified marked",
			info: Info{Version: "1.0.0", Commit: "abc", Modified: true},
			want: "1.0.0 (commit abc, modified)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.info.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadReturnsVersionAndGoVersion(t *testing.T) {
	t.Parallel()

	info := Read()
	if info.Version == "" {
		t.Error("Read().Version is empty")
	}

	if info.GoVersion == "" {
		t.Error("Read().GoVersion is empty")
	}
}

func buildWithModuleVersion(moduleVersion string) *debug.BuildInfo {
	return &debug.BuildInfo{
		GoVersion: testGoVersion,
		Main:      debug.Module{Path: "example.com/test", Version: moduleVersion},
	}
}
