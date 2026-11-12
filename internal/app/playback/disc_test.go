package playback_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	blurayFileID       = "f1000000-0000-4000-8000-000000000101"
	dvdImageFileID     = "f1000000-0000-4000-8000-000000000102"
	blurayRelativePath = "Movie (2019)/BDMV"
	blurayDiscRoot     = "Movie (2019)"
	dvdImageRelative   = "Show/Show S01E02.iso"
	discKeyframe       = 18 * time.Second
)

// discDirectories joins like joiningResolver, or fails with err.
type discDirectories struct {
	err error
}

func (directories discDirectories) ResolveDirectory(root, relativePath string) (string, error) {
	if directories.err != nil {
		return "", directories.err
	}

	return root + "/" + relativePath, nil
}

// discTitles returns a title per layout, or err, and records the disc paths it resolved.
type discTitles struct {
	mutex    sync.Mutex
	err      error
	resolved []string
}

func (titles *discTitles) Resolve(_ context.Context, discPath string, layout media.Layout) (media.DiscTitle, error) {
	titles.mutex.Lock()
	defer titles.mutex.Unlock()

	titles.resolved = append(titles.resolved, discPath)

	if titles.err != nil {
		return nil, titles.err
	}

	return titleOf(layout), nil
}

func titleOf(layout media.Layout) media.DiscTitle {
	if layout == media.LayoutISO {
		return media.DiscTitle{{Offset: 4096, Length: 8192, Duration: fileDuration}}
	}

	return media.DiscTitle{{Path: "BDMV/STREAM/00001.m2ts", Duration: fileDuration}}
}

// discKeyframes answers every disc lookup with discKeyframe and records the sources.
type discKeyframes struct {
	mutex   sync.Mutex
	sources []media.DiscSource
	roots   []string
}

func (locator *discKeyframes) DiscKeyframeAtOrBefore(_ context.Context, libraryRoot string, disc media.DiscSource, _ time.Duration) (time.Duration, error) {
	locator.mutex.Lock()
	defer locator.mutex.Unlock()

	locator.sources = append(locator.sources, disc)
	locator.roots = append(locator.roots, libraryRoot)

	return discKeyframe, nil
}

// discEnvironment adds a Blu-ray folder (H.264 and AAC: remux) and a DVD image (MPEG-2 and
// AC-3: transcode) to the catalogue and wires the disc ports.
func discEnvironment() (*environment, *discTitles, *discKeyframes) {
	env := newEnvironment()

	bluray := catalogFile(blurayFileID, blurayRelativePath, media.ContainerM2TS, h264Video(0), audio(1, "aac"))
	bluray.File.Layout = media.LayoutBDMV

	mpeg2 := h264Video(0)
	mpeg2.Codec, mpeg2.Profile = "mpeg2video", "Main"
	image := catalogFile(dvdImageFileID, dvdImageRelative, media.ContainerVOB, mpeg2, audio(1, "ac3"))
	image.File.Layout = media.LayoutISO

	env.files.files[blurayFileID] = bluray
	env.files.files[dvdImageFileID] = image

	titles := &discTitles{}
	keyframes := &discKeyframes{}
	env.discs = appplayback.DiscPorts{Directories: discDirectories{}, Titles: titles, Keyframes: keyframes}

	return env, titles, keyframes
}

func TestStartPlaysDiscsThroughHLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileID   string
		layout   media.Layout
		discRoot string
		mode     playback.Mode
	}{
		{name: "Blu-ray folder is remuxed", fileID: blurayFileID, layout: media.LayoutBDMV, discRoot: blurayDiscRoot, mode: playback.ModeRemux},
		{name: "DVD image is transcoded", fileID: dvdImageFileID, layout: media.LayoutISO, discRoot: dvdImageRelative, mode: playback.ModeTranscode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env, titles, _ := discEnvironment()
			session := mustStart(t, env.service(t), request(test.fileID))

			if session.Plan.Mode != test.mode || session.Plan.Delivery != playback.DeliveryHLS {
				t.Errorf("plan = %s/%s, want %s over HLS", session.Plan.Mode, session.Plan.Delivery, test.mode)
			}

			if len(session.Plan.Reasons) == 0 || session.Plan.Reasons[0].Code != playback.ReasonDiscSource {
				t.Errorf("reasons = %v, want disc_source first", session.Plan.Reasons)
			}

			discPath := libraryRoot + "/" + test.discRoot
			assertDiscJob(t, env.producer.jobs()[0].job, appplayback.Job{
				LibraryRoot: libraryRoot, RelativePath: test.discRoot, InputPath: discPath,
				Layout: test.layout, DiscTitle: titleOf(test.layout),
			})

			if !slices.Equal(titles.resolved, []string{discPath}) {
				t.Errorf("resolved titles of %v, want [%s]", titles.resolved, discPath)
			}
		})
	}
}

// assertDiscJob compares the input fields of a disc job.
func assertDiscJob(t *testing.T, job, want appplayback.Job) {
	t.Helper()

	if job.LibraryRoot != want.LibraryRoot || job.RelativePath != want.RelativePath || job.InputPath != want.InputPath ||
		job.Layout != want.Layout || !slices.Equal(job.DiscTitle, want.DiscTitle) {
		t.Errorf("job = %+v, want the disc input %+v", job, want)
	}
}

// TestStartSeeksCopiedDiscVideoToAKeyframeOfTheTitle: the keyframe of a remuxed disc is
// searched in the joined title, not in the disc folder (BACKLOG m3-16f).
func TestStartSeeksCopiedDiscVideoToAKeyframeOfTheTitle(t *testing.T) {
	t.Parallel()

	env, _, keyframes := discEnvironment()
	startRequest := request(blurayFileID)
	startRequest.StartPosition = 21 * time.Second

	session := mustStart(t, env.service(t), startRequest)

	if session.StartPosition != discKeyframe || env.producer.jobs()[0].job.StartPosition != discKeyframe {
		t.Errorf("StartPosition = %s, want the title keyframe %s", session.StartPosition, discKeyframe)
	}

	want := media.DiscSource{RelativePath: blurayDiscRoot, Layout: media.LayoutBDMV, Title: titleOf(media.LayoutBDMV)}
	if len(keyframes.sources) != 1 || keyframes.roots[0] != libraryRoot ||
		keyframes.sources[0].RelativePath != want.RelativePath || !slices.Equal(keyframes.sources[0].Title, want.Title) {
		t.Errorf("disc keyframe lookups = %+v in %v, want [%+v] in %s", keyframes.sources, keyframes.roots, want, libraryRoot)
	}

	if calls := env.keyframes.recordedCalls(); len(calls) != 0 {
		t.Errorf("file keyframe lookups = %v, want none", calls)
	}
}

func TestStartRefusesDiscsThatCannotPlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(env *environment, titles *discTitles)
		wantErr  error
		wantText string
	}{
		{
			name: "encrypted disc",
			prepare: func(_ *environment, titles *discTitles) {
				titles.err = fmt.Errorf("resolve: %w", media.ErrDiscEncrypted)
			},
			wantErr:  domain.ErrUnsupported,
			wantText: "disc_encrypted",
		},
		{
			name:     "disc without a title is no missing file",
			prepare:  func(_ *environment, titles *discTitles) { titles.err = fmt.Errorf("resolve: %w", media.ErrDiscNoTitle) },
			wantErr:  domain.ErrUnsupported,
			wantText: "disc_no_title",
		},
		{
			name:     "unreadable disc",
			prepare:  func(_ *environment, titles *discTitles) { titles.err = errBoom },
			wantErr:  errBoom,
			wantText: blurayDiscRoot,
		},
		{
			name: "disc folder that leaves the library",
			prepare: func(env *environment, _ *discTitles) {
				env.discs.Directories = discDirectories{err: fmt.Errorf("link: %w", domain.ErrForbidden)}
			},
			wantErr:  appplayback.ErrFileOutsideLibrary,
			wantText: blurayRelativePath,
		},
		{
			name:     "disc playback not configured",
			prepare:  func(env *environment, _ *discTitles) { env.discs = appplayback.DiscPorts{} },
			wantErr:  appplayback.ErrDiscPlaybackUnavailable,
			wantText: blurayFileID,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env, titles, _ := discEnvironment()
			test.prepare(env, titles)

			_, err := env.service(t).Start(adminContext(t), request(blurayFileID))

			assertWrappedError(t, err, test.wantErr, test.wantText)

			if errors.Is(err, domain.ErrNotFound) {
				t.Errorf("error = %v, must not read as a missing file", err)
			}

			if jobs := env.producer.jobs(); len(jobs) != 0 {
				t.Errorf("started %d productions, want none", len(jobs))
			}
		})
	}
}
