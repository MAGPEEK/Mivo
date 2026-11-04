package scan_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestBrokenNFOKeepsWhatAGoodOneGave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		readErr error
	}{
		{"malformed", "garbage", nil},
		{"unreadable", "", domain.ErrForbidden},
		{"oversized", "", domain.ErrUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "m1")
			rig.sidecars.content["Heat/movie.nfo"] = "heat"
			rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "Heat NFO", Summary: "A heist."})
			request := scan.RefreshRequest{
				ItemIDs:  []string{"m1"},
				Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 1995))},
				Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1)},
			}

			rig.mustRefresh(t, request)

			rig.sidecars.content["Heat/movie.nfo"] = test.content
			rig.sidecars.errs["Heat/movie.nfo"] = test.readErr
			rig.mustRefresh(t, request)

			if record := rig.catalog.state("m1").Record; record.Title != "Heat NFO" || record.Summary != "A heist." {
				t.Errorf("record = %+v, want the data of the earlier good NFO", record)
			}
		})
	}
}

func TestUnavailableNFOFailsTheRefreshForARetry(t *testing.T) {
	t.Parallel()

	for _, readErr := range []error{domain.ErrUnavailable, domain.ErrNotFound} {
		rig := newRefreshRig(t, "m1")
		rig.sidecars.errs["Heat/movie.nfo"] = readErr

		err := rig.refresh(context.Background(), scan.RefreshRequest{
			ItemIDs:  []string{"m1"},
			Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 1995))},
			Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1)},
		})

		if !errors.Is(err, readErr) || rig.catalog.metadataSaves != 0 {
			t.Errorf("error = %v, saves = %d, want %v and no save", err, rig.catalog.metadataSaves, readErr)
		}
	}
}
