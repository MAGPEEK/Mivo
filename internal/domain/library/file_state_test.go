package library_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestChanged(t *testing.T) {
	t.Parallel()

	modified := time.Date(2026, time.October, 30, 12, 0, 0, 123_000_000, time.UTC)
	previous := library.FileState{RelativePath: "a/movie.mkv", Size: 1_000, ModifiedAt: modified}

	tests := []struct {
		name    string
		current library.FileState
		want    bool
	}{
		{name: "identical", current: previous, want: false},
		{
			name:    "sub-millisecond difference is ignored",
			current: library.FileState{RelativePath: "a/movie.mkv", Size: 1_000, ModifiedAt: modified.Add(999_999)},
			want:    false,
		},
		{
			name: "same instant in another zone",
			current: library.FileState{
				RelativePath: "a/movie.mkv", Size: 1_000,
				ModifiedAt: modified.In(time.FixedZone("CET", 3600)),
			},
			want: false,
		},
		{
			name:    "one millisecond later",
			current: library.FileState{RelativePath: "a/movie.mkv", Size: 1_000, ModifiedAt: modified.Add(time.Millisecond)},
			want:    true,
		},
		{
			name:    "earlier",
			current: library.FileState{RelativePath: "a/movie.mkv", Size: 1_000, ModifiedAt: modified.Add(-time.Second)},
			want:    true,
		},
		{
			name:    "size differs",
			current: library.FileState{RelativePath: "a/movie.mkv", Size: 1_001, ModifiedAt: modified},
			want:    true,
		},
		{
			name:    "other path",
			current: library.FileState{RelativePath: "b/movie.mkv", Size: 1_000, ModifiedAt: modified},
			want:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.Changed(previous, test.current); got != test.want {
				t.Errorf("Changed() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestChangedTruncatesInsteadOfRounding(t *testing.T) {
	t.Parallel()

	// SQLite stores Unix milliseconds (ADR 0004), so 12.0009 s is read back as 12.000 s.
	stored := time.UnixMilli(12_000)
	onDisk := time.Unix(12, 900_000)

	previous := library.FileState{RelativePath: "movie.mkv", Size: 1, ModifiedAt: stored}
	current := library.FileState{RelativePath: "movie.mkv", Size: 1, ModifiedAt: onDisk}

	if library.Changed(previous, current) {
		t.Error("Changed() = true for times within the same millisecond, want false")
	}
}
