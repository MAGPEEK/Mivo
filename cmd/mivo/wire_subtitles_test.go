package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	appsubtitles "github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// hangingExtractor never returns until its context ends, like a converter that hangs.
type hangingExtractor struct {
	gotTrack ffmpeg.SubtitleTrack
	deadline bool
}

func (extractor *hangingExtractor) ExtractWebVTT(ctx context.Context, _ string, track ffmpeg.SubtitleTrack, _ string) error {
	extractor.gotTrack = track
	_, extractor.deadline = ctx.Deadline()

	<-ctx.Done()

	return ctx.Err() //nolint:wrapcheck // a fake
}

// TestSubtitleConverterBoundsAHangingConversion: a hung converter must not hold one of the
// service's two slots for ever (M4-13c review).
func TestSubtitleConverterBoundsAHangingConversion(t *testing.T) {
	t.Parallel()

	extractor := &hangingExtractor{}
	converter := subtitleConverter{extractor: extractor, timeout: 50 * time.Millisecond}

	started := time.Now()
	err := converter.ConvertWebVTT(t.Context(), "/media/x.mkv",
		appsubtitles.Track{External: true, Ordinal: 3, Codec: "subrip"}, "/cache/out.vtt")

	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 5*time.Second {
		t.Errorf("ConvertWebVTT = %v after %s, want the deadline", err, time.Since(started))
	}

	if !extractor.deadline || extractor.gotTrack != (ffmpeg.SubtitleTrack{External: true, Ordinal: 3, Codec: "subrip"}) {
		t.Errorf("extractor saw track %+v with deadline %t", extractor.gotTrack, extractor.deadline)
	}
}

func TestExternalTracksCarryTheStoredFields(t *testing.T) {
	t.Parallel()

	subtitles := []media.ExternalSubtitle{
		{RelativePath: "Movie.de.forced.srt", Format: media.SubtitleFormatSubRip, Language: "deu", Forced: true, TextBased: true},
		{RelativePath: "Movie.sup", Format: media.SubtitleFormatPGS, Language: "eng", SDH: true, Default: true, Title: "t"},
	}

	tracks := externalTracks(subtitles)

	if len(tracks) != 2 {
		t.Fatalf("tracks = %+v", tracks)
	}

	first, second := tracks[0], tracks[1]
	if first.ID != subtitles[0].TrackID() || !first.External || !first.TextBased || !first.Forced || first.Language != "deu" {
		t.Errorf("first track = %+v", first)
	}

	if second.ID != subtitles[1].TrackID() || second.TextBased || !second.SDH || !second.Default || second.Title != "t" ||
		second.Format != string(media.SubtitleFormatPGS) {
		t.Errorf("second track = %+v", second)
	}
}

func TestFileStatterReportsAMissingFileAsNotFound(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	present := filepath.Join(directory, "a.srt")

	if err := os.WriteFile(present, []byte("12345"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stat, err := fileStatter{}.Stat(t.Context(), present)
	if err != nil || stat.Size != 5 || stat.ModifiedAt.IsZero() {
		t.Errorf("Stat = %+v, %v", stat, err)
	}

	if _, err := (fileStatter{}).Stat(t.Context(), filepath.Join(directory, "missing.srt")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Stat(missing) = %v, want not found", err)
	}
}

type recordingSessions struct {
	viewerAdmin bool
	viewerFound bool
}

func (sessions *recordingSessions) List(ctx context.Context) ([]appplayback.SessionInfo, error) {
	viewer, found := appaccess.ViewerFrom(ctx)
	sessions.viewerFound, sessions.viewerAdmin = found, viewer.Admin

	return []appplayback.SessionInfo{{ID: "s"}}, nil
}

// TestAdminSessionsListsAsTheSystem: the trickplay worker has no viewer, but the session list
// needs one; the worker must see every session.
func TestAdminSessionsListsAsTheSystem(t *testing.T) {
	t.Parallel()

	sessions := &recordingSessions{}

	infos, err := adminSessions{sessions: sessions}.List(t.Context())
	if err != nil || len(infos) != 1 {
		t.Fatalf("List = %v, %v", infos, err)
	}

	if !sessions.viewerFound || !sessions.viewerAdmin {
		t.Errorf("viewer found %t admin %t, want an administrator", sessions.viewerFound, sessions.viewerAdmin)
	}
}
