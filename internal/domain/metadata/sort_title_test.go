package metadata_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestSortTitleFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record metadata.Record
		want   string
	}{
		{
			name:   "explicit sort title is case-folded only",
			record: metadata.Record{Title: "Matrix", SortTitle: "The Matrix 1"},
			want:   "the matrix 1",
		},
		{
			name:   "explicit sort title with umlaut",
			record: metadata.Record{Title: "Anything", SortTitle: "ÜBER alles"},
			want:   "über alles",
		},
		{
			name:   "no sort title uses the library rule",
			record: metadata.Record{Title: "Über Alles"},
			want:   library.SortTitle("Über Alles"),
		},
		{
			name:   "blank sort title uses the library rule",
			record: metadata.Record{Title: "Matrix Reloaded", SortTitle: "  "},
			want:   library.SortTitle("Matrix Reloaded"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := metadata.SortTitleFor(test.record); got != test.want {
				t.Errorf("SortTitleFor(%+v) = %q, want %q", test.record, got, test.want)
			}
		})
	}
}
